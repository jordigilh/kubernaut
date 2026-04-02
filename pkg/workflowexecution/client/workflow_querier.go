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
)

// WorkflowQuerier retrieves workflow metadata from the Data Storage catalog.
// DD-WE-006: WFE queries DS on demand using the workflow ID.
type WorkflowQuerier interface {
	// GetWorkflowSchemaMetadata fetches the workflow from DS and returns all
	// schema artifacts (engine, engineConfig, dependencies, declared parameters)
	// from a single GetWorkflowByID call. F6: Consolidates 3 separate DS calls
	// into one to reduce latency and DS load during reconciliation.
	GetWorkflowSchemaMetadata(ctx context.Context, workflowID string) (*SchemaMetadata, error)
}

// WorkflowCatalogClient is a narrow interface satisfied by the ogen-generated
// *ogenclient.Client. Defined here for testability (mock injection).
type WorkflowCatalogClient interface {
	GetWorkflowByID(ctx context.Context, params ogenclient.GetWorkflowByIDParams) (ogenclient.GetWorkflowByIDRes, error)
}

// SchemaMetadata bundles all workflow catalog artifacts needed by the WE
// reconciler from a single DS GetWorkflowByID call. F6: Consolidates engine,
// engineConfig, dependencies, and declared parameter names to eliminate
// redundant DS round-trips during reconciliation.
type SchemaMetadata struct {
	// Engine is the execution engine from the DS catalog entry (e.g. "tekton", "job", "ansible").
	// Issue #518: resolved at runtime, not from the WFE spec.
	Engine string
	// WorkflowName is the human-readable workflow name from the DS catalog entry.
	WorkflowName string
	// EngineConfig is the raw JSON engine-specific configuration extracted from
	// the schema's execution.engineConfig section. nil when absent.
	// DD-WORKFLOW-017: execution details come from the workflow catalog entry.
	EngineConfig json.RawMessage
	// Dependencies are the infrastructure resources (Secrets, ConfigMaps) declared
	// in the workflow schema. DD-WE-006.
	Dependencies *models.WorkflowDependencies
	// DeclaredParameterNames is the set of parameter names declared in the
	// workflow schema. #243: defense-in-depth parameter filtering.
	//   nil   → no schema content, no filtering (backward compatible)
	//   empty → schema exists but declares no params, strip all
	DeclaredParameterNames map[string]bool
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

	transport := auth.NewServiceAccountTransportWithBase(&http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	})

	ogenClient, err := ogenclient.NewClient(baseURL, ogenclient.WithClient(&http.Client{
		Timeout:   timeout,
		Transport: transport,
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to create ogen client for workflow queries: %w", err)
	}

	return &OgenWorkflowQuerier{client: ogenClient}, nil
}

// GetWorkflowSchemaMetadata fetches the workflow from DS and returns all
// schema artifacts from a single GetWorkflowByID call. F6: Consolidates
// engine resolution (#518), engineConfig (DD-WORKFLOW-017), dependencies
// (DD-WE-006), and parameter names (#243) into one round-trip.
//
// Engine and WorkflowName are always populated from the DS response.
// Schema-derived fields (Dependencies, DeclaredParameterNames, EngineConfig)
// are only populated when the workflow has non-empty Content.
func (q *OgenWorkflowQuerier) GetWorkflowSchemaMetadata(ctx context.Context, workflowID string) (*SchemaMetadata, error) {
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

	wf, ok := res.(*ogenclient.RemediationWorkflow)
	if !ok {
		return nil, fmt.Errorf("workflow %s not found in catalog", workflowID)
	}

	meta := &SchemaMetadata{
		Engine:       wf.ExecutionEngine,
		WorkflowName: wf.WorkflowName,
	}

	if wf.Content == "" {
		return meta, nil
	}

	parser := schema.NewParser()
	parsed, err := parser.Parse(wf.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workflow schema for %s: %w", workflowID, err)
	}

	meta.Dependencies = parser.ExtractDependencies(parsed)

	if rawEC := parser.ExtractEngineConfig(parsed); rawEC != nil {
		meta.EngineConfig = *rawEC
	}

	if len(parsed.Parameters) > 0 {
		meta.DeclaredParameterNames = make(map[string]bool, len(parsed.Parameters))
		for _, p := range parsed.Parameters {
			meta.DeclaredParameterNames[p.Name] = true
		}
	}

	return meta, nil
}
