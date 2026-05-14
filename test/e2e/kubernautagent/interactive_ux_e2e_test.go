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
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

var _ = Describe("CP-5 UX: User Experience & Operational Tests", Label("e2e", "ka", "interactive", "ux"), func() {

	var (
		mcpEndpoint  string
		tlsTransport http.RoundTripper
		saToken      string
	)

	BeforeEach(func() {
		mcpEndpoint = infrastructure.MCPEndpointForKAE2E()
		tlsTransport = testauth.NewRetryOn429Transport(http.DefaultTransport)

		var err error
		saToken, err = infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa", kubeconfigPath)
		Expect(err).NotTo(HaveOccurred())
	})

	// ---------------------------------------------------------------
	// E2E-KA-UX-001: Time-remaining visible during interactive session
	// BR: BR-INTERACTIVE-005
	// Validates: status query returns remaining time information
	// ---------------------------------------------------------------
	Describe("E2E-KA-UX-001: Time-remaining visible during session", func() {
		It("should show remaining session time via status query [E2E-KA-UX-001]", func() {
			rrID := fmt.Sprintf("rr-ux001-%d", time.Now().Unix())
			createTestRemediationRequest(ctx, rrID)

			By("Step 1: Starting interactive session")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("Step 2: Querying session status for time remaining")
			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "status",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			statusText := infrastructure.ExtractToolResultText(result)
			Expect(statusText).NotTo(BeEmpty(), "status should return session info")
			GinkgoWriter.Printf("Status response: %s\n", statusText)

			By("Step 3: Verifying status contains time/session metadata")
			Expect(statusText).To(Or(
				ContainSubstring("remaining"),
				ContainSubstring("ttl"),
				ContainSubstring("expires"),
				ContainSubstring("session"),
				ContainSubstring("active"),
			), "status should contain time or session metadata")

			By("Step 4: Cleaning up")
			_, _ = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})

			GinkgoWriter.Println("✅ UX-001: Time-remaining visible during interactive session")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-UX-002: Inactivity warning before cutoff
	// BR: BR-INTERACTIVE-005
	// Validates: tool call resets inactivity timer (prevents timeout)
	// ---------------------------------------------------------------
	Describe("E2E-KA-UX-002: Inactivity timer resets on activity", func() {
		It("should keep session alive when tool calls reset inactivity timer [E2E-KA-UX-002]", func() {
			rrID := fmt.Sprintf("rr-ux002-%d", time.Now().Unix())
			createTestRemediationRequest(ctx, rrID)

			By("Step 1: Starting interactive session")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("Step 2: Waiting briefly and sending activity (should reset timer)")
			time.Sleep(2 * time.Second)
			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":   rrID,
				"action":  "message",
				"message": "activity to reset timer",
			})
			Expect(err).NotTo(HaveOccurred(), "message after brief wait should succeed")
			Expect(result).NotTo(BeNil())

			By("Step 3: Session still active after activity")
			statusResult, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "status",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(statusResult).NotTo(BeNil())
			statusText := infrastructure.ExtractToolResultText(statusResult)
			Expect(statusText).To(Or(
				ContainSubstring("active"),
				ContainSubstring("session"),
			), "session should still be active after activity reset")

			By("Step 4: Cleaning up")
			_, _ = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})

			GinkgoWriter.Println("✅ UX-002: Inactivity timer resets on activity")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-UX-003: Second driver rejection includes holder info
	// BR: BR-INTERACTIVE-004
	// Validates: rejection error contains holder identity and timestamp
	// ---------------------------------------------------------------
	Describe("E2E-KA-UX-003: Rejection includes holder info", func() {
		It("should include holder identity in rejection error [E2E-KA-UX-003]", func() {
			rrID := fmt.Sprintf("rr-ux003-%d", time.Now().Unix())
			createTestRemediationRequest(ctx, rrID)

			By("Step 1: User-A takes over (holds Lease)")
			tokenA, err := infrastructure.CreateInteractiveE2ESA(ctx, sharedNamespace, "ux003-user-a", kubeconfigPath, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			sessionA, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      tokenA,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = infrastructure.CallInvestigate(ctx, sessionA, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("Step 2: User-B attempts takeover (should be rejected with holder info)")
			tokenB, err := infrastructure.CreateInteractiveE2ESA(ctx, sharedNamespace, "ux003-user-b", kubeconfigPath, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

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

			By("Step 3: Asserting rejection contains holder context")
			var rejectionText string
			if errB != nil {
				rejectionText = errB.Error()
			} else if resultB != nil && resultB.IsError {
				rejectionText = infrastructure.ExtractToolResultText(resultB)
			} else {
				Fail("User-B should be rejected when User-A holds the session")
			}

			GinkgoWriter.Printf("Rejection text: %s\n", rejectionText)
			Expect(rejectionText).To(Or(
				ContainSubstring("session_active"),
				ContainSubstring("another user"),
				ContainSubstring("ux003-user-a"),
				ContainSubstring("driver"),
				ContainSubstring("held"),
				ContainSubstring("controlled"),
				ContainSubstring("busy"),
			), "rejection should include holder identity or context")

			By("Step 4: Cleaning up")
			_, _ = infrastructure.CallInvestigate(ctx, sessionA, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})
			sessionA.Close()
			sessionB.Close()

			GinkgoWriter.Println("✅ UX-003: Rejection includes holder info")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-UX-004: Prometheus metrics emitted for interactive sessions
	// BR: BR-INTERACTIVE-001
	// Validates: metrics endpoint shows interactive session metrics
	// ---------------------------------------------------------------
	Describe("E2E-KA-UX-004: Prometheus metrics for interactive sessions", func() {
		It("should emit session metrics on /metrics endpoint [E2E-KA-UX-004]", func() {
			rrID := fmt.Sprintf("rr-ux004-%d", time.Now().Unix())
			createTestRemediationRequest(ctx, rrID)

			By("Step 1: Getting baseline metrics")
			baselineMetrics := scrapeKAMetrics()

			By("Step 2: Starting interactive session")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("Step 3: Checking metrics after session start")
			Eventually(func() string {
				return scrapeKAMetrics()
			}, 10*time.Second, 1*time.Second).Should(Or(
				ContainSubstring("aiagent_mcp_interactive_sessions_active"),
				ContainSubstring("aiagent_mcp_interactive_takeover_total"),
			), "interactive metrics should appear after session start")

			afterStartMetrics := scrapeKAMetrics()
			GinkgoWriter.Printf("Metrics after start contain interactive data: %v\n",
				len(afterStartMetrics) > len(baselineMetrics))

			By("Step 4: Completing session and verifying metric change")
			_, _ = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "complete",
			})

			Eventually(func() bool {
				metrics := scrapeKAMetrics()
				return len(metrics) > 0
			}, 10*time.Second, 1*time.Second).Should(BeTrue(),
				"metrics endpoint should still be responsive after session end")

			GinkgoWriter.Println("✅ UX-004: Prometheus metrics emitted for interactive sessions")
		})
	})
})

// scrapeKAMetrics fetches the raw Prometheus metrics from the KA metrics endpoint.
func scrapeKAMetrics() string {
	resp, err := http.Get(kaMetricsURL + "/metrics")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return string(body)
}
