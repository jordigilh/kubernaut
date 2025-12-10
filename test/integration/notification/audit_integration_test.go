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
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"

	// PostgreSQL driver for database verification
	_ "github.com/lib/pq"
)

// ========================================
// NOTIFICATION AUDIT INTEGRATION TESTS
// ========================================
//
// Authority: TESTING_GUIDELINES.md - Integration tests use REAL infrastructure
// Related: DD-AUDIT-002, ADR-038, BR-NOT-062, BR-NOT-063, BR-NOT-064
// Port Allocation: DD-TEST-001 (Data Storage: 18090, PostgreSQL: 15433)
//
// Prerequisites:
//   podman-compose -f podman-compose.test.yml up -d postgres redis datastorage
//
// These tests validate audit event persistence against REAL Data Storage Service.
// They will SKIP if infrastructure isn't available.
//
// ========================================

var _ = Describe("Notification Audit Integration Tests (Real Infrastructure)", func() {
	var (
		dataStorageURL string
		postgresURL    string
		auditStore     audit.AuditStore
		db             *sql.DB
		ctx            context.Context
		httpClient     *http.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		httpClient = &http.Client{Timeout: 10 * time.Second}

		// Get Data Storage URL from environment or use DD-TEST-001 default
		dataStorageURL = os.Getenv("DATA_STORAGE_URL")
		if dataStorageURL == "" {
			dataStorageURL = "http://localhost:18090" // DD-TEST-001 integration port
		}

		// Get PostgreSQL URL from environment or use DD-TEST-001 default
		postgresURL = os.Getenv("POSTGRES_URL")
		if postgresURL == "" {
			postgresURL = "postgres://slm_user:test_password@localhost:15433/action_history?sslmode=disable"
		}

		// Check if Data Storage is available
		resp, err := httpClient.Get(dataStorageURL + "/health")
		if err != nil {
			Fail(fmt.Sprintf(
				"REQUIRED: Data Storage not available at %s\n"+
					"  Per TESTING_GUIDELINES.md: Integration tests MUST use real services\n"+
					"  Start with: podman-compose -f podman-compose.test.yml up -d",
				dataStorageURL))
		}
		resp.Body.Close()

		// Connect to PostgreSQL for verification
		db, err = sql.Open("postgres", postgresURL)
		Expect(err).ToNot(HaveOccurred(),
			"REQUIRED: PostgreSQL connection failed\n"+
				"  Per TESTING_GUIDELINES.md: Integration tests MUST use real services\n"+
				"  Start with: podman-compose -f podman-compose.test.yml up -d postgres")

		// Verify PostgreSQL connection
		err = db.Ping()
		Expect(err).ToNot(HaveOccurred(),
			"REQUIRED: PostgreSQL not reachable\n"+
				"  Per TESTING_GUIDELINES.md: Integration tests MUST use real services\n"+
				"  Verify PostgreSQL is running: podman-compose -f podman-compose.test.yml ps")

		// Create audit store with REAL Data Storage client
		dataStorageClient := audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)

		config := audit.Config{
			BufferSize:    1000,
			BatchSize:     10,
			FlushInterval: 100 * time.Millisecond,
			MaxRetries:    3,
		}

		logger := crzap.New(crzap.UseDevMode(true))
		auditStore, err = audit.NewBufferedStore(dataStorageClient, config, "notification-controller", logger)
		Expect(err).ToNot(HaveOccurred(), "Failed to create audit store")
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
	// TEST 1: Data Storage HTTP Integration (BR-NOT-062)
	// Validates: Audit events are written to Data Storage and persisted to PostgreSQL
	// ========================================
	Context("BR-NOT-062: Unified Audit Table Integration", func() {
		It("should write audit event to Data Storage Service and persist to PostgreSQL", func() {
			// Create unique correlation ID for this test
			correlationID := fmt.Sprintf("integration-test-%d", time.Now().UnixNano())
			notification := createTestNotificationForIntegration()

			// Create audit event (simulate message sent)
			event := audit.NewAuditEvent()
			event.EventType = "notification.message.sent"
			event.EventCategory = "notification"
			event.EventAction = "sent"
			event.EventOutcome = "success"
			event.ActorType = "service"
			event.ActorID = "notification-controller"
			event.ResourceType = "NotificationRequest"
			event.ResourceID = notification.Name
			event.CorrelationID = correlationID
			event.EventTimestamp = time.Now()
			event.EventData = []byte(`{"notification_id": "integration-test-notification", "channel": "slack", "recipient": "#integration-tests"}`)

			// Store audit event (fire-and-forget)
			err := auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred(), "Storing audit event should not fail")

			// Flush the buffer
			err = auditStore.Close()
			Expect(err).ToNot(HaveOccurred(), "Closing audit store should flush events")
			auditStore = nil // Prevent double-close in AfterEach

			// REAL INFRASTRUCTURE VERIFICATION: Query PostgreSQL directly
			var count int
			Eventually(func() int {
				err := db.QueryRow(`
					SELECT COUNT(*) FROM audit_events
					WHERE correlation_id = $1
					AND event_type = 'notification.message.sent'
				`, correlationID).Scan(&count)
				if err != nil {
					GinkgoWriter.Printf("Query error: %v\n", err)
					return 0
				}
				return count
			}, 5*time.Second, 200*time.Millisecond).Should(Equal(1),
				"Audit event should be persisted to PostgreSQL audit_events table")
		})
	})

	// ========================================
	// TEST 2: Async Buffer Flush (BR-NOT-062)
	// Validates: Multiple events are batched and persisted correctly
	// ========================================
	Context("BR-NOT-062: Async Buffered Audit Writes", func() {
		It("should flush batch of events to PostgreSQL", func() {
			// Create unique correlation ID for this batch
			correlationID := fmt.Sprintf("batch-test-%d", time.Now().UnixNano())
			notification := createTestNotificationForIntegration()

			// Write 15 events (BatchSize = 10, so should flush in 2 batches)
			for i := 0; i < 15; i++ {
				event := audit.NewAuditEvent()
				event.EventType = "notification.message.sent"
				event.EventCategory = "notification"
				event.EventAction = "sent"
				event.EventOutcome = "success"
				event.ActorType = "service"
				event.ActorID = "notification-controller"
				event.ResourceType = "NotificationRequest"
				event.ResourceID = notification.Name
				event.CorrelationID = correlationID
				event.EventTimestamp = time.Now()
				event.EventData = []byte(fmt.Sprintf(`{"notification_id": "batch-notification-%d", "channel": "slack"}`, i))

				err := auditStore.StoreAudit(ctx, event)
				Expect(err).ToNot(HaveOccurred(), "Storing audit event should not fail")
			}

			// Flush the buffer
			err := auditStore.Close()
			Expect(err).ToNot(HaveOccurred(), "Closing audit store should flush events")
			auditStore = nil // Prevent double-close in AfterEach

			// REAL INFRASTRUCTURE VERIFICATION: Query PostgreSQL directly
			var count int
			Eventually(func() int {
				err := db.QueryRow(`
					SELECT COUNT(*) FROM audit_events
					WHERE correlation_id = $1
				`, correlationID).Scan(&count)
				if err != nil {
					GinkgoWriter.Printf("Query error: %v\n", err)
					return 0
				}
				return count
			}, 5*time.Second, 200*time.Millisecond).Should(Equal(15),
				"All 15 audit events should be persisted to PostgreSQL")
		})
	})

	// ========================================
	// TEST 3: Graceful Degradation (BR-NOT-063)
	// Validates: Audit failures don't block notification delivery
	// ========================================
	Context("BR-NOT-063: Graceful Audit Degradation", func() {
		It("should not block when storing audit events (fire-and-forget pattern)", func() {
			notification := createTestNotificationForIntegration()
			correlationID := fmt.Sprintf("graceful-test-%d", time.Now().UnixNano())

			// Create audit event
			event := audit.NewAuditEvent()
			event.EventType = "notification.message.failed"
			event.EventCategory = "notification"
			event.EventAction = "failed"
			event.EventOutcome = "failure"
			event.ActorType = "service"
			event.ActorID = "notification-controller"
			event.ResourceType = "NotificationRequest"
			event.ResourceID = notification.Name
			event.CorrelationID = correlationID
			event.EventTimestamp = time.Now()
			event.EventData = []byte(`{"notification_id": "graceful-test", "error": "delivery_failed"}`)

			// Store should not block (fire-and-forget pattern)
			start := time.Now()
			err := auditStore.StoreAudit(ctx, event)
			elapsed := time.Since(start)

			// Verify StoreAudit returns quickly (non-blocking)
			Expect(err).ToNot(HaveOccurred(), "Storing audit event should not fail")
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond),
				"StoreAudit should return immediately (fire-and-forget)")
		})
	})

	// ========================================
	// TEST 4: Graceful Shutdown (No Data Loss)
	// Validates: All buffered events are flushed before shutdown
	// ========================================
	Context("Graceful Shutdown", func() {
		It("should flush all remaining events before shutdown", func() {
			notification := createTestNotificationForIntegration()
			correlationID := fmt.Sprintf("shutdown-test-%d", time.Now().UnixNano())

			// Buffer 5 events (less than BatchSize = 10, so won't auto-flush immediately)
			for i := 0; i < 5; i++ {
				event := audit.NewAuditEvent()
				event.EventType = "notification.message.acknowledged"
				event.EventCategory = "notification"
				event.EventAction = "acknowledged"
				event.EventOutcome = "success"
				event.ActorType = "service"
				event.ActorID = "notification-controller"
				event.ResourceType = "NotificationRequest"
				event.ResourceID = notification.Name
				event.CorrelationID = correlationID
				event.EventTimestamp = time.Now()
				event.EventData = []byte(fmt.Sprintf(`{"notification_id": "shutdown-notification-%d", "acknowledged_by": "user@example.com"}`, i))

				err := auditStore.StoreAudit(ctx, event)
				Expect(err).ToNot(HaveOccurred(), "Storing audit event should not fail")
			}

			// Immediately close the audit store (should trigger flush)
			err := auditStore.Close()
			Expect(err).ToNot(HaveOccurred(), "Closing audit store should not fail")
			auditStore = nil // Prevent double-close in AfterEach

			// REAL INFRASTRUCTURE VERIFICATION: Verify all 5 events were flushed
			var count int
			Eventually(func() int {
				err := db.QueryRow(`
					SELECT COUNT(*) FROM audit_events
					WHERE correlation_id = $1
				`, correlationID).Scan(&count)
				if err != nil {
					GinkgoWriter.Printf("Query error: %v\n", err)
					return 0
				}
				return count
			}, 5*time.Second, 200*time.Millisecond).Should(Equal(5),
				"All 5 buffered events should be flushed on graceful shutdown")
		})
	})

	// ========================================
	// TEST 5: Correlation ID Tracing (BR-NOT-064)
	// Validates: Events can be queried by correlation_id for workflow tracing
	// ========================================
	Context("BR-NOT-064: Audit Event Correlation", func() {
		It("should enable workflow tracing via correlation_id", func() {
			notification := createTestNotificationForIntegration()
			correlationID := fmt.Sprintf("correlation-test-%d", time.Now().UnixNano())

			// Create multiple event types with same correlation_id
			// Event 1: Message sent
			sentEvent := audit.NewAuditEvent()
			sentEvent.EventType = "notification.message.sent"
			sentEvent.EventCategory = "notification"
			sentEvent.EventAction = "sent"
			sentEvent.EventOutcome = "success"
			sentEvent.ActorType = "service"
			sentEvent.ActorID = "notification-controller"
			sentEvent.ResourceType = "NotificationRequest"
			sentEvent.ResourceID = notification.Name
			sentEvent.CorrelationID = correlationID
			sentEvent.EventTimestamp = time.Now()
			sentEvent.EventData = []byte(`{"notification_id": "correlation-test", "channel": "slack"}`)
			err := auditStore.StoreAudit(ctx, sentEvent)
			Expect(err).ToNot(HaveOccurred())

			// Event 2: Message failed (different channel)
			failedEvent := audit.NewAuditEvent()
			failedEvent.EventType = "notification.message.failed"
			failedEvent.EventCategory = "notification"
			failedEvent.EventAction = "failed"
			failedEvent.EventOutcome = "failure"
			failedEvent.ActorType = "service"
			failedEvent.ActorID = "notification-controller"
			failedEvent.ResourceType = "NotificationRequest"
			failedEvent.ResourceID = notification.Name
			failedEvent.CorrelationID = correlationID
			failedEvent.EventTimestamp = time.Now()
			failedEvent.EventData = []byte(`{"notification_id": "correlation-test", "channel": "email", "error": "SMTP timeout"}`)
			err = auditStore.StoreAudit(ctx, failedEvent)
			Expect(err).ToNot(HaveOccurred())

			// Flush
			err = auditStore.Close()
			Expect(err).ToNot(HaveOccurred())
			auditStore = nil

			// REAL INFRASTRUCTURE VERIFICATION: Query all events by correlation_id
			var count int
			Eventually(func() int {
				err := db.QueryRow(`
					SELECT COUNT(*) FROM audit_events
					WHERE correlation_id = $1
				`, correlationID).Scan(&count)
				if err != nil {
					GinkgoWriter.Printf("Query error: %v\n", err)
					return 0
				}
				return count
			}, 5*time.Second, 200*time.Millisecond).Should(Equal(2),
				"Both events should be queryable by correlation_id")

			// Verify event types are correct (order-independent check)
			rows, err := db.Query(`
				SELECT event_type, event_outcome FROM audit_events
				WHERE correlation_id = $1
			`, correlationID)
			Expect(err).ToNot(HaveOccurred())
			defer rows.Close()

			eventTypes := make(map[string]string) // event_type -> outcome
			for rows.Next() {
				var eventType, outcome string
				err := rows.Scan(&eventType, &outcome)
				Expect(err).ToNot(HaveOccurred())
				eventTypes[eventType] = outcome
			}

			Expect(eventTypes).To(HaveLen(2), "Should find 2 events with same correlation_id")
			Expect(eventTypes).To(HaveKey("notification.message.sent"))
			Expect(eventTypes["notification.message.sent"]).To(Equal("success"))
			Expect(eventTypes).To(HaveKey("notification.message.failed"))
			Expect(eventTypes["notification.message.failed"]).To(Equal("failure"))
		})
	})

	// ========================================
	// TEST 6: ADR-034 Compliance Verification
	// Validates: Event format matches unified audit table schema
	// ========================================
	Context("ADR-034: Unified Audit Table Format", func() {
		It("should persist event with all ADR-034 required fields", func() {
			notification := createTestNotificationForIntegration()
			correlationID := fmt.Sprintf("adr034-test-%d", time.Now().UnixNano())

			event := audit.NewAuditEvent()
			event.EventType = "notification.message.sent"
			event.EventCategory = "notification"
			event.EventAction = "sent"
			event.EventOutcome = "success"
			event.ActorType = "service"
			event.ActorID = "notification-controller"
			event.ResourceType = "NotificationRequest"
			event.ResourceID = notification.Name
			event.CorrelationID = correlationID
			event.EventTimestamp = time.Now()
			event.EventData = []byte(`{"notification_id": "adr034-test", "channel": "slack"}`)

			err := auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			err = auditStore.Close()
			Expect(err).ToNot(HaveOccurred())
			auditStore = nil

			// REAL INFRASTRUCTURE VERIFICATION: Query event with all ADR-034 fields
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

			Eventually(func() error {
				return db.QueryRow(`
					SELECT
						event_type, event_category, event_action, event_outcome,
						actor_type, actor_id, resource_type, resource_id,
						retention_days
					FROM audit_events
					WHERE correlation_id = $1
				`, correlationID).Scan(
					&eventType, &eventCategory, &eventAction, &eventOutcome,
					&actorType, &actorID, &resourceType, &resourceID,
					&retentionDays,
				)
			}, 5*time.Second, 200*time.Millisecond).Should(Succeed(),
				"Event should be queryable from database")

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

// ===== INTEGRATION TEST HELPERS =====

// createTestNotificationForIntegration creates a test notification for integration tests
func createTestNotificationForIntegration() *notificationv1alpha1.NotificationRequest {
	return &notificationv1alpha1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "integration-test-notification",
			Namespace: "default",
		},
		Spec: notificationv1alpha1.NotificationRequestSpec{
			Type:     notificationv1alpha1.NotificationTypeSimple,
			Priority: notificationv1alpha1.NotificationPriorityCritical,
			Subject:  "Integration Test Alert",
			Body:     "Test notification for audit integration",
			Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelSlack},
			Recipients: []notificationv1alpha1.Recipient{
				{Slack: "#integration-tests"},
			},
			Metadata: map[string]string{
				"remediationRequestName": "integration-test-remediation",
				"cluster":                "test-cluster",
			},
		},
	}
}

// validateAuditEventADR034 validates that an audit event follows ADR-034 format
func validateAuditEventADR034(event *audit.AuditEvent) {
	Expect(event).ToNot(BeNil(), "Audit event should not be nil")
	Expect(event.EventType).To(MatchRegexp(`^notification\.(message|status)\.(sent|failed|acknowledged|escalated)$`),
		"Event type must follow ADR-034 format: <service>.<category>.<action>")
	Expect(event.EventCategory).To(Equal("notification"), "Event category must be 'notification'")
	Expect(event.ActorType).To(Equal("service"), "Actor type must be 'service'")
	Expect(event.ActorID).To(Equal("notification-controller"), "Actor ID must match service name")
	Expect(event.ResourceType).To(Equal("NotificationRequest"), "Resource type must be 'NotificationRequest'")
	Expect(event.RetentionDays).To(Equal(2555), "Retention must be 2555 days (7 years) for compliance")
	Expect(event.EventData).ToNot(BeNil(), "Event data must be populated")

	// Validate event_data is valid JSON
	var eventData map[string]interface{}
	err := json.Unmarshal(event.EventData, &eventData)
	Expect(err).ToNot(HaveOccurred(), "Event data must be valid JSON (JSONB compatible)")
	Expect(eventData).To(HaveKey("notification_id"), "Event data must contain notification_id")
	Expect(eventData).To(HaveKey("channel"), "Event data must contain channel")
}
