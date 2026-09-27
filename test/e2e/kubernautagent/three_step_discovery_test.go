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

package kubernautagent

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	kaaudit "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// ========================================
// E2E-KA-017: Three-Step Workflow Discovery (DD-KA-017)
// ========================================
//
// Business Requirements:
//   - BR-KA-017-001: Three-step tool implementation
//
// Design Decisions:
//   - DD-KA-017: Three-Step Workflow Discovery Integration
//   - DD-WORKFLOW-016: Action-Type Workflow Catalog Indexing
//
// Test Strategy:
//   The three-step discovery protocol (list_available_actions → list_workflows → get_workflow)
//   is transparent to API callers. The KA Python toolset handles the multi-turn tool call
//   loop internally. Mock LLM is programmed to follow the three-step sequence when it detects
//   the discovery tools in the available tools list.
//
//   These tests verify that the full stack (KA → Mock LLM → DS) works with the new protocol
//   by exercising incident flows that trigger workflow discovery.

var _ = Describe("E2E-KA-017: Three-Step Workflow Discovery", Label("e2e", "ka", "discovery", "three-step"), func() {

	Context("BR-KA-017-001: Incident flow with three-step discovery", func() {

		It("E2E-KA-2459-001: Incident analysis persists three-step discovery evidence", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-2459-001
			// Business Outcome: Full incident analysis uses three-step discovery with Mock LLM.
			//   Mock LLM calls list_available_actions → list_workflows → get_workflow,
			//   KA returns a valid investigation result with selected workflow.
			// BR: BR-KA-017-001
			// Phase: 11 (DD-KA-017 Implementation Plan)

			// ========================================
			// ARRANGE
			// ========================================
			remediationID := "test-discovery-2459-" + uuid.NewString()
			dataStorageClient, err := ogenclient.NewClient(dataStorageURL, ogenclient.WithClient(authHTTPClient))
			Expect(err).NotTo(HaveOccurred(), "authenticated Data Storage query client should be available")

			// OOMKilled signal triggers the "oomkilled" Mock LLM scenario.
			// Mock LLM three-step flow:
			//   Step 1: list_available_actions → KA's etcd-backed Catalog returns action types
			//   Step 2: list_workflows(action_type="IncreaseMemoryLimits") → Catalog returns candidates
			//   Step 3: get_workflow(workflow_id=<selected workflow UUID>) → Catalog returns its permitted projection
			//   Step 4: Final analysis with selected_workflow
			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: remediationID, Namespace: sharedNamespace},
				IncidentID:            remediationID,
				RemediationID:         remediationID,
				SignalName:            "OOMKilled",
				Severity:              "critical",
				SignalSource:          "prometheus",
				ResourceNamespace:     "production",
				ResourceKind:          "Pod",
				ResourceName:          "api-server-abc123",
				ErrorMessage:          "Container memory limit exceeded - testing three-step discovery",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			// ========================================
			// ACT (#2190: AgentSession CRD flow)
			// ========================================
			incidentResp, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis should succeed with three-step discovery")

			// ========================================
			// ASSERT
			// ========================================

			// BEHAVIOR: Workflow selected via three-step discovery
			Expect(incidentResp.SelectedWorkflow).ToNot(BeNil(),
				"selectedWorkflow must be present — three-step discovery should find oomkill-increase-memory-v1")
			// DataStorage assigns the catalog UUID from the seeded workflow content;
			// assert the stable workflow identity rather than a retired deterministic ID.
			Expect(string(incidentResp.SelectedWorkflow.Raw)).To(ContainSubstring(
				`"workflow_name":"oomkill-increase-memory-v1"`),
				"selectedWorkflow must be the workflow returned by list_workflows")
			var selectedWorkflow struct {
				WorkflowID   string `json:"workflow_id"`
				WorkflowName string `json:"workflow_name"`
			}
			Expect(json.Unmarshal(incidentResp.SelectedWorkflow.Raw, &selectedWorkflow)).To(Succeed())

			By("querying Data Storage by the unique remediation ID until both discovery events arrive")
			var auditEvents []ogenclient.AuditEvent
			Eventually(func() bool {
				resp, queryErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(remediationID),
				})
				if queryErr != nil {
					return false
				}
				auditEvents = resp.Data
				hasActions, hasWorkflows := false, false
				for _, event := range auditEvents {
					hasActions = hasActions || event.EventType == kaaudit.EventTypeActionsListed
					hasWorkflows = hasWorkflows || event.EventType == kaaudit.EventTypeWorkflowsListed
				}
				return hasActions && hasWorkflows
			}, 30*time.Second, time.Second).Should(BeTrue(),
				"Data Storage must persist both workflow discovery events for this remediation ID")
			rawURL := dataStorageURL + "/api/v1/audit/events?" + url.Values{"correlation_id": {remediationID}}.Encode()
			rawRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
			Expect(err).NotTo(HaveOccurred())
			rawResponse, err := authHTTPClient.Do(rawRequest)
			Expect(err).NotTo(HaveOccurred())
			rawAuditJSON, err := io.ReadAll(rawResponse.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(rawResponse.Body.Close()).To(Succeed())
			Expect(rawResponse.StatusCode).To(Equal(http.StatusOK), string(rawAuditJSON))
			Expect(string(rawAuditJSON)).NotTo(ContainSubstring("synthetic-sensitive-audit-sentinel-2459"),
				"raw persisted Data Storage JSON must exclude the seeded parameter sentinel")
			var rawAuditEnvelope struct {
				Data []struct {
					EventType string          `json:"event_type"`
					EventData json.RawMessage `json:"event_data"`
				} `json:"data"`
			}
			Expect(json.Unmarshal(rawAuditJSON, &rawAuditEnvelope)).To(Succeed())
			getWorkflowAuditFound := false
			for _, rawEvent := range rawAuditEnvelope.Data {
				if rawEvent.EventType != kaaudit.EventTypeLLMToolCall {
					continue
				}
				var toolCall struct {
					ToolName          string          `json:"tool_name"`
					ToolResult        json.RawMessage `json:"tool_result"`
					ToolResultPreview string          `json:"tool_result_preview"`
				}
				Expect(json.Unmarshal(rawEvent.EventData, &toolCall)).To(Succeed())
				if toolCall.ToolName != "get_workflow" {
					continue
				}
				getWorkflowAuditFound = true
				Expect(string(toolCall.ToolResult)).To(MatchJSON(`{"result_omitted":true,"reason":"workflow_schema_not_persisted"}`))
				Expect(string(toolCall.ToolResult)).NotTo(ContainSubstring("MEMORY_LIMIT_NEW"))
				Expect(string(toolCall.ToolResult)).NotTo(ContainSubstring("parameters"))
				Expect(toolCall.ToolResultPreview).NotTo(ContainSubstring("synthetic-sensitive-audit-sentinel-2459"))
				Expect(toolCall.ToolResultPreview).NotTo(ContainSubstring("executionBundle"))
			}
			Expect(getWorkflowAuditFound).To(BeTrue(), "the persisted get_workflow tool-call event must use the minimized audit result")

			var actionsEvent, workflowsEvent *ogenclient.AuditEvent
			for i := range auditEvents {
				event := &auditEvents[i]
				if event.EventType != kaaudit.EventTypeActionsListed && event.EventType != kaaudit.EventTypeWorkflowsListed {
					continue
				}
				Expect(event.CorrelationID).To(Equal(remediationID))
				Expect(event.EventTimestamp).NotTo(BeZero())
				Expect(string(event.EventCategory)).To(Equal("workflow"))
				Expect(event.EventAction).To(Equal("discovery"))
				Expect(string(event.EventOutcome)).To(Equal("success"))
				Expect(event.ActorType.Value).To(Equal("service"))
				Expect(event.ActorID.Value).To(Equal("kubernaut-agent"))
				Expect(event.EventID.Set).To(BeTrue())
				if event.EventType == kaaudit.EventTypeActionsListed {
					Expect(actionsEvent).To(BeNil(), "the unique remediation should have one actions_listed event")
					actionsEvent = event
				} else {
					Expect(workflowsEvent).To(BeNil(), "the unique remediation should have one workflows_listed event")
					workflowsEvent = event
				}
			}
			Expect(actionsEvent).NotTo(BeNil())
			Expect(workflowsEvent).NotTo(BeNil())

			actionsPayload := discoveryAuditPayload2459(*actionsEvent)
			Expect(actionsEvent.EventData.IsWorkflowActionsListedAuditPayload()).To(BeTrue())
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
			Expect(json.Unmarshal(actionsPayload["query"], &actionQuery)).To(Succeed())
			Expect(actionQuery.TopK).To(Equal(10))
			Expect(actionQuery.Offset).To(Equal(0))
			Expect(actionQuery.Filters.Severity).To(Equal("critical"))
			// RCA re-enriches this OOM incident's remediation target to
			// apps/v1/Deployment. Discovery uses the resolved target GVK when
			// available, rather than the original Pod alert resource.
			Expect(actionQuery.Filters.Component).To(Equal("apps/v1/Deployment"))
			Expect(actionQuery.Filters.Environment).To(Equal("production"))
			Expect(actionQuery.Filters.Priority).To(Equal("P1"))
			var actionResults struct {
				Actions []struct {
					ActionType string `json:"action_type"`
				} `json:"actions"`
				Returned int `json:"returned"`
			}
			Expect(json.Unmarshal(actionsPayload["results"], &actionResults)).To(Succeed())
			Expect(actionResults.Returned).To(Equal(len(actionResults.Actions)))
			Expect(actionResults.Actions).NotTo(BeEmpty())
			hasIncreaseMemoryAction := false
			for _, action := range actionResults.Actions {
				hasIncreaseMemoryAction = hasIncreaseMemoryAction || action.ActionType == "IncreaseMemoryLimits"
			}
			Expect(hasIncreaseMemoryAction).To(BeTrue())

			workflowsPayload := discoveryAuditPayload2459(*workflowsEvent)
			Expect(workflowsEvent.EventData.IsWorkflowCandidatesListedAuditPayload()).To(BeTrue())
			var actionType string
			Expect(json.Unmarshal(workflowsPayload["action_type"], &actionType)).To(Succeed())
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
			Expect(json.Unmarshal(workflowsPayload["results"], &workflowResults)).To(Succeed())
			Expect(workflowResults.TotalFound).To(BeNumerically(">=", workflowResults.Returned))
			Expect(workflowResults.Returned).To(Equal(len(workflowResults.Workflows)))
			Expect(workflowResults.Workflows).NotTo(BeEmpty())
			selectedWasAudited := false
			for i, candidate := range workflowResults.Workflows {
				Expect(candidate.Rank).To(Equal(i + 1))
				Expect(candidate.FinalScore).To(Equal(0.5), "the unfiltered detected-label rank score is the cache baseline")
				Expect(candidate.Scoring.Confidence).To(Equal(candidate.FinalScore),
					"the legacy score alias must remain the cache ranking score")
				if candidate.WorkflowID == selectedWorkflow.WorkflowID {
					selectedWasAudited = true
					Expect(candidate.Title).To(Equal(selectedWorkflow.WorkflowName))
				}
			}
			Expect(selectedWasAudited).To(BeTrue(), "the selected workflow must be among the persisted candidates")
			workflowData, err := json.Marshal(workflowsEvent.EventData)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(workflowData)).NotTo(ContainSubstring("parameters"))
			Expect(string(workflowData)).NotTo(ContainSubstring("execution_bundle"))
			Expect(string(workflowData)).NotTo(ContainSubstring("do-not-audit-sentinel"))

			// CORRECTNESS: Confident recommendation (Mock LLM oomkilled scenario returns 0.95)
			Expect(incidentResp.Confidence).To(BeNumerically("~", 0.95, 0.10),
				"Confidence should be ~0.95 for OOMKilled scenario via three-step discovery")

			// BEHAVIOR: No human review needed for confident recommendation
			Expect(incidentResp.NeedsHumanReview).To(BeFalse(),
				"needsHumanReview must be false when three-step discovery finds a confident workflow")

			// CORRECTNESS: Analysis contains RCA
			Expect(incidentResp.Analysis).ToNot(BeEmpty(),
				"Analysis text should be populated from Mock LLM final response")

			logger.Info("✅ E2E-KA-017-001-001: Incident three-step discovery PASSED",
				"incident_id", incidentResp.IncidentID,
				"confidence", incidentResp.Confidence,
				"selected_workflow_set", incidentResp.SelectedWorkflow != nil)
		})

		It("E2E-KA-017-001-001b: CrashLoop incident also uses three-step discovery", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-017-001-001b (variant)
			// Business Outcome: Different signal type also works with three-step discovery.
			// BR: BR-KA-017-001

			// ========================================
			// ARRANGE
			// ========================================
			// CrashLoopBackOff triggers the "crashloop" Mock LLM scenario.
			// Three-step: list_available_actions → list_workflows(RestartDeployment) → get_workflow
			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: "test-rem-017-001b", Namespace: sharedNamespace},
				IncidentID:            "test-discovery-017-001b",
				RemediationID:         "test-rem-017-001b",
				SignalName:            "CrashLoopBackOff",
				Severity:              "high",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "staging",
				ResourceKind:          "Deployment",
				ResourceAPIVersion:    "apps/v1",
				ResourceName:          "worker",
				ErrorMessage:          "Container failing due to config error - testing three-step variant",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			// ========================================
			// ACT (#2190: AgentSession CRD flow)
			// ========================================
			incidentResp, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis should succeed for CrashLoop via three-step")

			// ========================================
			// ASSERT
			// ========================================
			Expect(incidentResp.SelectedWorkflow).ToNot(BeNil(),
				"selectedWorkflow must be present for CrashLoop via three-step discovery")
			Expect(string(incidentResp.SelectedWorkflow.Raw)).To(ContainSubstring(
				`"workflow_name":"crashloop-config-fix-v1"`),
				"selectedWorkflow must be the workflow returned by list_workflows")
			Expect(incidentResp.Confidence).To(BeNumerically("~", 0.95, 0.05),
				"Confidence should be ~0.95 for CrashLoop scenario")

			logger.Info("✅ E2E-KA-017-001-001b: CrashLoop three-step discovery PASSED",
				"incident_id", incidentResp.IncidentID,
				"confidence", incidentResp.Confidence)
		})
	})
})

func discoveryAuditPayload2459(event ogenclient.AuditEvent) map[string]json.RawMessage {
	encoded, err := json.Marshal(event.EventData)
	Expect(err).NotTo(HaveOccurred())
	var payload map[string]json.RawMessage
	Expect(json.Unmarshal(encoded, &payload)).To(Succeed())
	return payload
}
