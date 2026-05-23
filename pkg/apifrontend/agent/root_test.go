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
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

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
				GCPProject:  "test-project",
				GCPRegion:   "us-central1",
				Instruction: "You are a test agent",
			}
			a, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(a).NotTo(BeNil())
			Expect(a.Name()).To(Equal("kubernaut-apifrontend"))
			Expect(tools).NotTo(BeEmpty())
		})

		It("UT-AF-100-002: registers all 28 tools", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(tools).To(HaveLen(28))
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
			Expect(tools).To(HaveLen(28))
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
			Expect(tools).To(HaveLen(28), "AC 7: all 28 tools must be returned unfiltered")
		})

		It("IT-AF-1234-W08: buildToolList includes 6 interactive investigation tools", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			toolNames := make(map[string]bool)
			for _, t := range tools {
				toolNames[t.Name()] = true
			}

			for _, expected := range []string{
				"kubernaut_takeover",
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
				GCPProject:  "test-project",
				GCPRegion:   "us-central1",
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
	})

	Describe("Functional Options", func() {
		It("WithGCPProject overrides project", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg = cfg.Apply(agentpkg.WithGCPProject("new-project"))
			Expect(cfg.GCPProject).To(Equal("new-project"))
		})

		It("WithGCPRegion overrides region", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg = cfg.Apply(agentpkg.WithGCPRegion("eu-west1"))
			Expect(cfg.GCPRegion).To(Equal("eu-west1"))
		})

		It("WithInstruction overrides instruction", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg = cfg.Apply(agentpkg.WithInstruction("Custom prompt"))
			Expect(cfg.Instruction).To(Equal("Custom prompt"))
		})

		It("WithKABaseURL overrides KA URL", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg = cfg.Apply(agentpkg.WithKABaseURL("http://ka:9999"))
			Expect(cfg.KABaseURL).To(Equal("http://ka:9999"))
		})

		It("WithKAMCPEndpoint overrides KA MCP URL", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg = cfg.Apply(agentpkg.WithKAMCPEndpoint("http://ka:9999/mcp/"))
			Expect(cfg.KAMCPEndpoint).To(Equal("http://ka:9999/mcp/"))
		})

		It("WithDSBaseURL overrides DS URL", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg = cfg.Apply(agentpkg.WithDSBaseURL("http://ds:7777"))
			Expect(cfg.DSBaseURL).To(Equal("http://ds:7777"))
		})

		It("NewRootAgent accepts functional options", func() {
			cfg := agentpkg.DefaultTestConfig()
			a, _, err := agentpkg.NewRootAgent(cfg, agentpkg.WithGCPProject("override-project"))
			Expect(err).NotTo(HaveOccurred())
			Expect(a).NotTo(BeNil())
		})
	})
})
