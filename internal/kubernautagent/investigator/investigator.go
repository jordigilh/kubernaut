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
	"log/slog"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/sanitization"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/summarizer"
)

const maxSelfCorrectionAttempts = 3

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
// ScopeResolver determines whether a Kubernetes kind is cluster-scoped (#763).
type ScopeResolver interface {
	IsClusterScoped(kind string) (bool, error)
}

type Config struct {
	Client        llm.Client
	Builder       *prompt.Builder
	ResultParser  *parser.ResultParser
	Enricher      *enrichment.Enricher
	AuditStore    audit.AuditStore
	Logger        *slog.Logger
	MaxTurns      int
	PhaseTools    katypes.PhaseToolMap
	Registry      *registry.Registry
	Pipeline      Pipeline
	ModelName     string
	ScopeResolver ScopeResolver
	Swappable     *llm.SwappableClient
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
	logger        *slog.Logger
	maxTurns      int
	phaseTools    katypes.PhaseToolMap
	registry      *registry.Registry
	pipeline      Pipeline
	modelName     string
	scopeResolver ScopeResolver
	swappable     *llm.SwappableClient
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
	}
}

// Investigate runs the two-invocation investigation and returns the result.
// Per BR-AUDIT-005, all audit events use signal.RemediationID as correlation ID
// so that DataStorage queries by remediation_id return the full investigation trail.
func (inv *Investigator) Investigate(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error) {
	inv.pipeline.AnomalyDetector.Reset()

	// #783: Pin client and model name for the duration of this investigation.
	// Subsequent hot-reload swaps do not affect an in-flight investigation.
	client := inv.client
	modelName := inv.modelName
	if inv.swappable != nil {
		pinned := inv.swappable.Snapshot()
		client = llm.NewInstrumentedClient(pinned)
		modelName = inv.swappable.ModelName()
	}

	correlationID := signal.RemediationID
	enrichmentCache := make(map[string]*enrichment.EnrichmentResult)

	signalKind, signalName, signalNS := ResolveEnrichmentTarget(signal, nil)
	signalNS = inv.normalizeNamespace(signalKind, signalNS)
	enrichData := inv.resolveEnrichmentCached(ctx, enrichmentCache, signalKind, signalName, signalNS, signal.IncidentID)
	promptEnrichment := toPromptEnrichment(enrichData)
	tokens := &TokenAccumulator{}

	rcaResult, err := inv.runRCA(ctx, signal, promptEnrichment, tokens, correlationID, client, modelName)
	if err != nil {
		return nil, fmt.Errorf("RCA invocation: %w", err)
	}

	inv.emitRCAComplete(ctx, rcaResult, tokens, correlationID)

	if rcaResult.HumanReviewNeeded {
		backfillSeverity(rcaResult, signal)
		attachDetectedLabels(rcaResult, enrichData)
		InjectRemediationTarget(rcaResult, signal, enrichData)
		injectTargetResourceParameters(rcaResult)
		inv.emitResponseComplete(ctx, rcaResult, tokens, correlationID)
		return rcaResult, nil
	}

	// GAP-001 / ADR-056: Re-enrich using RCA-identified remediation target if different.
	// H3-fix: retain pre-RCA enrichment if re-enrichment fails.
	workflowSignal := signal
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
			inv.logger.Warn("enrichment owner chain hard-failed, triggering rca_incomplete",
				slog.String("error", reEnriched.OwnerChainError.Error()),
			)
			rcaResult.HumanReviewNeeded = true
			rcaResult.HumanReviewReason = "rca_incomplete"
			backfillSeverity(rcaResult, signal)
			attachDetectedLabels(rcaResult, enrichData)
			InjectRemediationTarget(rcaResult, workflowSignal, enrichData)
			injectTargetResourceParameters(rcaResult)
			inv.emitResponseComplete(ctx, rcaResult, tokens, correlationID)
			return rcaResult, nil
		}

		if reEnriched != nil && !allLabelDetectionsFailed(reEnriched.DetectedLabels) {
			enrichData = reEnriched
		} else if reEnriched != nil {
			inv.logger.Warn("re-enrichment labels all failed (RCA target not found), preserving signal-target labels",
				"rca_target", postRCAKind+"/"+postRCAName)
		} else {
			inv.logger.Warn("re-enrichment returned nil, retaining pre-RCA enrichment data")
		}
		promptEnrichment = toPromptEnrichment(enrichData)

		workflowSignal.ResourceKind = postRCAKind
		workflowSignal.ResourceName = postRCAName
		workflowSignal.Namespace = postRCANS
	}

	inv.pipeline.AnomalyDetector.Reset()

	p1Ctx := BuildPhase1Context(rcaResult)

	workflowResult, err := inv.runWorkflowSelection(ctx, workflowSignal, rcaResult.RCASummary, promptEnrichment, p1Ctx, tokens, correlationID, client, modelName)
	if err != nil {
		return nil, fmt.Errorf("workflow selection invocation: %w", err)
	}

	if workflowResult.RCASummary == "" {
		workflowResult.RCASummary = rcaResult.RCASummary
	}

	MergePhase1Fallbacks(workflowResult, p1Ctx)

	backfillSeverity(workflowResult, signal)
	attachDetectedLabels(workflowResult, enrichData)
	InjectRemediationTarget(workflowResult, workflowSignal, enrichData)
	injectTargetResourceParameters(workflowResult)
	inv.emitResponseComplete(ctx, workflowResult, tokens, correlationID)
	return workflowResult, nil
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
	result, err := inv.enricher.Enrich(ctx, kind, name, namespace, "", incidentID)
	if err != nil {
		inv.logger.Warn("enrichment failed", slog.String("error", err.Error()))
		return nil
	}
	return result
}

func (inv *Investigator) runRCA(ctx context.Context, signal katypes.SignalContext, enrichData *prompt.EnrichmentData, tokens *TokenAccumulator, correlationID string, client llm.Client, modelName string) (*katypes.InvestigationResult, error) {
	systemPrompt, err := inv.builder.RenderInvestigation(signalToPrompt(signal))
	if err != nil {
		return nil, fmt.Errorf("rendering investigation prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Investigate: %s %s in %s — %s", signal.Severity, signal.Name, signal.Namespace, signal.Message)},
	}

	loopRes, err := inv.runLLMLoop(ctx, messages, katypes.PhaseRCA, tokens, correlationID, client, modelName)
	if err != nil {
		return nil, err
	}

	var content string
	switch r := loopRes.(type) {
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
		if retried := inv.retryRCASubmit(ctx, content, messages, tokens, correlationID, client, modelName); retried != nil {
			result = retried
			parseErr = nil
		}
	}
	if parseErr != nil {
		inv.logger.Warn("RCA parse failed after retry, treating as summary",
			slog.String("error", parseErr.Error()),
			slog.String("correlation_id", correlationID))
		return &katypes.InvestigationResult{
			RCASummary: content,
		}, nil
	}

	// Same-kind sentinel validation gate (Issue #847): when the LLM's
	// remediation_target.kind matches the signal's resource_kind, the LLM may
	// be targeting the symptom reporter instead of the actual root cause.
	// Inject a single correction message and re-run. Max 1 retry.
	result = inv.sameKindValidationGate(ctx, result, signal, messages, tokens, correlationID, client, modelName)

	// Defense-in-depth: RCA phase must never abort the pipeline via
	// needs_human_review. Only max-turns exhaustion (above) is a valid RCA abort.
	// Aligned with HAPI v1.2.1 where needs_human_review is parser-driven in Phase 3.
	if result.HumanReviewNeeded {
		inv.logger.Info("clearing HumanReviewNeeded set during RCA (parser-driven in Phase 3 only)",
			slog.String("reason", result.HumanReviewReason),
			slog.String("correlation_id", correlationID))
		result.HumanReviewNeeded = false
		result.HumanReviewReason = ""
	}

	return result, nil
}

const maxRCAParseRetries = 1

// retryRCASubmit performs a single correction attempt when the RCA parse fails
// (e.g. double-serialized JSON that wasn't caught by the unwrap heuristic, or
// garbage fields). Mirrors retryWorkflowSubmit but scoped to the RCA phase.
func (inv *Investigator) retryRCASubmit(ctx context.Context, lastContent string, history []llm.Message, tokens *TokenAccumulator, correlationID string, client llm.Client, modelName string) *katypes.InvestigationResult {
	submitOnlyTools := []llm.ToolDefinition{
		{
			Name:        SubmitResultToolName,
			Description: "Submit root cause analysis result.",
			Parameters:  parser.RCAResultSchema(),
		},
	}

	correctionMsg := `Your response could not be parsed. You MUST call submit_result with a JSON object like:
{"root_cause_analysis":{"summary":"...","severity":"critical","signal_name":"SignalName","contributing_factors":["factor1"],"remediation_target":{"kind":"Deployment","name":"resource","namespace":"ns"}},"confidence":0.9}

CRITICAL: root_cause_analysis must be a JSON object, NOT a string. Do NOT wrap it in quotes.`

	retryMessages := make([]llm.Message, len(history))
	copy(retryMessages, history)
	retryMessages = append(retryMessages,
		llm.Message{Role: "assistant", Content: lastContent},
	)

	for attempt := 0; attempt < maxRCAParseRetries; attempt++ {
		inv.logger.Info("parse-level retry for RCA submit",
			slog.Int("attempt", attempt+1),
			slog.Int("max", maxRCAParseRetries),
			slog.String("correlation_id", correlationID))

		retryMessages = append(retryMessages,
			llm.Message{Role: "user", Content: correctionMsg},
		)

		retryEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
		retryEvent.EventAction = audit.ActionLLMRequest
		retryEvent.EventOutcome = audit.OutcomeSuccess
		retryEvent.Data["model"] = modelName
		retryEvent.Data["retry_attempt"] = attempt + 1
		retryEvent.Data["retry_max"] = maxRCAParseRetries
		retryEvent.Data["phase"] = string(katypes.PhaseRCA)
		retryEvent.Data["retry_reason"] = "rca_parse_correction"
		audit.StoreBestEffort(ctx, inv.auditStore, retryEvent, inv.logger)

		resp, err := client.Chat(ctx, llm.ChatRequest{
			Messages: retryMessages,
			Tools:    submitOnlyTools,
			Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
		})
		if err != nil {
			inv.logger.Warn("RCA retry LLM call failed",
				slog.String("error", err.Error()),
				slog.String("correlation_id", correlationID))
			continue
		}
		if tokens != nil {
			tokens.Add(resp.Usage)
		}

		for _, tc := range resp.ToolCalls {
			if tc.Name == SubmitResultToolName {
				result, parseErr := inv.resultParser.Parse(tc.Arguments)
				if parseErr != nil {
					inv.logger.Warn("RCA retry parse still failed",
						slog.String("error", parseErr.Error()),
						slog.String("correlation_id", correlationID))
					retryMessages = append(retryMessages, resp.Message)
					continue
				}
				inv.logger.Info("RCA retry succeeded",
					slog.String("correlation_id", correlationID))
				return result
			}
		}

		if resp.Message.Content != "" {
			result, parseErr := inv.resultParser.Parse(resp.Message.Content)
			if parseErr == nil {
				inv.logger.Info("RCA retry succeeded from message content",
					slog.String("correlation_id", correlationID))
				return result
			}
		}

		retryMessages = append(retryMessages, resp.Message)
	}
	return nil
}

// sameKindValidationGate checks whether the LLM's remediation_target.kind
// matches the signal's resource_kind. When they match, the LLM may be targeting
// the symptom reporter (e.g., Node) rather than the actual root cause (e.g.,
// Deployment). This gate injects one correction message requesting the LLM to
// re-evaluate its target. If the retry also returns the same kind, the result
// is accepted as-is (the LLM explicitly confirmed its choice).
// Issue #847 / DD-HAPI-847, Layer 3: Programmatic Sentinel Validation Gate.
func (inv *Investigator) sameKindValidationGate(
	ctx context.Context,
	result *katypes.InvestigationResult,
	signal katypes.SignalContext,
	history []llm.Message,
	tokens *TokenAccumulator,
	correlationID string,
	client llm.Client,
	modelName string,
) *katypes.InvestigationResult {
	if signal.ResourceKind == "" || result.RemediationTarget.Kind == "" {
		return result
	}
	if !strings.EqualFold(result.RemediationTarget.Kind, signal.ResourceKind) {
		return result
	}

	inv.logger.Info("same-kind validation gate triggered: remediation_target.kind matches signal resource_kind",
		slog.String("target_kind", result.RemediationTarget.Kind),
		slog.String("signal_resource_kind", signal.ResourceKind),
		slog.String("correlation_id", correlationID))

	gateEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
	gateEvent.EventAction = "same_kind_validation_gate"
	gateEvent.EventOutcome = audit.OutcomeSuccess
	gateEvent.Data["signal_resource_kind"] = signal.ResourceKind
	gateEvent.Data["target_kind"] = result.RemediationTarget.Kind
	gateEvent.Data["target_name"] = result.RemediationTarget.Name
	audit.StoreBestEffort(ctx, inv.auditStore, gateEvent, inv.logger)

	correctionMsg := fmt.Sprintf(
		`Your remediation_target.kind is "%s", which is the same resource kind as the input signal. `+
			`Signals often propagate upward: workload-level issues manifest as conditions on parent resources `+
			`(e.g., pod memory leaks cause node DiskPressure, deployment misconfigurations appear as node conditions). `+
			`Please re-evaluate: is a child resource (Deployment, StatefulSet, DaemonSet, Pod) the actual root cause `+
			`whose configuration should be modified? If after re-evaluation you are confident the %s itself is the `+
			`correct remediation target, confirm by resubmitting with the same target and explain why in your `+
			`due_diligence.target_accuracy field.`,
		result.RemediationTarget.Kind,
		result.RemediationTarget.Kind,
	)

	submitOnlyTools := []llm.ToolDefinition{
		{
			Name:        SubmitResultToolName,
			Description: "Submit root cause analysis result.",
			Parameters:  parser.RCAResultSchema(),
		},
	}

	retryMessages := make([]llm.Message, len(history))
	copy(retryMessages, history)
	retryMessages = append(retryMessages,
		llm.Message{Role: "user", Content: correctionMsg},
	)

	resp, err := client.Chat(ctx, llm.ChatRequest{
		Messages: retryMessages,
		Tools:    submitOnlyTools,
		Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
	})
	if err != nil {
		inv.logger.Warn("same-kind validation gate retry failed, keeping original result",
			slog.String("error", err.Error()),
			slog.String("correlation_id", correlationID))
		return result
	}
	if tokens != nil {
		tokens.Add(resp.Usage)
	}

	var retryContent string
	for _, tc := range resp.ToolCalls {
		if tc.Name == SubmitResultToolName {
			retryContent = tc.Arguments
			break
		}
	}
	if retryContent == "" && resp.Message.Content != "" {
		retryContent = resp.Message.Content
	}
	if retryContent == "" {
		inv.logger.Warn("same-kind validation gate: no content in retry response, keeping original",
			slog.String("correlation_id", correlationID))
		return result
	}

	retryResult, parseErr := inv.resultParser.Parse(retryContent)
	if parseErr != nil {
		inv.logger.Warn("same-kind validation gate: retry parse failed, keeping original",
			slog.String("error", parseErr.Error()),
			slog.String("correlation_id", correlationID))
		return result
	}

	if retryResult.RemediationTarget.Kind == "" && result.RemediationTarget.Kind != "" {
		inv.logger.Warn("same-kind validation gate: retry lost remediation_target, keeping original",
			slog.String("original_target", result.RemediationTarget.Kind+"/"+result.RemediationTarget.Name),
			slog.String("correlation_id", correlationID))
		return result
	}

	inv.logger.Info("same-kind validation gate: accepted retry result",
		slog.String("original_target", result.RemediationTarget.Kind+"/"+result.RemediationTarget.Name),
		slog.String("retry_target", retryResult.RemediationTarget.Kind+"/"+retryResult.RemediationTarget.Name),
		slog.String("correlation_id", correlationID))
	return retryResult
}

func (inv *Investigator) runWorkflowSelection(ctx context.Context, signal katypes.SignalContext, rcaSummary string, enrichData *prompt.EnrichmentData, p1Ctx *prompt.Phase1Data, tokens *TokenAccumulator, correlationID string, client llm.Client, modelName string) (*katypes.InvestigationResult, error) {
	// Attach signal context so workflow discovery tools (list_available_actions,
	// list_workflows) can extract severity/component/environment/priority from
	// ctx instead of using hardcoded values. Fix for #779.
	ctx = katypes.WithSignalContext(ctx, signal)

	systemPrompt, err := inv.builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
		Signal:     signalToPrompt(signal),
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

	loopRes, err := inv.runLLMLoop(ctx, messages, katypes.PhaseWorkflowDiscovery, tokens, correlationID, client, modelName)
	if err != nil {
		return nil, err
	}

	var content string
	switch r := loopRes.(type) {
	case *ExhaustedResult:
		return &katypes.InvestigationResult{
			RCASummary:        rcaSummary,
			HumanReviewNeeded: true,
			Reason:            fmt.Sprintf("%s during workflow selection (maxTurns=%d)", r.Reason, inv.maxTurns),
		}, nil
	case *SubmitNoWorkflowResult:
		inv.logger.Info("submit_result_no_workflow sentinel: classifying as no_matching_workflows",
			slog.String("correlation_id", correlationID))
		return &katypes.InvestigationResult{
			RCASummary:        rcaSummary,
			HumanReviewNeeded: true,
			HumanReviewReason: "no_matching_workflows",
			Reason:            "LLM explicitly declined workflow selection via submit_result_no_workflow",
		}, nil
	case *SubmitWithWorkflowResult:
		content = r.Content
	case *SubmitResult:
		content = r.Content
	case *TextResult:
		// #760 v2: LLM returned text instead of a tool call. Try parsing
		// first — the text may contain a valid investigation result (e.g.
		// problem_resolved or predictive_no_action where no workflow is
		// expected). Only fall through to parse-level retry when the
		// content cannot be parsed at all.
		if _, textErr := inv.resultParser.Parse(r.Content); textErr == nil {
			inv.logger.Info("workflow selection: parsed text response directly (no tool call)",
				slog.String("correlation_id", correlationID))
			content = r.Content
		} else {
			retryResult := inv.retryWorkflowSubmit(ctx, r.Content, messages, rcaSummary, tokens, correlationID, client, modelName)
			if retryResult != nil {
				return retryResult, nil
			}
			inv.logger.Warn("workflow selection: all retries exhausted, classifying as no_matching_workflows",
				slog.String("correlation_id", correlationID))
			return &katypes.InvestigationResult{
				RCASummary:        rcaSummary,
				HumanReviewNeeded: true,
				HumanReviewReason: "no_matching_workflows",
				Reason:            "workflow selection: LLM did not use submit tool after retries",
			}, nil
		}
	}

	result, parseErr := inv.resultParser.Parse(content)
	if parseErr != nil {
		retryResult := inv.retryWorkflowSubmit(ctx, content, messages, rcaSummary, tokens, correlationID, client, modelName)
		if retryResult != nil {
			return retryResult, nil
		}
		inv.logger.Warn("workflow selection parse failed after retries, classifying as no_matching_workflows",
			slog.String("error", parseErr.Error()),
			slog.String("correlation_id", correlationID))
		return &katypes.InvestigationResult{
			RCASummary:        rcaSummary,
			HumanReviewNeeded: true,
			HumanReviewReason: "no_matching_workflows",
			Reason:            fmt.Sprintf("workflow selection: LLM did not produce parseable result: %s", parseErr),
		}, nil
	}

	if inv.pipeline.CatalogFetcher != nil {
		validator, fetchErr := inv.pipeline.CatalogFetcher.FetchValidator(ctx)
		if fetchErr != nil {
			inv.logger.Warn("workflow catalog unavailable, requiring human review",
				slog.String("error", fetchErr.Error()))
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

			correctionMsg := fmt.Sprintf("Validation failed: %s. Please select a valid workflow.", validationErr)
			messages = append(messages, llm.Message{Role: "assistant", Content: content})
			messages = append(messages, llm.Message{Role: "user", Content: correctionMsg})

			corrLoopRes, corrErr := inv.runLLMLoop(ctx, messages, katypes.PhaseWorkflowDiscovery, tokens, correlationID, client, modelName)
			if corrErr != nil {
				return nil, corrErr
			}
			switch cr := corrLoopRes.(type) {
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
			return nil, fmt.Errorf("validation self-correction failed: %w", corrErr)
		}
		isValid := !corrected.HumanReviewNeeded
		var finalErrors []string
		if !isValid {
			finalErrors = []string{"validation exhausted all attempts"}
		}
		inv.emitValidationEvent(ctx, attempt+1, maxSelfCorrectionAttempts, isValid, finalErrors, corrected.WorkflowID, correlationID)
		enrichFromCatalog(corrected, validator)
		return corrected, nil
	}

	return result, nil
}

const maxParseRetries = 2

// retryWorkflowSubmit performs up to maxParseRetries correction attempts when
// the LLM returns text or unparseable JSON instead of calling a submit tool.
// Each retry sends a correction message with examples of both submit tools,
// with only the two submit tools available (prevents re-investigation).
// Returns non-nil *InvestigationResult on success or nil when retries exhaust.
func (inv *Investigator) retryWorkflowSubmit(ctx context.Context, lastContent string, history []llm.Message, rcaSummary string, tokens *TokenAccumulator, correlationID string, client llm.Client, modelName string) *katypes.InvestigationResult {
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
		inv.logger.Info("parse-level retry for workflow submit",
			slog.Int("attempt", attempt+1),
			slog.Int("max", maxParseRetries),
			slog.String("correlation_id", correlationID))

		retryMessages = append(retryMessages,
			llm.Message{Role: "user", Content: correctionTemplate},
		)

		retryEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
		retryEvent.EventAction = audit.ActionLLMRequest
		retryEvent.EventOutcome = audit.OutcomeSuccess
		retryEvent.Data["model"] = modelName
		retryEvent.Data["retry_attempt"] = attempt + 1
		retryEvent.Data["retry_max"] = maxParseRetries
		retryEvent.Data["phase"] = string(katypes.PhaseWorkflowDiscovery)
		retryEvent.Data["retry_reason"] = "parse_level_correction"
		audit.StoreBestEffort(ctx, inv.auditStore, retryEvent, inv.logger)

		resp, err := client.Chat(ctx, llm.ChatRequest{
			Messages: retryMessages,
			Tools:    submitOnlyTools,
			Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.InvestigationResultSchema()},
		})
		if err != nil {
			inv.logger.Warn("retry LLM call failed",
				slog.String("error", err.Error()),
				slog.String("correlation_id", correlationID))
			continue
		}
		if tokens != nil {
			tokens.Add(resp.Usage)
		}

		if len(resp.ToolCalls) > 0 {
			for _, tc := range resp.ToolCalls {
				switch tc.Name {
				case SubmitResultNoWorkflowToolName:
					inv.logger.Info("retry succeeded: submit_result_no_workflow",
					slog.String("correlation_id", correlationID))
					return &katypes.InvestigationResult{
						RCASummary:        rcaSummary,
						HumanReviewNeeded: true,
						HumanReviewReason: "no_matching_workflows",
						Reason:            "LLM used submit_result_no_workflow after retry",
					}
				case SubmitResultWithWorkflowToolName:
					inv.logger.Info("retry succeeded: submit_result_with_workflow",
					slog.String("correlation_id", correlationID))
					result, parseErr := inv.resultParser.Parse(tc.Arguments)
					if parseErr != nil {
						inv.logger.Warn("retry submit_result_with_workflow parse failed",
							slog.String("error", parseErr.Error()),
							slog.String("correlation_id", correlationID))
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

// enrichFromCatalog backfills execution metadata from the workflow catalog
// into the InvestigationResult, replicating KA's behavior of including
// execution_engine, execution_bundle, execution_bundle_digest, and
// service_account_name so downstream controllers (WE) can use them.
func enrichFromCatalog(result *katypes.InvestigationResult, v *parser.Validator) {
	if result == nil || v == nil || result.WorkflowID == "" {
		return
	}
	meta, ok := v.GetWorkflowMeta(result.WorkflowID)
	if !ok {
		return
	}
	if result.ExecutionEngine == "" {
		result.ExecutionEngine = meta.ExecutionEngine
	}
	if result.ExecutionBundle == "" {
		result.ExecutionBundle = meta.ExecutionBundle
	}
	if result.ExecutionBundleDigest == "" {
		result.ExecutionBundleDigest = meta.ExecutionBundleDigest
	}
	if result.ServiceAccountName == "" {
		result.ServiceAccountName = meta.ServiceAccountName
	}
	result.WorkflowVersion = meta.Version
}

// runLLMLoop executes the multi-turn LLM conversation loop with tool
// execution routed through the registry. correlationID is propagated to
// all audit events per BR-AUDIT-005 (remediation_id as query key).
// Returns a sealed LoopResult; callers dispatch via type switch (#760 v2).
func (inv *Investigator) runLLMLoop(ctx context.Context, messages []llm.Message, phase katypes.Phase, tokens *TokenAccumulator, correlationID string, client llm.Client, modelName string) (LoopResult, error) {
	toolDefs := inv.toolDefinitionsForPhase(phase)
	loopStart := time.Now()
	truncationRetried := false
	maxTokens := 0

	for turn := 0; turn < inv.maxTurns; turn++ {
		reqEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
		reqEvent.EventAction = audit.ActionLLMRequest
		reqEvent.EventOutcome = audit.OutcomeSuccess
		reqEvent.Data["model"] = modelName
		reqEvent.Data["prompt_length"] = totalPromptLength(messages)
		reqEvent.Data["prompt_preview"] = lastUserMessage(messages, 500)
		reqEvent.Data["toolsets_enabled"] = toolNames(toolDefs)
		audit.StoreBestEffort(ctx, inv.auditStore, reqEvent, inv.logger)

		resp, err := client.Chat(ctx, llm.ChatRequest{
			Messages: messages,
			Tools:    toolDefs,
			Options:  llm.ChatOptions{JSONMode: true, OutputSchema: submitResultSchemaForPhase(phase), MaxTokens: maxTokens},
		})
		if err != nil {
			failEvent := audit.NewEvent(audit.EventTypeResponseFailed, correlationID)
			failEvent.EventAction = audit.ActionResponseFailed
			failEvent.EventOutcome = audit.OutcomeFailure
			failEvent.Data["error_message"] = err.Error()
			failEvent.Data["phase"] = string(phase)
			failEvent.Data["duration_seconds"] = time.Since(loopStart).Seconds()
			audit.StoreBestEffort(ctx, inv.auditStore, failEvent, inv.logger)
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
		respEvent.Data["tool_call_count"] = len(resp.ToolCalls)
		respEvent.Data["finish_reason"] = resp.FinishReason
		audit.StoreBestEffort(ctx, inv.auditStore, respEvent, inv.logger)

		if len(resp.ToolCalls) > 0 {
			for _, tc := range resp.ToolCalls {
				if sr := sentinelResult(tc); sr != nil {
					inv.logger.Info("sentinel detected",
						slog.String("tool", tc.Name),
						slog.String("phase", string(phase)),
						slog.String("correlation_id", correlationID))
					return sr, nil
				}
			}

			messages = append(messages, resp.Message)
			for i, tc := range resp.ToolCalls {
				toolResult := inv.executeTool(ctx, tc.Name, json.RawMessage(tc.Arguments))

				tcEvent := audit.NewEvent(audit.EventTypeLLMToolCall, correlationID)
				tcEvent.EventAction = audit.ActionToolExecution
				tcEvent.EventOutcome = audit.OutcomeSuccess
				tcEvent.Data["tool_call_index"] = i
				tcEvent.Data["tool_name"] = tc.Name
				tcEvent.Data["tool_arguments"] = tc.Arguments
				tcEvent.Data["tool_result"] = toolResult
				tcEvent.Data["tool_result_preview"] = truncatePreview(toolResult, 500)
				audit.StoreBestEffort(ctx, inv.auditStore, tcEvent, inv.logger)

				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    toolResult,
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
			inv.logger.Warn("LLM response truncated, retrying with escalated MaxTokens",
				slog.String("phase", string(phase)),
				slog.Int("escalated_max_tokens", maxTokens),
				slog.String("correlation_id", correlationID))

			truncEvent := audit.NewEvent(audit.EventTypeLLMResponse, correlationID)
			truncEvent.EventAction = "truncation_detected"
			truncEvent.EventOutcome = audit.OutcomeFailure
			truncEvent.Data["finish_reason"] = resp.FinishReason
			truncEvent.Data["escalated_max_tokens"] = maxTokens
			truncEvent.Data["truncated_content_length"] = len(resp.Message.Content)
			audit.StoreBestEffort(ctx, inv.auditStore, truncEvent, inv.logger)

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
func escalateMaxTokens(completionTokens int) int {
	if completionTokens > 0 {
		escalated := completionTokens * 2
		if escalated > 16384 {
			return 16384
		}
		return escalated
	}
	return 8192
}

func totalPromptLength(messages []llm.Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
	}
	return total
}

func lastUserMessage(messages []llm.Message, maxLen int) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return truncatePreview(messages[i].Content, maxLen)
		}
	}
	return ""
}

func truncatePreview(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func toolNames(defs []llm.ToolDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

func (inv *Investigator) toolDefinitionsForPhase(phase katypes.Phase) []llm.ToolDefinition {
	var defs []llm.ToolDefinition
	if inv.registry != nil {
		phaseTools := inv.registry.ToolsForPhase(phase, inv.phaseTools)
		defs = make([]llm.ToolDefinition, 0, len(phaseTools)+2)
		for _, t := range phaseTools {
			defs = append(defs, llm.ToolDefinition{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			})
		}
	}

	if phase == katypes.PhaseWorkflowDiscovery {
		defs = append(defs,
			llm.ToolDefinition{
				Name:        SubmitResultWithWorkflowToolName,
				Description: "Submit investigation result WITH a selected workflow. Call this when you have identified a matching workflow.",
				Parameters:  parser.WithWorkflowResultSchema(),
			},
			llm.ToolDefinition{
				Name:        SubmitResultNoWorkflowToolName,
				Description: "Submit investigation result when NO matching workflow exists. Call this when none of the available workflows can remediate the incident.",
				Parameters:  parser.NoWorkflowResultSchema(),
			},
		)
	} else {
		defs = append(defs, llm.ToolDefinition{
			Name:        SubmitResultToolName,
			Description: "Submit the final investigation result as structured JSON. Call this tool when your analysis is complete.",
			Parameters:  submitResultSchemaForPhase(phase),
		})
	}
	return defs
}

// submitResultSchemaForPhase returns the phase-appropriate JSON Schema for
// the submit_result tool. RCA uses a restricted schema without workflow/escalation
// fields; workflow discovery uses the full InvestigationResultSchema.
func submitResultSchemaForPhase(phase katypes.Phase) json.RawMessage {
	if phase == katypes.PhaseRCA {
		return parser.RCAResultSchema()
	}
	return parser.InvestigationResultSchema()
}

func (inv *Investigator) executeTool(ctx context.Context, name string, args json.RawMessage) string {
	if inv.registry == nil {
		return toolErrorJSON("no registry configured for tool " + name)
	}

	if ar := inv.pipeline.AnomalyDetector.CheckToolCall(name, args); !ar.Allowed {
		inv.logger.Warn("anomaly detector rejected tool call",
			slog.String("tool", name),
			slog.String("reason", ar.Reason),
		)
		return toolErrorJSON(ar.Reason)
	}

	result, err := inv.registry.Execute(ctx, name, args)
	if err != nil {
		inv.logger.Warn("tool execution failed",
			slog.String("tool", name),
			slog.String("error", err.Error()),
		)
		if ar := inv.pipeline.AnomalyDetector.RecordFailure(name, args); !ar.Allowed {
			return toolErrorJSON(ar.Reason)
		}
		return toolErrorJSON(err.Error())
	}

	if inv.pipeline.Sanitizer != nil {
		sanitized, sanitizeErr := inv.pipeline.Sanitizer.Run(ctx, result)
		if sanitizeErr != nil {
			inv.logger.Warn("sanitization failed, returning raw output",
				slog.String("tool", name),
				slog.String("error", sanitizeErr.Error()),
			)
		} else {
			result = sanitized
		}
	}

	if inv.pipeline.Summarizer != nil {
		summarized, sumErr := inv.pipeline.Summarizer.MaybeSummarize(ctx, name, result)
		if sumErr != nil {
			inv.logger.Warn("summarization failed, returning unsummarized output",
				slog.String("tool", name),
				slog.String("error", sumErr.Error()),
			)
		} else {
			result = summarized
		}
	}

	if inv.pipeline.MaxToolOutputSize > 0 {
		result = summarizer.TruncateToolOutput(result, name, inv.pipeline.MaxToolOutputSize)
	}

	return result
}

func (inv *Investigator) emitResponseComplete(ctx context.Context, result *katypes.InvestigationResult, tokens *TokenAccumulator, correlationID string) {
	completeEvent := audit.NewEvent(audit.EventTypeResponseComplete, correlationID)
	completeEvent.EventAction = audit.ActionResponseSent
	completeEvent.EventOutcome = audit.OutcomeSuccess
	for k, v := range tokens.AuditData() {
		completeEvent.Data[k] = v
	}
	if b, err := json.Marshal(ResultToAuditJSON(result)); err == nil {
		completeEvent.Data["response_data"] = string(b)
	}
	audit.StoreBestEffort(ctx, inv.auditStore, completeEvent, inv.logger)
}

func (inv *Investigator) emitRCAComplete(ctx context.Context, result *katypes.InvestigationResult, tokens *TokenAccumulator, correlationID string) {
	ev := audit.NewEvent(audit.EventTypeRCAComplete, correlationID)
	ev.EventAction = audit.ActionLLMResponse
	ev.EventOutcome = audit.OutcomeSuccess
	for k, v := range tokens.AuditData() {
		ev.Data[k] = v
	}
	if b, err := json.Marshal(ResultToAuditJSON(result)); err == nil {
		ev.Data["response_data"] = string(b)
	}
	audit.StoreBestEffort(ctx, inv.auditStore, ev, inv.logger)
}

func ResultToAuditJSON(r *katypes.InvestigationResult) map[string]interface{} {
	m := map[string]interface{}{
		"rca_summary":        r.RCASummary,
		"severity":           r.Severity,
		"confidence":         r.Confidence,
		"needs_human_review": r.HumanReviewNeeded,
	}
	if r.WorkflowID != "" {
		m["workflow_id"] = r.WorkflowID
	}
	if r.ExecutionBundle != "" {
		m["execution_bundle"] = r.ExecutionBundle
	}
	if len(r.ContributingFactors) > 0 {
		m["contributing_factors"] = r.ContributingFactors
	}
	if r.HumanReviewReason != "" {
		m["human_review_reason"] = r.HumanReviewReason
	} else if r.Reason != "" {
		m["human_review_reason"] = r.Reason
	}
	if len(r.Warnings) > 0 {
		m["warnings"] = r.Warnings
	}
	if len(r.Parameters) > 0 {
		m["parameters"] = r.Parameters
	}
	if r.RemediationTarget.Kind != "" {
		m["remediation_target"] = map[string]interface{}{
			"kind":      r.RemediationTarget.Kind,
			"name":      r.RemediationTarget.Name,
			"namespace": r.RemediationTarget.Namespace,
		}
	}
	if len(r.AlternativeWorkflows) > 0 {
		alts := make([]map[string]interface{}, len(r.AlternativeWorkflows))
		for i, alt := range r.AlternativeWorkflows {
			a := map[string]interface{}{"workflow_id": alt.WorkflowID}
			if alt.Rationale != "" {
				a["rationale"] = alt.Rationale
			}
			alts[i] = a
		}
		m["alternative_workflows"] = alts
	}
	if len(r.CausalChain) > 0 {
		m["causal_chain"] = r.CausalChain
	}
	if r.DueDiligence != nil {
		m["due_diligence"] = map[string]interface{}{
			"causal_completeness":    r.DueDiligence.CausalCompleteness,
			"target_accuracy":        r.DueDiligence.TargetAccuracy,
			"evidence_sufficiency":   r.DueDiligence.EvidenceSufficiency,
			"alternative_hypotheses": r.DueDiligence.AlternativeHypotheses,
			"scope_completeness":     r.DueDiligence.ScopeCompleteness,
			"proportionality":        r.DueDiligence.Proportionality,
			"regression_awareness":   r.DueDiligence.RegressionAwareness,
			"confidence_calibration": r.DueDiligence.ConfidenceCalibration,
		}
	}
	return m
}

func (inv *Investigator) emitValidationEvent(ctx context.Context, attempt, maxAttempts int, isValid bool, errors []string, workflowID, correlationID string) {
	valEvent := audit.NewEvent(audit.EventTypeValidationAttempt, correlationID)
	valEvent.EventAction = audit.ActionValidation
	if isValid {
		valEvent.EventOutcome = audit.OutcomeSuccess
	} else {
		valEvent.EventOutcome = audit.OutcomeFailure
	}
	valEvent.Data["attempt"] = attempt
	valEvent.Data["max_attempts"] = maxAttempts
	valEvent.Data["is_valid"] = isValid
	valEvent.Data["errors"] = errors
	valEvent.Data["workflow_id"] = workflowID
	valEvent.Data["is_final_attempt"] = attempt == maxAttempts
	audit.StoreBestEffort(ctx, inv.auditStore, valEvent, inv.logger)
}

func toolErrorJSON(msg string) string {
	payload := struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}{Status: "error", Error: msg}
	b, _ := json.Marshal(payload)
	return string(b)
}

// BuildPhase1Context extracts structured assessment fields from the Phase 1
// InvestigationResult for propagation into Phase 3 (HAPI parity: #715).
func BuildPhase1Context(rcaResult *katypes.InvestigationResult) *prompt.Phase1Data {
	if rcaResult == nil {
		return nil
	}
	return &prompt.Phase1Data{
		Severity:            rcaResult.Severity,
		ContributingFactors: rcaResult.ContributingFactors,
		RemediationTarget: prompt.Phase1RemediationTarget{
			Kind:      rcaResult.RemediationTarget.Kind,
			Name:      rcaResult.RemediationTarget.Name,
			Namespace: rcaResult.RemediationTarget.Namespace,
		},
		InvestigationOutcome:  rcaResult.InvestigationOutcome,
		Confidence:            rcaResult.Confidence,
		InvestigationAnalysis: rcaResult.InvestigationAnalysis,
		CausalChain:           rcaResult.CausalChain,
		DueDiligence:          rcaResult.DueDiligence,
	}
}

// MergePhase1Fallbacks applies Phase 1 assessment fields to the Phase 3 result
// when Phase 3 did not produce them. Matches HAPI's result.setdefault() pattern:
// Phase 3 values always take precedence; Phase 1 fills in gaps only.
func MergePhase1Fallbacks(result *katypes.InvestigationResult, p1 *prompt.Phase1Data) {
	if result == nil || p1 == nil {
		return
	}
	if result.Severity == "" && p1.Severity != "" {
		result.Severity = p1.Severity
	}
	if len(result.ContributingFactors) == 0 && len(p1.ContributingFactors) > 0 {
		result.ContributingFactors = p1.ContributingFactors
	}
	if result.Confidence == 0 && p1.Confidence > 0 {
		result.Confidence = p1.Confidence
	}
	if result.InvestigationOutcome == "" && p1.InvestigationOutcome != "" {
		result.InvestigationOutcome = p1.InvestigationOutcome
		parser.ApplyInvestigationOutcome(result, p1.InvestigationOutcome)
		// #301 defense-in-depth: Phase 1 problem_resolved overrides
		// contradictory HumanReviewNeeded set by Phase 3 (e.g. the
		// SubmitNoWorkflowResult branch hardcodes HumanReviewNeeded=true,
		// but that should not apply when the investigation is resolved).
		if p1.InvestigationOutcome == "problem_resolved" && result.HumanReviewNeeded {
			result.HumanReviewNeeded = false
			result.HumanReviewReason = ""
		}
	}
	if len(result.CausalChain) == 0 && len(p1.CausalChain) > 0 {
		result.CausalChain = p1.CausalChain
	}
	if result.DueDiligence == nil && p1.DueDiligence != nil {
		result.DueDiligence = p1.DueDiligence
	}
}

func signalToPrompt(s katypes.SignalContext) prompt.SignalData {
	return prompt.SignalData{
		Name:             s.Name,
		Namespace:        s.Namespace,
		Severity:         s.Severity,
		Message:          s.Message,
		ResourceKind:     s.ResourceKind,
		ResourceName:     s.ResourceName,
		ClusterName:      s.ClusterName,
		Environment:      s.Environment,
		Priority:         s.Priority,
		RiskTolerance:    s.RiskTolerance,
		SignalSource:     s.SignalSource,
		BusinessCategory: s.BusinessCategory,
		Description:      s.Description,
		SignalMode:       s.SignalMode,
		FiringTime:       s.FiringTime,
		ReceivedTime:     s.ReceivedTime,
		IsDuplicate:                s.IsDuplicate,
		OccurrenceCount:            s.OccurrenceCount,
		DeduplicationWindowMinutes: s.DeduplicationWindowMinutes,
		FirstSeen:                  s.FirstSeen,
		LastSeen:                   s.LastSeen,
	}
}

func toPromptEnrichment(data *enrichment.EnrichmentResult) *prompt.EnrichmentData {
	if data == nil {
		return nil
	}
	pe := &prompt.EnrichmentData{}

	for _, entry := range data.OwnerChain {
		if entry.Namespace != "" {
			pe.OwnerChain = append(pe.OwnerChain, entry.Kind+"/"+entry.Name+"("+entry.Namespace+")")
		} else {
			pe.OwnerChain = append(pe.OwnerChain, entry.Kind+"/"+entry.Name)
		}
	}

	if data.DetectedLabels != nil {
		pe.DetectedLabels = detectedLabelsToPromptMap(data.DetectedLabels)
	}

	if len(data.QuotaDetails) > 0 {
		pe.QuotaDetails = data.QuotaDetails
	}

	pe.HistoryResult = data.RemediationHistory
	return pe
}

func detectedLabelsToPromptMap(dl *enrichment.DetectedLabels) map[string]string {
	m := make(map[string]string)
	if dl.GitOpsManaged {
		m["gitOpsManaged"] = "true"
		if dl.GitOpsTool != "" {
			m["gitOpsTool"] = dl.GitOpsTool
		}
	}
	if dl.HPAEnabled {
		m["hpaEnabled"] = "true"
	}
	if dl.PDBProtected {
		m["pdbProtected"] = "true"
	}
	if dl.Stateful {
		m["stateful"] = "true"
	}
	if dl.HelmManaged {
		m["helmManaged"] = "true"
	}
	if dl.NetworkIsolated {
		m["networkIsolated"] = "true"
	}
	if dl.ServiceMesh != "" {
		m["serviceMesh"] = dl.ServiceMesh
	}
	if dl.ResourceQuotaConstrained {
		m["resourceQuotaConstrained"] = "true"
	}
	if len(dl.FailedDetections) > 0 {
		m["failedDetections"] = strings.Join(dl.FailedDetections, ",")
	}
	return m
}

func detectedLabelsToResult(dl *enrichment.DetectedLabels) map[string]interface{} {
	m := make(map[string]interface{})
	m["gitOpsManaged"] = dl.GitOpsManaged
	if dl.GitOpsTool != "" {
		m["gitOpsTool"] = dl.GitOpsTool
	}
	m["hpaEnabled"] = dl.HPAEnabled
	m["pdbProtected"] = dl.PDBProtected
	m["stateful"] = dl.Stateful
	m["helmManaged"] = dl.HelmManaged
	m["networkIsolated"] = dl.NetworkIsolated
	if dl.ServiceMesh != "" {
		m["serviceMesh"] = dl.ServiceMesh
	}
	m["resourceQuotaConstrained"] = dl.ResourceQuotaConstrained
	if len(dl.FailedDetections) > 0 {
		m["failedDetections"] = dl.FailedDetections
	}
	return m
}

func attachDetectedLabels(result *katypes.InvestigationResult, enrichData *enrichment.EnrichmentResult) {
	if result == nil || enrichData == nil || enrichData.DetectedLabels == nil {
		return
	}
	result.DetectedLabels = detectedLabelsToResult(enrichData.DetectedLabels)
}

// InjectRemediationTarget resolves the authoritative remediation target.
//
// Root owner resolution priority:
//  1. Owner chain last entry (most common: Pod → RS → Deployment)
//  2. Enrichment source identity (after re-enrichment, the chain is empty
//     because the enriched resource IS the root — see #694)
//  3. Signal identity (fallback when enrichData is nil)
//
// LLM target handling:
//   - Kind == "" or same as root: override with K8s-verified root identity
//   - Different Kind (cross-type): preserve the LLM's target
func InjectRemediationTarget(result *katypes.InvestigationResult, signal katypes.SignalContext, enrichData *enrichment.EnrichmentResult) {
	if result == nil {
		return
	}
	rootKind := signal.ResourceKind
	rootName := signal.ResourceName
	rootNS := signal.Namespace
	if enrichData != nil && len(enrichData.OwnerChain) > 0 {
		root := enrichData.OwnerChain[len(enrichData.OwnerChain)-1]
		rootKind = root.Kind
		rootName = root.Name
		rootNS = root.Namespace
	} else if enrichData != nil && enrichData.ResourceKind != "" {
		rootKind = enrichData.ResourceKind
		rootName = enrichData.ResourceName
		rootNS = enrichData.ResourceNamespace
	}

	llmKind := result.RemediationTarget.Kind
	if llmKind == "" || llmKind == rootKind {
		result.RemediationTarget = katypes.RemediationTarget{
			Kind:      rootKind,
			Name:      rootName,
			Namespace: rootNS,
		}
		return
	}

	// BR-496 v2 / BR-HAPI-261 AC#5: if the LLM's kind is a descendant
	// in the ownership hierarchy (e.g. Pod when root is Deployment),
	// resolve upward to the K8s-verified root owner. Only preserve
	// the LLM's target when its kind is genuinely cross-type (not in
	// the owner chain at all, e.g. Node vs Deployment).
	if enrichData != nil && isKindInOwnerChain(llmKind, signal.ResourceKind, enrichData.OwnerChain) {
		result.RemediationTarget = katypes.RemediationTarget{
			Kind:      rootKind,
			Name:      rootName,
			Namespace: rootNS,
		}
		return
	}
}

// isKindInOwnerChain returns true when the given kind matches the signal's own
// resource kind or any entry in the enrichment owner chain. This identifies
// descendants within the same K8s ownership hierarchy (e.g. Pod, ReplicaSet
// under a Deployment) as opposed to genuinely cross-type targets (e.g. Node).
func isKindInOwnerChain(kind, signalKind string, chain []enrichment.OwnerChainEntry) bool {
	if strings.EqualFold(kind, signalKind) {
		return true
	}
	for _, entry := range chain {
		if strings.EqualFold(entry.Kind, kind) {
			return true
		}
	}
	return false
}

// injectTargetResourceParameters merges TARGET_RESOURCE_NAME, TARGET_RESOURCE_KIND,
// and TARGET_RESOURCE_NAMESPACE into result.Parameters from the final
// RemediationTarget. KA injected these so that WorkflowExecution Jobs receive
// the correct target identity as environment variables.
func injectTargetResourceParameters(result *katypes.InvestigationResult) {
	if result == nil || result.RemediationTarget.Kind == "" {
		return
	}
	if result.Parameters == nil {
		result.Parameters = make(map[string]interface{})
	}
	result.Parameters["TARGET_RESOURCE_NAME"] = result.RemediationTarget.Name
	result.Parameters["TARGET_RESOURCE_KIND"] = result.RemediationTarget.Kind
	result.Parameters["TARGET_RESOURCE_NAMESPACE"] = result.RemediationTarget.Namespace
}

// allLabelDetectionsFailed returns true when every detection category is in
// FailedDetections, indicating the target resource could not be fetched at all.
// Used to decide whether to keep the original signal-target labels instead.
func allLabelDetectionsFailed(labels *enrichment.DetectedLabels) bool {
	if labels == nil {
		return false
	}
	return len(labels.FailedDetections) >= len(enrichment.AllDetectionCategories)
}

// EnrichmentCacheKey returns the dedup cache key for a (kind, name, namespace) tuple (#764).
func EnrichmentCacheKey(kind, name, namespace string) string {
	return kind + "/" + name + "/" + namespace
}

// resolveEnrichmentCached wraps resolveEnrichment with a per-call cache (#764).
// If the same (kind, name, namespace) was already enriched in this investigation,
// returns the cached result without making another API call.
func (inv *Investigator) resolveEnrichmentCached(ctx context.Context, cache map[string]*enrichment.EnrichmentResult, kind, name, namespace, incidentID string) *enrichment.EnrichmentResult {
	key := EnrichmentCacheKey(kind, name, namespace)
	if cached, ok := cache[key]; ok {
		inv.logger.Info("enrichment cache hit, reusing cached result",
			"kind", kind, "name", name, "namespace", namespace)
		return cached
	}
	result := inv.resolveEnrichment(ctx, kind, name, namespace, incidentID)
	cache[key] = result
	return result
}

// normalizeNamespace forces namespace="" for cluster-scoped resources (#763).
// If no ScopeResolver is configured, returns the namespace unchanged.
func (inv *Investigator) normalizeNamespace(kind, namespace string) string {
	if inv.scopeResolver == nil {
		return namespace
	}
	isCluster, err := inv.scopeResolver.IsClusterScoped(kind)
	if err != nil {
		inv.logger.Warn("ScopeResolver error, preserving namespace",
			"kind", kind, "error", err)
		return namespace
	}
	if isCluster {
		return ""
	}
	return namespace
}

// mapperScopeResolver implements ScopeResolver using a RESTMapper.
type mapperScopeResolver struct {
	mapper meta.RESTMapper
}

// NewMapperScopeResolver creates a ScopeResolver backed by a RESTMapper.
func NewMapperScopeResolver(mapper meta.RESTMapper) ScopeResolver {
	return &mapperScopeResolver{mapper: mapper}
}

func (r *mapperScopeResolver) IsClusterScoped(kind string) (bool, error) {
	plural := strings.ToLower(kind) + "s"
	gvr, err := r.mapper.ResourceFor(schema.GroupVersionResource{Resource: plural})
	if err != nil {
		return false, err
	}
	gvk, err := r.mapper.KindFor(gvr)
	if err != nil {
		return false, err
	}
	mapping, err := r.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, err
	}
	return mapping.Scope.Name() != meta.RESTScopeNameNamespace, nil
}
