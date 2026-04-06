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
	"log/slog"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

type recordingAuditStore struct {
	events []*audit.AuditEvent
}

func (r *recordingAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	r.events = append(r.events, event)
	return nil
}

type mockLLMClient struct {
	calls     []llm.ChatRequest
	responses []llm.ChatResponse
	callIdx   int
}

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
			Content: `{"rca_summary":"no more responses","human_review_needed":true}`,
		},
	}, nil
}

type fakeK8sClient struct {
	ownerChain []enrichment.OwnerChainEntry
	err        error
}

func (f *fakeK8sClient) GetOwnerChain(_ context.Context, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	return f.ownerChain, f.err
}

func (f *fakeK8sClient) GetSpecHash(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}

// resourceAwareK8sClient returns different owner chains based on the resource name.
// Used by IT-KA-433-AP-020 to test cross-target label contamination.
type resourceAwareK8sClient struct {
	chains map[string][]enrichment.OwnerChainEntry
}

func (r *resourceAwareK8sClient) GetOwnerChain(_ context.Context, _, name, _ string) ([]enrichment.OwnerChainEntry, error) {
	if chain, ok := r.chains[name]; ok {
		return chain, nil
	}
	return nil, nil
}

func (r *resourceAwareK8sClient) GetSpecHash(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}

type fakeDataStorageClient struct {
	history *enrichment.RemediationHistoryResult
	err     error
}

func (f *fakeDataStorageClient) GetRemediationHistory(_ context.Context, _, _, _, _ string) (*enrichment.RemediationHistoryResult, error) {
	return f.history, f.err
}

type fakeTool struct {
	name   string
	result string
	err    error
}

func (f *fakeTool) Name() string                                                         { return f.name }
func (f *fakeTool) Description() string                                                  { return "fake " + f.name }
func (f *fakeTool) Parameters() json.RawMessage                                          { return nil }
func (f *fakeTool) Execute(_ context.Context, _ json.RawMessage) (string, error) { return f.result, f.err }

var _ = Describe("Kubernaut Agent Investigator Integration — #433", func() {

	var (
		logger     *slog.Logger
		auditStore *recordingAuditStore
		mockClient *mockLLMClient
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		auditStore = &recordingAuditStore{}
		mockClient = &mockLLMClient{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		k8sClient := &fakeK8sClient{
			ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				{Kind: "ReplicaSet", Name: "api-server-abc", Namespace: "production"},
			},
		}
		dsClient := &fakeDataStorageClient{
			history: &enrichment.RemediationHistoryResult{
				Tier1: []enrichment.Tier1Entry{
					{RemediationUID: "oom-increase-memory", Outcome: "success"},
				},
			},
		}
		enricher = enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-433-005: Two-invocation investigation produces RCA then workflow", func() {
		It("should return an InvestigationResult with both RCA summary and workflow_id", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled due to memory limit exceeded"}`}},
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"oom-increase-memory","confidence":0.9,"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production"}}`}},
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:      "api-server-abc",
				Namespace: "production",
				Severity:  "critical",
				Message:   "OOMKilled",
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
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart-pod","confidence":0.8}`}},
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "pod-abc", Namespace: "default", Severity: "warning", Message: "CrashLoopBackOff",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2), "should make 2 LLM calls (RCA + workflow)")
		})
	})

	Describe("IT-KA-433-007: Investigation preserves conversation history", func() {
		It("should pass RCA context into the workflow selection invocation", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"memory leak in api-server container"}`}},
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"oom-increase-memory","confidence":0.88}`}},
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", Severity: "critical", Message: "OOMKilled",
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
				{Message: llm.Message{Role: "assistant", Content: "I need more information", ToolCalls: []llm.ToolCall{
					{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"api","namespace":"default"}`},
				}}},
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 1, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "critical", Message: "OOMKilled",
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
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"generic-restart","confidence":0.7}`}},
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
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
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.5}`}},
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "critical", Message: "OOMKilled",
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
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.7}`}},
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
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
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.7}`}},
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
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

	Describe("IT-KA-433W-005: Investigator with enricher includes owner chain in RCA system prompt", func() {
		It("should include owner chain and remediation history strings in the RCA system prompt", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Memory pressure detected"}`}},
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"oom-increase-memory","confidence":0.9}`}},
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:      "api-server-abc",
				Namespace: "production",
				Severity:  "critical",
				Message:   "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))

			rcaCall := mockClient.calls[0]
			systemPrompt := rcaCall.Messages[0].Content
			Expect(systemPrompt).To(ContainSubstring("Deployment/api-server"),
				"RCA system prompt should contain owner chain entries")
			Expect(systemPrompt).To(ContainSubstring("oom-increase-memory"),
				"RCA system prompt should contain remediation history workflow ID")
		})
	})

	Describe("IT-KA-433W-006: Investigator with nil enricher degrades gracefully", func() {
		It("should produce investigation result without enrichment data and without panic", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Issue found"}`}},
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.7}`}},
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: nil, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "test-pod", Namespace: "default", Severity: "warning", Message: "CrashLoop",
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

	Describe("IT-KA-433-009: Investigation emits audit events", func() {
		It("should emit audit events at correct investigation points", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"found issue"}`}},
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"oom-increase-memory","confidence":0.85}`}},
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server", Namespace: "production", Severity: "critical", Message: "OOMKilled",
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
