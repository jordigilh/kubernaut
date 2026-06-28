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

// Package mcpclient provides a shared MCP resource client for accessing
// Kubernetes resources on remote clusters via the MCP Gateway.
// All services that need remote cluster access import this package.
//
// Routing pattern: only used when ClusterID is non-empty (remote cluster).
// Local cluster operations continue using existing direct K8s API paths.
package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Compile-time interface compliance.
var _ ResourceClient = (*Client)(nil)
var _ client.Reader = (*Client)(nil)

// Client provides K8s-compatible read access to resources on a remote cluster
// via the MCP Gateway. The target cluster is fixed at construction time via
// WithClusterID and injected into each MCP tool call as a name prefix.
// When WithToolPrefix is set, tool names use the gateway-specific prefix;
// otherwise the EAIGW "{clusterID}__{tool}" convention is applied.
type Client struct {
	session    *mcp.ClientSession
	clusterID  string
	toolPrefix string
	mu         sync.Mutex
	closed     bool
}

// New creates a Client connected to the given MCP Gateway endpoint.
// The connection is established immediately; returns error if unreachable.
// Use WithClusterID to bind the client to a specific remote cluster.
func New(ctx context.Context, endpoint string, opts ...Option) (*Client, error) {
	cfg := &clientConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	mcpClient := mcp.NewClient(
		&mcp.Implementation{Name: "kubernaut-fleet-client", Version: "v0.1.0"},
		nil,
	)

	transport := &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: cfg.httpClient,
	}

	session, err := mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to MCP Gateway at %s: %w", endpoint, err)
	}

	return &Client{session: session, clusterID: cfg.clusterID, toolPrefix: cfg.toolPrefix}, nil
}

// NewFromSession creates a Client from an existing MCP session, skipping the
// connection handshake. This is used by the FMC service to create per-cluster
// readers from the shared ResilientClient's session, avoiding duplicate
// connections to the MCP Gateway.
//
// Options (optional): WithToolPrefix sets the gateway-specific tool prefix.
//
// Panics if session is nil, since a nil session invariably leads to nil-pointer
// panics on the first tool call -- failing fast makes the root cause obvious.
func NewFromSession(session *mcp.ClientSession, clusterID string, opts ...Option) *Client {
	if session == nil {
		panic("mcpclient.NewFromSession: session must not be nil")
	}
	cfg := &clientConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return &Client{session: session, clusterID: clusterID, toolPrefix: cfg.toolPrefix}
}

// Get implements client.Reader. It retrieves a single Kubernetes resource from
// the bound remote cluster via MCP Gateway and populates obj in place.
//
// The GVK must be set on the object before calling (same contract as controller-runtime).
// The apiVersion is derived from the GVK and sent as a mandatory parameter to the
// K8s MCP Server.
//
// Supported object types:
//   - *unstructured.Unstructured: full object populated
//   - *metav1.PartialObjectMetadata: metadata fields populated (ownerReferences, labels, etc.)
//   - Typed objects (e.g. *corev1.Pod): populated via JSON round-trip
func (c *Client) Get(ctx context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" {
		return fmt.Errorf("object GVK Kind must be set before calling Get")
	}

	fetched, err := c.getResource(ctx, gvk.Kind, gvk.GroupVersion().String(), key.Namespace, key.Name)
	if err != nil {
		return err
	}

	return populateObject(fetched, obj)
}

// List implements client.Reader. It retrieves Kubernetes resources of a given
// kind from the bound remote cluster via MCP Gateway.
//
// The GVK must be set on the list object before calling. The Kind typically has
// a "List" suffix (e.g. "PodList"); the suffix is stripped to derive the item
// kind for the MCP tool call. The apiVersion is derived from the GVK.
//
// Supported list types:
//   - *unstructured.UnstructuredList: items populated directly
//   - Typed ObjectLists (e.g. *corev1.PodList): populated via JSON round-trip
//
// Supported ListOptions: InNamespace, MatchingLabels. Other options are ignored.
func (c *Client) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	listOpts := client.ListOptions{}
	for _, o := range opts {
		o.ApplyToList(&listOpts)
	}

	gvk := list.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" {
		return fmt.Errorf("list object GVK Kind must be set before calling List")
	}
	itemKind := strings.TrimSuffix(gvk.Kind, "List")
	apiVersion := gvk.GroupVersion().String()

	var labels map[string]string
	if listOpts.LabelSelector != nil {
		labels = parseSelectorToMap(listOpts.LabelSelector.String())
	}

	items, err := c.listResources(ctx, itemKind, apiVersion, listOpts.Namespace, labels)
	if err != nil {
		return err
	}

	if ul, ok := list.(*unstructured.UnstructuredList); ok {
		ul.Items = items
		return nil
	}

	return populateListObject(items, list)
}

// Close terminates the MCP session. Safe to call multiple times.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.session.Close()
}

// ClusterID returns the cluster this client is bound to.
func (c *Client) ClusterID() string {
	return c.clusterID
}

// Session returns the underlying MCP client session for direct tool calls.
// Used by BridgeTool to call discovered tools without creating new sessions.
func (c *Client) Session() *mcp.ClientSession {
	return c.session
}

// resolveToolName returns the gateway-prefixed tool name. When toolPrefix is
// set (e.g. from ClusterInfo.ToolPrefix), it uses ClusterToolWithPrefix;
// otherwise falls back to the EAIGW convention via ClusterTool.
func (c *Client) resolveToolName(tool string) string {
	if c.toolPrefix != "" {
		return ClusterToolWithPrefix(c.toolPrefix, tool)
	}
	return ClusterTool(c.clusterID, tool)
}

// getResource calls the MCP get_resource tool and returns the parsed unstructured object.
// apiVersion is a mandatory parameter required by the K8s MCP Server.
func (c *Client) getResource(ctx context.Context, kind, apiVersion, namespace, name string) (*unstructured.Unstructured, error) {
	toolName := c.resolveToolName(ToolGet)
	args := map[string]any{
		"kind":       kind,
		"apiVersion": apiVersion,
		"name":       name,
	}
	if namespace != "" {
		args["namespace"] = namespace
	}

	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", toolName, err)
	}

	text := ExtractText(result)
	obj, err := parseUnstructured(text)
	if err != nil {
		return nil, fmt.Errorf("parse Get response for %s/%s: %w", kind, name, err)
	}
	return obj, nil
}

// listResources calls the MCP list_resources tool and returns parsed unstructured items.
// apiVersion is a mandatory parameter required by the K8s MCP Server.
func (c *Client) listResources(ctx context.Context, kind, apiVersion, namespace string, labels map[string]string) ([]unstructured.Unstructured, error) {
	toolName := c.resolveToolName(ToolList)
	args := map[string]any{
		"kind":       kind,
		"apiVersion": apiVersion,
	}
	if namespace != "" {
		args["namespace"] = namespace
	}
	if len(labels) > 0 {
		args["labelSelector"] = formatLabelSelector(labels)
	}

	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", toolName, err)
	}

	if sc := extractStructured(result); sc != nil {
		items := make([]unstructured.Unstructured, len(sc))
		for i, m := range sc {
			items[i] = unstructured.Unstructured{Object: m}
		}
		return items, nil
	}

	text := ExtractText(result)
	items, err := parseMultiFormat(text)
	if err != nil {
		return nil, fmt.Errorf("parse List response for %s: %w", kind, err)
	}
	return items, nil
}

// populateObject copies data from a fetched unstructured object into the target
// client.Object. Handles *unstructured.Unstructured directly, all other types
// (including *metav1.PartialObjectMetadata and typed objects like *corev1.Pod)
// via JSON round-trip.
func populateObject(fetched *unstructured.Unstructured, target client.Object) error {
	if t, ok := target.(*unstructured.Unstructured); ok {
		t.Object = fetched.Object
		return nil
	}

	data, err := json.Marshal(fetched.Object)
	if err != nil {
		return fmt.Errorf("marshal fetched object: %w", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("unmarshal into %T: %w", target, err)
	}
	return nil
}

// populateListObject converts unstructured items into a typed ObjectList via JSON
// round-trip. This enables typed list queries (e.g. *corev1.PodList) over MCP.
func populateListObject(items []unstructured.Unstructured, list client.ObjectList) error {
	rawItems := make([]map[string]any, len(items))
	for i, item := range items {
		rawItems[i] = item.Object
	}
	wrapper := map[string]any{
		"items": rawItems,
	}
	data, err := json.Marshal(wrapper)
	if err != nil {
		return fmt.Errorf("marshal items for typed list: %w", err)
	}
	if err := json.Unmarshal(data, list); err != nil {
		return fmt.Errorf("unmarshal into %T: %w", list, err)
	}
	return nil
}

// parseSelectorToMap converts a simple "key=value,key2=value2" selector string
// into a map. Only handles equality-based selectors (which is what MCP Gateway
// supports via labelSelector).
func parseSelectorToMap(s string) map[string]string {
	if s == "" {
		return nil
	}
	result := make(map[string]string)
	for _, part := range strings.Split(s, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			result[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func formatLabelSelector(labels map[string]string) string {
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

// extractStructured extracts items from StructuredContent if present.
// Returns nil when StructuredContent is absent, empty, or not a []any of maps.
func extractStructured(result *mcp.CallToolResult) []map[string]any {
	if result == nil || result.StructuredContent == nil {
		return nil
	}
	items, ok := result.StructuredContent.([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// ExtractText extracts and concatenates all text content from an MCP tool result.
// Returns text parts joined with newlines, or a JSON-serialized fallback if no text parts are found.
func ExtractText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}

	var texts []string
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			texts = append(texts, tc.Text)
		}
	}
	if len(texts) > 0 {
		return strings.Join(texts, "\n")
	}

	data, err := json.Marshal(result.Content)
	if err != nil {
		return ""
	}
	return string(data)
}

