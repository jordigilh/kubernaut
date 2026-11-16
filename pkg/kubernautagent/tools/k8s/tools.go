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
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	sigsyaml "sigs.k8s.io/yaml"
)

// AllToolNames lists the baseline K8s tool names.
var AllToolNames = []string{
	"kubectl_describe",
	"kubectl_get_by_name",
	"kubectl_get_by_name_in_cluster",
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
	"api_resources_list",
	"resources_list",
}

// NewAllTools creates the baseline K8s tools. The resolver handles generic
// get/list operations via the dynamic client; the typed client is used for
// tools that need subresource access (logs) or typed field access (memory, events).
func NewAllTools(client kubernetes.Interface, resolver ResourceResolver) []tools.Tool {
	jqTools := newJQTools(resolver)

	result := make([]tools.Tool, 0, 20+len(jqTools))
	result = append(result,
		newAPIResourcesList(client),
		newResourcesList(resolver),
		newDescribe(resolver),
		newGetByName(resolver),
		newGetByNameInCluster(resolver),
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
	)
	result = append(result, jqTools...)
	return result
}

// apiResourceInfo is the bounded, model-facing projection of one discovered
// resource. It mirrors the kube-mcp-server api_resources_list contract while
// keeping local discovery independent from the REST mapper used by generic
// get/list tools.
type apiResourceInfo struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespaced bool   `json:"namespaced"`
}

type apiResourcesListResult struct {
	Resources []apiResourceInfo `json:"resources"`
	Warning   string            `json:"warning,omitempty"`
}

type apiResourcesListTool struct {
	discovery discovery.DiscoveryInterface
}

func newAPIResourcesList(client kubernetes.Interface) *apiResourcesListTool {
	var clientDiscovery discovery.DiscoveryInterface
	if client != nil {
		clientDiscovery = client.Discovery()
	}
	return &apiResourcesListTool{discovery: clientDiscovery}
}

func (t *apiResourcesListTool) Name() string { return "api_resources_list" }
func (t *apiResourcesListTool) Description() string {
	return "Look up the Kubernetes API resource type(s) matching a specific kind using the target cluster's preferred API discovery"
}
func (t *apiResourcesListTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string","minLength":1,"description":"Required case-insensitive exact Kubernetes Kind filter"}},"required":["kind"]}`)
}

func (t *apiResourcesListTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("parsing api resource discovery args: %w", err)
	}
	input.Kind = strings.TrimSpace(input.Kind)
	if input.Kind == "" {
		return "", fmt.Errorf("api resource discovery kind must not be empty")
	}
	if t.discovery == nil {
		return "", fmt.Errorf("api resource discovery unavailable: discovery client not configured")
	}

	resourceLists, discoveryErr := discovery.ServerPreferredResourcesWithContext(ctx, discovery.ToDiscoveryInterfaceWithContext(t.discovery))
	result := apiResourcesListResult{Resources: make([]apiResourceInfo, 0)}
	for _, resourceList := range resourceLists {
		if resourceList == nil {
			continue
		}
		for _, resource := range resourceList.APIResources {
			if !strings.EqualFold(resource.Kind, input.Kind) {
				continue
			}
			result.Resources = append(result.Resources, apiResourceInfo{
				APIVersion: resourceList.GroupVersion,
				Kind:       resource.Kind,
				Name:       resource.Name,
				Namespaced: resource.Namespaced,
			})
		}
	}
	sort.Slice(result.Resources, func(i, j int) bool {
		if result.Resources[i].APIVersion != result.Resources[j].APIVersion {
			return result.Resources[i].APIVersion < result.Resources[j].APIVersion
		}
		return result.Resources[i].Name < result.Resources[j].Name
	})
	if discoveryErr != nil {
		if len(resourceLists) == 0 {
			return "", fmt.Errorf("failed to list API resources: %w", discoveryErr)
		}
		result.Warning = fmt.Sprintf("some API groups could not be queried and are omitted from this list: %v", discoveryErr)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshaling API resource discovery: %w", err)
	}
	return string(data), nil
}

type resourcesListTool struct {
	resolver ExactResourceListResolver
}

func newResourcesList(resolver ResourceResolver) *resourcesListTool {
	exactResolver, _ := resolver.(ExactResourceListResolver)
	return &resourcesListTool{resolver: exactResolver}
}

func (t *resourcesListTool) Name() string { return "resources_list" }
func (t *resourcesListTool) Description() string {
	return "List resources from the target cluster using an exact apiVersion and kind; an empty result still proves the resource endpoint is served"
}
func (t *resourcesListTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"apiVersion":{"type":"string"},"kind":{"type":"string"},"namespace":{"type":"string"},"labelSelector":{"type":"string"},"fieldSelector":{"type":"string"}},"required":["apiVersion","kind"]}`)
}

func (t *resourcesListTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		APIVersion    string `json:"apiVersion"`
		Kind          string `json:"kind"`
		Namespace     string `json:"namespace"`
		LabelSelector string `json:"labelSelector"`
		FieldSelector string `json:"fieldSelector"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("parsing resources_list args: %w", err)
	}
	if strings.TrimSpace(input.APIVersion) == "" || strings.TrimSpace(input.Kind) == "" {
		return "", fmt.Errorf("resources_list requires apiVersion and kind")
	}
	if t.resolver == nil {
		return "", fmt.Errorf("resources_list unavailable: exact resource resolver not configured")
	}
	result, err := t.resolver.ListForAPIVersion(ctx, input.APIVersion, input.Kind, input.Namespace, input.LabelSelector, input.FieldSelector)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshaling resources_list result: %w", err)
	}
	return string(data), nil
}

const (
	// DefaultLogTailLines is the default number of log lines to tail when the
	// LLM omits both tailLines and limitBytes. Prevents unbounded log fetches.
	DefaultLogTailLines int64 = 500

	// DefaultEventLimit caps the number of events returned by kubectl_events
	// to prevent oversized output from high-churn resources.
	DefaultEventLimit = 200
)

type resourceArgs struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	APIGroup  string `json:"api_group,omitempty"`
}

type logArgs struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Container  string `json:"container"`
	TailLines  *int64 `json:"tailLines,omitempty"`
	LimitBytes *int64 `json:"limitBytes,omitempty"`
	Pattern    string `json:"pattern,omitempty"`
}

const apiGroupDesc = `API group for disambiguation when a kind exists in multiple groups (e.g. empty string for core, apps for Deployments, operators.coreos.com for OLM). Omit for unambiguous kinds.`

var (
	objParams         = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"name":{"type":"string"},"namespace":{"type":"string"},"api_group":{"type":"string","description":"` + apiGroupDesc + `"}},"required":["kind","name","namespace"]}`)
	clusterObjParams  = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string","description":"Kubernetes resource kind"},"name":{"type":"string","description":"Exact resource name to find across all namespaces"},"api_group":{"type":"string","description":"` + apiGroupDesc + `"}},"required":["kind","name"]}`)
	listParams        = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"namespace":{"type":"string"},"api_group":{"type":"string","description":"` + apiGroupDesc + `"}},"required":["kind","namespace"]}`)
	clusterListParams = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"api_group":{"type":"string","description":"` + apiGroupDesc + `"}},"required":["kind"]}`)
	findParams        = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"keyword":{"type":"string","description":"Substring to match in resource name, namespace, or labels"},"api_group":{"type":"string","description":"` + apiGroupDesc + `"}},"required":["kind","keyword"]}`)
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

func (t *resourceTool) Name() string                { return t.toolName }
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
	data, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("marshaling resource: %w", err)
	}
	return string(data), nil
}

func newDescribe(resolver ResourceResolver) *resourceTool {
	return &resourceTool{
		toolName: "kubectl_describe",
		desc:     "Describe a Kubernetes resource as structured JSON", params: objParams,
		fetchFunc: func(ctx context.Context, a resourceArgs) (interface{}, error) {
			return resolver.Get(ctx, a.Kind, a.Name, a.Namespace, a.APIGroup)
		},
	}
}

func newGetByName(resolver ResourceResolver) *resourceTool {
	return &resourceTool{
		toolName: "kubectl_get_by_name",
		desc:     "Get a Kubernetes resource by name as JSON", params: objParams,
		fetchFunc: func(ctx context.Context, a resourceArgs) (interface{}, error) {
			return resolver.Get(ctx, a.Kind, a.Name, a.Namespace, a.APIGroup)
		},
	}
}

func newGetByKindInNamespace(resolver ResourceResolver) *resourceTool {
	return &resourceTool{
		toolName: "kubectl_get_by_kind_in_namespace",
		desc:     "List Kubernetes resources of a kind in a namespace", params: listParams,
		fetchFunc: func(ctx context.Context, a resourceArgs) (interface{}, error) {
			return resolver.List(ctx, a.Kind, a.Namespace, a.APIGroup)
		},
	}
}

func newGetByKindInCluster(resolver ResourceResolver) *resourceTool {
	return &resourceTool{
		toolName: "kubectl_get_by_kind_in_cluster",
		desc:     "List Kubernetes resources of a kind across all namespaces", params: clusterListParams,
		fetchFunc: func(ctx context.Context, a resourceArgs) (interface{}, error) {
			return resolver.List(ctx, a.Kind, "", a.APIGroup)
		},
	}
}

// --- get by name in cluster tool (#752) ---

type getByNameInClusterTool struct {
	resolver ResourceResolver
}

func newGetByNameInCluster(resolver ResourceResolver) *getByNameInClusterTool {
	return &getByNameInClusterTool{resolver: resolver}
}

func (t *getByNameInClusterTool) Name() string { return "kubectl_get_by_name_in_cluster" }
func (t *getByNameInClusterTool) Description() string {
	return "Get a single Kubernetes resource by exact name across all namespaces (avoids listing all resources of a kind)"
}
func (t *getByNameInClusterTool) Parameters() json.RawMessage { return clusterObjParams }

// Execute performs one exact-name cluster lookup and serializes its targeted
// full-object result.
func (t *getByNameInClusterTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Kind     string `json:"kind"`
		Name     string `json:"name"`
		APIGroup string `json:"api_group,omitempty"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	resource, err := t.resolver.GetByNameInCluster(ctx, a.Kind, a.Name, a.APIGroup)
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(resource)
	if err != nil {
		return "", fmt.Errorf("marshaling resource: %w", err)
	}
	if len(data) > maxJQOutputChars {
		return "", fmt.Errorf("serialized resource exceeds %d-byte output limit", maxJQOutputChars)
	}
	return string(data), nil
}

// --- find resource tool ---

type findResourceTool struct {
	resolver ResourceResolver
}

func newFindResource(resolver ResourceResolver) *findResourceTool {
	return &findResourceTool{resolver: resolver}
}

func (t *findResourceTool) Name() string { return "kubectl_find_resource" }
func (t *findResourceTool) Description() string {
	return "Find resources by keyword over retained metadata fields across all namespaces; the first 100 items are searched and results may be truncated"
}
func (t *findResourceTool) Parameters() json.RawMessage { return findParams }

// Execute searches only the retained metadata projection and reports when the
// bounded source page may hide additional matches.
func (t *findResourceTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Kind     string `json:"kind"`
		Keyword  string `json:"keyword"`
		APIGroup string `json:"api_group,omitempty"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	a.Keyword = strings.TrimSpace(a.Keyword)
	if a.Keyword == "" {
		return "", fmt.Errorf("keyword must not be empty; use kubectl_get_by_kind_in_cluster to list all resources of a kind")
	}

	resources, err := t.resolver.List(ctx, a.Kind, "", a.APIGroup)
	if err != nil {
		return "", err
	}
	summary, isSummary := resources.(*ResourceListSummary)

	data, err := json.Marshal(resources)
	if err != nil {
		return "", fmt.Errorf("marshaling resources: %w", err)
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
		return findResourceNoMatch(summary, isSummary), nil
	}
	result, err := json.Marshal(matched)
	if err != nil {
		return "", fmt.Errorf("marshaling matched resources: %w", err)
	}
	return findResourceMatches(string(result), summary, isSummary), nil
}

// findResourceMatches appends a bounded-source warning when matched results
// come from a truncated metadata page.
func findResourceMatches(result string, summary *ResourceListSummary, isSummary bool) string {
	if !isSummary || summary == nil || !summary.Truncated {
		return result
	}
	if summary.TotalCount != nil {
		return fmt.Sprintf("%s\nmatches found in the first %d of %d resources; results are truncated; narrow the query", result, summary.ItemCount, *summary.TotalCount)
	}
	return fmt.Sprintf("%s\nmatches found in the first %d resources; results are truncated; narrow the query", result, summary.ItemCount)
}

// findResourceNoMatch distinguishes a true empty result from a truncated page.
func findResourceNoMatch(summary *ResourceListSummary, isSummary bool) string {
	if !isSummary || summary == nil || !summary.Truncated {
		return "[]"
	}
	if summary.TotalCount != nil {
		return fmt.Sprintf("no match in the first %d of %d resources; results are truncated; narrow the query", summary.ItemCount, *summary.TotalCount)
	}
	return fmt.Sprintf("no match in the first %d resources; results are truncated; narrow the query", summary.ItemCount)
}

// --- get yaml tool ---

type getYAMLTool struct {
	resolver ResourceResolver
}

func newGetYAML(resolver ResourceResolver) *getYAMLTool {
	return &getYAMLTool{resolver: resolver}
}

func (t *getYAMLTool) Name() string                { return "kubectl_get_yaml" }
func (t *getYAMLTool) Description() string         { return "Get a Kubernetes resource as YAML output" }
func (t *getYAMLTool) Parameters() json.RawMessage { return objParams }

func (t *getYAMLTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a resourceArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}
	obj, err := t.resolver.Get(ctx, a.Kind, a.Name, a.Namespace, a.APIGroup)
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
	client   kubernetes.Interface
	toolName string
	desc     string
	params   json.RawMessage
	allNS    bool
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

func (t *memoryRequestsTool) Name() string                { return t.toolName }
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

type eventsTool struct {
	client kubernetes.Interface
}

func newEvents(c kubernetes.Interface) *eventsTool {
	return &eventsTool{client: c}
}

func (t *eventsTool) Name() string                { return "kubectl_events" }
func (t *eventsTool) Description() string         { return "Get events for a Kubernetes resource" }
func (t *eventsTool) Parameters() json.RawMessage { return objParams }

func (t *eventsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a resourceArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	events, err := t.client.CoreV1().Events(a.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", a.Name),
	})
	if err != nil {
		return "", fmt.Errorf("listing events: %w", err)
	}

	items := events.Items
	truncated := len(items) > DefaultEventLimit
	if truncated {
		items = items[:DefaultEventLimit]
	}

	data, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("marshaling events: %w", err)
	}

	if truncated {
		return string(data) + fmt.Sprintf("\n... [TRUNCATED] Showing %d of %d events. Use kubectl_describe or narrow your query for more detail.", DefaultEventLimit, len(events.Items)), nil
	}
	return string(data), nil
}

// --- log tools ---

type logTool struct {
	client   kubernetes.Interface
	toolName string
	desc     string
	previous bool
	allConts bool
	grep     bool
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

func (t *logTool) Name() string                { return t.toolName }
func (t *logTool) Description() string         { return t.desc }
func (t *logTool) Parameters() json.RawMessage { return logParams }

func (t *logTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a logArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	if a.TailLines == nil && a.LimitBytes == nil {
		defaultTail := DefaultLogTailLines
		a.TailLines = &defaultTail
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
