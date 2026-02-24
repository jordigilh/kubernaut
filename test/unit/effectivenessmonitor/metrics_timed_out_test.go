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
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
)

// ============================================================================
// METRICS_TIMED_OUT ASSESSMENT REASON TESTS (ADR-EM-001, Batch 3)
// Business Requirement: EM distinguishes metrics_timed_out from generic partial
//
// All tests drive the reconciler through the public Reconcile() method.
// When an EA has expired validity and specific component states, the reconciler
// reaches determineAssessmentReason() via completeAssessment().
// ============================================================================
var _ = Describe("Assessment Reason: metrics_timed_out (ADR-EM-001, Batch 3)", func() {

	buildScheme := func() *runtime.Scheme {
		s := runtime.NewScheme()
		Expect(eav1.AddToScheme(s)).To(Succeed())
		Expect(corev1.AddToScheme(s)).To(Succeed())
		return s
	}

	makeReconciler := func(s *runtime.Scheme, promEnabled, amEnabled bool, objs ...client.Object) (*controller.Reconciler, client.Client) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithStatusSubresource(&eav1.EffectivenessAssessment{}).
			Build()

		cfg := controller.DefaultReconcilerConfig()
		cfg.PrometheusEnabled = promEnabled
		cfg.AlertManagerEnabled = amEnabled
		// Very short validity to ensure expiration
		cfg.ValidityWindow = 1 * time.Millisecond

		r := controller.NewReconciler(
			fakeClient, s,
			record.NewFakeRecorder(100),
			emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			nil, nil, // Prom + AM clients
			nil, nil, // AuditManager, DSQuerier
			cfg,
		)
		return r, fakeClient
	}

	// seedAssessingEA creates an EA in Assessing phase with expired ValidityDeadline
	// and specific component states. This simulates an EA that has been assessed for
	// some components but the validity window expired before metrics could be collected.
	seedAssessingEA := func(ns, name string, components eav1.EAComponents) *eav1.EffectivenessAssessment {
		pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Hour))
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         ns,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-" + name,
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 0},
				},
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseAssessing,
				ValidityDeadline: &pastDeadline,
				Components:       components,
			},
		}
	}

	// ========================================
	// UT-EM-MT-001: Health+hash done, metrics not done, Prom enabled → metrics_timed_out
	// ========================================
	It("UT-EM-MT-001: should set metrics_timed_out when health+hash done but metrics not assessed", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-mt-001"

		healthScore := 1.0
		ea := seedAssessingEA(ns, name, eav1.EAComponents{
			HealthAssessed: true,
			HealthScore:    &healthScore,
			HashComputed:   true,
			// MetricsAssessed: false (default) — this is the key
			// AlertAssessed: false (AM disabled, so irrelevant)
		})

		r, fc := makeReconciler(s, true, false, ea) // Prom=enabled, AM=disabled

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonMetricsTimedOut),
			"Should be metrics_timed_out, not partial, when health+hash done but metrics unavailable")
	})

	// ========================================
	// UT-EM-MT-002: Only health done, metrics not done → partial (not metrics_timed_out)
	// ========================================
	It("UT-EM-MT-002: should set partial when only health done but hash not computed", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-mt-002"

		healthScore := 1.0
		ea := seedAssessingEA(ns, name, eav1.EAComponents{
			HealthAssessed: true,
			HealthScore:    &healthScore,
			HashComputed:   false, // Hash not done → cannot be metrics_timed_out
		})

		r, fc := makeReconciler(s, true, false, ea)

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonPartial),
			"Should be partial when hash not computed (metrics_timed_out requires health+hash)")
	})

	// ========================================
	// UT-EM-MT-003: Prometheus disabled → never metrics_timed_out
	// ========================================
	It("UT-EM-MT-003: should not set metrics_timed_out when Prometheus is disabled", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-mt-003"

		healthScore := 1.0
		ea := seedAssessingEA(ns, name, eav1.EAComponents{
			HealthAssessed: true,
			HealthScore:    &healthScore,
			HashComputed:   true,
			// Prom disabled → MetricsAssessed irrelevant
		})

		r, fc := makeReconciler(s, false, false, ea) // Prom=disabled

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		// With Prom disabled, health+hash done = all done = full
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull),
			"Should be full when Prom disabled and health+hash done")
	})

	// ========================================
	// UT-EM-MT-004: Nothing assessed + expired → expired (not metrics_timed_out)
	// ========================================
	It("UT-EM-MT-004: should set expired when no components assessed", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-mt-004"

		ea := seedAssessingEA(ns, name, eav1.EAComponents{
			// All false/unassessed
		})

		r, fc := makeReconciler(s, true, false, ea)

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonExpired),
			"Should be expired when no components assessed at all")
	})

	// ========================================
	// UT-EM-MT-005: Health+hash done, AM enabled but alerts NOT assessed, metrics NOT assessed, expired → partial (NOT metrics_timed_out)
	// GAP-1 fix: metrics_timed_out requires alerts to also be done (or AM disabled).
	// When both alerts and metrics are missing, the correct reason is "partial".
	// ========================================
	It("UT-EM-MT-005: should set partial when alerts and metrics both unassessed with AM enabled", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-mt-005"

		ea := seedAssessingEA(ns, name, eav1.EAComponents{
			HealthAssessed: true,
			HashComputed:   true,
			// AlertAssessed:   false (AM enabled, alerts NOT done)
			// MetricsAssessed: false (Prom enabled, metrics NOT done)
		})

		// Both Prometheus AND AlertManager enabled
		r, fc := makeReconciler(s, true, true, ea)

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonPartial),
			"Should be partial when both alerts and metrics are unassessed (not metrics_timed_out)")
	})
})
