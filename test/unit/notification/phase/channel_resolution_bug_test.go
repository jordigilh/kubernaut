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

package phase

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationphase "github.com/jordigilh/kubernaut/pkg/notification/phase"
)

var _ = Describe("#263: Channel Resolution Variable Shadowing Bug", func() {

	// ========================================
	// Bug: Routing resolves channels from rules (#260/#261). Due to Go variable
	// shadowing (:= inside an if block), the resolved channels never reach the
	// delivery orchestrator or phase transition logic. DetermineTransition receives
	// 0 channels, and 0 == 0 makes it return Sent/AllDeliveriesSucceeded.
	// ========================================

	Context("UT-NT-263-001: Zero channels must not produce false Sent", func() {
		It("should NOT return Sent when channels is empty but deliveries were attempted", func() {
			notification := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nr-completion-rr-test",
					Namespace: "kubernaut-system",
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeCompletion,
					Subject:  "Remediation Completed: KubePodCrashLooping",
					Body:     "Remediation completed successfully",
					Severity: "critical",
					Priority: notificationv1.NotificationPriorityLow,
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase:                notificationv1.NotificationPhaseSending,
					SuccessfulDeliveries: 0,
				},
			}

			deliveryResult := &notificationphase.DeliveryResult{
				ChannelResults: map[string]error{},
				FailureCount:   0,
			}

			channelStates := map[string]notificationphase.ChannelState{}

			decision := notificationphase.DetermineTransition(
				notification, nil, deliveryResult, channelStates, 5,
			)

			Expect(decision.NextPhase).ToNot(Equal(notificationphase.Sent),
				"#263: Zero channels must NOT produce Sent — this masks delivery failures. "+
					"Current bug: totalSuccessful(0) == totalChannels(0) returns true.")
			Expect(decision.Reason).ToNot(Equal("AllDeliveriesSucceeded"),
				"#263: 0 successful deliveries to 0 channels is NOT 'all deliveries succeeded'")
		})

		It("should return NoChannelsResolved when channels is empty", func() {
			notification := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nr-completion-rr-nochannels",
					Namespace: "kubernaut-system",
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeCompletion,
					Subject:  "Test: no channels resolved",
					Body:     "Should fail gracefully",
					Severity: "critical",
					Priority: notificationv1.NotificationPriorityLow,
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase: notificationv1.NotificationPhaseSending,
				},
			}

			deliveryResult := &notificationphase.DeliveryResult{
				ChannelResults: map[string]error{},
				FailureCount:   0,
			}

			decision := notificationphase.DetermineTransition(
				notification, []notificationv1.Channel{}, deliveryResult,
				map[string]notificationphase.ChannelState{}, 5,
			)

			Expect(decision.NextPhase).To(Equal(notificationphase.Failed),
				"#263: Empty channel list must transition to Failed, not Sent")
			Expect(decision.Reason).To(Equal("NoChannelsResolved"),
				"#263: Reason must indicate no channels were resolved for delivery")
			Expect(decision.IsTerminal).To(BeTrue())
			Expect(decision.IsPermanentFailure).To(BeTrue())
		})
	})

	Context("UT-NT-263-002: Routing-resolved channels must reach delivery and phase transition", func() {
		It("should correctly count channels resolved from routing", func() {
			channels := []notificationv1.Channel{
				notificationv1.Channel("slack:default-console"),
				notificationv1.Channel("console"),
			}

			notification := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nr-completion-rr-routing",
					Namespace: "kubernaut-system",
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeCompletion,
					Subject:  "Remediation Completed: KubePodCrashLooping",
					Body:     "AI diagnosed invalid nginx config and rolled back deployment",
					Severity: "critical",
					Priority: notificationv1.NotificationPriorityLow,
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase:                notificationv1.NotificationPhaseSending,
					SuccessfulDeliveries: 0,
				},
			}

			deliveryResult := &notificationphase.DeliveryResult{
				ChannelResults: map[string]error{
					"slack:default-console": nil,
					"console":               nil,
				},
				FailureCount: 0,
				DeliveryAttempts: []notificationv1.DeliveryAttempt{
					{Channel: "slack:default-console", Status: "success", Attempt: 1, Timestamp: metav1.Now()},
					{Channel: "console", Status: "success", Attempt: 1, Timestamp: metav1.Now()},
				},
			}

			channelStates := map[string]notificationphase.ChannelState{
				"slack:default-console": {AlreadySucceeded: true, AttemptCount: 1},
				"console":               {AlreadySucceeded: true, AttemptCount: 1},
			}

			decision := notificationphase.DetermineTransition(
				notification, channels, deliveryResult, channelStates, 5,
			)

			Expect(decision.NextPhase).To(Equal(notificationphase.Sent),
				"Both routing-resolved channels succeeded → Sent")
			Expect(decision.Reason).To(Equal("AllDeliveriesSucceeded"))
			Expect(decision.Message).To(ContainSubstring("2 channel(s)"),
				"Message must reflect the 2 routing-resolved channels, not 0")
			Expect(decision.IsTerminal).To(BeTrue())
		})

		It("should transition to Retrying when Slack fails but console succeeds", func() {
			channels := []notificationv1.Channel{
				notificationv1.Channel("slack:default-console"),
				notificationv1.Channel("console"),
			}

			notification := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nr-completion-rr-partial",
					Namespace: "kubernaut-system",
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeCompletion,
					Subject:  "Remediation Completed",
					Body:     "Testing partial delivery with routing-resolved channels",
					Severity: "critical",
					Priority: notificationv1.NotificationPriorityLow,
					RetryPolicy: &notificationv1.RetryPolicy{
						MaxAttempts:           5,
						InitialBackoffSeconds: 5,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase:                notificationv1.NotificationPhaseSending,
					SuccessfulDeliveries: 0,
				},
			}

			deliveryResult := &notificationphase.DeliveryResult{
				ChannelResults: map[string]error{
					"console":               nil,
					"slack:default-console": fmt.Errorf("webhook returned 403: invalid_token"),
				},
				FailureCount: 1,
				DeliveryAttempts: []notificationv1.DeliveryAttempt{
					{Channel: "console", Status: "success", Attempt: 1, Timestamp: metav1.Now()},
					{Channel: "slack:default-console", Status: "failed", Error: "webhook returned 403", Attempt: 1, Timestamp: metav1.Now()},
				},
			}

			channelStates := map[string]notificationphase.ChannelState{
				"console":               {AlreadySucceeded: true, AttemptCount: 1},
				"slack:default-console": {AlreadySucceeded: false, AttemptCount: 1},
			}

			decision := notificationphase.DetermineTransition(
				notification, channels, deliveryResult, channelStates, 5,
			)

			Expect(decision.NextPhase).To(Equal(notificationphase.Retrying),
				"Console succeeded, Slack failed with retries remaining → Retrying")
			Expect(decision.ShouldRequeue).To(BeTrue())
			Expect(decision.IsTerminal).To(BeFalse())
			Expect(decision.MaxFailedAttemptCount).To(Equal(1))
		})
	})
})
