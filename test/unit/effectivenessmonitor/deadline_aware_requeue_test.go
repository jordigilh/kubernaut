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
	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
)

// emptyPromQuerier returns empty results for all queries, causing assessMetrics
// to return Assessed=false (no metric data). This keeps MetricsAssessed=false
// so the reconciler reaches Step 10 (requeue for remaining components).
type emptyPromQuerier struct{}

func (q *emptyPromQuerier) Query(_ context.Context, _ string, _ time.Time) (*emclient.QueryResult, error) {
	return &emclient.QueryResult{}, nil
}

func (q *emptyPromQuerier) QueryRange(_ context.Context, _ string, _, _ time.Time, _ time.Duration) (*emclient.QueryResult, error) {
	return &emclient.QueryResult{}, nil
}

func (q *emptyPromQuerier) Ready(_ context.Context) error {
	return nil
}

// ============================================================================
// DEADLINE-AWARE REQUEUE TESTS (BR-EM-007, Issue #591 E2E flake)
//
// When an Assessing EA has incomplete components (e.g., metrics pending),
// the reconciler requeues at RequeueAssessmentInProgress (default 15s, CI 45s).
// If the ValidityDeadline is closer than that interval, the requeue must be
// capped at the remaining time so the expiry fires on time instead of up to
// one full interval late.
//
// Root cause: E2E-EM-HC-001 flaked because a 45s requeue overshot a ~10s
// remaining validity window after a pod restart, causing a 150s test timeout.
// ============================================================================
var _ = Describe("Deadline-Aware Requeue (BR-EM-007, Issue #591)", func() {

	const requeueInterval = 45 * time.Second

	buildScheme := func() *runtime.Scheme {
		s := runtime.NewScheme()
		Expect(eav1.AddToScheme(s)).To(Succeed())
		Expect(corev1.AddToScheme(s)).To(Succeed())
		return s
	}

	makeReconciler := func(s *runtime.Scheme, objs ...client.Object) (*controller.Reconciler, client.Client) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithStatusSubresource(&eav1.EffectivenessAssessment{}).
			Build()

		cfg := controller.DefaultReconcilerConfig()
		cfg.PrometheusEnabled = true
		cfg.AlertManagerEnabled = false
		cfg.RequeueAssessmentInProgress = requeueInterval

		r := controller.NewReconciler(
			fakeClient, fakeClient,
			s,
			record.NewFakeRecorder(100),
			emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			&emptyPromQuerier{}, nil,
			nil, nil,
			cfg,
		)
		return r, fakeClient
	}

	// seedAssessingEA creates an EA already in Assessing phase with a specific
	// ValidityDeadline and all components done except metrics.
	seedAssessingEA := func(ns, name string, deadline time.Time) *eav1.EffectivenessAssessment {
		dl := metav1.NewTime(deadline)
		healthScore := 0.0
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         ns,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
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
				ValidityDeadline: &dl,
				Components: eav1.EAComponents{
					HashComputed:   true,
					HealthAssessed: true,
					HealthScore:    &healthScore,
					AlertAssessed:  true,
					MetricsAssessed: false,
				},
			},
		}
	}

	// ========================================
	// UT-EM-DAR-001: Deadline closer than RequeueAssessmentInProgress
	// Requeue must be capped at remaining time to deadline.
	// BR-EM-007: Validity window enforcement precision.
	// ========================================
	It("UT-EM-DAR-001: should cap requeue at remaining validity time when deadline is closer than default interval", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-dar-001"

		deadline := time.Now().Add(10 * time.Second)
		ea := seedAssessingEA(ns, name, deadline)
		r, _ := makeReconciler(s, ea)

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"Behavior: reconciler must requeue when metrics are pending")
		Expect(result.RequeueAfter).To(BeNumerically("<=", 12*time.Second),
			"Correctness: requeue must be capped at ~remaining validity time (10s), not default interval (45s)")
		Expect(result.RequeueAfter).To(BeNumerically("<", requeueInterval),
			"Accuracy: requeue must be shorter than RequeueAssessmentInProgress when deadline is closer")
	})

	// ========================================
	// UT-EM-DAR-002: Deadline further than RequeueAssessmentInProgress
	// Requeue remains at the default interval.
	// ========================================
	It("UT-EM-DAR-002: should use default requeue interval when deadline is far away", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-dar-002"

		deadline := time.Now().Add(5 * time.Minute)
		ea := seedAssessingEA(ns, name, deadline)
		r, _ := makeReconciler(s, ea)

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(result.RequeueAfter).To(Equal(requeueInterval),
			"Correctness: requeue should remain at default interval when deadline is far away")
	})
})
