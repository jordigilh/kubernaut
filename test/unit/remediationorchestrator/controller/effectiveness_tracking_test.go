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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
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
// EFFECTIVENESS TRACKING UNIT TESTS (ADR-EM-001, GAP-RO-2)
// Business Requirement: RO tracks EA lifecycle and sets EffectivenessAssessed condition on RR
//
// All tests drive the reconciler through the public Reconcile() method.
// Terminal RRs with EA refs trigger the terminal-phase housekeeping block
// which calls trackEffectivenessStatus internally.
// ============================================================================
var _ = Describe("Effectiveness Assessment Tracking (ADR-EM-001, GAP-RO-2)", func() {

	var (
		ctx    context.Context
		scheme = setupScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// reconcileAndVerify builds a reconciler from the given fake client, calls Reconcile,
	// and returns the reconcile result plus the shared client for post-reconcile verification.
	reconcileAndVerify := func(fakeClient client.WithWatch, rrName, namespace string) (ctrl.Result, error) {
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

	// ========================================
	// UT-RO-ET-001: Completed EA sets EffectivenessAssessed=True
	// ========================================
	It("UT-RO-ET-001: should set EffectivenessAssessed=True when EA completes", func() {
		rrName := "rr-et-001"
		namespace := "test-ns"
		eaName := "ea-" + rrName

		// Create terminal RR with EA ref
		rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseCompleted)
		rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
			Kind:       "EffectivenessAssessment",
			Name:       eaName,
			Namespace:  namespace,
			APIVersion: eav1.GroupVersion.String(),
		}

		// Create completed EA
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      eaName,
				Namespace: namespace,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseCompleted,
				AssessmentReason: eav1.AssessmentReasonFull,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr).
			Build()

		_, err := reconcileAndVerify(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		// Refetch RR from the same client to verify persisted condition
		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())

		cond := meta.FindStatusCondition(fetchedRR.Status.Conditions, "EffectivenessAssessed")
		Expect(cond).ToNot(BeNil(), "EffectivenessAssessed condition should be set")
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal("AssessmentCompleted"))
		Expect(cond.Message).To(ContainSubstring("full"))
	})

	// ========================================
	// UT-RO-ET-002: Failed EA sets EffectivenessAssessed=False
	// ========================================
	It("UT-RO-ET-002: should set EffectivenessAssessed=False when EA fails", func() {
		rrName := "rr-et-002"
		namespace := "test-ns"
		eaName := "ea-" + rrName

		rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseCompleted)
		rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
			Kind:       "EffectivenessAssessment",
			Name:       eaName,
			Namespace:  namespace,
			APIVersion: eav1.GroupVersion.String(),
		}

		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      eaName,
				Namespace: namespace,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseFailed,
				AssessmentReason: "target_not_found",
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr).
			Build()

		_, err := reconcileAndVerify(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())

		cond := meta.FindStatusCondition(fetchedRR.Status.Conditions, "EffectivenessAssessed")
		Expect(cond).ToNot(BeNil(), "EffectivenessAssessed condition should be set")
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal("AssessmentFailed"))
		Expect(cond.Message).To(ContainSubstring("target_not_found"))
	})

	// ========================================
	// UT-RO-ET-003: Pending EA does not set condition
	// ========================================
	It("UT-RO-ET-003: should not set condition when EA is still Pending", func() {
		rrName := "rr-et-003"
		namespace := "test-ns"
		eaName := "ea-" + rrName

		rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseCompleted)
		rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
			Kind:       "EffectivenessAssessment",
			Name:       eaName,
			Namespace:  namespace,
			APIVersion: eav1.GroupVersion.String(),
		}

		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      eaName,
				Namespace: namespace,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase: eav1.PhasePending,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr).
			Build()

		_, err := reconcileAndVerify(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())

		cond := meta.FindStatusCondition(fetchedRR.Status.Conditions, "EffectivenessAssessed")
		Expect(cond).To(BeNil(), "EffectivenessAssessed condition should NOT be set for Pending EA")
	})

	// ========================================
	// UT-RO-ET-004: No EA ref means no tracking
	// ========================================
	It("UT-RO-ET-004: should not set condition when no EA ref exists", func() {
		rrName := "rr-et-004"
		namespace := "test-ns"

		rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseCompleted)
		// No EffectivenessAssessmentRef set

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(rr).
			Build()

		_, err := reconcileAndVerify(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())

		cond := meta.FindStatusCondition(fetchedRR.Status.Conditions, "EffectivenessAssessed")
		Expect(cond).To(BeNil(), "EffectivenessAssessed condition should NOT be set when no EA ref")
	})

	// ========================================
	// UT-RO-ET-005: Idempotent - condition already set means no re-write
	// ========================================
	It("UT-RO-ET-005: should be idempotent when condition already set", func() {
		rrName := "rr-et-005"
		namespace := "test-ns"
		eaName := "ea-" + rrName

		rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseCompleted)
		rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
			Kind:       "EffectivenessAssessment",
			Name:       eaName,
			Namespace:  namespace,
			APIVersion: eav1.GroupVersion.String(),
		}
		// Pre-set the condition
		rr.Status.Conditions = append(rr.Status.Conditions, metav1.Condition{
			Type:               "EffectivenessAssessed",
			Status:             metav1.ConditionTrue,
			Reason:             "AssessmentCompleted",
			Message:            "Already set",
			LastTransitionTime: metav1.Now(),
		})

		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      eaName,
				Namespace: namespace,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseCompleted,
				AssessmentReason: eav1.AssessmentReasonFull,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr).
			Build()

		_, err := reconcileAndVerify(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		// Condition message should remain "Already set" (not overwritten)
		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())

		cond := meta.FindStatusCondition(fetchedRR.Status.Conditions, "EffectivenessAssessed")
		Expect(cond).ToNot(BeNil())
		Expect(cond.Message).To(Equal("Already set"), "Condition should not be overwritten")
	})

	// ========================================
	// UT-RO-ET-006: Expired EA sets EffectivenessAssessed=True with Reason=AssessmentExpired
	// ADR-EM-001 lines 898-906: Expired EAs use a distinct Reason to differentiate
	// from normal completions.
	// ========================================
	It("UT-RO-ET-006: should set Reason=AssessmentExpired when EA completes with reason=expired", func() {
		rrName := "rr-et-006"
		namespace := "test-ns"
		eaName := "ea-" + rrName

		rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseCompleted)
		rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
			Kind:       "EffectivenessAssessment",
			Name:       eaName,
			Namespace:  namespace,
			APIVersion: eav1.GroupVersion.String(),
		}

		// EA completed with reason=expired (validity window exceeded)
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      eaName,
				Namespace: namespace,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseCompleted,
				AssessmentReason: eav1.AssessmentReasonExpired,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr).
			Build()

		_, err := reconcileAndVerify(fakeClient, rrName, namespace)
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())

		cond := meta.FindStatusCondition(fetchedRR.Status.Conditions, "EffectivenessAssessed")
		Expect(cond).ToNot(BeNil(), "EffectivenessAssessed condition should be set")
		Expect(cond.Status).To(Equal(metav1.ConditionTrue),
			"Expired is still a completed assessment (Status=True)")
		Expect(cond.Reason).To(Equal("AssessmentExpired"),
			"ADR-EM-001: expired EAs must use Reason=AssessmentExpired, not AssessmentCompleted")
		Expect(cond.Message).To(ContainSubstring("expired"))
	})
})
