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
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// #1430 / BR-HAPI-200: When the RCA concludes no action is required
// (problem_resolved or predictive_no_action), the investigator should skip
// workflow discovery (Phase 3) entirely. This avoids unnecessary compute and
// latency for signals that the RCA already determined are resolved or benign.

var _ = Describe("Investigator skip workflow discovery (#1430 / BR-HAPI-200)", func() {

	var (
		invLogger    logr.Logger
		auditStore   *recordingAuditStore
		mockClient   *mockLLMClient
		builder      *prompt.Builder
		rp           *parser.ResultParser
		enricher     *enrichment.Enricher
		phaseTools   katypes.PhaseToolMap
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = &recordingAuditStore{}
		mockClient = &mockLLMClient{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		k8sClient := &fakeK8sClient{
			ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
			},
		}
		dsClient := &fakeDataStorageClient{}
		enricher = enrichment.NewEnricher(k8sClient, dsClient, auditStore, invLogger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	// IT-KA-1430-001: problem_resolved outcome skips workflow discovery.
	// FedRAMP AU-2: audit trail records investigation completed without discovery.
	// FedRAMP SI-4: the skip is observable (only 1 LLM call, not 2).
	Describe("IT-KA-1430-001: problem_resolved skips workflow discovery (AU-2, SI-4)", func() {
		It("should return without invoking Phase 3 when RCA outcome is problem_resolved", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary": "Network partition self-healed after pod restart",
					"investigation_outcome": "problem_resolved",
					"confidence": 0.90,
					"actionable": false
				}`}},
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production",
				Severity: "warning", Message: "NetworkPartition",
				Environment: "Production", Priority: "P1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(BeEmpty(),
				"#1430: no workflow should be selected for problem_resolved")
			Expect(result.InvestigationOutcome).To(Equal("problem_resolved"),
				"#1430: investigation_outcome must be preserved")
			Expect(result.IsActionable).NotTo(BeNil(),
				"#1430: IsActionable must be set by parser")
			Expect(*result.IsActionable).To(BeFalse(),
				"#1430: IsActionable must be false for problem_resolved")
			Expect(mockClient.calls).To(HaveLen(1),
				"#1430 / AU-2: only 1 LLM call (RCA) should occur — Phase 3 skipped")
		})
	})

	// IT-KA-1430-002: predictive_no_action outcome skips workflow discovery.
	// FedRAMP AU-2, SI-4: same observability as IT-KA-1430-001.
	Describe("IT-KA-1430-002: predictive_no_action skips workflow discovery (AU-2, SI-4)", func() {
		It("should return without invoking Phase 3 when RCA outcome is predictive_no_action", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary": "Alert predicted to clear within SLO window based on scaling trend",
					"investigation_outcome": "predictive_no_action",
					"confidence": 0.85,
					"actionable": false
				}`}},
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "worker-pod", Namespace: "staging",
				Severity: "low", Message: "HighLatency",
				Environment: "Staging", Priority: "P3",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(BeEmpty(),
				"#1430: no workflow for predictive_no_action")
			Expect(result.IsActionable).NotTo(BeNil())
			Expect(*result.IsActionable).To(BeFalse(),
				"#1430: IsActionable must be false for predictive_no_action")
			Expect(mockClient.calls).To(HaveLen(1),
				"#1430 / AU-2: only RCA call — Phase 3 skipped for predictive_no_action")
		})
	})

	// IT-KA-1430-003: short-circuit still emits response_complete audit event.
	// FedRAMP AU-3: audit content must include is_actionable and correlation_id.
	// FedRAMP AU-12: the audit event must be generated on the early-return path.
	Describe("IT-KA-1430-003: short-circuit emits response_complete audit (AU-3, AU-12)", func() {
		It("should emit a response_complete audit event with actionability data", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary": "Transient DNS resolution failure cleared",
					"investigation_outcome": "problem_resolved",
					"confidence": 0.92,
					"actionable": false
				}`}},
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "dns-pod", Namespace: "kube-system",
				Severity: "warning", Message: "DNSTimeout",
				RemediationID: "rr-dns-001",
				Environment: "Production", Priority: "P2",
			})
			Expect(err).NotTo(HaveOccurred())

			responseCompleteEvents := filterEvents(auditStore.events, audit.EventTypeResponseComplete)
			Expect(responseCompleteEvents).NotTo(BeEmpty(),
				"#1430 / AU-12: response_complete audit event must be emitted on short-circuit path")
			Expect(responseCompleteEvents[0].CorrelationID).To(Equal("rr-dns-001"),
				"#1430 / AU-3: correlation_id must match signal.RemediationID")
		})
	})

	// IT-KA-1430-004: skip is logged for observability.
	// FedRAMP SI-4: system monitoring must capture the decision to skip.
	Describe("IT-KA-1430-004: skip is logged for observability (SI-4)", func() {
		It("should emit a structured log when skipping workflow discovery", func() {
			var logMessages []string
			recordingLogger := funcr.New(func(prefix, args string) {
				logMessages = append(logMessages, args)
			}, funcr.Options{})

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary": "Problem self-resolved after auto-scaling",
					"investigation_outcome": "problem_resolved",
					"confidence": 0.88,
					"actionable": false
				}`}},
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: recordingLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "hpa-target", Namespace: "production",
				Severity: "low", Message: "ScalingEvent",
				RemediationID: "rr-hpa-002",
				Environment: "Production", Priority: "P3",
			})
			Expect(err).NotTo(HaveOccurred())

			found := false
			for _, msg := range logMessages {
				if containsAll(msg, "skipping workflow discovery", "problem_resolved") {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(),
				"#1430 / SI-4: structured log must contain 'skipping workflow discovery' with investigation_outcome")
		})
	})

	// IT-KA-1430-005: actionable=false WITH workflow_id does NOT skip (defense-in-depth).
	// Mirrors the AA response processor's !hasSelectedWorkflow guard.
	// #1431: Uses nested selected_workflow to verify the flat-path merge extracts
	// workflow_id from the nested object, preventing the short-circuit guard from
	// misfiring on hybrid JSON.
	Describe("IT-KA-1430-005: actionable=false with workflow_id does NOT skip", func() {
		It("should proceed to Phase 3 when RCA contradicts itself with a workflow_id", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary": "Problem resolved but workflow recommended as precaution",
					"investigation_outcome": "problem_resolved",
					"confidence": 0.80,
					"actionable": false,
					"selected_workflow": {
						"workflow_id": "wf-contradictory",
						"confidence": 0.80
					}
				}`}},
				wfToolResp(`{"workflow_id":"wf-contradictory","confidence":0.80,"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production"}}`),
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production",
				Severity: "warning", Message: "TransientError",
				Environment: "Production", Priority: "P2",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(mockClient.calls).To(HaveLen(2),
				"#1430: Phase 3 must still execute when RCA includes a workflow_id despite actionable=false")
		})
	})

	// IT-KA-1430-006: finalization steps (severity backfill, detected labels,
	// remediation target, TARGET_RESOURCE_* params) match the HumanReviewNeeded path.
	Describe("IT-KA-1430-006: finalization steps match HumanReviewNeeded path", func() {
		It("should backfill severity and inject remediation target on the short-circuit path", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary": "OOM condition self-healed after container restart",
					"investigation_outcome": "problem_resolved",
					"confidence": 0.95,
					"actionable": false
				}`}},
			}
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:         "api-server-abc",
				Namespace:    "production",
				Severity:     "critical",
				Message:      "OOMKilled",
				ResourceKind: "Pod",
				ResourceName: "api-server-abc",
				Environment:  "Production",
				Priority:     "P0",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			By("severity is backfilled from signal when RCA omits it")
			Expect(result.Severity).NotTo(BeEmpty(),
				"#1430: severity must be backfilled from signal context")

			By("remediation target is populated from enrichment owner chain")
			Expect(result.RemediationTarget.Kind).NotTo(BeEmpty(),
				"#1430: RemediationTarget.Kind must be populated via InjectRemediationTarget")

			By("TARGET_RESOURCE_* parameters are injected")
			Expect(result.Parameters).NotTo(BeNil(),
				"#1430: Parameters must be non-nil after InjectTargetResourceParameters")
			Expect(result.Parameters).To(HaveKey("TARGET_RESOURCE_KIND"),
				"#1430: TARGET_RESOURCE_KIND must be present in parameters")
			Expect(result.Parameters).To(HaveKey("TARGET_RESOURCE_NAME"),
				"#1430: TARGET_RESOURCE_NAME must be present in parameters")
		})
	})
})

func containsAll(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
