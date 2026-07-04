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

package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Compile-time interface compliance.
var _ ResourceWriter = (*WriterClient)(nil)

// WriterClient provides K8s-compatible write access to resources on a remote
// cluster via the MCP Gateway. It is the write counterpart to Client (which
// implements client.Reader). The two types are intentionally separate so that
// read-only consumers (SP, FMC) never gain write access.
//
// The target cluster is fixed at construction time via clusterID.
// When toolPrefix is set, tool names use the gateway-specific prefix;
// otherwise the EAIGW "{clusterID}__{tool}" convention is applied.
type WriterClient struct {
	session    *mcp.ClientSession
	clusterID  string
	toolPrefix string
}

// NewWriterFromSession creates a WriterClient from an existing MCP session.
// Options (optional): WithToolPrefix sets the gateway-specific tool prefix.
// Panics if session is nil (fail-fast, same contract as NewFromSession).
func NewWriterFromSession(session *mcp.ClientSession, clusterID string, opts ...Option) *WriterClient {
	if session == nil {
		panic("mcpclient.NewWriterFromSession: session must not be nil")
	}
	cfg := &clientConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return &WriterClient{session: session, clusterID: clusterID, toolPrefix: cfg.toolPrefix}
}

func (w *WriterClient) resolveToolName(tool string) string {
	if w.toolPrefix != "" {
		return ClusterToolWithPrefix(w.toolPrefix, tool)
	}
	if w.clusterID == "" {
		return tool
	}
	return ClusterTool(w.clusterID, tool)
}

// Create implements client.Writer. It serializes the object to JSON and sends
// it to the remote cluster via the MCP resources_create_or_update tool.
//
// The tool argument MUST be named "resource" per the upstream K8s MCP Server
// contract (github.com/containers/kubernetes-mcp-server); the tool call fails
// with "missing argument resource" otherwise.
func (w *WriterClient) Create(ctx context.Context, obj client.Object, _ ...client.CreateOption) error {
	manifest, err := objectToJSON(obj)
	if err != nil {
		return fmt.Errorf("serialize object for Create: %w", err)
	}

	toolName := w.resolveToolName(ToolCreateOrUpdate)
	result, err := w.session.CallTool(ctx, &mcp.CallToolParams{
		Name: toolName,
		Arguments: map[string]any{
			"resource": manifest,
		},
	})
	if err != nil {
		return fmt.Errorf("call %s: %w", toolName, err)
	}
	if result.IsError {
		return fmt.Errorf("call %s returned error: %s", toolName, ExtractText(result))
	}

	text := ExtractText(result)
	if text == "" {
		return nil
	}

	return populateFromResponse(text, obj)
}

// Delete implements client.Writer. It sends a delete request to the remote
// cluster via the MCP delete_resource tool.
func (w *WriterClient) Delete(ctx context.Context, obj client.Object, _ ...client.DeleteOption) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" {
		return fmt.Errorf("object GVK Kind must be set before calling Delete")
	}

	toolName := w.resolveToolName(ToolDelete)
	args := map[string]any{
		"kind":       gvk.Kind,
		"apiVersion": gvk.GroupVersion().String(),
		"name":       obj.GetName(),
	}
	if ns := obj.GetNamespace(); ns != "" {
		args["namespace"] = ns
	}

	result, err := w.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		return fmt.Errorf("call %s: %w", toolName, err)
	}
	if result.IsError {
		return fmt.Errorf("call %s returned error: %s", toolName, ExtractText(result))
	}
	return nil
}

// Update implements client.Writer. It serializes the object and sends it to
// the remote cluster via the MCP resources_create_or_update tool.
//
// The tool argument MUST be named "resource" per the upstream K8s MCP Server
// contract (github.com/containers/kubernetes-mcp-server); the tool call fails
// with "missing argument resource" otherwise.
func (w *WriterClient) Update(ctx context.Context, obj client.Object, _ ...client.UpdateOption) error {
	manifest, err := objectToJSON(obj)
	if err != nil {
		return fmt.Errorf("serialize object for Update: %w", err)
	}

	toolName := w.resolveToolName(ToolCreateOrUpdate)
	result, err := w.session.CallTool(ctx, &mcp.CallToolParams{
		Name: toolName,
		Arguments: map[string]any{
			"resource": manifest,
		},
	})
	if err != nil {
		return fmt.Errorf("call %s: %w", toolName, err)
	}
	if result.IsError {
		return fmt.Errorf("call %s returned error: %s", toolName, ExtractText(result))
	}

	text := ExtractText(result)
	if text == "" {
		return nil
	}

	return populateFromResponse(text, obj)
}

// Close is a no-op for WriterClient since it shares the session with its parent.
func (w *WriterClient) Close() error {
	return nil
}

// objectToJSON converts a client.Object to a JSON string suitable for the MCP
// create_resource/update_resource manifest argument.
func objectToJSON(obj client.Object) (string, error) {
	switch t := obj.(type) {
	case *unstructured.Unstructured:
		data, err := json.Marshal(t.Object)
		if err != nil {
			return "", fmt.Errorf("marshal unstructured object: %w", err)
		}
		return string(data), nil
	default:
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return "", fmt.Errorf("convert typed object to unstructured: %w", err)
		}
		data, err := json.Marshal(u)
		if err != nil {
			return "", fmt.Errorf("marshal converted object: %w", err)
		}
		return string(data), nil
	}
}

// populateFromResponse attempts to update the object's metadata from the MCP
// response (e.g., server-assigned UID, resourceVersion). Best-effort: errors
// are silently ignored since the Create/Update succeeded on the remote cluster.
func populateFromResponse(text string, obj client.Object) error {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		var response map[string]interface{}
		if err := json.Unmarshal([]byte(text), &response); err == nil {
			u.Object = response
		}
		return nil
	}
	return nil
}
