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

// Package adapters bridges MCP tool interfaces to production implementations,
// breaking the import cycle between mcp and tools packages.
package adapters

import (
	"context"
	"fmt"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	wfclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"
)

// InvestigatorRunnerAdapter bridges the MCP tools.InvestigatorRunner interface
// to the real investigator.Investigator.RunInteractiveTurn method.
//
// Type conversions:
//   - tools.LLMMessage{Role, Content} → llm.Message{Role, Content}
//   - investigator.LoopResult (sealed) → string via type switch
type InvestigatorRunnerAdapter struct {
	inv *investigator.Investigator
}

// NewInvestigatorRunnerAdapter creates an adapter wrapping a real Investigator.
func NewInvestigatorRunnerAdapter(inv *investigator.Investigator) *InvestigatorRunnerAdapter {
	return &InvestigatorRunnerAdapter{inv: inv}
}

// RunInteractiveTurn implements tools.InvestigatorRunner.
func (a *InvestigatorRunnerAdapter) RunInteractiveTurn(ctx context.Context, messages []tools.LLMMessage, correlationID string) (string, error) {
	llmMessages := make([]llm.Message, len(messages))
	for i, m := range messages {
		llmMessages[i] = llm.Message{Role: m.Role, Content: m.Content}
	}

	result, err := a.inv.RunInteractiveTurn(ctx, llmMessages, correlationID)
	if err != nil {
		return "", fmt.Errorf("interactive turn: %w", err)
	}

	return ExtractContent(result)
}

// ExtractContent maps the sealed investigator.LoopResult to a plain string.
func ExtractContent(result investigator.LoopResult) (string, error) {
	switch r := result.(type) {
	case *investigator.TextResult:
		return r.Content, nil
	case *investigator.SubmitResult:
		return r.Content, nil
	case *investigator.SubmitWithWorkflowResult:
		return r.Content, nil
	case *investigator.SubmitNoWorkflowResult:
		return r.Content, nil
	case *investigator.ExhaustedResult:
		return "", fmt.Errorf("investigation exhausted: %s", r.Reason)
	case *investigator.CancelledResult:
		return "", fmt.Errorf("investigation cancelled at turn %d", r.Turn)
	default:
		return "", fmt.Errorf("unexpected loop result type: %T", result)
	}
}

// Compile-time interface compliance check.
var _ tools.InvestigatorRunner = (*InvestigatorRunnerAdapter)(nil)

// WorkflowCatalogAdapter bridges the MCP tools.WorkflowCatalog interface
// to the real wfclient.WorkflowQuerier.ResolveWorkflowCatalogMetadata method.
type WorkflowCatalogAdapter struct {
	querier wfclient.WorkflowQuerier
}

// NewWorkflowCatalogAdapter creates an adapter wrapping a real WorkflowQuerier.
func NewWorkflowCatalogAdapter(querier wfclient.WorkflowQuerier) *WorkflowCatalogAdapter {
	return &WorkflowCatalogAdapter{querier: querier}
}

// GetWorkflowByID implements tools.WorkflowCatalog.
func (a *WorkflowCatalogAdapter) GetWorkflowByID(ctx context.Context, workflowID string) (*tools.CatalogWorkflow, error) {
	meta, err := a.querier.ResolveWorkflowCatalogMetadata(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow catalog lookup: %w", err)
	}

	return &tools.CatalogWorkflow{
		WorkflowID:         workflowID,
		WorkflowName:       meta.WorkflowName,
		ExecutionEngine:    meta.ExecutionEngine,
		ExecutionBundle:    meta.ExecutionBundle,
		ServiceAccountName: meta.ServiceAccountName,
	}, nil
}

// Compile-time interface compliance check.
var _ tools.WorkflowCatalog = (*WorkflowCatalogAdapter)(nil)
