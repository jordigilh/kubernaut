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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
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
	// Clear audit store before each test for isolation
	BeforeEach(func() {
		if testAuditStore != nil {
			testAuditStore.Clear()
		}
	})

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
					Name:      notificationName,
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  "Audit Emission Test - Console Success",
					Body:     "Testing controller emits audit on successful console delivery",
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
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: "default"}, &n); err != nil {
					return ""
				}
				return n.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			// DEFENSE-IN-DEPTH VERIFICATION: Check audit store for sent event
			Eventually(func() int {
				return len(testAuditStore.GetEventsByType("notification.message.sent"))
			}, 5*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1),
				"Controller should emit notification.message.sent audit event")

			// Verify event details
			events := testAuditStore.GetEventsByResourceID(notificationName)
			Expect(events).ToNot(BeEmpty(), "Should have audit events for this notification")

			// Find the sent event
			var sentEvent *struct {
				EventType    string
				EventOutcome string
				Channel      string
			}
			for _, e := range events {
				if e.EventType == "notification.message.sent" {
					sentEvent = &struct {
						EventType    string
						EventOutcome string
						Channel      string
					}{
						EventType:    e.EventType,
						EventOutcome: e.EventOutcome,
					}
					break
				}
			}
			Expect(sentEvent).ToNot(BeNil(), "Should have notification.message.sent event")
			Expect(sentEvent.EventOutcome).To(Equal("success"))

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
					Name:      notificationName,
					Namespace: "default",
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
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: "default"}, &n); err != nil {
					return ""
				}
				return n.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			// DEFENSE-IN-DEPTH VERIFICATION: Check audit store
			Eventually(func() int {
				return len(testAuditStore.GetEventsByResourceID(notificationName))
			}, 5*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1),
				"Controller should emit audit event for Slack delivery")

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
					Name:      notificationName,
					Namespace: "default",
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
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: "default"}, &n); err != nil {
					return ""
				}
				return n.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			// DEFENSE-IN-DEPTH VERIFICATION: Check correlation_id matches remediationID
			Eventually(func() string {
				events := testAuditStore.GetEventsByResourceID(notificationName)
				for _, e := range events {
					if e.CorrelationID == remediationID {
						return e.CorrelationID
					}
				}
				return ""
			}, 5*time.Second, 200*time.Millisecond).Should(Equal(remediationID),
				"Audit event correlation_id should match remediationID for workflow tracing")

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
					Name:      notificationName,
					Namespace: "default",
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
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: "default"}, &n); err != nil {
					return ""
				}
				return n.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			// DEFENSE-IN-DEPTH VERIFICATION: Should have audit events for each channel
			Eventually(func() int {
				return len(testAuditStore.GetEventsByResourceID(notificationName))
			}, 5*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 2),
				"Controller should emit audit event for each channel (Console + Slack)")

			// Cleanup
			Expect(k8sClient.Delete(ctx, notification)).To(Succeed())
		})
	})

	// ========================================
	// TEST 5: ADR-034 compliance in emitted events
	// ========================================
	Context("ADR-034: Field Compliance in Controller Events", func() {
		It("should emit events with all ADR-034 required fields", func() {
			testID := fmt.Sprintf("audit-adr034-%d", time.Now().UnixNano())
			notificationName := fmt.Sprintf("audit-adr034-%s", testID[:8])

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notificationName,
					Namespace: "default",
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
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: "default"}, &n); err != nil {
					return ""
				}
				return n.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			// DEFENSE-IN-DEPTH VERIFICATION: Check ADR-034 fields
			var validEvent bool
			Eventually(func() bool {
				events := testAuditStore.GetEventsByResourceID(notificationName)
				for _, e := range events {
					if e.EventType == "notification.message.sent" {
						// Verify all ADR-034 required fields
						validEvent = true
						validEvent = validEvent && e.EventCategory == "notification"
						validEvent = validEvent && e.EventAction == "sent"
						validEvent = validEvent && e.EventOutcome == "success"
						validEvent = validEvent && e.ActorType == "service"
						validEvent = validEvent && e.ActorID == "notification-controller"
						validEvent = validEvent && e.ResourceType == "NotificationRequest"
						validEvent = validEvent && e.ResourceID == notificationName
						validEvent = validEvent && e.RetentionDays == 2555
						return validEvent
					}
				}
				return false
			}, 5*time.Second, 200*time.Millisecond).Should(BeTrue(),
				"Audit event should have all ADR-034 required fields populated correctly")

			// Cleanup
			Expect(k8sClient.Delete(ctx, notification)).To(Succeed())
		})
	})
})

