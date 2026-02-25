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
// VALIDITY WINDOW RUNTIME GUARD TESTS (Issue #188, BR-EM-009)
//
// When the EA's StabilizationWindow (set by RO) >= the EM's ValidityWindow,
// the reconciler must extend ValidityDeadline so metrics can be queried after
// stabilization completes. Without this guard, ValidityDeadline expires before
// stabilization ends and the EA completes as metrics_timed_out.
//
// Self-healing: effectiveValidity = StabilizationWindow + ValidityWindow
// ============================================================================
var _ = Describe("Validity Window Runtime Guard (Issue #188, BR-EM-009)", func() {

	buildScheme := func() *runtime.Scheme {
		s := runtime.NewScheme()
		Expect(eav1.AddToScheme(s)).To(Succeed())
		Expect(corev1.AddToScheme(s)).To(Succeed())
		return s
	}

	makeReconcilerWithConfig := func(s *runtime.Scheme, validityWindow time.Duration, objs ...client.Object) (*controller.Reconciler, client.Client) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithStatusSubresource(&eav1.EffectivenessAssessment{}).
			Build()

		cfg := controller.DefaultReconcilerConfig()
		cfg.ValidityWindow = validityWindow

		r := controller.NewReconciler(
			fakeClient, s,
			record.NewFakeRecorder(100),
			emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			nil, nil,
			nil, nil,
			cfg,
		)
		return r, fakeClient
	}

	makePendingEA := func(ns, name string, stabilizationWindow time.Duration) *eav1.EffectivenessAssessment {
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
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
					StabilizationWindow: metav1.Duration{Duration: stabilizationWindow},
				},
			},
		}
	}

	// ========================================
	// UT-EM-VWG-001: StabilizationWindow (5m) > ValidityWindow (2m)
	// ValidityDeadline must be extended to creation + 5m + 2m = 7m
	// ========================================
	It("UT-EM-VWG-001: should extend ValidityDeadline when StabilizationWindow exceeds ValidityWindow", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-vwg-001"

		stabilizationWindow := 5 * time.Minute
		validityWindow := 2 * time.Minute
		ea := makePendingEA(ns, name, stabilizationWindow)

		r, fc := makeReconcilerWithConfig(s, validityWindow, ea)

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.ValidityDeadline).NotTo(BeNil())

		expectedDeadline := fetchedEA.CreationTimestamp.Add(stabilizationWindow + validityWindow)
		Expect(fetchedEA.Status.ValidityDeadline.Time).To(
			BeTemporally("~", expectedDeadline, 2*time.Second),
			"ValidityDeadline should be creation + StabilizationWindow + ValidityWindow (5m+2m=7m)")

		Expect(fetchedEA.Status.PrometheusCheckAfter).NotTo(BeNil())
		expectedCheckAfter := fetchedEA.CreationTimestamp.Add(stabilizationWindow)
		Expect(fetchedEA.Status.PrometheusCheckAfter.Time).To(
			BeTemporally("~", expectedCheckAfter, 2*time.Second),
			"PrometheusCheckAfter should remain creation + StabilizationWindow")

		Expect(fetchedEA.Status.ValidityDeadline.Time.After(fetchedEA.Status.PrometheusCheckAfter.Time)).To(BeTrue(),
			"ValidityDeadline must be after PrometheusCheckAfter (invariant)")
	})

	// ========================================
	// UT-EM-VWG-002: StabilizationWindow (30s) < ValidityWindow (2m)
	// ValidityDeadline remains creation + 2m (no extension)
	// ========================================
	It("UT-EM-VWG-002: should not extend ValidityDeadline when StabilizationWindow is less than ValidityWindow", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-vwg-002"

		stabilizationWindow := 30 * time.Second
		validityWindow := 2 * time.Minute
		ea := makePendingEA(ns, name, stabilizationWindow)

		r, fc := makeReconcilerWithConfig(s, validityWindow, ea)

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.ValidityDeadline).NotTo(BeNil())

		expectedDeadline := fetchedEA.CreationTimestamp.Add(validityWindow)
		Expect(fetchedEA.Status.ValidityDeadline.Time).To(
			BeTemporally("~", expectedDeadline, 2*time.Second),
			"ValidityDeadline should be creation + ValidityWindow (2m) with no extension")
	})

	// ========================================
	// UT-EM-VWG-003: StabilizationWindow == ValidityWindow (boundary)
	// ValidityDeadline must be extended to creation + SW + VW
	// ========================================
	It("UT-EM-VWG-003: should extend ValidityDeadline when StabilizationWindow equals ValidityWindow", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-vwg-003"

		window := 2 * time.Minute
		ea := makePendingEA(ns, name, window)

		r, fc := makeReconcilerWithConfig(s, window, ea)

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.ValidityDeadline).NotTo(BeNil())

		expectedDeadline := fetchedEA.CreationTimestamp.Add(window + window)
		Expect(fetchedEA.Status.ValidityDeadline.Time).To(
			BeTemporally("~", expectedDeadline, 2*time.Second),
			"ValidityDeadline should be creation + 2*window when StabilizationWindow == ValidityWindow")

		Expect(fetchedEA.Status.ValidityDeadline.Time.After(fetchedEA.Status.PrometheusCheckAfter.Time)).To(BeTrue(),
			"ValidityDeadline must be after PrometheusCheckAfter (invariant)")
	})
})
