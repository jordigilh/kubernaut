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

	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
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
		It("should build audit event with V1.0 scoring (confidence only)", func() {
			// BUSINESS SCENARIO: BR-AUDIT-026 - Scoring capture
			// DD-WORKFLOW-004 v2.0: V1.0 = confidence only (no boost/penalty breakdown)
			//
			// TDD Phase: RED - This test should FAIL initially
			// Expected Error: undefined: NewWorkflowSearchAuditEvent

			// ARRANGE: Create search request and response
			searchRequest := &models.WorkflowSearchRequest{
				Query:         "OOMKilled critical memory increase",
				RemediationID: "rr-2025-11-27-abc123",
				TopK:          3,
			}

			// DD-WORKFLOW-002 v3.0: Flat response structure with UUID workflow_id
			searchResponse := &models.WorkflowSearchResponse{
				Workflows: []models.WorkflowSearchResult{
					{
						WorkflowID:      "550e8400-e29b-41d4-a716-446655440000",
						Title:           "Pod OOM GitOps Recovery",
						Description:     "OOMKilled critical: Increases memory via GitOps PR",
						SignalType:      "OOMKilled",
						ContainerImage:  "quay.io/kubernaut/workflow-oom:v1.0.0",
						ContainerDigest: "sha256:abc123",
						Confidence:      0.92,
					},
					{
						WorkflowID:      "660f9500-f30c-52e5-b827-557766551111",
						Title:           "Pod OOM Manual Recovery",
						Description:     "OOMKilled critical: Increases memory via kubectl",
						SignalType:      "OOMKilled",
						ContainerImage:  "quay.io/kubernaut/workflow-oom-manual:v1.0.0",
						ContainerDigest: "sha256:def456",
						Confidence:      0.88,
					},
				},
				TotalResults: 2,
				Query:        "OOMKilled critical memory increase",
			}

			searchDuration := 45 * time.Millisecond

			// ACT: Build audit event
			// This function should be implemented in pkg/datastorage/audit/workflow_search_event.go
			auditEvent, err := NewWorkflowSearchAuditEvent(
				searchRequest,
				searchResponse,
				searchDuration,
			)

			// ASSERT: Event should be built correctly
			Expect(err).ToNot(HaveOccurred(), "Audit event builder should not error")
			Expect(auditEvent).ToNot(BeNil(), "Audit event should not be nil")

			// Verify event metadata
			Expect(auditEvent.EventType).To(Equal("workflow.catalog.search_completed"))
			Expect(auditEvent.EventCategory).To(Equal("workflow"))
			Expect(auditEvent.EventAction).To(Equal("search_completed"))
			Expect(auditEvent.EventOutcome).To(Equal("success"))
			Expect(auditEvent.CorrelationID).To(Equal("rr-2025-11-27-abc123"))

			// Unmarshal event_data to STRUCTURED type (compile-time safe)
			var eventData audit.WorkflowSearchEventData
			err = json.Unmarshal(auditEvent.EventData, &eventData)
			Expect(err).ToNot(HaveOccurred(), "event_data should be valid JSON")

			// Verify query metadata (BR-AUDIT-025) - typed access
			Expect(eventData.Query.Text).To(Equal("OOMKilled critical memory increase"))
			Expect(eventData.Query.TopK).To(Equal(3))

			// Verify results (BR-AUDIT-027) - typed access
			Expect(eventData.Results.TotalFound).To(Equal(2))
			Expect(eventData.Results.Returned).To(Equal(2))
			Expect(eventData.Results.Workflows).To(HaveLen(2))

			// Verify V1.0 scoring (BR-AUDIT-026) - typed access
			firstWorkflow := eventData.Results.Workflows[0]
			Expect(firstWorkflow.Scoring.Confidence).To(BeNumerically("==", 0.92))

			// V1.0: ScoringV1 struct only has Confidence field
			// No boost/penalty breakdown by design (DD-WORKFLOW-004 v2.0)

			// Verify search metadata (BR-AUDIT-028) - typed access
			Expect(eventData.SearchMetadata.DurationMs).To(BeNumerically("==", 45))
			Expect(eventData.SearchMetadata.EmbeddingModel).To(Equal("all-mpnet-base-v2"))
			Expect(eventData.SearchMetadata.EmbeddingDimensions).To(Equal(768))

			// BUSINESS OUTCOME: BR-AUDIT-026 validated (V1.0: confidence only)
		})

		It("should include remediation_id as correlation_id", func() {
			// BUSINESS SCENARIO: BR-AUDIT-021 - Remediation ID propagation
			// DD-WORKFLOW-014 v2.1: remediation_id in JSON body for audit correlation
			//
			// TDD Phase: RED

			searchRequest := &models.WorkflowSearchRequest{
				Query:         "CrashLoopBackOff high",
				RemediationID: "remediation-2025-11-27-xyz789",
				TopK:          5,
			}

			searchResponse := &models.WorkflowSearchResponse{
				Workflows:    []models.WorkflowSearchResult{},
				TotalResults: 0,
				Query:        "CrashLoopBackOff high",
			}

			auditEvent, err := NewWorkflowSearchAuditEvent(
				searchRequest,
				searchResponse,
				30*time.Millisecond,
			)

			Expect(err).ToNot(HaveOccurred())
			Expect(auditEvent.CorrelationID).To(Equal("remediation-2025-11-27-xyz789"),
				"remediation_id should be used as correlation_id")
		})

		It("should handle empty results gracefully", func() {
			// BUSINESS SCENARIO: No matching workflows found
			// BR-AUDIT-023: Every search generates audit event
			//
			// TDD Phase: RED

			searchRequest := &models.WorkflowSearchRequest{
				Query:         "NonExistentSignal unknown",
				RemediationID: "rr-empty-results",
				TopK:          3,
			}

			searchResponse := &models.WorkflowSearchResponse{
				Workflows:    []models.WorkflowSearchResult{},
				TotalResults: 0,
				Query:        "NonExistentSignal unknown",
			}

			auditEvent, err := NewWorkflowSearchAuditEvent(
				searchRequest,
				searchResponse,
				25*time.Millisecond,
			)

			Expect(err).ToNot(HaveOccurred())
			Expect(auditEvent.EventOutcome).To(Equal("success"),
				"Empty results is still a successful search")

			// Unmarshal to structured type
			var eventData audit.WorkflowSearchEventData
			err = json.Unmarshal(auditEvent.EventData, &eventData)
			Expect(err).ToNot(HaveOccurred())

			Expect(eventData.Results.TotalFound).To(Equal(0))
			Expect(eventData.Results.Returned).To(Equal(0))
			Expect(eventData.Results.Workflows).To(BeEmpty())
		})

		It("should capture workflow metadata for each result", func() {
			// BUSINESS SCENARIO: BR-AUDIT-027 - Workflow metadata capture
			// DD-WORKFLOW-014: All workflow fields for debugging
			//
			// TDD Phase: RED

			searchRequest := &models.WorkflowSearchRequest{
				Query:         "OOMKilled critical",
				RemediationID: "rr-metadata-test",
				TopK:          1,
			}

			// DD-WORKFLOW-002 v3.0: Flat response structure with UUID workflow_id
			searchResponse := &models.WorkflowSearchResponse{
				Workflows: []models.WorkflowSearchResult{
					{
						WorkflowID:      "770e8400-e29b-41d4-a716-446655440002",
						Title:           "Pod OOM Recovery",
						Description:     "OOMKilled critical: Comprehensive OOM recovery",
						SignalType:      "OOMKilled",
						ContainerImage:  "quay.io/kubernaut/workflow-oom:v2.1.0",
						ContainerDigest: "sha256:ghi789",
						Confidence:      0.95,
					},
				},
				TotalResults: 1,
				Query:        "OOMKilled critical",
			}

			auditEvent, err := NewWorkflowSearchAuditEvent(
				searchRequest,
				searchResponse,
				50*time.Millisecond,
			)

			Expect(err).ToNot(HaveOccurred())

			// Unmarshal to structured type
			var eventData audit.WorkflowSearchEventData
			err = json.Unmarshal(auditEvent.EventData, &eventData)
			Expect(err).ToNot(HaveOccurred())

			firstWorkflow := eventData.Results.Workflows[0]

			// Verify workflow metadata - typed access
			// DD-WORKFLOW-002 v3.0: workflow_id is UUID, version removed from search response
			Expect(firstWorkflow.WorkflowID).To(Equal("770e8400-e29b-41d4-a716-446655440002"))
			Expect(firstWorkflow.Title).To(Equal("Pod OOM Recovery"))
			Expect(firstWorkflow.Description).To(Equal("OOMKilled critical: Comprehensive OOM recovery"))
		})
	})

	// ========================================
	// TEST 2: Handler Integration - Async Audit
	// ========================================
	// NOTE: Handler integration tests moved to:
	// test/integration/datastorage/workflow_search_audit_test.go
	// per testing strategy (03-testing-strategy.mdc)

	// ========================================
	// TEST 3: Validation - Required Fields
	// ========================================
	Context("when validating audit event", func() {
		It("should require remediation_id for correlation", func() {
			// BUSINESS SCENARIO: BR-AUDIT-021 - Remediation ID mandatory
			// DD-WORKFLOW-014: remediation_id enables audit correlation
			//
			// TDD Phase: RED

			searchRequest := &models.WorkflowSearchRequest{
				Query:         "OOMKilled critical",
				RemediationID: "", // Empty - should still work but generate warning
				TopK:          3,
			}

			searchResponse := &models.WorkflowSearchResponse{
				Workflows:    []models.WorkflowSearchResult{},
				TotalResults: 0,
			}

			auditEvent, err := NewWorkflowSearchAuditEvent(
				searchRequest,
				searchResponse,
				30*time.Millisecond,
			)

			// Event should still be created (backwards compatibility)
			Expect(err).ToNot(HaveOccurred())
			// But correlation_id should be empty or use fallback
			// (actual behavior to be defined in GREEN phase)
			Expect(auditEvent).ToNot(BeNil())
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
) (*pkgaudit.AuditEvent, error) {
	// GREEN PHASE: Use actual implementation
	_ = context.Background() // Prevent unused import error
	return audit.NewWorkflowSearchAuditEvent(request, response, duration)
}
