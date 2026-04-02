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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

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
}

// New creates an Investigator with the given dependencies.
func New(client llm.Client, builder *prompt.Builder, resultParser *parser.ResultParser,
	enricher *enrichment.Enricher, auditStore audit.AuditStore, logger *slog.Logger,
	maxTurns int, phaseTools katypes.PhaseToolMap) *Investigator {
	return &Investigator{
		client:       client,
		builder:      builder,
		resultParser: resultParser,
		enricher:     enricher,
		auditStore:   auditStore,
		logger:       logger,
		maxTurns:     maxTurns,
		phaseTools:   phaseTools,
	}
}

// Investigate runs the two-invocation investigation and returns the result.
func (inv *Investigator) Investigate(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error) {
	enrichData := inv.resolveEnrichment(ctx, signal)
	promptEnrichment := toPromptEnrichment(enrichData)

	rcaResult, err := inv.runRCA(ctx, signal, promptEnrichment)
	if err != nil {
		return nil, fmt.Errorf("RCA invocation: %w", err)
	}

	if rcaResult.HumanReviewNeeded {
		audit.StoreBestEffort(ctx, inv.auditStore, audit.NewEvent(audit.EventTypeResponseComplete, ""), inv.logger)
		return rcaResult, nil
	}

	workflowResult, err := inv.runWorkflowSelection(ctx, signal, rcaResult.RCASummary, promptEnrichment)
	if err != nil {
		return nil, fmt.Errorf("workflow selection invocation: %w", err)
	}

	if workflowResult.RCASummary == "" {
		workflowResult.RCASummary = rcaResult.RCASummary
	}

	audit.StoreBestEffort(ctx, inv.auditStore, audit.NewEvent(audit.EventTypeResponseComplete, ""), inv.logger)
	return workflowResult, nil
}

func (inv *Investigator) resolveEnrichment(ctx context.Context, signal katypes.SignalContext) *enrichment.EnrichmentResult {
	if inv.enricher == nil {
		return nil
	}
	kind := signal.ResourceKind
	if kind == "" {
		kind = "Pod"
	}
	result, err := inv.enricher.Enrich(ctx, kind, signal.Name, signal.Namespace)
	if err != nil {
		inv.logger.Warn("enrichment failed", slog.String("error", err.Error()))
		return nil
	}
	return result
}

func (inv *Investigator) runRCA(ctx context.Context, signal katypes.SignalContext, enrichData *prompt.EnrichmentData) (*katypes.InvestigationResult, error) {
	systemPrompt, err := inv.builder.RenderInvestigation(signalToPrompt(signal), enrichData)
	if err != nil {
		return nil, fmt.Errorf("rendering investigation prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Investigate: %s %s in %s — %s", signal.Severity, signal.Name, signal.Namespace, signal.Message)},
	}

	content, exhausted, err := inv.runLLMLoop(ctx, messages, "rca")
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

func (inv *Investigator) runWorkflowSelection(ctx context.Context, signal katypes.SignalContext, rcaSummary string, enrichData *prompt.EnrichmentData) (*katypes.InvestigationResult, error) {
	systemPrompt, err := inv.builder.RenderWorkflowSelection(signalToPrompt(signal), rcaSummary, enrichData)
	if err != nil {
		return nil, fmt.Errorf("rendering workflow selection prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("RCA findings: %s\n\nSelect the appropriate remediation workflow.", rcaSummary)},
	}

	content, exhausted, err := inv.runLLMLoop(ctx, messages, "workflow_selection")
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
	return result, nil
}

// runLLMLoop executes the multi-turn LLM conversation loop with stub tool
// execution. Real tool execution will be wired via Registry in Task 1.6.
func (inv *Investigator) runLLMLoop(ctx context.Context, messages []llm.Message, phase string) (string, bool, error) {
	for turn := 0; turn < inv.maxTurns; turn++ {
		audit.StoreBestEffort(ctx, inv.auditStore, audit.NewEvent(audit.EventTypeLLMRequest, ""), inv.logger)

		resp, err := inv.client.Chat(ctx, llm.ChatRequest{
			Messages: messages,
		})
		if err != nil {
			audit.StoreBestEffort(ctx, inv.auditStore, audit.NewEvent(audit.EventTypeResponseFailed, ""), inv.logger)
			return "", false, fmt.Errorf("%s LLM call turn %d: %w", phase, turn, err)
		}

		audit.StoreBestEffort(ctx, inv.auditStore, audit.NewEvent(audit.EventTypeLLMResponse, ""), inv.logger)

		if len(resp.ToolCalls) > 0 {
			audit.StoreBestEffort(ctx, inv.auditStore, audit.NewEvent(audit.EventTypeLLMToolCall, ""), inv.logger)
			messages = append(messages, resp.Message)
			for _, tc := range resp.ToolCalls {
				toolResult := fmt.Sprintf(`{"error":"no registry configured for tool %s"}`, tc.Name)
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    toolResult,
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
				})
			}
			continue
		}

		return resp.Message.Content, false, nil
	}

	return "", true, nil
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
	}
}

func toPromptEnrichment(data *enrichment.EnrichmentResult) *prompt.EnrichmentData {
	if data == nil {
		return nil
	}
	pe := &prompt.EnrichmentData{
		OwnerChain: data.OwnerChain,
	}

	for _, h := range data.RemediationHistory {
		pe.RemediationHistory = append(pe.RemediationHistory, prompt.RemediationHistoryEntry{
			WorkflowID: h.WorkflowID,
			Outcome:    h.Outcome,
			Timestamp:  h.Timestamp,
		})
	}
	return pe
}
