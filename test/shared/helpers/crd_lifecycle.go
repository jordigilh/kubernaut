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

package helpers

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"     //nolint:revive

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// WaitForSPCreation waits for the RO controller to create a SignalProcessing CRD
// in the given namespace. Returns the first SP found.
func WaitForSPCreation(ctx context.Context, k8sClient client.Client, namespace string, timeout, interval time.Duration) *signalprocessingv1.SignalProcessing {
	var sp *signalprocessingv1.SignalProcessing
	By("Waiting for RO to create SignalProcessing CRD")
	Eventually(func() bool {
		spList := &signalprocessingv1.SignalProcessingList{}
		_ = k8sClient.List(ctx, spList, client.InNamespace(namespace))
		if len(spList.Items) == 0 {
			return false
		}
		sp = &spList.Items[0]
		return true
	}, timeout, interval).Should(BeTrue(), "SignalProcessing should be created by RO")
	return sp
}

// SimulateSPCompletion updates the given SP status to Completed with standard
// enrichment fields, simulating what the SP controller would do.
// Uses RetryOnConflict to handle races with the RO controller.
func SimulateSPCompletion(ctx context.Context, k8sClient client.Client, sp *signalprocessingv1.SignalProcessing) {
	By("Simulating SP completion (SP controller behavior)")
	Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), sp); err != nil {
			return err
		}
		sp.Status.Phase = signalprocessingv1.PhaseCompleted
		sp.Status.Severity = normalizeSeverity(sp.Spec.Signal.Severity)
		sp.Status.SignalMode = "reactive"
		sp.Status.SignalName = sp.Spec.Signal.Name
		sp.Status.EnvironmentClassification = &signalprocessingv1.EnvironmentClassification{
			Environment:  "production",
			Source:       "namespace-labels",
			ClassifiedAt: metav1.Now(),
		}
		sp.Status.PriorityAssignment = &signalprocessingv1.PriorityAssignment{
			Priority:   "P1",
			Source:     "rego-policy",
			AssignedAt: metav1.Now(),
		}
		return k8sClient.Status().Update(ctx, sp)
	})).To(Succeed())
}

// normalizeSeverity maps raw severity values (from RR/signal sources) to the
// SP CRD status enum: critical, high, medium, low, unknown.
// The SP spec.signal.severity has no enum restriction, but status.severity does.
func normalizeSeverity(raw string) string {
	switch strings.ToLower(raw) {
	case "critical", "high", "medium", "low", "unknown":
		return strings.ToLower(raw)
	case "warning":
		return "medium"
	case "info", "informational":
		return "low"
	default:
		return "unknown"
	}
}

// WaitForAICreation waits for the RO controller to create an AIAnalysis CRD
// in the given namespace. Returns the first AI found.
func WaitForAICreation(ctx context.Context, k8sClient client.Client, namespace string, timeout, interval time.Duration) *aianalysisv1.AIAnalysis {
	var ai *aianalysisv1.AIAnalysis
	By("Waiting for RO to create AIAnalysis CRD")
	Eventually(func() bool {
		aiList := &aianalysisv1.AIAnalysisList{}
		_ = k8sClient.List(ctx, aiList, client.InNamespace(namespace))
		if len(aiList.Items) == 0 {
			return false
		}
		ai = &aiList.Items[0]
		return true
	}, timeout, interval).Should(BeTrue(), "AIAnalysis should be created by RO")
	return ai
}

// AICompletionOpts configures how the AI status should be simulated.
type AICompletionOpts struct {
	ApprovalRequired bool
	ApprovalReason   string
	Confidence       float64
	TargetKind       string
	TargetName       string
	TargetNamespace  string
}

// SimulateAICompletedWithWorkflow updates AI status as completed with a workflow
// selection and AffectedResource (required for routing to WorkflowExecution).
// Uses RetryOnConflict to handle races with the RO controller.
func SimulateAICompletedWithWorkflow(ctx context.Context, k8sClient client.Client, ai *aianalysisv1.AIAnalysis, opts AICompletionOpts) {
	By("Simulating AI completion with workflow selection (AA controller behavior)")
	confidence := opts.Confidence
	if confidence == 0 {
		confidence = 0.92
	}
	targetKind := opts.TargetKind
	if targetKind == "" {
		targetKind = "Deployment"
	}
	targetName := opts.TargetName
	if targetName == "" {
		targetName = "test-app"
	}

	Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ai), ai); err != nil {
			return err
		}
		ai.Status.Phase = aianalysisv1.PhaseCompleted
		ai.Status.Reason = "AnalysisCompleted"
		ai.Status.Message = "Workflow recommended"
		ai.Status.RootCause = "Root cause identified"
		ai.Status.ApprovalRequired = opts.ApprovalRequired
		if opts.ApprovalReason != "" {
			ai.Status.ApprovalReason = opts.ApprovalReason
		}
		ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:      "restart-pod-v1",
			Version:         "1.0.0",
			ExecutionBundle: "ghcr.io/kubernaut/workflows/restart-pod:v1.0.0",
			Confidence:      confidence,
		}
		ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "Root cause identified",
			Severity:   "critical",
			SignalType: "alert",
			AffectedResource: &aianalysisv1.AffectedResource{
				Kind:      targetKind,
				Name:      targetName,
				Namespace: opts.TargetNamespace,
			},
		}
		return k8sClient.Status().Update(ctx, ai)
	})).To(Succeed())
}

// SimulateAIWorkflowNotNeeded updates AI status as completed with WorkflowNotNeeded outcome.
// Uses RetryOnConflict to handle races with the RO controller.
func SimulateAIWorkflowNotNeeded(ctx context.Context, k8sClient client.Client, ai *aianalysisv1.AIAnalysis) {
	By("Simulating AI completion with WorkflowNotNeeded (AA controller behavior)")
	Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ai), ai); err != nil {
			return err
		}
		ai.Status.Phase = aianalysisv1.PhaseCompleted
		ai.Status.Reason = "WorkflowNotNeeded"
		ai.Status.SubReason = "ProblemResolved"
		ai.Status.Message = "Problem self-resolved: transient error no longer present"
		return k8sClient.Status().Update(ctx, ai)
	})).To(Succeed())
}

// SimulateAINeedsHumanReview updates AI status as failed with NeedsHumanReview.
// Uses RetryOnConflict to handle races with the RO controller.
func SimulateAINeedsHumanReview(ctx context.Context, k8sClient client.Client, ai *aianalysisv1.AIAnalysis) {
	By("Simulating AI failure with NeedsHumanReview (AA controller behavior)")
	Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ai), ai); err != nil {
			return err
		}
		ai.Status.Phase = aianalysisv1.PhaseFailed
		ai.Status.Reason = "WorkflowResolutionFailed"
		ai.Status.NeedsHumanReview = true
		ai.Status.HumanReviewReason = "rca_incomplete"
		ai.Status.Message = "RCA analysis incomplete: missing affectedResource field in incident data"
		return k8sClient.Status().Update(ctx, ai)
	})).To(Succeed())
}

// WaitForWECreation waits for the RO controller to create a WorkflowExecution CRD
// in the given namespace. Returns the first WE found.
func WaitForWECreation(ctx context.Context, k8sClient client.Client, namespace string, timeout, interval time.Duration) *workflowexecutionv1.WorkflowExecution {
	var we *workflowexecutionv1.WorkflowExecution
	By("Waiting for RO to create WorkflowExecution CRD")
	Eventually(func() bool {
		weList := &workflowexecutionv1.WorkflowExecutionList{}
		_ = k8sClient.List(ctx, weList, client.InNamespace(namespace))
		if len(weList.Items) == 0 {
			return false
		}
		we = &weList.Items[0]
		return true
	}, timeout, interval).Should(BeTrue(), "WorkflowExecution should be created by RO")
	return we
}

// SimulateWECompletion updates the given WE status to Completed.
// Uses RetryOnConflict to handle races with the RO controller.
func SimulateWECompletion(ctx context.Context, k8sClient client.Client, we *workflowexecutionv1.WorkflowExecution) {
	By("Simulating WE completion (WE controller behavior)")
	Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(we), we); err != nil {
			return err
		}
		we.Status.Phase = workflowexecutionv1.PhaseCompleted
		return k8sClient.Status().Update(ctx, we)
	})).To(Succeed())
}

// WaitForRARCreation waits for the RO controller to create a RemediationApprovalRequest CRD
// in the given namespace. Returns the first RAR found.
func WaitForRARCreation(ctx context.Context, k8sClient client.Client, namespace string, timeout, interval time.Duration) *remediationv1.RemediationApprovalRequest {
	var rar *remediationv1.RemediationApprovalRequest
	By("Waiting for RO to create RemediationApprovalRequest CRD")
	Eventually(func() bool {
		rarList := &remediationv1.RemediationApprovalRequestList{}
		_ = k8sClient.List(ctx, rarList, client.InNamespace(namespace))
		if len(rarList.Items) == 0 {
			return false
		}
		rar = &rarList.Items[0]
		return true
	}, timeout, interval).Should(BeTrue(), "RemediationApprovalRequest should be created by RO")
	return rar
}

// WaitForRRPhase waits for a RemediationRequest to reach the specified phase.
func WaitForRRPhase(ctx context.Context, k8sClient client.Client, rr *remediationv1.RemediationRequest, phase remediationv1.RemediationPhase, timeout, interval time.Duration) {
	By("Waiting for RR to reach phase: " + string(phase))
	Eventually(func() remediationv1.RemediationPhase {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return ""
		}
		return rr.Status.OverallPhase
	}, timeout, interval).Should(Equal(phase),
		"RemediationRequest should reach phase "+string(phase))
}
