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

package remediationorchestrator_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/handler"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("WorkflowExecutionHandler", func() {
	var (
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = workflowexecutionv1.AddToScheme(scheme)
		_ = signalprocessingv1.AddToScheme(scheme)
		_ = notificationv1.AddToScheme(scheme)
	})

	Describe("Constructor", func() {
		// Test #1: Constructor returns non-nil
		It("should return non-nil WorkflowExecutionHandler", func() {
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			h := handler.NewWorkflowExecutionHandler(client, scheme)
			Expect(h).ToNot(BeNil())
		})
	})

	Describe("HandleSkipped", func() {
		var (
			fakeClient *fake.ClientBuilder
			h          *handler.WorkflowExecutionHandler
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			ctx = context.Background()
		})

		Context("BR-ORCH-032: ResourceBusy skip reason", func() {
			// Test #2: HandleSkipped with ResourceBusy sets OverallPhase="Skipped" and requeues
			It("should set OverallPhase=Skipped and SkipReason=ResourceBusy and requeue", func() {
				client := fakeClient.Build()
				h = handler.NewWorkflowExecutionHandler(client, scheme)

				rr := testutil.NewRemediationRequest("test-rr", "default")
				sp := testutil.NewCompletedSignalProcessing("test-sp", "default")
				we := &workflowexecutionv1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "we-test-rr",
						Namespace: "default",
					},
					Status: workflowexecutionv1.WorkflowExecutionStatus{
						Phase: "Skipped",
						SkipDetails: &workflowexecutionv1.SkipDetails{
							Reason:    "ResourceBusy",
							Message:   "Another workflow is executing on this resource",
							SkippedAt: metav1.Now(),
							ConflictingWorkflow: &workflowexecutionv1.ConflictingWorkflowRef{
								Name:           "we-parent-rr",
								WorkflowID:     "restart-pod",
								StartedAt:      metav1.Now(),
								TargetResource: "default/Pod/test-pod",
							},
						},
					},
				}

				result, err := h.HandleSkipped(ctx, rr, we, sp)
				Expect(err).ToNot(HaveOccurred())

				// Verify status update
				Expect(rr.Status.OverallPhase).To(Equal("Skipped"))
				Expect(rr.Status.SkipReason).To(Equal("ResourceBusy"))
				Expect(rr.Status.DuplicateOf).To(Equal("we-parent-rr"))

				// Verify requeue
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))
				Expect(result.RequeueAfter).To(BeNumerically("<=", 30*time.Second))
			})
		})

		Context("BR-ORCH-032: RecentlyRemediated skip reason", func() {
			// Test #3: HandleSkipped with RecentlyRemediated sets OverallPhase="Skipped" and requeues
			It("should set OverallPhase=Skipped and SkipReason=RecentlyRemediated and requeue", func() {
				client := fakeClient.Build()
				h = handler.NewWorkflowExecutionHandler(client, scheme)

				rr := testutil.NewRemediationRequest("test-rr", "default")
				sp := testutil.NewCompletedSignalProcessing("test-sp", "default")
				we := &workflowexecutionv1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "we-test-rr",
						Namespace: "default",
					},
					Status: workflowexecutionv1.WorkflowExecutionStatus{
						Phase: "Skipped",
						SkipDetails: &workflowexecutionv1.SkipDetails{
							Reason:    "RecentlyRemediated",
							Message:   "Target was remediated within cooldown period",
							SkippedAt: metav1.Now(),
							RecentRemediation: &workflowexecutionv1.RecentRemediationRef{
								Name:              "we-previous-rr",
								WorkflowID:        "restart-pod",
								CompletedAt:       metav1.Now(),
								Outcome:           "Completed",
								TargetResource:    "default/Pod/test-pod",
								CooldownRemaining: "4m30s",
							},
						},
					},
				}

				result, err := h.HandleSkipped(ctx, rr, we, sp)
				Expect(err).ToNot(HaveOccurred())

				// Verify status update
				Expect(rr.Status.OverallPhase).To(Equal("Skipped"))
				Expect(rr.Status.SkipReason).To(Equal("RecentlyRemediated"))
				Expect(rr.Status.DuplicateOf).To(Equal("we-previous-rr"))

				// Verify requeue with fixed 1 minute interval (WE owns backoff logic)
				Expect(result.RequeueAfter).To(Equal(1 * time.Minute))
			})
		})

		Context("BR-ORCH-032, BR-ORCH-036: ExhaustedRetries skip reason", func() {
			// Test #4: HandleSkipped with ExhaustedRetries sets OverallPhase="Failed" + RequiresManualReview
			It("should set OverallPhase=Failed and RequiresManualReview=true and NOT requeue", func() {
				client := fakeClient.Build()
				h = handler.NewWorkflowExecutionHandler(client, scheme)

				rr := testutil.NewRemediationRequest("test-rr", "default")
				sp := testutil.NewCompletedSignalProcessing("test-sp", "default")
				we := &workflowexecutionv1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "we-test-rr",
						Namespace: "default",
					},
					Status: workflowexecutionv1.WorkflowExecutionStatus{
						Phase: "Skipped",
						SkipDetails: &workflowexecutionv1.SkipDetails{
							Reason:    "ExhaustedRetries",
							Message:   "5+ consecutive pre-execution failures",
							SkippedAt: metav1.Now(),
						},
						ConsecutiveFailures: 5,
					},
				}

				result, err := h.HandleSkipped(ctx, rr, we, sp)
				Expect(err).ToNot(HaveOccurred())

				// Verify status - FAILED, not Skipped (per BR-ORCH-032 v1.1)
				Expect(rr.Status.OverallPhase).To(Equal("Failed"))
				Expect(rr.Status.SkipReason).To(Equal("ExhaustedRetries"))
				Expect(rr.Status.RequiresManualReview).To(BeTrue())
				Expect(rr.Status.DuplicateOf).To(BeEmpty()) // NOT a duplicate

				// NO requeue - manual intervention required
				Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
			})
		})

		Context("BR-ORCH-032, BR-ORCH-036: PreviousExecutionFailed skip reason", func() {
			// Test #5: HandleSkipped with PreviousExecutionFailed sets OverallPhase="Failed" + RequiresManualReview
			It("should set OverallPhase=Failed and RequiresManualReview=true for cluster state concerns", func() {
				client := fakeClient.Build()
				h = handler.NewWorkflowExecutionHandler(client, scheme)

				rr := testutil.NewRemediationRequest("test-rr", "default")
				sp := testutil.NewCompletedSignalProcessing("test-sp", "default")
				we := &workflowexecutionv1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "we-test-rr",
						Namespace: "default",
					},
					Status: workflowexecutionv1.WorkflowExecutionStatus{
						Phase: "Skipped",
						SkipDetails: &workflowexecutionv1.SkipDetails{
							Reason:    "PreviousExecutionFailed",
							Message:   "Previous workflow execution failed - cluster state may be inconsistent",
							SkippedAt: metav1.Now(),
						},
					},
				}

				result, err := h.HandleSkipped(ctx, rr, we, sp)
				Expect(err).ToNot(HaveOccurred())

				// Verify status - FAILED, not Skipped (per BR-ORCH-032 v1.1)
				Expect(rr.Status.OverallPhase).To(Equal("Failed"))
				Expect(rr.Status.SkipReason).To(Equal("PreviousExecutionFailed"))
				Expect(rr.Status.RequiresManualReview).To(BeTrue())
				Expect(rr.Status.DuplicateOf).To(BeEmpty()) // NOT a duplicate

				// NO requeue - manual intervention required
				Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
			})
		})

		// Tests #6-10: mapSkipReasonToSeverity mapping via DescribeTable
		DescribeTable("BR-ORCH-036: Skip reason to severity mapping",
			func(skipReason string, expectedSeverity string) {
				client := fakeClient.Build()
				h = handler.NewWorkflowExecutionHandler(client, scheme)
				severity := h.MapSkipReasonToSeverity(skipReason)
				Expect(severity).To(Equal(expectedSeverity))
			},
			Entry("PreviousExecutionFailed → critical", "PreviousExecutionFailed", "critical"),
			Entry("ExhaustedRetries → high", "ExhaustedRetries", "high"),
			Entry("ResourceBusy → medium", "ResourceBusy", "medium"),
			Entry("RecentlyRemediated → medium", "RecentlyRemediated", "medium"),
			Entry("unknown → medium", "unknown", "medium"),
		)

		// Tests #11-13: mapSkipReasonToPriority mapping via DescribeTable
		DescribeTable("BR-ORCH-036: Skip reason to priority mapping",
			func(skipReason string, expectedPriority string) {
				client := fakeClient.Build()
				h = handler.NewWorkflowExecutionHandler(client, scheme)
				priority := h.MapSkipReasonToPriority(skipReason)
				Expect(priority).To(Equal(expectedPriority))
			},
			Entry("PreviousExecutionFailed → critical", "PreviousExecutionFailed", "critical"),
			Entry("ExhaustedRetries → high", "ExhaustedRetries", "high"),
			Entry("ResourceBusy → medium", "ResourceBusy", "medium"),
		)

		Context("BR-ORCH-032, DD-WE-004: HandleFailed", func() {
			// Test #8: HandleFailed with WasExecutionFailure sets RequiresManualReview=true
			It("should set RequiresManualReview=true when WasExecutionFailure=true", func() {
				client := fakeClient.Build()
				h = handler.NewWorkflowExecutionHandler(client, scheme)

				rr := testutil.NewRemediationRequest("test-rr", "default")
				sp := testutil.NewCompletedSignalProcessing("test-sp", "default")
				we := &workflowexecutionv1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "we-test-rr",
						Namespace: "default",
					},
					Status: workflowexecutionv1.WorkflowExecutionStatus{
						Phase: "Failed",
						FailureDetails: &workflowexecutionv1.FailureDetails{
							WasExecutionFailure:            true,
							NaturalLanguageSummary:         "Workflow step 2 failed during pod restart",
							FailedTaskIndex:                2,
							FailedTaskName:                 "restart-pod",
							Reason:                         "OOMKilled",
							Message:                        "Container killed due to OOM",
							FailedAt:                       metav1.Now(),
							ExecutionTimeBeforeFailure:     "2m30s",
						},
					},
				}

				result, err := h.HandleFailed(ctx, rr, we, sp)
				Expect(err).ToNot(HaveOccurred())

				// Verify status - execution failure requires manual review
				Expect(rr.Status.OverallPhase).To(Equal("Failed"))
				Expect(rr.Status.RequiresManualReview).To(BeTrue())
				Expect(rr.Status.Message).To(Equal(we.Status.FailureDetails.NaturalLanguageSummary))

				// NO requeue - manual intervention required
				Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
			})

			// Test #9: HandleFailed without WasExecutionFailure considers recovery
			It("should consider recovery when WasExecutionFailure=false", func() {
				client := fakeClient.Build()
				h = handler.NewWorkflowExecutionHandler(client, scheme)

				rr := testutil.NewRemediationRequest("test-rr", "default")
				sp := testutil.NewCompletedSignalProcessing("test-sp", "default")
				we := &workflowexecutionv1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "we-test-rr",
						Namespace: "default",
					},
					Status: workflowexecutionv1.WorkflowExecutionStatus{
						Phase: "Failed",
						FailureDetails: &workflowexecutionv1.FailureDetails{
							WasExecutionFailure:            false, // Pre-execution failure
							NaturalLanguageSummary:         "Failed to resolve workflow from catalog",
							FailedTaskIndex:                0,
							FailedTaskName:                 "validate-workflow",
							Reason:                         "ConfigurationError",
							Message:                        "Workflow not found in catalog",
							FailedAt:                       metav1.Now(),
							ExecutionTimeBeforeFailure:     "0s",
						},
					},
				}

				result, err := h.HandleFailed(ctx, rr, we, sp)
				Expect(err).ToNot(HaveOccurred())

				// Pre-execution failures may be recoverable
				// For now, also mark as failed but may requeue for recovery
				Expect(rr.Status.OverallPhase).To(Equal("Failed"))
				// Pre-execution failures don't require manual review by default
				Expect(rr.Status.RequiresManualReview).To(BeFalse())
				Expect(result.Requeue).To(BeFalse())
			})
		})

		Context("BR-ORCH-033: trackDuplicate", func() {
			// Test #10: trackDuplicate increments DuplicateCount on parent RR
			It("should increment DuplicateCount on parent RR", func() {
				// Create parent RR that exists in the cluster
				parentRR := testutil.NewRemediationRequest("parent-rr", "default")
				parentRR.Status.OverallPhase = "executing"
				parentRR.Status.DuplicateCount = 0

				client := fakeClient.WithObjects(parentRR).WithStatusSubresource(parentRR).Build()
				h = handler.NewWorkflowExecutionHandler(client, scheme)

				// Create child RR that is a duplicate
				childRR := testutil.NewRemediationRequest("child-rr", "default")
				we := &workflowexecutionv1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "we-child-rr",
						Namespace: "default",
					},
					Status: workflowexecutionv1.WorkflowExecutionStatus{
						Phase: "Skipped",
						SkipDetails: &workflowexecutionv1.SkipDetails{
							Reason:    "ResourceBusy",
							Message:   "Another workflow executing",
							SkippedAt: metav1.Now(),
							ConflictingWorkflow: &workflowexecutionv1.ConflictingWorkflowRef{
								Name:           "we-parent-rr",
								WorkflowID:     "restart-pod",
								StartedAt:      metav1.Now(),
								TargetResource: "default/Pod/test-pod",
							},
						},
					},
				}

				// Act
				err := h.TrackDuplicate(ctx, childRR, we, "parent-rr")
				Expect(err).ToNot(HaveOccurred())

				// Verify parent RR DuplicateCount was incremented
				updatedParent := &remediationv1.RemediationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: "parent-rr", Namespace: "default"}, updatedParent)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedParent.Status.DuplicateCount).To(Equal(1))
			})

			// Test #11: trackDuplicate appends to DuplicateRefs
			It("should append to DuplicateRefs on parent RR", func() {
				// Create parent RR with existing duplicates
				parentRR := testutil.NewRemediationRequest("parent-rr", "default")
				parentRR.Status.OverallPhase = "executing"
				parentRR.Status.DuplicateCount = 1
				parentRR.Status.DuplicateRefs = []string{"existing-dup"}

				client := fakeClient.WithObjects(parentRR).WithStatusSubresource(parentRR).Build()
				h = handler.NewWorkflowExecutionHandler(client, scheme)

				// Create child RR that is a duplicate
				childRR := testutil.NewRemediationRequest("child-rr", "default")
				we := &workflowexecutionv1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "we-child-rr",
						Namespace: "default",
					},
					Status: workflowexecutionv1.WorkflowExecutionStatus{
						Phase: "Skipped",
						SkipDetails: &workflowexecutionv1.SkipDetails{
							Reason:    "ResourceBusy",
							Message:   "Another workflow executing",
							SkippedAt: metav1.Now(),
							ConflictingWorkflow: &workflowexecutionv1.ConflictingWorkflowRef{
								Name:           "we-parent-rr",
								WorkflowID:     "restart-pod",
								StartedAt:      metav1.Now(),
								TargetResource: "default/Pod/test-pod",
							},
						},
					},
				}

				// Act
				err := h.TrackDuplicate(ctx, childRR, we, "parent-rr")
				Expect(err).ToNot(HaveOccurred())

				// Verify parent RR DuplicateRefs was appended
				updatedParent := &remediationv1.RemediationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: "parent-rr", Namespace: "default"}, updatedParent)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedParent.Status.DuplicateCount).To(Equal(2))
				Expect(updatedParent.Status.DuplicateRefs).To(ContainElements("existing-dup", "child-rr"))
			})
		})

		Context("BR-ORCH-036: Manual review notification creation", func() {
			// Test #14: CreateManualReviewNotification generates correct notification
			It("should create manual review notification with correct type and severity labels", func() {
				client := fakeClient.Build()
				h = handler.NewWorkflowExecutionHandler(client, scheme)

				rr := testutil.NewRemediationRequest("test-rr", "default")
				sp := testutil.NewCompletedSignalProcessing("test-sp", "default")
				we := &workflowexecutionv1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "we-test-rr",
						Namespace: "default",
					},
					Status: workflowexecutionv1.WorkflowExecutionStatus{
						Phase: "Skipped",
						SkipDetails: &workflowexecutionv1.SkipDetails{
							Reason:    "ExhaustedRetries",
							Message:   "5+ consecutive pre-execution failures",
							SkippedAt: metav1.Now(),
						},
						ConsecutiveFailures: 5,
					},
				}

				// Act
				notificationName, err := h.CreateManualReviewNotification(ctx, rr, we, sp)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(notificationName).To(Equal("nr-manual-review-test-rr"))

				// Verify notification was created with correct fields
				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				// Verify type is ManualReview (BR-ORCH-036)
				Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))

				// Verify severity label from skip reason
				Expect(nr.Labels["kubernaut.ai/severity"]).To(Equal("high"))

				// Verify owner reference
				Expect(nr.OwnerReferences).To(HaveLen(1))
				Expect(nr.OwnerReferences[0].Name).To(Equal(rr.Name))
			})

			// Test #15: CreateManualReviewNotification idempotency
			It("should return existing notification name if already exists", func() {
				existingNR := &notificationv1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nr-manual-review-test-rr",
						Namespace: "default",
					},
					Spec: notificationv1.NotificationRequestSpec{
						Type: notificationv1.NotificationTypeManualReview,
					},
				}
				client := fakeClient.WithObjects(existingNR).Build()
				h = handler.NewWorkflowExecutionHandler(client, scheme)

				rr := testutil.NewRemediationRequest("test-rr", "default")
				sp := testutil.NewCompletedSignalProcessing("test-sp", "default")
				we := &workflowexecutionv1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "we-test-rr",
						Namespace: "default",
					},
					Status: workflowexecutionv1.WorkflowExecutionStatus{
						Phase: "Skipped",
						SkipDetails: &workflowexecutionv1.SkipDetails{
							Reason:    "ExhaustedRetries",
							Message:   "5+ consecutive pre-execution failures",
							SkippedAt: metav1.Now(),
						},
					},
				}

				// Act
				notificationName, err := h.CreateManualReviewNotification(ctx, rr, we, sp)

				// Assert - should reuse existing
				Expect(err).ToNot(HaveOccurred())
				Expect(notificationName).To(Equal("nr-manual-review-test-rr"))
			})
		})
	})
})

