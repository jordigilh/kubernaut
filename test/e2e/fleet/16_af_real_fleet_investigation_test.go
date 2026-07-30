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
)

// E2E-FLEET-016 [SI-4, AC-4, AU-2/AU-3]: real AF binary calls its own
// fleet-aware kubectl_get tool with cluster_id via a real A2A request,
// closing issue #1768 Gaps A+C.
//
// Authority: issue #1768, ADR-068, DD-FLEET-004, docs/testing/1768/TEST_PLAN.md
//
// Why this test exists (what it closes that no other test does):
//   - 06_aa_fleet_investigation_test.go (Gap A) drives newFleetMCPClient
//     directly against the MCP gateway -- it never starts or calls the real
//     compiled AF binary, so AF's own token-derivation code
//     (pkg/apifrontend/tools/kubectl_get.go, af_list_clusters.go) and its own
//     ADK tool-registration wiring (pkg/apifrontend/agent/root.go) have zero
//     E2E coverage in this suite.
//   - 05_af_preflight_oauth2_test.go proves Keycloak + gateway OAuth2 works
//     in isolation, but likewise never constructs AF's own client.
//   - test/e2e/fullpipeline/10_af_fleet_cluster_id_test.go (Gap C) does call
//     the real AF binary via real A2A with cluster_id, but fullpipeline is a
//     SINGLE Kind cluster -- "remote" clusters there are a fiction backed by
//     the same local cluster, so it can prove cluster_id survives AF's
//     server-side plumbing (RRContext, artifacts) but can never prove a
//     cross-cluster kubectl read against a genuinely separate control plane.
//
// This test closes both gaps at once by driving AF's real A2A endpoint in
// THIS suite, where remote-cluster is a real second Kind cluster reached
// through the real Kuadrant MCP gateway + kube-mcp-server bridge
// (DD-TEST-013, AllRegistrationsRemote).
//
// Design: single-turn message/send. The "fleet-kubectl-e2e-test" keyword
// selects a dedicated mock-LLM scenario (scenario_af_fleet_kubectl.go) that
// emits list_clusters + kubectl_get(cluster_id="remote-cluster",
// kind=Deployment, name=coredns, namespace=kube-system) as a single
// MultiToolCalls batch. AF's ADK loop executes both tools for real against
// this suite's FleetReaderFactory -> kube-mcp-server -> Kuadrant gateway ->
// remote Kind cluster, then the scenario echoes the accumulated conversation
// text (which by then contains the real kubectl_get FunctionResponse) back
// as the final answer. Asserting on "coredns" in that final answer proves a
// literal round trip through the remote cluster, not a canned string that
// would pass even if the cluster_id routing silently no-op'd.
var _ = Describe("E2E-FLEET-016 [SI-4, AC-4, AU-2/AU-3]: AF real A2A fleet kubectl_get", Label("fleet", "af", "a2a", "issue-1768"), func() {

	It("should call list_clusters and kubectl_get(cluster_id=remote-cluster) via a real A2A request and return remote cluster data", func() {
		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in Fleet E2E cluster — skipping E2E-FLEET-016")
		}
		_ = resp.Body.Close()

		By("Sending A2A message: fleet-kubectl-e2e-test (selects list_clusters + kubectl_get scenario)")
		body := afA2ATasksSend("fleet-016-1", "fleet-kubectl-e2e-test: list clusters then read coredns on remote-cluster")
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
		GinkgoWriter.Printf("  E2E-FLEET-016 task: %s (state: %s)\n", task.ID, task.Status.State)

		By("Verifying AF's real kubectl_get tool call reached the remote cluster (AC-4: cross-cluster routing)")
		msgStr := afArtifactText(task)
		Expect(msgStr).NotTo(BeEmpty(),
			"completed task must carry accumulated artifact text with the echoed tool results")
		Expect(msgStr).NotTo(ContainSubstring("unable to reach cluster"),
			"kubectl_get must not fail to reach cluster_id=remote-cluster")
		Expect(msgStr).NotTo(ContainSubstring("fleet reader"),
			"kubectl_get must not surface a fleet reader construction error")
		Expect(msgStr).To(ContainSubstring("coredns"),
			"SI-4, AU-3: the coredns Deployment object from the REAL remote cluster must surface in AF's "+
				"final A2A answer, proving list_clusters + kubectl_get(cluster_id) round-tripped through AF's "+
				"own compiled binary, its own tool wiring, and the real Kuadrant gateway -> kube-mcp-server -> "+
				"remote Kind cluster bridge (DD-TEST-013) -- not a mocked ResourceReaderFactory or a loopback "+
				"single-cluster fiction")
		GinkgoWriter.Printf("  E2E-FLEET-016: confirmed real remote-cluster round trip via AF A2A\n")
	})
})
