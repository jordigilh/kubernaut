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

package dlq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// ========================================
// PHASE 2 (P0): DLQ CRITICAL DATA-LOSS FIX TESTS
// Issue #1048 | BR-AUDIT-001 | ADR-032
// ========================================
//
// FINDING-DF-1: Notification retry ACK-without-write
// FINDING-DF-2: Retry worker event payload parsing misalignment
// FINDING-DF-3: Drain XDel-on-DB-failure
//
// TDD RED PHASE: These tests define the contract for the fixes.
// ========================================

var _ = Describe("DLQ Critical Data-Loss Fixes (#1048 Phase 2)", func() {

	// ========================================
	// DF-1: Notification retry must persist, not silently drop
	// ========================================
	Describe("DF-1: Notification retry worker must persist notifications (UT-DS-1048-DF1)", func() {
		var (
			miniRedis   *miniredis.Miniredis
			redisClient *redis.Client
			dlqClient   *dlq.Client
			logger      logr.Logger
		)

		BeforeEach(func() {
			var err error
			miniRedis = miniredis.RunT(GinkgoT())
			redisClient = redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
			logger = logr.Discard()
			dlqClient, err = dlq.NewClient(redisClient, logger, 1000)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if redisClient != nil {
				_ = redisClient.Close()
			}
			if miniRedis != nil {
				miniRedis.Close()
			}
		})

		It("should persist notification audit records via the retry worker (UT-DS-1048-DF1-001)", func() {
			ctx := context.Background()

			// ARRANGE: Insert a notification DLQ message with a timestamp far enough
			// in the past that IsReadyForRetry returns true (backoff for retry 0 = 1m).
			notif := &models.NotificationAudit{
				RemediationID:   "remediation-df1-test",
				NotificationID:  uuid.New().String(),
				Recipient:       "oncall@example.com",
				Channel:         "slack",
				MessageSummary:  "DF-1 test notification",
				Status:          "sent",
				SentAt:          time.Now(),
				EscalationLevel: 0,
			}
			err := addDLQMessageWithPastTimestamp(ctx, redisClient, "notifications", notif, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred())

			// ARRANGE: Create mock notification repo that tracks Create calls
			mockNotifRepo := &MockNotificationRepository{
				createdAudits: []models.NotificationAudit{},
			}

			// ARRANGE: Create retry worker WITH notification repo
			workerConfig := server.DLQRetryWorkerConfig{
				PollInterval:  100 * time.Millisecond,
				MaxBatchSize:  10,
				MaxRetries:    6,
				ConsumerGroup: "test-df1-workers",
				ConsumerName:  "test-df1-worker-1",
			}
			worker := server.NewDLQRetryWorker(dlqClient, nil, mockNotifRepo, workerConfig, logger)

			// ACT: Start and let the worker process one cycle
			worker.Start(ctx)
			// ✅ APPROVED EXCEPTION: Waiting for goroutine-based retry worker poll cycle
			time.Sleep(300 * time.Millisecond)
			worker.Stop()

			// ASSERT: Notification was persisted (not silently dropped)
			Expect(mockNotifRepo.createdAudits).To(HaveLen(1),
				"DF-1: notification retry must persist via NotificationAuditRepository.Create, not silently drop")
			Expect(mockNotifRepo.createdAudits[0].RemediationID).To(Equal("remediation-df1-test"))
			Expect(mockNotifRepo.createdAudits[0].Channel).To(Equal("slack"))
		})

		It("should handle notification write failure with retry (UT-DS-1048-DF1-002)", func() {
			ctx := context.Background()

			// ARRANGE: Insert a notification DLQ message with past timestamp
			notif := &models.NotificationAudit{
				RemediationID:  "remediation-df1-fail",
				NotificationID: uuid.New().String(),
				Recipient:      "oncall@example.com",
				Channel:        "slack",
				MessageSummary: "DF-1 failure test",
				Status:         "sent",
				SentAt:         time.Now(),
			}
			err := addDLQMessageWithPastTimestamp(ctx, redisClient, "notifications", notif, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred())

			// ARRANGE: Mock repo that fails on Create
			failingNotifRepo := &MockNotificationRepository{
				createError: fmt.Errorf("simulated persistent DB error"),
			}

			workerConfig := server.DLQRetryWorkerConfig{
				PollInterval:  100 * time.Millisecond,
				MaxBatchSize:  10,
				MaxRetries:    6,
				ConsumerGroup: "test-df1-fail-workers",
				ConsumerName:  "test-df1-fail-1",
			}
			worker := server.NewDLQRetryWorker(dlqClient, nil, failingNotifRepo, workerConfig, logger)

			// ACT: Let worker attempt processing
			worker.Start(ctx)
			// ✅ APPROVED EXCEPTION: Waiting for goroutine-based retry worker poll cycle
			time.Sleep(300 * time.Millisecond)
			worker.Stop()

			// ASSERT: Message should NOT have been ACKed (it's still pending)
			// The message remains in the DLQ for future retry
			depth, err := dlqClient.GetDLQDepth(ctx, "notifications")
			Expect(err).ToNot(HaveOccurred())
			Expect(depth).To(BeNumerically(">=", 1),
				"DF-1: failed notification write must NOT ack the message")
		})
	})

	// ========================================
	// DF-2: Retry worker event parsing must align with drain path
	// ========================================
	Describe("DF-2: Retry worker event parsing must use proper unmarshal (UT-DS-1048-DF2)", func() {
		var (
			miniRedis   *miniredis.Miniredis
			redisClient *redis.Client
			dlqClient   *dlq.Client
			logger      logr.Logger
		)

		BeforeEach(func() {
			var err error
			miniRedis = miniredis.RunT(GinkgoT())
			redisClient = redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
			logger = logr.Discard()
			dlqClient, err = dlq.NewClient(redisClient, logger, 1000)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if redisClient != nil {
				_ = redisClient.Close()
			}
			if miniRedis != nil {
				miniRedis.Close()
			}
		})

		It("should preserve all audit event fields through retry (UT-DS-1048-DF2-001)", func() {
			ctx := context.Background()

			// ARRANGE: Create a rich audit event with all fields populated
			eventID := uuid.New()
			event := &audit.AuditEvent{
				EventID:        eventID,
				EventVersion:   "1.0",
				EventType:      "workflow.execution.started",
				EventCategory:  "workflow",
				EventAction:    "started",
				EventOutcome:   "success",
				EventTimestamp: time.Now().UTC().Truncate(time.Millisecond),
				CorrelationID:  "corr-df2-full-fidelity",
				ActorType:      "service",
				ActorID:        "workflow-service",
				ResourceType:   "workflow",
				ResourceID:     "wf-123",
				EventData:      []byte(`{"workflow_id":"wf-123","step":"start","metadata":{"version":"2.1"}}`),
				RetentionDays:  2555,
			}
			err := addDLQMessageWithPastTimestamp(ctx, redisClient, "events", event, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred())

			// ARRANGE: Mock repo that captures the full repository.AuditEvent
			mockEventsRepo := &MockRepositoryEventsRepository{
				createdEvents: []*repository.AuditEvent{},
			}

			workerConfig := server.DLQRetryWorkerConfig{
				PollInterval:  100 * time.Millisecond,
				MaxBatchSize:  10,
				MaxRetries:    6,
				ConsumerGroup: "test-df2-workers",
				ConsumerName:  "test-df2-worker-1",
			}
			worker := server.NewDLQRetryWorker(dlqClient, mockEventsRepo, nil, workerConfig, logger)

			// ACT: Process one cycle
			worker.Start(ctx)
			// ✅ APPROVED EXCEPTION: Waiting for goroutine-based retry worker poll cycle
			time.Sleep(300 * time.Millisecond)
			worker.Stop()

			// ASSERT: Event was persisted with ALL fields intact
			Expect(mockEventsRepo.createdEvents).To(HaveLen(1),
				"DF-2: retry worker must persist events using proper unmarshal")

			persisted := mockEventsRepo.createdEvents[0]
			Expect(persisted.EventID).To(Equal(eventID), "EventID must round-trip")
			Expect(persisted.Version).To(Equal("1.0"), "EventVersion -> Version must round-trip")
			Expect(persisted.EventType).To(Equal("workflow.execution.started"))
			Expect(persisted.EventCategory).To(Equal("workflow"))
			Expect(persisted.EventAction).To(Equal("started"))
			Expect(persisted.EventOutcome).To(Equal("success"))
			Expect(persisted.CorrelationID).To(Equal("corr-df2-full-fidelity"),
				"DF-2: CorrelationID must be preserved (was lost by old getString parser)")
			Expect(persisted.ActorType).To(Equal("service"))
			Expect(persisted.ActorID).To(Equal("workflow-service"))
			Expect(persisted.ResourceType).To(Equal("workflow"))
			Expect(persisted.ResourceID).To(Equal("wf-123"))
			Expect(persisted.EventData).To(HaveKey("workflow_id"),
				"DF-2: EventData must be preserved as map (was lost by old getString parser)")
			Expect(persisted.EventData["workflow_id"]).To(Equal("wf-123"))
		})

		It("should handle empty EventData gracefully (UT-DS-1048-DF2-002)", func() {
			ctx := context.Background()

			// ARRANGE: Event with no EventData, inserted with past timestamp
			event := &audit.AuditEvent{
				EventID:        uuid.New(),
				EventVersion:   "1.0",
				EventType:      "system.health.check",
				EventCategory:  "system",
				EventAction:    "checked",
				EventOutcome:   "success",
				EventTimestamp: time.Now().UTC(),
				CorrelationID:  "corr-df2-empty-data",
				ActorType:      "service",
				ActorID:        "health-service",
				ResourceType:   "system",
				ResourceID:     "sys-1",
				RetentionDays:  2555,
			}
			err := addDLQMessageWithPastTimestamp(ctx, redisClient, "events", event, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred())

			mockEventsRepo := &MockRepositoryEventsRepository{
				createdEvents: []*repository.AuditEvent{},
			}

			workerConfig := server.DLQRetryWorkerConfig{
				PollInterval:  100 * time.Millisecond,
				MaxBatchSize:  10,
				MaxRetries:    6,
				ConsumerGroup: "test-df2-empty-workers",
				ConsumerName:  "test-df2-empty-1",
			}
			worker := server.NewDLQRetryWorker(dlqClient, mockEventsRepo, nil, workerConfig, logger)

			worker.Start(ctx)
			// ✅ APPROVED EXCEPTION: Waiting for goroutine-based retry worker poll cycle
			time.Sleep(300 * time.Millisecond)
			worker.Stop()

			// ASSERT: Event persisted with default empty EventData
			Expect(mockEventsRepo.createdEvents).To(HaveLen(1),
				"DF-2: empty EventData must not cause parsing failure")
		})
	})

	// ========================================
	// DF-3: Drain must NOT delete messages on DB write failure
	// ========================================
	Describe("DF-3: Drain must preserve messages on DB write failure (UT-DS-1048-DF3)", func() {
		var (
			miniRedis   *miniredis.Miniredis
			redisClient *redis.Client
			dlqClient   *dlq.Client
			logger      logr.Logger
		)

		BeforeEach(func() {
			var err error
			miniRedis = miniredis.RunT(GinkgoT())
			redisClient = redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
			logger = kubelog.NewLogger(kubelog.DefaultOptions())
			dlqClient, err = dlq.NewClient(redisClient, logger, 1000)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if redisClient != nil {
				_ = redisClient.Close()
			}
			if miniRedis != nil {
				miniRedis.Close()
			}
		})

		It("should NOT delete messages from Redis when DB write fails (UT-DS-1048-DF3-001)", func() {
			ctx := context.Background()

			// ARRANGE: Enqueue 3 events to DLQ
			for i := 0; i < 3; i++ {
				event := &audit.AuditEvent{
					EventID:        uuid.New(),
					EventVersion:   "1.0",
					EventType:      fmt.Sprintf("test.event.%d", i),
					EventCategory:  "test",
					EventAction:    "tested",
					EventOutcome:   "success",
					EventTimestamp: time.Now().UTC(),
					CorrelationID:  fmt.Sprintf("corr-df3-%d", i),
					ActorType:      "service",
					ActorID:        "test-service",
					ResourceType:   "test",
					ResourceID:     fmt.Sprintf("test-%d", i),
					EventData:      []byte(`{"key":"value"}`),
					RetentionDays:  2555,
				}
				err := dlqClient.EnqueueAuditEvent(ctx, event, fmt.Errorf("DB error"))
				Expect(err).ToNot(HaveOccurred())
			}

			// Verify 3 messages in DLQ
			depth, err := dlqClient.GetDLQDepth(ctx, "events")
			Expect(err).ToNot(HaveOccurred())
			Expect(depth).To(Equal(int64(3)))

			// ARRANGE: Mock repo that ALWAYS fails
			failingRepo := &MockRepositoryEventsRepository{
				createError: fmt.Errorf("persistent database failure"),
			}
			mockNotifRepo := &MockNotificationRepository{}

			// ACT: Drain with failing repo
			drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			stats, err := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, failingRepo)

			// ASSERT: Drain completes without panic
			Expect(err).ToNot(HaveOccurred())
			Expect(stats.TotalProcessed).To(Equal(0),
				"no messages should be marked as processed when all writes fail")

			// ASSERT: Messages are STILL in Redis (not deleted)
			finalDepth, err := dlqClient.GetDLQDepth(ctx, "events")
			Expect(err).ToNot(HaveOccurred())
			Expect(finalDepth).To(Equal(int64(3)),
				"DF-3: messages must NOT be deleted from Redis when DB write fails")
		})

		It("should delete messages only on successful DB write (UT-DS-1048-DF3-002)", func() {
			ctx := context.Background()

			// ARRANGE: Enqueue 2 events
			for i := 0; i < 2; i++ {
				event := &audit.AuditEvent{
					EventID:        uuid.New(),
					EventVersion:   "1.0",
					EventType:      "test.event.success",
					EventCategory:  "test",
					EventAction:    "tested",
					EventOutcome:   "success",
					EventTimestamp: time.Now().UTC(),
					CorrelationID:  fmt.Sprintf("corr-df3-success-%d", i),
					ActorType:      "service",
					ActorID:        "test-service",
					ResourceType:   "test",
					ResourceID:     fmt.Sprintf("test-%d", i),
					EventData:      []byte(`{"ok":true}`),
					RetentionDays:  2555,
				}
				err := dlqClient.EnqueueAuditEvent(ctx, event, fmt.Errorf("DB error"))
				Expect(err).ToNot(HaveOccurred())
			}

			// ARRANGE: Mock repo that succeeds
			successRepo := &MockRepositoryEventsRepository{
				createdEvents: []*repository.AuditEvent{},
			}
			mockNotifRepo := &MockNotificationRepository{}

			// ACT: Drain with successful repo
			drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			stats, err := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, successRepo)

			// ASSERT: All messages processed and removed
			Expect(err).ToNot(HaveOccurred())
			Expect(stats.EventsProcessed).To(Equal(2))

			finalDepth, err := dlqClient.GetDLQDepth(ctx, "events")
			Expect(err).ToNot(HaveOccurred())
			Expect(finalDepth).To(Equal(int64(0)),
				"DF-3: messages must be deleted after successful DB write")

			// ASSERT: Events persisted
			Expect(successRepo.createdEvents).To(HaveLen(2))
		})

		It("should not enter infinite loop when DB write fails (UT-DS-1048-DF3-003)", func() {
			ctx := context.Background()

			// ARRANGE: Single message that will fail
			event := &audit.AuditEvent{
				EventID:        uuid.New(),
				EventVersion:   "1.0",
				EventType:      "test.event.loop",
				EventCategory:  "test",
				EventAction:    "tested",
				EventOutcome:   "failure",
				EventTimestamp: time.Now().UTC(),
				CorrelationID:  "corr-df3-loop",
				ActorType:      "service",
				ActorID:        "test-service",
				ResourceType:   "test",
				ResourceID:     "test-loop",
				EventData:      []byte(`{"loop":"test"}`),
				RetentionDays:  2555,
			}
			err := dlqClient.EnqueueAuditEvent(ctx, event, fmt.Errorf("DB error"))
			Expect(err).ToNot(HaveOccurred())

			failingRepo := &MockRepositoryEventsRepository{
				createError: fmt.Errorf("permanent failure"),
			}
			mockNotifRepo := &MockNotificationRepository{}

			// ACT: Drain must complete within 2 seconds (no infinite loop)
			drainCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			start := time.Now()
			stats, err := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, failingRepo)
			elapsed := time.Since(start)

			// ASSERT: Completes quickly (cursor advances past failed message)
			Expect(err).ToNot(HaveOccurred())
			Expect(stats.TotalProcessed).To(Equal(0))
			Expect(elapsed).To(BeNumerically("<", 1*time.Second),
				"DF-3: drain must not spin in infinite loop on DB failure; cursor must advance")

			// ASSERT: Message preserved in Redis
			depth, err := dlqClient.GetDLQDepth(ctx, "events")
			Expect(err).ToNot(HaveOccurred())
			Expect(depth).To(Equal(int64(1)), "failed message must remain in stream")
		})

		It("should process remaining messages after one fails (UT-DS-1048-DF3-004)", func() {
			ctx := context.Background()

			// ARRANGE: Enqueue 3 events
			eventIDs := make([]uuid.UUID, 3)
			for i := 0; i < 3; i++ {
				eventIDs[i] = uuid.New()
				event := &audit.AuditEvent{
					EventID:        eventIDs[i],
					EventVersion:   "1.0",
					EventType:      fmt.Sprintf("test.event.mixed.%d", i),
					EventCategory:  "test",
					EventAction:    "tested",
					EventOutcome:   "success",
					EventTimestamp: time.Now().UTC(),
					CorrelationID:  fmt.Sprintf("corr-df3-mixed-%d", i),
					ActorType:      "service",
					ActorID:        "test-service",
					ResourceType:   "test",
					ResourceID:     fmt.Sprintf("test-%d", i),
					EventData:      []byte(fmt.Sprintf(`{"index":%d}`, i)),
					RetentionDays:  2555,
				}
				err := dlqClient.EnqueueAuditEvent(ctx, event, fmt.Errorf("DB error"))
				Expect(err).ToNot(HaveOccurred())
			}

			// ARRANGE: Mock repo that fails on the second event only
			selectiveRepo := &SelectiveFailEventsRepository{
				createdEvents: []*repository.AuditEvent{},
				failOnIndex:   1,
				callCount:     0,
			}
			mockNotifRepo := &MockNotificationRepository{}

			// ACT: Drain
			drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			stats, err := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, selectiveRepo)

			// ASSERT: 2 of 3 succeeded (events 0 and 2 written)
			Expect(err).ToNot(HaveOccurred())
			Expect(stats.EventsProcessed).To(Equal(2))
			Expect(selectiveRepo.createdEvents).To(HaveLen(2),
				"DF-3: successful events must still be written even when one fails")

			// ASSERT: Only the failed message remains in Redis
			finalDepth, err := dlqClient.GetDLQDepth(ctx, "events")
			Expect(err).ToNot(HaveOccurred())
			Expect(finalDepth).To(Equal(int64(1)),
				"DF-3: only the failed message should remain in the stream")
		})
	})
})

// ========================================
// MOCK: Selective failure repository
// ========================================

// SelectiveFailEventsRepository fails on a specific call index.
type SelectiveFailEventsRepository struct {
	createdEvents []*repository.AuditEvent
	failOnIndex   int
	callCount     int
}

func (m *SelectiveFailEventsRepository) Create(ctx context.Context, event *repository.AuditEvent) (*repository.AuditEvent, error) {
	idx := m.callCount
	m.callCount++
	if idx == m.failOnIndex {
		return nil, fmt.Errorf("selective failure on call %d", idx)
	}
	m.createdEvents = append(m.createdEvents, event)
	return event, nil
}

// ========================================
// Mock for notification repo (used by DF-1 tests)
// The existing MockNotificationRepository from drain_test.go is in the same
// package so we reuse it. If it weren't, we'd define it here.
// getString and parseAuditEventPayload have been replaced, so no mocks needed.
// ========================================

// addDLQMessageWithPastTimestamp inserts a DLQ message directly into Redis
// with a timestamp far enough in the past that IsReadyForRetry returns true.
// This bypasses the Enqueue* methods which always set Timestamp to time.Now().
func addDLQMessageWithPastTimestamp(ctx context.Context, redisClient *redis.Client, auditType string, payload interface{}, age time.Duration) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	auditMsg := dlq.AuditMessage{
		Type:       auditType,
		Payload:    payloadJSON,
		Timestamp:  time.Now().Add(-age),
		RetryCount: 0,
		LastError:  "simulated error",
	}

	messageJSON, err := json.Marshal(auditMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal audit message: %w", err)
	}

	streamKey := fmt.Sprintf("audit:dlq:%s", auditType)
	return redisClient.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: 10000,
		Approx: true,
		ID:     "*",
		Values: map[string]interface{}{
			"message": string(messageJSON),
		},
	}).Err()
}
