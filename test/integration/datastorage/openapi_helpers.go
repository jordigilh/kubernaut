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

package datastorage

import (
	"context"
	"fmt"
	"time"

	dsgen ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// OPENAPI CLIENT HELPERS FOR INTEGRATION TESTS
// ========================================
// These helpers provide a clean interface for integration tests
// to use the typed OpenAPI client instead of raw HTTP + maps
//
// Benefits:
// - Type safety at compile time
// - API contract validation
// - Better IDE support and autocomplete
// - Easier refactoring when API changes
// ========================================

// createOpenAPIClient creates a configured OpenAPI client for integration tests
//
//nolint:unused // shared test helper used by multiple test files in this package
func createOpenAPIClient(baseURL string) (*dsgen.ClientWithResponses, error) {
	return dsgen.NewClientWithResponses(baseURL)
}

// createAuditEventRequest is a helper to build a typed AuditEventRequest
// This replaces the map[string]interface{} pattern used in raw HTTP tests
func createAuditEventRequest(
	eventType string,
	eventCategory string,
	eventAction string,
	eventOutcome string,
	correlationID string,
	eventData map[string]interface{},
) dsgen.AuditEventRequest {
	// Use timestamp 5 seconds in the past to avoid clock skew validation failures
	// DS service validates: timestamp.After(now.Add(5 * time.Minute))
	// See: pkg/datastorage/server/helpers/validation.go:54-55
	timestamp := time.Now().Add(-5 * time.Second).UTC()
	version := "1.0"

	// Convert string to typed enum (ADR-034 v1.2)
	category := dsgen.AuditEventRequestEventCategory(eventCategory)
	outcome := dsgen.AuditEventRequestEventOutcome(eventOutcome)

	return dsgen.AuditEventRequest{
		Version:        version,
		EventType:      eventType,
		EventTimestamp: timestamp,
		EventCategory:  category,
		EventAction:    eventAction,
		EventOutcome:   outcome,
		CorrelationId:  correlationID,
		EventData:      eventData,
	}
}

// createAuditEventWithDefaults creates an audit event with common defaults
// This is useful for tests that don't care about specific field values
func createAuditEventWithDefaults( //nolint:unused
	eventType string,
	correlationID string,
	eventData map[string]interface{},
) dsgen.AuditEventRequest {
	return createAuditEventRequest(
		eventType,
		"test",    // event_category
		"test",    // event_action
		"success", // event_outcome
		correlationID,
		eventData,
	)
}

// postAuditEvent is a helper that sends an audit event using the OpenAPI client
// Returns the event ID and any error
func postAuditEvent(
	ctx context.Context,
	client *dsgen.ClientWithResponses,
	event dsgen.AuditEventRequest,
) (string, error) {
	resp, err := client.CreateAuditEventWithResponse(ctx, event)
	if err != nil {
		return "", fmt.Errorf("failed to create audit event: %w", err)
	}

	// Check response status
	if resp.StatusCode() != 201 && resp.StatusCode() != 202 {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	// Extract event ID from response
	if resp.JSON201 != nil {
		return resp.JSON201.EventId.String(), nil
	}
	if resp.JSON202 != nil {
		return resp.JSON202.EventId.String(), nil
	}

	return "", fmt.Errorf("no event ID in response")
}

// postAuditEventBatch sends multiple audit events in a batch
// Returns the list of event IDs and any error
func postAuditEventBatch( //nolint:unused
	ctx context.Context,
	client *dsgen.ClientWithResponses,
	events []dsgen.AuditEventRequest,
) ([]string, error) {
	resp, err := client.CreateAuditEventsBatchWithResponse(ctx, events)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit events batch: %w", err)
	}

	// Check response status
	if resp.StatusCode() != 201 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	// Extract event IDs from response
	if resp.JSON201 != nil && resp.JSON201.EventIds != nil {
		eventIDs := make([]string, len(*resp.JSON201.EventIds))
		for i, id := range *resp.JSON201.EventIds {
			eventIDs[i] = id.String()
		}
		return eventIDs, nil
	}

	return nil, fmt.Errorf("no event IDs in response")
}

// ========================================
// WORKFLOW ENDPOINT HELPERS
// ========================================
// Workflow endpoints are now available in the OpenAPI client (2025-12-13)
// ========================================

// searchWorkflows performs a workflow search using the OpenAPI client
func searchWorkflows( //nolint:unused
	ctx context.Context,
	client *dsgen.ClientWithResponses,
	searchRequest dsgen.WorkflowSearchRequest,
) (*dsgen.WorkflowSearchResponse, error) {
	resp, err := client.SearchWorkflowsWithResponse(ctx, searchRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to search workflows: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("no search results in response")
	}

	return resp.JSON200, nil
}

// createWorkflow creates a workflow using the OpenAPI client
func createWorkflow( //nolint:unused
	ctx context.Context,
	client *dsgen.ClientWithResponses,
	workflow dsgen.RemediationWorkflow,
) (*dsgen.RemediationWorkflow, error) {
	resp, err := client.CreateWorkflowWithResponse(ctx, workflow)
	if err != nil {
		return nil, fmt.Errorf("failed to create workflow: %w", err)
	}

	if resp.StatusCode() != 201 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	if resp.JSON201 == nil {
		return nil, fmt.Errorf("no workflow in response")
	}

	return resp.JSON201, nil
}
