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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// ============================================================================
// INCONCLUSIVE BACKOFF UNIT TESTS (BR-ORCH-042.6, Issue #1091)
//
// Business Requirement: When EA determines Outcome=Inconclusive (alert still
// firing, alertScore=0), RO sets exponential backoff (ConsecutiveFailureCount,
// NextAllowedExecution) on the RR to prevent GW from creating new RRs
// immediately.
//
// All tests drive the reconciler through the public Reconcile() method.
// Verifying RRs with completed EAs trigger completeVerificationIfNeeded,
// which applies backoff for Inconclusive outcomes.
// ============================================================================
var _ = Describe("Inconclusive Backoff (BR-ORCH-042.6, Issue #1091)", func() {

	var (
		ctx    context.Context
		scheme = setupScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	reconcileVerifying := func(fakeClient client.WithWatch, rrName, namespace string) (ctrl.Result, error) {
		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		recorder := record.NewFakeRecorder(20)
		reconciler := controller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder, roMetrics,
			controller.TimeoutConfig{},
			&MockRoutingEngine{},
		)
		return reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
	}

	newVerifyingRRWithEA := func(name, namespace, eaName string) *remediationv1.RemediationRequest {
		rr := newRemediationRequest(name, namespace, remediationv1.PhaseVerifying)
		startTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))
		rr.Status.StartTime = &startTime
		rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
			Kind:       "EffectivenessAssessment",
			Name:       eaName,
			Namespace:  namespace,
			APIVersion: eav1.GroupVersion.String(),
		}
		// VerificationDeadline must be set and in the future so the Verifying handler
		// proceeds past the deadline checks to TrackEffectivenessStatus.
		futureDeadline := metav1.NewTime(time.Now().Add(1 * time.Hour))
		rr.Status.VerificationDeadline = &futureDeadline
		return rr
	}

	newInconclusiveEA := func(name, namespace string) *eav1.EffectivenessAssessment {
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseCompleted,
				AssessmentReason: eav1.AssessmentReasonFull,
				Components: eav1.EAComponents{
					AlertAssessed: true,
					AlertScore:    float64Ptr(0.0),
				},
			},
		}
	}

	newRemediatedEA := func(name, namespace string) *eav1.EffectivenessAssessment {
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseCompleted,
				AssessmentReason: eav1.AssessmentReasonFull,
				Components: eav1.EAComponents{
					AlertAssessed: true,
					AlertScore:    float64Ptr(1.0),
				},
			},
		}
	}

	// UT-RO-1091-001: Inconclusive sets ConsecutiveFailureCount and NextAllowedExecution
	It("UT-RO-1091-001: Inconclusive EA sets ConsecutiveFailureCount=1 and NextAllowedExecution in future", func() {
		rrName := "rr-1091-001"
		namespace := "test-ns"
		eaName := "ea-" + rrName

		rr := newVerifyingRRWithEA(rrName, namespace, eaName)
		ea := newInconclusiveEA(eaName, namespace)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr).
			Build()

		_, err := reconcileVerifying(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())

		Expect(fetchedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted),
			"Inconclusive should still transition to Completed")
		Expect(fetchedRR.Status.Outcome).To(Equal("Inconclusive"))
		Expect(fetchedRR.Status.ConsecutiveFailureCount).To(Equal(int32(1)),
			"Inconclusive should increment ConsecutiveFailureCount")
		Expect(fetchedRR.Status.NextAllowedExecution).ToNot(BeNil(),
			"Inconclusive should set NextAllowedExecution")
		Expect(fetchedRR.Status.NextAllowedExecution.Time.After(time.Now())).To(BeTrue(),
			"NextAllowedExecution should be in the future")
	})

	// UT-RO-1091-002: Pre-existing ConsecutiveFailureCount is preserved and incremented
	It("UT-RO-1091-002: Inconclusive with pre-existing count=1 increments to 2 with longer backoff", func() {
		rrName := "rr-1091-002"
		namespace := "test-ns"
		eaName := "ea-" + rrName

		rr := newVerifyingRRWithEA(rrName, namespace, eaName)
		rr.Status.ConsecutiveFailureCount = 1
		ea := newInconclusiveEA(eaName, namespace)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr).
			Build()

		_, err := reconcileVerifying(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())

		Expect(fetchedRR.Status.ConsecutiveFailureCount).To(Equal(int32(2)),
			"Should increment from 1 to 2")
		Expect(fetchedRR.Status.NextAllowedExecution).ToNot(BeNil(),
			"Should set NextAllowedExecution for count=2")
	})

	// UT-RO-1091-003: Inconclusive when EA phase is Failed (not Completed) still applies backoff
	It("UT-RO-1091-003: Inconclusive from Failed EA still transitions with backoff", func() {
		rrName := "rr-1091-003"
		namespace := "test-ns"
		eaName := "ea-" + rrName

		rr := newVerifyingRRWithEA(rrName, namespace, eaName)
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      eaName,
				Namespace: namespace,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseFailed,
				AssessmentReason: "target_not_found",
				Components: eav1.EAComponents{
					AlertAssessed: true,
					AlertScore:    float64Ptr(0.0),
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr).
			Build()

		_, err := reconcileVerifying(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())

		Expect(fetchedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted))
		Expect(fetchedRR.Status.Outcome).To(Equal("Inconclusive"))
		Expect(fetchedRR.Status.ConsecutiveFailureCount).To(Equal(int32(1)))
		Expect(fetchedRR.Status.NextAllowedExecution).To(
			Satisfy(func(t *metav1.Time) bool { return t != nil && t.After(time.Now()) }),
			"Inconclusive should set NextAllowedExecution to a future time")
	})

	// UT-RO-1091-004: At threshold, count increments but NextAllowedExecution is NOT set
	It("UT-RO-1091-004: Inconclusive at threshold increments count but skips NextAllowedExecution", func() {
		rrName := "rr-1091-004"
		namespace := "test-ns"
		eaName := "ea-" + rrName

		rr := newVerifyingRRWithEA(rrName, namespace, eaName)
		rr.Status.ConsecutiveFailureCount = 3 // At threshold (MockRoutingEngine threshold=3)
		ea := newInconclusiveEA(eaName, namespace)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr).
			Build()

		_, err := reconcileVerifying(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())

		Expect(fetchedRR.Status.ConsecutiveFailureCount).To(Equal(int32(4)),
			"Count should still increment even at threshold (TQ-7)")
		Expect(fetchedRR.Status.NextAllowedExecution).To(BeNil(),
			"At threshold, NextAllowedExecution should NOT be set (routing engine blocks instead)")
	})

	// UT-RO-1091-005: Remediated outcome does NOT set backoff (negative/regression test)
	It("UT-RO-1091-005: Remediated EA does NOT set backoff fields", func() {
		rrName := "rr-1091-005"
		namespace := "test-ns"
		eaName := "ea-" + rrName

		rr := newVerifyingRRWithEA(rrName, namespace, eaName)
		ea := newRemediatedEA(eaName, namespace)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr).
			Build()

		_, err := reconcileVerifying(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())

		Expect(fetchedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted))
		Expect(fetchedRR.Status.Outcome).To(Equal("Remediated"))
		Expect(fetchedRR.Status.ConsecutiveFailureCount).To(Equal(int32(0)),
			"Remediated should NOT increment ConsecutiveFailureCount")
		Expect(fetchedRR.Status.NextAllowedExecution).To(BeNil(),
			"Remediated should NOT set NextAllowedExecution")
	})

	// UT-RO-1091-006: Idempotency - second reconcile does not re-apply backoff
	It("UT-RO-1091-006: second reconcile after Inconclusive does not re-apply backoff", func() {
		rrName := "rr-1091-006"
		namespace := "test-ns"
		eaName := "ea-" + rrName

		rr := newVerifyingRRWithEA(rrName, namespace, eaName)
		ea := newInconclusiveEA(eaName, namespace)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr).
			Build()

		// First reconcile: transitions to Completed with Inconclusive + backoff
		_, err := reconcileVerifying(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())
		firstCount := fetchedRR.Status.ConsecutiveFailureCount

		// Second reconcile: should be idempotent (RR is now Completed, not Verifying)
		_, err = reconcileVerifying(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())
		Expect(fetchedRR.Status.ConsecutiveFailureCount).To(Equal(firstCount),
			"Second reconcile should NOT increment ConsecutiveFailureCount again")
	})
})
