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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaaudit "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

func itToolCtx() context.Context {
	return katypes.WithSignalContext(context.Background(), katypes.SignalContext{
		Severity:           "critical",
		ResourceAPIVersion: "apps/v1",
		ResourceKind:       "Deployment",
		Environment:        "production",
		Priority:           "P0",
	})
}

var _ = Describe("Kubernaut Agent Custom Tools Integration — #433", func() {

	var reg *registry.Registry

	BeforeEach(func() {
		Expect(wfCatalog).NotTo(BeNil(), "workflow catalog must be initialized by SynchronizedBeforeSuite")

		reg = registry.New()

		allTools := custom.NewAllTools(wfCatalog, nil, logr.Discard())
		Expect(allTools).To(HaveLen(3), "should create 3 custom tools")
		for _, t := range allTools {
			reg.Register(t)
		}
	})

	Describe("IT-KA-433-033: list_available_actions queries KA's informer-backed catalog", func() {
		It("should return action types from the real workflow catalog", func() {
			result, err := reg.Execute(itToolCtx(), "list_available_actions",
				json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring("actionTypes"))
		})
	})

	Describe("IT-KA-2478-002: production discovery dispatch returns ranked action evidence", func() {
		It("should expose deterministic rank/preference fields without internal scores", func() {
			result, err := reg.Execute(itToolCtx(), "list_available_actions", json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())

			var response struct {
				ActionTypes []struct {
					Rank      int  `json:"rank"`
					Preferred bool `json:"preferred"`
				} `json:"actionTypes"`
			}
			Expect(json.Unmarshal([]byte(result), &response)).To(Succeed())
			Expect(response.ActionTypes).NotTo(BeEmpty())
			Expect(response.ActionTypes[0].Rank).To(Equal(1))
			Expect(response.ActionTypes[0].Preferred).To(BeTrue())
			Expect(result).NotTo(ContainSubstring("bestMatchScore"),
				"numeric catalog scores are audit-only")
		})
	})

	Describe("IT-KA-2478-003: KA audit store persists ranked discovery evidence", func() {
		It("should persist bounded ranking evidence with compliance metadata queryable by correlation ID", func() {
			correlationID := fmt.Sprintf("it-ka-2478-audit-%d", time.Now().UnixNano())
			ctx := katypes.WithSignalContext(context.Background(), katypes.SignalContext{
				Severity:           "warning",
				ResourceAPIVersion: "v1",
				ResourceKind:       "Pod",
				Environment:        "production",
				Priority:           "P2",
				RemediationID:      correlationID,
				DetectedLabelsJSON: `{"helmManaged":true}`,
			})

			reg := registry.New()
			for _, tool := range custom.NewAllTools(wfCatalog, kaaudit.NewDSAuditStore(ogenClient), logr.Discard()) {
				reg.Register(tool)
			}

			result, err := reg.Execute(ctx, "list_available_actions", json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred(), "the real KA discovery dispatch must succeed")

			var response struct {
				ActionTypes []struct {
					ActionType string `json:"actionType"`
					Rank       int    `json:"rank"`
					Preferred  bool   `json:"preferred"`
				} `json:"actionTypes"`
			}
			Expect(json.Unmarshal([]byte(result), &response)).To(Succeed())
			Expect(response.ActionTypes).NotTo(BeEmpty())
			Expect(response.ActionTypes[0].ActionType).To(Equal("HelmRollback"),
				"BR-KA-017-007: Helm-aware action family must be preferred")
			Expect(response.ActionTypes[0].Rank).To(Equal(1))
			Expect(response.ActionTypes[0].Preferred).To(BeTrue())
			Expect(result).NotTo(ContainSubstring("bestMatchScore"),
				"ASVS V5.5.2: internal scores must not cross the LLM-facing boundary")

			var persisted *ogenclient.AuditEvent
			Eventually(func() bool {
				params := ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(correlationID),
					EventType:     ogenclient.NewOptString(kaaudit.EventTypeActionsListed),
					Limit:         ogenclient.NewOptInt(100),
				}
				resp, queryErr := ogenClient.QueryAuditEvents(ctx, params)
				if queryErr != nil {
					return false
				}
				for i := range resp.Data {
					if resp.Data[i].EventType == kaaudit.EventTypeActionsListed {
						event := resp.Data[i]
						persisted = &event
						return true
					}
				}
				return false
			}, 15*time.Second, 250*time.Millisecond).Should(BeTrue(),
				"AU-2 / CC7.2: actions_listed must be queryable by correlation ID")

			Expect(persisted.EventID.IsSet()).To(BeTrue(), "AU-3: persisted events require event_id")
			Expect(persisted.EventCategory).To(Equal(ogenclient.AuditEventEventCategoryWorkflow))
			Expect(persisted.EventAction).To(Equal(kaaudit.ActionDiscovery))
			Expect(persisted.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeSuccess))
			Expect(persisted.CorrelationID).To(Equal(correlationID))
			Expect(persisted.ActorType.IsSet()).To(BeTrue(), "AU-3: actor_type is required")
			Expect(persisted.ActorType.Value).To(Equal("service"))
			Expect(persisted.ActorID.IsSet()).To(BeTrue(), "AU-3: actor_id is required")
			Expect(persisted.ActorID.Value).To(Equal("kubernaut-agent"))

			payload, ok := persisted.EventData.GetWorkflowActionsListedAuditPayload()
			Expect(ok).To(BeTrue(), "AU-3: event_data must use the typed actions-listed payload")
			Expect(payload.Results.ActionTypes).NotTo(BeEmpty())
			var helm, generic *ogenclient.ActionTypeResultAudit
			for i := range payload.Results.ActionTypes {
				action := &payload.Results.ActionTypes[i]
				switch action.ActionType {
				case "HelmRollback":
					helm = action
				case "RestartPod":
					generic = action
				}
			}
			Expect(helm).NotTo(BeNil())
			Expect(helm.Rank).To(Equal(int32(1)))
			Expect(helm.Preferred).To(BeTrue())
			Expect(helm.BestMatchScore).To(BeNumerically(">=", 0))
			Expect(helm.BestMatchScore).To(BeNumerically("<=", 1))
			Expect(helm.BestWorkflowID.IsSet()).To(BeTrue())
			Expect(helm.PreferenceReason).To(ContainSubstring("helmManaged=true"))
			Expect(generic).NotTo(BeNil(), "generic action families remain auditable and selectable")
			Expect(generic.Preferred).To(BeFalse())

			filters, filtersSet := payload.Query.Filters.Get()
			Expect(filtersSet).To(BeTrue(), "AU-3: signal filters must be reconstructable")
			detected, detectedSet := filters.DetectedLabels.Get()
			Expect(detectedSet).To(BeTrue())
			helmManaged, helmManagedSet := detected.HelmManaged.Get()
			Expect(helmManagedSet).To(BeTrue())
			Expect(helmManaged).To(BeTrue())
		})
	})

	Describe("IT-KA-433-034: list_workflows searches the real workflow catalog with criteria", func() {
		It("should return seeded workflows from the informer-backed catalog", func() {
			result, err := reg.Execute(itToolCtx(), "list_workflows",
				json.RawMessage(`{"action_type":"IncreaseMemory"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring("workflows"))
		})
	})

	Describe("IT-KA-433-035: get_workflow retrieves a specific workflow from the catalog", func() {
		It("should return the seeded workflow definition by UUID", func() {
			Expect(workflowUUIDs).NotTo(BeEmpty(), "workflow UUIDs must be seeded")

			wfUUID, ok := workflowUUIDs["oom-recovery-v1:production"]
			Expect(ok).To(BeTrue(), "oom-recovery-v1:production must be seeded")
			Expect(wfUUID).NotTo(BeEmpty())

			workflow, err := wfCatalog.GetByID(itToolCtx(), wfUUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(workflow).NotTo(BeNil())
			Expect(workflow.WorkflowID).To(Equal(wfUUID))
			Expect(workflow.WorkflowName).To(Equal("oom-recovery-v1"))

			result, err := reg.Execute(itToolCtx(), "get_workflow",
				json.RawMessage(fmt.Sprintf(`{"workflow_id":"%s"}`, wfUUID)))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring(workflow.Description.What))
			Expect(result).To(ContainSubstring("DEPLOYMENT_NAME"))
			Expect(result).NotTo(ContainSubstring(workflow.WorkflowName),
				"the LLM-facing projection must not expose the workflow identity")
		})
	})
})

var _ = Describe("IT-KA-2466-001: LLM workflow projection from the etcd-backed Catalog", func() {
	var reg *registry.Registry

	BeforeEach(func() {
		Expect(wfCatalog).NotTo(BeNil(), "workflow catalog must be initialized by SynchronizedBeforeSuite")
		reg = registry.New()
		for _, tool := range custom.NewAllTools(wfCatalog, nil, logr.Discard()) {
			reg.Register(tool)
		}
	})

	It("returns the description and operational parameters without KA target or execution metadata", func() {
		wfUUID, ok := workflowUUIDs["oomkill-increase-memory-v1:production"]
		Expect(ok).To(BeTrue(), "oomkill-increase-memory-v1:production must be seeded")

		result, err := reg.Execute(itToolCtx(), "get_workflow",
			json.RawMessage(fmt.Sprintf(`{"workflow_id":%q}`, wfUUID)))
		Expect(err).NotTo(HaveOccurred())

		var response map[string]json.RawMessage
		Expect(json.Unmarshal([]byte(result), &response)).To(Succeed())
		Expect(response).To(HaveKey("description"))
		Expect(response).To(HaveKey("parameters"))
		Expect(response).To(HaveLen(2))
		Expect(result).NotTo(ContainSubstring("TARGET_RESOURCE_NAME"))
		Expect(result).NotTo(ContainSubstring("TARGET_RESOURCE_KIND"))
		Expect(result).NotTo(ContainSubstring("TARGET_RESOURCE_NAMESPACE"))
		Expect(result).NotTo(ContainSubstring("TARGET_RESOURCE_API_VERSION"))
		Expect(result).NotTo(ContainSubstring("schemaImage"))
		Expect(result).NotTo(ContainSubstring("executionEngine"))
		Expect(result).NotTo(ContainSubstring("executionBundle"))
		Expect(result).NotTo(ContainSubstring("serviceAccountName"))
		Expect(result).NotTo(ContainSubstring("contentHash"))

		var parameters struct {
			Schema struct {
				Parameters []struct {
					Name        string `json:"name"`
					Type        string `json:"type"`
					Required    bool   `json:"required"`
					Description string `json:"description"`
				} `json:"parameters"`
			} `json:"schema"`
		}
		Expect(json.Unmarshal(response["parameters"], &parameters)).To(Succeed())
		Expect(parameters.Schema.Parameters).To(HaveLen(1))
		Expect(parameters.Schema.Parameters[0].Name).To(Equal("MEMORY_LIMIT_NEW"))
		Expect(parameters.Schema.Parameters[0].Type).To(Equal("string"))
		Expect(parameters.Schema.Parameters[0].Required).To(BeTrue())
		Expect(parameters.Schema.Parameters[0].Description).To(ContainSubstring("New memory limit"))
		Expect(result).NotTo(ContainSubstring("synthetic-sensitive-audit-sentinel-2459"),
			"the seeded metadata annotation must not be present in the LLM-facing projection")
	})
})

var _ = Describe("IT-KA-2459-001: discovery results persist through the buffered Data Storage audit path", func() {
	It("reconstructs the returned actions and ranked workflow candidates by remediation ID", func() {
		remediationID := "it-ka-2459-" + uuid.NewString()
		toolCtx := katypes.WithSignalContext(context.Background(), katypes.SignalContext{
			Severity: "critical", ResourceKind: "Pod", Environment: "production", Priority: "P1",
			RemediationID: remediationID,
		})
		store, err := kaaudit.NewBufferedDSAuditStore(dsAuditClient, logr.Discard(),
			kaaudit.WithFlushInterval(time.Hour), kaaudit.WithBufferSize(20), kaaudit.WithBatchSize(20))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(store.Close()).To(Succeed())
		})

		reg := registry.New()
		custom.RegisterAll(reg, wfCatalog, store, nil, nil, logr.Discard())

		By("executing Step 1 and retaining the actual action options returned to the caller")
		actionsResult, err := reg.Execute(toolCtx, "list_available_actions", json.RawMessage(`{}`))
		Expect(err).NotTo(HaveOccurred())
		var actionsResponse struct {
			ActionTypes []struct {
				ActionType    string `json:"actionType"`
				WorkflowCount int    `json:"workflowCount"`
				Description   struct {
					What          string `json:"what"`
					WhenToUse     string `json:"whenToUse"`
					WhenNotToUse  string `json:"whenNotToUse"`
					Preconditions string `json:"preconditions"`
				} `json:"description"`
			} `json:"actionTypes"`
		}
		Expect(json.Unmarshal([]byte(actionsResult), &actionsResponse)).To(Succeed())
		Expect(actionsResponse.ActionTypes).NotTo(BeEmpty())

		By("executing Step 2 and retaining the ordered candidate identities returned to the caller")
		workflowsResult, err := reg.Execute(toolCtx, "list_workflows", json.RawMessage(`{"action_type":"IncreaseMemoryLimits"}`))
		Expect(err).NotTo(HaveOccurred())
		var workflowsResponse struct {
			Workflows []struct {
				WorkflowID   string `json:"workflowId"`
				WorkflowName string `json:"workflowName"`
				Version      string `json:"version"`
			} `json:"workflows"`
		}
		Expect(json.Unmarshal([]byte(workflowsResult), &workflowsResponse)).To(Succeed())
		Expect(workflowsResponse.Workflows).NotTo(BeEmpty())

		By("flushing the async buffer before querying with the independent Data Storage client")
		flushCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		Expect(store.Flush(flushCtx)).To(Succeed())

		resp, err := ogenClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
			CorrelationID: ogenclient.NewOptString(remediationID),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Data).To(HaveLen(2), "the unique remediation ID should return the two discovery audit events")

		By("checking the raw Data Storage response so typed decoding cannot hide leaked extra JSON fields")
		rawURL := dsURL + "/api/v1/audit/events?" + url.Values{"correlation_id": {remediationID}}.Encode()
		rawRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
		Expect(err).NotTo(HaveOccurred())
		rawResponse, err := dsHTTPClient.Do(rawRequest)
		Expect(err).NotTo(HaveOccurred())
		rawBody, err := io.ReadAll(rawResponse.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(rawResponse.Body.Close()).To(Succeed())
		Expect(rawResponse.StatusCode).To(Equal(http.StatusOK), string(rawBody))
		var rawEnvelope struct {
			Data []struct {
				EventType string          `json:"event_type"`
				EventData json.RawMessage `json:"event_data"`
			} `json:"data"`
		}
		Expect(json.Unmarshal(rawBody, &rawEnvelope)).To(Succeed())
		Expect(rawEnvelope.Data).To(HaveLen(2))
		for _, rawEvent := range rawEnvelope.Data {
			Expect(rawEvent.EventType).To(Or(Equal(kaaudit.EventTypeActionsListed), Equal(kaaudit.EventTypeWorkflowsListed)))
			Expect(string(rawEvent.EventData)).NotTo(ContainSubstring("parameters"))
			Expect(string(rawEvent.EventData)).NotTo(ContainSubstring("execution_bundle"))
			Expect(string(rawEvent.EventData)).NotTo(ContainSubstring("synthetic-sensitive-audit-sentinel-2459"))
		}

		var actionsEvent, workflowsEvent *ogenclient.AuditEvent
		for i := range resp.Data {
			event := &resp.Data[i]
			Expect(event.CorrelationID).To(Equal(remediationID))
			Expect(event.EventTimestamp).NotTo(BeZero())
			Expect(event.EventCategory).To(Equal(ogenclient.AuditEventEventCategoryWorkflow))
			Expect(event.EventAction).To(Equal("discovery"))
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeSuccess))
			Expect(event.ActorType.Value).To(Equal("service"))
			Expect(event.ActorID.Value).To(Equal("kubernaut-agent"))
			Expect(event.EventID.Set).To(BeTrue())
			switch event.EventType {
			case kaaudit.EventTypeActionsListed:
				actionsEvent = event
			case kaaudit.EventTypeWorkflowsListed:
				workflowsEvent = event
			}
		}
		Expect(actionsEvent).NotTo(BeNil())
		Expect(workflowsEvent).NotTo(BeNil())
		Expect(actionsEvent.EventData.IsWorkflowActionsListedAuditPayload()).To(BeTrue())
		Expect(workflowsEvent.EventData.IsWorkflowCandidatesListedAuditPayload()).To(BeTrue())

		var actionPayload map[string]json.RawMessage
		actionData, err := json.Marshal(actionsEvent.EventData)
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(actionData, &actionPayload)).To(Succeed())
		var actionQuery struct {
			TopK    int `json:"top_k"`
			Offset  int `json:"offset"`
			Filters struct {
				Severity    string `json:"severity"`
				Component   string `json:"component"`
				Environment string `json:"environment"`
				Priority    string `json:"priority"`
			} `json:"filters"`
		}
		Expect(json.Unmarshal(actionPayload["query"], &actionQuery)).To(Succeed())
		Expect(actionQuery.TopK).To(Equal(10))
		Expect(actionQuery.Offset).To(Equal(0))
		Expect(actionQuery.Filters.Severity).To(Equal("critical"))
		Expect(actionQuery.Filters.Component).To(Equal("pod"))
		Expect(actionQuery.Filters.Environment).To(Equal("production"))
		Expect(actionQuery.Filters.Priority).To(Equal("P1"))
		var actionResults struct {
			TotalFound int `json:"total_found"`
			Returned   int `json:"returned"`
			Actions    []struct {
				ActionType    string `json:"action_type"`
				WorkflowCount int    `json:"workflow_count"`
				Description   struct {
					What          string `json:"what"`
					WhenToUse     string `json:"when_to_use"`
					WhenNotToUse  string `json:"when_not_to_use"`
					Preconditions string `json:"preconditions"`
				} `json:"description"`
			} `json:"actions"`
		}
		Expect(json.Unmarshal(actionPayload["results"], &actionResults)).To(Succeed())
		Expect(actionResults.TotalFound).To(BeNumerically(">=", actionResults.Returned))
		Expect(actionResults.Returned).To(Equal(len(actionsResponse.ActionTypes)))
		Expect(actionResults.Actions).To(HaveLen(len(actionsResponse.ActionTypes)))
		for i, action := range actionsResponse.ActionTypes {
			Expect(actionResults.Actions[i].ActionType).To(Equal(action.ActionType))
			Expect(actionResults.Actions[i].WorkflowCount).To(Equal(action.WorkflowCount))
			Expect(actionResults.Actions[i].Description.What).To(Equal(action.Description.What))
			Expect(actionResults.Actions[i].Description.WhenToUse).To(Equal(action.Description.WhenToUse))
			Expect(actionResults.Actions[i].Description.WhenNotToUse).To(Equal(action.Description.WhenNotToUse))
			Expect(actionResults.Actions[i].Description.Preconditions).To(Equal(action.Description.Preconditions))
		}

		var workflowPayload map[string]json.RawMessage
		workflowData, err := json.Marshal(workflowsEvent.EventData)
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(workflowData, &workflowPayload)).To(Succeed())
		var actionType string
		Expect(json.Unmarshal(workflowPayload["action_type"], &actionType)).To(Succeed())
		Expect(actionType).To(Equal("IncreaseMemoryLimits"))
		var workflowResults struct {
			TotalFound int `json:"total_found"`
			Returned   int `json:"returned"`
			Workflows  []struct {
				WorkflowID string  `json:"workflow_id"`
				Title      string  `json:"title"`
				Version    string  `json:"version"`
				Rank       int     `json:"rank"`
				FinalScore float64 `json:"final_score"`
				Scoring    struct {
					Confidence float64 `json:"confidence"`
				} `json:"scoring"`
			} `json:"workflows"`
		}
		Expect(json.Unmarshal(workflowPayload["results"], &workflowResults)).To(Succeed())
		Expect(workflowResults.TotalFound).To(BeNumerically(">=", workflowResults.Returned))
		Expect(workflowResults.Returned).To(Equal(len(workflowsResponse.Workflows)))
		Expect(workflowResults.Workflows).To(HaveLen(len(workflowsResponse.Workflows)))
		for i, candidate := range workflowResults.Workflows {
			Expect(candidate.WorkflowID).To(Equal(workflowsResponse.Workflows[i].WorkflowID))
			Expect(candidate.Title).To(Equal(workflowsResponse.Workflows[i].WorkflowName))
			Expect(candidate.Version).To(Equal(workflowsResponse.Workflows[i].Version))
			Expect(candidate.Rank).To(Equal(i + 1))
			Expect(candidate.FinalScore).To(Equal(0.5))
			Expect(candidate.Scoring.Confidence).To(Equal(candidate.FinalScore))
		}
		Expect(string(workflowData)).NotTo(ContainSubstring("parameters"))
		Expect(string(workflowData)).NotTo(ContainSubstring("execution_bundle"))
		Expect(string(workflowData)).NotTo(ContainSubstring("synthetic-sensitive-audit-sentinel-2459"))
	})
})

var _ = Describe("Cursor-Based Pagination over KA's Workflow Catalog — #688", func() {

	var reg *registry.Registry

	BeforeEach(func() {
		Expect(wfCatalog).NotTo(BeNil(), "workflow catalog must be initialized by SynchronizedBeforeSuite")

		reg = registry.New()
		for _, t := range custom.NewAllTools(wfCatalog, nil, logr.Discard()) {
			reg.Register(t)
		}
	})

	Describe("IT-KA-688-401: list_workflows cursor pagination through the catalog tool wire", func() {
		// Two seeded IncreaseMemoryLimits workflows match the hardcoded filters
		// (oomkill-increase-memory-v1 and oom-recovery-aggressive-v1 both have
		// severity=critical, component=*, environment=production, priority=*).
		// A cursor with limit=1 forces pagination so each page returns one workflow.

		It("should paginate forward and backward using cursor tokens", func() {
			By("Page 1: first page with limit=1 cursor")
			cursor1 := custom.EncodeCursor(0, 1)
			args1 := json.RawMessage(fmt.Sprintf(
				`{"action_type":"IncreaseMemoryLimits","page":"next","cursor":"%s"}`, cursor1))

			result1, err := reg.Execute(itToolCtx(), "list_workflows", args1)
			Expect(err).NotTo(HaveOccurred())

			var page1 map[string]json.RawMessage
			Expect(json.Unmarshal([]byte(result1), &page1)).To(Succeed())

			var workflows1 []json.RawMessage
			Expect(json.Unmarshal(page1["workflows"], &workflows1)).To(Succeed())
			Expect(workflows1).To(HaveLen(1), "limit=1 should return exactly 1 workflow")

			Expect(page1).To(HaveKey("pagination"), "page 1 of 2 must have pagination")
			var pag1 map[string]interface{}
			Expect(json.Unmarshal(page1["pagination"], &pag1)).To(Succeed())
			Expect(pag1["hasNext"]).To(BeTrue(), "more workflows exist on next page")
			Expect(pag1).To(HaveKey("nextCursor"))
			Expect(pag1).NotTo(HaveKey("hasPrevious"), "first page has no previous")
			Expect(pag1).NotTo(HaveKey("totalCount"), "totalCount must never be exposed to LLM")

			By("Page 2: navigate forward using nextCursor")
			nextCursor := pag1["nextCursor"].(string)
			args2 := json.RawMessage(fmt.Sprintf(
				`{"action_type":"IncreaseMemoryLimits","page":"next","cursor":"%s"}`, nextCursor))

			result2, err := reg.Execute(itToolCtx(), "list_workflows", args2)
			Expect(err).NotTo(HaveOccurred())

			var page2 map[string]json.RawMessage
			Expect(json.Unmarshal([]byte(result2), &page2)).To(Succeed())

			var workflows2 []json.RawMessage
			Expect(json.Unmarshal(page2["workflows"], &workflows2)).To(Succeed())
			Expect(workflows2).To(HaveLen(1), "second page should return the remaining workflow")

			Expect(page2).To(HaveKey("pagination"), "offset>0 means pagination present")
			var pag2 map[string]interface{}
			Expect(json.Unmarshal(page2["pagination"], &pag2)).To(Succeed())
			Expect(pag2).NotTo(HaveKey("hasNext"), "last page has no next")
			Expect(pag2["hasPrevious"]).To(BeTrue(), "second page can go back")
			Expect(pag2).To(HaveKey("previousCursor"))
			Expect(pag2).NotTo(HaveKey("totalCount"))

			By("Page 3: navigate backward using previousCursor")
			prevCursor := pag2["previousCursor"].(string)
			args3 := json.RawMessage(fmt.Sprintf(
				`{"action_type":"IncreaseMemoryLimits","page":"previous","cursor":"%s"}`, prevCursor))

			result3, err := reg.Execute(itToolCtx(), "list_workflows", args3)
			Expect(err).NotTo(HaveOccurred())

			var page3 map[string]json.RawMessage
			Expect(json.Unmarshal([]byte(result3), &page3)).To(Succeed())

			var workflows3 []json.RawMessage
			Expect(json.Unmarshal(page3["workflows"], &workflows3)).To(Succeed())
			Expect(workflows3).To(HaveLen(1), "back to first page, 1 workflow")

			Expect(page3).To(HaveKey("pagination"))
			var pag3 map[string]interface{}
			Expect(json.Unmarshal(page3["pagination"], &pag3)).To(Succeed())
			Expect(pag3["hasNext"]).To(BeTrue(), "first page still has more")
			Expect(pag3).NotTo(HaveKey("hasPrevious"), "back at first page")
			Expect(pag3).NotTo(HaveKey("totalCount"))

			By("Verifying page 1 and page 3 return the same workflow (idempotent navigation)")
			Expect(string(workflows1[0])).To(Equal(string(workflows3[0])),
				"navigating back should return the same first-page workflow")
		})
	})

	Describe("IT-KA-2442-001: list_workflows discovery membership through real catalog wire", func() {
		It("should accumulate IDs from every page in the current discovery context", func() {
			state := katypes.NewDiscoveredWorkflowState()
			ctx := katypes.WithDiscoveredWorkflowState(itToolCtx(), state)

			page1, err := reg.Execute(ctx, "list_workflows", json.RawMessage(fmt.Sprintf(
				`{"action_type":"IncreaseMemoryLimits","page":"next","cursor":"%s"}`,
				custom.EncodeCursor(0, 1))))
			Expect(err).NotTo(HaveOccurred())
			page1Data := decodeWorkflowPage(page1)

			var page1Pagination map[string]interface{}
			Expect(json.Unmarshal(page1Data["pagination"], &page1Pagination)).To(Succeed())
			nextCursor, ok := page1Pagination["nextCursor"].(string)
			Expect(ok).To(BeTrue())

			page2, err := reg.Execute(ctx, "list_workflows", json.RawMessage(fmt.Sprintf(
				`{"action_type":"IncreaseMemoryLimits","page":"next","cursor":"%s"}`,
				nextCursor)))
			Expect(err).NotTo(HaveOccurred())
			page2Data := decodeWorkflowPage(page2)

			for _, page := range []map[string]json.RawMessage{page1Data, page2Data} {
				var workflows []struct {
					WorkflowID string `json:"workflowId"`
				}
				Expect(json.Unmarshal(page["workflows"], &workflows)).To(Succeed())
				for _, workflow := range workflows {
					Expect(state.Contains(workflow.WorkflowID)).To(BeTrue())
				}
			}
		})
	})

	Describe("IT-KA-688-402: list_workflows without cursor returns all matching workflows", func() {
		It("should return all matching workflows with pagination stripped (single page)", func() {
			By("Calling without page/cursor — DS uses default limit=10, all 2 workflows fit in one page")
			result, err := reg.Execute(itToolCtx(), "list_workflows",
				json.RawMessage(`{"action_type":"IncreaseMemoryLimits"}`))
			Expect(err).NotTo(HaveOccurred())

			var resp map[string]json.RawMessage
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())

			var workflows []json.RawMessage
			Expect(json.Unmarshal(resp["workflows"], &workflows)).To(Succeed())
			Expect(len(workflows)).To(BeNumerically(">=", 2),
				"default limit=10 should return all seeded matching workflows")

			Expect(resp).NotTo(HaveKey("pagination"),
				"single-page response should have pagination stripped by TransformPagination")
		})
	})
})

func decodeWorkflowPage(data string) map[string]json.RawMessage {
	var page map[string]json.RawMessage
	Expect(json.Unmarshal([]byte(data), &page)).To(Succeed())
	return page
}
