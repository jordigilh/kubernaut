package workflowexecution_test

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

func parentForWorkflowExecution(wfe *workflowexecutionv1alpha1.WorkflowExecution) *remediationv1.RemediationRequest {
	return &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wfe.Spec.RemediationRequestRef.Name,
			Namespace: wfe.Spec.RemediationRequestRef.Namespace,
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SignalName:        "workflowexecution-unit-parent",
			Severity:          "critical",
			SignalType:        "alert",
			TargetType:        "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind: "Deployment", Name: "workflowexecution-unit-target", Namespace: wfe.Namespace,
			},
			FiringTime:   metav1.Now(),
			ReceivedTime: metav1.Now(),
		},
		Status: remediationv1.RemediationRequestStatus{
			OverallPhase: remediationv1.PhaseExecuting,
		},
	}
}
