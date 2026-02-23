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

// QueryPreRemediationHash queries DataStorage for audit events matching the
// correlation ID and extracts the pre_remediation_spec_hash from the
// remediation.workflow_created event (DD-EM-002).
func (q *dataStorageHTTPQuerier) QueryPreRemediationHash(ctx context.Context, correlationID string) (string, error) {
	// Build URL: GET /api/v1/audit/events?correlation_id=<id>&event_type=remediation.workflow_created
	u, err := url.Parse(q.baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid DS base URL: %w", err)
	}
	u.Path = "/api/v1/audit/events"
	params := url.Values{}
	params.Set("correlation_id", correlationID)
	params.Set("event_type", "remediation.workflow_created")
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create DS request: %w", err)
	}

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("DS query failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("DS returned HTTP %d for correlation_id=%s", resp.StatusCode, correlationID)
	}

	var events []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return "", fmt.Errorf("failed to decode DS response: %w", err)
	}

	// Find the first remediation.workflow_created event with a pre_remediation_spec_hash
	for _, event := range events {
		eventData, ok := event["event_data"].(map[string]interface{})
		if !ok {
			continue
		}
		if hash, ok := eventData["pre_remediation_spec_hash"].(string); ok && hash != "" {
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
	// Build URL: GET /api/v1/audit/events?correlation_id=<id>&event_type=workflowexecution.workflow.started
	u, err := url.Parse(q.baseURL)
	if err != nil {
		return false, fmt.Errorf("invalid DS base URL: %w", err)
	}
	u.Path = "/api/v1/audit/events"
	params := url.Values{}
	params.Set("correlation_id", correlationID)
	params.Set("event_type", "workflowexecution.workflow.started")
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, fmt.Errorf("failed to create DS request: %w", err)
	}

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("DS query failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("DS returned HTTP %d for correlation_id=%s", resp.StatusCode, correlationID)
	}

	var events []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return false, fmt.Errorf("failed to decode DS response: %w", err)
	}

	return len(events) > 0, nil
}
