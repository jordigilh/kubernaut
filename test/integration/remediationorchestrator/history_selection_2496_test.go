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

package remediationorchestrator

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// DD-KA-2496 / BR-KA-016 / BR-AUDIT-005: the existing suite runs the real RO
// manager, admitted CRDs, ServiceAccount TokenReview/SAR and deployed DS. Child
// statuses are external controller inputs, not replacements for RO business logic.
var _ = Describe("#2496 RO selection history and no-execution safety", func() {
	DescribeTable("IT-RO-2496-002: admitted selection reaches authenticated audit and target-scoped history (AU-3, CC7.2, AC-4)",
		func(confidence float64) {
			rr, analysis := prepareHistorySelectionRO2496()
			selection := selectedHistoryRO2496(rr, confidence)
			analysis.Status.Phase = aianalysisv1.PhaseCompleted
			analysis.Status.EnsureRCAResult().SelectedWorkflow = selection
			completed := metav1.Now()
			analysis.Status.CompletedAt = &completed
			Expect(k8sClient.Status().Update(ctx, analysis)).To(Succeed())

			we := &workflowexecutionv1.WorkflowExecution{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: "we-" + rr.Name, Namespace: rr.Namespace}, we)
			}, timeout, interval).Should(Succeed(), "RO must create the execution through its production dispatch")
			Expect(we.Spec.WorkflowRef.WorkflowID).To(Equal(selection.WorkflowID))
			Expect(we.Spec.ClusterID).To(Equal(selection.ExecutionClusterID))
			Expect(we.Spec.ClusterID).NotTo(Equal(rr.Spec.ClusterID), "execution-cluster attribution must not replace the history origin")

			var event api.AuditEvent
			Eventually(func(g Gomega) {
				response, err := dsClients.OpenAPIClient.QueryAuditEvents(ctx, api.QueryAuditEventsParams{
					CorrelationID: api.NewOptString(rr.Name), EventCategory: api.NewOptString(roaudit.EventCategoryOrchestration),
					EventType: api.NewOptString(roaudit.EventTypeRemediationWorkflowCreated),
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(response.Data).To(HaveLen(1))
				event = response.Data[0]
			}, timeout, interval).Should(Succeed(), "controller audit must survive real authenticated ingestion")
			Expect(event.CorrelationID).To(Equal(rr.Name))
			Expect(event.EventID.IsSet()).To(BeTrue())
			Expect(event.EventID.Value).NotTo(Equal(uuid.Nil))
			Expect(event.ActorType.Value).To(Equal("service"))
			Expect(event.ActorID.Value).To(Equal(roaudit.ServiceName))
			Expect(event.EventAction).To(Equal(roaudit.ActionWorkflowCreated))
			Expect(string(event.EventOutcome)).To(Equal("success"))
			Expect(event.ClusterID.Value).To(Equal(rr.Spec.ClusterID))
			raw, err := json.Marshal(event.EventData)
			Expect(err).NotTo(HaveOccurred())
			var payload api.RemediationOrchestratorAuditPayload
			Expect(json.Unmarshal(raw, &payload)).To(Succeed())
			Expect(payload.WorkflowID).To(Equal(api.NewOptString(selection.WorkflowID)))
			Expect(payload.WorkflowName).To(Equal(api.NewOptString(selection.WorkflowName)))
			Expect(payload.WorkflowSelectionConfidence).To(Equal(api.NewOptFloat64(confidence)))
			Expect(payload.TargetResource.Value).To(Equal(rr.Spec.TargetResource.Namespace + "/Deployment/test-app"))
			Expect(payload.PreRemediationSpecHash.Value).NotTo(BeEmpty())
			Expect(string(raw)).NotTo(ContainSubstring("parameters"))
			Expect(string(raw)).NotTo(ContainSubstring("private-ro-2496"))

			params := api.GetRemediationHistoryContextParams{
				TargetKind: rr.Spec.TargetResource.Kind, TargetName: rr.Spec.TargetResource.Name,
				TargetNamespace: rr.Spec.TargetResource.Namespace, CurrentSpecHash: payload.PreRemediationSpecHash.Value,
			}
			history := queryHistorySelectionRO2496(params)
			Expect(history.Tier1.Chain).To(HaveLen(1))
			entry := history.Tier1.Chain[0]
			Expect(entry.RemediationUID).To(Equal(rr.Name))
			Expect(entry.WorkflowId).To(Equal(payload.WorkflowID))
			Expect(entry.WorkflowName).To(Equal(payload.WorkflowName))
			Expect(entry.WorkflowSelectionConfidence).To(Equal(api.NewOptFloat64(confidence)))
			Expect(entry.CompletedAt).To(BeTemporally("==", event.EventTimestamp), "history retains the recorded workflow-created instant, not invented execution completion")
			Expect(history.Tier2.Chain).To(BeEmpty())
			raw, err = json.Marshal(history)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(raw)).NotTo(ContainSubstring("private-ro-2496"))

			params.ClusterId = api.NewOptString(selection.ExecutionClusterID)
			history = queryHistorySelectionRO2496(params)
			Expect(history.Tier1.Chain).To(BeEmpty(), "history cannot borrow the destination execution cluster")
			Expect(history.Tier2.Chain).To(BeEmpty())
			params.ClusterId = api.OptString{}
			params.TargetName += "-other"
			history = queryHistorySelectionRO2496(params)
			Expect(history.Tier1.Chain).To(BeEmpty(), "same workflow/hash cannot leak into another target")
			Expect(history.Tier2.Chain).To(BeEmpty())
		},
		Entry("known nonzero", .84), Entry("known zero stays present", 0.),
	)

	DescribeTable("IT-RO-2496-001: rejected, declined and low-confidence repeat context stays non-executable (BR-KA-197, SI-10)",
		func(mode string) {
			rr, analysis := prepareHistorySelectionRO2496()
			analysis.Status.Phase = aianalysisv1.PhaseFailed
			analysis.Status.Reason = aianalysisv1.ReasonWorkflowResolutionFailed
			analysis.Status.EnsureReview().NeedsHumanReview = true
			analysis.Status.EnsureReview().HumanReviewReason = aianalysisv1.HumanReviewReasonRCAIncomplete
			switch mode {
			case "no-workflow":
				analysis.Status.Phase = aianalysisv1.PhaseCompleted
				analysis.Status.EnsureReview().HumanReviewReason = aianalysisv1.HumanReviewReasonNoMatchingWorkflows
			case "low-confidence":
				analysis.Status.EnsureReview().HumanReviewReason = aianalysisv1.HumanReviewReasonLowConfidence
				analysis.Status.EnsureRCAResult().SelectedWorkflow = selectedHistoryRO2496(rr, .42)
			default:
				required, matched := true, true
				analysis.Status.EnsureRCAResult().SelectionAssessment = &aianalysisv1.SelectionAssessment{
					ProposedWorkflowID: "rejected-id-2496", WorkflowName: "restart-workload",
					RepeatValidationOutcome: "rejected", RepeatJustificationRequired: &required, PriorWorkflowMatch: &matched,
					RepeatJustification: &sharedtypes.RepeatJustificationAssessment{EvidenceBasis: "New event observed"},
					HandoffDigest:       "sha256:" + strings.Repeat("b", 64), PriorRemediationUIDs: []string{"trusted-prior-2496"},
				}
			}
			Expect(k8sClient.Status().Update(ctx, analysis)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
				g.Expect(rr.Status.GetCompletionStatus().Outcome).To(Equal("ManualReviewRequired"))
				g.Expect(rr.Status.GetCompletionStatus().RequiresManualReview).To(BeTrue())
				g.Expect(rr.Status.GetCompletionStatus().NotificationRequestRefs).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())
			notification := &notificationv1.NotificationRequest{}
			ref := rr.Status.GetCompletionStatus().NotificationRequestRefs[0]
			Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, notification)).To(Succeed())
			Expect(notification.Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
			Expect(notification.Spec.Context).NotTo(BeNil())
			Expect(notification.Spec.Context.Review).NotTo(BeNil())
			Expect(notification.Spec.Context.Review.HumanReviewReason).To(Equal(analysis.Status.GetReview().HumanReviewReason))
			Consistently(func(g Gomega) {
				list := &workflowexecutionv1.WorkflowExecutionList{}
				g.Expect(k8sManager.GetAPIReader().List(ctx, list, client.InNamespace(rr.Namespace))).To(Succeed())
				for _, we := range list.Items {
					g.Expect(we.Spec.RemediationRequestRef.Name).NotTo(Equal(rr.Name))
				}
			}, 2*time.Second, interval).Should(Succeed(), "neither rejected assessment nor partial selected context authorizes execution")
			Expect(auditStore.Flush(ctx)).To(Succeed())
			response, err := dsClients.OpenAPIClient.QueryAuditEvents(ctx, api.QueryAuditEventsParams{
				CorrelationID: api.NewOptString(rr.Name), EventType: api.NewOptString(roaudit.EventTypeRemediationWorkflowCreated),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Data).To(BeEmpty(), "a review-only outcome cannot become an executable history skeleton")
		},
		Entry("rejected repeat", "rejected"), Entry("declined all candidates", "no-workflow"), Entry("below-floor repeat", "low-confidence"),
	)
})

func prepareHistorySelectionRO2496() (*remediationv1.RemediationRequest, *aianalysisv1.AIAnalysis) {
	GinkgoHelper()
	ns := createTestNamespace(ctx, "history-2496")
	DeferCleanup(deleteTestNamespace, ns)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: ns, Labels: map[string]string{"kubernaut.ai/managed": "true"}},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test-app"}},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test-app"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx:stable"}}}},
		},
	}
	Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
	rr := createRemediationRequest(ns, "rr-2496-"+uuid.NewString())
	Eventually(func() error {
		return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: "sp-" + rr.Name, Namespace: rr.Namespace}, &signalprocessingv1.SignalProcessing{})
	}, timeout, interval).Should(Succeed())
	Expect(updateSPStatus("sp-"+rr.Name, "high")).To(Succeed())
	analysis := &aianalysisv1.AIAnalysis{}
	Eventually(func() error {
		return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: "ai-" + rr.Name, Namespace: rr.Namespace}, analysis)
	}, timeout, interval).Should(Succeed())
	analysis.Status.EnsureRCAResult().RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
		Summary: "Memory pressure", Severity: "high", SignalType: "alert",
		RemediationTarget: &aianalysisv1.RemediationTarget{Kind: "Deployment", APIVersion: "apps/v1", Name: "test-app", Namespace: ns},
	}
	return rr, analysis
}

func selectedHistoryRO2496(rr *remediationv1.RemediationRequest, confidence float64) *aianalysisv1.SelectedWorkflow {
	required, matched := true, true
	return &aianalysisv1.SelectedWorkflow{
		WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
			WorkflowID: uuid.NewString(), WorkflowName: "restart-workload", ActionType: "RestartPod", Version: "1.0.0",
			ExecutionEngine: "job", ExecutionBundle: "ghcr.io/kubernaut/restart:1.0.0", ExecutionClusterID: "execution-" + rr.Name,
		},
		Confidence: confidence, WorkflowSelectionConfidence: &confidence, Rationale: "Current event and prior assessment considered",
		Parameters:                  map[string]string{"TARGET_NAMESPACE": rr.Spec.TargetResource.Namespace, "PRIVATE": "private-ro-2496"},
		RepeatJustification:         &sharedtypes.RepeatJustification{EvidenceBasis: "Current memory observation", PriorOutcomeRelevance: "Prior Full zero", ExpectedMechanism: "Restart workload"},
		RepeatJustificationRequired: &required, PriorWorkflowMatch: &matched, RepeatValidationOutcome: "accepted",
		HandoffDigest: "sha256:" + strings.Repeat("a", 64), PriorRemediationUIDs: []string{"trusted-prior-2496"},
	}
}

func queryHistorySelectionRO2496(params api.GetRemediationHistoryContextParams) *api.RemediationHistoryContext {
	GinkgoHelper()
	response, err := dsClients.OpenAPIClient.GetRemediationHistoryContext(ctx, params)
	Expect(err).NotTo(HaveOccurred())
	history, ok := response.(*api.RemediationHistoryContext)
	Expect(ok).To(BeTrue(), "authenticated read must return the complete success DTO")
	return history
}
