package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

type spyAuditor struct {
	events []*audit.Event
}

func (s *spyAuditor) Emit(_ context.Context, e *audit.Event) {
	s.events = append(s.events, e)
}

var _ = Describe("Interactive Action Handlers (G1)", func() {
	var (
		ctx     context.Context
		mockMCP *ka.MockMCPClient
		spy     *spyAuditor
	)

	BeforeEach(func() {
		ctx = context.Background()
		spy = &spyAuditor{}
	})

	Describe("HandleMessage", func() {
		It("UT-AF-1234-034: happy path with message text", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, args ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					Expect(args.Action).To(Equal("message"))
					Expect(args.Message).To(Equal("Check pod logs for OOM"))
					return &ka.InvokeActionResult{Status: "active"}, nil
				},
			}
			result, err := tools.HandleMessage(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID:    "rr-prod-001",
				Message: "Check pod logs for OOM",
			}, spy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("active"))
		})

		It("UT-AF-1234-035: empty message rejected", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					return &ka.InvokeActionResult{Status: "active"}, nil
				},
			}
			_, err := tools.HandleMessage(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID:    "rr-prod-001",
				Message: "",
			}, spy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("message"))
		})

		It("UT-AF-1234-036: KA error returns user-friendly message", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					return nil, ka.ErrMCPUnavailable
				},
			}
			_, err := tools.HandleMessage(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID:    "rr-prod-001",
				Message: "test",
			}, spy)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("HandleComplete", func() {
		It("UT-AF-1234-037: happy path returns completed status", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, args ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					Expect(args.Action).To(Equal("complete"))
					return &ka.InvokeActionResult{Status: "completed"}, nil
				},
			}
			result, err := tools.HandleComplete(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID: "rr-prod-001",
			}, spy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))
		})

		It("UT-AF-1234-038: KA error", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					return nil, ka.ErrMCPUnavailable
				},
			}
			_, err := tools.HandleComplete(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID: "rr-prod-001",
			}, spy)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("HandleCancel", func() {
		It("UT-AF-1234-039: happy path returns cancelled status", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, args ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					Expect(args.Action).To(Equal("cancel"))
					return &ka.InvokeActionResult{Status: "cancelled"}, nil
				},
			}
			result, err := tools.HandleCancel(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID: "rr-prod-001",
			}, spy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("cancelled"))
		})

		It("UT-AF-1234-040: KA error", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					return nil, ka.ErrMCPUnavailable
				},
			}
			_, err := tools.HandleCancel(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID: "rr-prod-001",
			}, spy)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("HandleStatus", func() {
		It("UT-AF-1234-041: happy path returns session state", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, args ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					Expect(args.Action).To(Equal("status"))
					return &ka.InvokeActionResult{Status: "active", SessionID: "s-001"}, nil
				},
			}
			result, err := tools.HandleStatus(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID: "rr-prod-001",
			}, spy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("active"))
		})

		It("UT-AF-1234-042: KA error", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					return nil, ka.ErrMCPUnavailable
				},
			}
			_, err := tools.HandleStatus(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID: "rr-prod-001",
			}, spy)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("HandleReconnect", func() {
		It("UT-AF-1234-043: happy path returns reconnected status", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, args ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					Expect(args.Action).To(Equal("reconnect"))
					return &ka.InvokeActionResult{Status: "active"}, nil
				},
			}
			result, err := tools.HandleReconnect(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID: "rr-prod-001",
			}, spy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("active"))
		})

		It("UT-AF-1234-044: KA error", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					return nil, ka.ErrMCPUnavailable
				},
			}
			_, err := tools.HandleReconnect(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID: "rr-prod-001",
			}, spy)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Rename (G8)", func() {
		It("UT-AF-1234-059: present_decision ADK tool name is kubernaut_present_decision", func() {
			t, err := tools.NewPresentDecisionTool()
			Expect(err).NotTo(HaveOccurred())
			Expect(t.Name()).To(Equal("kubernaut_present_decision"))
		})
	})

	Describe("Constructors (G1)", func() {
		It("UT-AF-1234-060: NewInvestigateMCPTool constructor returns valid tool", func() {
			t, err := tools.NewInvestigateMCPTool(nil, nil, "", nil, nil, nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.Name()).To(Equal("kubernaut_investigate"))
		})

		It("UT-AF-1234-061: Interactive tool constructors return valid tools", func() {
			constructors := []struct {
				name string
				fn   func() (interface{ Name() string }, error)
			}{
			{"kubernaut_message", func() (interface{ Name() string }, error) { return tools.NewMessageTool(nil, nil) }},
				{"kubernaut_complete", func() (interface{ Name() string }, error) { return tools.NewCompleteTool(nil, nil) }},
				{"kubernaut_cancel", func() (interface{ Name() string }, error) { return tools.NewCancelInvestigationTool(nil, nil) }},
				{"kubernaut_status", func() (interface{ Name() string }, error) { return tools.NewStatusTool(nil, nil) }},
				{"kubernaut_reconnect", func() (interface{ Name() string }, error) { return tools.NewReconnectTool(nil, nil) }},
			}
			for _, c := range constructors {
				t, err := c.fn()
				Expect(err).NotTo(HaveOccurred(), "constructor for %s", c.name)
				Expect(t.Name()).To(Equal(c.name))
			}
		})
	})

	Describe("Audit emission", func() {
		// invokeForAudit is a test helper that invokes a handler and returns the emitted audit event.
		invokeForAudit := func(
			handler func(context.Context, ka.MCPClient, tools.InteractiveActionArgs, audit.Emitter) (tools.InteractiveActionResult, error),
			args tools.InteractiveActionArgs,
		) *audit.Event {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					return &ka.InvokeActionResult{Status: "active", SessionID: "s-001"}, nil
				},
			}
			_, err := handler(ctx, mockMCP, args, spy)
			Expect(err).NotTo(HaveOccurred())
			Expect(spy.events).To(HaveLen(1))
			return spy.events[0]
		}

		It("UT-AF-1234-064: HandleMessage emits EventToolExecuted audit", func() {
			ev := invokeForAudit(tools.HandleMessage, tools.InteractiveActionArgs{RRID: "rr-prod-001", Message: "test message"})
			Expect(ev.Type).To(Equal(audit.EventToolExecuted))
		})

		type auditDetailCase struct {
			testID  string
			handler func(context.Context, ka.MCPClient, tools.InteractiveActionArgs, audit.Emitter) (tools.InteractiveActionResult, error)
			args    tools.InteractiveActionArgs
			key     string
			value   string
		}

		auditCases := []auditDetailCase{
			{"UT-AF-1300-001", tools.HandleStatus, tools.InteractiveActionArgs{RRID: "rr-prod-001"}, "tool_outcome", "success"},
			{"UT-AF-1300-002", tools.HandleMessage, tools.InteractiveActionArgs{RRID: "rr-prod-001", Message: "check logs"}, "tool_outcome", "success"},
			{"UT-AF-AUDIT-001", tools.HandleComplete, tools.InteractiveActionArgs{RRID: "rr-prod-001"}, "result_type", "rca_complete"},
			{"UT-AF-AUDIT-002", tools.HandleCancel, tools.InteractiveActionArgs{RRID: "rr-prod-001"}, "result_type", "cancelled"},
			{"UT-AF-AUDIT-003", tools.HandleMessage, tools.InteractiveActionArgs{RRID: "rr-prod-001", Message: "check logs"}, "tool_name", "kubernaut_message"},
			{"UT-AF-AUDIT-004", tools.HandleStatus, tools.InteractiveActionArgs{RRID: "rr-prod-001"}, "tool_name", "kubernaut_status"},
		}

		for _, tc := range auditCases {
			tc := tc
			It(tc.testID+": audit detail["+tc.key+"]="+tc.value, func() {
				ev := invokeForAudit(tc.handler, tc.args)
				Expect(ev.Detail).To(HaveKeyWithValue(tc.key, tc.value),
					"OpenAPI schema requires %s=%s in audit event", tc.key, tc.value)
			})
		}
	})
})
