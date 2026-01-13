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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"

	"github.com/google/uuid"
)

// Test 21: CRD Lifecycle Operations (BR-GATEWAY-068, BR-GATEWAY-076, BR-GATEWAY-077)
// Business Outcome: Validate Gateway request validation, error handling, and K8s CRD creation
// Coverage Target: pkg/gateway/validation/* + pkg/gateway/processing/* (+30% estimated)
//
// This test validates:
// - Malformed JSON rejection (HTTP validation)
// - Missing required fields validation (HTTP validation)
// - RFC7807 error response format (error handling)
// - Valid alerts create actual CRDs in Kubernetes (K8s client + informer interaction)
// - Invalid Content-Type rejection (middleware validation)
var _ = Describe("Test 21: CRD Lifecycle Operations", Ordered, Label("crd-lifecycle"), func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testNamespace string
		k8sClient     client.Client
		httpClient    *http.Client
		testLogger    = logger.WithValues("test", "crd-lifecycle")
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		httpClient = &http.Client{Timeout: 10 * time.Second}
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 21: CRD Lifecycle Operations - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique namespace
		processID := GinkgoParallelProcess()
		testNamespace = fmt.Sprintf("crd-lifecycle-%d-%s", processID, uuid.New().String()[:8])
		testLogger.Info("Creating test namespace...", "namespace", testNamespace)

		// Create namespace
		k8sClient = getKubernetesClient()
		CreateNamespaceAndWait(testCtx, k8sClient, testNamespace)

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
	})

	AfterAll(func() {
		if !CurrentSpecReport().Failed() {
			testLogger.Info("Cleaning up test namespace...", "namespace", testNamespace)
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
			_ = k8sClient.Delete(testCtx, ns)
		} else {
			testLogger.Info("⚠️ Test failed - preserving namespace for debugging", "namespace", testNamespace)
		}
		testCancel()
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 21: CRD Lifecycle Operations - Complete")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	It("should reject malformed JSON with 400 Bad Request", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 21a: Malformed JSON Rejection")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		malformedJSON := `{"receiver": "kubernaut", "status": "firing", "alerts": [{"labels": {"severity": "critical"` // Malformed!

		req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL), bytes.NewBufferString(malformedJSON))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", time.Now().UTC().Format(time.RFC3339))

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var errorResp map[string]interface{}
		err = json.Unmarshal(body, &errorResp)
		Expect(err).ToNot(HaveOccurred())

		// RFC7807 Problem Details format
		Expect(errorResp).To(HaveKey("type"))
		Expect(errorResp).To(HaveKey("title"))
		Expect(errorResp).To(HaveKey("status"))
		Expect(errorResp).To(HaveKey("detail"))

		testLogger.Info("✅ Test 21a PASSED: Malformed JSON Rejected (RFC7807 format)")
	})

	It("should successfully create RemediationRequest CRD for valid alert", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 21b: Valid Alert Creates CRD")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testLogger.Info("Step 1: Create Prometheus webhook payload")
		payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
			AlertName: "HighCPUUsage",
			Namespace: testNamespace,
			Severity:  "critical",
			PodName:   "test-pod-12345",
			Labels: map[string]string{
				"component": "frontend",
			},
		})

		testLogger.Info("Step 2: Send valid alert to Gateway")
		// Create request with mandatory X-Timestamp header (per mandatory timestamp validation)
		req, err := http.NewRequest("POST",
			fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
			bytes.NewBuffer(payload))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

		resp, err := httpClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		body, _ := io.ReadAll(resp.Body)
		Expect(resp.StatusCode).To(Equal(http.StatusCreated),
			"Valid Prometheus webhook should create CRD", "response", string(body))

		testLogger.Info("Step 2: Verify CRD created in Kubernetes (Gateway K8s client validation)")
		var crdCount int
		Eventually(func() int {
			crdList := &remediationv1alpha1.RemediationRequestList{}
			err := k8sClient.List(testCtx, crdList, client.InNamespace(testNamespace))
			if err != nil {
				testLogger.V(1).Info("Failed to list CRDs", "error", err)
				return 0
			}
			crdCount = len(crdList.Items)
			if crdCount == 0 {
				GinkgoWriter.Printf("⚠️  No CRDs yet (K8s cache sync delay), retrying...\n")
			}
			return crdCount
		}, 240*time.Second, 3*time.Second).Should(BeNumerically("==", 1),
			"Exactly 1 CRD should be created for valid alert (increased timeout for K8s client cache synchronization)")

		testLogger.Info("Step 3: Validate CRD spec fields")
		crdList := &remediationv1alpha1.RemediationRequestList{}
		err = k8sClient.List(testCtx, crdList, client.InNamespace(testNamespace))
		Expect(err).ToNot(HaveOccurred())
		Expect(crdList.Items).To(HaveLen(1))

		crd := crdList.Items[0]
		Expect(crd.Spec.SignalName).To(Equal("HighCPUUsage"))
		Expect(crd.Spec.Severity).To(Equal("critical"))
		Expect(crd.Spec.TargetResource.Namespace).To(Equal(testNamespace))

		testLogger.Info("✅ Test 21b PASSED: CRD created successfully", "crdName", crd.Name)
	})

	It("should reject alert with missing alertname field", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 21c: Missing Required Field Validation")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Create webhook payload with empty alertname (invalid)
		payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
			AlertName: "", // Empty alertname - invalid!
			Namespace: testNamespace,
			Severity:  "critical",
		})

		req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL), bytes.NewBuffer(payload))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", time.Now().UTC().Format(time.RFC3339))

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var errorResp map[string]interface{}
		err = json.Unmarshal(body, &errorResp)
		Expect(err).ToNot(HaveOccurred())
		Expect(errorResp).To(HaveKey("detail"))

		testLogger.Info("✅ Test 21c PASSED: Missing Required Field Rejected")
	})

	It("should reject alert with invalid Content-Type header", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 21d: Invalid Content-Type Rejection")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Use proper webhook format but wrong Content-Type
		payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
			AlertName: "TestContentType",
			Namespace: testNamespace,
			Severity:  "warning",
		})

		req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL), bytes.NewBuffer(payload))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "text/plain") // Invalid! Should be application/json
		req.Header.Set("X-Timestamp", time.Now().UTC().Format(time.RFC3339))

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		// Gateway returns 400 Bad Request when JSON parsing fails due to wrong Content-Type
		// This is acceptable behavior (400 vs 415) - both indicate client error
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

		testLogger.Info("✅ Test 21d PASSED: Invalid Content-Type Rejected")
	})
})
