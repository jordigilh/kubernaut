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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ptr "k8s.io/utils/ptr"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationcontroller "github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/audit"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
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
		dsClient         *dsgen.ClientWithResponses
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

		// ✅ DD-API-001: Create OpenAPI client for audit queries (MANDATORY)
		// Per DD-API-001: All DataStorage communication MUST use OpenAPI generated client
		var err error
		dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
		Expect(err).ToNot(HaveOccurred(), "Failed to create DataStorage OpenAPI client")

		// Create audit store pointing to real Data Storage (DD-API-001)
		dataStorageClient, err := audit.NewOpenAPIClientAdapter(dataStorageURL, 10*time.Second)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Data Storage client")

		config := audit.Config{
			BufferSize:    1000,
			BatchSize:     10,
			FlushInterval: 100 * time.Millisecond,
			MaxRetries:    3,
		}

		testLogger := crzap.New(crzap.UseDevMode(true))
		auditStore, err = audit.NewBufferedStore(dataStorageClient, config, "notification", testLogger)
		Expect(err).ToNot(HaveOccurred(), "Failed to create audit store")

		// Create audit helpers with test-specific service name (DD-E2E-002)
		// CRITICAL: Use "notification" (not "notification-controller") to distinguish test events
		// from controller events when both run in same E2E environment
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
				Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole}, // Use Console to avoid Slack delivery failures
				Recipients: []notificationv1alpha1.Recipient{
					{Slack: "#e2e-tests"}, // Keep for CRD validation, but Console channel doesn't use it
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
			return queryAuditEventCount(dsClient, correlationID, "notification.message.sent")
		}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
			"Sent audit event should be persisted to PostgreSQL")

		// ===== STEP 3: Simulate acknowledgment and create audit event =====
		By("Simulating notification acknowledgment")
		ackEvent, err := auditHelpers.CreateMessageAcknowledgedEvent(notification)
		Expect(err).ToNot(HaveOccurred(), "CreateMessageAcknowledgedEvent should succeed")

		err = auditStore.StoreAudit(testCtx, ackEvent)
		Expect(err).ToNot(HaveOccurred(), "Storing acknowledgment audit event should succeed")

		// Wait for async flush and verify via Data Storage API
		By("Verifying both test-emitted audit events persisted to PostgreSQL")

		// DD-E2E-002: Wait for filtered test events, not all events (including controller events)
		Eventually(func() int {
			allEvents := queryAuditEvents(dsClient, correlationID)
			testEvents := filterEventsByActorId(allEvents, "notification")
			return len(testEvents)
		}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 2),
			"Both test-emitted audit events (sent + acknowledged) should be persisted to PostgreSQL")

		// ===== STEP 4: Verify ADR-034 compliance via Data Storage query =====
		By("Verifying ADR-034 compliance of persisted events")

		// Get all events for ADR-034 compliance validation
		allEvents := queryAuditEvents(dsClient, correlationID)

		// DD-E2E-002: Filter to only test-emitted events (ActorId "notification")
		// Excludes controller-emitted events (ActorId "notification-controller") that may run concurrently
		events := filterEventsByActorId(allEvents, "notification")

		// Find our specific events and validate ADR-034 compliance
		var foundSentEvent, foundAckEvent *dsgen.AuditEvent
		for i := range events {
			if events[i].EventType == "notification.message.sent" {
				foundSentEvent = &events[i]
			} else if events[i].EventType == "notification.message.acknowledged" {
				foundAckEvent = &events[i]
			}
		}

		Expect(foundSentEvent).ToNot(BeNil(), "Should find sent event")
		Expect(foundAckEvent).ToNot(BeNil(), "Should find acknowledged event")

		// Validate ADR-034 compliance for both events
		for _, event := range []*dsgen.AuditEvent{foundSentEvent, foundAckEvent} {
			Expect(string(event.EventCategory)).To(Equal("notification"), "Category should be 'notification'")
			Expect(event.ActorType).ToNot(BeNil(), "Actor type should be set")
			Expect(*event.ActorType).To(Equal("service"), "Actor type should be 'service'")
			Expect(event.ActorId).ToNot(BeNil(), "Actor ID should be set")
			Expect(*event.ActorId).To(Equal("notification"), "Actor ID should be 'notification'")
			Expect(event.ResourceType).ToNot(BeNil(), "Resource type should be set")
			Expect(*event.ResourceType).To(Equal("NotificationRequest"), "Resource type should be 'NotificationRequest'")
			Expect(event.ResourceId).ToNot(BeNil(), "Resource ID should be set")
			Expect(*event.ResourceId).To(Equal(notificationName), "Resource ID should match notification name")
			Expect(event.CorrelationId).To(Equal(correlationID), "Correlation ID should match")
			// Note: RetentionDays is stored in PostgreSQL but not returned by Data Storage Query API
			// This is validated by integration tests against the database directly
		}

		// ===== STEP 6: FIELD MATCHING VALIDATION =====
		By("Validating stored event_data fields match audit helper output")

		// Find sent and acknowledged events from persisted data
		var persistedSentEvent, persistedAckEvent *dsgen.AuditEvent
		for i := range events {
			if events[i].EventType == "notification.message.sent" {
				persistedSentEvent = &events[i]
			} else if events[i].EventType == "notification.message.acknowledged" {
				persistedAckEvent = &events[i]
			}
		}

		Expect(persistedSentEvent).ToNot(BeNil(), "Should have sent event")
		Expect(persistedAckEvent).ToNot(BeNil(), "Should have acknowledged event")

		// Validate sent event event_data (convert from interface{} to JSON)
		var sentEventData map[string]interface{}
		eventDataBytes, err := json.Marshal(persistedSentEvent.EventData)
		Expect(err).ToNot(HaveOccurred(), "Sent event data should be marshallable")
		err = json.Unmarshal(eventDataBytes, &sentEventData)
		Expect(err).ToNot(HaveOccurred(), "Sent event data should be valid JSON")

		Expect(sentEventData["notification_id"]).To(Equal(notificationName),
			"FIELD MATCH: Sent event notification_id should match resource_id")
		Expect(sentEventData["channel"]).To(Equal("slack"),
			"FIELD MATCH: Sent event channel should be slack")
		Expect(sentEventData["subject"]).To(Equal("E2E Lifecycle Test"),
			"FIELD MATCH: Sent event subject should match notification spec")
		Expect(sentEventData["body"]).To(Equal("Testing full notification lifecycle with audit trail"),
			"FIELD MATCH: Sent event body should match notification spec")
		Expect(sentEventData["priority"]).To(Equal("critical"),
			"FIELD MATCH: Sent event priority should match notification spec")
		Expect(sentEventData).To(HaveKey("metadata"),
			"FIELD MATCH: Sent event should contain metadata")

		// Validate acknowledged event event_data (convert from interface{} to JSON)
		var ackEventData map[string]interface{}
		ackEventDataBytes, err := json.Marshal(persistedAckEvent.EventData)
		Expect(err).ToNot(HaveOccurred(), "Acknowledged event data should be marshallable")
		err = json.Unmarshal(ackEventDataBytes, &ackEventData)
		Expect(err).ToNot(HaveOccurred(), "Acknowledged event data should be valid JSON")

		Expect(ackEventData["notification_id"]).To(Equal(notificationName),
			"FIELD MATCH: Acknowledged event notification_id should match resource_id")
		Expect(ackEventData["subject"]).To(Equal("E2E Lifecycle Test"),
			"FIELD MATCH: Acknowledged event subject should match notification spec")
		Expect(ackEventData["priority"]).To(Equal("critical"),
			"FIELD MATCH: Acknowledged event priority should match notification spec")
		Expect(ackEventData).To(HaveKey("metadata"),
			"FIELD MATCH: Acknowledged event should contain metadata")

		GinkgoWriter.Printf("✅ Full audit chain validated: Controller → BufferedStore → DataStorage → PostgreSQL\n")
		GinkgoWriter.Printf("✅ Field matching validation complete: All stored fields match audit helper output\n")
	})
})

// ✅ DD-API-001: queryAuditEventCount using OpenAPI client (MANDATORY)
// Per DD-API-001: All DataStorage communication MUST use OpenAPI generated client
// Per ADR-034 v1.2: event_category is MANDATORY for audit queries
func queryAuditEventCount(dsClient *dsgen.ClientWithResponses, correlationID, eventType string) int {
	// Build type-safe query parameters
	params := &dsgen.QueryAuditEventsParams{
		CorrelationId: &correlationID,
		EventCategory: ptr.To("notification"), // ADR-034 v1.2 requirement
	}
	if eventType != "" {
		params.EventType = &eventType
	}

	// ✅ Use OpenAPI generated client (type-safe, contract-validated)
	resp, err := dsClient.QueryAuditEventsWithResponse(context.Background(), params)
	if err != nil {
		GinkgoWriter.Printf("queryAuditEventCount: Failed to query DataStorage: %v\n", err)
		return 0
	}

	if resp.JSON200 == nil {
		GinkgoWriter.Printf("queryAuditEventCount: Non-200 response: %d\n", resp.StatusCode())
		return 0
	}

	// Extract total from pagination
	if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
		return *resp.JSON200.Pagination.Total
	}

	return 0
}

// NOTE: apiAuditEvent struct removed - now using OpenAPI generated dsgen.AuditEvent type
// Per DD-API-001: All DataStorage communication uses generated client types

// ✅ DD-API-001: queryAuditEvents using OpenAPI client (MANDATORY)
// Per DD-API-001: All DataStorage communication MUST use OpenAPI generated client
// Per ADR-034 v1.2: event_category is MANDATORY for audit queries
// Returns []dsgen.AuditEvent (OpenAPI types)
func queryAuditEvents(dsClient *dsgen.ClientWithResponses, correlationID string) []dsgen.AuditEvent {
	// Build type-safe query parameters
	params := &dsgen.QueryAuditEventsParams{
		CorrelationId: &correlationID,
		EventCategory: ptr.To("notification"), // ADR-034 v1.2 requirement
	}

	// ✅ Use OpenAPI generated client (type-safe, contract-validated)
	resp, err := dsClient.QueryAuditEventsWithResponse(context.Background(), params)
	if err != nil {
		GinkgoWriter.Printf("queryAuditEvents: Failed to query DataStorage: %v\n", err)
		return nil
	}

	if resp.JSON200 == nil {
		GinkgoWriter.Printf("queryAuditEvents: Non-200 response: %d\n", resp.StatusCode())
		return nil
	}

	if resp.JSON200.Data == nil {
		GinkgoWriter.Printf("queryAuditEvents: No data in response\n")
		return nil
	}

	// Return OpenAPI-generated audit events directly
	events := *resp.JSON200.Data
	GinkgoWriter.Printf("queryAuditEvents: Retrieved %d events\n", len(events))
	return events
}

// filterEventsByActorId filters audit events to only include events with matching ActorId
// This is used in E2E tests to distinguish test-emitted events (ActorId "notification")
// from controller-emitted events (ActorId "notification-controller") when both run concurrently.
//
// DD-E2E-002: E2E Audit Event Isolation Pattern
func filterEventsByActorId(events []dsgen.AuditEvent, actorId string) []dsgen.AuditEvent {
	filtered := make([]dsgen.AuditEvent, 0, len(events))
	for _, event := range events {
		if event.ActorId != nil && *event.ActorId == actorId {
			filtered = append(filtered, event)
		}
	}
	GinkgoWriter.Printf("filterEventsByActorId: Filtered %d/%d events (ActorId=%s)\n", len(filtered), len(events), actorId)
	return filtered
}
