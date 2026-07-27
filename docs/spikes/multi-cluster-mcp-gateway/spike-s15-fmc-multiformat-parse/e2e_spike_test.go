package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func startMCPServer(t *testing.T, listOutput string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	cmd := exec.CommandContext(ctx, "/tmp/kube-mcp-server",
		"--list-output", listOutput,
		"--log-level", "0",
		"--toolsets", "core",
	)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start kube-mcp-server: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill() })

	mcpClient := mcp.NewClient(
		&mcp.Implementation{Name: "fmc-spike-e2e", Version: "v0.0.1"},
		nil,
	)
	session, err := mcpClient.Connect(ctx, &mcp.IOTransport{Reader: stdout, Writer: stdin}, nil)
	if err != nil {
		t.Fatalf("connect (list_output=%s): %v", listOutput, err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func extractText(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func extractStructured(result *mcp.CallToolResult) []map[string]any {
	if result == nil || result.StructuredContent == nil {
		return nil
	}
	items, ok := result.StructuredContent.([]any)
	if !ok {
		return nil
	}
	result2 := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			result2 = append(result2, m)
		}
	}
	return result2
}

// fullPriorityChain simulates the actual FMC client parse logic.
func fullPriorityChain(result *mcp.CallToolResult) ([]unstructured.Unstructured, string, error) {
	// Priority 1: StructuredContent
	if sc := extractStructured(result); sc != nil {
		items := make([]unstructured.Unstructured, len(sc))
		for i, m := range sc {
			items[i] = unstructured.Unstructured{Object: m}
		}
		return items, "structuredContent", nil
	}

	// Priority 2+3+4: Text parsing
	text := extractText(result)
	items, err := parseMultiFormat(text)
	if err != nil {
		return nil, "", err
	}

	// Determine which format was used
	format := "unknown"
	if len(text) > 0 {
		if text[0] == '{' || text[0] == '[' {
			format = "json"
		} else if looksLikeTable(text) {
			format = "table"
		} else {
			format = "yaml"
		}
	}
	return items, format, nil
}

func TestE2ETableDeployment(t *testing.T) {
	session := startMCPServer(t, "table")
	ctx := context.Background()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "resources_list",
		Arguments: map[string]any{
			"kind": "Deployment", "apiVersion": "apps/v1",
			"namespace": "kubernaut-spike", "labelSelector": "kubernaut.ai/managed=true",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	items, format, err := fullPriorityChain(result)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	t.Logf("Format detected: %s", format)
	t.Logf("StructuredContent nil: %v", result.StructuredContent == nil)
	assertEqual(t, "format", format, "table")

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	assertEqual(t, "Name", items[0].GetName(), "spike-managed-web")
	assertEqual(t, "Namespace", items[0].GetNamespace(), "kubernaut-spike")
	assertEqual(t, "Kind", items[0].GetKind(), "Deployment")
	assertEqual(t, "APIVersion", items[0].GetAPIVersion(), "apps/v1")
	t.Logf("PASS: Table Deployment -> %s/%s %s/%s", items[0].GetAPIVersion(), items[0].GetKind(), items[0].GetNamespace(), items[0].GetName())
}

func TestE2ETableNodeClusterScoped(t *testing.T) {
	session := startMCPServer(t, "table")
	ctx := context.Background()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "resources_list",
		Arguments: map[string]any{
			"kind": "Node", "apiVersion": "v1",
			"labelSelector": "kubernaut.ai/managed=true",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	items, format, err := fullPriorityChain(result)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	assertEqual(t, "format", format, "table")
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	assertEqual(t, "Name", items[0].GetName(), "dev-worker-0.redhat-internal.com")
	assertEqual(t, "Namespace", items[0].GetNamespace(), "")
	assertEqual(t, "Kind", items[0].GetKind(), "Node")
	t.Logf("PASS: Table Node (cluster-scoped) -> %s/%s %s", items[0].GetAPIVersion(), items[0].GetKind(), items[0].GetName())
}

func TestE2EYAMLDeployment(t *testing.T) {
	session := startMCPServer(t, "yaml")
	ctx := context.Background()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "resources_list",
		Arguments: map[string]any{
			"kind": "Deployment", "apiVersion": "apps/v1",
			"namespace": "kubernaut-spike", "labelSelector": "kubernaut.ai/managed=true",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	items, format, err := fullPriorityChain(result)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	assertEqual(t, "format", format, "yaml")
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	assertEqual(t, "Name", items[0].GetName(), "spike-managed-web")
	assertEqual(t, "Namespace", items[0].GetNamespace(), "kubernaut-spike")
	assertEqual(t, "Kind", items[0].GetKind(), "Deployment")

	labels := items[0].GetLabels()
	if labels["kubernaut.ai/managed"] != "true" {
		t.Errorf("expected managed label, got: %v", labels)
	}
	t.Logf("PASS: YAML Deployment -> full object with %d labels", len(labels))
}

func TestE2EYAMLNodeClusterScoped(t *testing.T) {
	session := startMCPServer(t, "yaml")
	ctx := context.Background()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "resources_list",
		Arguments: map[string]any{
			"kind": "Node", "apiVersion": "v1",
			"labelSelector": "kubernaut.ai/managed=true",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	items, format, err := fullPriorityChain(result)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	assertEqual(t, "format", format, "yaml")
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	assertEqual(t, "Name", items[0].GetName(), "dev-worker-0.redhat-internal.com")
	assertEqual(t, "Namespace", items[0].GetNamespace(), "")
	t.Logf("PASS: YAML Node (cluster-scoped) -> full object")
}

func TestE2EStructuredContentNil(t *testing.T) {
	session := startMCPServer(t, "table")
	ctx := context.Background()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "resources_list",
		Arguments: map[string]any{
			"kind": "Service", "apiVersion": "v1",
			"namespace": "kubernaut-spike", "labelSelector": "kubernaut.ai/managed=true",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	if result.StructuredContent != nil {
		sc, _ := json.MarshalIndent(result.StructuredContent, "", "  ")
		t.Logf("UNEXPECTED: StructuredContent is set: %s", string(sc))
	} else {
		t.Logf("CONFIRMED: StructuredContent is nil (kube-mcp-server has not adopted structured output for resources_list yet)")
	}
}
