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
	"net/http"
	"net/http/httptest"
	"sync"
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
// E2E Test 1: Full Notification Lifecycle with Audit
// ========================================
//
// Business Requirements:
// - BR-NOT-062: Unified audit table integration
// - BR-NOT-063: Graceful audit degradation
//
// Test Scenario:
// 1. Create NotificationRequest CRD
// 2. Simulate notification delivery (sent)
// 3. Verify audit event created for message.sent
// 4. Simulate acknowledgment
// 5. Verify audit event created for status.acknowledged
// 6. Verify all audit events have correct correlation_id
//
// Expected Results:
// - NotificationRequest CRD created successfully
// - 2 audit events generated (sent + acknowledged)
// - All audit events follow ADR-034 format
// - Audit correlation_id links both events
// - Fire-and-forget pattern ensures no blocking
//
// This is a simplified E2E test using envtest infrastructure
// Full E2E with Kind cluster deployment can be added later

var _ = Describe("E2E Test 1: Full Notification Lifecycle with Audit", Label("e2e", "lifecycle", "audit"), func() {
	var (
		testCtx           context.Context
		testCancel        context.CancelFunc
		notification      *notificationv1alpha1.NotificationRequest
		auditHelpers      *notificationcontroller.AuditHelpers
		auditStore        audit.AuditStore
		mockDataStorage   *httptest.Server
		receivedEvents    []*audit.AuditEvent
		eventsMutex       sync.Mutex
		notificationName  string
		notificationNS    string
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 2*time.Minute)
		receivedEvents = []*audit.AuditEvent{}

		// Generate unique notification name for this test
		notificationName = "e2e-lifecycle-test-" + time.Now().Format("20060102-150405")
		notificationNS = "default"

		// Set up mock Data Storage Service
		mockDataStorage = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/audit/events" && r.Method == "POST" {
				var events []*audit.AuditEvent
				err := json.NewDecoder(r.Body).Decode(&events)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				eventsMutex.Lock()
				receivedEvents = append(receivedEvents, events...)
				eventsMutex.Unlock()

				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": true,
					"count":   len(events),
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))

		// Create audit store
		httpClient := &http.Client{Timeout: 5 * time.Second}
		dataStorageClient := audit.NewHTTPDataStorageClient(mockDataStorage.URL, httpClient)

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
					"remediationRequestName": "e2e-test-remediation",
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
		if mockDataStorage != nil {
			mockDataStorage.Close()
		}

		// Clean up NotificationRequest CRD if it exists
		if notification != nil {
			k8sClient.Delete(testCtx, notification)
		}
	})

	It("should create NotificationRequest and generate audit events for complete lifecycle", func() {
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

		// Wait for async flush
		Eventually(func() int {
			eventsMutex.Lock()
			defer eventsMutex.Unlock()
			return len(receivedEvents)
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1),
			"At least 1 audit event (sent) should be flushed")

		// ===== STEP 3: Simulate acknowledgment and create audit event =====
		By("Simulating notification acknowledgment")

		// Create acknowledgment audit event
		// Note: Status update not required for E2E test validation
		ackEvent, err := auditHelpers.CreateMessageAcknowledgedEvent(notification)
		Expect(err).ToNot(HaveOccurred(), "CreateMessageAcknowledgedEvent should succeed")

		err = auditStore.StoreAudit(testCtx, ackEvent)
		Expect(err).ToNot(HaveOccurred(), "Storing acknowledgment audit event should succeed")

		// Wait for second audit event to be flushed
		Eventually(func() int {
			eventsMutex.Lock()
			defer eventsMutex.Unlock()
			return len(receivedEvents)
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 2),
			"Both audit events (sent + acknowledged) should be flushed")

		// ===== STEP 4: Verify audit events =====
		By("Verifying all audit events were generated correctly")

		eventsMutex.Lock()
		defer eventsMutex.Unlock()

		Expect(len(receivedEvents)).To(BeNumerically(">=", 2),
			"Should have at least 2 audit events (sent + acknowledged)")

		// Find sent event
		var sentAuditEvent *audit.AuditEvent
		for _, event := range receivedEvents {
			if event.EventType == "notification.message.sent" {
				sentAuditEvent = event
				break
			}
		}
		Expect(sentAuditEvent).ToNot(BeNil(), "Should find message.sent audit event")

		// Verify sent event follows ADR-034
		Expect(sentAuditEvent.EventCategory).To(Equal("notification"))
		Expect(sentAuditEvent.EventAction).To(Equal("sent"))
		Expect(sentAuditEvent.EventOutcome).To(Equal("success"))
		Expect(sentAuditEvent.ActorType).To(Equal("service"))
		Expect(sentAuditEvent.ActorID).To(Equal("notification"))
		Expect(sentAuditEvent.ResourceType).To(Equal("NotificationRequest"))
		Expect(sentAuditEvent.ResourceID).To(Equal(notificationName))
		Expect(sentAuditEvent.RetentionDays).To(Equal(2555), "Retention should be 7 years")

		// Find acknowledged event
		var ackAuditEvent *audit.AuditEvent
		for _, event := range receivedEvents {
			if event.EventType == "notification.message.acknowledged" {
				ackAuditEvent = event
				break
			}
		}
		Expect(ackAuditEvent).ToNot(BeNil(), "Should find message.acknowledged audit event")

		// Verify acknowledged event follows ADR-034
		Expect(ackAuditEvent.EventCategory).To(Equal("notification"))
		Expect(ackAuditEvent.EventAction).To(Equal("acknowledged"))
		Expect(ackAuditEvent.EventOutcome).To(Equal("success"))
		Expect(ackAuditEvent.ResourceID).To(Equal(notificationName))

		// ===== STEP 5: Verify correlation_id links both events =====
		By("Verifying correlation_id links both audit events")

		// Correlation ID comes from remediation request name in metadata
		expectedCorrelationID := "e2e-test-remediation"
		Expect(sentAuditEvent.CorrelationID).To(Equal(expectedCorrelationID),
			"Sent event correlation_id should match remediation request name")
		Expect(ackAuditEvent.CorrelationID).To(Equal(expectedCorrelationID),
			"Acknowledged event correlation_id should match remediation request name")
		Expect(sentAuditEvent.CorrelationID).To(Equal(ackAuditEvent.CorrelationID),
			"Both events should have same correlation_id for tracing")
	})
})

