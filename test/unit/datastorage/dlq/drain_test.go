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

package dlq

import (
	"context"
	"fmt"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// DD-008: DLQ DRAIN DURING GRACEFUL SHUTDOWN TESTS
// ========================================
//
// Business Requirement: BR-AUDIT-001 - Complete audit trail (no data loss during shutdown)
// Design Decision: DD-008 - DLQ Drain During Graceful Shutdown
//
// Test Strategy:
// - Unit Tests (70%+): DLQ drain logic with mock Redis
// - Testing Goal: Verify DLQ messages are persisted before shutdown
//
// ========================================

var _ = Describe("DLQ Drain During Graceful Shutdown (DD-008)", func() {
	var (
		ctx            context.Context
		cancel         context.CancelFunc
		miniRedis      *miniredis.Miniredis
		redisClient    *redis.Client
		dlqClient      *dlq.Client
		logger         logr.Logger
		mockNotifRepo  *MockNotificationRepository
		mockEventsRepo *MockEventsRepository
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// Start mini Redis
		var err error
		miniRedis = miniredis.RunT(GinkgoT())
		Expect(miniRedis).ToNot(BeNil())

		// Create Redis client
		redisClient = redis.NewClient(&redis.Options{
			Addr: miniRedis.Addr(),
		})

		// Create logger
		logger = kubelog.NewLogger(kubelog.DefaultOptions())

		// Create DLQ client
		dlqClient, err = dlq.NewClient(redisClient, logger, 1000)
		Expect(err).ToNot(HaveOccurred())

		// Create mock repositories
		mockNotifRepo = &MockNotificationRepository{
			createdAudits: []models.NotificationAudit{},
		}
		mockEventsRepo = &MockEventsRepository{
			createdEvents: []audit.AuditEvent{},
		}
	})

	AfterEach(func() {
		cancel()
		if redisClient != nil {
			_ = redisClient.Close()
		}
		if miniRedis != nil {
			miniRedis.Close()
		}
	})

	Context("DrainWithTimeout method", func() {
		It("should drain notification DLQ messages successfully", func() {
			// BR-AUDIT-001: Ensure notification audit messages are not lost during shutdown

			// ARRANGE: Add notification messages to DLQ
			notif1 := &models.NotificationAudit{
				RemediationID:   "remediation-1",
				NotificationID:  uuid.New().String(),
				Recipient:       "test@example.com",
				Channel:         "slack",
				MessageSummary:  "Test notification 1",
				Status:          "sent",
				SentAt:          time.Now(),
				EscalationLevel: 0,
			}
			notif2 := &models.NotificationAudit{
				RemediationID:   "remediation-2",
				NotificationID:  uuid.New().String(),
				Recipient:       "test@example.com",
				Channel:         "slack",
				MessageSummary:  "Test notification 2",
				Status:          "sent",
				SentAt:          time.Now(),
				EscalationLevel: 0,
			}

			err := dlqClient.EnqueueNotificationAudit(ctx, notif1, fmt.Errorf("test error"))
			Expect(err).ToNot(HaveOccurred())
			err = dlqClient.EnqueueNotificationAudit(ctx, notif2, fmt.Errorf("test error"))
			Expect(err).ToNot(HaveOccurred())

			// Verify messages are in DLQ
			depth, err := dlqClient.GetDLQDepth(ctx, "notifications")
			Expect(err).ToNot(HaveOccurred())
			Expect(depth).To(Equal(int64(2)))

			// ACT: Drain DLQ with 5 second timeout
			drainCtx, drainCancel := context.WithTimeout(ctx, 5*time.Second)
			defer drainCancel()

			stats, err := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, mockEventsRepo)

			// ASSERT: Drain completes successfully
			Expect(err).ToNot(HaveOccurred())
			Expect(stats).ToNot(BeNil())
			Expect(stats.NotificationsProcessed).To(Equal(2), "should process 2 notification messages")
			Expect(stats.EventsProcessed).To(Equal(0), "should process 0 event messages")
			Expect(stats.TotalProcessed).To(Equal(2))
			Expect(stats.TimedOut).To(BeFalse(), "should complete before timeout")
			Expect(stats.Duration).To(BeNumerically("<", 5*time.Second))

			// ASSERT: Messages were written to mock repository
			Expect(mockNotifRepo.createdAudits).To(HaveLen(2))

			// ASSERT: DLQ is empty after drain
			finalDepth, err := dlqClient.GetDLQDepth(ctx, "notifications")
			Expect(err).ToNot(HaveOccurred())
			Expect(finalDepth).To(Equal(int64(0)), "DLQ should be empty after drain")
		})

		It("should drain event DLQ messages successfully", func() {
			// BR-AUDIT-001: Ensure audit event messages are not lost during shutdown

			// ARRANGE: Add event messages to DLQ
			event1 := &audit.AuditEvent{
				EventID:        uuid.New(),
				EventType:      "test.event.occurred",
				EventCategory:  "test",
				EventTimestamp: time.Now(),
				CorrelationID:  "test-correlation-1",
			}
			event2 := &audit.AuditEvent{
				EventID:        uuid.New(),
				EventType:      "test.event.completed",
				EventCategory:  "test",
				EventTimestamp: time.Now(),
				CorrelationID:  "test-correlation-2",
			}

			err := dlqClient.EnqueueAuditEvent(ctx, event1, fmt.Errorf("test error"))
			Expect(err).ToNot(HaveOccurred())
			err = dlqClient.EnqueueAuditEvent(ctx, event2, fmt.Errorf("test error"))
			Expect(err).ToNot(HaveOccurred())

			// Verify messages are in DLQ
			depth, err := dlqClient.GetDLQDepth(ctx, "events")
			Expect(err).ToNot(HaveOccurred())
			Expect(depth).To(Equal(int64(2)))

			// ACT: Drain DLQ with 5 second timeout
			drainCtx, drainCancel := context.WithTimeout(ctx, 5*time.Second)
			defer drainCancel()

			stats, err := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, mockEventsRepo)

			// ASSERT: Drain completes successfully
			Expect(err).ToNot(HaveOccurred())
			Expect(stats).ToNot(BeNil())
			Expect(stats.NotificationsProcessed).To(Equal(0), "should process 0 notification messages")
			Expect(stats.EventsProcessed).To(Equal(2), "should process 2 event messages")
			Expect(stats.TotalProcessed).To(Equal(2))
			Expect(stats.TimedOut).To(BeFalse(), "should complete before timeout")

			// ASSERT: Messages were written to mock repository
			Expect(mockEventsRepo.createdEvents).To(HaveLen(2))

			// ASSERT: DLQ is empty after drain
			finalDepth, err := dlqClient.GetDLQDepth(ctx, "events")
			Expect(err).ToNot(HaveOccurred())
			Expect(finalDepth).To(Equal(int64(0)), "DLQ should be empty after drain")
		})

		It("should drain both notification and event DLQ messages", func() {
			// BR-AUDIT-001: Ensure all audit messages are preserved

			// ARRANGE: Add messages to both DLQ streams
			notif := &models.NotificationAudit{
				RemediationID:   "remediation-test",
				NotificationID:  uuid.New().String(),
				Recipient:       "test@example.com",
				Channel:         "slack",
				MessageSummary:  "Test notification",
				Status:          "sent",
				SentAt:          time.Now(),
				EscalationLevel: 0,
			}
			event := &audit.AuditEvent{
				EventID:        uuid.New(),
				EventType:      "test.event",
				EventCategory:  "test",
				EventTimestamp: time.Now(),
				CorrelationID:  "test-event",
			}

			err := dlqClient.EnqueueNotificationAudit(ctx, notif, fmt.Errorf("test error"))
			Expect(err).ToNot(HaveOccurred())
			err = dlqClient.EnqueueAuditEvent(ctx, event, fmt.Errorf("test error"))
			Expect(err).ToNot(HaveOccurred())

			// ACT: Drain DLQ
			drainCtx, drainCancel := context.WithTimeout(ctx, 5*time.Second)
			defer drainCancel()

			stats, err := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, mockEventsRepo)

			// ASSERT: Both types processed
			Expect(err).ToNot(HaveOccurred())
			Expect(stats.NotificationsProcessed).To(Equal(1))
			Expect(stats.EventsProcessed).To(Equal(1))
			Expect(stats.TotalProcessed).To(Equal(2))
			Expect(stats.TimedOut).To(BeFalse())
		})

		It("should handle timeout during drain gracefully", func() {
			// DD-008: Graceful handling of timeout during DLQ drain

			// ARRANGE: Add many messages to DLQ to ensure timeout
			for i := 0; i < 100; i++ {
				notif := &models.NotificationAudit{
					RemediationID:   fmt.Sprintf("remediation-%d", i),
					NotificationID:  uuid.New().String(),
					Recipient:       "test@example.com",
					Channel:         "slack",
					MessageSummary:  fmt.Sprintf("Test notification %d", i),
					Status:          "sent",
					SentAt:          time.Now(),
					EscalationLevel: 0,
				}
				err := dlqClient.EnqueueNotificationAudit(ctx, notif, fmt.Errorf("test error"))
				Expect(err).ToNot(HaveOccurred())
			}

			// ACT: Drain with very short timeout (1ms should cause timeout)
			drainCtx, drainCancel := context.WithTimeout(ctx, 1*time.Millisecond)
			defer drainCancel()

			// Add small delay to ensure context expires
			time.Sleep(2 * time.Millisecond)

			stats, err := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, mockEventsRepo)

			// ASSERT: Timeout handled gracefully (no error, but marked as timed out)
			Expect(err).ToNot(HaveOccurred(), "timeout should not return error")
			Expect(stats).ToNot(BeNil())
			Expect(stats.TimedOut).To(BeTrue(), "should indicate timeout occurred")
			// Some messages may have been processed before timeout
			Expect(stats.TotalProcessed).To(BeNumerically(">=", 0))
		})

		It("should handle empty DLQ gracefully", func() {
			// DD-008: Handle empty DLQ without errors

			// ARRANGE: DLQ is already empty (no messages added)

			// ACT: Drain empty DLQ
			drainCtx, drainCancel := context.WithTimeout(ctx, 5*time.Second)
			defer drainCancel()

			stats, err := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, mockEventsRepo)

			// ASSERT: Completes successfully with zero messages processed
			Expect(err).ToNot(HaveOccurred())
			Expect(stats).ToNot(BeNil())
			Expect(stats.TotalProcessed).To(Equal(0))
			Expect(stats.TimedOut).To(BeFalse())
		})

		// TDD RED PHASE: This test validates the CORRECT interface usage
		// Test will FAIL until we fix the DLQ client code to use repository.AuditEvent
		It("should write event messages to database using repository.AuditEvent type", func() {
			// BR-AUDIT-001: Audit events from DLQ must persist to database during shutdown
			// Bug Fix: DLQ client must use repository.AuditEvent type, not audit.AuditEvent

			// ARRANGE: Create mock repository that implements CORRECT interface
			mockRepoEvents := &MockRepositoryEventsRepository{
				createdEvents: []*repository.AuditEvent{},
			}

			// Add event messages to DLQ (stored as audit.AuditEvent)
			event1 := &audit.AuditEvent{
				EventID:        uuid.New(),
				EventVersion:   "1.0",
				EventType:      "workflow.execution.started",
				EventCategory:  "workflow",
				EventAction:    "started",
				EventOutcome:   "success",
				EventTimestamp: time.Now(),
				CorrelationID:  "test-drain-repo-1",
				ActorType:      "service",
				ActorID:        "workflow-service",
				ResourceType:   "workflow",
				ResourceID:     "workflow-123",
				EventData:      []byte(`{"workflow_id": "workflow-123", "step": "start"}`),
			}

			err := dlqClient.EnqueueAuditEvent(ctx, event1, fmt.Errorf("simulated DB error"))
			Expect(err).ToNot(HaveOccurred())

			// Verify message is in DLQ
			depth, err := dlqClient.GetDLQDepth(ctx, "events")
			Expect(err).ToNot(HaveOccurred())
			Expect(depth).To(Equal(int64(1)))

			// ACT: Drain DLQ with repository that expects repository.AuditEvent
			drainCtx, drainCancel := context.WithTimeout(ctx, 5*time.Second)
			defer drainCancel()

			stats, err := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, mockRepoEvents)

			// ASSERT: Drain completes successfully
			Expect(err).ToNot(HaveOccurred())
			Expect(stats).ToNot(BeNil())
			Expect(stats.EventsProcessed).To(Equal(1), "should process 1 event message")
			Expect(stats.TotalProcessed).To(Equal(1))
			Expect(stats.TimedOut).To(BeFalse())

			// ASSERT: Event was written to database using repository.AuditEvent type
			Expect(mockRepoEvents.createdEvents).To(HaveLen(1), "event should be persisted to database")
			persistedEvent := mockRepoEvents.createdEvents[0]
			Expect(persistedEvent.EventType).To(Equal("workflow.execution.started"))
			Expect(persistedEvent.EventCategory).To(Equal("workflow"))
			Expect(persistedEvent.CorrelationID).To(Equal("test-drain-repo-1"))
			Expect(persistedEvent.Version).To(Equal("1.0"), "EventVersion should map to Version")

			// ASSERT: EventData was converted from []byte to map[string]interface{}
			Expect(persistedEvent.EventData).To(HaveKey("workflow_id"))
			Expect(persistedEvent.EventData["workflow_id"]).To(Equal("workflow-123"))

			// ASSERT: DLQ is empty after drain
			finalDepth, err := dlqClient.GetDLQDepth(ctx, "events")
			Expect(err).ToNot(HaveOccurred())
			Expect(finalDepth).To(Equal(int64(0)), "DLQ should be empty after successful drain")
		})
	})
})

// ========================================
// MOCK REPOSITORIES FOR TESTING
// ========================================

// MockNotificationRepository mocks notification repository for testing
type MockNotificationRepository struct {
	createdAudits []models.NotificationAudit
	createError   error
}

func (m *MockNotificationRepository) Create(ctx context.Context, audit *models.NotificationAudit) (*models.NotificationAudit, error) {
	if m.createError != nil {
		return nil, m.createError
	}
	m.createdAudits = append(m.createdAudits, *audit)
	return audit, nil
}

// MockEventsRepository mocks events repository for testing
// This now implements the CORRECT interface: repository.AuditEventsRepository.Create()
type MockEventsRepository struct {
	createdEvents []audit.AuditEvent // Keep tracking as audit.AuditEvent for test assertions
	createError   error
}

func (m *MockEventsRepository) Create(ctx context.Context, event *repository.AuditEvent) (*repository.AuditEvent, error) {
	if m.createError != nil {
		return nil, m.createError
	}
	// Convert back to audit.AuditEvent for test assertions
	auditEvent := audit.AuditEvent{
		EventID:        event.EventID,
		EventVersion:   event.Version,
		EventType:      event.EventType,
		EventCategory:  event.EventCategory,
		EventAction:    event.EventAction,
		EventOutcome:   event.EventOutcome,
		EventTimestamp: event.EventTimestamp,
		CorrelationID:  event.CorrelationID,
		ActorType:      event.ActorType,
		ActorID:        event.ActorID,
		ResourceType:   event.ResourceType,
		ResourceID:     event.ResourceID,
	}
	m.createdEvents = append(m.createdEvents, auditEvent)
	return event, nil
}

// MockRepositoryEventsRepository mocks the CORRECT repository interface
// This implements repository.AuditEventsRepository.Create() method signature
type MockRepositoryEventsRepository struct {
	createdEvents []*repository.AuditEvent
	createError   error
}

func (m *MockRepositoryEventsRepository) Create(ctx context.Context, event *repository.AuditEvent) (*repository.AuditEvent, error) {
	if m.createError != nil {
		return nil, m.createError
	}
	m.createdEvents = append(m.createdEvents, event)
	return event, nil
}
