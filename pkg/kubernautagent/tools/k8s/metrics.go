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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

// MetricsToolNames lists the metrics tool names (separate from AllToolNames since
// they require a MetricsClientInterface rather than kubernetes.Interface).
var MetricsToolNames = []string{
	"kubectl_top_pods",
	"kubectl_top_nodes",
}

var (
	topPodsParams  = json.RawMessage(`{"type":"object","properties":{"namespace":{"type":"string","description":"Namespace to query (empty for all namespaces)"}},"required":[]}`)
	topNodesParams = json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
)

// MetricsClientInterface abstracts the metrics API for testability.
type MetricsClientInterface interface {
	ListPodMetrics(ctx context.Context, namespace string) (*metricsv1beta1.PodMetricsList, error)
	ListNodeMetrics(ctx context.Context) (*metricsv1beta1.NodeMetricsList, error)
}

type realMetricsClient struct {
	client metricsclient.Interface
}

func (c *realMetricsClient) ListPodMetrics(ctx context.Context, namespace string) (*metricsv1beta1.PodMetricsList, error) {
	return c.client.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
}

func (c *realMetricsClient) ListNodeMetrics(ctx context.Context) (*metricsv1beta1.NodeMetricsList, error) {
	return c.client.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
}

// NewMetricsClient wraps a real k8s.io/metrics client.
func NewMetricsClient(client metricsclient.Interface) MetricsClientInterface {
	return &realMetricsClient{client: client}
}

// NewMetricsTools creates the kubectl_top_pods and kubectl_top_nodes tools.
func NewMetricsTools(mc MetricsClientInterface) []tools.Tool {
	return []tools.Tool{
		&topPodsTool{mc: mc},
		&topNodesTool{mc: mc},
	}
}

type topPodsTool struct {
	mc MetricsClientInterface
}

func (t *topPodsTool) Name() string               { return "kubectl_top_pods" }
func (t *topPodsTool) Description() string         { return "Display resource usage (CPU/memory) for pods" }
func (t *topPodsTool) Parameters() json.RawMessage { return topPodsParams }

func (t *topPodsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Namespace string `json:"namespace"`
	}
	if args != nil {
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("parsing args: %w", err)
		}
	}

	podMetrics, err := t.mc.ListPodMetrics(ctx, a.Namespace)
	if err != nil {
		return "", fmt.Errorf("listing pod metrics: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("NAMESPACE\tNAME\tCPU\tMEMORY\n")
	for _, pm := range podMetrics.Items {
		var cpuTotal, memTotal int64
		for _, c := range pm.Containers {
			cpuTotal += c.Usage.Cpu().MilliValue()
			memTotal += c.Usage.Memory().Value()
		}
		fmt.Fprintf(&sb, "%s\t%s\t%dm\t%dMi\n", pm.Namespace, pm.Name, cpuTotal, memTotal/(1024*1024))
	}
	return sb.String(), nil
}

type topNodesTool struct {
	mc MetricsClientInterface
}

func (t *topNodesTool) Name() string               { return "kubectl_top_nodes" }
func (t *topNodesTool) Description() string         { return "Display resource usage (CPU/memory) for nodes" }
func (t *topNodesTool) Parameters() json.RawMessage { return topNodesParams }

func (t *topNodesTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	nodeMetrics, err := t.mc.ListNodeMetrics(ctx)
	if err != nil {
		return "", fmt.Errorf("listing node metrics: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("NAME\tCPU\tMEMORY\n")
	for _, nm := range nodeMetrics.Items {
		cpu := nm.Usage.Cpu().MilliValue()
		mem := nm.Usage.Memory().Value()
		fmt.Fprintf(&sb, "%s\t%dm\t%dMi\n", nm.Name, cpu, mem/(1024*1024))
	}
	return sb.String(), nil
}
