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
)

// BR-SECURITY-1900 (AU-3, extends BR-AUDIT-005): the persisted audit-table
// event for a 401/403 must distinguish an audience-bound TokenReview
// mismatch (cross-service token replay) from a routine
// missing/malformed/expired token or generic authorization denial, so SOC2
// CC8.1 reconstruction can tell them apart without cross-referencing
// application logs.
var _ = Describe("DS Store — auth failure/denied reason mapping [BR-SECURITY-1900]", func() {

	Describe("UT-KA-1900-012: buildEventData for EventTypeAuthFailure includes reason when present", func() {
		It("maps Data[\"reason\"] into the persisted AIAgentAuthFailurePayload.Reason field", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := &audit.AuditEvent{
				EventType:     audit.EventTypeAuthFailure,
				EventCategory: audit.EventCategory,
				EventAction:   audit.ActionAuthFailure,
				EventOutcome:  audit.OutcomeFailure,
				Data: map[string]interface{}{
					"event_id":  "evt-af-001",
					"source_ip": "10.0.0.5",
					"path":      "/api/v1/mcp",
					"method":    "POST",
					"reason":    "invalid_token_audience",
				},
			}
			Expect(store.StoreAudit(context.Background(), event)).To(Succeed())
			Expect(recorder.calls).To(HaveLen(1))

			ed := recorder.calls[0].EventData
			payload, ok := ed.GetAIAgentAuthFailurePayload()
			Expect(ok).To(BeTrue(), "EventData must be AIAgentAuthFailurePayload")
			reason, isSet := payload.Reason.Get()
			Expect(isSet).To(BeTrue())
			Expect(reason).To(Equal("invalid_token_audience"))
		})

		It("omits Reason when Data has no \"reason\" key (backward compatible)", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := &audit.AuditEvent{
				EventType:     audit.EventTypeAuthFailure,
				EventCategory: audit.EventCategory,
				EventAction:   audit.ActionAuthFailure,
				EventOutcome:  audit.OutcomeFailure,
				Data: map[string]interface{}{
					"event_id": "evt-af-002",
				},
			}
			Expect(store.StoreAudit(context.Background(), event)).To(Succeed())

			ed := recorder.calls[0].EventData
			payload, ok := ed.GetAIAgentAuthFailurePayload()
			Expect(ok).To(BeTrue())
			_, isSet := payload.Reason.Get()
			Expect(isSet).To(BeFalse())
		})
	})

	Describe("UT-KA-1900-013: buildEventData for EventTypeAuthDenied includes reason when present", func() {
		It("maps Data[\"reason\"] into the persisted AIAgentAuthDeniedPayload.Reason field", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := &audit.AuditEvent{
				EventType:     audit.EventTypeAuthDenied,
				EventCategory: audit.EventCategory,
				EventAction:   audit.ActionAuthDenied,
				EventOutcome:  audit.OutcomeFailure,
				Data: map[string]interface{}{
					"event_id": "evt-ad-001",
					"reason":   "authorization_denied",
				},
			}
			Expect(store.StoreAudit(context.Background(), event)).To(Succeed())

			ed := recorder.calls[0].EventData
			payload, ok := ed.GetAIAgentAuthDeniedPayload()
			Expect(ok).To(BeTrue(), "EventData must be AIAgentAuthDeniedPayload")
			reason, isSet := payload.Reason.Get()
			Expect(isSet).To(BeTrue())
			Expect(reason).To(Equal("authorization_denied"))
		})
	})
})
