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

package types

// Phase represents a stage in the investigation flow.
// Per DD-HAPI-019-002: RCA uses K8s+Prom tools, Discovery uses workflow tools,
// Validation uses no tools (I4 per-phase tool scoping).
type Phase string

const (
	PhaseRCA               Phase = "rca"
	PhaseWorkflowDiscovery Phase = "workflow_discovery"
	PhaseValidation        Phase = "validation"
)

// PhaseToolMap defines which tool names are available in each phase (I4).
type PhaseToolMap map[Phase][]string

// InvestigationResult holds the final output of an investigation.
type InvestigationResult struct {
	RCASummary        string                 `json:"rca_summary"`
	WorkflowID        string                 `json:"workflow_id,omitempty"`
	RemediationTarget RemediationTarget      `json:"remediation_target,omitempty"`
	Parameters        map[string]interface{} `json:"parameters,omitempty"`
	Confidence        float64                `json:"confidence"`
	HumanReviewNeeded bool                   `json:"human_review_needed"`
	Reason            string                 `json:"reason,omitempty"`
	IsActionable      *bool                  `json:"is_actionable,omitempty"`
	Warnings          []string               `json:"warnings,omitempty"`
}

// RemediationTarget identifies the K8s resource to remediate.
type RemediationTarget struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// SignalContext holds the input signal data for an investigation.
type SignalContext struct {
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	Severity         string `json:"severity"`
	Message          string `json:"message"`
	ResourceKind     string `json:"resource_kind,omitempty"`
	ResourceName     string `json:"resource_name,omitempty"`
	ClusterName      string `json:"cluster_name,omitempty"`
	Environment      string `json:"environment,omitempty"`
	Priority         string `json:"priority,omitempty"`
	RiskTolerance    string `json:"risk_tolerance,omitempty"`
	SignalSource     string `json:"signal_source,omitempty"`
	BusinessCategory string `json:"business_category,omitempty"`
	Description      string `json:"description,omitempty"`
}
