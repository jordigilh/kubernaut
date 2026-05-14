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

var _ = Describe("DS Store — Shadow LLM Audit Event Mapping (#1059)", func() {

	Describe("UT-SA-1059-010: buildEventData for EventTypeShadowLLMRequest", func() {
		It("should produce a ShadowLLMRequestPayload with correct fields", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := &audit.AuditEvent{
				EventType:     audit.EventTypeShadowLLMRequest,
				EventCategory: audit.EventCategory,
				CorrelationID: "rr-ds-001",
				EventAction:   audit.ActionShadowLLMRequest,
				EventOutcome:  audit.OutcomePending,
				Data: map[string]interface{}{
					"event_id":      "evt-001",
					"step_index":    3,
					"step_kind":     "tool_result",
					"prompt_length": 512,
				},
			}
			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.calls).To(HaveLen(1))

			ed := recorder.calls[0].EventData
			payload, ok := ed.GetShadowLLMRequestPayload()
			Expect(ok).To(BeTrue(), "EventData must be ShadowLLMRequestPayload")
			Expect(payload.EventType).To(Equal(ogenclient.ShadowLLMRequestPayloadEventTypeAiagentShadowLlmRequest))
			Expect(payload.EventID).To(Equal("evt-001"))
			Expect(payload.IncidentID).To(Equal("rr-ds-001"))
			Expect(payload.StepIndex).To(Equal(3))
			Expect(payload.StepKind).To(Equal("tool_result"))
			Expect(payload.PromptLength).To(Equal(512))
		})
	})

	Describe("UT-SA-1059-011: buildEventData for EventTypeShadowLLMResponse", func() {
		It("should produce a ShadowLLMResponsePayload with correct token fields", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := &audit.AuditEvent{
				EventType:     audit.EventTypeShadowLLMResponse,
				EventCategory: audit.EventCategory,
				CorrelationID: "rr-ds-002",
				EventAction:   audit.ActionShadowLLMResponse,
				EventOutcome:  audit.OutcomeSuccess,
				Data: map[string]interface{}{
					"event_id":          "evt-002",
					"step_index":        1,
					"step_kind":         "llm_reasoning",
					"prompt_tokens":     100,
					"completion_tokens": 200,
					"total_tokens":      300,
					"attempt":           2,
					"evaluation_result": "success",
				},
			}
			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.calls).To(HaveLen(1))

			ed := recorder.calls[0].EventData
			payload, ok := ed.GetShadowLLMResponsePayload()
			Expect(ok).To(BeTrue(), "EventData must be ShadowLLMResponsePayload")
			Expect(payload.EventType).To(Equal(ogenclient.ShadowLLMResponsePayloadEventTypeAiagentShadowLlmResponse))
			Expect(payload.EventID).To(Equal("evt-002"))
			Expect(payload.IncidentID).To(Equal("rr-ds-002"))
			Expect(payload.StepIndex).To(Equal(1))
			Expect(payload.StepKind).To(Equal("llm_reasoning"))
			Expect(payload.PromptTokens).To(Equal(100))
			Expect(payload.CompletionTokens).To(Equal(200))
			Expect(payload.TotalTokens).To(Equal(300))
			Expect(payload.Attempt.Value).To(Equal(2))
			Expect(payload.EvaluationResult.Value).To(Equal(
				ogenclient.ShadowLLMResponsePayloadEvaluationResultSuccess))
		})
	})

	Describe("UT-SA-1059-012: AlignmentVerdict payload includes shadow token totals", func() {
		It("should map shadow_prompt_tokens, shadow_completion_tokens, shadow_total_tokens", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := &audit.AuditEvent{
				EventType:     audit.EventTypeAlignmentVerdict,
				EventCategory: audit.EventCategory,
				CorrelationID: "rr-ds-003",
				EventAction:   audit.ActionAlignmentVerdict,
				EventOutcome:  audit.OutcomeSuccess,
				Data: map[string]interface{}{
					"event_id":                  "evt-003",
					"result":                    "aligned",
					"summary":                   "all steps passed",
					"flagged":                   0,
					"total":                     5,
					"shadow_prompt_tokens":      150,
					"shadow_completion_tokens":  250,
					"shadow_total_tokens":       400,
				},
			}
			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.calls).To(HaveLen(1))

			ed := recorder.calls[0].EventData
			payload, ok := ed.GetAIAgentAlignmentVerdictPayload()
			Expect(ok).To(BeTrue(), "EventData must be AlignmentVerdictPayload")
			Expect(payload.ShadowPromptTokens.Value).To(Equal(150))
			Expect(payload.ShadowCompletionTokens.Value).To(Equal(250))
			Expect(payload.ShadowTotalTokens.Value).To(Equal(400))
		})
	})

	Describe("UT-SA-1059-013: AlignmentVerdict payload omits shadow tokens when zero", func() {
		It("should not set optional shadow token fields when values are zero", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := &audit.AuditEvent{
				EventType:     audit.EventTypeAlignmentVerdict,
				EventCategory: audit.EventCategory,
				CorrelationID: "rr-ds-004",
				EventAction:   audit.ActionAlignmentVerdict,
				EventOutcome:  audit.OutcomeSuccess,
				Data: map[string]interface{}{
					"event_id": "evt-004",
					"result":   "aligned",
					"flagged":  0,
					"total":    5,
				},
			}
			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.calls).To(HaveLen(1))

			ed := recorder.calls[0].EventData
			payload, ok := ed.GetAIAgentAlignmentVerdictPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.ShadowPromptTokens.Set).To(BeFalse(), "shadow_prompt_tokens should not be set when zero")
			Expect(payload.ShadowCompletionTokens.Set).To(BeFalse(), "shadow_completion_tokens should not be set when zero")
			Expect(payload.ShadowTotalTokens.Set).To(BeFalse(), "shadow_total_tokens should not be set when zero")
		})
	})
})
