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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationcontroller "github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/audit"
)

// ========================================
// E2E Test 1: Full Notification Lifecycle with Audit (REAL DATA STORAGE)
// ========================================
//
// Business Requirements:
// - BR-NOT-062: Unified audit table integration
// - BR-NOT-063: Graceful audit degradation
// - BR-NOT-064: Audit event correlation
//
// Defense-in-Depth: This test validates the FULL audit chain:
// - Controller emits event → BufferedStore buffers → HTTPClient sends →
// - Data Storage receives → PostgreSQL persists
//
// Test Scenario:
// 1. Create NotificationRequest CRD
// 2. Simulate notification delivery (sent)
// 3. Verify audit event persisted to PostgreSQL via Data Storage API
// 4. Simulate acknowledgment
// 5. Verify audit event persisted to PostgreSQL via Data Storage API
// 6. Verify all audit events have correct correlation_id
//
// Expected Results:
// - NotificationRequest CRD created successfully
// - 2 audit events persisted to PostgreSQL (sent + acknowledged)
// - All audit events follow ADR-034 format
// - Audit correlation_id links both events
// - Fire-and-forget pattern ensures no blocking

var _ = Describe("E2E Test 1: Full Notification Lifecycle with Audit", Label("e2e", "lifecycle", "audit"), func() {
	var (
		testCtx          context.Context
		testCancel       context.CancelFunc
		notification     *notificationv1alpha1.NotificationRequest
		auditHelpers     *notificationcontroller.AuditHelpers
		auditStore       audit.AuditStore
		dataStorageURL   string
		notificationName string
		notificationNS   string
		correlationID    string
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 2*time.Minute)

		// Generate unique identifiers for this test
		testID := time.Now().Format("20060102-150405")
		notificationName = "e2e-lifecycle-test-" + testID
		notificationNS = "default"
		correlationID = "e2e-remediation-" + testID

		// Use real Data Storage URL from Kind cluster
		// Data Storage is deployed via DeployNotificationAuditInfrastructure() in suite setup
		dataStorageURL = fmt.Sprintf("http://localhost:%d", dataStorageNodePort)

		// Create audit store pointing to real Data Storage
		httpClient := &http.Client{Timeout: 10 * time.Second}
		dataStorageClient := audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)

		config := audit.Config{
			BufferSize:    1000,
			BatchSize:     10,
			FlushInterval: 100 * time.Millisecond,
			MaxRetries:    3,
		}

		testLogger := crzap.New(crzap.UseDevMode(true))
		auditStore, _ = audit.NewBufferedStore(dataStorageClient, config, "notification", testLogger)

		// Create audit helpers
		auditHelpers = notificationcontroller.NewAuditHelpers("notification")

		// Create NotificationRequest CRD
		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      notificationName,
				Namespace: notificationNS,
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Type:     notificationv1alpha1.NotificationTypeSimple,
				Priority: notificationv1alpha1.NotificationPriorityCritical,
				Subject:  "E2E Lifecycle Test",
				Body:     "Testing full notification lifecycle with audit trail",
				Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelSlack},
				Recipients: []notificationv1alpha1.Recipient{
					{Slack: "#e2e-tests"},
				},
				Metadata: map[string]string{
					"remediationRequestName": correlationID,
					"cluster":                "test-cluster",
				},
			},
		}
	})

	AfterEach(func() {
		testCancel()
		if auditStore != nil {
			auditStore.Close()
		}

		// Clean up NotificationRequest CRD if it exists
		if notification != nil {
			k8sClient.Delete(testCtx, notification)
		}
	})

	It("should create NotificationRequest and persist audit events to PostgreSQL", func() {
		// Per TESTING_GUIDELINES.md: E2E tests MUST use real services, Skip() is FORBIDDEN
		// If Data Storage is unavailable, test MUST fail with clear error
		Expect(dataStorageNodePort).ToNot(Equal(0),
			"REQUIRED: Data Storage not available\n"+
				"  Per TESTING_GUIDELINES.md: E2E tests MUST use real services\n"+
				"  Per DD-AUDIT-003: Audit infrastructure is MANDATORY\n"+
				"  Audit infrastructure should be deployed in SynchronizedBeforeSuite")

		// ===== STEP 1: Create NotificationRequest CRD =====
		By("Creating NotificationRequest CRD")
		err := k8sClient.Create(testCtx, notification)
		Expect(err).ToNot(HaveOccurred(), "NotificationRequest CRD creation should succeed")

		// Verify CRD was created
		createdNotification := &notificationv1alpha1.NotificationRequest{}
		err = k8sClient.Get(testCtx, types.NamespacedName{
			Name:      notificationName,
			Namespace: notificationNS,
		}, createdNotification)
		Expect(err).ToNot(HaveOccurred(), "Should be able to get created NotificationRequest")
		Expect(createdNotification.Name).To(Equal(notificationName))

		// ===== STEP 2: Simulate message sent and create audit event =====
		By("Simulating notification delivery (sent)")
		sentEvent, err := auditHelpers.CreateMessageSentEvent(notification, "slack")
		Expect(err).ToNot(HaveOccurred(), "CreateMessageSentEvent should succeed")

		err = auditStore.StoreAudit(testCtx, sentEvent)
		Expect(err).ToNot(HaveOccurred(), "Storing sent audit event should succeed")

		// Wait for async flush and verify via Data Storage API
		By("Verifying sent event persisted to PostgreSQL via Data Storage API")
		Eventually(func() int {
			return queryAuditEventCount(dataStorageURL, correlationID, "notification.message.sent")
		}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
			"Sent audit event should be persisted to PostgreSQL")

		// ===== STEP 3: Simulate acknowledgment and create audit event =====
		By("Simulating notification acknowledgment")
		ackEvent, err := auditHelpers.CreateMessageAcknowledgedEvent(notification)
		Expect(err).ToNot(HaveOccurred(), "CreateMessageAcknowledgedEvent should succeed")

		err = auditStore.StoreAudit(testCtx, ackEvent)
		Expect(err).ToNot(HaveOccurred(), "Storing acknowledgment audit event should succeed")

		// Wait for async flush and verify via Data Storage API
		By("Verifying acknowledged event persisted to PostgreSQL via Data Storage API")
		Eventually(func() int {
			return queryAuditEventCount(dataStorageURL, correlationID, "notification.message.acknowledged")
		}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
			"Acknowledged audit event should be persisted to PostgreSQL")

		// ===== STEP 4: Verify both events have same correlation_id =====
		By("Verifying correlation_id links both audit events in PostgreSQL")
		totalEvents := queryAuditEventCount(dataStorageURL, correlationID, "")
		Expect(totalEvents).To(BeNumerically(">=", 2),
			"Should have at least 2 audit events with same correlation_id")

		// ===== STEP 5: Verify ADR-034 compliance via Data Storage query =====
		By("Verifying ADR-034 compliance of persisted events")
		events := queryAuditEvents(dataStorageURL, correlationID)
		Expect(events).To(HaveLen(2), "Should have exactly 2 events")

		for _, event := range events {
			Expect(event.EventCategory).To(Equal("notification"), "Category should be 'notification'")
			Expect(event.ActorType).To(Equal("service"), "Actor type should be 'service'")
			Expect(event.ActorID).To(Equal("notification"), "Actor ID should be 'notification'")
			Expect(event.ResourceType).To(Equal("NotificationRequest"), "Resource type should be 'NotificationRequest'")
			Expect(event.ResourceID).To(Equal(notificationName), "Resource ID should match notification name")
			Expect(event.CorrelationID).To(Equal(correlationID), "Correlation ID should match")
			// Note: RetentionDays is stored in PostgreSQL but not returned by Data Storage Query API
			// This is validated by integration tests against the database directly
		}

		GinkgoWriter.Printf("✅ Full audit chain validated: Controller → BufferedStore → DataStorage → PostgreSQL\n")
	})
})

// queryAuditEventCount queries Data Storage API for audit event count
// Uses Data Storage API response format: { "data": [...], "pagination": { "total": N, ... } }
func queryAuditEventCount(baseURL, correlationID, eventType string) int {
	url := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s", baseURL, correlationID)
	if eventType != "" {
		url += "&event_type=" + eventType
	}

	resp, err := http.Get(url)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0
	}

	// Data Storage API returns: { "data": [...], "pagination": { "total": N, ... } }
	var result struct {
		Data       []interface{} `json:"data"`
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0
	}

	return result.Pagination.Total
}

// apiAuditEvent mirrors the Data Storage API response format for audit events
// This is needed because API returns event_data as JSON object, not []byte
type apiAuditEvent struct {
	EventID           string          `json:"event_id"`
	EventVersion      string          `json:"event_version"`
	EventTimestamp    string          `json:"event_timestamp"`
	EventType         string          `json:"event_type"`
	EventCategory     string          `json:"event_category"`
	EventAction       string          `json:"event_action"`
	EventOutcome      string          `json:"event_outcome"`
	CorrelationID     string          `json:"correlation_id"`
	ResourceType      string          `json:"resource_type"`
	ResourceID        string          `json:"resource_id"`
	ResourceNamespace string          `json:"resource_namespace"`
	ClusterID         string          `json:"cluster_id"`
	ActorType         string          `json:"actor_type"`
	ActorID           string          `json:"actor_id"`
	Service           string          `json:"service"`
	RetentionDays     int             `json:"retention_days"`
	IsSensitive       bool            `json:"is_sensitive"`
	EventData         json.RawMessage `json:"event_data"`
}

// queryAuditEvents queries Data Storage API for audit events
// Uses Data Storage API response format: { "data": [...], "pagination": { ... } }
func queryAuditEvents(baseURL, correlationID string) []audit.AuditEvent {
	url := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s", baseURL, correlationID)

	resp, err := http.Get(url)
	if err != nil {
		GinkgoWriter.Printf("queryAuditEvents: HTTP request failed: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		GinkgoWriter.Printf("queryAuditEvents: Non-OK status: %d\n", resp.StatusCode)
		return nil
	}

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		GinkgoWriter.Printf("queryAuditEvents: Failed to read body: %v\n", err)
		return nil
	}

	// Data Storage API returns event_data as JSON object
	var result struct {
		Data []apiAuditEvent `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		GinkgoWriter.Printf("queryAuditEvents: JSON unmarshal failed: %v\n", err)
		return nil
	}

	// Convert to []audit.AuditEvent
	// Note: Some fields have different names between API response and audit.AuditEvent
	events := make([]audit.AuditEvent, len(result.Data))
	for i, d := range result.Data {
		// Parse timestamp string to time.Time
		timestamp, _ := time.Parse(time.RFC3339, d.EventTimestamp)

		events[i] = audit.AuditEvent{
			EventVersion:   d.EventVersion,
			EventTimestamp: timestamp,
			EventType:      d.EventType,
			EventCategory:  d.EventCategory,
			EventAction:    d.EventAction,
			EventOutcome:   d.EventOutcome,
			CorrelationID:  d.CorrelationID,
			ResourceType:   d.ResourceType,
			ResourceID:     d.ResourceID,
			ActorType:      d.ActorType,
			ActorID:        d.ActorID,
			RetentionDays:  d.RetentionDays,
			IsSensitive:    d.IsSensitive,
			EventData:      d.EventData,
		}
	}

	GinkgoWriter.Printf("queryAuditEvents: Parsed %d events\n", len(events))
	return events
}
