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

// DD-EVENT-001: Notification Controller K8s Event Observability Integration Tests
//
// BR-NT-095: K8s Event Observability business requirement
//
// These tests validate event emission in the context of the envtest framework
// with real EventRecorder (k8sManager.GetEventRecorderFor). They use the
// pattern: create CR → wait for target phase → list corev1.Events filtered
// by involvedObject.name → assert expected event reasons.
//
// IMPORTANT: These tests require the full integration environment (CRDs,
// DataStorage, mock Slack, etc.) to run. Structure compiles; execution depends on
// `make test-integration-notification`.
package notification

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// listEventsForObject returns corev1.Events for the given object name in the namespace,
// sorted by FirstTimestamp for deterministic ordering.
func listEventsForObject(ctx context.Context, c client.Client, objectName, namespace string) []corev1.Event {
	eventList := &corev1.EventList{}
	_ = c.List(ctx, eventList, client.InNamespace(namespace))
	var filtered []corev1.Event
	for _, evt := range eventList.Items {
		if evt.InvolvedObject.Name == objectName {
			filtered = append(filtered, evt)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].FirstTimestamp.Before(&filtered[j].FirstTimestamp)
	})
	return filtered
}

func containsEventReason(reasons []string, reason string) bool {
	for _, r := range reasons {
		if r == reason {
			return true
		}
	}
	return false
}

func eventReasons(evts []corev1.Event) []string {
	reasons := make([]string, len(evts))
	for i, e := range evts {
		reasons[i] = e.Reason
	}
	return reasons
}

var _ = Describe("Notification K8s Event Observability (DD-EVENT-001, BR-NT-095)", Label("integration", "events"), func() {
	var uniqueSuffix string

	BeforeEach(func() {
		uniqueSuffix = fmt.Sprintf("%d", GinkgoRandomSeed())
		ConfigureFailureMode("none", 0, 0)
		resetSlackRequests()
	})

	Context("IT-NT-095-01: All channels succeed", func() {
		It("should emit ReconcileStarted, PhaseTransition (Pending→Sending), NotificationSent when all channels deliver", func() {
			notifName := fmt.Sprintf("events-all-succeed-%s", uniqueSuffix)
			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "IT-NT-095-01: All Channels Succeed",
					Body:     "Event trail validation - all succeed",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for Phase=Sent")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"All channels should deliver successfully")

			By("Listing events and asserting ReconcileStarted, PhaseTransition, NotificationSent")
			var evts []corev1.Event
			Eventually(func() bool {
				evts = listEventsForObject(ctx, k8sClient, notifName, testNamespace)
				reasons := eventReasons(evts)
				return containsEventReason(reasons, events.EventReasonReconcileStarted) &&
					containsEventReason(reasons, events.EventReasonPhaseTransition) &&
					containsEventReason(reasons, events.EventReasonNotificationSent)
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			reasons := eventReasons(evts)
			Expect(containsEventReason(reasons, events.EventReasonReconcileStarted)).To(BeTrue())
			Expect(containsEventReason(reasons, events.EventReasonPhaseTransition)).To(BeTrue())
			Expect(containsEventReason(reasons, events.EventReasonNotificationSent)).To(BeTrue())

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("IT-NT-095-02: Partial success + retry exhaustion", func() {
		It("should emit ReconcileStarted, PhaseTransition, NotificationRetrying (at least once), NotificationPartiallySent", func() {
			notifName := fmt.Sprintf("events-partial-retry-%s", uniqueSuffix)
			ConfigureFailureMode("always", 0, 503) // Slack always fails (retryable 503)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "IT-NT-095-02: Partial Success + Retry Exhaustion",
					Body:     "Event trail validation - partial with retries",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
						notificationv1alpha1.ChannelConsole,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           3,
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
			}

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for Phase=PartiallySent")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 25*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhasePartiallySent),
				"Console succeeds, Slack fails after retry exhaustion")

			By("Listing events and asserting ReconcileStarted, PhaseTransition, NotificationRetrying, NotificationPartiallySent")
			var evts []corev1.Event
			Eventually(func() bool {
				evts = listEventsForObject(ctx, k8sClient, notifName, testNamespace)
				reasons := eventReasons(evts)
				return containsEventReason(reasons, events.EventReasonReconcileStarted) &&
					containsEventReason(reasons, events.EventReasonPhaseTransition) &&
					containsEventReason(reasons, events.EventReasonNotificationRetrying) &&
					containsEventReason(reasons, events.EventReasonNotificationPartiallySent)
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			reasons := eventReasons(evts)
			Expect(containsEventReason(reasons, events.EventReasonReconcileStarted)).To(BeTrue())
			Expect(containsEventReason(reasons, events.EventReasonPhaseTransition)).To(BeTrue())
			Expect(containsEventReason(reasons, events.EventReasonNotificationRetrying)).To(BeTrue())
			Expect(containsEventReason(reasons, events.EventReasonNotificationPartiallySent)).To(BeTrue())

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("IT-NT-095-03: All channels fail permanently", func() {
		It("should emit ReconcileStarted, PhaseTransition, NotificationFailed when all channels fail permanently", func() {
			notifName := fmt.Sprintf("events-all-fail-%s", uniqueSuffix)
			ConfigureFailureMode("always", 0, http.StatusBadRequest) // Permanent 4xx, no retries

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "IT-NT-095-03: All Channels Fail",
					Body:     "Event trail validation - all fail permanently",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           3,
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
			}

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for Phase=Failed")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseFailed),
				"All channels should fail permanently without retries")

			By("Listing events and asserting ReconcileStarted, PhaseTransition, NotificationFailed")
			var evts []corev1.Event
			Eventually(func() bool {
				evts = listEventsForObject(ctx, k8sClient, notifName, testNamespace)
				reasons := eventReasons(evts)
				return containsEventReason(reasons, events.EventReasonReconcileStarted) &&
					containsEventReason(reasons, events.EventReasonPhaseTransition) &&
					containsEventReason(reasons, events.EventReasonNotificationFailed)
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			reasons := eventReasons(evts)
			Expect(containsEventReason(reasons, events.EventReasonReconcileStarted)).To(BeTrue())
			Expect(containsEventReason(reasons, events.EventReasonPhaseTransition)).To(BeTrue())
			Expect(containsEventReason(reasons, events.EventReasonNotificationFailed)).To(BeTrue())

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
