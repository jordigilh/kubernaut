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

package audit

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Investigation decision ledger audit payload — #2520", func() {
	It("UT-KA-2520-001: registers the typed decision event in the audit payload registry", func() {
		event := NewEvent(EventTypeInvestigationDecision, "rr-1")
		event.EventAction = "investigation_decision"
		event.EventOutcome = "success"
		event.Data["phase"] = "rca"
		event.Data["turn_kind"] = "state_transition"
		event.Data["terminal_outcome"] = "evidence_transport_unavailable"
		event.Data["workflow_discovery_status"] = "skipped_due_to_failure"

		payload, ok := buildEventData(event)
		Expect(ok).To(BeTrue())
		raw, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).To(ContainSubstring("aiagent.investigation.decision"))
		Expect(string(raw)).To(ContainSubstring("evidence_transport_unavailable"))
	})

	It("UT-KA-2520-002: excludes prompts, reasoning, secrets, and unrestricted tool data", func() {
		event := NewEvent(EventTypeInvestigationDecision, "rr-1")
		event.EventAction = "investigation_decision"
		event.EventOutcome = "failure"
		event.Data["phase"] = "rca"
		event.Data["turn_kind"] = "terminal_decision"
		event.Data["terminal_outcome"] = "evidence_transport_unavailable"
		event.Data["prompt"] = "authorization: Bearer secret-token"
		event.Data["reasoning"] = "private chain of thought"
		event.Data["tool_arguments"] = `{"password":"secret"}`
		event.Data["raw_transport_error"] = "upstream reset with credentials"

		payload, ok := buildEventData(event)
		Expect(ok).To(BeTrue())
		raw, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).NotTo(ContainSubstring("Bearer secret-token"))
		Expect(string(raw)).NotTo(ContainSubstring("private chain of thought"))
		Expect(string(raw)).NotTo(ContainSubstring(`"password":"secret"`))
		Expect(string(raw)).To(ContainSubstring("evidence_transport_unavailable"))

		boundedEvent := NewEvent(EventTypeInvestigationDecision, "rr-1")
		boundedEvent.Data["phase"] = "workflow_discovery"
		boundedEvent.Data["turn_kind"] = "state_transition"
		boundedEvent.Data["turn_id"] = strings.Repeat("t", decisionLedgerStringMaxLen+32)
		boundedEvent.Data["model"] = strings.Repeat("m", decisionLedgerModelMaxLen+32)
		boundedEvent.Data["terminal_outcome"] = "workflow_discovery_failed"
		boundedEvent.Data["workflow_discovery_status"] = "failed"
		boundedEvent.Data["related_event_ids"] = make([]string, decisionLedgerMaxRelated+8)
		for i := range boundedEvent.Data["related_event_ids"].([]string) {
			boundedEvent.Data["related_event_ids"].([]string)[i] = strings.Repeat("e", decisionLedgerStringMaxLen+8)
		}

		boundedPayload, boundedOK := buildEventData(boundedEvent)
		Expect(boundedOK).To(BeTrue())
		boundedRaw, boundedErr := json.Marshal(boundedPayload)
		Expect(boundedErr).NotTo(HaveOccurred())
		var encoded struct {
			Model           string   `json:"model"`
			TurnID          string   `json:"turn_id"`
			RelatedEventIDs []string `json:"related_event_ids"`
		}
		Expect(json.Unmarshal(boundedRaw, &encoded)).To(Succeed())
		Expect(encoded.Model).To(HaveLen(decisionLedgerModelMaxLen))
		Expect(encoded.TurnID).To(HaveLen(decisionLedgerStringMaxLen))
		Expect(encoded.RelatedEventIDs).To(HaveLen(decisionLedgerMaxRelated))
		for _, id := range encoded.RelatedEventIDs {
			Expect(id).To(HaveLen(decisionLedgerStringMaxLen))
		}
	})
})
