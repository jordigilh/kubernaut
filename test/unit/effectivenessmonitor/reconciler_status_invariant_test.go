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

// Status Update Invariant Test
//
// Issue #254: Verifies the per-path Status().Update count invariant.
// Each code path in the reconciler executes at most one Status().Update
// per Reconcile invocation. This test uses an interceptor to count
// SubResource update calls and asserts the exact count for each path.
//
// Per-path matrix (from audit §17.1):
//   WFP persist:          1 (then return)
//   Stabilizing:          1 (then return)
//   Alert deferral 7b:    0 or 1 (then return)
//   Step 9 (Assessing):   1 (then return)
//   completeAssessment:   1 (separate call stack)
//   failAssessment:       1 (separate call stack)
//
// No two Status().Update calls execute in the same Reconcile invocation.
package effectivenessmonitor

import (
	"context"
	"sync/atomic"
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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
)

var _ = Describe("Status Update Invariant (UT-EM-254-003, #254)", func() {

	buildScheme := func() *runtime.Scheme {
		s := runtime.NewScheme()
		Expect(eav1.AddToScheme(s)).To(Succeed())
		Expect(corev1.AddToScheme(s)).To(Succeed())
		return s
	}

	// makeReconcilerWithCounter creates a reconciler backed by a fake client
	// that counts SubResource Update calls via an interceptor.
	makeReconcilerWithCounter := func(s *runtime.Scheme, objs ...client.Object) (*controller.Reconciler, client.Client, *int32) {
		var updateCount int32

		fakeClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithStatusSubresource(&eav1.EffectivenessAssessment{}).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					atomic.AddInt32(&updateCount, 1)
					return c.Status().Update(ctx, obj, opts...)
				},
			}).
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
			nil, nil,
			cfg,
		)
		return r, fakeClient, &updateCount
	}

	seedPendingEA := func(ns, name string) *eav1.EffectivenessAssessment {
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: ns,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-inv-" + name,
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
		}
	}

	It("UT-EM-254-003a: Assessing path (Pending→Completed) executes exactly one Status().Update", func() {
		s := buildScheme()
		ea := seedPendingEA("default", "ea-inv-003a")
		r, _, updateCount := makeReconcilerWithCounter(s, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-inv-003a", Namespace: "default",
		}}

		// Reset counter
		atomic.StoreInt32(updateCount, 0)

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		count := atomic.LoadInt32(updateCount)
		Expect(count).To(Equal(int32(1)),
			"Assessing path (Pending→Completed with disabled components) must execute exactly one Status().Update")

		// Verify completion
		Expect(result.Requeue).To(BeFalse(), "Completed assessment must not requeue")
		Expect(result.RequeueAfter).To(BeZero(), "Completed assessment must have zero RequeueAfter")
	})

	It("UT-EM-254-003b: WFP path executes exactly one Status().Update", func() {
		s := buildScheme()
		hcd := metav1.Duration{Duration: 10 * time.Minute}
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ea-inv-003b", Namespace: "default",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-inv-003b",
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: "default"},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: "default"},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 0},
					HashComputeDelay:    &hcd,
				},
			},
		}
		r, _, updateCount := makeReconcilerWithCounter(s, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-inv-003b", Namespace: "default",
		}}

		atomic.StoreInt32(updateCount, 0)

		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		count := atomic.LoadInt32(updateCount)
		Expect(count).To(Equal(int32(1)),
			"WFP path must execute exactly one Status().Update (persist phase + timing)")
	})

	It("UT-EM-254-003c: Stabilizing path executes exactly one Status().Update", func() {
		s := buildScheme()
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ea-inv-003c", Namespace: "default",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-inv-003c",
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: "default"},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: "default"},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 5 * time.Minute},
				},
			},
		}
		r, _, updateCount := makeReconcilerWithCounter(s, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-inv-003c", Namespace: "default",
		}}

		atomic.StoreInt32(updateCount, 0)

		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		count := atomic.LoadInt32(updateCount)
		Expect(count).To(Equal(int32(1)),
			"Stabilizing path must execute exactly one Status().Update (persist phase + timing)")
	})

	It("UT-EM-254-003d: Terminal state (Completed) executes zero Status().Updates", func() {
		s := buildScheme()
		now := metav1.Now()
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ea-inv-003d", Namespace: "default",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-inv-003d",
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: "default"},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: "default"},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 0},
				},
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseCompleted,
				AssessmentReason: eav1.AssessmentReasonFull,
				CompletedAt:      &now,
			},
		}
		r, _, updateCount := makeReconcilerWithCounter(s, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-inv-003d", Namespace: "default",
		}}

		atomic.StoreInt32(updateCount, 0)

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}), "Terminal state must return empty result")

		count := atomic.LoadInt32(updateCount)
		Expect(count).To(Equal(int32(0)),
			"Terminal state must execute zero Status().Updates")
	})
})
