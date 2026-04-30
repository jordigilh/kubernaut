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
	"errors"
	"log/slog"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

type mockAuditQuerier struct {
	response *ogenclient.AuditEventsQueryResponse
	err      error
}

func (m *mockAuditQuerier) QueryAuditEvents(_ context.Context, _ ogenclient.QueryAuditEventsParams) (*ogenclient.AuditEventsQueryResponse, error) {
	return m.response, m.err
}

var _ = Describe("DSContextReconstructor — #703 BR-INTERACTIVE-007/008", func() {
	var (
		ctx    context.Context
		logger *slog.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})

	Describe("UT-KA-703-J01: Returns ordered ConversationTurns for correlationID", func() {
		It("should return turns ordered by timestamp", func() {
			t1 := time.Now().Add(-2 * time.Minute)
			t2 := time.Now().Add(-1 * time.Minute)
			querier := &mockAuditQuerier{
				response: &ogenclient.AuditEventsQueryResponse{
					Data: []ogenclient.AuditEvent{
						makeLLMRequestEvent("rr-100", "prompt A", t1),
						makeLLMResponseEvent("rr-100", "response B", t2),
					},
				},
			}

			r := mcpinternal.NewDSContextReconstructor(querier, 5*time.Second, logger)
			turns, err := r.Reconstruct(ctx, "rr-100", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(turns).To(HaveLen(2))
			Expect(turns[0].Role).To(Equal("user"))
			Expect(turns[1].Role).To(Equal("assistant"))
			Expect(turns[0].Timestamp.Before(turns[1].Timestamp)).To(BeTrue())
		})
	})

	Describe("UT-KA-703-J02: Excludes events from the current session by actor_id filter", func() {
		It("should filter out events whose actor_id matches the exclude user", func() {
			t1 := time.Now().Add(-2 * time.Minute)
			t2 := time.Now().Add(-1 * time.Minute)

			keepEvt := makeLLMRequestEvent("rr-200", "prior context", t1)
			keepEvt.ActorID.SetTo("kubernaut-agent")

			excludeEvt := makeLLMResponseEvent("rr-200", "current session response", t2)
			excludeEvt.ActorID.SetTo("alice@example.com")

			querier := &mockAuditQuerier{
				response: &ogenclient.AuditEventsQueryResponse{
					Data: []ogenclient.AuditEvent{keepEvt, excludeEvt},
				},
			}

			r := mcpinternal.NewDSContextReconstructor(querier, 5*time.Second, logger)
			turns, err := r.Reconstruct(ctx, "rr-200", "alice@example.com")
			Expect(err).NotTo(HaveOccurred())
			Expect(turns).To(HaveLen(1))
		})
	})

	Describe("UT-KA-703-J03: Returns empty slice (not error) when DS unavailable", func() {
		It("should return empty slice and log warning on DS failure", func() {
			querier := &mockAuditQuerier{
				err: errors.New("connection refused"),
			}

			r := mcpinternal.NewDSContextReconstructor(querier, 5*time.Second, logger)
			turns, err := r.Reconstruct(ctx, "rr-300", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(turns).To(BeEmpty())
		})
	})

	Describe("UT-KA-703-J04: Maps aiagent.llm.request/response to user/assistant roles", func() {
		It("should map request to user and response to assistant", func() {
			t1 := time.Now()
			querier := &mockAuditQuerier{
				response: &ogenclient.AuditEventsQueryResponse{
					Data: []ogenclient.AuditEvent{
						makeLLMRequestEvent("rr-400", "question", t1),
						makeLLMResponseEvent("rr-400", "answer", t1.Add(time.Second)),
					},
				},
			}

			r := mcpinternal.NewDSContextReconstructor(querier, 5*time.Second, logger)
			turns, err := r.Reconstruct(ctx, "rr-400", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(turns).To(HaveLen(2))
			Expect(turns[0].Role).To(Equal("user"))
			Expect(turns[1].Role).To(Equal("assistant"))
		})
	})

	Describe("UT-KA-703-J05: Handles empty DS response gracefully", func() {
		It("should return empty slice for empty DS response", func() {
			querier := &mockAuditQuerier{
				response: &ogenclient.AuditEventsQueryResponse{
					Data: []ogenclient.AuditEvent{},
				},
			}

			r := mcpinternal.NewDSContextReconstructor(querier, 5*time.Second, logger)
			turns, err := r.Reconstruct(ctx, "rr-500", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(turns).To(BeEmpty())
		})
	})
})

func makeLLMRequestEvent(correlationID, prompt string, ts time.Time) ogenclient.AuditEvent {
	evt := ogenclient.AuditEvent{
		Version:        "1.0",
		EventType:      "aiagent.llm.request",
		EventTimestamp: ts,
		EventCategory:  ogenclient.AuditEventEventCategoryAiagent,
		EventAction:    "llm_request",
		EventOutcome:   ogenclient.AuditEventEventOutcomeSuccess,
		CorrelationID:  correlationID,
	}
	evt.ActorType.SetTo("Service")
	evt.ActorID.SetTo("kubernaut-agent")
	payload := ogenclient.LLMRequestPayload{
		EventType:     ogenclient.LLMRequestPayloadEventTypeAiagentLlmRequest,
		EventID:       "evt-req-1",
		IncidentID:    correlationID,
		Model:         "test-model",
		PromptLength:  len(prompt),
		PromptPreview: prompt,
	}
	evt.EventData = ogenclient.NewLLMRequestPayloadAuditEventEventData(payload)
	return evt
}

func makeLLMResponseEvent(correlationID, analysis string, ts time.Time) ogenclient.AuditEvent {
	evt := ogenclient.AuditEvent{
		Version:        "1.0",
		EventType:      "aiagent.llm.response",
		EventTimestamp: ts,
		EventCategory:  ogenclient.AuditEventEventCategoryAiagent,
		EventAction:    "llm_response",
		EventOutcome:   ogenclient.AuditEventEventOutcomeSuccess,
		CorrelationID:  correlationID,
	}
	evt.ActorType.SetTo("Service")
	evt.ActorID.SetTo("kubernaut-agent")
	payload := ogenclient.LLMResponsePayload{
		EventType:       ogenclient.LLMResponsePayloadEventTypeAiagentLlmResponse,
		EventID:         "evt-resp-1",
		IncidentID:      correlationID,
		HasAnalysis:     true,
		AnalysisLength:  len(analysis),
		AnalysisPreview: analysis,
	}
	evt.EventData = ogenclient.NewLLMResponsePayloadAuditEventEventData(payload)
	return evt
}
