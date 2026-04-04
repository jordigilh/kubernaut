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
	"fmt"
	"log/slog"

	"encoding/json"

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

// Pipeline groups the optional tool-output processing stages that the
// Investigator applies inside executeTool and runWorkflowSelection.
// All fields may be nil; nil fields are skipped.
type Pipeline struct {
	Sanitizer       *sanitization.Pipeline
	AnomalyDetector *AnomalyDetector
	Validator       *parser.Validator
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
	}
}

// Investigate runs the two-invocation investigation and returns the result.
func (inv *Investigator) Investigate(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error) {
	signalKind, signalName, signalNS := ResolveEnrichmentTarget(signal, nil)
	enrichData := inv.resolveEnrichment(ctx, signalKind, signalName, signalNS, signal.IncidentID)
	promptEnrichment := toPromptEnrichment(enrichData)
	tokens := &TokenAccumulator{}

	rcaResult, err := inv.runRCA(ctx, signal, promptEnrichment, tokens)
	if err != nil {
		return nil, fmt.Errorf("RCA invocation: %w", err)
	}

	if rcaResult.HumanReviewNeeded {
		attachDetectedLabels(rcaResult, enrichData)
		completeEvent := audit.NewEvent(audit.EventTypeResponseComplete, "")
		for k, v := range tokens.AuditData() {
			completeEvent.Data[k] = v
		}
		audit.StoreBestEffort(ctx, inv.auditStore, completeEvent, inv.logger)
		return rcaResult, nil
	}

	// GAP-001 / ADR-056: Re-enrich using RCA-identified remediation target if different.
	postRCAKind, postRCAName, postRCANS := ResolveEnrichmentTarget(signal, rcaResult)
	if postRCAKind != signalKind || postRCAName != signalName || postRCANS != signalNS {
		inv.logger.Info("re-enriching with RCA remediation target",
			"signal", signalKind+"/"+signalName,
			"rca_target", postRCAKind+"/"+postRCAName,
		)
		enrichData = inv.resolveEnrichment(ctx, postRCAKind, postRCAName, postRCANS, signal.IncidentID)
		promptEnrichment = toPromptEnrichment(enrichData)
	}

	workflowResult, err := inv.runWorkflowSelection(ctx, signal, rcaResult.RCASummary, promptEnrichment, tokens)
	if err != nil {
		return nil, fmt.Errorf("workflow selection invocation: %w", err)
	}

	if workflowResult.RCASummary == "" {
		workflowResult.RCASummary = rcaResult.RCASummary
	}

	attachDetectedLabels(workflowResult, enrichData)
	completeEvent := audit.NewEvent(audit.EventTypeResponseComplete, "")
	for k, v := range tokens.AuditData() {
		completeEvent.Data[k] = v
	}
	audit.StoreBestEffort(ctx, inv.auditStore, completeEvent, inv.logger)
	return workflowResult, nil
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
	return kind, signal.Name, signal.Namespace
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

func (inv *Investigator) runRCA(ctx context.Context, signal katypes.SignalContext, enrichData *prompt.EnrichmentData, tokens *TokenAccumulator) (*katypes.InvestigationResult, error) {
	systemPrompt, err := inv.builder.RenderInvestigation(signalToPrompt(signal), enrichData)
	if err != nil {
		return nil, fmt.Errorf("rendering investigation prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Investigate: %s %s in %s — %s", signal.Severity, signal.Name, signal.Namespace, signal.Message)},
	}

	content, exhausted, err := inv.runLLMLoop(ctx, messages, katypes.PhaseRCA, tokens)
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

func (inv *Investigator) runWorkflowSelection(ctx context.Context, signal katypes.SignalContext, rcaSummary string, enrichData *prompt.EnrichmentData, tokens *TokenAccumulator) (*katypes.InvestigationResult, error) {
	systemPrompt, err := inv.builder.RenderWorkflowSelection(signalToPrompt(signal), rcaSummary, enrichData)
	if err != nil {
		return nil, fmt.Errorf("rendering workflow selection prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("RCA findings: %s\n\nSelect the appropriate remediation workflow.", rcaSummary)},
	}

	content, exhausted, err := inv.runLLMLoop(ctx, messages, katypes.PhaseWorkflowDiscovery, tokens)
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

	audit.StoreBestEffort(ctx, inv.auditStore, audit.NewEvent(audit.EventTypeValidationAttempt, ""), inv.logger)

	if inv.pipeline.Validator != nil {
		correctionFn := func(r *katypes.InvestigationResult, validationErr error) (*katypes.InvestigationResult, error) {
			correctionMsg := fmt.Sprintf("Validation failed: %s. Please select a valid workflow.", validationErr)
			messages = append(messages, llm.Message{Role: "assistant", Content: content})
			messages = append(messages, llm.Message{Role: "user", Content: correctionMsg})

			correctedContent, corrExhausted, corrErr := inv.runLLMLoop(ctx, messages, katypes.PhaseWorkflowDiscovery, tokens)
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

		corrected, corrErr := inv.pipeline.Validator.SelfCorrect(result, maxSelfCorrectionAttempts, correctionFn)
		if corrErr != nil {
			return nil, fmt.Errorf("validation self-correction failed: %w", corrErr)
		}
		return corrected, nil
	}

	return result, nil
}

// runLLMLoop executes the multi-turn LLM conversation loop with tool
// execution routed through the registry.
func (inv *Investigator) runLLMLoop(ctx context.Context, messages []llm.Message, phase katypes.Phase, tokens *TokenAccumulator) (string, bool, error) {
	toolDefs := inv.toolDefinitionsForPhase(phase)

	for turn := 0; turn < inv.maxTurns; turn++ {
		audit.StoreBestEffort(ctx, inv.auditStore, audit.NewEvent(audit.EventTypeLLMRequest, ""), inv.logger)

		resp, err := inv.client.Chat(ctx, llm.ChatRequest{
			Messages: messages,
			Tools:    toolDefs,
			Options:  llm.ChatOptions{JSONMode: true},
		})
		if err != nil {
			audit.StoreBestEffort(ctx, inv.auditStore, audit.NewEvent(audit.EventTypeResponseFailed, ""), inv.logger)
			return "", false, fmt.Errorf("%s LLM call turn %d: %w", phase, turn, err)
		}

		if tokens != nil {
			tokens.Add(resp.Usage)
		}

		respEvent := audit.NewEvent(audit.EventTypeLLMResponse, "")
		respEvent.Data["prompt_tokens"] = resp.Usage.PromptTokens
		respEvent.Data["completion_tokens"] = resp.Usage.CompletionTokens
		respEvent.Data["total_tokens"] = resp.Usage.TotalTokens
		audit.StoreBestEffort(ctx, inv.auditStore, respEvent, inv.logger)

		if len(resp.ToolCalls) > 0 {
			audit.StoreBestEffort(ctx, inv.auditStore, audit.NewEvent(audit.EventTypeLLMToolCall, ""), inv.logger)
			messages = append(messages, resp.Message)
			for _, tc := range resp.ToolCalls {
				toolResult := inv.executeTool(ctx, tc.Name, json.RawMessage(tc.Arguments))
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

func (inv *Investigator) toolDefinitionsForPhase(phase katypes.Phase) []llm.ToolDefinition {
	if inv.registry == nil {
		return nil
	}
	phaseTools := inv.registry.ToolsForPhase(phase, inv.phaseTools)
	defs := make([]llm.ToolDefinition, 0, len(phaseTools))
	for _, t := range phaseTools {
		defs = append(defs, llm.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
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
