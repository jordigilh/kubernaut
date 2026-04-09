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

func (t *FetchPodLogsTool) Name() string               { return ToolName }
func (t *FetchPodLogsTool) Description() string         { return "Fetch pod logs with time-range filtering, regex include/exclude, and previous+current log merge" }
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

	var wg sync.WaitGroup
	currentCh := make(chan []string, 1)
	previousCh := make(chan []string, 1)

	fetchLogs := func(previous bool, ch chan<- []string) {
		defer wg.Done()
		var allLines []string
		for _, c := range pod.Spec.Containers {
			opts := &corev1.PodLogOptions{
				Container:  c.Name,
				Previous:   previous,
				Timestamps: true,
			}
			if a.StartTime != "" {
				if st := parseTime(a.StartTime); st != nil {
					sinceTime := metav1.NewTime(*st)
					opts.SinceTime = &sinceTime
				}
			}

			raw, fetchErr := t.client.CoreV1().Pods(a.Namespace).GetLogs(a.PodName, opts).Do(ctx).Raw()
			if fetchErr != nil {
				continue
			}

			multiContainer := len(pod.Spec.Containers) > 1
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
		ch <- allLines
	}

	wg.Add(2)
	go fetchLogs(false, currentCh)
	go fetchLogs(true, previousCh)
	wg.Wait()
	close(currentCh)
	close(previousCh)

	var merged []string
	merged = append(merged, <-previousCh...)
	merged = append(merged, <-currentCh...)

	merged = ApplyFilters(merged, a.Filter, a.ExcludeFilter, a.Limit)

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

	return sb.String(), nil
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
