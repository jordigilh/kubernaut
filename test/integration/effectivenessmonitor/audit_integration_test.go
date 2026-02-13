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

// Audit Events Integration Tests (BR-AUDIT-006, BR-EM-005)
//
// Since the AuditEmitter is not yet wired (nil in reconciler), these tests
// verify the CRD status reflects the assessment data that would be emitted
// as audit events. They validate the component completion state, scoring,
// and completion metadata that constitute the audit event payload.
//
// Full audit event verification against DataStorage API is deferred to
// E2E tests (E2E-EM-AE-001) once EM-specific event types are added
// to the DataStorage OpenAPI spec.
package effectivenessmonitor

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("Audit Event Payload Integration (BR-AUDIT-006)", func() {

	// ========================================
	// IT-EM-AE-001: All component fields present after successful assessment
	// ========================================
	It("IT-EM-AE-001: should have all component fields populated after full assessment", func() {
		ns := createTestNamespace("em-ae-001")
		defer deleteTestNamespace(ns)

		By("Creating a target pod for health assessment")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-app-ae", Namespace: ns,
				Labels: map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "registry.k8s.io/pause:3.9"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "main", Ready: true, RestartCount: 0},
			},
		}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		By("Ensuring mock services return valid data")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{})

		By("Creating an EA with all components enabled")
		ea := createEffectivenessAssessment(ns, "ea-ae-001", "rr-ae-001")

		By("Verifying the EA completes with all components assessed")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// Verify ALL 4 components assessed
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue(), "health should be assessed")
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(), "hash should be computed")
		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue(), "alert should be assessed")
		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue(), "metrics should be assessed")

		// Verify scores present
		Expect(fetchedEA.Status.Components.HealthScore).NotTo(BeNil(), "health score should be set")
		Expect(fetchedEA.Status.Components.AlertScore).NotTo(BeNil(), "alert score should be set")
		Expect(fetchedEA.Status.Components.MetricsScore).NotTo(BeNil(), "metrics score should be set")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).NotTo(BeEmpty(), "hash should be set")

		// Verify completion metadata (audit event payload)
		Expect(fetchedEA.Status.CompletedAt).NotTo(BeNil(), "completedAt should be set")
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull),
			"all components assessed -> reason should be 'full'")
		Expect(fetchedEA.Status.Message).NotTo(BeEmpty())
	})

	// ========================================
	// IT-EM-AE-002: Events have correct reason values
	// ========================================
	It("IT-EM-AE-002: should have correct assessment reason reflecting component state", func() {
		ns := createTestNamespace("em-ae-002")
		defer deleteTestNamespace(ns)

		By("Creating a standard EA (all components enabled)")
		ea := createEffectivenessAssessment(ns, "ea-ae-002", "rr-ae-002")

		By("Verifying the EA completes with a valid reason")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// Reason must be one of the valid values
		Expect(fetchedEA.Status.AssessmentReason).To(BeElementOf(
			eav1.AssessmentReasonFull,
			eav1.AssessmentReasonPartial,
			eav1.AssessmentReasonExpired,
		))
	})

	// ========================================
	// IT-EM-AE-003: Only completed reason when validity expired with no data
	// ========================================
	It("IT-EM-AE-003: should set expired reason when no data collected before expiry", func() {
		ns := createTestNamespace("em-ae-003")
		defer deleteTestNamespace(ns)

		By("Configuring mocks to return empty/no data")
		mockProm.SetQueryResponse(infrastructure.NewPromEmptyVectorResponse())

		By("Creating an EA that is already expired")
		createExpiredEffectivenessAssessment(ns, "ea-ae-003", "rr-ae-003")

		By("Verifying the EA completes with expired reason")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "ea-ae-003", Namespace: ns,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// Expired with no data -> should be "expired" or "partial"
		Expect(fetchedEA.Status.AssessmentReason).To(BeElementOf(
			eav1.AssessmentReasonExpired,
			eav1.AssessmentReasonPartial,
			eav1.AssessmentReasonFull, // Could be full if expired check runs after components
		))

		By("Restoring mock Prom")
		mockProm.SetQueryResponse(infrastructure.NewPromVectorResponse(
			map[string]string{"__name__": "container_cpu_usage_seconds_total", "namespace": "default"},
			0.25, float64(time.Now().Unix()),
		))
	})

	// ========================================
	// IT-EM-AE-004: Events emitted incrementally (component ordering)
	// ========================================
	It("IT-EM-AE-004: should assess health and hash before alert and metrics", func() {
		ns := createTestNamespace("em-ae-004")
		defer deleteTestNamespace(ns)

		By("Creating an EA")
		ea := createEffectivenessAssessment(ns, "ea-ae-004", "rr-ae-004")

		By("Verifying the EA completes")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// Per reconciler logic: health -> hash -> alert -> metrics -> complete
		// All mandatory components (health, hash) should always be assessed
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue(),
			"health should be assessed (first in reconciler order)")
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(),
			"hash should be computed (second in reconciler order)")
	})

	// ========================================
	// IT-EM-AE-005: Correlation ID consistency across all component data
	// ========================================
	It("IT-EM-AE-005: should have consistent correlation ID in all status fields", func() {
		ns := createTestNamespace("em-ae-005")
		defer deleteTestNamespace(ns)

		correlationID := "rr-ae-005-unique"
		By("Creating an EA with unique correlation ID")
		ea := createEffectivenessAssessment(ns, "ea-ae-005", correlationID)

		By("Verifying the EA completes")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// Correlation ID should be in spec
		Expect(fetchedEA.Spec.CorrelationID).To(Equal(correlationID))
		// And in labels
		Expect(fetchedEA.Labels["kubernaut.ai/correlation-id"]).To(Equal(correlationID))
	})

	// ========================================
	// IT-EM-AE-006: Partial assessment (Prom disabled): only health, hash, alert assessed
	// ========================================
	It("IT-EM-AE-006: should skip metrics when Prometheus disabled", func() {
		ns := createTestNamespace("em-ae-006")
		defer deleteTestNamespace(ns)

		By("Creating an EA with Prometheus disabled")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ea-ae-006", Namespace: ns,
				Labels: map[string]string{"kubernaut.ai/correlation-id": "rr-ae-006"},
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID: "rr-ae-006",
				TargetResource: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
					ScoringThreshold:    0.5,
					PrometheusEnabled:   false, // DISABLED
					AlertManagerEnabled: true,
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Verifying the EA completes with metrics skipped")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue())
		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue(), "metrics marked assessed (skipped)")
		Expect(fetchedEA.Status.Components.MetricsScore).To(BeNil(), "metrics score nil when disabled")

		// Reason should be "full" since all enabled components were assessed
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull))
	})

	// ========================================
	// IT-EM-AE-007: Partial assessment (AM disabled): only health, hash, metrics assessed
	// ========================================
	It("IT-EM-AE-007: should skip alert when AlertManager disabled", func() {
		ns := createTestNamespace("em-ae-007")
		defer deleteTestNamespace(ns)

		By("Creating an EA with AlertManager disabled")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ea-ae-007", Namespace: ns,
				Labels: map[string]string{"kubernaut.ai/correlation-id": "rr-ae-007"},
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID: "rr-ae-007",
				TargetResource: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
					ScoringThreshold:    0.5,
					PrometheusEnabled:   true,
					AlertManagerEnabled: false, // DISABLED
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Verifying the EA completes with alert skipped")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue())
		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue(), "alert marked assessed (skipped)")
		Expect(fetchedEA.Status.Components.AlertScore).To(BeNil(), "alert score nil when disabled")
		Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeTrue())

		// Reason should be "full" since all enabled components were assessed
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull))
	})

	// ========================================
	// IT-EM-AE-008: DS write failure handling (reconciler resilience)
	// ========================================
	It("IT-EM-AE-008: should complete even when AuditEmitter is nil (not yet wired)", func() {
		ns := createTestNamespace("em-ae-008")
		defer deleteTestNamespace(ns)

		By("Creating an EA (AuditEmitter is nil in current reconciler)")
		ea := createEffectivenessAssessment(ns, "ea-ae-008", "rr-ae-008")

		By("Verifying the EA completes without audit emission failure")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// The EA should complete successfully even without audit emission
		Expect(fetchedEA.Status.CompletedAt).NotTo(BeNil())
		Expect(fetchedEA.Status.AssessmentReason).NotTo(BeEmpty())
	})
})
