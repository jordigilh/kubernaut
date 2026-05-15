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

package dlq_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
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
				ReadTimeout:   100 * time.Millisecond,
				ConsumerGroup: "test-df1-workers",
				ConsumerName:  "test-df1-worker-1",
			}
			worker := server.NewDLQRetryWorker(dlqClient, nil, mockNotifRepo, workerConfig, logger, nil)

			worker.Start(ctx)
			Eventually(func() int {
				return mockNotifRepo.CreatedCount()
			}).WithTimeout(2 * time.Second).WithPolling(50 * time.Millisecond).Should(Equal(1),
				"DF-1: notification retry must persist via NotificationAuditRepository.Create, not silently drop")
			worker.Stop()
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
				ReadTimeout:   100 * time.Millisecond,
				ConsumerGroup: "test-df1-fail-workers",
				ConsumerName:  "test-df1-fail-1",
			}
			worker := server.NewDLQRetryWorker(dlqClient, nil, failingNotifRepo, workerConfig, logger, nil)

			worker.Start(ctx)
			Eventually(func() int64 {
				depth, _ := dlqClient.GetDLQDepth(ctx, "notifications")
				return depth
			}).WithTimeout(2 * time.Second).WithPolling(50 * time.Millisecond).Should(BeNumerically(">=", 1),
				"DF-1: failed notification write must NOT ack the message")
			worker.Stop()
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
				ReadTimeout:   100 * time.Millisecond,
				ConsumerGroup: "test-df2-workers",
				ConsumerName:  "test-df2-worker-1",
			}
			worker := server.NewDLQRetryWorker(dlqClient, mockEventsRepo, nil, workerConfig, logger, nil)

			worker.Start(ctx)
			Eventually(func() int {
				return mockEventsRepo.CreatedCount()
			}).WithTimeout(2 * time.Second).WithPolling(50 * time.Millisecond).Should(Equal(1),
				"DF-2: retry worker must persist events using proper unmarshal")
			worker.Stop()

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
				ReadTimeout:   100 * time.Millisecond,
				ConsumerGroup: "test-df2-empty-workers",
				ConsumerName:  "test-df2-empty-1",
			}
			worker := server.NewDLQRetryWorker(dlqClient, mockEventsRepo, nil, workerConfig, logger, nil)

			worker.Start(ctx)
			Eventually(func() int {
				return mockEventsRepo.CreatedCount()
			}).WithTimeout(2 * time.Second).WithPolling(50 * time.Millisecond).Should(Equal(1),
				"DF-2: empty EventData must not cause parsing failure")
			worker.Stop()
		})
	})

	// ========================================
	// ARCH-F1: Cursor policy must not permanently skip failed messages
	// ========================================
	Describe("ARCH-F1: Drain retry pass must recover skipped messages (UT-DS-1048-ARCH-F1)", func() {
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

		It("should recover transiently-failed messages via retry pass (UT-DS-1048-ARCH-F1-001)", func() {
			ctx := context.Background()

			// ARRANGE: Enqueue 2 events
			for i := 0; i < 2; i++ {
				event := &audit.AuditEvent{
					EventID:        uuid.New(),
					EventVersion:   "1.0",
					EventType:      fmt.Sprintf("test.event.transient.%d", i),
					EventCategory:  "test",
					EventAction:    "tested",
					EventOutcome:   "success",
					EventTimestamp: time.Now().UTC(),
					CorrelationID:  fmt.Sprintf("corr-arch-f1-%d", i),
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

			// ARRANGE: Repo that fails on the FIRST call only (transient error).
			// Pass 1: event 0 fails (call 0), event 1 succeeds (call 1).
			// Pass 2: event 0 retried (call 2) and succeeds.
			transientRepo := &TransientFailEventsRepository{
				createdEvents:   []*repository.AuditEvent{},
				failFirstNCalls: 1,
			}
			mockNotifRepo := &MockNotificationRepository{}

			// ACT: Drain
			drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			stats, err := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, transientRepo)

			// ASSERT: All 2 messages processed (1 in pass 1, 1 recovered in pass 2)
			Expect(err).ToNot(HaveOccurred())
			Expect(stats.EventsProcessed).To(Equal(2),
				"ARCH-F1: both messages must be processed; retry pass recovers transient failures")
			Expect(transientRepo.createdEvents).To(HaveLen(2))

			// ASSERT: DLQ is empty
			finalDepth, err := dlqClient.GetDLQDepth(ctx, "events")
			Expect(err).ToNot(HaveOccurred())
			Expect(finalDepth).To(Equal(int64(0)),
				"ARCH-F1: retry pass should have recovered the transiently-failed message")
		})

		It("should leave permanently-failing messages after retry pass (UT-DS-1048-ARCH-F1-002)", func() {
			ctx := context.Background()

			// ARRANGE: Enqueue 2 events
			for i := 0; i < 2; i++ {
				event := &audit.AuditEvent{
					EventID:        uuid.New(),
					EventVersion:   "1.0",
					EventType:      fmt.Sprintf("test.event.perm.%d", i),
					EventCategory:  "test",
					EventAction:    "tested",
					EventOutcome:   "success",
					EventTimestamp: time.Now().UTC(),
					CorrelationID:  fmt.Sprintf("corr-arch-f1-perm-%d", i),
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

			// ARRANGE: Repo that ALWAYS fails
			failingRepo := &MockRepositoryEventsRepository{
				createError: fmt.Errorf("permanent failure"),
			}
			mockNotifRepo := &MockNotificationRepository{}

			// ACT: Drain (two passes, both fail)
			drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			stats, drainErr := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, failingRepo)

			// ASSERT: DF-H1: Drain returns error when messages fail both passes
			Expect(drainErr).To(HaveOccurred(),
				"DF-H1: DrainWithTimeout must return error when messages remain after 2 passes")
			Expect(stats.EventsProcessed).To(Equal(0))

			// ASSERT: Both messages still in Redis for next startup
			finalDepth, err := dlqClient.GetDLQDepth(ctx, "events")
			Expect(err).ToNot(HaveOccurred())
			Expect(finalDepth).To(Equal(int64(2)),
				"ARCH-F1: permanently failing messages must remain in Redis after both passes")
		})
	})

	// ========================================
	// SRE-M1: Nil auditRepo guard in retry worker
	// ========================================
	Describe("SRE-M1: Nil auditRepo guard in retry worker (UT-DS-1048-SRE-M1)", func() {
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

		It("should not panic when auditRepo is nil (UT-DS-1048-SRE-M1-001)", func() {
			ctx := context.Background()

			// ARRANGE: Enqueue an event DLQ message with past timestamp
			event := &audit.AuditEvent{
				EventID:        uuid.New(),
				EventVersion:   "1.0",
				EventType:      "test.nil.guard",
				EventCategory:  "test",
				EventAction:    "tested",
				EventOutcome:   "success",
				EventTimestamp: time.Now().UTC(),
				CorrelationID:  "corr-sre-m1-nil",
				ActorType:      "service",
				ActorID:        "test-service",
				ResourceType:   "test",
				ResourceID:     "test-nil",
				EventData:      []byte(`{"guard":"nil"}`),
				RetentionDays:  2555,
			}
			err := addDLQMessageWithPastTimestamp(ctx, redisClient, "events", event, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred())

			// ARRANGE: Worker with nil auditRepo (and nil notificationRepo)
			workerConfig := server.DLQRetryWorkerConfig{
				PollInterval:  100 * time.Millisecond,
				MaxBatchSize:  10,
				MaxRetries:    6,
				ReadTimeout:   100 * time.Millisecond,
				ConsumerGroup: "test-nil-guard",
				ConsumerName:  "test-nil-1",
			}
			worker := server.NewDLQRetryWorker(dlqClient, nil, nil, workerConfig, logger, nil)

			worker.Start(ctx)
			Eventually(func() int64 {
				depth, _ := dlqClient.GetDLQDepth(ctx, "events")
				return depth
			}).WithTimeout(2 * time.Second).WithPolling(50 * time.Millisecond).Should(BeNumerically(">=", 1),
				"SRE-M1: nil auditRepo must return error, message stays in DLQ for retry")
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
			stats, drainErr := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, failingRepo)

			// ASSERT: DF-H1: Drain returns error when all writes fail
			Expect(drainErr).To(HaveOccurred(),
				"DF-H1: DrainWithTimeout must return error when messages remain after 2 passes")
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
			stats, drainErr := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, failingRepo)
			elapsed := time.Since(start)

			// ASSERT: Completes quickly and returns error (DF-H1)
			Expect(drainErr).To(HaveOccurred(),
				"DF-H1: DrainWithTimeout must return error when messages remain")
			Expect(stats.TotalProcessed).To(Equal(0))
			Expect(elapsed).To(BeNumerically("<", 1*time.Second),
				"DF-3: drain must not spin in infinite loop on DB failure; cursor must advance")

			// ASSERT: Message preserved in Redis
			depth, err := dlqClient.GetDLQDepth(ctx, "events")
			Expect(err).ToNot(HaveOccurred())
			Expect(depth).To(Equal(int64(1)), "failed message must remain in stream")
		})

		It("should process remaining messages after one fails and recover via retry pass (UT-DS-1048-DF3-004)", func() {
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

			// ARRANGE: Mock repo that fails on the second event only (transient).
			// Pass 1 writes events 0+2, skips event 1. Pass 2 retries event 1
			// (call index 3 != failOnIndex 1) and succeeds.
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

			// ASSERT: All 3 recovered (2 in pass 1, 1 in retry pass)
			Expect(err).ToNot(HaveOccurred())
			Expect(stats.EventsProcessed).To(Equal(3),
				"ARCH-F1: retry pass must recover the transiently-failed message")
			Expect(selectiveRepo.createdEvents).To(HaveLen(3),
				"DF-3+ARCH-F1: all events must be written after two-pass drain")

			// ASSERT: DLQ fully drained
			finalDepth, err := dlqClient.GetDLQDepth(ctx, "events")
			Expect(err).ToNot(HaveOccurred())
			Expect(finalDepth).To(Equal(int64(0)),
				"ARCH-F1: retry pass should have recovered all messages")
		})
	})
})

// ========================================
// MOCK: Selective failure repository
// ========================================

// SelectiveFailEventsRepository fails on a specific call index.
// Mutex-protected to be safe under concurrent drain goroutines.
type SelectiveFailEventsRepository struct {
	mu            sync.Mutex
	createdEvents []*repository.AuditEvent
	failOnIndex   int
	callCount     int
}

func (m *SelectiveFailEventsRepository) Create(ctx context.Context, event *repository.AuditEvent) (*repository.AuditEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.callCount
	m.callCount++
	if idx == m.failOnIndex {
		return nil, fmt.Errorf("selective failure on call %d", idx)
	}
	m.createdEvents = append(m.createdEvents, event)
	return event, nil
}

// ========================================
// MOCK: Transient failure repository (ARCH-F1 tests)
// ========================================

// TransientFailEventsRepository fails for the first N calls, then succeeds.
// Mutex-protected to be safe under concurrent drain goroutines.
type TransientFailEventsRepository struct {
	mu              sync.Mutex
	createdEvents   []*repository.AuditEvent
	failFirstNCalls int
	callCount       int
}

func (m *TransientFailEventsRepository) Create(_ context.Context, event *repository.AuditEvent) (*repository.AuditEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.callCount <= m.failFirstNCalls {
		return nil, fmt.Errorf("transient failure on call %d", m.callCount)
	}
	m.createdEvents = append(m.createdEvents, event)
	return event, nil
}

// ========================================
// QE-H1: Context cancellation branch tests
// ========================================

var _ = Describe("#1048 QE-H1: Drain context cancellation branches", func() {
	var (
		ctx         context.Context
		miniRedis   *miniredis.Miniredis
		redisClient *redis.Client
		dlqClient   *dlq.Client
		logger      logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		miniRedis = miniredis.RunT(GinkgoT())
		redisClient = redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
		logger = kubelog.NewLogger(kubelog.DefaultOptions())
		var err error
		dlqClient, err = dlq.NewClient(redisClient, logger, 1000)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		_ = redisClient.Close()
		miniRedis.Close()
	})

	It("UT-DS-1048-QEH1-001: should stop draining when context is cancelled mid-batch", func() {
		// ARRANGE: Add enough messages to span multiple iterations
		successRepo := &MockRepositoryEventsRepository{createdEvents: []*repository.AuditEvent{}}
		mockNotifRepo := &MockNotificationRepository{}
		for i := 0; i < 5; i++ {
			event := &audit.AuditEvent{
				EventID:       uuid.New(),
				EventVersion:  "1.0",
				EventType:     "test.cancel.event",
				EventCategory: "test",
				EventAction:   "processed",
				EventOutcome:  "success",
				EventTimestamp: time.Now(),
				CorrelationID: fmt.Sprintf("corr-cancel-%d", i),
				ActorType:     "service",
				ActorID:       "test-service",
				ResourceType:  "test",
				ResourceID:    fmt.Sprintf("res-%d", i),
				EventData:     []byte(`{"k":"v"}`),
				RetentionDays: 2555,
			}
			err := dlqClient.EnqueueAuditEvent(ctx, event, fmt.Errorf("test"))
			Expect(err).ToNot(HaveOccurred())
		}

		// ACT: Create a context that is already cancelled
		cancelCtx, cancelFn := context.WithCancel(ctx)
		cancelFn()

		stats, err := dlqClient.DrainWithTimeout(cancelCtx, mockNotifRepo, successRepo)

		// ASSERT: Drain returns without error but processes zero messages
		Expect(err).ToNot(HaveOccurred())
		Expect(stats.TotalProcessed).To(Equal(0))
		Expect(stats.TimedOut).To(BeTrue())

		// Messages remain in DLQ
		depth, err := dlqClient.GetDLQDepth(ctx, "events")
		Expect(err).ToNot(HaveOccurred())
		Expect(depth).To(Equal(int64(5)))
	})

	It("UT-DS-1048-QEH1-002: should handle context deadline mid-processing", func() {
		// ARRANGE: A repo that cancels context after first write
		mockNotifRepo := &MockNotificationRepository{}
		cancelCtx, cancelFn := context.WithCancel(ctx)
		wrappedRepo := &CancellingEventsRepository{
			cancelAfterN: 1,
			cancelFn:     cancelFn,
		}
		for i := 0; i < 3; i++ {
			event := &audit.AuditEvent{
				EventID:       uuid.New(),
				EventVersion:  "1.0",
				EventType:     "test.cancel.mid",
				EventCategory: "test",
				EventAction:   "processed",
				EventOutcome:  "success",
				EventTimestamp: time.Now(),
				CorrelationID: fmt.Sprintf("corr-mid-%d", i),
				ActorType:     "service",
				ActorID:       "test-service",
				ResourceType:  "test",
				ResourceID:    fmt.Sprintf("res-%d", i),
				EventData:     []byte(`{"k":"v"}`),
				RetentionDays: 2555,
			}
			err := dlqClient.EnqueueAuditEvent(ctx, event, fmt.Errorf("test"))
			Expect(err).ToNot(HaveOccurred())
		}

		// ACT
		stats, err := dlqClient.DrainWithTimeout(cancelCtx, mockNotifRepo, wrappedRepo)

		// ASSERT: Partial drain — at most 1 message processed before cancel
		Expect(err).ToNot(HaveOccurred())
		Expect(stats.EventsProcessed).To(BeNumerically("<=", 1))

		// Remaining messages still in DLQ
		depth, err := dlqClient.GetDLQDepth(context.Background(), "events")
		Expect(err).ToNot(HaveOccurred())
		Expect(depth).To(BeNumerically(">=", 2))
	})
})

// ========================================
// QE-M2: Missing branch coverage tests
// ========================================

var _ = Describe("#1048 QE-M2: DLQ branch coverage", func() {
	var (
		ctx         context.Context
		miniRedis   *miniredis.Miniredis
		redisClient *redis.Client
		dlqClient   *dlq.Client
		logger      logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		miniRedis = miniredis.RunT(GinkgoT())
		redisClient = redis.NewClient(&redis.Options{Addr: miniRedis.Addr()})
		logger = kubelog.NewLogger(kubelog.DefaultOptions())
		var err error
		dlqClient, err = dlq.NewClient(redisClient, logger, 1000)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		_ = redisClient.Close()
		miniRedis.Close()
	})

	It("UT-DS-1048-QEM2-001: should handle unmarshal failure in drain (malformed payload)", func() {
		mockNotifRepo := &MockNotificationRepository{}
		successRepo := &MockRepositoryEventsRepository{createdEvents: []*repository.AuditEvent{}}

		// ARRANGE: Insert a stream entry where the "message" value is not valid
		// AuditMessage JSON. This simulates a corrupt entry in the Redis stream.
		streamKey := "audit:dlq:events"
		err := redisClient.XAdd(ctx, &redis.XAddArgs{
			Stream: streamKey,
			ID:     "*",
			Values: map[string]interface{}{"message": `{not valid json at all`},
		}).Err()
		Expect(err).ToNot(HaveOccurred())

		// ACT
		drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		stats, drainErr := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, successRepo)

		// ASSERT: DF-H1: Drain returns error for unrecoverable malformed messages
		Expect(drainErr).To(HaveOccurred(),
			"DF-H1: malformed message remains after 2 passes → drain must report error")
		Expect(stats.EventsProcessed).To(Equal(0))

		depth, err := dlqClient.GetDLQDepth(ctx, "events")
		Expect(err).ToNot(HaveOccurred())
		Expect(depth).To(Equal(int64(1)), "malformed message remains in stream")
	})

	It("UT-DS-1048-QEM2-002: should reject oversized payload (FEDRAMP SI-10)", func() {
		mockNotifRepo := &MockNotificationRepository{}
		successRepo := &MockRepositoryEventsRepository{createdEvents: []*repository.AuditEvent{}}

		// ARRANGE: Build an AuditMessage whose Payload exceeds maxPayloadSize.
		// We create a valid JSON object with a large string field.
		largeValue := make([]byte, dlq.MaxPayloadSize()+100)
		for i := range largeValue {
			largeValue[i] = 'x'
		}
		bigPayload := []byte(fmt.Sprintf(`{"big":"%s"}`, string(largeValue)))

		bigMsg := dlq.AuditMessage{
			Type:      "audit_event",
			Payload:   bigPayload,
			Timestamp: time.Now(),
		}
		msgJSON, err := json.Marshal(bigMsg)
		Expect(err).ToNot(HaveOccurred())

		streamKey := "audit:dlq:events"
		err = redisClient.XAdd(ctx, &redis.XAddArgs{
			Stream: streamKey,
			ID:     "*",
			Values: map[string]interface{}{"message": string(msgJSON)},
		}).Err()
		Expect(err).ToNot(HaveOccurred())

		// ACT
		drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		stats, drainErr := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, successRepo)

		// ASSERT: DF-H1: Oversized payload remains after 2 passes → drain reports error
		Expect(drainErr).To(HaveOccurred(),
			"DF-H1: oversized message remains after 2 passes → drain must report error")
		Expect(stats.EventsProcessed).To(Equal(0))

		depth, err := dlqClient.GetDLQDepth(ctx, "events")
		Expect(err).ToNot(HaveOccurred())
		Expect(depth).To(Equal(int64(1)), "oversized message remains in stream")
	})

	It("UT-DS-1048-QEM2-003: should handle drain pagination (>100 messages)", func() {
		mockNotifRepo := &MockNotificationRepository{}
		successRepo := &MockRepositoryEventsRepository{createdEvents: []*repository.AuditEvent{}}

		// ARRANGE: Enqueue 150 event messages (drainBatchSize is 100)
		for i := 0; i < 150; i++ {
			event := &audit.AuditEvent{
				EventID:       uuid.New(),
				EventVersion:  "1.0",
				EventType:     "test.pagination",
				EventCategory: "test",
				EventAction:   "processed",
				EventOutcome:  "success",
				EventTimestamp: time.Now(),
				CorrelationID: fmt.Sprintf("corr-page-%d", i),
				ActorType:     "service",
				ActorID:       "test-service",
				ResourceType:  "test",
				ResourceID:    fmt.Sprintf("res-%d", i),
				EventData:     []byte(`{"k":"v"}`),
				RetentionDays: 2555,
			}
			err := dlqClient.EnqueueAuditEvent(ctx, event, fmt.Errorf("test"))
			Expect(err).ToNot(HaveOccurred())
		}

		depth, err := dlqClient.GetDLQDepth(ctx, "events")
		Expect(err).ToNot(HaveOccurred())
		Expect(depth).To(Equal(int64(150)))

		// ACT
		drainCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		stats, err := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, successRepo)

		// ASSERT: All 150 messages processed across multiple batches
		Expect(err).ToNot(HaveOccurred())
		Expect(stats.EventsProcessed).To(Equal(150))
		Expect(successRepo.createdEvents).To(HaveLen(150))

		finalDepth, err := dlqClient.GetDLQDepth(ctx, "events")
		Expect(err).ToNot(HaveOccurred())
		Expect(finalDepth).To(Equal(int64(0)))
	})
})

// ========================================
// CancellingEventsRepository: cancels context after N successful writes (for QE-H1)
// ========================================

type CancellingEventsRepository struct {
	cancelAfterN int
	cancelFn     context.CancelFunc
	callCount    int
}

func (m *CancellingEventsRepository) Create(_ context.Context, event *repository.AuditEvent) (*repository.AuditEvent, error) {
	m.callCount++
	if m.callCount >= m.cancelAfterN {
		m.cancelFn()
	}
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

	// QE-M3: Use production-consistent Type values.
	// Production enqueues "notification_audit" / "audit_event"; match those here.
	msgType := auditType
	switch auditType {
	case "notifications":
		msgType = "notification_audit"
	case "events":
		msgType = "audit_event"
	}

	auditMsg := dlq.AuditMessage{
		Type:       msgType,
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
