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

package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentSessionPhase represents the lifecycle state of an AgentSession, as
// exclusively written by KA (BR-AA-KA-065.4).
type AgentSessionPhase string

// AgentSessionPhase values.
const (
	// AgentSessionPhasePending means AA has created the AgentSession but no
	// KA replica has yet acquired the dispatch Lease (BR-AA-KA-065.3).
	AgentSessionPhasePending AgentSessionPhase = "Pending"
	// AgentSessionPhaseInvestigating means a KA replica won the dispatch
	// Lease and the investigation is running.
	AgentSessionPhaseInvestigating AgentSessionPhase = "Investigating"
	// AgentSessionPhaseCompleted means the investigation finished and
	// Status.Result carries the curated outcome.
	AgentSessionPhaseCompleted AgentSessionPhase = "Completed"
	// AgentSessionPhaseFailed means the investigation could not complete;
	// Status.Error carries a curated, user-facing message (SI-11).
	AgentSessionPhaseFailed AgentSessionPhase = "Failed"
	// AgentSessionPhaseCancelled means the interactive session ended
	// (e.g. the driving user disconnected without a takeover).
	AgentSessionPhaseCancelled AgentSessionPhase = "Cancelled"
)

// AgentSessionStatus.Reason values.
const (
	// AgentSessionReasonCapacityExceeded means KA rejected dispatch because
	// its per-process session.Store MaxConcurrentInvestigations capacity
	// was exceeded (session.ErrMaxInvestigationsReached) -- a transient,
	// self-resolving backpressure condition, not a genuine investigation
	// failure. BR-AI-009 (retry transient errors with backoff): AA treats
	// this Reason as retryable, distinct from all other Failed causes.
	AgentSessionReasonCapacityExceeded = "CapacityExceeded"
)

// ObjectRef is a reference to a namespaced Kubernetes object.
type ObjectRef struct {
	// Name of the referenced object.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
	// Namespace of the referenced object.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Namespace string `json:"namespace"`
}

// AgentSessionSpec defines the desired state (immutable after creation).
// Exists uniformly for every investigation AA initiates, autonomous or
// interactive. Beyond RemediationRequestRef, the incident-payload fields
// below are a 1:1, lossless translation of the retired
// agentclient.IncidentRequest HTTP request body (BR-AA-KA-065.2): AA
// populates them at Create time from the exact same source
// (AIAnalysis.Spec.AnalysisRequest) RequestBuilder.BuildIncidentRequest used
// to read, so removing the HTTP channel loses no content KA previously
// received. KA's dispatch watcher reads Spec directly -- no additional
// RemediationRequest/AIAnalysis RBAC is required on KA's side.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable"
type AgentSessionSpec struct {
	// RemediationRequestRef references the RR this session investigates.
	// The AgentSession MUST be created in the same namespace as the RR.
	// OwnerReference is set to the AIAnalysis that creates this session
	// (not directly to the RR) -- AA already holds that object live, with
	// no additional RemediationRequest RBAC required (AC-6). Cascade
	// deletion still reaches the RR transitively: RO already sets the RR
	// as AIAnalysis's owner, so RR deletion -> AIAnalysis cascade-deletes
	// -> AgentSession cascade-deletes, same end state as a direct edge,
	// one hop deeper. Kubernetes' garbage collector walks owner chains
	// transitively regardless of depth.
	RemediationRequestRef ObjectRef `json:"remediationRequestRef"`

	// IncidentID is the unique incident identifier (AIAnalysis CR name).
	IncidentID string `json:"incidentID"`
	// RemediationID is for audit correlation ONLY -- never used for RCA or
	// workflow matching.
	RemediationID string `json:"remediationID"`
	// SignalName is the canonical signal name (e.g. OOMKilled).
	SignalName string `json:"signalName"`
	// Severity is the signal severity (BR-SEVERITY-001).
	// +kubebuilder:validation:Enum=critical;high;warning;info;unknown
	Severity string `json:"severity"`
	// SignalSource is the monitoring system that produced the signal.
	// +optional
	SignalSource string `json:"signalSource,omitempty"`
	// ResourceNamespace is the target resource's Kubernetes namespace.
	// +optional
	ResourceNamespace string `json:"resourceNamespace,omitempty"`
	// ResourceKind is the target resource's Kubernetes kind.
	// +optional
	ResourceKind string `json:"resourceKind,omitempty"`
	// ResourceName is the target resource's name.
	// +optional
	ResourceName string `json:"resourceName,omitempty"`
	// ResourceAPIVersion disambiguates the target resource's API group.
	// +optional
	ResourceAPIVersion string `json:"resourceAPIVersion,omitempty"`
	// ErrorMessage is an optional error message associated with the signal.
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`
	// Description is an optional additional description.
	// +optional
	Description string `json:"description,omitempty"`
	// Environment is the deployment environment.
	// +optional
	Environment string `json:"environment,omitempty"`
	// Priority is the business priority.
	// +optional
	Priority string `json:"priority,omitempty"`
	// RiskTolerance is the configured risk tolerance.
	// +optional
	RiskTolerance string `json:"riskTolerance,omitempty"`
	// BusinessCategory is the configured business category.
	// +optional
	BusinessCategory string `json:"businessCategory,omitempty"`
	// ClusterName is the Kubernetes cluster name.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`
	// Cluster is the optional fleet business classification (BR-FLEET-003).
	// +optional
	Cluster string `json:"cluster,omitempty"`
	// IsDuplicate reports whether this is a duplicate signal.
	// +optional
	IsDuplicate *bool `json:"isDuplicate,omitempty"`
	// OccurrenceCount is the number of times this signal has occurred.
	// +optional
	OccurrenceCount *int `json:"occurrenceCount,omitempty"`
	// DeduplicationWindowMinutes is the dedup window used by SignalProcessing.
	// +optional
	DeduplicationWindowMinutes *int `json:"deduplicationWindowMinutes,omitempty"`
	// FiringTime is the ISO timestamp the signal started firing.
	// +optional
	FiringTime string `json:"firingTime,omitempty"`
	// ReceivedTime is the ISO timestamp kubernaut received the signal.
	// +optional
	ReceivedTime string `json:"receivedTime,omitempty"`
	// FirstSeen is the ISO timestamp of the first occurrence.
	// +optional
	FirstSeen string `json:"firstSeen,omitempty"`
	// LastSeen is the ISO timestamp of the most recent occurrence.
	// +optional
	LastSeen string `json:"lastSeen,omitempty"`
	// SignalLabels are the signal's Kubernetes-style labels.
	// +optional
	SignalLabels map[string]string `json:"signalLabels,omitempty"`
	// SignalAnnotations are alert-author annotations from the original
	// signal (e.g. description/summary from AlertManager).
	// +optional
	SignalAnnotations map[string]string `json:"signalAnnotations,omitempty"`
	// EnrichmentResults is the enriched context from SignalProcessing
	// (Kubernetes context, custom labels), free-form per its evolving
	// schema -- same raw-JSON precedent as AgentSessionResult's fields.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	EnrichmentResults *apiextensionsv1.JSON `json:"enrichmentResults,omitempty"`
	// SignalMode controls KA's prompt strategy: "reactive" or "proactive"
	// (ADR-054).
	// +kubebuilder:validation:Enum=reactive;proactive
	// +optional
	SignalMode string `json:"signalMode,omitempty"`

	// TimesOutAt is the absolute deadline for this investigation, propagated
	// verbatim from AIAnalysis.Spec.TimesOutAt (itself propagated from
	// RemediationRequest.Status.TimeoutConfig.Analyzing by
	// RemediationOrchestrator, DD-TIMEOUT-002) by AA at AgentSession creation
	// time. #2170 (DD-AA-KA-001 Amendment N): KA's dispatcher independently
	// self-enforces this same deadline (resync loop) so a partitioned or
	// crashed AA replica -- which has no other way to tell KA to stop, now
	// that HTTP polling's CancelSession RPC is gone -- can never leave an
	// investigation running forever. An absolute timestamp (rather than a
	// relative duration) avoids clock-skew ambiguity between AA and KA,
	// mirroring AIAnalysis.Spec.TimesOutAt's own rationale. If nil, KA
	// applies no self-enforced deadline for this AgentSession (AA's own
	// checkInvestigationTimeout fallback-duration enforcement still
	// applies independently on AA's side).
	// +optional
	TimesOutAt *metav1.Time `json:"timesOutAt,omitempty"`

	// Interactive is deliberately NOT a field on Spec. DD-AA-KA-001
	// Amendment Gap 1: Spec is immutable after Create, but whether this
	// investigation should be interactive can become true either before or
	// after AA's Create (a human can call AF's MCP start action at any
	// time) -- there is no create-time snapshot that stays correct. KA's
	// dispatcher is the sole owner of this determination, checking
	// InvestigationSession existence at the actual dispatch decision point
	// and writing the result to Status.Interactive (BR-AA-KA-065.5), never
	// to Spec.
}

// AgentSessionAlternativeWorkflow mirrors the retired
// agentclient.AlternativeWorkflow shape as a plain, CRD-native type
// (context/audit only -- never used for automatic fallback execution).
type AgentSessionAlternativeWorkflow struct {
	WorkflowID      string  `json:"workflowID,omitempty"`
	ExecutionBundle string  `json:"executionBundle,omitempty"`
	Confidence      float64 `json:"confidence,omitempty"`
	Rationale       string  `json:"rationale,omitempty"`
}

// AgentSessionValidationAttempt mirrors the retired
// agentclient.ValidationAttempt shape as a plain, CRD-native type.
type AgentSessionValidationAttempt struct {
	Attempt    int      `json:"attempt,omitempty"`
	WorkflowID string   `json:"workflowID,omitempty"`
	IsValid    bool     `json:"isValid,omitempty"`
	Errors     []string `json:"errors,omitempty"`
	Timestamp  string   `json:"timestamp,omitempty"`
}

// AgentSessionAlignmentFinding mirrors one finding from the shadow-agent
// alignment verdict.
type AgentSessionAlignmentFinding struct {
	StepIndex   int    `json:"stepIndex,omitempty"`
	StepKind    string `json:"stepKind,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Explanation string `json:"explanation,omitempty"`
}

// AgentSessionAlignmentVerdict mirrors the retired
// agentclient.AlignmentVerdict shape as a plain, CRD-native type. Present
// when KA's alignment check is enabled; primary content when
// CircuitBreakerActivated=true.
type AgentSessionAlignmentVerdict struct {
	Result                  string                         `json:"result,omitempty"`
	CircuitBreakerActivated bool                           `json:"circuitBreakerActivated,omitempty"`
	Summary                 string                         `json:"summary,omitempty"`
	Flagged                 int                            `json:"flagged,omitempty"`
	Total                   int                            `json:"total,omitempty"`
	Findings                []AgentSessionAlignmentFinding `json:"findings,omitempty"`
}

// AgentSessionResult carries the completed investigation's curated output,
// written by KA to AgentSession.Status.Result on the Completed transition.
// SI-10: this MUST exclude internal workflow/validation/alignment state --
// same curation boundary as the retired MarshalRCASubset precedent.
//
// RootCauseAnalysis, SelectedWorkflow, and DetectedLabels are intentionally
// raw JSON (not fully-typed structs): KA's schema for these fields evolves
// independently of this CRD (the same reason the retired ogen schema
// treated them as free-form additionalProperties), and AA's existing
// map-extraction helpers (GetStringFromMap, GetFloat64FromMap, etc.)
// operate generically on the decoded map regardless of source.
type AgentSessionResult struct {
	// IncidentID is KA's investigation identifier for correlation.
	IncidentID string `json:"incidentID,omitempty"`
	// Analysis is the natural-language analysis produced by the LLM.
	Analysis string `json:"analysis,omitempty"`
	// RootCauseAnalysis is the structured RCA (summary, severity,
	// contributing_factors, remediationTarget), free-form per KA's schema.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	RootCauseAnalysis *apiextensionsv1.JSON `json:"rootCauseAnalysis,omitempty"`
	// SelectedWorkflow is the workflow KA selected (workflow_id,
	// execution_bundle, confidence, parameters), free-form per KA's schema.
	// Absent (nil) means no workflow was selected.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	SelectedWorkflow *apiextensionsv1.JSON `json:"selectedWorkflow,omitempty"`
	// Confidence is KA's overall confidence in the analysis (0.0-1.0).
	Confidence float64 `json:"confidence,omitempty"`
	// Timestamp is the ISO timestamp of analysis completion.
	Timestamp string `json:"timestamp,omitempty"`
	// NeedsHumanReview is true when KA could not produce a reliable result.
	// When true, AA must NOT create a WorkflowExecution.
	NeedsHumanReview bool `json:"needsHumanReview,omitempty"`
	// HumanReviewReason is the structured reason when NeedsHumanReview=true.
	// Empty means no specific reason was recorded.
	// +optional
	HumanReviewReason string `json:"humanReviewReason,omitempty"`
	// IsActionable is the LLM's assessment of whether the alert warrants
	// action. nil means the LLM did not explicitly assess actionability.
	// +optional
	IsActionable *bool `json:"isActionable,omitempty"`
	// Warnings are non-fatal warnings from the investigation.
	// +optional
	Warnings []string `json:"warnings,omitempty"`
	// AlternativeWorkflows lists workflows considered but not selected
	// (operator context/audit trail only, never automatic fallback).
	// +optional
	AlternativeWorkflows []AgentSessionAlternativeWorkflow `json:"alternativeWorkflows,omitempty"`
	// ValidationAttemptsHistory records LLM self-correction attempts.
	// +optional
	ValidationAttemptsHistory []AgentSessionValidationAttempt `json:"validationAttemptsHistory,omitempty"`
	// DetectedLabels are cluster characteristics detected at runtime by KA
	// (ADR-056), free-form per KA's schema.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	DetectedLabels *apiextensionsv1.JSON `json:"detectedLabels,omitempty"`
	// AlignmentVerdict is the shadow-agent alignment verdict, present when
	// KA's alignment check is enabled.
	// +optional
	AlignmentVerdict *AgentSessionAlignmentVerdict `json:"alignmentVerdict,omitempty"`
}

// AgentSessionStatus defines the observed state (mutable, KA-only per
// BR-AA-KA-065.9 -- AA and AF never write Status, only watch it).
type AgentSessionStatus struct {
	// Phase is the current lifecycle state, exclusively written by KA.
	// +kubebuilder:validation:Enum=Pending;Investigating;Completed;Failed;Cancelled
	Phase AgentSessionPhase `json:"phase,omitempty"`

	// SessionID is KA's internal investigation session identifier, used for
	// audit correlation (AU-2/AU-3). Set the instant a KA replica wins the
	// dispatch Lease and begins investigating.
	SessionID string `json:"sessionID,omitempty"`

	// Interactive is true once a human driver has taken over this
	// investigation via KA's UpgradeToInteractive (BR-AA-KA-065.5). Written
	// atomically with, never before, the interactive-driver Lease actually
	// being held. AA and AF MUST read this field, not infer interactivity
	// from InvestigationSession existence or any other secondhand signal.
	Interactive bool `json:"interactive,omitempty"`

	// ActingUser is the authenticated identity currently driving an
	// interactive session, if any.
	// +optional
	ActingUser string `json:"actingUser,omitempty"`

	// ActingUserGroups are the RBAC-relevant groups of ActingUser.
	// +optional
	ActingUserGroups []string `json:"actingUserGroups,omitempty"`

	// Result is the curated investigation outcome, set on the Completed
	// transition (SI-10: never carries internal workflow/validation/
	// alignment state).
	// +optional
	Result *AgentSessionResult `json:"result,omitempty"`

	// Error is a curated, user-facing failure message, set on the Failed
	// transition (SI-11: never a raw internal error string).
	// +optional
	Error string `json:"error,omitempty"`

	// Reason is a curated, machine-readable failure classification, set
	// alongside Error on the Failed transition. Currently the sole defined
	// value is AgentSessionReasonCapacityExceeded (BR-AI-009, DD-AA-KA-001
	// amendment) -- distinguishing a transient, self-resolving capacity
	// rejection (session.ErrMaxInvestigationsReached) from any other
	// investigation failure, so AA can retry instead of permanently
	// failing the AIAnalysis. Empty for all other failure causes.
	// +optional
	Reason string `json:"reason,omitempty"`

	// DispatchedAt is the timestamp when a KA replica won the dispatch
	// Lease and began investigating.
	// +optional
	DispatchedAt *metav1.Time `json:"dispatchedAt,omitempty"`

	// CompletedAt is the timestamp when the session reached a terminal
	// phase (Completed, Failed, or Cancelled).
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// Conditions provide detailed status information.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:selectablefield:JSONPath=.spec.remediationRequestRef.name
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Interactive",type=boolean,JSONPath=`.status.interactive`
// +kubebuilder:printcolumn:name="Session ID",type=string,JSONPath=`.status.sessionID`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=asess

// AgentSession is the single AA<->KA dispatch/result channel (DD-AA-KA-001,
// BR-AA-KA-065). AA creates one AgentSession per investigation (autonomous
// or interactive) with an ownerRef to the AIAnalysis that creates it (cascade
// reaches the RemediationRequest transitively, see RemediationRequestRef);
// KA watches for Create/Update events, dispatches exactly once via a
// per-AgentSession coordination/v1 Lease, and exclusively writes Status.
// Replaces the retired pkg/agentclient HTTP submit/poll/result channel.
type AgentSession struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSessionSpec   `json:"spec,omitempty"`
	Status AgentSessionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentSessionList contains a list of AgentSession.
type AgentSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentSession `json:"items"`
}
