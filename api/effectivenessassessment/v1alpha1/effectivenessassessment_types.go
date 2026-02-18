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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================================
// EffectivenessAssessment CRD Types
// ============================================================================
//
// The EffectivenessAssessment (EA) CRD is created by the Remediation Orchestrator
// after a remediation workflow completes. The Effectiveness Monitor controller
// watches EA CRDs and performs 4 assessment checks (health, alert, metrics, hash),
// emitting component audit events to DataStorage.
//
// Architecture: ADR-EM-001 (Effectiveness Monitor Service Integration)
// Immutability: Spec is immutable after creation (CEL validation, ADR-001)
// Owner: RemediationRequest (ownerRef for garbage collection)
//
// Note: correlation-id and rr-phase were formerly labels; now in spec for immutability.
// ============================================================================

// Phase constants for EffectivenessAssessment
const (
	// PhasePending indicates the EA has been created by RO but EM has not yet reconciled it.
	PhasePending = "Pending"
	// PhaseAssessing indicates EM is actively performing assessment checks.
	PhaseAssessing = "Assessing"
	// PhaseCompleted indicates all assessment checks have finished (or validity expired).
	PhaseCompleted = "Completed"
	// PhaseFailed indicates the assessment could not be performed (e.g., target not found).
	PhaseFailed = "Failed"
)

// AssessmentReason constants describe why an assessment completed with a particular outcome.
const (
	// AssessmentReasonFull indicates all enabled components were assessed successfully.
	AssessmentReasonFull = "full"
	// AssessmentReasonPartial indicates some components were assessed but not all.
	AssessmentReasonPartial = "partial"
	// AssessmentReasonNoExecution indicates no workflow execution was found for this RR.
	AssessmentReasonNoExecution = "no_execution"
	// AssessmentReasonMetricsTimedOut indicates metrics were not available before validity expired.
	AssessmentReasonMetricsTimedOut = "metrics_timed_out"
	// AssessmentReasonExpired indicates the validity window expired with no data collected.
	AssessmentReasonExpired = "expired"
	// AssessmentReasonSpecDrift indicates the target resource spec was modified during assessment.
	// The remediation is considered unsuccessful — DS score = 0.0 (DD-EM-002 v1.1).
	AssessmentReasonSpecDrift = "spec_drift"
)

// EffectivenessAssessmentSpec defines the desired state of an EffectivenessAssessment.
//
// The spec is set by the Remediation Orchestrator at creation time and is immutable.
// Immutability is enforced by CEL validation (self == oldSelf) to prevent tampering.
//
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation (ADR-001)"
type EffectivenessAssessmentSpec struct {
	// CorrelationID is the name of the parent RemediationRequest.
	// Used as the correlation ID for audit events (DD-AUDIT-CORRELATION-002).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	CorrelationID string `json:"correlationID"`

	// RemediationRequestPhase is the RemediationRequest's OverallPhase at the time
	// the EA was created. Captured as an immutable spec field so the EM can branch
	// assessment logic based on the RR outcome (Completed, Failed, TimedOut).
	// Previously stored as the mutable label kubernaut.ai/rr-phase; moved to spec
	// for immutability and security.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Completed;Failed;TimedOut
	RemediationRequestPhase string `json:"remediationRequestPhase"`

	// TargetResource identifies the Kubernetes resource that was remediated.
	// +kubebuilder:validation:Required
	TargetResource TargetResource `json:"targetResource"`

	// Config contains the assessment configuration parameters.
	// +kubebuilder:validation:Required
	Config EAConfig `json:"config"`

	// RemediationCreatedAt is the creation timestamp of the parent RemediationRequest.
	// Set by the RO at EA creation time from rr.CreationTimestamp.
	// Used by the audit manager to compute resolution_time_seconds in the
	// assessment.completed event (CompletedAt - RemediationCreatedAt).
	// +optional
	RemediationCreatedAt *metav1.Time `json:"remediationCreatedAt,omitempty"`

	// SignalName is the original alert/signal name from the parent RemediationRequest.
	// Set by the RO at EA creation time from rr.Spec.SignalName.
	// Used by the audit manager to populate the alert_name field in assessment.completed
	// events (OBS-1: distinct from CorrelationID which is the RR name).
	// +optional
	SignalName string `json:"signalName,omitempty"`

	// PreRemediationSpecHash is the canonical spec hash of the target resource BEFORE
	// remediation was applied. Copied from rr.Status.PreRemediationSpecHash by the RO
	// at EA creation time. The EM uses this to compare pre vs post-remediation state
	// for spec drift detection, eliminating the need to query DataStorage audit events.
	// Reference: ADR-EM-001, DD-EM-002
	// +optional
	PreRemediationSpecHash string `json:"preRemediationSpecHash,omitempty"`
}

// TargetResource identifies a Kubernetes resource by kind, name, and namespace.
type TargetResource struct {
	// Kind is the Kubernetes resource kind (e.g., "Deployment", "StatefulSet").
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`
	// Name is the resource name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// Namespace is the resource namespace.
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`
}

// EAConfig contains assessment configuration set by RO at creation time.
// Only StabilizationWindow is set by the RO — it controls how long the EM
// waits after remediation before starting assessment checks.
// All other assessment parameters (PrometheusEnabled, AlertManagerEnabled,
// ValidityWindow) are EM-internal configuration read from effectivenessmonitor.Config.
// The EM emits individual component audit events to DataStorage; the overall
// effectiveness score is computed by DataStorage on demand, not by the EM.
type EAConfig struct {
	// StabilizationWindow is the duration to wait after remediation before assessment.
	// Set by the Remediation Orchestrator. The EM uses this to delay assessment
	// until the system stabilizes post-remediation.
	// +kubebuilder:validation:Required
	StabilizationWindow metav1.Duration `json:"stabilizationWindow"`
}

// EffectivenessAssessmentStatus defines the observed state of an EffectivenessAssessment.
type EffectivenessAssessmentStatus struct {
	// Phase is the current lifecycle phase of the assessment.
	// +kubebuilder:validation:Enum=Pending;Assessing;Completed;Failed
	Phase string `json:"phase,omitempty"`

	// ValidityDeadline is the absolute time after which the assessment expires.
	// Computed by the EM controller on first reconciliation as:
	//   EA.creationTimestamp + validityWindow (from EM config).
	// This follows Kubernetes spec/status convention: the RO sets desired state
	// (StabilizationWindow in spec), and the EM computes observed/derived state
	// (ValidityDeadline in status). This prevents misconfiguration where
	// StabilizationWindow > ValidityDeadline.
	// +optional
	ValidityDeadline *metav1.Time `json:"validityDeadline,omitempty"`

	// PrometheusCheckAfter is the earliest time to query Prometheus for metrics.
	// Computed by the EM controller on first reconciliation as:
	//   EA.creationTimestamp + StabilizationWindow (from EA spec).
	// Stored in status to avoid recomputation on every reconcile and for
	// operator observability of the assessment timeline.
	// +optional
	PrometheusCheckAfter *metav1.Time `json:"prometheusCheckAfter,omitempty"`

	// AlertManagerCheckAfter is the earliest time to check AlertManager for alert resolution.
	// Computed by the EM controller on first reconciliation as:
	//   EA.creationTimestamp + StabilizationWindow (from EA spec).
	// Stored in status to avoid recomputation on every reconcile and for
	// operator observability of the assessment timeline.
	// +optional
	AlertManagerCheckAfter *metav1.Time `json:"alertManagerCheckAfter,omitempty"`

	// Components tracks the completion state of each assessment component.
	Components EAComponents `json:"components,omitempty"`

	// AssessmentReason describes why the assessment completed with this outcome.
	// +kubebuilder:validation:Enum=full;partial;no_execution;metrics_timed_out;expired;spec_drift
	AssessmentReason string `json:"assessmentReason,omitempty"`

	// CompletedAt is the timestamp when the assessment finished.
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// Message provides human-readable details about the current state.
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations of the EA's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// EAComponents tracks the completion state and scores of each assessment component.
// The EM updates these fields as each component check completes.
// This enables restart recovery: if EM restarts mid-assessment, it can skip
// already-completed components by checking these flags.
type EAComponents struct {
	// HealthAssessed indicates whether the health check has been completed.
	HealthAssessed bool `json:"healthAssessed,omitempty"`
	// HealthScore is the health check score (0.0-1.0), nil if not yet assessed.
	HealthScore *float64 `json:"healthScore,omitempty"`

	// HashComputed indicates whether the spec hash comparison has been completed.
	HashComputed bool `json:"hashComputed,omitempty"`
	// PostRemediationSpecHash is the hash of the target resource spec after remediation.
	PostRemediationSpecHash string `json:"postRemediationSpecHash,omitempty"`
	// CurrentSpecHash is the most recent hash of the target resource spec,
	// re-computed on each reconcile after HashComputed is true (DD-EM-002 v1.1).
	// If it differs from PostRemediationSpecHash, spec drift was detected.
	CurrentSpecHash string `json:"currentSpecHash,omitempty"`

	// AlertAssessed indicates whether the alert resolution check has been completed.
	AlertAssessed bool `json:"alertAssessed,omitempty"`
	// AlertScore is the alert resolution score (0.0 or 1.0), nil if not yet assessed.
	AlertScore *float64 `json:"alertScore,omitempty"`

	// MetricsAssessed indicates whether the metric comparison has been completed.
	MetricsAssessed bool `json:"metricsAssessed,omitempty"`
	// MetricsScore is the metric comparison score (0.0-1.0), nil if not yet assessed.
	MetricsScore *float64 `json:"metricsScore,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.assessmentReason`
// +kubebuilder:printcolumn:name="CorrelationID",type=string,JSONPath=`.spec.correlationID`
// +kubebuilder:printcolumn:name="ReadyReason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// EffectivenessAssessment is the Schema for the effectivenessassessments API.
// It is created by the Remediation Orchestrator and watched by the Effectiveness Monitor.
type EffectivenessAssessment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EffectivenessAssessmentSpec   `json:"spec,omitempty"`
	Status EffectivenessAssessmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EffectivenessAssessmentList contains a list of EffectivenessAssessment.
type EffectivenessAssessmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EffectivenessAssessment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EffectivenessAssessment{}, &EffectivenessAssessmentList{})
}
