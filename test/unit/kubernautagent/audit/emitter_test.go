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

	"github.com/go-logr/logr"
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
			Entry("aiagent.workflow.validation_attempt", audit.EventTypeValidationAttempt),
			Entry("aiagent.response.complete", audit.EventTypeResponseComplete),
			Entry("aiagent.rca.complete", audit.EventTypeRCAComplete),
			Entry("aiagent.response.failed", audit.EventTypeResponseFailed),
			Entry("aiagent.enrichment.completed", audit.EventTypeEnrichmentCompleted),
			Entry("aiagent.enrichment.failed", audit.EventTypeEnrichmentFailed),
			Entry("aiagent.alignment.step", audit.EventTypeAlignmentStep),
			Entry("aiagent.alignment.verdict", audit.EventTypeAlignmentVerdict),
		)

		It("should define exactly 23 event types", func() {
			Expect(audit.AllEventTypes).To(HaveLen(23))
		})

		It("should include aiagent.rca.complete in AllEventTypes", func() {
			Expect(audit.AllEventTypes).To(ContainElement(audit.EventTypeRCAComplete))
			Expect(audit.EventTypeRCAComplete).To(Equal("aiagent.rca.complete"))
		})
	})

	Describe("UT-KA-823-A01: Session event types registered in AllEventTypes", func() {
		It("should include all 4 session lifecycle event types", func() {
			sessionTypes := []string{
				audit.EventTypeSessionStarted,
				audit.EventTypeSessionCancelled,
				audit.EventTypeSessionCompleted,
				audit.EventTypeSessionFailed,
			}
			for _, et := range sessionTypes {
				Expect(audit.AllEventTypes).To(ContainElement(et),
					"AllEventTypes should contain %s", et)
			}
		})

		It("should have no duplicates in AllEventTypes", func() {
			seen := make(map[string]bool)
			for _, et := range audit.AllEventTypes {
				Expect(seen[et]).To(BeFalse(), "duplicate event type: %s", et)
				seen[et] = true
			}
		})
	})

	Describe("UT-KA-823-A02: Session action constants are non-empty", func() {
		DescribeTable("should have non-empty action for each session lifecycle transition",
			func(action string) {
				Expect(action).NotTo(BeEmpty())
			},
			Entry("started", audit.ActionSessionStarted),
			Entry("cancelled", audit.ActionSessionCancelled),
			Entry("completed", audit.ActionSessionCompleted),
			Entry("failed", audit.ActionSessionFailed),
		)
	})

	Describe("UT-KA-823-A03: NewEvent produces correct fields for session events", func() {
		DescribeTable("should create correct audit event for session event types",
			func(eventType string) {
				event := audit.NewEvent(eventType, "rr-audit-test")
				Expect(event).NotTo(BeNil())
				Expect(event.EventType).To(Equal(eventType))
				Expect(event.EventCategory).To(Equal(audit.EventCategory))
				Expect(event.CorrelationID).To(Equal("rr-audit-test"))
				Expect(event.Data).To(HaveKey("event_id"))
			},
			Entry("session.started", audit.EventTypeSessionStarted),
			Entry("session.cancelled", audit.EventTypeSessionCancelled),
			Entry("session.completed", audit.EventTypeSessionCompleted),
			Entry("session.failed", audit.EventTypeSessionFailed),
		)
	})

	Describe("UT-KA-823-A04: Investigation cancellation event registered and well-formed", func() {
		It("should include investigation.cancelled in AllEventTypes", func() {
			Expect(audit.AllEventTypes).To(ContainElement(audit.EventTypeInvestigationCancelled))
		})

		It("should have correct event type prefix", func() {
			Expect(audit.EventTypeInvestigationCancelled).To(HavePrefix("aiagent."))
		})

		It("should produce a well-formed event with NewEvent", func() {
			event := audit.NewEvent(audit.EventTypeInvestigationCancelled, "rr-cancel-test")
			Expect(event.EventType).To(Equal("aiagent.investigation.cancelled"))
			Expect(event.EventCategory).To(Equal(audit.EventCategory))
			Expect(event.CorrelationID).To(Equal("rr-cancel-test"))
		})

		It("should have a non-empty action constant", func() {
			Expect(audit.ActionInvestigationCancelled).NotTo(BeEmpty())
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

			Expect(func() {
				audit.StoreBestEffort(context.Background(), store, event, logr.Discard())
			}).NotTo(Panic())
		})

		It("should successfully store event when store is healthy", func() {
			store := &mockAuditStore{}
			event := &audit.AuditEvent{
				EventType:     audit.EventTypeLLMRequest,
				EventCategory: audit.EventCategory,
				CorrelationID: "corr-789",
			}

			audit.StoreBestEffort(context.Background(), store, event, logr.Discard())
			Expect(store.events).To(HaveLen(1))
			Expect(store.events[0].CorrelationID).To(Equal("corr-789"))
		})
	})

	Describe("UT-KA-OBS-016: InstrumentedAuditStore records event type on success (BR-KA-OBSERVABILITY-001.7)", func() {
		It("calls recorder with event type after successful store", func() {
			inner := &mockAuditStore{}
			var recorded []string
			store := audit.NewInstrumentedAuditStore(inner, func(eventType string) {
				recorded = append(recorded, eventType)
			})

			event := &audit.AuditEvent{
				EventType:     audit.EventTypeSessionStarted,
				EventCategory: audit.EventCategory,
				CorrelationID: "corr-obs-1",
			}
			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())
			Expect(inner.events).To(HaveLen(1))
			Expect(recorded).To(Equal([]string{audit.EventTypeSessionStarted}))
		})

		It("does not call recorder when inner store fails", func() {
			inner := &mockAuditStore{err: errors.New("ds unreachable")}
			var recorded []string
			store := audit.NewInstrumentedAuditStore(inner, func(eventType string) {
				recorded = append(recorded, eventType)
			})

			event := &audit.AuditEvent{
				EventType:     audit.EventTypeLLMRequest,
				EventCategory: audit.EventCategory,
				CorrelationID: "corr-obs-2",
			}
			err := store.StoreAudit(context.Background(), event)
			Expect(err).To(HaveOccurred())
			Expect(recorded).To(BeEmpty())
		})

		It("returns inner store directly when recorder is nil", func() {
			inner := &mockAuditStore{}
			store := audit.NewInstrumentedAuditStore(inner, nil)
			Expect(store).To(BeIdenticalTo(inner))
		})

		It("records multiple event types across multiple calls", func() {
			inner := &mockAuditStore{}
			var recorded []string
			store := audit.NewInstrumentedAuditStore(inner, func(eventType string) {
				recorded = append(recorded, eventType)
			})

			for _, et := range []string{
				audit.EventTypeSessionStarted,
				audit.EventTypeLLMRequest,
				audit.EventTypeLLMResponse,
			} {
				err := store.StoreAudit(context.Background(), &audit.AuditEvent{
					EventType:     et,
					EventCategory: audit.EventCategory,
				})
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(recorded).To(Equal([]string{
				audit.EventTypeSessionStarted,
				audit.EventTypeLLMRequest,
				audit.EventTypeLLMResponse,
			}))
		})
	})

	Describe("BR-AUDIT-005 #998: Audit actor attribution from context", func() {
		It("UT-KA-998-001: WithActor sets actor retrievable via ActorFromContext", func() {
			ctx := audit.WithActor(context.Background(), "user@example.com", "User")
			actorID, actorType, ok := audit.ActorFromContext(ctx)
			Expect(ok).To(BeTrue())
			Expect(actorID).To(Equal("user@example.com"))
			Expect(actorType).To(Equal("User"))
		})

		It("UT-KA-998-002: ActorFromContext returns false on bare context", func() {
			_, _, ok := audit.ActorFromContext(context.Background())
			Expect(ok).To(BeFalse())
		})

		It("UT-KA-998-003: StoreBestEffort auto-populates actor from context when event fields are empty", func() {
			store := &mockAuditStore{}
			ctx := audit.WithActor(context.Background(), "analyst@corp.io", "User")
			event := audit.NewEvent(audit.EventTypeLLMRequest, "corr-998")

			audit.StoreBestEffort(ctx, store, event, logr.Discard())
			Expect(store.events).To(HaveLen(1))
			Expect(store.events[0].ActorID).To(Equal("analyst@corp.io"))
			Expect(store.events[0].ActorType).To(Equal("User"))
		})

		It("UT-KA-998-004: StoreBestEffort preserves explicitly set actor fields", func() {
			store := &mockAuditStore{}
			ctx := audit.WithActor(context.Background(), "context-user", "User")
			event := audit.NewEvent(audit.EventTypeLLMRequest, "corr-998")
			event.ActorID = "system-override"
			event.ActorType = "Service"

			audit.StoreBestEffort(ctx, store, event, logr.Discard())
			Expect(store.events).To(HaveLen(1))
			Expect(store.events[0].ActorID).To(Equal("system-override"))
			Expect(store.events[0].ActorType).To(Equal("Service"))
		})

		It("UT-KA-998-005: StoreBestEffort leaves actor empty when no context actor and no event actor", func() {
			store := &mockAuditStore{}
			event := audit.NewEvent(audit.EventTypeLLMRequest, "corr-998")

			audit.StoreBestEffort(context.Background(), store, event, logr.Discard())
			Expect(store.events).To(HaveLen(1))
			Expect(store.events[0].ActorID).To(BeEmpty())
			Expect(store.events[0].ActorType).To(BeEmpty())
		})
	})
})
