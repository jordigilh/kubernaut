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
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// WorkflowQuerier retrieves workflow dependencies from the Data Storage catalog.
// DD-WE-006: WFE queries DS on demand using the workflow ID.
type WorkflowQuerier interface {
	GetWorkflowDependencies(ctx context.Context, workflowID string) (*models.WorkflowDependencies, error)
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

	wf, ok := res.(*ogenclient.RemediationWorkflow)
	if !ok {
		return nil, fmt.Errorf("workflow %s not found in catalog", workflowID)
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
