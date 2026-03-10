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

// Package dsclient defines the unified Data Storage client interface, composing
// the per-concern interfaces currently scattered across pkg/audit and pkg/authwebhook.
//
// RF-3 (v1.1): This package is the target for consolidation. Today it serves as
// documentation of the intended unified interface. In v1.1, both
// pkg/audit.OpenAPIClientAdapter and pkg/authwebhook.DSClientAdapter will be
// replaced by a single adapter implementing this interface.
//
// Current adapters wrapping *ogenclient.Client:
//   - pkg/audit.OpenAPIClientAdapter       (implements AuditBatchWriter)
//   - pkg/authwebhook.DSClientAdapter      (implements WorkflowCatalog + ActionTypeCatalog + WorkflowCounter)
package dsclient

import (
	"context"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// AuditBatchWriter writes audit event batches to DS.
// Currently: pkg/audit.DataStorageClient
type AuditBatchWriter interface {
	StoreBatch(ctx context.Context, events []*ogenclient.AuditEventRequest) error
}

// WorkflowCatalog manages workflow registration and lifecycle in DS.
// Currently: pkg/authwebhook.WorkflowCatalogClient (via DSClientAdapter)
type WorkflowCatalog interface {
	CreateWorkflowInline(ctx context.Context, content, source, registeredBy string) (WorkflowResult, error)
	DisableWorkflow(ctx context.Context, workflowID, reason, updatedBy string) error
}

// ActionTypeCatalog manages action type taxonomy CRUD in DS.
// Currently: pkg/authwebhook.ActionTypeCatalogClient (via DSClientAdapter)
type ActionTypeCatalog interface {
	CreateActionType(ctx context.Context, name string, description ogenclient.ActionTypeDescription, registeredBy string) (ActionTypeResult, error)
	UpdateActionType(ctx context.Context, name string, description ogenclient.ActionTypeDescription, updatedBy string) (ActionTypeUpdateResult, error)
	DisableActionType(ctx context.Context, name string, disabledBy string) (ActionTypeDisableResult, error)
}

// WorkflowCounter queries active workflow counts for an action type.
// Currently: pkg/authwebhook.ActionTypeWorkflowCounter (via DSClientAdapter)
type WorkflowCounter interface {
	GetActiveWorkflowCount(ctx context.Context, actionType string) (int, error)
}

// Client is the unified DS client interface composing all per-concern interfaces.
// v1.1: A single adapter implementing this interface will replace the current
// two adapters (OpenAPIClientAdapter + DSClientAdapter).
type Client interface {
	AuditBatchWriter
	WorkflowCatalog
	ActionTypeCatalog
	WorkflowCounter
}

// WorkflowResult holds the DS response after registering or re-enabling a workflow.
type WorkflowResult struct {
	WorkflowID        string
	WorkflowName      string
	Version           string
	Status            string
	PreviouslyExisted bool
}

// ActionTypeResult holds the DS response after registering or re-enabling an action type.
type ActionTypeResult struct {
	ActionType   string
	Status       string
	WasReenabled bool
}

// ActionTypeUpdateResult holds the DS response after updating an action type description.
type ActionTypeUpdateResult struct {
	ActionType    string
	UpdatedFields []string
}

// ActionTypeDisableResult holds the DS response when disabling an action type.
type ActionTypeDisableResult struct {
	Disabled               bool
	DependentWorkflowCount int
	DependentWorkflows     []string
}
