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
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
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
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

const maxSelfCorrectionAttempts = 3

// DefaultToolCallTimeout bounds a single LLM-requested tool call dispatched
// from processToolCalls when Config.ToolCallTimeout is unset (zero value),
// mirroring the AnomalyDetector nil-fallback below (BR-KA-267, #1949).
const DefaultToolCallTimeout = 60 * time.Second

// DefaultLLMCallTimeout bounds a single streaming LLM call dispatched from
// chatOrStream when RuntimeParams.TimeoutSeconds is unset/non-positive
// (BR-KA-267, #1949). Previously that condition left the call with no
// deadline at all — the parent ctx was passed through unbounded — so a
// stalled provider connection hung the investigation goroutine indefinitely.
// The value mirrors types.DefaultLLMTimeoutSeconds, the equivalent fallback
// already used by the non-streaming ChatWithParams/HTTP-client call chains.
const DefaultLLMCallTimeout = 120 * time.Second

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

// rcaLoopBudget is shared by the initial RCA and server-assisted history
// continuations so the whole RCA investigation uses one max-turn budget.
type rcaLoopBudget struct {
	used int
}

// SubmitResult is returned when the LLM calls the generic submit_result tool
// in the RCA phase. Messages carries the paired conversation turns accumulated
// before the sentinel call; the sentinel itself is intentionally not replayed.

type SubmitResult struct {
	Content   string
	Reasoning *llm.ReasoningBlock
	Messages  []llm.Message
}

func (*SubmitResult) loopResult() {}

// SubmitWithWorkflowResult is returned when the LLM calls submit_result_with_workflow.
type SubmitWithWorkflowResult struct {
	Content   string
	Reasoning *llm.ReasoningBlock
	Messages  []llm.Message
}

func (*SubmitWithWorkflowResult) loopResult() {}

// SubmitNoWorkflowResult is returned when the LLM calls submit_result_no_workflow.
type SubmitNoWorkflowResult struct {
	Content   string
	Reasoning *llm.ReasoningBlock
	Messages  []llm.Message
}

func (*SubmitNoWorkflowResult) loopResult() {}

// TextResult is returned when the LLM responds with plain text (no tool call).
// Messages carries the fully-paired assistant/tool turns accumulated by
// runLLMLoop prior to this final text turn (#1935/#1936 root cause #2).
type TextResult struct {
	Content   string
	Reasoning *llm.ReasoningBlock
	Messages  []llm.Message
}

func (*TextResult) loopResult() {}

// ReasonToolBudgetExhausted is the ExhaustedResult.Reason value set when the
// AnomalyDetector's total tool-call budget trips mid-investigation. Exported
// as a named constant (rather than a bare string literal) so
// adapters.ExtractContent can match on it precisely to surface
// tools.ErrCodeToolBudgetExhausted instead of a generic internal error
// (#1889 gap 1).
const ReasonToolBudgetExhausted = "tool budget exhausted"

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

// loopResultMessages extracts the accumulated message history from a
// LoopResult, for the concrete types that carry one (#1935/#1936 root cause
// #2). CancelledResult and ExhaustedResult are handled separately by their
// own callers (cancellation snapshot / abort path) and return nil here.
func loopResultMessages(r LoopResult) []llm.Message {
	switch v := r.(type) {
	case *SubmitResult:
		return v.Messages
	case *TextResult:
		return v.Messages
	case *SubmitWithWorkflowResult:
		return v.Messages
	case *SubmitNoWorkflowResult:
		return v.Messages
	default:
		return nil
	}
}

// sentinelResult maps a sentinel tool call to its LoopResult type, attaching
// the reasoning block (if any) from the turn whose response produced tc, so
// reasoning captured at the LLM-client layer (BR-AI-086 AC1-3) reaches the
// InvestigationResult that gets built from the sentinel's Content. messages
// is the history accumulated so far — i.e. every PRIOR, fully-paired
// assistant(tool_use)/tool(tool_result) turn, NOT including the sentinel
// call itself (tc). Deliberately excluding the sentinel call avoids ever
// replaying a dangling, unpaired tool_use block to the LLM on a later
// retry — the sentinel is consumed here, never executed as a real tool, so
// it has no matching tool_result (#1935/#1936 root cause #2).
func sentinelResult(tc llm.ToolCall, reasoning *llm.ReasoningBlock, messages []llm.Message) LoopResult {
	switch tc.Name {
	case SubmitResultToolName:
		return &SubmitResult{Content: tc.Arguments, Reasoning: reasoning, Messages: messages}
	case SubmitResultWithWorkflowToolName:
		return &SubmitWithWorkflowResult{Content: tc.Arguments, Reasoning: reasoning, Messages: messages}
	case SubmitResultNoWorkflowToolName:
		return &SubmitNoWorkflowResult{Content: tc.Arguments, Reasoning: reasoning, Messages: messages}
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
	// DD-KA-2496: applies to the complete frozen packet, never tool results.
	MaxEvidenceHandoffBytes int
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
	// FleetOverlayResolver resolves the target cluster's tools into the
	// generic-name overlay consulted by toolDefinitionsForPhase/executeTool
	// (DD-FLEET-005 cluster-transparent tool exposure). When nil (fleet
	// disabled), Investigate never attempts pre-scoping and every
	// investigation behaves exactly as it did before DD-FLEET-005.
	FleetOverlayResolver FleetOverlayResolver
	// ResolvedConfidenceThreshold and InconclusiveConfidenceThreshold are the
	// operator-configurable investigation-outcome confidence bands (BR-KA-213,
	// Issue #1826) threaded into the Phase 3 workflow-selection prompt. Zero
	// means "not configured" — RenderWorkflowSelection substitutes its V1.0
	// defaults (0.7/0.5) in that case; see prompt.WorkflowSelectionInput.
	ResolvedConfidenceThreshold     float64
	InconclusiveConfidenceThreshold float64
	// ToolCallTimeout bounds a single LLM-requested tool call dispatched from
	// processToolCalls (BR-KA-267, #1949). Zero falls back to
	// DefaultToolCallTimeout in New().
	ToolCallTimeout time.Duration
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
	// fleetOverlayResolver mirrors Config.FleetOverlayResolver; see its doc
	// comment. Read-only after construction — Investigator is a singleton
	// shared across all concurrent investigations, so this must never be
	// mutated post-New (DD-FLEET-005 preflight finding).
	fleetOverlayResolver FleetOverlayResolver
	// resolvedConfidenceThreshold and inconclusiveConfidenceThreshold mirror
	// Config's fields of the same name (BR-KA-213, Issue #1826).
	resolvedConfidenceThreshold     float64
	inconclusiveConfidenceThreshold float64
	// anomalyScope hands out a per-correlationID AnomalyDetector clone so
	// concurrent investigations sharing this Investigator never corrupt each
	// other's tool-call budgets (#1892). pipeline.AnomalyDetector remains the
	// config template; production code must go through anomalyDetectorFor
	// rather than pipeline.AnomalyDetector directly.
	anomalyScope *anomalyScope
	// toolCallTimeout mirrors Config.ToolCallTimeout; see its doc comment
	// (BR-KA-267, #1949). Always non-zero after New().
	toolCallTimeout time.Duration
	// metricsScope hands out a per-correlationID InvestigationMetrics so
	// concurrent investigations sharing this Investigator never corrupt each
	// other's LLM-turn/tool-call counts (#2387). Mirrors anomalyScope: the
	// singleton Investigator must never hold a single shared counter.
	metricsScope *metricsScope
	// tokenScope accumulates per-correlationID token usage across every leg
	// of a remediation request for console reporting (#2387 tokens; raw
	// provider counts, never costs). Unlike TokenAccumulator (audit-only,
	// single-Investigate threading), this spans interactive and discovery
	// legs that carry no accumulator at all.
	tokenScope *tokenScope
	// DD-KA-2496: ephemeral, correlation-scoped pre-transform evidence; never
	// an audit/conversation lookup or a singleton shared observation slice.
	evidence *evidenceRuntime
}

// FleetOverlayResolver returns the configured authoritative fleet overlay
// resolver for interactive workflow-selection flows that run outside the main
// Investigator pipeline.
func (inv *Investigator) FleetOverlayResolver() FleetOverlayResolver {
	return inv.fleetOverlayResolver
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
	toolCallTimeout := cfg.ToolCallTimeout
	if toolCallTimeout <= 0 {
		toolCallTimeout = DefaultToolCallTimeout
	}
	phaseTools := cfg.PhaseTools
	if phaseTools == nil {
		// DD-KA-2496: omitted configuration gets the same strict policy for
		// advertisement and dispatch, never an unrestricted registry fallback.
		phaseTools = DefaultPhaseToolMap()
	}
	return &Investigator{
		client:                          cfg.Client,
		builder:                         cfg.Builder,
		resultParser:                    cfg.ResultParser,
		enricher:                        cfg.Enricher,
		auditStore:                      cfg.AuditStore,
		logger:                          cfg.Logger,
		maxTurns:                        cfg.MaxTurns,
		phaseTools:                      phaseTools,
		registry:                        cfg.Registry,
		pipeline:                        pipeline,
		modelName:                       cfg.ModelName,
		scopeResolver:                   cfg.ScopeResolver,
		swappable:                       cfg.Swappable,
		metrics:                         cfg.Metrics,
		pinDecorator:                    cfg.PinDecorator,
		phaseResolver:                   cfg.PhaseResolver,
		fleetOverlayResolver:            cfg.FleetOverlayResolver,
		resolvedConfidenceThreshold:     cfg.ResolvedConfidenceThreshold,
		inconclusiveConfidenceThreshold: cfg.InconclusiveConfidenceThreshold,
		anomalyScope: &anomalyScope{
			template: pipeline.AnomalyDetector,
			entries:  make(map[string]*anomalyDetectorEntry),
		},
		metricsScope:    newMetricsScope(),
		tokenScope:      newTokenScope(),
		toolCallTimeout: toolCallTimeout,
		evidence:        newEvidenceRuntime(pipeline),
	}
}

// RunInteractiveTurn executes a single interactive LLM loop iteration.
// Used by the MCP kubernaut_investigate tool for interactive sessions.
// Uses PhaseRCA tool set. Streaming works via LazySink on context.
//
// Issue #1768 Gap D: an interactive session's tool calls must be pre-scoped
// to the operator's actual target cluster, exactly like Investigate()'s
// autonomous path (DD-FLEET-005), instead of silently defaulting to the hub
// cluster's tools. InvestigateTool.handleMessage already resolves the
// target ClusterID via signalResolver and attaches it to ctx on every turn
// (#1374/F9, katypes.WithSignalContext) — this reuses that existing value
// rather than threading a new parameter through the MCP dispatch path.
//
// Pre-scoping is skipped entirely (byte-identical to pre-#1768 behavior) if
// ctx carries no SignalContext at all — hub-local sessions, and the
// reconstruction-turn replay path (ReconRunnerAdapter.RunReconTurn, see
// TEST_PLAN_TRACK2.md §6 out-of-scope).
//
// FAIL-CLOSED for fleet-target turns (amendment 2026-08-30, see
// prescopeFleetOverlay's doc comment and DD-FLEET-005): if a SignalContext IS
// present but overlay resolution fails, this turn returns an error instead
// of silently falling back to the local tool registry — the hub is never the
// cluster an interactive operator is actually driving against.
func (inv *Investigator) RunInteractiveTurn(ctx context.Context, messages []llm.Message, correlationID string) (LoopResult, error) {
	if signal, ok := katypes.SignalContextFromContext(ctx); ok {
		var err error
		ctx, err = inv.prescopeFleetOverlay(ctx, signal.ClusterID, correlationID)
		if err != nil {
			return nil, err
		}
	}
	client, modelName, runtimeParams := inv.resolveForPhase(katypes.PhaseRCA)
	return inv.runLLMLoop(ctx, messages, katypes.PhaseRCA, LLMInvocationContext{
		CorrelationID:  correlationID,
		Client:         client,
		ModelName:      modelName,
		RuntimeParams:  runtimeParams,
		decisionLoopID: "interactive",
	})
}

// RunRCAExtractionFromConversation appends a submit-RCA prompt to an existing
// conversation and runs a single LLM call with only submit_result as the
// available tool. Returns the parsed InvestigationResult (RCA only, no workflow).
// Used by discover_workflows to extract structured RCA from interactive history.
func (inv *Investigator) RunRCAExtractionFromConversation(ctx context.Context, messages []llm.Message, correlationID string) (*katypes.InvestigationResult, error) {
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

	// #2088 (main port of #2086): this extraction call is non-streamed and
	// emits no sink events for its duration. discover_workflows callers can
	// wait through this call while AF's bridge-inactivity timer is running,
	// so a keepalive is required to prevent AF from falsely reporting
	// completion.
	emitGateRetryKeepalive(ctx, "extracting_rca_from_conversation")

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
	// #2387: this single submit-only extraction call is one LLM turn with no
	// real tool dispatches. Recorded into the per-RR scope (the single source
	// of truth — results never pre-carry counts), so later legs accumulate on
	// top without double-counting. Token usage likewise joins the cumulative
	// per-RR scope; this leg carries no TokenAccumulator of its own.
	inv.metricsFor(correlationID).IncLLMTurns()
	inv.recordTokenUsage(correlationID, resp.Usage)
	return result, nil
}

// Investigate runs the two-invocation investigation and returns the result.
// Per BR-AUDIT-005, all audit events use signal.RemediationID as correlation ID
// so that DataStorage queries by remediation_id return the full investigation trail.
type rcaHistoryReviewParams struct {
	Signal        katypes.SignalContext
	Result        *katypes.InvestigationResult
	Messages      []llm.Message
	Enrichment    *enrichment.EnrichmentResult
	EnrichmentMap map[string]*enrichment.EnrichmentResult
	LLMContext    LLMInvocationContext
}

// continueRCAWithHistory continues the existing RCA conversation after the
// server has resolved the current target and fetched its remediation history.
// Target changes are handled by the same loop and are bounded only by the
// shared RCA turn/tool budgets.
func (inv *Investigator) resolveRCAReviewEnrichment(ctx context.Context, p rcaHistoryReviewParams, result *katypes.InvestigationResult, currentTargetKey string) (*enrichment.EnrichmentResult, string) {
	kind, name, namespace := ResolveEnrichmentTarget(p.Signal, result)
	namespace = inv.normalizeNamespace(kind, namespace)
	apiVersion := p.Signal.ResourceAPIVersion
	if result != nil && result.RemediationTarget.Kind != "" {
		apiVersion = result.RemediationTarget.APIVersion
	}
	target := enrichmentTargetRef{Kind: kind, Name: name, Namespace: namespace, APIVersion: apiVersion}
	targetKey := EnrichmentCacheKeyWithAPIVersion(target.Kind, target.Name, target.Namespace, target.APIVersion)
	if targetKey == currentTargetKey {
		return p.Enrichment, targetKey
	}
	return inv.resolveEnrichmentCached(ctx, p.EnrichmentMap, target, p.Signal.ClusterID, p.Signal.IncidentID), targetKey
}

// mergeRCAContinuationResult preserves accepted RCA evidence when a
// history-review response is structurally valid but omits optional fields.
// A changed target is always authoritative; target fields are inherited only
// when the continuation omitted the target or repeated the same identity.
func preserveRCAContinuationCore(previous, merged *katypes.InvestigationResult) {
	if merged.RCASummary == "" {
		merged.RCASummary = previous.RCASummary
	}
	if merged.Severity == "" {
		merged.Severity = previous.Severity
	}
	if merged.SignalName == "" {
		merged.SignalName = previous.SignalName
	}
	if merged.InvestigationOutcome == "" {
		merged.InvestigationOutcome = previous.InvestigationOutcome
	}
	if merged.InvestigationAnalysis == "" {
		merged.InvestigationAnalysis = previous.InvestigationAnalysis
	}
	if merged.CausalMechanismStatus == "" {
		merged.CausalMechanismStatus = previous.CausalMechanismStatus
	}
}

func preserveRCAContinuationEvidence(previous, merged *katypes.InvestigationResult) {
	preserveRCAContinuationCore(previous, merged)
	if len(merged.ContributingFactors) == 0 {
		merged.ContributingFactors = append([]string(nil), previous.ContributingFactors...)
	}
	if len(merged.CausalChain) == 0 {
		merged.CausalChain = append([]string(nil), previous.CausalChain...)
	}
	if merged.DueDiligence == nil {
		merged.DueDiligence = previous.DueDiligence
	}
	if merged.RCAConfidence == nil {
		merged.RCAConfidence = previous.RCAConfidence
	}
	if !merged.ConfidenceSet && merged.Confidence == 0 && previous.Confidence != 0 {
		merged.Confidence = previous.Confidence
	}
	if !merged.ConfidenceSet && previous.ConfidenceSet {
		merged.Confidence = previous.Confidence
		merged.ConfidenceSet = true
	}
	if len(previous.Warnings) > 0 {
		merged.Warnings = append(append([]string(nil), previous.Warnings...), merged.Warnings...)
	}
	if merged.IsActionable == nil && previous.IsActionable != nil {
		actionable := *previous.IsActionable
		merged.IsActionable = &actionable
	}
}

func mergeRCAContinuationTarget(previous, merged *katypes.InvestigationResult) {
	if merged.RemediationTarget.Kind == "" && merged.RemediationTarget.Name == "" {
		merged.RemediationTarget = previous.RemediationTarget
		return
	}
	if merged.RemediationTarget.Kind != previous.RemediationTarget.Kind ||
		merged.RemediationTarget.Name != previous.RemediationTarget.Name {
		return
	}
	if merged.RemediationTarget.Namespace == "" {
		merged.RemediationTarget.Namespace = previous.RemediationTarget.Namespace
	}
	if merged.RemediationTarget.APIVersion == "" {
		merged.RemediationTarget.APIVersion = previous.RemediationTarget.APIVersion
	}
}

func mergeRCAContinuationResult(previous, updated *katypes.InvestigationResult) *katypes.InvestigationResult {
	if previous == nil {
		return updated
	}
	if updated == nil {
		return previous
	}
	merged := *updated
	preserveRCAContinuationEvidence(previous, &merged)
	mergeRCAContinuationTarget(previous, &merged)
	return &merged
}

func markRCAContinuationCancelled(result *katypes.InvestigationResult, messages []llm.Message, budget *rcaLoopBudget) *katypes.InvestigationResult {
	if result == nil {
		result = &katypes.InvestigationResult{}
	}
	result.Cancelled = true
	result.CancelledPhase = string(katypes.PhaseRCA)
	result.TerminalOutcome = sharedtypes.TerminalOutcomeCancelled
	if result.WorkflowDiscoveryStatus == "" {
		result.WorkflowDiscoveryStatus = sharedtypes.WorkflowDiscoveryStatusNotStarted
	}
	result.AccumulatedMessages = messagesToAuditFormat(messages)
	if budget != nil {
		result.CancelledAtTurn = budget.used
	}
	return result
}

func (inv *Investigator) runRCAHistoryReviewTurn(ctx context.Context, p rcaHistoryReviewParams, result *katypes.InvestigationResult, messages []llm.Message, history *enrichment.RemediationHistoryResult) (*katypes.InvestigationResult, []llm.Message, bool, error) {
	provisionalPrompt := prompt.BuildRCAProvisionalReviewSection(result)
	reviewPrompt := prompt.BuildRCAHistoryReviewSection(history)
	if provisionalPrompt != "" {
		reviewPrompt = provisionalPrompt + "\n\n" + reviewPrompt
	}
	reviewPrompt += "\n\nAfter re-evaluating the evidence, call `submit_result` with the complete updated RCA."
	messages = append(append([]llm.Message(nil), messages...), llm.Message{Role: "user", Content: reviewPrompt})
	loopRes, err := inv.runLLMLoop(ctx, messages, katypes.PhaseRCA, p.LLMContext)
	if err != nil {
		return nil, messages, true, fmt.Errorf("history-aware RCA continuation: %w", err)
	}
	if cancelled, ok := loopRes.(*CancelledResult); ok {
		messages = cancelled.Messages
	} else if extended := loopResultMessages(loopRes); len(extended) > 0 {
		messages = extended
	}

	finalized, content, reasoning, done := inv.classifyRCALoopResult(loopRes)
	if done {
		return mergeRCAContinuationResult(result, finalized), messages, true, nil
	}
	updated, parseErr := inv.resultParser.Parse(content)
	if parseErr != nil {
		if retried, retriedReasoning := inv.retryRCASubmit(ctx, content, messages, p.LLMContext); retried != nil {
			updated = retried
			reasoning = retriedReasoning
			parseErr = nil
		}
	}
	if parseErr != nil {
		if ctx.Err() != nil {
			//nolint:nilerr // cancellation is represented by the typed result
			return markRCAContinuationCancelled(result, messages, p.LLMContext.rcaBudget), messages, true, nil
		}
		result.HumanReviewNeeded = true
		result.HumanReviewReason = humanReviewReasonLLMParsingError
		result.Warnings = append(result.Warnings, "history-aware RCA response could not be parsed after retry")
		//nolint:nilerr // malformed continuation is an existing review outcome
		return result, messages, true, nil
	}

	updated = mergeRCAContinuationResult(result, updated)
	updated.Reasoning = toReasoningSummary(reasoning)
	applySignalSeverity(updated, p.Signal)
	accepted := updated
	updated = inv.sameKindValidationGate(ctx, updated, p.Signal, messages, p.LLMContext)
	if updated.HumanReviewNeeded {
		updated.HumanReviewNeeded = false
		updated.HumanReviewReason = ""
	}
	updated = inv.apiVersionValidationGate(ctx, updated, messages, p.LLMContext)
	if ctx.Err() != nil {
		//nolint:nilerr // cancellation is represented by the typed result
		return markRCAContinuationCancelled(accepted, messages, p.LLMContext.rcaBudget), messages, true, nil
	}
	updated = mergeRCAContinuationResult(accepted, updated)
	return updated, messages, false, nil
}

func (inv *Investigator) resolveRCAContinuationEnrichment(ctx context.Context, p rcaHistoryReviewParams, result *katypes.InvestigationResult, currentTargetKey string) (*enrichment.EnrichmentResult, string, bool) {
	if ctx.Err() != nil {
		return nil, currentTargetKey, true
	}
	currentEnrichment, targetKey := inv.resolveRCAReviewEnrichment(ctx, p, result, currentTargetKey)
	if ctx.Err() != nil {
		return currentEnrichment, targetKey, true
	}
	return currentEnrichment, targetKey, false
}

func (inv *Investigator) continueRCAWithHistory(ctx context.Context, p rcaHistoryReviewParams) (*katypes.InvestigationResult, []llm.Message, *enrichment.EnrichmentResult, error) {
	result, messages := p.Result, p.Messages
	if result == nil || result.Cancelled || result.HumanReviewNeeded {
		return result, messages, p.Enrichment, nil
	}
	// Keep the signal-target labels available as a fallback throughout RCA
	// continuation. A changed target can be unavailable to the label detector;
	// in that case the target enrichment must retain its own history/identity
	// while the existing all-failed-label preservation contract supplies the
	// original labels to workflow discovery (ADR-056, DD-KA-2511).
	signalEnrichment := p.Enrichment
	reviewedTargets := make(map[string]struct{})
	kind, name, namespace := ResolveEnrichmentTarget(p.Signal, nil)
	namespace = inv.normalizeNamespace(kind, namespace)
	currentTargetKey := EnrichmentCacheKeyWithAPIVersion(kind, name, namespace, p.Signal.ResourceAPIVersion)

	for {
		currentEnrichment, targetKey, cancelled := inv.resolveRCAContinuationEnrichment(ctx, p, result, currentTargetKey)
		if cancelled {
			//nolint:nilerr // cancellation is represented by the typed result
			return markRCAContinuationCancelled(result, messages, p.LLMContext.rcaBudget), messages, p.Enrichment, nil
		}
		currentEnrichment = preserveSignalTargetLabelsOnFailedRCAEnrichment(currentEnrichment, signalEnrichment)
		if currentEnrichment == nil {
			return result, messages, currentEnrichment, nil
		}
		if currentEnrichment.HistoryUnavailable {
			// History transport/schema failure is a typed evidence terminal
			// outcome, not RCA incompleteness. Stop before an ungrounded
			// continuation and let the downstream evidence mapper route it as
			// an API/evidence failure instead of WorkflowNotFound.
			markHistoryEnrichmentUnavailable(result, currentEnrichment)
			result.HumanReviewNeeded = false
			result.HumanReviewReason = ""
			result.Warnings = append(result.Warnings, "target-scoped remediation history is unavailable; RCA requires operator review")
			return result, messages, currentEnrichment, nil
		}
		history := currentEnrichment.RemediationHistory
		if history == nil || len(history.Tier1) == 0 && len(history.Tier2) == 0 {
			return result, messages, currentEnrichment, nil
		}
		if _, reviewed := reviewedTargets[targetKey]; reviewed {
			return result, messages, currentEnrichment, nil
		}
		reviewedTargets[targetKey] = struct{}{}

		updated, updatedMessages, done, err := inv.runRCAHistoryReviewTurn(ctx, p, result, messages, history)
		if err != nil {
			return nil, updatedMessages, currentEnrichment, err
		}
		result, messages = updated, updatedMessages
		if done {
			return result, messages, currentEnrichment, nil
		}
		p.Enrichment = currentEnrichment
		currentTargetKey = targetKey
	}
}

// initializeInvestigationState establishes the per-remediation correlation and
// resets the call-scoped accounting used by all investigation phases.
func (inv *Investigator) initializeInvestigationState(ctx context.Context, correlationID string) context.Context {
	// Preserve one logical remediation correlation through every deep enrichment
	// and audit call, including target-shift re-enrichment and retry paths.
	ctx = audit.WithCorrelationID(ctx, correlationID)
	// #1892: reset the per-investigation AnomalyDetector, not the shared config
	// template.
	inv.anomalyDetectorFor(correlationID).Reset()
	// #2387 (BR-KA-OBSERVABILITY-001): reset once here — never between phases or
	// legs — so every leg of one remediation request accumulates into cumulative
	// per-RR totals (turns, tools, and tokens alike).
	inv.resetMetrics(correlationID)
	inv.resetTokenScope(correlationID)
	inv.resetEvidence(correlationID)
	return ctx
}

func (inv *Investigator) Investigate(ctx context.Context, signal katypes.SignalContext) (result *katypes.InvestigationResult, retErr error) {
	evidenceScope := katypes.NewEvidenceStatusScope()
	ctx = katypes.WithEvidenceStatusScope(ctx, evidenceScope)
	defer func() {
		if status := evidenceScope.Snapshot(); status != nil {
			if result != nil {
				result.EvidenceStatus = status
			} else if retErr != nil {
				retErr = katypes.NewInvestigationError(retErr, status)
			}
		}
	}()

	ctx, err := inv.prescopeFleetOverlay(ctx, signal.ClusterID, signal.RemediationID)
	if err != nil {
		return nil, err
	}

	defer inv.startDiagSummary(ctx)()

	correlationID := signal.RemediationID
	ctx = inv.initializeInvestigationState(ctx, correlationID)

	// #783 + #1470: Pin client per phase. Each phase resolves its own client,
	// model name, and runtime params. Subsequent hot-reload swaps do not
	// affect in-flight work.
	rcaClient, rcaModelName, rcaRuntimeParams := inv.resolveForPhase(katypes.PhaseRCA)

	enrichmentCache := make(map[string]*enrichment.EnrichmentResult)

	signalKind, signalName, signalNS := ResolveEnrichmentTarget(signal, nil)
	signalNS = inv.normalizeNamespace(signalKind, signalNS)
	enrichData := inv.resolveEnrichmentCached(ctx, enrichmentCache, enrichmentTargetRef{
		Kind: signalKind, Name: signalName, Namespace: signalNS, APIVersion: signal.ResourceAPIVersion,
	}, signal.ClusterID, signal.IncidentID)
	tokens := &TokenAccumulator{}

	rcaLLMContext := LLMInvocationContext{
		Tokens:        tokens,
		CorrelationID: correlationID,
		Client:        rcaClient,
		ModelName:     rcaModelName,
		RuntimeParams: rcaRuntimeParams,
		rcaBudget:     &rcaLoopBudget{},
	}
	rcaResult, rcaMessages, err := inv.runRCAWithMessages(ctx, signal, rcaLLMContext)
	if err != nil {
		return nil, fmt.Errorf("RCA invocation: %w", err)
	}
	// SignalProcessing's Rego result is the authoritative severity. Apply it
	// before emitting RCA events or passing the RCA into later phases.
	applySignalSeverity(rcaResult, signal)
	rcaResult, rcaMessages, enrichData, err = inv.continueRCAWithHistory(ctx, rcaHistoryReviewParams{
		Signal:        signal,
		Result:        rcaResult,
		Messages:      rcaMessages,
		Enrichment:    enrichData,
		EnrichmentMap: enrichmentCache,
		LLMContext:    rcaLLMContext,
	})
	if err != nil {
		return nil, fmt.Errorf("history-aware RCA continuation: %w", err)
	}
	applyEnrichmentHistoryStatus(rcaResult, enrichData)
	if rcaResult.WorkflowDiscoveryStatus == "" {
		rcaResult.WorkflowDiscoveryStatus = sharedtypes.WorkflowDiscoveryStatusNotStarted
	}
	alignment.NotifyRCAComplete(ctx, rcaMessages)

	if rcaResult.Cancelled {
		// #2387: cancelled snapshots still carry the partial counts
		// accumulated up to the cancellation turn (BR-AUDIT-005). The scope
		// supersedes the per-phase accumulator: identical in single-leg
		// runs, cumulative across takeover legs.
		inv.applyMetricsFromScope(rcaResult, correlationID)
		inv.setTokenUsageFromScope(rcaResult, correlationID)
		inv.emitCancellationAudit(ctx, rcaResult, correlationID)
		return rcaResult, nil
	}

	inv.emitRCAComplete(ctx, rcaResult, tokens, correlationID, rcaModelName)

	if early, done := inv.checkRCAEarlyReturn(ctx, rcaResult, signal, enrichData, tokens, correlationID, rcaModelName); done {
		return early, nil
	}

	return inv.runWorkflowDiscoveryPhase(ctx, workflowDiscoveryPhaseParams{
		Signal: signal, RCAResult: rcaResult, EnrichData: enrichData, EnrichmentCache: enrichmentCache,
		SignalKind: signalKind, SignalName: signalName, SignalNS: signalNS, Tokens: tokens, CorrelationID: correlationID, ModelName: rcaModelName,
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
	ModelName                        string
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
		SignalKind: p.SignalKind, SignalName: p.SignalName, SignalNS: p.SignalNS, SignalAPIVersion: p.Signal.ResourceAPIVersion,
	})
	if hardFailed {
		historyUnavailable := p.RCAResult.HistoryStatus == sharedtypes.HistoryStatusTransportUnavailable ||
			p.RCAResult.HistoryStatus == sharedtypes.HistoryStatusSchemaError
		markDiscoverySkippedDueToFailure(p.RCAResult, p.RCAResult.HistoryStatus,
			historyUnavailable && p.RCAResult.HumanReviewReason == katypes.HumanReviewReasonRCAIncomplete)
		return inv.finalizeAndEmitRCAOnlyResult(ctx, p.RCAResult, p.Signal, enrichData, p.Tokens, p.CorrelationID, p.ModelName), nil
	}
	if enrichData != nil && enrichData.HistoryUnavailable {
		// Keep the RCA target identity, but do not run workflow selection with
		// unavailable repeat evidence. The typed source status is merged by
		// Enrich and attached to the terminal result by Investigate.
		markHistoryEnrichmentUnavailable(p.RCAResult, enrichData)
		return inv.finalizeAndEmitRCAOnlyResult(ctx, p.RCAResult, p.Signal, enrichData, p.Tokens, p.CorrelationID, p.ModelName), nil
	}
	if enrichData != nil && isHistoryUnavailableStatus(enrichData.HistoryStatus) {
		markHistoryEnrichmentUnavailable(p.RCAResult, enrichData)
		return inv.finalizeAndEmitRCAOnlyResult(ctx, p.RCAResult, p.Signal, enrichData, p.Tokens, p.CorrelationID, p.ModelName), nil
	}
	markWorkflowDiscoveryStarted(p.RCAResult)
	inv.emitInvestigationDecision(ctx, investigationDecisionParams{
		CorrelationID:      p.CorrelationID,
		Phase:              katypes.PhaseWorkflowDiscovery,
		TurnKind:           investigationDecisionTurnState,
		TurnID:             "workflow_discovery:started",
		Model:              "",
		Result:             p.RCAResult,
		ContinuationReason: "workflow_discovery_started",
		RetryAttempt:       0,
	})
	promptEnrichment := toPromptEnrichment(enrichData)
	workflowSignal = inv.enrichWorkflowSignalForDiscovery(workflowSignal, p.Signal, p.RCAResult, enrichData, p.CorrelationID)

	inv.anomalyDetectorFor(p.CorrelationID).Reset()

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
	applyWorkflowDiscoveryTypedStatus(workflowResult, p.RCAResult, enrichData)

	if workflowResult.Cancelled {
		applySignalSeverity(workflowResult, p.Signal)
		workflowResult.TerminalOutcome = sharedtypes.TerminalOutcomeCancelled
		// #2387: see the RCA-cancelled path above — partial counts apply here too.
		inv.applyMetricsFromScope(workflowResult, p.CorrelationID)
		inv.setTokenUsageFromScope(workflowResult, p.CorrelationID)
		inv.emitCancellationAudit(ctx, workflowResult, p.CorrelationID)
		return workflowResult, nil
	}

	return inv.mergeAndFinalizeWorkflowResult(ctx, mergeAndFinalizeWorkflowResultParams{
		WorkflowResult: workflowResult, RCAResult: p.RCAResult, Signal: p.Signal, WorkflowSignal: workflowSignal,
		EnrichData: enrichData, P1Ctx: p1Ctx, Tokens: p.Tokens, CorrelationID: p.CorrelationID,
		ModelName: wfModelName,
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
// #1390), human-review-needed, and not-actionable (#1430 / BR-KA-200)
// short-circuit conditions after RCA completes. Returns done=true when
// Investigate must return result immediately without running workflow
// discovery.
func (inv *Investigator) checkRCAEarlyReturn(ctx context.Context, rcaResult *katypes.InvestigationResult, signal katypes.SignalContext, enrichData *enrichment.EnrichmentResult, tokens *TokenAccumulator, correlationID, modelName string) (result *katypes.InvestigationResult, done bool) {
	if signal.Interactive || session.InteractiveUpgradeFromContext(ctx) {
		rcaResult.InteractiveHold = true
		return inv.finalizeAndEmitRCAOnlyResult(ctx, rcaResult, signal, enrichData, tokens, correlationID, modelName), true
	}
	if rcaResult.HumanReviewNeeded {
		return inv.finalizeAndEmitRCAOnlyResult(ctx, rcaResult, signal, enrichData, tokens, correlationID, modelName), true
	}
	if enrichData != nil && enrichData.HistoryUnavailable {
		// The source status is carried by the investigation evidence scope;
		// do not turn an unavailable history backend into RCA-incomplete or
		// allow workflow discovery to manufacture a no-workflow outcome.
		return inv.finalizeAndEmitRCAOnlyResult(ctx, rcaResult, signal, enrichData, tokens, correlationID, modelName), true
	}
	// Guard: only short-circuit not-actionable when no workflow was already
	// identified by the RCA (defense-in-depth against LLM self-contradiction).
	if rcaResult.IsActionable != nil && !*rcaResult.IsActionable && rcaResult.WorkflowID == "" {
		inv.logger.Info("skipping workflow discovery: RCA concluded not actionable",
			"investigation_outcome", rcaResult.InvestigationOutcome,
			"correlation_id", correlationID)
		return inv.finalizeAndEmitRCAOnlyResult(ctx, rcaResult, signal, enrichData, tokens, correlationID, modelName), true
	}
	return nil, false
}

// finalizeAndEmitRCAOnlyResult applies the common Phase-1-only finalization
// steps (authoritative signal severity, label attachment,
// remediation-target injection, and audit emission) shared by every
// early-return path in Investigate: the interactive-hold, human-review-needed,
// not-actionable, and re-enrichment hard-fail branches. Callers set any
// branch-specific fields on rcaResult before invoking this helper.
func (inv *Investigator) finalizeAndEmitRCAOnlyResult(ctx context.Context, rcaResult *katypes.InvestigationResult, signal katypes.SignalContext, enrichData *enrichment.EnrichmentResult, tokens *TokenAccumulator, correlationID, modelName string) *katypes.InvestigationResult {
	applyEnrichmentHistoryStatus(rcaResult, enrichData)
	if rcaResult.WorkflowDiscoveryStatus == "" {
		rcaResult.WorkflowDiscoveryStatus = sharedtypes.WorkflowDiscoveryStatusNotStarted
	}
	historyEnrichmentUnavailable := isHistoryUnavailableStatus(rcaResult.HistoryStatus) &&
		(rcaResult.TerminalOutcome == sharedtypes.TerminalOutcomeHistoryEnrichmentUnavailable ||
			(enrichData != nil && (enrichData.HistoryUnavailable || isHistoryUnavailableStatus(enrichData.HistoryStatus))))
	// Reclassify after enrichment is applied so a late #2518/history status can
	// override a provisional RCA-incomplete value, while valid typed outcomes
	// remain authoritative when no higher-priority unavailable state exists.
	rcaResult.TerminalOutcome = ClassifyTerminalOutcome(OutcomeClassificationInput{
		Result:                       rcaResult,
		HistoryStatus:                rcaResult.HistoryStatus,
		WorkflowDiscoveryStatus:      rcaResult.WorkflowDiscoveryStatus,
		HistoryEnrichmentUnavailable: historyEnrichmentUnavailable,
	})
	applySignalSeverity(rcaResult, signal)
	attachDetectedLabels(rcaResult, enrichData)
	InjectRemediationTarget(rcaResult, signal, enrichData)
	InjectTargetResourceParameters(rcaResult)
	// #2387 (BR-KA-OBSERVABILITY-001): inject server-computed call-level
	// counts before the result leaves KA — the LLM is never asked to supply
	// them (#2073/#2074), so every early-return path must apply them here.
	inv.applyMetricsFromScope(rcaResult, correlationID)
	// #2387 tokens: stamp the cumulative per-RR token totals (raw provider
	// counts, never costs) so completed investigations report tokens exactly
	// like cancelled ones already do via TokenUsage.
	inv.setTokenUsageFromScope(rcaResult, correlationID)
	inv.emitResponseComplete(ctx, rcaResult, tokens, correlationID, modelName)
	return rcaResult
}

// reEnrichWorkflowTargetParams groups the fields needed by
// reEnrichWorkflowTarget. Extracted per AGENTS.md's 8+-param
// Options-pattern rule.
type reEnrichWorkflowTargetParams struct {
	Signal                                             katypes.SignalContext
	RCAResult                                          *katypes.InvestigationResult
	EnrichData                                         *enrichment.EnrichmentResult
	EnrichmentCache                                    map[string]*enrichment.EnrichmentResult
	SignalKind, SignalName, SignalNS, SignalAPIVersion string
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
	postRCAAPIVersion := p.SignalAPIVersion
	if rcaResult != nil && rcaResult.RemediationTarget.Kind != "" {
		postRCAAPIVersion = rcaResult.RemediationTarget.APIVersion
	}

	targetUnchanged := postRCAKind == signalKind && postRCAName == signalName && postRCANS == signalNS && postRCAAPIVersion == p.SignalAPIVersion
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
		PostRCAKind: postRCAKind, PostRCAName: postRCAName, PostRCANS: postRCANS, PostRCAAPIVersion: postRCAAPIVersion,
	})
	return workflowSignal, enrichData, hardFailed
}

// reEnrichedTargetParams groups the fields needed by applyReEnrichedTarget.
// Extracted per AGENTS.md's 8+-param Options-pattern rule.
type reEnrichedTargetParams struct {
	EnrichmentCache                                        map[string]*enrichment.EnrichmentResult
	RCAResult                                              *katypes.InvestigationResult
	WorkflowSignal                                         *katypes.SignalContext
	EnrichData                                             *enrichment.EnrichmentResult
	Signal                                                 katypes.SignalContext
	SignalKind, SignalName                                 string
	PostRCAKind, PostRCAName, PostRCANS, PostRCAAPIVersion string
}

// applyReEnrichedTarget handles the RCA-target-changed branch of
// reEnrichWorkflowTarget: re-enriches using the post-RCA target, applies the
// hard-fail/deleted-resource/label-preservation rules (BR-KA-261 AC#7,
// #704), and updates p.WorkflowSignal's resource identity in place. Returns
// hardFailed=true when the caller must treat rcaResult as final.
func (inv *Investigator) applyReEnrichedTarget(ctx context.Context, p reEnrichedTargetParams) (updatedEnrichData *enrichment.EnrichmentResult, hardFailed bool) {
	enrichData := p.EnrichData
	inv.logger.Info("re-enriching with RCA remediation target",
		"signal", p.SignalKind+"/"+p.SignalName,
		"rca_target", p.PostRCAKind+"/"+p.PostRCAName,
	)
	reEnriched := inv.resolveEnrichmentCached(ctx, p.EnrichmentCache, enrichmentTargetRef{
		Kind: p.PostRCAKind, Name: p.PostRCAName, Namespace: p.PostRCANS, APIVersion: p.PostRCAAPIVersion,
	}, p.Signal.ClusterID, p.Signal.IncidentID)

	// BR-KA-261 AC#7 / #704: check HardFail BEFORE label merge
	// to prevent the merge from silently dropping the failure signal.
	// Use enrichData (initial enrichment) for labels because the failed
	// re-enrichment has empty/all-failed detections — preserving signal-level
	// labels (e.g. pdbProtected) matches pre-#704 behaviour where
	// allLabelDetectionsFailed() fell through to keep initial labels.
	if reEnriched != nil && reEnriched.HardFail {
		inv.logger.Error(reEnriched.OwnerChainError, "enrichment owner chain hard-failed, triggering rca_incomplete")
		if isHistoryUnavailableStatus(reEnriched.HistoryStatus) || reEnriched.HistoryUnavailable {
			markHistoryEnrichmentUnavailable(p.RCAResult, reEnriched)
		} else {
			applyEnrichmentHistoryStatus(p.RCAResult, reEnriched)
			p.RCAResult.HumanReviewNeeded = true
			p.RCAResult.HumanReviewReason = katypes.HumanReviewReasonRCAIncomplete
			markDiscoverySkippedDueToFailure(p.RCAResult, p.RCAResult.HistoryStatus, false)
		}
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
		enrichData = preserveSignalTargetLabelsOnFailedRCAEnrichment(reEnriched, p.EnrichData)
	default:
		inv.logger.Info("re-enrichment returned nil, retaining only target identity and allowed label fallback")
		enrichData = &enrichment.EnrichmentResult{
			ResourceKind:      p.PostRCAKind,
			ResourceName:      p.PostRCAName,
			ResourceNamespace: p.PostRCANS,
		}
		enrichData = preserveSignalTargetLabelsOnFailedRCAEnrichment(enrichData, p.EnrichData)
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
	ModelName                 string
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
	if workflowResult.HistoryStatus == "" {
		workflowResult.HistoryStatus = rcaResult.HistoryStatus
	}
	if workflowResult.WorkflowDiscoveryStatus == "" {
		workflowResult.WorkflowDiscoveryStatus = rcaResult.WorkflowDiscoveryStatus
	}
	if !workflowResult.TerminalOutcome.IsValid() && rcaResult.TerminalOutcome.IsValid() &&
		rcaResult.TerminalOutcome != sharedtypes.TerminalOutcomeRCAIncomplete {
		workflowResult.TerminalOutcome = rcaResult.TerminalOutcome
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

	applySignalSeverity(workflowResult, signal)
	attachDetectedLabels(workflowResult, enrichData)
	InjectRemediationTarget(workflowResult, workflowSignal, enrichData)
	// Issue #1044: propagate RCA-identified api_version to the workflow result
	// when the workflow selection didn't produce one. The RCA gate ensures
	// api_version is set for ambiguous kinds; this must survive through Phase 3.
	if workflowResult.RemediationTarget.APIVersion == "" && rcaResult.RemediationTarget.APIVersion != "" {
		workflowResult.RemediationTarget.APIVersion = rcaResult.RemediationTarget.APIVersion
	}
	InjectTargetResourceParameters(workflowResult)
	// #2387 (BR-KA-OBSERVABILITY-001): see finalizeAndEmitRCAOnlyResult —
	// the full-path result gets the same server-side injection.
	inv.applyMetricsFromScope(workflowResult, correlationID)
	inv.setTokenUsageFromScope(workflowResult, correlationID)
	inv.emitResponseComplete(ctx, workflowResult, tokens, correlationID, p.ModelName)
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

// applySignalSeverity makes the SignalProcessing classification authoritative
// over any severity supplied by the LLM. When the trusted input is absent,
// leave the result empty rather than inventing an unknown classification.
func applySignalSeverity(result *katypes.InvestigationResult, signal katypes.SignalContext) {
	// An exhausted RCA parse is not an RCA result. Do not let the trusted
	// signal severity make an unparsed response look like a genuine finding.
	if result != nil && result.HumanReviewNeeded &&
		result.HumanReviewReason == humanReviewReasonLLMParsingError && result.RCASummary == "" {
		return
	}
	result.Severity = signal.Severity
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

func (inv *Investigator) resolveEnrichment(ctx context.Context, target enrichmentTargetRef, clusterID, incidentID string) *enrichment.EnrichmentResult {
	if inv.enricher == nil {
		return nil
	}
	// DD-KA-2520-2521 requires one top-level correlation across enrichment and
	// investigation audit events for a remediation. IncidentID remains bounded
	// event metadata; the remediation-wide context is authoritative whenever it
	// is available. Preserve the IncidentID fallback for legacy callers.
	correlationID := incidentID
	if value, ok := audit.CorrelationIDFromContext(ctx); ok && value != "" {
		correlationID = value
	}
	result, err := inv.enricher.Enrich(ctx, enrichment.EnrichRequest{
		Kind: target.Kind, Name: target.Name, Namespace: target.Namespace, APIVersion: target.APIVersion,
		ClusterID: clusterID, IncidentID: incidentID, CorrelationID: correlationID,
	})
	if err != nil {
		inv.logger.Error(err, "enrichment failed")
		if enrichment.IsHistoryUnavailableError(err) || errors.Is(err, enrichment.ErrRBACForbidden) {
			status := enrichment.HistoryEvidenceStatusForError(err, target.Kind, target.Name, target.Namespace, clusterID, incidentID)
			evidenceStatus := &katypes.EvidenceStatus{
				Version:         status.Version,
				Class:           status.Class,
				Source:          status.Source,
				Outcome:         status.Outcome,
				TargetRef:       status.TargetRef,
				ClusterID:       status.ClusterID,
				Attempts:        status.Attempts,
				LastFailureCode: status.LastFailureCode,
				CorrelationID:   status.CorrelationID,
				Sources:         []katypes.EvidenceSourceStatus{status},
			}
			if scope, ok := katypes.EvidenceStatusScopeFromContext(ctx); ok {
				scope.MergeSource(status)
			}
			return &enrichment.EnrichmentResult{
				ResourceKind:       target.Kind,
				ResourceName:       target.Name,
				ResourceNamespace:  target.Namespace,
				HistoryUnavailable: true,
				EvidenceStatus:     evidenceStatus,
			}
		}
		// Preserve the existing best-effort behavior for unrelated enrichment
		// failures (for example optional label/RBAC reads). History/spec-hash
		// acquisition failures are represented explicitly by EnrichmentResult.
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
	Tokens            *TokenAccumulator
	CorrelationID     string
	Client            llm.Client
	ModelName         string
	RuntimeParams     llm.RuntimeParams
	WorkflowDiscovery *workflowDiscoveryAuditContext
	RepeatSelection   *parser.RepeatSelectionContext
	decisionLoopID    string
	rcaBudget         *rcaLoopBudget
}

// workflowDiscoveryAuditContext is the safe provenance summary persisted on
// LLM request events for Phase 3. It records only structured label context and
// handoff presence, never the full prompt or arbitrary resource labels.
type workflowDiscoveryAuditContext struct {
	EnrichmentLabelsPresent bool
	SignalLabelsPresent     bool
	PromptLabelsPresent     bool
	DetectedLabels          map[string]string
}
