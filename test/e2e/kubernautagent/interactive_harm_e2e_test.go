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

package kubernautagent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

var _ = Describe("CP-5 HARM: Holistic Adversarial Regression & Misuse Scenarios", Label("e2e", "ka", "interactive", "harm"), func() {

	var (
		mcpEndpoint  string
		tlsTransport http.RoundTripper
	)

	BeforeEach(func() {
		mcpEndpoint = infrastructure.MCPEndpointForKAE2E()
		tlsTransport = testauth.NewRetryOn429Transport(http.DefaultTransport)
	})

	// ---------------------------------------------------------------
	// E2E-KA-HARM-003: Invalid token to MCP endpoint
	// BR: BR-INTERACTIVE-002
	// ---------------------------------------------------------------
	Describe("E2E-KA-HARM-003: Invalid token to MCP endpoint", func() {
		It("should reject with 401 Unauthorized and RFC 7807 response", func() {
			By("Sending HTTP POST to MCP endpoint with invalid Bearer token")
			req, err := http.NewRequestWithContext(ctx, "POST", mcpEndpoint, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer invalid-token-xyz-12345")
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Transport: tlsTransport, Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			By("Asserting 401 Unauthorized status")
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
				"invalid token must be rejected by real TokenReview")

			By("Asserting RFC 7807 Problem JSON response format")
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			var problem map[string]interface{}
			err = json.Unmarshal(body, &problem)
			Expect(err).NotTo(HaveOccurred(), "response should be valid JSON, got: %s", string(body))

			Expect(problem).To(HaveKey("status"), "RFC 7807: 'status' field required")
			Expect(problem).To(HaveKey("title"), "RFC 7807: 'title' field required")

			By("Verifying no session was created (no Lease artifacts)")
			clientset, err := getKubernetesClientset()
			Expect(err).NotTo(HaveOccurred())

			leases, err := clientset.CoordinationV1().Leases(sharedNamespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/managed-by=kubernaut-interactive",
			})
			Expect(err).NotTo(HaveOccurred())
			for _, lease := range leases.Items {
				Expect(lease.Name).NotTo(ContainSubstring("invalid"),
					"no Lease should be created for an invalid-token request")
			}
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-HARM-004: Takeover of non-existent remediation
	// BR: BR-INTERACTIVE-004
	// ---------------------------------------------------------------
	Describe("E2E-KA-HARM-004: Takeover of non-existent remediation", func() {
		It("should return tool error with helpful message and no orphaned resources", func() {
			saToken, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa", kubeconfigPath)
			Expect(err).NotTo(HaveOccurred())

			By("Connecting MCP client with valid SA token")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred(), "MCP client should connect with valid SA token")
			defer session.Close()

			By("Calling kubernaut_investigate with non-existent rr_id")
			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  "ghost-rr-does-not-exist",
				"action": "start",
			})

			By("Asserting either tool error or result indicates failure")
			if err != nil {
				Expect(err.Error()).To(Or(
					ContainSubstring("ghost-rr"),
					ContainSubstring("not found"),
					ContainSubstring("error"),
				), "error should reference the missing RR or be descriptive")
			} else {
				Expect(result).NotTo(BeNil())
				text := infrastructure.ExtractToolResultText(result)
				Expect(text).NotTo(BeEmpty(), "result should contain an error message")
			}

			By("Verifying no Lease was created for the ghost RR")
			clientset, err := getKubernetesClientset()
			Expect(err).NotTo(HaveOccurred())

			leases, err := clientset.CoordinationV1().Leases(sharedNamespace).List(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())
			for _, lease := range leases.Items {
				Expect(lease.Name).NotTo(ContainSubstring("ghost-rr"),
					"no Lease should be created for a non-existent RR")
			}
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-HARM-005: RBAC-restricted SA fails impersonation-gated ops
	// BR: BR-INTERACTIVE-002
	// Test plan: CP-5_RELEASE_GATE.md — user with limited RBAC takes over
	// and impersonated K8s call to "production" namespace fails with 403.
	// ---------------------------------------------------------------
	Describe("E2E-KA-HARM-005: RBAC-restricted SA fails impersonation-gated ops", func() {
		It("should reject enrichment that requires impersonation rights", func() {
			By("Creating a limited-RBAC SA (pods read only in sharedNamespace, no impersonate)")
			limitedToken, err := infrastructure.CreateLimitedRBACSA(
				ctx, sharedNamespace, "harm005-limited", kubeconfigPath, sharedNamespace, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred(), "limited-RBAC SA should be created")

			By("Creating RR so HARM-004 existence check passes")
			createTestRemediationRequest(ctx, "rr-harm005-test")

			By("Connecting MCP client with limited SA token")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      limitedToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred(), "MCP client should connect (authentication passes)")
			defer session.Close()

			By("Starting interactive session (no impersonation needed for session management)")
			startResult, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  "rr-harm005-test",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred(), "session start should succeed")
			Expect(startResult.IsError).To(BeFalse(), "session start should not be a tool error")

			By("Calling kubernaut_select_workflow with enrichment targeting production namespace — limited SA has no RBAC there (#1012)")
			result, err := infrastructure.CallSelectWorkflow(ctx, session, map[string]any{
				"rr_id":       "rr-harm005-test",
				"workflow_id": "wf-rbac-test",
				"kind":        "Pod",
				"name":        "api-server-def456",
				"namespace":   "production",
			})

			By("Asserting that the tool invocation fails due to RBAC/impersonation restriction")
			if err != nil {
				Expect(err.Error()).To(Or(
					ContainSubstring("forbidden"),
					ContainSubstring("impersonate"),
					ContainSubstring("unauthorized"),
					ContainSubstring("RBAC"),
					ContainSubstring("cannot"),
					ContainSubstring("enrich failed"),
				), "error should indicate forbidden/impersonation failure")
			} else {
				Expect(result).NotTo(BeNil())
				if result.IsError {
					text := infrastructure.ExtractToolResultText(result)
					Expect(text).To(Or(
						ContainSubstring("forbidden"),
						ContainSubstring("impersonate"),
						ContainSubstring("unauthorized"),
						ContainSubstring("enrich failed"),
					), "tool error should indicate RBAC restriction")
				} else {
					Fail("enrichment should NOT succeed with a limited-RBAC SA (no RBAC in production namespace)")
				}
			}
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-HARM-006: Concurrent users — second driver rejected
	// BR: BR-INTERACTIVE-004
	// ---------------------------------------------------------------
	Describe("E2E-KA-HARM-006: Concurrent users — second driver rejected", func() {
		It("should reject second takeover and allow after first releases", func() {
			By("Creating two distinct SA tokens for User-A and User-B")
			tokenA, err := infrastructure.CreateInteractiveE2ESA(ctx, sharedNamespace, "harm006-user-a", kubeconfigPath, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred(), "User-A SA should be created")

			tokenB, err := infrastructure.CreateInteractiveE2ESA(ctx, sharedNamespace, "harm006-user-b", kubeconfigPath, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred(), "User-B SA should be created")

			rrID := "rr-harm006-concurrent"
			createTestRemediationRequest(ctx, rrID)

			By("User-A connects and starts interactive session (acquires Lease)")
			sessionA, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      tokenA,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())

			resultA, err := infrastructure.CallInvestigate(ctx, sessionA, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred(), "User-A start should succeed")
			Expect(resultA).NotTo(BeNil())
			textA := infrastructure.ExtractToolResultText(resultA)
			GinkgoWriter.Printf("User-A result: %s\n", textA)

			By("User-B attempts start on same RR (should be rejected — Lease held)")
			sessionB, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      tokenB,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())

			resultB, errB := infrastructure.CallInvestigate(ctx, sessionB, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})

			By("Asserting User-B was rejected with session_active or similar error")
			if errB != nil {
				Expect(errB.Error()).To(Or(
					ContainSubstring("session_active"),
					ContainSubstring("another user"),
					ContainSubstring("max_sessions"),
					ContainSubstring("lease"),
					ContainSubstring("held"),
					ContainSubstring("busy"),
					ContainSubstring("conflict"),
				), "User-B error should indicate Lease contention")
			} else {
				Expect(resultB).NotTo(BeNil())
				if resultB.IsError {
					text := infrastructure.ExtractToolResultText(resultB)
					Expect(text).To(Or(
						ContainSubstring("session_active"),
						ContainSubstring("another user"),
						ContainSubstring("max_sessions"),
						ContainSubstring("lease"),
						ContainSubstring("held"),
						ContainSubstring("busy"),
					), "User-B tool error should indicate Lease held")
				} else {
					Fail("User-B should NOT succeed when User-A holds the Lease")
				}
			}

			By("Verifying Lease exists in cluster with User-A as holder")
			clientset, err := getKubernetesClientset()
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				leases, err := clientset.CoordinationV1().Leases(sharedNamespace).List(ctx, metav1.ListOptions{})
				if err != nil {
					return false
				}
				for _, lease := range leases.Items {
					if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity != "" {
						return true
					}
				}
				return false
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"a Lease should exist with a holder identity")

			By("User-A completes session (releases Lease)")
			_, err = infrastructure.CallInvestigate(ctx, sessionA, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred(), "User-A complete should succeed")
			sessionA.Close()

			By("Waiting for Lease to be released")
			Eventually(func() bool {
				_, err := clientset.CoordinationV1().Leases(sharedNamespace).Get(
					ctx, leaseNameForRR(rrID), metav1.GetOptions{})
				return err != nil
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Lease should be deleted after User-A completes")

			By("User-B retries start — should succeed now")
			resultB2, err := infrastructure.CallInvestigate(ctx, sessionB, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred(), "User-B retry should succeed after Lease released")
			Expect(resultB2).NotTo(BeNil())

			_, err = infrastructure.CallInvestigate(ctx, sessionB, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred())
			sessionB.Close()
		})
	})
})

// getKubernetesClientset creates a Kubernetes clientset from the E2E kubeconfig.
func getKubernetesClientset() (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("build kubeconfig: %w", err)
	}
	return kubernetes.NewForConfig(config)
}

// leaseNameForRR returns the expected Lease name for a given RR ID.
// Must match the naming convention in internal/kubernautagent/mcp/session_manager.go.
func leaseNameForRR(rrID string) string {
	return "kubernaut-interactive-" + rrID
}

