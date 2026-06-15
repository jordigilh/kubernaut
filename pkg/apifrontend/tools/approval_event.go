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
package tools

import (
	"encoding/json"
	"time"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// ApprovalRequestEventPayload is the structured payload emitted as a
// TaskStatusUpdateEvent with metadata.type="approval_request" when
// kubernaut_watch detects AwaitingApproval. The console uses this to
// render a rich approval card with Approve/Decline buttons.
type ApprovalRequestEventPayload struct {
	Name                   string                       `json:"name"`
	Namespace              string                       `json:"namespace"`
	RemediationRequestName string                       `json:"remediationRequestName"`
	Confidence             float64                      `json:"confidence"`
	ConfidenceLevel        string                       `json:"confidenceLevel"`
	Reason                 string                       `json:"reason"`
	WhyApprovalRequired    string                       `json:"whyApprovalRequired"`
	RecommendedWorkflow    *ApprovalWorkflowPayload     `json:"recommendedWorkflow,omitempty"`
	InvestigationSummary   string                       `json:"investigationSummary"`
	EvidenceCollected      []string                     `json:"evidenceCollected,omitempty"`
	RecommendedActions     []ApprovalActionPayload      `json:"recommendedActions,omitempty"`
	AlternativesConsidered []ApprovalAlternativePayload `json:"alternativesConsidered,omitempty"`
	PolicyEvaluation       *ApprovalPolicyPayload       `json:"policyEvaluation,omitempty"`
	RequiredBy             string                       `json:"requiredBy"`
}

// ApprovalWorkflowPayload represents the recommended workflow in the approval event.
type ApprovalWorkflowPayload struct {
	WorkflowID      string `json:"workflowId"`
	Version         string `json:"version"`
	ExecutionBundle string `json:"executionBundle,omitempty"`
	Rationale       string `json:"rationale"`
}

// ApprovalActionPayload represents a recommended action in the approval event.
type ApprovalActionPayload struct {
	Action    string `json:"action"`
	Rationale string `json:"rationale"`
}

// ApprovalAlternativePayload represents an alternative considered in the approval event.
type ApprovalAlternativePayload struct {
	Approach string `json:"approach"`
	ProsCons string `json:"prosCons"`
}

// ApprovalPolicyPayload represents policy evaluation results in the approval event.
type ApprovalPolicyPayload struct {
	PolicyName   string   `json:"policyName"`
	MatchedRules []string `json:"matchedRules,omitempty"`
	Decision     string   `json:"decision"`
}

// ApprovalResolvedEventPayload is the structured payload emitted as a
// TaskStatusUpdateEvent with metadata.type="approval_request_resolved"
// when the RAR decision is made or expires.
type ApprovalResolvedEventPayload struct {
	Name             string                           `json:"name"`
	Decision         string                           `json:"decision"`
	DecidedBy        string                           `json:"decidedBy,omitempty"`
	DecidedAt        string                           `json:"decidedAt,omitempty"`
	DecisionMessage  string                           `json:"decisionMessage,omitempty"`
	WorkflowOverride *ApprovalWorkflowOverridePayload `json:"workflowOverride,omitempty"`
}

// ApprovalWorkflowOverridePayload represents an operator workflow override.
type ApprovalWorkflowOverridePayload struct {
	WorkflowName string            `json:"workflowName,omitempty"`
	Parameters   map[string]string `json:"parameters,omitempty"`
	Rationale    string            `json:"rationale,omitempty"`
}

// MarshalApprovalRequestPayload extracts RAR spec fields from a typed
// RemediationApprovalRequest and returns a JSON string suitable for
// EmitStructuredMeta emission.
func MarshalApprovalRequestPayload(rar *remediationv1.RemediationApprovalRequest) (string, error) {
	payload := ApprovalRequestEventPayload{
		Name:                   rar.Name,
		Namespace:              rar.Namespace,
		RemediationRequestName: rar.Spec.RemediationRequestRef.Name,
		Confidence:             rar.Spec.Confidence,
		ConfidenceLevel:        rar.Spec.ConfidenceLevel,
		Reason:                 rar.Spec.Reason,
		WhyApprovalRequired:    rar.Spec.WhyApprovalRequired,
		InvestigationSummary:   rar.Spec.InvestigationSummary,
		EvidenceCollected:      rar.Spec.EvidenceCollected,
	}

	if !rar.Spec.RequiredBy.IsZero() {
		payload.RequiredBy = rar.Spec.RequiredBy.Format(time.RFC3339)
	}

	wf := rar.Spec.RecommendedWorkflow
	if wf.WorkflowID != "" || wf.Version != "" {
		payload.RecommendedWorkflow = &ApprovalWorkflowPayload{
			WorkflowID:      wf.WorkflowID,
			Version:         wf.Version,
			ExecutionBundle: wf.ExecutionBundle,
			Rationale:       wf.Rationale,
		}
	}

	for _, a := range rar.Spec.RecommendedActions {
		payload.RecommendedActions = append(payload.RecommendedActions, ApprovalActionPayload{
			Action:    a.Action,
			Rationale: a.Rationale,
		})
	}

	for _, a := range rar.Spec.AlternativesConsidered {
		payload.AlternativesConsidered = append(payload.AlternativesConsidered, ApprovalAlternativePayload{
			Approach: a.Approach,
			ProsCons: a.ProsCons,
		})
	}

	if rar.Spec.PolicyEvaluation != nil {
		payload.PolicyEvaluation = &ApprovalPolicyPayload{
			PolicyName:   rar.Spec.PolicyEvaluation.PolicyName,
			Decision:     rar.Spec.PolicyEvaluation.Decision,
			MatchedRules: rar.Spec.PolicyEvaluation.MatchedRules,
		}
	}

	b, err := json.Marshal(payload)
	return string(b), err
}

// MarshalApprovalResolvedPayload extracts RAR status decision fields from a
// typed RemediationApprovalRequest and returns a JSON string for the resolution
// event.
func MarshalApprovalResolvedPayload(rar *remediationv1.RemediationApprovalRequest) (string, error) {
	payload := ApprovalResolvedEventPayload{
		Name:     rar.Name,
		Decision: string(rar.Status.Decision),
		DecidedBy: rar.Status.DecidedBy,
		DecisionMessage: rar.Status.DecisionMessage,
	}

	if rar.Status.DecidedAt != nil {
		payload.DecidedAt = rar.Status.DecidedAt.Format(time.RFC3339)
	}

	if wo := rar.Status.WorkflowOverride; wo != nil {
		payload.WorkflowOverride = &ApprovalWorkflowOverridePayload{
			WorkflowName: wo.WorkflowName,
			Parameters:   wo.Parameters,
			Rationale:    wo.Rationale,
		}
	}

	b, err := json.Marshal(payload)
	return string(b), err
}
