/*
Copyright 2025 Jordi Gil.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// newRemediationApprovalRequestPendingWithRequiredBy creates a pending RAR with a custom requiredBy time.
// Used for testing timeout detection path (controller detects deadline passed and marks as expired).
func newRemediationApprovalRequestPendingWithRequiredBy(name, namespace, rrName string, requiredBy metav1.Time) *remediationv1.RemediationApprovalRequest {
	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rrName,
					UID:        types.UID(rrName + "-uid"),
					Controller: ptr(true),
				},
			},
		},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rrName,
				Namespace:  namespace,
			},
			AIAnalysisRef: remediationv1.ObjectRef{
				Name: "ai-" + rrName,
			},
			Confidence:           0.4,
			ConfidenceLevel:      "low",
			Reason:               "Low confidence score requires approval",
			RequiredBy:           requiredBy,
			RecommendedWorkflow:  remediationv1.RecommendedWorkflowSummary{},
			InvestigationSummary: "Test investigation summary",
		},
		Status: remediationv1.RemediationApprovalRequestStatus{
			Decision: remediationv1.ApprovalDecisionPending,
		},
	}
}

var _ = Describe("RAR Status.Expired and Status.TimeRemaining (Bug Fix 3 & 4)", func() {
	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		fakeClient client.Client
		reconciler *prodcontroller.Reconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = signalprocessingv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
		_ = workflowexecutionv1.AddToScheme(scheme)
		_ = notificationv1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		_ = eav1.AddToScheme(scheme)
	})

	Describe("Bug Fix 3: Status.Expired on timeout", func() {
		Context("when RAR spec.requiredBy is in the past and decision is pending", func() {
			It("should set Status.Expired=true when controller detects deadline passed", func() {
				// Given: RR in AwaitingApproval with pending RAR whose requiredBy is in the past
				pastTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
				rar := newRemediationApprovalRequestPendingWithRequiredBy("rar-test-rr", "default", "test-rr", pastTime)

				initialObjects := []client.Object{
					newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAwaitingApproval, "test-rr-sp", "test-rr-ai", ""),
					newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
					newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.4, "risky-workflow"),
					rar,
				}

				fakeClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(initialObjects...).
					WithStatusSubresource(
						&remediationv1.RemediationRequest{},
						&remediationv1.RemediationApprovalRequest{},
						&signalprocessingv1.SignalProcessing{},
						&aianalysisv1.AIAnalysis{},
						&workflowexecutionv1.WorkflowExecution{},
					).
					Build()

				mockRouting := &MockRoutingEngine{}
				recorder := record.NewFakeRecorder(20)
				reconciler = prodcontroller.NewReconciler(
					fakeClient,
					fakeClient,
					scheme,
					nil,
					recorder,
					rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
					prodcontroller.TimeoutConfig{
						Global:     1 * time.Hour,
						Processing: 5 * time.Minute,
						Analyzing:  10 * time.Minute,
						Executing:  30 * time.Minute,
					},
					mockRouting,
				)

				// When: Reconcile the RR (triggers handleAwaitingApprovalPhase timeout path)
				_, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
				})
				Expect(err).ToNot(HaveOccurred())

				// Then: RAR Status.Expired must be true
				updatedRAR := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "rar-test-rr", Namespace: "default"}, updatedRAR)).To(Succeed())
				Expect(updatedRAR.Status.Expired).To(BeTrue(), "Status.Expired must be set to true when approval times out")
				Expect(updatedRAR.Status.TimeRemaining).To(Equal("0s"), "Status.TimeRemaining must be 0s when expired")
			})
		})

		Context("negative cases: Expired should remain false", func() {
			It("should have Expired=false when RAR is approved", func() {
				initialObjects := []client.Object{
					newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAwaitingApproval, "test-rr-sp", "test-rr-ai", ""),
					newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
					newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.4, "risky-workflow"),
					newRemediationApprovalRequestApproved("rar-test-rr", "default", "test-rr", "admin@example.com"),
				}

				fakeClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(initialObjects...).
					WithStatusSubresource(
						&remediationv1.RemediationRequest{},
						&remediationv1.RemediationApprovalRequest{},
						&signalprocessingv1.SignalProcessing{},
						&aianalysisv1.AIAnalysis{},
						&workflowexecutionv1.WorkflowExecution{},
					).
					Build()

				mockRouting := &MockRoutingEngine{}
				recorder := record.NewFakeRecorder(20)
				reconciler = prodcontroller.NewReconciler(
					fakeClient, fakeClient, scheme, nil, recorder,
					rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
					prodcontroller.TimeoutConfig{Global: 1 * time.Hour, Processing: 5 * time.Minute, Analyzing: 10 * time.Minute, Executing: 30 * time.Minute},
					mockRouting,
				)

				_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"}})
				Expect(err).ToNot(HaveOccurred())

				updatedRAR := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "rar-test-rr", Namespace: "default"}, updatedRAR)).To(Succeed())
				Expect(updatedRAR.Status.Expired).To(BeFalse(), "Status.Expired must be false when RAR is approved")
			})

			It("should have Expired=false when RAR is rejected", func() {
				initialObjects := []client.Object{
					newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAwaitingApproval, "test-rr-sp", "test-rr-ai", ""),
					newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
					newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.4, "risky-workflow"),
					newRemediationApprovalRequestRejected("rar-test-rr", "default", "test-rr", "admin@example.com", "Too risky"),
				}

				fakeClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(initialObjects...).
					WithStatusSubresource(
						&remediationv1.RemediationRequest{},
						&remediationv1.RemediationApprovalRequest{},
						&signalprocessingv1.SignalProcessing{},
						&aianalysisv1.AIAnalysis{},
						&workflowexecutionv1.WorkflowExecution{},
					).
					Build()

				mockRouting := &MockRoutingEngine{}
				recorder := record.NewFakeRecorder(20)
				reconciler = prodcontroller.NewReconciler(
					fakeClient, fakeClient, scheme, nil, recorder,
					rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
					prodcontroller.TimeoutConfig{Global: 1 * time.Hour, Processing: 5 * time.Minute, Analyzing: 10 * time.Minute, Executing: 30 * time.Minute},
					mockRouting,
				)

				_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"}})
				Expect(err).ToNot(HaveOccurred())

				updatedRAR := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "rar-test-rr", Namespace: "default"}, updatedRAR)).To(Succeed())
				Expect(updatedRAR.Status.Expired).To(BeFalse(), "Status.Expired must be false when RAR is rejected")
			})
		})
	})

	Describe("Bug Fix 4: Status.TimeRemaining", func() {
		Context("when RAR is pending with requiredBy in the future", func() {
			It("should set Status.TimeRemaining to non-empty string (e.g. contains 's' for seconds)", func() {
				// Given: RAR with requiredBy 60 seconds in the future
				futureTime := metav1.NewTime(time.Now().Add(60 * time.Second))
				rar := newRemediationApprovalRequestPendingWithRequiredBy("rar-test-rr", "default", "test-rr", futureTime)

				initialObjects := []client.Object{
					newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAwaitingApproval, "test-rr-sp", "test-rr-ai", ""),
					newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
					newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.4, "risky-workflow"),
					rar,
				}

				fakeClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(initialObjects...).
					WithStatusSubresource(
						&remediationv1.RemediationRequest{},
						&remediationv1.RemediationApprovalRequest{},
						&signalprocessingv1.SignalProcessing{},
						&aianalysisv1.AIAnalysis{},
						&workflowexecutionv1.WorkflowExecution{},
					).
					Build()

				mockRouting := &MockRoutingEngine{}
				recorder := record.NewFakeRecorder(20)
				reconciler = prodcontroller.NewReconciler(
					fakeClient, fakeClient, scheme, nil, recorder,
					rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
					prodcontroller.TimeoutConfig{Global: 1 * time.Hour, Processing: 5 * time.Minute, Analyzing: 10 * time.Minute, Executing: 30 * time.Minute},
					mockRouting,
				)

				// When: Reconcile (RAR is pending, not expired)
				_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"}})
				Expect(err).ToNot(HaveOccurred())

				// Then: TimeRemaining must be non-empty and contain "s" (Go duration format)
				updatedRAR := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "rar-test-rr", Namespace: "default"}, updatedRAR)).To(Succeed())
				Expect(updatedRAR.Status.TimeRemaining).ToNot(BeEmpty(), "Status.TimeRemaining must be populated when RAR is pending")
				Expect(updatedRAR.Status.TimeRemaining).To(ContainSubstring("s"), "TimeRemaining should use Go duration format (e.g. 59s, 1m0s)")
			})
		})
	})
})
