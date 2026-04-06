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

// Issue #453 Phase B: integration coverage for NotificationCreator typed NotificationContext
// using envtest (real K8s API server with etcd).
//
// Business requirements:
// - BR-ORCH-001, BR-ORCH-036, BR-ORCH-045, BR-ORCH-034, BR-NOT-058
package remediationorchestrator

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// uniqueFingerprint returns a unique 64-char hex SHA256 fingerprint for test isolation.
func uniqueFingerprint() string {
	h := sha256.Sum256([]byte(uuid.New().String()))
	return hex.EncodeToString(h[:])
}

var _ = Describe("Issue #453 Phase B: Notification Context Integration Tests", Label("integration", "notification-context"), func() {
	var (
		nc            *creator.NotificationCreator
		testNamespace string
	)

	BeforeEach(func() {
		testNamespace = createTestNamespace("notif-ctx")
		nc = creator.NewNotificationCreator(
			k8sClient,
			k8sManager.GetScheme(),
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		)
	})

	AfterEach(func() {
		deleteTestNamespace(testNamespace)
	})

	// newRR creates a RemediationRequest in envtest via the real K8s API.
	// The UID is assigned by the API server, enabling real owner reference validation.
	newRR := func(name string) *remediationv1.RemediationRequest {
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ROControllerNamespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: uniqueFingerprint(),
				SignalName:        "it-453b-signal",
				Severity:          "high",
				SignalType:        "alert",
				SignalSource:      "test",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Deployment", Name: "test-app", Namespace: testNamespace,
				},
				FiringTime:   now,
				ReceivedTime: now,
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
		// Re-read to get the API-server-assigned UID
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
		return rr
	}

	It("IT-NOT-453B-003 [BR-ORCH-001] should populate typed Context on approval NotificationRequest", func() {
		rr := newRR("rr-it-453b-003")
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ai-it-453b-003",
				Namespace: ROControllerNamespace,
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:          aianalysisv1.PhaseCompleted,
				ApprovalReason: "policy_requires_human_gate",
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowID:      "wf-approval-003",
					Version:         "v1",
					ExecutionBundle: "oci://test/bundle@sha256:003",
					Confidence:      0.85,
					Rationale:       "test",
				},
			},
		}

		name, err := nc.CreateApprovalNotification(ctx, rr, ai)
		Expect(err).NotTo(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, nr)).To(Succeed())

		Expect(nr.Spec.Context.Lineage.RemediationRequest).To(Equal(rr.Name))
		Expect(nr.Spec.Context.Lineage.AIAnalysis).To(Equal(ai.Name))
		Expect(nr.Spec.Context.Workflow.SelectedWorkflow).To(Equal(ai.Status.SelectedWorkflow.WorkflowID))
		Expect(nr.Spec.Context.Workflow.Confidence).To(Equal("0.85"))
		Expect(nr.Spec.Context.Analysis.ApprovalReason).To(Equal(ai.Status.ApprovalReason))
		Expect(nr.Spec.Extensions).To(BeEmpty())
		// Validate owner reference points to the real RR UID assigned by the API server
		Expect(nr.Spec.RemediationRequestRef).To(HaveField("UID", Equal(rr.UID)))
	})

	It("IT-NOT-453B-004 [BR-ORCH-036] should populate Review context for manual review (AI source)", func() {
		rr := newRR("rr-it-453b-004")
		reviewCtx := &creator.ManualReviewContext{
			Source:            notificationv1.ReviewSourceAIAnalysis,
			Reason:            "WorkflowResolutionFailed",
			SubReason:         "WorkflowNotFound",
			HumanReviewReason: "workflow_not_found",
		}

		name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
		Expect(err).NotTo(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, nr)).To(Succeed())

		Expect(nr.Spec.Context.Review.Reason).To(Equal("WorkflowResolutionFailed"))
		Expect(nr.Spec.Context.Review.SubReason).To(Equal("WorkflowNotFound"))
		Expect(nr.Spec.Context.Review.HumanReviewReason).To(Equal("workflow_not_found"))
		Expect(nr.Spec.Context.Execution).To(BeNil())
		Expect(nr.Spec.ReviewSource).To(Equal(notificationv1.ReviewSourceAIAnalysis))
	})

	It("IT-NOT-453B-005 [BR-ORCH-036] should populate Execution context for manual review (WE source)", func() {
		rr := newRR("rr-it-453b-005")
		reviewCtx := &creator.ManualReviewContext{
			Source:            notificationv1.ReviewSourceWorkflowExecution,
			Reason:            "ExhaustedRetries",
			SubReason:         "WorkflowNotFound",
			HumanReviewReason: "workflow_not_found",
			RetryCount:        3,
			MaxRetries:        5,
			LastExitCode:      1,
			PreviousExecution: "we-prev-001",
		}

		name, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
		Expect(err).NotTo(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, nr)).To(Succeed())

		Expect(nr.Spec.Context.Execution.RetryCount).To(Equal("3"))
		Expect(nr.Spec.Context.Execution.MaxRetries).To(Equal("5"))
		Expect(nr.Spec.Context.Execution.LastExitCode).To(Equal("1"))
		Expect(nr.Spec.Context.Execution.PreviousExecution).To(Equal("we-prev-001"))
	})

	It("IT-NOT-453B-006 [BR-ORCH-045] should populate typed Context on completion NotificationRequest", func() {
		rr := newRR("rr-it-453b-006")

		// Set status Outcome via status subresource (not settable at create time)
		rr.Status.Outcome = "Remediated"
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ai-it-453b-006",
				Namespace: ROControllerNamespace,
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     aianalysisv1.PhaseCompleted,
				RootCause: "OOM",
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowID:      "wf-complete-006",
					Version:         "v2",
					ExecutionBundle: "oci://test/bundle@sha256:006",
					Confidence:      0.9,
					Rationale:       "done",
					ExecutionEngine: "tekton",
				},
			},
		}

		name, err := nc.CreateCompletionNotification(ctx, rr, ai, "", nil)
		Expect(err).NotTo(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, nr)).To(Succeed())

		Expect(nr.Spec.Context.Analysis.Outcome).To(Equal(string(rr.Status.Outcome)))
		Expect(nr.Spec.Context.Workflow.WorkflowID).To(Equal(ai.Status.SelectedWorkflow.WorkflowID))
		Expect(nr.Spec.Context.Lineage.RemediationRequest).To(Equal(rr.Name))
		Expect(nr.Spec.Context.Lineage.AIAnalysis).To(Equal(ai.Name))
	})

	It("IT-NOT-453B-007 [BR-ORCH-034] should populate Lineage and Dedup on bulk duplicate NotificationRequest", func() {
		rr := newRR("rr-it-453b-007")

		// Re-fetch to get the latest ResourceVersion (controller may have reconciled)
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())

		// Set status DuplicateCount via status subresource
		rr.Status.DuplicateCount = 5
		rr.Status.OverallPhase = remediationv1.PhaseCompleted
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		name, err := nc.CreateBulkDuplicateNotification(ctx, rr)
		Expect(err).NotTo(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, nr)).To(Succeed())

		Expect(nr.Spec.Context.Lineage.RemediationRequest).To(Equal(rr.Name))
		Expect(nr.Spec.Context.Dedup.DuplicateCount).To(Equal("5"))
		Expect(nr.Spec.Context.Workflow).To(BeNil())
		Expect(nr.Spec.Context.Analysis).To(BeNil())
	})

	It("IT-NOT-453B-008 [BR-NOT-058] should flatten timeout NotificationContext maps for routing compatibility", func() {
		globalCtx := &notificationv1.NotificationContext{
			Lineage: &notificationv1.LineageContext{
				RemediationRequest: "rr-timeout-001",
			},
			Execution: &notificationv1.ExecutionContext{
				TimeoutPhase: string(remediationv1.PhaseProcessing),
			},
			Target: &notificationv1.TargetContext{
				TargetResource: "Deployment/nginx",
			},
		}
		flat := globalCtx.FlattenToMap()
		Expect(flat).To(HaveKeyWithValue("remediationRequest", "rr-timeout-001"))
		Expect(flat).To(HaveKeyWithValue("timeoutPhase", "Processing"))
		Expect(flat).To(HaveKeyWithValue("targetResource", "Deployment/nginx"))
		Expect(flat).NotTo(HaveKey("severity"))

		phaseCtx := &notificationv1.NotificationContext{
			Lineage: &notificationv1.LineageContext{
				RemediationRequest: "rr-timeout-002",
			},
			Execution: &notificationv1.ExecutionContext{
				TimeoutPhase: "Analyzing",
				PhaseTimeout: "5m0s",
			},
			Target: &notificationv1.TargetContext{
				TargetResource: "Pod/test-pod",
			},
		}
		phaseFlat := phaseCtx.FlattenToMap()
		Expect(phaseFlat).To(HaveKeyWithValue("timeoutPhase", "Analyzing"))
		Expect(phaseFlat).To(HaveKeyWithValue("phaseTimeout", "5m0s"))
		Expect(phaseFlat).To(HaveKeyWithValue("targetResource", "Pod/test-pod"))
	})
})
