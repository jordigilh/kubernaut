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

// Golden Snapshot Test: Stabilizing Path
//
// Issue #254: Captures the monolith's Stabilizing behavior as a frozen baseline.
// The Stabilizing path is entered when the StabilizationWindow hasn't elapsed yet.
// The reconciler persists derived timing (ValidityDeadline, check-after times),
// transitions to Stabilizing phase, and requeues until stabilization completes.
//
// This test MUST pass against the monolith (RED phase captures baseline)
// and continue to pass after decomposition (GREEN/REFACTOR phases).
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

var _ = Describe("Golden Snapshot — Stabilizing Path (UT-EM-254-002, #254)", func() {

	const stabilizationWindow = 5 * time.Minute

	buildScheme := func() *runtime.Scheme {
		s := runtime.NewScheme()
		Expect(eav1.AddToScheme(s)).To(Succeed())
		Expect(corev1.AddToScheme(s)).To(Succeed())
		return s
	}

	makeReconciler := func(s *runtime.Scheme, recorder *record.FakeRecorder, objs ...client.Object) (*controller.Reconciler, client.Client) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithStatusSubresource(&eav1.EffectivenessAssessment{}).
			Build()

		cfg := controller.DefaultReconcilerConfig()
		cfg.PrometheusEnabled = false
		cfg.AlertManagerEnabled = false

		r := controller.NewReconciler(
			fakeClient, fakeClient,
			s, recorder,
			emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			nil, nil,
			nil, nil,
			cfg,
		)
		return r, fakeClient
	}

	seedStabilizingEA := func(ns, name string) *eav1.EffectivenessAssessment {
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         ns,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-stab-" + name,
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

	It("UT-EM-254-002: Stabilizing path produces correct phase, timing, and requeue", func() {
		s := buildScheme()
		recorder := record.NewFakeRecorder(100)
		ea := seedStabilizingEA("default", "ea-stab-002")
		r, fc := makeReconciler(s, recorder, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-stab-002", Namespace: "default",
		}}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// --- Snapshot: ctrl.Result ---
		// Stabilizing returns with RequeueAfter = remaining stabilization time.
		// Creation was 1m ago, stabilization is 5m → ~4m remaining.
		Expect(result.RequeueAfter).To(BeNumerically(">", 3*time.Minute),
			"RequeueAfter must reflect remaining stabilization time")
		Expect(result.RequeueAfter).To(BeNumerically("<=", stabilizationWindow),
			"RequeueAfter must not exceed the full StabilizationWindow")

		// --- Snapshot: EA Status ---
		fetched := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(ctx, req.NamespacedName, fetched)).To(Succeed())

		Expect(fetched.Status.Phase).To(Equal(eav1.PhaseStabilizing),
			"Phase must transition to Stabilizing")
		Expect(fetched.Status.ValidityDeadline).NotTo(BeNil(),
			"ValidityDeadline must be persisted during Stabilizing (BR-EM-009)")
		Expect(fetched.Status.PrometheusCheckAfter).NotTo(BeNil(),
			"PrometheusCheckAfter must be persisted during Stabilizing")
		Expect(fetched.Status.AlertManagerCheckAfter).NotTo(BeNil(),
			"AlertManagerCheckAfter must be persisted during Stabilizing")

		// Derived timing validation: PrometheusCheckAfter = creation + stabilization
		expectedCheckAfter := ea.CreationTimestamp.Add(stabilizationWindow)
		Expect(fetched.Status.PrometheusCheckAfter.Time).To(
			BeTemporally("~", expectedCheckAfter, 2*time.Second),
			"PrometheusCheckAfter must equal creation + StabilizationWindow")

		// ValidityDeadline = creation + ValidityWindow (default 30m)
		Expect(fetched.Status.ValidityDeadline.Time).To(
			BeTemporally(">", expectedCheckAfter),
			"ValidityDeadline must be after PrometheusCheckAfter")

		// --- Snapshot: Components ---
		// No component checks run during stabilization.
		Expect(fetched.Status.Components.HashComputed).To(BeFalse(),
			"Hash must NOT be computed during Stabilizing")
		Expect(fetched.Status.Components.HealthAssessed).To(BeFalse(),
			"Health must NOT be assessed during Stabilizing")

		// --- Snapshot: Events ---
		Eventually(recorder.Events).Should(Receive(ContainSubstring("Scheduled")),
			"Stabilizing must emit a Scheduled event on first entry")
	})

	It("UT-EM-254-002b: WFP→Stabilizing transition when HCA elapsed", func() {
		s := buildScheme()
		recorder := record.NewFakeRecorder(100)

		// EA already in WaitingForPropagation — HCA has now elapsed (creation was
		// far enough in the past that creation + HCD is before now), but
		// stabilization window hasn't elapsed yet.
		hcd := metav1.Duration{Duration: 30 * time.Second}
		vd := metav1.NewTime(time.Now().Add(25 * time.Minute))
		ca := metav1.NewTime(time.Now().Add(4 * time.Minute))
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "ea-wfp-to-stab",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-wfp-stab",
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "async-app", Namespace: "default"},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "async-app", Namespace: "default"},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: stabilizationWindow},
					HashComputeDelay:    &hcd,
				},
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:                  eav1.PhaseWaitingForPropagation,
				ValidityDeadline:       &vd,
				PrometheusCheckAfter:   &ca,
				AlertManagerCheckAfter: &ca,
			},
		}

		r, fc := makeReconciler(s, recorder, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-wfp-to-stab", Namespace: "default",
		}}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// Should requeue for remaining stabilization time
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"Must requeue during stabilization")

		fetched := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(ctx, req.NamespacedName, fetched)).To(Succeed())

		Expect(fetched.Status.Phase).To(Equal(eav1.PhaseStabilizing),
			"Must transition from WFP to Stabilizing when HCA elapsed")
	})
})
