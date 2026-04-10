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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

// Issue #416: Label-Based Notification Routing — Integration Tests
var _ = Describe("IT-NOT-416: Label-Based Routing Integration", Label("integration", "label-routing"), func() {

	// IT-NOT-416-001: Routing on notification-target + team attributes with matchRe
	It("IT-NOT-416-001: should route on notification-target and team with matchRe", func() {
		By("Loading routing config with matchRe rules for team routing")
		testRouter := routing.NewRouter(logr.Discard())
		err := testRouter.LoadConfig([]byte(`
route:
  receiver: default-console
  routes:
    - match:
        notification-target: signal
      matchRe:
        team: "^sre-.*"
      receiver: sre-signal-receiver
      continue: true
    - match:
        notification-target: rca
      matchRe:
        team: "^sre-.*"
      receiver: sre-rca-receiver
receivers:
  - name: default-console
    consoleConfigs:
      - enabled: true
  - name: sre-signal-receiver
    slackConfigs:
      - channel: '#sre-signals'
  - name: sre-rca-receiver
    pagerdutyConfigs:
      - serviceKey: sre-rca-key
`))
		Expect(err).ToNot(HaveOccurred())

		By("Routing signal notification for SRE team")
		signalAttrs := map[string]string{
			"notification-target": "signal",
			"team":                "sre-platform",
			"type":                "Completion",
		}
		signalReceivers := testRouter.FindReceivers(signalAttrs)
		Expect(signalReceivers).To(HaveLen(1))
		Expect(signalReceivers[0].Name).To(Equal("sre-signal-receiver"))

		By("Routing RCA notification for SRE team")
		rcaAttrs := map[string]string{
			"notification-target": "rca",
			"team":                "sre-platform",
			"type":                "Completion",
		}
		rcaReceivers := testRouter.FindReceivers(rcaAttrs)
		Expect(rcaReceivers).To(HaveLen(1))
		Expect(rcaReceivers[0].Name).To(Equal("sre-rca-receiver"))

		By("Routing non-SRE team falls back to default")
		otherAttrs := map[string]string{
			"notification-target": "signal",
			"team":                "dev-team",
			"type":                "Completion",
		}
		otherReceivers := testRouter.FindReceivers(otherAttrs)
		Expect(otherReceivers).To(HaveLen(1))
		Expect(otherReceivers[0].Name).To(Equal("default-console"))
	})
})
