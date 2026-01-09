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
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
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
			// TDD Phase: RED (this test should FAIL initially)
			// Expected Error: undefined: validateWorkflowAuditEvent

			eventData := map[string]interface{}{
				"query": map[string]interface{}{
					"text": "OOMKilled critical",
					"filters": map[string]interface{}{
						"signal_type": "OOMKilled",
						"severity":    "critical",
					},
					"top_k":          3,
					"min_similarity": 0.7,
				},
				"results": map[string]interface{}{
					"total_found": 5,
					"returned":    3,
					"workflows": []interface{}{
						map[string]interface{}{
							"workflow_id": "pod-oom-gitops",
							"version":     "v1.0.0",
							"title":       "Pod OOM GitOps Recovery",
							"rank":        1,
							"scoring": map[string]interface{}{
								// V1.0: confidence only (no boost/penalty breakdown)
								"confidence": 0.92,
							},
							"labels": map[string]interface{}{
								"signal_type": "OOMKilled",
								"severity":    "critical",
							},
						},
					},
				},
				"search_metadata": map[string]interface{}{
					"duration_ms":          45,
					"data_storage_url":     "http://localhost:8080/api/v1/workflows/search",
					"embedding_dimensions": 768,
					"embedding_model":      "all-mpnet-base-v2",
				},
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
			pkgaudit.SetEventData(event, eventData)

			// BEHAVIOR: Validate event schema
			err := validateWorkflowAuditEvent(event)

			// CORRECTNESS: Event should be valid
			Expect(err).ToNot(HaveOccurred(), "Valid V1.0 workflow audit event should be accepted")

			// BUSINESS OUTCOME: Workflow selection audit trail is complete
		})

		It("should reject workflow audit event with missing query", func() {
			// BUSINESS SCENARIO: Incomplete audit event is rejected
			// BR-AUDIT-025: Query metadata capture
			//
			// TDD Phase: RED (this test should FAIL initially)

			eventData := map[string]interface{}{
				"results": map[string]interface{}{
					"total_found": 0,
					"returned":    0,
					"workflows":   []interface{}{},
				},
				// Missing "query" field
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
			pkgaudit.SetEventData(event, eventData)

			// BEHAVIOR: Validate event schema
			err := validateWorkflowAuditEvent(event)

			// CORRECTNESS: Event should be rejected
			Expect(err).To(HaveOccurred(), "Event with missing query should be rejected")
			Expect(err.Error()).To(ContainSubstring("missing required field: query"))

			// BUSINESS OUTCOME: Incomplete audit events are prevented
		})

		It("should reject workflow audit event with missing results", func() {
			// BUSINESS SCENARIO: Audit event without results is rejected
			// BR-AUDIT-027: Workflow metadata capture
			//
			// TDD Phase: RED (this test should FAIL initially)

			eventData := map[string]interface{}{
				"query": map[string]interface{}{
					"text": "OOMKilled critical",
				},
				// Missing "results" field
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
			pkgaudit.SetEventData(event, eventData)

			// BEHAVIOR: Validate event schema
			err := validateWorkflowAuditEvent(event)

			// CORRECTNESS: Event should be rejected
			Expect(err).To(HaveOccurred(), "Event with missing results should be rejected")
			Expect(err.Error()).To(ContainSubstring("missing required field: results"))

			// BUSINESS OUTCOME: Incomplete audit events are prevented
		})

		It("should reject workflow audit event with missing confidence (V1.0)", func() {
			// BUSINESS SCENARIO: Audit event without confidence is rejected
			// BR-AUDIT-026: Scoring capture (V1.0: confidence only)
			// DD-WORKFLOW-004 v2.0: V1.0 requires confidence field
			//
			// TDD Phase: RED (this test should FAIL initially)

			eventData := map[string]interface{}{
				"query": map[string]interface{}{
					"text": "OOMKilled critical",
				},
				"results": map[string]interface{}{
					"workflows": []interface{}{
						map[string]interface{}{
							"workflow_id": "pod-oom-gitops",
							"scoring":     map[string]interface{}{
								// Missing "confidence" field (required in V1.0)
							},
						},
					},
				},
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
			pkgaudit.SetEventData(event, eventData)

			// BEHAVIOR: Validate event schema
			err := validateWorkflowAuditEvent(event)

			// CORRECTNESS: Event should be rejected
			Expect(err).To(HaveOccurred(), "Event with missing confidence should be rejected")
			Expect(err.Error()).To(ContainSubstring("missing required field: confidence"))

			// BUSINESS OUTCOME: V1.0 scoring validation enforced
		})

		It("should accept workflow audit event with empty results", func() {
			// BUSINESS SCENARIO: Audit event with no matching workflows is valid
			// BR-AUDIT-023: Every search generates audit event
			//
			// TDD Phase: RED (this test should FAIL initially)

			eventData := map[string]interface{}{
				"query": map[string]interface{}{
					"text": "NonExistentSignal critical",
				},
				"results": map[string]interface{}{
					"total_found": 0,
					"returned":    0,
					"workflows":   []interface{}{},
				},
				"search_metadata": map[string]interface{}{
					"duration_ms": 30,
				},
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
			pkgaudit.SetEventData(event, eventData)

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
func validateWorkflowAuditEvent(event *dsgen.AuditEventRequest) error {
	return audit.ValidateWorkflowAuditEvent(event)
}
