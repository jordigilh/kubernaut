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

// Alert Deferral Requeue Cap Test
//
// Issue #254: Captures the monolith's Step 7b behavior — when all components
// except alert are done and the alert is deferred for a proactive signal (#277),
// the reconciler persists any pending status changes and returns with a precise
// requeue capped at the ValidityDeadline.
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

// noopAlertManagerClient satisfies emclient.AlertManagerClient but is never called
// in this test — the alert deferral logic runs BEFORE the AM client is invoked.
type noopAlertManagerClient struct{}

func (s *noopAlertManagerClient) GetAlerts(_ context.Context, _ emclient.AlertFilters) ([]emclient.Alert, error) {
	return nil, nil
}

func (s *noopAlertManagerClient) Ready(_ context.Context) error {
	return nil
}

var _ = Describe("Alert Deferral Requeue Cap (UT-EM-254-004, #254)", func() {

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
		cfg.PrometheusEnabled = false
		cfg.AlertManagerEnabled = true

		r := controller.NewReconciler(
			fakeClient, fakeClient,
			s,
			record.NewFakeRecorder(100),
			emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			nil, &noopAlertManagerClient{},
			nil, nil,
			cfg,
		)
		return r, fakeClient
	}

	// seedAlertDeferredEA creates an EA already in Assessing with
	// AlertManagerCheckAfter in the future. Hash and health are NOT pre-set
	// (they get computed in Step 7), and Prom is disabled (metrics auto-skip).
	// This avoids the Step 6.5 spec drift check (requires HashComputed=true)
	// and lets the reconciler naturally reach Step 7b (alert deferral requeue).
	seedAlertDeferredEA := func(ns, name string, alertCheckAfter, validityDeadline time.Time) *eav1.EffectivenessAssessment {
		aca := metav1.NewTime(alertCheckAfter)
		vd := metav1.NewTime(validityDeadline)
		pca := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: ns,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-ad-" + name,
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 0},
				},
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:                  eav1.PhaseAssessing,
				ValidityDeadline:       &vd,
				PrometheusCheckAfter:   &pca,
				AlertManagerCheckAfter: &aca,
			},
		}
	}

	It("UT-EM-254-004a: Requeue is capped at remaining alert deferral time", func() {
		s := buildScheme()
		alertCheckAfter := time.Now().Add(3 * time.Minute)
		validityDeadline := time.Now().Add(25 * time.Minute)
		ea := seedAlertDeferredEA("default", "ea-ad-004a", alertCheckAfter, validityDeadline)
		r, _ := makeReconciler(s, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-ad-004a", Namespace: "default",
		}}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// Step 7b returns with requeue capped at alert deferral time.
		// Alert check is ~3 min from now, well within validity deadline.
		Expect(result.RequeueAfter).To(BeNumerically(">", 2*time.Minute),
			"Requeue must reflect remaining alert deferral time")
		Expect(result.RequeueAfter).To(BeNumerically("<=", 3*time.Minute+time.Second),
			"Requeue must not exceed alert deferral remaining")
	})

	It("UT-EM-254-004b: Requeue is capped at validity deadline when closer than alert deferral", func() {
		s := buildScheme()
		alertCheckAfter := time.Now().Add(10 * time.Minute)
		validityDeadline := time.Now().Add(2 * time.Minute)
		ea := seedAlertDeferredEA("default", "ea-ad-004b", alertCheckAfter, validityDeadline)
		r, _ := makeReconciler(s, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-ad-004b", Namespace: "default",
		}}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// capRequeueAtDeadline should cap the requeue to the remaining validity.
		// Validity is ~2m from now, alert deferral is ~10m.
		Expect(result.RequeueAfter).To(BeNumerically(">", 1*time.Minute),
			"Requeue must be positive (within validity)")
		Expect(result.RequeueAfter).To(BeNumerically("<=", 2*time.Minute+time.Second),
			"Requeue must be capped at validity deadline, not alert deferral")
	})
})
