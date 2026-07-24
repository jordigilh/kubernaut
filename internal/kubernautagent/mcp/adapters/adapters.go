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
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
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

// RunRCAExtraction implements tools.InvestigatorRunner.
func (a *InvestigatorRunnerAdapter) RunRCAExtraction(ctx context.Context, messages []tools.LLMMessage, correlationID string) (*katypes.InvestigationResult, error) {
	llmMessages := make([]llm.Message, len(messages))
	for i, m := range messages {
		llmMessages[i] = llm.Message{Role: m.Role, Content: m.Content}
	}
	return a.inv.RunRCAExtractionFromConversation(ctx, llmMessages, correlationID)
}

// RunWorkflowDiscovery implements tools.InvestigatorRunner.
func (a *InvestigatorRunnerAdapter) RunWorkflowDiscovery(ctx context.Context, signal katypes.SignalContext, rcaResult *katypes.InvestigationResult, enrichData *prompt.EnrichmentData, correlationID string) (*katypes.InvestigationResult, error) {
	return a.inv.RunWorkflowDiscoveryFromRCA(ctx, signal, rcaResult, enrichData, correlationID)
}

// RunFullInvestigation implements tools.InvestigatorRunner.
// F4 (#1374): Delegates to the full autonomous Investigate() pipeline.
func (a *InvestigatorRunnerAdapter) RunFullInvestigation(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error) {
	return a.inv.Investigate(ctx, signal)
}

// Compile-time interface compliance check.
var _ tools.InvestigatorRunner = (*InvestigatorRunnerAdapter)(nil)

// ReconRunnerAdapter bridges the mcp.ReconRunner interface to the real
// investigator.Investigator.RunInteractiveTurn method for reconstruction.
type ReconRunnerAdapter struct {
	inv *investigator.Investigator
}

// NewReconRunnerAdapter creates an adapter for reconstruction turns.
func NewReconRunnerAdapter(inv *investigator.Investigator) *ReconRunnerAdapter {
	return &ReconRunnerAdapter{inv: inv}
}

// RunReconTurn implements mcp.ReconRunner using RunInteractiveTurn.
func (a *ReconRunnerAdapter) RunReconTurn(ctx context.Context, messages []mcpinternal.ReconMessage, correlationID string) (string, error) {
	llmMessages := make([]llm.Message, len(messages))
	for i, m := range messages {
		llmMessages[i] = llm.Message{Role: m.Role, Content: m.Content}
	}

	result, err := a.inv.RunInteractiveTurn(ctx, llmMessages, correlationID)
	if err != nil {
		return "", fmt.Errorf("reconstruction turn: %w", err)
	}

	return ExtractContent(result)
}

// WorkflowCatalogFetcher is the subset of workflowcatalog.Catalog's read API
// consumed by WorkflowCatalogAdapter. Defined here (rather than importing
// the workflowcatalog package) for testability, mirroring
// custom.WorkflowCatalog's decoupling pattern (Issue #1677 Phase 2d).
type WorkflowCatalogFetcher interface {
	GetByID(ctx context.Context, workflowID string) (*models.RemediationWorkflow, error)
}

// WorkflowCatalogAdapter bridges the MCP tools.WorkflowCatalog interface to
// KA's own cache-backed workflow catalog. Issue #1677 Phase 2e
// (DD-WORKFLOW-019): replaces the former wfclient.WorkflowQuerier/DS
// ogen-client indirection -- select_workflow/investigate_discovery now query
// the same informer-cache-backed Catalog the 3 custom MCP tools use
// (Phase 2d), instead of a separate DataStorage round-trip.
type WorkflowCatalogAdapter struct {
	catalog WorkflowCatalogFetcher
}

// NewWorkflowCatalogAdapter creates an adapter wrapping a real workflow catalog.
func NewWorkflowCatalogAdapter(catalog WorkflowCatalogFetcher) *WorkflowCatalogAdapter {
	return &WorkflowCatalogAdapter{catalog: catalog}
}

// GetWorkflowByID implements tools.WorkflowCatalog.
func (a *WorkflowCatalogAdapter) GetWorkflowByID(ctx context.Context, workflowID string) (*tools.CatalogWorkflow, error) {
	wf, err := a.catalog.GetByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow catalog lookup: %w", err)
	}

	catalogWorkflow := &tools.CatalogWorkflow{
		WorkflowID: wf.WorkflowID,
		// WorkflowName was already wired here; ActionType (Issue #1661
		// Change 12) closes the sibling gap -- both catalog-authoritative,
		// mirroring the autonomous path's enrichFromCatalog.
		WorkflowName:    wf.WorkflowName,
		ActionType:      wf.ActionType,
		Version:         wf.Version,
		ExecutionEngine: string(wf.ExecutionEngine),
	}
	if wf.ExecutionBundle != nil {
		catalogWorkflow.ExecutionBundle = *wf.ExecutionBundle
	}
	if wf.ExecutionBundleDigest != nil {
		catalogWorkflow.ExecutionBundleDigest = *wf.ExecutionBundleDigest
	}
	if wf.ServiceAccountName != nil {
		catalogWorkflow.ServiceAccountName = *wf.ServiceAccountName
	}
	return catalogWorkflow, nil
}

// Compile-time interface compliance check.
var _ tools.WorkflowCatalog = (*WorkflowCatalogAdapter)(nil)
