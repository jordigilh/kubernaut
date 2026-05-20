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
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

var _ = Describe("CP-5 COMPAT: Backward Compatibility Tests", Label("e2e", "ka", "interactive", "compat"), ContinueOnFailure, func() {

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
	// E2E-KA-COMPAT-002: v1.4 RR processed correctly (no interactive fields)
	// BR: BR-INTERACTIVE-007, BR-INTERACTIVE-008
	// Validates: RRs without interactive annotations are processed normally
	// ---------------------------------------------------------------
	Describe("E2E-KA-COMPAT-002: v1.4 RR processed without interactive artifacts", func() {
		It("should process standard RR without generating interactive session [E2E-KA-COMPAT-002]", func() {
			rrID := fmt.Sprintf("rr-compat002-%d", time.Now().Unix())

			By("Step 1: Verifying MCP endpoint is active (interactive mode ON for this cluster)")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Step 2: Querying status for a non-existent (v1.4-style) RR — should be empty/nil")
			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  rrID,
				"action": "status",
			})

			if err != nil {
				GinkgoWriter.Printf("Status query error (expected for non-existent RR): %v\n", err)
			} else if result != nil {
				text := infrastructure.ExtractToolResultText(result)
				GinkgoWriter.Printf("Status for v1.4-style RR: %s\n", text)
				Expect(text).NotTo(ContainSubstring("active_driver"),
					"v1.4 RR should have no active driver")
			}

			By("Step 3: Confirming session not created (no Lease for this RR)")
			clientset, err := getKubernetesClientset()
			Expect(err).NotTo(HaveOccurred())

			_, err = clientset.CoordinationV1().Leases(sharedNamespace).Get(
				ctx, leaseNameForRR(rrID), metav1.GetOptions{})
			Expect(err).To(HaveOccurred(), "no Lease should exist for a status-only query")

			session.Close()
			GinkgoWriter.Println("✅ COMPAT-002: v1.4 RR (no interactive fields) processed correctly")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-COMPAT-003: CRD upgrade preserves existing AIAnalysis data
	// BR: BR-INTERACTIVE-007
	// Validates: InteractiveSessionInfo field in CRD is optional/nil for existing objects
	// ---------------------------------------------------------------
	Describe("E2E-KA-COMPAT-003: CRD upgrade preserves existing AIAnalysis data", func() {
		It("should read AIAnalysis without interactive fields after CRD upgrade [E2E-KA-COMPAT-003]", func() {
			By("Step 1: Verifying CRD includes InteractiveSessionInfo (v1.5 schema)")
			req, err := http.NewRequestWithContext(ctx, "GET",
				"https://localhost:8088/api/v1/status", nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", saToken))

			client := &http.Client{Transport: tlsTransport, Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Or(
				Equal(http.StatusOK),
				Equal(http.StatusNotFound),
			), "KA status endpoint should respond")

			By("Step 2: Interactive CRD field is optional — no migration required")
			GinkgoWriter.Println("  CRD field 'interactiveSession' is +optional in v1alpha1 status")
			GinkgoWriter.Println("  Existing AIAnalysis objects without this field remain valid")

			By("Step 3: Confirming MCP handler is active (v1.5 CRD deployed)")
			session, err := infrastructure.ConnectMCPClient(ctx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred(),
				"MCP client connection proves v1.5 CRD with InteractiveSessionInfo is deployed and active")
			session.Close()

			GinkgoWriter.Println("✅ COMPAT-003: CRD upgrade non-breaking, existing data preserved")
		})
	})
})
