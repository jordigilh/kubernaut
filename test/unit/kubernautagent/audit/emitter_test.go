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
	"errors"
	"log/slog"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
)

type mockAuditStore struct {
	events []*audit.AuditEvent
	err    error
}

func (m *mockAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, event)
	return nil
}

var _ = Describe("Kubernaut Agent Audit Emitter — #433", func() {

	Describe("UT-KA-433-005: Audit event factory produces correct event_type and event_category", func() {
		DescribeTable("should create correct event for each type",
			func(eventType string) {
				event := audit.NewEvent(eventType, "corr-123")
				Expect(event).NotTo(BeNil(), "NewEvent should not return nil")
				Expect(event.EventType).To(Equal(eventType))
				Expect(event.EventCategory).To(Equal(audit.EventCategory))
				Expect(event.CorrelationID).To(Equal("corr-123"))
			},
			Entry("aiagent.llm.request", audit.EventTypeLLMRequest),
			Entry("aiagent.llm.response", audit.EventTypeLLMResponse),
			Entry("aiagent.llm.tool_call", audit.EventTypeLLMToolCall),
			Entry("aiagent.conversation.turn", audit.EventTypeConversationTurn),
			Entry("aiagent.workflow.validation_attempt", audit.EventTypeValidationAttempt),
			Entry("aiagent.response.complete", audit.EventTypeResponseComplete),
			Entry("aiagent.rca.complete", audit.EventTypeRCAComplete),
			Entry("aiagent.response.failed", audit.EventTypeResponseFailed),
			Entry("aiagent.enrichment.completed", audit.EventTypeEnrichmentCompleted),
			Entry("aiagent.enrichment.failed", audit.EventTypeEnrichmentFailed),
			Entry("aiagent.alignment.step", audit.EventTypeAlignmentStep),
			Entry("aiagent.alignment.verdict", audit.EventTypeAlignmentVerdict),
		)

		It("should define exactly 12 event types", func() {
			Expect(audit.AllEventTypes).To(HaveLen(12))
		})

		It("should include aiagent.rca.complete in AllEventTypes", func() {
			Expect(audit.AllEventTypes).To(ContainElement(audit.EventTypeRCAComplete))
			Expect(audit.EventTypeRCAComplete).To(Equal("aiagent.rca.complete"))
		})
	})

	Describe("UT-KA-433-013: Audit best-effort helper does not propagate errors", func() {
		It("should not panic or return error when StoreAudit fails", func() {
			store := &mockAuditStore{err: errors.New("audit store unavailable")}
			event := &audit.AuditEvent{
				EventType:     audit.EventTypeLLMRequest,
				EventCategory: audit.EventCategory,
				CorrelationID: "corr-456",
			}
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

			Expect(func() {
				audit.StoreBestEffort(context.Background(), store, event, logger)
			}).NotTo(Panic())
		})

		It("should successfully store event when store is healthy", func() {
			store := &mockAuditStore{}
			event := &audit.AuditEvent{
				EventType:     audit.EventTypeLLMRequest,
				EventCategory: audit.EventCategory,
				CorrelationID: "corr-789",
			}
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

			audit.StoreBestEffort(context.Background(), store, event, logger)
			Expect(store.events).To(HaveLen(1))
			Expect(store.events[0].CorrelationID).To(Equal("corr-789"))
		})
	})
})
