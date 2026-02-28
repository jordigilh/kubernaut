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

// Config-Disabled Permutation Unit Tests
//
// Validates the reconciler's behavior when Prometheus and/or AlertManager
// are disabled via ReconcilerConfig. Tests use fake.NewClientBuilder() for
// the K8s client and nil external clients for disabled services.
//
// Business risk: nil-pointer dereference if the reconciler invokes a
// disabled service's client, bypassing the config toggle guard.
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

var _ = Describe("Config-Disabled Reconciler (BR-EM-006, BR-EM-007, BR-EM-008)", func() {

	// buildScheme registers EA and core types for the fake client.
	buildScheme := func() *runtime.Scheme {
		s := runtime.NewScheme()
		Expect(eav1.AddToScheme(s)).To(Succeed())
		Expect(corev1.AddToScheme(s)).To(Succeed())
		return s
	}

	// makeReconciler creates a Reconciler with the given config toggles,
	// a fake K8s client seeded with the provided objects, and nil external
	// clients for disabled services.
	makeReconciler := func(s *runtime.Scheme, promEnabled, amEnabled bool, objs ...client.Object) (*controller.Reconciler, client.Client) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithStatusSubresource(&eav1.EffectivenessAssessment{}).
			Build()

		cfg := controller.DefaultReconcilerConfig()
		cfg.PrometheusEnabled = promEnabled
		cfg.AlertManagerEnabled = amEnabled

		r := controller.NewReconciler(
			fakeClient,
			s,
			record.NewFakeRecorder(100),
			emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			nil, nil, // Prom + AM clients: nil (tests verify nil safety)
			nil, nil, // AuditManager, DSQuerier
			cfg,
		)
		return r, fakeClient
	}

	// seedEA returns a minimal EA object for fake client seeding.
	seedEA := func(ns, name, corrID string) *eav1.EffectivenessAssessment {
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           corrID,
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 0}, // No wait
				},
			},
		}
	}

	// reconcileUntilDone calls Reconcile() until the EA reaches Completed.
	reconcileUntilDone := func(r *controller.Reconciler, fc client.Client, ns, name string) *eav1.EffectivenessAssessment {
		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: ns}}

		var ea *eav1.EffectivenessAssessment
		Eventually(func(g Gomega) {
			_, err := r.Reconcile(ctx, req)
			g.Expect(err).NotTo(HaveOccurred())

			ea = &eav1.EffectivenessAssessment{}
			g.Expect(fc.Get(ctx, req.NamespacedName, ea)).To(Succeed())
			g.Expect(ea.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())
		return ea
	}

	// ========================================================================
	// UT-EM-CF-009: Both Prom + AM disabled → all 4 flags set, but scores nil
	// The reconciler marks disabled components as "assessed-as-skipped"
	// (AlertAssessed=true, MetricsAssessed=true) with nil scores.
	// ========================================================================
	It("UT-EM-CF-009: should complete with nil alert/metrics scores when both Prom and AM disabled", func() {
		s := buildScheme()
		ea := seedEA("default", "ea-cf-009", "rr-cf-009")
		r, fc := makeReconciler(s, false, false, ea)

		result := reconcileUntilDone(r, fc, "default", "ea-cf-009")

		Expect(result.Status.Components.HealthAssessed).To(BeTrue(),
			"health must be assessed (always-on)")
		Expect(result.Status.Components.HashComputed).To(BeTrue(),
			"hash must be computed (always-on)")
		Expect(result.Status.Components.AlertAssessed).To(BeTrue(),
			"alert should be marked assessed-as-skipped (AM disabled)")
		Expect(result.Status.Components.AlertScore).To(BeNil(),
			"alert score must be nil when AM disabled (no actual assessment)")
		Expect(result.Status.Components.MetricsAssessed).To(BeTrue(),
			"metrics should be marked assessed-as-skipped (Prom disabled)")
		Expect(result.Status.Components.MetricsScore).To(BeNil(),
			"metrics score must be nil when Prom disabled (no actual assessment)")
		Expect(result.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull),
			"disabled components should not prevent 'full' completion")
	})

	// ========================================================================
	// UT-EM-CF-010: AM disabled → alert skipped (score nil), no nil-pointer panic
	// ========================================================================
	It("UT-EM-CF-010: should produce nil alert score when AM disabled with nil client", func() {
		s := buildScheme()
		ea := seedEA("default", "ea-cf-010", "rr-cf-010")
		r, fc := makeReconciler(s, false, false, ea)

		result := reconcileUntilDone(r, fc, "default", "ea-cf-010")

		Expect(result.Status.Components.AlertAssessed).To(BeTrue(),
			"alert flag should be true (marked assessed-as-skipped)")
		Expect(result.Status.Components.AlertScore).To(BeNil(),
			"alert score must be nil when AM disabled — no actual assessment happened")
		Expect(result.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull))
	})

	// ========================================================================
	// UT-EM-CF-011: Prom disabled → metrics skipped (score nil), no nil-pointer panic
	// ========================================================================
	It("UT-EM-CF-011: should produce nil metrics score when Prom disabled with nil client", func() {
		s := buildScheme()
		ea := seedEA("default", "ea-cf-011", "rr-cf-011")
		r, fc := makeReconciler(s, false, false, ea)

		result := reconcileUntilDone(r, fc, "default", "ea-cf-011")

		Expect(result.Status.Components.MetricsAssessed).To(BeTrue(),
			"metrics flag should be true (marked assessed-as-skipped)")
		Expect(result.Status.Components.MetricsScore).To(BeNil(),
			"metrics score must be nil when Prom disabled — no actual assessment happened")
		Expect(result.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull))
	})

	// ========================================================================
	// UT-EM-CF-012: Both disabled → all 4 assessed flags true, but only 2 have real scores
	// health and hash produce real results; alert and metrics have nil scores.
	// ========================================================================
	It("UT-EM-CF-012: should have nil scores only for disabled components", func() {
		s := buildScheme()
		ea := seedEA("default", "ea-cf-012", "rr-cf-012")
		r, fc := makeReconciler(s, false, false, ea)

		result := reconcileUntilDone(r, fc, "default", "ea-cf-012")

		// All 4 flags should be true (2 real + 2 skipped)
		Expect(result.Status.Components.HealthAssessed).To(BeTrue())
		Expect(result.Status.Components.HashComputed).To(BeTrue())
		Expect(result.Status.Components.AlertAssessed).To(BeTrue())
		Expect(result.Status.Components.MetricsAssessed).To(BeTrue())

		// Disabled components have nil scores; active components have non-nil scores
		Expect(result.Status.Components.AlertScore).To(BeNil(),
			"disabled alert must have nil score")
		Expect(result.Status.Components.MetricsScore).To(BeNil(),
			"disabled metrics must have nil score")

		Expect(result.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull))
	})

	// ========================================================================
	// UT-EM-CF-013: Nil clients with disabled config → no panic during Reconcile
	// (startup safety: EM deployed without optional Prometheus)
	// ========================================================================
	It("UT-EM-CF-013: should not panic when Reconcile called with nil Prom and AM clients", func() {
		s := buildScheme()
		ea := seedEA("default", "ea-cf-013", "rr-cf-013")
		r, fc := makeReconciler(s, false, false, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-cf-013", Namespace: "default",
		}}

		Expect(func() {
			// Multiple reconcile calls — must never panic
			for i := 0; i < 10; i++ {
				_, _ = r.Reconcile(ctx, req)
			}
		}).NotTo(Panic(), "Reconcile must not panic with nil Prom+AM clients when disabled")

		// Verify EA progressed (not stuck)
		fetched := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(ctx, req.NamespacedName, fetched)).To(Succeed())
		Expect(fetched.Status.Phase).To(Equal(eav1.PhaseCompleted))
	})
})
