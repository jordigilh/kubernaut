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

package effectivenessmonitor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	emaudit "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/audit"
	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/conditions"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
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

// decayAuditSpy captures StoreAudit calls for assertion, tracking event types
// to distinguish decay audit events from health/alert re-probe events.
type decayAuditSpy struct {
	calls      int
	decayCalls int
}

func (s *decayAuditSpy) StoreAudit(_ context.Context, req *ogenclient.AuditEventRequest) error {
	s.calls++
	if req != nil && req.EventType == "effectiveness.alert_decay.detected" {
		s.decayCalls++
	}
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

		r := controller.NewReconciler(controller.ReconcilerDeps{
			Client:             fakeClient,
			APIReader:          fakeClient,
			Scheme:             s,
			Recorder:           record.NewFakeRecorder(100),
			Metrics:            emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			PrometheusClient:   nil, // Prometheus client (disabled)
			AlertManagerClient: amClient,
			AuditManager:       auditMgr,
			DSQuerier:          nil, // DSQuerier
		}, cfg)
		return r, fakeClient
	}

	// seedDecayEA creates an EA in Assessing phase with health=OK, hash=done,
	// alert=not-yet-assessed, validity in the future.
	seedDecayEA := func(name string) *eav1.EffectivenessAssessment {
		healthScore := 1.0
		futureDeadline := metav1.NewTime(time.Now().Add(1 * time.Hour))
		pastCheck := metav1.NewTime(time.Now().Add(-5 * time.Minute))
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         testNs,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-" + name,
				RemediationRequestPhase: "Verifying",
				SignalName:              "HighMemoryUsage",
				SignalTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: testNs,
				},
				RemediationTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: testNs,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 0},
				},
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:                  eav1.PhaseAssessing,
				ValidityDeadline:       &futureDeadline,
				PrometheusCheckAfter:   &pastCheck,
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
					Labels: map[string]string{"alertname": "HighMemoryUsage", "namespace": testNs},
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

	// seedHealthyPod creates a Running/Ready pod matching the label selector
	// used by getTargetHealthStatus (client.MatchingLabels{"app": targetName}).
	// Health scorer returns 1.0 for TotalReplicas=1, ReadyReplicas=1, RestartCount=0.
	seedHealthyPod := func() *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-pod-0",
				Namespace: testNs,
				Labels:    map[string]string{"app": "test-app"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "main",
						Ready:        true,
						RestartCount: 0,
					},
				},
			},
		}
	}

	// ========================================
	// UT-EM-DECAY-001: System prevents premature EA completion during decay
	// ========================================
	It("UT-EM-DECAY-001: should keep EA open when resource is healthy but alert still firing (decay)", func() {
		s := buildScheme()
		ns := testNs
		name := "ea-decay-001"

		ea := seedDecayEA(name)
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

		decayCond := meta.FindStatusCondition(fetchedEA.Status.Conditions, conditions.ConditionAlertDecayDetected)
		Expect(decayCond).ToNot(BeNil(), "AlertDecayDetected condition should be present")
		Expect(decayCond.Status).To(Equal(metav1.ConditionTrue),
			"AlertDecayDetected should be True (decay actively monitored)")
		Expect(decayCond.Reason).To(Equal(conditions.ReasonDecayActive),
			"Reason should be DecayActive")
	})

	// ========================================
	// UT-EM-DECAY-002: System reports full effectiveness when alert resolves
	// ========================================
	It("UT-EM-DECAY-002: should complete with full reason and AlertScore=1.0 when alert resolves after decay monitoring", func() {
		s := buildScheme()
		ns := testNs
		name := "ea-decay-002"

		ea := seedDecayEA(name)
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

		decayCond := meta.FindStatusCondition(fetchedEA.Status.Conditions, conditions.ConditionAlertDecayDetected)
		Expect(decayCond).ToNot(BeNil(), "AlertDecayDetected condition should be present")
		Expect(decayCond.Status).To(Equal(metav1.ConditionFalse),
			"AlertDecayDetected should be False (decay resolved)")
		Expect(decayCond.Reason).To(Equal(conditions.ReasonDecayResolved),
			"Reason should be DecayResolved")
	})

	// ========================================
	// UT-EM-DECAY-003: System distinguishes decay timeout from generic partial
	// ========================================
	It("UT-EM-DECAY-003: should set alert_decay_timeout when validity expires during decay monitoring", func() {
		s := buildScheme()
		ns := testNs
		name := "ea-decay-003"

		ea := seedDecayEA(name)
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

		r := controller.NewReconciler(controller.ReconcilerDeps{
			Client:             fakeClient,
			APIReader:          fakeClient,
			Scheme:             s,
			Recorder:           record.NewFakeRecorder(100),
			Metrics:            emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			PrometheusClient:   nil,
			AlertManagerClient: firingAMClient(),
			AuditManager:       nil,
			DSQuerier:          nil,
		}, cfg)

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

		decayCond := meta.FindStatusCondition(fetchedEA.Status.Conditions, conditions.ConditionAlertDecayDetected)
		Expect(decayCond).ToNot(BeNil(), "AlertDecayDetected condition should be present")
		Expect(decayCond.Status).To(Equal(metav1.ConditionFalse),
			"AlertDecayDetected should be False (decay timed out)")
		Expect(decayCond.Reason).To(Equal(conditions.ReasonDecayTimeout),
			"Reason should be DecayTimeout")
	})

	// ========================================
	// UT-EM-DECAY-004: No false decay detection on non-pod resources
	// ========================================
	It("UT-EM-DECAY-004: should assess alert normally when HealthScore is nil (non-pod resource)", func() {
		s := buildScheme()
		ns := testNs
		name := "ea-decay-004"

		ea := seedDecayEA(name)
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

		decayCond := meta.FindStatusCondition(fetchedEA.Status.Conditions, conditions.ConditionAlertDecayDetected)
		Expect(decayCond).To(BeNil(),
			"AlertDecayDetected condition should NOT be present (decay was never triggered)")
	})

	// ========================================
	// UT-EM-DECAY-005: Spec drift aborts decay monitoring immediately
	// ========================================
	It("UT-EM-DECAY-005: should complete with spec_drift when target spec changes during decay monitoring", func() {
		s := buildScheme()
		ns := testNs
		name := "ea-decay-005"

		ea := seedDecayEA(name)
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

		decayCond := meta.FindStatusCondition(fetchedEA.Status.Conditions, conditions.ConditionAlertDecayDetected)
		Expect(decayCond).ToNot(BeNil(), "AlertDecayDetected condition should be present (resolved on completion)")
		Expect(decayCond.Status).To(Equal(metav1.ConditionFalse),
			"AlertDecayDetected should be False (spec drift terminated decay monitoring)")
		Expect(decayCond.Reason).To(Equal(conditions.ReasonDecayResolved),
			"Reason should be DecayResolved (early termination resolves decay)")
	})

	// ========================================
	// UT-EM-DECAY-006: Accurate decay retry counting
	// ========================================
	It("UT-EM-DECAY-006: should accurately increment AlertDecayRetries on each reconcile", func() {
		s := buildScheme()
		ns := testNs
		name := "ea-decay-006"

		ea := seedDecayEA(name)
		pod := seedHealthyPod()
		r, fc := makeReconcilerWithAM(s, firingAMClient(), nil, ea, pod)

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

			decayCond := meta.FindStatusCondition(fetchedEA.Status.Conditions, conditions.ConditionAlertDecayDetected)
			Expect(decayCond).ToNot(BeNil(), "AlertDecayDetected condition should be present on reconcile %d", i)
			Expect(decayCond.Status).To(Equal(metav1.ConditionTrue),
				"AlertDecayDetected should be True (actively monitoring) on reconcile %d", i)
			Expect(decayCond.Reason).To(Equal(conditions.ReasonDecayActive),
				"Reason should be DecayActive on reconcile %d", i)
		}
	})

	// ========================================
	// UT-EM-DECAY-007: Single audit entry per decay detection
	// ========================================
	It("UT-EM-DECAY-007: should emit exactly one audit event on first decay detection, silence on subsequent", func() {
		s := buildScheme()
		ns := testNs
		name := "ea-decay-007"

		spy := &decayAuditSpy{}
		auditMgr := emaudit.NewManager(spy, ctrl.Log.WithName("test"))

		ea := seedDecayEA(name)
		pod := seedHealthyPod()
		r, fc := makeReconcilerWithAM(s, firingAMClient(), auditMgr, ea, pod)

		// First reconcile — should emit decay detected audit event
		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(spy.decayCalls).To(Equal(1),
			"First reconcile should emit exactly one decay audit event")

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(1)))

		// Second reconcile — health re-probe emits a health audit event,
		// but NO additional decay audit event should be emitted.
		_, err = r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(2)))

		Expect(spy.decayCalls).To(Equal(1),
			"No additional decay audit events on subsequent re-checks (silence on retries)")
	})

	// ========================================
	// UT-EM-DECAY-008: Metrics negative kills decay hypothesis (proactive signal)
	// BR-EM-012: When metrics prove remediation failed (MetricsScore <= 0.0),
	// the alert is genuine — not decay. The EA completes with the alert score
	// reflecting the real failure.
	// ========================================
	It("UT-EM-DECAY-008: should complete EA normally when metrics are negative (proactive signal kills decay hypothesis)", func() {
		s := buildScheme()
		ns := testNs
		name := "ea-decay-008"

		ea := seedDecayEA(name)
		metricsScore := 0.0
		ea.Status.Components.MetricsAssessed = true
		ea.Status.Components.MetricsScore = &metricsScore

		r, fc := makeReconcilerWithAM(s, firingAMClient(), nil, ea)

		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted),
			"EA should complete (metrics gate prevents decay detection)")
		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue(),
			"AlertAssessed should be true (alert accepted at face value)")
		Expect(fetchedEA.Status.Components.AlertScore).To(HaveValue(Equal(0.0)),
			"AlertScore should be 0.0 (alert firing, remediation failed)")
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(0)),
			"AlertDecayRetries should be 0 (decay was never triggered)")
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull),
			"AssessmentReason should be 'full' (all components assessed, not alert_decay_timeout)")
	})

	// ========================================
	// UT-EM-DECAY-009: Metrics nil/unavailable is neutral
	// BR-EM-012: When MetricsScore is nil (Prometheus returned no data),
	// decay detection should still proceed — absence of data is not evidence
	// of failure.
	// ========================================
	It("UT-EM-DECAY-009: should continue decay monitoring when metrics are nil (neutral, not negative)", func() {
		s := buildScheme()
		ns := testNs
		name := "ea-decay-009"

		ea := seedDecayEA(name)
		ea.Status.Components.MetricsAssessed = true
		ea.Status.Components.MetricsScore = nil

		r, fc := makeReconcilerWithAM(s, firingAMClient(), nil, ea)

		result, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseAssessing),
			"EA should remain in Assessing (decay monitoring active)")
		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeFalse(),
			"AlertAssessed should be false (decay monitoring continues)")
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(1)),
			"AlertDecayRetries should be 1 (nil metrics treated as neutral)")
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"Reconciler should requeue for next decay check")
	})

	// ========================================
	// UT-EM-DECAY-010: Health re-probed live on each decay pass
	// BR-EM-012: While decay monitoring is active (AlertDecayRetries > 0,
	// alert not yet assessed), health is re-probed live from the K8s API on
	// every pass. This prevents stale data from masking a genuine failure.
	//
	// Issue #1701: HealthAssessed is NOT destructively cleared between
	// probes — it stays true (reflecting the last confirmed assessment)
	// throughout decay monitoring, so a terminal EA never falsely reports
	// "health assessment did not complete" if validity expires before the
	// next probe runs. Live re-probing (freshness) is proven separately by
	// UT-EM-DECAY-011, which shows a degraded re-probe result overriding
	// the earlier healthy score.
	// ========================================
	It("UT-EM-DECAY-010: should keep HealthAssessed=true while re-probing health on each decay pass", func() {
		s := buildScheme()
		ns := testNs
		name := "ea-decay-010"

		ea := seedDecayEA(name)
		pod := seedHealthyPod()
		r, fc := makeReconcilerWithAM(s, firingAMClient(), nil, ea, pod)

		// Pass 1: decay detected. HealthAssessed remains true (already
		// confirmed); it is not destructively reset just to force a
		// future re-probe.
		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue(),
			"HealthAssessed should remain true after pass 1 (last confirmed assessment, not reset — Issue #1701)")
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(1)),
			"AlertDecayRetries should be 1 after pass 1")

		// Pass 2: health re-probed live again (pod still healthy) because
		// decay monitoring is still active, even though HealthAssessed
		// never dropped to false in between.
		_, err = r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue(),
			"HealthAssessed should remain true after pass 2 (re-probed live, still confirmed)")
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(2)),
			"AlertDecayRetries should be 2 after pass 2")
		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseAssessing),
			"EA should remain in Assessing throughout decay monitoring")
		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeFalse(),
			"AlertAssessed should remain false (EA stays open)")
	})

	// ========================================
	// UT-EM-DECAY-011: Health degradation during decay kills hypothesis
	// BR-EM-012: If health degrades between decay passes (e.g., pod crashes
	// after memory increase was insufficient), the alert is genuine.
	// ========================================
	It("UT-EM-DECAY-011: should kill decay hypothesis when health degrades on re-probe", func() {
		s := buildScheme()
		ns := testNs
		name := "ea-decay-011"

		ea := seedDecayEA(name)
		pod := seedHealthyPod()
		r, fc := makeReconcilerWithAM(s, firingAMClient(), nil, ea, pod)

		// Pass 1: decay detected (health=1.0, alert=0.0, hash stable)
		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(1)),
			"Pass 1 should detect decay")

		// Simulate health degradation: delete the pod so re-probe returns HealthScore=0.0
		Expect(fc.Delete(context.Background(), pod)).To(Succeed())

		// Pass 2: health re-probed as 0.0, isAlertDecay returns false, alert assessed normally
		_, err = r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted),
			"EA should complete (health degraded, alert is genuine)")
		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue(),
			"AlertAssessed should be true (alert accepted at face value)")
		Expect(fetchedEA.Status.Components.AlertScore).To(HaveValue(Equal(0.0)),
			"AlertScore should be 0.0 (alert firing, remediation failed)")
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(1)),
			"AlertDecayRetries should be 1 (only pass 1 was decay)")
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull),
			"AssessmentReason should be 'full' (normal completion, not alert_decay_timeout)")
	})

	// ========================================
	// UT-EM-DECAY-012: Validity expiry mid-decay-monitoring preserves last
	// confirmed health assessment (Issue #1701)
	//
	// BR-EM-012 / BR-EM-001: A terminal EA must never report
	// Components.HealthAssessed=false when a health assessment was in fact
	// already confirmed. Previously, handleAlertDecaySuspected reset
	// HealthAssessed=false on every decay pass to force a live re-probe on
	// the *next* pass; if the validity deadline was reached before that
	// next pass ran, handleExpired short-circuited and completed the EA
	// with the stale false flag despite a valid last-known HealthScore.
	//
	// This regression test reproduces the exact two-pass race:
	//   Pass 1: decay detected -> HealthAssessed destructively reset to
	//           false (pre-fix behavior), AlertDecayRetries incremented.
	//   (time passes, ValidityDeadline is reached before the next
	//    reconcile can run and re-probe health)
	//   Pass 2: handleExpired short-circuits on the expired deadline and
	//           completes the EA using the stale HealthAssessed=false.
	// ========================================
	It("UT-EM-DECAY-012: should preserve HealthAssessed=true when validity expires mid-decay-monitoring", func() {
		s := buildScheme()
		ns := testNs
		name := "ea-decay-012"

		ea := seedDecayEA(name)
		r, fc := makeReconcilerWithAM(s, firingAMClient(), nil, ea)

		// Pass 1: decay detected on a healthy, still-firing-alert EA.
		_, err := r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(1)),
			"sanity: decay should be detected on pass 1")

		// Simulate the deadline being reached before the next reconcile
		// (e.g. controller-runtime workqueue delay, node pressure, etc.)
		// by moving ValidityDeadline into the past on the persisted object.
		pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Second))
		fetchedEA.Status.ValidityDeadline = &pastDeadline
		Expect(fc.Status().Update(context.Background(), fetchedEA)).To(Succeed())

		// Pass 2: validity has expired; the EA must complete now.
		_, err = r.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(fc.Get(context.Background(), types.NamespacedName{Name: name, Namespace: ns}, fetchedEA)).To(Succeed())

		Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted),
			"EA should be Completed (validity expired)")
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonAlertDecayTimeout),
			"Reason should be 'alert_decay_timeout' (decay was actively monitored)")
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue(),
			"HealthAssessed must remain true — a valid health assessment was already confirmed on pass 1 and "+
				"must not be reported as incomplete just because validity expired before the next re-probe (Issue #1701)")
		Expect(fetchedEA.Status.Components.HealthScore).ToNot(BeNil(),
			"HealthScore must be populated for downstream consumers")
	})
})
