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
	"net/http"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// =============================================================================
// E2E-AF-1387: MCP Session Resilience — Ping-on-Acquire (#1387)
//
// Proves the full resilience journey through the deployed AF → KA stack:
//   1. Establish MCP session, start investigation (session enters pool)
//   2. Kill KA pod (transport dies for cached session)
//   3. Wait for KA pod to restart
//   4. Call discover_workflows through same AF MCP session
//   5. Assert transparent recovery (pool Ping detected dead session, evicted,
//      factory reconnected to restarted KA)
//
// NIST 800-53 Controls:
//   SI-4  — proactive health check detects failure
//   SC-24 — fail in known state (evict, don't use dead session)
//   CP-10 — auto-reconstitute without operator intervention
//
// Pyramid Invariant:
//   UT proves Ping-on-Acquire logic (session_pool_test.go)
//   IT proves wiring through PooledMCPClient (investigation_session_handoff_test.go)
//   E2E (this file) proves the journey through the full deployed stack
// =============================================================================

var _ = Describe("MCP Session Resilience (#1387)", Label("e2e", "mcp-resilience"), func() {

	var authToken string
	var mcpSessionID string

	BeforeEach(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "Failed to obtain DEX token")
		Expect(authToken).NotTo(BeEmpty())

		mcpSessionID, err = initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred(), "Failed to initialize MCP session")
		Expect(mcpSessionID).NotTo(BeEmpty())
	})

	mcpToolCallWithSession := func(rpcID, toolName string, args map[string]interface{}) ([]byte, error) {
		callBody := buildJSONRPC(rpcID, "tools/call", map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		})
		raw, code, err := mcpPOST(authToken, mcpSessionID, callBody)
		if err != nil {
			return nil, err
		}
		if code >= http.StatusBadRequest {
			return nil, fmt.Errorf("HTTP %d: %s", code, string(raw))
		}
		payload := unwrapSSEDataLine(raw)
		return []byte(payload), nil
	}

	It("E2E-AF-1387-001 [SI-4, SC-24, CP-10]: dead KA session auto-reconstituted after pod restart", func() {
		rrName := "e2e-resilience-1387-rr"

		By("Creating RR fixture for investigation")
		Expect(createRR(e2eNamespace, rrName, "test-deploy-1387")).To(Succeed())
		DeferCleanup(func() { deleteRR(e2eNamespace, rrName) })

		By("Step 1: Starting investigation to populate AF session pool")
		raw, err := mcpToolCallWithSession("res-1387-start", "kubernaut_investigate", map[string]interface{}{
			"rr_id": rrName,
		})
		Expect(err).NotTo(HaveOccurred(), "initial investigation should succeed")
		GinkgoWriter.Printf("  investigation result: %s\n", string(raw))

		var startResult map[string]interface{}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &startResult)
		}

		By("Step 2: Verifying baseline — discover_workflows callable")
		raw, err = mcpToolCallWithSession("res-1387-baseline", "kubernaut_discover_workflows", map[string]interface{}{
			"rr_id": rrName,
		})
		Expect(err).NotTo(HaveOccurred(), "baseline discover_workflows should succeed before pod kill")
		GinkgoWriter.Printf("  baseline discover_workflows: %s\n", string(raw))

		By("Step 3: Killing KA pod to invalidate cached transport session")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		deleteCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"-n", e2eNamespace, "delete", "pod", "-l", "app.kubernetes.io/name=kubernaut-agent",
			"--grace-period=0", "--force")
		deleteCmd.Stdout = GinkgoWriter
		deleteCmd.Stderr = GinkgoWriter
		Expect(deleteCmd.Run()).To(Succeed(), "kubectl delete pod should succeed")

		By("Step 4: Waiting for KA pod to restart and become ready")
		waitCtx, waitCancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer waitCancel()

		rolloutCmd := exec.CommandContext(waitCtx, "kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "status", "deployment/kubernaut-agent", "-n", e2eNamespace,
			"--timeout=120s")
		rolloutCmd.Stdout = GinkgoWriter
		rolloutCmd.Stderr = GinkgoWriter
		Expect(rolloutCmd.Run()).To(Succeed(), "KA deployment should become ready after pod restart")

		time.Sleep(5 * time.Second)

		By("Step 5: Calling discover_workflows — pool should detect dead session and auto-reconnect")
		Eventually(func(g Gomega) {
			raw, err = mcpToolCallWithSession("res-1387-after-kill", "kubernaut_discover_workflows", map[string]interface{}{
				"rr_id": rrName,
			})
			g.Expect(err).NotTo(HaveOccurred(),
				"CP-10: discover_workflows must succeed after KA pod restart — pool auto-reconstituted session")
		}, 30*time.Second, 3*time.Second).Should(Succeed(),
			"SI-4+SC-24+CP-10: Ping-on-Acquire must detect dead cached session, evict it, and transparently create a fresh session to the restarted KA")

		GinkgoWriter.Printf("  post-recovery discover_workflows: %s\n", string(raw))
	})
})
