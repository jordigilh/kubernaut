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
})
