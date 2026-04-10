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
	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-NOT-068: Multi-Channel Fanout support
// Issue #597: Continue route fanout on notification routing
var _ = Describe("Continue Route Fanout (BR-NOT-068, #597)", func() {

	Describe("Route.FindReceivers", func() {

		// UT-NOT-597-001: continue=false returns single receiver (backward compat)
		It("UT-NOT-597-001: should return single receiver when continue is false (backward compat)", func() {
			config, err := routing.ParseConfig([]byte(`
route:
  receiver: default
  routes:
    - match:
        type: approval_required
      receiver: pagerduty-oncall
    - match:
        type: approval_required
      receiver: slack-ops
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: pagerduty-oncall
    pagerdutyConfigs:
      - serviceKey: test-key
  - name: slack-ops
    slackConfigs:
      - channel: '#ops'
`))
			Expect(err).ToNot(HaveOccurred())

			attrs := map[string]string{"type": "approval_required"}
			receivers := config.Route.FindReceivers(attrs)
			Expect(receivers).To(HaveLen(1), "without continue, only first match returned")
			Expect(receivers[0]).To(Equal("pagerduty-oncall"))
		})

		// UT-NOT-597-002: continue=true fans out to multiple sibling receivers
		It("UT-NOT-597-002: should fan out to multiple sibling receivers when continue is true", func() {
			config, err := routing.ParseConfig([]byte(`
route:
  receiver: default
  routes:
    - match:
        type: approval_required
      receiver: pagerduty-oncall
      continue: true
    - match:
        type: approval_required
      receiver: slack-ops
      continue: true
    - match:
        type: approval_required
      receiver: email-team
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: pagerduty-oncall
    pagerdutyConfigs:
      - serviceKey: test-key
  - name: slack-ops
    slackConfigs:
      - channel: '#ops'
  - name: email-team
    emailConfigs:
      - to: team@example.com
`))
			Expect(err).ToNot(HaveOccurred())

			attrs := map[string]string{"type": "approval_required"}
			receivers := config.Route.FindReceivers(attrs)
			Expect(receivers).To(HaveLen(3), "all three matching siblings returned")
			Expect(receivers).To(Equal([]string{"pagerduty-oncall", "slack-ops", "email-team"}))
		})

		// UT-NOT-597-003: continue=true on first, false on second — stops at second
		It("UT-NOT-597-003: should stop collecting at first route without continue", func() {
			config, err := routing.ParseConfig([]byte(`
route:
  receiver: default
  routes:
    - match:
        type: completion
      receiver: slack-ops
      continue: true
    - match:
        type: completion
      receiver: pagerduty-oncall
    - match:
        type: completion
      receiver: email-team
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: slack-ops
    slackConfigs:
      - channel: '#ops'
  - name: pagerduty-oncall
    pagerdutyConfigs:
      - serviceKey: test-key
  - name: email-team
    emailConfigs:
      - to: team@example.com
`))
			Expect(err).ToNot(HaveOccurred())

			attrs := map[string]string{"type": "completion"}
			receivers := config.Route.FindReceivers(attrs)
			Expect(receivers).To(HaveLen(2), "stops at pagerduty-oncall which has no continue")
			Expect(receivers).To(Equal([]string{"slack-ops", "pagerduty-oncall"}))
		})

		// UT-NOT-597-004: Nested routes with continue propagation
		It("UT-NOT-597-004: should handle nested routes with continue correctly", func() {
			config, err := routing.ParseConfig([]byte(`
route:
  receiver: default
  routes:
    - match:
        severity: critical
      receiver: pagerduty-oncall
      continue: true
      routes:
        - match:
            type: approval_required
          receiver: slack-approvals
    - match:
        severity: critical
      receiver: email-escalation
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: pagerduty-oncall
    pagerdutyConfigs:
      - serviceKey: test-key
  - name: slack-approvals
    slackConfigs:
      - channel: '#approvals'
  - name: email-escalation
    emailConfigs:
      - to: escalation@example.com
`))
			Expect(err).ToNot(HaveOccurred())

			attrs := map[string]string{"severity": "critical", "type": "approval_required"}
			receivers := config.Route.FindReceivers(attrs)
			Expect(receivers).To(HaveLen(2))
			Expect(receivers).To(Equal([]string{"slack-approvals", "email-escalation"}))
		})

		// UT-NOT-597-005: Same receiver matched twice — deduplicated in results
		It("UT-NOT-597-005: should deduplicate when same receiver matched by multiple routes", func() {
			config, err := routing.ParseConfig([]byte(`
route:
  receiver: default
  routes:
    - match:
        type: approval_required
      receiver: slack-ops
      continue: true
    - match:
        severity: critical
      receiver: slack-ops
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: slack-ops
    slackConfigs:
      - channel: '#ops'
`))
			Expect(err).ToNot(HaveOccurred())

			attrs := map[string]string{"type": "approval_required", "severity": "critical"}
			receivers := config.Route.FindReceivers(attrs)
			Expect(receivers).To(HaveLen(1), "duplicate receiver deduplicated")
			Expect(receivers[0]).To(Equal("slack-ops"))
		})

		// UT-NOT-597-006: No matching routes — falls back to root receiver
		It("UT-NOT-597-006: should fall back to root receiver when no routes match", func() {
			config, err := routing.ParseConfig([]byte(`
route:
  receiver: default
  routes:
    - match:
        type: approval_required
      receiver: pagerduty-oncall
      continue: true
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: pagerduty-oncall
    pagerdutyConfigs:
      - serviceKey: test-key
`))
			Expect(err).ToNot(HaveOccurred())

			attrs := map[string]string{"type": "no-match"}
			receivers := config.Route.FindReceivers(attrs)
			Expect(receivers).To(HaveLen(1))
			Expect(receivers[0]).To(Equal("default"))
		})
	})

	Describe("Router.FindReceivers", func() {

		// UT-NOT-597-007: Router.FindReceivers returns multiple *Receiver objects
		It("UT-NOT-597-007: should return multiple Receiver objects via Router", func() {
			router := routing.NewRouter(logr.Discard())
			err := router.LoadConfig([]byte(`
route:
  receiver: default
  routes:
    - match:
        type: approval_required
      receiver: slack-ops
      continue: true
    - match:
        type: approval_required
      receiver: pagerduty-oncall
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: slack-ops
    slackConfigs:
      - channel: '#ops'
  - name: pagerduty-oncall
    pagerdutyConfigs:
      - serviceKey: test-key
`))
			Expect(err).ToNot(HaveOccurred())

			attrs := map[string]string{"type": "approval_required"}
			receivers := router.FindReceivers(attrs)
			Expect(receivers).To(HaveLen(2))
			Expect(receivers[0].Name).To(Equal("slack-ops"))
			Expect(receivers[1].Name).To(Equal("pagerduty-oncall"))
		})

		// UT-NOT-597-008: Channel dedup when multiple receivers have overlapping channels
		It("UT-NOT-597-008: should deduplicate channels from multiple receivers", func() {
			router := routing.NewRouter(logr.Discard())
			err := router.LoadConfig([]byte(`
route:
  receiver: default
  routes:
    - match:
        type: approval_required
      receiver: slack-team-a
      continue: true
    - match:
        type: approval_required
      receiver: console-and-log
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: slack-team-a
    slackConfigs:
      - channel: '#team-a'
        credentialRef: team-a-webhook
    consoleConfigs:
      - enabled: true
  - name: console-and-log
    consoleConfigs:
      - enabled: true
    logConfigs:
      - enabled: true
`))
			Expect(err).ToNot(HaveOccurred())

			attrs := map[string]string{"type": "approval_required"}
			receivers := router.FindReceivers(attrs)
			Expect(receivers).To(HaveLen(2))

			var allChannels []string
			seen := make(map[string]bool)
			for _, recv := range receivers {
				for _, ch := range recv.QualifiedChannels() {
					if !seen[ch] {
						allChannels = append(allChannels, ch)
						seen[ch] = true
					}
				}
			}
			Expect(allChannels).To(ContainElements("slack:slack-team-a", "console", "log"))
			Expect(allChannels).To(HaveLen(3), "console appears in both receivers but deduplicated")
		})
	})
})
