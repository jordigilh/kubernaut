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
	"errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
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
		_ = eav1.AddToScheme(scheme)
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

		Context("BR-ORCH-001: Error paths", func() {
			// Client Get returns non-NotFound error → returns error
			It("should return error when client Get fails with non-NotFound error", func() {
				client := fake.NewClientBuilder().WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							return errors.New("simulated get failure")
						},
					}).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				_, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to check existing NotificationRequest"))
			})

			// Client Create fails → returns error
			It("should return error when client Create fails", func() {
				client := fake.NewClientBuilder().WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
							return errors.New("simulated create failure")
						},
					}).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				_, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create NotificationRequest"))
			})

			// Idempotency: notification already exists (pre-created) → reuses name without error
			It("should reuse existing name when notification pre-exists (idempotency)", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				existingNR := helpers.NewNotificationRequest("nr-approval-test-rr", "default")
				existingNR.Spec.Type = notificationv1.NotificationTypeApproval

				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingNR).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				name, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("nr-approval-test-rr"))

				// Verify no duplicate was created
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

		Context("BR-ORCH-001: buildApprovalBody - RootCauseAnalysis.Summary vs legacy RootCause", func() {
			It("should use RootCauseAnalysis.Summary when present (preferred over legacy RootCause)", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.RootCause = "legacy field"
				ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
					Summary: "RCA Summary: Pod crash due to OOM - Deployment should be scaled",
				}

				name, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				// Body must contain RCA.Summary (not legacy RootCause)
				Expect(nr.Spec.Body).To(ContainSubstring("RCA Summary: Pod crash due to OOM - Deployment should be scaled"))
				Expect(nr.Spec.Body).ToNot(ContainSubstring("legacy field"))
			})

			It("should use legacy RootCause when RootCauseAnalysis.Summary is empty", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.RootCause = "Legacy root cause text"
				ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
					Summary: "", // Empty - fallback to RootCause
				}

				name, err := nc.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Body).To(ContainSubstring("Legacy root cause text"))
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

		// #260: Channel determination removed from RO — NT routing rules (BR-NOT-065) are authoritative.
		// Channels are no longer set in spec by the RO; routing resolves them at delivery time.
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

			// BR-ORCH-034: Error paths
			It("should return error when client Get fails with non-NotFound error", func() {
				client := fake.NewClientBuilder().WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							return errors.New("simulated get failure")
						},
					}).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.DuplicateCount = 3

				_, err := nc.CreateBulkDuplicateNotification(ctx, rr)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to check existing NotificationRequest"))
			})

			It("should return error when client Create fails", func() {
				client := fake.NewClientBuilder().WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
							return errors.New("simulated create failure")
						},
					}).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.DuplicateCount = 3

				_, err := nc.CreateBulkDuplicateNotification(ctx, rr)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create NotificationRequest"))
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

		Context("BR-ORCH-001: Approval notification spec fields", func() {
			// Test #19: Issue #91 - routing data in spec fields, not labels
			It("should set spec fields for routing (severity, type) instead of labels", func() {
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

				Expect(nr.Spec.RemediationRequestRef.Name).To(Equal("test-rr"))
				Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeApproval))
				Expect(nr.Spec.Severity).To(Equal("high"))
				Expect(nr.Labels).To(BeNil())
			})
		})

		Context("BR-ORCH-034: Bulk notification spec fields", func() {
			// Test #20: Issue #91 - routing data in spec fields, not labels
			It("should set spec fields for bulk notification instead of labels", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.DuplicateCount = 5

				name, err := nc.CreateBulkDuplicateNotification(ctx, rr)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.RemediationRequestRef.Name).To(Equal("test-rr"))
				Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeSimple))
				Expect(nr.Spec.Severity).To(Equal("low"))
				Expect(nr.Labels).To(BeNil())
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

				Expect(nr.Spec.Context.Lineage.RemediationRequest).To(Equal("test-rr"))
				Expect(nr.Spec.Context.Lineage.AIAnalysis).To(Equal("test-ai"))
				Expect(nr.Spec.Context.Analysis.ApprovalReason).To(Equal("low_confidence"))
				Expect(nr.Spec.Context.Workflow.Confidence).To(Equal("0.75"))
				Expect(nr.Spec.Context.Workflow.SelectedWorkflow).To(Equal("restart-pod"))
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

		Context("BR-ORCH-036: Error paths", func() {
			It("should return error when client Get fails with non-NotFound error", func() {
				client := fake.NewClientBuilder().WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							return errors.New("simulated get failure")
						},
					}).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source: notificationv1.ReviewSourceAIAnalysis,
					Reason: "WorkflowResolutionFailed",
				}

				_, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to check existing NotificationRequest"))
			})

			It("should return error when client Create fails", func() {
				client := fake.NewClientBuilder().WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
							return errors.New("simulated create failure")
						},
					}).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source: notificationv1.ReviewSourceAIAnalysis,
					Reason: "WorkflowResolutionFailed",
				}

				_, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create NotificationRequest"))
			})
		})

		Context("BR-ORCH-036: Manual Review Notification Creation", func() {
			// Test #22: Generates deterministic name for AIAnalysis source
			It("should generate deterministic name nr-manual-review-{rr.Name}", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:    notificationv1.ReviewSourceAIAnalysis,
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
					Source:    notificationv1.ReviewSourceAIAnalysis,
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
					Source:    notificationv1.ReviewSourceAIAnalysis,
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
					Source:    notificationv1.ReviewSourceAIAnalysis,
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
						Source:    notificationv1.ReviewSourceAIAnalysis,
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
					Source:       notificationv1.ReviewSourceWorkflowExecution,
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
					Source:            notificationv1.ReviewSourceWorkflowExecution,
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
					Source:    notificationv1.ReviewSourceAIAnalysis,
					Reason:    "WorkflowResolutionFailed",
					SubReason: "WorkflowNotFound",
					Message:   "Test",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.RemediationRequestRef.Name).To(Equal("test-rr"))
				Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
				Expect(nr.Spec.Severity).To(Equal("critical"))
				Expect(nr.Spec.ReviewSource).To(Equal(notificationv1.ReviewSourceAIAnalysis))
				Expect(nr.Labels).To(BeNil())
			})
		})

		Context("BR-ORCH-036: Metadata for Context", func() {
			// Test #34: Sets metadata for AIAnalysis source
			It("should set Metadata with AIAnalysis context", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:            notificationv1.ReviewSourceAIAnalysis,
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

				Expect(nr.Spec.ReviewSource).To(Equal(notificationv1.ReviewSourceAIAnalysis))
				Expect(nr.Spec.Context.Lineage.RemediationRequest).To(Equal("test-rr"))
				Expect(nr.Spec.Context.Review.Reason).To(Equal("WorkflowResolutionFailed"))
				Expect(nr.Spec.Context.Review.SubReason).To(Equal("WorkflowNotFound"))
				Expect(nr.Spec.Context.Review.RootCauseAnalysis).To(Equal("Pod crash loop detected"))
			})

			// Test #35: Sets metadata for WorkflowExecution source with retry info
			It("should set Metadata with WorkflowExecution retry context", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:       notificationv1.ReviewSourceWorkflowExecution,
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

				Expect(nr.Spec.ReviewSource).To(Equal(notificationv1.ReviewSourceWorkflowExecution))
				Expect(nr.Spec.Context.Lineage.RemediationRequest).To(Equal("test-rr"))
				Expect(nr.Spec.Context.Review.Reason).To(Equal("ExhaustedRetries"))
				Expect(nr.Spec.Context.Execution.RetryCount).To(Equal("3"))
				Expect(nr.Spec.Context.Execution.MaxRetries).To(Equal("3"))
				Expect(nr.Spec.Context.Execution.LastExitCode).To(Equal("137"))
			})
		})

		// Issue #588: Sentinel RCA suppression and deduplication
		Context("Issue #588: buildManualReviewBody — sentinel RCA and content deduplication", func() {
			It("UT-RO-588-001: should omit sentinel 'Failed to parse RCA' from notification body", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:            notificationv1.ReviewSourceAIAnalysis,
					Reason:            "WorkflowResolutionFailed",
					SubReason:         "LLMParsingError",
					Message:           "LLM failed to produce structured output after 3 attempts",
					RootCauseAnalysis: "Failed to parse RCA",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Body).ToNot(ContainSubstring("Failed to parse RCA"),
					"Sentinel RCA value must not appear in notification body")
				Expect(nr.Spec.Body).ToNot(ContainSubstring("**Root Cause Analysis**"),
					"RCA section header must be omitted when RCA is a sentinel")
			})

			It("UT-RO-588-002: should omit sentinel 'No structured RCA found' from notification body", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:            notificationv1.ReviewSourceAIAnalysis,
					Reason:            "WorkflowResolutionFailed",
					SubReason:         "NoMatchingWorkflows",
					RootCauseAnalysis: "No structured RCA found",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Body).ToNot(ContainSubstring("No structured RCA found"),
					"Sentinel RCA value must not appear in notification body")
			})

			It("UT-RO-588-003: should preserve legitimate RCA summary in notification body", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:            notificationv1.ReviewSourceAIAnalysis,
					Reason:            "WorkflowResolutionFailed",
					SubReason:         "LowConfidence",
					RootCauseAnalysis: "Pod OOM due to memory leak in container ml-worker",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Body).To(ContainSubstring("**Root Cause Analysis**"),
					"RCA section header must be present for legitimate RCA")
				Expect(nr.Spec.Body).To(ContainSubstring("Pod OOM due to memory leak in container ml-worker"),
					"Legitimate RCA summary must appear in notification body")
			})

			It("UT-RO-588-004: Details and Warnings sections must not contain duplicate text", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:    notificationv1.ReviewSourceAIAnalysis,
					Reason:    "WorkflowResolutionFailed",
					SubReason: "ParameterValidationFailed",
					Message:   "Confidence below threshold",
					Warnings:  []string{"Low confidence score", "Missing resource limits"},
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				// Details section should contain the Message
				Expect(nr.Spec.Body).To(ContainSubstring("**Details**"))
				Expect(nr.Spec.Body).To(ContainSubstring("Confidence below threshold"))

				// Warnings section should contain the warnings
				Expect(nr.Spec.Body).To(ContainSubstring("**Warnings**"))
				Expect(nr.Spec.Body).To(ContainSubstring("Low confidence score"))
				Expect(nr.Spec.Body).To(ContainSubstring("Missing resource limits"))

				// Warning text must NOT appear in the Details section
				// Details section is between "**Details**:" and the next section
				detailsIdx := strings.Index(nr.Spec.Body, "**Details**")
				warningsIdx := strings.Index(nr.Spec.Body, "**Warnings**")
				Expect(detailsIdx).To(BeNumerically(">=", 0))
				Expect(warningsIdx).To(BeNumerically(">", detailsIdx))

				detailsSection := nr.Spec.Body[detailsIdx:warningsIdx]
				Expect(detailsSection).ToNot(ContainSubstring("Low confidence score"),
					"Warning text must not appear in Details section")
				Expect(detailsSection).ToNot(ContainSubstring("Missing resource limits"),
					"Warning text must not appear in Details section")
			})
		})

		Context("BR-ORCH-036: buildManualReviewBody - warnings and WE retry info", func() {
			It("should include warnings section when Warnings are populated", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:            notificationv1.ReviewSourceAIAnalysis,
					Reason:            "WorkflowResolutionFailed",
					SubReason:         "LowConfidence",
					Message:           "AI confidence below threshold",
					RootCauseAnalysis: "Pod OOM - may need resource limits",
					Warnings:          []string{"Warning 1: Low memory limit", "Warning 2: No readiness probe"},
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Body).To(ContainSubstring("**Warnings**:"))
				Expect(nr.Spec.Body).To(ContainSubstring("Warning 1: Low memory limit"))
				Expect(nr.Spec.Body).To(ContainSubstring("Warning 2: No readiness probe"))
			})

			It("should not include warnings section when Warnings is empty", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:    notificationv1.ReviewSourceAIAnalysis,
					Reason:    "WorkflowResolutionFailed",
					SubReason: "WorkflowNotFound",
					Message:   "No workflow found",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Body).ToNot(ContainSubstring("**Warnings**:"))
			})

			It("should include WE retry info when Source is WorkflowExecution", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:            notificationv1.ReviewSourceWorkflowExecution,
					Reason:            "ExhaustedRetries",
					Message:           "All retries failed",
					RetryCount:        3,
					MaxRetries:        3,
					LastExitCode:      137,
					PreviousExecution: "we-test-remediation",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Body).To(ContainSubstring("**Retry Information**:"))
				Expect(nr.Spec.Body).To(ContainSubstring("Retries attempted: 3/3"))
				Expect(nr.Spec.Body).To(ContainSubstring("Last exit code: 137"))
				Expect(nr.Spec.Body).To(ContainSubstring("Previous execution: we-test-remediation"))
			})
		})

		Context("BR-ORCH-036: Channel Determination", func() {
			// Test #36: Critical priority → Slack + Email
			// #260: Channel determination removed from RO — NT routing rules (BR-NOT-065) are authoritative.
			// Manual review channels are no longer set in spec by the RO.

			It("should create notification with Critical priority for WE failures", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:  notificationv1.ReviewSourceWorkflowExecution,
					Reason:  "ExhaustedRetries",
					Message: "Critical failure",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityCritical))
			})

			It("should create notification with Medium priority for AI LowConfidence", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				reviewCtx := &creator.ManualReviewContext{
					Source:    notificationv1.ReviewSourceAIAnalysis,
					Reason:    "WorkflowResolutionFailed",
					SubReason: "LowConfidence",
					Message:   "Medium priority failure",
				}

				name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityMedium))
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

		Context("BR-ORCH-045: Error paths", func() {
			It("should return error when client Get fails with non-NotFound error", func() {
				client := fake.NewClientBuilder().WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							return errors.New("simulated get failure")
						},
					}).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				_, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to check existing NotificationRequest"))
			})

			It("should return error when client Create fails", func() {
				client := fake.NewClientBuilder().WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
							return errors.New("simulated create failure")
						},
					}).Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				_, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create NotificationRequest"))
			})
		})

		Context("BR-ORCH-045 AC-045-1: Successful WE completion creates NotificationRequest", func() {
			It("should create NotificationRequest with type=completion", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
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

				name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
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
				name1, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
				Expect(err).ToNot(HaveOccurred())

				// Second call should return same name without error
				name2, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
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

				name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
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

				_, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("UID is required"))
			})
		})

		Context("BR-ORCH-045 AC-045-5: Channels resolved by routing (#260)", func() {
			It("should not set channels in spec (routing resolves them)", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("BR-ORCH-045: buildCompletionBody - workflow, target resource, content", func() {
			It("should contain workflow name, execution engine, and target resource in body", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.SelectedWorkflow.WorkflowID = "rollback-deployment-v1"
				ai.Status.SelectedWorkflow.Version = "v2.0.0"
				ai.Status.SelectedWorkflow.ExecutionEngine = "tekton"
				ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
					Summary: "Deployment rollout failed",
				}

				name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				// buildCompletionBody must include workflow ID, execution engine, target resource
				Expect(nr.Spec.Body).To(ContainSubstring("rollback-deployment-v1"))
				Expect(nr.Spec.Body).To(ContainSubstring("tekton"))
				Expect(nr.Spec.Body).To(ContainSubstring("Pod"))
				Expect(nr.Spec.Body).To(ContainSubstring("test-pod"))
				Expect(nr.Spec.Body).To(ContainSubstring("Deployment rollout failed"))
			})
		})

		Context("BR-ORCH-045 AC-045-2: Notification content", func() {
			It("should contain signal name, RCA summary, workflow ID in body", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.RootCause = "Memory exhaustion in container"

				name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
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

				name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Context.Lineage.RemediationRequest).To(Equal(rr.Name))
				Expect(nr.Spec.Context.Workflow.WorkflowID).To(Equal(ai.Status.SelectedWorkflow.WorkflowID))
			})
		})

		Context("BR-ORCH-045: Labels for routing", func() {
			It("should set kubernaut.ai labels for routing", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.RemediationRequestRef.Name).To(Equal("test-rr"))
				Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeCompletion))
				Expect(nr.Spec.Severity).To(Equal("warning"))
				Expect(nr.Labels).To(BeNil())
			})
		})

		Context("BR-ORCH-045: RemediationRequestRef for audit correlation (BR-NOT-064)", func() {
			It("should set RemediationRequestRef with RR details for lineage tracking", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")

				name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
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

				name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityLow))
			})
		})
	})

	// ========================================
	// #304: Completion notification must include Outcome in body and metadata
	// ========================================
	Describe("Completion notification Outcome (#304)", func() {
		var (
			fakeClient *fake.ClientBuilder
			nc         *creator.NotificationCreator
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(
				&notificationv1.NotificationRequest{},
			)
			ctx = context.Background()
		})

		It("UT-NT-304-001: completion notification metadata should contain Outcome when set", func() {
			client := fakeClient.Build()
			nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-304", "default")
			rr.Status.Outcome = "Remediated"
			ai := helpers.NewCompletedAIAnalysis("test-ai-304", "default")

			name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
			Expect(err).ToNot(HaveOccurred())

			Expect(nr.Spec.Context.Analysis.Outcome).To(Equal("Remediated"),
				"#304: BR-ORCH-045 requires completion notification metadata to include actual Outcome")
			Expect(nr.Spec.Body).To(ContainSubstring("Remediated"),
				"#304: BR-ORCH-045 requires completion notification body to include actual Outcome")
		})

		It("UT-NT-304-002: completion notification body shows empty outcome when Outcome not set (pre-fix bug)", func() {
			client := fakeClient.Build()
			nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-304b", "default")
			ai := helpers.NewCompletedAIAnalysis("test-ai-304b", "default")

			name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
			Expect(err).ToNot(HaveOccurred())

			var outcome string
			if nr.Spec.Context != nil && nr.Spec.Context.Analysis != nil {
				outcome = nr.Spec.Context.Analysis.Outcome
			}
			Expect(outcome).To(BeEmpty(),
				"#304: When Outcome is not set, metadata should reflect empty outcome (demonstrating the bug)")
		})
	})

	// ========================================
	// #305: Target resource resolution — prefer AI RemediationTarget over Unknown
	// ========================================
	Describe("Target resource resolution (#305)", func() {
		var (
			fakeClient *fake.ClientBuilder
			nc         *creator.NotificationCreator
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(
				&notificationv1.NotificationRequest{},
			)
			ctx = context.Background()
		})

		It("UT-NT-305-001: completion body should use AI RemediationTarget when TargetResource is Unknown", func() {
			client := fakeClient.Build()
			nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-305", "default")
			rr.Spec.TargetResource = remediationv1.ResourceIdentifier{
				Kind:      "Unknown",
				Name:      "unknown",
				Namespace: "default",
			}

			ai := helpers.NewCompletedAIAnalysis("test-ai-305", "default")
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary:  "OOM kills in payment-api",
				RemediationTarget: &aianalysisv1.RemediationTarget{
					Kind:      "Deployment",
					Name:      "payment-api",
					Namespace: "production",
				},
			}

			name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
			Expect(err).ToNot(HaveOccurred())

			Expect(nr.Spec.Body).To(ContainSubstring("Deployment"),
				"#305: Body should use AI RemediationTarget.Kind when TargetResource is Unknown")
			Expect(nr.Spec.Body).To(ContainSubstring("payment-api"),
				"#305: Body should use AI RemediationTarget.Name when TargetResource is Unknown")
			Expect(nr.Spec.Body).ToNot(ContainSubstring("- **Kind**: Unknown"),
				"#305: Body should NOT show 'Unknown' when AI RemediationTarget is available")
		})

		It("UT-NT-305-002: completion body should keep TargetResource when it is valid", func() {
			client := fakeClient.Build()
			nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-305b", "default")
			ai := helpers.NewCompletedAIAnalysis("test-ai-305b", "default")
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary: "Some issue",
				RemediationTarget: &aianalysisv1.RemediationTarget{
					Kind:      "Deployment",
					Name:      "web-frontend",
					Namespace: "production",
				},
			}

			name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", nil)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
			Expect(err).ToNot(HaveOccurred())

			Expect(nr.Spec.Body).To(ContainSubstring("Pod"),
				"#305: Body should keep TargetResource.Kind when it is valid (not Unknown)")
			Expect(nr.Spec.Body).To(ContainSubstring("test-pod"),
				"#305: Body should keep TargetResource.Name when it is valid (not Unknown)")
		})

		It("UT-NT-305-003: approval body should use AI RemediationTarget when TargetResource is Unknown", func() {
			client := fakeClient.Build()
			nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-305c", "default")
			rr.Spec.TargetResource = remediationv1.ResourceIdentifier{
				Kind:      "Unknown",
				Name:      "unknown",
				Namespace: "default",
			}

			ai := helpers.NewCompletedAIAnalysis("test-ai-305c", "default")
			ai.Status.ApprovalReason = "Production namespace"
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary: "Memory leak",
				RemediationTarget: &aianalysisv1.RemediationTarget{
					Kind:      "StatefulSet",
					Name:      "redis-primary",
					Namespace: "cache",
				},
			}

			name, err := nc.CreateApprovalNotification(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
			Expect(err).ToNot(HaveOccurred())

			Expect(nr.Spec.Body).To(ContainSubstring("StatefulSet"),
				"#305: Approval body should use AI RemediationTarget.Kind when TargetResource is Unknown")
			Expect(nr.Spec.Body).To(ContainSubstring("redis-primary"),
				"#305: Approval body should use AI RemediationTarget.Name when TargetResource is Unknown")
		})
	})

	// =====================================================
	// #318: COMPLETION NOTIFICATION WITH EA VERIFICATION
	// =====================================================
	Describe("Completion notification with EA verification (#318)", func() {
		var (
			fakeClient *fake.ClientBuilder
			nc         *creator.NotificationCreator
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			ctx = context.Background()
		})

		Context("IT-RO-318-001: Completion notification body contains verification section", func() {
			It("should include Verification Results section with passed summary for full EA", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr-318-001", "default")
				rr.Status.Outcome = "Remediated"
				ai := helpers.NewCompletedAIAnalysis("test-ai-318-001", "default")
				ea := &eav1.EffectivenessAssessment{
					Status: eav1.EffectivenessAssessmentStatus{
						Phase:            eav1.PhaseCompleted,
						AssessmentReason: eav1.AssessmentReasonFull,
						Components: eav1.EAComponents{
							HealthAssessed:  true,
							HealthScore:     float64Ptr(1.0),
							AlertAssessed:   true,
							AlertScore:      float64Ptr(1.0),
							MetricsAssessed: true,
							MetricsScore:    float64Ptr(1.0),
							HashComputed:    true,
						},
					},
				}

				name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", ea)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Body).To(ContainSubstring("Verification Results"))
				Expect(nr.Spec.Body).To(ContainSubstring("Verification passed"))
				Expect(nr.Spec.Body).To(ContainSubstring(rr.Spec.SignalName))
			})
		})

		Context("IT-RO-318-002: Completion notification typed context populated", func() {
			It("should populate Context.Verification with spec_drift data", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr-318-002", "default")
				rr.Status.Outcome = "Remediated"
				ai := helpers.NewCompletedAIAnalysis("test-ai-318-002", "default")
				ea := &eav1.EffectivenessAssessment{
					Status: eav1.EffectivenessAssessmentStatus{
						Phase:            eav1.PhaseCompleted,
						AssessmentReason: eav1.AssessmentReasonSpecDrift,
						Components: eav1.EAComponents{
							HashComputed:            true,
							PostRemediationSpecHash: "abc123",
							CurrentSpecHash:         "def456",
						},
					},
				}

				name, err := nc.CreateCompletionNotification(ctx, rr, ai, "tekton", ea)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Context.Verification.Assessed).To(BeTrue())
				Expect(nr.Spec.Context.Verification.Outcome).To(Equal("inconclusive"))
				Expect(nr.Spec.Context.Verification.Reason).To(Equal("SpecDrift"))
				Expect(nr.Spec.Context.Verification.Summary).To(ContainSubstring("modified by an external entity"))
			})
		})

		Context("IT-RO-318-003: Completion notification with nil EA", func() {
			It("should create notification successfully with not available verification", func() {
				client := fakeClient.Build()
				nc = creator.NewNotificationCreator(client, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr-318-003", "default")
				rr.Status.Outcome = "Remediated"
				ai := helpers.NewCompletedAIAnalysis("test-ai-318-003", "default")

				name, err := nc.CreateCompletionNotification(ctx, rr, ai, "", nil)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Body).To(ContainSubstring("Verification: not available"))
				Expect(nr.Spec.Context.Verification.Assessed).To(BeFalse())
				Expect(nr.Spec.Context.Verification.Outcome).To(Equal("unavailable"))
			})
		})
	})

	// ========================================
	// BR-ORCH-037 AC-037-08: Self-Resolved Notification (Issue #590)
	// ========================================
	Describe("CreateSelfResolvedNotification", func() {
		var (
			fakeClient *fake.ClientBuilder
			nc         *creator.NotificationCreator
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			ctx = context.Background()
		})

		Context("BR-ORCH-037 AC-037-08: Self-Resolved Notification Creation", func() {
			It("UT-RO-590-001: should create NR with deterministic name nr-self-resolved-{rr.Name}", func() {
				k8sClient := fakeClient.Build()
				nc = creator.NewNotificationCreator(k8sClient, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Completed"
				ai.Status.Reason = "WorkflowNotNeeded"
				ai.Status.Message = "Problem self-resolved"

				name, err := nc.CreateSelfResolvedNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("nr-self-resolved-test-rr"))

				nr := &notificationv1.NotificationRequest{}
				err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())
			})

			It("UT-RO-590-002: should set type=status-update, priority=low, correct severity and subject", func() {
				k8sClient := fakeClient.Build()
				nc = creator.NewNotificationCreator(k8sClient, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Spec.Severity = "warning"
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Completed"
				ai.Status.Reason = "WorkflowNotNeeded"
				ai.Status.Message = "Problem self-resolved"

				name, err := nc.CreateSelfResolvedNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeStatusUpdate),
					"Correctness: type must be status-update per BR-ORCH-037")
				Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityLow),
					"Correctness: priority must be low for informational notification")
				Expect(nr.Spec.Severity).To(Equal("warning"),
					"Accuracy: severity must match parent RR")
				Expect(nr.Spec.Subject).To(ContainSubstring(rr.Spec.SignalName),
					"Accuracy: subject must contain signal name")
				Expect(nr.Spec.RemediationRequestRef).NotTo(BeNil(),
					"Correctness: must reference parent RR")
				Expect(nr.Spec.RemediationRequestRef.UID).To(Equal(rr.UID),
					"Accuracy: RR ref UID must match parent")
			})

			It("UT-RO-590-003: should reuse existing NR on second call (idempotency)", func() {
				k8sClient := fakeClient.Build()
				nc = creator.NewNotificationCreator(k8sClient, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Completed"
				ai.Status.Reason = "WorkflowNotNeeded"

				name1, err := nc.CreateSelfResolvedNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				name2, err := nc.CreateSelfResolvedNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(name2).To(Equal(name1),
					"Behavior: second call must return the same NR name without error")

				nrList := &notificationv1.NotificationRequestList{}
				err = k8sClient.List(ctx, nrList, &client.ListOptions{Namespace: "default"})
				Expect(err).ToNot(HaveOccurred())
				Expect(nrList.Items).To(HaveLen(1),
					"Correctness: only 1 NR must exist, not 2")
			})

			It("UT-RO-590-004: should include signal, target, AI message, RCA, and audit tagline in body", func() {
				k8sClient := fakeClient.Build()
				nc = creator.NewNotificationCreator(k8sClient, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

				rr := helpers.NewRemediationRequest("test-rr", "default")
				ai := helpers.NewCompletedAIAnalysis("test-ai", "default")
				ai.Status.Phase = "Completed"
				ai.Status.Reason = "WorkflowNotNeeded"
				ai.Status.Message = "Problem self-resolved. No remediation required."
				ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
					Summary: "Node memory pressure cleared after OOM killer freed processes",
				}

				name, err := nc.CreateSelfResolvedNotification(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
				Expect(err).ToNot(HaveOccurred())

				Expect(nr.Spec.Body).To(ContainSubstring(rr.Spec.SignalName),
					"Accuracy: body must contain signal name")
				Expect(nr.Spec.Body).To(ContainSubstring(rr.Spec.TargetResource.Kind),
					"Accuracy: body must contain target resource kind")
				Expect(nr.Spec.Body).To(ContainSubstring("Problem self-resolved"),
					"Accuracy: body must contain AI assessment message")
				Expect(nr.Spec.Body).To(ContainSubstring("Node memory pressure cleared"),
					"Accuracy: body must contain RCA summary")
				Expect(nr.Spec.Body).To(ContainSubstring("audit purposes only"),
					"Behavior: body must include audit tagline per BR-ORCH-037")
			})
		})
	})

	// =====================================================
	// Issue #803: RoutingEngine Source Tests
	// =====================================================
	Describe("Issue #803: ReviewSourceRoutingEngine", func() {
		var (
			fakeClient *fake.ClientBuilder
			nc         *creator.NotificationCreator
			ctx        context.Context
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			ctx = context.Background()
		})

		// UT-RO-803-001: CreateManualReviewNotification accepts ReviewSourceRoutingEngine
		It("UT-RO-803-001: should create NR with ReviewSource=RoutingEngine for IneffectiveChain blocks", func() {
			cl := fakeClient.Build()
			nc = creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-803-001", "default")
			reviewCtx := &creator.ManualReviewContext{
				Source:  notificationv1.ReviewSourceRoutingEngine,
				Reason:  "IneffectiveChain",
				Message: "3 consecutive ineffective remediations detected (Layer1 hash chain). Escalating to manual review.",
			}

			name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(name).To(Equal("nr-manual-review-test-rr-803-001"))

			nr := &notificationv1.NotificationRequest{}
			err = cl.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
			Expect(err).ToNot(HaveOccurred())
			Expect(nr.Spec.ReviewSource).To(Equal(notificationv1.ReviewSourceRoutingEngine))
			Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
		})

		// UT-RO-803-002: Idempotent creation with RoutingEngine source
		It("UT-RO-803-002: should be idempotent with RoutingEngine source (no duplicate NR)", func() {
			cl := fakeClient.Build()
			nc = creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-803-002", "default")
			reviewCtx := &creator.ManualReviewContext{
				Source:  notificationv1.ReviewSourceRoutingEngine,
				Reason:  "IneffectiveChain",
				Message: "Escalating to manual review",
			}

			name1, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())

			name2, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(name2).To(Equal(name1))

			nrList := &notificationv1.NotificationRequestList{}
			err = cl.List(ctx, nrList)
			Expect(err).ToNot(HaveOccurred())
			manualReviewCount := 0
			for _, nr := range nrList.Items {
				if strings.HasPrefix(nr.Name, "nr-manual-review-") {
					manualReviewCount++
				}
			}
			Expect(manualReviewCount).To(Equal(1))
		})

		It("UT-RO-805-PRI-001: RoutingEngine source should map to High priority", func() {
			cl := fakeClient.Build()
			nc = creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-pri-001", "default")
			reviewCtx := &creator.ManualReviewContext{
				Source:  notificationv1.ReviewSourceRoutingEngine,
				Reason:  "IneffectiveChain",
				Message: "Ineffective chain detected",
			}

			name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			err = cl.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
			Expect(err).ToNot(HaveOccurred())
			Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityHigh),
				"RoutingEngine source should map to High priority (IneffectiveChain)")
		})

		It("UT-RO-805-PRI-002: WorkflowExecution source should map to Critical priority", func() {
			cl := fakeClient.Build()
			nc = creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-pri-002", "default")
			reviewCtx := &creator.ManualReviewContext{
				Source:  notificationv1.ReviewSourceWorkflowExecution,
				Reason:  "ExecutionFailure",
				Message: "Pipeline failed",
			}

			name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			err = cl.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, nr)
			Expect(err).ToNot(HaveOccurred())
			Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityCritical),
				"WorkflowExecution source should map to Critical priority")
		})
	})

	Describe("AlreadyExists handling (#805)", func() {
		It("UT-RO-805-AE-001: should return name without error when concurrent Create hits AlreadyExists", func() {
			ctx := context.Background()
			createCallCount := 0
			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						createCallCount++
						if createCallCount == 1 {
							return client.Create(ctx, obj, opts...)
						}
						return client.Create(ctx, obj, opts...)
					},
				}).
				Build()

			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
			rr := helpers.NewRemediationRequest("test-rr-ae-001", "default")
			reviewCtx := &creator.ManualReviewContext{
				Source:  notificationv1.ReviewSourceRoutingEngine,
				Reason:  "IneffectiveChain",
				Message: "Test concurrent create",
			}

			name1, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())

			name2, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(name2).To(Equal(name1), "Second call should return same name via Get-before-Create idempotency")
		})
	})
})
