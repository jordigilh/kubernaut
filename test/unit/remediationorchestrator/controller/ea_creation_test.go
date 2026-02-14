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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"k8s.io/client-go/tools/record"
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
	// UT-RO-EA-001: transitionToCompleted creates EA with correct spec
	// ========================================
	It("UT-RO-EA-001: should create EA when RR transitions to Completed", func() {
		rrName := "rr-ea-001"
		namespace := "test-ns"

		rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseExecuting)
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(rr).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, stabilizationWindow)
		recorder := record.NewFakeRecorder(20)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil, // audit store (nil for unit tests per DD-AUDIT-003)
			recorder,
			roMetrics,
			controller.TimeoutConfig{},
			&MockRoutingEngine{},
			eaCreator,
		)

		_, err := reconciler.TransitionToCompletedForTest(ctx, rr, "WorkflowSucceeded")
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

		// Verify owner reference
		Expect(ea.OwnerReferences).To(HaveLen(1))
		Expect(ea.OwnerReferences[0].Name).To(Equal(rrName))
	})

	// ========================================
	// UT-RO-EA-002: transitionToFailed creates EA with correct spec
	// ========================================
	It("UT-RO-EA-002: should create EA when RR transitions to Failed", func() {
		rrName := "rr-ea-002"
		namespace := "test-ns"

		rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseExecuting)
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(rr).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, stabilizationWindow)
		recorder := record.NewFakeRecorder(20)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil,
			recorder,
			roMetrics,
			controller.TimeoutConfig{},
			&MockRoutingEngine{},
			eaCreator,
		)

		_, err := reconciler.TransitionToFailedForTest(ctx, rr, "Executing", nil)
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
	// UT-RO-EA-003: handleGlobalTimeout creates EA with correct spec
	// ========================================
	It("UT-RO-EA-003: should create EA when RR times out", func() {
		rrName := "rr-ea-003"
		namespace := "test-ns"

		// Create an RR that's past its timeout
		rr := newRemediationRequestWithTimeout(rrName, namespace, remediationv1.PhaseExecuting, -2*time.Hour)

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(rr).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, stabilizationWindow)
		recorder := record.NewFakeRecorder(20)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil,
			recorder,
			roMetrics,
			controller.TimeoutConfig{},
			&MockRoutingEngine{},
			eaCreator,
		)

		_, err := reconciler.HandleGlobalTimeoutForTest(ctx, rr)
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

		rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseExecuting)
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

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
			WithObjects(rr, existingEA).
			WithStatusSubresource(rr).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, stabilizationWindow)
		recorder := record.NewFakeRecorder(20)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil,
			recorder,
			roMetrics,
			controller.TimeoutConfig{},
			&MockRoutingEngine{},
			eaCreator,
		)

		// Should not error
		_, err := reconciler.TransitionToCompletedForTest(ctx, rr, "WorkflowSucceeded")
		Expect(err).ToNot(HaveOccurred())
	})

	// ========================================
	// UT-RO-EA-005: EA creation failure is non-fatal
	// ========================================
	It("UT-RO-EA-005: should complete transition even if EA creation fails", func() {
		rrName := "rr-ea-005"
		namespace := "test-ns"

		rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseExecuting)
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		// Use interceptor to make Create fail for EA objects
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
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
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, stabilizationWindow)
		recorder := record.NewFakeRecorder(20)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil,
			recorder,
			roMetrics,
			controller.TimeoutConfig{},
			&MockRoutingEngine{},
			eaCreator,
		)

		// Transition should still succeed despite EA creation failure
		_, err := reconciler.TransitionToCompletedForTest(ctx, rr, "WorkflowSucceeded")
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
		customWindow := 2 * time.Minute

		rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseExecuting)
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(rr).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, customWindow)
		recorder := record.NewFakeRecorder(20)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil,
			recorder,
			roMetrics,
			controller.TimeoutConfig{},
			&MockRoutingEngine{},
			eaCreator,
		)

		_, err := reconciler.TransitionToCompletedForTest(ctx, rr, "WorkflowSucceeded")
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
})
