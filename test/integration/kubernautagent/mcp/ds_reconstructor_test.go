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

package mcp_test

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

var _ = Describe("DSContextReconstructor IT — BR-INTERACTIVE-009", Label("integration", "reconstructor"), func() {

	var logger logr.Logger

	BeforeEach(func() {
		logger = logr.Discard()
	})

	Describe("IT-KA-RECON-001: reconstruct returns empty when no events exist", func() {
		It("should return zero conversation turns for a fresh session", func() {
			Expect(sharedDSClient).NotTo(BeNil(), "shared DS client must be initialized by suite")

			recon := mcpinternal.NewDSContextReconstructor(sharedDSClient, 5*time.Second, logger)
			turns, err := recon.Reconstruct(context.Background(), "rr-new", "sess-new")
			Expect(err).NotTo(HaveOccurred())
			Expect(turns).To(BeEmpty())
		})
	})

	Describe("IT-KA-RECON-002: reconstruct happy path with seeded LLM events", func() {
		It("should return ordered conversation turns from real DS events", func() {
			Expect(sharedDSClient).NotTo(BeNil(), "shared DS client must be initialized by suite")

			ctx := context.Background()
			corrID := fmt.Sprintf("rr-recon-it-%s", uuid.New().String()[:8])
			baseTime := time.Now().Add(-30 * time.Second).UTC()

			By("Seeding an LLM request event (user turn)")
			reqEvt := ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "aiagent.llm.request",
				EventTimestamp: baseTime,
				EventCategory:  ogenclient.AuditEventRequestEventCategoryAiagent,
				EventAction:    "llm.request",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorID:        ogenclient.OptString{Value: "agent-prior-session", Set: true},
				CorrelationID:  corrID,
			}
			reqEvt.EventData.SetLLMRequestPayload(ogenclient.LLMRequestPayload{
				EventType:     ogenclient.LLMRequestPayloadEventTypeAiagentLlmRequest,
				EventID:       uuid.New().String(),
				IncidentID:    corrID,
				Model:         "test-model",
				PromptLength:  42,
				PromptPreview: "What is the root cause of the OOMKill?",
			})
			_, err := sharedDSClient.CreateAuditEvent(ctx, &reqEvt)
			Expect(err).NotTo(HaveOccurred(), "seed LLM request event")

			By("Seeding an LLM response event (assistant turn)")
			respEvt := ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "aiagent.llm.response",
				EventTimestamp: baseTime.Add(2 * time.Second),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryAiagent,
				EventAction:    "llm.response",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorID:        ogenclient.OptString{Value: "agent-prior-session", Set: true},
				CorrelationID:  corrID,
			}
			respEvt.EventData.SetLLMResponsePayload(ogenclient.LLMResponsePayload{
				EventType:       ogenclient.LLMResponsePayloadEventTypeAiagentLlmResponse,
				EventID:         uuid.New().String(),
				IncidentID:      corrID,
				HasAnalysis:     true,
				AnalysisLength:  100,
				AnalysisPreview: "The container was OOMKilled due to memory limit of 256Mi.",
			})
			_, err = sharedDSClient.CreateAuditEvent(ctx, &respEvt)
			Expect(err).NotTo(HaveOccurred(), "seed LLM response event")

			By("Reconstructing context from seeded events")
			recon := mcpinternal.NewDSContextReconstructor(sharedDSClient, 5*time.Second, logger)
			turns, err := recon.Reconstruct(ctx, corrID, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(turns).To(HaveLen(2), "should have 2 turns (request + response)")

			Expect(turns[0].Role).To(Equal("user"))
			Expect(turns[0].Content).To(ContainSubstring("OOMKill"))
			Expect(turns[1].Role).To(Equal("assistant"))
			Expect(turns[1].Content).To(ContainSubstring("OOMKilled"))

			By("Verifying chronological ordering")
			Expect(turns[0].Timestamp.Before(turns[1].Timestamp)).To(BeTrue(),
				"user turn should be before assistant turn")
		})

		It("should exclude events matching the specified session ID", func() {
			Expect(sharedDSClient).NotTo(BeNil(), "shared DS client must be initialized by suite")

			ctx := context.Background()
			corrID := fmt.Sprintf("rr-recon-excl-%s", uuid.New().String()[:8])
			baseTime := time.Now().Add(-30 * time.Second).UTC()

			By("Seeding events from prior session")
			reqEvt := ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "aiagent.llm.request",
				EventTimestamp: baseTime,
				EventCategory:  ogenclient.AuditEventRequestEventCategoryAiagent,
				EventAction:    "llm.request",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorID:        ogenclient.OptString{Value: "prior-agent", Set: true},
				CorrelationID:  corrID,
			}
			reqEvt.EventData.SetLLMRequestPayload(ogenclient.LLMRequestPayload{
				EventType:     ogenclient.LLMRequestPayloadEventTypeAiagentLlmRequest,
				EventID:       uuid.New().String(),
				IncidentID:    corrID,
				Model:         "test-model",
				PromptLength:  20,
				PromptPreview: "prior session prompt",
			})
			_, err := sharedDSClient.CreateAuditEvent(ctx, &reqEvt)
			Expect(err).NotTo(HaveOccurred())

			By("Seeding events from current session (should be excluded)")
			currEvt := ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventType:      "aiagent.llm.request",
				EventTimestamp: baseTime.Add(5 * time.Second),
				EventCategory:  ogenclient.AuditEventRequestEventCategoryAiagent,
				EventAction:    "llm.request",
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				ActorID:        ogenclient.OptString{Value: "current-session", Set: true},
				CorrelationID:  corrID,
			}
			currEvt.EventData.SetLLMRequestPayload(ogenclient.LLMRequestPayload{
				EventType:     ogenclient.LLMRequestPayloadEventTypeAiagentLlmRequest,
				EventID:       uuid.New().String(),
				IncidentID:    corrID,
				Model:         "test-model",
				PromptLength:  30,
				PromptPreview: "current session prompt",
			})
			_, err = sharedDSClient.CreateAuditEvent(ctx, &currEvt)
			Expect(err).NotTo(HaveOccurred())

			By("Reconstructing with exclusion of current session")
			recon := mcpinternal.NewDSContextReconstructor(sharedDSClient, 5*time.Second, logger)
			turns, err := recon.Reconstruct(ctx, corrID, "current-session")
			Expect(err).NotTo(HaveOccurred())
			Expect(turns).To(HaveLen(1), "should exclude current session events")
			Expect(turns[0].Content).To(ContainSubstring("prior session"))
		})
	})

	Describe("IT-KA-RECON-003: reconstruct respects context cancellation", func() {
		It("should return empty turns when context is already cancelled", func() {
			Expect(sharedDSClient).NotTo(BeNil(), "shared DS client must be initialized by suite")

			recon := mcpinternal.NewDSContextReconstructor(sharedDSClient, 5*time.Second, logger)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			turns, err := recon.Reconstruct(ctx, "rr-cancelled", "sess-cancelled")
			Expect(err).NotTo(HaveOccurred(), "best-effort: should return empty on cancellation")
			Expect(turns).To(BeEmpty())
		})
	})
})
