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
	"time"

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
	Sanitizer       *sanitization.Pipeline
	AnomalyDetector *AnomalyDetector
	CatalogFetcher  CatalogFetcher
	Summarizer      *summarizer.Summarizer
}

// Config holds all dependencies for constructing an Investigator.
// Using a struct instead of positional parameters makes the constructor
// stable and self-documenting. Optional fields (Registry, Pipeline)
// default to their zero values when omitted.
type Config struct {
	Client       llm.Client
	Builder      *prompt.Builder
	ResultParser *parser.ResultParser
	Enricher     *enrichment.Enricher
	AuditStore   audit.AuditStore
	Logger       *slog.Logger
	MaxTurns     int
	PhaseTools   katypes.PhaseToolMap
	Registry     *registry.Registry
	Pipeline     Pipeline
	ModelName    string
}

// Investigator orchestrates the two-invocation architecture:
// Invocation 1 (RCA): system prompt + tool calls -> RCA summary
// Invocation 2 (Workflow Selection): new session with RCA context -> workflow choice
type Investigator struct {
	client       llm.Client
	builder      *prompt.Builder
	resultParser *parser.ResultParser
	enricher     *enrichment.Enricher
	auditStore   audit.AuditStore
	logger       *slog.Logger
	maxTurns     int
	phaseTools   katypes.PhaseToolMap
	registry     *registry.Registry
	pipeline     Pipeline
	modelName    string
}

// New creates an Investigator from the given configuration.
// Config.Registry may be nil (tool execution will be skipped).
// Config.Pipeline fields default to nil (their features are skipped).
func New(cfg Config) *Investigator {
	return &Investigator{
		client:       cfg.Client,
		builder:      cfg.Builder,
		resultParser: cfg.ResultParser,
		enricher:     cfg.Enricher,
		auditStore:   cfg.AuditStore,
		logger:       cfg.Logger,
		maxTurns:     cfg.MaxTurns,
		phaseTools:   cfg.PhaseTools,
		registry:     cfg.Registry,
		pipeline:     cfg.Pipeline,
		modelName:    cfg.ModelName,
	}
}

// Investigate runs the two-invocation investigation and returns the result.
// Per BR-AUDIT-005, all audit events use signal.RemediationID as correlation ID
// so that DataStorage queries by remediation_id return the full investigation trail.
func (inv *Investigator) Investigate(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error) {
	correlationID := signal.RemediationID
	signalKind, signalName, signalNS := ResolveEnrichmentTarget(signal, nil)
	enrichData := inv.resolveEnrichment(ctx, signalKind, signalName, signalNS, signal.IncidentID)
	promptEnrichment := toPromptEnrichment(enrichData)
	tokens := &TokenAccumulator{}

	rcaResult, err := inv.runRCA(ctx, signal, promptEnrichment, tokens, correlationID)
	if err != nil {
		return nil, fmt.Errorf("RCA invocation: %w", err)
	}

	if rcaResult.HumanReviewNeeded {
		backfillSeverity(rcaResult, signal)
		attachDetectedLabels(rcaResult, enrichData)
		injectRemediationTarget(rcaResult, signal, enrichData)
		injectTargetResourceParameters(rcaResult)
		inv.emitResponseComplete(ctx, rcaResult, tokens, correlationID)
		return rcaResult, nil
	}

	// GAP-001 / ADR-056: Re-enrich using RCA-identified remediation target if different.
	// H3-fix: retain pre-RCA enrichment if re-enrichment fails.
	postRCAKind, postRCAName, postRCANS := ResolveEnrichmentTarget(signal, rcaResult)
	if postRCAKind != signalKind || postRCAName != signalName || postRCANS != signalNS {
		inv.logger.Info("re-enriching with RCA remediation target",
			"signal", signalKind+"/"+signalName,
			"rca_target", postRCAKind+"/"+postRCAName,
		)
		reEnriched := inv.resolveEnrichment(ctx, postRCAKind, postRCAName, postRCANS, signal.IncidentID)
		if reEnriched != nil && !allLabelDetectionsFailed(reEnriched.DetectedLabels) {
			enrichData = reEnriched
		} else if reEnriched != nil {
			inv.logger.Warn("re-enrichment labels all failed (RCA target not found), preserving signal-target labels",
				"rca_target", postRCAKind+"/"+postRCAName)
		} else {
			inv.logger.Warn("re-enrichment returned nil, retaining pre-RCA enrichment data")
		}
		promptEnrichment = toPromptEnrichment(enrichData)
	}

	workflowResult, err := inv.runWorkflowSelection(ctx, signal, rcaResult.RCASummary, promptEnrichment, tokens, correlationID)
	if err != nil {
		return nil, fmt.Errorf("workflow selection invocation: %w", err)
	}

	if workflowResult.RCASummary == "" {
		workflowResult.RCASummary = rcaResult.RCASummary
	}

	backfillSeverity(workflowResult, signal)
	attachDetectedLabels(workflowResult, enrichData)
	injectRemediationTarget(workflowResult, signal, enrichData)
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

func (inv *Investigator) runRCA(ctx context.Context, signal katypes.SignalContext, enrichData *prompt.EnrichmentData, tokens *TokenAccumulator, correlationID string) (*katypes.InvestigationResult, error) {
	systemPrompt, err := inv.builder.RenderInvestigation(signalToPrompt(signal), enrichData)
	if err != nil {
		return nil, fmt.Errorf("rendering investigation prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Investigate: %s %s in %s — %s", signal.Severity, signal.Name, signal.Namespace, signal.Message)},
	}

	content, exhausted, err := inv.runLLMLoop(ctx, messages, katypes.PhaseRCA, tokens, correlationID)
	if err != nil {
		return nil, err
	}
	if exhausted {
		return &katypes.InvestigationResult{
			HumanReviewNeeded: true,
			Reason:            fmt.Sprintf("max turns (%d) exhausted during RCA", inv.maxTurns),
		}, nil
	}

	result, parseErr := inv.resultParser.Parse(content)
	if parseErr != nil {
		inv.logger.Warn("RCA parse failed, treating as summary",
			slog.String("error", parseErr.Error()))
		return &katypes.InvestigationResult{
			RCASummary: content,
		}, nil
	}
	return result, nil
}

func (inv *Investigator) runWorkflowSelection(ctx context.Context, signal katypes.SignalContext, rcaSummary string, enrichData *prompt.EnrichmentData, tokens *TokenAccumulator, correlationID string) (*katypes.InvestigationResult, error) {
	systemPrompt, err := inv.builder.RenderWorkflowSelection(signalToPrompt(signal), rcaSummary, enrichData)
	if err != nil {
		return nil, fmt.Errorf("rendering workflow selection prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("RCA findings: %s\n\nSelect the appropriate remediation workflow.", rcaSummary)},
	}

	content, exhausted, err := inv.runLLMLoop(ctx, messages, katypes.PhaseWorkflowDiscovery, tokens, correlationID)
	if err != nil {
		return nil, err
	}
	if exhausted {
		return &katypes.InvestigationResult{
			RCASummary:        rcaSummary,
			HumanReviewNeeded: true,
			Reason:            fmt.Sprintf("max turns (%d) exhausted during workflow selection", inv.maxTurns),
		}, nil
	}

	result, parseErr := inv.resultParser.Parse(content)
	if parseErr != nil {
		inv.logger.Warn("workflow selection parse failed",
			slog.String("error", parseErr.Error()))
		return &katypes.InvestigationResult{
			RCASummary:        rcaSummary,
			HumanReviewNeeded: true,
			Reason:            "failed to parse workflow selection response",
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

			correctedContent, corrExhausted, corrErr := inv.runLLMLoop(ctx, messages, katypes.PhaseWorkflowDiscovery, tokens, correlationID)
			if corrErr != nil {
				return nil, corrErr
			}
			if corrExhausted {
				r.HumanReviewNeeded = true
				r.Reason = "self-correction exhausted LLM turns"
				return r, nil
			}
			content = correctedContent
			return inv.resultParser.Parse(correctedContent)
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
func (inv *Investigator) runLLMLoop(ctx context.Context, messages []llm.Message, phase katypes.Phase, tokens *TokenAccumulator, correlationID string) (string, bool, error) {
	toolDefs := inv.toolDefinitionsForPhase(phase)
	loopStart := time.Now()

	for turn := 0; turn < inv.maxTurns; turn++ {
		reqEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
		reqEvent.EventAction = audit.ActionLLMRequest
		reqEvent.EventOutcome = audit.OutcomeSuccess
		reqEvent.Data["model"] = inv.modelName
		reqEvent.Data["prompt_length"] = totalPromptLength(messages)
		reqEvent.Data["prompt_preview"] = lastUserMessage(messages, 500)
		reqEvent.Data["toolsets_enabled"] = toolNames(toolDefs)
		audit.StoreBestEffort(ctx, inv.auditStore, reqEvent, inv.logger)

		resp, err := inv.client.Chat(ctx, llm.ChatRequest{
			Messages: messages,
			Tools:    toolDefs,
			Options:  llm.ChatOptions{JSONMode: true},
		})
		if err != nil {
			failEvent := audit.NewEvent(audit.EventTypeResponseFailed, correlationID)
			failEvent.EventAction = audit.ActionResponseFailed
			failEvent.EventOutcome = audit.OutcomeFailure
			failEvent.Data["error_message"] = err.Error()
			failEvent.Data["phase"] = string(phase)
			failEvent.Data["duration_seconds"] = time.Since(loopStart).Seconds()
			audit.StoreBestEffort(ctx, inv.auditStore, failEvent, inv.logger)
			return "", false, fmt.Errorf("%s LLM call turn %d: %w", phase, turn, err)
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
		audit.StoreBestEffort(ctx, inv.auditStore, respEvent, inv.logger)

		if len(resp.ToolCalls) > 0 {
			for _, tc := range resp.ToolCalls {
				if tc.Name == SubmitResultToolName {
					inv.logger.Info("submit_result sentinel detected", slog.String("phase", string(phase)))
					return tc.Arguments, false, nil
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
			if inv.pipeline.AnomalyDetector != nil && inv.pipeline.AnomalyDetector.TotalExceeded() {
				return "", true, nil
			}
			continue
		}

		return resp.Message.Content, false, nil
	}

	return "", true, nil
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
		defs = make([]llm.ToolDefinition, 0, len(phaseTools)+1)
		for _, t := range phaseTools {
			defs = append(defs, llm.ToolDefinition{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			})
		}
	}

	defs = append(defs, llm.ToolDefinition{
		Name:        SubmitResultToolName,
		Description: "Submit the final investigation result as structured JSON. Call this tool when your analysis is complete.",
		Parameters:  parser.InvestigationResultSchema(),
	})
	return defs
}

func (inv *Investigator) executeTool(ctx context.Context, name string, args json.RawMessage) string {
	if inv.registry == nil {
		return toolErrorJSON("no registry configured for tool " + name)
	}

	if inv.pipeline.AnomalyDetector != nil {
		if ar := inv.pipeline.AnomalyDetector.CheckToolCall(name, args); !ar.Allowed {
			inv.logger.Warn("anomaly detector rejected tool call",
				slog.String("tool", name),
				slog.String("reason", ar.Reason),
			)
			return toolErrorJSON(ar.Reason)
		}
	}

	result, err := inv.registry.Execute(ctx, name, args)
	if err != nil {
		inv.logger.Warn("tool execution failed",
			slog.String("tool", name),
			slog.String("error", err.Error()),
		)
		if inv.pipeline.AnomalyDetector != nil {
			if ar := inv.pipeline.AnomalyDetector.RecordFailure(name, args); !ar.Allowed {
				return toolErrorJSON(ar.Reason)
			}
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

	return result
}

func (inv *Investigator) emitResponseComplete(ctx context.Context, result *katypes.InvestigationResult, tokens *TokenAccumulator, correlationID string) {
	completeEvent := audit.NewEvent(audit.EventTypeResponseComplete, correlationID)
	completeEvent.EventAction = audit.ActionResponseSent
	completeEvent.EventOutcome = audit.OutcomeSuccess
	for k, v := range tokens.AuditData() {
		completeEvent.Data[k] = v
	}
	if b, err := json.Marshal(resultToAuditJSON(result)); err == nil {
		completeEvent.Data["response_data"] = string(b)
	}
	audit.StoreBestEffort(ctx, inv.auditStore, completeEvent, inv.logger)
}

func resultToAuditJSON(r *katypes.InvestigationResult) map[string]interface{} {
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
		IsDuplicate:      s.IsDuplicate,
		OccurrenceCount:  s.OccurrenceCount,
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

// injectRemediationTarget replicates KA's _inject_target_resource behavior.
//
// Logic:
//   - If the LLM did not provide a remediation_target (Kind==""), inject the
//     K8s root owner from the owner chain.
//   - If the LLM provided a target with the SAME Kind as the root owner
//     (e.g., both Deployment), override with the root owner. K8s identity is
//     authoritative for same-kind resources — the LLM may return a name that
//     doesn't exist in the cluster.
//   - If the LLM provided a target with a DIFFERENT Kind (e.g., Certificate
//     when the root owner is Deployment), preserve the LLM's target. The LLM
//     identified a cross-type resource that owner-chain resolution can't reach.
func injectRemediationTarget(result *katypes.InvestigationResult, signal katypes.SignalContext, enrichData *enrichment.EnrichmentResult) {
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
		if root.Namespace != "" {
			rootNS = root.Namespace
		}
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
	// LLM identified a different Kind (cross-type target) — preserve it.
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
