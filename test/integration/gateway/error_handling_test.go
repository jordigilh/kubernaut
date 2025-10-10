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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	var testNamespace string

	BeforeEach(func() {
		// Create unique namespace for test isolation
		testNamespace = fmt.Sprintf("test-err-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())

		// Clear Redis
		Expect(redisClient.FlushDB(context.Background()).Err()).To(Succeed())
	})

	AfterEach(func() {
		// Cleanup namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		_ = k8sClient.Delete(context.Background(), ns)
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
			"http://localhost:8090/api/v1/signals/prometheus",
			bytes.NewBufferString(malformedJSON))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		By("Gateway returns 400 Bad Request (not crash)")
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"Invalid JSON should return 400, not 500")

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
					"namespace": "%s"
				},
				"annotations": {
					"description": "%s"
				}
			}]
		}`, testNamespace, largeAnnotation)

		By("Sending large payload to Gateway")
		req, err := http.NewRequest("POST",
			"http://localhost:8090/api/v1/signals/prometheus",
			bytes.NewBufferString(largePayload))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		By("Gateway rejects large payload")
		// Expected: 413 Payload Too Large or 400 Bad Request
		Expect(resp.StatusCode).To(BeElementOf(
			[]int{http.StatusRequestEntityTooLarge, http.StatusBadRequest}),
			"Large payloads should be rejected")

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
			"http://localhost:8090/api/v1/signals/prometheus",
			bytes.NewBufferString(invalidAlert))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		By("Gateway returns validation error")
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"Missing required field should return 400")

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

	It("returns 500 when Kubernetes API is unavailable (for retry)", func() {
		// BUSINESS SCENARIO: K8s API down, Gateway can't create CRD
		// Expected: 500 Internal Server Error (AlertManager retries)
		//
		// WHY THIS MATTERS: Transient K8s failures should retry
		// Example: K8s API server restart → Brief downtime
		// 500 error triggers AlertManager retry logic

		Skip("Requires Kubernetes API failure simulation (complex infrastructure)")

		// This test would:
		// 1. Simulate K8s API unavailability (stop API server or network partition)
		// 2. Send alert to Gateway
		// 3. Verify 500 Internal Server Error returned
		// 4. Restore K8s API
		// 5. Verify AlertManager retry succeeds
		//
		// Implementation note: Requires control over K8s API server
		//
		// BUSINESS OUTCOME:
		// ✅ Transient failures trigger retry
		// ✅ AlertManager retry logic handles temporary issues
		// ✅ No alerts lost due to brief downtime
	})

	It("handles namespace not found by using default namespace fallback", func() {
		// BUSINESS SCENARIO: Alert references non-existent namespace
		// Expected: CRD created in default namespace (graceful fallback)
		//
		// WHY THIS MATTERS: Invalid namespace shouldn't block remediation
		// Example: Namespace deleted after alert fired
		// Fallback ensures alert still processed

		nonExistentNamespace := fmt.Sprintf("does-not-exist-%d", time.Now().UnixNano())

		alertPayload := fmt.Sprintf(`{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"alertname": "NamespaceTest",
					"severity": "warning",
					"namespace": "%s",
					"pod": "orphan-pod"
				}
			}]
		}`, nonExistentNamespace)

		By("Sending alert for non-existent namespace")
		req, err := http.NewRequest("POST",
			"http://localhost:8090/api/v1/signals/prometheus",
			bytes.NewBufferString(alertPayload))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusCreated),
			"Gateway should process alert despite invalid namespace (201 Created)")
		resp.Body.Close()

		By("Gateway creates CRD in default namespace as fallback")
		Eventually(func() bool {
			// Check both the specified namespace and default namespace
			rrList := &remediationv1alpha1.RemediationRequestList{}

			// Try non-existent namespace first
			err1 := k8sClient.List(context.Background(), rrList,
				client.InNamespace(nonExistentNamespace))
			if err1 == nil && len(rrList.Items) > 0 {
				return true
			}

			// Fall back to default namespace
			err2 := k8sClient.List(context.Background(), rrList,
				client.InNamespace("default"))
			return err2 == nil && len(rrList.Items) > 0
		}, 10*time.Second).Should(BeTrue(),
			"CRD created in fallback namespace")

		// BUSINESS OUTCOME VERIFIED:
		// ✅ Invalid namespace doesn't block remediation
		// ✅ Graceful fallback ensures alert processed
		// ✅ Operator can later investigate and fix
	})
})
