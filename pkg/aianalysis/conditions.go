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

package aianalysis

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// Condition types for AIAnalysis
const (
	// ConditionReady indicates the AIAnalysis is ready (aggregate condition)
	ConditionReady = "Ready"

	// ConditionInvestigationComplete indicates investigation phase finished
	ConditionInvestigationComplete = "InvestigationComplete"

	// ConditionAnalysisComplete indicates analysis phase finished
	ConditionAnalysisComplete = "AnalysisComplete"

	// ConditionWorkflowResolved indicates a workflow was successfully selected
	ConditionWorkflowResolved = "WorkflowResolved"

	// ConditionApprovalRequired indicates human approval is needed
	ConditionApprovalRequired = "ApprovalRequired"

	// ConditionInvestigationSessionReady indicates the HAPI session health
	// BR-AA-HAPI-064.7: Operators can observe session health via standard K8s Conditions
	ConditionInvestigationSessionReady = "InvestigationSessionReady"
)

// Condition reasons
const (
	// ReasonReady - AIAnalysis is ready
	ReasonReady = "Ready"

	// ReasonNotReady - AIAnalysis is not ready
	ReasonNotReady = "NotReady"

	// ReasonInvestigationSucceeded - investigation completed successfully
	ReasonInvestigationSucceeded = "InvestigationSucceeded"

	// ReasonInvestigationFailed - investigation failed
	ReasonInvestigationFailed = "InvestigationFailed"

	// ReasonAnalysisSucceeded - analysis completed successfully
	ReasonAnalysisSucceeded = "AnalysisSucceeded"

	// ReasonAnalysisFailed - analysis failed
	ReasonAnalysisFailed = "AnalysisFailed"

	// ReasonWorkflowSelected - a workflow was selected
	ReasonWorkflowSelected = "WorkflowSelected"

	// ReasonNoWorkflowNeeded - problem resolved, no workflow needed
	ReasonNoWorkflowNeeded = "NoWorkflowNeeded"

	// ReasonWorkflowResolutionFailed - workflow selection failed
	ReasonWorkflowResolutionFailed = "WorkflowResolutionFailed"

	// ReasonLowConfidence - confidence below threshold
	ReasonLowConfidence = "LowConfidence"

	// ReasonPolicyRequiresApproval - Rego policy requires approval
	ReasonPolicyRequiresApproval = "PolicyRequiresApproval"

	// ========================================
	// Session-related reasons (BR-AA-HAPI-064)
	// ========================================

	// ReasonSessionCreated - HAPI session was created successfully
	ReasonSessionCreated = "SessionCreated"

	// ReasonSessionActive - HAPI session is active and being polled
	ReasonSessionActive = "SessionActive"

	// ReasonSessionLost - HAPI session was lost (404 on poll)
	ReasonSessionLost = "SessionLost"

	// ReasonSessionRegenerated - HAPI session was regenerated after loss
	ReasonSessionRegenerated = "SessionRegenerated"

	// ReasonSessionRegenerationExceeded - session regeneration cap exceeded
	ReasonSessionRegenerationExceeded = "SessionRegenerationExceeded"
)

// SetCondition sets or updates a condition on the AIAnalysis status
func SetCondition(analysis *aianalysisv1.AIAnalysis, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
		ObservedGeneration: analysis.Generation,
	}
	meta.SetStatusCondition(&analysis.Status.Conditions, condition)
}

// SetReady sets the Ready condition on the AIAnalysis
func SetReady(analysis *aianalysisv1.AIAnalysis, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}
	SetCondition(analysis, ConditionReady, status, reason, message)
}

// GetCondition returns the condition with the specified type, or nil if not found
func GetCondition(analysis *aianalysisv1.AIAnalysis, conditionType string) *metav1.Condition {
	return meta.FindStatusCondition(analysis.Status.Conditions, conditionType)
}

// SetInvestigationComplete sets the InvestigationComplete condition
func SetInvestigationComplete(analysis *aianalysisv1.AIAnalysis, succeeded bool, message string) {
	status := metav1.ConditionTrue
	reason := ReasonInvestigationSucceeded
	if !succeeded {
		status = metav1.ConditionFalse
		reason = ReasonInvestigationFailed
	}
	SetCondition(analysis, ConditionInvestigationComplete, status, reason, message)
}

// SetAnalysisComplete sets the AnalysisComplete condition
func SetAnalysisComplete(analysis *aianalysisv1.AIAnalysis, succeeded bool, message string) {
	status := metav1.ConditionTrue
	reason := ReasonAnalysisSucceeded
	if !succeeded {
		status = metav1.ConditionFalse
		reason = ReasonAnalysisFailed
	}
	SetCondition(analysis, ConditionAnalysisComplete, status, reason, message)
}

// SetWorkflowResolved sets the WorkflowResolved condition
func SetWorkflowResolved(analysis *aianalysisv1.AIAnalysis, resolved bool, reason, message string) {
	status := metav1.ConditionTrue
	if !resolved {
		status = metav1.ConditionFalse
	}
	SetCondition(analysis, ConditionWorkflowResolved, status, reason, message)
}

// SetApprovalRequired sets the ApprovalRequired condition
func SetApprovalRequired(analysis *aianalysisv1.AIAnalysis, required bool, reason, message string) {
	status := metav1.ConditionTrue
	if !required {
		status = metav1.ConditionFalse
	}
	SetCondition(analysis, ConditionApprovalRequired, status, reason, message)
}

// SetInvestigationSessionReady sets the InvestigationSessionReady condition
// BR-AA-HAPI-064.7: Operators can observe session health via standard K8s Conditions
func SetInvestigationSessionReady(analysis *aianalysisv1.AIAnalysis, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}
	SetCondition(analysis, ConditionInvestigationSessionReady, status, reason, message)
}
