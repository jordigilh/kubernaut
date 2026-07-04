// spike-mcp-stream: standalone MCP client that connects to KA via port-forward,
// calls action=start on kubernaut_investigate, and counts LoggingMessage events.
// This removes kagenti and AF from the picture to isolate the transport issue.
//
// Usage:
//
//	kubectl port-forward -n kubernaut-system svc/kubernaut-agent 18443:8443 &
//	SA_TOKEN=$(kubectl create token apifrontend -n kubernaut-system --duration=10m)
//	go run ./cmd/spike-mcp-stream --rr-id=<rr-name> --token=$SA_TOKEN
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

// spikeFlags holds the parsed command-line flags for the spike tool.
type spikeFlags struct {
	rrID     string
	token    string
	endpoint string
	timeout  time.Duration
}

// parseSpikeFlags parses and validates the required --rr-id/--token flags.
// Exits the process with a usage error if either is missing.
func parseSpikeFlags() spikeFlags {
	var f spikeFlags
	flag.StringVar(&f.rrID, "rr-id", "", "RemediationRequest name (required)")
	flag.StringVar(&f.token, "token", os.Getenv("SA_TOKEN"), "SA bearer token (or SA_TOKEN env)")
	flag.StringVar(&f.endpoint, "endpoint", "https://localhost:18443/api/v1/mcp", "KA MCP endpoint")
	flag.DurationVar(&f.timeout, "timeout", 120*time.Second, "max wait for events")
	flag.Parse()

	if f.rrID == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --rr-id is required")
		os.Exit(1)
	}
	if f.token == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --token or SA_TOKEN env is required")
		os.Exit(1)
	}
	return f
}

// eventCounters tracks LoggingMessage events received from KA's
// EventLogBridge over the lifetime of the MCP session.
type eventCounters struct {
	count        atomic.Int64
	firstEventAt atomic.Int64 // unix nanos
}

// connectMCP builds the MCP client with a bearer-authenticated streamable
// transport, wires the LoggingMessageHandler to counters, and connects the
// session. Exits the process on connection failure.
func connectMCP(ctx context.Context, f spikeFlags, counters *eventCounters) *mcp.ClientSession {
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "spike-mcp-stream",
		Version: "0.0.1",
	}, &mcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			n := counters.count.Add(1)
			if n == 1 {
				counters.firstEventAt.Store(time.Now().UnixNano())
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
			token: f.token,
			base: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
			},
		},
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint:   f.endpoint,
		HTTPClient: httpClient,
	}

	fmt.Print("1. Connecting to KA MCP... ")
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		fmt.Printf("FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK")

	return session
}

// callInvestigateStart sets the session logging level, then invokes
// kubernaut_investigate action=start and prints its response. Exits the
// process on RPC failure or a tool-level error result.
func callInvestigateStart(ctx context.Context, session *mcp.ClientSession, rrID string, startTime time.Time) {
	fmt.Print("2. Setting logging level to 'info'... ")
	if err := session.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "info"}); err != nil {
		fmt.Printf("FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK")

	fmt.Print("3. Calling kubernaut_investigate action=start... ")
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
}

// waitForEvents blocks until ctx is done, printing a periodic tally of
// events received from KA's EventLogBridge.
func waitForEvents(ctx context.Context, timeout time.Duration, startTime time.Time, counters *eventCounters) {
	fmt.Printf("\n4. Waiting for LoggingMessage events (up to %s)...\n", timeout)
	fmt.Println("   (Events arriving from KA's EventLogBridge via sess.Log)")
	fmt.Println()

	fmt.Printf("   Events received during CallTool: %d\n", counters.count.Load())

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Printf("   [%s] events so far: %d\n",
				time.Since(startTime).Round(time.Second), counters.count.Load())
		}
	}
}

// printResults prints the final event tally and a pass/fail verdict.
func printResults(startTime time.Time, counters *eventCounters) {
	total := counters.count.Load()
	fmt.Println()
	fmt.Println("=== RESULTS ===")
	fmt.Printf("Total events received:    %d\n", total)
	fmt.Printf("Total wall time:          %s\n", time.Since(startTime).Round(time.Second))
	if total > 0 && counters.firstEventAt.Load() > 0 {
		firstAt := time.Unix(0, counters.firstEventAt.Load())
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

func main() {
	f := parseSpikeFlags()

	var counters eventCounters

	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()

	fmt.Println("=== SPIKE: MCP Stream Test ===")
	fmt.Printf("Endpoint: %s\n", f.endpoint)
	fmt.Printf("RR ID:    %s\n", f.rrID)
	fmt.Printf("Timeout:  %s\n", f.timeout)
	fmt.Println()

	session := connectMCP(ctx, f, &counters)
	defer func() { _ = session.Close() }()

	startTime := time.Now()
	callInvestigateStart(ctx, session, f.rrID, startTime)
	waitForEvents(ctx, f.timeout, startTime, &counters)
	printResults(startTime, &counters)
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
