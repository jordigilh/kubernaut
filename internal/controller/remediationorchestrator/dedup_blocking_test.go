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

package controller_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	oomKilled = "OOM killed"
)

// ========================================
// Issue #614: RO-level DuplicateInProgress Outcome Inheritance
// ========================================

// newBlockedDuplicateRR creates a Blocked RR with BlockReason=DuplicateInProgress
// referencing the given original RR name.
func newBlockedDuplicateRR(name, namespace, duplicateOf string) *remediationv1.RemediationRequest {
	rr := newRemediationRequest(name, namespace, remediationv1.PhaseBlocked)
	rr.Status.BlockReason = remediationv1.BlockReasonDuplicateInProgress
	rr.Status.BlockMessage = "Another remediation is in progress for this fingerprint"
	rr.Status.DuplicateOf = duplicateOf
	rr.Status.StartTime = &metav1.Time{Time: time.Now().Add(-30 * time.Second)}
	rr.Status.ObservedGeneration = rr.Generation
	return rr
}

var _ = Describe("Issue #614: RO-level DuplicateInProgress Outcome Inheritance", func() {
	var (
		ctx    context.Context
		scheme = setupScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("Core inheritance from original RemediationRequest", func() {

		It("UT-RO-614-001: original RR Completed → duplicate inherits Completed with InheritedCompleted outcome", func() {
			originalRR := newRemediationRequest("original-rr-001", defaultFixture, remediationv1.PhaseCompleted)
			originalRR.Status.Outcome = remediationv1.OutcomeRemediated

			dupRR := newBlockedDuplicateRR("dup-rr-001", defaultFixture, "original-rr-001")

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(originalRR, dupRR).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
				Client:        fakeClient,
				APIReader:     fakeClient,
				Scheme:        scheme,
				AuditStore:    nil,
				Recorder:      record.NewFakeRecorder(20),
				Metrics:       rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				Timeouts:      prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				RoutingEngine: &MockRoutingEngine{},
			})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "dup-rr-001", Namespace: defaultFixture},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "dup-rr-001", Namespace: defaultFixture}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted),
				"Behavior: Blocked/DuplicateInProgress RR must inherit Completed when original RR completes")
			Expect(updated.Status.Outcome).To(Equal(remediationv1.OutcomeRemediated),
				"Behavior: Outcome must be Remediated (lineage tracked via DuplicateOf + K8s events)")
			Expect(updated.Status.CompletedAt).NotTo(BeNil(),
				"Behavior: CompletedAt must be set for terminal transition")
			Expect(updated.Status.BlockReason).To(BeEmpty(),
				"Behavior: BlockReason must be cleared after RR-level inheritance (#614 F2)")
			Expect(updated.Status.BlockMessage).To(BeEmpty(),
				"Behavior: BlockMessage must be cleared after RR-level inheritance (#614 F2)")
			Expect(updated.Status.DuplicateOf).To(BeEmpty(),
				"Behavior: DuplicateOf must be cleared after RR-level inheritance (#614 F2)")
		})

		It("UT-RO-614-002: original RR Failed → duplicate inherits Failed with FailurePhaseDeduplicated", func() {
			failPhase := remediationv1.FailurePhaseWorkflowExecution
			failReason := oomKilled
			originalRR := newRemediationRequest("original-rr-002", defaultFixture, remediationv1.PhaseFailed)
			originalRR.Status.FailurePhase = &failPhase
			originalRR.Status.FailureReason = &failReason

			dupRR := newBlockedDuplicateRR("dup-rr-002", defaultFixture, "original-rr-002")

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(originalRR, dupRR).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
				Client:        fakeClient,
				APIReader:     fakeClient,
				Scheme:        scheme,
				AuditStore:    nil,
				Recorder:      record.NewFakeRecorder(20),
				Metrics:       rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				Timeouts:      prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				RoutingEngine: &MockRoutingEngine{},
			})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "dup-rr-002", Namespace: defaultFixture},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "dup-rr-002", Namespace: defaultFixture}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed),
				"Behavior: Blocked/DuplicateInProgress RR must inherit Failed when original RR fails")
			Expect(updated.Status.FailurePhase).NotTo(BeNil(),
				"Behavior: FailurePhase must be set for inherited failures")
			Expect(*updated.Status.FailurePhase).To(Equal(remediationv1.FailurePhaseDeduplicated),
				"Behavior: FailurePhase must be Deduplicated (not the original's failure phase)")
			Expect(updated.Status.FailureReason).NotTo(BeNil(),
				"Behavior: FailureReason must contain reference to original RR")
			Expect(*updated.Status.FailureReason).To(ContainSubstring("original-rr-002"),
				"Behavior: FailureReason must mention original RR name for traceability")
			Expect(updated.Status.BlockReason).To(BeEmpty(),
				"Behavior: BlockReason must be cleared after RR-level inheritance (#614 F2)")
			Expect(updated.Status.BlockMessage).To(BeEmpty(),
				"Behavior: BlockMessage must be cleared after RR-level inheritance (#614 F2)")
			Expect(updated.Status.DuplicateOf).To(BeEmpty(),
				"Behavior: DuplicateOf must be cleared after RR-level inheritance (#614 F2)")
		})
	})

	Context("Edge cases", func() {

		It("UT-RO-614-003: deleted original RR → duplicate inherits Failed (dangling reference)", func() {
			dupRR := newBlockedDuplicateRR("dup-rr-003", defaultFixture, "deleted-original-003")

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(dupRR).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
				Client:        fakeClient,
				APIReader:     fakeClient,
				Scheme:        scheme,
				AuditStore:    nil,
				Recorder:      record.NewFakeRecorder(20),
				Metrics:       rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				Timeouts:      prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				RoutingEngine: &MockRoutingEngine{},
			})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "dup-rr-003", Namespace: defaultFixture},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "dup-rr-003", Namespace: defaultFixture}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed),
				"Behavior: deleted original must cause inherited failure, not silent clearing")
			Expect(updated.Status.FailurePhase).NotTo(BeNil())
			Expect(*updated.Status.FailurePhase).To(Equal(remediationv1.FailurePhaseDeduplicated),
				"Behavior: FailurePhase must be Deduplicated for deleted original")
			Expect(updated.Status.BlockReason).To(BeEmpty(),
				"Behavior: BlockReason must be cleared after RR-level inheritance (#614 F2)")
			Expect(updated.Status.BlockMessage).To(BeEmpty(),
				"Behavior: BlockMessage must be cleared after RR-level inheritance (#614 F2)")
			Expect(updated.Status.DuplicateOf).To(BeEmpty(),
				"Behavior: DuplicateOf must be cleared after RR-level inheritance (#614 F2)")
			Expect(updated.Status.FailureReason).NotTo(BeNil())
			Expect(*updated.Status.FailureReason).To(ContainSubstring("deleted-original-003"),
				"Behavior: FailureReason must reference the deleted original RR")
		})

		It("UT-RO-614-004: transient Get error on original RR → reconcile error (retryable)", func() {
			dupRR := newBlockedDuplicateRR("dup-rr-004", defaultFixture, "transient-original-004")

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(dupRR).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if key.Name == "transient-original-004" {
							return fmt.Errorf("simulated transient API server error")
						}
						return c.Get(ctx, key, obj, opts...)
					},
				}).
				Build()

			reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
				Client:        fakeClient,
				APIReader:     fakeClient,
				Scheme:        scheme,
				AuditStore:    nil,
				Recorder:      record.NewFakeRecorder(20),
				Metrics:       rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				Timeouts:      prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				RoutingEngine: &MockRoutingEngine{},
			})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "dup-rr-004", Namespace: defaultFixture},
			})
			Expect(err).NotTo(HaveOccurred(),
				"Behavior: transient errors are handled with RequeueAfter, not propagated as errors")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"Behavior: must requeue after transient error")

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "dup-rr-004", Namespace: defaultFixture}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked),
				"Behavior: RR must remain Blocked on transient error")
		})
	})

	Context("Observability: gauge decrement", func() {

		It("UT-RO-614-005: CurrentBlockedGauge decrements after successful Completed inheritance", func() {
			originalRR := newRemediationRequest("original-rr-005", defaultFixture, remediationv1.PhaseCompleted)
			originalRR.Status.Outcome = remediationv1.OutcomeRemediated

			dupRR := newBlockedDuplicateRR("dup-rr-005", defaultFixture, "original-rr-005")

			m := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			m.CurrentBlockedGauge.WithLabelValues(defaultFixture).Set(1)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(originalRR, dupRR).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
				Client:        fakeClient,
				APIReader:     fakeClient,
				Scheme:        scheme,
				AuditStore:    nil,
				Recorder:      record.NewFakeRecorder(20),
				Metrics:       m,
				Timeouts:      prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				RoutingEngine: &MockRoutingEngine{},
			})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "dup-rr-005", Namespace: defaultFixture},
			})
			Expect(err).NotTo(HaveOccurred())

			gaugeValue := testutil.ToFloat64(m.CurrentBlockedGauge.WithLabelValues(defaultFixture))
			Expect(gaugeValue).To(Equal(float64(0)),
				"Behavior: CurrentBlockedGauge must decrement from 1 to 0 after successful inheritance")
		})

		It("UT-RO-614-006: CurrentBlockedGauge decrements after successful Failed inheritance", func() {
			failPhase := remediationv1.FailurePhaseWorkflowExecution
			failReason := oomKilled
			originalRR := newRemediationRequest("original-rr-006", defaultFixture, remediationv1.PhaseFailed)
			originalRR.Status.FailurePhase = &failPhase
			originalRR.Status.FailureReason = &failReason

			dupRR := newBlockedDuplicateRR("dup-rr-006", defaultFixture, "original-rr-006")

			m := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			m.CurrentBlockedGauge.WithLabelValues(defaultFixture).Set(1)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(originalRR, dupRR).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
				Client:        fakeClient,
				APIReader:     fakeClient,
				Scheme:        scheme,
				AuditStore:    nil,
				Recorder:      record.NewFakeRecorder(20),
				Metrics:       m,
				Timeouts:      prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				RoutingEngine: &MockRoutingEngine{},
			})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "dup-rr-006", Namespace: defaultFixture},
			})
			Expect(err).NotTo(HaveOccurred())

			gaugeValue := testutil.ToFloat64(m.CurrentBlockedGauge.WithLabelValues(defaultFixture))
			Expect(gaugeValue).To(Equal(float64(0)),
				"Behavior: CurrentBlockedGauge must decrement from 1 to 0 after Failed inheritance")
		})
	})

	Context("Observability: K8s events", func() {

		It("UT-RO-614-007: Completed inheritance emits K8s event with RemediationRequest provenance", func() {
			originalRR := newRemediationRequest("original-rr-007", defaultFixture, remediationv1.PhaseCompleted)
			originalRR.Status.Outcome = remediationv1.OutcomeRemediated

			dupRR := newBlockedDuplicateRR("dup-rr-007", defaultFixture, "original-rr-007")

			fakeRecorder := record.NewFakeRecorder(20)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(originalRR, dupRR).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
				Client:        fakeClient,
				APIReader:     fakeClient,
				Scheme:        scheme,
				AuditStore:    nil,
				Recorder:      fakeRecorder,
				Metrics:       rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				Timeouts:      prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				RoutingEngine: &MockRoutingEngine{},
			})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "dup-rr-007", Namespace: defaultFixture},
			})
			Expect(err).NotTo(HaveOccurred())

			var foundEvent bool
			for len(fakeRecorder.Events) > 0 {
				event := <-fakeRecorder.Events
				if strings.Contains(event, "InheritedCompleted") && strings.Contains(event, "RemediationRequest") && strings.Contains(event, "original-rr-007") {
					foundEvent = true
					break
				}
			}
			Expect(foundEvent).To(BeTrue(),
				"Behavior: Completed inheritance must emit K8s event with sourceKind=RemediationRequest and original RR name")
		})

		It("UT-RO-614-008: Failed inheritance emits K8s event with RemediationRequest provenance", func() {
			failPhase := remediationv1.FailurePhaseWorkflowExecution
			failReason := oomKilled
			originalRR := newRemediationRequest("original-rr-008", defaultFixture, remediationv1.PhaseFailed)
			originalRR.Status.FailurePhase = &failPhase
			originalRR.Status.FailureReason = &failReason

			dupRR := newBlockedDuplicateRR("dup-rr-008", defaultFixture, "original-rr-008")

			fakeRecorder := record.NewFakeRecorder(20)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(originalRR, dupRR).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
				Client:        fakeClient,
				APIReader:     fakeClient,
				Scheme:        scheme,
				AuditStore:    nil,
				Recorder:      fakeRecorder,
				Metrics:       rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				Timeouts:      prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				RoutingEngine: &MockRoutingEngine{},
			})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "dup-rr-008", Namespace: defaultFixture},
			})
			Expect(err).NotTo(HaveOccurred())

			var foundEvent bool
			for len(fakeRecorder.Events) > 0 {
				event := <-fakeRecorder.Events
				if strings.Contains(event, "InheritedFailed") && strings.Contains(event, "RemediationRequest") && strings.Contains(event, "original-rr-008") {
					foundEvent = true
					break
				}
			}
			Expect(foundEvent).To(BeTrue(),
				"Behavior: Failed inheritance must emit K8s event with sourceKind=RemediationRequest and original RR name")
		})
	})

	Context("Regression guards", func() {

		It("UT-RO-614-009: active original RR → duplicate stays Blocked, requeues", func() {
			originalRR := newRemediationRequest("original-rr-009", defaultFixture, remediationv1.PhaseExecuting)
			originalRR.Status.StartTime = &metav1.Time{Time: time.Now()}

			dupRR := newBlockedDuplicateRR("dup-rr-009", defaultFixture, "original-rr-009")

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(originalRR, dupRR).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
				Client:        fakeClient,
				APIReader:     fakeClient,
				Scheme:        scheme,
				AuditStore:    nil,
				Recorder:      record.NewFakeRecorder(20),
				Metrics:       rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				Timeouts:      prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				RoutingEngine: &MockRoutingEngine{},
			})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "dup-rr-009", Namespace: defaultFixture},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"Behavior: must requeue to poll original RR status")

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "dup-rr-009", Namespace: defaultFixture}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked),
				"Regression: duplicate must stay Blocked while original is still active")
		})

		It("UT-RO-614-010: empty DuplicateOf → clears to Pending (pre-#614 behavior preserved)", func() {
			dupRR := newRemediationRequest("dup-rr-010", defaultFixture, remediationv1.PhaseBlocked)
			dupRR.Status.BlockReason = remediationv1.BlockReasonDuplicateInProgress
			dupRR.Status.DuplicateOf = ""
			dupRR.Status.ObservedGeneration = dupRR.Generation

			m := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			m.CurrentBlockedGauge.WithLabelValues(defaultFixture).Set(1)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(dupRR).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
				Client:        fakeClient,
				APIReader:     fakeClient,
				Scheme:        scheme,
				AuditStore:    nil,
				Recorder:      record.NewFakeRecorder(20),
				Metrics:       m,
				Timeouts:      prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				RoutingEngine: &MockRoutingEngine{},
			})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "dup-rr-010", Namespace: defaultFixture},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "dup-rr-010", Namespace: defaultFixture}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhasePending),
				"Regression: empty DuplicateOf must still clear to Pending (not inherit)")

			gaugeValue := testutil.ToFloat64(m.CurrentBlockedGauge.WithLabelValues(defaultFixture))
			Expect(gaugeValue).To(Equal(float64(0)),
				"Regression: gauge must decrement when clearing event-based block")
		})

		It("UT-RO-614-011: inherited failure does NOT increment ConsecutiveFailureCount", func() {
			failPhase := remediationv1.FailurePhaseWorkflowExecution
			failReason := oomKilled
			originalRR := newRemediationRequest("original-rr-011", defaultFixture, remediationv1.PhaseFailed)
			originalRR.Status.FailurePhase = &failPhase
			originalRR.Status.FailureReason = &failReason

			dupRR := newBlockedDuplicateRR("dup-rr-011", defaultFixture, "original-rr-011")

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(originalRR, dupRR).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
				Client:        fakeClient,
				APIReader:     fakeClient,
				Scheme:        scheme,
				AuditStore:    nil,
				Recorder:      record.NewFakeRecorder(20),
				Metrics:       rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				Timeouts:      prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				RoutingEngine: &MockRoutingEngine{},
			})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "dup-rr-011", Namespace: defaultFixture},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "dup-rr-011", Namespace: defaultFixture}, updated)).To(Succeed())

			Expect(updated.Status.ConsecutiveFailureCount).To(Equal(int32(0)),
				"Behavior: inherited failures from RR-level dedup must NOT increment ConsecutiveFailureCount")
		})

		It("UT-RO-614-012: original RR TimedOut → duplicate inherits Failed (non-Completed terminal phases map to Failed)", func() {
			originalRR := newRemediationRequest("original-rr-012", defaultFixture, remediationv1.PhaseTimedOut)

			dupRR := newBlockedDuplicateRR("dup-rr-012", defaultFixture, "original-rr-012")

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(originalRR, dupRR).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
				Client:        fakeClient,
				APIReader:     fakeClient,
				Scheme:        scheme,
				AuditStore:    nil,
				Recorder:      record.NewFakeRecorder(20),
				Metrics:       rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				Timeouts:      prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				RoutingEngine: &MockRoutingEngine{},
			})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "dup-rr-012", Namespace: defaultFixture},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "dup-rr-012", Namespace: defaultFixture}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed),
				"Behavior: non-Completed terminal phases (TimedOut, Cancelled, etc.) must map to inherited Failed")
			Expect(updated.Status.FailurePhase).NotTo(BeNil())
			Expect(*updated.Status.FailurePhase).To(Equal(remediationv1.FailurePhaseDeduplicated),
				"Behavior: FailurePhase must be Deduplicated regardless of original's terminal phase")
			Expect(updated.Status.FailureReason).NotTo(BeNil())
			Expect(*updated.Status.FailureReason).To(ContainSubstring("TimedOut"),
				"Behavior: FailureReason must include original's terminal phase for traceability")
		})
	})

	Context("F-3: Notification guard for RR-level inheritance", func() {

		It("UT-RO-614-F3: ensureNotificationsCreated must NOT be called for RR-level inheritance (no AIAnalysis exists)", func() {
			originalRR := newRemediationRequest("original-rr-f3", defaultFixture, remediationv1.PhaseCompleted)
			originalRR.Status.Outcome = remediationv1.OutcomeRemediated

			dupRR := newBlockedDuplicateRR("dup-rr-f3", defaultFixture, "original-rr-f3")

			fakeRecorder := record.NewFakeRecorder(20)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(originalRR, dupRR).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
				Client:        fakeClient,
				APIReader:     fakeClient,
				Scheme:        scheme,
				AuditStore:    nil,
				Recorder:      fakeRecorder,
				Metrics:       rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				Timeouts:      prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				RoutingEngine: &MockRoutingEngine{},
			})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "dup-rr-f3", Namespace: defaultFixture},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "dup-rr-f3", Namespace: defaultFixture}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted),
				"Behavior: inheritance must succeed without notification creation")
			Expect(updated.Status.NotificationRequestRefs).To(BeEmpty(),
				"Behavior: no notification refs should be created for RR-level inheritance (F-3 guard)")
		})
	})
})
