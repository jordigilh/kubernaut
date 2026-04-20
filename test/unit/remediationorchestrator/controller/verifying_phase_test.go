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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// ============================================================================
// VERIFYING PHASE UNIT TESTS (#280)
// Business Requirement: BR-GATEWAY-185 (dedup), BR-EM-010 (EA gates completion)
//
// After WFE completes successfully, RR transitions to Verifying (not Completed).
// RO creates the EA, and the RR stays non-terminal until EA finishes or times out.
// ============================================================================
var _ = Describe("Verifying Phase Transition (#280)", func() {

	var (
		ctx                 context.Context
		scheme              = setupScheme()
		stabilizationWindow = 30 * time.Second
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// UT-VERIFY-001: WFE completion triggers Verifying (not Completed)
	// ========================================
	It("UT-VERIFY-001: should transition to Verifying (not Completed) when WFE completes", func() {
		rrName := "rr-verify-001"
		namespace := "test-ns"
		weName := "we-" + rrName

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseExecuting, "", "", weName)
		we := newWorkflowExecutionCompleted(weName, namespace, rrName)

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, we).
			WithStatusSubresource(rr).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		recorder := record.NewFakeRecorder(20)
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, stabilizationWindow)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil, recorder, roMetrics,
			controller.TimeoutConfig{},
			&MockRoutingEngine{},
			eaCreator,
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)
		Expect(err).ToNot(HaveOccurred())

		Expect(fetchedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseVerifying),
			"#280: WFE completion should transition to Verifying, not Completed")
		Expect(fetchedRR.Status.CompletedAt).To(BeNil(),
			"CompletedAt should NOT be set during Verifying")
	})

	// ========================================
	// UT-VERIFY-002: EA is created during Verifying transition
	// ========================================
	It("UT-VERIFY-002: should create EA when transitioning to Verifying", func() {
		rrName := "rr-verify-002"
		namespace := "test-ns"
		weName := "we-" + rrName

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseExecuting, "", "", weName)
		we := newWorkflowExecutionCompleted(weName, namespace, rrName)

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, we).
			WithStatusSubresource(rr).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		recorder := record.NewFakeRecorder(20)
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, stabilizationWindow)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil, recorder, roMetrics,
			controller.TimeoutConfig{},
			&MockRoutingEngine{},
			eaCreator,
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		ea := &eav1.EffectivenessAssessment{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      "ea-" + rrName,
			Namespace: namespace,
		}, ea)
		Expect(err).ToNot(HaveOccurred(), "EA should be created during Verifying transition")

		fetchedRR := &remediationv1.RemediationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)
		Expect(err).ToNot(HaveOccurred())
		Expect(fetchedRR.Status.EffectivenessAssessmentRef).ToNot(BeNil(),
			"EffectivenessAssessmentRef should be set on RR after EA creation")
	})

	// ========================================
	// UT-VERIFY-003: EA completion triggers Verifying -> Completed
	// ========================================
	It("UT-VERIFY-003: should transition from Verifying to Completed when EA completes", func() {
		rrName := "rr-verify-003"
		namespace := "test-ns"

		deadline := metav1.NewTime(time.Now().Add(10 * time.Minute))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rrName,
				Namespace: namespace,
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseVerifying,
				EffectivenessAssessmentRef: &corev1.ObjectReference{
					Kind:      "EffectivenessAssessment",
					Name:      "ea-" + rrName,
					Namespace: namespace,
				},
				VerificationDeadline: &deadline,
			},
		}
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-" + rrName,
				Namespace: namespace,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseCompleted,
				AssessmentReason: eav1.AssessmentReasonFull,
				Message:          "Assessment completed: Full",
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr, ea).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		recorder := record.NewFakeRecorder(20)
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, stabilizationWindow)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil, recorder, roMetrics,
			controller.TimeoutConfig{},
			&MockRoutingEngine{},
			eaCreator,
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)
		Expect(err).ToNot(HaveOccurred())

		Expect(fetchedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted),
			"#280: EA terminal should trigger Verifying -> Completed")
		Expect(fetchedRR.Status.CompletedAt).ToNot(BeNil(),
			"CompletedAt should be set when transitioning to Completed")
		Expect(fetchedRR.Status.Outcome).To(Equal("Remediated"),
			"Outcome should be Remediated after successful verification")
	})

	// ========================================
	// UT-VERIFY-004: EA failure also triggers Verifying -> Completed
	// ========================================
	It("UT-VERIFY-004: should transition from Verifying to Completed when EA fails", func() {
		rrName := "rr-verify-004"
		namespace := "test-ns"

		deadline := metav1.NewTime(time.Now().Add(10 * time.Minute))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rrName,
				Namespace: namespace,
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseVerifying,
				EffectivenessAssessmentRef: &corev1.ObjectReference{
					Kind:      "EffectivenessAssessment",
					Name:      "ea-" + rrName,
					Namespace: namespace,
				},
				VerificationDeadline: &deadline,
			},
		}
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-" + rrName,
				Namespace: namespace,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseFailed,
				AssessmentReason: "TargetNotFound",
				Message:          "Assessment failed: target not found",
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr, ea).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		recorder := record.NewFakeRecorder(20)
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, stabilizationWindow)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil, recorder, roMetrics,
			controller.TimeoutConfig{},
			&MockRoutingEngine{},
			eaCreator,
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)
		Expect(err).ToNot(HaveOccurred())

		Expect(fetchedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted),
			"#280: EA failure should also trigger Verifying -> Completed")
		Expect(fetchedRR.Status.CompletedAt).To(HaveValue(Not(BeZero())),
			"CompletedAt should be set when transitioning to Completed")
		Expect(fetchedRR.Status.Outcome).To(Equal("Remediated"),
			"Outcome is Remediated because the remediation itself succeeded, even if EA failed")
	})

	// ========================================
	// UT-VERIFY-006: Verification timeout triggers Completed with VerificationTimedOut
	// ========================================
	It("UT-VERIFY-006: should transition to Completed with VerificationTimedOut when deadline expires", func() {
		rrName := "rr-verify-006"
		namespace := "test-ns"

		expiredDeadline := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rrName,
				Namespace: namespace,
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseVerifying,
				EffectivenessAssessmentRef: &corev1.ObjectReference{
					Kind:      "EffectivenessAssessment",
					Name:      "ea-" + rrName,
					Namespace: namespace,
				},
				VerificationDeadline: &expiredDeadline,
			},
		}
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-" + rrName,
				Namespace: namespace,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase: eav1.PhasePending,
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr, ea).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		recorder := record.NewFakeRecorder(20)
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, stabilizationWindow)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil, recorder, roMetrics,
			controller.TimeoutConfig{},
			&MockRoutingEngine{},
			eaCreator,
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)
		Expect(err).ToNot(HaveOccurred())

		Expect(fetchedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted),
			"#280: Expired VerificationDeadline should trigger Completed")
		Expect(fetchedRR.Status.Outcome).To(Equal("VerificationTimedOut"),
			"Outcome should be VerificationTimedOut when deadline expires")
		Expect(fetchedRR.Status.CompletedAt).To(HaveValue(Not(BeZero())),
			"CompletedAt should be set on verification timeout")
	})

	// ========================================
	// UT-VERIFY-TIMEOUT-004: Safety-net timeout when VerificationDeadline is never set
	// ========================================
	It("UT-VERIFY-TIMEOUT-004: should timeout when VerificationDeadline is nil and RR is older than configured verifying timeout", func() {
		rrName := "rr-verify-timeout-004"
		namespace := "test-ns"

		verifyingTimeout := 10 * time.Minute
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              rrName,
				Namespace:         namespace,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-15 * time.Minute)),
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseVerifying,
				EffectivenessAssessmentRef: &corev1.ObjectReference{
					Kind:      "EffectivenessAssessment",
					Name:      "ea-" + rrName,
					Namespace: namespace,
				},
			},
		}
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-" + rrName,
				Namespace: namespace,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase: eav1.PhasePending,
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr, ea).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		recorder := record.NewFakeRecorder(20)
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, stabilizationWindow)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil, recorder, roMetrics,
			controller.TimeoutConfig{Verifying: verifyingTimeout},
			&MockRoutingEngine{},
			eaCreator,
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)
		Expect(err).ToNot(HaveOccurred())

		Expect(fetchedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted),
			"#280: Safety-net should fire when VerificationDeadline is nil and RR age exceeds configured verifying timeout")
		Expect(fetchedRR.Status.Outcome).To(Equal("VerificationTimedOut"),
			"Outcome should be VerificationTimedOut for safety-net timeout")
		Expect(fetchedRR.Status.CompletedAt).To(HaveValue(Not(BeZero())),
			"CompletedAt should be set on safety-net timeout")
	})

	// ========================================
	// UT-VERIFY-005: Verifying phase populates VerificationDeadline from EA ValidityDeadline
	// ========================================
	It("UT-VERIFY-005: should populate VerificationDeadline when EA has ValidityDeadline", func() {
		rrName := "rr-verify-005"
		namespace := "test-ns"

		deadline := metav1.NewTime(time.Now().Add(10 * time.Minute))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rrName,
				Namespace: namespace,
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseVerifying,
				EffectivenessAssessmentRef: &corev1.ObjectReference{
					Kind:      "EffectivenessAssessment",
					Name:      "ea-" + rrName,
					Namespace: namespace,
				},
			},
		}
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-" + rrName,
				Namespace: namespace,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhasePending,
				ValidityDeadline: &deadline,
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ea).
			WithStatusSubresource(rr, ea).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		recorder := record.NewFakeRecorder(20)
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, stabilizationWindow)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil, recorder, roMetrics,
			controller.TimeoutConfig{},
			&MockRoutingEngine{},
			eaCreator,
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		fetchedRR := &remediationv1.RemediationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)
		Expect(err).ToNot(HaveOccurred())

		Expect(fetchedRR.Status.VerificationDeadline).ToNot(BeNil(),
			"VerificationDeadline should be populated from EA.Status.ValidityDeadline + buffer")
	})
})
