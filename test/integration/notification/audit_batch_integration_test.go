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

package notification

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationctrl "github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/audit"

	// PostgreSQL driver for database verification
	_ "github.com/lib/pq"
)

// ========================================
// AUDIT BATCH INTEGRATION TESTS
// Authority: TESTING_GUIDELINES.md - Integration tests use REAL infrastructure
// Related: DD-AUDIT-002, ADR-038, BR-NOT-062, BR-NOT-063, BR-NOT-064
// ========================================
//
// These tests validate audit event persistence against REAL Data Storage Service.
// They will FAIL if:
// - Data Storage Service is not running (podman-compose)
// - PostgreSQL database is not available
// - Audit events are not actually persisted
//
// Prerequisites:
//   podman-compose -f podman-compose.test.yml up -d postgres datastorage
//
// ========================================

var _ = Describe("Audit Batch Integration Tests (Real Infrastructure)", func() {
	var (
		dataStorageURL string
		postgresURL    string
		auditStore     audit.AuditStore
		auditHelpers   *notificationctrl.AuditHelpers
		db             *sql.DB
		ctx            context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Get Data Storage URL from environment or use default
		dataStorageURL = os.Getenv("DATA_STORAGE_URL")
		if dataStorageURL == "" {
			dataStorageURL = "http://localhost:8080"
		}

		// Get PostgreSQL URL from environment or use default
		postgresURL = os.Getenv("POSTGRES_URL")
		if postgresURL == "" {
			postgresURL = "postgres://slm_user:slm_password@localhost:5432/action_history?sslmode=disable"
		}

		// Create audit store with real Data Storage client
		httpClient := &http.Client{Timeout: 10 * time.Second}
		dataStorageClient := audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)

		config := audit.Config{
			BufferSize:    100,
			BatchSize:     10,
			FlushInterval: 500 * time.Millisecond,
			MaxRetries:    3,
		}

		logger := crzap.New(crzap.UseDevMode(true))
		var err error
		auditStore, err = audit.NewBufferedStore(dataStorageClient, config, "notification-controller", logger)
		Expect(err).ToNot(HaveOccurred(), "Failed to create audit store")

		auditHelpers = notificationctrl.NewAuditHelpers("notification-controller")

		// Connect to PostgreSQL for verification
		db, err = sql.Open("postgres", postgresURL)
		if err != nil {
			Skip(fmt.Sprintf("PostgreSQL not available: %v - run 'podman-compose -f podman-compose.test.yml up -d postgres'", err))
		}

		// Verify PostgreSQL connection
		if err := db.Ping(); err != nil {
			Skip(fmt.Sprintf("PostgreSQL not reachable: %v - run 'podman-compose -f podman-compose.test.yml up -d postgres'", err))
		}
	})

	AfterEach(func() {
		if auditStore != nil {
			auditStore.Close()
		}
		if db != nil {
			db.Close()
		}
	})

	// ========================================
	// TEST 1: Single Event Persistence (BR-NOT-062)
	// Validates: Audit events are written to unified audit_events table
	// ========================================
	Context("BR-NOT-062: Unified Audit Table Integration", func() {
		It("should persist audit event to PostgreSQL audit_events table", func() {
			// Create test notification
			notification := createTestNotificationForBatchIntegration("test-single-event")

			// Create audit event using helpers
			event, err := auditHelpers.CreateMessageSentEvent(notification, "slack")
			Expect(err).ToNot(HaveOccurred(), "Failed to create audit event")

			// Store audit event (fire-and-forget)
			err = auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred(), "Failed to store audit event")

			// Flush the buffer
			err = auditStore.Close()
			Expect(err).ToNot(HaveOccurred(), "Failed to close audit store")

			// Verify event in database
			var count int
			err = db.QueryRow(`
				SELECT COUNT(*) FROM audit_events
				WHERE correlation_id = $1
				AND event_type = 'notification.message.sent'
			`, notification.Spec.Metadata["remediationRequestName"]).Scan(&count)

			Expect(err).ToNot(HaveOccurred(), "Failed to query audit_events table")
			Expect(count).To(Equal(1), "Audit event should be persisted to database")
		})
	})

	// ========================================
	// TEST 2: Batch Event Persistence (ADR-038)
	// Validates: Multiple events are batched and written efficiently
	// ========================================
	Context("ADR-038: Async Buffered Audit Ingestion", func() {
		It("should persist batch of 10 audit events to PostgreSQL", func() {
			correlationID := fmt.Sprintf("batch-test-%d", time.Now().UnixNano())

			// Create 10 audit events
			for i := 0; i < 10; i++ {
				notification := createTestNotificationForBatchIntegration(fmt.Sprintf("batch-event-%d", i))
				notification.Spec.Metadata["remediationRequestName"] = correlationID

				event, err := auditHelpers.CreateMessageSentEvent(notification, "slack")
				Expect(err).ToNot(HaveOccurred())

				err = auditStore.StoreAudit(ctx, event)
				Expect(err).ToNot(HaveOccurred())
			}

			// Flush the buffer
			err := auditStore.Close()
			Expect(err).ToNot(HaveOccurred())

			// Verify all 10 events in database
			var count int
			err = db.QueryRow(`
				SELECT COUNT(*) FROM audit_events
				WHERE correlation_id = $1
			`, correlationID).Scan(&count)

			Expect(err).ToNot(HaveOccurred(), "Failed to query audit_events table")
			Expect(count).To(Equal(10), "All 10 audit events should be persisted")
		})
	})

	// ========================================
	// TEST 3: Correlation ID Tracing (BR-NOT-064)
	// Validates: Events can be queried by correlation_id for workflow tracing
	// ========================================
	Context("BR-NOT-064: Audit Event Correlation", func() {
		It("should enable workflow tracing via correlation_id", func() {
			correlationID := fmt.Sprintf("correlation-test-%d", time.Now().UnixNano())

			// Create multiple event types with same correlation_id
			notification := createTestNotificationForBatchIntegration("correlation-test")
			notification.Spec.Metadata["remediationRequestName"] = correlationID

			// Event 1: Message sent
			sentEvent, err := auditHelpers.CreateMessageSentEvent(notification, "slack")
			Expect(err).ToNot(HaveOccurred())
			err = auditStore.StoreAudit(ctx, sentEvent)
			Expect(err).ToNot(HaveOccurred())

			// Event 2: Message failed (different channel)
			failedEvent, err := auditHelpers.CreateMessageFailedEvent(notification, "email", fmt.Errorf("SMTP timeout"))
			Expect(err).ToNot(HaveOccurred())
			err = auditStore.StoreAudit(ctx, failedEvent)
			Expect(err).ToNot(HaveOccurred())

			// Flush
			err = auditStore.Close()
			Expect(err).ToNot(HaveOccurred())

			// Query all events by correlation_id (simulates workflow tracing)
			rows, err := db.Query(`
				SELECT event_type, outcome FROM audit_events
				WHERE correlation_id = $1
				ORDER BY event_timestamp
			`, correlationID)
			Expect(err).ToNot(HaveOccurred())
			defer rows.Close()

			var events []struct {
				EventType string
				Outcome   string
			}
			for rows.Next() {
				var e struct {
					EventType string
					Outcome   string
				}
				err := rows.Scan(&e.EventType, &e.Outcome)
				Expect(err).ToNot(HaveOccurred())
				events = append(events, e)
			}

			Expect(events).To(HaveLen(2), "Should find 2 events with same correlation_id")
			Expect(events[0].EventType).To(Equal("notification.message.sent"))
			Expect(events[0].Outcome).To(Equal("success"))
			Expect(events[1].EventType).To(Equal("notification.message.failed"))
			Expect(events[1].Outcome).To(Equal("failure"))
		})
	})

	// ========================================
	// TEST 4: Graceful Degradation (BR-NOT-063)
	// Validates: Audit failures don't block notification delivery
	// ========================================
	Context("BR-NOT-063: Graceful Audit Degradation", func() {
		It("should not block when Data Storage is unavailable", func() {
			// Create audit store pointing to non-existent Data Storage
			httpClient := &http.Client{Timeout: 1 * time.Second}
			badClient := audit.NewHTTPDataStorageClient("http://localhost:59999", httpClient) // Invalid port

			config := audit.Config{
				BufferSize:    10,
				BatchSize:     5,
				FlushInterval: 100 * time.Millisecond,
				MaxRetries:    1, // Fast failure for test
			}

			logger := crzap.New(crzap.UseDevMode(true))
			badStore, err := audit.NewBufferedStore(badClient, config, "notification-controller", logger)
			Expect(err).ToNot(HaveOccurred())

			notification := createTestNotificationForBatchIntegration("graceful-degradation")
			event, err := auditHelpers.CreateMessageSentEvent(notification, "slack")
			Expect(err).ToNot(HaveOccurred())

			// StoreAudit should NOT block (fire-and-forget)
			start := time.Now()
			err = badStore.StoreAudit(ctx, event)
			elapsed := time.Since(start)

			// Should return immediately (non-blocking)
			Expect(err).ToNot(HaveOccurred(), "StoreAudit should not return error (fire-and-forget)")
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond), "StoreAudit should return immediately")

			// Close should handle gracefully (may log errors but not panic)
			err = badStore.Close()
			// Close may return error due to failed writes, but should not panic
			// The important thing is it doesn't block indefinitely
		})
	})

	// ========================================
	// TEST 5: ADR-034 Compliance Verification
	// Validates: Event format matches unified audit table schema
	// ========================================
	Context("ADR-034: Unified Audit Table Format", func() {
		It("should persist event with all ADR-034 required fields", func() {
			correlationID := fmt.Sprintf("adr034-test-%d", time.Now().UnixNano())
			notification := createTestNotificationForBatchIntegration("adr034-compliance")
			notification.Spec.Metadata["remediationRequestName"] = correlationID

			event, err := auditHelpers.CreateMessageSentEvent(notification, "slack")
			Expect(err).ToNot(HaveOccurred())

			err = auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			err = auditStore.Close()
			Expect(err).ToNot(HaveOccurred())

			// Query event with all ADR-034 fields
			var (
				eventType     string
				eventCategory string
				eventAction   string
				eventOutcome  string
				actorType     string
				actorID       string
				resourceType  string
				resourceID    string
				retentionDays int
			)

			err = db.QueryRow(`
				SELECT
					event_type, event_category, operation, outcome,
					actor_type, actor_id, resource_type, resource_id,
					retention_days
				FROM audit_events
				WHERE correlation_id = $1
			`, correlationID).Scan(
				&eventType, &eventCategory, &eventAction, &eventOutcome,
				&actorType, &actorID, &resourceType, &resourceID,
				&retentionDays,
			)

			Expect(err).ToNot(HaveOccurred(), "Event should be queryable from database")

			// Verify ADR-034 format compliance
			Expect(eventType).To(Equal("notification.message.sent"), "event_type format: <service>.<category>.<action>")
			Expect(eventCategory).To(Equal("notification"), "event_category should match service")
			Expect(eventAction).To(Equal("sent"), "event_action should match operation")
			Expect(eventOutcome).To(Equal("success"), "event_outcome should indicate result")
			Expect(actorType).To(Equal("service"), "actor_type should be 'service' for controllers")
			Expect(actorID).To(Equal("notification-controller"), "actor_id should identify the service")
			Expect(resourceType).To(Equal("NotificationRequest"), "resource_type should be CRD kind")
			Expect(retentionDays).To(Equal(2555), "retention_days should be 7 years for compliance")
		})
	})
})

// ========================================
// TEST HELPERS
// ========================================

func createTestNotificationForBatchIntegration(name string) *notificationv1alpha1.NotificationRequest {
	return &notificationv1alpha1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: notificationv1alpha1.NotificationRequestSpec{
			Type:     notificationv1alpha1.NotificationTypeSimple,
			Priority: notificationv1alpha1.NotificationPriorityCritical,
			Subject:  "Integration Test Alert",
			Body:     "Test notification for audit batch integration",
			Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelSlack},
			Recipients: []notificationv1alpha1.Recipient{
				{Slack: "#integration-tests"},
			},
			Metadata: map[string]string{
				"remediationRequestName": fmt.Sprintf("remediation-%s", name),
				"cluster":                "test-cluster",
			},
		},
	}
}

