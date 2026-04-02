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

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

// AllToolNames lists the baseline K8s tool names.
var AllToolNames = []string{
	"kubectl_describe",
	"kubectl_get_by_name",
	"kubectl_get_by_kind_in_namespace",
	"kubectl_events",
	"kubectl_logs",
	"kubectl_previous_logs",
	"kubectl_logs_all_containers",
	"kubectl_container_logs",
	"kubectl_container_previous_logs",
	"kubectl_previous_logs_all_containers",
	"kubectl_logs_grep",
}

// NewAllTools creates the baseline K8s tools backed by the given client.
func NewAllTools(client kubernetes.Interface) []tools.Tool {
	return []tools.Tool{
		newDescribe(client),
		newGetByName(client),
		newGetByKindInNamespace(client),
		newEvents(client),
		newLogs(client, false, false),
		newPreviousLogs(client),
		newLogsAllContainers(client),
		newContainerLogs(client),
		newContainerPreviousLogs(client),
		newPreviousLogsAllContainers(client),
		newLogsGrep(client),
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
	objParams  = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"name":{"type":"string"},"namespace":{"type":"string"}},"required":["kind","name","namespace"]}`)
	listParams = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"namespace":{"type":"string"}},"required":["kind","namespace"]}`)
	logParams  = json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"namespace":{"type":"string"},"container":{"type":"string"},"tailLines":{"type":"integer"},"limitBytes":{"type":"integer"},"pattern":{"type":"string"}},"required":["name","namespace"]}`)
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
	default:
		return nil, fmt.Errorf("unsupported kind: %s", kind)
	}
}

func listResources(ctx context.Context, client kubernetes.Interface, kind, namespace string) (interface{}, error) {
	opts := metav1.ListOptions{}
	gvr := kindToGVR(kind)
	switch gvr.Resource {
	case "pods":
		return client.CoreV1().Pods(namespace).List(ctx, opts)
	case "deployments":
		return client.AppsV1().Deployments(namespace).List(ctx, opts)
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
	default:
		return schema.GroupVersionResource{Resource: strings.ToLower(kind) + "s"}
	}
}
