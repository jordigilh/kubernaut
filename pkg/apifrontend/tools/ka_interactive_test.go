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

	Describe("HandleTakeover", func() {
		It("UT-AF-1234-031: happy path returns session_id + status", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, args ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					Expect(args.Action).To(Equal("takeover"))
					Expect(args.RRID).To(Equal("prod/rr-001"))
					return &ka.InvokeActionResult{SessionID: "s-001", Status: "active"}, nil
				},
			}
			result, err := tools.HandleTakeover(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID: "prod/rr-001",
			}, spy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("s-001"))
			Expect(result.Status).To(Equal("active"))
		})

		It("UT-AF-1234-032: KA error returns user-friendly message", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					return nil, ka.ErrMCPUnavailable
				},
			}
			_, err := tools.HandleTakeover(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID: "prod/rr-001",
			}, spy)
			Expect(err).To(HaveOccurred())
		})

		It("UT-AF-1234-033: nil MCPClient returns error", func() {
			_, err := tools.HandleTakeover(ctx, nil, tools.InteractiveActionArgs{
				RRID: "prod/rr-001",
			}, spy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not available"))
		})
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
				RRID:    "prod/rr-001",
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
				RRID:    "prod/rr-001",
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
				RRID:    "prod/rr-001",
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
				RRID: "prod/rr-001",
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
				RRID: "prod/rr-001",
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
				RRID: "prod/rr-001",
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
				RRID: "prod/rr-001",
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
				RRID: "prod/rr-001",
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
				RRID: "prod/rr-001",
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
				RRID: "prod/rr-001",
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
				RRID: "prod/rr-001",
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
		It("UT-AF-1234-060: NewStreamInvestigationTool constructor returns valid tool", func() {
			t, err := tools.NewStreamInvestigationTool(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.Name()).To(Equal("kubernaut_stream_investigation"))
		})

		It("UT-AF-1234-061: Interactive tool constructors return valid tools", func() {
			constructors := []struct {
				name string
				fn   func() (interface{ Name() string }, error)
			}{
				{"kubernaut_takeover", func() (interface{ Name() string }, error) { return tools.NewTakeoverTool(nil, nil) }},
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
		It("UT-AF-1234-063: HandleTakeover emits EventKADelegated audit", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					return &ka.InvokeActionResult{Status: "active"}, nil
				},
			}
			_, err := tools.HandleTakeover(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID: "prod/rr-001",
			}, spy)
			Expect(err).NotTo(HaveOccurred())
			Expect(spy.events).NotTo(BeEmpty())
			Expect(spy.events[0].Type).To(Equal(audit.EventKADelegated))
		})

		It("UT-AF-1234-064: HandleMessage emits EventToolExecuted audit", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					return &ka.InvokeActionResult{Status: "active"}, nil
				},
			}
			_, err := tools.HandleMessage(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID:    "prod/rr-001",
				Message: "test message",
			}, spy)
			Expect(err).NotTo(HaveOccurred())
			Expect(spy.events).NotTo(BeEmpty())
		})

		It("UT-AF-1300-001: HandleStatus audit includes tool_outcome=success for OpenAPI compliance", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					return &ka.InvokeActionResult{Status: "active", SessionID: "s-001"}, nil
				},
			}
			_, err := tools.HandleStatus(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID: "prod/rr-001",
			}, spy)
			Expect(err).NotTo(HaveOccurred())
			Expect(spy.events).To(HaveLen(1))
			Expect(spy.events[0].Detail).To(HaveKeyWithValue("tool_outcome", "success"),
				"EventToolExecuted audit must include tool_outcome for OpenAPI validation — missing field causes 'not one of allowed values' error")
		})

		It("UT-AF-1300-002: HandleMessage audit includes tool_outcome=success", func() {
			mockMCP = &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					return &ka.InvokeActionResult{Status: "active"}, nil
				},
			}
			_, err := tools.HandleMessage(ctx, mockMCP, tools.InteractiveActionArgs{
				RRID:    "prod/rr-001",
				Message: "check logs",
			}, spy)
			Expect(err).NotTo(HaveOccurred())
			Expect(spy.events).To(HaveLen(1))
			Expect(spy.events[0].Detail).To(HaveKeyWithValue("tool_outcome", "success"))
		})
	})
})
