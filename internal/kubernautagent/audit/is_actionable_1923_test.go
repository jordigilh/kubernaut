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

// UT-KA-1923-001..003 (audit layer): buildResponseCompletePayload/toIncidentResponseData
// map the response_data JSON's "is_actionable" field into
// ogenclient.IncidentResponseData.IsActionable, closing the audit-completeness
// gap from Issue #1923 -- KA already computes is_actionable
// (InvestigationResult.IsActionable *bool) but it was silently dropped before
// reaching DataStorage's persisted schema (BR-AUDIT-005 v2.0, SOC2 CC8.1
// reconstruction, FedRAMP AU-3 structured audit content).
var _ = Describe("UT-KA-1923: AIAgentResponsePayload carries is_actionable", func() {
	It("UT-KA-1923-001: should map is_actionable=true from response_data into IncidentResponseData", func() {
		recorder := &fakeOgenClient{}
		store := audit.NewDSAuditStore(recorder)

		event := audit.NewEvent(audit.EventTypeResponseComplete, "corr-actionable-true")
		event.Data["response_data"] = `{
			"rca_summary": "OOMKilled due to memory leak",
			"severity": "high",
			"confidence": 0.9,
			"is_actionable": true
		}`

		err := store.StoreAudit(context.Background(), event)
		Expect(err).NotTo(HaveOccurred())

		payload, ok := recorder.calls[0].EventData.GetAIAgentResponsePayload()
		Expect(ok).To(BeTrue())

		actionable, isSet := payload.ResponseData.IsActionable.Get()
		Expect(isSet).To(BeTrue(), "is_actionable must be set when present in response_data")
		Expect(actionable).To(BeTrue())
	})

	It("UT-KA-1923-002: should map is_actionable=false from response_data into IncidentResponseData", func() {
		recorder := &fakeOgenClient{}
		store := audit.NewDSAuditStore(recorder)

		event := audit.NewEvent(audit.EventTypeRCAComplete, "corr-actionable-false")
		event.Data["response_data"] = `{
			"rca_summary": "transient blip, self-recovered",
			"severity": "info",
			"confidence": 0.6,
			"is_actionable": false
		}`

		err := store.StoreAudit(context.Background(), event)
		Expect(err).NotTo(HaveOccurred())

		payload, ok := recorder.calls[0].EventData.GetAIAgentRCACompletePayload()
		Expect(ok).To(BeTrue())

		actionable, isSet := payload.ResponseData.IsActionable.Get()
		Expect(isSet).To(BeTrue(), "is_actionable must be set when explicitly false")
		Expect(actionable).To(BeFalse())
	})

	It("UT-KA-1923-003: should leave is_actionable unset when response_data has no is_actionable key (pre-#1923 audit JSON shape)", func() {
		recorder := &fakeOgenClient{}
		store := audit.NewDSAuditStore(recorder)

		event := audit.NewEvent(audit.EventTypeResponseComplete, "corr-actionable-absent")
		event.Data["response_data"] = `{"rca_summary":"pre-1923 audit record","severity":"info","confidence":0.5}`

		err := store.StoreAudit(context.Background(), event)
		Expect(err).NotTo(HaveOccurred())

		payload, ok := recorder.calls[0].EventData.GetAIAgentResponsePayload()
		Expect(ok).To(BeTrue())

		_, isSet := payload.ResponseData.IsActionable.Get()
		Expect(isSet).To(BeFalse(), "is_actionable must stay unset for audit records that predate Issue #1923's field, not default to false")
	})
})
