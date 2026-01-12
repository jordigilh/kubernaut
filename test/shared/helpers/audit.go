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

// Package shared provides shared utilities for E2E tests across all services.
//
// This package is the recommended location for E2E test helpers that are used
// by multiple services (Gateway, WorkflowExecution, SignalProcessing, etc.).
//
// Architecture Decision:
// - test/e2e/shared/ = Shared E2E helpers (cross-service utilities)
// - test/shared/ = Shared test mocks and builders (unit/integration)
// - test/e2e/[service]/ = Service-specific E2E helpers
package helpers

import (
	"context"
	"fmt"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// QueryAuditEvents queries DataStorage for audit events using the OpenAPI client.
//
// This is a shared helper for ALL E2E tests that need to verify audit event emission.
// It provides a consistent interface across Gateway, WorkflowExecution, SignalProcessing,
// RemediationOrchestrator, AIAnalysis, and Notification services.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - client: OpenAPI client (created via ogenclient.NewClient(dataStorageURL))
//   - correlationID: Optional correlation ID filter (pass nil to omit)
//   - eventType: Optional event type filter (pass nil to omit)
//   - eventCategory: Optional event category filter (pass nil to omit)
//
// Returns:
//   - []ogenclient.AuditEvent: Array of audit events matching the query
//   - int: Total count from pagination metadata (0 if not available)
//   - error: Query error or nil on success
//
// Authority:
//   - DD-API-001: OpenAPI Client Mandate (MUST use generated client, NOT raw HTTP)
//   - ADR-034 v1.2: Audit event schema and query parameters
//
// Usage Pattern:
//
//	// Create client
//	dsClient, err := ogenclient.NewClient("http://127.0.0.1:18090")
//	Expect(err).ToNot(HaveOccurred())
//
//	// Query with all filters
//	correlationID := "test-correlation-id"
//	eventType := "gateway.signal.received"
//	eventCategory := "gateway"
//	events, total, err := shared.QueryAuditEvents(ctx, dsClient, &correlationID, &eventType, &eventCategory)
//
//	// Query with only correlation ID
//	events, total, err := shared.QueryAuditEvents(ctx, dsClient, &correlationID, nil, nil)
//
//	// Query with only event type
//	events, total, err := shared.QueryAuditEvents(ctx, dsClient, nil, &eventType, nil)
//
// Used By:
//   - test/e2e/gateway/*_test.go (10+ test files)
//   - test/e2e/workflowexecution/02_observability_test.go
//   - test/e2e/signalprocessing/business_requirements_test.go
//   - test/e2e/remediationorchestrator/audit_wiring_e2e_test.go
//   - test/e2e/aianalysis/05_audit_trail_test.go
//   - test/e2e/notification/01_notification_lifecycle_audit_test.go
func QueryAuditEvents(
	ctx context.Context,
	client *ogenclient.Client,
	correlationID *string,
	eventType *string,
	eventCategory *string,
) ([]ogenclient.AuditEvent, int, error) {
	// Build query parameters
	// Per ADR-034 v1.2: All query parameters are optional
	params := ogenclient.QueryAuditEventsParams{
		Limit: ogenclient.NewOptInt(100), // Default limit to prevent unbounded queries
	}

	// Add optional filters
	if correlationID != nil {
		params.CorrelationID = ogenclient.NewOptString(*correlationID)
	}
	if eventType != nil {
		params.EventType = ogenclient.NewOptString(*eventType)
	}
	if eventCategory != nil {
		params.EventCategory = ogenclient.NewOptString(*eventCategory)
	}

	// Execute query using OpenAPI client (DD-API-001 compliance)
	resp, err := client.QueryAuditEvents(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("audit query failed: %w", err)
	}

	// Extract total count from pagination metadata
	// Per ADR-034 v1.2: Pagination is optional, use 0 if not present
	total := 0
	if resp.Pagination.Set && resp.Pagination.Value.Total.Set {
		total = resp.Pagination.Value.Total.Value
	}

	// Return typed response (ogen pattern)
	return resp.Data, total, nil
}

// QueryAuditEventsByCorrelationID is a convenience wrapper for the most common query pattern.
//
// This helper queries audit events by correlation ID only, which is the most common
// use case in E2E tests (e.g., finding all events for a specific RemediationRequest).
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - client: OpenAPI client
//   - correlationID: Correlation ID to filter by
//
// Returns:
//   - []ogenclient.AuditEvent: Array of audit events
//   - int: Total count
//   - error: Query error or nil
//
// Usage:
//
//	events, total, err := shared.QueryAuditEventsByCorrelationID(ctx, dsClient, "test-rr-123")
func QueryAuditEventsByCorrelationID(
	ctx context.Context,
	client *ogenclient.Client,
	correlationID string,
) ([]ogenclient.AuditEvent, int, error) {
	return QueryAuditEvents(ctx, client, &correlationID, nil, nil)
}

// QueryAuditEventsByType is a convenience wrapper for querying by event type.
//
// This helper is useful for finding all events of a specific type across the system,
// regardless of correlation ID or category.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - client: OpenAPI client
//   - eventType: Event type to filter by (e.g., "gateway.signal.received")
//
// Returns:
//   - []ogenclient.AuditEvent: Array of audit events
//   - int: Total count
//   - error: Query error or nil
//
// Usage:
//
//	events, total, err := shared.QueryAuditEventsByType(ctx, dsClient, "gateway.signal.received")
func QueryAuditEventsByType(
	ctx context.Context,
	client *ogenclient.Client,
	eventType string,
) ([]ogenclient.AuditEvent, int, error) {
	return QueryAuditEvents(ctx, client, nil, &eventType, nil)
}

// QueryAuditEventsByCategory is a convenience wrapper for querying by event category.
//
// This helper is useful for finding all events from a specific service category
// (e.g., all "gateway" events, all "workflowexecution" events).
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - client: OpenAPI client
//   - eventCategory: Event category to filter by (e.g., "gateway", "workflowexecution")
//
// Returns:
//   - []ogenclient.AuditEvent: Array of audit events
//   - int: Total count
//   - error: Query error or nil
//
// Usage:
//
//	events, total, err := shared.QueryAuditEventsByCategory(ctx, dsClient, "gateway")
func QueryAuditEventsByCategory(
	ctx context.Context,
	client *ogenclient.Client,
	eventCategory string,
) ([]ogenclient.AuditEvent, int, error) {
	return QueryAuditEvents(ctx, client, nil, nil, &eventCategory)
}
