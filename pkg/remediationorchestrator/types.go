// Package remediationorchestrator provides the central orchestration logic
// for the Kubernaut remediation lifecycle.
//
// Business Requirements:
// - BR-ORCH-001: Approval notification creation
// - BR-ORCH-025: Workflow data pass-through
// - BR-ORCH-026: Approval orchestration
// - BR-ORCH-027: Global timeout management
// - BR-ORCH-028: Per-phase timeout management
// - BR-ORCH-029-031: Notification handling
// - BR-ORCH-032-034: Resource lock deduplication handling
package remediationorchestrator

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

// PhaseTimeouts defines configurable timeouts per phase.
// Reference: BR-ORCH-027 (global), BR-ORCH-028 (per-phase)
type PhaseTimeouts struct {
	// Processing timeout for SignalProcessing phase (default: 5 minutes)
	Processing time.Duration

	// Analyzing timeout for AIAnalysis phase (default: 10 minutes)
	Analyzing time.Duration

	// AwaitingApproval timeout for approval phase (default: 24 hours)
	// Reference: BR-ORCH-026
	AwaitingApproval time.Duration

	// Executing timeout for WorkflowExecution phase (default: 30 minutes)
	Executing time.Duration

	// Global timeout for entire remediation lifecycle (default: 60 minutes)
	// Reference: BR-ORCH-027
	Global time.Duration
}

// DefaultPhaseTimeouts returns sensible default timeout values.
func DefaultPhaseTimeouts() PhaseTimeouts {
	return PhaseTimeouts{
		Processing:       5 * time.Minute,
		Analyzing:        10 * time.Minute,
		AwaitingApproval: 24 * time.Hour,
		Executing:        30 * time.Minute,
		Global:           60 * time.Minute,
	}
}

// OrchestratorConfig holds configuration for the Remediation Orchestrator controller.
type OrchestratorConfig struct {
	// Timeouts for each phase
	Timeouts PhaseTimeouts

	// RetentionPeriod after completion before cleanup (default: 24h)
	RetentionPeriod time.Duration

	// MaxConcurrentReconciles limits parallel reconciliations (default: 10)
	MaxConcurrentReconciles int

	// EnableMetrics enables Prometheus metrics (default: true)
	EnableMetrics bool
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() OrchestratorConfig {
	return OrchestratorConfig{
		Timeouts:                DefaultPhaseTimeouts(),
		RetentionPeriod:         24 * time.Hour,
		MaxConcurrentReconciles: 10,
		EnableMetrics:           true,
	}
}

// ChildCRDRefs holds references to created child CRDs.
// These are stored in RemediationRequest.Status.ChildCRDs
type ChildCRDRefs struct {
	// SignalProcessing CRD name
	SignalProcessing string

	// AIAnalysis CRD name
	AIAnalysis string

	// WorkflowExecution CRD name
	WorkflowExecution string

	// NotificationRequest CRD name (for approval notifications)
	// Reference: BR-ORCH-001
	NotificationRequest string
}

// HasAllCore returns true when all core child CRD refs are set.
// NotificationRequest is optional (only for approval flow).
func (c ChildCRDRefs) HasAllCore() bool {
	return c.SignalProcessing != "" &&
		c.AIAnalysis != "" &&
		c.WorkflowExecution != ""
}

// ReconcileResult contains the result of a reconciliation cycle.
type ReconcileResult struct {
	// Requeue indicates whether to requeue immediately
	Requeue bool

	// RequeueAfter indicates duration to wait before requeue
	RequeueAfter time.Duration

	// Phase is the current/new phase after reconciliation
	Phase phase.Phase

	// Error contains any error that occurred
	Error error

	// ChildCreated is the name of child CRD created (if any)
	ChildCreated string
}

// ShouldRequeue returns true if the result indicates requeue is needed.
func (r ReconcileResult) ShouldRequeue() bool {
	return r.Requeue || r.RequeueAfter > 0
}

// AggregatedStatus contains combined status from all child CRDs.
type AggregatedStatus struct {
	// SignalProcessing status
	SignalProcessingPhase string
	SignalProcessingReady bool
	EnrichmentResults     interface{} // From SignalProcessing.status.enrichmentResults

	// AIAnalysis status
	AIAnalysisPhase  string
	AIAnalysisReady  bool
	RequiresApproval bool
	Approved         bool
	SelectedWorkflow interface{} // From AIAnalysis.status.selectedWorkflow

	// WorkflowExecution status
	WorkflowExecutionPhase string
	WorkflowExecutionReady bool
	ExecutionSkipped       bool
	SkipReason             string
	DuplicateOf            string // For BR-ORCH-033

	// Overall status
	OverallReady bool
	Error        error
}

