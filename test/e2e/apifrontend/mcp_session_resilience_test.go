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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	investigationsessionv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
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
//
// Serial (Issue #2165): Step 3 deletes ALL kubernaut-agent pods cluster-wide
// (label selector, no name scoping) and Step 4 waits up to 120s for the
// Deployment to roll back to Ready -- KA is unavailable to every other spec
// for that entire window. Since apifrontend E2E tests run with --procs > 1
// (true parallel Ginkgo OS processes sharing the same deployed KA), any spec
// in ANY other process that calls kubernaut_investigate/discover_workflows
// during that window can have its in-flight KA session torn down mid-call,
// starving AF's grounding-guard of investigation content and forcing its
// documented fail-closed fallback (observed: E2E-AF-1395-001 failing with a
// truncated 431-char payload instead of the expected >512, CI run
// 31948595609/job 95170442227 -- must-gather confirmed the structured-decision
// test's own groundSession() call returned summary_len:0/channel_closed
// squarely inside this spec's pod-kill-to-rollout-complete window). This is
// the same "process-wide shared state, non-Serial spec" collision class
// already fixed once in this package for the connection-tracker cap test
// (streaming_test.go's Serial Describe, TC-E2E-STREAM-04) -- Serial is
// Ginkgo's authoritative mechanism for it: it guarantees no other spec runs
// on any parallel process while this one executes
// (https://onsi.github.io/ginkgo/#serial-specs), eliminating the collision
// window at its source rather than just reducing its odds.
// =============================================================================

var _ = Describe("MCP Session Resilience (#1387)", Serial, Label("e2e", "mcp-resilience"), func() {

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

	It("E2E-AF-KA-168-001 [BR-INTERACTIVE-004, BR-INTERACTIVE-010]: durable AF owner survives KA Lease reclaim and restart", func() {
		ctx := context.Background()
		rrName := fmt.Sprintf("e2e-af-ka-168-%d", time.Now().UnixNano())
		leaseName := "kubernaut-interactive-" + rrName
		userA, err := auditOIDCSubject(authToken)
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token must expose the authenticated subject")

		Expect(createRR(e2eNamespace, rrName, "test-deploy-af-ka-168")).To(Succeed())
		var (
			ownerSessionName        string
			mcpSessionAAfterRestart string
		)
		DeferCleanup(func() {
			cleanupSessionID := mcpSessionAAfterRestart
			if cleanupSessionID == "" {
				cleanupSessionID = mcpSessionID
			}
			if cleanupSessionID != "" {
				cleanupBody := buildJSONRPC("af-ka-168-cleanup", "tools/call", map[string]interface{}{
					"name": "kubernaut_complete",
					"arguments": map[string]interface{}{
						"rr_id": rrName,
					},
				})
				cleanupRaw, _, cleanupErr := mcpPOST(authToken, cleanupSessionID, cleanupBody)
				if cleanupErr != nil {
					GinkgoWriter.Printf("  cleanup kubernaut_complete failed: %v\n", cleanupErr)
				} else {
					GinkgoWriter.Printf("  cleanup kubernaut_complete result: %s\n", string(cleanupRaw))
				}
			}
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := clientset.CoordinationV1().Leases(e2eNamespace).Delete(cleanupCtx, leaseName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				GinkgoWriter.Printf("  cleanup Lease %s/%s deletion failed: %v\n", e2eNamespace, leaseName, err)
			}
			if ownerSessionName != "" {
				if err := k8sClient.Delete(cleanupCtx, &investigationsessionv1alpha1.InvestigationSession{
					ObjectMeta: metav1.ObjectMeta{Name: ownerSessionName, Namespace: e2eNamespace},
				}); err != nil && !apierrors.IsNotFound(err) {
					GinkgoWriter.Printf("  cleanup InvestigationSession %s/%s deletion failed: %v\n", e2eNamespace, ownerSessionName, err)
				}
			}
			deleteRR(e2eNamespace, rrName)
		})

		callInvestigate := func(token, sessionID, rpcID string) ([]byte, int, error) {
			body := buildJSONRPC(rpcID, "tools/call", map[string]interface{}{
				"name": "kubernaut_investigate",
				"arguments": map[string]interface{}{
					"rr_id": rrName,
				},
			})
			return mcpPOST(token, sessionID, body)
		}

		listNonTerminalSessions := func() ([]investigationsessionv1alpha1.InvestigationSession, error) {
			var list investigationsessionv1alpha1.InvestigationSessionList
			if err := k8sClient.List(ctx, &list, client.InNamespace(e2eNamespace)); err != nil {
				return nil, err
			}
			matches := make([]investigationsessionv1alpha1.InvestigationSession, 0, 1)
			for i := range list.Items {
				item := list.Items[i]
				if item.Spec.RemediationRequestRef.Name != rrName {
					continue
				}
				if item.Status.Phase == investigationsessionv1alpha1.SessionPhaseCompleted ||
					item.Status.Phase == investigationsessionv1alpha1.SessionPhaseCancelled ||
					item.Status.Phase == investigationsessionv1alpha1.SessionPhaseFailed {
					continue
				}
				matches = append(matches, item)
			}
			return matches, nil
		}

		By("Step 1: User A starts the deployed AF → KA investigation path")
		raw, statusCode, err := callInvestigate(authToken, mcpSessionID, "af-ka-168-start-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(statusCode).To(BeNumerically("<", http.StatusInternalServerError),
			"User A's investigation must reach the deployed AF/KA path: %s", string(raw))

		By("Step 2: Verifying one durable InvestigationSession owned by User A and an ephemeral KA Lease")
		Eventually(func(g Gomega) {
			matches, listErr := listNonTerminalSessions()
			g.Expect(listErr).NotTo(HaveOccurred())
			g.Expect(matches).To(HaveLen(1), "User A must have exactly one non-terminal durable owner")
			if len(matches) == 0 {
				return
			}
			ownerSessionName = matches[0].Name
			g.Expect(matches[0].Spec.UserIdentity.Username).To(Equal(userA))
		}, 60*time.Second, 2*time.Second).Should(Succeed(),
			"User A's durable InvestigationSession must become active")

		var initialHolder string
		Eventually(func(g Gomega) {
			lease, getErr := clientset.CoordinationV1().Leases(e2eNamespace).Get(ctx, leaseName, metav1.GetOptions{})
			g.Expect(getErr).NotTo(HaveOccurred())
			g.Expect(lease.Spec.HolderIdentity).NotTo(BeNil())
			if lease.Spec.HolderIdentity == nil {
				return
			}
			initialHolder = *lease.Spec.HolderIdentity
			g.Expect(initialHolder).NotTo(BeEmpty())
		}, 60*time.Second, 2*time.Second).Should(Succeed(),
			"User A's KA Lease must be observable before the restart")

		By("Step 3: Backdating the real KA Lease so startup reconciliation can reclaim it")
		lease, err := clientset.CoordinationV1().Leases(e2eNamespace).Get(ctx, leaseName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		expiredAcquire := metav1.NewMicroTime(time.Now().Add(-2 * time.Hour))
		lease.Spec.AcquireTime = &expiredAcquire
		_, err = clientset.CoordinationV1().Leases(e2eNamespace).Update(ctx, lease, metav1.UpdateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("Step 4: Restarting KA and waiting for the fresh deployment epoch")
		restartCtx, restartCancel := context.WithTimeout(ctx, 30*time.Second)
		defer restartCancel()
		deleteCmd := exec.CommandContext(restartCtx, "kubectl", "--kubeconfig", kubeconfigPath,
			"-n", e2eNamespace, "delete", "pod", "-l", "app.kubernetes.io/name=kubernaut-agent",
			"--grace-period=0", "--force")
		deleteCmd.Stdout = GinkgoWriter
		deleteCmd.Stderr = GinkgoWriter
		Expect(deleteCmd.Run()).To(Succeed(), "kubectl delete KA pod should succeed")

		rolloutCtx, rolloutCancel := context.WithTimeout(ctx, 120*time.Second)
		defer rolloutCancel()
		rolloutCmd := exec.CommandContext(rolloutCtx, "kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "status", "deployment/kubernaut-agent", "-n", e2eNamespace, "--timeout=120s")
		rolloutCmd.Stdout = GinkgoWriter
		rolloutCmd.Stderr = GinkgoWriter
		Expect(rolloutCmd.Run()).To(Succeed(), "KA deployment should become ready after restart")

		By("Step 5: Proving the fresh KA epoch reclaimed the expired orphan Lease")
		Eventually(func(g Gomega) {
			_, getErr := clientset.CoordinationV1().Leases(e2eNamespace).Get(ctx, leaseName, metav1.GetOptions{})
			g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue(),
				"startup reconciliation must delete the expired KA Lease before AF re-entry")
		}, 60*time.Second, 2*time.Second).Should(Succeed())

		By("Step 6: User B re-enters through AF and is denied by User A's durable owner")
		tokenB, err := fetchDEXTokenForPersona("ai-orchestrator")
		Expect(err).NotTo(HaveOccurred())
		mcpSessionB, err := initMCPSession(tokenB)
		Expect(err).NotTo(HaveOccurred())
		raw, statusCode, err = callInvestigate(tokenB, mcpSessionB, "af-ka-168-start-b")
		Expect(err).NotTo(HaveOccurred())
		Expect(statusCode).To(BeNumerically("<", http.StatusInternalServerError),
			"AF must return the durable-owner denial as a structured MCP result: %s", string(raw))
		Expect(strings.ToLower(string(raw))).To(ContainSubstring("session_active"),
			"User B must be denied by AF's durable InvestigationSession owner")
		_, err = clientset.CoordinationV1().Leases(e2eNamespace).Get(ctx, leaseName, metav1.GetOptions{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(),
			"AF must deny User B before KA creates a replacement Lease")

		By("Step 7: User A reacquires through a fresh AF MCP session")
		mcpSessionAAfterRestart, err = initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred())
		raw, statusCode, err = callInvestigate(authToken, mcpSessionAAfterRestart, "af-ka-168-reacquire-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(statusCode).To(BeNumerically("<", http.StatusInternalServerError),
			"User A must reacquire after KA restart: %s", string(raw))

		By("Step 8: Verifying User A has a new KA Lease and remains the sole durable owner")
		var reacquiredHolder string
		Eventually(func(g Gomega) {
			lease, getErr := clientset.CoordinationV1().Leases(e2eNamespace).Get(ctx, leaseName, metav1.GetOptions{})
			g.Expect(getErr).NotTo(HaveOccurred())
			g.Expect(lease.Spec.HolderIdentity).NotTo(BeNil())
			if lease.Spec.HolderIdentity == nil {
				return
			}
			reacquiredHolder = *lease.Spec.HolderIdentity
			g.Expect(reacquiredHolder).NotTo(BeEmpty())
			g.Expect(reacquiredHolder).NotTo(Equal(initialHolder),
				"reacquisition must use a fresh KA Lease holder after restart")
		}, 60*time.Second, 2*time.Second).Should(Succeed())

		Eventually(func(g Gomega) {
			matches, listErr := listNonTerminalSessions()
			g.Expect(listErr).NotTo(HaveOccurred())
			g.Expect(matches).To(HaveLen(1), "Lease reclaim/reacquisition must not create a second durable owner")
			if len(matches) == 0 {
				return
			}
			g.Expect(matches[0].Name).To(Equal(ownerSessionName))
			g.Expect(matches[0].Spec.UserIdentity.Username).To(Equal(userA))
		}, 60*time.Second, 2*time.Second).Should(Succeed(),
			"User A's original durable InvestigationSession must remain the sole non-terminal owner")
	})
})
