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

package authwebhook

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// DSClientAdapter wraps the ogen-generated Data Storage client to implement
// WorkflowCatalogClient. This adapter translates between the admission handler's
// domain interface and the OpenAPI-generated HTTP client.
//
// ADR-058: Used by RemediationWorkflowHandler to register/disable workflows in DS.
type DSClientAdapter struct {
	client     *ogenclient.Client
	logger     logr.Logger
	baseURL    string
	httpClient *http.Client
}

// NewDSClientAdapterFromClient wraps an existing ogen client as a DSClientAdapter.
// Use when the ogen client is shared across multiple adapters (e.g., audit + workflow).
func NewDSClientAdapterFromClient(client *ogenclient.Client, logger logr.Logger) *DSClientAdapter {
	return &DSClientAdapter{client: client, logger: logger, httpClient: http.DefaultClient}
}

// NewDSClientAdapter creates a DSClientAdapter from a Data Storage service URL.
func NewDSClientAdapter(baseURL string, timeout time.Duration, logger logr.Logger) (*DSClientAdapter, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL cannot be empty")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// Issue #750: TLS_CA_FILE honoured for inter-service HTTPS.
	// Issue #853: Wrapped with RetryTransport for transient failure resilience.
	baseTransport, err := sharedtls.DefaultBaseTransportWithRetry()
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS-aware base transport: %w", err)
	}
	transport := auth.NewAuthTransport(auth.NewDefaultTokenSource(), baseTransport)

	return newDSClientAdapterWithTransport(baseURL, timeout, transport, logger)
}

// NewDSClientAdapterWithTransport creates a DSClientAdapter with a caller-provided
// http.RoundTripper. Use in unit tests to avoid requiring a Kubernetes SA token.
func NewDSClientAdapterWithTransport(baseURL string, timeout time.Duration, transport http.RoundTripper, logger logr.Logger) (*DSClientAdapter, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL cannot be empty")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return newDSClientAdapterWithTransport(baseURL, timeout, transport, logger)
}

func newDSClientAdapterWithTransport(baseURL string, timeout time.Duration, transport http.RoundTripper, logger logr.Logger) (*DSClientAdapter, error) {
	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	client, err := ogenclient.NewClient(baseURL, ogenclient.WithClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create ogen client: %w", err)
	}

	return &DSClientAdapter{
		client:     client,
		logger:     logger,
		baseURL:    baseURL,
		httpClient: httpClient,
	}, nil
}

// NOTE: CreateWorkflowInline and DisableWorkflow were removed by #1661
// Change 8c -- AW no longer bridges RemediationWorkflow CRD lifecycle to a DS
// workflow catalog (registerWorkflow/handleDelete in
// remediationworkflow_handler.go now compute and patch everything locally).
// mapOgenWorkflowToResult (the CreateWorkflowInline response mapper) was
// removed alongside them.

// NOTE: rfc7807Error (the RFC 7807 Problem Details formatter used by the
// removed CreateActionType/CreateWorkflowInline/DisableWorkflow methods) was
// removed alongside them in #1661 Phase 55c -- DSClientAdapter no longer
// makes any mutating DS calls that would need to unpack a Problem Details
// error response.

// NOTE: ActionTypeRegistrationResult, ActionTypeDisableResult, and
// ActionTypeUpdateResult (the CreateActionType/DisableActionType/
// UpdateActionType response types) were removed by #1661 Phase 55c, once
// DS's ActionType REST endpoints were deleted (DD-WORKFLOW-018) and the
// ActionTypeCatalogClient marker interface had zero remaining implementers
// to migrate. CreateActionType, UpdateActionType, DisableActionType, and
// ForceDisableActionType were removed earlier by #1661 Change 8d -- AW no
// longer bridges ActionType CRD lifecycle to a DS action-type catalog
// (handleCreate/handleUpdate/handleDelete in actiontype_handler.go now
// compute and patch everything locally, and the K8s-native RemediationWorkflow
// list is the sole dependents gate on DELETE), mirroring CreateWorkflowInline/
// DisableWorkflow's removal above for Change 8c. GetActiveWorkflowCount was
// removed alongside them -- refreshActionTypeWorkflowCount
// (remediationworkflow_handler.go, rw_reconciler.go) now counts live
// RemediationWorkflow CRDs directly via listDependentWorkflowNames.
