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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// dataStorageHTTPQuerier implements DataStorageQuerier via HTTP calls to DataStorage.
type dataStorageHTTPQuerier struct {
	baseURL    string
	httpClient *http.Client
}

// NewDataStorageHTTPQuerier creates a new DS querier with default timeout and
// ServiceAccount authentication (DD-AUTH-005).
func NewDataStorageHTTPQuerier(baseURL string) DataStorageQuerier {
	return NewDataStorageHTTPQuerierWithTransport(baseURL, 10*time.Second, nil)
}

// NewDataStorageHTTPQuerierWithTimeout creates a new DS querier with custom timeout
// and ServiceAccount authentication (DD-AUTH-005).
func NewDataStorageHTTPQuerierWithTimeout(baseURL string, timeout time.Duration) DataStorageQuerier {
	return NewDataStorageHTTPQuerierWithTransport(baseURL, timeout, nil)
}

// NewDataStorageHTTPQuerierWithTransport creates a DS querier with explicit transport.
// When transport is nil, ServiceAccount token auth is used automatically
// (same pattern as audit.NewOpenAPIClientAdapter -- DD-AUTH-005).
func NewDataStorageHTTPQuerierWithTransport(baseURL string, timeout time.Duration, transport http.RoundTripper) DataStorageQuerier {
	if transport == nil {
		transport = auth.NewServiceAccountTransport()
	}
	return &dataStorageHTTPQuerier{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

// auditEventsResponse matches the AuditEventsQueryResponse envelope returned by
// GET /api/v1/audit/events (see api/openapi/data-storage-v1.yaml).
// Issue #575: EM was decoding the bare array, but DS wraps events in this object.
type auditEventsResponse struct {
	Data []auditEvent `json:"data"`
}

// auditEvent carries the subset of fields EM needs from each audit event.
type auditEvent struct {
	EventType string         `json:"event_type"`
	EventData auditEventData `json:"event_data"`
}

type auditEventData struct {
	PreRemediationSpecHash string `json:"pre_remediation_spec_hash,omitempty"`
}

// QueryPreRemediationHash queries DataStorage for audit events matching the
// correlation ID and extracts the pre_remediation_spec_hash from the
// remediation.workflow_created event (DD-EM-002).
func (q *dataStorageHTTPQuerier) QueryPreRemediationHash(ctx context.Context, correlationID string) (string, error) {
	events, err := q.queryAuditEvents(ctx, correlationID, "remediation.workflow_created")
	if err != nil {
		return "", err
	}

	for _, event := range events {
		if hash := event.EventData.PreRemediationSpecHash; hash != "" {
			return hash, nil
		}
	}

	return "", nil
}

// HasWorkflowStarted checks if a workflowexecution.workflow.started event
// exists for the given correlation ID (ADR-EM-001 Section 5).
// Returns false when no such event exists, indicating the remediation
// failed before workflow execution began (e.g., AA failed, approval rejected).
func (q *dataStorageHTTPQuerier) HasWorkflowStarted(ctx context.Context, correlationID string) (bool, error) {
	events, err := q.queryAuditEvents(ctx, correlationID, "workflowexecution.workflow.started")
	if err != nil {
		return false, err
	}

	return len(events) > 0, nil
}

// queryAuditEvents calls GET /api/v1/audit/events with the given filters and
// decodes the paginated envelope response.
func (q *dataStorageHTTPQuerier) queryAuditEvents(ctx context.Context, correlationID, eventType string) ([]auditEvent, error) {
	u, err := url.Parse(q.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid DS base URL: %w", err)
	}
	u.Path = "/api/v1/audit/events"
	params := url.Values{}
	params.Set("correlation_id", correlationID)
	params.Set("event_type", eventType)
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create DS request: %w", err)
	}

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DS query failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DS returned HTTP %d for correlation_id=%s", resp.StatusCode, correlationID)
	}

	var envelope auditEventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("failed to decode DS response: %w", err)
	}

	return envelope.Data, nil
}
