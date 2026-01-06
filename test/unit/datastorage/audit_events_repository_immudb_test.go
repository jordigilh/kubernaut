/*
Copyright 2025 Jordi Gil.

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

package datastorage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

// ========================================
// IMMUDB AUDIT EVENTS REPOSITORY UNIT TESTS
// 📋 SOC2 Gap #9: Tamper-Evident Audit Trail - Phase 5.1
// Authority: AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md - Day 7
// Business Requirement: BR-AUDIT-005 (Tamper Detection)
// ========================================
//
// These tests validate the Immudb repository implementation WITHOUT external dependencies.
// Tests use mock Immudb client to verify business logic, not infrastructure.
//
// TESTING STRATEGY (Defense-in-Depth):
// - **Unit Tests** (THIS FILE): Business logic validation with mocks
// - **Integration Tests**: Real Immudb container (Phase 5.2)
// - **E2E Tests**: Full DataStorage service with Immudb (Phase 5.4)
//
// PHASE 5.1 SCOPE:
// - ✅ Create() method implementation
// - ✅ Event ID generation
// - ✅ Timestamp generation
// - ✅ Default value assignment
// - ✅ JSON serialization
// - ✅ Key format validation
// - ✅ Error handling
//
// ========================================

var _ = Describe("ImmudbAuditEventsRepository", func() {
	var (
		ctx        context.Context
		mockClient *testutil.MockImmudbClient
		repo       *repository.ImmudbAuditEventsRepository
		logger     = kubelog.NewLogger(kubelog.DefaultOptions())
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = testutil.NewMockImmudbClient()
		repo = repository.NewImmudbAuditEventsRepository(mockClient, logger)
	})

	Describe("Create() - Phase 5.1", func() {
		Context("when creating a valid audit event", func() {
			It("should insert event into Immudb with automatic hash chain", func() {
				// Arrange
				event := &repository.AuditEvent{
					EventType:     "workflow.execution.started",
					EventCategory: "workflow",
					EventAction:   "started",
					EventOutcome:  "success",
					CorrelationID: "rr-test-001",
					ResourceType:  "WorkflowExecution",
					ResourceID:    "wfe-12345",
					ActorID:       "system",
					ActorType:     "system",
					Severity:      "info",
					EventData: map[string]interface{}{
						"workflow_name": "test-workflow",
						"test_mode":     true,
					},
				}

				// Act
				createdEvent, err := repo.Create(ctx, event)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(createdEvent).ToNot(BeNil())

				// Verify event ID was generated
				Expect(createdEvent.EventID).ToNot(Equal(uuid.Nil))

				// Verify timestamp was set
				Expect(createdEvent.EventTimestamp).ToNot(BeZero())

				// Verify event_date was set from timestamp
				Expect(createdEvent.EventDate.Time()).ToNot(BeZero())

				// Verify default values were applied
				Expect(createdEvent.Version).To(Equal("1.0"))
				Expect(createdEvent.RetentionDays).To(Equal(2555)) // 7 years default

				// Verify Immudb VerifiedSet was called
				Expect(mockClient.VerifiedSetCalls).To(HaveLen(1))
				call := mockClient.VerifiedSetCalls[0]

				// Verify key format: audit_event:{event_id}
				expectedKey := fmt.Sprintf("audit_event:%s", createdEvent.EventID.String())
				Expect(string(call.Key)).To(Equal(expectedKey))

				// Verify value is JSON-serialized event
				var storedEvent repository.AuditEvent
				err = json.Unmarshal(call.Value, &storedEvent)
				Expect(err).ToNot(HaveOccurred())
				Expect(storedEvent.EventType).To(Equal(event.EventType))
				Expect(storedEvent.CorrelationID).To(Equal(event.CorrelationID))
				Expect(storedEvent.EventCategory).To(Equal(event.EventCategory))
			})

			It("should preserve provided event ID if already set", func() {
				// Arrange
				providedID := uuid.New()
				event := &repository.AuditEvent{
					EventID:       providedID,
					EventType:     "workflow.execution.started",
					EventCategory: "workflow",
					EventAction:   "started",
					EventOutcome:  "success",
					CorrelationID: "rr-test-002",
					ResourceType:  "WorkflowExecution",
					ResourceID:    "wfe-67890",
					ActorID:       "system",
					ActorType:     "system",
				}

				// Act
				createdEvent, err := repo.Create(ctx, event)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(createdEvent.EventID).To(Equal(providedID))
			})

			It("should preserve provided timestamp if already set", func() {
				// Arrange
				providedTimestamp := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
				event := &repository.AuditEvent{
					EventTimestamp: providedTimestamp,
					EventType:      "workflow.execution.started",
					EventCategory:  "workflow",
					EventAction:    "started",
					EventOutcome:   "success",
					CorrelationID:  "rr-test-003",
					ResourceType:   "WorkflowExecution",
					ResourceID:     "wfe-111",
					ActorID:        "system",
					ActorType:      "system",
				}

				// Act
				createdEvent, err := repo.Create(ctx, event)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(createdEvent.EventTimestamp).To(Equal(providedTimestamp))
			})

			It("should set event_date from event_timestamp for partitioning", func() {
				// Arrange
				timestamp := time.Date(2025, 1, 15, 14, 30, 45, 0, time.UTC)
				event := &repository.AuditEvent{
					EventTimestamp: timestamp,
					EventType:      "workflow.execution.started",
					EventCategory:  "workflow",
					EventAction:    "started",
					EventOutcome:   "success",
					CorrelationID:  "rr-test-004",
					ResourceType:   "WorkflowExecution",
					ResourceID:     "wfe-222",
					ActorID:        "system",
					ActorType:      "system",
				}

				// Act
				createdEvent, err := repo.Create(ctx, event)

				// Assert
				Expect(err).ToNot(HaveOccurred())

				// event_date should be date-only (2025-01-15)
				expectedDate := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
				Expect(createdEvent.EventDate.Time()).To(Equal(expectedDate))
			})

			It("should apply default version if not provided", func() {
				// Arrange
				event := &repository.AuditEvent{
					EventType:     "workflow.execution.started",
					EventCategory: "workflow",
					EventAction:   "started",
					EventOutcome:  "success",
					CorrelationID: "rr-test-005",
					ResourceType:  "WorkflowExecution",
					ResourceID:    "wfe-333",
					ActorID:       "system",
					ActorType:     "system",
					// Version not set - should default to "1.0"
				}

				// Act
				createdEvent, err := repo.Create(ctx, event)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(createdEvent.Version).To(Equal("1.0"))
			})

			It("should apply default retention days if not provided", func() {
				// Arrange
				event := &repository.AuditEvent{
					EventType:     "workflow.execution.started",
					EventCategory: "workflow",
					EventAction:   "started",
					EventOutcome:  "success",
					CorrelationID: "rr-test-006",
					ResourceType:  "WorkflowExecution",
					ResourceID:    "wfe-444",
					ActorID:       "system",
					ActorType:     "system",
					// RetentionDays not set - should default to 2555 (7 years)
				}

				// Act
				createdEvent, err := repo.Create(ctx, event)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(createdEvent.RetentionDays).To(Equal(2555)) // 7 years SOC2 compliance
			})

			It("should preserve custom version and retention days if provided", func() {
				// Arrange
				event := &repository.AuditEvent{
					EventType:     "workflow.execution.started",
					EventCategory: "workflow",
					EventAction:   "started",
					EventOutcome:  "success",
					CorrelationID: "rr-test-007",
					ResourceType:  "WorkflowExecution",
					ResourceID:    "wfe-555",
					ActorID:       "system",
					ActorType:     "system",
					Version:       "2.0", // Custom version
					RetentionDays: 3650,  // 10 years custom retention
				}

				// Act
				createdEvent, err := repo.Create(ctx, event)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(createdEvent.Version).To(Equal("2.0"))
				Expect(createdEvent.RetentionDays).To(Equal(3650))
			})
		})

		Context("when Immudb operation fails", func() {
			It("should return error if VerifiedSet fails", func() {
				// Arrange
				mockClient.VerifiedSetError = fmt.Errorf("Immudb connection timeout")
				event := &repository.AuditEvent{
					EventType:     "workflow.execution.started",
					EventCategory: "workflow",
					EventAction:   "started",
					EventOutcome:  "success",
					CorrelationID: "rr-test-008",
					ResourceType:  "WorkflowExecution",
					ResourceID:    "wfe-666",
					ActorID:       "system",
					ActorType:     "system",
				}

				// Act
				createdEvent, err := repo.Create(ctx, event)

				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to insert audit event into Immudb"))
				Expect(err.Error()).To(ContainSubstring("Immudb connection timeout"))
				Expect(createdEvent).To(BeNil())
			})
		})

		Context("when event data contains invalid JSON", func() {
			It("should handle complex event_data correctly", func() {
				// Arrange
				event := &repository.AuditEvent{
					EventType:     "workflow.execution.started",
					EventCategory: "workflow",
					EventAction:   "started",
					EventOutcome:  "success",
					CorrelationID: "rr-test-009",
					ResourceType:  "WorkflowExecution",
					ResourceID:    "wfe-777",
					ActorID:       "system",
					ActorType:     "system",
					EventData: map[string]interface{}{
						"workflow_name": "complex-workflow",
						"nested_data": map[string]interface{}{
							"level1": map[string]interface{}{
								"level2": "deep-value",
								"array":  []string{"a", "b", "c"},
							},
						},
						"number":  42,
						"boolean": true,
					},
				}

				// Act
				createdEvent, err := repo.Create(ctx, event)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(createdEvent).ToNot(BeNil())

				// Verify complex JSON was serialized correctly
				Expect(mockClient.VerifiedSetCalls).To(HaveLen(1))
				var storedEvent repository.AuditEvent
				err = json.Unmarshal(mockClient.VerifiedSetCalls[0].Value, &storedEvent)
				Expect(err).ToNot(HaveOccurred())
				Expect(storedEvent.EventData).To(HaveKey("nested_data"))
			})
		})
	})

	Describe("HealthCheck()", func() {
		Context("when Immudb is healthy", func() {
			It("should return no error", func() {
				// Act
				err := repo.HealthCheck(ctx)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(mockClient.CurrentStateCalls).To(Equal(1))
			})
		})

		Context("when Immudb is unhealthy", func() {
			It("should return error", func() {
				// Arrange
				mockClient.CurrentStateError = fmt.Errorf("Immudb connection lost")

				// Act
				err := repo.HealthCheck(ctx)

				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Immudb health check failed"))
				Expect(err.Error()).To(ContainSubstring("Immudb connection lost"))
			})
		})
	})
})
