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
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// PHASE 9C-RED: Drain Error Contract Tests
// ========================================
//
// Issue: #1088 GA Readiness — DF-H1, SRE-M2
// File Under Test: pkg/datastorage/dlq/client.go
//
// DF-H1: DrainWithTimeout must return an error when messages fail
// SRE-M2: Shutdown must propagate drain errors
//
// NOTE: DrainWithTimeout requires non-nil repos. DLQ is a hard
// dependency — if repos are unavailable, DS must not start.
// Tests use a FailingEventsRepository to simulate DB-down scenarios.
// ========================================

var _ = Describe("Phase 9C: Drain Error Contract (DF-H1, SRE-M2)", func() {

	var (
		miniRedis   *miniredis.Miniredis
		redisClient *redis.Client
		dlqClient   *dlq.Client
		logger      logr.Logger
	)

	BeforeEach(func() {
		miniRedis = miniredis.RunT(GinkgoT())
		redisClient = redis.NewClient(&redis.Options{
			Addr: miniRedis.Addr(),
		})
		logger = logr.Discard()
		var err error
		dlqClient, err = dlq.NewClient(redisClient, logger, 10000)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		redisClient.Close()
		miniRedis.Close()
	})

	Describe("UT-DS-1088-GA-020: DrainWithTimeout returns error on failures", func() {
		It("should return a non-nil error when drain encounters per-message failures", func() {
			ctx := context.Background()

			validEvent := &audit.AuditEvent{
				EventType:     "test.event",
				EventCategory: "security",
				EventAction:   "test",
				EventOutcome:  "success",
				ActorType:     "service",
				ActorID:       "test-service",
				ResourceType:  "test",
				ResourceID:    "test-1",
				CorrelationID: "drain-test-001",
				EventData:     json.RawMessage(`{"test": true}`),
				RetentionDays: 2555,
			}
			err := dlqClient.EnqueueAuditEvent(ctx, validEvent, fmt.Errorf("simulated DB error"))
			Expect(err).ToNot(HaveOccurred())

			drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			// DB is down during drain — repos exist but all writes fail
			failingRepo := &PermanentFailEventsRepository{}
			mockNotifRepo := &MockNotificationRepository{}
			stats, drainErr := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, failingRepo)

			// DF-H1: DrainWithTimeout MUST return an error when messages fail
			Expect(stats).ToNot(BeIdenticalTo(nil), "stats should be returned even on error")
			Expect(drainErr).To(HaveOccurred(),
				"DF-H1: drain must report error when DB writes permanently fail")
			Expect(len(stats.Errors)).To(BeNumerically(">", 0),
				"drain should accumulate errors when per-message writes fail")
		})
	})

	Describe("UT-DS-1088-GA-021: Drain errors propagate to shutdown", func() {
		It("should join drain errors into a single error for shutdown consumption", func() {
			// SRE-M2: DrainWithTimeout must return errors.Join when failures occur
			ctx := context.Background()

			validEvent := &audit.AuditEvent{
				EventType:     "test.event",
				EventCategory: "security",
				EventAction:   "test",
				EventOutcome:  "success",
				ActorType:     "service",
				ActorID:       "test-service",
				ResourceType:  "test",
				ResourceID:    "test-1",
				CorrelationID: "drain-test-002",
				EventData:     json.RawMessage(`{"test": true}`),
				RetentionDays: 2555,
			}
			err := dlqClient.EnqueueAuditEvent(ctx, validEvent, fmt.Errorf("simulated DB error"))
			Expect(err).ToNot(HaveOccurred())

			drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			failingRepo := &PermanentFailEventsRepository{}
			mockNotifRepo := &MockNotificationRepository{}
			stats, drainErr := dlqClient.DrainWithTimeout(drainCtx, mockNotifRepo, failingRepo)

			// SRE-M2: errors.Join produces a single error for shutdown
			Expect(drainErr).To(HaveOccurred(),
				"SRE-M2: drain must return joined error for shutdown propagation")
			Expect(stats.Errors).To(HaveLen(1),
				"SRE-M2: events drain error should be captured in stats.Errors")
			joinedErr := stats.JoinErrors()
			Expect(joinedErr).To(HaveOccurred(),
				"SRE-M2: JoinErrors must produce non-nil error")
		})
	})
})

// PermanentFailEventsRepository always returns an error — simulates DB-down.
type PermanentFailEventsRepository struct{}

func (r *PermanentFailEventsRepository) Create(_ context.Context, _ *repository.AuditEvent) (*repository.AuditEvent, error) {
	return nil, fmt.Errorf("permanent database failure")
}
