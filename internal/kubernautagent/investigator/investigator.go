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

package investigator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/sanitization"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/summarizer"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

const maxSelfCorrectionAttempts = 3

// Diagnostic counters for emitToSink — temporary, remove after RCA.
var (
	diagSendOK   atomic.Int64
	diagSendDrop atomic.Int64
	diagSinkNil  atomic.Int64
)

// maxForensicPayloadBytes caps serialized accumulated messages in cancellation
// audit events to 64KB (SEC-1, OPS-3). Generous to preserve forensic RAG value.
const maxForensicPayloadBytes = 64 * 1024

// SubmitResultToolName is the sentinel tool name that the LLM calls to deliver
// its structured investigation result. When detected in runLLMLoop, the tool
// call arguments are returned as content without executing any real tool.
const SubmitResultToolName = "submit_result"

// SubmitResultWithWorkflowToolName is the sentinel for the workflow-selected
// path during PhaseWorkflowDiscovery (#760 v2).
const SubmitResultWithWorkflowToolName = "submit_result_with_workflow"

// SubmitResultNoWorkflowToolName is the sentinel for the no-workflow path
// during PhaseWorkflowDiscovery (#760 v2).
const SubmitResultNoWorkflowToolName = "submit_result_no_workflow"

// LoopResult is a sealed interface representing the outcome of runLLMLoop.
// Callers dispatch on the concrete type via a type switch.
type LoopResult interface {
	loopResult()
}

// SubmitResult is returned when the LLM calls the generic submit_result tool
// (RCA phase). Reasoning carries the winning turn's thinking block, when the
// LLM's reasoning capability was enabled (BR-AI-086 AC6) — nil otherwise.
type SubmitResult struct {
	Content   string
	Reasoning *llm.ReasoningBlock
}

func (*SubmitResult) loopResult() {}

// SubmitWithWorkflowResult is returned when the LLM calls submit_result_with_workflow.
type SubmitWithWorkflowResult struct {
	Content   string
	Reasoning *llm.ReasoningBlock
}

func (*SubmitWithWorkflowResult) loopResult() {}

// SubmitNoWorkflowResult is returned when the LLM calls submit_result_no_workflow.
type SubmitNoWorkflowResult struct {
	Content   string
	Reasoning *llm.ReasoningBlock
}

func (*SubmitNoWorkflowResult) loopResult() {}

// TextResult is returned when the LLM responds with plain text (no tool call).
type TextResult struct {
	Content   string
	Reasoning *llm.ReasoningBlock
}

func (*TextResult) loopResult() {}

// ExhaustedResult is returned when the loop exhausts maxTurns, tool budget,
// or the truncation-retry escalation (#1614: the escalated retry is ALSO
// truncated). Reason distinguishes the cases for observability (#770).
type ExhaustedResult struct{ Reason string }

func (*ExhaustedResult) loopResult() {}

// CancelledResult is returned when the loop detects context cancellation
// (BR-SESSION-001). It carries accumulated state so callers can produce a
// partial InvestigationResult for snapshot retrieval (BR-SESSION-002).
type CancelledResult struct {
	Messages []llm.Message
	Turn     int
	Phase    string
	Tokens   *TokenAccumulator
}

func (*CancelledResult) loopResult() {}

// sentinelResult maps a sentinel tool call to its LoopResult type, attaching
// the reasoning block (if any) from the turn whose response produced tc, so
// reasoning captured at the LLM-client layer (BR-AI-086 AC1-3) reaches the
// InvestigationResult that gets built from the sentinel's Content.
func sentinelResult(tc llm.ToolCall, reasoning *llm.ReasoningBlock) LoopResult {
	switch tc.Name {
	case SubmitResultToolName:
		return &SubmitResult{Content: tc.Arguments, Reasoning: reasoning}
	case SubmitResultWithWorkflowToolName:
		return &SubmitWithWorkflowResult{Content: tc.Arguments, Reasoning: reasoning}
	case SubmitResultNoWorkflowToolName:
		return &SubmitNoWorkflowResult{Content: tc.Arguments, Reasoning: reasoning}
	default:
		return nil
	}
}

// CatalogFetcher retrieves a fresh workflow validator from the catalog.
// Implementations query DataStorage at request time so KA always sees the
// current catalog without boot-time prefetch or caching (see #665).
type CatalogFetcher interface {
	FetchValidator(ctx context.Context) (*parser.Validator, error)
}

// Pipeline groups the optional tool-output processing stages that the
// Investigator applies inside executeTool and runWorkflowSelection.
// All fields may be nil; nil fields are skipped.
type Pipeline struct {
	Sanitizer         *sanitization.Pipeline
	AnomalyDetector   *AnomalyDetector
	CatalogFetcher    CatalogFetcher
	Summarizer        *summarizer.Summarizer
	MaxToolOutputSize int
}

// Config holds all dependencies for constructing an Investigator.
// Using a struct instead of positional parameters makes the constructor
// stable and self-documenting. Optional fields (Registry, Pipeline)
// default to their zero values when omitted.
// ScopeResolver determines whether a Kubernetes kind is cluster-scoped (#763)
// and whether it exists in multiple API groups (#1044).
type ScopeResolver interface {
	IsClusterScoped(kind string) (bool, error)
	IsAmbiguousKind(kind string) (bool, []schema.GroupVersionResource, error)
}

type Config struct {
	Client        llm.Client
	Builder       *prompt.Builder
	ResultParser  *parser.ResultParser
	Enricher      *enrichment.Enricher
	AuditStore    audit.AuditStore
	Logger        logr.Logger
	MaxTurns      int
	PhaseTools    katypes.PhaseToolMap
	Registry      registry.ToolRegistry
	Pipeline      Pipeline
	ModelName     string
	ScopeResolver ScopeResolver
	Swappable     *llm.SwappableClient
	Metrics       *metrics.Metrics
	// PinDecorator wraps the pinned client snapshot before use.
	// When alignment is enabled, this preserves the LLMProxy chain so the
	// shadow agent observes LLM reasoning steps (C-1 bypass fix).
	// When nil, falls back to llm.NewInstrumentedClient(pinned).
	PinDecorator func(llm.Client) llm.Client
	// PhaseResolver provides per-phase LLM client resolution (#1470).
	// When non-nil, each investigation method resolves its client through
	// this resolver instead of using the legacy single-pin pattern.
	// When nil, falls back to legacy Swappable + PinDecorator behavior.
	PhaseResolver PhaseClientResolver
}

// Investigator orchestrates the two-invocation architecture:
// Invocation 1 (RCA): system prompt + tool calls -> RCA summary
// Invocation 2 (Workflow Selection): new session with RCA context -> workflow choice
type Investigator struct {
	client        llm.Client
	builder       *prompt.Builder
	resultParser  *parser.ResultParser
	enricher      *enrichment.Enricher
	auditStore    audit.AuditStore
	logger        logr.Logger
	maxTurns      int
	phaseTools    katypes.PhaseToolMap
	registry      registry.ToolRegistry
	pipeline      Pipeline
	modelName     string
	scopeResolver ScopeResolver
	swappable     *llm.SwappableClient
	metrics       *metrics.Metrics
	pinDecorator  func(llm.Client) llm.Client
	phaseResolver PhaseClientResolver
}

func (inv *Investigator) auditLog() logr.Logger {
	return inv.logger
}

// resolveForPhase returns the LLM client, model name, and runtime params for
// the given investigation phase. When a PhaseClientResolver is configured, it
// delegates to the resolver (which may return a phase-specific client).
// Otherwise, falls back to the legacy Swappable + PinDecorator pattern.
//
// The legacy branch captures client/model/params via a single
// SwappableClient.Pin() call so the three describe the same Swap version —
// calling Snapshot/ModelName/RuntimeParameters separately would risk a torn
// read if a hot-reload landed between calls (#1610).
func (inv *Investigator) resolveForPhase(phase katypes.Phase) (llm.Client, string, llm.RuntimeParams) {
	if inv.phaseResolver != nil {
		return inv.phaseResolver.ResolvePhase(phase)
	}
	client := inv.client
	modelName := inv.modelName
	var runtimeParams llm.RuntimeParams
	if inv.swappable != nil {
		snap := inv.swappable.Pin()
		if inv.pinDecorator != nil {
			client = inv.pinDecorator(snap.Client)
			if client == nil {
				client = llm.NewInstrumentedClient(snap.Client)
			}
		} else {
			client = llm.NewInstrumentedClient(snap.Client)
		}
		modelName = snap.ModelName
		runtimeParams = snap.RuntimeParams
	}
	return client, modelName, runtimeParams
}

// New creates an Investigator from the given configuration.
// Config.Registry may be nil (tool execution will be skipped).
// Config.Pipeline fields default to nil (their features are skipped).
func New(cfg Config) *Investigator {
	pipeline := cfg.Pipeline
	if pipeline.AnomalyDetector == nil {
		pipeline.AnomalyDetector = NewAnomalyDetector(DefaultAnomalyConfig(), nil)
	}
	return &Investigator{
		client:        cfg.Client,
		builder:       cfg.Builder,
		resultParser:  cfg.ResultParser,
		enricher:      cfg.Enricher,
		auditStore:    cfg.AuditStore,
		logger:        cfg.Logger,
		maxTurns:      cfg.MaxTurns,
		phaseTools:    cfg.PhaseTools,
		registry:      cfg.Registry,
		pipeline:      pipeline,
		modelName:     cfg.ModelName,
		scopeResolver: cfg.ScopeResolver,
		swappable:     cfg.Swappable,
		metrics:       cfg.Metrics,
		pinDecorator:  cfg.PinDecorator,
		phaseResolver: cfg.PhaseResolver,
	}
}

// RunInteractiveTurn executes a single interactive LLM loop iteration.
// Used by the MCP kubernaut_investigate tool for interactive sessions.
// Uses PhaseRCA tool set. Streaming works via LazySink on context.
func (inv *Investigator) RunInteractiveTurn(ctx context.Context, messages []llm.Message, correlationID string) (LoopResult, error) {
	client, modelName, runtimeParams := inv.resolveForPhase(katypes.PhaseRCA)
	return inv.runLLMLoop(ctx, messages, katypes.PhaseRCA, LLMInvocationContext{
		CorrelationID: correlationID,
		Client:        client,
		ModelName:     modelName,
		RuntimeParams: runtimeParams,
	})
}

// RunRCAExtractionFromConversation appends a submit-RCA prompt to an existing
// conversation and runs a single LLM call with only submit_result as the
// available tool. Returns the parsed InvestigationResult (RCA only, no workflow).
// Used by discover_workflows to extract structured RCA from interactive history.
func (inv *Investigator) RunRCAExtractionFromConversation(ctx context.Context, messages []llm.Message, correlationID string) (*katypes.InvestigationResult, error) {
	_ = correlationID // reserved for future audit event emission
	client, _, runtimeParams := inv.resolveForPhase(katypes.PhaseRCA)

	submitOnlyTools := []llm.ToolDefinition{
		{
			Name:        SubmitResultToolName,
			Description: "Submit your root cause analysis findings as a structured result.",
			Parameters:  parser.RCAResultSchema(),
		},
	}

	messages = append(messages, llm.Message{
		Role:    "user",
		Content: "Based on your investigation so far, please submit your root cause analysis findings using the submit_result tool.",
	})

	req := llm.ChatRequest{
		Messages: messages,
		Tools:    submitOnlyTools,
	}

	resp, err := llm.ChatWithParams(ctx, client, req, runtimeParams)
	if err != nil {
		return nil, fmt.Errorf("RCA extraction LLM call: %w", err)
	}

	var content string
	if len(resp.ToolCalls) > 0 && resp.ToolCalls[0].Name == SubmitResultToolName {
		content = resp.ToolCalls[0].Arguments
	} else {
		content = resp.Message.Content
	}

	result, parseErr := inv.resultParser.Parse(content)
	if parseErr != nil {
		return nil, fmt.Errorf("RCA extraction parse: %w", parseErr)
	}
	return result, nil
}

// Investigate runs the two-invocation investigation and returns the result.
// Per BR-AUDIT-005, all audit events use signal.RemediationID as correlation ID
// so that DataStorage queries by remediation_id return the full investigation trail.
func (inv *Investigator) Investigate(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error) {
	defer inv.startDiagSummary(ctx)()
	inv.pipeline.AnomalyDetector.Reset()

	// #783 + #1470: Pin client per phase. Each phase resolves its own client,
	// model name, and runtime params. Subsequent hot-reload swaps do not
	// affect in-flight work.
	rcaClient, rcaModelName, rcaRuntimeParams := inv.resolveForPhase(katypes.PhaseRCA)

	correlationID := signal.RemediationID
	enrichmentCache := make(map[string]*enrichment.EnrichmentResult)

	signalKind, signalName, signalNS := ResolveEnrichmentTarget(signal, nil)
	signalNS = inv.normalizeNamespace(signalKind, signalNS)
	enrichData := inv.resolveEnrichmentCached(ctx, enrichmentCache, signalKind, signalName, signalNS, signal.IncidentID)
	promptEnrichment := toPromptEnrichment(enrichData)
	tokens := &TokenAccumulator{}

	rcaResult, err := inv.runRCA(ctx, signal, promptEnrichment, LLMInvocationContext{
		Tokens:        tokens,
		CorrelationID: correlationID,
		Client:        rcaClient,
		ModelName:     rcaModelName,
		RuntimeParams: rcaRuntimeParams,
	})
	if err != nil {
		return nil, fmt.Errorf("RCA invocation: %w", err)
	}

	if rcaResult.Cancelled {
		inv.emitCancellationAudit(ctx, rcaResult, correlationID)
		return rcaResult, nil
	}

	inv.emitRCAComplete(ctx, rcaResult, tokens, correlationID)

	if early, done := inv.checkRCAEarlyReturn(ctx, rcaResult, signal, enrichData, tokens, correlationID); done {
		return early, nil
	}

	return inv.runWorkflowDiscoveryPhase(ctx, workflowDiscoveryPhaseParams{
		Signal: signal, RCAResult: rcaResult, EnrichData: enrichData, EnrichmentCache: enrichmentCache,
		SignalKind: signalKind, SignalName: signalName, SignalNS: signalNS, Tokens: tokens, CorrelationID: correlationID,
	})
}

// workflowDiscoveryPhaseParams groups the fields needed by
// runWorkflowDiscoveryPhase. Extracted per AGENTS.md's 8+-param
// Options-pattern rule.
type workflowDiscoveryPhaseParams struct {
	Signal                           katypes.SignalContext
	RCAResult                        *katypes.InvestigationResult
	EnrichData                       *enrichment.EnrichmentResult
	EnrichmentCache                  map[string]*enrichment.EnrichmentResult
	SignalKind, SignalName, SignalNS string
	Tokens                           *TokenAccumulator
	CorrelationID                    string
}

// runWorkflowDiscoveryPhase runs Phase 3 (workflow discovery/selection) of
// Investigate: GAP-001/ADR-056 target re-enrichment, workflow-signal
// enrichment, and the workflow-selection LLM invocation, finalizing the
// result via mergeAndFinalizeWorkflowResult.
func (inv *Investigator) runWorkflowDiscoveryPhase(ctx context.Context, p workflowDiscoveryPhaseParams) (*katypes.InvestigationResult, error) {
	// GAP-001 / ADR-056: Re-enrich using RCA-identified remediation target if
	// different. H3-fix: retain pre-RCA enrichment if re-enrichment fails.
	workflowSignal, enrichData, hardFailed := inv.reEnrichWorkflowTarget(ctx, reEnrichWorkflowTargetParams{
		Signal: p.Signal, RCAResult: p.RCAResult, EnrichData: p.EnrichData, EnrichmentCache: p.EnrichmentCache,
		SignalKind: p.SignalKind, SignalName: p.SignalName, SignalNS: p.SignalNS,
	})
	if hardFailed {
		return inv.finalizeAndEmitRCAOnlyResult(ctx, p.RCAResult, p.Signal, enrichData, p.Tokens, p.CorrelationID), nil
	}
	promptEnrichment := toPromptEnrichment(enrichData)
	workflowSignal = inv.enrichWorkflowSignalForDiscovery(workflowSignal, p.Signal, p.RCAResult, enrichData, p.CorrelationID)

	inv.pipeline.AnomalyDetector.Reset()

	wfClient, wfModelName, wfRuntimeParams := inv.resolveForPhase(katypes.PhaseWorkflowDiscovery)

	p1Ctx := BuildPhase1Context(p.RCAResult)

	workflowResult, err := inv.runWorkflowSelection(ctx, workflowSignal, p.RCAResult.RCASummary, promptEnrichment, p1Ctx, LLMInvocationContext{
		Tokens:        p.Tokens,
		CorrelationID: p.CorrelationID,
		Client:        wfClient,
		ModelName:     wfModelName,
		RuntimeParams: wfRuntimeParams,
	})
	if err != nil {
		return nil, fmt.Errorf("workflow selection invocation: %w", err)
	}

	if workflowResult.Cancelled {
		inv.emitCancellationAudit(ctx, workflowResult, p.CorrelationID)
		return workflowResult, nil
	}

	return inv.mergeAndFinalizeWorkflowResult(ctx, mergeAndFinalizeWorkflowResultParams{
		WorkflowResult: workflowResult, RCAResult: p.RCAResult, Signal: p.Signal, WorkflowSignal: workflowSignal,
		EnrichData: enrichData, P1Ctx: p1Ctx, Tokens: p.Tokens, CorrelationID: p.CorrelationID,
	}), nil
}

// startDiagSummary resets the emitToSink diagnostic counters and returns a
// func to defer that logs the accumulated sent/dropped/nil-sink counts at
// Investigate's exit.
func (inv *Investigator) startDiagSummary(ctx context.Context) func() {
	diagSendOK.Store(0)
	diagSendDrop.Store(0)
	diagSinkNil.Store(0)
	return func() {
		inv.logger.Info("DIAG emitToSink summary",
			"session_id", session.SessionIDFromContext(ctx),
			"sent", diagSendOK.Load(),
			"dropped", diagSendDrop.Load(),
			"sink_nil", diagSinkNil.Load(),
			"sink_ptr", fmt.Sprintf("%p", session.EventSinkFromContext(ctx)))
	}
}

// checkRCAEarlyReturn evaluates the interactive-hold (BR-INTERACTIVE-010 /
// #1390), human-review-needed, and not-actionable (#1430 / BR-HAPI-200)
// short-circuit conditions after RCA completes. Returns done=true when
// Investigate must return result immediately without running workflow
// discovery.
func (inv *Investigator) checkRCAEarlyReturn(ctx context.Context, rcaResult *katypes.InvestigationResult, signal katypes.SignalContext, enrichData *enrichment.EnrichmentResult, tokens *TokenAccumulator, correlationID string) (result *katypes.InvestigationResult, done bool) {
	if signal.Interactive || session.InteractiveUpgradeFromContext(ctx) {
		rcaResult.InteractiveHold = true
		return inv.finalizeAndEmitRCAOnlyResult(ctx, rcaResult, signal, enrichData, tokens, correlationID), true
	}
	if rcaResult.HumanReviewNeeded {
		return inv.finalizeAndEmitRCAOnlyResult(ctx, rcaResult, signal, enrichData, tokens, correlationID), true
	}
	// Guard: only short-circuit not-actionable when no workflow was already
	// identified by the RCA (defense-in-depth against LLM self-contradiction).
	if rcaResult.IsActionable != nil && !*rcaResult.IsActionable && rcaResult.WorkflowID == "" {
		inv.logger.Info("skipping workflow discovery: RCA concluded not actionable",
			"investigation_outcome", rcaResult.InvestigationOutcome,
			"correlation_id", correlationID)
		return inv.finalizeAndEmitRCAOnlyResult(ctx, rcaResult, signal, enrichData, tokens, correlationID), true
	}
	return nil, false
}

// finalizeAndEmitRCAOnlyResult applies the common Phase-1-only finalization
// steps (severity backfill, label attachment, remediation-target injection,
// audit emission) shared by every early-return path in Investigate: the
// interactive-hold, human-review-needed, not-actionable, and re-enrichment
// hard-fail branches. Callers set any branch-specific fields on rcaResult
// before invoking this helper.
func (inv *Investigator) finalizeAndEmitRCAOnlyResult(ctx context.Context, rcaResult *katypes.InvestigationResult, signal katypes.SignalContext, enrichData *enrichment.EnrichmentResult, tokens *TokenAccumulator, correlationID string) *katypes.InvestigationResult {
	backfillSeverity(rcaResult, signal)
	attachDetectedLabels(rcaResult, enrichData)
	InjectRemediationTarget(rcaResult, signal, enrichData)
	InjectTargetResourceParameters(rcaResult)
	inv.emitResponseComplete(ctx, rcaResult, tokens, correlationID)
	return rcaResult
}

// reEnrichWorkflowTargetParams groups the fields needed by
// reEnrichWorkflowTarget. Extracted per AGENTS.md's 8+-param
// Options-pattern rule.
type reEnrichWorkflowTargetParams struct {
	Signal                           katypes.SignalContext
	RCAResult                        *katypes.InvestigationResult
	EnrichData                       *enrichment.EnrichmentResult
	EnrichmentCache                  map[string]*enrichment.EnrichmentResult
	SignalKind, SignalName, SignalNS string
}

// reEnrichWorkflowTarget re-enriches using the RCA-identified remediation
// target when it differs from the pre-RCA signal target (GAP-001/ADR-056).
// hardFailed is true when the re-enrichment owner-chain lookup hard-failed;
// callers must treat rcaResult (already marked HumanReviewNeeded/rca_incomplete
// by this function) as the final result in that case.
func (inv *Investigator) reEnrichWorkflowTarget(ctx context.Context, p reEnrichWorkflowTargetParams) (workflowSignal katypes.SignalContext, updatedEnrichData *enrichment.EnrichmentResult, hardFailed bool) {
	signal, rcaResult, enrichData := p.Signal, p.RCAResult, p.EnrichData
	enrichmentCache := p.EnrichmentCache
	signalKind, signalName, signalNS := p.SignalKind, p.SignalName, p.SignalNS
	workflowSignal = signal
	postRCAKind, postRCAName, postRCANS := ResolveEnrichmentTarget(signal, rcaResult)
	postRCANS = inv.normalizeNamespace(postRCAKind, postRCANS)

	targetUnchanged := postRCAKind == signalKind && postRCAName == signalName && postRCANS == signalNS
	if targetUnchanged {
		if enrichData != nil && enrichData.TargetResourceDeleted {
			rcaResult.Warnings = append(rcaResult.Warnings,
				deletedResourceWarning(signalKind, signalName, signalNS))
		}
		return workflowSignal, enrichData, false
	}

	enrichData, hardFailed = inv.applyReEnrichedTarget(ctx, reEnrichedTargetParams{
		EnrichmentCache: enrichmentCache, RCAResult: rcaResult, WorkflowSignal: &workflowSignal,
		EnrichData: enrichData, Signal: signal, SignalKind: signalKind, SignalName: signalName,
		PostRCAKind: postRCAKind, PostRCAName: postRCAName, PostRCANS: postRCANS,
	})
	return workflowSignal, enrichData, hardFailed
}

// reEnrichedTargetParams groups the fields needed by applyReEnrichedTarget.
// Extracted per AGENTS.md's 8+-param Options-pattern rule.
type reEnrichedTargetParams struct {
	EnrichmentCache                     map[string]*enrichment.EnrichmentResult
	RCAResult                           *katypes.InvestigationResult
	WorkflowSignal                      *katypes.SignalContext
	EnrichData                          *enrichment.EnrichmentResult
	Signal                              katypes.SignalContext
	SignalKind, SignalName              string
	PostRCAKind, PostRCAName, PostRCANS string
}

// applyReEnrichedTarget handles the RCA-target-changed branch of
// reEnrichWorkflowTarget: re-enriches using the post-RCA target, applies the
// hard-fail/deleted-resource/label-preservation rules (BR-HAPI-261 AC#7,
// #704), and updates p.WorkflowSignal's resource identity in place. Returns
// hardFailed=true when the caller must treat rcaResult as final.
func (inv *Investigator) applyReEnrichedTarget(ctx context.Context, p reEnrichedTargetParams) (updatedEnrichData *enrichment.EnrichmentResult, hardFailed bool) {
	enrichData := p.EnrichData
	inv.logger.Info("re-enriching with RCA remediation target",
		"signal", p.SignalKind+"/"+p.SignalName,
		"rca_target", p.PostRCAKind+"/"+p.PostRCAName,
	)
	reEnriched := inv.resolveEnrichmentCached(ctx, p.EnrichmentCache, p.PostRCAKind, p.PostRCAName, p.PostRCANS, p.Signal.IncidentID)

	// BR-HAPI-261 AC#7 / #704: check HardFail BEFORE label merge
	// to prevent the merge from silently dropping the failure signal.
	// Use enrichData (initial enrichment) for labels because the failed
	// re-enrichment has empty/all-failed detections — preserving signal-level
	// labels (e.g. pdbProtected) matches pre-#704 behaviour where
	// allLabelDetectionsFailed() fell through to keep initial labels.
	if reEnriched != nil && reEnriched.HardFail {
		inv.logger.Error(reEnriched.OwnerChainError, "enrichment owner chain hard-failed, triggering rca_incomplete")
		p.RCAResult.HumanReviewNeeded = true
		p.RCAResult.HumanReviewReason = "rca_incomplete"
		return enrichData, true
	}

	if reEnriched != nil && reEnriched.TargetResourceDeleted {
		p.RCAResult.Warnings = append(p.RCAResult.Warnings,
			deletedResourceWarning(p.PostRCAKind, p.PostRCAName, p.PostRCANS))
	}

	switch {
	case reEnriched != nil && !allLabelDetectionsFailed(reEnriched.DetectedLabels):
		enrichData = reEnriched
	case reEnriched != nil:
		inv.logger.Info("re-enrichment labels all failed (RCA target not found), preserving signal-target labels",
			"rca_target", p.PostRCAKind+"/"+p.PostRCAName)
	default:
		inv.logger.Info("re-enrichment returned nil, retaining pre-RCA enrichment data")
	}

	p.WorkflowSignal.ResourceKind = p.PostRCAKind
	p.WorkflowSignal.ResourceName = p.PostRCAName
	p.WorkflowSignal.Namespace = p.PostRCANS
	return enrichData, false
}

// enrichWorkflowSignalForDiscovery propagates the RCA-resolved apiVersion and
// detected-label JSON onto workflowSignal so that ComponentGVK() and DS
// catalog queries see the correct, GitOps-aware target for Phase 3 (#1051,
// #1052 / BR-AI-056).
func (inv *Investigator) enrichWorkflowSignalForDiscovery(workflowSignal, signal katypes.SignalContext, rcaResult *katypes.InvestigationResult, enrichData *enrichment.EnrichmentResult, correlationID string) katypes.SignalContext {
	// When APIVersion is empty but the RCA changed the target kind, clear any
	// stale ResourceAPIVersion to prevent an invalid GVK combination (old
	// apiVersion + new kind).
	if rcaResult.RemediationTarget.APIVersion != "" {
		workflowSignal.ResourceAPIVersion = rcaResult.RemediationTarget.APIVersion
	} else if workflowSignal.ResourceKind != signal.ResourceKind {
		workflowSignal.ResourceAPIVersion = ""
	}

	if enrichData != nil && enrichData.DetectedLabels != nil {
		if dlJSON, err := json.Marshal(enrichData.DetectedLabels); err == nil {
			workflowSignal.DetectedLabelsJSON = string(dlJSON)
			dl := enrichData.DetectedLabels
			trueCount := countTrueLabels(dl.GitOpsManaged, dl.PDBProtected, dl.HPAEnabled,
				dl.Stateful, dl.HelmManaged, dl.NetworkIsolated, dl.ResourceQuotaConstrained)
			inv.logger.V(1).Info("detected labels attached for workflow discovery scoring",
				"correlation_id", correlationID,
				"true_label_count", trueCount,
				"gitops_tool", dl.GitOpsTool)
		} else {
			inv.logger.Error(err, "failed to marshal detected labels for workflow discovery, scoring will be inactive",
				"correlation_id", correlationID)
		}
	}
	return workflowSignal
}

// mergeAndFinalizeWorkflowResultParams groups the fields needed by
// mergeAndFinalizeWorkflowResult. Extracted per AGENTS.md's 8+-param
// Options-pattern rule.
type mergeAndFinalizeWorkflowResultParams struct {
	WorkflowResult, RCAResult *katypes.InvestigationResult
	Signal, WorkflowSignal    katypes.SignalContext
	EnrichData                *enrichment.EnrichmentResult
	P1Ctx                     *prompt.Phase1Data
	Tokens                    *TokenAccumulator
	CorrelationID             string
}

// mergeAndFinalizeWorkflowResult merges Phase 1 (RCA) fallback fields into
// the Phase 3 workflow-selection result, then applies the same finalization
// steps as finalizeAndEmitRCAOnlyResult before emitting the audit trail.
func (inv *Investigator) mergeAndFinalizeWorkflowResult(ctx context.Context, p mergeAndFinalizeWorkflowResultParams) *katypes.InvestigationResult {
	workflowResult, rcaResult := p.WorkflowResult, p.RCAResult
	signal, workflowSignal := p.Signal, p.WorkflowSignal
	enrichData, p1Ctx, tokens, correlationID := p.EnrichData, p.P1Ctx, p.Tokens, p.CorrelationID
	if workflowResult.RCASummary == "" {
		workflowResult.RCASummary = rcaResult.RCASummary
	}
	if workflowResult.SignalName == "" && rcaResult.SignalName != "" {
		workflowResult.SignalName = rcaResult.SignalName
	}
	if len(rcaResult.Warnings) > 0 {
		workflowResult.Warnings = append(rcaResult.Warnings, workflowResult.Warnings...)
	}
	// BR-AI-086 AC6: RCA reasoning is Phase-1-specific by definition (it
	// explains the root cause analysis, not the workflow selection), so it
	// is copied by reference here — not routed through Phase1Data/the Phase
	// 3 prompt template, which would leak Phase 1's raw reasoning text into
	// Phase 3's LLM context and violate the phase-isolation invariant
	// (IT-KA-1578-002).
	if workflowResult.Reasoning == nil && rcaResult.Reasoning != nil {
		workflowResult.Reasoning = rcaResult.Reasoning
	}

	MergePhase1Fallbacks(workflowResult, p1Ctx)

	backfillSeverity(workflowResult, signal)
	attachDetectedLabels(workflowResult, enrichData)
	InjectRemediationTarget(workflowResult, workflowSignal, enrichData)
	// Issue #1044: propagate RCA-identified api_version to the workflow result
	// when the workflow selection didn't produce one. The RCA gate ensures
	// api_version is set for ambiguous kinds; this must survive through Phase 3.
	if workflowResult.RemediationTarget.APIVersion == "" && rcaResult.RemediationTarget.APIVersion != "" {
		workflowResult.RemediationTarget.APIVersion = rcaResult.RemediationTarget.APIVersion
	}
	InjectTargetResourceParameters(workflowResult)
	inv.emitResponseComplete(ctx, workflowResult, tokens, correlationID)
	return workflowResult
}

func deletedResourceWarning(kind, name, ns string) string {
	return fmt.Sprintf("target resource %s/%s in %s was deleted; enrichment data is sparse", kind, name, ns)
}

func countTrueLabels(flags ...bool) int {
	n := 0
	for _, f := range flags {
		if f {
			n++
		}
	}
	return n
}

// backfillSeverity ensures InvestigationResult.Severity is never empty.
// If the LLM didn't provide severity, fall back to the signal's severity.
// If still empty, use "unknown" to satisfy the CRD enum validation.
func backfillSeverity(result *katypes.InvestigationResult, signal katypes.SignalContext) {
	if result.Severity != "" {
		return
	}
	if signal.Severity != "" {
		result.Severity = signal.Severity
		return
	}
	result.Severity = "unknown"
}

// ResolveEnrichmentTarget determines the K8s resource to enrich.
// Per GAP-001 / ADR-056: after RCA, the parser may identify a different
// remediation target than the signal resource (e.g., signal=Pod but RCA
// identifies Deployment as root cause). This function prefers the RCA target
// and falls back to the signal resource.
func ResolveEnrichmentTarget(signal katypes.SignalContext, rcaResult *katypes.InvestigationResult) (kind, name, namespace string) {
	if rcaResult != nil && rcaResult.RemediationTarget.Kind != "" {
		return rcaResult.RemediationTarget.Kind, rcaResult.RemediationTarget.Name, rcaResult.RemediationTarget.Namespace
	}
	kind = signal.ResourceKind
	if kind == "" {
		kind = "Pod"
	}
	// C1-fix: Use ResourceName (K8s object identity), not Name (signal type like "OOMKilled").
	// Fall back to Name only when ResourceName is not available.
	name = signal.ResourceName
	if name == "" {
		name = signal.Name
	}
	return kind, name, signal.Namespace
}

func (inv *Investigator) resolveEnrichment(ctx context.Context, kind, name, namespace, incidentID string) *enrichment.EnrichmentResult {
	if inv.enricher == nil {
		return nil
	}
	result, err := inv.enricher.Enrich(ctx, kind, name, namespace, "", "", incidentID)
	if err != nil {
		inv.logger.Error(err, "enrichment failed")
		return nil
	}
	return result
}

// LLMInvocationContext groups the LLM call parameters shared by the
// investigation pipeline's internal phase-runner and retry-gate methods:
// token accounting, correlation, the resolved client/model for the current
// phase, and runtime params. Extracted per AGENTS.md's 8+-param
// Options-pattern rule.
type LLMInvocationContext struct {
	Tokens        *TokenAccumulator
	CorrelationID string
	Client        llm.Client
	ModelName     string
	RuntimeParams llm.RuntimeParams
}
