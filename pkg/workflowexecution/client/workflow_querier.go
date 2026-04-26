/*
Copyright 2025 Jordi Gil.

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

// Package client provides clients for querying external services from the WFE controller.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls" // Issue #678: Inter-service TLS
)

// WorkflowCatalogMetadata holds all workflow metadata resolved from the DS
// catalog in a single GetWorkflowByID call (Issue #650). Consolidates what was
// previously 4 separate calls into one round-trip.
type WorkflowCatalogMetadata struct {
	ExecutionEngine       string
	WorkflowName          string
	ExecutionBundle       string
	ExecutionBundleDigest string
	ServiceAccountName    string
	EngineConfig          json.RawMessage
	Dependencies          *models.WorkflowDependencies
}

// WorkflowQuerier retrieves workflow metadata from the Data Storage catalog.
// DD-WE-006: WFE queries DS on demand using the workflow ID.
// Issue #650: Consolidated into a single method to avoid redundant DS calls.
type WorkflowQuerier interface {
	GetWorkflowDependencies(ctx context.Context, workflowID string) (*models.WorkflowDependencies, error)
	// GetWorkflowEngineConfig retrieves the engine_config for a workflow from the catalog.
	// DD-WORKFLOW-017: Execution details (engine, engineConfig) come from the workflow catalog entry.
	// Returns nil when the workflow has no engineConfig.
	GetWorkflowEngineConfig(ctx context.Context, workflowID string) (json.RawMessage, error)
	// GetWorkflowExecutionEngine retrieves the execution engine and workflow name
	// from the DS catalog. Issue #518: the WE controller resolves the engine at
	// runtime rather than reading it from the WFE spec.
	GetWorkflowExecutionEngine(ctx context.Context, workflowID string) (engine string, workflowName string, err error)
	// GetWorkflowExecutionBundle retrieves the execution bundle OCI reference and
	// its digest from the DS catalog. Defense-in-depth: the WE controller resolves
	// the bundle at runtime rather than blindly trusting the WFE spec value.
	GetWorkflowExecutionBundle(ctx context.Context, workflowID string) (bundle string, digest string, err error)
	// ResolveWorkflowCatalogMetadata fetches all workflow metadata (engine, bundle,
	// SA, engineConfig, dependencies) from DS in a single GetWorkflowByID call.
	// Issue #650: Replaces the 4 individual methods above.
	ResolveWorkflowCatalogMetadata(ctx context.Context, workflowID string) (*WorkflowCatalogMetadata, error)
}

// WorkflowCatalogClient is a narrow interface satisfied by the ogen-generated
// *ogenclient.Client. Defined here for testability (mock injection).
type WorkflowCatalogClient interface {
	GetWorkflowByID(ctx context.Context, params ogenclient.GetWorkflowByIDParams) (ogenclient.GetWorkflowByIDRes, error)
}

// OgenWorkflowQuerier implements WorkflowQuerier using the ogen-generated DS client.
// DD-API-001 compliant: uses generated OpenAPI client for type safety.
type OgenWorkflowQuerier struct {
	client WorkflowCatalogClient
}

// NewOgenWorkflowQuerier creates a WorkflowQuerier from an existing ogen client wrapper.
func NewOgenWorkflowQuerier(client WorkflowCatalogClient) *OgenWorkflowQuerier {
	return &OgenWorkflowQuerier{client: client}
}

// NewOgenWorkflowQuerierFromConfig creates a WorkflowQuerier with a standalone
// ogen client configured from the DataStorage URL and timeout.
// Uses ServiceAccount auth transport (same pattern as DSHistoryAdapter).
func NewOgenWorkflowQuerierFromConfig(baseURL string, timeout time.Duration) (*OgenWorkflowQuerier, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("DataStorage base URL cannot be empty")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	// Issue #853: Wrapped with RetryTransport for transient failure resilience.
	baseTransport, err := sharedtls.DefaultBaseTransportWithRetry()
	if err != nil {
		return nil, fmt.Errorf("failed to create base transport: %w", err)
	}
	transport := auth.NewServiceAccountTransportWithBase(baseTransport)

	ogenClient, err := ogenclient.NewClient(baseURL, ogenclient.WithClient(&http.Client{
		Timeout:   timeout,
		Transport: transport,
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to create ogen client for workflow queries: %w", err)
	}

	return &OgenWorkflowQuerier{client: ogenClient}, nil
}

// classifyGetWorkflowResponse maps the polymorphic ogen response to either the
// successful workflow object or a properly classified error. This eliminates the
// misleading "not found" message that was previously returned for all non-200
// responses including HTTP 500 (Issue #658).
func classifyGetWorkflowResponse(res ogenclient.GetWorkflowByIDRes, workflowID string) (*ogenclient.RemediationWorkflow, error) {
	switch r := res.(type) {
	case *ogenclient.RemediationWorkflow:
		return r, nil
	case *ogenclient.GetWorkflowByIDNotFound:
		return nil, fmt.Errorf("workflow %s not found in catalog", workflowID)
	case *ogenclient.GetWorkflowByIDInternalServerError:
		return nil, fmt.Errorf("DS internal server error for workflow %s", workflowID)
	default:
		return nil, fmt.Errorf("unexpected DS response type %T for workflow %s", res, workflowID)
	}
}

// GetWorkflowDependencies fetches the workflow from DS by ID and extracts
// schema-declared dependencies from the Content field (raw YAML).
// Returns nil if the workflow has no dependencies declared.
func (q *OgenWorkflowQuerier) GetWorkflowDependencies(ctx context.Context, workflowID string) (*models.WorkflowDependencies, error) {
	uid, err := uuid.Parse(workflowID)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow ID %q: %w", workflowID, err)
	}

	res, err := q.client.GetWorkflowByID(ctx, ogenclient.GetWorkflowByIDParams{
		WorkflowID: uid,
	})
	if err != nil {
		return nil, fmt.Errorf("DS query failed for workflow %s: %w", workflowID, err)
	}

	wf, err := classifyGetWorkflowResponse(res, workflowID)
	if err != nil {
		return nil, err
	}

	if wf.Content == "" {
		return nil, nil
	}

	parser := schema.NewParser()
	parsed, err := parser.Parse(wf.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workflow schema for %s: %w", workflowID, err)
	}

	return parser.ExtractDependencies(parsed), nil
}

// GetWorkflowEngineConfig retrieves the engine_config from the DS catalog.
// DD-WORKFLOW-017: The WE controller resolves execution details from the catalog.
// Returns nil when the workflow has no engineConfig section.
func (q *OgenWorkflowQuerier) GetWorkflowEngineConfig(ctx context.Context, workflowID string) (json.RawMessage, error) {
	uid, err := uuid.Parse(workflowID)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow ID %q: %w", workflowID, err)
	}

	res, err := q.client.GetWorkflowByID(ctx, ogenclient.GetWorkflowByIDParams{
		WorkflowID: uid,
	})
	if err != nil {
		return nil, fmt.Errorf("DS query failed for workflow %s: %w", workflowID, err)
	}

	wf, err := classifyGetWorkflowResponse(res, workflowID)
	if err != nil {
		return nil, err
	}

	if wf.Content == "" {
		return nil, nil
	}

	parser := schema.NewParser()
	parsed, err := parser.Parse(wf.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workflow schema for %s: %w", workflowID, err)
	}

	rawMsg := parser.ExtractEngineConfig(parsed)
	if rawMsg == nil {
		return nil, nil
	}
	return *rawMsg, nil
}

// GetWorkflowExecutionEngine retrieves the execution engine and workflow name
// from the DS catalog entry. Issue #518: the WE controller resolves the engine
// at runtime rather than reading it from the (now-removed) WFE spec field.
func (q *OgenWorkflowQuerier) GetWorkflowExecutionEngine(ctx context.Context, workflowID string) (string, string, error) {
	uid, err := uuid.Parse(workflowID)
	if err != nil {
		return "", "", fmt.Errorf("invalid workflow ID %q: %w", workflowID, err)
	}

	res, err := q.client.GetWorkflowByID(ctx, ogenclient.GetWorkflowByIDParams{
		WorkflowID: uid,
	})
	if err != nil {
		return "", "", fmt.Errorf("DS query failed for workflow %s: %w", workflowID, err)
	}

	wf, err := classifyGetWorkflowResponse(res, workflowID)
	if err != nil {
		return "", "", err
	}

	return wf.ExecutionEngine, wf.WorkflowName, nil
}

// GetWorkflowExecutionBundle retrieves the execution bundle OCI reference and
// its digest from the DS catalog. Returns empty strings when the catalog entry
// does not define an execution bundle (caller preserves the existing spec value).
func (q *OgenWorkflowQuerier) GetWorkflowExecutionBundle(ctx context.Context, workflowID string) (string, string, error) {
	uid, err := uuid.Parse(workflowID)
	if err != nil {
		return "", "", fmt.Errorf("invalid workflow ID %q: %w", workflowID, err)
	}

	res, err := q.client.GetWorkflowByID(ctx, ogenclient.GetWorkflowByIDParams{
		WorkflowID: uid,
	})
	if err != nil {
		return "", "", fmt.Errorf("DS query failed for workflow %s: %w", workflowID, err)
	}

	wf, err := classifyGetWorkflowResponse(res, workflowID)
	if err != nil {
		return "", "", err
	}

	var bundle, digest string
	if wf.ExecutionBundle.IsSet() {
		bundle = wf.ExecutionBundle.Value
	}
	if wf.ExecutionBundleDigest.IsSet() {
		digest = wf.ExecutionBundleDigest.Value
	}
	return bundle, digest, nil
}

// ResolveWorkflowCatalogMetadata fetches all workflow metadata from DS in one
// GetWorkflowByID call (Issue #650).
func (q *OgenWorkflowQuerier) ResolveWorkflowCatalogMetadata(ctx context.Context, workflowID string) (*WorkflowCatalogMetadata, error) {
	uid, err := uuid.Parse(workflowID)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow ID %q: %w", workflowID, err)
	}

	res, err := q.client.GetWorkflowByID(ctx, ogenclient.GetWorkflowByIDParams{
		WorkflowID: uid,
	})
	if err != nil {
		return nil, fmt.Errorf("DS query failed for workflow %s: %w", workflowID, err)
	}

	wf, err := classifyGetWorkflowResponse(res, workflowID)
	if err != nil {
		return nil, err
	}

	meta := &WorkflowCatalogMetadata{
		ExecutionEngine: wf.ExecutionEngine,
		WorkflowName:    wf.WorkflowName,
	}
	if wf.ExecutionBundle.IsSet() {
		meta.ExecutionBundle = wf.ExecutionBundle.Value
	}
	if wf.ExecutionBundleDigest.IsSet() {
		meta.ExecutionBundleDigest = wf.ExecutionBundleDigest.Value
	}
	if wf.ServiceAccountName.IsSet() {
		meta.ServiceAccountName = wf.ServiceAccountName.Value
	}

	if wf.Content != "" {
		parser := schema.NewParser()
		parsed, err := parser.Parse(wf.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to parse workflow schema for %s: %w", workflowID, err)
		}
		meta.Dependencies = parser.ExtractDependencies(parsed)
		rawMsg := parser.ExtractEngineConfig(parsed)
		if rawMsg != nil {
			meta.EngineConfig = *rawMsg
		}
	}

	return meta, nil
}
