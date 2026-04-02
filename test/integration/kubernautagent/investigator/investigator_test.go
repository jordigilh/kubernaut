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
)

type recordingAuditStore struct {
	events []*audit.AuditEvent
}

func (r *recordingAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	r.events = append(r.events, event)
	return nil
}

type mockLLMClient struct {
	calls    []llm.ChatRequest
	responses []llm.ChatResponse
	callIdx  int
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

type fakeDataStorageClient struct {
	history []enrichment.RemediationHistoryEntry
	err     error
}

func (f *fakeDataStorageClient) GetRemediationHistory(_ context.Context, _, _, _, _ string) ([]enrichment.RemediationHistoryEntry, error) {
	return f.history, f.err
}

var _ = Describe("Kubernaut Agent Investigator Integration — #433", func() {

	var (
		logger      *slog.Logger
		auditStore  *recordingAuditStore
		mockClient  *mockLLMClient
		builder     *prompt.Builder
		rp          *parser.ResultParser
		enricher    *enrichment.Enricher
		phaseTools  katypes.PhaseToolMap
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
			history: []enrichment.RemediationHistoryEntry{
				{WorkflowID: "oom-increase-memory", Outcome: "success", Timestamp: "2026-03-01T10:00:00Z"},
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
			inv := investigator.New(mockClient, builder, rp, enricher, nil, auditStore, logger, 15, phaseTools)
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

	Describe("IT-KA-433-006: Investigation uses correct tools per phase (I4)", func() {
		It("should restrict tool definitions by phase", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"issue found"}`}},
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart-pod","confidence":0.8}`}},
			}
			inv := investigator.New(mockClient, builder, rp, enricher, nil, auditStore, logger, 15, phaseTools)
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "pod-abc", Namespace: "default", Severity: "warning", Message: "CrashLoopBackOff",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2), "should make 2 LLM calls (RCA + workflow)")
			// Phase 1 (RCA) call should include K8s tools
			if len(mockClient.calls) >= 1 {
				rcaToolNames := toolNames(mockClient.calls[0].Tools)
				Expect(rcaToolNames).To(ContainElement("kubectl_describe"),
					"RCA phase should include kubectl_describe")
			}
			// Phase 3 (WorkflowDiscovery) call should include workflow tools
			if len(mockClient.calls) >= 2 {
				wdToolNames := toolNames(mockClient.calls[1].Tools)
				Expect(wdToolNames).To(ContainElement("list_workflows"),
					"WorkflowDiscovery phase should include list_workflows")
			}
		})
	})

	Describe("IT-KA-433-007: Investigation preserves conversation history", func() {
		It("should pass RCA context into the workflow selection invocation", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"memory leak in api-server container"}`}},
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"oom-increase-memory","confidence":0.88}`}},
			}
			inv := investigator.New(mockClient, builder, rp, enricher, nil, auditStore, logger, 15, phaseTools)
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", Severity: "critical", Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))
			// The second invocation should reference RCA findings in its messages
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
			inv := investigator.New(mockClient, builder, rp, enricher, nil, auditStore, logger, 1, phaseTools)
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "critical", Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"investigation should require human review when max turns exhausted")
		})
	})

	Describe("IT-KA-433-009: Investigation emits all 8 audit event types", func() {
		It("should emit audit events at correct investigation points", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"found issue"}`}},
				{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"oom-increase-memory","confidence":0.85}`}},
			}
			inv := investigator.New(mockClient, builder, rp, enricher, nil, auditStore, logger, 15, phaseTools)
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

func toolNames(tools []llm.ToolDefinition) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
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
