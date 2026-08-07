package tools_test

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// hangingPoolSession is a ka.PoolSession fake whose CallTool blocks until
// the caller's ctx ends, then returns ctx.Err() — reproducing #1954's
// "MCP response lost on a reused pooled session" symptom, where only a
// caller-side deadline (not the server) can ever unblock the call.
type hangingPoolSession struct {
	mu     sync.Mutex
	closed bool
}

func (s *hangingPoolSession) CallTool(ctx context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (s *hangingPoolSession) Ping(_ context.Context, _ *mcp.PingParams) error { return nil }

func (s *hangingPoolSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func identityCtxFor1954(username string) context.Context {
	return auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
		Username: username,
		Groups:   []string{"sre"},
	})
}

// specSafetyBound is comfortably longer than tools.PooledToolCallTimeout's
// shrunk-for-test value (20ms) but short enough to fail RED-phase specs
// fast instead of hanging the suite.
const specSafetyBound = 2 * time.Second

// awaitWithSafetyBound runs fn in a goroutine and fails the spec fast if it
// doesn't complete within specSafetyBound, instead of letting a regression
// (no timeout applied anywhere in the chain) hang the whole suite until the
// outer `go test` process timeout.
func awaitWithSafetyBound(fn func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(specSafetyBound):
		Fail("operation did not return within the safety bound — #1954 timeout not enforced")
	}
}

var _ = Describe("Pooled MCP tool-call timeout (#1954, BR-INTERACTIVE-010)", func() {
	var origTimeout time.Duration

	BeforeEach(func() {
		origTimeout = tools.PooledToolCallTimeout
		tools.PooledToolCallTimeout = 20 * time.Millisecond
	})

	AfterEach(func() {
		tools.PooledToolCallTimeout = origTimeout
	})

	Describe("HandleDiscoverWorkflows", func() {
		It("UT-AF-1954-001: returns instead of hanging when the pooled MCP call never responds", func() {
			mockMCP := &ka.MockMCPClient{
				DiscoverWorkflowsFn: func(ctx context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				},
			}

			var result tools.DiscoverWorkflowsResult
			var err error
			awaitWithSafetyBound(func() {
				result, err = tools.HandleDiscoverWorkflows(context.Background(), mockMCP, tools.DiscoverWorkflowsArgs{})
			})

			// HandleDiscoverWorkflows deliberately downgrades a deadline/cancel
			// error to an empty-but-successful result (pre-existing graceful
			// degradation, left unchanged by #1954's fix) — what matters here
			// is that it returns at all within the safety bound above.
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(0))
		})
	})

	Describe("HandleSelectWorkflow", func() {
		It("UT-AF-1954-002: returns an error instead of hanging when the pooled MCP call never responds", func() {
			mockMCP := &ka.MockMCPClient{
				SelectWorkflowFn: func(ctx context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				},
			}

			var err error
			awaitWithSafetyBound(func() {
				_, err = tools.HandleSelectWorkflow(context.Background(), mockMCP, tools.SelectWorkflowArgs{
					RRID:       "pay/rr-1954",
					WorkflowID: "wf-restart",
				}, nil)
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("selecting workflow"))
		})
	})

	Describe("invokeInteractiveAction (via HandleMessage)", func() {
		It("UT-AF-1954-003: returns an error instead of hanging when the pooled MCP call never responds", func() {
			mockMCP := &ka.MockMCPClient{
				InvokeActionFn: func(ctx context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				},
			}

			var err error
			awaitWithSafetyBound(func() {
				_, err = tools.HandleMessage(context.Background(), mockMCP, tools.InteractiveActionArgs{
					RRID:    "rr-prod-1954",
					Message: "still there?",
				}, nil)
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("message"))
		})
	})

	// IT-AF-1954-004 exercises the real production wiring end to end:
	// HandleDiscoverWorkflows -> the real ka.PooledMCPClient -> the real
	// ka.KASessionPool -> only the actual network boundary, PoolSession,
	// is faked. A mock at the ka.MCPClient layer (the three specs above)
	// only proves the handler applies a deadline to its own ctx argument;
	// this is the one that proves that deadline actually propagates
	// through the real pool/session plumbing far enough to cancel a
	// genuinely stuck call — the gap that would have let #1954 through.
	Describe("IT-AF-1954-004: real PooledMCPClient/KASessionPool wiring", func() {
		It("bounds a stuck pooled-session CallTool through the real production chain", func() {
			pool := ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(_ context.Context) (ka.PoolSession, error) {
					return &hangingPoolSession{}, nil
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})
			pooledClient := ka.NewPooledMCPClient(pool, logr.Discard())

			var result tools.DiscoverWorkflowsResult
			var err error
			awaitWithSafetyBound(func() {
				result, err = tools.HandleDiscoverWorkflows(identityCtxFor1954("alice"), pooledClient, tools.DiscoverWorkflowsArgs{
					RRID: "pay/rr-1954-it",
				})
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(0))
		})
	})
})
