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
	"strings"

	"github.com/itchyny/gojq"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

// AllToolNames lists the 19 K8s tool names matching HAPI v1.2 surface.
var AllToolNames = []string{
	"kubectl_describe",
	"kubectl_get_by_name",
	"kubectl_get_by_kind_in_namespace",
	"kubectl_get_by_kind_in_cluster",
	"kubectl_find_resource",
	"kubectl_get_yaml",
	"kubectl_events",
	"kubectl_logs",
	"kubectl_previous_logs",
	"kubectl_logs_all_containers",
	"kubectl_container_logs",
	"kubectl_container_previous_logs",
	"kubectl_previous_logs_all_containers",
	"kubectl_logs_grep",
	"kubectl_logs_all_containers_grep",
	"kubectl_get_memory_requests",
	"kubectl_get_deployment_memory_requests",
	"kubernetes_jq_query",
	"kubernetes_count",
}

// NewAllTools creates all 19 K8s tools backed by the given client.
func NewAllTools(client kubernetes.Interface) []tools.Tool {
	return []tools.Tool{
		newDescribe(client),
		newGetByName(client),
		newGetByKindInNamespace(client),
		newGetByKindInCluster(client),
		newFindResource(client),
		newGetYAML(client),
		newEvents(client),
		newLogs(client, false, false),
		newPreviousLogs(client),
		newLogsAllContainers(client),
		newContainerLogs(client),
		newContainerPreviousLogs(client),
		newPreviousLogsAllContainers(client),
		newLogsGrep(client),
		newLogsAllContainersGrep(client),
		newGetMemoryRequests(client),
		newGetDeploymentMemoryRequests(client),
		newJQQuery(client),
		newCount(client),
	}
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
	clusterListParams = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string","description":"Kubernetes resource kind"}},"required":["kind"]}`)
	findParams        = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"namespace":{"type":"string"},"label_selector":{"type":"string","description":"Label selector (e.g. app=nginx)"}},"required":["kind","namespace","label_selector"]}`)
	logParams         = json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"namespace":{"type":"string"},"container":{"type":"string"},"tailLines":{"type":"integer"},"limitBytes":{"type":"integer"},"pattern":{"type":"string"}},"required":["name","namespace"]}`)
	memParams         = json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Pod name"},"namespace":{"type":"string"}},"required":["name","namespace"]}`)
	depMemParams      = json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Deployment name"},"namespace":{"type":"string"}},"required":["name","namespace"]}`)
	jqParams          = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"namespace":{"type":"string"},"jq_expression":{"type":"string","description":"jq expression to apply to the resource list"}},"required":["kind","namespace","jq_expression"]}`)
	countParams       = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"namespace":{"type":"string"},"jq_filter":{"type":"string","description":"Optional jq filter to apply before counting"}},"required":["kind","namespace"]}`)
)

// resourceTool is a shared base for tools that get/describe a single K8s resource.
type resourceTool struct {
	client    kubernetes.Interface
	toolName  string
	desc      string
	params    json.RawMessage
	fetchFunc func(ctx context.Context, client kubernetes.Interface, a resourceArgs) (interface{}, error)
}

func (t *resourceTool) Name() string               { return t.toolName }
func (t *resourceTool) Description() string         { return t.desc }
func (t *resourceTool) Parameters() json.RawMessage { return t.params }

func (t *resourceTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a resourceArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}
	obj, err := t.fetchFunc(ctx, t.client, a)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(obj)
	return string(data), nil
}

func newDescribe(c kubernetes.Interface) *resourceTool {
	return &resourceTool{
		client: c, toolName: "kubectl_describe",
		desc: "Describe a Kubernetes resource as structured JSON", params: objParams,
		fetchFunc: func(ctx context.Context, cl kubernetes.Interface, a resourceArgs) (interface{}, error) {
			return getResource(ctx, cl, a.Kind, a.Name, a.Namespace)
		},
	}
}

func newGetByName(c kubernetes.Interface) *resourceTool {
	return &resourceTool{
		client: c, toolName: "kubectl_get_by_name",
		desc: "Get a Kubernetes resource by name as JSON", params: objParams,
		fetchFunc: func(ctx context.Context, cl kubernetes.Interface, a resourceArgs) (interface{}, error) {
			return getResource(ctx, cl, a.Kind, a.Name, a.Namespace)
		},
	}
}

func newGetByKindInNamespace(c kubernetes.Interface) *resourceTool {
	return &resourceTool{
		client: c, toolName: "kubectl_get_by_kind_in_namespace",
		desc: "List Kubernetes resources of a kind in a namespace", params: listParams,
		fetchFunc: func(ctx context.Context, cl kubernetes.Interface, a resourceArgs) (interface{}, error) {
			return listResources(ctx, cl, a.Kind, a.Namespace)
		},
	}
}

func newEvents(c kubernetes.Interface) *resourceTool {
	return &resourceTool{
		client: c, toolName: "kubectl_events",
		desc: "Get events for a Kubernetes resource", params: objParams,
		fetchFunc: func(ctx context.Context, cl kubernetes.Interface, a resourceArgs) (interface{}, error) {
			events, err := cl.CoreV1().Events(a.Namespace).List(ctx, metav1.ListOptions{
				FieldSelector: fmt.Sprintf("involvedObject.name=%s", a.Name),
			})
			if err != nil {
				return nil, fmt.Errorf("listing events: %w", err)
			}
			return events.Items, nil
		},
	}
}

func newGetByKindInCluster(c kubernetes.Interface) *resourceTool {
	return &resourceTool{
		client: c, toolName: "kubectl_get_by_kind_in_cluster",
		desc: "List Kubernetes resources of a kind across all namespaces", params: clusterListParams,
		fetchFunc: func(ctx context.Context, cl kubernetes.Interface, a resourceArgs) (interface{}, error) {
			return listResources(ctx, cl, a.Kind, "")
		},
	}
}

func newFindResource(c kubernetes.Interface) tools.Tool {
	return &findResourceTool{client: c}
}

type findResourceTool struct {
	client kubernetes.Interface
}

func (t *findResourceTool) Name() string               { return "kubectl_find_resource" }
func (t *findResourceTool) Description() string         { return "Find Kubernetes resources by label selector" }
func (t *findResourceTool) Parameters() json.RawMessage { return findParams }

func (t *findResourceTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Kind          string `json:"kind"`
		Namespace     string `json:"namespace"`
		LabelSelector string `json:"label_selector"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}
	result, err := listResourcesWithSelector(ctx, t.client, a.Kind, a.Namespace, a.LabelSelector)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

func newGetYAML(c kubernetes.Interface) tools.Tool {
	return &getYAMLTool{client: c}
}

type getYAMLTool struct {
	client kubernetes.Interface
}

func (t *getYAMLTool) Name() string               { return "kubectl_get_yaml" }
func (t *getYAMLTool) Description() string         { return "Get a Kubernetes resource as YAML" }
func (t *getYAMLTool) Parameters() json.RawMessage { return objParams }

func (t *getYAMLTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a resourceArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}
	obj, err := getResource(ctx, t.client, a.Kind, a.Name, a.Namespace)
	if err != nil {
		return "", err
	}
	jsonData, _ := json.Marshal(obj)
	yamlData, err := yaml.JSONToYAML(jsonData)
	if err != nil {
		return "", fmt.Errorf("converting to YAML: %w", err)
	}
	return string(yamlData), nil
}

func newGetMemoryRequests(c kubernetes.Interface) tools.Tool {
	return &memoryRequestsTool{client: c, toolName: "kubectl_get_memory_requests", desc: "Get memory requests and limits for a pod's containers", params: memParams}
}

func newGetDeploymentMemoryRequests(c kubernetes.Interface) tools.Tool {
	return &memoryRequestsTool{client: c, toolName: "kubectl_get_deployment_memory_requests", desc: "Get memory requests and limits for all pods in a deployment", params: depMemParams, deployment: true}
}

type memoryRequestsTool struct {
	client     kubernetes.Interface
	toolName   string
	desc       string
	params     json.RawMessage
	deployment bool
}

func (t *memoryRequestsTool) Name() string               { return t.toolName }
func (t *memoryRequestsTool) Description() string         { return t.desc }
func (t *memoryRequestsTool) Parameters() json.RawMessage { return t.params }

func (t *memoryRequestsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	if t.deployment {
		return t.deploymentMemory(ctx, a.Name, a.Namespace)
	}
	return t.podMemory(ctx, a.Name, a.Namespace)
}

func (t *memoryRequestsTool) podMemory(ctx context.Context, name, namespace string) (string, error) {
	pod, err := t.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting pod: %w", err)
	}
	return containerMemoryJSON(pod.Spec.Containers), nil
}

func (t *memoryRequestsTool) deploymentMemory(ctx context.Context, name, namespace string) (string, error) {
	dep, err := t.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting deployment: %w", err)
	}
	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return "", fmt.Errorf("building selector: %w", err)
	}
	pods, err := t.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return "", fmt.Errorf("listing pods: %w", err)
	}
	type podMem struct {
		Pod        string                    `json:"pod"`
		Containers []containerMemoryEntry    `json:"containers"`
	}
	var result []podMem
	for i := range pods.Items {
		p := pods.Items[i]
		var entries []containerMemoryEntry
		for _, c := range p.Spec.Containers {
			entries = append(entries, containerMemoryEntry{
				Container:     c.Name,
				MemoryRequest: c.Resources.Requests.Memory().String(),
				MemoryLimit:   c.Resources.Limits.Memory().String(),
			})
		}
		result = append(result, podMem{Pod: p.Name, Containers: entries})
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

type containerMemoryEntry struct {
	Container     string `json:"container"`
	MemoryRequest string `json:"memory_request"`
	MemoryLimit   string `json:"memory_limit"`
}

func containerMemoryJSON(containers []corev1.Container) string {
	var entries []containerMemoryEntry
	for _, c := range containers {
		entries = append(entries, containerMemoryEntry{
			Container:     c.Name,
			MemoryRequest: c.Resources.Requests.Memory().String(),
			MemoryLimit:   c.Resources.Limits.Memory().String(),
		})
	}
	data, _ := json.Marshal(entries)
	return string(data)
}

func newJQQuery(c kubernetes.Interface) tools.Tool {
	return &jqTool{client: c, toolName: "kubernetes_jq_query",
		desc: "Apply a jq expression to a Kubernetes resource list", params: jqParams}
}

func newCount(c kubernetes.Interface) tools.Tool {
	return &jqTool{client: c, toolName: "kubernetes_count",
		desc: "Count Kubernetes resources matching kind in namespace, optionally filtered by jq", params: countParams, countMode: true}
}

type jqTool struct {
	client    kubernetes.Interface
	toolName  string
	desc      string
	params    json.RawMessage
	countMode bool
}

func (t *jqTool) Name() string               { return t.toolName }
func (t *jqTool) Description() string         { return t.desc }
func (t *jqTool) Parameters() json.RawMessage { return t.params }

func (t *jqTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Kind         string `json:"kind"`
		Namespace    string `json:"namespace"`
		JQExpression string `json:"jq_expression"`
		JQFilter     string `json:"jq_filter"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	list, err := listResources(ctx, t.client, a.Kind, a.Namespace)
	if err != nil {
		return "", err
	}

	jsonData, _ := json.Marshal(list)
	var input interface{}
	_ = json.Unmarshal(jsonData, &input)

	if t.countMode {
		expr := a.JQFilter
		if expr == "" {
			expr = ".items | length"
		} else {
			expr = fmt.Sprintf("[.items[] | select(%s)] | length", expr)
		}
		return runJQ(expr, input)
	}

	if a.JQExpression == "" {
		return string(jsonData), nil
	}
	return runJQ(a.JQExpression, input)
}

func runJQ(expression string, input interface{}) (string, error) {
	query, err := gojq.Parse(expression)
	if err != nil {
		return "", fmt.Errorf("parsing jq expression %q: %w", expression, err)
	}
	iter := query.Run(input)

	var results []interface{}
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return "", fmt.Errorf("jq execution: %w", err)
		}
		results = append(results, v)
	}

	if len(results) == 1 {
		data, _ := json.Marshal(results[0])
		return string(data), nil
	}
	data, _ := json.Marshal(results)
	return string(data), nil
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
	return &logTool{client: c, toolName: "kubectl_logs_all_containers_grep", desc: "Get grep-filtered logs from all containers", allConts: true, grep: true}
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

// --- resource helpers ---

func getResource(ctx context.Context, client kubernetes.Interface, kind, name, namespace string) (interface{}, error) {
	gvr := kindToGVR(kind)
	switch gvr.Resource {
	case "pods":
		return client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	case "deployments":
		return client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	case "replicasets":
		return client.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
	case "statefulsets":
		return client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	case "daemonsets":
		return client.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	case "services":
		return client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	case "configmaps":
		return client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	case "secrets":
		return client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	case "events":
		return client.CoreV1().Events(namespace).Get(ctx, name, metav1.GetOptions{})
	case "namespaces":
		return client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	case "nodes":
		return client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	case "jobs":
		return client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
	case "cronjobs":
		return client.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
	case "poddisruptionbudgets":
		return client.PolicyV1().PodDisruptionBudgets(namespace).Get(ctx, name, metav1.GetOptions{})
	case "horizontalpodautoscalers":
		return client.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})
	case "networkpolicies":
		return client.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
	default:
		return nil, fmt.Errorf("unsupported kind: %s", kind)
	}
}

func listResources(ctx context.Context, client kubernetes.Interface, kind, namespace string) (interface{}, error) {
	return listResourcesWithSelector(ctx, client, kind, namespace, "")
}

func listResourcesWithSelector(ctx context.Context, client kubernetes.Interface, kind, namespace, labelSelector string) (interface{}, error) {
	opts := metav1.ListOptions{LabelSelector: labelSelector}
	gvr := kindToGVR(kind)
	switch gvr.Resource {
	case "pods":
		return client.CoreV1().Pods(namespace).List(ctx, opts)
	case "deployments":
		return client.AppsV1().Deployments(namespace).List(ctx, opts)
	case "replicasets":
		return client.AppsV1().ReplicaSets(namespace).List(ctx, opts)
	case "statefulsets":
		return client.AppsV1().StatefulSets(namespace).List(ctx, opts)
	case "daemonsets":
		return client.AppsV1().DaemonSets(namespace).List(ctx, opts)
	case "services":
		return client.CoreV1().Services(namespace).List(ctx, opts)
	case "configmaps":
		return client.CoreV1().ConfigMaps(namespace).List(ctx, opts)
	case "secrets":
		return client.CoreV1().Secrets(namespace).List(ctx, opts)
	case "events":
		return client.CoreV1().Events(namespace).List(ctx, opts)
	case "namespaces":
		return client.CoreV1().Namespaces().List(ctx, opts)
	case "nodes":
		return client.CoreV1().Nodes().List(ctx, opts)
	case "jobs":
		return client.BatchV1().Jobs(namespace).List(ctx, opts)
	case "cronjobs":
		return client.BatchV1().CronJobs(namespace).List(ctx, opts)
	case "poddisruptionbudgets":
		return client.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, opts)
	case "horizontalpodautoscalers":
		return client.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, opts)
	case "networkpolicies":
		return client.NetworkingV1().NetworkPolicies(namespace).List(ctx, opts)
	default:
		return nil, fmt.Errorf("unsupported kind for list: %s", kind)
	}
}

func kindToGVR(kind string) schema.GroupVersionResource {
	switch strings.ToLower(kind) {
	case "pod":
		return schema.GroupVersionResource{Resource: "pods"}
	case "deployment":
		return schema.GroupVersionResource{Group: "apps", Resource: "deployments"}
	case "replicaset":
		return schema.GroupVersionResource{Group: "apps", Resource: "replicasets"}
	case "statefulset":
		return schema.GroupVersionResource{Group: "apps", Resource: "statefulsets"}
	case "daemonset":
		return schema.GroupVersionResource{Group: "apps", Resource: "daemonsets"}
	case "service":
		return schema.GroupVersionResource{Resource: "services"}
	case "configmap":
		return schema.GroupVersionResource{Resource: "configmaps"}
	case "secret":
		return schema.GroupVersionResource{Resource: "secrets"}
	case "event":
		return schema.GroupVersionResource{Resource: "events"}
	case "namespace":
		return schema.GroupVersionResource{Resource: "namespaces"}
	case "node":
		return schema.GroupVersionResource{Resource: "nodes"}
	case "job":
		return schema.GroupVersionResource{Group: "batch", Resource: "jobs"}
	case "cronjob":
		return schema.GroupVersionResource{Group: "batch", Resource: "cronjobs"}
	case "poddisruptionbudget", "pdb":
		return schema.GroupVersionResource{Group: "policy", Resource: "poddisruptionbudgets"}
	case "horizontalpodautoscaler", "hpa":
		return schema.GroupVersionResource{Group: "autoscaling", Resource: "horizontalpodautoscalers"}
	case "networkpolicy":
		return schema.GroupVersionResource{Group: "networking.k8s.io", Resource: "networkpolicies"}
	default:
		return schema.GroupVersionResource{Resource: strings.ToLower(kind) + "s"}
	}
}
