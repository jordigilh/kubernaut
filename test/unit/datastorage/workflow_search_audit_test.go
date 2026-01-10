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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// WORKFLOW SEARCH AUDIT GENERATION TESTS
// ========================================
// Business Requirements:
// - BR-AUDIT-023: Audit event generation in Data Storage Service
// - BR-AUDIT-024: Asynchronous non-blocking audit (ADR-038)
// - BR-AUDIT-025: Query metadata capture
// - BR-AUDIT-026: Scoring capture (V1.0: confidence only)
// - BR-AUDIT-027: Workflow metadata capture
// - BR-AUDIT-028: Search metadata capture
//
// Design Decisions:
// - DD-WORKFLOW-014 v2.1: Workflow Selection Audit Trail
// - DD-WORKFLOW-004 v2.0: V1.0 scoring = confidence only (no boost/penalty)
//
// TDD Phase: RED (failing tests)
// ========================================

var _ = Describe("Workflow Search Audit Generation", func() {
	// ========================================
	// TEST 1: Audit Event Builder - V1.0 Scoring
	// ========================================
	Context("when building workflow search audit event", func() {
		It("should build audit event with V1.0 scoring (label-only search)", func() {
			// BUSINESS SCENARIO: BR-AUDIT-026 - Scoring capture
			// V1.0: Label-only search with confidence from label matching
			//
			// BEHAVIOR: Audit event captures search results with label-based scores
			// CORRECTNESS: Event contains filters, workflow IDs, and scores from response

			// ARRANGE: Create search request and response (V1.0 label-only)
			searchRequest := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "OOMKilled",
					Severity:    "critical",
					Component:   "pod",
					Environment: "production",
					Priority:    "P1",
				},
				TopK: 3,
			}

			// DD-WORKFLOW-002 v3.0: FLAT response structure with UUID workflow_id
			searchResponse := &models.WorkflowSearchResponse{
				Workflows: []models.WorkflowSearchResult{
					{
						// DD-WORKFLOW-002 v3.0: Flat fields (no nested Workflow)
						WorkflowID:   "550e8400-e29b-41d4-a716-446655440000",
						Title:        "Pod OOM GitOps Recovery",
						Description:  "OOMKilled critical: Increases memory via GitOps PR",
						Confidence:   0.92, // V1.0 scoring: label-based confidence
						LabelBoost:   0.15,
						LabelPenalty: 0.05,
						FinalScore:   0.92,
						Rank:         1,
					},
					{
						// DD-WORKFLOW-002 v3.0: Flat fields (no nested Workflow)
						WorkflowID:   "660f9500-f30c-52e5-b827-557766551111",
						Title:        "Pod OOM Manual Recovery",
						Description:  "OOMKilled critical: Increases memory via kubectl",
						Confidence:   0.88, // V1.0 scoring: label-based confidence
						LabelBoost:   0.10,
						LabelPenalty: 0.00,
						FinalScore:   0.88,
						Rank:         2,
					},
				},
				TotalResults: 2,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "OOMKilled",
					Severity:    "critical",
					Component:   "pod",
					Environment: "production",
					Priority:    "P1",
				},
			}

			searchDuration := 45 * time.Millisecond

			// ACT: Build audit event
			auditEvent, err := NewWorkflowSearchAuditEvent(
				searchRequest,
				searchResponse,
				searchDuration,
			)

			// ASSERT: Event should be built correctly
			Expect(err).ToNot(HaveOccurred(), "Audit event builder should not error")
			Expect(auditEvent).ToNot(BeNil(), "Audit event should not be nil")

			// BEHAVIOR: Event type follows ECS naming convention
			Expect(auditEvent.EventType).To(Equal("workflow.catalog.search_completed"),
				"Event type should follow ECS workflow.catalog.* pattern")
			Expect(string(auditEvent.EventCategory)).To(Equal("workflow"),
				"Event category should be 'workflow' for catalog operations")
			Expect(auditEvent.EventAction).To(Equal("search_completed"),
				"Event action should describe the completed operation")
			Expect(string(auditEvent.EventOutcome)).To(Equal("success"),
				"Event outcome should be 'success' when search returns results")

			// BEHAVIOR: CorrelationID is a filter hash for audit trail linking
			// V1.0: Implementation uses SHA256 hash of filters (first 16 hex chars) for deterministic correlation
			Expect(auditEvent.CorrelationID).To(MatchRegexp(`^[0-9a-f]{16}$`),
				"CorrelationID should be a 16-character hex hash of the filters for audit correlation")

			// Unmarshal event_data to STRUCTURED type (compile-time safe)
			// EventData is now map[string]interface{}, need to marshal then unmarshal
			eventDataBytes, err := json.Marshal(auditEvent.EventData)
			Expect(err).ToNot(HaveOccurred())
			var eventData audit.WorkflowSearchEventData
			err = json.Unmarshal(eventDataBytes, &eventData)
			Expect(err).ToNot(HaveOccurred(), "event_data should be valid JSON")

			// CORRECTNESS: Filter metadata matches input exactly (BR-AUDIT-025)
			Expect(eventData.Query.Filters).NotTo(BeNil(),
				"Query metadata should contain filters")
			Expect(eventData.Query.Filters.SignalType).To(Equal("OOMKilled"),
				"SignalType should match the exact search filter for audit reconstruction")
			Expect(eventData.Query.Filters.Severity).To(Equal("critical"),
				"Severity should match the exact search filter")
			Expect(eventData.Query.Filters.Component).To(Equal("pod"),
				"Component should match the exact search filter")
			Expect(eventData.Query.Filters.Environment).To(Equal("production"),
				"Environment should match the exact search filter")
			Expect(eventData.Query.Filters.Priority).To(Equal("P1"),
				"Priority should match the exact search filter")
			Expect(eventData.Query.TopK).To(Equal(3),
				"TopK should match requested result limit")

			// CORRECTNESS: Results count matches response (BR-AUDIT-027)
			Expect(eventData.Results.TotalFound).To(Equal(2),
				"TotalFound should match TotalResults from response")
			Expect(eventData.Results.Returned).To(Equal(2),
				"Returned should match actual workflow count")
			Expect(eventData.Results.Workflows).To(HaveLen(2),
				"Workflows array should contain all returned results")

			// CORRECTNESS: First workflow score matches input exactly (BR-AUDIT-026)
			firstWorkflow := eventData.Results.Workflows[0]
			Expect(firstWorkflow.Scoring.Confidence).To(BeNumerically("==", 0.92),
				"Confidence should equal BaseSimilarity from first workflow result")

			// CORRECTNESS: Search metadata captures performance data (BR-AUDIT-028)
			Expect(eventData.SearchMetadata.DurationMs).To(BeNumerically("==", 45),
				"DurationMs should equal search duration in milliseconds")
		})

		It("should generate deterministic correlation_id based on filter hash", func() {
			// BUSINESS SCENARIO: BR-AUDIT-021 - Audit trail correlation
			// BEHAVIOR: Each search generates a deterministic correlation ID based on filter hash
			// CORRECTNESS: Same filters produce same CorrelationID for traceability

			searchRequest := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "CrashLoopBackOff",
					Severity:    "high",
					Component:   "pod",
					Environment: "production",
					Priority:    "P2",
				},
				TopK: 5,
			}

			searchResponse := &models.WorkflowSearchResponse{
				Workflows:    []models.WorkflowSearchResult{},
				TotalResults: 0,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "CrashLoopBackOff",
					Severity:    "high",
					Component:   "pod",
					Environment: "production",
					Priority:    "P2",
				},
			}

			auditEvent, err := NewWorkflowSearchAuditEvent(
				searchRequest,
				searchResponse,
				30*time.Millisecond,
			)

			Expect(err).ToNot(HaveOccurred())

			// CORRECTNESS: CorrelationID is a deterministic filter hash (16 hex chars)
			// Same filters should produce same correlation ID for traceability
			Expect(auditEvent.CorrelationID).To(MatchRegexp(`^[0-9a-f]{16}$`),
				"CorrelationID should be a 16-character hex hash for audit correlation")

			// BEHAVIOR: Same filters produce same correlation ID (deterministic)
			auditEvent2, err := NewWorkflowSearchAuditEvent(
				searchRequest,
				searchResponse,
				30*time.Millisecond,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(auditEvent2.CorrelationID).To(Equal(auditEvent.CorrelationID),
				"Same filters should produce same correlation ID for deterministic audit linking")

			// BEHAVIOR: Different filters produce different correlation ID
			differentRequest := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "ImagePullBackOff",
					Severity:    "critical",
					Component:   "pod",
					Environment: "staging",
					Priority:    "P1",
				},
				TopK: 5,
			}
			auditEvent3, err := NewWorkflowSearchAuditEvent(
				differentRequest,
				searchResponse,
				30*time.Millisecond,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(auditEvent3.CorrelationID).ToNot(Equal(auditEvent.CorrelationID),
				"Different filters should produce different correlation ID")
		})

		It("should handle empty results with success outcome", func() {
			// BUSINESS SCENARIO: No matching workflows found
			// BR-AUDIT-023: Every search generates audit event regardless of results
			//
			// BEHAVIOR: Empty results is a valid search outcome (not an error)
			// CORRECTNESS: Event data accurately reflects zero results

		searchRequest := &models.WorkflowSearchRequest{
			Filters: &models.WorkflowSearchFilters{
				SignalType:  "NonExistentSignal",
				Severity:    "low",
				Component:   "pod",
				Environment: "production",
				Priority:    "P1",
			},
			TopK: 3,
		}

		searchResponse := &models.WorkflowSearchResponse{
			Workflows:    []models.WorkflowSearchResult{},
			TotalResults: 0,
			Filters: &models.WorkflowSearchFilters{
				SignalType:  "NonExistentSignal",
				Severity:    "low",
				Component:   "pod",
				Environment: "production",
				Priority:    "P1",
			},
		}

			auditEvent, err := NewWorkflowSearchAuditEvent(
				searchRequest,
				searchResponse,
				25*time.Millisecond,
			)

			Expect(err).ToNot(HaveOccurred(),
				"Empty results should not cause an error")

		// BEHAVIOR: Empty results is still a successful search operation
		Expect(string(auditEvent.EventOutcome)).To(Equal("success"),
			"EventOutcome should be 'success' because the search completed without error")

		// Access the WorkflowSearchAuditPayload from the discriminated union
		payload, ok := auditEvent.EventData.GetWorkflowSearchAuditPayload()
		Expect(ok).To(BeTrue(), "EventData should contain WorkflowSearchAuditPayload")
		Expect(payload).NotTo(BeNil(), "Payload should not be nil")

		// CORRECTNESS: Event data accurately reflects zero results
		Expect(payload.Results.TotalFound).To(Equal(int32(0)),
			"TotalFound should be 0 when no workflows match")
		Expect(payload.Results.Returned).To(Equal(int32(0)),
			"Returned should be 0 when no workflows match")
		Expect(payload.Results.Workflows).To(BeEmpty(),
			"Workflows array should be empty when no workflows match")

		// CORRECTNESS: Filters are still captured for debugging
		Expect(payload.Query.Filters.IsSet()).To(BeTrue(),
			"Filters should be captured even when no results found for debugging")
		filters := payload.Query.Filters.Value
		Expect(filters.SignalType).To(Equal("NonExistentSignal"),
			"SignalType should be captured for debugging")
		Expect(string(filters.Severity)).To(Equal("low"),
			"Severity should be captured for debugging")
		})

		It("should capture complete workflow metadata for each result", func() {
			// BUSINESS SCENARIO: BR-AUDIT-027 - Workflow metadata capture
			// DD-WORKFLOW-014: All workflow fields for debugging
			// BEHAVIOR: Each workflow result includes identifying metadata
			// CORRECTNESS: WorkflowID, Name, and Description match the response exactly

			searchRequest := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "OOMKilled",
					Severity:    "critical",
					Component:   "pod",
					Environment: "production",
					Priority:    "P1",
				},
				TopK: 1,
			}

			// DD-WORKFLOW-002 v3.0: Flat response structure with UUID workflow_id
			searchResponse := &models.WorkflowSearchResponse{
				Workflows: []models.WorkflowSearchResult{
					{
						// DD-WORKFLOW-002 v3.0: Flat fields (Workflow field is json:"-")
						WorkflowID:   "770e8400-e29b-41d4-a716-446655440002",
						Title:        "Pod OOM Recovery",
						Description:  "OOMKilled critical: Comprehensive OOM recovery",
						Confidence:   0.95,
						LabelBoost:   0.20,
						LabelPenalty: 0.00,
						FinalScore:   0.95,
						Rank:         1,
					},
				},
				TotalResults: 1,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "OOMKilled",
					Severity:    "critical",
					Component:   "pod",
					Environment: "production",
					Priority:    "P1",
				},
			}

			auditEvent, err := NewWorkflowSearchAuditEvent(
				searchRequest,
				searchResponse,
				50*time.Millisecond,
			)

			Expect(err).ToNot(HaveOccurred())

			// Unmarshal to structured type
			// EventData is now map[string]interface{}, need to marshal then unmarshal
			eventDataBytes, err := json.Marshal(auditEvent.EventData)
			Expect(err).ToNot(HaveOccurred())
			var eventData audit.WorkflowSearchEventData
			err = json.Unmarshal(eventDataBytes, &eventData)
			Expect(err).ToNot(HaveOccurred())

			Expect(eventData.Results.Workflows).To(HaveLen(1),
				"Should have exactly one workflow result")

			firstWorkflow := eventData.Results.Workflows[0]

			// CORRECTNESS: WorkflowID matches the UUID from response
			Expect(firstWorkflow.WorkflowID).To(Equal("770e8400-e29b-41d4-a716-446655440002"),
				"WorkflowID should match the UUID from the search result for traceability")

			// CORRECTNESS: Title/Name matches for human-readable identification
			Expect(firstWorkflow.Title).To(Equal("Pod OOM Recovery"),
				"Title should match workflow Name for human-readable audit logs")

			// CORRECTNESS: Description captured for context
			Expect(firstWorkflow.Description).To(Equal("OOMKilled critical: Comprehensive OOM recovery"),
				"Description should be captured for audit context")

			// CORRECTNESS: Score matches input
			Expect(firstWorkflow.Scoring.Confidence).To(BeNumerically("==", 0.95),
				"Scoring.Confidence should match Confidence from result")
		})
	})

	// ========================================
	// TEST 2: Handler Integration - Async Audit
	// ========================================
	// NOTE: Handler integration tests moved to:
	// test/integration/datastorage/workflow_search_audit_test.go
	// per testing strategy (03-testing-strategy.mdc)

	// ========================================
	// TEST 3: Validation - Graceful Degradation
	// ========================================
	Context("when validating audit event graceful degradation", func() {
		It("should create valid event even with minimal input", func() {
			// BUSINESS SCENARIO: Graceful degradation for audit reliability
			// BEHAVIOR: Audit events are always created to maintain audit trail
			// CORRECTNESS: Event is valid even with minimal search data

			searchRequest := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "Unknown",
					Severity:    "low",
					Component:   "pod",
					Environment: "development",
					Priority:    "P4",
				},
				TopK: 3,
			}

			searchResponse := &models.WorkflowSearchResponse{
				Workflows:    []models.WorkflowSearchResult{},
				TotalResults: 0,
				Filters: &models.WorkflowSearchFilters{
					SignalType:  "Unknown",
					Severity:    "low",
					Component:   "pod",
					Environment: "development",
					Priority:    "P4",
				},
			}

			auditEvent, err := NewWorkflowSearchAuditEvent(
				searchRequest,
				searchResponse,
				30*time.Millisecond,
			)

			// BEHAVIOR: Event is always created for audit reliability
			Expect(err).ToNot(HaveOccurred(),
				"Audit event should be created even with minimal input")
			Expect(auditEvent).ToNot(BeNil(),
				"Audit event should not be nil")

		// CORRECTNESS: Required fields are populated
		Expect(auditEvent.EventType).ToNot(BeEmpty(),
			"EventType should always be set")
		Expect(auditEvent.CorrelationID).To(MatchRegexp(`^[0-9a-f]{16}$`),
			"CorrelationID should be a 16-character hex hash")

		// EventData is a discriminated union - check it contains WorkflowSearchAuditPayload
		payload, ok := auditEvent.EventData.GetWorkflowSearchAuditPayload()
		Expect(ok).To(BeTrue(),
			"EventData should contain WorkflowSearchAuditPayload")
		Expect(payload).ToNot(BeNil(),
			"EventData payload should not be nil")
		})
	})
})

// ========================================
// IMPORT ACTUAL IMPLEMENTATION (GREEN PHASE)
// ========================================

// NewWorkflowSearchAuditEvent is imported from the audit package.
// This wrapper exists to maintain test isolation and allow for future mocking.
//
// Authority: DD-WORKFLOW-014 v2.1, BR-AUDIT-023-030
// V1.0: confidence only (DD-WORKFLOW-004 v2.0)
func NewWorkflowSearchAuditEvent(
	request *models.WorkflowSearchRequest,
	response *models.WorkflowSearchResponse,
	duration time.Duration,
) (*ogenclient.AuditEventRequest, error) {
	// GREEN PHASE: Use actual implementation
	_ = context.Background() // Prevent unused import error
	return audit.NewWorkflowSearchAuditEvent(request, response, duration)
}
