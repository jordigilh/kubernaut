/*
Copyright 2025 Jordi Gil.

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

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// =============================================================================
// BR-AUDIT-005 Gap #7: Gateway Error Details Standardization
// =============================================================================
//
// Business Requirements:
// - BR-AUDIT-005 v2.0 Gap #7: Standardized error details across all services
// - SOC2 Type II: Comprehensive error audit trail for compliance
// - RR Reconstruction: Reliable `.status.error` field reconstruction
//
// Authority Documents:
// - DD-004: RFC7807 error response standard (HTTP responses only)
// - DD-AUDIT-003 v1.4: Service audit trace requirements
// - ADR-034: Unified audit table design
// - SOC2_AUDIT_IMPLEMENTATION_PLAN.md: Day 4 - Error Details Standardization
//
// Test Strategy (per TESTING_GUIDELINES.md):
// - Integration tier: Call Gateway business logic directly (ProcessSignal)
// - Real infrastructure: K8s API + Data Storage (Podman)
// - NO HTTP layer: Direct Go function calls, not HTTP requests
// - Tests MUST Fail() if not implemented (NO Skip())
//
// Error Scenarios Tested:
// - Scenario 1: K8s CRD creation failure (ERR_K8S_*) - Integration test
// - Scenario 2: Adapter validation failure (ERR_INVALID_*) - Unit test (see test/unit/gateway/audit_errors_unit_test.go)
//
// To run these tests:
//   make test-integration-gateway
//
// =============================================================================

var _ = Describe("BR-GATEWAY-NAMESPACE-FALLBACK: Gateway Namespace Fallback E2E Validation", func() {
	var (
		testCtx       context.Context // ← Test-local context
		httpClient    *http.Client
		testNamespace string
		// k8sClient available from suite (DD-E2E-K8S-CLIENT-001)
	)

	BeforeEach(func() {
		testCtx = context.Background() // ← Uses local variable
		httpClient = &http.Client{Timeout: 10 * time.Second}

		// Create unique test namespace (Pattern: RO E2E)
		// This prevents circuit breaker degradation from "namespace not found" errors
		testNamespace = createTestNamespace("test-error-audit")

	})

	AfterEach(func() {
		// Clean up test namespace (Pattern: RO E2E)
		deleteTestNamespace(testNamespace)
	})

	Context("BR-GATEWAY-NAMESPACE-FALLBACK: Namespace Fallback Validation", func() {
		It("should successfully create CRD in fallback namespace when original namespace doesn't exist", func() {
			By("1. Create Prometheus alert with non-existent namespace")
			// Use a namespace that definitely doesn't exist
			nonExistentNamespace := "non-existent-ns-" + uuid.New().String()
			alertName := "NamespaceFallbackTest-" + uuid.New().String()[:8]

			// Create Prometheus webhook payload
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: alertName,
				Namespace: nonExistentNamespace, // Non-existent namespace triggers fallback
				Severity:  "warning",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			})

			By("2. Send HTTP request to Gateway - expect success with fallback")
			// E2E Pattern: Use HTTP POST to Gateway endpoint
			req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
			Expect(err).ToNot(HaveOccurred(), "HTTP request creation should succeed")
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			// BR-GATEWAY-NAMESPACE-FALLBACK: Gateway should succeed with namespace fallback
			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"Gateway should return 201 Created when CRD is created successfully via namespace fallback")
			GinkgoWriter.Printf("✅ Gateway response: HTTP %d (namespace fallback succeeded)\n", resp.StatusCode)

			// Parse response to get CRD details
			var gwResp GatewayResponse
			bodyBytes, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred(), "Should read response body")
			err = json.Unmarshal(bodyBytes, &gwResp)
			Expect(err).ToNot(HaveOccurred(), "Should parse Gateway response JSON")

			By("3. Verify CRD was created in fallback namespace (kubernaut-system)")
			crdName := gwResp.RemediationRequestName
			Expect(crdName).ToNot(BeEmpty(), "Gateway should return CRD name")

			// BR-GATEWAY-NAMESPACE-FALLBACK: CRD should be in fallback namespace
			fallbackNamespace := "kubernaut-system"
			var createdRR remediationv1alpha1.RemediationRequest
			Eventually(func() error {
				return k8sClient.Get(testCtx,
					client.ObjectKey{Namespace: fallbackNamespace, Name: crdName},
					&createdRR)
			}, 30*time.Second, 1*time.Second).Should(Succeed(),
				"CRD should be created in fallback namespace (kubernaut-system)")

			By("4. Verify CRD has namespace fallback labels")
			// BR-GATEWAY-NAMESPACE-FALLBACK: Labels preserve origin namespace information
			Expect(createdRR.Labels).To(HaveKeyWithValue("kubernaut.ai/origin-namespace", nonExistentNamespace),
				"CRD should have origin-namespace label with original (non-existent) namespace")
			Expect(createdRR.Labels).To(HaveKeyWithValue("kubernaut.ai/cluster-scoped", "true"),
				"CRD should have cluster-scoped=true label when namespace fallback is used")

			By("5. Verify CRD namespace is fallback namespace")
			Expect(createdRR.Namespace).To(Equal(fallbackNamespace),
				"CRD should be in fallback namespace (kubernaut-system), not original namespace")

			GinkgoWriter.Printf("✅ Namespace fallback validation complete:\n")
			GinkgoWriter.Printf("   - Original namespace: %s (non-existent)\n", nonExistentNamespace)
			GinkgoWriter.Printf("   - Fallback namespace: %s\n", fallbackNamespace)
			GinkgoWriter.Printf("   - CRD name: %s\n", crdName)
			GinkgoWriter.Printf("   - Labels: origin-namespace=%s, cluster-scoped=true\n",
				createdRR.Labels["kubernaut.ai/origin-namespace"])
		})
	})

	// NOTE: Scenario 2 (Adapter Validation Failure) moved to unit tests
	// Rationale: Adapter validation is pure logic without infrastructure needs
	// Location: test/unit/gateway/audit_errors_unit_test.go
	// This maintains proper test distribution (70% unit, >50% integration)
})
