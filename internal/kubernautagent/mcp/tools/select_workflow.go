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

package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

// EnrichmentRunner abstracts the enrichment call for testability.
type EnrichmentRunner interface {
	Enrich(ctx context.Context, kind, name, namespace, apiVersion, specHash, incidentID string) (*enrichment.EnrichmentResult, error)
}

// WorkflowCatalog abstracts the workflow catalog lookup for testability.
type WorkflowCatalog interface {
	GetWorkflowByID(ctx context.Context, workflowID string) (*CatalogWorkflow, error)
}

// CatalogWorkflow represents the essential fields from a DataStorage workflow
// entry needed for the interactive selection response.
type CatalogWorkflow struct {
	WorkflowID         string `json:"workflow_id"`
	WorkflowName       string `json:"workflow_name"`
	ActionType         string `json:"action_type"`
	Version            string `json:"version"`
	ExecutionEngine    string `json:"execution_engine,omitempty"`
	ExecutionBundle    string `json:"execution_bundle,omitempty"`
	ServiceAccountName string `json:"service_account_name,omitempty"`
}

// PreSelectionContext accumulates results from pre-selection pipeline hooks
// that run before catalog lookup. Each hook may populate fields consumed by
// subsequent hooks or the final output.
//
// Pipeline ordering (PROPOSAL-EXT-003 §3, five-phase model):
//
//	pre-investigation → investigation → rca-resolution → pre-workflow-selection → workflow-selection
//
// Within pre-workflow-selection, hooks execute in registration order:
//
//	[enrichment] → [Goose recipe injection] → catalog lookup
//
// Enrichment always runs first so that Goose recipe prompt injections have
// access to the full enrichment context (owner chain, labels, history).
// See PROPOSAL-EXT-003 §3.3 and PROPOSAL-EXT-002 §5.2 for the data contract.
type PreSelectionContext struct {
	Enrichment *enrichment.EnrichmentResult
}

// PreSelectionHook is a single stage in the pre-workflow-selection pipeline.
// Hooks run in registration order before catalog lookup. A hook may read and
// write fields on PreSelectionContext; errors abort the pipeline.
//
// v1.5: enrichment is the only registered hook.
// Next release: Goose recipe prompt injection hooks append after enrichment,
// receiving the populated Enrichment field as recipe parameter context.
type PreSelectionHook func(ctx context.Context, input SelectWorkflowInput, user mcpinternal.UserInfo, pctx *PreSelectionContext) error

// SelectWorkflowInput defines the input schema for the kubernaut_select_workflow MCP tool.
type SelectWorkflowInput struct {
	RRID       string `json:"rr_id"`
	WorkflowID string `json:"workflow_id"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	APIVersion string `json:"api_version,omitempty"`
	SpecHash   string `json:"spec_hash,omitempty"`
	IncidentID string `json:"incident_id,omitempty"`
}

// SelectWorkflowOutput defines the output schema for the kubernaut_select_workflow MCP tool.
type SelectWorkflowOutput struct {
	Status     string                       `json:"status"`
	Workflow   *CatalogWorkflow             `json:"workflow,omitempty"`
	Enrichment *enrichment.EnrichmentResult `json:"enrichment,omitempty"`
	Confidence float64                      `json:"confidence"`
	Rationale  string                       `json:"rationale"`
}

// SelectWorkflowOption configures optional dependencies on SelectWorkflowTool.
type SelectWorkflowOption func(*SelectWorkflowTool)

// WithEnrichmentRunner registers enrichment as the first pre-selection hook.
// Enrichment runs before any Goose recipe prompt injection hooks so that
// recipe parameters have access to the full enrichment context (#1012).
func WithEnrichmentRunner(runner EnrichmentRunner) SelectWorkflowOption {
	return func(t *SelectWorkflowTool) {
		hook := func(ctx context.Context, input SelectWorkflowInput, user mcpinternal.UserInfo, pctx *PreSelectionContext) error {
			if input.Kind == "" {
				t.logger.V(1).Info("enrichment skipped: kind not provided in select_workflow input")
				return nil
			}
			ctx = transport.WithImpersonatedUser(ctx, user.Username, user.Groups)
			result, err := runner.Enrich(ctx, input.Kind, input.Name, input.Namespace, input.APIVersion, input.SpecHash, input.IncidentID)
			if err != nil {
				if errors.Is(err, enrichment.ErrRBACForbidden) {
					return ErrCodeForbidden.WithDetail("namespace", input.Namespace)
				}
				return fmt.Errorf("enrich failed: %w", err)
			}
			pctx.Enrichment = result
			return nil
		}
		t.preSelectionHooks = append(t.preSelectionHooks, hook)
	}
}

// WithPreSelectionHook appends a hook to the pre-workflow-selection pipeline.
// Hooks run after any previously registered hooks (enrichment first by convention).
func WithPreSelectionHook(hook PreSelectionHook) SelectWorkflowOption {
	return func(t *SelectWorkflowTool) {
		t.preSelectionHooks = append(t.preSelectionHooks, hook)
	}
}

// SelectWorkflowTool handles the kubernaut_select_workflow MCP tool.
// BR-INTERACTIVE-005: enables interactive workflow selection.
// #1012: internalized enrichment via pre-selection pipeline.
type SelectWorkflowTool struct {
	catalog           WorkflowCatalog
	sessions          mcpinternal.SessionManager
	httpCompleter     HTTPSessionCompleter
	mutexProvider     SessionMutexProvider
	preSelectionHooks []PreSelectionHook
	logger            logr.Logger
}

// WithHTTPSessionCompleter enables session completion (auto-complete) for the
// select_workflow tool. When set, selecting a workflow writes the final result
// to the HTTP session store and releases the MCP lease.
func WithHTTPSessionCompleter(completer HTTPSessionCompleter) SelectWorkflowOption {
	return func(t *SelectWorkflowTool) {
		if completer != nil {
			t.httpCompleter = completer
		}
	}
}

// WithMutexProvider enables per-rrID mutex sharing for concurrency safety.
func WithMutexProvider(provider SessionMutexProvider) SelectWorkflowOption {
	return func(t *SelectWorkflowTool) {
		if provider != nil {
			t.mutexProvider = provider
		}
	}
}

// WithLogger sets the logger for the tool. Hooks use this logger to emit
// debug-level diagnostics (e.g. enrichment skipped when kind is empty).
func WithLogger(logger logr.Logger) SelectWorkflowOption {
	return func(t *SelectWorkflowTool) {
		t.logger = logger
	}
}

// NewSelectWorkflowTool creates the tool handler with its dependencies.
func NewSelectWorkflowTool(catalog WorkflowCatalog, sessions mcpinternal.SessionManager, opts ...SelectWorkflowOption) *SelectWorkflowTool {
	t := &SelectWorkflowTool{catalog: catalog, sessions: sessions, logger: logr.Discard()}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Handle executes the workflow selection after validating input and session.
//
// v1.5: requires a prior discover_workflows call (strict gating). The workflow_id
// must be one of the IDs from the discovery result. After selection, the tool
// auto-completes the session by writing the final InvestigationResult to the HTTP
// session store and releasing the MCP lease.
//
// The pre-workflow-selection pipeline runs all registered hooks in order before
// catalog lookup. In v1.5, enrichment is the only hook. Future releases add
// Goose recipe prompt injection as subsequent hooks — see PROPOSAL-EXT-003 §3.3
// (pre-workflow-selection) and PROPOSAL-EXT-002.
func (t *SelectWorkflowTool) Handle(ctx context.Context, input SelectWorkflowInput, user mcpinternal.UserInfo) (SelectWorkflowOutput, error) {
	if err := validateSelectWorkflowInput(input); err != nil {
		return SelectWorkflowOutput{}, err
	}

	if t.mutexProvider != nil {
		mu := t.mutexProvider.GetSessionMutex(input.RRID)
		mu.Lock()
		defer mu.Unlock()
	}

	if !t.sessions.IsDriverActive(input.RRID) {
		return SelectWorkflowOutput{}, fmt.Errorf("no active interactive session for rr_id")
	}

	driver, err := t.sessions.GetDriver(input.RRID)
	if err != nil || driver == nil {
		return SelectWorkflowOutput{}, fmt.Errorf("no active interactive session for rr_id")
	}

	if driver.ActingUser.Username != user.Username {
		return SelectWorkflowOutput{}, fmt.Errorf("caller is not the active driver for this session")
	}

	// v1.5 strict gating: require prior discover_workflows call.
	if driver.DiscoveryResult == nil {
		return SelectWorkflowOutput{}, fmt.Errorf("discover_workflows must be called before select_workflow")
	}

	// v1.5 strict validation: workflow_id must be from the discovery results.
	if !isWorkflowInDiscoveryResult(input.WorkflowID, driver.DiscoveryResult) {
		return SelectWorkflowOutput{}, fmt.Errorf("workflow_id %q not found in discovery results; call discover_workflows to refresh", input.WorkflowID)
	}

	pctx := &PreSelectionContext{}
	for _, hook := range t.preSelectionHooks {
		if err := hook(ctx, input, user, pctx); err != nil {
			return SelectWorkflowOutput{}, err
		}
	}

	workflow, err := t.catalog.GetWorkflowByID(ctx, input.WorkflowID)
	if err != nil {
		return SelectWorkflowOutput{}, fmt.Errorf("workflow catalog lookup failed: %w", err)
	}

	// Auto-complete: build final InvestigationResult from RCA + selected workflow,
	// write to HTTP session store, and release the MCP lease.
	if t.httpCompleter != nil && driver.RCAResult != nil {
		finalResult := buildFinalResult(driver.RCAResult, workflow, driver.DiscoveryResult)
		httpSessionID, found := t.httpCompleter.FindUserDrivingByRemediationID(input.RRID)
		if found {
			if completeErr := t.httpCompleter.CompleteUserDriving(httpSessionID, finalResult); completeErr != nil {
				t.logger.Error(completeErr, "failed to complete HTTP session",
					"rr_id", input.RRID, "http_session_id", httpSessionID)
			}
		} else if completeErr := t.httpCompleter.ForceCompleteByRemediationID(input.RRID, finalResult); completeErr != nil {
			t.logger.V(1).Info("no HTTP session found to force-complete on select_workflow",
				"rr_id", input.RRID, "error", completeErr)
		}

		if releaseErr := t.sessions.Release(driver.SessionID, "workflow_selected"); releaseErr != nil {
			if !errors.Is(releaseErr, mcpinternal.ErrSessionNotFound) {
				t.logger.Error(releaseErr, "failed to release MCP lease", "session_id", driver.SessionID)
			}
		}
	}

	return SelectWorkflowOutput{
		Status:     "workflow_selected",
		Workflow:   workflow,
		Enrichment: pctx.Enrichment,
		Confidence: 1.0,
		Rationale:  "User-selected via interactive mode",
	}, nil
}

// isWorkflowInDiscoveryResult checks if the given workflow_id is in the
// discovery result (recommended or alternatives).
func isWorkflowInDiscoveryResult(workflowID string, dr *mcpinternal.WorkflowDiscoveryResult) bool {
	if dr.Recommended != nil && dr.Recommended.WorkflowID == workflowID {
		return true
	}
	for _, alt := range dr.Alternatives {
		if alt.WorkflowID == workflowID {
			return true
		}
	}
	return false
}

// buildFinalResult constructs the final InvestigationResult by merging the RCA
// with the selected workflow details from the catalog and per-workflow parameters
// from the discovery result (#1169).
func buildFinalResult(rca *katypes.InvestigationResult, workflow *CatalogWorkflow, discovery *mcpinternal.WorkflowDiscoveryResult) *katypes.InvestigationResult {
	result := *rca
	if workflow != nil {
		result.WorkflowID = workflow.WorkflowID
		result.ExecutionEngine = workflow.ExecutionEngine
		result.ExecutionBundle = workflow.ExecutionBundle
		result.ServiceAccountName = workflow.ServiceAccountName
		result.WorkflowVersion = workflow.Version
		result.WorkflowRationale = "User-selected via interactive mode"
	}

	if discovery != nil && workflow != nil {
		if params, found := lookupDiscoveredParameters(workflow.WorkflowID, discovery); found {
			result.Parameters = cloneParameterMap(params)
		}
	}

	return &result
}

// lookupDiscoveredParameters finds the parameter map for a given workflow_id
// in the discovery result (recommended or alternatives).
func lookupDiscoveredParameters(workflowID string, dr *mcpinternal.WorkflowDiscoveryResult) (map[string]interface{}, bool) {
	if dr.Recommended != nil && dr.Recommended.WorkflowID == workflowID {
		return dr.Recommended.Parameters, true
	}
	for _, alt := range dr.Alternatives {
		if alt.WorkflowID == workflowID {
			return alt.Parameters, true
		}
	}
	return nil, false
}

// cloneParameterMap creates a shallow clone of a parameter map to prevent
// aliasing between the result and the stored discovery/RCA maps (Go mistake #78).
func cloneParameterMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func validateSelectWorkflowInput(input SelectWorkflowInput) error {
	if input.RRID == "" {
		return fmt.Errorf("rr_id is required")
	}
	if input.WorkflowID == "" {
		return fmt.Errorf("workflow_id is required")
	}
	return nil
}
