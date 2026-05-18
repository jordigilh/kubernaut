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

package fullpipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-FP-MCP: Interactive MCP lifecycle through the full remediation pipeline.
//
// Each test is fully self-contained: creates its own RR (direct CRD), SA, and
// MCP session. Tests run in parallel via Ginkgo --procs. Only FP-MCP-003a
// requires the real OOMKill pipeline path; all others use direct RR creation.
//
// BR: BR-INTERACTIVE-001, BR-INTERACTIVE-004, BR-INTERACTIVE-005, BR-INTERACTIVE-007

// ── FP-MCP-003a: Pipeline-triggered RR status ──

var _ = Describe("FP-MCP-003a: status of pipeline-triggered RR", Label("e2e", "fullpipeline", "interactive", "mcp"), func() {
	It("should return autonomous or not_found for a real OOMKill RR", func() {
		By("Creating test namespace and deploying OOMKill trigger")
		testNS := fmt.Sprintf("fp-mcp-003a-%d", time.Now().Unix())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testNS,
				Labels: map[string]string{"kubernaut.ai/managed": "true"},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNS}}) }()

		Expect(infrastructure.DeployMemoryEater(ctx, testNS, kubeconfigPath, GinkgoWriter)).To(Succeed())

		By("Waiting for OOMKill event")
		Eventually(func() bool {
			pods := &corev1.PodList{}
			if err := apiReader.List(ctx, pods, client.InNamespace(testNS),
				client.MatchingLabels{"app": "memory-eater"}); err != nil {
				return false
			}
			for _, pod := range pods.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					if (cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled") ||
						(cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled") ||
						(cs.RestartCount > 0 && cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff") {
						return true
					}
				}
			}
			return false
		}, 2*time.Minute, 2*time.Second).Should(BeTrue(), "memory-eater should OOMKill")

		By("Waiting for RemediationRequest created by Gateway")
		var rr *remediationv1.RemediationRequest
		Eventually(func() bool {
			rrList := &remediationv1.RemediationRequestList{}
			if err := apiReader.List(ctx, rrList, client.InNamespace(namespace)); err != nil {
				return false
			}
			for i := range rrList.Items {
				if rrList.Items[i].Spec.TargetResource.Namespace == testNS {
					rr = &rrList.Items[i]
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "Gateway should create RR for OOMKill")

		By("Setting up MCP session")
		setup, err := infrastructure.SetupMCPSession(ctx, namespace, "fp-mcp-003a-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer setup.Cleanup()

		By("Checking status via MCP")
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()
		result, err := infrastructure.CallInvestigate(callCtx, setup.Session, map[string]any{
			"rr_id":  rr.Name,
			"action": "status",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "status should not return error")

		text := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  Status response: %s\n", text)

		var outer map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &outer)).To(Succeed())
		responseStr, ok := outer["response"].(string)
		Expect(ok).To(BeTrue(), "status result should have a response field")
		var inner map[string]interface{}
		Expect(json.Unmarshal([]byte(responseStr), &inner)).To(Succeed())
		Expect(inner["mode"]).To(SatisfyAny(
			Equal("autonomous"),
			Equal("not_found"),
		), "RR should be autonomous (or already completed)")
	})
})

// ── FP-MCP-001: Full interactive lifecycle ──

var _ = Describe("FP-MCP-001: full interactive lifecycle", Label("e2e", "fullpipeline", "interactive", "mcp"), func() {
	It("should takeover, send message, complete, and show not_found status", func() {
		By("Creating direct RR")
		rrName, err := infrastructure.CreateDirectRR(ctx, namespace, "fp-mcp-001")
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Printf("  Created RR: %s\n", rrName)

		By("Setting up MCP session")
		setup, err := infrastructure.SetupMCPSession(ctx, namespace, "fp-mcp-001-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer setup.Cleanup()

		By("Takeover")
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()
		result, err := infrastructure.CallInvestigate(callCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "takeover",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "takeover should succeed")
		Expect(infrastructure.ExtractToolResultText(result)).To(ContainSubstring("takeover_started"))

		By("Sending message")
		msgCtx, msgCancel := context.WithTimeout(ctx, 60*time.Second)
		defer msgCancel()
		result, err = infrastructure.CallInvestigate(msgCtx, setup.Session, map[string]any{
			"rr_id":   rrName,
			"action":  "message",
			"message": "What caused this issue? Show me the resource limits.",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "message should succeed")
		Expect(infrastructure.ExtractToolResultText(result)).NotTo(BeEmpty())

		By("Completing session")
		completeCtx, completeCancel := context.WithTimeout(ctx, 30*time.Second)
		defer completeCancel()
		result, err = infrastructure.CallInvestigate(completeCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "complete",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "complete should succeed")
		Expect(infrastructure.ExtractToolResultText(result)).To(ContainSubstring("completed"))

		By("Verifying status returns not_found")
		statusCtx, statusCancel := context.WithTimeout(ctx, 30*time.Second)
		defer statusCancel()
		result, err = infrastructure.CallInvestigate(statusCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "status",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse())

		text := infrastructure.ExtractToolResultText(result)
		var outer map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &outer)).To(Succeed())
		responseStr, ok := outer["response"].(string)
		Expect(ok).To(BeTrue())
		var inner map[string]interface{}
		Expect(json.Unmarshal([]byte(responseStr), &inner)).To(Succeed())
		Expect(inner["mode"]).To(Equal("not_found"))
	})
})

// ── FP-MCP-006: CRD observability (InteractiveSession + CompletedAt) ──

var _ = Describe("FP-MCP-006: CRD InteractiveSession and CompletedAt", Label("e2e", "fullpipeline", "interactive", "mcp"), func() {
	It("should populate InteractiveSession after takeover and CompletedAt after complete", func() {
		// MCP session setup BEFORE RR creation minimizes the window between
		// AA entering Investigating and the takeover call. With a fast mock LLM
		// the autonomous investigation completes in ~200ms; the AA controller's
		// predicate-triggered re-reconcile can poll KA and see "completed"
		// before ForceTransitionToUserDriving runs. Pre-creating the MCP session
		// removes ~3s of SA/RBAC/connect overhead from that critical window.
		By("Setting up MCP session (pre-RR creation to minimize race window)")
		setup, err := infrastructure.SetupMCPSession(ctx, namespace, "fp-mcp-006-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer setup.Cleanup()

		By("Creating direct RR")
		rrName, err := infrastructure.CreateDirectRR(ctx, namespace, "fp-mcp-006")
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for AIAnalysis to reach Investigating phase")
		var aaName string
		Eventually(func() bool {
			aaList := &aianalysisv1.AIAnalysisList{}
			if err := apiReader.List(ctx, aaList, client.InNamespace(namespace)); err != nil {
				return false
			}
			for _, aa := range aaList.Items {
				if aa.Spec.RemediationRequestRef.Name == rrName {
					aaName = aa.Name
					return string(aa.Status.Phase) == "Investigating"
				}
			}
			return false
		}, timeout, 1*time.Second).Should(BeTrue(), "AIAnalysis should reach Investigating phase for RR")

		By("Takeover (immediately — MCP session already set up, beat the first poll)")
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()
		result, err := infrastructure.CallInvestigate(callCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "takeover",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "takeover should succeed")

		By("Waiting for InteractiveSession to be populated on AA CRD")
		aa := &aianalysisv1.AIAnalysis{}
		Eventually(func(g Gomega) {
			keepAliveCtx, keepAliveCancel := context.WithTimeout(ctx, 5*time.Second)
			defer keepAliveCancel()
			_, _ = infrastructure.CallInvestigate(keepAliveCtx, setup.Session, map[string]any{
				"rr_id":  rrName,
				"action": "status",
			})

			g.Expect(apiReader.Get(ctx, client.ObjectKey{Name: aaName, Namespace: namespace}, aa)).To(Succeed())
			g.Expect(aa.Status.InteractiveSession).NotTo(BeNil(),
				"BR-007: InteractiveSession must be populated after takeover")
		}, 90*time.Second, 5*time.Second).Should(Succeed())

		GinkgoWriter.Printf("  InteractiveSession: driver=%s, sessionID=%s\n",
			aa.Status.InteractiveSession.ActingUser, aa.Status.InteractiveSession.SessionID)
		Expect(aa.Status.InteractiveSession.ActingUser).NotTo(BeEmpty())
		Expect(aa.Status.InteractiveSession.SessionID).NotTo(BeEmpty())

		By("Completing session")
		completeCtx, completeCancel := context.WithTimeout(ctx, 30*time.Second)
		defer completeCancel()
		result, err = infrastructure.CallInvestigate(completeCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "complete",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "complete should succeed")

		By("Waiting for CompletedAt to be populated on AA CRD")
		Eventually(func(g Gomega) {
			g.Expect(apiReader.Get(ctx, client.ObjectKey{Name: aaName, Namespace: namespace}, aa)).To(Succeed())
			g.Expect(aa.Status.InteractiveSession).NotTo(BeNil())
			g.Expect(aa.Status.InteractiveSession.CompletedAt).NotTo(BeNil(),
				"BR-007: CompletedAt must be set after interactive complete")
		}, 90*time.Second, 2*time.Second).Should(Succeed())

		GinkgoWriter.Printf("  CompletedAt: %s\n", aa.Status.InteractiveSession.CompletedAt.Time)
	})
})

// ── FP-MCP-008: Re-takeover after proxy disconnect ──

var _ = Describe("FP-MCP-008: re-takeover after proxy disconnect", Label("e2e", "fullpipeline", "interactive", "mcp"), func() {
	It("should recover session via fresh takeover after network partition", func() {
		By("Creating direct RR")
		rrName, err := infrastructure.CreateDirectRR(ctx, namespace, "fp-mcp-008")
		Expect(err).NotTo(HaveOccurred())

		By("Setting up TLS transport")
		tlsTransport, err := infrastructure.NewTLSAwareTransport(kubeconfigPath)
		Expect(err).NotTo(HaveOccurred())

		By("Creating SA token")
		saToken, err := infrastructure.CreateInteractiveE2ESA(ctx, namespace, "fp-mcp-008-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())

		By("Creating TCP proxy to KA NodePort")
		proxy, err := infrastructure.NewInterruptibleProxy("localhost:8088")
		Expect(err).NotTo(HaveOccurred())
		defer proxy.Close()

		proxyEndpoint := fmt.Sprintf("https://%s/api/v1/mcp", proxy.Addr())

		By("Connecting MCP through proxy and acquiring session")
		proxiedSession, err := infrastructure.ConnectMCPClientWithRetry(ctx, infrastructure.MCPClientConfig{
			Endpoint:     proxyEndpoint,
			SAToken:      saToken,
			TLSTransport: tlsTransport,
		}, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred(), "MCP connect through proxy")

		takeoverCtx, takeoverCancel := context.WithTimeout(ctx, 30*time.Second)
		defer takeoverCancel()
		result, err := infrastructure.CallInvestigate(takeoverCtx, proxiedSession, map[string]any{
			"rr_id":  rrName,
			"action": "takeover",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "takeover via proxy should succeed")

		By("Disconnecting all proxy connections (simulates network partition)")
		proxy.DisconnectAll()
		_ = proxiedSession.Close()

		By("Creating new direct MCP session")
		directSession, err := infrastructure.ConnectMCPClientWithRetry(ctx, infrastructure.MCPClientConfig{
			Endpoint:     infrastructure.MCPEndpointForKAE2E(),
			SAToken:      saToken,
			TLSTransport: tlsTransport,
		}, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = directSession.Close() }()

		By("Re-acquiring session via fresh takeover (with retry for disconnect handler)")
		Eventually(func(g Gomega) {
			retakeoverCtx, retakeoverCancel := context.WithTimeout(ctx, 10*time.Second)
			defer retakeoverCancel()
			result, err = infrastructure.CallInvestigate(retakeoverCtx, directSession, map[string]any{
				"rr_id":  rrName,
				"action": "takeover",
			})
			g.Expect(err).NotTo(HaveOccurred())
			text := infrastructure.ExtractToolResultText(result)
			GinkgoWriter.Printf("  Re-takeover response (isError=%v): %s\n", result.IsError, text)
			g.Expect(result.IsError).To(BeFalse(), "re-takeover after partition should succeed; got: %s", text)
			g.Expect(text).To(SatisfyAny(
				ContainSubstring("takeover_started"),
				ContainSubstring("reconnected"),
			))
		}, 15*time.Second, 2*time.Second).Should(Succeed())
	})
})

// ── FP-MCP-009: Concurrent takeover contention ──

var _ = Describe("FP-MCP-009: concurrent takeover contention", Label("e2e", "fullpipeline", "interactive", "mcp"), func() {
	It("should reject second user's takeover while first user holds the session", func() {
		By("Creating direct RR")
		rrName, err := infrastructure.CreateDirectRR(ctx, namespace, "fp-mcp-009")
		Expect(err).NotTo(HaveOccurred())

		By("Setting up User A session")
		userASetup, err := infrastructure.SetupMCPSession(ctx, namespace, "fp-mcp-009-a-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer userASetup.Cleanup()

		By("User A takes over")
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()
		result, err := infrastructure.CallInvestigate(callCtx, userASetup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "takeover",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "User A takeover should succeed")

		By("Setting up User B session")
		userBSetup, err := infrastructure.SetupMCPSession(ctx, namespace, "fp-mcp-009-b-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer userBSetup.Cleanup()

		By("User B attempts takeover on same RR — should be rejected")
		userBCtx, userBCancel := context.WithTimeout(ctx, 30*time.Second)
		defer userBCancel()
		result, err = infrastructure.CallInvestigate(userBCtx, userBSetup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "takeover",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeTrue(), "takeover by User B should fail — session active for User A")

		text := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  Contention response: %s\n", text)
		Expect(text).To(SatisfyAny(
			ContainSubstring("session_active"),
			ContainSubstring("max_sessions"),
		), "takeover must be rejected with session_active (lease held) or max_sessions (pool exhausted)")
	})
})

// ── FP-MCP-005: Workflow discovery and selection ──

var _ = Describe("FP-MCP-005: discover_workflows and select_workflow", Label("e2e", "fullpipeline", "interactive", "mcp"), func() {
	It("should discover workflows and select if catalog matches", func() {
		By("Creating direct RR")
		rrName, err := infrastructure.CreateDirectRR(ctx, namespace, "fp-mcp-005")
		Expect(err).NotTo(HaveOccurred())

		By("Setting up MCP session")
		setup, err := infrastructure.SetupMCPSession(ctx, namespace, "fp-mcp-005-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer setup.Cleanup()

		By("Takeover")
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()
		result, err := infrastructure.CallInvestigate(callCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "takeover",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "takeover should succeed")

		By("Calling discover_workflows")
		discoverCtx, discoverCancel := context.WithTimeout(ctx, 60*time.Second)
		defer discoverCancel()
		result, err = infrastructure.CallInvestigate(discoverCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "discover_workflows",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "discover_workflows should succeed")

		text := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  DiscoverWorkflows response (first 300 chars): %.300s\n", text)
		Expect(text).To(ContainSubstring("workflows_discovered"))

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(text), &parsed); err != nil {
			GinkgoWriter.Printf("  Could not parse discover response: %v\n", err)
			return
		}
		resp, ok := parsed["response"].(string)
		if !ok || resp == "{}" || resp == "" {
			GinkgoWriter.Printf("  Discovery returned empty workflow set (mock LLM RCA did not match catalog) — skipping select_workflow\n")
			return
		}

		By("Calling select_workflow with discovered workflow")
		Expect(workflowUUIDs).To(HaveKey("oomkill-increase-memory-v1:production"),
			"workflow catalog must be seeded")
		workflowID := workflowUUIDs["oomkill-increase-memory-v1:production"]

		selectCtx, selectCancel := context.WithTimeout(ctx, 30*time.Second)
		defer selectCancel()
		result, err = infrastructure.CallSelectWorkflow(selectCtx, setup.Session, map[string]any{
			"rr_id":       rrName,
			"workflow_id": workflowID,
		})
		Expect(err).NotTo(HaveOccurred())
		selectText := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  SelectWorkflow response (isError=%v): %.300s\n", result.IsError, selectText)
		Expect(result.IsError).To(BeFalse(), "select_workflow should succeed; got: %s", selectText)
		Expect(selectText).To(ContainSubstring("workflow_selected"))
	})
})

// ── FP-MCP-002: Fresh interactive session (AF-style) ──

var _ = Describe("FP-MCP-002: AF-style fresh start lifecycle", Label("e2e", "fullpipeline", "interactive", "mcp"), func() {
	It("should create RR directly and run start -> message -> complete", func() {
		By("Creating direct RR")
		rrName, err := infrastructure.CreateDirectRR(ctx, namespace, "fp-mcp-002")
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Printf("  Created RR: %s\n", rrName)

		By("Setting up MCP session")
		setup, err := infrastructure.SetupMCPSession(ctx, namespace, "fp-mcp-002-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer setup.Cleanup()

		By("Starting interactive session")
		startCtx, startCancel := context.WithTimeout(ctx, 30*time.Second)
		defer startCancel()
		result, err := infrastructure.CallInvestigate(startCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "start",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "start should succeed")
		text := infrastructure.ExtractToolResultText(result)
		Expect(text).To(SatisfyAny(
			ContainSubstring("started"),
			ContainSubstring("takeover_started"),
		))

		By("Sending message")
		msgCtx, msgCancel := context.WithTimeout(ctx, 60*time.Second)
		defer msgCancel()
		result, err = infrastructure.CallInvestigate(msgCtx, setup.Session, map[string]any{
			"rr_id":   rrName,
			"action":  "message",
			"message": "What resources are affected?",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "message should succeed")
		Expect(infrastructure.ExtractToolResultText(result)).NotTo(BeEmpty())

		By("Completing session")
		completeCtx, completeCancel := context.WithTimeout(ctx, 30*time.Second)
		defer completeCancel()
		result, err = infrastructure.CallInvestigate(completeCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "complete",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "complete should succeed")
		Expect(infrastructure.ExtractToolResultText(result)).To(ContainSubstring("completed"))
	})
})

// ── FP-MCP-005c: complete_no_action ──

var _ = Describe("FP-MCP-005c: complete_no_action through full pipeline", Label("e2e", "fullpipeline", "interactive", "mcp"), func() {
	It("should complete with no workflow and session should be released", func() {
		By("Creating direct RR")
		rrName, err := infrastructure.CreateDirectRR(ctx, namespace, "fp-mcp-005c")
		Expect(err).NotTo(HaveOccurred())

		By("Setting up MCP session")
		setup, err := infrastructure.SetupMCPSession(ctx, namespace, "fp-mcp-005c-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		defer setup.Cleanup()

		By("Starting interactive session")
		startCtx, startCancel := context.WithTimeout(ctx, 30*time.Second)
		defer startCancel()
		result, err := infrastructure.CallInvestigate(startCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"action": "start",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse())

		By("Sending a message")
		msgCtx, msgCancel := context.WithTimeout(ctx, 60*time.Second)
		defer msgCancel()
		result, err = infrastructure.CallInvestigate(msgCtx, setup.Session, map[string]any{
			"rr_id":   rrName,
			"action":  "message",
			"message": "This looks like a transient issue, no action needed",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse())

		By("Calling complete_no_action")
		cnaCtx, cnaCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cnaCancel()
		result, err = infrastructure.CallCompleteNoAction(cnaCtx, setup.Session, map[string]any{
			"rr_id":  rrName,
			"reason": "transient issue, no remediation needed",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.IsError).To(BeFalse(), "complete_no_action should succeed")
		Expect(infrastructure.ExtractToolResultText(result)).To(ContainSubstring("completed_no_action"))

		By("Verifying session released (status returns not_found)")
		Eventually(func(g Gomega) {
			pollCtx, pollCancel := context.WithTimeout(ctx, 10*time.Second)
			defer pollCancel()
			statusResult, statusErr := infrastructure.CallInvestigate(pollCtx, setup.Session, map[string]any{
				"rr_id":  rrName,
				"action": "status",
			})
			g.Expect(statusErr).NotTo(HaveOccurred())
			g.Expect(statusResult.IsError).To(BeFalse())

			statusText := infrastructure.ExtractToolResultText(statusResult)
			var outer map[string]interface{}
			g.Expect(json.Unmarshal([]byte(statusText), &outer)).To(Succeed())
			responseStr, ok := outer["response"].(string)
			g.Expect(ok).To(BeTrue(), "status result should have a response field")
			var inner map[string]interface{}
			g.Expect(json.Unmarshal([]byte(responseStr), &inner)).To(Succeed())
			g.Expect(inner["mode"]).To(Equal("not_found"),
				"session should be released after complete_no_action")
		}, 15*time.Second, 1*time.Second).Should(Succeed())
	})
})
