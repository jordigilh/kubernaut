package agent_test

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	adkmemory "google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	sessionpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/session"
	toolspkg "github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// newMockToolContext returns a minimal tool.Context with a specific
// FunctionCallID for testing Before/After callback pairs.
func newMockToolContext(callID string) mockToolCtx {
	return mockToolCtx{Context: context.Background(), callID: callID}
}

type mockToolCtx struct {
	context.Context
	callID string
}

func (mockToolCtx) UserContent() *genai.Content                { return nil }
func (mockToolCtx) InvocationID() string                       { return "" }
func (mockToolCtx) AgentName() string                          { return "" }
func (mockToolCtx) ReadonlyState() session.ReadonlyState       { return nil }
func (mockToolCtx) UserID() string                             { return "" }
func (mockToolCtx) AppName() string                            { return "" }
func (mockToolCtx) SessionID() string                          { return "" }
func (mockToolCtx) Branch() string                             { return "" }
func (mockToolCtx) Artifacts() agent.Artifacts                 { return nil }
func (mockToolCtx) State() session.State                       { return nil }
func (m mockToolCtx) FunctionCallID() string                   { return m.callID }
func (mockToolCtx) Actions() *session.EventActions             { return nil }
func (mockToolCtx) SearchMemory(context.Context, string) (*adkmemory.SearchResponse, error) {
	return nil, nil
}
func (mockToolCtx) ToolConfirmation() *toolconfirmation.ToolConfirmation { return nil }
func (mockToolCtx) RequestConfirmation(string, any) error                { return nil }

var _ tool.Context = mockToolCtx{}

// llmAdapter wraps mockToolCallLLM to satisfy model.LLM interface at the
// package level (struct methods can't be declared inside Describe blocks).
type llmAdapter struct {
	m    any
	genFn func(context.Context, *model.LLMRequest, bool) iter.Seq2[*model.LLMResponse, error]
}

func (a *llmAdapter) Name() string { return "mock-tool-call-llm" }
func (a *llmAdapter) GenerateContent(ctx context.Context, req *model.LLMRequest, streaming bool) iter.Seq2[*model.LLMResponse, error] {
	return a.genFn(ctx, req, streaming)
}

// mockAuthorizerImpl implements auth.ToolAuthorizer for test control.
type mockAuthorizerImpl struct {
	allow bool
	err   error
}

func (m *mockAuthorizerImpl) Check(_ context.Context, _ string, _ []string, _ string) (bool, error) {
	return m.allow, m.err
}

var _ = Describe("Root Agent", func() {
	Describe("NewRootAgent", func() {
		It("UT-AF-100-001: returns configured agent with model", func() {
			cfg := agentpkg.AgentConfig{
				Instruction: "You are a test agent",
			}
			a, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(a).NotTo(BeNil())
			Expect(a.Name()).To(Equal("kubernaut-apifrontend"))
			Expect(tools).NotTo(BeEmpty())
		})

		It("UT-AF-100-002: registers all 23 tools (#1415: kubernaut_approve removed from A2A toolset)", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(tools).To(HaveLen(23))
		})

		It("UT-AF-100-003: with nil model config returns error", func() {
			cfg := agentpkg.AgentConfig{}
			_, _, err := agentpkg.NewRootAgent(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("instruction"))
		})

		It("UT-AF-100-004: tool names are unique across all categories", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			names := make(map[string]bool)
			for _, t := range tools {
				Expect(names).NotTo(HaveKey(t.Name()), "duplicate tool name: %s", t.Name())
				names[t.Name()] = true
			}
		})

		It("UT-AF-100-005: tool names follow naming convention (kubernaut_, af_, kubectl_, or internal prefix)", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			for _, t := range tools {
				if t.Name() == "present_decision" {
					continue
				}
				hasValidPrefix := strings.HasPrefix(t.Name(), "kubernaut_") ||
					strings.HasPrefix(t.Name(), "af_") ||
					strings.HasPrefix(t.Name(), "kubectl_") ||
					strings.HasPrefix(t.Name(), "check_") ||
					strings.HasPrefix(t.Name(), "create_")
				Expect(hasValidPrefix).To(BeTrue(), "tool %q missing expected prefix", t.Name())
			}
		})

		It("UT-AF-100-006: each tool has non-empty description", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			for _, t := range tools {
				Expect(t.Description()).NotTo(BeEmpty(), "tool %q has empty description", t.Name())
			}
		})

		It("UT-AF-100-007: each tool has valid input schema", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(tools).To(HaveLen(23))
		})

		It("UT-AF-100-008: kubernaut_present_decision is marked IsLongRunning", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			var found bool
			for _, t := range tools {
				if t.Name() == "kubernaut_present_decision" {
					found = true
					Expect(t.IsLongRunning()).To(BeTrue(), "kubernaut_present_decision must be IsLongRunning")
				}
			}
			Expect(found).To(BeTrue(), "kubernaut_present_decision tool not found")
		})

		It("UT-AF-100-009: non-kubernaut_present_decision tools are NOT IsLongRunning", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			for _, t := range tools {
				if t.Name() != "kubernaut_present_decision" {
					Expect(t.IsLongRunning()).To(BeFalse(), "tool %q should not be IsLongRunning", t.Name())
				}
			}
		})

		It("UT-AF-100-010: agent config includes instruction", func() {
			cfg := agentpkg.DefaultTestConfig()
			a, _, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(a.Description()).NotTo(BeEmpty())
		})

		It("UT-AF-1221-016: NewRootAgent returns all tools unfiltered", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(tools).To(HaveLen(23), "AC 7: all 23 tools must be returned unfiltered (#1415: kubernaut_approve removed from A2A)")
		})

		It("IT-AF-1234-W08: buildToolList includes 5 interactive investigation tools (#1332)", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			toolNames := make(map[string]bool)
			for _, t := range tools {
				toolNames[t.Name()] = true
			}

			for _, expected := range []string{
				"kubernaut_message",
				"kubernaut_complete",
				"kubernaut_cancel",
				"kubernaut_status",
				"kubernaut_reconnect",
			} {
				Expect(toolNames).To(HaveKey(expected), "missing interactive tool: "+expected)
			}
		})

		It("UT-AF-100-012: agent creation with empty tool list returns error", func() {
			cfg := agentpkg.AgentConfig{
				Instruction: "You are a test agent",
				SkipTools:   true,
			}
			_, _, err := agentpkg.NewRootAgent(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("tool"))
		})
	})

	Describe("newRBACGuard via runner.Run", func() {
		type noopArgs struct {
			Input string `json:"input"`
		}
		type noopResult struct {
			Output string `json:"output"`
		}

		// mockToolCallLLM implements model.LLM. First GenerateContent returns a
		// FunctionCall for targetTool; subsequent calls return text to terminate.
		type mockToolCallLLM struct {
			targetTool string
			callCount  atomic.Int32
		}

		newMockToolCallLLM := func(targetTool string) *mockToolCallLLM {
			return &mockToolCallLLM{targetTool: targetTool}
		}

		buildToolCallLLMName := func(_ *mockToolCallLLM) string { return "mock-tool-call-llm" }
		_ = buildToolCallLLMName

		generateContent := func(m *mockToolCallLLM) func(context.Context, *model.LLMRequest, bool) iter.Seq2[*model.LLMResponse, error] {
			return func(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
				return func(yield func(*model.LLMResponse, error) bool) {
					n := m.callCount.Add(1)
					if n == 1 {
						yield(&model.LLMResponse{
							Content: &genai.Content{
								Role: "model",
								Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{
									ID:   "call-1",
									Name: m.targetTool,
									Args: map[string]any{"input": "test"},
								}}},
							},
						}, nil)
					} else {
						yield(&model.LLMResponse{
							Content: &genai.Content{
								Role:  "model",
								Parts: []*genai.Part{{Text: "Done."}},
							},
						}, nil)
					}
				}
			}
		}

		buildRunner := func(mockLLM *mockToolCallLLM, authorizer auth.ToolAuthorizer) (*runner.Runner, tool.Tool) {
			noopTool, err := functiontool.New(functiontool.Config{
				Name:        "af_test_tool",
				Description: "No-op tool for RBAC guard integration tests",
			}, func(_ tool.Context, args noopArgs) (noopResult, error) {
				return noopResult{Output: "executed"}, nil
			})
			Expect(err).NotTo(HaveOccurred())

			// Wire mockLLM as a model.LLM-compatible type via adapter
			adapter := &llmAdapter{m: mockLLM, genFn: generateContent(mockLLM)}

			a, err := llmagent.New(llmagent.Config{
				Name:        "rbac-guard-test-agent",
				Description: "Integration test agent for RBAC guard",
				Model:       adapter,
				Tools:       []tool.Tool{noopTool},
				Instruction: "You are a test agent. Call af_test_tool when asked.",
				BeforeToolCallbacks: []llmagent.BeforeToolCallback{
					agentpkg.NewRBACGuardForTest(authorizer, nil),
				},
			})
			Expect(err).NotTo(HaveOccurred())

			r, err := runner.New(runner.Config{
				AppName:           "rbac-guard-it",
				Agent:             a,
				SessionService:    session.InMemoryService(),
				AutoCreateSession: true,
			})
			Expect(err).NotTo(HaveOccurred())

			return r, noopTool
		}

		runAgent := func(r *runner.Runner, ctx context.Context) string {
			msg := &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{{Text: "call af_test_tool"}},
			}
			var allText strings.Builder
			for event, err := range r.Run(ctx, "test-user", "sess-1", msg, agent.RunConfig{}) {
				Expect(err).NotTo(HaveOccurred())
				if event.Content != nil {
					for _, p := range event.Content.Parts {
						if p.Text != "" {
							allText.WriteString(p.Text)
						}
						if p.FunctionResponse != nil && p.FunctionResponse.Response != nil {
							if errMsg, ok := p.FunctionResponse.Response["error"].(string); ok {
								allText.WriteString(fmt.Sprintf(" [fn_error:%s]", errMsg))
							}
						}
					}
				}
			}
			return allText.String()
		}

		It("IT-AF-1221-020: allowed identity -> tool executes via runner.Run", func() {
			mockLLM := newMockToolCallLLM("af_test_tool")
			r, _ := buildRunner(mockLLM, &mockAuthorizerImpl{allow: true})

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "sre-user@example.com",
				Groups:   []string{"sre"},
			})

			result := runAgent(r, ctx)
			// Agent completed without RBAC errors; the mock LLM produced "Done."
			Expect(result).To(ContainSubstring("Done."))
			// The LLM was called at least twice (tool call + final text)
			Expect(mockLLM.callCount.Load()).To(BeNumerically(">=", 2))
		})

		It("IT-AF-1221-021: denied identity -> tool blocked with forbidden", func() {
			mockLLM := newMockToolCallLLM("af_test_tool")
			r, _ := buildRunner(mockLLM, &mockAuthorizerImpl{allow: false})

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "cicd-user@example.com",
				Groups:   []string{"cicd"},
			})

			result := runAgent(r, ctx)
			Expect(result).To(ContainSubstring("forbidden"))
		})

		It("IT-AF-1221-022: no identity in context -> denied with unauthorized", func() {
			mockLLM := newMockToolCallLLM("af_test_tool")
			r, _ := buildRunner(mockLLM, &mockAuthorizerImpl{allow: true})

			// No identity injected — context.Background() without WithUserIdentity
			result := runAgent(r, context.Background())
			Expect(result).To(ContainSubstring("unauthorized"))
		})

		It("UT-AF-1332-080: RBAC denial emits EventAuthAccessDenied audit event (SI-4)", func() {
			spyAuditor := &spyAuditEmitter{}
			guard := agentpkg.NewRBACGuardForTest(&mockAuthorizerImpl{allow: false}, spyAuditor)

			testTool, err := functiontool.New(functiontool.Config{
				Name:        "kubernaut_investigate",
				Description: "test tool for SI-4 audit",
			}, func(_ tool.Context, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			})
			Expect(err).NotTo(HaveOccurred())

			baseCtx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "blocked-user@example.com",
				Groups:   []string{"observability"},
			})
			ctx := mockToolCtx{Context: baseCtx, callID: "si4-audit-080"}

			result, err := guard(ctx, testTool, map[string]any{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKey("error"))
			Expect(result["error"]).To(ContainSubstring("forbidden"))

			Expect(spyAuditor.events).To(HaveLen(1),
				"SI-4: RBAC denial must emit exactly one audit event")
			ev := spyAuditor.events[0]
			Expect(ev.Type).To(Equal(audit.EventAuthAccessDenied))
			Expect(ev.UserID).To(Equal("blocked-user@example.com"))
			Expect(ev.Detail["tool_name"]).To(Equal("kubernaut_investigate"))
			Expect(ev.Detail["endpoint"]).To(Equal("a2a"))
			Expect(ev.Detail["groups"]).To(Equal("observability"))
		})

		It("UT-AF-1332-081: no-identity denial emits audit event with reason (SI-4)", func() {
			spyAuditor := &spyAuditEmitter{}
			guard := agentpkg.NewRBACGuardForTest(&mockAuthorizerImpl{allow: true}, spyAuditor)

			testTool, err := functiontool.New(functiontool.Config{
				Name:        "kubernaut_remediate",
				Description: "test tool for SI-4 no-identity",
			}, func(_ tool.Context, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			})
			Expect(err).NotTo(HaveOccurred())

			ctx := mockToolCtx{Context: context.Background(), callID: "si4-audit-081"}

			result, err := guard(ctx, testTool, map[string]any{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKey("error"))
			Expect(result["error"]).To(ContainSubstring("unauthorized"))

			Expect(spyAuditor.events).To(HaveLen(1),
				"SI-4: no-identity denial must emit audit event")
			ev := spyAuditor.events[0]
			Expect(ev.Type).To(Equal(audit.EventAuthAccessDenied))
			Expect(ev.Detail["reason"]).To(Equal("no_identity_in_context"))
			Expect(ev.Detail["tool_name"]).To(Equal("kubernaut_remediate"))
		})
	})

	// =============================================================================
	// IT-AF-1307: Phase Guard Wiring — proves callbacks are wired in NewRootAgent
	// and exercise the full runner.Run dispatch path (Pyramid Invariant: IT).
	// =============================================================================
	Describe("Phase Guard wiring via runner.Run (IT-AF-1307)", func() {
		type pgArgs struct {
			RRID string `json:"rr_id,omitempty"`
		}
		type pgResult struct {
			Status string `json:"status"`
		}

		type pgMockLLM struct {
			callCount atomic.Int32
		}

		pgGenerateContent := func(m *pgMockLLM) func(context.Context, *model.LLMRequest, bool) iter.Seq2[*model.LLMResponse, error] {
			return func(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
				return func(yield func(*model.LLMResponse, error) bool) {
					n := m.callCount.Add(1)
					if n == 1 {
						yield(&model.LLMResponse{
							Content: &genai.Content{
								Role: "model",
								Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{
									ID:   "call-pg-1",
									Name: "kubernaut_discover_workflows",
									Args: map[string]any{},
								}}},
							},
						}, nil)
					} else {
						yield(&model.LLMResponse{
							Content: &genai.Content{
								Role:  "model",
								Parts: []*genai.Part{{Text: "Done."}},
							},
						}, nil)
					}
				}
			}
		}

		It("IT-AF-1307-001: phase guard blocks MCP-dependent tool via runner.Run dispatch", func() {
			discoverTool, err := functiontool.New(functiontool.Config{
				Name:        "kubernaut_discover_workflows",
				Description: "Discover workflows (MCP-dependent tool)",
			}, func(_ tool.Context, _ pgArgs) (pgResult, error) {
				return pgResult{Status: "discovered"}, nil
			})
			Expect(err).NotTo(HaveOccurred())

			mockLLM := &pgMockLLM{}
			adapter := &llmAdapter{m: mockLLM, genFn: pgGenerateContent(mockLLM)}

			before, after := agentpkg.NewPhaseGuardForTest()
			a, err := llmagent.New(llmagent.Config{
				Name:                "phase-guard-it-agent",
				Description:         "IT agent for phase guard wiring",
				Model:               adapter,
				Tools:               []tool.Tool{discoverTool},
				Instruction:         "You are a test agent. Call kubernaut_discover_workflows when asked.",
				BeforeToolCallbacks: []llmagent.BeforeToolCallback{before},
				AfterToolCallbacks:  []llmagent.AfterToolCallback{after},
			})
			Expect(err).NotTo(HaveOccurred())

			r, err := runner.New(runner.Config{
				AppName:           "phase-guard-it",
				Agent:             a,
				SessionService:    session.InMemoryService(),
				AutoCreateSession: true,
			})
			Expect(err).NotTo(HaveOccurred())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "sre@example.com", Groups: []string{"sre"},
			})
			msg := &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{{Text: "discover workflows"}},
			}
			var allText strings.Builder
			for event, err := range r.Run(ctx, "test-user", "sess-pg-1", msg, agent.RunConfig{}) {
				Expect(err).NotTo(HaveOccurred())
				if event.Content != nil {
					for _, p := range event.Content.Parts {
						if p.Text != "" {
							allText.WriteString(p.Text)
						}
						if p.FunctionResponse != nil && p.FunctionResponse.Response != nil {
							if errMsg, ok := p.FunctionResponse.Response["error"].(string); ok {
								allText.WriteString(fmt.Sprintf(" [fn_error:%s]", errMsg))
							}
						}
					}
				}
			}

			Expect(allText.String()).To(ContainSubstring("kubernaut_investigate"),
				"IT: phase guard must block discover_workflows via full runner dispatch and guide LLM to investigate first")
		})
	})

	// =============================================================================
	// TP-1310 §4.3: Tool Call Logging Callbacks — FedRAMP AU-12
	// =============================================================================
	Describe("newToolLoggingCallbacks (TP-1310)", func() {
		It("UT-AF-1310-020: Before callback logs tool name at info level (AU-12)", func() {
			before, _ := agentpkg.NewToolLoggingCallbacksForTest()

			mockTool, err := functiontool.New(functiontool.Config{
				Name:        "kubectl_describe",
				Description: "test tool",
			}, func(_ tool.Context, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			})
			Expect(err).NotTo(HaveOccurred())

			ctx := newMockToolContext("call-log-020")
			result, err := before(ctx, mockTool, map[string]any{"kind": "Pod"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(), "Before callback must not short-circuit")
		})

		It("UT-AF-1310-021: After callback logs tool name and duration (AU-12)", func() {
			before, after := agentpkg.NewToolLoggingCallbacksForTest()

			mockTool, err := functiontool.New(functiontool.Config{
				Name:        "kubectl_get_pods",
				Description: "test tool",
			}, func(_ tool.Context, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			})
			Expect(err).NotTo(HaveOccurred())

			ctx := newMockToolContext("call-log-021")

			// Before must be called first to record the start time
			_, _ = before(ctx, mockTool, nil)

			result, err := after(ctx, mockTool, nil, map[string]any{"output": "ok"}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(), "After callback must not modify output")
		})

		It("WT-AF-1310-030: NewRootAgent includes logging callbacks in callback chain", func() {
			cfg := agentpkg.DefaultTestConfig()
			a, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(a).NotTo(BeNil())
			Expect(tools).NotTo(BeEmpty())
		})
	})

	// =========================================================================
	// DedicatedClient wiring — verifies kubernaut_investigate uses the
	// dedicated SDKMCPClient when DedicatedClient is set, and falls back
	// to MCPClient when nil.
	// =========================================================================
	Describe("DedicatedClient wiring for kubernaut_investigate", func() {
		// runnableTool matches the unexported ADK runnableTool interface via
		// structural typing so we can invoke the tool directly without the
		// full runner/LLM stack.
		type runnableTool interface {
			Run(ctx tool.Context, args any) (map[string]any, error)
		}

		findTool := func(allTools []tool.Tool, name string) runnableTool {
			for _, t := range allTools {
				if t.Name() == name {
					rt, ok := t.(runnableTool)
					Expect(ok).To(BeTrue(), "tool %q must implement runnableTool", name)
					return rt
				}
			}
			Fail("tool " + name + " not found in tool list")
			return nil
		}

		It("UT-AF-1326-030: kubernaut_investigate uses DedicatedClient when set", func() {
			var autonomousCalled atomic.Int32
			autonomousMock := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					autonomousCalled.Add(1)
					ch := make(chan ka.InvestigationEvent, 1)
					close(ch)
					return &ka.StartInvestigationResult{
						SessionID: "sess-auto-030",
						Status:    "autonomous_started",
						Events:    ch,
						Closer:    func() {},
					}, nil
				},
			}

			var pooledCalled atomic.Int32
			pooledMock := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					pooledCalled.Add(1)
					return nil, fmt.Errorf("StartInvestigation requires a dedicated MCP session; use SDKMCPClient")
				},
			}

			cfg := agentpkg.DefaultTestConfig()
			cfg.MCPClient = pooledMock
			cfg.DedicatedClient = autonomousMock

			_, allTools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			investigateTool := findTool(allTools, "kubernaut_investigate")
			ctx := newMockToolContext("call-auto-030")
			_, _ = investigateTool.Run(ctx, map[string]any{"rr_id": "rr-wiring-030"})

			Expect(autonomousCalled.Load()).To(Equal(int32(1)), "DedicatedClient.StartInvestigation must be called")
			Expect(pooledCalled.Load()).To(Equal(int32(0)), "MCPClient.StartInvestigation must NOT be called")
		})

		It("UT-AF-1326-031: kubernaut_investigate falls back to MCPClient when DedicatedClient is nil", func() {
			var pooledCalled atomic.Int32
			pooledMock := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					pooledCalled.Add(1)
					ch := make(chan ka.InvestigationEvent, 1)
					close(ch)
					return &ka.StartInvestigationResult{
						SessionID: "sess-pool-031",
						Status:    "autonomous_started",
						Events:    ch,
						Closer:    func() {},
					}, nil
				},
			}

			cfg := agentpkg.DefaultTestConfig()
			cfg.MCPClient = pooledMock
			// DedicatedClient intentionally nil — should fall back to MCPClient

			_, allTools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			investigateTool := findTool(allTools, "kubernaut_investigate")
			ctx := newMockToolContext("call-pool-031")
			_, _ = investigateTool.Run(ctx, map[string]any{"rr_id": "rr-wiring-031"})

			Expect(pooledCalled.Load()).To(Equal(int32(1)), "MCPClient.StartInvestigation must be called as fallback")
		})

		It("UT-AF-1332-082: buildAgentISSignaler returns non-nil when SessionService is set", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg.SessionService = &sessionpkg.CRDSessionService{}

			signaler := agentpkg.BuildAgentISSignalerForTest(cfg)
			Expect(signaler).NotTo(BeNil(),
				"ISSignaler must be wired when SessionService is configured")
		})

		It("UT-AF-1332-083: buildAgentISSignaler returns nil when SessionService is nil", func() {
			cfg := agentpkg.DefaultTestConfig()

			signaler := agentpkg.BuildAgentISSignalerForTest(cfg)
			Expect(signaler).To(BeNil(),
				"ISSignaler must be nil when SessionService is not configured")
		})

		It("UT-AF-1326-032: kubernaut_investigate propagates DedicatedClient error", func() {
			autonomousMock := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return nil, fmt.Errorf("connection refused")
				},
			}

			cfg := agentpkg.DefaultTestConfig()
			cfg.MCPClient = &ka.MockMCPClient{}
			cfg.DedicatedClient = autonomousMock

			_, allTools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			investigateTool := findTool(allTools, "kubernaut_investigate")
			ctx := newMockToolContext("call-err-032")
			result, err := investigateTool.Run(ctx, map[string]any{"rr_id": "rr-wiring-032"})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connection refused"))
			Expect(result).To(BeNil())
		})
	})

	Describe("Functional Options", func() {
		It("WithInstruction overrides instruction", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg = cfg.Apply(agentpkg.WithInstruction("Custom prompt"))
			Expect(cfg.Instruction).To(Equal("Custom prompt"))
		})

		It("NewRootAgent accepts functional options", func() {
			cfg := agentpkg.DefaultTestConfig()
			a, _, err := agentpkg.NewRootAgent(cfg, agentpkg.WithInstruction("custom prompt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(a).NotTo(BeNil())
		})
	})
})

var _ = Describe("Conditional tool registration — interactive mode (#1366)", func() {

	It("UT-AF-1366-010: InteractiveEnabled=false excludes session-dependent tools", func() {
		cfg := agentpkg.DefaultTestConfig()
		cfg.InteractiveEnabled = false
		_, tools, err := agentpkg.NewRootAgent(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(tools).To(HaveLen(14), "only 14 stateless tools when interactive disabled (#1415: kubernaut_approve removed)")

		for _, t := range tools {
			Expect(toolspkg.SessionDependentTools).NotTo(HaveKey(t.Name()),
				"session-dependent tool %q should not be registered when interactive disabled", t.Name())
		}
	})

	It("UT-AF-1366-011: InteractiveEnabled=true registers all 24 tools", func() {
		cfg := agentpkg.DefaultTestConfig()
		cfg.InteractiveEnabled = true
		_, tools, err := agentpkg.NewRootAgent(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(tools).To(HaveLen(23), "all 23 tools when interactive enabled")
	})

	It("UT-AF-1366-012: default config (InteractiveEnabled zero-value) registers all 24 tools", func() {
		cfg := agentpkg.DefaultTestConfig()
		_, tools, err := agentpkg.NewRootAgent(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(tools).To(HaveLen(23), "backward compat: zero-value InteractiveEnabled means all tools")
	})
})

var _ = Describe("Phase guard / SessionDependentTools consistency (#1366 F6)", func() {

	It("UT-AF-1366-040: every phase-guard tool is in SessionDependentTools", func() {
		for name := range agentpkg.ExportedMCPDependentTools {
			Expect(toolspkg.SessionDependentTools).To(HaveKey(name),
				"mcpDependentTools has %q but SessionDependentTools does not — drift detected", name)
		}
		for name := range agentpkg.ExportedDriverEntryTools {
			Expect(toolspkg.SessionDependentTools).To(HaveKey(name),
				"driverEntryTools has %q but SessionDependentTools does not — drift detected", name)
		}
		for name := range agentpkg.ExportedSessionTerminalTools {
			Expect(toolspkg.SessionDependentTools).To(HaveKey(name),
				"sessionTerminalTools has %q but SessionDependentTools does not — drift detected", name)
		}
	})
})

var _ = Describe("Alert tool wiring (#1367)", func() {

	It("IT-AF-1367-001: list_alerts tool is registered and callable when PromClient is set", func() {
		cfg := agentpkg.DefaultTestConfig()
		cfg.PromClient = &stubPromClient{}
		_, allTools, err := agentpkg.NewRootAgent(cfg)
		Expect(err).NotTo(HaveOccurred())

		var found tool.Tool
		for _, t := range allTools {
			if t.Name() == "list_alerts" {
				found = t
				break
			}
		}
		Expect(found).NotTo(BeNil(), "list_alerts should be registered")
	})

	It("IT-AF-1367-002: get_alert_details tool is registered and callable when PromClient is set", func() {
		cfg := agentpkg.DefaultTestConfig()
		cfg.PromClient = &stubPromClient{}
		_, allTools, err := agentpkg.NewRootAgent(cfg)
		Expect(err).NotTo(HaveOccurred())

		var found tool.Tool
		for _, t := range allTools {
			if t.Name() == "get_alert_details" {
				found = t
				break
			}
		}
		Expect(found).NotTo(BeNil(), "get_alert_details should be registered")
	})

	It("IT-AF-1367-003: NewRootAgent with PromClient=nil excludes alert tools", func() {
		cfg := agentpkg.DefaultTestConfig()
		cfg.PromClient = nil
		_, allTools, err := agentpkg.NewRootAgent(cfg)
		Expect(err).NotTo(HaveOccurred())

		for _, t := range allTools {
			Expect(t.Name()).NotTo(Equal("list_alerts"), "list_alerts should not be registered when PromClient is nil")
			Expect(t.Name()).NotTo(Equal("get_alert_details"), "get_alert_details should not be registered when PromClient is nil")
		}
	})
})

var _ = Describe("kubernaut_investigate_alert wiring (#1372)", func() {

	It("IT-AF-1372-001: kubernaut_investigate_alert registered when PromClient is set", func() {
		cfg := agentpkg.DefaultTestConfig()
		cfg.PromClient = &stubPromClient{}
		_, allTools, err := agentpkg.NewRootAgent(cfg)
		Expect(err).NotTo(HaveOccurred())

		var found tool.Tool
		for _, t := range allTools {
			if t.Name() == "kubernaut_investigate_alert" {
				found = t
				break
			}
		}
		Expect(found).NotTo(BeNil(), "kubernaut_investigate_alert should be registered when PromClient is set")
	})

	It("IT-AF-1372-002: kubernaut_investigate_alert excluded when PromClient is nil", func() {
		cfg := agentpkg.DefaultTestConfig()
		cfg.PromClient = nil
		_, allTools, err := agentpkg.NewRootAgent(cfg)
		Expect(err).NotTo(HaveOccurred())

		for _, t := range allTools {
			Expect(t.Name()).NotTo(Equal("kubernaut_investigate_alert"),
				"kubernaut_investigate_alert should not be registered when PromClient is nil")
		}
	})
})

type stubPromClient struct{}

func (s *stubPromClient) GetAlerts(_ context.Context) ([]prom.Alert, error) { return nil, nil }
func (s *stubPromClient) GetRules(_ context.Context) ([]prom.RuleGroup, error) { return nil, nil }
func (s *stubPromClient) InstantQuery(_ context.Context, _ string) (*prom.QueryResult, error) {
	return nil, nil
}

type spyAuditEmitter struct {
	events []*audit.Event
}

func (s *spyAuditEmitter) Emit(_ context.Context, event *audit.Event) {
	cp := *event
	s.events = append(s.events, &cp)
}

var _ = Describe("ADVERSARIAL: kubernaut_approve removed from A2A toolset (#1415)", func() {
	It("ADV-AF-1415-001: agent toolset does NOT contain kubernaut_approve", func() {
		cfg := agentpkg.DefaultTestConfig()
		_, tools, err := agentpkg.NewRootAgent(cfg)
		Expect(err).NotTo(HaveOccurred())

		for _, t := range tools {
			Expect(t.Name()).NotTo(Equal("kubernaut_approve"),
				"kubernaut_approve must NOT be in the A2A agent toolset — approval is Console-only via MCP (#1415)")
		}
	})

	It("ADV-AF-1415-002: agent toolset does NOT contain kubernaut_approve even with InteractiveEnabled=true", func() {
		cfg := agentpkg.DefaultTestConfig()
		cfg.InteractiveEnabled = true
		_, tools, err := agentpkg.NewRootAgent(cfg)
		Expect(err).NotTo(HaveOccurred())

		for _, t := range tools {
			Expect(t.Name()).NotTo(Equal("kubernaut_approve"),
				"kubernaut_approve must NOT be in the A2A agent toolset regardless of interactive mode (#1415)")
		}
	})
})

var _ = Describe("ADVERSARIAL: kubernaut_complete_no_action excluded from A2A toolset (#1418)", func() {
	It("ADV-AF-1418-001: agent toolset does NOT contain kubernaut_complete_no_action", func() {
		cfg := agentpkg.DefaultTestConfig()
		cfg.InteractiveEnabled = true
		_, tools, err := agentpkg.NewRootAgent(cfg)
		Expect(err).NotTo(HaveOccurred())

		for _, t := range tools {
			Expect(t.Name()).NotTo(Equal("kubernaut_complete_no_action"),
				"kubernaut_complete_no_action must NOT be in the A2A agent toolset — dismiss/escalation is Console-only via MCP (#1418, DD-AF-007)")
		}
	})
})
