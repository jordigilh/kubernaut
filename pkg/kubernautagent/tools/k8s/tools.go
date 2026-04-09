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

package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	sigsyaml "sigs.k8s.io/yaml"
)

// AllToolNames lists the baseline K8s tool names.
var AllToolNames = []string{
	"kubectl_describe",
	"kubectl_get_by_name",
	"kubectl_get_by_kind_in_namespace",
	"kubectl_get_by_kind_in_cluster",
	"kubectl_find_resource",
	"kubectl_get_yaml",
	"kubectl_memory_requests_all_namespaces",
	"kubectl_memory_requests_namespace",
	"kubectl_events",
	"kubectl_logs",
	"kubectl_previous_logs",
	"kubectl_logs_all_containers",
	"kubectl_container_logs",
	"kubectl_container_previous_logs",
	"kubectl_previous_logs_all_containers",
	"kubectl_logs_grep",
	"kubectl_logs_all_containers_grep",
	"kubernetes_jq_query",
	"kubernetes_count",
}

// NewAllTools creates the baseline K8s tools. The resolver handles generic
// get/list operations via the dynamic client; the typed client is used for
// tools that need subresource access (logs) or typed field access (memory, events).
func NewAllTools(client kubernetes.Interface, resolver ResourceResolver) []tools.Tool {
	result := []tools.Tool{
		newDescribe(resolver),
		newGetByName(resolver),
		newGetByKindInNamespace(resolver),
		newGetByKindInCluster(resolver),
		newFindResource(resolver),
		newGetYAML(resolver),
		newMemoryRequestsAllNamespaces(client),
		newMemoryRequestsNamespace(client),
		newEvents(client),
		newLogs(client, false, false),
		newPreviousLogs(client),
		newLogsAllContainers(client),
		newContainerLogs(client),
		newContainerPreviousLogs(client),
		newPreviousLogsAllContainers(client),
		newLogsGrep(client),
		newLogsAllContainersGrep(client),
	}
	result = append(result, newJQTools(resolver)...)
	return result
}

type resourceArgs struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type logArgs struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Container  string `json:"container"`
	TailLines  *int64 `json:"tailLines,omitempty"`
	LimitBytes *int64 `json:"limitBytes,omitempty"`
	Pattern    string `json:"pattern,omitempty"`
}

var (
	objParams         = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"name":{"type":"string"},"namespace":{"type":"string"}},"required":["kind","name","namespace"]}`)
	listParams        = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"namespace":{"type":"string"}},"required":["kind","namespace"]}`)
	clusterListParams = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"}},"required":["kind"]}`)
	findParams        = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"keyword":{"type":"string","description":"Substring to match in resource name, namespace, or labels"}},"required":["kind","keyword"]}`)
	nsOnlyParams      = json.RawMessage(`{"type":"object","properties":{"namespace":{"type":"string"}},"required":["namespace"]}`)
	noParams          = json.RawMessage(`{"type":"object","properties":{}}`)
	logParams         = json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"namespace":{"type":"string"},"container":{"type":"string"},"tailLines":{"type":"integer"},"limitBytes":{"type":"integer"},"pattern":{"type":"string"}},"required":["name","namespace"]}`)
)

// resourceTool is a shared base for tools that fetch K8s resources.
// The fetchFunc closure captures whatever client dependency it needs
// (ResourceResolver for generic ops, kubernetes.Interface for typed ops).
type resourceTool struct {
	toolName  string
	desc      string
	params    json.RawMessage
	fetchFunc func(ctx context.Context, a resourceArgs) (interface{}, error)
}

func (t *resourceTool) Name() string               { return t.toolName }
func (t *resourceTool) Description() string         { return t.desc }
func (t *resourceTool) Parameters() json.RawMessage { return t.params }

func (t *resourceTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a resourceArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}
	obj, err := t.fetchFunc(ctx, a)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(obj)
	return string(data), nil
}

func newDescribe(resolver ResourceResolver) *resourceTool {
	return &resourceTool{
		toolName: "kubectl_describe",
		desc:     "Describe a Kubernetes resource as structured JSON", params: objParams,
		fetchFunc: func(ctx context.Context, a resourceArgs) (interface{}, error) {
			return resolver.Get(ctx, a.Kind, a.Name, a.Namespace)
		},
	}
}

func newGetByName(resolver ResourceResolver) *resourceTool {
	return &resourceTool{
		toolName: "kubectl_get_by_name",
		desc:     "Get a Kubernetes resource by name as JSON", params: objParams,
		fetchFunc: func(ctx context.Context, a resourceArgs) (interface{}, error) {
			return resolver.Get(ctx, a.Kind, a.Name, a.Namespace)
		},
	}
}

func newGetByKindInNamespace(resolver ResourceResolver) *resourceTool {
	return &resourceTool{
		toolName: "kubectl_get_by_kind_in_namespace",
		desc:     "List Kubernetes resources of a kind in a namespace", params: listParams,
		fetchFunc: func(ctx context.Context, a resourceArgs) (interface{}, error) {
			return resolver.List(ctx, a.Kind, a.Namespace)
		},
	}
}

func newGetByKindInCluster(resolver ResourceResolver) *resourceTool {
	return &resourceTool{
		toolName: "kubectl_get_by_kind_in_cluster",
		desc:     "List Kubernetes resources of a kind across all namespaces", params: clusterListParams,
		fetchFunc: func(ctx context.Context, a resourceArgs) (interface{}, error) {
			return resolver.List(ctx, a.Kind, "")
		},
	}
}

// --- find resource tool ---

type findResourceTool struct {
	resolver ResourceResolver
}

func newFindResource(resolver ResourceResolver) *findResourceTool {
	return &findResourceTool{resolver: resolver}
}

func (t *findResourceTool) Name() string               { return "kubectl_find_resource" }
func (t *findResourceTool) Description() string         { return "Find resources by kind with keyword substring filter across all namespaces" }
func (t *findResourceTool) Parameters() json.RawMessage { return findParams }

func (t *findResourceTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Kind    string `json:"kind"`
		Keyword string `json:"keyword"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	resources, err := t.resolver.List(ctx, a.Kind, "")
	if err != nil {
		return "", err
	}

	data, _ := json.Marshal(resources)
	if a.Keyword == "" {
		return string(data), nil
	}

	var listObj struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(data, &listObj); err != nil || listObj.Items == nil {
		if strings.Contains(strings.ToLower(string(data)), strings.ToLower(a.Keyword)) {
			return string(data), nil
		}
		return "[]", nil
	}

	lowerKeyword := strings.ToLower(a.Keyword)
	var matched []json.RawMessage
	for _, item := range listObj.Items {
		if strings.Contains(strings.ToLower(string(item)), lowerKeyword) {
			matched = append(matched, item)
		}
	}

	if len(matched) == 0 {
		return "[]", nil
	}
	result, _ := json.Marshal(matched)
	return string(result), nil
}

// --- get yaml tool ---

type getYAMLTool struct {
	resolver ResourceResolver
}

func newGetYAML(resolver ResourceResolver) *getYAMLTool {
	return &getYAMLTool{resolver: resolver}
}

func (t *getYAMLTool) Name() string               { return "kubectl_get_yaml" }
func (t *getYAMLTool) Description() string         { return "Get a Kubernetes resource as YAML output" }
func (t *getYAMLTool) Parameters() json.RawMessage { return objParams }

func (t *getYAMLTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a resourceArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}
	obj, err := t.resolver.Get(ctx, a.Kind, a.Name, a.Namespace)
	if err != nil {
		return "", err
	}
	jsonData, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("marshaling resource: %w", err)
	}
	yamlData, err := sigsyaml.JSONToYAML(jsonData)
	if err != nil {
		return "", fmt.Errorf("converting to YAML: %w", err)
	}
	return string(yamlData), nil
}

// --- memory request tools ---

type memoryRequestsTool struct {
	client    kubernetes.Interface
	toolName  string
	desc      string
	params    json.RawMessage
	allNS     bool
}

func newMemoryRequestsAllNamespaces(c kubernetes.Interface) *memoryRequestsTool {
	return &memoryRequestsTool{
		client: c, toolName: "kubectl_memory_requests_all_namespaces",
		desc:   "Fetch memory requests for all pods across all namespaces in MiB",
		params: noParams, allNS: true,
	}
}

func newMemoryRequestsNamespace(c kubernetes.Interface) *memoryRequestsTool {
	return &memoryRequestsTool{
		client: c, toolName: "kubectl_memory_requests_namespace",
		desc:   "Fetch memory requests for all pods in a namespace in MiB",
		params: nsOnlyParams, allNS: false,
	}
}

func (t *memoryRequestsTool) Name() string               { return t.toolName }
func (t *memoryRequestsTool) Description() string         { return t.desc }
func (t *memoryRequestsTool) Parameters() json.RawMessage { return t.params }

func (t *memoryRequestsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Namespace string `json:"namespace"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	ns := a.Namespace
	if t.allNS {
		ns = ""
	}

	pods, err := t.client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("listing pods: %w", err)
	}

	type podMemory struct {
		namespace string
		name      string
		mib       float64
	}

	var entries []podMemory
	for _, pod := range pods.Items {
		var totalBytes float64
		for _, c := range pod.Spec.Containers {
			if memReq, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				totalBytes += float64(memReq.Value())
			}
		}
		entries = append(entries, podMemory{
			namespace: pod.Namespace,
			name:      pod.Name,
			mib:       totalBytes / (1024 * 1024),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].mib > entries[j].mib
	})

	var sb strings.Builder
	sb.WriteString("NAMESPACE\tNAME\tMEMORY_REQUEST\n")
	for _, e := range entries {
		fmt.Fprintf(&sb, "%s\t%s\t%.0f Mi\n", e.namespace, e.name, e.mib)
	}
	return sb.String(), nil
}

func newEvents(c kubernetes.Interface) *resourceTool {
	return &resourceTool{
		toolName: "kubectl_events",
		desc:     "Get events for a Kubernetes resource", params: objParams,
		fetchFunc: func(ctx context.Context, a resourceArgs) (interface{}, error) {
			events, err := c.CoreV1().Events(a.Namespace).List(ctx, metav1.ListOptions{
				FieldSelector: fmt.Sprintf("involvedObject.name=%s", a.Name),
			})
			if err != nil {
				return nil, fmt.Errorf("listing events: %w", err)
			}
			return events.Items, nil
		},
	}
}

// --- log tools ---

type logTool struct {
	client    kubernetes.Interface
	toolName  string
	desc      string
	previous  bool
	allConts  bool
	grep      bool
}

func newLogs(c kubernetes.Interface, previous, allConts bool) *logTool {
	return &logTool{client: c, toolName: "kubectl_logs", desc: "Get container logs", previous: previous, allConts: allConts}
}
func newPreviousLogs(c kubernetes.Interface) *logTool {
	return &logTool{client: c, toolName: "kubectl_previous_logs", desc: "Get previous container logs", previous: true}
}
func newLogsAllContainers(c kubernetes.Interface) *logTool {
	return &logTool{client: c, toolName: "kubectl_logs_all_containers", desc: "Get logs from all containers", allConts: true}
}
func newContainerLogs(c kubernetes.Interface) *logTool {
	return &logTool{client: c, toolName: "kubectl_container_logs", desc: "Get logs for a specific container"}
}
func newContainerPreviousLogs(c kubernetes.Interface) *logTool {
	return &logTool{client: c, toolName: "kubectl_container_previous_logs", desc: "Get previous logs for a specific container", previous: true}
}
func newPreviousLogsAllContainers(c kubernetes.Interface) *logTool {
	return &logTool{client: c, toolName: "kubectl_previous_logs_all_containers", desc: "Get previous logs from all containers", previous: true, allConts: true}
}
func newLogsGrep(c kubernetes.Interface) *logTool {
	return &logTool{client: c, toolName: "kubectl_logs_grep", desc: "Get logs filtered by grep pattern", grep: true}
}
func newLogsAllContainersGrep(c kubernetes.Interface) *logTool {
	return &logTool{client: c, toolName: "kubectl_logs_all_containers_grep", desc: "Get logs from all containers filtered by grep pattern", allConts: true, grep: true}
}

func (t *logTool) Name() string               { return t.toolName }
func (t *logTool) Description() string         { return t.desc }
func (t *logTool) Parameters() json.RawMessage { return logParams }

func (t *logTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a logArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	opts := &corev1.PodLogOptions{
		Previous: t.previous,
	}
	if a.Container != "" {
		opts.Container = a.Container
	}
	if a.TailLines != nil {
		opts.TailLines = a.TailLines
	}
	if a.LimitBytes != nil {
		opts.LimitBytes = a.LimitBytes
	}

	if t.allConts {
		return t.logsAllContainers(ctx, a)
	}

	req := t.client.CoreV1().Pods(a.Namespace).GetLogs(a.Name, opts)
	result := req.Do(ctx)
	raw, err := result.Raw()
	if err != nil {
		return fmt.Sprintf("(no logs available: %s)", err.Error()), nil
	}

	output := string(raw)
	if t.grep && a.Pattern != "" {
		output = grepLines(output, a.Pattern)
	}
	return output, nil
}

func (t *logTool) logsAllContainers(ctx context.Context, a logArgs) (string, error) {
	pod, err := t.client.CoreV1().Pods(a.Namespace).Get(ctx, a.Name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting pod: %w", err)
	}

	var parts []string
	for _, c := range pod.Spec.Containers {
		opts := &corev1.PodLogOptions{Container: c.Name, Previous: t.previous}
		if a.TailLines != nil {
			opts.TailLines = a.TailLines
		}
		if a.LimitBytes != nil {
			opts.LimitBytes = a.LimitBytes
		}
		req := t.client.CoreV1().Pods(a.Namespace).GetLogs(a.Name, opts)
		raw, err := req.Do(ctx).Raw()
		if err != nil {
			parts = append(parts, fmt.Sprintf("[%s] (no logs: %s)", c.Name, err.Error()))
			continue
		}
		output := string(raw)
		if t.grep && a.Pattern != "" {
			output = grepLines(output, a.Pattern)
		}
		if output != "" {
			parts = append(parts, fmt.Sprintf("[%s] %s", c.Name, output))
		}
	}
	return strings.Join(parts, "\n"), nil
}

func grepLines(text, pattern string) string {
	var matched []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, pattern) {
			matched = append(matched, line)
		}
	}
	return strings.Join(matched, "\n")
}

