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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

// MarshalApprovalRequestPayload extracts RAR spec fields from an unstructured
// object and returns a JSON string suitable for EmitStructuredMeta emission.
func MarshalApprovalRequestPayload(obj *unstructured.Unstructured) (string, error) {
	payload := ApprovalRequestEventPayload{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
	if spec == nil {
		b, err := json.Marshal(payload)
		return string(b), err
	}

	if ref, ok := spec["remediationRequestRef"].(map[string]interface{}); ok {
		if name, ok := ref["name"].(string); ok {
			payload.RemediationRequestName = name
		}
	}

	if v, ok := spec["confidence"].(float64); ok {
		payload.Confidence = v
	}
	if v, ok := spec["confidenceLevel"].(string); ok {
		payload.ConfidenceLevel = v
	}
	if v, ok := spec["reason"].(string); ok {
		payload.Reason = v
	}
	if v, ok := spec["whyApprovalRequired"].(string); ok {
		payload.WhyApprovalRequired = v
	}
	if v, ok := spec["investigationSummary"].(string); ok {
		payload.InvestigationSummary = v
	}
	if v, ok := spec["requiredBy"].(string); ok {
		payload.RequiredBy = v
	}

	if wf, ok := spec["recommendedWorkflow"].(map[string]interface{}); ok {
		payload.RecommendedWorkflow = &ApprovalWorkflowPayload{}
		if v, ok := wf["workflowId"].(string); ok {
			payload.RecommendedWorkflow.WorkflowID = v
		}
		if v, ok := wf["version"].(string); ok {
			payload.RecommendedWorkflow.Version = v
		}
		if v, ok := wf["executionBundle"].(string); ok {
			payload.RecommendedWorkflow.ExecutionBundle = v
		}
		if v, ok := wf["rationale"].(string); ok {
			payload.RecommendedWorkflow.Rationale = v
		}
	}

	if items, ok := spec["evidenceCollected"].([]interface{}); ok {
		for _, item := range items {
			if s, ok := item.(string); ok {
				payload.EvidenceCollected = append(payload.EvidenceCollected, s)
			}
		}
	}

	if items, ok := spec["recommendedActions"].([]interface{}); ok {
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				action := ApprovalActionPayload{}
				if v, ok := m["action"].(string); ok {
					action.Action = v
				}
				if v, ok := m["rationale"].(string); ok {
					action.Rationale = v
				}
				payload.RecommendedActions = append(payload.RecommendedActions, action)
			}
		}
	}

	if items, ok := spec["alternativesConsidered"].([]interface{}); ok {
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				alt := ApprovalAlternativePayload{}
				if v, ok := m["approach"].(string); ok {
					alt.Approach = v
				}
				if v, ok := m["prosCons"].(string); ok {
					alt.ProsCons = v
				}
				payload.AlternativesConsidered = append(payload.AlternativesConsidered, alt)
			}
		}
	}

	if pe, ok := spec["policyEvaluation"].(map[string]interface{}); ok {
		payload.PolicyEvaluation = &ApprovalPolicyPayload{}
		if v, ok := pe["policyName"].(string); ok {
			payload.PolicyEvaluation.PolicyName = v
		}
		if v, ok := pe["decision"].(string); ok {
			payload.PolicyEvaluation.Decision = v
		}
		if rules, ok := pe["matchedRules"].([]interface{}); ok {
			for _, r := range rules {
				if s, ok := r.(string); ok {
					payload.PolicyEvaluation.MatchedRules = append(payload.PolicyEvaluation.MatchedRules, s)
				}
			}
		}
	}

	b, err := json.Marshal(payload)
	return string(b), err
}

// MarshalApprovalResolvedPayload extracts RAR status decision fields from an
// unstructured object and returns a JSON string for the resolution event.
func MarshalApprovalResolvedPayload(obj *unstructured.Unstructured) (string, error) {
	payload := ApprovalResolvedEventPayload{
		Name: obj.GetName(),
	}

	status, _, _ := unstructured.NestedMap(obj.Object, "status")
	if status != nil {
		if v, ok := status["decision"].(string); ok {
			payload.Decision = v
		}
		if v, ok := status["decidedBy"].(string); ok {
			payload.DecidedBy = v
		}
		if v, ok := status["decidedAt"].(string); ok {
			payload.DecidedAt = v
		}
		if v, ok := status["decisionMessage"].(string); ok {
			payload.DecisionMessage = v
		}
		if wo, ok := status["workflowOverride"].(map[string]interface{}); ok {
			payload.WorkflowOverride = &ApprovalWorkflowOverridePayload{}
			if v, ok := wo["workflowName"].(string); ok {
				payload.WorkflowOverride.WorkflowName = v
			}
			if v, ok := wo["rationale"].(string); ok {
				payload.WorkflowOverride.Rationale = v
			}
			if params, ok := wo["parameters"].(map[string]interface{}); ok {
				payload.WorkflowOverride.Parameters = make(map[string]string, len(params))
				for k, v := range params {
					if s, ok := v.(string); ok {
						payload.WorkflowOverride.Parameters[k] = s
					}
				}
			}
		}
	}

	b, err := json.Marshal(payload)
	return string(b), err
}
