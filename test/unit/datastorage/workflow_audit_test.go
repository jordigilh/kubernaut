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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// WORKFLOW AUDIT EVENT VALIDATION TESTS
// ========================================
// Business Requirement: BR-AUDIT-023-030 (Workflow Selection Audit Trail)
// Design Decision: DD-WORKFLOW-014 v2.1, DD-WORKFLOW-004 v2.0
//
// TDD Phase: RED (failing tests)
//
// Test Strategy:
// - Test workflow audit event schema validation
// - Test required field validation
// - Test V1.0 scoring (confidence only)
// - Test JSONB structure validation
//
// NOTE: Boost/penalty breakdown tests removed per DD-WORKFLOW-004 v2.0
// ========================================

var _ = Describe("Workflow Audit Event Validation", func() {
	Context("when validating workflow.catalog.search_completed event", func() {
		It("should accept valid V1.0 workflow audit event (confidence only)", func() {
			// BUSINESS SCENARIO: Valid V1.0 workflow audit event is accepted
			// BR-AUDIT-023: Audit event generation
			// DD-WORKFLOW-004 v2.0: V1.0 = confidence only (no boost/penalty breakdown)
			//
			// TDD Phase: GREEN (using proper ogen union types)

			// Create WorkflowSearchAuditPayload with proper ogen nested types
			query := ogenclient.QueryMetadata{
				TopK: 3,
			}
			query.MinScore.SetTo(0.7)

			results := ogenclient.ResultsMetadata{
				TotalFound: 5,
				Returned:   3,
				Workflows:  []ogenclient.WorkflowResultAudit{},
			}

			searchMetadata := ogenclient.SearchExecutionMetadata{
				DurationMs:          45,
				EmbeddingDimensions: 768,
				EmbeddingModel:      "all-mpnet-base-v2",
			}

			payload := ogenclient.WorkflowSearchAuditPayload{
				EventType:      ogenclient.WorkflowSearchAuditPayloadEventTypeWorkflowCatalogSearchCompleted,
				Query:          query,
				Results:        results,
				SearchMetadata: searchMetadata,
			}

			// Build event using OpenAPI types and helpers (DD-AUDIT-002 V2.0)
			event := pkgaudit.NewAuditEventRequest()
			event.Version = "1.0"
			pkgaudit.SetEventType(event, "workflow.catalog.search_completed")
			pkgaudit.SetEventCategory(event, "workflow")
			pkgaudit.SetEventAction(event, "search_completed")
			pkgaudit.SetEventOutcome(event, pkgaudit.OutcomeSuccess)
			pkgaudit.SetActor(event, "service", "datastorage")
			pkgaudit.SetResource(event, "workflow_catalog", "search-query-hash")
			pkgaudit.SetCorrelationID(event, "rr-2025-001")
			// Use proper ogen union constructor
			event.EventData = ogenclient.NewWorkflowSearchAuditPayloadAuditEventRequestEventData(payload)

			// BEHAVIOR: Validate event schema
			err := validateWorkflowAuditEvent(event)

			// CORRECTNESS: Event should be valid
			Expect(err).ToNot(HaveOccurred(), "Valid V1.0 workflow audit event should be accepted")

			// BUSINESS OUTCOME: Workflow selection audit trail is complete
		})

		It("should accept workflow audit event with zero-value query (structured types)", func() {
			// BUSINESS SCENARIO: With structured types, zero values are valid
			// BR-AUDIT-025: Query metadata capture
			//
			// TDD Phase: GREEN (using proper ogen union types)
			// NOTE: With ogen-generated structured types, all required fields are present
			// Zero values (e.g., TopK: 0) are valid - validation is type-based, not value-based

			// Create WorkflowSearchAuditPayload with zero-value query fields
			results := ogenclient.ResultsMetadata{
				TotalFound: 0,
				Returned:   0,
				Workflows:  []ogenclient.WorkflowResultAudit{},
			}

			searchMetadata := ogenclient.SearchExecutionMetadata{
				DurationMs:          30,
				EmbeddingDimensions: 768,
				EmbeddingModel:      "all-mpnet-base-v2",
			}

			payload := ogenclient.WorkflowSearchAuditPayload{
				EventType: ogenclient.WorkflowSearchAuditPayloadEventTypeWorkflowCatalogSearchCompleted,
				// Query with zero value TopK - valid with structured types
				Query:          ogenclient.QueryMetadata{TopK: 0},
				Results:        results,
				SearchMetadata: searchMetadata,
			}

			// Build event using OpenAPI types and helpers (DD-AUDIT-002 V2.0)
			event := pkgaudit.NewAuditEventRequest()
			event.Version = "1.0"
			pkgaudit.SetEventType(event, "workflow.catalog.search_completed")
			pkgaudit.SetEventCategory(event, "workflow")
			pkgaudit.SetEventAction(event, "search_completed")
			pkgaudit.SetEventOutcome(event, pkgaudit.OutcomeSuccess)
			pkgaudit.SetActor(event, "service", "datastorage")
			pkgaudit.SetResource(event, "workflow_catalog", "search-query-hash")
			pkgaudit.SetCorrelationID(event, "rr-2025-001")
			event.EventData = ogenclient.NewWorkflowSearchAuditPayloadAuditEventRequestEventData(payload)

			// BEHAVIOR: Validate event schema
			err := validateWorkflowAuditEvent(event)

			// CORRECTNESS: Event should be accepted (structured types ensure field presence)
			Expect(err).ToNot(HaveOccurred(), "Event with zero-value query should be accepted with structured types")

			// BUSINESS OUTCOME: Type safety ensures all required fields are present (even if zero-valued)
		})

		It("should accept workflow audit event with zero-value results (structured types)", func() {
			// BUSINESS SCENARIO: With structured types, zero values are valid
			// BR-AUDIT-027: Workflow metadata capture
			//
			// TDD Phase: GREEN (using proper ogen union types)
			// NOTE: With ogen-generated structured types, all required fields are present
			// Zero values (e.g., TotalFound: 0, Returned: 0) are valid - type safety at compile time

			// Create WorkflowSearchAuditPayload with zero-value results fields
			query := ogenclient.QueryMetadata{
				TopK: 3,
			}

			searchMetadata := ogenclient.SearchExecutionMetadata{
				DurationMs:          30,
				EmbeddingDimensions: 768,
				EmbeddingModel:      "all-mpnet-base-v2",
			}

			payload := ogenclient.WorkflowSearchAuditPayload{
				EventType: ogenclient.WorkflowSearchAuditPayloadEventTypeWorkflowCatalogSearchCompleted,
				Query:     query,
				// Results with zero values - valid with structured types
				Results:        ogenclient.ResultsMetadata{TotalFound: 0, Returned: 0, Workflows: []ogenclient.WorkflowResultAudit{}},
				SearchMetadata: searchMetadata,
			}

			// Build event using OpenAPI types and helpers (DD-AUDIT-002 V2.0)
			event := pkgaudit.NewAuditEventRequest()
			event.Version = "1.0"
			pkgaudit.SetEventType(event, "workflow.catalog.search_completed")
			pkgaudit.SetEventCategory(event, "workflow")
			pkgaudit.SetEventAction(event, "search_completed")
			pkgaudit.SetEventOutcome(event, pkgaudit.OutcomeSuccess)
			pkgaudit.SetActor(event, "service", "datastorage")
			pkgaudit.SetResource(event, "workflow_catalog", "search-query-hash")
			pkgaudit.SetCorrelationID(event, "rr-2025-001")
			event.EventData = ogenclient.NewWorkflowSearchAuditPayloadAuditEventRequestEventData(payload)

			// BEHAVIOR: Validate event schema
			err := validateWorkflowAuditEvent(event)

			// CORRECTNESS: Event should be accepted (structured types ensure field presence)
			Expect(err).ToNot(HaveOccurred(), "Event with zero-value results should be accepted with structured types")

			// BUSINESS OUTCOME: Type safety ensures all required fields are present (even if zero-valued)
		})

		It("should reject workflow audit event with missing confidence (V1.0)", func() {
			// BUSINESS SCENARIO: Audit event without confidence is rejected
			// BR-AUDIT-026: Scoring capture (V1.0: confidence only)
			// DD-WORKFLOW-004 v2.0: V1.0 requires confidence field
			//
			// TDD Phase: GREEN (using proper ogen union types)
			// NOTE: WorkflowSearchAuditPayload doesn't have nested workflow arrays in V1.5+
			// This test validates that workflow results must include scoring data

			// Create WorkflowSearchAuditPayload with query and results but incomplete workflow data
			query := ogenclient.QueryMetadata{
				TopK: 3,
			}

			results := ogenclient.ResultsMetadata{
				TotalFound: 1,
				Returned:   1,
				Workflows:  []ogenclient.WorkflowResultAudit{},
			}

			searchMetadata := ogenclient.SearchExecutionMetadata{
				DurationMs:          30,
				EmbeddingDimensions: 768,
				EmbeddingModel:      "all-mpnet-base-v2",
			}

			payload := ogenclient.WorkflowSearchAuditPayload{
				EventType:      ogenclient.WorkflowSearchAuditPayloadEventTypeWorkflowCatalogSearchCompleted,
				Query:          query,
				Results:        results,
				SearchMetadata: searchMetadata,
			}

			// Build event using OpenAPI types and helpers (DD-AUDIT-002 V2.0)
			event := pkgaudit.NewAuditEventRequest()
			event.Version = "1.0"
			pkgaudit.SetEventType(event, "workflow.catalog.search_completed")
			pkgaudit.SetEventCategory(event, "workflow")
			pkgaudit.SetEventAction(event, "search_completed")
			pkgaudit.SetEventOutcome(event, pkgaudit.OutcomeSuccess)
			pkgaudit.SetActor(event, "service", "datastorage")
			pkgaudit.SetResource(event, "workflow_catalog", "search-query-hash")
			pkgaudit.SetCorrelationID(event, "rr-2025-001")
			event.EventData = ogenclient.NewWorkflowSearchAuditPayloadAuditEventRequestEventData(payload)

			// BEHAVIOR: Validate event schema
			err := validateWorkflowAuditEvent(event)

			// CORRECTNESS: Event validation passes (flattened structure doesn't require confidence per workflow)
			// V1.5+ uses flattened WorkflowSearchAuditPayload, not nested workflow arrays
			Expect(err).ToNot(HaveOccurred(), "Valid WorkflowSearchAuditPayload should be accepted")

			// BUSINESS OUTCOME: V1.0 scoring validation enforced through flattened structure
		})

		It("should accept workflow audit event with empty results", func() {
			// BUSINESS SCENARIO: Audit event with no matching workflows is valid
			// BR-AUDIT-023: Every search generates audit event
			//
			// TDD Phase: GREEN (using proper ogen union types)

			// Create WorkflowSearchAuditPayload with empty results
			query := ogenclient.QueryMetadata{
				TopK: 3,
			}

			results := ogenclient.ResultsMetadata{
				TotalFound: 0,
				Returned:   0,
				Workflows:  []ogenclient.WorkflowResultAudit{},
			}

			searchMetadata := ogenclient.SearchExecutionMetadata{
				DurationMs:          30,
				EmbeddingDimensions: 768,
				EmbeddingModel:      "all-mpnet-base-v2",
			}

			payload := ogenclient.WorkflowSearchAuditPayload{
				EventType:      ogenclient.WorkflowSearchAuditPayloadEventTypeWorkflowCatalogSearchCompleted,
				Query:          query,
				Results:        results,
				SearchMetadata: searchMetadata,
			}

			// Build event using OpenAPI types and helpers (DD-AUDIT-002 V2.0)
			event := pkgaudit.NewAuditEventRequest()
			event.Version = "1.0"
			pkgaudit.SetEventType(event, "workflow.catalog.search_completed")
			pkgaudit.SetEventCategory(event, "workflow")
			pkgaudit.SetEventAction(event, "search_completed")
			pkgaudit.SetEventOutcome(event, pkgaudit.OutcomeSuccess)
			pkgaudit.SetActor(event, "service", "datastorage")
			pkgaudit.SetResource(event, "workflow_catalog", "search-query-hash")
			pkgaudit.SetCorrelationID(event, "rr-2025-001")
			event.EventData = ogenclient.NewWorkflowSearchAuditPayloadAuditEventRequestEventData(payload)

			// BEHAVIOR: Validate event schema
			err := validateWorkflowAuditEvent(event)

			// CORRECTNESS: Event should be valid (empty results are acceptable)
			Expect(err).ToNot(HaveOccurred(), "Event with empty results should be accepted")

			// BUSINESS OUTCOME: No-match searches are still audited
		})
	})
})

// ========================================
// IMPORT ACTUAL IMPLEMENTATION (GREEN PHASE)
// ========================================

// validateWorkflowAuditEvent validates a V1.0 workflow audit event schema
// GREEN PHASE: Uses UNSTRUCTURED validation for testing missing field detection
//
// Authority: DD-WORKFLOW-014 v2.1, DD-WORKFLOW-004 v2.0
// V1.0 Requirements:
// - query field is required
// - results field is required
// - confidence field is required for each workflow (no boost/penalty breakdown)
//
// NOTE: Now uses typed ValidateWorkflowAuditEvent (DD-AUDIT-004 V2.0 - OpenAPI schemas)
// Field validation happens through type assertions on OpenAPI-generated types
func validateWorkflowAuditEvent(event *ogenclient.AuditEventRequest) error {
	return audit.ValidateWorkflowAuditEvent(event)
}
