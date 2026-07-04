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
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// Characterization tests for sameKindValidationGate (Issue #847), added as
// Wave C RED for the complexity-lint-gates remediation plan. This gate had
// 10.9% unit coverage prior to refactoring; these tests pin its documented
// behavior via the public Investigate() entry point before any decomposition.
var _ = Describe("Issue #847: sameKindValidationGate", func() {

	var (
		logger     logr.Logger
		auditStore *gateRecordingAuditStore
		mockClient *gateMockLLMClient
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		logger = logr.Discard()
		auditStore = &gateRecordingAuditStore{}
		mockClient = &gateMockLLMClient{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		enricher = enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, auditStore, logger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	signal := katypes.SignalContext{
		ResourceKind: "Node",
		ResourceName: "worker-1",
		Name:         "worker-1",
		Namespace:    "",
		Severity:     "critical",
		Message:      "DiskPressure condition on node",
	}

	newInv := func() *investigator.Investigator {
		return investigator.New(investigator.Config{
			Client: mockClient, Builder: builder, ResultParser: rp,
			Enricher: enricher, AuditStore: auditStore, Logger: logger,
			MaxTurns: 15, PhaseTools: phaseTools,
		})
	}

	Describe("UT-KA-847-001: RCA target kind matches signal kind, retry provides child resource", func() {
		It("should accept the retry result targeting a child resource (BR-AI-847)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Node under disk pressure",
					"confidence":0.85,
					"remediation_target":{"kind":"Node","name":"worker-1"}
				}`}},
				// Gate retry: LLM re-evaluates and targets the child Pod instead.
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Pod filled ephemeral storage causing node DiskPressure",
					"confidence":0.9,
					"remediation_target":{"kind":"Pod","name":"log-spammer","namespace":"default"}
				}`}},
				gateWfToolResp(`{"workflow_id":"evict-pod","confidence":0.9,"remediation_target":{"kind":"Pod","name":"log-spammer","namespace":"default"}}`),
			}

			result, err := newInv().Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Pod"),
				"UT-KA-847-001: gate must accept a retry result that re-targets a child resource")
			Expect(result.RemediationTarget.Name).To(Equal("log-spammer"))
		})
	})

	Describe("UT-KA-847-002: retry confirms the same kind after re-evaluation", func() {
		It("should accept the retry result even when it confirms the same kind (BR-AI-847 AC)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Node under disk pressure",
					"confidence":0.85,
					"remediation_target":{"kind":"Node","name":"worker-1"}
				}`}},
				// Gate retry: LLM confirms Node is indeed correct.
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Node itself has a stuck kubelet, confirmed root cause",
					"confidence":0.8,
					"remediation_target":{"kind":"Node","name":"worker-1"},
					"due_diligence":{"target_accuracy":"confirmed after re-evaluation, kubelet process is the issue"}
				}`}},
				gateWfToolResp(`{"workflow_id":"restart-kubelet","confidence":0.8,"remediation_target":{"kind":"Node","name":"worker-1"}}`),
			}

			result, err := newInv().Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Node"),
				"UT-KA-847-002: gate must accept a retry result that confirms the same kind")
		})
	})

	Describe("UT-KA-847-003: skip when signal.ResourceKind is empty", func() {
		It("should not trigger the gate and not add extra LLM calls (BR-AI-847 nil/zero)", func() {
			emptyKindSignal := katypes.SignalContext{
				ResourceKind: "",
				ResourceName: "worker-1",
				Name:         "worker-1",
				Severity:     "warning",
				Message:      "generic signal",
			}
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"generic issue",
					"confidence":0.75,
					"remediation_target":{"kind":"Node","name":"worker-1"}
				}`}},
				gateWfToolResp(`{"workflow_id":"generic-fix","confidence":0.8,"remediation_target":{"kind":"Node","name":"worker-1"}}`),
			}

			result, err := newInv().Investigate(context.Background(), emptyKindSignal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(mockClient.calls).To(HaveLen(2),
				"UT-KA-847-003: gate must not fire (no retry call) when signal.ResourceKind is empty")
		})
	})

	Describe("UT-KA-847-004: skip when RCA target kind differs from signal kind", func() {
		It("should not trigger the gate for a cross-type target (BR-AI-847)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Pod caused node pressure",
					"confidence":0.85,
					"remediation_target":{"kind":"Pod","name":"log-spammer","namespace":"default"}
				}`}},
				gateWfToolResp(`{"workflow_id":"evict-pod","confidence":0.9,"remediation_target":{"kind":"Pod","name":"log-spammer","namespace":"default"}}`),
			}

			result, err := newInv().Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(mockClient.calls).To(HaveLen(2),
				"UT-KA-847-004: gate must not fire when RCA target kind differs from signal kind")
		})
	})

	Describe("UT-KA-847-005: retry LLM call fails, keep original result", func() {
		It("should keep the original result when the retry call errors (BR-AI-847 error)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Node under disk pressure",
					"confidence":0.85,
					"remediation_target":{"kind":"Node","name":"worker-1"}
				}`}},
			}
			mockClient.errors = []error{nil, fmt.Errorf("LLM service unavailable")}

			result, err := newInv().Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Node"),
				"UT-KA-847-005: gate must keep the original result when the retry LLM call fails")
		})
	})

	Describe("UT-KA-847-006: retry returns empty content, keep original result", func() {
		It("should keep the original result when the retry response has no content (BR-AI-847 error)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Node under disk pressure",
					"confidence":0.85,
					"remediation_target":{"kind":"Node","name":"worker-1"}
				}`}},
				{Message: llm.Message{Role: "assistant", Content: ""}},
			}

			result, err := newInv().Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Node"),
				"UT-KA-847-006: gate must keep the original result when the retry response is empty")
		})
	})

	Describe("UT-KA-847-007: retry response is unparseable, keep original result", func() {
		It("should keep the original result when the retry response cannot be parsed (BR-AI-847 error)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Node under disk pressure",
					"confidence":0.85,
					"remediation_target":{"kind":"Node","name":"worker-1"}
				}`}},
				{Message: llm.Message{Role: "assistant", Content: `not valid json at all`}},
			}

			result, err := newInv().Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Node"),
				"UT-KA-847-007: gate must keep the original result when the retry response is unparseable")
		})
	})

	Describe("UT-KA-847-008: retry drops remediation_target, keep original result", func() {
		It("should keep the original result when the retry loses remediation_target (BR-AI-847 error)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Node under disk pressure",
					"confidence":0.85,
					"remediation_target":{"kind":"Node","name":"worker-1"}
				}`}},
				// Gate retry: no remediation_target at all.
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"confirmed issue but no clear target",
					"confidence":0.5
				}`}},
			}

			result, err := newInv().Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Node"),
				"UT-KA-847-008: gate must keep the original result when the retry drops remediation_target")
			Expect(result.RemediationTarget.Name).To(Equal("worker-1"))
		})
	})

	Describe("UT-KA-847-009: gate audit event emitted with target details", func() {
		It("should emit an audit event with ActionSameKindGate and target/kind data (BR-AI-847 observability)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Node under disk pressure",
					"confidence":0.85,
					"remediation_target":{"kind":"Node","name":"worker-1"}
				}`}},
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Pod caused it",
					"confidence":0.9,
					"remediation_target":{"kind":"Pod","name":"log-spammer","namespace":"default"}
				}`}},
				gateWfToolResp(`{"workflow_id":"evict-pod","confidence":0.9,"remediation_target":{"kind":"Pod","name":"log-spammer","namespace":"default"}}`),
			}

			_, err := newInv().Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			gateEvents := auditStore.eventsByAction(audit.ActionSameKindGate)
			Expect(gateEvents).NotTo(BeEmpty(),
				"UT-KA-847-009: gate must emit an audit event with ActionSameKindGate")
			ev := gateEvents[0]
			Expect(ev.Data).To(HaveKeyWithValue("signal_resource_kind", "Node"))
			Expect(ev.Data).To(HaveKeyWithValue("target_kind", "Node"))
			Expect(ev.Data).To(HaveKeyWithValue("target_name", "worker-1"))
		})
	})

	Describe("UT-KA-847-010: correction message names the offending kind", func() {
		It("should include the target kind in the correction message sent to the LLM (BR-AI-847)", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Node under disk pressure",
					"confidence":0.85,
					"remediation_target":{"kind":"Node","name":"worker-1"}
				}`}},
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary":"Pod caused it",
					"confidence":0.9,
					"remediation_target":{"kind":"Pod","name":"log-spammer","namespace":"default"}
				}`}},
				gateWfToolResp(`{"workflow_id":"evict-pod","confidence":0.9}`),
			}

			_, err := newInv().Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockClient.calls).To(HaveLen(3),
				"UT-KA-847-010: must have 3 LLM calls (RCA, gate retry, workflow)")
			gateCall := mockClient.calls[1]
			lastMsg := gateCall.Messages[len(gateCall.Messages)-1]
			Expect(lastMsg.Content).To(ContainSubstring("Node"),
				"UT-KA-847-010: correction message must name the offending kind")
			Expect(lastMsg.Content).To(ContainSubstring("root cause"),
				"UT-KA-847-010: correction message must ask the LLM to re-evaluate root cause")
		})
	})
})
