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

package datastorage_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// WORKFLOW DISCOVERY AUDIT EVENT UNIT TESTS
// ========================================
// GAP-WF-6: DD-WORKFLOW-014 v3.0 - queryDurationMs in discovery audit payloads
//
// Strategy: Unit tests for audit event constructors verify DurationMs is populated.
// ========================================

func assertDurationMsInPayload(event *ogenclient.AuditEventRequest, expectedDurationMs int64) {
	payload, ok := event.EventData.GetWorkflowDiscoveryAuditPayload()
	Expect(ok).To(BeTrue(), "EventData should be WorkflowDiscoveryAuditPayload")
	Expect(payload.SearchMetadata.DurationMs).To(Equal(expectedDurationMs))
}

var _ = Describe("Issue #1111: setCorrelationIDFromFilters and audit event correlation", func() {

	Describe("UT-DS-1111-001: setCorrelationIDFromFilters sets correlation from RemediationID", func() {
		It("should set correlation_id to RemediationID when non-empty", func() {
			filters := &models.WorkflowDiscoveryFilters{
				RemediationID: "rr-abc-123",
				Severity:      "critical",
				Component:     "pod",
				Environment:   "production",
				Priority:      "P0",
			}
			event, err := dsaudit.NewActionsListedAuditEvent(filters, 5, 42)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.CorrelationID).To(Equal("rr-abc-123"))
		})
	})

	Describe("UT-DS-1111-002: setCorrelationIDFromFilters uses fallbackID", func() {
		It("should set correlation_id to fallbackID when RemediationID is empty", func() {
			filters := &models.WorkflowDiscoveryFilters{
				RemediationID: "",
				Severity:      "critical",
				Component:     "pod",
				Environment:   "production",
				Priority:      "P0",
			}
			event, err := dsaudit.NewWorkflowRetrievedAuditEvent("wf-uuid-fallback", filters, 8)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.CorrelationID).To(Equal("wf-uuid-fallback"))
		})
	})

	Describe("UT-DS-1111-003: setCorrelationIDFromFilters leaves unset when both empty", func() {
		It("should leave correlation_id empty when both RemediationID and fallbackID are empty", func() {
			filters := &models.WorkflowDiscoveryFilters{
				RemediationID: "",
				Severity:      "critical",
				Component:     "pod",
				Environment:   "production",
				Priority:      "P0",
			}
			event, err := dsaudit.NewActionsListedAuditEvent(filters, 5, 42)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.CorrelationID).To(BeEmpty())
		})
	})

	Describe("UT-DS-1111-004: NewActionsListedAuditEvent with non-empty RemediationID produces valid correlation_id", func() {
		It("should produce a valid non-empty correlation_id", func() {
			filters := &models.WorkflowDiscoveryFilters{
				RemediationID: "rr-prod-deploy-001",
				Severity:      "critical",
				Component:     "deployment",
				Environment:   "production",
				Priority:      "P0",
			}
			event, err := dsaudit.NewActionsListedAuditEvent(filters, 3, 15)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.CorrelationID).To(Equal("rr-prod-deploy-001"))
			Expect(len(event.CorrelationID)).To(BeNumerically(">=", 1), "correlation_id must satisfy minLength:1")
		})
	})

	Describe("UT-DS-1111-005: NewWorkflowsListedAuditEvent with non-empty RemediationID produces valid correlation_id", func() {
		It("should produce a valid non-empty correlation_id", func() {
			filters := &models.WorkflowDiscoveryFilters{
				RemediationID: "rr-staging-pod-002",
				Severity:      "high",
				Component:     "pod",
				Environment:   "staging",
				Priority:      "P1",
			}
			event, err := dsaudit.NewWorkflowsListedAuditEvent("ScaleReplicas", filters, 2, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.CorrelationID).To(Equal("rr-staging-pod-002"))
		})
	})

	Describe("UT-DS-1111-006: Nil filters with non-empty fallbackID uses fallback", func() {
		It("should use fallbackID when filters is nil", func() {
			event, err := dsaudit.NewWorkflowRetrievedAuditEvent("wf-uuid-456", nil, 5)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.CorrelationID).To(Equal("wf-uuid-456"))
		})
	})

	Describe("UT-DS-1111-007: Empty filters.RemediationID with empty fallbackID", func() {
		It("should leave correlation_id empty", func() {
			filters := &models.WorkflowDiscoveryFilters{
				RemediationID: "",
			}
			event, err := dsaudit.NewActionsListedAuditEvent(filters, 0, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.CorrelationID).To(BeEmpty())
		})
	})
})

var _ = Describe("Workflow Discovery Audit Events (GAP-WF-6)", func() {

	Describe("GAP-WF-6: DurationMs populated in discovery audit payloads", func() {
		It("should set DurationMs in actions_listed audit event", func() {
			filters := &models.WorkflowDiscoveryFilters{
				Severity:    "critical",
				Component:   "v1/Pod",
				Environment: "production",
				Priority:    "P0",
			}
			durationMs := int64(42)

			event, err := dsaudit.NewActionsListedAuditEvent(filters, 5, durationMs)
			Expect(err).ToNot(HaveOccurred())
			assertDurationMsInPayload(event, durationMs)
		})

		It("should set DurationMs in workflows_listed audit event", func() {
			filters := &models.WorkflowDiscoveryFilters{
				Severity:    "high",
				Component:   "apps/v1/Deployment",
				Environment: "staging",
				Priority:    "P1",
			}
			durationMs := int64(15)

			event, err := dsaudit.NewWorkflowsListedAuditEvent("ScaleReplicas", filters, 3, durationMs)
			Expect(err).ToNot(HaveOccurred())
			assertDurationMsInPayload(event, durationMs)
		})

		It("should set DurationMs in workflow_retrieved audit event", func() {
			filters := &models.WorkflowDiscoveryFilters{
				Severity:    "critical",
				Component:   "v1/Pod",
				Environment: "production",
				Priority:    "P0",
			}
			durationMs := int64(8)

			event, err := dsaudit.NewWorkflowRetrievedAuditEvent("wf-uuid-123", filters, durationMs)
			Expect(err).ToNot(HaveOccurred())
			assertDurationMsInPayload(event, durationMs)
		})

		It("should set DurationMs in selection_validated audit event", func() {
			filters := &models.WorkflowDiscoveryFilters{
				Severity:    "critical",
				Component:   "v1/Pod",
				Environment: "production",
				Priority:    "P0",
			}
			durationMs := int64(3)

			event, err := dsaudit.NewSelectionValidatedAuditEvent("wf-uuid-456", filters, true, durationMs)
			Expect(err).ToNot(HaveOccurred())
			assertDurationMsInPayload(event, durationMs)
		})
	})
})
