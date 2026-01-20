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

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	"github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// ========================================
// HOLMESGPT-API CLIENT INTERFACE
// BR-AI-007: HolmesGPT-API integration for investigation
// BR-AI-082: Recovery flow support via InvestigateRecovery
// ========================================

// HolmesGPTClientInterface defines the contract for calling HolmesGPT-API.
// Uses generated OpenAPI types for type-safe HAPI contract compliance.
//
// Methods:
// - Investigate: Analyzes incidents via /incident/analyze endpoint
// - InvestigateRecovery: Analyzes recovery scenarios via /recovery/analyze endpoint
type HolmesGPTClientInterface interface {
	Investigate(ctx context.Context, req *client.IncidentRequest) (*client.IncidentResponse, error)
	InvestigateRecovery(ctx context.Context, req *client.RecoveryRequest) (*client.RecoveryResponse, error)
}

// ========================================
// AUDIT CLIENT INTERFACES
// DD-AUDIT-003: Injected audit client for dependency injection
// ========================================

// AuditClientInterface defines audit methods for the Investigating phase.
// Used for dependency injection to enable testing without real audit storage.
//
// Methods:
// - RecordHolmesGPTCall: Records HAPI API calls with status and duration
// - RecordPhaseTransition: Records phase transition events (DD-AUDIT-003)
type AuditClientInterface interface {
	RecordHolmesGPTCall(ctx context.Context, analysis *aianalysisv1.AIAnalysis, endpoint string, statusCode int, durationMs int)
	RecordPhaseTransition(ctx context.Context, analysis *aianalysisv1.AIAnalysis, from, to string)
	// BR-AUDIT-005 Gap #7: Record analysis failures with standardized ErrorDetails
	RecordAnalysisFailed(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) error
	// BR-HAPI-200: Record analysis completion (for problem_resolved path)
	RecordAnalysisComplete(ctx context.Context, analysis *aianalysisv1.AIAnalysis)
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
	RecordRegoEvaluation(ctx context.Context, analysis *aianalysisv1.AIAnalysis, outcome string, degraded bool, durationMs int, reason string)
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
