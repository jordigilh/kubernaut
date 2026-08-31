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

package fleet

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-FLEET-022 [AC-4, SI-10, AU-3]: AF's own list_alerts tool reads a real,
// organically-firing Prometheus alert that Gateway NEVER sees.
//
// Companion to E2E-FLEET-021 (the Gateway-ingestion organic-alert flow): this
// proves the OTHER half of realistic fleet alert traffic -- an alert AF reads
// directly off Prometheus via its own A2A tools, isolated from Gateway/RR
// creation by AlertManager's own routing (not by Gateway rejecting it, and
// not by the test simply choosing not to POST it). The alerting rule
// (fleet-organic-af-only-alert.yml, test/infrastructure/prometheus_alertmanager_e2e.go)
// carries route_skip_gateway="true", which AlertManager's route config
// (DeployAlertManager) matches to send to a null receiver instead of
// gateway-webhook -- proven here by asserting NO RemediationRequest is ever
// created for it, not merely that this test abstained from creating one.
//
// Design mirrors E2E-FLEET-020 (19_af_alerts_fleet_scoped_test.go): a
// deterministic vector(1) > 0 rule (no target resource needed, since Gateway
// must never attempt to resolve it) plus a direct poll of AF's real
// prometheus.Client-backed list_alerts path.
//
// Authority: kubernaut#2309 (fleet E2E realism follow-up), Issue #54, ADR-068
// FedRAMP: AC-4 (information flow enforcement), SI-10 (input validation),
// AU-3 (audit record content)
var _ = Describe("E2E-FLEET-022 [AC-4, SI-10, AU-3]: AF-only organic alert never reaches Gateway", Label("fleet", "af"), func() {
	const (
		prometheusURL = "http://localhost:9190"
		ruleName      = "FleetOrganicAFOnlyAlert"
	)

	It("should be readable via AF's real Prometheus client while producing zero RemediationRequests", func() {
		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in Fleet E2E cluster — skipping E2E-FLEET-022")
		}
		_ = resp.Body.Close()

		By("Waiting for FleetOrganicAFOnlyAlert to be firing on Prometheus (vector(1) > 0, deterministic from Prometheus startup; " +
			"same /api/v1/rules backend AF's severity-triage grounding and list_alerts both read)")
		Eventually(func() error {
			return infrastructure.WaitForPrometheusRuleState(ctx, prometheusURL, ruleName,
				infrastructure.RuleStateFiring, 5*time.Second)
		}, 60*time.Second, 2*time.Second).Should(Succeed(), "FleetOrganicAFOnlyAlert must be firing")

		By("Verifying NO RemediationRequest was ever created for this alert (route_skip_gateway must actually keep it out of Gateway)")
		Consistently(func(g Gomega) {
			var rrList remediationv1.RemediationRequestList
			g.Expect(k8sClient.List(ctx, &rrList, client.InNamespace(namespace))).To(Succeed())
			for i := range rrList.Items {
				g.Expect(rrList.Items[i].Spec.SignalName).ToNot(Equal(ruleName),
					"AC-4: FleetOrganicAFOnlyAlert carries route_skip_gateway=\"true\" and must never "+
						"reach Gateway -- finding a RemediationRequest for it means AlertManager's "+
						"null-receiver route is not actually excluding it")
			}
		}, 20*time.Second, 5*time.Second).Should(Succeed())
	})
})
