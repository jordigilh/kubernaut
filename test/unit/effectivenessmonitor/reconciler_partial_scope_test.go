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

		r := controller.NewReconciler(
			fakeClient, fakeClient,
			s,
			record.NewFakeRecorder(100),
			emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			nil, nil,
			nil, dsQuerier,
			cfg,
		)
		return r, fakeClient
	}

	It("UT-EM-254-006a: Within grace period — requeues at 5s for scope re-evaluation", func() {
		s := buildScheme()

		// EA created very recently (within 30s grace period).
		// Hash and health are NOT pre-set — they get computed in Step 7
		// (avoids Step 6.5 spec drift since HashComputed starts as false).
		// After Step 7 computes hash+health, Step 7a checks partial scope.
		vd := metav1.NewTime(time.Now().Add(25 * time.Minute))
		pca := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ea-partial-006a", Namespace: "default",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Second)),
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

		r, _ := makeReconciler(s, &partialScopeDSQuerier{}, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-partial-006a", Namespace: "default",
		}}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// Within grace period → requeue at 5s
		Expect(result.RequeueAfter).To(Equal(5*time.Second),
			"Within grace period: must requeue at exactly 5s for scope re-evaluation")
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
