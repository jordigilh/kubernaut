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
	"fmt"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/shared/validators"
)

// ========================================
// CONTROLLER AUDIT EMISSION TESTS
// ========================================
//
// Defense-in-Depth Testing Strategy (BR-NOT-062, BR-NOT-063, BR-NOT-064):
//
// Layer 1 (Unit): pkg/audit/* - Audit library functions
// Layer 2 (Unit): internal/controller/notification/audit.go - Audit helpers
// Layer 3 (Integration): audit_integration_test.go - AuditStore → DataStorage → PostgreSQL
// Layer 4 (Integration): THIS FILE - Controller → Audit emission at lifecycle points
//
// These tests verify the CONTROLLER emits audit events at correct lifecycle points:
// - Message sent → notification.message.sent event created
// - Message failed → notification.message.failed event created with error details
// - Correct correlation_id for workflow tracing
// - Per-channel audit events for multi-channel notifications
//
// ========================================

var _ = Describe("Controller Audit Event Emission (Defense-in-Depth Layer 4)", func() {
	var (
		dsClient       *ogenclient.Client
		dataStorageURL string
		queryCtx       context.Context
	)

	// Setup Data Storage REST API client for querying audit events
	// Per DD-AUDIT-003 mandate: Integration tests MUST use REAL Data Storage service
	BeforeEach(func() {
		queryCtx = context.Background()

		// Get Data Storage URL from environment or use NT integration port
		// MUST match the port in suite_test.go (line 252)
		dataStorageURL = os.Getenv("DATA_STORAGE_URL")
		if dataStorageURL == "" {
			dataStorageURL = "http://127.0.0.1:18096" // NT integration port (IPv4 explicit, matches suite_test.go)
		}

		// Create REST API client for querying audit events
		var err error
		dsClient, err = ogenclient.NewClient(dataStorageURL)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Data Storage REST API client")
	})

	// Helper function to query audit events from Data Storage REST API
	// Note: OpenAPI spec doesn't have resource_id query param, so we filter client-side
	queryAuditEvents := func(eventType, resourceID string) []ogenclient.AuditEvent {
		params := ogenclient.QueryAuditEventsParams{
			EventType:     ogenclient.NewOptString(eventType),
			EventCategory: ogenclient.NewOptString("notification"),
		}
		resp, err := dsClient.QueryAuditEvents(queryCtx, params)
		if err != nil || resp.Data == nil {
			return nil
		}

		// Client-side filtering by resource_id (OpenAPI spec gap)
		var filtered []ogenclient.AuditEvent
		for _, event := range resp.Data {
			if event.ResourceID.IsSet() && event.ResourceID.Value == resourceID {
				filtered = append(filtered, event)
			}
		}
		return filtered
	}

	// ========================================
	// TEST 1: Successful delivery emits audit event
	// BR-NOT-062: Unified audit table integration
	// ========================================
	Context("BR-NOT-062: Audit on Successful Delivery", func() {
		It("should emit notification.message.sent when Console delivery succeeds", func() {
			// Create unique notification name for this test
			testID := fmt.Sprintf("audit-sent-%d", time.Now().UnixNano())
			notificationName := fmt.Sprintf("audit-console-success-%s", testID[:8])

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notificationName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  "Audit Emission Test - Console Success",
					Body:     "Testing controller emits audit on successful console delivery",
					Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole},
					// No Metadata - let correlation ID fallback to notification.UID
				},
			}

			// Create the notification
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			// Wait for notification to be sent
			Eventually(func() notificationv1alpha1.NotificationPhase {
				var n notificationv1alpha1.NotificationRequest
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: testNamespace}, &n); err != nil {
					return ""
				}
				return n.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

		// DEFENSE-IN-DEPTH VERIFICATION: Query REAL Data Storage for sent event
		// Per DD-AUDIT-003: Integration tests MUST use real Data Storage service (no mocks)
		var sentEvent *ogenclient.AuditEvent
		Eventually(func() bool {
			// Flush audit buffer on each retry to ensure events are written to DataStorage
			_ = realAuditStore.Flush(queryCtx)

			events := queryAuditEvents("notification.message.sent", notificationName)
			if len(events) > 0 {
				sentEvent = &events[0]
				return true
			}
			return false
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"Controller should emit notification.message.sent audit event to Data Storage")

			Expect(sentEvent).ToNot(BeNil(), "Controller must emit 'notification.message.sent' event (DD-AUDIT-003)")

			// ========================================
			// SERVICE_MATURITY_REQUIREMENTS v1.2.0 (P0 - MANDATORY)
			// Use validators.ValidateAuditEvent for structured validation
			// ========================================
			validators.ValidateAuditEvent(*sentEvent, validators.ExpectedAuditEvent{
				EventType:     "notification.message.sent",
				EventCategory: ogenclient.AuditEventEventCategoryNotification,
				EventAction:   "sent",
				EventOutcome:  validators.EventOutcomePtr(ogenclient.AuditEventEventOutcomeSuccess),
				CorrelationID: string(notification.UID),
			})

			// Cleanup
			Expect(k8sClient.Delete(ctx, notification)).To(Succeed())
		})
	})

	// ========================================
	// TEST 2: Slack delivery emits audit event
	// BR-NOT-062: Unified audit table integration
	// ========================================
	Context("BR-NOT-062: Audit on Slack Delivery", func() {
		It("should emit notification.message.sent when Slack delivery succeeds", func() {
			// Create unique notification name
			testID := fmt.Sprintf("audit-slack-%d", time.Now().UnixNano())
			notificationName := fmt.Sprintf("audit-slack-success-%s", testID[:8])

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notificationName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  "Audit Emission Test - Slack Success",
					Body:     "Testing controller emits audit on successful Slack delivery",
					Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelSlack},
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test-channel"},
					},
					Metadata: map[string]string{
						"remediationRequestName": testID,
					},
				},
			}

			// Create the notification
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			// Wait for notification to be sent (Slack delivery to mock server)
			Eventually(func() notificationv1alpha1.NotificationPhase {
				var n notificationv1alpha1.NotificationRequest
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: testNamespace}, &n); err != nil {
					return ""
				}
				return n.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

		// DEFENSE-IN-DEPTH VERIFICATION: Query REAL Data Storage for sent event
		var slackEvent *ogenclient.AuditEvent
		Eventually(func() bool {
			// Flush audit buffer on each retry to ensure events are written to DataStorage
			_ = realAuditStore.Flush(queryCtx)

			events := queryAuditEvents("notification.message.sent", notificationName)
			if len(events) > 0 {
				slackEvent = &events[0]
				return true
			}
			return false
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"Controller should emit audit event for Slack delivery to Data Storage")

			Expect(slackEvent).ToNot(BeNil(), "Controller must emit audit event for Slack delivery")

			// SERVICE_MATURITY_REQUIREMENTS v1.2.0 (P0): Use testutil validator
			validators.ValidateAuditEvent(*slackEvent, validators.ExpectedAuditEvent{
				EventType:     "notification.message.sent",
				EventCategory: ogenclient.AuditEventEventCategoryNotification,
				EventAction:   "sent",
				EventOutcome:  validators.EventOutcomePtr(ogenclient.AuditEventEventOutcomeSuccess),
			})

			// Cleanup
			Expect(k8sClient.Delete(ctx, notification)).To(Succeed())
		})
	})

	// ========================================
	// TEST 3: Correlation ID propagation
	// BR-NOT-064: Workflow tracing via correlation_id
	// ========================================
	Context("BR-NOT-064: Correlation ID Propagation", func() {
		It("should include remediationRequestName as correlation_id in audit events", func() {
			// Create notification with specific remediation context
			testID := fmt.Sprintf("audit-corr-%d", time.Now().UnixNano())
			notificationName := fmt.Sprintf("audit-correlation-%s", testID[:8])
			remediationID := fmt.Sprintf("remediation-%s", testID[:12])

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notificationName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  "Correlation ID Test",
					Body:     "Testing correlation_id propagation to audit events",
					Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole},
					Metadata: map[string]string{
						"remediationRequestName": remediationID, // Used as correlation_id
					},
				},
			}

			// Create the notification
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			// Wait for notification to be processed
			Eventually(func() notificationv1alpha1.NotificationPhase {
				var n notificationv1alpha1.NotificationRequest
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: testNamespace}, &n); err != nil {
					return ""
				}
				return n.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			// DEFENSE-IN-DEPTH VERIFICATION: Query REAL Data Storage and check correlation_id matches remediationID
			var corrEvent *ogenclient.AuditEvent
			Eventually(func() bool {
				events := queryAuditEvents("notification.message.sent", notificationName)
				for _, e := range events {
					if e.CorrelationID == remediationID {
						corrEvent = &e
						return true
					}
				}
				return false
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Audit event correlation_id should match remediationID for workflow tracing (DD-AUDIT-003)")

			Expect(corrEvent).ToNot(BeNil())
			Expect(corrEvent.CorrelationID).To(Equal(remediationID), "Correlation ID must propagate to audit events")

			// Cleanup
			Expect(k8sClient.Delete(ctx, notification)).To(Succeed())
		})
	})

	// ========================================
	// TEST 4: Multi-channel audit events
	// BR-NOT-062: Per-channel audit tracking
	// ========================================
	Context("BR-NOT-062: Multi-Channel Audit Events", func() {
		It("should emit separate audit event for each channel delivery", func() {
			// Create notification with multiple channels
			testID := fmt.Sprintf("audit-multi-%d", time.Now().UnixNano())
			notificationName := fmt.Sprintf("audit-multichannel-%s", testID[:8])

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notificationName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  "Multi-Channel Audit Test",
					Body:     "Testing audit events for multiple channels",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
						notificationv1alpha1.ChannelSlack,
					},
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test-channel"},
					},
					Metadata: map[string]string{
						"remediationRequestName": testID,
					},
				},
			}

			// Create the notification
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			// Wait for notification to be processed
			Eventually(func() notificationv1alpha1.NotificationPhase {
				var n notificationv1alpha1.NotificationRequest
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: testNamespace}, &n); err != nil {
					return ""
				}
				return n.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

		// DEFENSE-IN-DEPTH VERIFICATION: Query REAL Data Storage for events from each channel
		Eventually(func() int {
			// Flush audit buffer on each retry to ensure events are written to DataStorage
			_ = realAuditStore.Flush(queryCtx)

			events := queryAuditEvents("notification.message.sent", notificationName)
			return len(events)
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(2),
			"Controller should emit exactly 2 audit events (1 per channel: Console + Slack) to Data Storage (DD-AUDIT-003, DD-TESTING-001)")

			// Cleanup
			Expect(k8sClient.Delete(ctx, notification)).To(Succeed())
		})
	})

	// ========================================
	// TEST 6: ADR-034 compliance in emitted events
	// ========================================
	Context("ADR-034: Field Compliance in Controller Events", func() {
		It("should emit events with all ADR-034 required fields", func() {
			testID := fmt.Sprintf("audit-adr034-%d", time.Now().UnixNano())
			notificationName := fmt.Sprintf("audit-adr034-%s", testID[:8])

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notificationName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  "ADR-034 Compliance Test",
					Body:     "Testing ADR-034 field compliance",
					Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole},
					Metadata: map[string]string{
						"remediationRequestName": testID,
					},
				},
			}

			// Create the notification
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			// Wait for notification to be sent
			Eventually(func() notificationv1alpha1.NotificationPhase {
				var n notificationv1alpha1.NotificationRequest
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: testNamespace}, &n); err != nil {
					return ""
				}
				return n.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

		// DEFENSE-IN-DEPTH VERIFICATION: Query REAL Data Storage and check ADR-034 fields
		var validEvent *ogenclient.AuditEvent
		Eventually(func() bool {
			// Flush audit buffer on each retry to ensure events are written to DataStorage
			_ = realAuditStore.Flush(queryCtx)

			events := queryAuditEvents("notification.message.sent", notificationName)
			for _, e := range events {
				// Verify all ADR-034 required fields (OptString types)
				if string(e.EventCategory) == "notification" &&
					e.EventAction == "sent" &&
					string(e.EventOutcome) == "success" &&
					e.ActorType.IsSet() && e.ActorType.Value == "service" &&
					e.ActorID.IsSet() && e.ActorID.Value == "notification-controller" &&
					e.ResourceType.IsSet() && e.ResourceType.Value == "NotificationRequest" &&
					e.ResourceID.IsSet() && e.ResourceID.Value == notificationName {
					validEvent = &e
					return true
				}
			}
			return false
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"Audit event should have all ADR-034 required fields populated correctly (DD-AUDIT-003)")

			Expect(validEvent).ToNot(BeNil(), "Must find valid ADR-034 compliant event in Data Storage")

			// Cleanup
			Expect(k8sClient.Delete(ctx, notification)).To(Succeed())
		})
	})

	// ========================================
	// TEST 7: Failed delivery emits audit event with error details
	// BR-NOT-062: Unified audit table integration
	// ========================================
	Context("BR-NOT-062: Audit on Failed Delivery", func() {
		It("should emit notification.message.failed when Slack delivery fails", func() {
			// Configure mock Slack webhook to fail (triggers delivery failure)
			ConfigureFailureMode("always", 0, http.StatusServiceUnavailable)
			defer ConfigureFailureMode("none", 0, 0) // Reset after test

			// Create notification with Slack channel to trigger failure
			testID := fmt.Sprintf("audit-failed-%d", time.Now().UnixNano())
			notificationName := fmt.Sprintf("audit-slack-failure-%s", testID[:8])

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notificationName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  "Audit Emission Test - Slack Failure",
					Body:     "Testing controller emits audit on failed Slack delivery",
					Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelSlack},
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test-failure"},
					},
					Metadata: map[string]string{
						"remediationRequestName": testID,
					},
					// Add retry policy with shorter backoff for test completion
					// Default 30s initial backoff would cause test timeout
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           3,
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
			}

			// Create the notification
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			// Wait for notification to reach Failed phase
			// (Slack delivery will fail due to mock returning 503)
			Eventually(func() notificationv1alpha1.NotificationPhase {
				var n notificationv1alpha1.NotificationRequest
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: testNamespace}, &n); err != nil {
					return ""
				}
				return n.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(
				Equal(notificationv1alpha1.NotificationPhaseFailed),
				"Notification should reach Failed phase due to mock Slack webhook failure (503)")

			// DEFENSE-IN-DEPTH VERIFICATION: Query REAL Data Storage for failed event
			var failedEvent *ogenclient.AuditEvent
			Eventually(func() bool {
				events := queryAuditEvents("notification.message.failed", notificationName)
				if len(events) > 0 {
					failedEvent = &events[0]
					return true
				}
				return false
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Controller should emit notification.message.failed audit event when delivery fails (DD-AUDIT-003)")

			Expect(failedEvent).ToNot(BeNil(), "Controller must emit 'notification.message.failed' event")

			// SERVICE_MATURITY_REQUIREMENTS v1.2.0 (P0): Use testutil validator
			validators.ValidateAuditEvent(*failedEvent, validators.ExpectedAuditEvent{
				EventType:     "notification.message.failed",
				EventCategory: ogenclient.AuditEventEventCategoryNotification,
				EventAction:   "sent", // Action was "sent" (attempted), outcome is "failure"
				EventOutcome:  validators.EventOutcomePtr(ogenclient.AuditEventEventOutcomeFailure),
			})

			// Verify error details are included in event_data
			// Per DD-AUDIT-004: Type-safe audit events with structured payloads
			Expect(failedEvent.EventData).ToNot(BeNil(), "Failed event must include error details in event_data")

			// Cleanup
			Expect(k8sClient.Delete(ctx, notification)).To(Succeed())
		})
	})

	// ========================================
	// TEST 8: Escalated notification emits audit event
	// BR-NOT-062: Unified audit table integration
	// ========================================
	// NOTE: Escalation is a V2.0 roadmap feature not yet implemented
	// Test removed per "NO SKIPPED TESTS" rule - will be added when escalation is implemented
	// Related audit function exists but is never called: CreateMessageEscalatedEvent()
})
