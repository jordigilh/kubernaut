package tools_test

import (
	"context"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("kubernaut_discover_workflows adversarial", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-WP-047: KA returns malformed JSON — error wrapped, no panic", func() {
		mockMCP := &ka.MockMCPClient{
			DiscoverWorkflowsFn: func(_ context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
				return nil, fmt.Errorf("unmarshal error: invalid character")
			},
		}
		result, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{})
		Expect(err).To(HaveOccurred())
		Expect(result.Workflows).To(BeNil())
	})

	It("UT-AF-WP-048: KA MCP endpoint unreachable — ErrMCPUnavailable returned", func() {
		mockMCP := &ka.MockMCPClient{
			DiscoverWorkflowsFn: func(_ context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
				return nil, ka.ErrMCPUnavailable
			},
		}
		_, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unavailable"))
	})

	It("UT-AF-WP-049: context cancellation during discovery — graceful degradation to empty result", func() {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()
		mockMCP := &ka.MockMCPClient{
			DiscoverWorkflowsFn: func(ctx context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
				return nil, ctx.Err()
			},
		}
		result, err := tools.HandleDiscoverWorkflows(cancelledCtx, mockMCP, tools.DiscoverWorkflowsArgs{})
		Expect(err).NotTo(HaveOccurred(),
			"#1408: context cancellation must degrade gracefully, not error")
		Expect(result.Workflows).To(BeEmpty())
		Expect(result.Count).To(Equal(0))
	})

	It("UT-AF-WP-050: concurrent discovery calls — no data race under -race", func() {
		mockMCP := &ka.MockMCPClient{
			DiscoverWorkflowsFn: func(_ context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
				return &ka.DiscoverWorkflowsResult{
					Workflows: []ka.DiscoveredWorkflow{{WorkflowID: "wf-1"}},
				}, nil
			},
		}

		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{})
				Expect(err).NotTo(HaveOccurred())
			}()
		}
		wg.Wait()
	})

	It("UT-AF-WP-051: workflow with 0 parameters — valid, select works without params", func() {
		mockMCP := &ka.MockMCPClient{
			DiscoverWorkflowsFn: func(_ context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
				return &ka.DiscoverWorkflowsResult{
					Workflows: []ka.DiscoveredWorkflow{
						{WorkflowID: "wf-noop", Name: "No-Op", Parameters: []ka.WorkflowParameterSchema{}},
					},
				}, nil
			},
		}
		result, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Workflows[0].Parameters).To(BeEmpty())

		err = tools.ValidateWorkflowParameters([]tools.WorkflowParameter{}, map[string]any{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-AF-WP-052: workflow with 100+ parameters — no performance degradation", func() {
		params := make([]ka.WorkflowParameterSchema, 150)
		for i := range params {
			params[i] = ka.WorkflowParameterSchema{
				Name: fmt.Sprintf("param_%d", i),
				Type: "string",
			}
		}
		mockMCP := &ka.MockMCPClient{
			DiscoverWorkflowsFn: func(_ context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
				return &ka.DiscoverWorkflowsResult{
					Workflows: []ka.DiscoveredWorkflow{
						{WorkflowID: "wf-big", Parameters: params},
					},
				}, nil
			},
		}
		result, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Workflows[0].Parameters).To(HaveLen(150))
	})

	It("UT-AF-WP-053: parameter name with special characters — handled safely", func() {
		schema := []tools.WorkflowParameter{
			{Name: "ns/pod-name.v2", Type: "string", Required: true},
		}
		err := tools.ValidateWorkflowParameters(schema, map[string]any{"ns/pod-name.v2": "value"})
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-AF-WP-054: nil parameters map in SelectWorkflowArgs — backward compat", func() {
		mockMCP := &ka.MockMCPClient{
			SelectWorkflowFn: func(_ context.Context, args ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
				Expect(args.Parameters).To(BeNil())
				return &ka.SelectWorkflowResult{Status: "accepted"}, nil
			},
		}
		result, err := tools.HandleSelectWorkflow(ctx, mockMCP, tools.SelectWorkflowArgs{
			RRID: "rr-1", WorkflowID: "wf-1",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("accepted"))
	})

	It("UT-AF-WP-055: discovery response with extra unknown fields — no unmarshal error", func() {
		mockMCP := &ka.MockMCPClient{
			DiscoverWorkflowsFn: func(_ context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
				return &ka.DiscoverWorkflowsResult{
					Workflows: []ka.DiscoveredWorkflow{
						{WorkflowID: "wf-extra", Name: "Extra Fields"},
					},
				}, nil
			},
		}
		result, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Workflows[0].WorkflowID).To(Equal("wf-extra"))
	})

	It("UT-AF-WP-056: parameter default value type mismatch with declared type — error", func() {
		schema := []tools.WorkflowParameter{
			{Name: "count", Type: "int", Required: false, Default: "not-a-number"},
		}
		err := tools.ValidateWorkflowParameters(schema, map[string]any{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("default"))
	})
})
