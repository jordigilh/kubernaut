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

package client

import (
	"context"
	"fmt"
	"net/http"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/ogenx"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// ========================================
// DD-API-001: ogen-backed DataStorageQuerier (Issue #236)
//
// MIGRATION FROM dataStorageHTTPQuerier:
//
//   // OLD (deprecated - violates DD-API-001)
//   querier := emclient.NewDataStorageHTTPQuerierWithTimeout(url, timeout)
//
//   // NEW (DD-API-001 compliant)
//   querier, err := emclient.NewOgenDataStorageQuerier(url, timeout)
//
// Authority: DD-API-001 (OpenAPI Generated Client MANDATORY)
// ========================================

// ogenDataStorageQuerier implements DataStorageQuerier using the ogen-generated
// OpenAPI client for type-safe DS queries (DD-API-001).
type ogenDataStorageQuerier struct {
	client *ogenclient.Client
}

// NewOgenDataStorageQuerier creates a DD-API-001 compliant DataStorageQuerier.
// Production transport uses ServiceAccount token authentication (DD-AUTH-005).
func NewOgenDataStorageQuerier(baseURL string, timeout time.Duration) (DataStorageQuerier, error) {
	return NewOgenDataStorageQuerierWithTransport(baseURL, timeout, nil)
}

// NewOgenDataStorageQuerierWithTransport creates a DataStorageQuerier with custom transport.
// When transport is nil, ServiceAccount token auth is used (DD-AUTH-005).
// Integration tests use this to inject mock transports.
func NewOgenDataStorageQuerierWithTransport(baseURL string, timeout time.Duration, transport http.RoundTripper) (DataStorageQuerier, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL cannot be empty")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	if transport == nil {
		baseTransport := &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		}
		transport = auth.NewServiceAccountTransportWithBase(baseTransport)
	}

	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	client, err := ogenclient.NewClient(baseURL, ogenclient.WithClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create ogen DS client: %w", err)
	}

	return &ogenDataStorageQuerier{client: client}, nil
}

// QueryPreRemediationHash queries DataStorage for audit events matching the
// correlation ID and extracts the pre_remediation_spec_hash from the
// remediation.workflow_created event (DD-EM-002).
func (q *ogenDataStorageQuerier) QueryPreRemediationHash(ctx context.Context, correlationID string) (string, error) {
	resp, err := q.client.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
		CorrelationID: ogenclient.NewOptString(correlationID),
		EventType:     ogenclient.NewOptString("remediation.workflow_created"),
	})
	err = ogenx.ToError(resp, err)
	if err != nil {
		return "", fmt.Errorf("DS query failed for correlation_id=%s: %w", correlationID, err)
	}

	for _, event := range resp.Data {
		if payload, ok := event.EventData.GetRemediationOrchestratorAuditPayload(); ok {
			if hash, ok := payload.PreRemediationSpecHash.Get(); ok && hash != "" {
				return hash, nil
			}
		}
	}

	return "", nil
}

// HasWorkflowStarted checks if a workflowexecution.execution.started event
// exists for the given correlation ID (ADR-EM-001 Section 5).
// Returns false when no such event exists, indicating the remediation
// failed before workflow execution began (e.g., AA failed, approval rejected).
//
// Note: The WE controller emits "workflowexecution.execution.started" (Gap #6,
// BR-AUDIT-005) when a PipelineRun is created. The ADR references the higher-level
// "workflowexecution.workflow.started" but that event type is never emitted.
func (q *ogenDataStorageQuerier) HasWorkflowStarted(ctx context.Context, correlationID string) (bool, error) {
	return q.hasEvent(ctx, correlationID, "workflowexecution.execution.started")
}

// HasWorkflowCompleted checks if a workflowexecution.workflow.completed event
// exists for the given correlation ID (ADR-EM-001 Section 5).
// Used to differentiate partial vs full assessment paths (#573 G4).
//
// Note: Unlike execution.started (#575), the WE controller emits the higher-level
// "workflowexecution.workflow.completed" event type (see pkg/workflowexecution/audit/manager.go).
func (q *ogenDataStorageQuerier) HasWorkflowCompleted(ctx context.Context, correlationID string) (bool, error) {
	return q.hasEvent(ctx, correlationID, "workflowexecution.workflow.completed")
}

// hasEvent queries DS for audit events matching correlation ID and event type,
// returning true if at least one event exists.
func (q *ogenDataStorageQuerier) hasEvent(ctx context.Context, correlationID, eventType string) (bool, error) {
	resp, err := q.client.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
		CorrelationID: ogenclient.NewOptString(correlationID),
		EventType:     ogenclient.NewOptString(eventType),
	})
	err = ogenx.ToError(resp, err)
	if err != nil {
		return false, fmt.Errorf("DS query failed for correlation_id=%s: %w", correlationID, err)
	}

	return len(resp.Data) > 0, nil
}
