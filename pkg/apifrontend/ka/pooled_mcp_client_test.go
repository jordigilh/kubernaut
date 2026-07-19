package ka_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

func ctxWithIdentity(username string, groups []string) context.Context {
	return auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
		Username: username,
		Groups:   groups,
	})
}

var _ = Describe("PooledMCPClient (#1306)", func() {
	var (
		pool         *ka.KASessionPool
		connectCount atomic.Int32
	)

	successSession := func() *mockPoolSession {
		return &mockPoolSession{
			callFn: func(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
				resp := map[string]any{"status": "active", "session_id": "sess-001"}
				b, _ := json.Marshal(resp)
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
				}, nil
			},
		}
	}

	countingFactory := func() ka.SessionFactory {
		return func(_ context.Context) (ka.PoolSession, error) {
			connectCount.Add(1)
			return successSession(), nil
		}
	}

	BeforeEach(func() {
		connectCount.Store(0)
		pool = ka.NewKASessionPool(ka.PoolConfig{
			Factory:    countingFactory(),
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})
	})

	Describe("InvokeAction", func() {
		It("UT-AF-1306-001: acquires session from pool using (rr_id, username)", func() {
			client := ka.NewPooledMCPClient(pool, logr.Discard())
			ctx := ctxWithIdentity("alice", []string{"sre"})

			result, err := client.InvokeAction(ctx, ka.InvokeActionArgs{
				RRID:   "ns/rr-001",
				Action: "takeover",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Status).To(Equal("active"))
			Expect(connectCount.Load()).To(Equal(int32(1)),
				"pool factory should be called exactly once")
		})

		It("UT-AF-1306-002: reuses existing session for sequential calls", func() {
			client := ka.NewPooledMCPClient(pool, logr.Discard())
			ctx := ctxWithIdentity("alice", []string{"sre"})

			_, err := client.InvokeAction(ctx, ka.InvokeActionArgs{
				RRID: "ns/rr-001", Action: "takeover",
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = client.InvokeAction(ctx, ka.InvokeActionArgs{
				RRID: "ns/rr-001", Action: "message", Message: "hello",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(connectCount.Load()).To(Equal(int32(1)),
				"factory should only be called once — session reused")
		})

		It("UT-AF-1306-003: complete action releases pooled session", func() {
			client := ka.NewPooledMCPClient(pool, logr.Discard())
			ctx := ctxWithIdentity("alice", []string{"sre"})

			_, err := client.InvokeAction(ctx, ka.InvokeActionArgs{
				RRID: "ns/rr-001", Action: "takeover",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(pool.Size()).To(Equal(1))

			_, err = client.InvokeAction(ctx, ka.InvokeActionArgs{
				RRID: "ns/rr-001", Action: "complete",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(pool.Size()).To(Equal(0),
				"complete must release the pooled session")
		})

		It("UT-AF-1306-004: cancel action releases pooled session", func() {
			client := ka.NewPooledMCPClient(pool, logr.Discard())
			ctx := ctxWithIdentity("alice", []string{"sre"})

			_, err := client.InvokeAction(ctx, ka.InvokeActionArgs{
				RRID: "ns/rr-001", Action: "takeover",
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = client.InvokeAction(ctx, ka.InvokeActionArgs{
				RRID: "ns/rr-001", Action: "cancel",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(pool.Size()).To(Equal(0),
				"cancel must release the pooled session")
		})

		It("UT-AF-1306-007: different users get different sessions", func() {
			client := ka.NewPooledMCPClient(pool, logr.Discard())

			_, err := client.InvokeAction(
				ctxWithIdentity("alice", nil),
				ka.InvokeActionArgs{RRID: "ns/rr-001", Action: "takeover"},
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.InvokeAction(
				ctxWithIdentity("bob", nil),
				ka.InvokeActionArgs{RRID: "ns/rr-001", Action: "takeover"},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(connectCount.Load()).To(Equal(int32(2)),
				"different users must get separate sessions (G9 user isolation)")
		})

		It("UT-AF-1306-008: missing identity returns error", func() {
			client := ka.NewPooledMCPClient(pool, logr.Discard())
			_, err := client.InvokeAction(context.Background(), ka.InvokeActionArgs{
				RRID: "ns/rr-001", Action: "takeover",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("identity"))
		})

		It("UT-AF-1306-009: pool acquire error propagated", func() {
			failPool := ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(_ context.Context) (ka.PoolSession, error) {
					return nil, fmt.Errorf("connection refused")
				},
				Logger: logr.Discard(),
			})
			client := ka.NewPooledMCPClient(failPool, logr.Discard())
			ctx := ctxWithIdentity("alice", nil)

			_, err := client.InvokeAction(ctx, ka.InvokeActionArgs{
				RRID: "ns/rr-001", Action: "takeover",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})
	})

	Describe("DiscoverWorkflows", func() {
		It("UT-AF-1306-005: acquires session and dispatches CallTool", func() {
			wfSession := &mockPoolSession{
				callFn: func(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
					Expect(params.Name).To(Equal("kubernaut_investigate"))
					Expect(params.Arguments).To(HaveKeyWithValue("action", "discover_workflows"))
					resp := `{"workflows":[],"count":0}`
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: resp}},
					}, nil
				},
			}
			dwPool := ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(_ context.Context) (ka.PoolSession, error) {
					return wfSession, nil
				},
				Logger: logr.Discard(),
			})
			client := ka.NewPooledMCPClient(dwPool, logr.Discard())
			ctx := ctxWithIdentity("alice", []string{"sre"})

			result, err := client.DiscoverWorkflows(ctx, ka.DiscoverWorkflowsArgs{RRID: "ns/rr-001"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})
	})

	Describe("SelectWorkflow", func() {
		It("UT-AF-1306-006: acquires session and dispatches CallTool", func() {
			swSession := &mockPoolSession{
				callFn: func(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
					Expect(params.Name).To(Equal("kubernaut_select_workflow"))
					resp := `{"status":"selected","message":"ok"}`
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: resp}},
					}, nil
				},
			}
			swPool := ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(_ context.Context) (ka.PoolSession, error) {
					return swSession, nil
				},
				Logger: logr.Discard(),
			})
			client := ka.NewPooledMCPClient(swPool, logr.Discard())
			ctx := ctxWithIdentity("alice", []string{"sre"})

			result, err := client.SelectWorkflow(ctx, ka.SelectWorkflowArgs{
				RRID: "ns/rr-001", WorkflowID: "wf-001",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Status).To(Equal("selected"))
		})
	})

	It("UT-AF-1306-012: compile-time MCPClient interface check", func() {
		// This test passes at compile time — if PooledMCPClient doesn't
		// implement MCPClient, the test file won't compile.
		var _ ka.MCPClient = (*ka.PooledMCPClient)(nil)
	})

	Describe("Stale-session evict+retry (#1386)", func() {
		It("UT-AF-1386-001: retries on stale session and succeeds with fresh session", func() {
			var callCount atomic.Int32
			staleSession := &mockPoolSession{
				callFn: func(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
					return nil, fmt.Errorf("MCP call kubernaut_investigate: failed to connect (session ID: WVIPJI3): %w", mcp.ErrSessionMissing)
				},
			}
			freshSession := &mockPoolSession{
				callFn: func(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
					callCount.Add(1)
					resp := `{"status":"active","session_id":"sess-fresh"}`
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: resp}},
					}, nil
				},
			}

			retryPool := ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(_ context.Context) (ka.PoolSession, error) {
					return freshSession, nil
				},
				Logger: logr.Discard(),
			})

			client := ka.NewPooledMCPClient(retryPool, logr.Discard())
			ctx := ctxWithIdentity("alice", []string{"sre"})

			retryPool.Inject("ns/rr-stale", "alice", staleSession)
			Expect(retryPool.Size()).To(Equal(1))

			result, err := client.InvokeAction(ctx, ka.InvokeActionArgs{
				RRID: "ns/rr-stale", Action: "takeover",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Status).To(Equal("active"))
			Expect(callCount.Load()).To(Equal(int32(1)),
				"fresh session should have been called once after retry")
		})

		It("UT-AF-1386-002: retries on 'session not found' string match", func() {
			staleSession := &mockPoolSession{
				callFn: func(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
					return nil, fmt.Errorf("MCP call kubernaut_investigate: sending request: session not found")
				},
			}
			freshSession := &mockPoolSession{
				callFn: func(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
					resp := `{"workflows":[],"count":0}`
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: resp}},
					}, nil
				},
			}

			retryPool := ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(_ context.Context) (ka.PoolSession, error) {
					return freshSession, nil
				},
				Logger: logr.Discard(),
			})

			client := ka.NewPooledMCPClient(retryPool, logr.Discard())
			ctx := ctxWithIdentity("alice", []string{"sre"})

			retryPool.Inject("ns/rr-stale2", "alice", staleSession)

			result, err := client.DiscoverWorkflows(ctx, ka.DiscoverWorkflowsArgs{RRID: "ns/rr-stale2"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		It("UT-AF-1386-003: non-stale errors are not retried", func() {
			failSession := &mockPoolSession{
				callFn: func(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
					return nil, fmt.Errorf("MCP call kubernaut_investigate: internal server error")
				},
			}

			noRetryPool := ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(_ context.Context) (ka.PoolSession, error) {
					return failSession, nil
				},
				Logger: logr.Discard(),
			})

			client := ka.NewPooledMCPClient(noRetryPool, logr.Discard())
			ctx := ctxWithIdentity("alice", []string{"sre"})

			_, err := client.InvokeAction(ctx, ka.InvokeActionArgs{
				RRID: "ns/rr-fail", Action: "takeover",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("internal server error"))
			Expect(noRetryPool.Size()).To(Equal(1),
				"pool entry should NOT be evicted for non-stale errors")
		})

		It("UT-AF-1386-004: retry fails if re-acquire fails", func() {
			staleSession := &mockPoolSession{
				callFn: func(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
					return nil, fmt.Errorf("MCP call: %w", mcp.ErrSessionMissing)
				},
			}

			failPool := ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(_ context.Context) (ka.PoolSession, error) {
					return nil, fmt.Errorf("KA unreachable")
				},
				Logger: logr.Discard(),
			})

			client := ka.NewPooledMCPClient(failPool, logr.Discard())
			ctx := ctxWithIdentity("alice", []string{"sre"})

			failPool.Inject("ns/rr-dead", "alice", staleSession)

			_, err := client.InvokeAction(ctx, ka.InvokeActionArgs{
				RRID: "ns/rr-dead", Action: "takeover",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("re-acquire MCP session after stale eviction"))
		})
	})
})

// #1637: PooledMCPClient must attach the caller's ctx to the pooled entry's
// EventRelay for the exact duration of the pooled CallTool, so that
// WatchTerminalEvents (the sole consumer of the session's residual event
// channel after handoff) can discover which A2A call is "live" and relay
// KA's mid-call notifications to it. See DD-AF-009.
var _ = Describe("PooledMCPClient live event relay attach/detach — #1637", func() {

	It("IT-AF-1637-003: callPooledTool attaches ctx to the relay during CallTool and detaches after", func() {
		var duringCallCurrent context.Context
		var duringCallOK bool

		session := &mockPoolSession{}
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return session, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		relay, err := pool.InjectVerified(context.Background(), "ns/rr-relay-call", "alice", session)
		Expect(err).NotTo(HaveOccurred())
		Expect(relay).NotTo(BeNil())

		session.callFn = func(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
			duringCallCurrent = relay.Current()
			duringCallOK = duringCallCurrent != nil
			resp := `{"status":"message_received","session_id":"sess-relay"}`
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: resp}},
			}, nil
		}

		client := ka.NewPooledMCPClient(pool, logr.Discard())
		ctx := ctxWithIdentity("alice", []string{"sre"})

		_, err = client.InvokeAction(ctx, ka.InvokeActionArgs{
			RRID: "ns/rr-relay-call", Action: "message", Message: "hello",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(duringCallOK).To(BeTrue(),
			"IT-AF-1637-003: relay.Current() must be non-nil while CallTool is in flight")
		Expect(duringCallCurrent).To(Equal(ctx),
			"IT-AF-1637-003: relay.Current() during the call must be the exact ctx passed to InvokeAction")
		Expect(relay.Current()).To(BeNil(),
			"IT-AF-1637-003: relay must be detached after the pooled call returns")
	})

	It("IT-AF-1637-003: does not attach when the pooled entry has no relay (plain Inject, no events channel)", func() {
		var duringCallCurrent context.Context

		session := &mockPoolSession{}
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return session, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})
		pool.Inject("ns/rr-no-relay", "alice", session)
		Expect(pool.RelayFor("ns/rr-no-relay", "alice")).To(BeNil())

		session.callFn = func(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
			duringCallCurrent = context.Background() // sentinel: overwritten only if a relay existed
			resp := `{"status":"message_received","session_id":"sess-no-relay"}`
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: resp}},
			}, nil
		}

		client := ka.NewPooledMCPClient(pool, logr.Discard())
		ctx := ctxWithIdentity("alice", []string{"sre"})

		_, err := client.InvokeAction(ctx, ka.InvokeActionArgs{
			RRID: "ns/rr-no-relay", Action: "message", Message: "hello",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(duringCallCurrent).To(Equal(context.Background()),
			"no relay means callPooledTool has nothing to attach — call must still succeed unaffected")
	})
})
