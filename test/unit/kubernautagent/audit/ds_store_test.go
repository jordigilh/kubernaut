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

package audit_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

var _ = Describe("Kubernaut Agent DS Audit Store — TP-433-WIR Phase 7", func() {

	Describe("UT-KA-433W-008: DSAuditStore maps event fields to ogen request", func() {
		It("should map EventType, EventCategory, CorrelationID, EventAction, EventOutcome", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := &audit.AuditEvent{
				EventType:     audit.EventTypeLLMRequest,
				EventCategory: audit.EventCategory,
				CorrelationID: "corr-123",
				EventAction:   "llm.request",
				EventOutcome:  "success",
			}
			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.calls).To(HaveLen(1))

			req := recorder.calls[0]
			Expect(req.EventType).To(Equal(audit.EventTypeLLMRequest))
			Expect(string(req.EventCategory)).To(Equal(audit.EventCategory))
			Expect(req.CorrelationID).To(Equal("corr-123"))
			Expect(req.EventAction).To(Equal("llm.request"))
			Expect(string(req.EventOutcome)).To(Equal("success"))
		})
	})

	Describe("UT-KA-433W-009: DSAuditStore propagates ogen client errors", func() {
		It("should return wrapped error when ogen client fails", func() {
			recorder := &fakeOgenClient{err: errFakeOgen}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeLLMResponse, "corr-456")
			err := store.StoreAudit(context.Background(), event)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("audit store"))
		})
	})

	Describe("UT-KA-433W-010: DSAuditStore builds enrichment.completed EventData payload", func() {
		It("should populate AIAgentEnrichmentCompletedPayload from event Data map", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeEnrichmentCompleted, "corr-enr")
			event.Data["event_id"] = "evt-001"
			event.Data["incident_id"] = "inc-001"
			event.Data["root_owner_kind"] = "Deployment"
			event.Data["root_owner_name"] = "api-server"
			event.Data["root_owner_namespace"] = "production"
			event.Data["owner_chain_length"] = 2
			event.Data["remediation_history_fetched"] = true

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.calls).To(HaveLen(1))

			req := recorder.calls[0]
			Expect(req.EventData.Type).To(Equal(ogenclient.AIAgentEnrichmentCompletedPayloadAuditEventRequestEventData))

			payload, ok := req.EventData.GetAIAgentEnrichmentCompletedPayload()
			Expect(ok).To(BeTrue(), "should extract AIAgentEnrichmentCompletedPayload")
			Expect(payload.EventID).To(Equal("evt-001"))
			Expect(payload.IncidentID).To(Equal("inc-001"))
			Expect(payload.RootOwnerKind).To(Equal("Deployment"))
			Expect(payload.RootOwnerName).To(Equal("api-server"))
			Expect(payload.RootOwnerNamespace.Value).To(Equal("production"))
			Expect(payload.OwnerChainLength).To(Equal(2))
			Expect(payload.RemediationHistoryFetched).To(BeTrue())
		})
	})

	Describe("UT-KA-433W-011: DSAuditStore builds enrichment.failed EventData payload", func() {
		It("should populate AIAgentEnrichmentFailedPayload from event Data map", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeEnrichmentFailed, "corr-enr")
			event.Data["event_id"] = "evt-002"
			event.Data["incident_id"] = "inc-002"
			event.Data["reason"] = "all_enrichment_sources_failed"
			event.Data["detail"] = "owner_chain: K8s down; history: DS down"
			event.Data["affected_resource_kind"] = "Pod"
			event.Data["affected_resource_name"] = "web-abc"
			event.Data["affected_resource_namespace"] = "default"

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.calls).To(HaveLen(1))

			req := recorder.calls[0]
			Expect(req.EventData.Type).To(Equal(ogenclient.AIAgentEnrichmentFailedPayloadAuditEventRequestEventData))

			payload, ok := req.EventData.GetAIAgentEnrichmentFailedPayload()
			Expect(ok).To(BeTrue(), "should extract AIAgentEnrichmentFailedPayload")
			Expect(payload.EventID).To(Equal("evt-002"))
			Expect(payload.IncidentID).To(Equal("inc-002"))
			Expect(payload.Reason).To(Equal("all_enrichment_sources_failed"))
			Expect(payload.Detail).To(ContainSubstring("K8s down"))
			Expect(payload.AffectedResourceKind).To(Equal("Pod"))
			Expect(payload.AffectedResourceName).To(Equal("web-abc"))
			Expect(payload.AffectedResourceNamespace.Value).To(Equal("default"))
		})
	})

	Describe("UT-KA-433W-012: DSAuditStore builds EventData for LLM request events", func() {
		It("should populate LLMRequestPayload EventData for LLM request events", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeLLMRequest, "corr-llm")
			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.calls).To(HaveLen(1))

			req := recorder.calls[0]
			Expect(req.EventData.Type).To(Equal(ogenclient.LLMRequestPayloadAuditEventRequestEventData),
				"LLM request events should have LLMRequestPayload EventData")
		})
	})

	Describe("UT-KA-PR9-001: buildEventData coverage for all AllEventTypes (MNT-2)", func() {
		It("should produce a non-zero EventData type for every event type in AllEventTypes", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			for _, eventType := range audit.AllEventTypes {
				event := audit.NewEvent(eventType, "corr-coverage")
				event.EventAction = "test_action"
				event.EventOutcome = audit.OutcomeSuccess
				event.Data["session_id"] = "sess-001"
				event.Data["incident_id"] = "inc-001"
				event.Data["model"] = "gpt-4"
				event.Data["prompt_length"] = 100
				event.Data["prompt_preview"] = "test"
				event.Data["has_analysis"] = true
				event.Data["analysis_length"] = 50
				event.Data["analysis_preview"] = "test"
				event.Data["tool_call_index"] = 0
				event.Data["tool_name"] = "test_tool"
				event.Data["tool_result"] = "{}"
				event.Data["user_id"] = "user1"
				event.Data["question"] = "why?"
				event.Data["attempt"] = 1
				event.Data["max_attempts"] = 3
				event.Data["is_valid"] = true
				event.Data["root_owner_kind"] = "Deployment"
				event.Data["root_owner_name"] = "api"
				event.Data["owner_chain_length"] = 1
				event.Data["remediation_history_fetched"] = true
				event.Data["reason"] = "test"
				event.Data["detail"] = "test"
				event.Data["affected_resource_kind"] = "Pod"
				event.Data["affected_resource_name"] = "pod-1"
				event.Data["error_message"] = "test"
				event.Data["phase"] = "rca"
				event.Data["cancelled_phase"] = "rca"
				event.Data["cancelled_at_turn"] = 3
				event.Data["endpoint"] = "/api/test"
				event.Data["requesting_user"] = "attacker"
				event.Data["step_index"] = 1
				event.Data["step_kind"] = "tool_call"
				event.Data["explanation"] = "suspicious"
				event.Data["result"] = "aligned"
				event.Data["summary"] = "ok"
				event.Data["flagged"] = 0
				event.Data["total"] = 5

				recorder.calls = nil
				err := store.StoreAudit(context.Background(), event)
				Expect(err).NotTo(HaveOccurred(), "StoreAudit should succeed for %s", eventType)
				Expect(recorder.calls).To(HaveLen(1), "should record call for %s", eventType)
				Expect(recorder.calls[0].EventData.Type).NotTo(BeZero(),
					"buildEventData should return a non-zero type for %s", eventType)
			}
		})
	})

	Describe("UT-KA-PR9-002: DSAuditStore builds session.started payload (AUD-2)", func() {
		It("should populate AIAgentSessionStartedPayload with enriched fields", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeSessionStarted, "rem-001")
			event.EventAction = audit.ActionSessionStarted
			event.EventOutcome = audit.OutcomeSuccess
			event.Data["session_id"] = "sess-001"
			event.Data["incident_id"] = "inc-001"
			event.Data["signal_name"] = "OOMKilled"
			event.Data["severity"] = "critical"
			event.Data["created_by"] = "system:serviceaccount:test:sa"

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.calls).To(HaveLen(1))

			req := recorder.calls[0]
			payload, ok := req.EventData.GetAIAgentSessionStartedPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.SessionID).To(Equal("sess-001"))
			Expect(payload.IncidentID.Value).To(Equal("inc-001"))
			Expect(payload.SignalName.Value).To(Equal("OOMKilled"))
			Expect(payload.Severity.Value).To(Equal("critical"))
			Expect(payload.CreatedBy.Value).To(Equal("system:serviceaccount:test:sa"))
		})
	})

	Describe("UT-KA-PR9-003: DSAuditStore builds session.access_denied payload (SEC-2)", func() {
		It("should populate AIAgentSessionAccessDeniedPayload with session_owner", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeSessionAccessDenied, "rem-002")
			event.EventAction = audit.ActionSessionAccessDenied
			event.EventOutcome = audit.OutcomeFailure
			event.Data["session_id"] = "sess-002"
			event.Data["endpoint"] = "/api/v1/incident/session/sess-002/status"
			event.Data["requesting_user"] = "attacker-user"
			event.Data["session_owner"] = "owner-user"

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			payload, ok := recorder.calls[0].EventData.GetAIAgentSessionAccessDeniedPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.SessionID).To(Equal("sess-002"))
			Expect(payload.Endpoint).To(Equal("/api/v1/incident/session/sess-002/status"))
			Expect(payload.RequestingUser).To(Equal("attacker-user"))
			Expect(payload.SessionOwner.Value).To(Equal("owner-user"))
		})
	})

	Describe("UT-KA-PR9-004: DSAuditStore builds investigation.cancelled payload (COR-2)", func() {
		It("should populate AIAgentInvestigationCancelledPayload with tokens and messages", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeInvestigationCancelled, "rem-003")
			event.EventAction = audit.ActionInvestigationCancelled
			event.EventOutcome = audit.OutcomeFailure
			event.Data["cancelled_phase"] = "rca"
			event.Data["cancelled_at_turn"] = 5
			event.Data["total_prompt_tokens"] = 1000
			event.Data["total_completion_tokens"] = 500
			event.Data["total_tokens"] = 1500
			event.Data["accumulated_messages"] = `[{"role":"user","content":"test"}]`

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			payload, ok := recorder.calls[0].EventData.GetAIAgentInvestigationCancelledPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.CancelledPhase).To(Equal("rca"))
			Expect(payload.CancelledAtTurn).To(Equal(5))
			Expect(payload.TotalPromptTokens.Value).To(Equal(1000))
			Expect(payload.TotalCompletionTokens.Value).To(Equal(500))
			Expect(payload.TotalTokens.Value).To(Equal(1500))
			Expect(payload.AccumulatedMessages.Value).To(ContainSubstring("test"))
		})
	})

	Describe("UT-KA-PR9-005: DSAuditStore builds alignment.step payload (AUD-5)", func() {
		It("should populate AIAgentAlignmentStepPayload with step details", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeAlignmentStep, "rem-004")
			event.EventAction = audit.ActionAlignmentEvaluate
			event.EventOutcome = audit.OutcomeFailure
			event.Data["step_index"] = 2
			event.Data["step_kind"] = "tool_call"
			event.Data["tool"] = "kubectl_exec"
			event.Data["explanation"] = "Attempted to exec into pod — suspicious"

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			payload, ok := recorder.calls[0].EventData.GetAIAgentAlignmentStepPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.StepIndex).To(Equal(2))
			Expect(payload.StepKind).To(Equal("tool_call"))
			Expect(payload.Tool.Value).To(Equal("kubectl_exec"))
			Expect(payload.Explanation).To(ContainSubstring("suspicious"))
		})
	})

	Describe("UT-KA-PR9-006: DSAuditStore builds alignment.verdict payload (AUD-5)", func() {
		It("should populate AIAgentAlignmentVerdictPayload with verdict details", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeAlignmentVerdict, "rem-005")
			event.EventAction = audit.ActionAlignmentVerdict
			event.EventOutcome = audit.OutcomeSuccess
			event.Data["result"] = "aligned"
			event.Data["summary"] = "All steps within bounds"
			event.Data["flagged"] = 0
			event.Data["total"] = 8

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			payload, ok := recorder.calls[0].EventData.GetAIAgentAlignmentVerdictPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.Result).To(Equal("aligned"))
			Expect(payload.Summary.Value).To(Equal("All steps within bounds"))
			Expect(payload.Flagged).To(Equal(0))
			Expect(payload.Total).To(Equal(8))
		})
	})

	Describe("UT-KA-PR9-007: DSAuditStore builds session.observed payload (SEC-4)", func() {
		It("should populate AIAgentSessionObservedPayload with observer and owner", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeSessionObserved, "rem-006")
			event.EventAction = audit.ActionSessionObserved
			event.EventOutcome = audit.OutcomeSuccess
			event.Data["session_id"] = "sess-006"
			event.Data["observer_user"] = "operator-1"
			event.Data["session_owner"] = "sa-initiator"

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			payload, ok := recorder.calls[0].EventData.GetAIAgentSessionObservedPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.SessionID).To(Equal("sess-006"))
			Expect(payload.ObserverUser.Value).To(Equal("operator-1"))
			Expect(payload.SessionOwner.Value).To(Equal("sa-initiator"))
		})
	})
})
