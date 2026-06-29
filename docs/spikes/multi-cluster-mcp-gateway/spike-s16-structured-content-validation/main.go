package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	endpoint := "http://localhost:31975/mcp"
	if v := os.Getenv("MCP_ENDPOINT"); v != "" {
		endpoint = v
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Printf("Connecting to MCP Gateway at %s ...\n", endpoint)

	mcpClient := mcp.NewClient(
		&mcp.Implementation{Name: "spike-structured-content", Version: "v0.0.1"},
		nil,
	)

	transport := &mcp.StreamableClientTransport{Endpoint: endpoint}
	session, err := mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer session.Close()
	fmt.Println("Connected.\n")

	tests := []struct {
		label string
		tool  string
		args  map[string]any
	}{
		{
			label: "resources_list (Namespace, cluster-scoped)",
			tool:  "loopback_cluster_resources_list",
			args:  map[string]any{"kind": "Namespace", "apiVersion": "v1"},
		},
		{
			label: "resources_get (Namespace/default)",
			tool:  "loopback_cluster_resources_get",
			args:  map[string]any{"kind": "Namespace", "apiVersion": "v1", "name": "default"},
		},
		{
			label: "resources_list (Pod, kubernaut-system)",
			tool:  "loopback_cluster_resources_list",
			args:  map[string]any{"kind": "Pod", "apiVersion": "v1", "namespace": "kubernaut-system"},
		},
		{
			label: "resources_get (Service/kube-mcp-server)",
			tool:  "loopback_cluster_resources_get",
			args:  map[string]any{"kind": "Service", "apiVersion": "v1", "namespace": "kubernaut-system", "name": "kube-mcp-server"},
		},
	}

	allPassed := true
	for _, tc := range tests {
		fmt.Printf("━━━ %s ━━━\n", tc.label)
		result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args})
		if callErr != nil {
			fmt.Printf("  ❌ CallTool error: %v\n\n", callErr)
			allPassed = false
			continue
		}
		if result.IsError {
			fmt.Printf("  ❌ Tool returned error: %s\n\n", extractText(result))
			allPassed = false
			continue
		}

		dumpResult(result)
		if result.StructuredContent == nil {
			allPassed = false
		}
		fmt.Println()
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if allPassed {
		fmt.Println("✅ All tests returned structuredContent — kube-mcp-server PR #1232 is live")
	} else {
		fmt.Println("⚠️  Some tests did NOT return structuredContent")
	}
}

func dumpResult(result *mcp.CallToolResult) {
	// StructuredContent
	if result.StructuredContent != nil {
		sc, _ := json.MarshalIndent(result.StructuredContent, "", "  ")
		s := string(sc)
		fmt.Printf("  ✅ StructuredContent present (type=%T)\n", result.StructuredContent)
		if len(s) > 1500 {
			fmt.Printf("  Preview (first 1500 chars):\n%s\n  ... [TRUNCATED, total %d chars]\n", s[:1500], len(s))
		} else {
			fmt.Println(s)
		}
	} else {
		fmt.Println("  ❌ StructuredContent: nil")
	}

	// Text content summary
	for i, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			preview := tc.Text
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			fmt.Printf("  TextContent[%d] (%d chars): %s\n", i, len(tc.Text), preview)
		}
	}
}

func extractText(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return "<no text>"
}
