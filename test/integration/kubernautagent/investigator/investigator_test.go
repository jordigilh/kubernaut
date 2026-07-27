/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package investigator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	sha256FixtureHash = "sha256:fixture-hash"
)

// staticCatalogFetcher returns a pre-built validator for tests that need
// workflow validation without a real DataStorage backend (#665).
type staticCatalogFetcher struct {
	validator *parser.Validator
}

func (f *staticCatalogFetcher) FetchValidator(_ context.Context) (*parser.Validator, error) {
	return f.validator, nil
}

type mockLLMClient struct {
	calls     []llm.ChatRequest
	responses []llm.ChatResponse
	callIdx   int
}

func (m *mockLLMClient) StreamChat(ctx context.Context, req llm.ChatRequest, cb func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	resp, err := m.Chat(ctx, req)
	if err == nil {
		_ = cb(llm.ChatStreamEvent{Delta: resp.Message.Content, Done: true})
	}
	return resp, err
}

func (m *mockLLMClient) Close() error { return nil }

func (m *mockLLMClient) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.calls = append(m.calls, req)
	if m.callIdx < len(m.responses) {
		resp := m.responses[m.callIdx]
		m.callIdx++
		return resp, nil
	}
	return llm.ChatResponse{
		Message: llm.Message{
			Role:    "assistant",
			Content: `{"rca_summary":"no more responses","confidence":0.1}`,
		},
	}, nil
}

// k8sFixtureClient provides deterministic owner chains for testing investigator
// logic that depends on enrichment context. This is a TEST FIXTURE provider —
// not a DataStorage mock. The real K8s adapter is available as suiteK8sAdapter
// for tests that don't need controlled owner chains.
type k8sFixtureClient struct {
	ownerChain []enrichment.OwnerChainEntry
	err        error
}

func (f *k8sFixtureClient) GetOwnerChain(_ context.Context, _, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	return f.ownerChain, f.err
}

func (f *k8sFixtureClient) GetSpecHash(_ context.Context, _, _, _, _ string) (string, error) {
	return sha256FixtureHash, nil
}

// resourceAwareFixtureClient returns different owner chains based on resource name.
// Used for tests that exercise cross-target re-enrichment logic.
type resourceAwareFixtureClient struct {
	chains map[string][]enrichment.OwnerChainEntry
}

func (r *resourceAwareFixtureClient) GetOwnerChain(_ context.Context, _, name, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	if chain, ok := r.chains[name]; ok {
		return chain, nil
	}
	return nil, nil
}

func (r *resourceAwareFixtureClient) GetSpecHash(_ context.Context, _, _, _, _ string) (string, error) {
	return sha256FixtureHash, nil
}

type fakeTool struct {
	name   string
	result string
	err    error
}

func (f *fakeTool) Name() string                { return f.name }
func (f *fakeTool) Description() string         { return "fake " + f.name }
func (f *fakeTool) Parameters() json.RawMessage { return nil }
func (f *fakeTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return f.result, f.err
}

func filterEvents(events []*audit.AuditEvent, eventType string) []*audit.AuditEvent {
	var filtered []*audit.AuditEvent
	for _, e := range events {
		if e.EventType == eventType {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

var _ = Describe("Kubernaut Agent Investigator Integration — #433", func() {

	var (
		invLogger  logr.Logger
		auditStore *capturingAuditStore
		mockClient *mockLLMClient
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = newCapturingAuditStore(suiteAuditStore)
		mockClient = &mockLLMClient{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		enricher = enrichment.NewEnricher(suiteK8sAdapter, suiteDSAdapter, auditStore, invLogger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-433-005: Two-invocation investigation produces RCA then workflow", func() {
		It("should return an InvestigationResult with both RCA summary and workflow_id", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled due to memory limit exceeded"}`}},
				wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.9,"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production"}}`),
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:        "api-server-abc",
				Namespace:   "production",
				Severity:    "critical",
				Environment: "Production",
				Priority:    "P0",
				Message:     "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil(), "Investigate should return a result")
			Expect(result.RCASummary).To(ContainSubstring("OOMKilled"))
			Expect(result.WorkflowID).To(Equal("oom-increase-memory"))
		})
	})

	Describe("IT-KA-433-006: Investigation uses LLM loop with stub tool execution", func() {
		It("should make 2 LLM calls (RCA + workflow)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"issue found"}`}},
				wfToolResp(`{"workflow_id":"restart-pod","confidence":0.8}`),
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "pod-abc", Namespace: "default", Severity: "warning", Message: "CrashLoopBackOff",
				Environment: "Development", Priority: "P2",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2), "should make 2 LLM calls (RCA + workflow)")
		})
	})

	Describe("IT-KA-433-007: Investigation preserves interaction history", func() {
		It("should pass RCA context into the workflow selection invocation", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"memory leak in api-server container"}`}},
				wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.88}`),
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", Severity: "critical", Message: "OOMKilled",
				Environment: "Production", Priority: "P0",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))
			if len(mockClient.calls) >= 2 {
				secondCallContent := allMessageContent(mockClient.calls[1].Messages)
				Expect(secondCallContent).To(ContainSubstring("memory leak"),
					"workflow selection invocation should reference RCA findings")
			}
		})
	})

	Describe("IT-KA-433-008: Investigation stops at max turns and returns human-review", func() {
		It("should return HumanReviewNeeded when max turns exhausted", func() {
			mockClient.responses = []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant", Content: "I need more information"},
					ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"api","namespace":"default"}`}},
				},
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 1, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "critical", Message: "OOMKilled",
				Environment: "Development", Priority: "P1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"investigation should require human review when max turns exhausted")
		})
	})

	Describe("IT-KA-433-210: Investigation uses registry.Execute for tool calls", func() {
		It("should pass tool call arguments to registry and return the result to LLM", func() {
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_describe", result: `{"kind":"Pod","metadata":{"name":"api"}}`})

			mockClient.responses = []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant", Content: "Let me check"},
					ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"api","namespace":"default"}`}},
				},
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Pod api is healthy"}`}},
				wfToolResp(`{"workflow_id":"generic-restart","confidence":0.7}`),
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
				Environment: "Development", Priority: "P2",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(mockClient.calls).To(HaveLen(3))
			secondCall := mockClient.calls[1]
			toolMsg := secondCall.Messages[len(secondCall.Messages)-1]
			Expect(toolMsg.Role).To(Equal("tool"))
			Expect(toolMsg.Content).To(ContainSubstring("api"))
			Expect(toolMsg.Content).NotTo(ContainSubstring("no registry"))
		})
	})

	Describe("IT-KA-433-211: Tool execution errors return JSON error to LLM", func() {
		It("should not abort the loop when a tool returns an error", func() {
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_describe", err: fmt.Errorf("connection refused")})

			mockClient.responses = []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant", Content: "checking"},
					ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "kubectl_describe", Arguments: `{}`}},
				},
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"could not investigate"}`}},
				wfToolResp(`{"workflow_id":"restart","confidence":0.5}`),
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "critical", Message: "OOMKilled",
				Environment: "Development", Priority: "P1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			secondCall := mockClient.calls[1]
			toolMsg := secondCall.Messages[len(secondCall.Messages)-1]
			Expect(toolMsg.Role).To(Equal("tool"))
			Expect(toolMsg.Content).To(ContainSubstring("error"))
			Expect(toolMsg.Content).To(ContainSubstring("connection refused"))
		})
	})

	Describe("IT-KA-433-212: ChatRequest includes ToolDefinitions for the active phase", func() {
		It("should send RCA-phase tool definitions in the first LLM call", func() {
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_describe", result: `{}`})

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"found issue"}`}},
				wfToolResp(`{"workflow_id":"restart","confidence":0.7}`),
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
				Environment: "Development", Priority: "P2",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))

			rcaCall := mockClient.calls[0]
			Expect(rcaCall.Tools).NotTo(BeEmpty(),
				"RCA call should include tool definitions from the registry")
			toolNames := make([]string, len(rcaCall.Tools))
			for i, td := range rcaCall.Tools {
				toolNames[i] = td.Name
			}
			Expect(toolNames).To(ContainElement("kubectl_describe"),
				"tool definitions should include registered tools available in the RCA phase")
		})

		It("should send workflow discovery tool definitions in the second LLM call", func() {
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_describe", result: `{}`})
			reg.Register(&fakeTool{name: "list_available_actions", result: `[]`})

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"found issue"}`}},
				wfToolResp(`{"workflow_id":"restart","confidence":0.7}`),
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
				Environment: "Development", Priority: "P2",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))

			wdCall := mockClient.calls[1]
			Expect(wdCall.Tools).NotTo(BeEmpty(),
				"workflow discovery call should include tool definitions")
			toolNames := make([]string, len(wdCall.Tools))
			for i, td := range wdCall.Tools {
				toolNames[i] = td.Name
			}
			Expect(toolNames).NotTo(ContainElement("kubectl_describe"),
				"workflow discovery should NOT include RCA-only tools")
		})
	})

	Describe("IT-KA-433W-005: Investigation prompt excludes enrichment, workflow selection includes it", func() {
		It("should NOT include enrichment in RCA prompt; remediation history appears in Phase 3 only", func() {
			By("Seeding remediation history into PostgreSQL so the real DS returns it")
			testCtx := context.Background()
			specHash := sha256FixtureHash
			corrID := "oom-increase-memory"
			target := "production/Pod/api-server-abc"
			now := time.Now()
			seedAuditEvent(testCtx, "remediation.workflow_created", "remediation", corrID,
				map[string]interface{}{
					"target_resource":           target,
					"pre_remediation_spec_hash": specHash,
					"action_type":               "IncreaseMemory",
					"signal_type":               "OOMKilled",
					"signal_fingerprint":        "fp-test-433w005",
					"outcome":                   "success",
				}, now.Add(-30*time.Minute))
			seedAuditEvent(testCtx, "effectiveness.hash.computed", "effectiveness", corrID,
				map[string]interface{}{
					"pre_remediation_spec_hash":  specHash,
					"post_remediation_spec_hash": "sha256:post-fixture",
				}, now.Add(-25*time.Minute))
			DeferCleanup(func() {
				_, _ = seedDB.ExecContext(testCtx,
					"DELETE FROM audit_events WHERE correlation_id = $1", corrID)
			})

			By("Using k8sFixtureClient so enrichment succeeds with controlled data")
			fixtureK8s := &k8sFixtureClient{
				ownerChain: []enrichment.OwnerChainEntry{
					{Kind: "Pod", Name: "api-server-abc", Namespace: "production"},
					{Kind: "ReplicaSet", Name: "api-server-rs", Namespace: "production"},
					{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				},
			}
			fixtureEnricher := enrichment.NewEnricher(fixtureK8s, suiteDSAdapter, auditStore, invLogger)

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Memory pressure detected"}`}},
				wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.9}`),
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: fixtureEnricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:        "api-server-abc",
				Namespace:   "production",
				Severity:    "critical",
				Environment: "Production",
				Priority:    "P0",
				Message:     "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))

			rcaCall := mockClient.calls[0]
			systemPrompt := rcaCall.Messages[0].Content
			Expect(systemPrompt).NotTo(ContainSubstring("Owner Chain"),
				"RCA system prompt must NOT contain enrichment owner chain (Phase 3 only)")
			Expect(systemPrompt).NotTo(ContainSubstring("Detected Labels"),
				"RCA system prompt must NOT contain enrichment labels (Phase 3 only)")
			Expect(systemPrompt).NotTo(ContainSubstring("oom-increase-memory"),
				"RCA system prompt must NOT contain remediation history (Phase 3 only)")

			By("remediation history should appear in workflow selection prompt instead")
			wdCall := mockClient.calls[1]
			wdSystemPrompt := wdCall.Messages[0].Content
			Expect(wdSystemPrompt).To(ContainSubstring("oom-increase-memory"),
				"workflow selection prompt should contain remediation history workflow ID")
		})
	})

	Describe("IT-KA-433W-006: Investigator with nil enricher degrades gracefully", func() {
		It("should produce investigation result without enrichment data and without panic", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Issue found"}`}},
				wfToolResp(`{"workflow_id":"restart","confidence":0.7}`),
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: nil, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "test-pod", Namespace: "default", Severity: "warning", Message: "CrashLoop",
				Environment: "Development", Priority: "P2",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(Equal("restart"))

			rcaCall := mockClient.calls[0]
			systemPrompt := rcaCall.Messages[0].Content
			Expect(systemPrompt).NotTo(ContainSubstring("Owner Chain"),
				"system prompt should not contain enrichment sections when enricher is nil")
		})
	})

	Describe("IT-KA-686-001: submit_result tool call returns structured investigation result", func() {
		It("should capture submit_result arguments as investigation result instead of executing a tool", func() {
			submitArgs := `{"root_cause_analysis":{"summary":"OOMKilled due to memory limit exceeded","severity":"critical","remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production"}},"selected_workflow":{"workflow_id":"oom-increase-memory","confidence":0.95,"rationale":"increase memory limit","parameters":{"MEMORY_LIMIT":"512Mi"}},"confidence":0.95,"severity":"critical"}`

			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_describe", result: `{"kind":"Pod","metadata":{"name":"api-server"},"status":{"phase":"Running"}}`})

			mockClient.responses = []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: "Let me investigate"},
					ToolCalls: []llm.ToolCall{
						{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"api-server","namespace":"production"}`},
					},
				},
				{
					Message: llm.Message{Role: "assistant", Content: "Found the issue"},
					ToolCalls: []llm.ToolCall{
						{ID: "tc_submit", Name: "submit_result", Arguments: submitArgs},
					},
				},
				wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.95}`),
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server", Namespace: "production", Severity: "critical", Message: "OOMKilled",
				Environment: "Production", Priority: "P0",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).To(ContainSubstring("OOMKilled"))
			Expect(result.WorkflowID).To(Equal("oom-increase-memory"))
			Expect(result.Confidence).To(BeNumerically("~", 0.95, 0.01))
		})
	})

	Describe("IT-KA-686-002: submit_result in both RCA and workflow discovery phases", func() {
		It("should include submit_result in RCA and submit_result_with_workflow / submit_result_no_workflow in workflow discovery", func() {
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_describe", result: `{}`})
			reg.Register(&fakeTool{name: "list_available_actions", result: `[]`})

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"found issue"}`}},
				wfToolResp(`{"workflow_id":"restart","confidence":0.7}`),
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
				Environment: "Development", Priority: "P2",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))

			By("RCA phase includes submit_result")
			rcaToolNames := toolNamesFromCall(mockClient.calls[0])
			Expect(rcaToolNames).To(ContainElement("submit_result"),
				"RCA phase should include submit_result tool definition")

			By("Workflow discovery phase includes submit_result_with_workflow and submit_result_no_workflow")
			wdToolNames := toolNamesFromCall(mockClient.calls[1])
			Expect(wdToolNames).To(ContainElement("submit_result_with_workflow"),
				"Workflow discovery phase should include submit_result_with_workflow tool definition")
			Expect(wdToolNames).To(ContainElement("submit_result_no_workflow"),
				"Workflow discovery phase should include submit_result_no_workflow tool definition")
		})
	})

	Describe("IT-KA-686-003: submit_result bypasses anomaly detector", func() {
		It("should not invoke anomaly detector CheckToolCall for submit_result", func() {
			submitArgs := `{"root_cause_analysis":{"summary":"issue found"},"confidence":0.9}`

			mockClient.responses = []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: ""},
					ToolCalls: []llm.ToolCall{
						{ID: "tc_submit", Name: "submit_result", Arguments: submitArgs},
					},
				},
				wfToolResp(`{"workflow_id":"restart","confidence":0.7}`),
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
				Environment: "Development", Priority: "P2",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).To(ContainSubstring("issue found"))
		})
	})

	Describe("IT-KA-433-009: Investigation emits audit events", func() {
		It("should emit audit events at correct investigation points", func() {
			fixtureK8s := &k8sFixtureClient{
				ownerChain: []enrichment.OwnerChainEntry{
					{Kind: "Pod", Name: "api-server", Namespace: "production"},
					{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				},
			}
			fixtureEnricher := enrichment.NewEnricher(fixtureK8s, suiteDSAdapter, auditStore, invLogger)

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"found issue"}`}},
				wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.85}`),
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: fixtureEnricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server", Namespace: "production", Severity: "critical", Message: "OOMKilled",
				Environment: "Production", Priority: "P0",
			})
			Expect(err).NotTo(HaveOccurred())

			eventTypes := auditEventTypes(auditStore.events)
			Expect(eventTypes).To(ContainElement(audit.EventTypeLLMRequest), "should emit llm.request")
			Expect(eventTypes).To(ContainElement(audit.EventTypeLLMResponse), "should emit llm.response")
			Expect(eventTypes).To(ContainElement(audit.EventTypeResponseComplete), "should emit response.complete")
			Expect(eventTypes).To(ContainElement(audit.EventTypeEnrichmentCompleted), "should emit enrichment.completed")
		})
	})
})

var _ = Describe("TP-693: Workflow signal override after re-enrichment", func() {

	var (
		invLogger  logr.Logger
		auditStore *capturingAuditStore
		mockClient *mockLLMClient
		builder    *prompt.Builder
		rp         *parser.ResultParser
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = newCapturingAuditStore(suiteAuditStore)
		mockClient = &mockLLMClient{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("UT-KA-693-005: Workflow selection prompt shows re-enriched target", func() {
		It("should include the RCA-identified Deployment name in workflow selection prompt", func() {
			k8s := &resourceAwareFixtureClient{
				chains: map[string][]enrichment.OwnerChainEntry{
					"worker-77784c6cf7-l27g4": {
						{Kind: "ReplicaSet", Name: "worker-77784c6cf7", Namespace: "demo-crashloop"},
						{Kind: "Deployment", Name: "worker", Namespace: "demo-crashloop"},
					},
					"worker": {}, // Deployment is root, empty chain
				},
			}
			enricher := enrichment.NewEnricher(k8s, suiteDSAdapter, auditStore, invLogger)

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled in worker deployment","remediation_target":{"kind":"Deployment","name":"worker","namespace":"demo-crashloop"}}`}},
				wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.9,"remediation_target":{"kind":"Deployment","name":"worker","namespace":"demo-crashloop"}}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "worker-77784c6cf7-l27g4",
				Name:         "worker-77784c6cf7-l27g4",
				Namespace:    "demo-crashloop",
				Severity:     "critical",
				Environment:  "Development",
				Priority:     "P1",
				Message:      "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(mockClient.calls).To(HaveLen(2))
			workflowCallContent := allMessageContent(mockClient.calls[1].Messages)
			Expect(workflowCallContent).To(ContainSubstring("demo-crashloop/Deployment/worker"),
				"UT-KA-693-005: workflow selection prompt Resource field must show re-enriched target")
			Expect(workflowCallContent).NotTo(ContainSubstring("demo-crashloop/Pod/worker-77784c6cf7-l27g4"),
				"UT-KA-693-005: workflow selection prompt Resource field must NOT show original Pod identity")
		})
	})

	Describe("UT-KA-693-006: No re-enrichment leaves signal unchanged", func() {
		It("should preserve original signal when RCA returns same target", func() {
			k8s := &k8sFixtureClient{
				ownerChain: []enrichment.OwnerChainEntry{
					{Kind: "ReplicaSet", Name: "worker-77784c6cf7", Namespace: "demo-crashloop"},
					{Kind: "Deployment", Name: "worker", Namespace: "demo-crashloop"},
				},
			}
			enricher := enrichment.NewEnricher(k8s, suiteDSAdapter, auditStore, invLogger)

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"CrashLoop in worker pod"}`}},
				wfToolResp(`{"workflow_id":"restart-pod","confidence":0.8}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "worker-77784c6cf7-l27g4",
				Name:         "worker-77784c6cf7-l27g4",
				Namespace:    "demo-crashloop",
				Severity:     "warning",
				Environment:  "Development",
				Priority:     "P2",
				Message:      "CrashLoopBackOff",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"),
				"UT-KA-693-006: target kind should come from owner chain")
			Expect(result.RemediationTarget.Name).To(Equal("worker"),
				"UT-KA-693-006: target name should come from owner chain root")
		})
	})

	Describe("UT-KA-693-007: Injection receives same signal identity as prompt", func() {
		It("should produce remediation target consistent with prompt identity after re-enrichment", func() {
			k8s := &resourceAwareFixtureClient{
				chains: map[string][]enrichment.OwnerChainEntry{
					"worker-77784c6cf7-l27g4": {
						{Kind: "ReplicaSet", Name: "worker-77784c6cf7", Namespace: "demo-crashloop"},
						{Kind: "Deployment", Name: "worker", Namespace: "demo-crashloop"},
					},
					"worker": {}, // Deployment is root, empty chain
				},
			}
			enricher := enrichment.NewEnricher(k8s, suiteDSAdapter, auditStore, invLogger)

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled targeting Deployment worker","remediation_target":{"kind":"Deployment","name":"worker","namespace":"demo-crashloop"}}`}},
				wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.95,"remediation_target":{"kind":"Deployment","name":"worker-77784c6cf7","namespace":"demo-crashloop"}}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "worker-77784c6cf7-l27g4",
				Name:         "worker-77784c6cf7-l27g4",
				Namespace:    "demo-crashloop",
				Severity:     "critical",
				Environment:  "Development",
				Priority:     "P1",
				Message:      "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"),
				"UT-KA-693-007: injection must use re-enrichment source kind")
			Expect(result.RemediationTarget.Name).To(Equal("worker"),
				"UT-KA-693-007: injection must use re-enrichment source name, not LLM hallucination")
		})
	})

	Describe("IT-KA-847-D-001: sameKindValidationGate rejects retry that lost remediation_target", func() {
		It("should keep original target when retry response drops remediation_target (DD-HAPI-847)", func() {
			// Signal is a Node, RCA also names Node → same-kind gate fires.
			// Retry response (fallback) has no remediation_target.
			// Approach D defensive check must preserve the original Node target.
			k8s := &k8sFixtureClient{ownerChain: nil, err: nil}
			localEnricher := enrichment.NewEnricher(k8s, suiteDSAdapter, auditStore, invLogger)

			mockClient.responses = []llm.ChatResponse{
				// Phase 1 RCA: same kind as signal (Node == Node) → gate triggers
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"disk pressure from emptyDir overuse",
					"confidence":0.85,
					"remediation_target":{"kind":"Node","name":"ip-10-0-1-42","namespace":""}
				}`}},
				// Gate retry: fallback response with NO remediation_target
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"confirmed disk pressure on node",
					"confidence":0.80
				}`}},
				// Workflow selection
				wfToolResp(`{"workflow_id":"drain-node","confidence":0.9,"remediation_target":{"kind":"Node","name":"ip-10-0-1-42","namespace":""}}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: localEnricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				ResourceKind: "Node",
				ResourceName: "ip-10-0-1-42",
				Name:         "ip-10-0-1-42",
				Namespace:    "",
				Severity:     "warning",
				Environment:  "Development",
				Priority:     "P2",
				Message:      "DiskPressure condition detected",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Node"),
				"IT-KA-847-D-001: defensive check must preserve original Node target when retry loses it")
			Expect(result.RemediationTarget.Name).To(Equal("ip-10-0-1-42"),
				"IT-KA-847-D-001: must preserve original name")
			Expect(result.RCASummary).To(ContainSubstring("disk pressure"),
				"IT-KA-847-D-001: RCA summary should come from the original (not retry)")
		})
	})

	Describe("IT-KA-704-001: Owner chain failure triggers rca_incomplete", func() {
		It("should set needs_human_review=true with rca_incomplete when GetOwnerChain fails", func() {
			// Issue #1039: NotFound is now exempt from HardFail (deleted resources
			// proceed to workflow selection). Use InternalError to test rca_incomplete
			// for genuine API failures that still trigger HardFail.
			apiErr := apierrors.NewInternalError(fmt.Errorf("etcd timeout"))
			k8s := &k8sFixtureClient{
				ownerChain: nil,
				err:        apiErr,
			}
			enricher := enrichment.NewEnricher(k8s, suiteDSAdapter, auditStore, invLogger).
				WithRetryConfig(enrichment.RetryConfig{
					MaxRetries:  3,
					BaseBackoff: 1 * time.Millisecond,
				})

			// RCA names a DIFFERENT kind+name than the signal to trigger
			// re-enrichment. Using Deployment (vs signal Pod) avoids the
			// same-kind validation gate (#847) so only one LLM response is needed.
			// Flow returns at rca_incomplete before workflow selection.
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled due to memory limit exceeded","remediation_target":{"kind":"Deployment","name":"target-deploy","namespace":"production"}}`}},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "signal-pod",
				Name:         "signal-pod",
				Namespace:    "production",
				Severity:     "critical",
				Environment:  "Production",
				Priority:     "P0",
				Message:      "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"IT-KA-704-001: owner chain failure must trigger human review")
			Expect(result.HumanReviewReason).To(Equal("rca_incomplete"),
				"IT-KA-704-001: reason must be rca_incomplete per BR-HAPI-261 AC#7")
			Expect(result.RCASummary).NotTo(BeEmpty(),
				"IT-KA-704-001: RCA phase should complete before enrichment check")
			Expect(result.WorkflowID).To(BeEmpty(),
				"IT-KA-704-001: workflow selection should be skipped when rca_incomplete")
		})
	})
})

// wfToolResp creates a mock ChatResponse where the LLM calls submit_result_with_workflow
// with the given JSON content. Used to adapt pre-#760v2 tests that previously returned
// workflow selection as plain text.
func wfToolResp(jsonContent string) llm.ChatResponse {
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: ""},
		ToolCalls: []llm.ToolCall{
			{ID: "tc_wf", Name: "submit_result_with_workflow", Arguments: jsonContent},
		},
	}
}

func toolNamesFromCall(call llm.ChatRequest) []string {
	names := make([]string, len(call.Tools))
	for i, td := range call.Tools {
		names[i] = td.Name
	}
	return names
}

func allMessageContent(msgs []llm.Message) string {
	var sb string
	for _, m := range msgs {
		sb += m.Content + " "
	}
	return sb
}

func auditEventTypes(events []*audit.AuditEvent) []string {
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.EventType
	}
	return types
}

// ---------------------------------------------------------------------------
// F5+F6: RunWorkflowDiscoveryFromRCA enrichment + DetectedLabelsJSON parity
// ---------------------------------------------------------------------------

var _ = Describe("F5/F6: RunWorkflowDiscoveryFromRCA enrichment parity (#1374)", func() {

	var (
		invLogger  logr.Logger
		auditStore *capturingAuditStore
		mockClient *mockLLMClient
		builder    *prompt.Builder
		rp         *parser.ResultParser
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = newCapturingAuditStore(suiteAuditStore)
		mockClient = &mockLLMClient{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-1374-F5-001: RunWorkflowDiscoveryFromRCA resolves enrichment when enricher is wired [BR-INTERACTIVE-010]", func() {
		It("should include owner chain in workflow selection prompt", func() {
			k8s := &resourceAwareFixtureClient{
				chains: map[string][]enrichment.OwnerChainEntry{
					"api-server": {
						{Kind: "ReplicaSet", Name: "api-server-7f8c9d", Namespace: "production"},
						{Kind: "Deployment", Name: "api-server", Namespace: "production", APIVersion: "apps/v1"},
					},
				},
			}
			enricher := enrichment.NewEnricher(k8s, suiteDSAdapter, auditStore, invLogger)

			mockClient.responses = []llm.ChatResponse{
				wfToolResp(`{"workflow_id":"oom-fix","confidence":0.9,"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production","api_version":"apps/v1"}}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			signal := katypes.SignalContext{
				ResourceKind: "Deployment",
				ResourceName: "api-server",
				Namespace:    "production",
				Severity:     "critical",
				Environment:  "Production",
				Priority:     "P0",
				Message:      "OOMKilled",
			}
			rcaResult := &katypes.InvestigationResult{
				RCASummary: "OOMKilled in api-server container",
				Confidence: 0.92,
				RemediationTarget: katypes.RemediationTarget{
					Kind:       "Deployment",
					Name:       "api-server",
					Namespace:  "production",
					APIVersion: "apps/v1",
				},
			}

			result, err := inv.RunWorkflowDiscoveryFromRCA(
				context.Background(), signal, rcaResult, nil, "corr-f5-001",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(mockClient.calls).To(HaveLen(1))
			promptContent := allMessageContent(mockClient.calls[0].Messages)
			Expect(promptContent).To(ContainSubstring("Owner chain:"),
				"IT-KA-1374-F5-001: workflow prompt must contain enrichment Owner chain section")
			Expect(promptContent).To(ContainSubstring("ReplicaSet/api-server-7f8c9d"),
				"IT-KA-1374-F5-001: Owner chain must include ReplicaSet entry from enrichment")
		})
	})

	Describe("IT-KA-1374-F5-002: RunWorkflowDiscoveryFromRCA degrades gracefully without enricher [BR-INTERACTIVE-010]", func() {
		It("should produce valid result when enricher is nil", func() {
			mockClient.responses = []llm.ChatResponse{
				wfToolResp(`{"workflow_id":"restart","confidence":0.8,"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"default"}}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: nil, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			signal := katypes.SignalContext{
				ResourceKind: "Deployment",
				ResourceName: "api-server",
				Namespace:    "default",
				Severity:     "warning",
				Environment:  "Development",
				Priority:     "P2",
				Message:      "CrashLoop",
			}
			rcaResult := &katypes.InvestigationResult{
				RCASummary: "CrashLoop detected",
				Confidence: 0.7,
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Deployment", Name: "api-server", Namespace: "default",
				},
			}

			result, err := inv.RunWorkflowDiscoveryFromRCA(
				context.Background(), signal, rcaResult, nil, "corr-f5-002",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"))
		})
	})

	Describe("IT-KA-1374-F5-003: RunWorkflowDiscoveryFromRCA post-RCA re-enrichment [BR-INTERACTIVE-010, BR-HAPI-261]", func() {
		It("should re-enrich when RCA target differs from signal", func() {
			k8s := &resourceAwareFixtureClient{
				chains: map[string][]enrichment.OwnerChainEntry{
					"worker-pod-abc": {
						{Kind: "ReplicaSet", Name: "worker-7f8c9d", Namespace: "production"},
						{Kind: "Deployment", Name: "worker", Namespace: "production", APIVersion: "apps/v1"},
					},
					"worker": {},
				},
			}
			enricher := enrichment.NewEnricher(k8s, suiteDSAdapter, auditStore, invLogger)

			mockClient.responses = []llm.ChatResponse{
				wfToolResp(`{"workflow_id":"oom-fix","confidence":0.9,"remediation_target":{"kind":"Deployment","name":"worker","namespace":"production","api_version":"apps/v1"}}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "worker-pod-abc",
				Namespace:    "production",
				Severity:     "critical",
				Environment:  "Production",
				Priority:     "P0",
				Message:      "OOMKilled",
			}
			rcaResult := &katypes.InvestigationResult{
				RCASummary: "OOMKilled — root cause is Deployment worker",
				Confidence: 0.95,
				RemediationTarget: katypes.RemediationTarget{
					Kind:       "Deployment",
					Name:       "worker",
					Namespace:  "production",
					APIVersion: "apps/v1",
				},
			}

			result, err := inv.RunWorkflowDiscoveryFromRCA(
				context.Background(), signal, rcaResult, nil, "corr-f5-003",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"),
				"IT-KA-1374-F5-003: RemediationTarget.Kind must reflect RCA target after re-enrichment")
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"))
		})
	})
})
