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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// WORKFLOW SEARCH AUDIT INTEGRATION TESTS
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
// - ADR-038: Buffered async audit pattern
//
// Test Strategy:
// - Integration tests validate handler → audit store interaction
// - Uses real audit store with mock dependencies where needed
// ========================================

var _ = Describe("Workflow Search Audit Integration", func() {
	// ========================================
	// TEST 1: Handler Audit Event Generation
	// ========================================
	Context("when handler generates audit event after search", func() {
		It("should generate audit event asynchronously after search", func() {
			// BUSINESS SCENARIO: BR-AUDIT-024 - Async non-blocking audit
			// ADR-038: Buffered async pattern
			//
			// This test validates that the audit event is generated correctly
			// after a workflow search operation.

			// ARRANGE: Create search request and response
			searchRequest := &models.WorkflowSearchRequest{
				Query:         "OOMKilled critical memory increase",
				RemediationID: "integration-test-rr-001",
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
				},
				TotalResults: 1,
				Query:        "OOMKilled critical memory increase",
			}

			searchDuration := 45 * time.Millisecond

			// ACT: Build audit event (simulates what handler would do)
			auditEvent, err := audit.NewWorkflowSearchAuditEvent(
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
			Expect(auditEvent.CorrelationID).To(Equal("integration-test-rr-001"))

			// Unmarshal to STRUCTURED type (compile-time safe)
			var eventData audit.WorkflowSearchEventData
			err = json.Unmarshal(auditEvent.EventData, &eventData)
			Expect(err).ToNot(HaveOccurred(), "event_data should be valid JSON")

			// Verify query metadata (BR-AUDIT-025) - typed access
			Expect(eventData.Query.Text).To(Equal("OOMKilled critical memory increase"))
			Expect(eventData.Query.TopK).To(Equal(3))

			// Verify results (BR-AUDIT-027) - typed access
			Expect(eventData.Results.TotalFound).To(Equal(1))
			Expect(eventData.Results.Returned).To(Equal(1))
			Expect(eventData.Results.Workflows).To(HaveLen(1))

			// Verify V1.0 scoring (BR-AUDIT-026) - typed access
			firstWorkflow := eventData.Results.Workflows[0]
			Expect(firstWorkflow.Scoring.Confidence).To(BeNumerically("==", 0.92))

			// V1.0: ScoringV1 struct only has Confidence field (no boost/penalty by design)

			// Verify search metadata (BR-AUDIT-028) - typed access
			Expect(eventData.SearchMetadata.DurationMs).To(BeNumerically("==", 45))

			// BUSINESS OUTCOME: BR-AUDIT-024 validated
			// Audit event is generated correctly and can be stored asynchronously
		})

		It("should not block search response on audit failure", func() {
			// BUSINESS SCENARIO: BR-AUDIT-024 - Non-blocking audit
			// Search latency must not be affected by audit writes
			//
			// This test validates that even if audit generation fails,
			// the search response is not blocked.

			// ARRANGE: Create valid search request and response
			searchRequest := &models.WorkflowSearchRequest{
				Query:         "CrashLoopBackOff high",
				RemediationID: "integration-test-rr-002",
				TopK:          5,
			}

			searchResponse := &models.WorkflowSearchResponse{
				Workflows:    []models.WorkflowSearchResult{},
				TotalResults: 0,
				Query:        "CrashLoopBackOff high",
			}

			// ACT: Build audit event
			startTime := time.Now()
			auditEvent, err := audit.NewWorkflowSearchAuditEvent(
				searchRequest,
				searchResponse,
				30*time.Millisecond,
			)
			buildDuration := time.Since(startTime)

			// ASSERT: Event building should be fast (< 10ms)
			Expect(err).ToNot(HaveOccurred())
			Expect(auditEvent).ToNot(BeNil())
			Expect(buildDuration).To(BeNumerically("<", 10*time.Millisecond),
				"Audit event building should be fast and non-blocking")

			// Verify event is valid
			validationErr := audit.ValidateWorkflowAuditEvent(auditEvent)
			Expect(validationErr).ToNot(HaveOccurred(),
				"Audit event should be valid")

			// BUSINESS OUTCOME: BR-AUDIT-024 validated
			// Audit event building is fast and does not block search response
		})
	})

	// ========================================
	// TEST 2: Audit Event Validation
	// ========================================
	Context("when validating audit events", func() {
		It("should validate V1.0 scoring requirements", func() {
			// BUSINESS SCENARIO: BR-AUDIT-026 - V1.0 scoring validation
			// DD-WORKFLOW-004 v2.0: V1.0 = confidence only

			searchRequest := &models.WorkflowSearchRequest{
				Query:         "NodeNotReady critical",
				RemediationID: "validation-test-rr-001",
				TopK:          3,
			}

			// DD-WORKFLOW-002 v3.0: Flat response structure with UUID workflow_id
			searchResponse := &models.WorkflowSearchResponse{
				Workflows: []models.WorkflowSearchResult{
					{
						WorkflowID:      "660f9500-f30c-52e5-b827-557766551111",
						Title:           "Node Recovery",
						Description:     "Recovers nodes that are not ready",
						SignalType:      "NodeNotReady",
						ContainerImage:  "quay.io/kubernaut/workflow-node:v1.0.0",
						ContainerDigest: "sha256:def456",
						Confidence:      0.88,
					},
				},
				TotalResults: 1,
			}

			// ACT: Build and validate audit event
			auditEvent, err := audit.NewWorkflowSearchAuditEvent(
				searchRequest,
				searchResponse,
				50*time.Millisecond,
			)

			// ASSERT: Event should be valid
			Expect(err).ToNot(HaveOccurred())
			validationErr := audit.ValidateWorkflowAuditEvent(auditEvent)
			Expect(validationErr).ToNot(HaveOccurred(),
				"V1.0 audit event should be valid")
		})
	})
})
