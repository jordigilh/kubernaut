package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: fmc-spike <table|yaml>")
	}
	listOutput := os.Args[1]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/tmp/kube-mcp-server",
		"--list-output", listOutput,
		"--log-level", "0",
		"--toolsets", "core",
	)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("start kube-mcp-server: %v", err)
	}
	defer cmd.Process.Kill()

	mcpClient := mcp.NewClient(
		&mcp.Implementation{Name: "fmc-spike", Version: "v0.0.1"},
		nil,
	)

	transport := &mcp.IOTransport{
		Reader: stdout,
		Writer: stdin,
	}

	session, err := mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer session.Close()

	// Test 1: Namespaced resource (Deployment) with label selector
	fmt.Printf("\n=== %s: Deployment (namespaced, labelSelector=kubernaut.ai/managed=true) ===\n", listOutput)
	callAndDump(ctx, session, "resources_list", map[string]any{
		"kind":          "Deployment",
		"apiVersion":    "apps/v1",
		"namespace":     "kubernaut-spike",
		"labelSelector": "kubernaut.ai/managed=true",
	})

	// Test 2: Cluster-scoped resource (Node) with label selector
	fmt.Printf("\n=== %s: Node (cluster-scoped, labelSelector=kubernaut.ai/managed=true) ===\n", listOutput)
	callAndDump(ctx, session, "resources_list", map[string]any{
		"kind":          "Node",
		"apiVersion":    "v1",
		"labelSelector": "kubernaut.ai/managed=true",
	})

	// Test 3: Service (namespaced, with label selector)
	fmt.Printf("\n=== %s: Service (namespaced, labelSelector=kubernaut.ai/managed=true) ===\n", listOutput)
	callAndDump(ctx, session, "resources_list", map[string]any{
		"kind":          "Service",
		"apiVersion":    "v1",
		"namespace":     "kubernaut-spike",
		"labelSelector": "kubernaut.ai/managed=true",
	})

	// Test 4: Single resource get (Deployment) - for comparison
	fmt.Printf("\n=== %s: resources_get (single Deployment) ===\n", listOutput)
	callAndDump(ctx, session, "resources_get", map[string]any{
		"kind":       "Deployment",
		"apiVersion": "apps/v1",
		"namespace":  "kubernaut-spike",
		"name":       "spike-managed-web",
	})
}

func callAndDump(ctx context.Context, session *mcp.ClientSession, tool string, args map[string]any) {
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      tool,
		Arguments: args,
	})
	if err != nil {
		log.Printf("  ERROR calling %s: %v", tool, err)
		return
	}

	// Dump StructuredContent
	fmt.Printf("\n--- StructuredContent ---\n")
	if result.StructuredContent != nil {
		sc, _ := json.MarshalIndent(result.StructuredContent, "", "  ")
		fmt.Printf("TYPE: %T\n", result.StructuredContent)
		s := string(sc)
		if len(s) > 4000 {
			s = s[:4000] + "\n... [TRUNCATED]"
		}
		fmt.Println(s)
	} else {
		fmt.Println("nil (not set)")
	}

	// Dump Content (text blocks)
	fmt.Printf("\n--- Content (text blocks) ---\n")
	for i, content := range result.Content {
		fmt.Printf("  [%d] type=%T\n", i, content)
		if tc, ok := content.(*mcp.TextContent); ok {
			text := tc.Text
			if len(text) > 4000 {
				text = text[:4000] + "\n... [TRUNCATED]"
			}
			fmt.Println(text)
		}
	}

	fmt.Printf("\n--- IsError: %v ---\n", result.IsError)
}
