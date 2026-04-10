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

// EntryPhase Immutability Characterization Test
//
// Issue #254, §17.2: `currentPhase` is read once from `ea.Status.Phase` at
// Reconcile entry (~line 263) and never updated. Later in-memory mutations to
// `ea.Status.Phase` (e.g., Step 6 sets Phase=Assessing) do not affect the
// original `currentPhase` value. Branch decisions at Steps 3b, 5, and 6 all
// use the entry-time value.
//
// If extraction replaces `currentPhase` with fresh reads of `ea.Status.Phase`,
// behavior would silently drift. This characterization test locks down the
// read-once semantics by verifying that transitions from intermediate phases
// produce the expected outcomes.
//
// This test MUST pass against the monolith and after decomposition.
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

var _ = Describe("EntryPhase Immutability (UT-EM-254-007, #254)", func() {

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

	It("UT-EM-254-007a: Pending → transitions through Assessing to Completed in single reconcile", func() {
		s := buildScheme()
		recorder := record.NewFakeRecorder(100)
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ea-entry-007a", Namespace: "default",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-entry-007a",
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: "default"},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: "default"},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 0},
				},
			},
		}
		r, fc := makeReconciler(s, recorder, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-entry-007a", Namespace: "default",
		}}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// Pending + StabilizationWindow=0 + disabled components → completes in 1 reconcile.
		// currentPhase=Pending at entry → Step 6 condition matches → transitions to Assessing
		// in-memory → all components done → completing → Status().Update → done.
		Expect(result).To(Equal(ctrl.Result{}),
			"Must complete without requeue")

		fetched := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(ctx, req.NamespacedName, fetched)).To(Succeed())

		Expect(fetched.Status.Phase).To(Equal(eav1.PhaseCompleted),
			"Must reach Completed from Pending in one reconcile")
		Expect(fetched.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull),
			"Must complete with 'full' reason")

		// Verify Assessing transition event was emitted (proves pendingTransition=true).
		foundAssessing := false
		close(recorder.Events)
		for event := range recorder.Events {
			if containsAny(event, "Assessing", "AssessmentStarted") {
				foundAssessing = true
			}
		}
		Expect(foundAssessing).To(BeTrue(),
			"Assessing transition event must be emitted (proves entryPhase=Pending triggered transition)")
	})

	It("UT-EM-254-007b: Stabilizing (persisted) → transitions to Assessing when window elapsed", func() {
		s := buildScheme()
		recorder := record.NewFakeRecorder(100)

		// EA was persisted as Stabilizing with timing set. Stabilization has now elapsed.
		vd := metav1.NewTime(time.Now().Add(25 * time.Minute))
		pca := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ea-entry-007b", Namespace: "default",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-entry-007b",
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: "default"},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: "default"},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 5 * time.Minute},
				},
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:                  eav1.PhaseStabilizing,
				ValidityDeadline:       &vd,
				PrometheusCheckAfter:   &pca,
				AlertManagerCheckAfter: &pca,
			},
		}
		r, fc := makeReconciler(s, recorder, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-entry-007b", Namespace: "default",
		}}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// currentPhase=Stabilizing at entry → Step 5 skipped (window elapsed →
		// WindowActive) → Step 6 condition matches (Stabilizing is in the list)
		// → transitions to Assessing in-memory → all components done → completes.
		Expect(result).To(Equal(ctrl.Result{}),
			"Must complete without requeue")

		fetched := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(ctx, req.NamespacedName, fetched)).To(Succeed())

		Expect(fetched.Status.Phase).To(Equal(eav1.PhaseCompleted),
			"Must reach Completed from Stabilizing in one reconcile when window elapsed")

		// Verify Assessing transition event was emitted.
		foundAssessing := false
		close(recorder.Events)
		for event := range recorder.Events {
			if containsAny(event, "Assessing", "AssessmentStarted") {
				foundAssessing = true
			}
		}
		Expect(foundAssessing).To(BeTrue(),
			"Assessing transition event must be emitted (proves entryPhase=Stabilizing triggered transition)")
	})

	It("UT-EM-254-007c: Assessing (already transitioned) skips Step 6 transition", func() {
		s := buildScheme()
		recorder := record.NewFakeRecorder(100)

		// EA already in Assessing — some components done, some not.
		// currentPhase=Assessing at entry → Step 6 condition does NOT match
		// (Assessing is not in [Pending, WFP, Stabilizing, ""]) → pendingTransition=false.
		vd := metav1.NewTime(time.Now().Add(25 * time.Minute))
		pca := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ea-entry-007c", Namespace: "default",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-entry-007c",
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: "default"},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: "default"},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 0},
				},
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:                  eav1.PhaseAssessing,
				ValidityDeadline:       &vd,
				PrometheusCheckAfter:   &pca,
				AlertManagerCheckAfter: &pca,
			},
		}
		r, fc := makeReconciler(s, recorder, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-entry-007c", Namespace: "default",
		}}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// Should complete (all disabled components marked as done).
		Expect(result).To(Equal(ctrl.Result{}))

		fetched := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(ctx, req.NamespacedName, fetched)).To(Succeed())
		Expect(fetched.Status.Phase).To(Equal(eav1.PhaseCompleted))

		// Verify NO Assessing transition event (pendingTransition=false
		// because currentPhase=Assessing didn't trigger Step 6).
		close(recorder.Events)
		for event := range recorder.Events {
			Expect(event).NotTo(ContainSubstring("AssessmentStarted"),
				"Already-Assessing EA must NOT emit AssessmentStarted (no pending transition)")
		}
	})
})

// containsAny checks if a string contains any of the provided substrings.
func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
