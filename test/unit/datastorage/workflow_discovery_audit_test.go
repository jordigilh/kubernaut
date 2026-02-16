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

var _ = Describe("Workflow Discovery Audit Events (GAP-WF-6)", func() {

	Describe("GAP-WF-6: DurationMs populated in discovery audit payloads", func() {
		It("should set DurationMs in actions_listed audit event", func() {
			filters := &models.WorkflowDiscoveryFilters{
				Severity:    "critical",
				Component:   "pod",
				Environment: "production",
				Priority:    "P0",
			}
			durationMs := int64(42)

			event, err := dsaudit.NewActionsListedAuditEvent(filters, 5, durationMs)
			Expect(err).ToNot(HaveOccurred())
			Expect(event).ToNot(BeNil())
			assertDurationMsInPayload(event, durationMs)
		})

		It("should set DurationMs in workflows_listed audit event", func() {
			filters := &models.WorkflowDiscoveryFilters{
				Severity:    "high",
				Component:   "deployment",
				Environment: "staging",
				Priority:    "P1",
			}
			durationMs := int64(15)

			event, err := dsaudit.NewWorkflowsListedAuditEvent("ScaleReplicas", filters, 3, durationMs)
			Expect(err).ToNot(HaveOccurred())
			Expect(event).ToNot(BeNil())
			assertDurationMsInPayload(event, durationMs)
		})

		It("should set DurationMs in workflow_retrieved audit event", func() {
			filters := &models.WorkflowDiscoveryFilters{
				Severity:    "critical",
				Component:   "pod",
				Environment: "production",
				Priority:    "P0",
			}
			durationMs := int64(8)

			event, err := dsaudit.NewWorkflowRetrievedAuditEvent("wf-uuid-123", filters, durationMs)
			Expect(err).ToNot(HaveOccurred())
			Expect(event).ToNot(BeNil())
			assertDurationMsInPayload(event, durationMs)
		})

		It("should set DurationMs in selection_validated audit event", func() {
			filters := &models.WorkflowDiscoveryFilters{
				Severity:    "critical",
				Component:   "pod",
				Environment: "production",
				Priority:    "P0",
			}
			durationMs := int64(3)

			event, err := dsaudit.NewSelectionValidatedAuditEvent("wf-uuid-456", filters, true, durationMs)
			Expect(err).ToNot(HaveOccurred())
			Expect(event).ToNot(BeNil())
			assertDurationMsInPayload(event, durationMs)
		})
	})
})
