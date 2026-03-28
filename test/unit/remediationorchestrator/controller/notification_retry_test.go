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
	"sync/atomic"
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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// ============================================================================
// NOTIFICATION RETRY UNIT TESTS (#281)
// Business Requirement: BR-ORCH-045 (completion notification), BR-ORCH-034 (bulk duplicate)
//
// The RO must retry NotificationRequest creation on transient failures.
// handleVerifyingPhase requeues periodically, giving a natural retry loop.
// ensureNotificationsCreated uses hasNotificationRef for idempotency.
// ============================================================================
var _ = Describe("NotificationRequest Retry (#281)", func() {

	var (
		ctx                 context.Context
		scheme              = setupScheme()
		stabilizationWindow = 30 * time.Second
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// UT-NT-RETRY-001: Completion notification transient failure triggers retry
	// ========================================
	It("UT-NT-RETRY-001: should retry completion notification creation on next reconcile after transient failure", func() {
		rrName := "rr-nt-retry-001"
		namespace := "test-ns"
		aiName := fmt.Sprintf("ai-%s", rrName)
		eaName := fmt.Sprintf("ea-%s", rrName)
		completionNTName := fmt.Sprintf("nr-completion-%s", rrName)

		deadline := metav1.NewTime(time.Now().Add(10 * time.Minute))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              rrName,
				Namespace:         namespace,
				CreationTimestamp: metav1.Now(),
				UID:               types.UID(rrName + "-uid"),
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalName:        "test-signal",
				SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: namespace,
				},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseVerifying,
				Outcome:      "Remediated",
				EffectivenessAssessmentRef: &corev1.ObjectReference{
					Kind:      "EffectivenessAssessment",
					Name:      eaName,
					Namespace: namespace,
				},
				VerificationDeadline: &deadline,
			},
		}

		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiName,
				Namespace: namespace,
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     aianalysisv1.PhaseCompleted,
				RootCause: "Memory leak in container",
			},
		}

		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      eaName,
				Namespace: namespace,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           rrName,
				RemediationRequestPhase: "Verifying",
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseAssessing,
				ValidityDeadline: &deadline,
			},
		}

		// Interceptor: fail first Create for NotificationRequest, succeed on second
		var createCallCount int32
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, ea).
			WithStatusSubresource(rr, ea).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*notificationv1.NotificationRequest); ok {
						count := atomic.AddInt32(&createCallCount, 1)
						if count == 1 {
							return fmt.Errorf("simulated transient API failure")
						}
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
			controller.TimeoutConfig{Verifying: 30 * time.Minute},
			&MockRoutingEngine{},
			eaCreator,
		)

		// First reconcile: notification creation fails (transient), but RR stays in Verifying
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		// NotificationRequest should NOT exist yet
		nt := &notificationv1.NotificationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: completionNTName, Namespace: namespace}, nt)
		Expect(err).To(HaveOccurred(), "NotificationRequest should not exist after first failed attempt")

		// Second reconcile: notification creation succeeds
		_, err = reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		// NotificationRequest should now exist
		err = k8sClient.Get(ctx, types.NamespacedName{Name: completionNTName, Namespace: namespace}, nt)
		Expect(err).ToNot(HaveOccurred(), "NotificationRequest should exist after retry")

		// NotificationRequestRefs should contain the completion ref
		fetchedRR := &remediationv1.RemediationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)
		Expect(err).ToNot(HaveOccurred())
		Expect(fetchedRR.Status.NotificationRequestRefs).To(ContainElement(
			HaveField("Name", completionNTName),
		), "NotificationRequestRefs should track the completion notification after retry")
	})

	// ========================================
	// UT-NT-RETRY-002: Bulk duplicate notification transient failure triggers retry
	// ========================================
	It("UT-NT-RETRY-002: should retry bulk duplicate notification creation on next reconcile after transient failure", func() {
		rrName := "rr-nt-retry-002"
		namespace := "test-ns"
		aiName := fmt.Sprintf("ai-%s", rrName)
		eaName := fmt.Sprintf("ea-%s", rrName)
		bulkNTName := fmt.Sprintf("nr-bulk-%s", rrName)
		completionNTName := fmt.Sprintf("nr-completion-%s", rrName)

		deadline := metav1.NewTime(time.Now().Add(10 * time.Minute))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              rrName,
				Namespace:         namespace,
				CreationTimestamp: metav1.Now(),
				UID:               types.UID(rrName + "-uid"),
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalName:        "test-signal",
				SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: namespace,
				},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase:   remediationv1.PhaseVerifying,
				Outcome:        "Remediated",
				DuplicateCount: 3,
				EffectivenessAssessmentRef: &corev1.ObjectReference{
					Kind:      "EffectivenessAssessment",
					Name:      eaName,
					Namespace: namespace,
				},
				VerificationDeadline: &deadline,
				// Completion notification already tracked (only bulk fails)
				NotificationRequestRefs: []corev1.ObjectReference{
					{
						Kind:       "NotificationRequest",
						Name:       completionNTName,
						Namespace:  namespace,
						APIVersion: "notification.kubernaut.ai/v1alpha1",
					},
				},
			},
		}

		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiName,
				Namespace: namespace,
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     aianalysisv1.PhaseCompleted,
				RootCause: "Memory leak in container",
			},
		}

		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      eaName,
				Namespace: namespace,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           rrName,
				RemediationRequestPhase: "Verifying",
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseAssessing,
				ValidityDeadline: &deadline,
			},
		}

		// Pre-create the completion notification (already exists, only bulk should be retried)
		completionNT := &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      completionNTName,
				Namespace: namespace,
			},
			Spec: notificationv1.NotificationRequestSpec{
				Type:     notificationv1.NotificationTypeSimple,
				Priority: notificationv1.NotificationPriorityMedium,
				Severity: "info",
				Subject:  "Completion",
				Body:     "Remediation completed",
			},
		}

		// Interceptor: fail first Create for bulk NotificationRequest, succeed on second
		var bulkCreateCount int32
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, ea, completionNT).
			WithStatusSubresource(rr, ea).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if nr, ok := obj.(*notificationv1.NotificationRequest); ok {
						if nr.Name == bulkNTName {
							count := atomic.AddInt32(&bulkCreateCount, 1)
							if count == 1 {
								return fmt.Errorf("simulated transient API failure for bulk")
							}
						}
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
			controller.TimeoutConfig{Verifying: 30 * time.Minute},
			&MockRoutingEngine{},
			eaCreator,
		)

		// First reconcile: bulk notification creation fails (transient)
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		// Bulk NotificationRequest should NOT exist yet
		nt := &notificationv1.NotificationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: bulkNTName, Namespace: namespace}, nt)
		Expect(err).To(HaveOccurred(), "Bulk NotificationRequest should not exist after first failed attempt")

		// Second reconcile: bulk notification creation succeeds
		_, err = reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		// Bulk NotificationRequest should now exist
		err = k8sClient.Get(ctx, types.NamespacedName{Name: bulkNTName, Namespace: namespace}, nt)
		Expect(err).ToNot(HaveOccurred(), "Bulk NotificationRequest should exist after retry")

		// NotificationRequestRefs should contain both completion and bulk refs
		fetchedRR := &remediationv1.RemediationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)
		Expect(err).ToNot(HaveOccurred())
		Expect(fetchedRR.Status.NotificationRequestRefs).To(ContainElement(
			HaveField("Name", bulkNTName),
		), "NotificationRequestRefs should track the bulk duplicate notification after retry")
	})

	// ========================================
	// UT-NT-RETRY-003: Idempotency -- already-created notifications are not duplicated
	// ========================================
	It("UT-NT-RETRY-003: should not create duplicate notifications when refs already tracked", func() {
		rrName := "rr-nt-retry-003"
		namespace := "test-ns"
		aiName := fmt.Sprintf("ai-%s", rrName)
		eaName := fmt.Sprintf("ea-%s", rrName)
		completionNTName := fmt.Sprintf("nr-completion-%s", rrName)
		bulkNTName := fmt.Sprintf("nr-bulk-%s", rrName)

		deadline := metav1.NewTime(time.Now().Add(10 * time.Minute))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              rrName,
				Namespace:         namespace,
				CreationTimestamp: metav1.Now(),
				UID:               types.UID(rrName + "-uid"),
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalName:        "test-signal",
				SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: namespace,
				},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase:   remediationv1.PhaseVerifying,
				Outcome:        "Remediated",
				DuplicateCount: 2,
				EffectivenessAssessmentRef: &corev1.ObjectReference{
					Kind:      "EffectivenessAssessment",
					Name:      eaName,
					Namespace: namespace,
				},
				VerificationDeadline: &deadline,
				// Both notifications already tracked
				NotificationRequestRefs: []corev1.ObjectReference{
					{
						Kind:       "NotificationRequest",
						Name:       completionNTName,
						Namespace:  namespace,
						APIVersion: "notification.kubernaut.ai/v1alpha1",
					},
					{
						Kind:       "NotificationRequest",
						Name:       bulkNTName,
						Namespace:  namespace,
						APIVersion: "notification.kubernaut.ai/v1alpha1",
					},
				},
			},
		}

		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiName,
				Namespace: namespace,
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     aianalysisv1.PhaseCompleted,
				RootCause: "Memory leak in container",
			},
		}

		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      eaName,
				Namespace: namespace,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           rrName,
				RemediationRequestPhase: "Verifying",
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseAssessing,
				ValidityDeadline: &deadline,
			},
		}

		// Pre-create both notifications in the cluster
		completionNT := &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      completionNTName,
				Namespace: namespace,
			},
			Spec: notificationv1.NotificationRequestSpec{
				Type:     notificationv1.NotificationTypeSimple,
				Priority: notificationv1.NotificationPriorityMedium,
				Severity: "info",
				Subject:  "Completion",
				Body:     "Remediation completed",
			},
		}
		bulkNT := &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bulkNTName,
				Namespace: namespace,
			},
			Spec: notificationv1.NotificationRequestSpec{
				Type:     notificationv1.NotificationTypeSimple,
				Priority: notificationv1.NotificationPriorityLow,
				Severity: "low",
				Subject:  "Bulk Duplicate",
				Body:     "Bulk duplicate notification",
			},
		}

		// Track Create calls to detect duplicate creation attempts
		var createCallCount int32
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, ea, completionNT, bulkNT).
			WithStatusSubresource(rr, ea).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*notificationv1.NotificationRequest); ok {
						atomic.AddInt32(&createCallCount, 1)
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
			controller.TimeoutConfig{Verifying: 30 * time.Minute},
			&MockRoutingEngine{},
			eaCreator,
		)

		// Reconcile with all notifications already tracked
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		// No new NotificationRequest Create calls should have been made
		Expect(atomic.LoadInt32(&createCallCount)).To(Equal(int32(0)),
			"No new NotificationRequests should be created when refs are already tracked")

		// Refs should remain unchanged
		fetchedRR := &remediationv1.RemediationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)
		Expect(err).ToNot(HaveOccurred())
		Expect(fetchedRR.Status.NotificationRequestRefs).To(HaveLen(2),
			"NotificationRequestRefs should remain unchanged")
	})
})

// ============================================================================
// VERIFICATION CONTEXT RECONCILER INTEGRATION TESTS (#318)
// Business Requirement: BR-ORCH-045 (completion notification includes EA verification)
//
// These tests exercise the full reconciler path: ensureNotificationsCreated
// fetches the EA via EffectivenessAssessmentRef and passes it to
// CreateCompletionNotification, which populates the verification section.
// ============================================================================
var _ = Describe("Completion Notification Verification Context (#318)", func() {

	var (
		ctx                 context.Context
		scheme              = setupScheme()
		stabilizationWindow = 30 * time.Second
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// IT-RO-318-004: Reconciler fetches EA via ref
	// ========================================
	It("IT-RO-318-004: should include EA verification data in completion notification when EA ref exists", func() {
		rrName := "rr-it-318-004"
		namespace := "test-ns"
		aiName := fmt.Sprintf("ai-%s", rrName)
		eaName := fmt.Sprintf("ea-%s", rrName)
		completionNTName := fmt.Sprintf("nr-completion-%s", rrName)

		deadline := metav1.NewTime(time.Now().Add(10 * time.Minute))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              rrName,
				Namespace:         namespace,
				CreationTimestamp: metav1.Now(),
				UID:               types.UID(rrName + "-uid"),
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalName:        "test-signal",
				SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: namespace,
				},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseVerifying,
				Outcome:      "Remediated",
				EffectivenessAssessmentRef: &corev1.ObjectReference{
					Kind:      "EffectivenessAssessment",
					Name:      eaName,
					Namespace: namespace,
				},
				VerificationDeadline: &deadline,
			},
		}

		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiName,
				Namespace: namespace,
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     aianalysisv1.PhaseCompleted,
				RootCause: "Memory leak in container",
			},
		}

		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      eaName,
				Namespace: namespace,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           rrName,
				RemediationRequestPhase: "Verifying",
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseCompleted,
				AssessmentReason: eav1.AssessmentReasonFull,
				ValidityDeadline: &deadline,
				Components: eav1.EAComponents{
					HealthAssessed:          true,
					HealthScore:             float64Ptr(1.0),
					AlertAssessed:           true,
					AlertScore:              float64Ptr(1.0),
					MetricsAssessed:         true,
					MetricsScore:            float64Ptr(1.0),
					HashComputed:            true,
					PostRemediationSpecHash: "sha256:abc123",
					CurrentSpecHash:         "sha256:abc123",
				},
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, ea).
			WithStatusSubresource(rr, ea).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		recorder := record.NewFakeRecorder(20)
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, stabilizationWindow)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil, recorder, roMetrics,
			controller.TimeoutConfig{Verifying: 30 * time.Minute},
			&MockRoutingEngine{},
			eaCreator,
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: completionNTName, Namespace: namespace}, nr)
		Expect(err).ToNot(HaveOccurred(), "Completion NotificationRequest should exist")

		Expect(nr.Spec.Body).To(ContainSubstring("Verification Results"))
		Expect(nr.Spec.Body).To(ContainSubstring("Verification passed"))

		Expect(nr.Spec.Context.Verification.Assessed).To(BeTrue())
		Expect(nr.Spec.Context.Verification.Outcome).To(Equal("passed"))
		Expect(nr.Spec.Context.Verification.Reason).To(Equal("full"))
	})

	// ========================================
	// IT-RO-318-005: Reconciler handles missing EA ref gracefully
	// ========================================
	It("IT-RO-318-005: should create completion notification with 'not available' when EA ref is nil", func() {
		rrName := "rr-it-318-005"
		namespace := "test-ns"
		aiName := fmt.Sprintf("ai-%s", rrName)
		completionNTName := fmt.Sprintf("nr-completion-%s", rrName)

		deadline := metav1.NewTime(time.Now().Add(10 * time.Minute))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              rrName,
				Namespace:         namespace,
				CreationTimestamp: metav1.Now(),
				UID:               types.UID(rrName + "-uid"),
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalName:        "test-signal",
				SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: namespace,
				},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase:               remediationv1.PhaseVerifying,
				Outcome:                    "Remediated",
				EffectivenessAssessmentRef: nil,
				VerificationDeadline:       &deadline,
			},
		}

		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aiName,
				Namespace: namespace,
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     aianalysisv1.PhaseCompleted,
				RootCause: "Memory leak in container",
			},
		}

		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai).
			WithStatusSubresource(rr).
			Build()

		roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		recorder := record.NewFakeRecorder(20)
		eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, stabilizationWindow)
		reconciler := controller.NewReconciler(
			k8sClient, k8sClient, scheme,
			nil, recorder, roMetrics,
			controller.TimeoutConfig{Verifying: 30 * time.Minute},
			&MockRoutingEngine{},
			eaCreator,
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: completionNTName, Namespace: namespace}, nr)
		Expect(err).ToNot(HaveOccurred(), "Completion NotificationRequest should exist even without EA ref")

		Expect(nr.Spec.Body).To(ContainSubstring("not available"))

		Expect(nr.Spec.Context.Verification.Assessed).To(BeFalse())
		Expect(nr.Spec.Context.Verification.Outcome).To(Equal("unavailable"))
	})
})
