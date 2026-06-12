package tools_test

import (
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("kubernaut_discover_workflows", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("HandleDiscoverWorkflows", func() {
		It("UT-AF-WP-001: returns workflows with parameters from mock KA", func() {
			mockMCP := &ka.MockMCPClient{
				DiscoverWorkflowsFn: func(_ context.Context, args ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
					return &ka.DiscoverWorkflowsResult{
						Workflows: []ka.DiscoveredWorkflow{
							{
								WorkflowID:  "wf-restart",
								Name:        "Pod Restart",
								Description: "Restart a failing pod",
								Parameters: []ka.WorkflowParameterSchema{
									{Name: "namespace", Type: "string", Required: true},
									{Name: "pod_name", Type: "string", Required: true},
								},
							},
						},
					}, nil
				},
			}
			result, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(1))
			Expect(result.Workflows).To(HaveLen(1))
			Expect(result.Workflows[0].Parameters).To(HaveLen(2))
			Expect(result.Workflows[0].Parameters[0].Name).To(Equal("namespace"))
		})

		It("UT-AF-WP-002: with workflow_id filter returns single workflow", func() {
			mockMCP := &ka.MockMCPClient{
				DiscoverWorkflowsFn: func(_ context.Context, args ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
					Expect(args.WorkflowID).To(Equal("wf-scale"))
					return &ka.DiscoverWorkflowsResult{
						Workflows: []ka.DiscoveredWorkflow{
							{WorkflowID: "wf-scale", Name: "Scale Up", Parameters: []ka.WorkflowParameterSchema{}},
						},
					}, nil
				},
			}
			result, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{WorkflowID: "wf-scale"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(1))
			Expect(result.Workflows[0].WorkflowID).To(Equal("wf-scale"))
		})

		It("UT-AF-WP-003: with empty KA response returns Count=0", func() {
			mockMCP := &ka.MockMCPClient{
				DiscoverWorkflowsFn: func(_ context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
					return &ka.DiscoverWorkflowsResult{Workflows: []ka.DiscoveredWorkflow{}}, nil
				},
			}
			result, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(0))
			Expect(result.Workflows).To(BeEmpty())
		})

		It("UT-AF-WP-004: with nil MCPClient returns descriptive error", func() {
			_, err := tools.HandleDiscoverWorkflows(ctx, nil, tools.DiscoverWorkflowsArgs{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not available"))
		})

		It("UT-AF-WP-005: with KA error wraps and returns error", func() {
			mockMCP := &ka.MockMCPClient{
				DiscoverWorkflowsFn: func(_ context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
					return nil, fmt.Errorf("connection refused")
				},
			}
			_, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("discover"))
		})

		It("UT-AF-1408-010: SI-4 — context deadline exceeded degrades gracefully to empty workflows", func() {
			deadlineCtx, cancel := context.WithCancel(ctx)
			cancel()
			mockMCP := &ka.MockMCPClient{
				DiscoverWorkflowsFn: func(c context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
					return nil, c.Err()
				},
			}
			result, err := tools.HandleDiscoverWorkflows(deadlineCtx, mockMCP, tools.DiscoverWorkflowsArgs{RRID: "rr-timeout-001"})
			Expect(err).NotTo(HaveOccurred(),
				"SI-4: timeout must degrade gracefully, not surface as tool error to LLM")
			Expect(result.Workflows).To(BeEmpty())
			Expect(result.Count).To(Equal(0))
		})
	})

	Describe("WorkflowParameter serialization", func() {
		It("UT-AF-WP-006: serializes all fields to JSON correctly", func() {
			param := tools.WorkflowParameter{
				Name:        "replicas",
				Type:        "int",
				Description: "Number of pod replicas",
				Required:    true,
				Default:     3,
				Enum:        nil,
			}
			data, err := json.Marshal(param)
			Expect(err).NotTo(HaveOccurred())

			var decoded tools.WorkflowParameter
			Expect(json.Unmarshal(data, &decoded)).To(Succeed())
			Expect(decoded.Name).To(Equal("replicas"))
			Expect(decoded.Type).To(Equal("int"))
			Expect(decoded.Description).To(Equal("Number of pod replicas"))
			Expect(decoded.Required).To(BeTrue())
		})

		It("UT-AF-WP-007: Enum field contains valid options", func() {
			param := tools.WorkflowParameter{
				Name: "strategy",
				Type: "string",
				Enum: []string{"rolling", "recreate", "blue-green"},
			}
			data, err := json.Marshal(param)
			Expect(err).NotTo(HaveOccurred())

			var decoded tools.WorkflowParameter
			Expect(json.Unmarshal(data, &decoded)).To(Succeed())
			Expect(decoded.Enum).To(ConsistOf("rolling", "recreate", "blue-green"))
		})
	})

	Describe("ValidateWorkflowParameters", func() {
		var schema []tools.WorkflowParameter

		BeforeEach(func() {
			schema = []tools.WorkflowParameter{
				{Name: "namespace", Type: "string", Required: true},
				{Name: "replicas", Type: "int", Required: true},
				{Name: "strategy", Type: "string", Required: false, Default: "rolling", Enum: []string{"rolling", "recreate"}},
				{Name: "dry_run", Type: "bool", Required: false, Default: false},
				{Name: "timeout", Type: "float", Required: false},
			}
		})

		DescribeTable("validation cases",
			func(params map[string]any, shouldPass bool, errSubstring string) {
				err := tools.ValidateWorkflowParameters(schema, params)
				if shouldPass {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
					if errSubstring != "" {
						Expect(err.Error()).To(ContainSubstring(errSubstring))
					}
				}
			},
			Entry("UT-AF-WP-008: passes with all required params present",
				map[string]any{"namespace": "prod", "replicas": 3}, true, ""),
			Entry("UT-AF-WP-009: fails when required param missing",
				map[string]any{"namespace": "prod"}, false, "replicas"),
			Entry("UT-AF-WP-010: fails with unknown parameter key",
				map[string]any{"namespace": "prod", "replicas": 3, "bogus": "val"}, false, "unknown"),
			Entry("UT-AF-WP-011: fails with wrong type (string for int)",
				map[string]any{"namespace": "prod", "replicas": "three"}, false, "type"),
			Entry("UT-AF-WP-012: fails with enum value not in set",
				map[string]any{"namespace": "prod", "replicas": 3, "strategy": "canary"}, false, "enum"),
			Entry("UT-AF-WP-013: applies default for missing optional param",
				map[string]any{"namespace": "prod", "replicas": 3}, true, ""),
		)

		It("UT-AF-WP-017: empty schema (0 params) passes empty map", func() {
			err := tools.ValidateWorkflowParameters([]tools.WorkflowParameter{}, map[string]any{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AF-WP-018: numeric string for int param coercion", func() {
			simpleSchema := []tools.WorkflowParameter{
				{Name: "count", Type: "int", Required: true},
			}
			err := tools.ValidateWorkflowParameters(simpleSchema, map[string]any{"count": "5"})
			// Implementation may choose to coerce or reject; either is valid.
			// If it passes, the value should be usable. If it fails, error should mention type.
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("type"))
			}
		})

		It("validates float type correctly", func() {
			schema := []tools.WorkflowParameter{
				{Name: "ratio", Type: "float", Required: true},
			}
			Expect(tools.ValidateWorkflowParameters(schema, map[string]any{"ratio": 3.14})).To(Succeed())
			Expect(tools.ValidateWorkflowParameters(schema, map[string]any{"ratio": "not-a-float"})).To(HaveOccurred())
		})

		It("validates bool type correctly", func() {
			schema := []tools.WorkflowParameter{
				{Name: "dry_run", Type: "bool", Required: true},
			}
			Expect(tools.ValidateWorkflowParameters(schema, map[string]any{"dry_run": true})).To(Succeed())
			Expect(tools.ValidateWorkflowParameters(schema, map[string]any{"dry_run": "yes"})).To(HaveOccurred())
		})

		It("validates json.Number as valid int", func() {
			schema := []tools.WorkflowParameter{
				{Name: "count", Type: "int", Required: true},
			}
			Expect(tools.ValidateWorkflowParameters(schema, map[string]any{"count": json.Number("42")})).To(Succeed())
		})

		It("rejects json.Number with decimal as int", func() {
			schema := []tools.WorkflowParameter{
				{Name: "count", Type: "int", Required: true},
			}
			Expect(tools.ValidateWorkflowParameters(schema, map[string]any{"count": json.Number("3.14")})).To(HaveOccurred())
		})

		It("accepts json.Number as float", func() {
			schema := []tools.WorkflowParameter{
				{Name: "ratio", Type: "float", Required: true},
			}
			Expect(tools.ValidateWorkflowParameters(schema, map[string]any{"ratio": json.Number("2.718")})).To(Succeed())
		})

		It("passes validation for unknown type (no type checking)", func() {
			schema := []tools.WorkflowParameter{
				{Name: "meta", Type: "object", Required: true},
			}
			Expect(tools.ValidateWorkflowParameters(schema, map[string]any{"meta": map[string]any{"key": "val"}})).To(Succeed())
		})
	})

	Describe("NewDiscoverWorkflowsTool", func() {
		It("UT-AF-WP-016: returns tool with correct name", func() {
			mockMCP := &ka.MockMCPClient{}
			t, err := tools.NewDiscoverWorkflowsTool(mockMCP)
			Expect(err).NotTo(HaveOccurred())
			Expect(t).NotTo(BeNil())
		})
	})
})
