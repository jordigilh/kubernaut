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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ========================================
// BR-SCOPE-002: Gateway Scope Filtering E2E Tests
// ========================================
//
// Test Plan Reference: docs/testing/BR-SCOPE-001/TEST_PLAN.md
// Test IDs: E2E-GW-002-001 through E2E-GW-002-006
//
// These E2E tests validate the complete signal-to-rejection flow
// against a real Gateway deployed in a Kind cluster:
//
// Group 1: Core scope filtering (test plan alignment)
// - E2E-GW-002-001: Managed namespace -> HTTP 201 + RR created
// - E2E-GW-002-002: Unmanaged namespace -> HTTP 200 + rejection, no RR
// - E2E-GW-002-003: Namespace inheritance (Pod without label, NS managed) -> HTTP 201
//
// Group 2: 2-level hierarchy overrides (defense-in-depth)
// - E2E-GW-002-004: Resource opt-out in managed NS -> HTTP 200 rejection
//
// Group 3: Observability & operator UX
// - E2E-GW-002-005: Actionable rejection response with kubectl instructions
// - E2E-GW-002-006: Metric gateway_signals_rejected_total on /metrics
//
// Business Requirements:
// - BR-SCOPE-002: Gateway Signal Filtering
// - NFR-SCOPE-001: No performance degradation for managed signals
// ========================================

var _ = Describe("Test 37: BR-SCOPE-002 Gateway Scope Filtering (E2E)", Ordered, Label("scope", "e2e"), func() {
	var (
		testLogger  logr.Logger
		httpClient  *http.Client
		managedNS   string
		unmanagedNS string
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "scope-filtering-e2e")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 37: Gateway Scope Filtering (E2E) - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Create managed namespace (with kubernaut.ai/managed=true label)
		managedNS = helpers.CreateTestNamespace(ctx, k8sClient, "scope-managed-e2e")
		testLogger.Info("Created managed namespace", "namespace", managedNS)

		// Create unmanaged namespace (without kubernaut.ai/managed label)
		unmanagedNS = helpers.CreateTestNamespace(ctx, k8sClient, "scope-unmanaged-e2e",
			helpers.WithoutManagedLabel())
		testLogger.Info("Created unmanaged namespace", "namespace", unmanagedNS)

		// Create test pods in both namespaces
		testPods := []corev1.Pod{
			{
				// Managed NS: pod WITH managed label (explicit opt-in for E2E-GW-002-001)
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scope-test-pod",
					Namespace: managedNS,
					Labels:    map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueTrue},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test", Image: "busybox:latest"}}},
			},
			{
				// Managed NS: pod WITHOUT label (for inheritance test E2E-GW-002-003)
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scope-inherit-pod",
					Namespace: managedNS,
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test", Image: "busybox:latest"}}},
			},
			{
				// Managed NS: pod WITH opt-out label (for E2E-GW-002-004)
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scope-optout-pod",
					Namespace: managedNS,
					Labels:    map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueFalse},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test", Image: "busybox:latest"}}},
			},
			{
				// Unmanaged NS: pod without label (for E2E-GW-002-002)
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scope-test-pod",
					Namespace: unmanagedNS,
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test", Image: "busybox:latest"}}},
			},
		}
		for i := range testPods {
			Expect(k8sClient.Create(ctx, &testPods[i])).To(Succeed())
		}

		testLogger.Info("Scope filtering E2E setup complete",
			"managedNS", managedNS, "unmanagedNS", unmanagedNS)
	})

	AfterAll(func() {
		testLogger.Info("Cleaning up scope filtering E2E tests")
		helpers.DeleteTestNamespace(ctx, k8sClient, managedNS)
		helpers.DeleteTestNamespace(ctx, k8sClient, unmanagedNS)
	})

	// ─────────────────────────────────────────────
	// Group 1: Core scope filtering
	// ─────────────────────────────────────────────

	// E2E-GW-002-001: Signal from managed namespace → HTTP 201 + RR created
	It("[E2E-GW-002-001] should return HTTP 201 and create RR for managed resource (BR-SCOPE-002)", func() {
		By("1. Send Prometheus alert signal referencing pod in managed namespace")
		alertName := fmt.Sprintf("ScopeTest-Managed-%s", uuid.New().String()[:8])
		payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
			AlertName: alertName,
			Namespace: managedNS,
			PodName:   "scope-test-pod",
			Severity:  "critical",
		})

		var resp *http.Response
		Eventually(func() error {
			var err error
			req, reqErr := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
			if reqErr != nil {
				return reqErr
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			resp, err = httpClient.Do(req)
			return err
		}, 10*time.Second, 1*time.Second).Should(Succeed())
		defer func() { _ = resp.Body.Close() }()

		By("2. Verify HTTP 201 Created (signal accepted, RR created)")
		Expect(resp.StatusCode).To(Or(Equal(http.StatusCreated), Equal(http.StatusAccepted)),
			"E2E-GW-002-001: Managed signal should return HTTP 201/202")

		By("3. Verify RemediationRequest CRD exists in managed namespace")
		Eventually(func() int {
			var rrList remediationv1alpha1.RemediationRequestList
			err := k8sClient.List(ctx, &rrList, client.InNamespace(managedNS))
			if err != nil {
				return 0
			}
			return len(rrList.Items)
		}, 10*time.Second, 1*time.Second).Should(BeNumerically(">=", 1),
			"E2E-GW-002-001: At least one RR should exist in managed namespace")

		testLogger.Info("Signal from managed namespace accepted and RR created",
			"alertName", alertName, "namespace", managedNS)
	})

	// E2E-GW-002-002: Signal from unmanaged namespace → HTTP 200 + rejection
	It("[E2E-GW-002-002] should return HTTP 200 with rejection for unmanaged resource (BR-SCOPE-002)", func() {
		By("1. Send Prometheus alert signal referencing pod in unmanaged namespace")
		alertName := fmt.Sprintf("ScopeTest-Unmanaged-%s", uuid.New().String()[:8])
		payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
			AlertName: alertName,
			Namespace: unmanagedNS,
			PodName:   "scope-test-pod",
			Severity:  "warning",
		})

		req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

		resp, err := httpClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)

		By("2. Verify HTTP 200 response (not 4xx/5xx — rejection is not an error)")
		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"E2E-GW-002-002: Scope rejection should return HTTP 200")

		By("3. Verify response body indicates rejection")
		var respBody map[string]interface{}
		Expect(json.Unmarshal(body, &respBody)).To(Succeed())
		Expect(respBody).To(HaveKeyWithValue("status", "rejected"),
			"E2E-GW-002-002: Response status must be 'rejected'")

		By("4. Verify no RemediationRequest was created in unmanaged namespace")
		var rrList remediationv1alpha1.RemediationRequestList
		Expect(k8sClient.List(ctx, &rrList, client.InNamespace(unmanagedNS))).To(Succeed())
		Expect(rrList.Items).To(BeEmpty(),
			"E2E-GW-002-002: No RR should exist in unmanaged namespace")

		testLogger.Info("Signal from unmanaged namespace correctly rejected",
			"alertName", alertName, "namespace", unmanagedNS)
	})

	// E2E-GW-002-003: Namespace inheritance — Pod without label in managed NS
	It("[E2E-GW-002-003] should accept signal via namespace inheritance (BR-SCOPE-002)", func() {
		By("1. Send signal referencing pod WITHOUT own label in managed namespace")
		alertName := fmt.Sprintf("ScopeTest-Inherit-%s", uuid.New().String()[:8])
		payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
			AlertName: alertName,
			Namespace: managedNS,
			PodName:   "scope-inherit-pod", // no kubernaut.ai/managed label
			Severity:  "warning",
		})

		req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

		resp, err := httpClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		By("2. Verify HTTP 201 (namespace inheritance: pod inherits managed from NS)")
		Expect(resp.StatusCode).To(Or(Equal(http.StatusCreated), Equal(http.StatusAccepted)),
			"E2E-GW-002-003: Pod without label must inherit managed from namespace")

		testLogger.Info("Signal accepted via namespace inheritance",
			"alertName", alertName, "pod", "scope-inherit-pod")
	})

	// ─────────────────────────────────────────────
	// Group 2: 2-level hierarchy overrides
	// ─────────────────────────────────────────────

	// E2E-GW-002-004: Resource opt-out in managed NS → HTTP 200 rejection
	It("[E2E-GW-002-004] should reject signal when resource has opt-out in managed NS (BR-SCOPE-002)", func() {
		By("1. Send signal referencing pod WITH managed=false in managed namespace")
		alertName := fmt.Sprintf("ScopeTest-OptOut-%s", uuid.New().String()[:8])
		payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
			AlertName: alertName,
			Namespace: managedNS,
			PodName:   "scope-optout-pod", // has kubernaut.ai/managed=false
			Severity:  "critical",
		})

		req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

		resp, err := httpClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)

		By("2. Verify HTTP 200 (resource opt-out overrides managed namespace)")
		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"E2E-GW-002-004: Resource opt-out should override managed namespace")

		By("3. Verify response body indicates rejection")
		var respBody map[string]interface{}
		Expect(json.Unmarshal(body, &respBody)).To(Succeed())
		Expect(respBody).To(HaveKeyWithValue("status", "rejected"),
			"E2E-GW-002-004: Response status must be 'rejected' for opt-out resource")

		testLogger.Info("Signal correctly rejected for opt-out resource in managed NS",
			"alertName", alertName, "pod", "scope-optout-pod")
	})

	// ─────────────────────────────────────────────
	// Group 3: Observability & operator UX
	// ─────────────────────────────────────────────

	// E2E-GW-002-005: Actionable rejection response with kubectl instructions
	It("[E2E-GW-002-005] should include actionable label instructions in rejection response (BR-SCOPE-002)", func() {
		By("1. Send signal to unmanaged namespace")
		alertName := fmt.Sprintf("ScopeTest-Action-%s", uuid.New().String()[:8])
		payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
			AlertName: alertName,
			Namespace: unmanagedNS,
			PodName:   "scope-test-pod",
			Severity:  "warning",
		})

		req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

		resp, err := httpClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)

		By("2. Parse rejection response")
		var respBody map[string]interface{}
		Expect(json.Unmarshal(body, &respBody)).To(Succeed())

		By("3. Verify rejection contains actionable details")
		rejection, ok := respBody["rejection"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "E2E-GW-002-005: Response must contain 'rejection' object")
		Expect(rejection).To(HaveKeyWithValue("reason", "unmanaged_resource"),
			"E2E-GW-002-005: Rejection reason must be 'unmanaged_resource'")
		Expect(rejection).To(HaveKey("action"),
			"E2E-GW-002-005: Rejection must include 'action' instructions")

		// Verify the action contains the kubectl label command
		action, _ := rejection["action"].(string)
		Expect(action).To(ContainSubstring("kubectl label"),
			"E2E-GW-002-005: Action must contain kubectl label command")
		Expect(action).To(ContainSubstring(scope.ManagedLabelKey),
			"E2E-GW-002-005: Action must reference the managed label key")

		testLogger.Info("Rejection response contains actionable instructions",
			"action", action)
	})

	// E2E-GW-002-006: Metric gateway_signals_rejected_total on /metrics
	It("[E2E-GW-002-006] should expose gateway_signals_rejected_total metric (BR-SCOPE-002)", func() {
		By("1. Send signal to unmanaged namespace (should be rejected)")
		alertName := fmt.Sprintf("ScopeTest-Metric-%s", uuid.New().String()[:8])
		payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
			AlertName: alertName,
			Namespace: unmanagedNS,
			PodName:   "scope-test-pod",
			Severity:  "warning",
		})

		req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

		resp, err := httpClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		_ = resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		By("2. Verify metric is visible on /metrics endpoint")
		Eventually(func() bool {
			metricsResp, err := httpClient.Get(gatewayURL + "/metrics")
			if err != nil {
				return false
			}
			defer func() { _ = metricsResp.Body.Close() }()
			body, _ := io.ReadAll(metricsResp.Body)
			return strings.Contains(string(body), "gateway_signals_rejected_total")
		}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"E2E-GW-002-006: gateway_signals_rejected_total metric must be visible on /metrics")

		testLogger.Info("Rejection metric verified on /metrics endpoint",
			"alertName", alertName)
	})
})
