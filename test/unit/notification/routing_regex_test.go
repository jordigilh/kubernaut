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

// Issue #416: Label-based notification routing — matchRe and new routing attributes
var _ = Describe("Label-Based Routing: matchRe and Attributes (#416)", func() {

	Describe("matchRe Regex Matching", func() {

		// UT-NOT-416-001: matchRe with valid regex matches attribute values
		It("UT-NOT-416-001: should match attributes using regex patterns", func() {
			config, err := routing.ParseConfig([]byte(`
route:
  receiver: default
  routes:
    - matchRe:
        team: "^sre-.*"
      receiver: sre-receiver
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: sre-receiver
    slackConfigs:
      - channel: '#sre'
`))
			Expect(err).ToNot(HaveOccurred())

			attrs := map[string]string{"team": "sre-platform"}
			receivers := config.Route.FindReceivers(attrs)
			Expect(receivers).To(HaveLen(1))
			Expect(receivers[0]).To(Equal("sre-receiver"))
		})

		// UT-NOT-416-002: matchRe with invalid regex rejected at ParseConfig time
		It("UT-NOT-416-002: should reject invalid regex at ParseConfig time", func() {
			_, err := routing.ParseConfig([]byte(`
route:
  receiver: default
  routes:
    - matchRe:
        team: "[invalid"
      receiver: sre-receiver
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: sre-receiver
    slackConfigs:
      - channel: '#sre'
`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid"))
		})

		// UT-NOT-416-003: matchRe + match combined — both must satisfy
		It("UT-NOT-416-003: should require both match and matchRe to satisfy (AND semantics)", func() {
			config, err := routing.ParseConfig([]byte(`
route:
  receiver: default
  routes:
    - match:
        type: Completion
      matchRe:
        team: "^sre-.*"
      receiver: sre-completion
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: sre-completion
    slackConfigs:
      - channel: '#sre-completions'
`))
			Expect(err).ToNot(HaveOccurred())

			By("Both conditions satisfied — should match")
			attrs := map[string]string{"type": "Completion", "team": "sre-platform"}
			receivers := config.Route.FindReceivers(attrs)
			Expect(receivers).To(HaveLen(1))
			Expect(receivers[0]).To(Equal("sre-completion"))

			By("Only match satisfied, matchRe not — should NOT match")
			attrs2 := map[string]string{"type": "Completion", "team": "dev-team"}
			receivers2 := config.Route.FindReceivers(attrs2)
			Expect(receivers2).To(HaveLen(1))
			Expect(receivers2[0]).To(Equal("default"), "falls back to root when matchRe fails")
		})

		// UT-NOT-416-005: Empty matchRe map — match-all (same as empty match)
		It("UT-NOT-416-005: should treat empty matchRe as match-all", func() {
			config, err := routing.ParseConfig([]byte(`
route:
  receiver: default
  routes:
    - matchRe: {}
      receiver: catch-all
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: catch-all
    slackConfigs:
      - channel: '#all'
`))
			Expect(err).ToNot(HaveOccurred())

			attrs := map[string]string{"team": "anything"}
			receivers := config.Route.FindReceivers(attrs)
			Expect(receivers).To(HaveLen(1))
			Expect(receivers[0]).To(Equal("catch-all"))
		})
	})

	Describe("Routing Attributes from Extensions", func() {

		// UT-NOT-416-004: RoutingAttributesFromSpec extracts new attributes from Extensions
		It("UT-NOT-416-004: should extract team, owner, notification-target, target-kind from Extensions", func() {
			nr := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nr-test-416",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeCompletion,
					Severity: "critical",
					Subject:  "Test",
					Body:     "Test body",
					Extensions: map[string]string{
						routing.AttrNotificationTarget: "signal",
						routing.AttrTeam:               "sre-platform",
						routing.AttrOwner:              "jdoe",
						routing.AttrTargetKind:         "Deployment",
					},
				},
			}

			attrs := routing.RoutingAttributesFromSpec(nr)
			Expect(attrs[routing.AttrNotificationTarget]).To(Equal("signal"))
			Expect(attrs[routing.AttrTeam]).To(Equal("sre-platform"))
			Expect(attrs[routing.AttrOwner]).To(Equal("jdoe"))
			Expect(attrs[routing.AttrTargetKind]).To(Equal("Deployment"))
		})
	})

	Describe("Backward Compatibility", func() {

		// UT-NOT-416-006: Existing routing configs without matchRe still work
		It("UT-NOT-416-006: should pass existing routing without matchRe unchanged", func() {
			config, err := routing.ParseConfig([]byte(`
route:
  receiver: default
  routes:
    - match:
        type: Completion
      receiver: completions
receivers:
  - name: default
    consoleConfigs:
      - enabled: true
  - name: completions
    slackConfigs:
      - channel: '#completions'
`))
			Expect(err).ToNot(HaveOccurred())

			attrs := map[string]string{"type": "Completion"}
			receivers := config.Route.FindReceivers(attrs)
			Expect(receivers).To(HaveLen(1))
			Expect(receivers[0]).To(Equal("completions"))
		})
	})
})
