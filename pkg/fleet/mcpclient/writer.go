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
	session         *mcp.ClientSession
	sessionProvider SessionProvider
	reconnect       func(context.Context) error
	clusterID       string
	toolPrefix      string
	scheme          *runtime.Scheme
}

// NewWriterFromSession creates a WriterClient from an existing MCP session.
// Options (optional): WithToolPrefix sets the gateway-specific tool prefix;
// WithScheme sets the GVK-inference scheme (see ensureGVK), defaulting to
// clientgoscheme.Scheme.
// Panics if session is nil (fail-fast, same contract as NewFromSession).
func NewWriterFromSession(session *mcp.ClientSession, clusterID string, opts ...Option) *WriterClient {
	if session == nil {
		panic("mcpclient.NewWriterFromSession: session must not be nil")
	}
	cfg := &clientConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return &WriterClient{session: session, clusterID: clusterID, toolPrefix: cfg.toolPrefix, scheme: cfg.resolvedScheme()}
}

// NewWriterFromSessionProvider creates a WriterClient that lazily resolves
// the MCP session on each call via the provided SessionProvider, mirroring
// NewFromSessionProvider's reader-side pattern (issue #2317). Pass
// WithReconnect(rc.Reconnect) so a dead session (e.g. an SSE stream failure
// mid-lifetime) is actively repaired instead of every write call failing
// forever until the pod restarts -- SessionProvider alone only re-reads
// whatever session is currently stored, it never repairs a broken one.
func NewWriterFromSessionProvider(provider SessionProvider, clusterID string, opts ...Option) *WriterClient {
	if provider == nil {
		panic("mcpclient.NewWriterFromSessionProvider: provider must not be nil")
	}
	cfg := &clientConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return &WriterClient{
		sessionProvider: provider, reconnect: cfg.reconnect, clusterID: clusterID,
		toolPrefix: cfg.toolPrefix, scheme: cfg.resolvedScheme(),
	}
}

// currentSession returns the active MCP session, resolving lazily via
// sessionProvider on each call when set (mirrors Client.currentSession).
func (w *WriterClient) currentSession() *mcp.ClientSession {
	if w.sessionProvider != nil {
		return w.sessionProvider()
	}
	return w.session
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

// callTool invokes the named MCP tool against the current session, retrying
// once via the reconnect callback (if set) when the call fails with a
// retryable session error. Mirrors Client.callTool exactly (issue #2317);
// kept as a WriterClient-local copy rather than a shared helper because the
// two types' CallTool signatures and zero-value returns differ ((*mcp.CallToolResult, error) vs bespoke per-method returns).
func (w *WriterClient) callTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	session := w.currentSession()
	if session == nil {
		return nil, fmt.Errorf("call %s: no active MCP session", toolName)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: args})
	if err == nil {
		return result, nil
	}
	if w.reconnect == nil || !isRetryableSessionError(err) {
		return nil, fmt.Errorf("call %s: %w", toolName, err)
	}

	if reconnErr := w.reconnect(ctx); reconnErr != nil {
		return nil, fmt.Errorf("call %s: reconnect failed: %w (original: %w)", toolName, reconnErr, err)
	}

	session = w.currentSession()
	if session == nil {
		return nil, fmt.Errorf("call %s: no active MCP session after reconnect", toolName)
	}
	return session.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: args})
}

// Create implements client.Writer. It serializes the object to JSON and sends
// it to the remote cluster via the MCP resources_create_or_update tool.
//
// The tool argument MUST be named "resource" per the upstream K8s MCP Server
// contract (github.com/containers/kubernetes-mcp-server); the tool call fails
// with "missing argument resource" otherwise.
func (w *WriterClient) Create(ctx context.Context, obj client.Object, _ ...client.CreateOption) error {
	return w.createOrUpdate(ctx, obj, "Create")
}

// Delete implements client.Writer. It sends a delete request to the remote
// cluster via the MCP delete_resource tool.
func (w *WriterClient) Delete(ctx context.Context, obj client.Object, _ ...client.DeleteOption) error {
	gvk, err := ensureGVK(obj, w.scheme)
	if err != nil {
		return err
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

	result, err := w.callTool(ctx, toolName, args)
	if err != nil {
		return err
	}
	if result.IsError {
		errText := ExtractText(result)
		// Issue #2349: a NotFound must arrive typed so an idempotent delete
		// (client.IgnoreNotFound) of an already-gone remote resource -- e.g.
		// JobExecutor.Cleanup, pkg/workflowexecution/executor/job.go -- is
		// actually recognized as such instead of hard-failing.
		if nf := asRemoteNotFound(errText, gvk, obj.GetName()); nf != nil {
			return nf
		}
		return fmt.Errorf("call %s returned error: %s", toolName, errText)
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
	return w.createOrUpdate(ctx, obj, "Update")
}

// createOrUpdate implements the shared Create/Update logic: serialize obj,
// call the resources_create_or_update MCP tool, and best-effort populate the
// object's metadata from the response. Issue #1530 (dupl): Create and Update
// were byte-identical apart from the "Create"/"Update" wording in the
// serialization error message (opName below).
func (w *WriterClient) createOrUpdate(ctx context.Context, obj client.Object, opName string) error {
	if _, err := ensureGVK(obj, w.scheme); err != nil {
		return err
	}

	manifest, err := objectToJSON(obj)
	if err != nil {
		return fmt.Errorf("serialize object for %s: %w", opName, err)
	}

	toolName := w.resolveToolName(ToolCreateOrUpdate)
	result, err := w.callTool(ctx, toolName, map[string]any{
		"resource": manifest,
	})
	if err != nil {
		return err
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

// Close is a no-op for WriterClient since it shares the session with its
// parent (or, when session-provider-backed, the session lifecycle is owned
// by the provider's owner, typically ResilientClient).
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
