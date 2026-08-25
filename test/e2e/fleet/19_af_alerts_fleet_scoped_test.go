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
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-FLEET-020 [AC-4, SI-10, AU-3, BR-FLEET-054]: real AF binary calls its
// own cluster-scoped list_alerts tool via a real A2A request against a real
// Prometheus, closing Issue #2274's AF gap.
//
// Authority: Issue #2274, ADR-068, DD-EM-005 v1.3
//
// Why this test exists (what it closes that no other test does):
//   - pkg/apifrontend/tools/af_alerts_test.go (UT-AF-2274-001..006) proves
//     HandleListAlerts/HandleGetAlertDetails filter by cluster_id in
//     isolation (mock prom.Client), but never through AF's own compiled
//     binary, its own ADK tool-registration wiring, or a real Prometheus.
//   - E2E-FLEET-016 proved the analogous pattern for kubectl_get(cluster_id)
//     but never touched list_alerts/get_alert_details.
//
// CI RCA (PR #2286): the original design of this test injected alerts
// directly into AlertManager (infrastructure.InjectAlerts) and polled its
// /api/v2/alerts API. That can never work: AF's list_alerts/get_alert_details
// tools call pkg/apifrontend/prometheus.Client.GetAlerts, which queries THIS
// suite's Prometheus's own /api/v1/alerts (alerting-rule-derived active
// alerts) -- confirmed at that client's baseURL, wired from
// severityTriage.prometheusURL (monitoring.prometheus.url), a config value
// entirely separate from monitoring.alertManager.url
// (cmd/apifrontend/backend_deps.go only ever wires deps.PromClient from
// prometheusURL). AF's alert tools never read AlertManager's alert store at
// all. This mirrors the pre-existing, passing precedent
// test/e2e/apifrontend/alert_prioritization_e2e_test.go, which grounds AF's
// list_alerts via a real Prometheus alerting rule instead of AlertManager
// injection.
//
// Design: mirrors E2E-FLEET-016 (single-turn message/send, keyword-selected
// mock-LLM scenario, MultiToolCalls, echoed final answer). The
// "fleet-alerts-e2e-test" keyword selects scenario_af_fleet_alerts.go, which
// emits list_alerts(cluster_id="remote-cluster", namespace="fleet-alerts-e2e-ns")
// as a single MultiToolCalls batch. AF's ADK loop executes the tool for real
// against this suite's shared Prometheus instance
// (test/infrastructure/prometheus_alertmanager_e2e.go's
// fleet-alerts-cluster-scoped-2274.yml rule group), then the scenario echoes
// the accumulated conversation text (which by then contains the real
// list_alerts FunctionResponse) back as the final answer.
//
// The two alerting rules (Fleet2274MatchingClusterAlert,
// Fleet2274CollisionClusterAlert) share every label AF's query selects on
// EXCEPT `cluster` -- same collision-simulation approach used by EM's
// E2E-FLEET-019a/b (07_em_fleet_metrics_test.go) -- and assert AF's real
// list_alerts tool call surfaces ONLY the matching-cluster alert's
// distinguishing marker text. Pre-#2274-fix (no cluster_id filtering in
// af_alerts.go), both alerts would be returned, so the colliding cluster's
// marker would incorrectly appear in the final answer too.
var _ = Describe("E2E-FLEET-020 [AC-4, SI-10, AU-3]: AF real A2A cluster-scoped list_alerts (BR-FLEET-054, Issue #2274)", Label("fleet", "af", "a2a", "issue-2274"), func() {

	const (
		prometheusURL = "http://localhost:9190"

		// Markers are lowercase: the mock-LLM's ConfigForContext echoes back
		// ctx.AllText, which the mock-llm request handler always lowercases
		// before scenario matching (test/services/mock-llm/handlers/
		// {gemini,openai}.go). An uppercase marker here would never survive
		// that echo intact, so both the alerting rule's annotation (see
		// prometheus_alertmanager_e2e.go) and this assertion must use the
		// same lowercase form (CI RCA, PR #2286).
		matchingMarker  = "marker-remote-cluster-visible"
		collisionMarker = "marker-collision-cluster-hidden"

		matchingRuleName  = "Fleet2274MatchingClusterAlert"
		collisionRuleName = "Fleet2274CollisionClusterAlert"
	)

	It("should call list_alerts(cluster_id=remote-cluster) via a real A2A request and surface ONLY the matching cluster's alert", func() {
		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in Fleet E2E cluster — skipping E2E-FLEET-020")
		}
		_ = resp.Body.Close()

		By("Waiting for both cluster-collision Prometheus alerting rules to be firing " +
			"(fleet-alerts-cluster-scoped-2274.yml: vector(1) > 0, never stale, deterministic)")
		ctx := context.Background()
		Eventually(func() error {
			return infrastructure.WaitForPrometheusRuleState(ctx, prometheusURL, matchingRuleName,
				infrastructure.RuleStateFiring, 5*time.Second)
		}, 60*time.Second, 2*time.Second).Should(Succeed(), "matching-cluster alert must be firing before driving AF")
		Eventually(func() error {
			return infrastructure.WaitForPrometheusRuleState(ctx, prometheusURL, collisionRuleName,
				infrastructure.RuleStateFiring, 5*time.Second)
		}, 60*time.Second, 2*time.Second).Should(Succeed(), "colliding-cluster alert must be firing before driving AF")

		By("Sending A2A message: fleet-alerts-e2e-test (selects list_alerts scenario)")
		body := afA2ATasksSend("fleet-020-1", "fleet-alerts-e2e-test: list alerts on remote-cluster")
		resp, err = afA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "A2A message/send should return 200")

		rpc, parseErr := afParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "A2A should not return a JSON-RPC error")

		task, taskErr := afExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		Expect(task.ID).NotTo(BeEmpty(), "A2A task ID must not be empty")
		GinkgoWriter.Printf("  E2E-FLEET-020 task: %s (state: %s)\n", task.ID, task.Status.State)

		By("Verifying AF's real list_alerts tool call was cluster-scoped (BR-FLEET-054)")
		msgStr := afArtifactText(task)
		Expect(msgStr).NotTo(BeEmpty(),
			"completed task must carry accumulated artifact text with the echoed tool results")
		Expect(msgStr).To(ContainSubstring(matchingMarker),
			"AC-4, AU-3: the matching cluster's alert must surface in AF's real list_alerts response")
		Expect(msgStr).NotTo(ContainSubstring(collisionMarker),
			"Issue #2274 regression: a same-namespace alert from a DIFFERENT fleet cluster "+
				"must NOT surface in AF's list_alerts(cluster_id=remote-cluster) response -- this would "+
				"indicate af_alerts.go silently dropped its cluster_id filter")
		GinkgoWriter.Printf("  E2E-FLEET-020: confirmed AF's real list_alerts is cluster-scoped\n")
	})
})
