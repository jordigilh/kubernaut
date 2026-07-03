/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ToolName is the registered name of the fetch_pod_logs tool.
const ToolName = "fetch_pod_logs"

var fetchPodLogsSchema = json.RawMessage(`{"type":"object","properties":{"pod_name":{"type":"string","description":"Name of the pod"},"namespace":{"type":"string","description":"Namespace of the pod"},"start_time":{"type":"string","description":"Start time (RFC3339 or negative seconds like -3600)"},"filter":{"type":"string","description":"Regex include filter (case-insensitive)"},"exclude_filter":{"type":"string","description":"Regex exclude filter (case-insensitive)"},"limit":{"type":"integer","description":"Max log lines to return (default 100)"}},"required":["pod_name","namespace"]}`)

type fetchArgs struct {
	PodName       string `json:"pod_name"`
	Namespace     string `json:"namespace"`
	StartTime     string `json:"start_time,omitempty"`
	Filter        string `json:"filter,omitempty"`
	ExcludeFilter string `json:"exclude_filter,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

// FetchPodLogsTool implements the Holmes SDK fetch_pod_logs functionality.
type FetchPodLogsTool struct {
	client kubernetes.Interface
}

// NewFetchPodLogsTool creates a new fetch_pod_logs tool.
func NewFetchPodLogsTool(client kubernetes.Interface) *FetchPodLogsTool {
	return &FetchPodLogsTool{client: client}
}

func (t *FetchPodLogsTool) Name() string { return ToolName }
func (t *FetchPodLogsTool) Description() string {
	return "Fetch pod logs with time-range filtering, regex include/exclude, and previous+current log merge"
}
func (t *FetchPodLogsTool) Parameters() json.RawMessage { return fetchPodLogsSchema }

func (t *FetchPodLogsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a fetchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	if a.Limit <= 0 {
		a.Limit = 100
	}

	pod, err := t.client.CoreV1().Pods(a.Namespace).Get(ctx, a.PodName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting pod %s/%s: %w", a.Namespace, a.PodName, err)
	}

	merged := fetchCurrentAndPreviousLogs(ctx, t.client, pod, a)
	merged = ApplyFilters(merged, a.Filter, a.ExcludeFilter, a.Limit)

	return formatLogsOutput(merged, a), nil
}

// fetchContainerLogs retrieves logs for every container in containers,
// prefixing lines with "[container] " when the pod has more than one
// container. Per-container fetch errors are skipped (best-effort: a pod with
// one unreachable container should not blank out the others' logs).
func fetchContainerLogs(ctx context.Context, client kubernetes.Interface, namespace, podName string, containers []corev1.Container, previous bool, startTime string) []string {
	var allLines []string
	multiContainer := len(containers) > 1
	for _, c := range containers {
		opts := &corev1.PodLogOptions{
			Container:  c.Name,
			Previous:   previous,
			Timestamps: true,
		}
		if startTime != "" {
			if st := parseTime(startTime); st != nil {
				sinceTime := metav1.NewTime(*st)
				opts.SinceTime = &sinceTime
			}
		}

		raw, fetchErr := client.CoreV1().Pods(namespace).GetLogs(podName, opts).Do(ctx).Raw()
		if fetchErr != nil {
			continue
		}

		for _, line := range strings.Split(string(raw), "\n") {
			if line == "" {
				continue
			}
			if multiContainer {
				allLines = append(allLines, fmt.Sprintf("[%s] %s", c.Name, line))
			} else {
				allLines = append(allLines, line)
			}
		}
	}
	return allLines
}

// fetchCurrentAndPreviousLogs fetches current and previous-container-instance
// logs concurrently (previous logs require a separate GetLogs call per the
// K8s API) and merges them with previous logs first, matching the original
// chronological ordering (previous instance's tail precedes the current
// instance's log stream).
func fetchCurrentAndPreviousLogs(ctx context.Context, client kubernetes.Interface, pod *corev1.Pod, a fetchArgs) []string {
	var wg sync.WaitGroup
	currentCh := make(chan []string, 1)
	previousCh := make(chan []string, 1)

	fetch := func(previous bool, ch chan<- []string) {
		defer wg.Done()
		ch <- fetchContainerLogs(ctx, client, a.Namespace, a.PodName, pod.Spec.Containers, previous, a.StartTime)
	}

	wg.Add(2)
	go fetch(false, currentCh)
	go fetch(true, previousCh)
	wg.Wait()
	close(currentCh)
	close(previousCh)

	previousLogs := <-previousCh
	currentLogs := <-currentCh
	merged := make([]string, 0, len(previousLogs)+len(currentLogs))
	merged = append(merged, previousLogs...)
	merged = append(merged, currentLogs...)
	return merged
}

// formatLogsOutput renders the merged, filtered log lines with the
// "--- fetch_pod_logs metadata ---" footer summarizing the request.
func formatLogsOutput(merged []string, a fetchArgs) string {
	var sb strings.Builder
	sb.WriteString(strings.Join(merged, "\n"))
	sb.WriteString(fmt.Sprintf("\n\n--- fetch_pod_logs metadata ---\npod: %s/%s\nlines: %d\n",
		a.Namespace, a.PodName, len(merged)))
	if a.Filter != "" {
		sb.WriteString(fmt.Sprintf("filter: %s\n", a.Filter))
	}
	if a.ExcludeFilter != "" {
		sb.WriteString(fmt.Sprintf("exclude_filter: %s\n", a.ExcludeFilter))
	}
	return sb.String()
}

// ApplyFilters applies include/exclude regex filters and a tail limit to log lines.
func ApplyFilters(lines []string, includePattern, excludePattern string, limit int) []string {
	if includePattern != "" {
		lines = applyIncludeFilter(lines, includePattern)
	}
	if excludePattern != "" {
		lines = applyExcludeFilter(lines, excludePattern)
	}
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines
}

func parseTime(s string) *time.Time {
	if s == "now" || s == "" {
		return nil
	}
	if strings.HasPrefix(s, "-") {
		var seconds int
		if _, err := fmt.Sscanf(s, "-%d", &seconds); err == nil {
			t := time.Now().Add(-time.Duration(seconds) * time.Second)
			return &t
		}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	return nil
}

func applyIncludeFilter(lines []string, pattern string) []string {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		var filtered []string
		lp := strings.ToLower(pattern)
		for _, l := range lines {
			if strings.Contains(strings.ToLower(l), lp) {
				filtered = append(filtered, l)
			}
		}
		return filtered
	}
	var filtered []string
	for _, l := range lines {
		if re.MatchString(l) {
			filtered = append(filtered, l)
		}
	}
	return filtered
}

func applyExcludeFilter(lines []string, pattern string) []string {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		var filtered []string
		lp := strings.ToLower(pattern)
		for _, l := range lines {
			if !strings.Contains(strings.ToLower(l), lp) {
				filtered = append(filtered, l)
			}
		}
		return filtered
	}
	var filtered []string
	for _, l := range lines {
		if !re.MatchString(l) {
			filtered = append(filtered, l)
		}
	}
	return filtered
}
