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
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-NOT-065: Channel Routing Based on Labels
// BR-NOT-066: Alertmanager-Compatible Configuration Format
var _ = Describe("Notification Routing Configuration (BR-NOT-065, BR-NOT-066)", func() {

	Describe("Config Parsing", func() {

		Context("when parsing valid Alertmanager-compatible configuration", func() {

			It("should parse a minimal routing configuration", func() {
				// BR-NOT-066: Alertmanager-compatible format
				configYAML := `
route:
  receiver: default-receiver
receivers:
  - name: default-receiver
    slack_configs:
      - channel: '#alerts'
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())
				Expect(config.Route).ToNot(BeNil())
				Expect(config.Route.Receiver).To(Equal("default-receiver"))
				Expect(config.Receivers).To(HaveLen(1))
				Expect(config.Receivers[0].Name).To(Equal("default-receiver"))
			})

			It("should parse routing rules with label matchers", func() {
				// BR-NOT-065: Label-based routing
				configYAML := `
route:
  receiver: default-receiver
  routes:
    - match:
        kubernaut.ai/notification-type: approval_required
        kubernaut.ai/severity: critical
      receiver: pagerduty-oncall
    - match:
        kubernaut.ai/notification-type: completed
      receiver: slack-ops
receivers:
  - name: default-receiver
    slack_configs:
      - channel: '#alerts'
  - name: pagerduty-oncall
    pagerduty_configs:
      - service_key: test-key
  - name: slack-ops
    slack_configs:
      - channel: '#ops'
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())
				Expect(config.Route.Routes).To(HaveLen(2))
				Expect(config.Route.Routes[0].Match).To(HaveKeyWithValue("kubernaut.ai/notification-type", "approval_required"))
				Expect(config.Route.Routes[0].Match).To(HaveKeyWithValue("kubernaut.ai/severity", "critical"))
				Expect(config.Route.Routes[0].Receiver).To(Equal("pagerduty-oncall"))
			})

			It("should parse group_by configuration", func() {
				configYAML := `
route:
  receiver: default
  group_by: ['kubernaut.ai/environment', 'kubernaut.ai/severity']
receivers:
  - name: default
    slack_configs:
      - channel: '#alerts'
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())
				Expect(config.Route.GroupBy).To(ContainElements("kubernaut.ai/environment", "kubernaut.ai/severity"))
			})

			It("should parse receiver configurations for multiple channels", func() {
				configYAML := `
route:
  receiver: multi-channel
receivers:
  - name: multi-channel
    slack_configs:
      - channel: '#alerts'
        api_url: 'https://hooks.slack.com/services/xxx'
    email_configs:
      - to: 'oncall@example.com'
        from: 'kubernaut@example.com'
    webhook_configs:
      - url: 'https://webhook.example.com/notify'
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())
				receiver := config.GetReceiver("multi-channel")
				Expect(receiver).ToNot(BeNil())
				Expect(receiver.SlackConfigs).To(HaveLen(1))
				Expect(receiver.EmailConfigs).To(HaveLen(1))
				Expect(receiver.WebhookConfigs).To(HaveLen(1))
			})
		})

		Context("when parsing invalid configuration", func() {

			It("should return error for empty configuration", func() {
				config, err := routing.ParseConfig([]byte(""))
				Expect(err).To(HaveOccurred())
				Expect(config).To(BeNil())
			})

			It("should return error for missing route", func() {
				configYAML := `
receivers:
  - name: default
    slack_configs:
      - channel: '#alerts'
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("route"))
				Expect(config).To(BeNil())
			})

			It("should return error for missing receivers", func() {
				configYAML := `
route:
  receiver: non-existent
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).To(HaveOccurred())
				Expect(config).To(BeNil())
			})

			It("should return error for invalid YAML", func() {
				config, err := routing.ParseConfig([]byte("invalid: yaml: syntax: ["))
				Expect(err).To(HaveOccurred())
				Expect(config).To(BeNil())
			})

			It("should return error when receiver references non-existent receiver", func() {
				configYAML := `
route:
  receiver: non-existent
receivers:
  - name: actual-receiver
    slack_configs:
      - channel: '#alerts'
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("non-existent"))
				Expect(config).To(BeNil())
			})
		})
	})

	Describe("Route Matching (BR-NOT-065)", func() {

		var config *routing.Config

		BeforeEach(func() {
			configYAML := `
route:
  receiver: default-receiver
  routes:
    - match:
        kubernaut.ai/notification-type: approval_required
        kubernaut.ai/severity: critical
      receiver: pagerduty-critical
    - match:
        kubernaut.ai/notification-type: approval_required
      receiver: slack-approvals
    - match:
        kubernaut.ai/notification-type: completed
      receiver: slack-ops
    - match:
        kubernaut.ai/notification-type: failed
      receiver: pagerduty-oncall
receivers:
  - name: default-receiver
    slack_configs:
      - channel: '#alerts'
  - name: pagerduty-critical
    pagerduty_configs:
      - service_key: critical-key
  - name: slack-approvals
    slack_configs:
      - channel: '#approvals'
  - name: slack-ops
    slack_configs:
      - channel: '#ops'
  - name: pagerduty-oncall
    pagerduty_configs:
      - service_key: oncall-key
`
			var err error
			config, err = routing.ParseConfig([]byte(configYAML))
			Expect(err).ToNot(HaveOccurred())
		})

		It("should match critical approval notifications to pagerduty-critical", func() {
			labels := map[string]string{
				"kubernaut.ai/notification-type": "approval_required",
				"kubernaut.ai/severity":          "critical",
				"kubernaut.ai/environment":       "production",
			}

			receiver := config.Route.FindReceiver(labels)
			Expect(receiver).To(Equal("pagerduty-critical"))
		})

		It("should match non-critical approval notifications to slack-approvals", func() {
			labels := map[string]string{
				"kubernaut.ai/notification-type": "approval_required",
				"kubernaut.ai/severity":          "high",
			}

			receiver := config.Route.FindReceiver(labels)
			Expect(receiver).To(Equal("slack-approvals"))
		})

		It("should match completed notifications to slack-ops", func() {
			labels := map[string]string{
				"kubernaut.ai/notification-type": "completed",
			}

			receiver := config.Route.FindReceiver(labels)
			Expect(receiver).To(Equal("slack-ops"))
		})

		It("should match failed notifications to pagerduty-oncall", func() {
			labels := map[string]string{
				"kubernaut.ai/notification-type": "failed",
			}

			receiver := config.Route.FindReceiver(labels)
			Expect(receiver).To(Equal("pagerduty-oncall"))
		})

		It("should fall back to default receiver when no routes match", func() {
			labels := map[string]string{
				"kubernaut.ai/notification-type": "unknown-type",
			}

			receiver := config.Route.FindReceiver(labels)
			Expect(receiver).To(Equal("default-receiver"))
		})

		It("should match first matching route (ordered evaluation)", func() {
			// Both routes could match, but first should win
			labels := map[string]string{
				"kubernaut.ai/notification-type": "approval_required",
				"kubernaut.ai/severity":          "critical",
			}

			receiver := config.Route.FindReceiver(labels)
			// First route matches both conditions, second only matches type
			Expect(receiver).To(Equal("pagerduty-critical"))
		})

		It("should handle empty labels by returning default receiver", func() {
			labels := map[string]string{}

			receiver := config.Route.FindReceiver(labels)
			Expect(receiver).To(Equal("default-receiver"))
		})

		It("should handle nil labels by returning default receiver", func() {
			receiver := config.Route.FindReceiver(nil)
			Expect(receiver).To(Equal("default-receiver"))
		})
	})

	Describe("Receiver Resolution", func() {

		var config *routing.Config

		BeforeEach(func() {
			configYAML := `
route:
  receiver: default
receivers:
  - name: default
    slack_configs:
      - channel: '#alerts'
  - name: pagerduty-oncall
    pagerduty_configs:
      - service_key: test-key
  - name: email-team
    email_configs:
      - to: 'team@example.com'
`
			var err error
			config, err = routing.ParseConfig([]byte(configYAML))
			Expect(err).ToNot(HaveOccurred())
		})

		It("should resolve existing receiver by name", func() {
			receiver := config.GetReceiver("default")
			Expect(receiver).ToNot(BeNil())
			Expect(receiver.Name).To(Equal("default"))
		})

		It("should return nil for non-existent receiver", func() {
			receiver := config.GetReceiver("non-existent")
			Expect(receiver).To(BeNil())
		})

		It("should resolve receiver with slack config", func() {
			receiver := config.GetReceiver("default")
			Expect(receiver.SlackConfigs).To(HaveLen(1))
			Expect(receiver.SlackConfigs[0].Channel).To(Equal("#alerts"))
		})

		It("should resolve receiver with pagerduty config", func() {
			receiver := config.GetReceiver("pagerduty-oncall")
			Expect(receiver.PagerDutyConfigs).To(HaveLen(1))
			Expect(receiver.PagerDutyConfigs[0].ServiceKey).To(Equal("test-key"))
		})

		It("should resolve receiver with email config", func() {
			receiver := config.GetReceiver("email-team")
			Expect(receiver.EmailConfigs).To(HaveLen(1))
			Expect(receiver.EmailConfigs[0].To).To(Equal("team@example.com"))
		})
	})

	Describe("Channel Extraction from Receiver", func() {

		It("should extract channels from slack receiver", func() {
			receiver := &routing.Receiver{
				Name: "slack-test",
				SlackConfigs: []routing.SlackConfig{
					{Channel: "#alerts"},
				},
			}

			channels := receiver.GetChannels()
			Expect(channels).To(ContainElement("slack"))
		})

		It("should extract channels from pagerduty receiver", func() {
			receiver := &routing.Receiver{
				Name: "pagerduty-test",
				PagerDutyConfigs: []routing.PagerDutyConfig{
					{ServiceKey: "key"},
				},
			}

			channels := receiver.GetChannels()
			Expect(channels).To(ContainElement("pagerduty"))
		})

		It("should extract channels from email receiver", func() {
			receiver := &routing.Receiver{
				Name: "email-test",
				EmailConfigs: []routing.EmailConfig{
					{To: "test@example.com"},
				},
			}

			channels := receiver.GetChannels()
			Expect(channels).To(ContainElement("email"))
		})

		It("should extract channels from webhook receiver", func() {
			receiver := &routing.Receiver{
				Name: "webhook-test",
				WebhookConfigs: []routing.WebhookConfig{
					{URL: "https://example.com/webhook"},
				},
			}

			channels := receiver.GetChannels()
			Expect(channels).To(ContainElement("webhook"))
		})

		It("should extract multiple channels from multi-channel receiver", func() {
			receiver := &routing.Receiver{
				Name: "multi-channel",
				SlackConfigs: []routing.SlackConfig{
					{Channel: "#alerts"},
				},
				EmailConfigs: []routing.EmailConfig{
					{To: "test@example.com"},
				},
			}

			channels := receiver.GetChannels()
			Expect(channels).To(HaveLen(2))
			Expect(channels).To(ContainElements("slack", "email"))
		})
	})

	Describe("Default Configuration", func() {

		It("should provide sensible defaults when loading from empty ConfigMap", func() {
			config := routing.DefaultConfig()
			Expect(config).ToNot(BeNil())
			Expect(config.Route).ToNot(BeNil())
			Expect(config.Route.Receiver).To(Equal("console-fallback"))
			Expect(config.Receivers).To(HaveLen(1))
			Expect(config.Receivers[0].Name).To(Equal("console-fallback"))
		})
	})
})
