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
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-NOT-065: Channel Routing Based on Spec Fields
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
    slackConfigs:
      - channel: '#alerts'
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())
				Expect(config.Route.Receiver).To(Equal("default-receiver"))
				Expect(config.Receivers).To(HaveLen(1))
				Expect(config.Receivers[0].Name).To(Equal("default-receiver"))
			})

			It("should parse routing rules with attribute matchers", func() {
				// BR-NOT-065: Attribute-based routing
				configYAML := `
route:
  receiver: default-receiver
  routes:
    - match:
        type: approval_required
        severity: critical
      receiver: pagerduty-oncall
    - match:
        type: completed
      receiver: slack-ops
receivers:
  - name: default-receiver
    slackConfigs:
      - channel: '#alerts'
  - name: pagerduty-oncall
    pagerdutyConfigs:
      - serviceKey: test-key
  - name: slack-ops
    slackConfigs:
      - channel: '#ops'
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())
				Expect(config.Route.Routes).To(HaveLen(2))
				Expect(config.Route.Routes[0].Match).To(HaveKeyWithValue("type", "approval_required"))
				Expect(config.Route.Routes[0].Match).To(HaveKeyWithValue("severity", "critical"))
				Expect(config.Route.Routes[0].Receiver).To(Equal("pagerduty-oncall"))
			})

			It("should parse group_by configuration", func() {
				configYAML := `
route:
  receiver: default
  groupBy: ['environment', 'severity']
receivers:
  - name: default
    slackConfigs:
      - channel: '#alerts'
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())
				Expect(config.Route.GroupBy).To(ContainElements("environment", "severity"))
			})

			It("should parse receiver configurations for multiple channels", func() {
				configYAML := `
route:
  receiver: multi-channel
receivers:
  - name: multi-channel
    slackConfigs:
      - channel: '#alerts'
    emailConfigs:
      - to: 'oncall@example.com'
        from: 'kubernaut@example.com'
    webhookConfigs:
      - url: 'https://webhook.example.com/notify'
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())
				receiver := config.GetReceiver("multi-channel")
				Expect(receiver.Name).To(Equal("multi-channel"))
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
    slackConfigs:
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
    slackConfigs:
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
        type: approval_required
        severity: critical
      receiver: pagerduty-critical
    - match:
        type: approval_required
      receiver: slack-approvals
    - match:
        type: completed
      receiver: slack-ops
    - match:
        type: failed
      receiver: pagerduty-oncall
receivers:
  - name: default-receiver
    slackConfigs:
      - channel: '#alerts'
  - name: pagerduty-critical
    pagerdutyConfigs:
      - serviceKey: critical-key
  - name: slack-approvals
    slackConfigs:
      - channel: '#approvals'
  - name: slack-ops
    slackConfigs:
      - channel: '#ops'
  - name: pagerduty-oncall
    pagerdutyConfigs:
      - serviceKey: oncall-key
`
			var err error
			config, err = routing.ParseConfig([]byte(configYAML))
			Expect(err).ToNot(HaveOccurred())
		})

		It("should match critical approval notifications to pagerduty-critical", func() {
			attrs := map[string]string{
				"type": "approval_required",
				"severity":          "critical",
				"environment":       "production",
			}

			receiver := config.Route.FindReceiver(attrs)
			Expect(receiver).To(Equal("pagerduty-critical"))
		})

		It("should match non-critical approval notifications to slack-approvals", func() {
			attrs := map[string]string{
				"type": "approval_required",
				"severity":          "high",
			}

			receiver := config.Route.FindReceiver(attrs)
			Expect(receiver).To(Equal("slack-approvals"))
		})

		It("should match completed notifications to slack-ops", func() {
			attrs := map[string]string{
				"type": "completed",
			}

			receiver := config.Route.FindReceiver(attrs)
			Expect(receiver).To(Equal("slack-ops"))
		})

		It("should match failed notifications to pagerduty-oncall", func() {
			attrs := map[string]string{
				"type": "failed",
			}

			receiver := config.Route.FindReceiver(attrs)
			Expect(receiver).To(Equal("pagerduty-oncall"))
		})

		It("should fall back to default receiver when no routes match", func() {
			attrs := map[string]string{
				"type": "unknown-type",
			}

			receiver := config.Route.FindReceiver(attrs)
			Expect(receiver).To(Equal("default-receiver"))
		})

		It("should match first matching route (ordered evaluation)", func() {
			// Both routes could match, but first should win
			attrs := map[string]string{
				"type": "approval_required",
				"severity":          "critical",
			}

			receiver := config.Route.FindReceiver(attrs)
			// First route matches both conditions, second only matches type
			Expect(receiver).To(Equal("pagerduty-critical"))
		})

		It("should handle empty attributes by returning default receiver", func() {
			attrs := map[string]string{}

			receiver := config.Route.FindReceiver(attrs)
			Expect(receiver).To(Equal("default-receiver"))
		})

		It("should handle nil attributes by returning default receiver", func() {
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
    slackConfigs:
      - channel: '#alerts'
  - name: pagerduty-oncall
    pagerdutyConfigs:
      - serviceKey: test-key
  - name: email-team
    emailConfigs:
      - to: 'team@example.com'
`
			var err error
			config, err = routing.ParseConfig([]byte(configYAML))
			Expect(err).ToNot(HaveOccurred())
		})

		It("should resolve existing receiver by name", func() {
			receiver := config.GetReceiver("default")
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
			Expect(config.Route.Receiver).To(Equal("console-fallback"))
			Expect(config.Receivers).To(HaveLen(1))
			Expect(config.Receivers[0].Name).To(Equal("console-fallback"))
		})
	})

	// =============================================================================
	// SKIP-REASON ATTRIBUTE ROUTING (BR-NOT-065, DD-WE-004)
	// =============================================================================
	// Added: Day 13 Enhancement
	// Purpose: Verify skip-reason based routing for WorkflowExecution failures
	// Cross-Team: WE→NOT Q7, RO Q8 (2025-12-06)
	// =============================================================================

	Describe("Skip-Reason Attribute Routing (BR-NOT-065, DD-WE-004)", func() {

		Context("Attribute Constants Verification", func() {

			// Test 1: Verify attribute key constant
			It("should define correct skip-reason attribute key", func() {
				Expect(routing.AttrSkipReason).To(Equal("skip-reason"))
			})

			// Test 2: Verify skip reason value constants
			It("should define all DD-WE-004 skip reason values", func() {
				Expect(routing.SkipReasonPreviousExecutionFailed).To(Equal("PreviousExecutionFailed"))
				Expect(routing.SkipReasonExhaustedRetries).To(Equal("ExhaustedRetries"))
				Expect(routing.SkipReasonResourceBusy).To(Equal("ResourceBusy"))
				Expect(routing.SkipReasonRecentlyRemediated).To(Equal("RecentlyRemediated"))
			})
		})

		Context("Skip-Reason Routing Rules", func() {
			var config *routing.Config

			BeforeEach(func() {
				// Production-like routing config with skip-reason rules
				configYAML := `
route:
  routes:
    # CRITICAL: Execution failures → PagerDuty
    - match:
        skip-reason: PreviousExecutionFailed
      receiver: pagerduty-critical
    # HIGH: Exhausted retries → Slack
    - match:
        skip-reason: ExhaustedRetries
      receiver: slack-ops
    # LOW: Temporary conditions → Console
    - match:
        skip-reason: ResourceBusy
      receiver: console-bulk
    - match:
        skip-reason: RecentlyRemediated
      receiver: console-bulk
  receiver: default-slack
receivers:
  - name: pagerduty-critical
    pagerdutyConfigs:
      - serviceKey: test-critical-key
  - name: slack-ops
    slackConfigs:
      - channel: '#kubernaut-ops'
  - name: console-bulk
    console_config:
      enabled: true
  - name: default-slack
    slackConfigs:
      - channel: '#kubernaut-alerts'
`
				var err error
				config, err = routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())
			})

			// Test 3: DescribeTable for all skip-reason routing scenarios
			DescribeTable("should route to correct receiver based on skip-reason",
				func(skipReason, expectedReceiver string, description string) {
					attrs := map[string]string{
						routing.AttrSkipReason: skipReason,
					}
					receiverName := config.Route.FindReceiver(attrs)
					Expect(receiverName).To(Equal(expectedReceiver), description)
				},
				Entry("CRITICAL: PreviousExecutionFailed → pagerduty-critical",
					routing.SkipReasonPreviousExecutionFailed, "pagerduty-critical",
					"Execution failures require immediate PagerDuty alerting"),
				Entry("HIGH: ExhaustedRetries → slack-ops",
					routing.SkipReasonExhaustedRetries, "slack-ops",
					"Infrastructure issues route to ops channel"),
				Entry("LOW: ResourceBusy → console-bulk",
					routing.SkipReasonResourceBusy, "console-bulk",
					"Temporary condition - bulk notification only"),
				Entry("LOW: RecentlyRemediated → console-bulk",
					routing.SkipReasonRecentlyRemediated, "console-bulk",
					"Cooldown active - bulk notification only"),
				Entry("FALLBACK: unknown-reason → default-slack",
					"unknown-skip-reason", "default-slack",
					"Unknown skip reasons fall back to default receiver"),
			)

			// Test 4: Combined attributes (skip-reason + severity)
			It("should match most specific rule when skip-reason combined with severity", func() {
				// Config with combined matching (more specific first)
				combinedConfigYAML := `
route:
  routes:
    - match:
        skip-reason: PreviousExecutionFailed
        severity: critical
      receiver: pagerduty-immediate
    - match:
        skip-reason: PreviousExecutionFailed
      receiver: slack-escalation
  receiver: default-console
receivers:
  - name: pagerduty-immediate
    pagerdutyConfigs:
      - serviceKey: immediate-key
  - name: slack-escalation
    slackConfigs:
      - channel: '#escalation'
  - name: default-console
    console_config:
      enabled: true
`
				combinedConfig, err := routing.ParseConfig([]byte(combinedConfigYAML))
				Expect(err).ToNot(HaveOccurred())

				// Both attributes - should match first (more specific) rule
				attrsWithSeverity := map[string]string{
					routing.AttrSkipReason: routing.SkipReasonPreviousExecutionFailed,
					routing.AttrSeverity:   routing.SeverityCritical,
				}
				Expect(combinedConfig.Route.FindReceiver(attrsWithSeverity)).To(
					Equal("pagerduty-immediate"),
					"Combined skip-reason+severity should match specific rule")

				// Only skip-reason - should match second rule
				attrsOnlySkip := map[string]string{
					routing.AttrSkipReason: routing.SkipReasonPreviousExecutionFailed,
				}
				Expect(combinedConfig.Route.FindReceiver(attrsOnlySkip)).To(
					Equal("slack-escalation"),
					"Skip-reason alone should match less specific rule")
			})

			// Test 5: Empty/nil attributes fallback
			It("should fall back to default receiver when no skip-reason attribute present", func() {
				emptyAttrs := map[string]string{}
				Expect(config.Route.FindReceiver(emptyAttrs)).To(Equal("default-slack"))

				Expect(config.Route.FindReceiver(nil)).To(Equal("default-slack"))
			})
		})
	})

	// =============================================================================
	// BR-HAPI-200: Investigation Outcome Routing Tests
	// Purpose: Verify investigation-outcome based routing for HolmesGPT-API results
	// Cross-Team: KA→NOT (2025-12-07)
	// =============================================================================

	// =============================================================================
	// BR-NOT-068: Multi-Channel Fanout (Route Continue Flag) — Issue #597
	// =============================================================================

	Describe("Route Fanout (BR-NOT-068, #597)", func() {

		Context("FindReceivers with continue flag", func() {

			It("[UT-NOT-597-001] should collect all matching receivers when continue is true", func() {
				configYAML := `
route:
  receiver: default
  routes:
    - receiver: fanout-A
      match:
        team: sre
      continue: true
    - receiver: fanout-B
      match:
        team: sre
      continue: true
    - receiver: fanout-C
      match:
        team: sre
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: fanout-A
    consoleConfigs:
      - enabled: true
  - name: fanout-B
    fileConfigs:
      - enabled: true
  - name: fanout-C
    logConfigs:
      - enabled: true
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())

				receivers := config.Route.FindReceivers(map[string]string{"team": "sre"})
				Expect(receivers).To(HaveLen(3))
				Expect(receivers).To(Equal([]string{"fanout-A", "fanout-B", "fanout-C"}))
			})

			It("[UT-NOT-597-002] should stop at first match when continue is false (regression)", func() {
				configYAML := `
route:
  receiver: default
  routes:
    - receiver: first-match
      match:
        team: sre
    - receiver: second-match
      match:
        team: sre
    - receiver: third-match
      match:
        team: sre
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: first-match
    consoleConfigs:
      - enabled: true
  - name: second-match
    fileConfigs:
      - enabled: true
  - name: third-match
    logConfigs:
      - enabled: true
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())

				receivers := config.Route.FindReceivers(map[string]string{"team": "sre"})
				Expect(receivers).To(HaveLen(1))
				Expect(receivers).To(Equal([]string{"first-match"}))
			})

			It("[UT-NOT-597-003] should stop at first continue:false after collecting continue:true", func() {
				configYAML := `
route:
  receiver: default
  routes:
    - receiver: collected-A
      match:
        team: sre
      continue: true
    - receiver: collected-B
      match:
        team: sre
    - receiver: skipped-C
      match:
        team: sre
      continue: true
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: collected-A
    consoleConfigs:
      - enabled: true
  - name: collected-B
    fileConfigs:
      - enabled: true
  - name: skipped-C
    logConfigs:
      - enabled: true
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())

				receivers := config.Route.FindReceivers(map[string]string{"team": "sre"})
				Expect(receivers).To(HaveLen(2))
				Expect(receivers).To(Equal([]string{"collected-A", "collected-B"}))
			})

			It("[UT-NOT-597-004] should handle nested routes with continue at different levels", func() {
				// Alertmanager semantic: children take over from parent receiver
				configYAML := `
route:
  receiver: root
  routes:
    - receiver: parent-A
      match:
        team: sre
      continue: true
      routes:
        - receiver: child-A1
          match:
            severity: critical
    - receiver: parent-B
      match:
        team: sre
receivers:
  - name: root
    consoleConfigs:
      - enabled: true
  - name: parent-A
    consoleConfigs:
      - enabled: true
  - name: child-A1
    fileConfigs:
      - enabled: true
  - name: parent-B
    logConfigs:
      - enabled: true
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())

				// team=sre, severity=critical: child-A1 matches under parent-A (continue), parent-B matches
				receivers := config.Route.FindReceivers(map[string]string{
					"team":     "sre",
					"severity": "critical",
				})
				Expect(receivers).To(HaveLen(2))
				Expect(receivers).To(Equal([]string{"child-A1", "parent-B"}))
			})

			It("[UT-NOT-597-005] should deduplicate when same receiver appears in multiple routes", func() {
				configYAML := `
route:
  receiver: default
  routes:
    - receiver: shared-receiver
      match:
        team: sre
      continue: true
    - receiver: shared-receiver
      match:
        team: sre
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: shared-receiver
    consoleConfigs:
      - enabled: true
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())

				receivers := config.Route.FindReceivers(map[string]string{"team": "sre"})
				Expect(receivers).To(HaveLen(1))
				Expect(receivers).To(Equal([]string{"shared-receiver"}))
			})

			It("[UT-NOT-597-006] should fall back to parent receiver when no children match", func() {
				configYAML := `
route:
  receiver: parent-fallback
  routes:
    - receiver: child-only
      match:
        team: does-not-exist
receivers:
  - name: parent-fallback
    consoleConfigs:
      - enabled: true
  - name: child-only
    fileConfigs:
      - enabled: true
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())

				receivers := config.Route.FindReceivers(map[string]string{"team": "sre"})
				Expect(receivers).To(HaveLen(1))
				Expect(receivers).To(Equal([]string{"parent-fallback"}))
			})
		})

		Context("FindReceiver backward compatibility", func() {

			It("[UT-NOT-597-007] should return first matched receiver (backward compat wrapper)", func() {
				configYAML := `
route:
  receiver: default
  routes:
    - receiver: fanout-A
      match:
        team: sre
      continue: true
    - receiver: fanout-B
      match:
        team: sre
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: fanout-A
    consoleConfigs:
      - enabled: true
  - name: fanout-B
    fileConfigs:
      - enabled: true
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())

				// FindReceiver (singular) should still return first match only
				receiver := config.Route.FindReceiver(map[string]string{"team": "sre"})
				Expect(receiver).To(Equal("fanout-A"))
			})
		})

		Context("Multi-channel delivery plan from fanout", func() {

			It("[UT-NOT-597-009] should produce a complete multi-channel delivery plan from continue:true routing (BR-NOT-068, BR-NOT-069)", func() {
				configYAML := `
route:
  receiver: default
  routes:
    - receiver: console-recv
      match:
        team: sre
      continue: true
    - receiver: file-recv
      match:
        team: sre
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: console-recv
    consoleConfigs:
      - enabled: true
  - name: file-recv
    fileConfigs:
      - enabled: true
`
				config, err := routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())

				receiverNames := config.Route.FindReceivers(map[string]string{"team": "sre"})
				Expect(receiverNames).To(HaveLen(2),
					"Fanout should resolve 2 receivers for a single notification")

				// Verify that the resolved receivers produce distinct channels,
				// confirming the business outcome: one notification → multiple channels
				var allChannels []string
				for _, name := range receiverNames {
					recv := config.GetReceiver(name)
					Expect(recv.Name).To(Equal(name),
						"Each resolved name should map to a valid receiver in the config")
					allChannels = append(allChannels, recv.GetChannels()...)
				}

				Expect(allChannels).To(ConsistOf("console", "file"),
					"Fanout should produce delivery to console (from console-recv) and file (from file-recv)")
			})
		})
	})

	Describe("Investigation-Outcome Attribute Routing (BR-HAPI-200)", func() {

		Context("Attribute Constants Verification", func() {

			// Test 1: Verify attribute key constant
			It("should define correct investigation-outcome attribute key", func() {
				Expect(routing.AttrInvestigationOutcome).To(Equal("investigation-outcome"))
			})

			// Test 2: Verify investigation outcome value constants
			It("should define all BR-HAPI-200 investigation outcome values", func() {
				Expect(routing.InvestigationOutcomeResolved).To(Equal("resolved"))
				Expect(routing.InvestigationOutcomeInconclusive).To(Equal("inconclusive"))
				Expect(routing.InvestigationOutcomeWorkflowSelected).To(Equal("workflow_selected"))
			})
		})

		Context("Investigation-Outcome Routing Rules", func() {
			var config *routing.Config

			BeforeEach(func() {
				// Production-like routing config with investigation-outcome rules
				configYAML := `
route:
  receiver: default-slack
  routes:
    # Resolved: Skip notification (route to no-op/silent receiver)
    - match:
        investigation-outcome: resolved
      receiver: silent-noop
    # Inconclusive: Route to ops for human review
    - match:
        investigation-outcome: inconclusive
      receiver: slack-ops
    # Workflow selected: Standard routing (falls through to default)
    - match:
        investigation-outcome: workflow_selected
      receiver: default-slack
receivers:
  - name: silent-noop
    # No delivery configs = silent/skip notification
  - name: slack-ops
    slackConfigs:
      - channel: '#ops'
  - name: default-slack
    slackConfigs:
      - channel: '#alerts'
`
				var err error
				config, err = routing.ParseConfig([]byte(configYAML))
				Expect(err).ToNot(HaveOccurred())
			})

			// Test 3: DescribeTable for all investigation-outcome routing scenarios
			DescribeTable("should route to correct receiver based on investigation-outcome",
				func(outcome, expectedReceiver, description string) {
					attrs := map[string]string{
						routing.AttrInvestigationOutcome: outcome,
					}
					receiverName := config.Route.FindReceiver(attrs)
					Expect(receiverName).To(Equal(expectedReceiver), description)
				},
				Entry("resolved → silent-noop",
					routing.InvestigationOutcomeResolved, "silent-noop",
					"Self-resolved alerts skip notification to prevent alert fatigue"),
				Entry("OPS: inconclusive → slack-ops",
					routing.InvestigationOutcomeInconclusive, "slack-ops",
					"Inconclusive investigations route to ops for human review"),
				Entry("DEFAULT: workflow_selected → default-slack",
					routing.InvestigationOutcomeWorkflowSelected, "default-slack",
					"Normal workflow selection uses default routing"),
				Entry("FALLBACK: unknown-outcome → default-slack",
					"unknown-outcome", "default-slack",
					"Unknown outcomes fall back to default receiver"),
			)

			// Test 4: Empty attributes fallback
			It("should fall back to default receiver when no investigation-outcome attribute present", func() {
				emptyAttrs := map[string]string{}
				Expect(config.Route.FindReceiver(emptyAttrs)).To(Equal("default-slack"))
			})
		})
	})
})
