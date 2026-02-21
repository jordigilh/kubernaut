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
	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BR-NOT-065: Channel Routing Based on Spec Fields - Controller Integration
// Issue #91: Routing now uses spec fields instead of labels
var _ = Describe("Routing Controller Integration (BR-NOT-065)", func() {

	Describe("ResolveChannelsFromSpecFields", func() {

		var config *routing.Config

		BeforeEach(func() {
			configYAML := `
route:
  receiver: default-slack
  routes:
    - match:
        type: approval_required
        severity: critical
      receiver: pagerduty-critical
    - match:
        type: approval_required
      receiver: slack-approvals
    - match:
        type: failed
      receiver: pagerduty-oncall
    - match:
        type: completed
      receiver: slack-ops
receivers:
  - name: default-slack
    slackConfigs:
      - channel: '#kubernaut-alerts'
  - name: pagerduty-critical
    pagerdutyConfigs:
      - serviceKey: critical-key
  - name: slack-approvals
    slackConfigs:
      - channel: '#approvals'
  - name: pagerduty-oncall
    pagerdutyConfigs:
      - serviceKey: oncall-key
  - name: slack-ops
    slackConfigs:
      - channel: '#ops'
`
			var err error
			config, err = routing.ParseConfig([]byte(configYAML))
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when NotificationRequest has no spec.channels", func() {

			It("should resolve channels from spec fields for critical approval notification", func() {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-notification",
						Namespace: "kubernaut-system",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     "approval_required",
						Priority: notificationv1alpha1.NotificationPriorityCritical,
						Severity: "critical",
						Subject:  "Approval Required",
						Body:     "Test body",
						Metadata: map[string]string{
							"environment": "production",
						},
					},
				}

				channels := routing.ResolveChannelsForNotification(config, notification)
				Expect(channels).To(ContainElement("pagerduty"))
				Expect(channels).ToNot(ContainElement("slack"))
			})

			It("should resolve channels from spec fields for non-critical approval notification", func() {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-notification",
						Namespace: "kubernaut-system",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     "approval_required",
						Priority: notificationv1alpha1.NotificationPriorityHigh,
						Severity: "high",
						Subject:  "Approval Required",
						Body:     "Test body",
					},
				}

				channels := routing.ResolveChannelsForNotification(config, notification)
				Expect(channels).To(ContainElement("slack"))
			})

			It("should resolve channels from spec fields for failed notification", func() {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-notification",
						Namespace: "kubernaut-system",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     "failed",
						Priority: notificationv1alpha1.NotificationPriorityHigh,
						Subject:  "Remediation Failed",
						Body:     "Test body",
					},
				}

				channels := routing.ResolveChannelsForNotification(config, notification)
				Expect(channels).To(ContainElement("pagerduty"))
			})

			It("should fall back to default receiver when no spec fields match", func() {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-notification",
						Namespace: "kubernaut-system",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     "unknown-type",
						Priority: notificationv1alpha1.NotificationPriorityLow,
						Subject:  "Test",
						Body:     "Test body",
					},
				}

				channels := routing.ResolveChannelsForNotification(config, notification)
				Expect(channels).To(ContainElement("slack"))
			})

			It("should handle notification with no spec fields by using default receiver", func() {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-notification",
						Namespace: "kubernaut-system",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Priority: notificationv1alpha1.NotificationPriorityLow,
						Subject:  "Test",
						Body:     "Test body",
					},
				}

				channels := routing.ResolveChannelsForNotification(config, notification)
				Expect(channels).To(ContainElement("slack"))
			})
		})

		Context("when NotificationRequest has spec.channels specified", func() {

			It("should use spec.channels when explicitly specified", func() {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-notification",
						Namespace: "kubernaut-system",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     "approval_required",
						Priority: notificationv1alpha1.NotificationPriorityCritical,
						Severity: "critical",
						Subject:  "Test",
						Body:     "Test body",
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole,
							notificationv1alpha1.ChannelSlack,
						},
					},
				}

				hasExplicitChannels := len(notification.Spec.Channels) > 0
				Expect(hasExplicitChannels).To(BeTrue())
				Expect(notification.Spec.Channels).To(ContainElements(
					notificationv1alpha1.ChannelConsole,
					notificationv1alpha1.ChannelSlack,
				))
			})
		})

		Context("when routing configuration is nil or empty", func() {

			It("should return console fallback when config is nil", func() {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-notification",
						Namespace: "kubernaut-system",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     "approval_required",
						Priority: notificationv1alpha1.NotificationPriorityCritical,
						Subject:  "Test",
						Body:     "Test body",
					},
				}

				defaultConfig := routing.DefaultConfig()
				channels := routing.ResolveChannelsForNotification(defaultConfig, notification)
				Expect(channels).To(ContainElement("console"))
			})
		})
	})

	Describe("GetReceiverConfig", func() {

		It("should return receiver configuration for routing decisions", func() {
			configYAML := `
route:
  receiver: slack-default
receivers:
  - name: slack-default
    slackConfigs:
      - channel: '#alerts'
`
			config, err := routing.ParseConfig([]byte(configYAML))
			Expect(err).ToNot(HaveOccurred())

			receiver := config.GetReceiver("slack-default")
			Expect(receiver).ToNot(BeNil())
			Expect(receiver.SlackConfigs).To(HaveLen(1))
			Expect(receiver.SlackConfigs[0].Channel).To(Equal("#alerts"))
		})
	})

	Describe("Routing Attribute Key Constants", func() {

		It("should use simplified keys for spec-field-based routing (Issue #91)", func() {
			Expect(routing.AttrType).To(Equal("type"))
			Expect(routing.AttrSeverity).To(Equal("severity"))
			Expect(routing.AttrEnvironment).To(Equal("environment"))
			Expect(routing.AttrPriority).To(Equal("priority"))
			Expect(routing.AttrSkipReason).To(Equal("skip-reason"))
			Expect(routing.AttrInvestigationOutcome).To(Equal("investigation-outcome"))
		})
	})
})
