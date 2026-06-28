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
	"k8s.io/client-go/kubernetes"
)

// NodeTool is an alias for the shared tool interface, exported for test access.
type NodeTool = tools.Tool

// NodeProxyToolNames lists the node proxy tool names for phase mapping.
var NodeProxyToolNames = []string{
	"nodes_log",
	"nodes_stats_summary",
}

// NodeProxyClientInterface abstracts the kubelet proxy API for testability.
type NodeProxyClientInterface interface {
	GetNodeLogs(ctx context.Context, node, logPath string, tailLines int) (string, error)
	GetNodeStats(ctx context.Context, node string) (string, error)
}

var (
	nodesLogParams = json.RawMessage(`{
		"type": "object",
		"properties": {
			"node":       {"type": "string", "description": "Node name to retrieve logs from"},
			"path":       {"type": "string", "description": "Log file path relative to /var/log (e.g. kubelet.log, kube-proxy.log)"},
			"tail_lines": {"type": "integer", "description": "Number of lines from the end to return (optional, default: all)"}
		},
		"required": ["node", "path"]
	}`)
	nodesStatsSummaryParams = json.RawMessage(`{
		"type": "object",
		"properties": {
			"node": {"type": "string", "description": "Node name to retrieve stats summary from"}
		},
		"required": ["node"]
	}`)
)

// NewNodeProxyTools creates the nodes_log and nodes_stats_summary tools.
func NewNodeProxyTools(npc NodeProxyClientInterface, sizeLimit int) []tools.Tool {
	if sizeLimit <= 0 {
		sizeLimit = 30000
	}
	return []tools.Tool{
		&nodesLogTool{npc: npc, sizeLimit: sizeLimit},
		&nodesStatsSummaryTool{npc: npc, sizeLimit: sizeLimit},
	}
}

type nodesLogTool struct {
	npc       NodeProxyClientInterface
	sizeLimit int
}

func (t *nodesLogTool) Name() string               { return "nodes_log" }
func (t *nodesLogTool) Description() string         { return "Retrieve node-level logs (kubelet, kube-proxy) via kubelet proxy API" }
func (t *nodesLogTool) Parameters() json.RawMessage { return nodesLogParams }

func (t *nodesLogTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Node      string `json:"node"`
		Path      string `json:"path"`
		TailLines int    `json:"tail_lines"`
	}

	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("parsing args: %w", err)
		}
	}

	if a.Node == "" {
		return "", fmt.Errorf("node is required")
	}
	if a.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.Contains(a.Path, "..") || strings.HasPrefix(a.Path, "/") {
		return "", fmt.Errorf("invalid path: path must be relative and must not contain '..'")
	}

	result, err := t.npc.GetNodeLogs(ctx, a.Node, a.Path, a.TailLines)
	if err != nil {
		return "", fmt.Errorf("fetching node logs: %w", err)
	}

	return truncateNodeOutput(result, t.sizeLimit), nil
}

type nodesStatsSummaryTool struct {
	npc       NodeProxyClientInterface
	sizeLimit int
}

func (t *nodesStatsSummaryTool) Name() string               { return "nodes_stats_summary" }
func (t *nodesStatsSummaryTool) Description() string         { return "Retrieve detailed node resource stats (CPU, memory, filesystem, network) via kubelet Summary API" }
func (t *nodesStatsSummaryTool) Parameters() json.RawMessage { return nodesStatsSummaryParams }

func (t *nodesStatsSummaryTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Node string `json:"node"`
	}

	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("parsing args: %w", err)
		}
	}

	if a.Node == "" {
		return "", fmt.Errorf("node is required")
	}

	result, err := t.npc.GetNodeStats(ctx, a.Node)
	if err != nil {
		return "", fmt.Errorf("fetching node stats: %w", err)
	}

	return truncateNodeOutput(result, t.sizeLimit), nil
}

func truncateNodeOutput(text string, sizeLimit int) string {
	if len(text) <= sizeLimit {
		return text
	}
	return text[:sizeLimit] + "\n... [TRUNCATED] Response exceeded limit. Use tail_lines parameter or request specific log paths to reduce output."
}

// realNodeProxyClient implements NodeProxyClientInterface using the K8s clientset.
type realNodeProxyClient struct {
	clientset kubernetes.Interface
}

// NewNodeProxyClient wraps a real K8s clientset for kubelet proxy access.
func NewNodeProxyClient(clientset kubernetes.Interface) NodeProxyClientInterface {
	return &realNodeProxyClient{clientset: clientset}
}

func (c *realNodeProxyClient) GetNodeLogs(ctx context.Context, node, logPath string, tailLines int) (string, error) {
	req := c.clientset.CoreV1().RESTClient().Get().
		AbsPath("/api/v1/nodes", node, "proxy", "logs", logPath)

	if tailLines > 0 {
		req = req.Param("tailLines", fmt.Sprintf("%d", tailLines))
	}

	result, err := req.DoRaw(ctx)
	if err != nil {
		return "", fmt.Errorf("kubelet proxy logs request: %w", err)
	}
	return string(result), nil
}

func (c *realNodeProxyClient) GetNodeStats(ctx context.Context, node string) (string, error) {
	result, err := c.clientset.CoreV1().RESTClient().Get().
		AbsPath("/api/v1/nodes", node, "proxy", "stats", "summary").
		DoRaw(ctx)
	if err != nil {
		return "", fmt.Errorf("kubelet proxy stats request: %w", err)
	}
	return string(result), nil
}
