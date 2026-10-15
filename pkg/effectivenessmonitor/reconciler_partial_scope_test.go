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

// Partial Scope Grace Period Test
//
// Issue #254: Captures the monolith's Step 7a behavior — when the DS scope
// is "partial" (workflow started but not completed), health+hash are done,
// and the EA is within the 30s grace period, the reconciler requeues at 5s
// to re-evaluate scope. After the grace period, it completes as "partial".
//
// This test MUST pass against the monolith and after decomposition.
package effectivenessmonitor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

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
	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
)

// partialScopeDSQuerier returns partial scope: workflow started but not completed.
type partialScopeDSQuerier struct{}

func (q *partialScopeDSQuerier) QueryPreRemediationHash(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (q *partialScopeDSQuerier) HasWorkflowStarted(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func (q *partialScopeDSQuerier) HasWorkflowCompleted(_ context.Context, _ string) (bool, error) {
	return false, nil
}

var _ emclient.DataStorageQuerier = (*partialScopeDSQuerier)(nil)

var _ = Describe("Partial Scope Grace (UT-EM-254-006, #254)", func() {

	buildScheme := func() *runtime.Scheme {
		s := runtime.NewScheme()
		Expect(eav1.AddToScheme(s)).To(Succeed())
		Expect(corev1.AddToScheme(s)).To(Succeed())
		return s
	}

	makeReconciler := func(s *runtime.Scheme, dsQuerier emclient.DataStorageQuerier, objs ...client.Object) (*controller.Reconciler, client.Client) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithStatusSubresource(&eav1.EffectivenessAssessment{}).
			Build()

		cfg := controller.DefaultReconcilerConfig()
		cfg.PrometheusEnabled = false
		cfg.AlertManagerEnabled = false

		r := controller.NewReconciler(controller.ReconcilerDeps{
			Client:             fakeClient,
			APIReader:          fakeClient,
			Scheme:             s,
			Recorder:           record.NewFakeRecorder(100),
			Metrics:            emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			PrometheusClient:   nil,
			AlertManagerClient: nil,
			AuditManager:       nil,
			DSQuerier:          dsQuerier,
		}, cfg)
		return r, fakeClient
	}

	It("UT-EM-254-006a: Within grace period — requeues at 5s for scope re-evaluation", func() {
		scenarioAt := time.Now()
		s := buildScheme()
		schemeReadyAt := time.Now()

		constructionAt := time.Now()
		r, fc := makeReconciler(s, &partialScopeDSQuerier{})
		constructionReadyAt := time.Now()

		// EA created very recently (within 30s grace period).
		// Establish its age after dependency setup, not before the fake
		// client's managed-fields schema initialization (BR-EM-001).
		// Hash and health are NOT pre-set — they get computed in Step 7
		// (avoids Step 6.5 spec drift since HashComputed starts as false).
		// After Step 7 computes hash+health, Step 7a checks partial scope.
		fixtureAt := time.Now()
		vd := metav1.NewTime(fixtureAt.Add(25 * time.Minute))
		pca := metav1.NewTime(fixtureAt.Add(-1 * time.Minute))
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ea-partial-006a", Namespace: "default",
				CreationTimestamp: metav1.NewTime(fixtureAt.Add(-5 * time.Second)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-partial-006a",
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

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-partial-006a", Namespace: "default",
		}}
		createAt := time.Now()
		Expect(fc.Create(ctx, ea)).To(Succeed())
		createElapsed := time.Since(createAt)

		// BR-EM-001 / UT-EM-254-006a: verify the same now-5s fixture age
		// at dispatch without changing production grace or a stored timestamp.
		before := &eav1.EffectivenessAssessment{}
		beforeReadAt := time.Now()
		Expect(fc.Get(ctx, req.NamespacedName, before)).To(Succeed())
		dispatchAt := time.Now()
		GinkgoWriter.Printf("UT-EM-254-006a scheme=%s fake_client_reconciler=%s fixture_create=%s pre_dispatch_read=%s setup_total=%s persisted_creation=%s age_before_reconcile=%s\n",
			schemeReadyAt.Sub(scenarioAt), constructionReadyAt.Sub(constructionAt), createElapsed, dispatchAt.Sub(beforeReadAt), dispatchAt.Sub(scenarioAt),
			before.CreationTimestamp.Format(time.RFC3339Nano), dispatchAt.Sub(before.CreationTimestamp.Time))
		Expect(dispatchAt.Sub(before.CreationTimestamp.Time)).To(BeNumerically(">=", 5*time.Second))
		Expect(dispatchAt.Sub(before.CreationTimestamp.Time)).To(BeNumerically("<", 30*time.Second),
			"recent EA precondition must hold after fixture creation")
		Expect(before.Status.Phase).To(Equal(eav1.PhaseAssessing))
		Expect(before.Status.Components.HashComputed).To(BeFalse())
		Expect(before.Status.Components.HealthAssessed).To(BeFalse())
		reconcileAt := time.Now()
		result, err := r.Reconcile(ctx, req)
		reconcileElapsed := time.Since(reconcileAt)
		GinkgoWriter.Printf("UT-EM-254-006a reconcile=%s result=%+v error=%v age_after_reconcile=%s\n",
			reconcileElapsed, result, err, time.Since(before.CreationTimestamp.Time))
		Expect(err).NotTo(HaveOccurred())
		fetched := &eav1.EffectivenessAssessment{}
		readbackAt := time.Now()
		readbackErr := fc.Get(ctx, req.NamespacedName, fetched)
		GinkgoWriter.Printf("UT-EM-254-006a readback=%s error=%v phase=%s assessment_reason=%s total=%s\n",
			time.Since(readbackAt), readbackErr, fetched.Status.Phase, fetched.Status.AssessmentReason, time.Since(scenarioAt))
		Expect(readbackErr).NotTo(HaveOccurred())

		// Within grace period → requeue at 5s
		Expect(result.RequeueAfter).To(Equal(5*time.Second),
			"Within grace period: must requeue at exactly 5s for scope re-evaluation")
		Expect(fetched.Status.Phase).To(Equal(eav1.PhaseAssessing))
		Expect(fetched.Status.CompletedAt).To(BeNil())
		Expect(fetched.Status.AssessmentReason).To(BeEmpty())

		// The grace requeue precedes the status flush. Prove real component
		// activity through its existing metrics, not fabricated persisted flags.
		for _, component := range []string{"hash", "health"} {
			var assessments float64
			for _, outcome := range []string{"success", "error", "skipped"} {
				counter, metricErr := r.Metrics.ComponentAssessmentsTotal.GetMetricWithLabelValues(component, outcome)
				Expect(metricErr).NotTo(HaveOccurred())
				assessments += testutil.ToFloat64(counter)
			}
			GinkgoWriter.Printf("UT-EM-254-006a component=%s assessments=%g\n", component, assessments)
			Expect(assessments).To(Equal(float64(1)),
				"%s assessment must remain observable during partial grace", component)
		}
	})

	It("UT-EM-254-006b: After grace period — completes as 'partial'", func() {
		s := buildScheme()

		// EA created >30s ago (past grace period). Hash and health are NOT
		// pre-set — they get computed in Step 7 (avoids Step 6.5 spec drift).
		vd := metav1.NewTime(time.Now().Add(25 * time.Minute))
		pca := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ea-partial-006b", Namespace: "default",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-60 * time.Second)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-partial-006b",
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

		r, fc := makeReconciler(s, &partialScopeDSQuerier{}, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-partial-006b", Namespace: "default",
		}}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// Past grace period → complete as partial
		Expect(result.Requeue).To(BeFalse(),
			"After grace: must complete without requeue")
		Expect(result.RequeueAfter).To(BeZero(),
			"After grace: must have zero RequeueAfter")

		fetched := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(ctx, req.NamespacedName, fetched)).To(Succeed())

		Expect(fetched.Status.Phase).To(Equal(eav1.PhaseCompleted),
			"Phase must be Completed after grace expires")
		Expect(fetched.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonPartial),
			"AssessmentReason must be 'Partial'")
		Expect(fetched.Status.CompletedAt).NotTo(BeNil(),
			"CompletedAt must be set")
	})
})
