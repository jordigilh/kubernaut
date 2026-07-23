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
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
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
	WorkflowID            string `json:"workflow_id"`
	WorkflowName          string `json:"workflow_name"`
	ActionType            string `json:"action_type"`
	Version               string `json:"version"`
	ExecutionEngine       string `json:"execution_engine,omitempty"`
	ExecutionBundle       string `json:"execution_bundle,omitempty"`
	ExecutionBundleDigest string `json:"execution_bundle_digest,omitempty"`
	ServiceAccountName    string `json:"service_account_name,omitempty"`
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
	RRID             string         `json:"rr_id"`
	WorkflowID       string         `json:"workflow_id"`
	Kind             string         `json:"kind,omitempty"`
	Name             string         `json:"name,omitempty"`
	Namespace        string         `json:"namespace,omitempty"`
	APIVersion       string         `json:"api_version,omitempty"`
	SpecHash         string         `json:"spec_hash,omitempty"`
	IncidentID       string         `json:"incident_id,omitempty"`
	Parameters       map[string]any `json:"parameters,omitempty"`
	ActingUser       string         `json:"acting_user,omitempty"`
	ActingUserGroups []string       `json:"acting_user_groups,omitempty"`
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
			result, err := runner.Enrich(ctx, input.Kind, input.Name, input.Namespace, input.APIVersion, input.SpecHash, input.IncidentID)
			if err != nil {
				if errors.Is(err, enrichment.ErrRBACForbidden) {
					return ErrCodeForbidden.WithDetail("namespace", input.Namespace)
				}
				t.logger.Error(err, "enrichment failed", "namespace", input.Namespace, "kind", input.Kind)
				return ErrCodeInternalError.WithDetail("stage", "enrichment")
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
	timeoutTracker    TimeoutTracker
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

// WithSelectWorkflowTimeoutTracker sets the inactivity-timeout tracker so
// select_workflow can cancel the session's timer on completion (#1654), the
// same way complete_no_action and investigate(cancel) already do. Without
// this, a session terminated by workflow selection keeps its inactivity
// timer running until it fires ~InactivityTimeout later against an
// already-terminal session.
func WithSelectWorkflowTimeoutTracker(tt TimeoutTracker) SelectWorkflowOption {
	return func(t *SelectWorkflowTool) {
		if tt != nil {
			t.timeoutTracker = tt
		}
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

	driver, driverErr := t.authorizeSelectionDriver(input, user)
	if driverErr != nil {
		return SelectWorkflowOutput{}, driverErr
	}

	backfillTargetFromRCA(&input, driver)

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

	// #1654: stop the inactivity timer synchronously (not deferred to the
	// completion goroutine below) — the session is terminating regardless of
	// whether an HTTP completer is wired, so the timer must not be left
	// running to fire later against an already-terminal session.
	if t.timeoutTracker != nil {
		t.timeoutTracker.StopTracking(driver.SessionID)
	}

	// Auto-complete: build final InvestigationResult from RCA + selected workflow,
	// write to HTTP session store, and release the MCP lease. Deferred to a
	// goroutine so the tool response reaches the caller before session teardown
	// closes the transport (fixes "incomplete chunked read" in kagenti).
	t.completeSelectionAsync(input, driver, workflow)

	return SelectWorkflowOutput{
		Status:     "workflow_selected",
		Workflow:   workflow,
		Enrichment: pctx.Enrichment,
		Confidence: 1.0,
		Rationale:  "User-selected via interactive mode",
	}, nil
}

// authorizeSelectionDriver verifies the requesting user is the active driver
// of an interactive session that has already completed discover_workflows
// for a workflow_id present in the discovery result (v1.5 strict gating).
func (t *SelectWorkflowTool) authorizeSelectionDriver(input SelectWorkflowInput, user mcpinternal.UserInfo) (*mcpinternal.InteractiveSession, error) {
	if !t.sessions.IsDriverActive(input.RRID) {
		return nil, ErrCodeNoSession
	}

	driver, err := t.sessions.GetDriver(input.RRID)
	if err != nil || driver == nil {
		return nil, ErrCodeNoSession
	}

	if driver.ActingUser.Username != user.Username {
		return nil, ErrCodeNotDriver
	}

	// v1.5 strict gating: require prior discover_workflows call.
	if driver.DiscoveryResult == nil {
		return nil, ErrCodeDiscoveryRequired
	}

	// v1.5 strict validation: workflow_id must be from the discovery results.
	if !isWorkflowInDiscoveryResult(input.WorkflowID, driver.DiscoveryResult) {
		return nil, ErrCodeInvalidWorkflow
	}

	return driver, nil
}

// backfillTargetFromRCA fills empty target-resource fields on input from the
// stored RCA when the caller (AF's LLM) omits them. The RCA already
// identified the target during investigation — no reason to require the
// caller to repeat it.
func backfillTargetFromRCA(input *SelectWorkflowInput, driver *mcpinternal.InteractiveSession) {
	if driver.RCAResult == nil {
		return
	}
	target := driver.RCAResult.RemediationTarget
	if input.Kind == "" {
		input.Kind = target.Kind
	}
	if input.Name == "" {
		input.Name = target.Name
	}
	if input.Namespace == "" {
		input.Namespace = target.Namespace
	}
	if input.APIVersion == "" {
		input.APIVersion = target.APIVersion
	}
}

// completeSelectionAsync builds the final InvestigationResult from the RCA +
// selected workflow, then writes it to the HTTP session store and releases
// the MCP lease in a background goroutine. Deferring to a goroutine lets the
// tool response reach the caller before session teardown closes the
// transport (fixes "incomplete chunked read" in kagenti). No-op when the
// HTTP completer or RCA result is unavailable.
func (t *SelectWorkflowTool) completeSelectionAsync(input SelectWorkflowInput, driver *mcpinternal.InteractiveSession, workflow *CatalogWorkflow) {
	if t.httpCompleter == nil || driver.RCAResult == nil {
		return
	}

	finalResult := buildFinalResult(driver.RCAResult, workflow, driver.DiscoveryResult)
	if finalResult.Parameters == nil && driver.DiscoveryResult != nil {
		t.logger.V(1).Info("no discovered parameters resolved for selected workflow",
			"rr_id", input.RRID, "workflow_id", input.WorkflowID)
	}
	sessionID := driver.SessionID
	rrID := input.RRID
	logger := t.logger
	completer := t.httpCompleter
	sessions := t.sessions
	mutexProvider := t.mutexProvider
	go func() {
		// KA-HIGH-3: Re-acquire the session mutex to prevent TOCTOU
		// between response delivery and HTTP/lease cleanup.
		if mutexProvider != nil {
			mu := mutexProvider.GetSessionMutex(rrID)
			mu.Lock()
			defer mu.Unlock()
		}
		CompleteHTTPSession(completer, rrID, finalResult, logger, "select_workflow")

		if releaseErr := sessions.Release(sessionID, "workflow_selected"); releaseErr != nil {
			if !errors.Is(releaseErr, mcpinternal.ErrSessionNotFound) {
				logger.Error(releaseErr, "failed to release MCP lease", "session_id", sessionID)
			}
		}
	}()
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

	applyDiscoveryFullResult(&result, discovery)

	// KA-HIGH-1: Use Phase 3 confidence when a discovery recommendation exists,
	// since Phase 3 has additional workflow-specific scoring context.
	if discovery != nil && discovery.Recommended != nil && discovery.Recommended.Confidence > 0 {
		result.Confidence = discovery.Recommended.Confidence
	}

	applySelectedWorkflow(&result, workflow)

	if discovery != nil && workflow != nil {
		if params, found := lookupDiscoveredParameters(workflow.WorkflowID, discovery); found {
			result.Parameters = cloneParameterMap(params)
		}
	}

	// KA-MED-2: Propagate alternative workflows from discovery so AA and the
	// operator have visibility into what other options were available.
	result.AlternativeWorkflows = discoveryAlternativeWorkflows(discovery)

	// Re-inject KA-managed target resource parameters after the discovery
	// merge, which may have replaced the RCA parameter map entirely.
	investigator.InjectTargetResourceParameters(&result)

	return &result
}

// applyDiscoveryFullResult overlays the Phase 3 (FullResult) fields onto
// result. Phase 3 is authoritative for post-RCA fields populated during
// workflow discovery: RemediationTarget (K8s-verified vs LLM-parsed),
// confidence, labels, warnings, outcome, and actionability. Overrides Phase 2
// values only when Phase 3 provides non-zero/non-nil replacements.
func applyDiscoveryFullResult(result *katypes.InvestigationResult, discovery *mcpinternal.WorkflowDiscoveryResult) {
	if discovery == nil || discovery.FullResult == nil {
		return
	}
	fr := discovery.FullResult
	if fr.RemediationTarget.Kind != "" {
		result.RemediationTarget = fr.RemediationTarget
	}
	if fr.Confidence > 0 {
		result.Confidence = fr.Confidence
	}
	if len(fr.DetectedLabels) > 0 {
		result.DetectedLabels = fr.DetectedLabels
	}
	if len(fr.Warnings) > 0 {
		result.Warnings = fr.Warnings
	}
	if fr.IsActionable != nil {
		result.IsActionable = fr.IsActionable
	}
	if fr.InvestigationOutcome != "" {
		result.InvestigationOutcome = fr.InvestigationOutcome
	}
}

// applySelectedWorkflow copies the catalog-resolved workflow identity/version
// fields onto result.
func applySelectedWorkflow(result *katypes.InvestigationResult, workflow *CatalogWorkflow) {
	if workflow == nil {
		return
	}
	result.WorkflowID = workflow.WorkflowID
	result.ExecutionEngine = workflow.ExecutionEngine
	result.ExecutionBundle = workflow.ExecutionBundle
	result.ExecutionBundleDigest = workflow.ExecutionBundleDigest
	result.ServiceAccountName = workflow.ServiceAccountName
	result.WorkflowVersion = workflow.Version
	result.WorkflowRationale = "User-selected via interactive mode"
	// Issue #1661 Change 12: catalog-authoritative, mirroring the
	// autonomous path's enrichFromCatalog -- CatalogWorkflow already
	// carries both from the DS catalog lookup one call up.
	result.ActionType = workflow.ActionType
	result.WorkflowName = workflow.WorkflowName
}

// discoveryAlternativeWorkflows converts the discovery result's alternative
// workflows into the katypes wire format, returning nil when there are none
// (preserving the original nil-vs-empty-slice semantics for callers that
// serialize this field).
func discoveryAlternativeWorkflows(discovery *mcpinternal.WorkflowDiscoveryResult) []katypes.AlternativeWorkflow {
	if discovery == nil || len(discovery.Alternatives) == 0 {
		return nil
	}
	alts := make([]katypes.AlternativeWorkflow, 0, len(discovery.Alternatives))
	for _, alt := range discovery.Alternatives {
		alts = append(alts, katypes.AlternativeWorkflow{
			WorkflowID:      alt.WorkflowID,
			ExecutionBundle: alt.ExecutionBundle,
			Confidence:      alt.Confidence,
			Rationale:       alt.Rationale,
			Parameters:      cloneParameterMap(alt.Parameters),
		})
	}
	return alts
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
		return ErrCodeInvalidInput.WithDetail("field", "rr_id")
	}
	if input.WorkflowID == "" {
		return ErrCodeInvalidInput.WithDetail("field", "workflow_id")
	}
	return nil
}
