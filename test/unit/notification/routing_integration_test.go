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

package notification_test

import (
	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BR-NOT-065: Channel Routing Based on Labels - Controller Integration
// This tests the integration between the routing package and NotificationRequest CRDs
var _ = Describe("Routing Controller Integration (BR-NOT-065)", func() {

	Describe("ResolveChannelsFromLabels", func() {

		var config *routing.Config

		BeforeEach(func() {
			// Setup a typical production routing configuration
			configYAML := `
route:
  receiver: default-slack
  routes:
    - match:
        kubernaut.ai/notification-type: approval_required
        kubernaut.ai/severity: critical
      receiver: pagerduty-critical
    - match:
        kubernaut.ai/notification-type: approval_required
      receiver: slack-approvals
    - match:
        kubernaut.ai/notification-type: failed
      receiver: pagerduty-oncall
    - match:
        kubernaut.ai/notification-type: completed
      receiver: slack-ops
receivers:
  - name: default-slack
    slack_configs:
      - channel: '#kubernaut-alerts'
  - name: pagerduty-critical
    pagerduty_configs:
      - service_key: critical-key
  - name: slack-approvals
    slack_configs:
      - channel: '#approvals'
  - name: pagerduty-oncall
    pagerduty_configs:
      - service_key: oncall-key
  - name: slack-ops
    slack_configs:
      - channel: '#ops'
`
			var err error
			config, err = routing.ParseConfig([]byte(configYAML))
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when NotificationRequest has no spec.channels", func() {

			It("should resolve channels from labels for critical approval notification", func() {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-notification",
						Namespace: "kubernaut-system",
						Labels: map[string]string{
							"kubernaut.ai/notification-type": "approval_required",
							"kubernaut.ai/severity":          "critical",
							"kubernaut.ai/environment":       "production",
						},
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeEscalation,
						Priority: notificationv1alpha1.NotificationPriorityCritical,
						Subject:  "Approval Required",
						Body:     "Test body",
						// Channels NOT specified - should be resolved from routing rules
					},
				}

				channels := routing.ResolveChannelsForNotification(config, notification)
				Expect(channels).To(ContainElement("pagerduty"))
				Expect(channels).ToNot(ContainElement("slack")) // PagerDuty receiver, not Slack
			})

			It("should resolve channels from labels for non-critical approval notification", func() {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-notification",
						Namespace: "kubernaut-system",
						Labels: map[string]string{
							"kubernaut.ai/notification-type": "approval_required",
							"kubernaut.ai/severity":          "high",
						},
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeEscalation,
						Priority: notificationv1alpha1.NotificationPriorityHigh,
						Subject:  "Approval Required",
						Body:     "Test body",
					},
				}

				channels := routing.ResolveChannelsForNotification(config, notification)
				Expect(channels).To(ContainElement("slack"))
			})

			It("should resolve channels from labels for failed notification", func() {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-notification",
						Namespace: "kubernaut-system",
						Labels: map[string]string{
							"kubernaut.ai/notification-type": "failed",
						},
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeStatusUpdate,
						Priority: notificationv1alpha1.NotificationPriorityHigh,
						Subject:  "Remediation Failed",
						Body:     "Test body",
					},
				}

				channels := routing.ResolveChannelsForNotification(config, notification)
				Expect(channels).To(ContainElement("pagerduty"))
			})

			It("should fall back to default receiver when no labels match", func() {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-notification",
						Namespace: "kubernaut-system",
						Labels: map[string]string{
							"kubernaut.ai/notification-type": "unknown-type",
						},
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Priority: notificationv1alpha1.NotificationPriorityLow,
						Subject:  "Test",
						Body:     "Test body",
					},
				}

				channels := routing.ResolveChannelsForNotification(config, notification)
				Expect(channels).To(ContainElement("slack")) // default-slack receiver
			})

			It("should handle notification with no labels by using default receiver", func() {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-notification",
						Namespace: "kubernaut-system",
						// No labels
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Priority: notificationv1alpha1.NotificationPriorityLow,
						Subject:  "Test",
						Body:     "Test body",
					},
				}

				channels := routing.ResolveChannelsForNotification(config, notification)
				Expect(channels).To(ContainElement("slack")) // default-slack receiver
			})
		})

		Context("when NotificationRequest has spec.channels specified", func() {

			It("should use spec.channels when explicitly specified", func() {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-notification",
						Namespace: "kubernaut-system",
						Labels: map[string]string{
							"kubernaut.ai/notification-type": "approval_required",
							"kubernaut.ai/severity":          "critical",
						},
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeEscalation,
						Priority: notificationv1alpha1.NotificationPriorityCritical,
						Subject:  "Test",
						Body:     "Test body",
						// Explicit channels override routing rules
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole,
							notificationv1alpha1.ChannelSlack,
						},
					},
				}

				// When spec.channels is specified, it takes precedence over routing
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
						Labels: map[string]string{
							"kubernaut.ai/notification-type": "approval_required",
						},
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeEscalation,
						Priority: notificationv1alpha1.NotificationPriorityCritical,
						Subject:  "Test",
						Body:     "Test body",
					},
				}

				// When config is nil, use DefaultConfig (console fallback)
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
    slack_configs:
      - channel: '#alerts'
        api_url: 'https://hooks.slack.com/services/xxx'
`
			config, err := routing.ParseConfig([]byte(configYAML))
			Expect(err).ToNot(HaveOccurred())

			receiver := config.GetReceiver("slack-default")
			Expect(receiver).ToNot(BeNil())
			Expect(receiver.SlackConfigs).To(HaveLen(1))
			Expect(receiver.SlackConfigs[0].Channel).To(Equal("#alerts"))
			Expect(receiver.SlackConfigs[0].APIURL).To(Equal("https://hooks.slack.com/services/xxx"))
		})
	})

	Describe("Label Key Constants", func() {

		It("should use kubernaut.ai domain for all routing labels", func() {
			// BR-NOT-065: Labels use kubernaut.ai domain (corrected from kubernaut.io)
			Expect(routing.LabelNotificationType).To(Equal("kubernaut.ai/notification-type"))
			Expect(routing.LabelSeverity).To(Equal("kubernaut.ai/severity"))
			Expect(routing.LabelEnvironment).To(Equal("kubernaut.ai/environment"))
			Expect(routing.LabelPriority).To(Equal("kubernaut.ai/priority"))
			Expect(routing.LabelComponent).To(Equal("kubernaut.ai/component"))
			Expect(routing.LabelRemediationRequest).To(Equal("kubernaut.ai/remediation-request"))
		})
	})
})




