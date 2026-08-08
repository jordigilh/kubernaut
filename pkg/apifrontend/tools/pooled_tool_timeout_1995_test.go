package tools_test

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// containsMessage reports whether sink captured a log entry with the given message.
func (s *logCaptureSink) containsMessage(msg string) bool {
	for _, m := range s.messages {
		if m == msg {
			return true
		}
	}
	return false
}

var _ = Describe("Pooled KA tool-call timeout observability (#1995, BR-INTERACTIVE-010)", func() {
	var origTimeout time.Duration

	BeforeEach(func() {
		origTimeout = tools.PooledToolCallTimeout
		tools.PooledToolCallTimeout = 20 * time.Millisecond
	})

	AfterEach(func() {
		tools.PooledToolCallTimeout = origTimeout
	})

	Describe("HandleDiscoverWorkflows", func() {
		It("UT-AF-1995-005: logs a warning with rr_id and elapsed time when the pooled call times out", func() {
			mockMCP := &ka.MockMCPClient{
				DiscoverWorkflowsFn: func(ctx context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				},
			}

			sink := &logCaptureSink{}
			logger := logr.New(sink)
			ctx := logr.NewContext(context.Background(), logger)

			var result tools.DiscoverWorkflowsResult
			var err error
			awaitWithSafetyBound(2*time.Second, func() {
				result, err = tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{RRID: "pay/rr-1995-disc"})
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(0))

			found := false
			for i, msg := range sink.messages {
				if msg == "pooled KA tool call timed out or was canceled" {
					found = true
					kv := sink.kvPairs[i]
					Expect(kv).To(HaveKeyWithValue("rr_id", "pay/rr-1995-disc"))
					Expect(kv).To(HaveKeyWithValue("tool", "discover_workflows"))
					Expect(kv).To(HaveKey("elapsed"))
				}
			}
			Expect(found).To(BeTrue(), "expected a timeout-fired log line from HandleDiscoverWorkflows; got messages: %v", sink.messages)
		})
	})

	Describe("HandleSelectWorkflow", func() {
		It("UT-AF-1995-006: logs a warning with rr_id and elapsed time when the pooled call times out", func() {
			mockMCP := &ka.MockMCPClient{
				SelectWorkflowFn: func(ctx context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				},
			}

			sink := &logCaptureSink{}
			logger := logr.New(sink)
			ctx := logr.NewContext(context.Background(), logger)

			var err error
			awaitWithSafetyBound(2*time.Second, func() {
				_, err = tools.HandleSelectWorkflow(ctx, mockMCP, tools.SelectWorkflowArgs{
					RRID:       "pay/rr-1995-sel",
					WorkflowID: "wf-restart",
				}, nil)
			})

			Expect(err).To(HaveOccurred())

			found := sink.containsMessage("pooled KA tool call timed out or was canceled")
			Expect(found).To(BeTrue(), "expected a timeout-fired log line from HandleSelectWorkflow; got messages: %v", sink.messages)
		})
	})

	Describe("HandleCompleteNoAction", func() {
		It("UT-AF-1995-007: returns instead of hanging when the pooled MCP call never responds", func() {
			mockMCP := &ka.MockMCPClient{
				CompleteNoActionFn: func(ctx context.Context, _ ka.CompleteNoActionArgs) (*ka.CompleteNoActionResult, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				},
			}

			var err error
			awaitWithSafetyBound(2*time.Second, func() {
				_, err = tools.HandleCompleteNoAction(context.Background(), mockMCP, tools.CompleteNoActionArgs{
					RRID: "pay/rr-1995-cna",
				}, nil)
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("complete_no_action"))
		})
	})
})
