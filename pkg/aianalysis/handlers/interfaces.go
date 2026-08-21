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

// Package handlers implements phase handlers for the AIAnalysis controller.
//
// P1.3 Refactoring: Consolidated interfaces from investigating.go and analyzing.go
// for better organization and discoverability.
package handlers

import (
	"context"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

// ========================================
// AGENTSESSION GET-OR-CREATOR INTERFACE
// DD-AA-KA-001, BR-AA-KA-065.1: AA<->KA channel via the AgentSession CRD
// ========================================

// AgentSessionGetOrCreator gets-or-creates the AgentSession CRD backing an
// investigation, replacing the retired AgentClientInterface's
// SubmitInvestigation/PollSession/GetSessionResult trio. GetOrCreate is
// naturally idempotent -- Create on the first call (no AgentSession exists
// yet), a plain Get on every call thereafter -- so the old submit-vs-poll
// distinction collapses into a single call site: Handle() calls this once
// per reconcile and branches on the returned object's Status.Phase.
type AgentSessionGetOrCreator interface {
	GetOrCreate(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (*agentsessionv1.AgentSession, error)

	// DeleteForRetry deletes as so the next GetOrCreate call falls through
	// to Create, giving a BR-AI-009 capacity-exceeded retry attempt a fresh
	// AgentSession (DD-AA-KA-001 amendment). Idempotent: NotFound is not an
	// error.
	DeleteForRetry(ctx context.Context, as *agentsessionv1.AgentSession) error
}

// ========================================
// AUDIT CLIENT INTERFACES
// DD-AUDIT-003: Injected audit client for dependency injection
// ========================================

// AuditClientInterface defines audit methods for the Investigating phase.
// Used for dependency injection to enable testing without real audit storage.
//
// Methods:
// - RecordAIAgentCall: Records AI agent API calls with status and duration
// - RecordPhaseTransition: Records phase transition events (DD-AUDIT-003)
// - RecordAIAgentSubmit: Records AI agent submit event at AgentSession create time (BR-AA-KA-065.1)
// - RecordAIAgentResult: Records AI agent result retrieval at AgentSession Status-read time (BR-AA-KA-065.1)
//
// RecordAIAgentSessionLost (BR-AA-KA-064) was retired, not repurposed, along with the
// regeneration-cap mechanism it audited -- see DD-AA-KA-001, BR-AA-KA-065.7.
type AuditClientInterface interface {
	RecordAIAgentCall(ctx context.Context, analysis *aianalysisv1.AIAnalysis, endpoint string, statusCode int, durationMs int)
	RecordPhaseTransition(ctx context.Context, analysis *aianalysisv1.AIAnalysis, from, to string)
	// BR-AUDIT-005 Gap #7: Record analysis failures with standardized ErrorDetails
	RecordAnalysisFailed(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) error
	// BR-KA-200: Record analysis completion (for problem_resolved path)
	RecordAnalysisComplete(ctx context.Context, analysis *aianalysisv1.AIAnalysis)

	// ========================================
	// Session audit methods (BR-AA-KA-065.1)
	// ========================================

	// RecordAIAgentSubmit records an AI agent submit event with session ID
	RecordAIAgentSubmit(ctx context.Context, analysis *aianalysisv1.AIAnalysis, sessionID string)
	// RecordAIAgentResult records an AI agent result retrieval with investigation time
	RecordAIAgentResult(ctx context.Context, analysis *aianalysisv1.AIAnalysis, investigationTimeMs int64)
}

// AnalyzingAuditClientInterface defines audit methods for the Analyzing phase.
// Used for dependency injection to enable testing without real audit storage.
//
// Methods:
// - RecordRegoEvaluation: Records Rego policy evaluation results
// - RecordApprovalDecision: Records approval/auto-execute decisions
// - RecordAnalysisComplete: Records analysis completion event (AA-BUG-006)
//
// Note (AA-BUG-008): Phase transitions are recorded by CONTROLLER ONLY (phase_handlers.go:215)
// Handlers change phase but do NOT record transitions (follows InvestigatingHandler pattern)
type AnalyzingAuditClientInterface interface {
	// policyHash pins the SHA-256 hash of the policy that produced this
	// evaluation onto the audit trail (BR-AI-030, Issue #1981/#2005); empty
	// when no policy was loaded.
	RecordRegoEvaluation(ctx context.Context, analysis *aianalysisv1.AIAnalysis, outcome string, degraded bool, durationMs int, reason string, policyHash string)
	RecordApprovalDecision(ctx context.Context, analysis *aianalysisv1.AIAnalysis, decision string, reason string)
	RecordAnalysisComplete(ctx context.Context, analysis *aianalysisv1.AIAnalysis)
	// DD-AUDIT-003: Record analysis failure events
	RecordAnalysisFailed(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) error
}

// ========================================
// REGO EVALUATOR INTERFACE
// BR-AI-012: Rego policy evaluation for approval decisions
// BR-AI-014: Graceful degradation for policy failures
// ========================================

// RegoEvaluatorInterface defines the contract for Rego policy evaluation.
// Used for dependency injection to enable testing without real Rego engine.
//
// Methods:
// - Evaluate: Evaluates Rego policy with given input and returns decision
type RegoEvaluatorInterface interface {
	Evaluate(ctx context.Context, input *rego.PolicyInput) (*rego.PolicyResult, error)
}
