/*
Copyright 2026 Jordi Gil.

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

package custom_test

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaaudit "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	katools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// fakeAuditStore captures every AuditEvent passed to StoreAudit (Issue #1677
// Phase 2d: proves the 3 workflow discovery tools emit the 4 catalog audit
// events built in Phase 2c, DD-WORKFLOW-019/BR-AUDIT-023).
type fakeAuditStore struct {
	events []*kaaudit.AuditEvent
}

func (f *fakeAuditStore) StoreAudit(_ context.Context, event *kaaudit.AuditEvent) error {
	f.events = append(f.events, event)
	return nil
}

func newAuditedTools(catalog custom.WorkflowCatalog, store *fakeAuditStore) []katools.Tool {
	return custom.NewAllTools(catalog, store, logr.Discard())
}

var _ = Describe("IT-KA-1677-AUDIT-001..004: workflow discovery tools emit catalog audit events", func() {

	var (
		fake  *fakeWorkflowDS
		store *fakeAuditStore
		ctx   context.Context
	)

	BeforeEach(func() {
		fake = &fakeWorkflowDS{
			listActionsEntries: []models.ActionTypeEntry{
				{ActionType: "ScaleReplicas", Description: models.ActionTypeDescription{What: "test", WhenToUse: "test"}, WorkflowCount: 1},
			},
			listActionsTotal: 1,
			listWorkflowsEntries: []models.RemediationWorkflow{
				{WorkflowID: "550e8400-e29b-41d4-a716-446655440000", WorkflowName: "scale-conservative-v1", Name: "Scale Conservative", Description: models.StructuredDescription{What: "test", WhenToUse: "test"}},
			},
			listWorkflowsTotal: 1,
			getWorkflowResult: &models.RemediationWorkflow{
				WorkflowID:   "550e8400-e29b-41d4-a716-446655440000",
				WorkflowName: "scale-conservative-v1",
			},
		}
		store = &fakeAuditStore{}
		ctx = katypes.WithSignalContext(context.Background(), katypes.SignalContext{
			Severity:      "critical",
			ResourceKind:  "Deployment",
			Environment:   "production",
			Priority:      "P0",
			RemediationID: "rr-audit-test-001",
		})
	})

	Describe("IT-KA-1677-AUDIT-001: list_available_actions emits workflow.catalog.actions_listed", func() {
		It("should emit exactly one event with the correct type, category, action, and correlation", func() {
			allTools := newAuditedTools(fake, store)
			listActions := allTools[0]

			_, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(store.events).To(HaveLen(1))
			ev := store.events[0]
			Expect(ev.EventType).To(Equal(kaaudit.EventTypeActionsListed))
			Expect(ev.EventCategory).To(Equal(kaaudit.WorkflowCatalogEventCategory))
			Expect(ev.EventAction).To(Equal(kaaudit.ActionDiscovery))
			Expect(ev.EventOutcome).To(Equal(kaaudit.OutcomeSuccess))
			Expect(ev.CorrelationID).To(Equal("rr-audit-test-001"))
			Expect(ev.Data["total_count"]).To(Equal(1))
			Expect(ev.Data["severity"]).To(Equal("critical"))
			Expect(ev.Data["component"]).To(Equal("deployment"))
			Expect(ev.Data["environment"]).To(Equal("production"))
			Expect(ev.Data["priority"]).To(Equal("P0"))
		})
	})

	Describe("IT-KA-1677-AUDIT-002: list_workflows emits workflow.catalog.workflows_listed", func() {
		It("should emit exactly one event with the correct type, category, action, and action_type", func() {
			allTools := newAuditedTools(fake, store)
			listWorkflows := allTools[1]

			_, err := listWorkflows.Execute(ctx, json.RawMessage(`{"action_type":"ScaleReplicas"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(store.events).To(HaveLen(1))
			ev := store.events[0]
			Expect(ev.EventType).To(Equal(kaaudit.EventTypeWorkflowsListed))
			Expect(ev.EventCategory).To(Equal(kaaudit.WorkflowCatalogEventCategory))
			Expect(ev.EventAction).To(Equal(kaaudit.ActionDiscovery))
			Expect(ev.EventOutcome).To(Equal(kaaudit.OutcomeSuccess))
			Expect(ev.CorrelationID).To(Equal("rr-audit-test-001"))
			Expect(ev.Data["total_count"]).To(Equal(1))
			Expect(ev.Data["action_type"]).To(Equal("ScaleReplicas"))
		})
	})

	Describe("IT-KA-1677-AUDIT-003/004: get_workflow emits workflow_retrieved + selection_validated when context filters are present", func() {
		It("should emit both events with ResourceType/ResourceID set to the workflow", func() {
			allTools := newAuditedTools(fake, store)
			getWorkflow := allTools[2]

			_, err := getWorkflow.Execute(ctx, json.RawMessage(`{"workflow_id":"550e8400-e29b-41d4-a716-446655440000"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(store.events).To(HaveLen(2))

			retrieved := store.events[0]
			Expect(retrieved.EventType).To(Equal(kaaudit.EventTypeWorkflowRetrieved))
			Expect(retrieved.EventCategory).To(Equal(kaaudit.WorkflowCatalogEventCategory))
			Expect(retrieved.EventAction).To(Equal(kaaudit.ActionRetrieve))
			Expect(retrieved.EventOutcome).To(Equal(kaaudit.OutcomeSuccess))
			Expect(retrieved.CorrelationID).To(Equal("rr-audit-test-001"))
			Expect(retrieved.ResourceType).To(Equal("Workflow"))
			Expect(retrieved.ResourceID).To(Equal("550e8400-e29b-41d4-a716-446655440000"))

			validated := store.events[1]
			Expect(validated.EventType).To(Equal(kaaudit.EventTypeSelectionValidated))
			Expect(validated.EventCategory).To(Equal(kaaudit.WorkflowCatalogEventCategory))
			Expect(validated.EventAction).To(Equal(kaaudit.ActionValidate))
			Expect(validated.EventOutcome).To(Equal(kaaudit.OutcomeSuccess))
			Expect(validated.CorrelationID).To(Equal("rr-audit-test-001"))
			Expect(validated.ResourceType).To(Equal("Workflow"))
			Expect(validated.ResourceID).To(Equal("550e8400-e29b-41d4-a716-446655440000"))
		})
	})

	Describe("get_workflow does not emit audit events when no context filters are present", func() {
		It("should emit zero events for a bare workflow lookup without signal context", func() {
			allTools := newAuditedTools(fake, store)
			getWorkflow := allTools[2]

			_, err := getWorkflow.Execute(context.Background(), json.RawMessage(`{"workflow_id":"550e8400-e29b-41d4-a716-446655440000"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(store.events).To(BeEmpty(), "get_workflow must not emit audit events absent context filters (DD-WORKFLOW-014 v3.0)")
		})
	})

	Describe("audit emission is a no-op when auditStore is nil", func() {
		It("should not panic and should still return a valid result", func() {
			allTools := custom.NewAllTools(fake, nil, logr.Discard())
			listActions := allTools[0]

			result, err := listActions.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
		})
	})
})
