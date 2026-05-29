// spike-mcp-stream: standalone MCP client that connects to KA via port-forward,
// calls action=start on kubernaut_investigate, and counts LoggingMessage events.
// This removes kagenti and AF from the picture to isolate the transport issue.
//
// Usage:
//   kubectl port-forward -n kubernaut-system svc/kubernaut-agent 18443:8443 &
//   SA_TOKEN=$(kubectl create token apifrontend -n kubernaut-system --duration=10m)
//   go run ./cmd/spike-mcp-stream --rr-id=<rr-name> --token=$SA_TOKEN
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	var (
		rrID     string
		token    string
		endpoint string
		timeout  time.Duration
	)
	flag.StringVar(&rrID, "rr-id", "", "RemediationRequest name (required)")
	flag.StringVar(&token, "token", os.Getenv("SA_TOKEN"), "SA bearer token (or SA_TOKEN env)")
	flag.StringVar(&endpoint, "endpoint", "https://localhost:18443/api/v1/mcp", "KA MCP endpoint")
	flag.DurationVar(&timeout, "timeout", 120*time.Second, "max wait for events")
	flag.Parse()

	if rrID == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --rr-id is required")
		os.Exit(1)
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --token or SA_TOKEN env is required")
		os.Exit(1)
	}

	var eventCount atomic.Int64
	var firstEventAt atomic.Int64 // unix nanos

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "spike-mcp-stream",
		Version: "0.0.1",
	}, &mcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			n := eventCount.Add(1)
			if n == 1 {
				firstEventAt.Store(time.Now().UnixNano())
			}
			raw, _ := json.Marshal(req.Params.Data)
			var envelope struct {
				Type string `json:"type"`
				Seq  int64  `json:"seq"`
			}
			_ = json.Unmarshal(raw, &envelope)
			fmt.Printf("  [EVENT #%d] level=%s type=%s seq=%d logger=%s\n",
				n, req.Params.Level, envelope.Type, envelope.Seq, req.Params.Logger)
		},
	})

	httpClient := &http.Client{
		Transport: &bearerTransport{
			token: token,
			base: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
			},
		},
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: httpClient,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Println("=== SPIKE: MCP Stream Test ===")
	fmt.Printf("Endpoint: %s\n", endpoint)
	fmt.Printf("RR ID:    %s\n", rrID)
	fmt.Printf("Timeout:  %s\n", timeout)
	fmt.Println()

	// Step 1: Connect
	fmt.Print("1. Connecting to KA MCP... ")
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = session.Close() }()
	fmt.Println("OK")

	// Step 2: SetLoggingLevel
	fmt.Print("2. Setting logging level to 'info'... ")
	if err := session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "info"}); err != nil {
		fmt.Printf("FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK")

	// Step 3: CallTool kubernaut_investigate action=start
	fmt.Print("3. Calling kubernaut_investigate action=start... ")
	startTime := time.Now()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "kubernaut_investigate",
		Arguments: map[string]any{
			"rr_id":              rrID,
			"action":             "start",
			"acting_user":        "spike-test",
			"acting_user_groups": []string{"system:masters"},
		},
	})
	callDuration := time.Since(startTime)
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("OK (took %s)\n", callDuration.Round(time.Millisecond))

	if result.IsError {
		msg := "unknown error"
		if len(result.Content) > 0 {
			if tc, ok := result.Content[0].(*mcp.TextContent); ok {
				msg = tc.Text
			}
		}
		fmt.Printf("   Tool returned error: %s\n", msg)
		os.Exit(1)
	}

	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			fmt.Printf("   Response: %s\n", tc.Text)
		}
	}

	// Step 4: Wait for events
	fmt.Printf("\n4. Waiting for LoggingMessage events (up to %s)...\n", timeout)
	fmt.Println("   (Events arriving from KA's EventLogBridge via sess.Log)")
	fmt.Println()

	eventsAtCallEnd := eventCount.Load()
	fmt.Printf("   Events received during CallTool: %d\n", eventsAtCallEnd)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			goto done
		case <-ticker.C:
			current := eventCount.Load()
			fmt.Printf("   [%s] events so far: %d\n",
				time.Since(startTime).Round(time.Second), current)
			eventsAtCallEnd = current
		}
	}

done:
	total := eventCount.Load()
	fmt.Println()
	fmt.Println("=== RESULTS ===")
	fmt.Printf("Total events received:    %d\n", total)
	fmt.Printf("Total wall time:          %s\n", time.Since(startTime).Round(time.Second))
	if total > 0 && firstEventAt.Load() > 0 {
		firstAt := time.Unix(0, firstEventAt.Load())
		fmt.Printf("Time to first event:      %s\n", firstAt.Sub(startTime).Round(time.Millisecond))
	} else {
		fmt.Println("Time to first event:      NEVER (no events received)")
	}
	fmt.Println()

	if total == 0 {
		fmt.Println("VERDICT: KA sent events but MCP client received NONE.")
		fmt.Println("         This confirms the transport-level issue.")
	} else {
		fmt.Println("VERDICT: MCP client DID receive events.")
		fmt.Println("         The issue is upstream (AF or kagenti).")
	}
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}
