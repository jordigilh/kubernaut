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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

// BR-NOT-068: Multi-Channel Fanout — Integration Tests
// Issue #597: Continue route fanout on notification routing
var _ = Describe("IT-NOT-597: Continue Route Fanout Integration", Label("integration", "routing-fanout"), func() {

	// IT-NOT-597-001: NR routed to multiple receivers via continue, both channel sets delivered
	It("IT-NOT-597-001: should route NR to multiple receivers via continue and deliver both channel sets", func() {
		By("Loading routing config with continue-enabled routes")
		testRouter := routing.NewRouter(logr.Discard())
		err := testRouter.LoadConfig([]byte(`
route:
  receiver: default-console
  routes:
    - match:
        type: Completion
        severity: critical
      receiver: slack-sre
      continue: true
    - match:
        type: Completion
        severity: critical
      receiver: pagerduty-escalation
receivers:
  - name: default-console
    consoleConfigs:
      - enabled: true
  - name: slack-sre
    slackConfigs:
      - channel: '#sre-critical'
  - name: pagerduty-escalation
    pagerdutyConfigs:
      - serviceKey: escalation-key
`))
		Expect(err).ToNot(HaveOccurred())

		By("Creating a NotificationRequest that matches both routes")
		nr := &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("nr-fanout-it-%s", testNamespace),
				Namespace: testNamespace,
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Type:     notificationv1alpha1.NotificationTypeCompletion,
				Severity: "critical",
				Subject:  "IT-597-001: Fanout integration test",
				Body:     "Testing continue route fanout",
			},
		}

		By("Resolving receivers via Router.FindReceivers (thread-safe)")
		attrs := routing.RoutingAttributesFromSpec(nr)
		receivers := testRouter.FindReceivers(attrs)
		Expect(receivers).To(HaveLen(2), "both routes should match via continue fanout")

		By("Verifying receiver names and channel types")
		Expect(receivers[0].Name).To(Equal("slack-sre"))
		Expect(receivers[1].Name).To(Equal("pagerduty-escalation"))

		By("Merging channels from all receivers with dedup")
		seen := make(map[string]bool)
		var allChannels []string
		for _, recv := range receivers {
			for _, ch := range recv.QualifiedChannels() {
				if !seen[ch] {
					allChannels = append(allChannels, ch)
					seen[ch] = true
				}
			}
		}
		Expect(allChannels).To(ContainElements("slack", "pagerduty"))
		Expect(allChannels).To(HaveLen(2))
	})

	// IT-NOT-597-002: Backward compat — existing single-receiver routing works unchanged
	It("IT-NOT-597-002: should preserve backward-compatible single-receiver routing", func() {
		By("Loading standard routing config without continue")
		testRouter := routing.NewRouter(logr.Discard())
		err := testRouter.LoadConfig([]byte(`
route:
  receiver: default-console
  routes:
    - match:
        type: Approval
      receiver: slack-approvals
    - match:
        type: Completion
      receiver: slack-completions
receivers:
  - name: default-console
    consoleConfigs:
      - enabled: true
  - name: slack-approvals
    slackConfigs:
      - channel: '#approvals'
  - name: slack-completions
    slackConfigs:
      - channel: '#completions'
`))
		Expect(err).ToNot(HaveOccurred())

		By("Routing with FindReceivers returns single receiver (same as FindReceiver)")
		attrs := map[string]string{"type": "Approval"}
		receivers := testRouter.FindReceivers(attrs)
		Expect(receivers).To(HaveLen(1))
		Expect(receivers[0].Name).To(Equal("slack-approvals"))

		By("FindReceiver still works as convenience wrapper")
		singleReceiver := testRouter.FindReceiver(attrs)
		Expect(singleReceiver.Name).To(Equal("slack-approvals"))
	})
})
