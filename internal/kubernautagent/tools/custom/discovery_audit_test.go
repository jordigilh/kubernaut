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
	"github.com/jordigilh/kubernaut/internal/kubernautagent/workflowcatalog"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	katools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
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
			Severity:           "critical",
			ResourceKind:       "Deployment",
			Environment:        "production",
			Priority:           "P0",
			RemediationID:      "rr-audit-test-001",
			DetectedLabelsJSON: `{"gitOpsManaged":true,"gitOpsTool":"argocd"}`,
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
			Expect(ev.Data["detected_labels_json"]).To(MatchJSON(`{"gitOpsManaged":true,"gitOpsTool":"argocd"}`))
		})
	})

	Describe("UT-KA-2459-001: actions_listed records the returned action choices and page", func() {
		It("should preserve the exact entries returned to the caller, not only their total count", func() {
			fake.listActionsEntries = []models.ActionTypeEntry{{
				ActionType: "IncreaseMemoryLimits",
				Description: models.ActionTypeDescription{
					What:          "Increase pod memory limits",
					WhenToUse:     "Pods are OOMKilled",
					WhenNotToUse:  "The issue is a memory leak",
					Preconditions: "The pod is managed by a controller",
				},
				WorkflowCount: 3,
			}}
			fake.listActionsTotal = 4
			listActions := newAuditedTools(fake, store)[0]
			args, err := json.Marshal(map[string]string{
				"page":   "next",
				"cursor": custom.EncodeCursor(1, 1),
			})
			Expect(err).NotTo(HaveOccurred())

			result, err := listActions.Execute(ctx, args)
			Expect(err).NotTo(HaveOccurred())
			var response struct {
				ActionTypes []models.ActionTypeEntry `json:"actionTypes"`
			}
			Expect(json.Unmarshal([]byte(result), &response)).To(Succeed())
			Expect(response.ActionTypes).To(Equal(fake.listActionsEntries))

			Expect(store.events).To(HaveLen(1))
			ev := store.events[0]
			actions, marshalErr := json.Marshal(ev.Data["actions"])
			Expect(marshalErr).NotTo(HaveOccurred())
			Expect(actions).To(MatchJSON(`[{"action_type":"IncreaseMemoryLimits","description":{"what":"Increase pod memory limits","when_to_use":"Pods are OOMKilled","when_not_to_use":"The issue is a memory leak","preconditions":"The pod is managed by a controller"},"workflow_count":3}]`))
			Expect(ev.Data["total_count"]).To(Equal(4))
			Expect(ev.Data["returned"]).To(Equal(1))
			Expect(ev.Data["offset"]).To(Equal(1))
			Expect(ev.Data["limit"]).To(Equal(1))
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

	Describe("UT-KA-2459-002: workflows_listed records the exact ranked candidates without audit-only leakage", func() {
		It("should preserve the cache score and rank in audit data while keeping them out of the LLM response", func() {
			parameters := json.RawMessage(`{"token":"do-not-audit-sentinel"}`)
			executionBundle := "registry.example.invalid/private/do-not-audit-sentinel@sha256:abc"
			workflow := models.RemediationWorkflow{
				WorkflowID:      "550e8400-e29b-41d4-a716-446655440000",
				WorkflowName:    "oomkill-increase-memory-v1",
				Version:         "1.2.3",
				Name:            "OOM memory recovery",
				Description:     models.StructuredDescription{What: "Increase pod memory limits"},
				ExecutionBundle: &executionBundle,
				Parameters:      &parameters,
			}
			fake.listScoredWorkflows = []workflowcatalog.ScoredWorkflow{{Workflow: workflow, FinalScore: 0.51}}
			fake.listWorkflowsTotal = 5
			listWorkflows := newAuditedTools(fake, store)[1]

			result, err := listWorkflows.Execute(ctx, json.RawMessage(`{"action_type":"IncreaseMemoryLimits"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("final_score"))
			Expect(result).NotTo(ContainSubstring("0.51"))

			Expect(store.events).To(HaveLen(1))
			ev := store.events[0]
			Expect(ev.Data["action_type"]).To(Equal("IncreaseMemoryLimits"))
			workflows, marshalErr := json.Marshal(ev.Data["workflows"])
			Expect(marshalErr).NotTo(HaveOccurred())
			Expect(workflows).To(MatchJSON(`[{"workflow_id":"550e8400-e29b-41d4-a716-446655440000","title":"oomkill-increase-memory-v1","version":"1.2.3","rank":1,"final_score":0.51}]`))
			Expect(string(workflows)).NotTo(ContainSubstring("do-not-audit-sentinel"))
			Expect(ev.Data["total_count"]).To(Equal(5))
			Expect(ev.Data["returned"]).To(Equal(1))
			Expect(ev.Data["offset"]).To(Equal(0))
			Expect(ev.Data["limit"]).To(Equal(10))
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
