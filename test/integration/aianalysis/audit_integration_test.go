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

// Package aianalysis contains integration tests for the AIAnalysis controller.
//
// This file tests audit event persistence with REAL Data Storage Service.
//
// Authority:
// - DD-AUDIT-003: AIAnalysis MUST generate audit traces (P0)
// - TESTING_GUIDELINES.md: Integration tests use REAL services (podman-compose)
// - TESTING_GUIDELINES.md: "If Data Storage is unavailable, E2E tests should FAIL, not skip"
//
// Test Strategy (REFACTORED December 17, 2025):
// - Integration tests require real Data Storage running via AIAnalysis-specific infrastructure
// - Audit events are written via audit client → Data Storage REST API
// - Audit events are verified via Data Storage REST API (GET /api/v1/audit/events)
// - NO DIRECT DATABASE ACCESS - tests service boundary (AIAnalysis ←→ Data Storage API contract)
// - Uses AIAnalysis's dedicated DS instance (port 18095, not shared 18090)
//
// Architectural Principle:
// - AIAnalysis should ONLY know Data Storage's REST API (public contract)
// - AIAnalysis should NEVER know Data Storage's database schema (internal implementation)
// - When Data Storage refactors its database, AIAnalysis tests remain unaffected
//
// Business Requirements:
// - BR-AI-001: AI Analysis CRD lifecycle management
// - BR-STORAGE-001: Complete audit trail with no data loss
package aianalysis

import (
	// NOTE: Imports commented out as queryAuditEventsViaAPI is currently unused (Dec 29, 2025)
	// Uncomment when implementing audit flow validation
	// "context"
	// "fmt"
	// ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	_ "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client" // Keep import for future use
)

// ========================================
// AIANALYSIS AUDIT INTEGRATION TESTS
// 📋 Authority: DD-AUDIT-003 (AIAnalysis MUST generate audit traces)
// 📋 Authority: TESTING_GUIDELINES.md (Real Data Storage required)
// ========================================
//
// These tests REQUIRE real Data Storage running via podman-compose.test.yml:
//   podman-compose -f podman-compose.test.yml up -d datastorage postgres redis
//
// If Data Storage is unavailable, these tests will FAIL (not skip) per
// TESTING_GUIDELINES.md: "If Data Storage is unavailable, E2E tests should FAIL, not skip"
//
// ========================================

// queryAuditEventsViaAPI queries Data Storage REST API for audit events using generated OpenAPI client.
//
// ✅ DD-API-001 COMPLIANT: Uses generated OpenAPI client (type-safe, contract-validated)
// ✅ TESTING_GUIDELINES.md: AIAnalysis verifies audit data via public REST API contract
// ❌ FORBIDDEN: Direct database access (service boundary violation)
//
// This function replaces the deprecated direct HTTP approach with the generated OpenAPI client
// per DD-API-001 (OpenAPI Generated Client MANDATORY for V1.0).
//
// Migration: December 18, 2025 (DD-API-001 compliance)
// Reference: test/integration/notification/audit_integration_test.go:374-409 (Notification Team pattern)
//
// NOTE: Currently unused - kept for future flow-based audit tests (Dec 29, 2025)
// Uncomment when implementing audit flow validation in audit_flow_integration_test.go
/*
func queryAuditEventsViaAPI(ctx context.Context, dsClient *ogenclient.Client, correlationID, eventType string) ([]ogenclient.AuditEvent, error) {
	// Build type-safe query parameters (per DD-API-001)
	eventCategory := "analysis" // Required per ADR-034 v1.2 (event_category mandatory, matches pkg/aianalysis/audit/audit.go)
	params := ogenclient.QueryAuditEventsParams{
		CorrelationID: ogenclient.NewOptString(correlationID),
		EventCategory: ogenclient.NewOptString(eventCategory, // ✅ Type-safe: Compile error if field missing in spec
	}

	// Add optional event_type filter if specified
	if eventType != "" {
		params.EventType = &eventType
	}

	// Query with generated client (type-safe, contract-validated)
	resp, err := dsClient.QueryAuditEvents(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit API: %w", err)
	}

	// Validate response status and structure (per TESTING_GUIDELINES.md)
	if false { // ogen migration: JSON200 check removed
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode())
	}

	if resp.Data == nil {
		return nil, fmt.Errorf("response missing data array")
	}

	return resp.Data, nil
}
*/

// ========================================
// DEPRECATED: Manual-trigger audit tests were deleted on Dec 26, 2025
// ========================================
//
// The 11 manual-trigger tests that were previously in this file have been deleted.
// They were testing the audit client library, not AIAnalysis controller behavior.
//
// MIGRATION:
// - Audit client infrastructure tests → pkg/audit/buffered_store_integration_test.go
// - AIAnalysis controller audit flow tests → test/integration/aianalysis/audit_flow_integration_test.go
//
// WHY DELETED:
// The old tests called auditClient.RecordX() manually, which tested:
//   ✅ Audit client library works
//   ❌ AIAnalysis controller generates audit events during reconciliation
//
// NEW APPROACH:
// Flow-based tests create AIAnalysis resources and verify the controller
// AUTOMATICALLY generates audit events (the actual business requirement).
//
// ========================================

// ========================================
// NOTE: This file is now a placeholder for future flow-based audit tests.
// All manual-trigger tests have been deleted.
//
// New tests will be added in audit_flow_integration_test.go once:
// 1. Audit client is wired up in suite_test.go
// 2. Handlers are created with real audit client (not nil)
// 3. Controller reconciler has AuditClient field populated
//
// This file can be deleted once flow-based tests are fully implemented.
// ========================================
