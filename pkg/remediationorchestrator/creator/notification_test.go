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

package creator_test

import (
	"context"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// Characterization tests for NotificationCreator (Wave B GREEN safety net, issue #1532).
// These tests pin the behavior of the Create*Notification methods before/after the
// funlen decomposition into existingNotification/persistNotification/build*Request helpers,
// so that pure extract-method refactors have a regression detector.
var _ = Describe("NotificationCreator", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
		m         *rometrics.Metrics
		rr        *remediationv1.RemediationRequest
		ai        *aianalysisv1.AIAnalysis
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		reg := prometheus.NewRegistry()
		m = rometrics.NewMetricsWithRegistry(reg)

		rr = &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-notif-test",
				Namespace: "kubernaut-system",
				UID:       "rr-notif-uid-001",
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef12345678",
				SignalName:        "HighCPU",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "api-server",
					Namespace: "prod",
				},
				FiringTime:   metav1.Now(),
				ReceivedTime: metav1.Now(),
				ClusterID:    "prod-east-1",
			},
		}

		ai = &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ai-notif-test",
				Namespace: "kubernaut-system",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						BusinessPriority: "P1",
					},
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
						WorkflowID:      "restart-pod",
						WorkflowName:    "restart-pod",
						Version:         "1.0.0",
						ExecutionBundle: "oci://registry/workflows/restart-pod:v1.0.0",
						ActionType:      "restart",
					},
					Confidence: 0.85,
					Rationale:  "Pod restart recommended",
				},
				ApprovalReason: "High severity requires approval",
				RootCause:      "Memory leak detected",
			},
		}

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr.DeepCopy()).
			WithStatusSubresource(rr).
			Build()
	})

	Describe("CreateApprovalNotification", func() {
		It("creates an Approval NotificationRequest with owner reference and expected fields", func() {
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			name, err := nc.CreateApprovalNotification(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())
			Expect(name).To(Equal("nr-approval-rr-notif-test"))

			nr := &notificationv1.NotificationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, nr)).To(Succeed())
			Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeApproval))
			Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityHigh))
			Expect(nr.Spec.ClusterID).To(Equal("prod-east-1"))
			Expect(nr.OwnerReferences).To(HaveLen(1))
			Expect(nr.OwnerReferences[0].Name).To(Equal(rr.Name))
			Expect(nr.Spec.Context.Workflow.SelectedWorkflow).To(Equal("restart-pod"))
			// Issue #1677 Phase 1: WorkflowName/ActionType sourced directly from
			// ai.Status.SelectedWorkflow -- no live DataStorage lookup needed by Notification.
			Expect(nr.Spec.Context.Workflow.WorkflowName).To(Equal("restart-pod"))
			Expect(nr.Spec.Context.Workflow.ActionType).To(Equal("restart"))
		})

		It("is idempotent: reusing an existing approval notification returns the same name without error", func() {
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			name1, err := nc.CreateApprovalNotification(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			name2, err := nc.CreateApprovalNotification(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())
			Expect(name2).To(Equal(name1))
		})

		It("returns an error when AIAnalysis is missing SelectedWorkflow", func() {
			ai.Status.SelectedWorkflow = nil
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			_, err := nc.CreateApprovalNotification(ctx, rr, ai)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing SelectedWorkflow"))
		})

		It("returns an error when SelectedWorkflow.WorkflowID is empty", func() {
			ai.Status.SelectedWorkflow.WorkflowID = ""
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			_, err := nc.CreateApprovalNotification(ctx, rr, ai)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing WorkflowID"))
		})

		It("returns an error when RemediationRequest has an empty UID (owner reference precondition)", func() {
			rr.UID = ""
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			_, err := nc.CreateApprovalNotification(ctx, rr, ai)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("UID is required"))
		})
	})

	Describe("CreateCompletionNotification", func() {
		It("creates a Completion NotificationRequest with verification unavailable when EA is nil", func() {
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			name, err := nc.CreateCompletionNotification(ctx, rr, ai, "kubernetes-job", nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(name).To(Equal("nr-completion-rr-notif-test"))

			nr := &notificationv1.NotificationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, nr)).To(Succeed())
			Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeCompletion))
			Expect(nr.Spec.Context.Workflow.WorkflowID).To(Equal("restart-pod"))
			Expect(nr.Spec.Context.Workflow.ExecutionEngine).To(Equal("kubernetes-job"))
			// Issue #1677 Phase 1: WorkflowName/ActionType sourced directly from
			// ai.Status.SelectedWorkflow -- no live DataStorage lookup needed by Notification.
			Expect(nr.Spec.Context.Workflow.WorkflowName).To(Equal("restart-pod"))
			Expect(nr.Spec.Context.Workflow.ActionType).To(Equal("restart"))
			Expect(nr.Spec.Context.Verification.Assessed).To(BeFalse())
			Expect(nr.Spec.Context.Verification.Outcome).To(Equal("unavailable"))
		})

		It("is idempotent: reusing an existing completion notification returns the same name", func() {
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			name1, err := nc.CreateCompletionNotification(ctx, rr, ai, "kubernetes-job", nil)
			Expect(err).ToNot(HaveOccurred())

			name2, err := nc.CreateCompletionNotification(ctx, rr, ai, "kubernetes-job", nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(name2).To(Equal(name1))
		})
	})

	Describe("CreateManualReviewNotification", func() {
		It("creates a ManualReview NotificationRequest with critical priority for WorkflowExecution source", func() {
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			reviewCtx := &creator.ManualReviewContext{
				Source:     notificationv1.ReviewSourceWorkflowExecution,
				Reason:     "ExhaustedRetries",
				RetryCount: 3,
				MaxRetries: 3,
			}
			name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(name).To(Equal("nr-manual-review-rr-notif-test"))

			nr := &notificationv1.NotificationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, nr)).To(Succeed())
			Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
			Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityCritical))
			Expect(nr.Spec.Body).To(ContainSubstring("Retries attempted: 3/3"))
		})

		It("creates a ManualReview NotificationRequest with medium priority for AIAnalysis low-confidence source", func() {
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			reviewCtx := &creator.ManualReviewContext{
				Source:    notificationv1.ReviewSourceAIAnalysis,
				Reason:    "LowConfidence",
				SubReason: "LowConfidence",
			}
			name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, nr)).To(Succeed())
			Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityMedium))
		})

		It("is idempotent: reusing an existing manual review notification returns the same name", func() {
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			reviewCtx := &creator.ManualReviewContext{Source: notificationv1.ReviewSourceAIAnalysis, Reason: "NoMatchingWorkflows"}
			name1, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())

			name2, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(name2).To(Equal(name1))
		})
	})

	Describe("CreateEscalationNotification", func() {
		It("creates an Escalation NotificationRequest with High priority", func() {
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			escCtx := &creator.EscalationContext{
				FailurePhase:  "WorkflowExecution",
				FailureReason: "ExecutionTimeout",
				Message:       "workflow timed out after 30m",
			}
			name, err := nc.CreateEscalationNotification(ctx, rr, escCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(name).To(Equal("nr-escalation-rr-notif-test"))

			nr := &notificationv1.NotificationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, nr)).To(Succeed())
			Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeEscalation))
			Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityHigh))
			Expect(nr.Spec.Body).To(ContainSubstring("workflow timed out after 30m"))
		})
	})

	Describe("CreateBlockNotification", func() {
		It("maps ConsecutiveFailures to an Escalation/High notification", func() {
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			blockCtx := &creator.BlockNotificationContext{
				BlockReason:  string(remediationv1.BlockReasonConsecutiveFailures),
				BlockMessage: "3 consecutive failures detected",
			}
			name, err := nc.CreateBlockNotification(ctx, rr, blockCtx)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, nr)).To(Succeed())
			Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeEscalation))
			Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityHigh))
		})

		It("maps a transient block reason to a StatusUpdate/Low notification", func() {
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			blockCtx := &creator.BlockNotificationContext{
				BlockReason: string(remediationv1.BlockReasonDuplicateInProgress),
			}
			name, err := nc.CreateBlockNotification(ctx, rr, blockCtx)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, nr)).To(Succeed())
			Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeStatusUpdate))
			Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityLow))
		})

		It("is idempotent per block reason: reusing the same block reason returns the same name", func() {
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			blockCtx := &creator.BlockNotificationContext{BlockReason: string(remediationv1.BlockReasonResourceBusy)}
			name1, err := nc.CreateBlockNotification(ctx, rr, blockCtx)
			Expect(err).ToNot(HaveOccurred())

			name2, err := nc.CreateBlockNotification(ctx, rr, blockCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(name2).To(Equal(name1))
		})
	})

	Describe("CreateSelfResolvedNotification", func() {
		It("creates a StatusUpdate NotificationRequest with NoActionRequired outcome", func() {
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			name, err := nc.CreateSelfResolvedNotification(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())
			Expect(name).To(Equal("nr-self-resolved-rr-notif-test"))

			nr := &notificationv1.NotificationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, nr)).To(Succeed())
			Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeStatusUpdate))
			Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityLow))
			Expect(nr.Spec.Context.Analysis.Outcome).To(Equal("NoActionRequired"))
		})

		It("is idempotent: reusing an existing self-resolved notification returns the same name", func() {
			nc := creator.NewNotificationCreator(k8sClient, scheme, m)
			name1, err := nc.CreateSelfResolvedNotification(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			name2, err := nc.CreateSelfResolvedNotification(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())
			Expect(name2).To(Equal(name1))
		})
	})
})
