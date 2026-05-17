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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// E2E-FP-MCP: Interactive MCP lifecycle through the full remediation pipeline.
//
// Validates v1.5 MCP functionality end-to-end:
//   - FP-MCP-001: OOMKill -> autonomous -> takeover -> message -> complete
//   - FP-MCP-002: Fresh interactive session (AF-style direct RR creation)
//   - FP-MCP-003: Status lifecycle (autonomous -> not_found) via MCP
//   - FP-MCP-005a: discover_workflows before select_workflow
//   - FP-MCP-005b: select_workflow with real DS catalog (requires prior discover_workflows)
//   - FP-MCP-005c: complete_no_action through full pipeline (AA routes to Completed/WorkflowNotNeeded)
//   - FP-MCP-006: BR-007 CRD observability (InteractiveSession + CompletedAt)
//   - FP-MCP-008: Reconnect via TCP proxy disconnect
//   - FP-MCP-009: Concurrent takeover contention (second user rejected)
//
// Uses a single OOMKill trigger shared across ordered specs for efficiency.
// BR: BR-INTERACTIVE-001, BR-INTERACTIVE-004, BR-INTERACTIVE-005, BR-INTERACTIVE-007
var _ = Describe("CP-5 MCP Interactive Lifecycle — Full Pipeline", Label("e2e", "fullpipeline", "interactive", "mcp"), Ordered, ContinueOnFailure, func() {

	var (
		testNamespace       string
		mcpSession          *mcpsdk.ClientSession
		interactiveSAToken  string
		contentionSAToken   string
		tlsTransport        http.RoundTripper
		remediationRequest  *remediationv1.RemediationRequest
		aaName              string

		takeoverDone          bool
		discoverWorkflowsDone bool
		sessionCompleted      bool
	)

	BeforeAll(func() {
		Expect(ctx).NotTo(BeNil(), "suite ctx must be initialized")
		Expect(kubeconfigPath).NotTo(BeEmpty(), "kubeconfigPath must be set")

		By("Creating interactive SA with KA client + Lease RBAC")
		var err error
		interactiveSAToken, err = infrastructure.CreateInteractiveE2ESA(
			ctx, namespace, "fp-mcp-interactive-sa", kubeconfigPath, GinkgoWriter,
		)
		Expect(err).NotTo(HaveOccurred(), "interactive SA creation")

		By("Creating contention SA for FP-MCP-009 (second user)")
		contentionSAToken, err = infrastructure.CreateInteractiveE2ESA(
			ctx, namespace, "fp-mcp-contention-sa", kubeconfigPath, GinkgoWriter,
		)
		Expect(err).NotTo(HaveOccurred(), "contention SA creation")

		By("Setting up TLS transport for MCP")
		tlsTransport, err = infrastructure.NewTLSAwareTransport(kubeconfigPath)
		Expect(err).NotTo(HaveOccurred(), "TLS transport for MCP")

		By("Creating test namespace for OOMKill trigger")
		testNamespace = fmt.Sprintf("fp-mcp-lifecycle-%d", time.Now().Unix())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testNamespace,
				Labels: map[string]string{"kubernaut.ai/managed": "true"},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		By("Deploying memory-eater pod (triggers OOMKill -> pipeline)")
		Expect(infrastructure.DeployMemoryEater(ctx, testNamespace, kubeconfigPath, GinkgoWriter)).To(Succeed())

		By("Waiting for OOMKill event")
		Eventually(func() bool {
			pods := &corev1.PodList{}
			if err := apiReader.List(ctx, pods, client.InNamespace(testNamespace),
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
		Eventually(func() bool {
			rrList := &remediationv1.RemediationRequestList{}
			if err := apiReader.List(ctx, rrList, client.InNamespace(namespace)); err != nil {
				return false
			}
			for i := range rrList.Items {
				rr := &rrList.Items[i]
				if rr.Spec.TargetResource.Namespace == testNamespace {
					remediationRequest = rr
					GinkgoWriter.Printf("  Found RR: %s\n", rr.Name)
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "Gateway should create RR for OOMKill")

		By("Waiting for AIAnalysis to reach Investigating phase (autonomous investigation running)")
		Eventually(func() string {
			aaList := &aianalysisv1.AIAnalysisList{}
			if err := apiReader.List(ctx, aaList, client.InNamespace(namespace)); err != nil {
				return ""
			}
			for _, aa := range aaList.Items {
				if aa.Spec.RemediationRequestRef.Name == remediationRequest.Name {
					aaName = aa.Name
					GinkgoWriter.Printf("  AA %s phase: %s\n", aa.Name, aa.Status.Phase)
					return aa.Status.Phase
				}
			}
			return ""
		}, timeout, interval).Should(SatisfyAny(
			Equal("Investigating"),
			Equal("Analyzing"),
			Equal("Completed"),
		), "AIAnalysis should reach Investigating or later")

		By("Connecting MCP SDK client to KA")
		connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
		defer connectCancel()
		mcpSession, err = infrastructure.ConnectMCPClient(connectCtx, infrastructure.MCPClientConfig{
			Endpoint:     infrastructure.MCPEndpointForKAE2E(),
			SAToken:      interactiveSAToken,
			TLSTransport: tlsTransport,
		})
		Expect(err).NotTo(HaveOccurred(), "MCP SDK connect to KA")
	})

	AfterAll(func() {
		if mcpSession != nil {
			_ = mcpSession.Close()
		}
		if testNamespace != "" {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
			_ = k8sClient.Delete(ctx, ns)
		}
	})

	It("FP-MCP-003a: status returns autonomous or not_found for pipeline-triggered RR", func() {
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()

		result, err := infrastructure.CallInvestigate(callCtx, mcpSession, map[string]any{
			"rr_id":  remediationRequest.Name,
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
		Expect(json.Unmarshal([]byte(responseStr), &inner)).To(Succeed(),
			"response field should contain valid JSON")
		Expect(inner["mode"]).To(SatisfyAny(
			Equal("autonomous"),
			Equal("not_found"),
		), "RR should be autonomous (or already completed)")
	})

	It("FP-MCP-001: takeover pipeline-triggered RR via MCP", func() {
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()

		result, err := infrastructure.CallInvestigate(callCtx, mcpSession, map[string]any{
			"rr_id":  remediationRequest.Name,
			"action": "takeover",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "takeover should succeed")

		text := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  Takeover response: %s\n", text)
		Expect(text).To(ContainSubstring("takeover_started"))
		takeoverDone = true
	})

	It("FP-MCP-006a: CRD InteractiveSession populated after takeover", func() {
		if !takeoverDone {
			Skip("depends on FP-MCP-001 (takeover)")
		}
		aa := &aianalysisv1.AIAnalysis{}
		Eventually(func(g Gomega) {
			keepAliveCtx, keepAliveCancel := context.WithTimeout(ctx, 5*time.Second)
			defer keepAliveCancel()
			_, _ = infrastructure.CallInvestigate(keepAliveCtx, mcpSession, map[string]any{
				"rr_id":  remediationRequest.Name,
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
	})

	It("FP-MCP-008: reconnect via proxy disconnect", func() {
		if !takeoverDone {
			Skip("depends on FP-MCP-001 (takeover)")
		}
		By("Creating TCP proxy to KA NodePort")
		proxy, err := infrastructure.NewInterruptibleProxy("localhost:8088")
		Expect(err).NotTo(HaveOccurred())
		defer proxy.Close()

		proxyEndpoint := fmt.Sprintf("https://%s/api/v1/mcp", proxy.Addr())
		GinkgoWriter.Printf("  Proxy endpoint: %s\n", proxyEndpoint)

		By("Closing current MCP session (no lease release — proxy will handle disconnect)")
		if mcpSession != nil {
			_ = mcpSession.Close()
			mcpSession = nil
		}

		By("Connecting MCP through proxy")
		proxyCtx, proxyCancel := context.WithTimeout(ctx, 30*time.Second)
		defer proxyCancel()
		proxiedSession, connErr := infrastructure.ConnectMCPClient(proxyCtx, infrastructure.MCPClientConfig{
			Endpoint:     proxyEndpoint,
			SAToken:      interactiveSAToken,
			TLSTransport: tlsTransport,
		})
		Expect(connErr).NotTo(HaveOccurred(), "MCP connect through proxy")

		By("Re-acquiring session via takeover through proxy")
		takeoverCtx, takeoverCancel := context.WithTimeout(ctx, 30*time.Second)
		defer takeoverCancel()
		result, takeoverErr := infrastructure.CallInvestigate(takeoverCtx, proxiedSession, map[string]any{
			"rr_id":  remediationRequest.Name,
			"action": "start",
		})
		Expect(takeoverErr).NotTo(HaveOccurred())
		takeoverText := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  Takeover via proxy: isError=%v, text=%s\n", result.IsError, takeoverText)
		Expect(result.IsError).To(BeFalse(), "start/takeover via proxy should succeed")

		By("Disconnecting all proxy connections (simulates network partition)")
		proxy.DisconnectAll()
		_ = proxiedSession.Close()

		By("Creating new direct MCP session (bypass proxy)")
		directCtx, directCancel := context.WithTimeout(ctx, 30*time.Second)
		defer directCancel()
		mcpSession, err = infrastructure.ConnectMCPClient(directCtx, infrastructure.MCPClientConfig{
			Endpoint:     infrastructure.MCPEndpointForKAE2E(),
			SAToken:      interactiveSAToken,
			TLSTransport: tlsTransport,
		})
		Expect(err).NotTo(HaveOccurred(), "direct MCP reconnect")

		By("Calling action: reconnect")
		reconCtx, reconCancel := context.WithTimeout(ctx, 30*time.Second)
		defer reconCancel()
		result, reconErr := infrastructure.CallInvestigate(reconCtx, mcpSession, map[string]any{
			"rr_id":  remediationRequest.Name,
			"action": "reconnect",
		})
		Expect(reconErr).NotTo(HaveOccurred())

		text := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  Reconnect response (isError=%v): %s\n", result.IsError, text)
		Expect(result.IsError).To(BeFalse(), "reconnect should succeed; got: %s", text)
		Expect(text).To(ContainSubstring("reconnected"))
	})

	It("FP-MCP-009: concurrent takeover contention (second user rejected)", func() {
		if !takeoverDone {
			Skip("depends on FP-MCP-001 (takeover)")
		}
		By("Waiting briefly for MCP server rate limiter to reset")
		time.Sleep(2 * time.Second) // ✅ APPROVED EXCEPTION: deliberate delay to avoid 429 rate limit

		By("Connecting User B MCP session")
		userBCtx, userBCancel := context.WithTimeout(ctx, 30*time.Second)
		defer userBCancel()
		userBSession, err := infrastructure.ConnectMCPClient(userBCtx, infrastructure.MCPClientConfig{
			Endpoint:     infrastructure.MCPEndpointForKAE2E(),
			SAToken:      contentionSAToken,
			TLSTransport: tlsTransport,
		})
		Expect(err).NotTo(HaveOccurred(), "User B MCP connect")
		defer func() { _ = userBSession.Close() }()

		By("User B attempts takeover on same RR — should be rejected")
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()
		result, callErr := infrastructure.CallInvestigate(callCtx, userBSession, map[string]any{
			"rr_id":  remediationRequest.Name,
			"action": "takeover",
		})
		Expect(callErr).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeTrue(), "takeover by User B should fail — session active for User A")

		text := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  Contention response: %s\n", text)
		Expect(text).To(ContainSubstring("session_active"))
	})

	It("FP-MCP-001: send message via MCP after takeover", func() {
		if !takeoverDone {
			Skip("depends on FP-MCP-001 (takeover)")
		}
		callCtx, callCancel := context.WithTimeout(ctx, 60*time.Second)
		defer callCancel()

		result, err := infrastructure.CallInvestigate(callCtx, mcpSession, map[string]any{
			"rr_id":   remediationRequest.Name,
			"action":  "message",
			"message": "What caused the OOMKill? Show me the pod resource limits.",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "message should succeed")

		text := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  Message response (first 200 chars): %.200s\n", text)
		Expect(text).NotTo(BeEmpty(), "LLM response must not be empty")
	})

	It("FP-MCP-005a: discover_workflows before select_workflow", func() {
		if !takeoverDone {
			Skip("depends on FP-MCP-001 (takeover)")
		}
		callCtx, callCancel := context.WithTimeout(ctx, 60*time.Second)
		defer callCancel()

		result, err := infrastructure.CallInvestigate(callCtx, mcpSession, map[string]any{
			"rr_id":  remediationRequest.Name,
			"action": "discover_workflows",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "discover_workflows should succeed")

		text := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  DiscoverWorkflows response (first 300 chars): %.300s\n", text)
		Expect(text).To(ContainSubstring("workflows_discovered"))
		discoverWorkflowsDone = true
	})

	It("FP-MCP-005b: select_workflow with real DS catalog (requires prior discover_workflows)", func() {
		if !discoverWorkflowsDone {
			Skip("depends on FP-MCP-005a (discover_workflows)")
		}
		Expect(workflowUUIDs).To(HaveKey("oomkill-increase-memory-v1:production"),
			"workflow catalog must be seeded")
		workflowID := workflowUUIDs["oomkill-increase-memory-v1:production"]

		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()

		result, err := infrastructure.CallSelectWorkflow(callCtx, mcpSession, map[string]any{
			"rr_id":       remediationRequest.Name,
			"workflow_id": workflowID,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "select_workflow should succeed")

		text := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  SelectWorkflow response (first 300 chars): %.300s\n", text)
		Expect(text).To(ContainSubstring("workflow_selected"))
	})

	It("FP-MCP-001: complete interactive session via MCP", func() {
		if !takeoverDone {
			Skip("depends on FP-MCP-001 (takeover)")
		}
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()

		result, err := infrastructure.CallInvestigate(callCtx, mcpSession, map[string]any{
			"rr_id":  remediationRequest.Name,
			"action": "complete",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "complete should succeed")

		text := infrastructure.ExtractToolResultText(result)
		Expect(text).To(ContainSubstring("completed"))
		sessionCompleted = true
	})

	It("FP-MCP-006b: CRD CompletedAt populated after complete", func() {
		if !sessionCompleted {
			Skip("depends on FP-MCP-001 (complete)")
		}
		aa := &aianalysisv1.AIAnalysis{}
		Eventually(func(g Gomega) {
			g.Expect(apiReader.Get(ctx, client.ObjectKey{Name: aaName, Namespace: namespace}, aa)).To(Succeed())
			g.Expect(aa.Status.InteractiveSession).NotTo(BeNil())
			g.Expect(aa.Status.InteractiveSession.CompletedAt).NotTo(BeNil(),
				"BR-007: CompletedAt must be set after interactive complete")
		}, 30*time.Second, 1*time.Second).Should(Succeed())

		GinkgoWriter.Printf("  CompletedAt: %s\n", aa.Status.InteractiveSession.CompletedAt.Time)
	})

	It("FP-MCP-003b: status returns not_found after interactive session completed", func() {
		if !sessionCompleted {
			Skip("depends on FP-MCP-001 (complete)")
		}
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()

		result, err := infrastructure.CallInvestigate(callCtx, mcpSession, map[string]any{
			"rr_id":  remediationRequest.Name,
			"action": "status",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse())

		text := infrastructure.ExtractToolResultText(result)
		var outer map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &outer)).To(Succeed())

		responseStr, ok := outer["response"].(string)
		Expect(ok).To(BeTrue(), "status result should have a response field")
		var inner map[string]interface{}
		Expect(json.Unmarshal([]byte(responseStr), &inner)).To(Succeed(),
			"response field should contain valid JSON")
		Expect(inner["mode"]).To(Equal("not_found"),
			"session should be not_found after complete (lease released)")
	})

	// ── FP-MCP-002: Fresh interactive session (AF-initiated path) ──
	// Creates an RR CRD directly (mirroring AF's af_create_rr), then exercises
	// the full interactive lifecycle without relying on an existing autonomous session.

	It("FP-MCP-002: fresh start — create RR directly and start interactive session", func() {
		By("Creating RR CRD directly (AF-style)")
		rrName := fmt.Sprintf("rr-fp-mcp-002-%d", time.Now().UnixMilli())
		fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(testNamespace+"/Deployment/fp-mcp-002-target")))
		now := metav1.Now()

		rr := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationRequest",
				"metadata": map[string]interface{}{
					"name":      rrName,
					"namespace": namespace,
				},
				"spec": map[string]interface{}{
					"signalFingerprint": fingerprint,
					"signalName":        "af-manual-Deployment-fp-mcp-002-target",
					"signalType":        "alert",
					"severity":          "high",
					"targetType":        "kubernetes",
					"firingTime":        now.UTC().Format(time.RFC3339),
					"receivedTime":      now.UTC().Format(time.RFC3339),
					"targetResource": map[string]interface{}{
						"kind":      "Deployment",
						"name":      "fp-mcp-002-target",
						"namespace": testNamespace,
					},
				},
			},
		}
		rrGVR := schema.GroupVersionResource{
			Group:    "kubernaut.ai",
			Version:  "v1alpha1",
			Resource: "remediationrequests",
		}

		cfg, cfgErr := config.GetConfig()
		Expect(cfgErr).NotTo(HaveOccurred())
		dynClient, dynErr := dynamic.NewForConfig(cfg)
		Expect(dynErr).NotTo(HaveOccurred())

		createCtx, createCancel := context.WithTimeout(ctx, 15*time.Second)
		defer createCancel()
		_, err := dynClient.Resource(rrGVR).Namespace(namespace).Create(createCtx, rr, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred(), "direct RR CRD creation")
		GinkgoWriter.Printf("  Created RR: %s\n", rrName)

		By("Calling action: start on the new RR")
		startCtx, startCancel := context.WithTimeout(ctx, 30*time.Second)
		defer startCancel()
		result, startErr := infrastructure.CallInvestigate(startCtx, mcpSession, map[string]any{
			"rr_id":  rrName,
			"action": "start",
		})
		Expect(startErr).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "start should succeed")

		text := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  Start response: %s\n", text)
		Expect(text).To(SatisfyAny(
			ContainSubstring("started"),
			ContainSubstring("takeover_started"),
		), "should start fresh or implicitly take over if autonomous was faster")

		By("Sending message in fresh session")
		msgCtx, msgCancel := context.WithTimeout(ctx, 60*time.Second)
		defer msgCancel()
		msgResult, msgErr := infrastructure.CallInvestigate(msgCtx, mcpSession, map[string]any{
			"rr_id":   rrName,
			"action":  "message",
			"message": "What resources are affected by this deployment issue?",
		})
		Expect(msgErr).NotTo(HaveOccurred())
		Expect(msgResult.IsError).To(BeFalse(), "message should succeed")
		msgText := infrastructure.ExtractToolResultText(msgResult)
		GinkgoWriter.Printf("  Message response (first 200 chars): %.200s\n", msgText)
		Expect(msgText).NotTo(BeEmpty())

		By("Completing fresh session")
		completeCtx, completeCancel := context.WithTimeout(ctx, 30*time.Second)
		defer completeCancel()
		completeResult, completeErr := infrastructure.CallInvestigate(completeCtx, mcpSession, map[string]any{
			"rr_id":  rrName,
			"action": "complete",
		})
		Expect(completeErr).NotTo(HaveOccurred())
		Expect(completeResult.IsError).To(BeFalse(), "complete should succeed")
		Expect(infrastructure.ExtractToolResultText(completeResult)).To(ContainSubstring("completed"))
	})
})

// FP-MCP-005c: complete_no_action MCP tool validation.
//
// Creates a fresh RR, starts an interactive session, sends a message, then calls
// complete_no_action. Validates the tool succeeds and the session is properly
// released (status returns not_found). AA routing to WorkflowNotNeeded is tested
// in the AA controller's own test suite (UT-KA-CNA-001 verifies IsActionable=false).
var _ = Describe("FP-MCP-005c: complete_no_action through full pipeline", Label("e2e", "fullpipeline", "interactive", "mcp"), func() {

	It("should complete with no workflow and session should be released", func() {
		By("Creating fresh RR for complete_no_action test")
		rrName := fmt.Sprintf("rr-fp-mcp-005c-%d", time.Now().UnixMilli())
		fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte("fp-mcp-005c-"+rrName)))
		now := metav1.Now()

		rr := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationRequest",
				"metadata": map[string]interface{}{
					"name":      rrName,
					"namespace": namespace,
				},
				"spec": map[string]interface{}{
					"signalFingerprint": fingerprint,
					"signalName":        "manual-test-complete-no-action",
					"signalType":        "alert",
					"severity":          "medium",
					"targetType":        "kubernetes",
					"firingTime":        now.UTC().Format(time.RFC3339),
					"receivedTime":      now.UTC().Format(time.RFC3339),
					"targetResource": map[string]interface{}{
						"kind":      "Deployment",
						"name":      "fp-mcp-005c-target",
						"namespace": namespace,
					},
				},
			},
		}
		rrGVR := schema.GroupVersionResource{
			Group:    "kubernaut.ai",
			Version:  "v1alpha1",
			Resource: "remediationrequests",
		}

		cfg, cfgErr := config.GetConfig()
		Expect(cfgErr).NotTo(HaveOccurred())
		dynClient, dynErr := dynamic.NewForConfig(cfg)
		Expect(dynErr).NotTo(HaveOccurred())

		createCtx, createCancel := context.WithTimeout(ctx, 15*time.Second)
		defer createCancel()
		_, err := dynClient.Resource(rrGVR).Namespace(namespace).Create(createCtx, rr, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred(), "direct RR CRD creation for 005c")

		By("Setting up MCP session — creating SA with interactive RBAC")
		saToken, err := infrastructure.CreateInteractiveE2ESA(
			ctx, namespace, "fp-mcp-005c-sa", kubeconfigPath, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred(), "interactive SA creation for 005c")
		tlsTransport, err := infrastructure.NewTLSAwareTransport(kubeconfigPath)
		Expect(err).NotTo(HaveOccurred())

		connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
		defer connectCancel()
		mcpSess, err := infrastructure.ConnectMCPClient(connectCtx, infrastructure.MCPClientConfig{
			Endpoint:     infrastructure.MCPEndpointForKAE2E(),
			SAToken:      saToken,
			TLSTransport: tlsTransport,
		})
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = mcpSess.Close() }()

		By("Starting interactive session")
		startCtx, startCancel := context.WithTimeout(ctx, 30*time.Second)
		defer startCancel()
		result, err := infrastructure.CallInvestigate(startCtx, mcpSess, map[string]any{
			"rr_id":  rrName,
			"action": "start",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse())

		By("Sending a message")
		msgCtx, msgCancel := context.WithTimeout(ctx, 60*time.Second)
		defer msgCancel()
		result, err = infrastructure.CallInvestigate(msgCtx, mcpSess, map[string]any{
			"rr_id":   rrName,
			"action":  "message",
			"message": "This looks like a transient issue, no action needed",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse())

		By("Calling complete_no_action")
		cnaCtx, cnaCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cnaCancel()
		result, err = infrastructure.CallCompleteNoAction(cnaCtx, mcpSess, map[string]any{
			"rr_id":  rrName,
			"reason": "transient issue, no remediation needed",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.IsError).To(BeFalse(), "complete_no_action should succeed")

		cnaText := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  complete_no_action response: %s\n", cnaText)
		Expect(cnaText).To(ContainSubstring("completed_no_action"))

		By("Verifying session released (status returns not_found)")
		statusCtx, statusCancel := context.WithTimeout(ctx, 30*time.Second)
		defer statusCancel()
		result, err = infrastructure.CallInvestigate(statusCtx, mcpSess, map[string]any{
			"rr_id":  rrName,
			"action": "status",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse())

		statusText := infrastructure.ExtractToolResultText(result)
		GinkgoWriter.Printf("  Post-CNA status: %s\n", statusText)
		var outer map[string]interface{}
		Expect(json.Unmarshal([]byte(statusText), &outer)).To(Succeed())
		responseStr, ok := outer["response"].(string)
		Expect(ok).To(BeTrue(), "status result should have a response field")
		var inner map[string]interface{}
		Expect(json.Unmarshal([]byte(responseStr), &inner)).To(Succeed())
		Expect(inner["mode"]).To(Equal("not_found"),
			"session should be released after complete_no_action")

		GinkgoWriter.Println("FP-MCP-005c: complete_no_action MCP tool validated")
	})
})
