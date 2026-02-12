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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			DescribeTable("should return correct result based on Phase and Reason",
				func(phase, reason string, expected bool) {
					ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
					ai.Status.Phase = phase
					ai.Status.Reason = reason

					Expect(handler.IsWorkflowResolutionFailed(ai)).To(Equal(expected))
				},
				Entry("returns true when Phase=Failed and Reason=WorkflowResolutionFailed", "Failed", "WorkflowResolutionFailed", true),
				Entry("returns false when Phase=Failed but Reason is different", "Failed", "APIError", false),
				Entry("returns false when Phase=Completed", "Completed", "WorkflowResolutionFailed", false),
				Entry("returns false when Phase=Analyzing", "Analyzing", "WorkflowResolutionFailed", false),
				Entry("returns false when Phase=Failed with empty Reason", "Failed", "", false),
			)
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
			fakeClientBuilder     *fake.ClientBuilder
			h                     *handler.AIAnalysisHandler
			nc                    *creator.NotificationCreator
			ctx                   context.Context
			mockTransitionFailed  func(context.Context, *remediationv1.RemediationRequest, string, error) (ctrl.Result, error)
			transitionFailedCalls int
		)

		BeforeEach(func() {
			fakeClientBuilder = fake.NewClientBuilder().WithScheme(scheme)
			ctx = context.Background()
			// Default no-op callback for tests that don't trigger failure paths
			transitionFailedCalls = 0
			mockTransitionFailed = func(ctx context.Context, rr *remediationv1.RemediationRequest, phase string, reason error) (ctrl.Result, error) {
				transitionFailedCalls++
				return ctrl.Result{}, nil
			}
		})

		// createMockTransitionFailed creates a mock callback that persists status changes
		// This must be called after the client is built to have access to the client instance
		createMockTransitionFailed := func(c client.WithWatch) func(context.Context, *remediationv1.RemediationRequest, string, error) (ctrl.Result, error) {
			transitionFailedCalls = 0
			return func(ctx context.Context, rr *remediationv1.RemediationRequest, phase string, reason error) (ctrl.Result, error) {
				transitionFailedCalls++
				// Simulate the transition by directly updating the phase and persisting to fake client
				rr.Status.OverallPhase = remediationv1.PhaseFailed
				failurePhase := phase
				rr.Status.FailurePhase = &failurePhase
				reasonStr := reason.Error()
				rr.Status.FailureReason = &reasonStr
				// Persist to fake client
				if err := c.Status().Update(ctx, rr); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
		}
		_ = createMockTransitionFailed // Suppress unused warning if not used immediately

		Context("In-Progress Phases", func() {
			// Test #10: Pending phase - no action
			It("should return no error for Pending phase", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Pending"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())
			})

			// Test #11: Investigating phase - no action
			It("should return no error for Investigating phase", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Investigating"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())
			})

			// Test #12: Analyzing phase - no action
			It("should return no error for Analyzing phase", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Analyzing"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())
			})
		})

		Context("BR-ORCH-037: WorkflowNotNeeded Handling", func() {
			// Test #13: Sets RR status to Completed with NoActionRequired
			It("should set RR status to Completed with Outcome=NoActionRequired", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Completed"
				ai.Status.Reason = "WorkflowNotNeeded"
				ai.Status.SubReason = "ProblemResolved"
				ai.Status.Message = "Issue self-resolved"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())

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
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Completed"
				ai.Status.ApprovalRequired = true
				ai.Status.ApprovalReason = "low_confidence"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())

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
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "WorkflowResolutionFailed"
				ai.Status.SubReason = "WorkflowNotFound"
				ai.Status.Message = "No matching workflow found"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())

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
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

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
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

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

			// populateManualReviewContext: RCA.Summary preferred over legacy RootCause
			It("should use RootCauseAnalysis.Summary when present (populateManualReviewContext)", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "WorkflowResolutionFailed"
				ai.Status.SubReason = "LowConfidence"
				ai.Status.RootCause = "legacy"
				ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
					Summary: "RCA Summary: OOM kill - scale deployment",
				}

				_, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))
				Expect(nrList.Items[0].Spec.Body).To(ContainSubstring("RCA Summary: OOM kill - scale deployment"))
				Expect(nrList.Items[0].Spec.Metadata).To(HaveKeyWithValue("rootCauseAnalysis", "RCA Summary: OOM kill - scale deployment"))
			})

			// populateManualReviewContext: Warnings population
			It("should populate Warnings from AIAnalysis into notification body", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "WorkflowResolutionFailed"
				ai.Status.SubReason = "LowConfidence"
				ai.Status.Message = "Confidence below threshold"
				ai.Status.Warnings = []string{"Warning A: Missing probes", "Warning B: Resource limits low"}

				_, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))
				Expect(nrList.Items[0].Spec.Body).To(ContainSubstring("**Warnings**:"))
				Expect(nrList.Items[0].Spec.Body).To(ContainSubstring("Warning A: Missing probes"))
				Expect(nrList.Items[0].Spec.Body).To(ContainSubstring("Warning B: Resource limits low"))
			})
		})

		// =====================================================
		// BR-ORCH-036 v3.0: Infrastructure Failure Escalation
		// Any failure without automatic recovery MUST be notified
		// =====================================================
		Context("BR-ORCH-036 v3.0: Infrastructure Failure Escalation", func() {
			// AC-036-30: NotificationRequest created for APIError/MaxRetriesExceeded
			It("should create escalation notification for APIError/MaxRetriesExceeded", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "APIError"
				ai.Status.SubReason = "MaxRetriesExceeded"
				ai.Status.Message = "Transient error exceeded max retries (5 attempts): HAPI request timeout"

				_, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				// Verify escalation notification was created
				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1), "Expected escalation notification for infrastructure failure")
				Expect(nrList.Items[0].Name).To(Equal("nr-manual-review-test-rr"))
				Expect(nrList.Items[0].Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
			})

			// AC-036-33: Priority is high for infrastructure failures
			It("should set high priority for infrastructure failure notifications", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "APIError"
				ai.Status.SubReason = "MaxRetriesExceeded"
				ai.Status.Message = "HAPI timeout after 5 retries"

				_, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))
				Expect(nrList.Items[0].Spec.Priority).To(Equal(notificationv1.NotificationPriorityHigh))
			})

			// AC-036-34: RR status updated with ManualReviewRequired
			It("should set RR status to Failed with Outcome=ManualReviewRequired", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "APIError"
				ai.Status.SubReason = "TransientError"
				ai.Status.Message = "Network timeout calling HAPI"

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

			// AC-036-31: NotificationRequest created for APIError/TransientError
			It("should create escalation notification for APIError/TransientError", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "APIError"
				ai.Status.SubReason = "TransientError"
				ai.Status.Message = "HAPI returned 503 Service Unavailable"

				_, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1), "Expected escalation notification for TransientError")
				Expect(nrList.Items[0].Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
			})

			// AC-036-32: NotificationRequest created for APIError/PermanentError
			It("should create escalation notification for APIError/PermanentError", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "APIError"
				ai.Status.SubReason = "PermanentError"
				ai.Status.Message = "HAPI returned 401 Unauthorized"

				_, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1), "Expected escalation notification for PermanentError")
				Expect(nrList.Items[0].Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
			})

			// Notification metadata contains reason and subReason
			It("should include reason and subReason in notification metadata", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.Reason = "APIError"
				ai.Status.SubReason = "MaxRetriesExceeded"
				ai.Status.Message = "HAPI timeout after 5 retries"

				_, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))
				Expect(nrList.Items[0].Spec.Metadata).To(HaveKeyWithValue("source", "AIAnalysis"))
				Expect(nrList.Items[0].Spec.Metadata).To(HaveKeyWithValue("reason", "APIError"))
				Expect(nrList.Items[0].Spec.Metadata).To(HaveKeyWithValue("subReason", "MaxRetriesExceeded"))
			})
		})

		Context("BR-HAPI-197: NeedsHumanReview Handling", func() {
			// UT-RO-197-001: Creates NotificationRequest when NeedsHumanReview=true
			It("should create manual review notification when NeedsHumanReview=true", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.NeedsHumanReview = true
				ai.Status.HumanReviewReason = "rca_incomplete"
				ai.Status.Message = "RCA is missing affectedResource - cannot determine target"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())

				// Verify manual review notification was created
				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))
				Expect(nrList.Items[0].Name).To(Equal("nr-manual-review-test-rr"))
				Expect(nrList.Items[0].Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
			})

			// UT-RO-197-002: NeedsHumanReview=false on normal completion - no notification
			It("should NOT create notification when NeedsHumanReview=false on normal completion", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Completed"
				ai.Status.NeedsHumanReview = false
				ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
					WorkflowID: "restart-pod-v1",
					Confidence: 0.85,
				}

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())

				// Verify NO notification was created
				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(0))
			})

			// UT-RO-197-003: NeedsHumanReview takes precedence over WorkflowResolutionFailed
			It("should handle NeedsHumanReview when BOTH NeedsHumanReview=true AND WorkflowResolutionFailed", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				// Both flags set (edge case)
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.NeedsHumanReview = true
				ai.Status.HumanReviewReason = "workflow_not_found"
				ai.Status.Reason = "WorkflowResolutionFailed"
				ai.Status.SubReason = "WorkflowNotFound"
				ai.Status.Message = "Workflow not found"

				result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())

				// Verify notification was created (BR-HAPI-197 path, not BR-ORCH-036)
				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))
				// Verify it went through NeedsHumanReview handler (not WorkflowResolutionFailed)
				Expect(nrList.Items[0].Spec.Metadata).To(HaveKeyWithValue("humanReviewReason", "workflow_not_found"))
			})

			// UT-RO-197-004: RR status updated correctly when NeedsHumanReview=true
			It("should set RR status to Failed with RequiresManualReview=true", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.NeedsHumanReview = true
				ai.Status.HumanReviewReason = "low_confidence"
				ai.Status.Message = "AI confidence below threshold"

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

			// UT-RO-197-005: All HumanReviewReason enum values map correctly
			It("should handle all 8 HumanReviewReason enum values", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				mockTransitionFailed = createMockTransitionFailed(client)
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				// Test all 8 enum values
				reasons := []string{
					"workflow_not_found",
					"image_mismatch",
					"parameter_validation_failed",
					"no_matching_workflows",
					"low_confidence",
					"llm_parsing_error",
					"investigation_inconclusive",
					"rca_incomplete",
				}

				for _, reason := range reasons {
					ai := helpers.NewCompletedAIAnalysis("test-ai-"+reason, "default")
					ai.Status.Phase = "Failed"
					ai.Status.NeedsHumanReview = true
					ai.Status.HumanReviewReason = reason
					ai.Status.Message = "Human review required: " + reason

					result, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
					Expect(err).ToNot(HaveOccurred(), "Should handle reason: "+reason)
					Expect(result.RequeueAfter).To(BeZero())

					// Verify notification was created for this reason
					nrList := &notificationv1.NotificationRequestList{}
					err = client.List(ctx, nrList)
					Expect(err).ToNot(HaveOccurred())
					Expect(nrList.Items).To(HaveLen(1), "Should create notification for reason: "+reason)

					// Clean up for next iteration
					err = client.Delete(ctx, &nrList.Items[0])
					Expect(err).ToNot(HaveOccurred())
				}
			})

			// UT-RO-197-006: Notification contains correct metadata
			It("should include HumanReviewReason in notification metadata", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				client := fakeClientBuilder.WithObjects(rr).WithStatusSubresource(rr).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
				h = handler.NewAIAnalysisHandler(client, scheme, nc, nil, mockTransitionFailed)

				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Failed"
				ai.Status.NeedsHumanReview = true
				ai.Status.HumanReviewReason = "rca_incomplete"
				ai.Status.Message = "RCA missing affectedResource"
				ai.Status.RootCause = "Pod crash loop detected"

				_, err := h.HandleAIAnalysisStatus(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				// Verify notification includes human review metadata
				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))

				// Verify metadata contains humanReviewReason
				Expect(nrList.Items[0].Spec.Metadata).To(HaveKeyWithValue("humanReviewReason", "rca_incomplete"))
				Expect(nrList.Items[0].Spec.Metadata).To(HaveKeyWithValue("rootCauseAnalysis", "Pod crash loop detected"))
			})
		})
	})
})
