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
})
