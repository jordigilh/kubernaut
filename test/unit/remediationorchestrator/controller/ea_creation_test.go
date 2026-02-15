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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// ============================================================================
// EA CREATION UNIT TESTS (BR-EM-001, ADR-EM-001)
// Business Requirement: RO creates EffectivenessAssessment CRD on terminal phases
//
// All tests drive the reconciler through the public Reconcile() method.
// - Completed transition: RR in Executing + completed WorkflowExecution
// - Failed transition: RR in Executing + failed WorkflowExecution
// - Timeout transition: RR in non-terminal phase with expired StartTime
// ============================================================================
var _ = Describe("EA Creation on Terminal Transitions (ADR-EM-001)", func() {

	var (
		ctx                 context.Context
		scheme              = setupScheme()
		stabilizationWindow = 30 * time.Second
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// UT-RO-EA-001: Reconcile with completed WE creates EA with correct spec
	// ========================================
	It("UT-RO-EA-001: should create EA when RR transitions to Completed via Reconcile", func() {
		rrName := "rr-ea-001"
		namespace := "test-ns"
		weName := "we-" + rrName

		// RR in Executing phase with WorkflowExecutionRef
		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseExecuting, "", "", weName)

		// Completed WorkflowExecution owned by RR
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

		// Verify EA was created
		ea := &eav1.EffectivenessAssessment{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      "ea-" + rrName,
			Namespace: namespace,
		}, ea)
		Expect(err).ToNot(HaveOccurred(), "EA should have been created")

		// Verify EA spec
		Expect(ea.Spec.CorrelationID).To(Equal(rrName))
		Expect(ea.Spec.RemediationRequestPhase).To(Equal("Completed"))
		Expect(ea.Spec.TargetResource.Kind).To(Equal("Deployment"))
		Expect(ea.Spec.TargetResource.Name).To(Equal("test-app"))
		Expect(ea.Spec.TargetResource.Namespace).To(Equal(namespace))
		Expect(ea.Spec.Config.StabilizationWindow.Duration).To(Equal(stabilizationWindow))

		// Verify owner reference (ADR-EM-001 Section 8: blockOwnerDeletion=false)
		Expect(ea.OwnerReferences).To(HaveLen(1))
		Expect(ea.OwnerReferences[0].Name).To(Equal(rrName))
		Expect(*ea.OwnerReferences[0].BlockOwnerDeletion).To(BeFalse(),
			"ADR-EM-001: blockOwnerDeletion must be false to prevent RR deletion blocking on EA")
	})

	// ========================================
	// UT-RO-EA-002: Reconcile with failed WE creates EA with Failed phase
	// ========================================
	It("UT-RO-EA-002: should create EA when RR transitions to Failed via Reconcile", func() {
		rrName := "rr-ea-002"
		namespace := "test-ns"
		weName := "we-" + rrName

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseExecuting, "", "", weName)

		// Failed WorkflowExecution
		we := newWorkflowExecutionFailed(weName, namespace, rrName, "workflow script failed")

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

		// Verify EA was created with Failed phase in spec
		ea := &eav1.EffectivenessAssessment{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      "ea-" + rrName,
			Namespace: namespace,
		}, ea)
		Expect(err).ToNot(HaveOccurred(), "EA should have been created on failure")
		Expect(ea.Spec.RemediationRequestPhase).To(Equal("Failed"))
	})

	// ========================================
	// UT-RO-EA-003: Reconcile with expired global timeout creates EA
	// ========================================
	It("UT-RO-EA-003: should create EA when RR times out via Reconcile", func() {
		rrName := "rr-ea-003"
		namespace := "test-ns"

		// Create an RR that's past its timeout (start time 2 hours ago)
		rr := newRemediationRequestWithTimeout(rrName, namespace, remediationv1.PhaseExecuting, -2*time.Hour)

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
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

		// Verify EA was created with TimedOut phase in spec
		ea := &eav1.EffectivenessAssessment{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      "ea-" + rrName,
			Namespace: namespace,
		}, ea)
		Expect(err).ToNot(HaveOccurred(), "EA should have been created on timeout")
		Expect(ea.Spec.RemediationRequestPhase).To(Equal("TimedOut"))
	})

	// ========================================
	// UT-RO-EA-004: EA creation is idempotent
	// ========================================
	It("UT-RO-EA-004: should not error when EA already exists (idempotent)", func() {
		rrName := "rr-ea-004"
		namespace := "test-ns"
		weName := "we-" + rrName

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseExecuting, "", "", weName)
		we := newWorkflowExecutionCompleted(weName, namespace, rrName)

		// Pre-create the EA
		existingEA := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-" + rrName,
				Namespace: namespace,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           rrName,
				RemediationRequestPhase: "Completed",
				TargetResource: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: namespace,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: stabilizationWindow},
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, we, existingEA).
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

		// Should not error
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())
	})

	// ========================================
	// UT-RO-EA-005: EA creation failure is non-fatal
	// ========================================
	It("UT-RO-EA-005: should complete transition even if EA creation fails", func() {
		rrName := "rr-ea-005"
		namespace := "test-ns"
		weName := "we-" + rrName

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseExecuting, "", "", weName)
		we := newWorkflowExecutionCompleted(weName, namespace, rrName)

		// Use interceptor to make Create fail for EA objects
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, we).
			WithStatusSubresource(rr).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*eav1.EffectivenessAssessment); ok {
						return fmt.Errorf("simulated EA creation failure")
					}
					return c.Create(ctx, obj, opts...)
				},
			}).
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

		// Transition should still succeed despite EA creation failure
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred(), "Phase transition must succeed even if EA creation fails")

		// Verify RR transitioned to Completed
		fetchedRR := &remediationv1.RemediationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)
		Expect(err).ToNot(HaveOccurred())
		Expect(fetchedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted))
	})

	// ========================================
	// UT-RO-EA-006: EA spec contains correct stabilizationWindow from config
	// ========================================
	It("UT-RO-EA-006: should propagate config-driven stabilization window to EA spec", func() {
		rrName := "rr-ea-006"
		namespace := "test-ns"
		weName := "we-" + rrName
		customWindow := 2 * time.Minute

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseExecuting, "", "", weName)
		we := newWorkflowExecutionCompleted(weName, namespace, rrName)

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, we).
			WithStatusSubresource(rr).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		recorder := record.NewFakeRecorder(20)
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, customWindow)
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
		Expect(err).ToNot(HaveOccurred())
		Expect(ea.Spec.Config.StabilizationWindow.Duration).To(Equal(customWindow),
			"EA should have custom stabilization window from config")
	})

	// ========================================
	// UT-RO-EA-007: EA ref persisted on RR status after EA creation (Batch 3)
	// ========================================
	It("UT-RO-EA-007: should persist EffectivenessAssessmentRef on RR status after EA creation", func() {
		rrName := "rr-ea-007"
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
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, 30*time.Second)
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

		// Refetch RR from the fake store to verify persistence
		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())

		// ADR-EM-001, Batch 3: EffectivenessAssessmentRef should be set and persisted
		Expect(fetchedRR.Status.EffectivenessAssessmentRef).ToNot(BeNil(),
			"EffectivenessAssessmentRef should be set after EA creation")
		Expect(fetchedRR.Status.EffectivenessAssessmentRef.Name).To(Equal("ea-"+rrName),
			"EffectivenessAssessmentRef should reference the created EA")
		Expect(fetchedRR.Status.EffectivenessAssessmentRef.Kind).To(Equal("EffectivenessAssessment"))
		Expect(fetchedRR.Status.EffectivenessAssessmentRef.Namespace).To(Equal(namespace))
	})

	// ========================================
	// UT-RO-EA-008: Initial EffectivenessAssessed=False on EA creation (GAP-2)
	// ADR-EM-001 Section 9.4.15: When EA is first created, the RR must have
	// EffectivenessAssessed=False / Reason: AssessmentInProgress so operators
	// can distinguish "no EA yet" from "EA in progress."
	// ========================================
	// ========================================
	// UT-RO-EA-009: EA creator propagates PreRemediationSpecHash from RR status to EA spec
	// ========================================
	It("UT-RO-EA-009: should copy PreRemediationSpecHash from RR status to EA spec", func() {
		rrName := "rr-ea-009"
		namespace := "test-ns"
		weName := "we-" + rrName

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseExecuting, "", "", weName)
		// Simulate RO having stored the pre-remediation hash on RR status
		rr.Status.PreRemediationSpecHash = "sha256:abc123def456pre"

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

		// Verify EA was created with PreRemediationSpecHash from RR status
		ea := &eav1.EffectivenessAssessment{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      "ea-" + rrName,
			Namespace: namespace,
		}, ea)
		Expect(err).ToNot(HaveOccurred(), "EA should have been created")
		Expect(ea.Spec.PreRemediationSpecHash).To(Equal("sha256:abc123def456pre"),
			"EA spec should contain PreRemediationSpecHash copied from RR status (DD-EM-002 v2.0)")
	})

	// ========================================
	// UT-RO-EA-010: EA creator handles empty PreRemediationSpecHash gracefully
	// ========================================
	It("UT-RO-EA-010: should create EA with empty PreRemediationSpecHash when RR has none", func() {
		rrName := "rr-ea-010"
		namespace := "test-ns"
		weName := "we-" + rrName

		rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseExecuting, "", "", weName)
		// RR has no PreRemediationSpecHash (backward compatibility)

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
		Expect(err).ToNot(HaveOccurred(), "EA should have been created")
		Expect(ea.Spec.PreRemediationSpecHash).To(BeEmpty(),
			"EA spec PreRemediationSpecHash should be empty when RR has none (backward compat)")
	})

	It("UT-RO-EA-008: should set EffectivenessAssessed=False/AssessmentInProgress on EA creation", func() {
		rrName := "rr-ea-008"
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

		// Refetch RR to verify persisted condition
		fetchedRR := &remediationv1.RemediationRequest{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())

		// ADR-EM-001 Section 9.4.15: Initial condition on EA creation
		cond := meta.FindStatusCondition(fetchedRR.Status.Conditions, "EffectivenessAssessed")
		Expect(cond).ToNot(BeNil(), "EffectivenessAssessed condition should be set on EA creation")
		Expect(cond.Status).To(Equal(metav1.ConditionFalse),
			"Initial EffectivenessAssessed should be False (assessment in progress)")
		Expect(cond.Reason).To(Equal("AssessmentInProgress"),
			"Reason should indicate assessment is in progress")
		Expect(cond.Message).To(ContainSubstring("ea-"+rrName),
			"Message should reference the EA name")
	})
})
