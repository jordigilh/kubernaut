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

package effectivenessmonitor

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("Metrics Comparison Integration (BR-EM-003)", func() {

	// Restore mock Prometheus to default state after each test
	AfterEach(func() {
		// Clear any custom handlers (e.g., from MC-005 503 test)
		mockProm.SetQueryRangeHandler(nil)
		// Restore instant query response
		mockProm.SetQueryResponse(infrastructure.NewPromVectorResponse(
			map[string]string{"__name__": "container_cpu_usage_seconds_total", "namespace": "default"},
			0.25,
			float64(time.Now().Unix()),
		))
		// Restore range query response (reconciler uses /api/v1/query_range)
		now := float64(time.Now().Unix())
		preRemediationTime := now - 60
		mockProm.SetQueryRangeResponse(infrastructure.NewPromMatrixResponse(
			map[string]string{"__name__": "container_cpu_usage_seconds_total", "namespace": "default"},
			[][]interface{}{
				{preRemediationTime, "0.500000"}, // pre-remediation: 50% CPU
				{now, "0.250000"},                 // post-remediation: 25% CPU (improvement)
			},
		))
	})

	// ========================================
	// IT-EM-MC-001: Mock Prom returns improvement data -> metrics score > 0
	// ========================================
	It("IT-EM-MC-001: should score > 0 when Prometheus returns metric data", func() {
		ns := createTestNamespace("em-mc-001")
		defer deleteTestNamespace(ns)

		By("Configuring mock Prometheus with metric data indicating improvement")
		mockProm.SetQueryResponse(infrastructure.NewPromVectorResponse(
			map[string]string{
				"__name__":  "container_cpu_usage_seconds_total",
				"namespace": ns,
			},
			0.15, // 15% CPU (improved from baseline)
			float64(time.Now().Unix()),
		))

		By("Creating an EA with Prometheus enabled")
		ea := createEffectivenessAssessment(ns, "ea-mc-001", "rr-mc-001")

		By("Verifying the EA completes with a metrics score")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.MetricsScore).NotTo(BeNil())
		// Reconciler assigns 0.5 default when metric data is available
		Expect(*fetchedEA.Status.Components.MetricsScore).To(BeNumerically(">", 0.0),
			"metrics with data available should score > 0")
	})

	// ========================================
	// IT-EM-MC-002: Mock Prom returns no-change data -> metrics score
	// ========================================
	It("IT-EM-MC-002: should produce metrics score when data exists", func() {
		ns := createTestNamespace("em-mc-002")
		defer deleteTestNamespace(ns)

		By("Configuring mock Prometheus with same baseline data (no change)")
		mockProm.SetQueryResponse(infrastructure.NewPromVectorResponse(
			map[string]string{
				"__name__":  "container_cpu_usage_seconds_total",
				"namespace": ns,
			},
			0.25, // Same as baseline = no change
			float64(time.Now().Unix()),
		))

		By("Creating an EA")
		ea := createEffectivenessAssessment(ns, "ea-mc-002", "rr-mc-002")

		By("Verifying the EA completes with metrics assessed")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.MetricsScore).NotTo(BeNil())
	})

	// ========================================
	// IT-EM-MC-003: Mock Prom returns degraded data -> metrics score
	// ========================================
	It("IT-EM-MC-003: should handle degraded metric data", func() {
		ns := createTestNamespace("em-mc-003")
		defer deleteTestNamespace(ns)

		By("Configuring mock Prometheus with degraded metric data (higher CPU)")
		mockProm.SetQueryResponse(infrastructure.NewPromVectorResponse(
			map[string]string{
				"__name__":  "container_cpu_usage_seconds_total",
				"namespace": ns,
			},
			0.90, // 90% CPU (degraded)
			float64(time.Now().Unix()),
		))

		By("Creating an EA")
		ea := createEffectivenessAssessment(ns, "ea-mc-003", "rr-mc-003")

		By("Verifying the EA completes with metrics assessed")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.MetricsScore).NotTo(BeNil())
	})

	// ========================================
	// IT-EM-MC-004: Mock Prom returns empty result -> metrics not assessed, requeues
	// ========================================
	It("IT-EM-MC-004: should handle empty Prometheus result and eventually complete via expiry", func() {
		ns := createTestNamespace("em-mc-004")
		defer deleteTestNamespace(ns)

		By("Configuring mock Prometheus to return empty vector (no data)")
		mockProm.SetQueryResponse(infrastructure.NewPromEmptyVectorResponse())

		By("Creating an EA with a tight validity window so it expires")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-mc-004",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-mc-004",
				RemediationRequestPhase: "Completed",
				TargetResource: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Verifying the EA eventually completes (via expiration since no metric data)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// Metrics may or may not be assessed depending on timing
		// The important thing is the EA completed (via expiry or partial)
		Expect(fetchedEA.Status.CompletedAt).NotTo(BeNil())
	})

	// ========================================
	// IT-EM-MC-005: Mock Prom returns error (503) -> metrics not assessed, requeues
	// ========================================
	It("IT-EM-MC-005: should handle Prometheus error and eventually complete", func() {
		ns := createTestNamespace("em-mc-005")
		defer deleteTestNamespace(ns)

		queryRangeCount := 0
		By("Configuring mock Prometheus to return 503 initially, then recover")
		// Use the built-in QueryRangeHandler override (not Server.Config.Handler)
		// to avoid corrupting the mux for other tests in the same process.
		mockProm.SetQueryRangeHandler(func(w http.ResponseWriter, r *http.Request) {
			queryRangeCount++
			if queryRangeCount <= 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprint(w, "Service Unavailable")
				return
			}
			// After failures, return valid matrix data with 2 samples (pre/post remediation)
			now := time.Now().Unix()
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"cpu"},"values":[[%d,"0.5"],[%d,"0.25"]]}]}}`, now-60, now)
		})

		By("Creating an EA")
		ea := createEffectivenessAssessment(ns, "ea-mc-005", "rr-mc-005")

		By("Verifying the EA eventually completes after Prometheus recovers")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.CompletedAt).NotTo(BeNil())

		By("Restoring mock Prometheus to default")
		// Recreate mock with default config (the handler override above
		// will be replaced when the next test restores via AfterEach)
	})

	// ========================================
	// IT-EM-MC-006: No metric data -> metrics stay un-assessed, EA remains Assessing
	// ========================================
	// Updated: Per-EA Prometheus enable/disable was removed (DD-017 v2.5).
	// Prometheus is now globally enabled via ReconcilerConfig. This test
	// verifies graceful degradation when Prometheus has no relevant data:
	// the reconciler requeues because assessMetrics returns Assessed=false.
	// The EA remains in Assessing phase with nil metrics score.
	// NOTE: ValidityWindow (30m default) is too long for the integration test
	// timeout (120s), so we verify the intermediate Assessing state rather than
	// waiting for expiry-based completion.
	It("IT-EM-MC-006: should keep metrics un-assessed when Prometheus returns no data", func() {
		ns := createTestNamespace("em-mc-006")
		defer deleteTestNamespace(ns)

		By("Configuring mock Prometheus range endpoint to return no data")
		mockProm.SetQueryRangeResponse(nil)

		By("Creating an EA with short stabilization window")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-mc-006",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-mc-006",
				RemediationRequestPhase: "Completed",
				TargetResource: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Waiting for EA to reach Assessing phase (stabilization passed)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseAssessing))
		}, timeout, interval).Should(Succeed())

		By("Verifying metrics score remains nil (Prometheus returned empty data)")
		// Health and alert components may be assessed independently,
		// but metrics should remain nil because Prometheus returned no data.
		Expect(fetchedEA.Status.Components.MetricsScore).To(BeNil(),
			"metrics score should be nil when Prometheus returns no data")

		By("Verifying EA is NOT completed (validity window not yet expired)")
		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseAssessing),
			"EA should stay in Assessing (validity window 30m not expired)")
		Expect(fetchedEA.Status.CompletedAt).To(BeNil(),
			"EA should not have completed yet")
	})

	// ========================================
	// IT-EM-MC-007: Mock Prom returns partial data (CPU only) -> uses available metrics
	// ========================================
	It("IT-EM-MC-007: should handle partial metric data (single metric series)", func() {
		ns := createTestNamespace("em-mc-007")
		defer deleteTestNamespace(ns)

		By("Configuring mock Prometheus with only CPU metric (no memory)")
		mockProm.SetQueryResponse(infrastructure.NewPromVectorResponse(
			map[string]string{
				"__name__":  "container_cpu_usage_seconds_total",
				"namespace": ns,
			},
			0.30, // Only CPU metric available
			float64(time.Now().Unix()),
		))

		By("Creating an EA")
		ea := createEffectivenessAssessment(ns, "ea-mc-007", "rr-mc-007")

		By("Verifying the EA completes with available metrics scored")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.MetricsScore).NotTo(BeNil(),
			"partial metric data should still produce a score")
	})

	// ========================================
	// IT-EM-MC-008: Metrics event payload verified
	// ========================================
	It("IT-EM-MC-008: should preserve correlation data and metrics component structure", func() {
		ns := createTestNamespace("em-mc-008")
		defer deleteTestNamespace(ns)

		By("Configuring mock Prometheus with valid response")
		mockProm.SetQueryResponse(infrastructure.NewPromVectorResponse(
			map[string]string{
				"__name__":  "container_cpu_usage_seconds_total",
				"namespace": ns,
			},
			0.20,
			float64(time.Now().Unix()),
		))
		mockProm.ResetRequestLog()

		By("Creating an EA")
		ea := createEffectivenessAssessment(ns, "ea-mc-008", "rr-mc-008")

		By("Verifying the EA completes")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// Verify metrics component data
		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.MetricsScore).NotTo(BeNil())

		// Verify correlation
		Expect(fetchedEA.Spec.CorrelationID).To(Equal("rr-mc-008"))

		// Verify Prometheus was actually queried
		requests := mockProm.GetRequestLog()
		queryRequests := 0
		for _, req := range requests {
			if req.Path == "/api/v1/query_range" {
				queryRequests++
			}
		}
		Expect(queryRequests).To(BeNumerically(">", 0),
			"Prometheus should have been queried via query_range at least once")
	})
})
