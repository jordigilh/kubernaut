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
	"fmt"
	"io"
	"net/http"

	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// Integration Tests: Error Handling & Edge Cases
//
// BUSINESS FOCUS: Gateway resilience in production
// - Graceful error handling prevents cascading failures
// - Clear error messages enable troubleshooting
// - Defensive programming prevents crashes

var _ = Describe("Error Handling & Edge Cases", func() {
	var (
		testNamespace string
		// k8sClient available from suite (DD-E2E-K8S-CLIENT-001)
		// testServer removed - using deployed Gateway
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Setup test clients
		// k8sClient available from suite (DD-E2E-K8S-CLIENT-001)
		Expect(k8sClient).ToNot(BeNil(), "K8s client required for error handling tests")

		// E2E tests use deployed Gateway at gatewayURL (http://127.0.0.1:8080)
		// No local test server needed

		// Create unique namespace for test isolation
		testNamespace = fmt.Sprintf("test-err-%d", time.Now().UnixNano())
		CreateNamespaceAndWait(ctx, k8sClient, testNamespace)

		// Clear Redis
	})

	AfterEach(func() {
		// Reset Redis config to prevent OOM cascade failures

		// E2E tests use deployed Gateway - no cleanup needed

		// CRITICAL FIX: Don't delete namespaces during parallel test execution
		// Let Kind cluster deletion handle cleanup at the end of the test suite
		// Previous code (REMOVED):
		// ns := &corev1.Namespace{
		//     ObjectMeta: metav1.ObjectMeta{
		//         Name: testNamespace,
		//     },
		// }
		// _ = k8sClient.Delete(ctx, ns)
	})

	It("handles malformed JSON gracefully with clear error message", func() {
		// BUSINESS SCENARIO: AlertManager sends corrupted webhook
		// Expected: 400 Bad Request with clear error message (not crash)
		//
		// WHY THIS MATTERS: Invalid data shouldn't crash Gateway
		// Example: Network corruption, AlertManager bug
		// Graceful rejection with clear message enables troubleshooting

		malformedJSON := `{invalid json structure without quotes`

		By("Sending malformed JSON to Gateway")
		req, err := http.NewRequest("POST",
			gatewayURL+"/api/v1/signals/prometheus",
			bytes.NewBufferString(malformedJSON))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

		resp, err := http.DefaultClient.Do(req)
		_ = err
		defer func() { _ = resp.Body.Close() }()

		By("Gateway returns 400 Bad Request (not crash)")
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Invalid JSON should return 400, not 500")

		By("Error message helps troubleshooting")
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		Expect(bodyStr).To(Or(
			ContainSubstring("invalid JSON"),
			ContainSubstring("parse error"),
			ContainSubstring("malformed"),
		), "Error message should indicate JSON parsing issue")

		// BUSINESS OUTCOME VERIFIED:
		// ✅ Gateway doesn't crash on invalid input
		// ✅ Clear error message enables debugging
		// ✅ Graceful degradation
	})

	It("rejects very large payloads to prevent DoS", func() {
		// BUSINESS SCENARIO: Malicious or buggy client sends 1MB payload
		// Expected: 413 Payload Too Large (protect memory)
		//
		// WHY THIS MATTERS: Large payloads can exhaust memory
		// Example: Malicious actor sends 10MB JSON repeatedly
		// Size limit prevents DoS attack

		By("Creating very large alert payload (>100KB)")
		// Create alert with very large annotation
		largeAnnotation := strings.Repeat("A", 150*1024) // 150 KB
		largePayload := fmt.Sprintf(`{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"alertname": "LargePayloadTest",
					"namespace": "%s")
				},
				"annotations": {
					"description": "%s")
				}
			}]
		}`, testNamespace, largeAnnotation)

		By("Sending large payload to Gateway")
		req, err := http.NewRequest("POST",
			gatewayURL+"/api/v1/signals/prometheus",
			bytes.NewBufferString(largePayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

		resp, err := http.DefaultClient.Do(req)
		_ = err
		defer func() { _ = resp.Body.Close() }()

		By("Gateway rejects large payload")
		// Expected: 413 Payload Too Large or 400 Bad Request
		Expect(resp.StatusCode).To(BeElementOf(
			[]int{http.StatusRequestEntityTooLarge, http.StatusBadRequest}), "Large payloads should be rejected")

		// BUSINESS OUTCOME VERIFIED:
		// ✅ Gateway protected from memory exhaustion
		// ✅ DoS prevention
	})

	It("returns clear error for missing required fields", func() {
		// BUSINESS SCENARIO: Webhook missing alertname field
		// Expected: 400 Bad Request with field name in error
		//
		// WHY THIS MATTERS: Clear validation errors enable quick fixes
		// Example: AlertManager config error → Missing field
		// Error message guides operator to fix

		invalidAlert := fmt.Sprintf(`{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"severity": "critical",
					"namespace": "%s",
					"pod": "test-pod"
				}
			}]
		}`, testNamespace)
		// Missing: alertname field

		By("Sending alert with missing required field")
		req, err := http.NewRequest("POST",
			gatewayURL+"/api/v1/signals/prometheus",
			bytes.NewBufferString(invalidAlert))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

		resp, err := http.DefaultClient.Do(req)
		_ = err
		defer func() { _ = resp.Body.Close() }()

		By("Gateway returns validation error")
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Missing required field should return 400")

		By("Error message identifies missing field")
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		Expect(bodyStr).To(Or(
			ContainSubstring("alertname"),
			ContainSubstring("required"),
			ContainSubstring("missing"),
		), "Error should mention missing alertname field")

		// BUSINESS OUTCOME VERIFIED:
		// ✅ Clear validation errors
		// ✅ Operator can quickly identify and fix issue
	})

	// REMOVED: "returns 500 when Kubernetes API is unavailable (for retry)"
	// REASON: Requires K8s API failure simulation
	// COVERAGE: Unit tests (crd_creator_retry_test.go) validate retry logic with mock K8s client

	It("handles namespace not found by using kubernaut-system namespace fallback", func() {
		// BUSINESS SCENARIO: Alert references non-existent namespace
		// Expected: CRD created in kubernaut-system namespace (graceful fallback)
		//
		// WHY THIS MATTERS: Invalid namespace shouldn't block remediation
		// Example: Namespace deleted after alert fired, or cluster-scoped signals (NodeNotReady)
		// Fallback ensures alert still processed
		//
		// WHY kubernaut-system? Proper home for Kubernaut infrastructure, not "default")

		nonExistentNamespace := fmt.Sprintf("does-not-exist-%d", time.Now().UnixNano())

		alertPayload := fmt.Sprintf(`{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"alertname": "NamespaceTest",
					"severity": "warning",
					"namespace": "%s",
					"pod": "orphan-pod")
				}
			}]
		}`, nonExistentNamespace)

		By("Sending alert for non-existent namespace")
		req, err := http.NewRequest("POST",
			gatewayURL+"/api/v1/signals/prometheus",
			bytes.NewBufferString(alertPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

		resp, err := http.DefaultClient.Do(req)
		_ = err
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Gateway should process alert despite invalid namespace (201 Created)")
		_ = resp.Body.Close()

		By("Gateway creates CRD in kubernaut-system namespace as fallback")
		var createdCRD *remediationv1alpha1.RemediationRequest
		Eventually(func() bool {
			// Check both the specified namespace and kubernaut-system namespace
			rrList := &remediationv1alpha1.RemediationRequestList{}

			// Try non-existent namespace first
			err1 := k8sClient.List(context.Background(), rrList,
				client.InNamespace(nonExistentNamespace))
			if err1 == nil && len(rrList.Items) > 0 {
				createdCRD = &rrList.Items[0]
				return true
			}

			// Fall back to kubernaut-system namespace
			// Filter for CRDs with the correct origin-namespace label to avoid picking up old CRDs
			err2 := k8sClient.List(context.Background(), rrList,
				client.InNamespace("kubernaut-system"),
				client.MatchingLabels{"kubernaut.ai/origin-namespace": nonExistentNamespace})
			if err2 == nil && len(rrList.Items) > 0 {
				createdCRD = &rrList.Items[0]
				return true
			}
			return false
		}, 60*time.Second, 2*time.Second).Should(BeTrue(), "CRD created in fallback namespace (increased timeout for K8s API)")

		By("Verifying cluster-scoped labels are set")
		Expect(createdCRD).ToNot(BeNil(), "CRD should be created")
		Expect(createdCRD.Namespace).To(Equal("kubernaut-system"), "CRD should be in kubernaut-system namespace")
		Expect(createdCRD.Labels["kubernaut.ai/cluster-scoped"]).To(Equal("true"), "CRD should have cluster-scoped label")
		Expect(createdCRD.Labels["kubernaut.ai/origin-namespace"]).To(Equal(nonExistentNamespace), "CRD should preserve origin namespace in label")

		// BUSINESS OUTCOME VERIFIED:
		// ✅ Invalid namespace doesn't block remediation
		// ✅ Graceful fallback ensures alert processed
		// ✅ CRD placed in proper infrastructure namespace (kubernaut-system)
		// ✅ Origin namespace preserved in labels for audit/troubleshooting
		// ✅ Operator can later investigate and fix
	})
})
