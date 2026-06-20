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

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// =============================================================================
// E2E-AF-1472: Stale Session Validation After Pod Restart
//
// Proves the full user journey for issue #1472:
//   1. Establish a session with an explicit contextId (session enters memory)
//   2. Kill AF pod (in-memory session store is lost)
//   3. Wait for AF pod to restart and become healthy
//   4. Send message with the SAME stale contextId
//   5. Verify AF starts a fresh conversation (no misleading "reconnecting" UX)
//
// FedRAMP Controls: SC-7 (boundary), SC-10 (disconnect), SI-10 (input validation)
// =============================================================================

var _ = Describe("Stale Session Validation After Pod Restart (#1472)", Label("e2e", "session", "1472"), func() {
	var (
		authToken string
	)

	BeforeEach(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "Failed to obtain SRE token")

		resp, err := a2aInvoke(httpClient, baseURL, authToken, a2aTasksSend("1472-gate", "ping"))
		Expect(err).NotTo(HaveOccurred())
		_ = resp.Body.Close()
		Expect(resp.StatusCode).NotTo(Equal(http.StatusNotImplemented),
			"A2A endpoint must be available — mock-LLM required for E2E")
	})

	It("E2E-AF-1472-001 [SC-7, SC-10, SI-10]: stale context_id after pod restart yields fresh conversation", func() {
		staleContextID := fmt.Sprintf("ctx-stale-1472-%d", time.Now().UnixNano())

		By("Step 1: Sending message with explicit contextId to establish session in memory")
		body1 := a2aTasksSendWithContext("1472-establish", staleContextID, "hello establish session")
		resp1, err := a2aInvoke(httpClient, baseURL, authToken, body1)
		Expect(err).NotTo(HaveOccurred())
		respBody1, _ := io.ReadAll(resp1.Body)
		_ = resp1.Body.Close()
		Expect(resp1.StatusCode).To(Equal(http.StatusOK),
			"First message must succeed to establish the session")

		var rpc1 rpcResponse
		Expect(json.Unmarshal(respBody1, &rpc1)).To(Succeed())
		Expect(rpc1.Error).To(BeNil(), "First message must not return JSON-RPC error")
		GinkgoWriter.Printf("  Step 1 complete: session established with contextId=%s\n", staleContextID)

		By("Step 2: Killing AF pod to destroy in-memory session store")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		deleteCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"-n", e2eNamespace, "delete", "pod", "-l", "app.kubernetes.io/name=apifrontend",
			"--grace-period=0", "--force")
		deleteCmd.Stdout = GinkgoWriter
		deleteCmd.Stderr = GinkgoWriter
		Expect(deleteCmd.Run()).To(Succeed(), "kubectl delete AF pod should succeed")

		By("Step 3: Waiting for AF pod to restart and become healthy")
		waitCtx, waitCancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer waitCancel()

		rolloutCmd := exec.CommandContext(waitCtx, "kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "status", "deployment/apifrontend", "-n", e2eNamespace,
			"--timeout=120s")
		rolloutCmd.Stdout = GinkgoWriter
		rolloutCmd.Stderr = GinkgoWriter
		Expect(rolloutCmd.Run()).To(Succeed(), "AF deployment should become ready after pod restart")

		Eventually(func() error {
			resp, err := httpClient.Get(baseURL + "/healthz")
			if err != nil {
				return fmt.Errorf("healthz failed: %w", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("healthz returned %d", resp.StatusCode)
			}
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed(),
			"AF must become healthy after restart before sending stale context")

		By("Step 4: Sending message with the STALE contextId (no longer backed by in-memory session)")
		body2 := a2aTasksSendWithContext("1472-stale", staleContextID, "this context should be stale")
		var resp2 *http.Response
		Eventually(func(g Gomega) {
			resp2, err = a2aInvoke(httpClient, baseURL, authToken, body2)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(resp2.StatusCode).To(Equal(http.StatusOK))
		}, 30*time.Second, 3*time.Second).Should(Succeed(),
			"SC-7: A2A request with stale context_id must succeed (fresh conversation started)")

		respBody2, _ := io.ReadAll(resp2.Body)
		_ = resp2.Body.Close()

		By("Step 5: Verifying response is a valid fresh conversation (no reconnection attempt)")
		var rpc2 rpcResponse
		Expect(json.Unmarshal(respBody2, &rpc2)).To(Succeed())
		Expect(rpc2.Error).To(BeNil(),
			"SI-10: stale context_id must not cause JSON-RPC error — validator clears it and ADK creates fresh session")

		GinkgoWriter.Printf("  Step 5 complete: response received successfully after stale context handling\n")
		GinkgoWriter.Printf("  SC-10: Post-restart stale session was invalidated, fresh conversation started\n")
	})
})
