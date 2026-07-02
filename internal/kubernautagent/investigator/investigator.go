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
	"golang.org/x/sync/errgroup"
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

// SubmitResult is returned when the LLM calls the generic submit_result tool (RCA phase).
type SubmitResult struct{ Content string }

func (*SubmitResult) loopResult() {}

// SubmitWithWorkflowResult is returned when the LLM calls submit_result_with_workflow.
type SubmitWithWorkflowResult struct{ Content string }

func (*SubmitWithWorkflowResult) loopResult() {}

// SubmitNoWorkflowResult is returned when the LLM calls submit_result_no_workflow.
type SubmitNoWorkflowResult struct{ Content string }

func (*SubmitNoWorkflowResult) loopResult() {}

// TextResult is returned when the LLM responds with plain text (no tool call).
type TextResult struct{ Content string }

func (*TextResult) loopResult() {}

// ExhaustedResult is returned when the loop exhausts maxTurns or tool budget.
// Reason distinguishes the two cases for observability (#770).
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

// sentinelResult maps a sentinel tool call to its LoopResult type.
func sentinelResult(tc llm.ToolCall) LoopResult {
	switch tc.Name {
	case SubmitResultToolName:
		return &SubmitResult{Content: tc.Arguments}
	case SubmitResultWithWorkflowToolName:
		return &SubmitWithWorkflowResult{Content: tc.Arguments}
	case SubmitResultNoWorkflowToolName:
		return &SubmitNoWorkflowResult{Content: tc.Arguments}
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
func (inv *Investigator) resolveForPhase(phase katypes.Phase) (llm.Client, string, llm.RuntimeParams) {
	if inv.phaseResolver != nil {
		return inv.phaseResolver.ResolvePhase(phase)
	}
	client := inv.client
	modelName := inv.modelName
	var runtimeParams llm.RuntimeParams
	if inv.swappable != nil {
		pinned := inv.swappable.Snapshot()
		if inv.pinDecorator != nil {
			client = inv.pinDecorator(pinned)
			if client == nil {
				client = llm.NewInstrumentedClient(pinned)
			}
		} else {
			client = llm.NewInstrumentedClient(pinned)
		}
		modelName = inv.swappable.ModelName()
		runtimeParams = inv.swappable.RuntimeParameters()
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

// RunWorkflowDiscoveryFromRCA executes Phase 3 (workflow selection) using a
// structured RCA result. This reuses the exact autonomous Phase 3 pipeline
// (BuildPhase1Context + runWorkflowSelection) so that interactive and
// autonomous workflow discovery produce consistent results.
//
// F5/F6 (#1374): When the investigator has an enricher wired, this function
// resolves enrichment internally (pre-RCA + post-RCA re-enrichment), mirroring
// the autonomous Investigate() pipeline. The caller-provided enrichData is used
// as fallback only when the enricher is nil.
func (inv *Investigator) RunWorkflowDiscoveryFromRCA(ctx context.Context, signal katypes.SignalContext, rcaResult *katypes.InvestigationResult, enrichData *prompt.EnrichmentData, correlationID string) (*katypes.InvestigationResult, error) {
	if rcaResult == nil {
		return nil, fmt.Errorf("RunWorkflowDiscoveryFromRCA: rcaResult must not be nil")
	}

	rcaCopy := *rcaResult
	rcaResult = &rcaCopy

	inv.pipeline.AnomalyDetector.Reset()

	inv.autoResolveRCATargetAPIVersion(rcaResult, correlationID)

	preKind := signal.ResourceKind
	signal = SyncSignalFromRCA(signal, rcaResult.RemediationTarget)
	signal.Namespace = inv.normalizeNamespace(signal.ResourceKind, signal.Namespace)

	inv.logger.Info("RunWorkflowDiscoveryFromRCA: RCA target state",
		"rca_target_kind", rcaResult.RemediationTarget.Kind,
		"rca_target_api_version", rcaResult.RemediationTarget.APIVersion,
		"signal_kind_before", preKind,
		"signal_kind", signal.ResourceKind,
		"signal_api_version", signal.ResourceAPIVersion,
		"signal_namespace", signal.Namespace,
		"correlation_id", correlationID)

	// F5 (#1374): Resolve enrichment when the enricher is wired, mirroring
	// the autonomous path (Investigate). Pre-RCA enrichment uses the signal
	// target; post-RCA re-enrichment uses the RCA-identified target when it
	// differs (cross-resource RCA).
	rawEnrichData, enricherWired, hardFailResult := inv.resolveRCAWorkflowDiscoveryEnrichment(ctx, signal, rcaResult, correlationID)
	if hardFailResult != nil {
		return hardFailResult, nil
	}
	if enricherWired {
		enrichData = toPromptEnrichment(rawEnrichData)
	}

	// F6 (#1374): Attach DetectedLabelsJSON to the signal so workflow
	// discovery tools forward labels to DS catalog queries, activating
	// GitOps-aware scoring (parity with autonomous path).
	inv.attachRCADetectedLabelsJSON(&signal, rawEnrichData, correlationID)

	client, modelName, runtimeParams := inv.resolveForPhase(katypes.PhaseWorkflowDiscovery)

	p1Ctx := BuildPhase1Context(rcaResult)
	workflowResult, err := inv.runWorkflowSelection(ctx, signal, rcaResult.RCASummary, enrichData, p1Ctx, LLMInvocationContext{
		CorrelationID: correlationID,
		Client:        client,
		ModelName:     modelName,
		RuntimeParams: runtimeParams,
	})
	if err != nil {
		return nil, err
	}

	FinalizeWorkflowResult(workflowResult, signal, rcaResult, rawEnrichData)
	return workflowResult, nil
}

// autoResolveRCATargetAPIVersion auto-resolves rcaResult.RemediationTarget.APIVersion
// when the LLM omitted it, mirroring the autonomous apiVersionValidationGate in
// runRCA. It mutates rcaResult in place. Uses the REST mapper via
// scopeResolver: if the kind maps to exactly one API group, the apiVersion
// can be inferred deterministically without an extra LLM turn.
func (inv *Investigator) autoResolveRCATargetAPIVersion(rcaResult *katypes.InvestigationResult, correlationID string) {
	if rcaResult.RemediationTarget.APIVersion != "" || rcaResult.RemediationTarget.Kind == "" || inv.scopeResolver == nil {
		return
	}
	ambiguous, gvrs, err := inv.scopeResolver.IsAmbiguousKind(rcaResult.RemediationTarget.Kind)
	if err != nil {
		inv.logger.Error(err, "RunWorkflowDiscoveryFromRCA: IsAmbiguousKind failed, skipping apiVersion auto-resolve",
			"kind", rcaResult.RemediationTarget.Kind, "correlation_id", correlationID)
		return
	}
	if !ambiguous && len(gvrs) == 1 {
		rcaResult.RemediationTarget.APIVersion = gvrToAPIVersion(gvrs[0])
		inv.logger.Info("RunWorkflowDiscoveryFromRCA: auto-resolved apiVersion for non-ambiguous kind",
			"kind", rcaResult.RemediationTarget.Kind,
			"api_version", rcaResult.RemediationTarget.APIVersion,
			"correlation_id", correlationID)
	}
}

// resolveRCAWorkflowDiscoveryEnrichment resolves (and, when the RCA target
// differs from the signal target, re-resolves) enrichment for the interactive
// workflow-discovery path. enricherWired is false when inv.enricher is nil,
// signaling that the caller must leave its own enrichData parameter
// untouched. hardFailResult is non-nil when re-enrichment hard-failed; the
// caller must return it immediately (it is already fully finalized).
func (inv *Investigator) resolveRCAWorkflowDiscoveryEnrichment(ctx context.Context, signal katypes.SignalContext, rcaResult *katypes.InvestigationResult, correlationID string) (rawEnrichData *enrichment.EnrichmentResult, enricherWired bool, hardFailResult *katypes.InvestigationResult) {
	if inv.enricher == nil {
		return nil, false, nil
	}

	enrichmentCache := make(map[string]*enrichment.EnrichmentResult)
	signalKind, signalName, signalNS := ResolveEnrichmentTarget(signal, nil)
	signalNS = inv.normalizeNamespace(signalKind, signalNS)
	rawEnrichData = inv.resolveEnrichmentCached(ctx, enrichmentCache, signalKind, signalName, signalNS, signal.IncidentID)

	postRCAKind, postRCAName, postRCANS := ResolveEnrichmentTarget(signal, rcaResult)
	postRCANS = inv.normalizeNamespace(postRCAKind, postRCANS)
	if postRCAKind != signalKind || postRCAName != signalName || postRCANS != signalNS {
		inv.logger.Info("RunWorkflowDiscoveryFromRCA: re-enriching with RCA target",
			"signal", signalKind+"/"+signalName,
			"rca_target", postRCAKind+"/"+postRCAName,
			"correlation_id", correlationID)
		reEnriched := inv.resolveEnrichmentCached(ctx, enrichmentCache, postRCAKind, postRCAName, postRCANS, signal.IncidentID)

		if reEnriched != nil && reEnriched.HardFail {
			inv.logger.Error(reEnriched.OwnerChainError,
				"RunWorkflowDiscoveryFromRCA: enrichment hard-failed, triggering rca_incomplete",
				"correlation_id", correlationID)
			rcaResult.HumanReviewNeeded = true
			rcaResult.HumanReviewReason = "rca_incomplete"
			backfillSeverity(rcaResult, signal)
			attachDetectedLabels(rcaResult, rawEnrichData)
			InjectRemediationTarget(rcaResult, signal, rawEnrichData)
			InjectTargetResourceParameters(rcaResult)
			return rawEnrichData, true, rcaResult
		} else if reEnriched != nil && reEnriched.TargetResourceDeleted {
			rcaResult.Warnings = append(rcaResult.Warnings,
				deletedResourceWarning(postRCAKind, postRCAName, postRCANS))
		}

		if reEnriched != nil && !reEnriched.HardFail && !allLabelDetectionsFailed(reEnriched.DetectedLabels) {
			rawEnrichData = reEnriched
		} else if reEnriched != nil && !reEnriched.HardFail {
			inv.logger.Info("RunWorkflowDiscoveryFromRCA: re-enrichment labels all failed, preserving signal-target labels",
				"rca_target", postRCAKind+"/"+postRCAName, "correlation_id", correlationID)
		}
	} else if rawEnrichData != nil && rawEnrichData.TargetResourceDeleted {
		rcaResult.Warnings = append(rcaResult.Warnings,
			deletedResourceWarning(signalKind, signalName, signalNS))
	}

	return rawEnrichData, true, nil
}

// attachRCADetectedLabelsJSON marshals rawEnrichData's DetectedLabels onto
// signal.DetectedLabelsJSON so workflow discovery tools forward labels to DS
// catalog queries, activating GitOps-aware scoring (parity with the
// autonomous Investigate path). No-op when rawEnrichData or its labels are
// unset (which is always the case when the enricher is not wired).
func (inv *Investigator) attachRCADetectedLabelsJSON(signal *katypes.SignalContext, rawEnrichData *enrichment.EnrichmentResult, correlationID string) {
	if rawEnrichData == nil || rawEnrichData.DetectedLabels == nil {
		return
	}
	dlJSON, err := json.Marshal(rawEnrichData.DetectedLabels)
	if err != nil {
		inv.logger.Error(err, "RunWorkflowDiscoveryFromRCA: failed to marshal detected labels",
			"correlation_id", correlationID)
		return
	}
	signal.DetectedLabelsJSON = string(dlJSON)
	dl := rawEnrichData.DetectedLabels
	trueCount := countTrueLabels(dl.GitOpsManaged, dl.PDBProtected, dl.HPAEnabled,
		dl.Stateful, dl.HelmManaged, dl.NetworkIsolated, dl.ResourceQuotaConstrained)
	inv.logger.V(1).Info("RunWorkflowDiscoveryFromRCA: detected labels attached for workflow scoring",
		"correlation_id", correlationID,
		"true_label_count", trueCount,
		"gitops_tool", dl.GitOpsTool)
}

// Investigate runs the two-invocation investigation and returns the result.
// Per BR-AUDIT-005, all audit events use signal.RemediationID as correlation ID
// so that DataStorage queries by remediation_id return the full investigation trail.
func (inv *Investigator) Investigate(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error) {
	diagSendOK.Store(0)
	diagSendDrop.Store(0)
	diagSinkNil.Store(0)
	defer func() {
		inv.logger.Info("DIAG emitToSink summary",
			"session_id", session.SessionIDFromContext(ctx),
			"sent", diagSendOK.Load(),
			"dropped", diagSendDrop.Load(),
			"sink_nil", diagSinkNil.Load(),
			"sink_ptr", fmt.Sprintf("%p", session.EventSinkFromContext(ctx)))
	}()
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

	// BR-INTERACTIVE-010 / #1390: When interactive (either via signal or runtime
	// upgrade flag), skip Phase 2+3 and return with InteractiveHold=true.
	if signal.Interactive || session.InteractiveUpgradeFromContext(ctx) {
		rcaResult.InteractiveHold = true
		return inv.finalizeAndEmitRCAOnlyResult(ctx, rcaResult, signal, enrichData, tokens, correlationID), nil
	}

	if rcaResult.HumanReviewNeeded {
		return inv.finalizeAndEmitRCAOnlyResult(ctx, rcaResult, signal, enrichData, tokens, correlationID), nil
	}

	// #1430 / BR-HAPI-200: When the RCA concludes no action is required
	// (problem_resolved or predictive_no_action), skip workflow discovery.
	// Guard: only short-circuit when no workflow was already identified by
	// the RCA (defense-in-depth against LLM self-contradiction).
	if rcaResult.IsActionable != nil && !*rcaResult.IsActionable && rcaResult.WorkflowID == "" {
		inv.logger.Info("skipping workflow discovery: RCA concluded not actionable",
			"investigation_outcome", rcaResult.InvestigationOutcome,
			"correlation_id", correlationID)
		return inv.finalizeAndEmitRCAOnlyResult(ctx, rcaResult, signal, enrichData, tokens, correlationID), nil
	}

	// GAP-001 / ADR-056: Re-enrich using RCA-identified remediation target if
	// different. H3-fix: retain pre-RCA enrichment if re-enrichment fails.
	workflowSignal, enrichData, hardFailed := inv.reEnrichWorkflowTarget(
		ctx, signal, rcaResult, enrichData, enrichmentCache, signalKind, signalName, signalNS)
	if hardFailed {
		return inv.finalizeAndEmitRCAOnlyResult(ctx, rcaResult, signal, enrichData, tokens, correlationID), nil
	}
	promptEnrichment = toPromptEnrichment(enrichData)
	workflowSignal = inv.enrichWorkflowSignalForDiscovery(workflowSignal, signal, rcaResult, enrichData, correlationID)

	inv.pipeline.AnomalyDetector.Reset()

	wfClient, wfModelName, wfRuntimeParams := inv.resolveForPhase(katypes.PhaseWorkflowDiscovery)

	p1Ctx := BuildPhase1Context(rcaResult)

	workflowResult, err := inv.runWorkflowSelection(ctx, workflowSignal, rcaResult.RCASummary, promptEnrichment, p1Ctx, LLMInvocationContext{
		Tokens:        tokens,
		CorrelationID: correlationID,
		Client:        wfClient,
		ModelName:     wfModelName,
		RuntimeParams: wfRuntimeParams,
	})
	if err != nil {
		return nil, fmt.Errorf("workflow selection invocation: %w", err)
	}

	if workflowResult.Cancelled {
		inv.emitCancellationAudit(ctx, workflowResult, correlationID)
		return workflowResult, nil
	}

	return inv.mergeAndFinalizeWorkflowResult(ctx, workflowResult, rcaResult, signal, workflowSignal, enrichData, p1Ctx, tokens, correlationID), nil
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

// reEnrichWorkflowTarget re-enriches using the RCA-identified remediation
// target when it differs from the pre-RCA signal target (GAP-001/ADR-056).
// hardFailed is true when the re-enrichment owner-chain lookup hard-failed;
// callers must treat rcaResult (already marked HumanReviewNeeded/rca_incomplete
// by this function) as the final result in that case.
func (inv *Investigator) reEnrichWorkflowTarget(ctx context.Context, signal katypes.SignalContext, rcaResult *katypes.InvestigationResult, enrichData *enrichment.EnrichmentResult, enrichmentCache map[string]*enrichment.EnrichmentResult, signalKind, signalName, signalNS string) (workflowSignal katypes.SignalContext, updatedEnrichData *enrichment.EnrichmentResult, hardFailed bool) {
	workflowSignal = signal
	postRCAKind, postRCAName, postRCANS := ResolveEnrichmentTarget(signal, rcaResult)
	postRCANS = inv.normalizeNamespace(postRCAKind, postRCANS)
	if postRCAKind != signalKind || postRCAName != signalName || postRCANS != signalNS {
		inv.logger.Info("re-enriching with RCA remediation target",
			"signal", signalKind+"/"+signalName,
			"rca_target", postRCAKind+"/"+postRCAName,
		)
		reEnriched := inv.resolveEnrichmentCached(ctx, enrichmentCache, postRCAKind, postRCAName, postRCANS, signal.IncidentID)

		// BR-HAPI-261 AC#7 / #704: check HardFail BEFORE label merge
		// to prevent the merge from silently dropping the failure signal.
		// Use enrichData (initial enrichment) for labels because the failed
		// re-enrichment has empty/all-failed detections — preserving signal-level
		// labels (e.g. pdbProtected) matches pre-#704 behaviour where
		// allLabelDetectionsFailed() fell through to keep initial labels.
		if reEnriched != nil && reEnriched.HardFail {
			inv.logger.Error(reEnriched.OwnerChainError, "enrichment owner chain hard-failed, triggering rca_incomplete")
			rcaResult.HumanReviewNeeded = true
			rcaResult.HumanReviewReason = "rca_incomplete"
			return workflowSignal, enrichData, true
		}

		if reEnriched != nil && reEnriched.TargetResourceDeleted {
			rcaResult.Warnings = append(rcaResult.Warnings,
				deletedResourceWarning(postRCAKind, postRCAName, postRCANS))
		}

		if reEnriched != nil && !allLabelDetectionsFailed(reEnriched.DetectedLabels) {
			enrichData = reEnriched
		} else if reEnriched != nil {
			inv.logger.Info("re-enrichment labels all failed (RCA target not found), preserving signal-target labels",
				"rca_target", postRCAKind+"/"+postRCAName)
		} else {
			inv.logger.Info("re-enrichment returned nil, retaining pre-RCA enrichment data")
		}

		workflowSignal.ResourceKind = postRCAKind
		workflowSignal.ResourceName = postRCAName
		workflowSignal.Namespace = postRCANS
	} else if enrichData != nil && enrichData.TargetResourceDeleted {
		rcaResult.Warnings = append(rcaResult.Warnings,
			deletedResourceWarning(signalKind, signalName, signalNS))
	}
	return workflowSignal, enrichData, false
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

// mergeAndFinalizeWorkflowResult merges Phase 1 (RCA) fallback fields into
// the Phase 3 workflow-selection result, then applies the same finalization
// steps as finalizeAndEmitRCAOnlyResult before emitting the audit trail.
func (inv *Investigator) mergeAndFinalizeWorkflowResult(ctx context.Context, workflowResult, rcaResult *katypes.InvestigationResult, signal, workflowSignal katypes.SignalContext, enrichData *enrichment.EnrichmentResult, p1Ctx *prompt.Phase1Data, tokens *TokenAccumulator, correlationID string) *katypes.InvestigationResult {
	if workflowResult.RCASummary == "" {
		workflowResult.RCASummary = rcaResult.RCASummary
	}
	if workflowResult.SignalName == "" && rcaResult.SignalName != "" {
		workflowResult.SignalName = rcaResult.SignalName
	}
	if len(rcaResult.Warnings) > 0 {
		workflowResult.Warnings = append(rcaResult.Warnings, workflowResult.Warnings...)
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

func (inv *Investigator) runRCA(ctx context.Context, signal katypes.SignalContext, enrichData *prompt.EnrichmentData, llmCtx LLMInvocationContext) (result *katypes.InvestigationResult, retErr error) {
	correlationID := llmCtx.CorrelationID
	promptSignal := SignalToPrompt(signal)
	LogLabelOverrideOrRejection(inv.logger, signal, promptSignal, correlationID, "RCA")
	systemPrompt, err := inv.builder.RenderInvestigation(promptSignal)
	if err != nil {
		return nil, fmt.Errorf("rendering investigation prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Investigate: %s %s in %s — %s", signal.Severity, signal.Name, signal.Namespace, signal.Message)},
	}

	loopRes, err := inv.runLLMLoop(ctx, messages, katypes.PhaseRCA, llmCtx)
	if err != nil {
		return nil, err
	}

	alignment.NotifyRCAComplete(ctx, messages)

	var content string
	switch r := loopRes.(type) {
	case *CancelledResult:
		cancelledResult := &katypes.InvestigationResult{
			Cancelled:           true,
			CancelledPhase:      string(katypes.PhaseRCA),
			CancelledAtTurn:     r.Turn,
			AccumulatedMessages: messagesToAuditFormat(r.Messages),
		}
		if r.Tokens != nil {
			s := r.Tokens.Summary()
			cancelledResult.TokenUsage = &katypes.TokenUsageSummary{
				PromptTokens:     s.PromptTokens,
				CompletionTokens: s.CompletionTokens,
				TotalTokens:      s.TotalTokens,
			}
		}
		return cancelledResult, nil
	case *ExhaustedResult:
		return &katypes.InvestigationResult{
			HumanReviewNeeded: true,
			Reason:            fmt.Sprintf("%s during RCA (maxTurns=%d)", r.Reason, inv.maxTurns),
		}, nil
	case *SubmitResult:
		content = r.Content
	case *TextResult:
		content = r.Content
	default:
		content = ""
	}

	result, parseErr := inv.resultParser.Parse(content)
	if parseErr != nil {
		if retried := inv.retryRCASubmit(ctx, content, messages, llmCtx); retried != nil {
			result = retried
			parseErr = nil
		}
	}
	if parseErr != nil && ctx.Err() != nil {
		return &katypes.InvestigationResult{
			Cancelled:      true,
			CancelledPhase: string(katypes.PhaseRCA),
		}, nil
	}
	if parseErr != nil {
		inv.logger.Info("RCA parse failed after retry, treating as summary",
			"error", parseErr.Error(),
			"correlation_id", correlationID)
		return &katypes.InvestigationResult{
			RCASummary: content,
		}, nil
	}

	// Same-kind sentinel validation gate (Issue #847): when the LLM's
	// remediation_target.kind matches the signal's resource_kind, the LLM may
	// be targeting the symptom reporter instead of the actual root cause.
	// Inject a single correction message and re-run. Max 1 retry.
	result = inv.sameKindValidationGate(ctx, result, signal, messages, llmCtx)

	// Defense-in-depth: RCA phase must never abort the pipeline via
	// needs_human_review from the parser. Only gate-level exhaustion (#1044)
	// is a valid RCA abort. Clear parser-set values BEFORE running the
	// apiVersion gate so the gate's decision is authoritative.
	// Aligned with HAPI v1.2.1 where needs_human_review is parser-driven in Phase 3.
	if result.HumanReviewNeeded {
		inv.logger.Info("clearing parser-set HumanReviewNeeded during RCA (Phase 3 only)",
			"reason", result.HumanReviewReason,
			"correlation_id", correlationID)
		result.HumanReviewNeeded = false
		result.HumanReviewReason = ""
	}

	// apiVersion validation gate (Issue #1044): when the remediation target kind
	// exists in multiple API groups and api_version is missing, the gate retries
	// once. On exhaustion it sets HumanReviewNeeded=true — a valid RCA abort.
	// Runs after clearing so its decision is authoritative and not cleared.
	result = inv.apiVersionValidationGate(ctx, result, messages, llmCtx)

	return result, nil
}

const maxRCAParseRetries = 1

// retryRCASubmit performs a single correction attempt when the RCA parse fails
// (e.g. double-serialized JSON that wasn't caught by the unwrap heuristic, or
// garbage fields). Mirrors retryWorkflowSubmit but scoped to the RCA phase.
func (inv *Investigator) retryRCASubmit(ctx context.Context, lastContent string, history []llm.Message, llmCtx LLMInvocationContext) *katypes.InvestigationResult {
	tokens, correlationID, client, modelName, runtimeParams :=
		llmCtx.Tokens, llmCtx.CorrelationID, llmCtx.Client, llmCtx.ModelName, llmCtx.RuntimeParams
	submitOnlyTools := []llm.ToolDefinition{
		{
			Name:        SubmitResultToolName,
			Description: "Submit root cause analysis result.",
			Parameters:  parser.RCAResultSchema(),
		},
	}

	correctionMsg := `Your response could not be parsed. You MUST call submit_result with a JSON object like:
{"root_cause_analysis":{"summary":"...","severity":"critical","signal_name":"SignalName","contributing_factors":["factor1"],"remediation_target":{"kind":"Deployment","name":"resource","namespace":"ns","api_version":"apps/v1"}},"confidence":0.9}

CRITICAL: root_cause_analysis must be a JSON object, NOT a string. Do NOT wrap it in quotes.`

	retryMessages := make([]llm.Message, len(history))
	copy(retryMessages, history)
	retryMessages = append(retryMessages,
		llm.Message{Role: "assistant", Content: lastContent},
	)

	for attempt := 0; attempt < maxRCAParseRetries; attempt++ {
		if ctx.Err() != nil {
			return nil
		}
		inv.logger.Info("parse-level retry for RCA submit",
			"attempt", attempt+1,
			"max", maxRCAParseRetries,
			"correlation_id", correlationID)

		retryMessages = append(retryMessages,
			llm.Message{Role: "user", Content: correctionMsg},
		)

		retryEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
		retryEvent.EventAction = audit.ActionLLMRequest
		retryEvent.EventOutcome = audit.OutcomeSuccess
		retryEvent.Data["model"] = modelName
		retryEvent.Data["prompt_length"] = totalPromptLength(retryMessages)
		retryEvent.Data["prompt_preview"] = lastUserMessage(retryMessages, 500)
		retryEvent.Data["retry_attempt"] = attempt + 1
		retryEvent.Data["retry_max"] = maxRCAParseRetries
		retryEvent.Data["phase"] = string(katypes.PhaseRCA)
		retryEvent.Data["retry_reason"] = "rca_parse_correction"
		audit.StoreBestEffort(ctx, inv.auditStore, retryEvent, inv.auditLog())

		resp, err := inv.chatOrStream(ctx, client, llm.ChatRequest{
			Messages: retryMessages,
			Tools:    submitOnlyTools,
			Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
		}, attempt+1, string(katypes.PhaseRCA), modelName, runtimeParams)
		if err != nil {
			inv.logger.Error(err, "RCA retry LLM call failed",
				"correlation_id", correlationID)
			continue
		}
		if tokens != nil {
			tokens.Add(resp.Usage)
		}

		emitToSink(ctx, session.EventTypeReasoningDelta, attempt+1, string(katypes.PhaseRCA), map[string]interface{}{
			"content":       resp.Message.Content,
			"retry_attempt": attempt + 1,
		})

		for _, tc := range resp.ToolCalls {
			if tc.Name == SubmitResultToolName {
				result, parseErr := inv.resultParser.Parse(tc.Arguments)
				if parseErr != nil {
					inv.logger.Error(parseErr, "RCA retry parse still failed",
						"correlation_id", correlationID)
					retryMessages = append(retryMessages, resp.Message)
					continue
				}
				inv.logger.Info("RCA retry succeeded",
					"correlation_id", correlationID)
				return result
			}
		}

		if resp.Message.Content != "" {
			result, parseErr := inv.resultParser.Parse(resp.Message.Content)
			if parseErr == nil {
				inv.logger.Info("RCA retry succeeded from message content",
					"correlation_id", correlationID)
				return result
			}
		}

		retryMessages = append(retryMessages, resp.Message)
	}
	return nil
}

func (inv *Investigator) runWorkflowSelection(ctx context.Context, signal katypes.SignalContext, rcaSummary string, enrichData *prompt.EnrichmentData, p1Ctx *prompt.Phase1Data, llmCtx LLMInvocationContext) (result *katypes.InvestigationResult, retErr error) {
	correlationID := llmCtx.CorrelationID
	// Apply signal label overrides (target_resource_kind / target_resource_name)
	// before attaching to context. This ensures workflow discovery tools
	// (list_available_actions, list_workflows) filter by the correct component.
	// Defense-in-depth for #1064/#1065: even if enrichment resolved a container
	// kind (e.g. Namespace), the label override corrects it for tool context.
	overriddenSignal := ApplySignalLabelOverrides(signal)
	ctx = katypes.WithSignalContext(ctx, overriddenSignal)
	inv.logger.Info("runWorkflowSelection: post-override signal",
		"component_gvk", overriddenSignal.ComponentGVK(),
		"resource_kind", overriddenSignal.ResourceKind,
		"resource_api_version", overriddenSignal.ResourceAPIVersion,
		"correlation_id", correlationID)

	wfPromptSignal := SignalToPrompt(signal)
	LogLabelOverrideOrRejection(inv.logger, signal, wfPromptSignal, correlationID, "workflow selection")
	systemPrompt, err := inv.builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
		Signal:     wfPromptSignal,
		RCASummary: rcaSummary,
		EnrichData: enrichData,
		Phase1:     p1Ctx,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering workflow selection prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("RCA findings: %s\n\nSelect the appropriate remediation workflow.", rcaSummary)},
	}

	loopRes, err := inv.runLLMLoop(ctx, messages, katypes.PhaseWorkflowDiscovery, llmCtx)
	if err != nil {
		return nil, err
	}

	content, early, err := inv.handleWorkflowSelectionLoopResult(ctx, loopRes, rcaSummary, messages, llmCtx)
	if early != nil || err != nil {
		return early, err
	}

	result, parseErr := inv.resultParser.Parse(content)
	if parseErr != nil {
		return inv.workflowSelectionRetryOrHumanReview(ctx, content, messages, rcaSummary, llmCtx, parseErr,
			fmt.Sprintf("workflow selection: LLM did not produce parseable result: %s", parseErr)), nil
	}

	if inv.pipeline.CatalogFetcher == nil {
		return result, nil
	}
	return inv.selfCorrectWorkflowSelection(ctx, result, content, messages, rcaSummary, correlationID, llmCtx)
}

// handleWorkflowSelectionLoopResult classifies the runLLMLoop outcome for
// PhaseWorkflowDiscovery. When early is non-nil, the caller must return it
// (and err) immediately without further parsing. Otherwise content holds the
// tool-call (or text) payload to parse into an InvestigationResult.
func (inv *Investigator) handleWorkflowSelectionLoopResult(ctx context.Context, loopRes LoopResult, rcaSummary string, messages []llm.Message, llmCtx LLMInvocationContext) (content string, early *katypes.InvestigationResult, err error) {
	correlationID := llmCtx.CorrelationID
	switch r := loopRes.(type) {
	case *CancelledResult:
		cancelledResult := &katypes.InvestigationResult{
			RCASummary:          rcaSummary,
			Cancelled:           true,
			CancelledPhase:      string(katypes.PhaseWorkflowDiscovery),
			CancelledAtTurn:     r.Turn,
			AccumulatedMessages: messagesToAuditFormat(r.Messages),
		}
		if r.Tokens != nil {
			s := r.Tokens.Summary()
			cancelledResult.TokenUsage = &katypes.TokenUsageSummary{
				PromptTokens:     s.PromptTokens,
				CompletionTokens: s.CompletionTokens,
				TotalTokens:      s.TotalTokens,
			}
		}
		return "", cancelledResult, nil
	case *ExhaustedResult:
		return "", &katypes.InvestigationResult{
			RCASummary:        rcaSummary,
			HumanReviewNeeded: true,
			Reason:            fmt.Sprintf("%s during workflow selection (maxTurns=%d)", r.Reason, inv.maxTurns),
		}, nil
	case *SubmitNoWorkflowResult:
		inv.logger.Info("submit_result_no_workflow sentinel: classifying as no_matching_workflows",
			"correlation_id", correlationID)
		return "", &katypes.InvestigationResult{
			RCASummary:        rcaSummary,
			HumanReviewNeeded: true,
			HumanReviewReason: "no_matching_workflows",
			Reason:            "LLM explicitly declined workflow selection via submit_result_no_workflow",
		}, nil
	case *SubmitWithWorkflowResult:
		return r.Content, nil, nil
	case *SubmitResult:
		return r.Content, nil, nil
	case *TextResult:
		// #760 v2: LLM returned text instead of a tool call. Try parsing
		// first — the text may contain a valid investigation result (e.g.
		// problem_resolved or predictive_no_action where no workflow is
		// expected). Only fall through to parse-level retry when the
		// content cannot be parsed at all.
		if _, textErr := inv.resultParser.Parse(r.Content); textErr == nil {
			inv.logger.Info("workflow selection: parsed text response directly (no tool call)",
				"correlation_id", correlationID)
			return r.Content, nil, nil
		}
		return "", inv.workflowSelectionRetryOrHumanReview(ctx, r.Content, messages, rcaSummary, llmCtx, nil,
			"workflow selection: LLM did not use submit tool after retries"), nil
	}
	return "", nil, nil
}

// workflowSelectionRetryOrHumanReview performs one retryWorkflowSubmit
// attempt and, on exhaustion, classifies the investigation as
// no_matching_workflows (or cancelled, if the context ended). Shared by the
// "LLM returned unparseable text" and "LLM's tool payload failed structural
// parsing" branches of runWorkflowSelection. When parseErr is non-nil the
// exhaustion is logged at Error level (a real parse failure); otherwise at
// Info level (the LLM simply never called a submit tool).
func (inv *Investigator) workflowSelectionRetryOrHumanReview(ctx context.Context, lastContent string, messages []llm.Message, rcaSummary string, llmCtx LLMInvocationContext, parseErr error, failureReason string) *katypes.InvestigationResult {
	retryResult := inv.retryWorkflowSubmit(ctx, lastContent, messages, rcaSummary, llmCtx)
	if retryResult != nil {
		return retryResult
	}
	if ctx.Err() != nil {
		return &katypes.InvestigationResult{
			RCASummary:     rcaSummary,
			Cancelled:      true,
			CancelledPhase: string(katypes.PhaseWorkflowDiscovery),
		}
	}
	if parseErr != nil {
		inv.logger.Error(parseErr, "workflow selection parse failed after retries, classifying as no_matching_workflows",
			"correlation_id", llmCtx.CorrelationID)
	} else {
		inv.logger.Info("workflow selection: all retries exhausted, classifying as no_matching_workflows",
			"correlation_id", llmCtx.CorrelationID)
	}
	return &katypes.InvestigationResult{
		RCASummary:        rcaSummary,
		HumanReviewNeeded: true,
		HumanReviewReason: "no_matching_workflows",
		Reason:            failureReason,
	}
}

// selfCorrectWorkflowSelection runs the catalog-validated self-correction
// loop (up to maxSelfCorrectionAttempts) for the parsed workflow-selection
// result, then applies catalog enrichment and target-alignment checks to the
// final accepted (or exhausted) result.
func (inv *Investigator) selfCorrectWorkflowSelection(ctx context.Context, result *katypes.InvestigationResult, content string, messages []llm.Message, rcaSummary, correlationID string, llmCtx LLMInvocationContext) (*katypes.InvestigationResult, error) {
	validator, fetchErr := inv.pipeline.CatalogFetcher.FetchValidator(ctx)
	if fetchErr != nil {
		inv.logger.Error(fetchErr, "workflow catalog unavailable, requiring human review")
		result.HumanReviewNeeded = true
		result.HumanReviewReason = "catalog_unavailable"
		result.Reason = fmt.Sprintf("workflow catalog unavailable: %s", fetchErr)
		return result, nil
	}

	attempt := 0
	correctionFn := func(r *katypes.InvestigationResult, validationErr error) (*katypes.InvestigationResult, error) {
		attempt++
		var errStrs []string
		if validationErr != nil {
			errStrs = []string{validationErr.Error()}
		}
		inv.emitValidationEvent(ctx, attempt, maxSelfCorrectionAttempts, false, errStrs, r.WorkflowID, correlationID)

		correctionMsg, renderErr := inv.renderCorrectionMessage(validationErr, attempt, maxSelfCorrectionAttempts)
		if renderErr != nil {
			inv.logger.Error(renderErr, "failed to render validation error template, using fallback")
			correctionMsg = fmt.Sprintf("Validation failed: %s. Please select a valid workflow.", validationErr)
		}
		messages = append(messages, llm.Message{Role: "assistant", Content: content})
		messages = append(messages, llm.Message{Role: "user", Content: correctionMsg})

		corrLoopRes, corrErr := inv.runLLMLoop(ctx, messages, katypes.PhaseWorkflowDiscovery, llmCtx)
		if corrErr != nil {
			return nil, corrErr
		}
		switch cr := corrLoopRes.(type) {
		case *CancelledResult:
			return nil, context.Canceled
		case *ExhaustedResult:
			r.HumanReviewNeeded = true
			r.Reason = fmt.Sprintf("self-correction: %s", cr.Reason)
			return r, nil
		case *SubmitNoWorkflowResult:
			return &katypes.InvestigationResult{
				RCASummary:        rcaSummary,
				HumanReviewNeeded: true,
				HumanReviewReason: "no_matching_workflows",
				Reason:            "LLM declined workflow during self-correction via submit_result_no_workflow",
			}, nil
		case *SubmitWithWorkflowResult:
			content = cr.Content
		case *SubmitResult:
			content = cr.Content
		case *TextResult:
			content = cr.Content
		}
		return inv.resultParser.Parse(content)
	}

	corrected, corrErr := validator.SelfCorrect(result, maxSelfCorrectionAttempts, correctionFn)
	if corrErr != nil {
		if errors.Is(corrErr, context.Canceled) || errors.Is(corrErr, context.DeadlineExceeded) {
			return &katypes.InvestigationResult{
				RCASummary:     rcaSummary,
				Cancelled:      true,
				CancelledPhase: string(katypes.PhaseWorkflowDiscovery),
			}, nil
		}
		return nil, fmt.Errorf("validation self-correction failed: %w", corrErr)
	}
	isValid := !corrected.HumanReviewNeeded
	var finalErrors []string
	if !isValid {
		finalErrors = []string{"validation exhausted all attempts"}
	}
	inv.emitValidationEvent(ctx, attempt+1, maxSelfCorrectionAttempts, isValid, finalErrors, corrected.WorkflowID, correlationID)
	enrichFromCatalog(corrected, validator)
	CheckWorkflowTargetAlignment(ctx, corrected, validator, correlationID, inv.auditStore, inv.logger)
	return corrected, nil
}

const maxParseRetries = 2

// retryWorkflowSubmit performs up to maxParseRetries correction attempts when
// the LLM returns text or unparseable JSON instead of calling a submit tool.
// Each retry sends a correction message with examples of both submit tools,
// with only the two submit tools available (prevents re-investigation).
// Returns non-nil *InvestigationResult on success or nil when retries exhaust.
func (inv *Investigator) retryWorkflowSubmit(ctx context.Context, lastContent string, history []llm.Message, rcaSummary string, llmCtx LLMInvocationContext) *katypes.InvestigationResult {
	tokens, correlationID, client, modelName, runtimeParams :=
		llmCtx.Tokens, llmCtx.CorrelationID, llmCtx.Client, llmCtx.ModelName, llmCtx.RuntimeParams
	submitOnlyTools := []llm.ToolDefinition{
		{
			Name:        SubmitResultWithWorkflowToolName,
			Description: "Submit investigation result WITH a selected workflow.",
			Parameters:  parser.WithWorkflowResultSchema(),
		},
		{
			Name:        SubmitResultNoWorkflowToolName,
			Description: "Submit investigation result when NO matching workflow exists.",
			Parameters:  parser.NoWorkflowResultSchema(),
		},
	}

	correctionTemplate := `Your response could not be parsed. You MUST call one of these tools:

1. If a workflow matches: call submit_result_with_workflow with JSON like:
   {"root_cause_analysis":{"summary":"..."},"selected_workflow":{"workflow_id":"...","confidence":0.9},"confidence":0.9}

2. If NO workflow matches: call submit_result_no_workflow with JSON like:
   {"root_cause_analysis":{"summary":"..."},"reasoning":"explanation why no workflow applies"}

Do NOT respond with plain text. You MUST call one of the above tools.`

	retryMessages := make([]llm.Message, len(history))
	copy(retryMessages, history)
	retryMessages = append(retryMessages,
		llm.Message{Role: "assistant", Content: lastContent},
	)

	for attempt := 0; attempt < maxParseRetries; attempt++ {
		if ctx.Err() != nil {
			return nil
		}
		inv.logger.Info("parse-level retry for workflow submit",
			"attempt", attempt+1,
			"max", maxParseRetries,
			"correlation_id", correlationID)

		retryMessages = append(retryMessages,
			llm.Message{Role: "user", Content: correctionTemplate},
		)

		retryEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
		retryEvent.EventAction = audit.ActionLLMRequest
		retryEvent.EventOutcome = audit.OutcomeSuccess
		retryEvent.Data["model"] = modelName
		retryEvent.Data["prompt_length"] = totalPromptLength(retryMessages)
		retryEvent.Data["prompt_preview"] = lastUserMessage(retryMessages, 500)
		retryEvent.Data["retry_attempt"] = attempt + 1
		retryEvent.Data["retry_max"] = maxParseRetries
		retryEvent.Data["phase"] = string(katypes.PhaseWorkflowDiscovery)
		retryEvent.Data["retry_reason"] = "parse_level_correction"
		audit.StoreBestEffort(ctx, inv.auditStore, retryEvent, inv.auditLog())

		resp, err := inv.chatOrStream(ctx, client, llm.ChatRequest{
			Messages: retryMessages,
			Tools:    submitOnlyTools,
			Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.InvestigationResultSchema()},
		}, attempt+1, string(katypes.PhaseWorkflowDiscovery), modelName, runtimeParams)
		if err != nil {
			inv.logger.Error(err, "retry LLM call failed",
				"correlation_id", correlationID)
			continue
		}
		if tokens != nil {
			tokens.Add(resp.Usage)
		}

		emitToSink(ctx, session.EventTypeReasoningDelta, attempt+1, string(katypes.PhaseWorkflowDiscovery), map[string]interface{}{
			"content":       resp.Message.Content,
			"retry_attempt": attempt + 1,
		})

		if len(resp.ToolCalls) > 0 {
			for _, tc := range resp.ToolCalls {
				switch tc.Name {
				case SubmitResultNoWorkflowToolName:
					inv.logger.Info("retry succeeded: submit_result_no_workflow",
						"correlation_id", correlationID)
					return &katypes.InvestigationResult{
						RCASummary:        rcaSummary,
						HumanReviewNeeded: true,
						HumanReviewReason: "no_matching_workflows",
						Reason:            "LLM used submit_result_no_workflow after retry",
					}
				case SubmitResultWithWorkflowToolName:
					inv.logger.Info("retry succeeded: submit_result_with_workflow",
						"correlation_id", correlationID)
					result, parseErr := inv.resultParser.Parse(tc.Arguments)
					if parseErr != nil {
						inv.logger.Error(parseErr, "retry submit_result_with_workflow parse failed",
							"correlation_id", correlationID)
						retryMessages = append(retryMessages, resp.Message)
						continue
					}
					return result
				}
			}
		}

		retryMessages = append(retryMessages, resp.Message)
	}
	return nil
}

func (inv *Investigator) runLLMLoop(ctx context.Context, messages []llm.Message, phase katypes.Phase, llmCtx LLMInvocationContext) (LoopResult, error) {
	tokens, correlationID, client, modelName, runtimeParams :=
		llmCtx.Tokens, llmCtx.CorrelationID, llmCtx.Client, llmCtx.ModelName, llmCtx.RuntimeParams
	toolDefs := inv.toolDefinitionsForPhase(phase)
	loopStart := time.Now()
	truncationRetried := false
	maxTokens := 0

	for turn := 0; turn < inv.maxTurns; turn++ {
		if ctx.Err() != nil {
			emitToSink(ctx, session.EventTypeCancelled, turn, string(phase), nil)
			return &CancelledResult{
				Messages: messages,
				Turn:     turn,
				Phase:    string(phase),
				Tokens:   tokens,
			}, nil
		}

		reqEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
		reqEvent.EventAction = audit.ActionLLMRequest
		reqEvent.EventOutcome = audit.OutcomeSuccess
		reqEvent.Data["model"] = modelName
		reqEvent.Data["prompt_length"] = totalPromptLength(messages)
		reqEvent.Data["prompt_preview"] = lastUserMessage(messages, 500)
		reqEvent.Data["toolsets_enabled"] = toolNames(toolDefs)
		reqEvent.Data["messages"] = messagesToAuditFormat(messages)
		audit.StoreBestEffort(ctx, inv.auditStore, reqEvent, inv.auditLog())

		chatReq := llm.ChatRequest{
			Messages: messages,
			Tools:    toolDefs,
			Options:  llm.ChatOptions{JSONMode: true, OutputSchema: submitResultSchemaForPhase(phase), MaxTokens: maxTokens},
		}
		resp, err := inv.chatOrStream(ctx, client, chatReq, turn, string(phase), modelName, runtimeParams)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				emitToSink(ctx, session.EventTypeCancelled, turn, string(phase), nil)
				return &CancelledResult{
					Messages: messages,
					Turn:     turn,
					Phase:    string(phase),
					Tokens:   tokens,
				}, nil
			}
			failEvent := audit.NewEvent(audit.EventTypeResponseFailed, correlationID)
			failEvent.EventAction = audit.ActionResponseFailed
			failEvent.EventOutcome = audit.OutcomeFailure
			failEvent.Data["error_message"] = err.Error()
			failEvent.Data["phase"] = string(phase)
			failEvent.Data["duration_seconds"] = time.Since(loopStart).Seconds()
			audit.StoreBestEffort(ctx, inv.auditStore, failEvent, inv.auditLog())
			return nil, fmt.Errorf("%s LLM call turn %d: %w", phase, turn, err)
		}

		if tokens != nil {
			tokens.Add(resp.Usage)
		}

		respEvent := audit.NewEvent(audit.EventTypeLLMResponse, correlationID)
		respEvent.EventAction = audit.ActionLLMResponse
		respEvent.EventOutcome = audit.OutcomeSuccess
		respEvent.Data["prompt_tokens"] = resp.Usage.PromptTokens
		respEvent.Data["completion_tokens"] = resp.Usage.CompletionTokens
		respEvent.Data["total_tokens"] = resp.Usage.TotalTokens
		respEvent.Data["has_analysis"] = resp.Message.Content != ""
		respEvent.Data["analysis_length"] = len(resp.Message.Content)
		respEvent.Data["analysis_preview"] = truncatePreview(resp.Message.Content, 500)
		respEvent.Data["analysis_full"] = resp.Message.Content
		respEvent.Data["analysis_content"] = resp.Message.Content
		respEvent.Data["tool_call_count"] = len(resp.ToolCalls)
		respEvent.Data["finish_reason"] = resp.FinishReason
		audit.StoreBestEffort(ctx, inv.auditStore, respEvent, inv.auditLog())

		emitToSink(ctx, session.EventTypeReasoningDelta, turn, string(phase), map[string]interface{}{
			"content_preview": truncatePreview(resp.Message.Content, 200),
			"tool_call_count": len(resp.ToolCalls),
		})

		if len(resp.ToolCalls) > 0 {
			for _, tc := range resp.ToolCalls {
				if sr := sentinelResult(tc); sr != nil {
					inv.logger.Info("sentinel detected",
						"tool", tc.Name,
						"phase", string(phase),
						"correlation_id", correlationID)
					return sr, nil
				}
			}

			assistantMsg := resp.Message
			assistantMsg.ToolCalls = resp.ToolCalls
			messages = append(messages, assistantMsg)

			toolResults := make([]string, len(resp.ToolCalls))
			var g errgroup.Group
			for i, tc := range resp.ToolCalls {
				emitToSink(ctx, session.EventTypeToolCallStart, turn, string(phase), map[string]interface{}{
					"tool_name":  tc.Name,
					"tool_index": i,
				})
				g.Go(func() error {
					toolResults[i] = inv.executeTool(ctx, tc.Name, json.RawMessage(tc.Arguments))
					return nil
				})
			}
			_ = g.Wait()

			for i, tc := range resp.ToolCalls {
				emitToSink(ctx, session.EventTypeToolResult, turn, string(phase), map[string]interface{}{
					"tool_name":      tc.Name,
					"tool_index":     i,
					"result_preview": truncatePreview(toolResults[i], 200),
				})

				tcEvent := audit.NewEvent(audit.EventTypeLLMToolCall, correlationID)
				tcEvent.EventAction = audit.ActionToolExecution
				tcEvent.EventOutcome = audit.OutcomeSuccess
				tcEvent.Data["tool_call_index"] = i
				tcEvent.Data["tool_name"] = tc.Name
				tcEvent.Data["tool_arguments"] = tc.Arguments
				tcEvent.Data["tool_result"] = toolResults[i]
				tcEvent.Data["tool_result_preview"] = truncatePreview(toolResults[i], 500)
				audit.StoreBestEffort(ctx, inv.auditStore, tcEvent, inv.auditLog())

				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    toolResults[i],
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
				})
			}
			if inv.pipeline.AnomalyDetector.TotalExceeded() {
				return &ExhaustedResult{Reason: "tool budget exhausted"}, nil
			}
			continue
		}

		if resp.FinishReason == llm.FinishReasonLength && !truncationRetried {
			truncationRetried = true
			maxTokens = escalateMaxTokens(resp.Usage.CompletionTokens)
			inv.logger.Info("LLM response truncated, retrying with escalated MaxTokens",
				"phase", string(phase),
				"escalated_max_tokens", maxTokens,
				"correlation_id", correlationID)

			truncEvent := audit.NewEvent(audit.EventTypeLLMResponse, correlationID)
			truncEvent.EventAction = "truncation_detected"
			truncEvent.EventOutcome = audit.OutcomeFailure
			truncEvent.Data["has_analysis"] = resp.Message.Content != ""
			truncEvent.Data["analysis_length"] = len(resp.Message.Content)
			truncEvent.Data["analysis_preview"] = truncatePreview(resp.Message.Content, 500)
			truncEvent.Data["finish_reason"] = resp.FinishReason
			truncEvent.Data["escalated_max_tokens"] = maxTokens
			truncEvent.Data["truncated_content_length"] = len(resp.Message.Content)
			audit.StoreBestEffort(ctx, inv.auditStore, truncEvent, inv.auditLog())

			messages = append(messages, resp.Message)
			messages = append(messages, llm.Message{
				Role:    "user",
				Content: "Your previous response was truncated (output token limit reached). Please provide your complete response. Use the submit_result tool to deliver your structured result.",
			})
			continue
		}

		return &TextResult{Content: resp.Message.Content}, nil
	}

	return &ExhaustedResult{Reason: "max turns exhausted"}, nil
}

// escalateMaxTokens computes a higher MaxTokens value for truncation recovery.
// If the truncated response used N completion tokens, we request 2x. Falls back
// to a default of 8192 if the usage data is unavailable.
func (inv *Investigator) chatOrStream(ctx context.Context, client llm.Client, req llm.ChatRequest, turn int, phase string, modelName string, runtimeParams llm.RuntimeParams) (llm.ChatResponse, error) {
	sink := session.EventSinkFromContext(ctx)
	if sink == nil {
		inv.logger.Info("chatOrStream: sink is nil, falling back to non-streaming Chat",
			"turn", turn, "phase", phase,
			"session_id", session.SessionIDFromContext(ctx))
		return llm.ChatWithParams(ctx, client, req, runtimeParams)
	}
	inv.logger.Info("chatOrStream: sink active, using streaming",
		"turn", turn, "phase", phase,
		"session_id", session.SessionIDFromContext(ctx),
		"sink_ptr", fmt.Sprintf("%p", sink),
		"diag_sent", diagSendOK.Load(),
		"diag_dropped", diagSendDrop.Load(),
		"diag_nil", diagSinkNil.Load())

	temp := runtimeParams.Temperature
	req.Options.Temperature = &temp

	callCtx := ctx
	var cancel context.CancelFunc
	if runtimeParams.TimeoutSeconds > 0 {
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(runtimeParams.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	return client.StreamChat(callCtx, req, func(evt llm.ChatStreamEvent) error {
		if evt.Delta != "" {
			emitToSink(ctx, session.EventTypeTokenDelta, turn, phase, map[string]interface{}{
				"delta": evt.Delta,
			})
		}
		return nil
	})
}

// emitToSink sends an InvestigationEvent to the context-carried event sink
// using non-blocking send semantics. If the sink is nil (no subscriber) or
// the channel buffer is full, the event is silently dropped. This ensures
// the investigation loop is never blocked by a slow SSE consumer.
func emitToSink(ctx context.Context, eventType string, turn int, phase string, data map[string]interface{}) {
	sink := session.EventSinkFromContext(ctx)
	if sink == nil {
		diagSinkNil.Add(1)
		return
	}
	var raw json.RawMessage
	if data != nil {
		var err error
		raw, err = json.Marshal(data)
		if err != nil {
			return
		}
	}
	event := session.InvestigationEvent{
		Type:  eventType,
		Turn:  turn,
		Phase: phase,
		Data:  raw,
	}
	select {
	case sink <- event:
		diagSendOK.Add(1)
	default:
		diagSendDrop.Add(1)
	}
}

// emitCancellationAudit emits an investigation-level cancellation event
// carrying the phase, turn, token usage, and accumulated messages at the point
// of cancellation. Enriched per COR-2 (token cost attribution), AUD-4
// (session cross-reference), AUD-6 (messages for forensic RAG), and SEC-1
// (content cap at 64KB). The context may already be cancelled so we use
// context.Background() for the audit store call (fire-and-forget per ADR-038).
func (inv *Investigator) emitCancellationAudit(ctx context.Context, result *katypes.InvestigationResult, correlationID string) {
	event := audit.NewEvent(audit.EventTypeInvestigationCancelled, correlationID, audit.WithSessionID(session.SessionIDFromContext(ctx)))
	event.EventAction = audit.ActionInvestigationCancelled
	event.EventOutcome = audit.OutcomeFailure
	event.Data["cancelled_phase"] = result.CancelledPhase
	event.Data["cancelled_at_turn"] = result.CancelledAtTurn
	if result.TokenUsage != nil {
		event.Data["total_prompt_tokens"] = result.TokenUsage.PromptTokens
		event.Data["total_completion_tokens"] = result.TokenUsage.CompletionTokens
		event.Data["total_tokens"] = result.TokenUsage.TotalTokens
	}
	if len(result.AccumulatedMessages) > 0 {
		if b, err := json.Marshal(result.AccumulatedMessages); err == nil {
			s := string(b)
			if len(s) > maxForensicPayloadBytes {
				s = s[:maxForensicPayloadBytes]
			}
			event.Data["accumulated_messages"] = s
		}
	}
	audit.StoreBestEffort(context.Background(), inv.auditStore, event, inv.logger)
}
