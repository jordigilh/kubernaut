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

package enrichment

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// DataStorageResolver resolves workflow names by calling the DataStorage
// catalog API (GET /api/v1/workflows/{workflow_id}).
type DataStorageResolver struct {
	client api.Invoker
	logger logr.Logger
}

// NewDataStorageResolver creates a resolver backed by the DataStorage ogen client.
func NewDataStorageResolver(client api.Invoker, logger logr.Logger) *DataStorageResolver {
	return &DataStorageResolver{client: client, logger: logger}
}

// ResolveWorkflowName looks up the human-readable workflow name from the
// DataStorage catalog. Returns ("", nil) on not-found for graceful degradation.
func (r *DataStorageResolver) ResolveWorkflowName(ctx context.Context, workflowID string) (string, error) {
	uid, err := uuid.Parse(workflowID)
	if err != nil {
		return "", fmt.Errorf("invalid workflow UUID %q: %w", workflowID, err)
	}

	res, err := r.client.GetWorkflowByID(ctx, api.GetWorkflowByIDParams{WorkflowID: uid})
	if err != nil {
		return "", fmt.Errorf("DataStorage lookup failed for workflow %s: %w", workflowID, err)
	}

	switch v := res.(type) {
	case *api.RemediationWorkflow:
		return v.WorkflowName, nil
	default:
		return "", nil
	}
}
