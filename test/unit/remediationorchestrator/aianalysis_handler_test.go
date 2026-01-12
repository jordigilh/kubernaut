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

package remediationorchestrator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/handler"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("AIAnalysisHandler", func() {
	var (
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = notificationv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
	})

	Describe("Helper Functions", func() {
		Context("IsWorkflowResolutionFailed", func() {
			// Test #2: Returns true for WorkflowResolutionFailed
			It("should return true when Phase=Failed and Reason=WorkflowResolutionFailed", func() {
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "WorkflowResolutionFailed"

				Expect(handler.IsWorkflowResolutionFailed(ai)).To(BeTrue())
			})

			// Test #3: Returns false for other failures
			It("should return false when Phase=Failed but Reason is different", func() {
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "APIError"

				Expect(handler.IsWorkflowResolutionFailed(ai)).To(BeFalse())
			})

			// Test #4: Returns false for Completed phase
			It("should return false when Phase=Completed", func() {
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Completed"
				ai.Status.Reason = "WorkflowResolutionFailed"

				Expect(handler.IsWorkflowResolutionFailed(ai)).To(BeFalse())
			})
		})

		Context("IsWorkflowNotNeeded", func() {
			// Test #5: Returns true for WorkflowNotNeeded
			It("should return true when Phase=Completed and Reason=WorkflowNotNeeded", func() {
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Completed"
				ai.Status.Reason = "WorkflowNotNeeded"

				Expect(handler.IsWorkflowNotNeeded(ai)).To(BeTrue())
			})

			// Test #6: Returns false for normal completion
			It("should return false when Phase=Completed but Reason is different", func() {
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Completed"
				ai.Status.Reason = ""

				Expect(handler.IsWorkflowNotNeeded(ai)).To(BeFalse())
			})

			// Test #7: Returns false for Failed phase
			It("should return false when Phase=Failed", func() {
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "WorkflowNotNeeded"

				Expect(handler.IsWorkflowNotNeeded(ai)).To(BeFalse())
			})
		})

		Context("RequiresManualReview", func() {
			// Test #8: Returns true for WorkflowResolutionFailed
			It("should return true for WorkflowResolutionFailed", func() {
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "WorkflowResolutionFailed"

				Expect(handler.RequiresManualReview(ai)).To(BeTrue())
			})

			// Test #9: Returns false for normal completion
			It("should return false for normal completion", func() {
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Completed"

				Expect(handler.RequiresManualReview(ai)).To(BeFalse())
			})
		})
	})

	Describe("HandleAIAnalysisStatus", func() {
		var (
			fakeClient *fake.ClientBuilder
			h          *handler.AIAnalysisHandler
			nc         *creator.NotificationCreator
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			ctx = context.Background()
		})

		Context("In-Progress Phases", func() {
			// Test #10: Pending phase - no action
			It("should return no error for Pending phase", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClient.WithObjects(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Pending"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
			})

			// Test #11: Investigating phase - no action
			It("should return no error for Investigating phase", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClient.WithObjects(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Investigating"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
			})

			// Test #12: Analyzing phase - no action
			It("should return no error for Analyzing phase", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClient.WithObjects(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Analyzing"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
			})
		})

		Context("BR-ORCH-037: WorkflowNotNeeded Handling", func() {
			// Test #13: Sets RR status to Completed with NoActionRequired
			It("should set RR status to Completed with Outcome=NoActionRequired", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClient.WithObjects(rr).WithStatusSubresource(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Completed"
				ai.Status.Reason = "WorkflowNotNeeded"
				ai.Status.SubReason = "ProblemResolved"
				ai.Status.Message = "Issue self-resolved"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())

				// Verify RR status was updated
				updatedRR := &remediationv1.RemediationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updatedRR)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted))
				Expect(updatedRR.Status.Outcome).To(Equal("NoActionRequired"))
				Expect(updatedRR.Status.CompletedAt).ToNot(BeNil())
			})
		})

		Context("BR-ORCH-001: Approval Required", func() {
			// Test #14: Creates approval notification when ApprovalRequired=true
			It("should create approval notification when ApprovalRequired=true", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClient.WithObjects(rr).WithStatusSubresource(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Completed"
				ai.Status.ApprovalRequired = true
				ai.Status.ApprovalReason = "low_confidence"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())

				// Verify notification was created
				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))
				Expect(nrList.Items[0].Name).To(Equal("nr-approval-test-rr"))
			})
		})

		Context("BR-ORCH-036: WorkflowResolutionFailed Handling", func() {
			// Test #15: Creates manual review notification
			It("should create manual review notification for WorkflowResolutionFailed", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClient.WithObjects(rr).WithStatusSubresource(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "WorkflowResolutionFailed"
				ai.Status.SubReason = "WorkflowNotFound"
				ai.Status.Message = "No matching workflow found"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())

				// Verify manual review notification was created
				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))
				Expect(nrList.Items[0].Name).To(Equal("nr-manual-review-test-rr"))
				Expect(nrList.Items[0].Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
			})

			// Test #16: Sets RR status to Failed with ManualReviewRequired
			It("should set RR status to Failed with Outcome=ManualReviewRequired", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClient.WithObjects(rr).WithStatusSubresource(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "WorkflowResolutionFailed"
				ai.Status.SubReason = "NoMatchingWorkflows"
				ai.Status.Message = "No workflows matched"

				_, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				// Verify RR status was updated
				updatedRR := &remediationv1.RemediationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updatedRR)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed))
				Expect(updatedRR.Status.Outcome).To(Equal("ManualReviewRequired"))
				Expect(*updatedRR.Status.FailurePhase).To(Equal("ai_analysis"))
				Expect(updatedRR.Status.RequiresManualReview).To(BeTrue())
			})

			// Test #17: Includes RootCauseAnalysis in context
			It("should include RootCauseAnalysis in notification context", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClient.WithObjects(rr).WithStatusSubresource(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "WorkflowResolutionFailed"
				ai.Status.SubReason = "LowConfidence"
				ai.Status.RootCause = "Pod crash loop detected"

				_, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				// Verify notification includes root cause
				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))
				Expect(nrList.Items[0].Spec.Metadata).To(HaveKeyWithValue("rootCauseAnalysis", "Pod crash loop detected"))
			})
		})

		Context("Other Failures", func() {
			// Test #18: Propagates non-WorkflowResolutionFailed failures
			It("should propagate failure to RR for non-WorkflowResolutionFailed", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClient.WithObjects(rr).WithStatusSubresource(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "APIError"
				ai.Status.Message = "LLM API timeout"

				_, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				// Verify RR status was updated
				updatedRR := &remediationv1.RemediationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updatedRR)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed))
				Expect(*updatedRR.Status.FailurePhase).To(Equal("ai_analysis"))
				Expect(*updatedRR.Status.FailureReason).To(ContainSubstring("APIError"))

				// No notification created for non-manual-review failures
				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(BeEmpty())
			})
		})
	})
})
