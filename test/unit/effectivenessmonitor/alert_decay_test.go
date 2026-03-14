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
	emaudit "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/audit"
	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// decayAMClient implements emclient.AlertManagerClient returning configurable alerts.
type decayAMClient struct {
	alerts []emclient.Alert
}

func (c *decayAMClient) GetAlerts(_ context.Context, _ emclient.AlertFilters) ([]emclient.Alert, error) {
	return c.alerts, nil
}

func (c *decayAMClient) Ready(_ context.Context) error {
	return nil
}

// decayAuditSpy captures StoreAudit calls for assertion.
type decayAuditSpy struct {
	calls int
}

func (s *decayAuditSpy) StoreAudit(_ context.Context, _ *ogenclient.AuditEventRequest) error {
	s.calls++
	return nil
}

func (s *decayAuditSpy) Flush(_ context.Context) error { return nil }
func (s *decayAuditSpy) Close() error                  { return nil }

// ============================================================================
// EM ALERT DECAY DETECTION TESTS (Issue #369, BR-EM-012)
//
// Business Requirement: When a remediation succeeds (resource healthy, spec
// stable) but the Prometheus alert is still firing due to lookback window
// decay, the EM should keep the EA open instead of completing with a
// misleading AlertScore=0.0. This prevents the Gateway from creating
// duplicate RemediationRequests for the same signal.
//
// All tests drive the reconciler through the public Reconcile() method.
// ============================================================================
var _ = Describe("Alert Decay Detection (Issue #369, BR-EM-012)", func() {

	buildScheme := func() *runtime.Scheme {
		s := runtime.NewScheme()
		Expect(eav1.AddToScheme(s)).To(Succeed())
		Expect(corev1.AddToScheme(s)).To(Succeed())
		return s
	}

	// makeReconcilerWithAM creates a reconciler with AM enabled and a configurable AM client.
	makeReconcilerWithAM := func(s *runtime.Scheme, amClient emclient.AlertManagerClient, auditMgr *emaudit.Manager, objs ...client.Object) (*controller.Reconciler, client.Client) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithStatusSubresource(&eav1.EffectivenessAssessment{}).
			Build()

		cfg := controller.DefaultReconcilerConfig()
		cfg.AlertManagerEnabled = true
		cfg.PrometheusEnabled = false
		cfg.ValidityWindow = 1 * time.Hour

		r := controller.NewReconciler(
			fakeClient, s,
			record.NewFakeRecorder(100),
			emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			nil,       // Prometheus client (disabled)
			amClient,
			auditMgr,
			nil, // DSQuerier
			cfg,
		)
		return r, fakeClient
	}

	// seedDecayEA creates an EA in Assessing phase with health=OK, hash=done,
	// alert=not-yet-assessed, validity in the future.
	seedDecayEA := func(ns, name string) *eav1.EffectivenessAssessment {
		healthScore := 1.0
		futureDeadline := metav1.NewTime(time.Now().Add(1 * time.Hour))
		pastCheck := metav1.NewTime(time.Now().Add(-5 * time.Minute))
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         ns,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-" + name,
				RemediationRequestPhase: "Verifying",
				SignalName:              "HighMemoryUsage",
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
				Phase:                 eav1.PhaseAssessing,
				ValidityDeadline:      &futureDeadline,
				PrometheusCheckAfter:  &pastCheck,
				AlertManagerCheckAfter: &pastCheck,
				Components: eav1.EAComponents{
					HealthAssessed: true,
					HealthScore:    &healthScore,
					HashComputed:   true,
				},
			},
		}
	}

	// firingAMClient returns a mock AM client where the alert is still active.
	firingAMClient := func() *decayAMClient {
		return &decayAMClient{
			alerts: []emclient.Alert{
				{
					Labels: map[string]string{"alertname": "HighMemoryUsage", "namespace": "test-ns"},
					State:  "active",
				},
			},
		}
	}

	// resolvedAMClient returns a mock AM client where the alert has resolved.
	resolvedAMClient := func() *decayAMClient {
		return &decayAMClient{
			alerts: []emclient.Alert{},
		}
	}

	// ========================================
	// UT-EM-DECAY-001: System prevents premature EA completion during decay
	// ========================================
	It("UT-EM-DECAY-001: should keep EA open when resource is healthy but alert still firing (decay)", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-decay-001"

		ea := seedDecayEA(ns, name)
		r, fc := makeReconcilerWithAM(s, firingAMClient(), nil, ea)

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseAssessing),
			"EA should remain in Assessing (not prematurely completed)")
		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeFalse(),
			"AlertAssessed should be false (decay detection keeps EA open for re-check)")
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(1)),
			"AlertDecayRetries should be 1 (first decay detection)")
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"Reconciler should requeue for next decay check")
	})

	// ========================================
	// UT-EM-DECAY-002: System reports full effectiveness when alert resolves
	// ========================================
	It("UT-EM-DECAY-002: should complete with full reason and AlertScore=1.0 when alert resolves after decay monitoring", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-decay-002"

		ea := seedDecayEA(ns, name)
		ea.Status.Components.AlertDecayRetries = 3
		r, fc := makeReconcilerWithAM(s, resolvedAMClient(), nil, ea)

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue(),
			"AlertAssessed should be true (alert resolved, assessment complete)")
		Expect(fetchedEA.Status.Components.AlertScore).To(HaveValue(Equal(1.0)),
			"AlertScore should be 1.0 (alert confirmed resolved)")
		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted),
			"EA should be Completed (all components done)")
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull),
			"Reason should be 'full' (all components assessed)")
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(3)),
			"AlertDecayRetries should be preserved for operator observability")
	})

	// ========================================
	// UT-EM-DECAY-003: System distinguishes decay timeout from generic partial
	// ========================================
	It("UT-EM-DECAY-003: should set alert_decay_timeout when validity expires during decay monitoring", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-decay-003"

		ea := seedDecayEA(ns, name)
		ea.Status.Components.AlertDecayRetries = 5
		pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Hour))
		ea.Status.ValidityDeadline = &pastDeadline

		cfg := controller.DefaultReconcilerConfig()
		cfg.AlertManagerEnabled = true
		cfg.PrometheusEnabled = false
		cfg.ValidityWindow = 1 * time.Millisecond

		fakeClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(ea).
			WithStatusSubresource(&eav1.EffectivenessAssessment{}).
			Build()

		r := controller.NewReconciler(
			fakeClient, s,
			record.NewFakeRecorder(100),
			emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			nil, firingAMClient(),
			nil, nil,
			cfg,
		)

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted),
			"EA should be Completed (validity expired)")
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonAlertDecayTimeout),
			"Reason should be 'alert_decay_timeout' (not 'partial'), distinguishing active decay monitoring from never-checked")
	})

	// ========================================
	// UT-EM-DECAY-004: No false decay detection on non-pod resources
	// ========================================
	It("UT-EM-DECAY-004: should assess alert normally when HealthScore is nil (non-pod resource)", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-decay-004"

		ea := seedDecayEA(ns, name)
		ea.Status.Components.HealthScore = nil

		r, fc := makeReconcilerWithAM(s, firingAMClient(), nil, ea)

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue(),
			"AlertAssessed should be true (assessed normally, not deferred)")
		Expect(fetchedEA.Status.Components.AlertScore).To(HaveValue(Equal(0.0)),
			"AlertScore should be 0.0 (alert firing, recorded as-is)")
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(0)),
			"AlertDecayRetries should be 0 (decay detection was not triggered)")
	})

	// ========================================
	// UT-EM-DECAY-005: Spec drift aborts decay monitoring immediately
	// ========================================
	It("UT-EM-DECAY-005: should complete with spec_drift when target spec changes during decay monitoring", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-decay-005"

		ea := seedDecayEA(ns, name)
		ea.Status.Components.AlertDecayRetries = 2
		ea.Status.Components.PostRemediationSpecHash = "sha256:abc123"
		ea.Status.Components.CurrentSpecHash = "sha256:abc123"

		r, fc := makeReconcilerWithAM(s, firingAMClient(), nil, ea)

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted),
			"EA should be Completed (spec drift detected)")
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonSpecDrift),
			"Reason should be 'spec_drift' (overrides decay monitoring)")
	})

	// ========================================
	// UT-EM-DECAY-006: Accurate decay retry counting
	// ========================================
	It("UT-EM-DECAY-006: should accurately increment AlertDecayRetries on each reconcile", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-decay-006"

		ea := seedDecayEA(ns, name)
		r, fc := makeReconcilerWithAM(s, firingAMClient(), nil, ea)

		for i := int32(1); i <= 3; i++ {
			_, err := r.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
			})
			Expect(err).ToNot(HaveOccurred())

			fetchedEA := &eav1.EffectivenessAssessment{}
			Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

			Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(i),
				"AlertDecayRetries should be %d after reconcile %d", i, i)
			Expect(fetchedEA.Status.Components.AlertAssessed).To(BeFalse(),
				"AlertAssessed should remain false throughout decay monitoring")
		}
	})

	// ========================================
	// UT-EM-DECAY-007: Single audit entry per decay detection
	// ========================================
	It("UT-EM-DECAY-007: should emit exactly one audit event on first decay detection, silence on subsequent", func() {
		s := buildScheme()
		ns := "test-ns"
		name := "ea-decay-007"

		spy := &decayAuditSpy{}
		auditMgr := emaudit.NewManager(spy, ctrl.Log.WithName("test"))

		ea := seedDecayEA(ns, name)
		r, fc := makeReconcilerWithAM(s, firingAMClient(), auditMgr, ea)

		// First reconcile — should emit decay detected audit event
		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())
		firstCallCount := spy.calls
		Expect(firstCallCount).To(BeNumerically(">=", 1),
			"First reconcile should emit at least one audit event (alert_decay.detected)")

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(1)))

		// Second reconcile — should NOT emit another decay audit event
		_, err = r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(2)))

		Expect(spy.calls - firstCallCount).To(Equal(0),
			"No additional audit events should be emitted on subsequent decay re-checks (silence on retries)")
	})
})
