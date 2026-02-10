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
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("NotificationCreator", func() {
	var (
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = notificationv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
	})

	Describe("CreateApprovalNotification", func() {
		var (
			fakeClient *fake.ClientBuilder
			nc         *creator.NotificationCreator
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			ctx = context.Background()
		})

		Context("BR-ORCH-001: Approval Notification Creation", func() {
			// Test #2: Generates deterministic name
			It("should generate deterministic name nr-approval-{rr.Name}", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("nr-approval-test-rr"))
			})
		})

		Context("BR-ORCH-031: Cascade Deletion", func() {
			// Test #3: Sets owner reference
			It("should set owner reference to RemediationRequest for cascade deletion", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				// Verify owner reference is set
				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.OwnerReferences).To(HaveLen(1))
				Expect(nr.OwnerReferences[0].Name).To(Equal(rr.Name))
				Expect(nr.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
			})
		})

		Context("BR-ORCH-001 AC-001-2: Idempotency", func() {
			// Test #4: Idempotency - returns existing name without error
			It("should be idempotent - return existing name without creating duplicate", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				// First call creates the notification
				name1, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(name1).To(Equal("nr-approval-test-rr"))

				// Second call should return same name without error
				name2, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(name2).To(Equal(name1))

				// Verify only one notification exists
				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))
			})
		})

		Context("Precondition Validation", func() {
			// Test #5: Returns error when SelectedWorkflow is nil
			It("should return error when SelectedWorkflow is nil", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.SelectedWorkflow = nil

				_, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing SelectedWorkflow"))
			})

			// Test #6: Returns error when WorkflowID is empty
			It("should return error when WorkflowID is empty", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.SelectedWorkflow.WorkflowID = ""

				_, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing WorkflowID"))
			})
		})

		// Test #7-11: Priority mapping via DescribeTable
		DescribeTable("BR-ORCH-001: Priority mapping",
			func(inputPriority string, expectedPriority notificationv1.NotificationPriority) {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				// Priority now comes from AIAnalysis.Spec.SignalContext.BusinessPriority
				// (set by AIAnalysisCreator from SP.Status, per schema update notice)
				ai.Spec.AnalysisRequest.SignalContext.BusinessPriority = inputPriority

				name, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())
				Expect(nr.Spec.Priority).To(Equal(expectedPriority))
			},
			Entry("P0 → Critical", "P0", notificationv1.NotificationPriorityCritical),
			Entry("P1 → High", "P1", notificationv1.NotificationPriorityHigh),
			Entry("P2 → Medium", "P2", notificationv1.NotificationPriorityMedium),
			Entry("P3 → Low", "P3", notificationv1.NotificationPriorityLow),
			Entry("unknown → Low", "unknown", notificationv1.NotificationPriorityLow),
		)

		// Test #12-14: Channel determination via DescribeTable
		DescribeTable("BR-ORCH-001: Channel determination",
			func(approvalReason string, expectedChannels []notificationv1.Channel) {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.ApprovalReason = approvalReason

				name, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())
				Expect(nr.Spec.Channels).To(ConsistOf(expectedChannels))
			},
			Entry("default → Slack only",
				"low_confidence",
				[]notificationv1.Channel{notificationv1.ChannelSlack}),
			Entry("high_risk_action → Slack + Email",
				"high_risk_action",
				[]notificationv1.Channel{notificationv1.ChannelSlack, notificationv1.ChannelEmail}),
		)
	})

	Describe("CreateBulkDuplicateNotification", func() {
		var (
			fakeClient *fake.ClientBuilder
			nc         *creator.NotificationCreator
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			ctx = context.Background()
		})

		Context("BR-ORCH-034: Bulk Duplicate Notification", func() {
			// Test #15: Generates deterministic name
			It("should generate deterministic name nr-bulk-{rr.Name}", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.DuplicateCount = 5

				name, err := nc.CreateBulkDuplicateNotification(ctx, rr)
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("nr-bulk-test-rr"))
			})

			// Test #16: Sets owner reference
			It("should set owner reference for cascade deletion (BR-ORCH-031)", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.DuplicateCount = 3

				name, err := nc.CreateBulkDuplicateNotification(ctx, rr)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.OwnerReferences).To(HaveLen(1))
				Expect(nr.OwnerReferences[0].Name).To(Equal(rr.Name))
			})

			// Test #17: Idempotency
			It("should be idempotent - return existing name without creating duplicate", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.DuplicateCount = 2

				// First call
				name1, err := nc.CreateBulkDuplicateNotification(ctx, rr)
				Expect(err).ToNot(HaveOccurred())

				// Second call should return same name
				name2, err := nc.CreateBulkDuplicateNotification(ctx, rr)
				Expect(err).ToNot(HaveOccurred())
				Expect(name2).To(Equal(name1))

				// Only one notification exists
				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))
			})

			// Test #18: Uses correct notification type
			It("should use NotificationTypeSimple for informational bulk notification", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.DuplicateCount = 4

				name, err := nc.CreateBulkDuplicateNotification(ctx, rr)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeSimple))
				Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityLow))
			})
		})
	})

	Describe("Label Setting", func() {
		var (
			fakeClient *fake.ClientBuilder
			nc         *creator.NotificationCreator
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			ctx = context.Background()
		})

		Context("BR-ORCH-001: Approval notification labels", func() {
			// Test #19: Sets correct labels for approval notification
			It("should set kubernaut.ai labels for routing (kubernaut.ai/severity, kubernaut.ai/notification-type)", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Spec.Severity = "high"
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/remediation-request", "test-rr"))
				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/notification-type", "approval"))
				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/severity", "high"))
				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/component", "remediation-orchestrator"))
			})
		})

		Context("BR-ORCH-034: Bulk notification labels", func() {
			// Test #20: Sets correct labels for bulk notification
			It("should set kubernaut.ai labels for bulk notification", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.DuplicateCount = 5

				name, err := nc.CreateBulkDuplicateNotification(ctx, rr)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/remediation-request", "test-rr"))
				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/notification-type", "bulk-duplicate"))
				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/component", "remediation-orchestrator"))
			})
		})
	})

	Describe("Metadata Setting", func() {
		var (
			fakeClient *fake.ClientBuilder
			nc         *creator.NotificationCreator
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			ctx = context.Background()
		})

		Context("BR-ORCH-001: Approval notification metadata", func() {
			// Test #21: Sets context metadata for approval notification
			It("should set Metadata with approval context for routing rules", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.ApprovalReason = "low_confidence"
				ai.Status.SelectedWorkflow.Confidence = 0.75
				ai.Status.SelectedWorkflow.WorkflowID = "restart-pod"

				name, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("remediationRequest", "test-rr"))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("aiAnalysis", "test-ai"))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("approvalReason", "low_confidence"))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("confidence", "0.75"))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("selectedWorkflow", "restart-pod"))
			})
		})
	})

	// =====================================================
	// BR-ORCH-036: Manual Review Notification Tests
	// =====================================================
	Describe("CreateManualReviewNotification", func() {
		var (
			fakeClient *fake.ClientBuilder
			nc         *creator.NotificationCreator
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			ctx = context.Background()
		})

		Context("BR-ORCH-036: Manual Review Notification Creation", func() {
			// Test #22: Generates deterministic name for AIAnalysis source
			It("should generate deterministic name nr-manual-review-{rr.Name}", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:    creator.ManualReviewSourceAIAnalysis,
					Reason:    "WorkflowResolutionFailed",
					SubReason: "WorkflowNotFound",
					Message:   "No matching workflow found",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("nr-manual-review-test-rr"))
			})

			// Test #23: Sets owner reference for cascade deletion
			It("should set owner reference to RemediationRequest for cascade deletion (BR-ORCH-031)", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:    creator.ManualReviewSourceAIAnalysis,
					Reason:    "WorkflowResolutionFailed",
					SubReason: "ImageMismatch",
					Message:   "Workflow image version mismatch",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.OwnerReferences).To(HaveLen(1))
				Expect(nr.OwnerReferences[0].Name).To(Equal(rr.Name))
				Expect(nr.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
			})

			// Test #24: Idempotency
			It("should be idempotent - return existing name without creating duplicate", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:    creator.ManualReviewSourceAIAnalysis,
					Reason:    "WorkflowResolutionFailed",
					SubReason: "NoMatchingWorkflows",
					Message:   "No workflows matched",
				}

				// First call creates the notification
				name1, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				// Second call should return same name without error
				name2, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())
				Expect(name2).To(Equal(name1))

				// Verify only one notification exists
				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))
			})

			// Test #25: Uses manual-review notification type
			It("should use NotificationTypeManualReview type", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:    creator.ManualReviewSourceAIAnalysis,
					Reason:    "WorkflowResolutionFailed",
					SubReason: "LowConfidence",
					Message:   "Confidence too low",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
			})
		})

		Context("BR-ORCH-036: Priority Mapping by SubReason", func() {
			// Test #26-30: Priority mapping via DescribeTable
			DescribeTable("should map SubReason to correct priority",
				func(subReason string, expectedPriority notificationv1.NotificationPriority) {
					client := fakeClient.Build()
					nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

					rr := helpers.NewRemediationRequest("test-rr", "default")
					reviewCtx := &creator.ManualReviewContext{
						Source:    creator.ManualReviewSourceAIAnalysis,
						Reason:    "WorkflowResolutionFailed",
						SubReason: subReason,
						Message:   "Test message",
					}

					name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
					Expect(err).ToNot(HaveOccurred())

					nr := &notificationv1.NotificationRequest{}
					err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
					Expect(err).ToNot(HaveOccurred())
					Expect(nr.Spec.Priority).To(Equal(expectedPriority))
				},
			// Workflow resolution failures
			Entry("WorkflowNotFound → High", "WorkflowNotFound", notificationv1.NotificationPriorityHigh),
			Entry("ImageMismatch → High", "ImageMismatch", notificationv1.NotificationPriorityHigh),
			Entry("ParameterValidationFailed → High", "ParameterValidationFailed", notificationv1.NotificationPriorityHigh),
			Entry("NoMatchingWorkflows → Medium", "NoMatchingWorkflows", notificationv1.NotificationPriorityMedium),
			Entry("LowConfidence → Medium", "LowConfidence", notificationv1.NotificationPriorityMedium),
			Entry("InvestigationInconclusive → Medium", "InvestigationInconclusive", notificationv1.NotificationPriorityMedium),
			// BR-ORCH-036 v3.0: Infrastructure failures
			Entry("AC-036-30: MaxRetriesExceeded → High", "MaxRetriesExceeded", notificationv1.NotificationPriorityHigh),
			Entry("AC-036-31: TransientError → High", "TransientError", notificationv1.NotificationPriorityHigh),
			Entry("AC-036-32: PermanentError → High", "PermanentError", notificationv1.NotificationPriorityHigh),
		)
		})

		Context("BR-ORCH-036: WorkflowExecution Source (Critical Priority)", func() {
			// Test #31: ExhaustedRetries → Critical priority
			It("should set Critical priority for ExhaustedRetries", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:       creator.ManualReviewSourceWorkflowExecution,
					Reason:       "ExhaustedRetries",
					SubReason:    "",
					Message:      "Maximum retry count reached",
					RetryCount:   3,
					MaxRetries:   3,
					LastExitCode: 1,
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityCritical))
			})

			// Test #32: PreviousExecutionFailed → Critical priority
			It("should set Critical priority for PreviousExecutionFailed", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:            creator.ManualReviewSourceWorkflowExecution,
					Reason:            "PreviousExecutionFailed",
					SubReason:         "",
					Message:           "Previous execution failed",
					PreviousExecution: "we-abc123",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityCritical))
			})
		})

		Context("BR-ORCH-036: Labels for Routing (BR-NOT-065)", func() {
			// Test #33: Sets correct labels for manual review notification
			It("should set kubernaut.ai labels including manual-review type for routing", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Spec.Severity = "critical"
				reviewCtx := &creator.ManualReviewContext{
					Source:    creator.ManualReviewSourceAIAnalysis,
					Reason:    "WorkflowResolutionFailed",
					SubReason: "WorkflowNotFound",
					Message:   "Test",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/remediation-request", "test-rr"))
				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/notification-type", "manual-review"))
				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/severity", "critical"))
				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/component", "remediation-orchestrator"))
				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/review-source", "AIAnalysis"))
			})
		})

		Context("BR-ORCH-036: Metadata for Context", func() {
			// Test #34: Sets metadata for AIAnalysis source
			It("should set Metadata with AIAnalysis context", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:            creator.ManualReviewSourceAIAnalysis,
					Reason:            "WorkflowResolutionFailed",
					SubReason:         "WorkflowNotFound",
					Message:           "No workflow found for alert type",
					RootCauseAnalysis: "Pod crash loop detected",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("remediationRequest", "test-rr"))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("source", "AIAnalysis"))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("reason", "WorkflowResolutionFailed"))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("subReason", "WorkflowNotFound"))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("rootCauseAnalysis", "Pod crash loop detected"))
			})

			// Test #35: Sets metadata for WorkflowExecution source with retry info
			It("should set Metadata with WorkflowExecution retry context", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:       creator.ManualReviewSourceWorkflowExecution,
					Reason:       "ExhaustedRetries",
					SubReason:    "",
					Message:      "Max retries exceeded",
					RetryCount:   3,
					MaxRetries:   3,
					LastExitCode: 137,
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("remediationRequest", "test-rr"))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("source", "WorkflowExecution"))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("reason", "ExhaustedRetries"))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("retryCount", "3"))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("maxRetries", "3"))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("lastExitCode", "137"))
			})
		})

		Context("BR-ORCH-036: Channel Determination", func() {
			// Test #36: Critical priority → Slack + Email
			It("should use Slack + Email for Critical priority", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:  creator.ManualReviewSourceWorkflowExecution,
					Reason:  "ExhaustedRetries",
					Message: "Critical failure",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Channels).To(ConsistOf(
					notificationv1.ChannelSlack,
					notificationv1.ChannelEmail,
				))
			})

			// Test #37: High/Medium priority → Slack only
			It("should use Slack only for High/Medium priority", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:    creator.ManualReviewSourceAIAnalysis,
					Reason:    "WorkflowResolutionFailed",
					SubReason: "LowConfidence",
					Message:   "Medium priority failure",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Channels).To(ConsistOf(notificationv1.ChannelSlack))
			})
		})
	})

	// ========================================
	// BR-ORCH-045: COMPLETION NOTIFICATION
	// ========================================
	Describe("CreateCompletionNotification", func() {
		var (
			fakeClient *fake.ClientBuilder
			nc         *creator.NotificationCreator
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			ctx = context.Background()
		})

		Context("BR-ORCH-045 AC-045-1: Successful WE completion creates NotificationRequest", func() {
			It("should create NotificationRequest with type=completion", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateCompletionNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("nr-completion-test-rr"))

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeCompletion))
			})
		})

		Context("BR-ORCH-045: Deterministic naming", func() {
			It("should generate deterministic name nr-completion-{rr.Name}", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("my-remediation", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateCompletionNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("nr-completion-my-remediation"))
			})
		})

		Context("BR-ORCH-045 AC-045-3: Idempotency", func() {
			It("should be idempotent - return existing name without creating duplicate", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				// First call creates the notification
				name1, err := nc.CreateCompletionNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				// Second call should return same name without error
				name2, err := nc.CreateCompletionNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(name2).To(Equal(name1))

				// Verify only one notification exists
				nrList := &notificationv1.NotificationRequestList{}
				err = client.List(ctx, nrList)
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1))
			})
		})

		Context("BR-ORCH-045 AC-045-4: Owner reference for cascade deletion (BR-ORCH-031)", func() {
			It("should set owner reference to RemediationRequest", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateCompletionNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.OwnerReferences).To(HaveLen(1))
				Expect(nr.OwnerReferences[0].Name).To(Equal(rr.Name))
				Expect(nr.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
			})

			It("should return error when RemediationRequest UID is empty", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.UID = "" // Clear UID
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				_, err := nc.CreateCompletionNotification(ctx, rr, ai)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("UID is required"))
			})
		})

		Context("BR-ORCH-045 AC-045-5: Channels include file and slack", func() {
			It("should include file and slack channels for completion notification", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateCompletionNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Channels).To(ConsistOf(
					notificationv1.ChannelSlack,
					notificationv1.ChannelFile,
				))
			})
		})

		Context("BR-ORCH-045 AC-045-2: Notification content", func() {
			It("should contain signal name, RCA summary, workflow ID in body", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.RootCause = "Memory exhaustion in container"

				name, err := nc.CreateCompletionNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				// Subject should contain signal name
				Expect(nr.Spec.Subject).To(ContainSubstring(rr.Spec.SignalName))

				// Body should contain RCA and workflow ID
				Expect(nr.Spec.Body).To(ContainSubstring("Memory exhaustion in container"))
				Expect(nr.Spec.Body).To(ContainSubstring(ai.Status.SelectedWorkflow.WorkflowID))
			})

			It("should include metadata with remediation context", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateCompletionNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("remediationRequest", rr.Name))
				Expect(nr.Spec.Metadata).To(HaveKeyWithValue("workflowId", ai.Status.SelectedWorkflow.WorkflowID))
			})
		})

		Context("BR-ORCH-045: Labels for routing", func() {
			It("should set kubernaut.ai labels for routing", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateCompletionNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/remediation-request", "test-rr"))
				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/notification-type", "completion"))
				Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/component", "remediation-orchestrator"))
			})
		})

		Context("BR-ORCH-045: RemediationRequestRef for audit correlation (BR-NOT-064)", func() {
			It("should set RemediationRequestRef with RR details for lineage tracking", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateCompletionNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				// BR-NOT-064: RemediationRequestRef must be set for audit correlation
				Expect(nr.Spec.RemediationRequestRef).ToNot(BeNil(),
					"RemediationRequestRef must be set for audit correlation (BR-NOT-064)")
				Expect(nr.Spec.RemediationRequestRef.Name).To(Equal(rr.Name))
				Expect(nr.Spec.RemediationRequestRef.Namespace).To(Equal(rr.Namespace))
				Expect(nr.Spec.RemediationRequestRef.UID).To(Equal(rr.UID))
				Expect(nr.Spec.RemediationRequestRef.Kind).To(Equal("RemediationRequest"))
				Expect(nr.Spec.RemediationRequestRef.APIVersion).To(Equal(remediationv1.GroupVersion.String()))
			})
		})

		Context("BR-ORCH-045: Priority mapping for completions", func() {
			It("should use low priority for successful completions (informational)", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateCompletionNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityLow))
			})
		})
	})
})
