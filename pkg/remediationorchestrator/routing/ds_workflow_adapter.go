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

package routing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// WorkflowDisplayInfo holds the resolved human-readable workflow metadata
// needed for the RR printer column display.
type WorkflowDisplayInfo struct {
	WorkflowName string
	ActionType   string
}

// WorkflowCatalogClient is a narrow interface for resolving workflow metadata
// from DataStorage. Satisfied by *ogenclient.Client.
type WorkflowCatalogClient interface {
	GetWorkflowByID(
		ctx context.Context,
		params ogenclient.GetWorkflowByIDParams,
	) (ogenclient.GetWorkflowByIDRes, error)
}

// WorkflowDisplayResolver resolves a workflow UUID to human-readable display
// metadata (WorkflowName + ActionType) by querying DataStorage.
// Returns nil on any failure (graceful degradation — caller falls back to UUID).
type WorkflowDisplayResolver interface {
	ResolveWorkflowDisplay(ctx context.Context, workflowID string) *WorkflowDisplayInfo
}

// DSWorkflowAdapter adapts the ogen-generated DataStorage client to the
// WorkflowDisplayResolver interface used by the RO reconciler.
type DSWorkflowAdapter struct {
	client WorkflowCatalogClient
}

var _ WorkflowDisplayResolver = (*DSWorkflowAdapter)(nil)

// NewDSWorkflowAdapter creates a new adapter wrapping the given DS client.
func NewDSWorkflowAdapter(client WorkflowCatalogClient) *DSWorkflowAdapter {
	if client == nil {
		panic("DSWorkflowAdapter: client must not be nil")
	}
	return &DSWorkflowAdapter{client: client}
}

// NewDSWorkflowAdapterFromConfig creates a DSWorkflowAdapter with a standalone
// ogen client configured from the DataStorage URL and timeout.
// Follows the same pattern as NewDSHistoryAdapterFromConfig.
func NewDSWorkflowAdapterFromConfig(baseURL string, timeout time.Duration) (*DSWorkflowAdapter, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("DataStorage base URL cannot be empty")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	baseTransport, err := sharedtls.DefaultBaseTransport()
	if err != nil {
		return nil, fmt.Errorf("failed to create base transport: %w", err)
	}
	transport := auth.NewServiceAccountTransportWithBase(baseTransport)

	ogenClient, err := ogenclient.NewClient(baseURL, ogenclient.WithClient(&http.Client{
		Timeout:   timeout,
		Transport: transport,
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to create ogen client for DS workflow catalog: %w", err)
	}

	return &DSWorkflowAdapter{client: ogenClient}, nil
}

// ResolveWorkflowDisplay queries DataStorage for the workflow matching the
// given UUID and returns its WorkflowName + ActionType.
// Returns nil if the UUID is invalid, the workflow is not found, or DS is unreachable.
func (a *DSWorkflowAdapter) ResolveWorkflowDisplay(ctx context.Context, workflowID string) *WorkflowDisplayInfo {
	uid, err := uuid.Parse(workflowID)
	if err != nil {
		return nil
	}

	res, err := a.client.GetWorkflowByID(ctx, ogenclient.GetWorkflowByIDParams{
		WorkflowID: uid,
	})
	if err != nil {
		return nil
	}

	wf, ok := res.(*ogenclient.RemediationWorkflow)
	if !ok {
		return nil
	}

	return &WorkflowDisplayInfo{
		WorkflowName: wf.WorkflowName,
		ActionType:   wf.ActionType,
	}
}
