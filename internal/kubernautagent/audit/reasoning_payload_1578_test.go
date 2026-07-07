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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
)

// UT-KA-AUDIT-001 (audit layer): buildRCACompletePayload/toIncidentResponseData
// map the response_data JSON's "reasoning" object into
// ogenclient.IncidentResponseDataRootCauseAnalysis, so BR-AI-086 AC6's
// audit-trail-surfacing requirement (SOC2 CC7.2/CC8.1 reconstruction) is
// satisfied end-to-end from the audit-record layer down. Only visible text
// and a redacted flag are carried — never an opaque replay signature.
var _ = Describe("UT-KA-AUDIT-001: AIAgentRCACompletePayload carries reasoning", func() {
	It("should map reasoning.text and reasoning.redacted from response_data into RootCauseAnalysis", func() {
		recorder := &fakeOgenClient{}
		store := audit.NewDSAuditStore(recorder)

		event := audit.NewEvent(audit.EventTypeRCAComplete, "corr-rca-reasoning")
		event.Data["response_data"] = `{
			"rca_summary": "OOMKilled due to memory leak",
			"severity": "critical",
			"confidence": 0.9,
			"reasoning": {
				"text": "Sustained memory climb over 6h rules out a transient spike.",
				"redacted": false
			}
		}`

		err := store.StoreAudit(context.Background(), event)
		Expect(err).NotTo(HaveOccurred())

		payload, ok := recorder.calls[0].EventData.GetAIAgentRCACompletePayload()
		Expect(ok).To(BeTrue())

		reasoning, hasReasoning := payload.ResponseData.RootCauseAnalysis.Reasoning.Get()
		Expect(hasReasoning).To(BeTrue(), "reasoning must be present in RootCauseAnalysis when captured during RCA")
		Expect(reasoning).To(Equal("Sustained memory climb over 6h rules out a transient spike."))

		redacted, hasRedacted := payload.ResponseData.RootCauseAnalysis.ReasoningRedacted.Get()
		Expect(hasRedacted).To(BeTrue())
		Expect(redacted).To(BeFalse())
	})

	It("should map reasoning.redacted=true with empty text when the provider withheld reasoning content", func() {
		recorder := &fakeOgenClient{}
		store := audit.NewDSAuditStore(recorder)

		event := audit.NewEvent(audit.EventTypeRCAComplete, "corr-rca-redacted")
		event.Data["response_data"] = `{
			"rca_summary": "OOMKilled due to memory leak",
			"severity": "critical",
			"confidence": 0.9,
			"reasoning": {"redacted": true}
		}`

		err := store.StoreAudit(context.Background(), event)
		Expect(err).NotTo(HaveOccurred())

		payload, ok := recorder.calls[0].EventData.GetAIAgentRCACompletePayload()
		Expect(ok).To(BeTrue())

		redacted, hasRedacted := payload.ResponseData.RootCauseAnalysis.ReasoningRedacted.Get()
		Expect(hasRedacted).To(BeTrue())
		Expect(redacted).To(BeTrue())
	})

	It("should omit reasoning fields entirely when response_data carries no reasoning object (default-disabled behavior, BR-AI-086 AC2)", func() {
		recorder := &fakeOgenClient{}
		store := audit.NewDSAuditStore(recorder)

		event := audit.NewEvent(audit.EventTypeRCAComplete, "corr-rca-no-reasoning")
		event.Data["response_data"] = `{"rca_summary":"test","severity":"info","confidence":0.5}`

		err := store.StoreAudit(context.Background(), event)
		Expect(err).NotTo(HaveOccurred())

		payload, ok := recorder.calls[0].EventData.GetAIAgentRCACompletePayload()
		Expect(ok).To(BeTrue())

		_, hasReasoning := payload.ResponseData.RootCauseAnalysis.Reasoning.Get()
		Expect(hasReasoning).To(BeFalse(), "reasoning must stay unset when the LLM's reasoning capability was disabled")
	})
})

// UT-KA-AUDIT-003: buildLLMResponsePayload maps the per-turn reasoning
// text/redacted flag captured by emitLLMResponseAudit into LLMResponsePayload
// (aiagent.llm.response), extending the existing per-turn audit event
// (AU-3 content-of-audit-records) to cover reasoning, not just analysis
// content/tokens/tool calls.
var _ = Describe("UT-KA-AUDIT-003: LLMResponsePayload carries per-turn reasoning", func() {
	It("should populate reasoning_text and reasoning_redacted when the turn's response carried reasoning", func() {
		recorder := &fakeOgenClient{}
		store := audit.NewDSAuditStore(recorder)

		event := audit.NewEvent(audit.EventTypeLLMResponse, "corr-llm-resp-reasoning")
		event.Data["has_analysis"] = true
		event.Data["analysis_length"] = 20
		event.Data["analysis_preview"] = "OOMKilled root cause"
		event.Data["reasoning_text"] = "Weighing leak vs spike hypotheses before concluding."
		event.Data["reasoning_redacted"] = false

		err := store.StoreAudit(context.Background(), event)
		Expect(err).NotTo(HaveOccurred())

		payload, ok := recorder.calls[0].EventData.GetLLMResponsePayload()
		Expect(ok).To(BeTrue())

		text, hasText := payload.ReasoningText.Get()
		Expect(hasText).To(BeTrue())
		Expect(text).To(Equal("Weighing leak vs spike hypotheses before concluding."))

		redacted, hasRedacted := payload.ReasoningRedacted.Get()
		Expect(hasRedacted).To(BeTrue())
		Expect(redacted).To(BeFalse())
	})

	It("should omit reasoning fields when the turn's response carried no reasoning block", func() {
		recorder := &fakeOgenClient{}
		store := audit.NewDSAuditStore(recorder)

		event := audit.NewEvent(audit.EventTypeLLMResponse, "corr-llm-resp-no-reasoning")
		event.Data["has_analysis"] = true
		event.Data["analysis_length"] = 20
		event.Data["analysis_preview"] = "OOMKilled root cause"

		err := store.StoreAudit(context.Background(), event)
		Expect(err).NotTo(HaveOccurred())

		payload, ok := recorder.calls[0].EventData.GetLLMResponsePayload()
		Expect(ok).To(BeTrue())

		_, hasText := payload.ReasoningText.Get()
		Expect(hasText).To(BeFalse())
	})
})

// UT-KA-AUDIT-008: reasoning payload builders cap oversized reasoning text
// at the Data-Storage payload size guard (BR-AI-086 AC6 REFACTOR),
// defense-in-depth against a runaway/misconfigured extended-thinking
// budget bloating audit storage regardless of upstream producer behavior.
var _ = Describe("UT-KA-AUDIT-008: reasoning payload size guard", func() {
	It("should truncate an oversized reasoning_text on LLMResponsePayload", func() {
		recorder := &fakeOgenClient{}
		store := audit.NewDSAuditStore(recorder)
		oversized := strings.Repeat("x", 45000)

		event := audit.NewEvent(audit.EventTypeLLMResponse, "corr-llm-resp-oversized")
		event.Data["reasoning_text"] = oversized
		event.Data["reasoning_redacted"] = false

		err := store.StoreAudit(context.Background(), event)
		Expect(err).NotTo(HaveOccurred())

		payload, ok := recorder.calls[0].EventData.GetLLMResponsePayload()
		Expect(ok).To(BeTrue())
		text, hasText := payload.ReasoningText.Get()
		Expect(hasText).To(BeTrue())
		Expect(len(text)).To(BeNumerically("<", len(oversized)))
	})

	It("should truncate an oversized reasoning text on IncidentResponseDataRootCauseAnalysis", func() {
		recorder := &fakeOgenClient{}
		store := audit.NewDSAuditStore(recorder)
		oversized := strings.Repeat("y", 45000)

		event := audit.NewEvent(audit.EventTypeRCAComplete, "corr-rca-oversized")
		event.Data["response_data"] = fmt.Sprintf(
			`{"rca_summary":"test","severity":"info","confidence":0.5,"reasoning":{"text":"%s","redacted":false}}`,
			oversized)

		err := store.StoreAudit(context.Background(), event)
		Expect(err).NotTo(HaveOccurred())

		payload, ok := recorder.calls[0].EventData.GetAIAgentRCACompletePayload()
		Expect(ok).To(BeTrue())
		reasoning, hasReasoning := payload.ResponseData.RootCauseAnalysis.Reasoning.Get()
		Expect(hasReasoning).To(BeTrue())
		Expect(len(reasoning)).To(BeNumerically("<", len(oversized)))
	})
})
