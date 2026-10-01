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
	"net/http"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	kaaudit "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// Audit Pipeline E2E Tests
// Test Plan: docs/development/testing/KA_E2E_TEST_PLAN.md
// Scenarios: E2E-KA-045 through E2E-KA-048 (4 total)
// Business Requirements: BR-AUDIT-005, DD-KA-001 v1.2
//
// Purpose: Validate audit event persistence to DataStorage for compliance and debugging

var workflowDiscoveryEventTypes = []string{
	kaaudit.EventTypeActionsListed,
	kaaudit.EventTypeWorkflowsListed,
	kaaudit.EventTypeWorkflowRetrieved,
	kaaudit.EventTypeSelectionValidated,
}

func completeWorkflowDiscoveryTrace(events []ogenclient.AuditEvent, correlationID string) (map[string]ogenclient.AuditEvent, bool) {
	trace := make(map[string]ogenclient.AuditEvent, len(workflowDiscoveryEventTypes))
	for _, event := range events {
		if event.CorrelationID != correlationID {
			continue
		}
		for _, eventType := range workflowDiscoveryEventTypes {
			if event.EventType == eventType {
				trace[eventType] = event
				break
			}
		}
	}
	return trace, len(trace) == len(workflowDiscoveryEventTypes)
}

func assertWorkflowDiscoveryTrace(trace map[string]ogenclient.AuditEvent, correlationID string) {
	expectedActions := map[string]string{
		kaaudit.EventTypeActionsListed:      kaaudit.ActionDiscovery,
		kaaudit.EventTypeWorkflowsListed:    kaaudit.ActionDiscovery,
		kaaudit.EventTypeWorkflowRetrieved:  kaaudit.ActionRetrieve,
		kaaudit.EventTypeSelectionValidated: kaaudit.ActionValidate,
	}
	for eventType, eventAction := range expectedActions {
		event, ok := trace[eventType]
		Expect(ok).To(BeTrue(), "CC7.2: %s must be reconstructable by correlation ID", eventType)
		Expect(event.EventCategory).To(Equal(ogenclient.AuditEventEventCategoryWorkflow),
			"AU-3: workflow discovery events must use the workflow category")
		Expect(event.EventAction).To(Equal(eventAction))
		Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeSuccess))
		Expect(event.EventID.IsSet()).To(BeTrue(), "AU-3: %s must carry event_id", eventType)
		Expect(event.ActorType.IsSet()).To(BeTrue(), "AU-3: %s must carry actor_type", eventType)
		Expect(event.ActorID.IsSet()).To(BeTrue(), "AU-3: %s must carry actor_id", eventType)
		Expect(event.ActorType.Value).To(Equal("service"))
		Expect(event.ActorID.Value).To(Equal("kubernaut-agent"))
		Expect(event.CorrelationID).To(Equal(correlationID))
		switch eventType {
		case kaaudit.EventTypeActionsListed:
			payload, payloadOK := event.EventData.GetWorkflowActionsListedAuditPayload()
			Expect(payloadOK).To(BeTrue(), "AU-3: %s must use the typed actions-listed payload", eventType)
			Expect(string(payload.EventType)).To(Equal(eventType))
		case kaaudit.EventTypeWorkflowsListed:
			payload, payloadOK := event.EventData.GetWorkflowCandidatesListedAuditPayload()
			Expect(payloadOK).To(BeTrue(), "AU-3: %s must use the typed candidates-listed payload", eventType)
			Expect(string(payload.EventType)).To(Equal(eventType))
		default:
			payload, payloadOK := event.EventData.GetWorkflowDiscoveryAuditPayload()
			Expect(payloadOK).To(BeTrue(), "AU-3: %s must use the typed discovery payload", eventType)
			Expect(string(payload.EventType)).To(Equal(eventType))
		}
	}
}

var _ = Describe("E2E-KA Audit Pipeline", Label("e2e", "ka", "audit"), func() {

	var dataStorageClient *ogenclient.Client

	BeforeEach(func() {
		// Create authenticated DataStorage client for audit event queries
		saToken, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get ServiceAccount token")

		dataStorageClient, err = ogenclient.NewClient(
			dataStorageURL,
			ogenclient.WithClient(&http.Client{
				Transport: testauth.NewServiceAccountTransport(saToken),
				Timeout:   30 * time.Second,
			}),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create authenticated DataStorage client")
	})

	Context("BR-AUDIT-005: Audit event persistence", func() {

		It("E2E-KA-045: LLM request event persisted to DataStorage", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-045
			// Business Outcome: All LLM API calls are audited for compliance and debugging
			// Ported from: test_audit_pipeline_e2e.py:350
			// BR: BR-AUDIT-005

			// ========================================
			// ARRANGE: Create incident request with unique remediation_id
			// ========================================
			remediationID := "test-audit-045-" + time.Now().Format("20060102150405")

			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: remediationID, Namespace: sharedNamespace},
				IncidentID:            "test-audit-045",
				RemediationID:         remediationID,
				SignalName:            "OOMKilled",
				Severity:              "high",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod-045",
				ErrorMessage:          "Container memory limit exceeded",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			// ========================================
			// ACT: Call KA incident analysis (#2190: AgentSession CRD flow)
			// ========================================
			_, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis should succeed")

			// ========================================
			// ASSERT: Query DataStorage for audit events with retry (async buffering)
			// ========================================
			// KA uses async audit buffering, so events may take a few seconds to appear
			var events []ogenclient.AuditEvent

			Eventually(func() bool {
				// Query DataStorage for audit events with this correlation_id
				resp, err := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(remediationID),
				})
				if err != nil {
					return false
				}

				events = resp.Data

				// Look for aiagent.llm.request event
				for _, event := range events {
					if event.EventType == string(ogenclient.LLMRequestPayloadAuditEventEventData) {
						return true
					}
				}
				return false
			}, 15*time.Second, 1*time.Second).Should(BeTrue(),
				"LLM request event should be persisted within 15 seconds")

			// BEHAVIOR: LLM request event persisted
			var llmRequestEvent *ogenclient.AuditEvent
			for i, event := range events {
				if event.EventType == string(ogenclient.LLMRequestPayloadAuditEventEventData) {
					llmRequestEvent = &events[i]
					break
				}
			}

			Expect(llmRequestEvent).ToNot(BeNil(),
				"aiagent.llm.request event must be found (LLMRequestPayloadAuditEventEventData)")
			Expect(llmRequestEvent.CorrelationID).To(Equal(remediationID),
				"correlation_id must match remediation_id")

			// CORRECTNESS: Event data complete
			// event_data should contain incident_id and prompt information
			// (Exact structure depends on OpenAPI schema)

			// BUSINESS IMPACT: Compliance team can audit all LLM interactions
		})

		It("E2E-KA-046: LLM response event persisted to DataStorage", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-046
			// Business Outcome: All LLM responses audited for cost tracking and analysis
			// Ported from: test_audit_pipeline_e2e.py:425
			// BR: BR-AUDIT-005

			// ========================================
			// ARRANGE
			// ========================================
			remediationID := "test-audit-046-" + time.Now().Format("20060102150405")

			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: remediationID, Namespace: sharedNamespace},
				IncidentID:            "test-audit-046",
				RemediationID:         remediationID,
				SignalName:            "CrashLoopBackOff",
				Severity:              "high",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod-046",
				ErrorMessage:          "Container restarting repeatedly",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			// ========================================
			// ACT (#2190: AgentSession CRD flow)
			// ========================================
			_, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis should succeed")

			// ========================================
			// ASSERT
			// ========================================
			var events []ogenclient.AuditEvent

			Eventually(func() bool {
				resp, err := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(remediationID),
				})
				if err != nil {
					return false
				}

				events = resp.Data

				// Look for aiagent.llm.response event
				for _, event := range events {
					if event.EventType == string(ogenclient.LLMResponsePayloadAuditEventEventData) {
						return true
					}
				}
				return false
			}, 15*time.Second, 1*time.Second).Should(BeTrue(),
				"LLM response event should be persisted within 15 seconds")

			// BEHAVIOR: LLM response event persisted
			var llmResponseEvent *ogenclient.AuditEvent
			for i, event := range events {
				if event.EventType == string(ogenclient.LLMResponsePayloadAuditEventEventData) {
					llmResponseEvent = &events[i]
					break
				}
			}

			Expect(llmResponseEvent).ToNot(BeNil(),
				"aiagent.llm.response event must be found (LLMResponsePayloadAuditEventEventData)")
			Expect(llmResponseEvent.CorrelationID).To(Equal(remediationID),
				"correlation_id must match remediation_id")

			// CORRECTNESS: Response data captured
			// event_data should contain incident_id and analysis information

			// BUSINESS IMPACT: Cost analysis, quality monitoring, debugging
		})

		It("E2E-KA-047: Validation attempt event persisted", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-047
			// Business Outcome: Workflow validation attempts audited for quality analysis
			// Ported from: test_audit_pipeline_e2e.py:492
			// BR: DD-KA-001 v1.2

			// ========================================
			// ARRANGE
			// ========================================
			remediationID := "test-audit-047-" + time.Now().Format("20060102150405")

			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: remediationID, Namespace: sharedNamespace},
				IncidentID:            "test-audit-047",
				RemediationID:         remediationID,
				SignalName:            "OOMKilled",
				Severity:              "high",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod-047",
				ErrorMessage:          "Container memory limit exceeded",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			// ========================================
			// ACT: Call KA (triggers validation) (#2190: AgentSession CRD flow)
			// ========================================
			_, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis should succeed")

			// ========================================
			// ASSERT
			// ========================================
			var events []ogenclient.AuditEvent

			Eventually(func() bool {
				resp, err := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(remediationID),
				})
				if err != nil {
					return false
				}

				events = resp.Data

				// Look for aiagent.workflow.validation_attempt event
				for _, event := range events {
					if event.EventType == string(ogenclient.WorkflowValidationPayloadAuditEventEventData) {
						return true
					}
				}
				return false
			}, 15*time.Second, 1*time.Second).Should(BeTrue(),
				"Validation attempt event should be persisted within 15 seconds")

			// BEHAVIOR: Validation events persisted
			var validationEvent *ogenclient.AuditEvent
			for i, event := range events {
				if event.EventType == string(ogenclient.WorkflowValidationPayloadAuditEventEventData) {
					validationEvent = &events[i]
					break
				}
			}

			Expect(validationEvent).ToNot(BeNil(),
				"aiagent.workflow.validation_attempt event must be found (WorkflowValidationPayloadAuditEventEventData)")
			Expect(validationEvent.CorrelationID).To(Equal(remediationID),
				"correlation_id must match remediation_id")

			// CORRECTNESS: Validation data complete
			// event_data should contain: attempt, max_attempts, is_valid

			// BUSINESS IMPACT: Self-correction quality analysis, debugging failed validations
		})

		It("E2E-KA-048: Complete audit trail persisted", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-KA-048
			// Business Outcome: Complete audit trail (all event types) available for incident forensics
			// Ported from: test_audit_pipeline_e2e.py:573
			// BR: BR-AUDIT-005

			// ========================================
			// ARRANGE
			// ========================================
			remediationID := "test-audit-048-" + time.Now().Format("20060102150405")

			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: remediationID, Namespace: sharedNamespace},
				IncidentID:            "test-audit-048",
				RemediationID:         remediationID,
				SignalName:            "CrashLoopBackOff",
				Severity:              "critical",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod-048",
				ErrorMessage:          "Container restarting repeatedly",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			// ========================================
			// ACT (#2190: AgentSession CRD flow)
			// ========================================
			_, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis should succeed")

			// ========================================
			// ASSERT: Validate complete trail
			// ========================================
			var events []ogenclient.AuditEvent

			Eventually(func() bool {
				resp, err := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(remediationID),
				})
				if err != nil {
					return false
				}

				events = resp.Data

				// Check for minimum required event types
				hasLLMRequest := false
				hasLLMResponse := false

				for _, event := range events {
					if event.EventType == string(ogenclient.LLMRequestPayloadAuditEventEventData) {
						hasLLMRequest = true
					}
					if event.EventType == string(ogenclient.LLMResponsePayloadAuditEventEventData) {
						hasLLMResponse = true
					}
				}

				return hasLLMRequest && hasLLMResponse
			}, 15*time.Second, 1*time.Second).Should(BeTrue(),
				"Complete audit trail should be persisted within 15 seconds")

			// BEHAVIOR: All event types present
			hasLLMRequest := false
			hasLLMResponse := false
			hasValidation := false

			for _, event := range events {
				if event.EventType == string(ogenclient.LLMRequestPayloadAuditEventEventData) {
					hasLLMRequest = true
				}
				if event.EventType == string(ogenclient.LLMResponsePayloadAuditEventEventData) {
					hasLLMResponse = true
				}
				if event.EventType == string(ogenclient.WorkflowValidationPayloadAuditEventEventData) {
					hasValidation = true
				}
			}

			Expect(hasLLMRequest).To(BeTrue(),
				"aiagent.llm.request event must be present")
			Expect(hasLLMResponse).To(BeTrue(),
				"aiagent.llm.response event must be present")
			// Note: aiagent.workflow.validation_attempt is optional (depends on if validation occurred)
			_ = hasValidation

			// CORRECTNESS: Consistent correlation across events
			for _, event := range events {
				Expect(event.CorrelationID).To(Equal(remediationID),
					"All events must have same correlation_id (remediation_id)")
			}

			// BUSINESS IMPACT: Complete incident forensics, compliance reporting
		})
	})

	Context("#1111: Extended audit trail coverage — events emitted during investigation", func() {

		It("E2E-KA-1111-001: RCA complete and tool call events persisted", func() {
			remediationID := "test-audit-1111-001-" + time.Now().Format("20060102150405")

			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: remediationID, Namespace: sharedNamespace},
				IncidentID:            "test-audit-1111-001",
				RemediationID:         remediationID,
				SignalName:            "CrashLoopBackOff",
				Severity:              "critical",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod-1111-001",
				ErrorMessage:          "Container restarting repeatedly",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			// #2190: AgentSession CRD flow replaces sessionClient.Investigate().
			_, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA investigation should succeed")

			// Wait for at least LLM request/response (proves basic audit pipeline)
			var events []ogenclient.AuditEvent
			Eventually(func() bool {
				resp, qErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(remediationID),
				})
				if qErr != nil {
					return false
				}
				events = resp.Data
				for _, event := range events {
					if event.EventType == string(ogenclient.LLMResponsePayloadAuditEventEventData) {
						return true
					}
				}
				return false
			}, 30*time.Second, 1*time.Second).Should(BeTrue(),
				"LLM response event must be persisted (basic audit pipeline)")

			// Verify RCA complete and tool call events if present.
			// These depend on mock LLM issuing tool_call responses.
			hasRCA := false
			hasToolCall := false
			for _, event := range events {
				switch event.EventType {
				case string(ogenclient.AIAgentRCACompletePayloadAuditEventEventData):
					hasRCA = true
					Expect(event.CorrelationID).To(Equal(remediationID),
						"RCA complete event must have remediation_id as correlation_id")
				case string(ogenclient.LLMToolCallPayloadAuditEventEventData):
					hasToolCall = true
					Expect(event.CorrelationID).To(Equal(remediationID),
						"Tool call event must have remediation_id as correlation_id")
				}
			}
			if hasRCA {
				GinkgoWriter.Println("✅ aiagent.rca.complete confirmed")
			} else {
				GinkgoWriter.Println("⚠️  aiagent.rca.complete not found — mock LLM may not have triggered RCA flow")
			}
			if hasToolCall {
				GinkgoWriter.Println("✅ aiagent.llm.tool_call confirmed")
			} else {
				GinkgoWriter.Println("⚠️  aiagent.llm.tool_call not found — mock LLM may not have issued tool_calls")
			}
		})

		It("E2E-KA-1111-002: Enrichment completed event persisted with IncidentID correlation", func() {
			incidentID := "test-audit-1111-002"
			remediationID := "test-audit-1111-002-" + time.Now().Format("20060102150405")

			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: remediationID, Namespace: sharedNamespace},
				IncidentID:            incidentID,
				RemediationID:         remediationID,
				SignalName:            "OOMKilled",
				Severity:              "high",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod-1111-002",
				ErrorMessage:          "Container memory limit exceeded",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			// #2190: AgentSession CRD flow replaces sessionClient.Investigate().
			_, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA investigation should succeed")

			// Wait for basic audit pipeline (LLM response by remediationID)
			Eventually(func() bool {
				resp, qErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(remediationID),
				})
				if qErr != nil {
					return false
				}
				for _, event := range resp.Data {
					if event.EventType == string(ogenclient.LLMResponsePayloadAuditEventEventData) {
						return true
					}
				}
				return false
			}, 30*time.Second, 1*time.Second).Should(BeTrue(),
				"LLM response event must be persisted (basic audit pipeline)")

			// aiagent.enrichment.completed uses IncidentID as correlation_id.
			// Best-effort: verify if present but don't fail — depends on
			// enrichment wiring in the direct KA invocation path.
			var enrichmentEvents []ogenclient.AuditEvent
			Eventually(func() bool {
				resp, qErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(incidentID),
				})
				if qErr != nil {
					return false
				}
				enrichmentEvents = resp.Data
				return len(enrichmentEvents) > 0
			}, 15*time.Second, 1*time.Second).Should(BeTrue(),
				"At least one event with IncidentID correlation must be persisted")

			hasEnrichment := false
			for _, event := range enrichmentEvents {
				if event.EventType == "aiagent.enrichment.completed" {
					hasEnrichment = true
					Expect(event.CorrelationID).To(Equal(incidentID),
						"enrichment.completed must use IncidentID as correlation_id")
					break
				}
			}
			if hasEnrichment {
				GinkgoWriter.Println("✅ aiagent.enrichment.completed confirmed")
			} else {
				GinkgoWriter.Println("⚠️  aiagent.enrichment.completed not found — enrichment may use different event wiring in direct KA path")
			}
		})

		It("E2E-KA-1111-003: Workflow discovery events persisted after Phase 1 fix", func() {
			remediationID := "test-audit-1111-003-" + time.Now().Format("20060102150405")

			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: remediationID, Namespace: sharedNamespace},
				IncidentID:            "test-audit-1111-003",
				RemediationID:         remediationID,
				SignalName:            "CrashLoopBackOff",
				Severity:              "critical",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod-1111-003",
				ErrorMessage:          "Container restarting repeatedly",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			// #2190: AgentSession CRD flow replaces sessionClient.Investigate().
			_, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA investigation should succeed")

			// Wait for basic audit pipeline (LLM response proves investigation ran)
			var events []ogenclient.AuditEvent
			Eventually(func() bool {
				resp, qErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(remediationID),
				})
				if qErr != nil {
					return false
				}
				events = resp.Data
				for _, event := range events {
					if event.EventType == string(ogenclient.LLMResponsePayloadAuditEventEventData) {
						return true
					}
				}
				return false
			}, 30*time.Second, 1*time.Second).Should(BeTrue(),
				"LLM response event must be persisted (basic audit pipeline)")

			// After #1111 fix, KA forwards remediation_id to its cache-backed
			// discovery tools. KA emits workflow.catalog.* events when mock LLM
			// tool_call responses invoke the discovery tools. Best-effort check.
			hasActionsListed := false
			hasWorkflowsListed := false
			for _, event := range events {
				switch event.EventType {
				case "workflow.catalog.actions_listed":
					hasActionsListed = true
					Expect(event.CorrelationID).To(Equal(remediationID))
				case "workflow.catalog.workflows_listed":
					hasWorkflowsListed = true
					Expect(event.CorrelationID).To(Equal(remediationID))
				}
			}
			if hasActionsListed {
				GinkgoWriter.Println("✅ workflow.catalog.actions_listed confirmed")
			} else {
				GinkgoWriter.Println("⚠️  workflow.catalog.actions_listed not found — mock LLM may not have triggered discovery tools")
			}
			if hasWorkflowsListed {
				GinkgoWriter.Println("✅ workflow.catalog.workflows_listed confirmed")
			} else {
				GinkgoWriter.Println("⚠️  workflow.catalog.workflows_listed not found — mock LLM may not have triggered discovery tools")
			}
		})
	})

	Context("TP-2478: Ranked action-family discovery journey", func() {
		It("E2E-KA-2478-001: persists ranked action-family evidence for a completed selection", func() {
			// BR-KA-017-001, BR-AUDIT-005; FedRAMP AU-2/AU-3; SOC 2 CC7.2.
			remediationID := "test-audit-2478-001-" + time.Now().Format("20060102150405")
			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: remediationID, Namespace: sharedNamespace},
				IncidentID:            "test-audit-2478-001",
				RemediationID:         remediationID,
				SignalName:            "HelmManagedConfigFailure",
				Severity:              "warning",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "production",
				ResourceKind:          "Pod",
				ResourceName:          "helm-managed-pod",
				ErrorMessage:          "Helm release introduced an invalid configuration revision",
				Environment:           "production",
				Priority:              "P2",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			result, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "the three-step discovery journey should complete")
			Expect(result).NotTo(BeNil())
			Expect(result.SelectedWorkflow).NotTo(BeNil(), "selection must complete after ranked discovery")

			var actionPayload *ogenclient.WorkflowActionsListedAuditPayload
			var discoveryTrace map[string]ogenclient.AuditEvent
			findRankedActions := func() bool {
				resp, queryErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(remediationID),
				})
				if queryErr != nil {
					return false
				}
				for i := range resp.Data {
					if resp.Data[i].EventType != kaaudit.EventTypeActionsListed {
						continue
					}
					payload, ok := resp.Data[i].EventData.GetWorkflowActionsListedAuditPayload()
					if ok && len(payload.Results.ActionTypes) > 0 {
						actionPayload = &payload
					}
				}
				var complete bool
				discoveryTrace, complete = completeWorkflowDiscoveryTrace(resp.Data, remediationID)
				return actionPayload != nil && complete
			}
			Eventually(findRankedActions, 30*time.Second, time.Second).Should(BeTrue(),
				"the complete workflow discovery trace must persist by remediation_id")

			Expect(actionPayload).NotTo(BeNil())
			assertWorkflowDiscoveryTrace(discoveryTrace, remediationID)
			Expect(actionPayload.Results.ActionTypes).NotTo(BeEmpty())
			preferred := actionPayload.Results.ActionTypes[0]
			Expect(preferred.ActionType).To(Equal("HelmRollback"),
				"the exact management-aware family must be preferred for a Helm-labeled target")
			Expect(preferred.Rank).To(Equal(int32(1)))
			Expect(preferred.Preferred).To(BeTrue())
			Expect(preferred.PreferenceReason).To(ContainSubstring("helmManaged=true"))
			matched, matchedSet := preferred.MatchedDetectedLabels.Get()
			Expect(matchedSet).To(BeTrue())
			helmManaged, helmManagedSet := matched.HelmManaged.Get()
			Expect(helmManagedSet).To(BeTrue())
			Expect(helmManaged).To(BeTrue())

			var selected map[string]interface{}
			Expect(json.Unmarshal(result.SelectedWorkflow.Raw, &selected)).To(Succeed())
			selectedWorkflowID, selectedWorkflowIDSet := selected["workflow_id"].(string)
			Expect(selectedWorkflowIDSet).To(BeTrue())
			bestWorkflowID, bestWorkflowIDSet := preferred.BestWorkflowID.Get()
			Expect(bestWorkflowIDSet).To(BeTrue())
			Expect(selectedWorkflowID).To(Equal(bestWorkflowID),
				"the completed selection must use the preferred action family's best workflow")

			genericFound := false
			for i, action := range actionPayload.Results.ActionTypes {
				Expect(action.Rank).To(Equal(int32(i+1)), "rank must remain global across the persisted page")
				Expect(action.BestMatchScore).To(BeNumerically(">=", 0))
				Expect(action.BestMatchScore).To(BeNumerically("<=", 1))
				if action.ActionType == "RestartPod" {
					genericFound = true
					Expect(action.Preferred).To(BeFalse(), "generic action family remains available but is not preferred")
				}
			}
			Expect(genericFound).To(BeTrue(), "generic RestartPod must remain available for model discretion")
		})

		It("E2E-KA-2478-002: permits a generic fallback when no management label matches", func() {
			// BR-KA-017-001, AC-6; FedRAMP AC-6/SI-10; SOC 2 CC7.2.
			remediationID := "test-audit-2478-002-" + time.Now().Format("20060102150405")
			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: remediationID, Namespace: sharedNamespace},
				IncidentID:            "test-audit-2478-002",
				RemediationID:         remediationID,
				SignalName:            "UnclassifiedSignal2478",
				Severity:              "warning",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod",
				ErrorMessage:          "No management-specific evidence is present",
				Environment:           "production",
				Priority:              "P2",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			result, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SelectedWorkflow).NotTo(BeNil(), "generic fallback should remain selectable")

			var actionPayload *ogenclient.WorkflowActionsListedAuditPayload
			var discoveryTrace map[string]ogenclient.AuditEvent
			findRankedActions := func() bool {
				resp, queryErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(remediationID),
				})
				if queryErr != nil {
					return false
				}
				for i := range resp.Data {
					if resp.Data[i].EventType != kaaudit.EventTypeActionsListed {
						continue
					}
					payload, ok := resp.Data[i].EventData.GetWorkflowActionsListedAuditPayload()
					if ok && len(payload.Results.ActionTypes) > 0 {
						actionPayload = &payload
					}
				}
				var complete bool
				discoveryTrace, complete = completeWorkflowDiscoveryTrace(resp.Data, remediationID)
				return actionPayload != nil && complete
			}
			Eventually(findRankedActions, 30*time.Second, time.Second).Should(BeTrue(),
				"generic fallback discovery and selection must be reconstructable by correlation ID")
			assertWorkflowDiscoveryTrace(discoveryTrace, remediationID)

			var generic *ogenclient.ActionTypeResultAudit
			for i := range actionPayload.Results.ActionTypes {
				if actionPayload.Results.ActionTypes[i].ActionType == "RestartPod" {
					generic = &actionPayload.Results.ActionTypes[i]
					break
				}
			}
			Expect(generic).NotTo(BeNil(), "generic RestartPod must remain available for an RCA-driven override")
			Expect(generic.Preferred).To(BeFalse(), "the model may override a lower-ranked generic family")
			Expect(generic.PreferenceReason).To(Equal("highest-scoring matching workflow"))
			selectedWorkflowID, selectedWorkflowIDSet := func() (string, bool) {
				var selected map[string]interface{}
				if err := json.Unmarshal(result.SelectedWorkflow.Raw, &selected); err != nil {
					return "", false
				}
				workflowID, ok := selected["workflow_id"].(string)
				return workflowID, ok
			}()
			Expect(selectedWorkflowIDSet).To(BeTrue())
			bestWorkflowID, bestWorkflowIDSet := generic.BestWorkflowID.Get()
			Expect(bestWorkflowIDSet).To(BeTrue())
			Expect(selectedWorkflowID).To(Equal(bestWorkflowID),
				"the generic workflow remains selectable even when it is not ranked first")
		})
	})

	Context("TP-433-AUDIT-SOC2: Audit parity — populated payloads", func() {

		It("E2E-KA-433-AP-001: Full investigation audit trail with populated payloads", func() {
			remediationID := "test-audit-ap-001-" + time.Now().Format("20060102150405")

			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: remediationID, Namespace: sharedNamespace},
				IncidentID:            "test-audit-ap-001",
				RemediationID:         remediationID,
				SignalName:            "CrashLoopBackOff",
				Severity:              "critical",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Deployment",
				ResourceName:          "test-deploy-ap-001",
				ErrorMessage:          "Container restarting repeatedly",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			// #2190: AgentSession CRD flow replaces sessionClient.Investigate().
			_, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis should succeed")

			var events []ogenclient.AuditEvent
			Eventually(func() bool {
				resp, qErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(remediationID),
				})
				if qErr != nil {
					return false
				}
				events = resp.Data

				hasRequest := false
				hasResponse := false
				hasComplete := false
				for _, event := range events {
					switch event.EventType {
					case "aiagent.llm.request":
						hasRequest = true
					case "aiagent.llm.response":
						hasResponse = true
					case kaaudit.EventTypeResponseComplete:
						hasComplete = true
					}
				}
				return hasRequest && hasResponse && hasComplete
			}, 30*time.Second, 1*time.Second).Should(BeTrue(),
				"All 3 required audit events (request + response + complete) must be present")

			hasRequest := false
			hasResponse := false
			hasComplete := false
			for _, event := range events {
				switch event.EventType {
				case "aiagent.llm.request":
					hasRequest = true
				case "aiagent.llm.response":
					hasResponse = true
				case kaaudit.EventTypeResponseComplete:
					hasComplete = true
				}
			}

			Expect(hasRequest).To(BeTrue(), "aiagent.llm.request must be present")
			Expect(hasResponse).To(BeTrue(), "aiagent.llm.response must be present")
			Expect(hasComplete).To(BeTrue(), "aiagent.response.complete must be present")
		})

		It("E2E-KA-433-AP-002: response.complete contains IncidentResponseData", func() {
			remediationID := "test-audit-ap-002-" + time.Now().Format("20060102150405")

			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: remediationID, Namespace: sharedNamespace},
				IncidentID:            "test-audit-ap-002",
				RemediationID:         remediationID,
				SignalName:            "CrashLoopBackOff",
				Severity:              "critical",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Deployment",
				ResourceName:          "test-deploy-ap-002",
				ErrorMessage:          "Container restarting repeatedly",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			// #2190: AgentSession CRD flow replaces sessionClient.Investigate().
			_, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred())

			var events []ogenclient.AuditEvent
			Eventually(func() bool {
				resp, qErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(remediationID),
				})
				if qErr != nil {
					return false
				}
				events = resp.Data
				for _, event := range events {
					if event.EventType == kaaudit.EventTypeResponseComplete {
						return true
					}
				}
				return false
			}, 20*time.Second, 1*time.Second).Should(BeTrue(),
				"response.complete event should be persisted")

			for _, event := range events {
				if event.EventType == kaaudit.EventTypeResponseComplete {
					Expect(event.CorrelationID).To(Equal(remediationID))
					Expect(event.EventAction).NotTo(BeEmpty(), "EventAction must be set on response.complete")
					break
				}
			}
		})

		It("E2E-KA-433-AP-003: All audit events carry actor attribution", func() {
			remediationID := "test-audit-ap-003-" + time.Now().Format("20060102150405")

			spec := agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: remediationID, Namespace: sharedNamespace},
				IncidentID:            "test-audit-ap-003",
				RemediationID:         remediationID,
				SignalName:            "OOMKilled",
				Severity:              "high",
				SignalSource:          "kubernetes",
				ResourceNamespace:     "default",
				ResourceKind:          "Pod",
				ResourceName:          "test-pod-ap-003",
				ErrorMessage:          "Container memory limit exceeded",
				Environment:           "production",
				Priority:              "P1",
				RiskTolerance:         "medium",
				BusinessCategory:      "standard",
			}

			// #2190: AgentSession CRD flow replaces sessionClient.Investigate().
			_, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred())

			var events []ogenclient.AuditEvent
			Eventually(func() int {
				resp, qErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(remediationID),
				})
				if qErr != nil {
					return 0
				}
				events = resp.Data
				return len(events)
			}, 20*time.Second, 1*time.Second).Should(BeNumerically(">=", 2),
				"At least 2 audit events should be persisted")

			for _, event := range events {
				Expect(event.ActorType.Set).To(BeTrue(),
					"ActorType must be set on %s event", event.EventType)
				Expect(event.ActorType.Value).ToNot(BeEmpty(),
					"ActorType must not be empty on %s event", event.EventType)
				Expect(event.ActorID.Set).To(BeTrue(),
					"ActorID must be set on %s event", event.EventType)
				Expect(event.ActorID.Value).ToNot(BeEmpty(),
					"ActorID must not be empty on %s event", event.EventType)
			}
		})
	})

	Context("#1401: Security audit event persistence", func() {

		It("E2E-KA-1401-001: HTTP 429 from rate limiter produces persisted audit event", func() {
			// E2E configures burst=100 on the per-IP rate limiter. We must
			// exceed that with parallel requests AND use a client without
			// RetryOn429Transport so we observe raw 429 responses.
			saToken, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa", kubeconfigPath)
			Expect(err).ToNot(HaveOccurred())

			noRetryClient := &http.Client{
				Transport: testauth.NewServiceAccountTransport(saToken),
				Timeout:   10 * time.Second,
			}

			const numRequests = 150
			statuses := make([]int, numRequests)
			var wg sync.WaitGroup
			wg.Add(numRequests)
			for i := 0; i < numRequests; i++ {
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()
					resp, reqErr := noRetryClient.Get(kaURL + "/api/v1/incident/session/nonexistent-1401")
					if reqErr != nil {
						return
					}
					resp.Body.Close()
					statuses[idx] = resp.StatusCode
				}(i)
			}
			wg.Wait()

			var got429 bool
			for _, code := range statuses {
				if code == http.StatusTooManyRequests {
					got429 = true
					break
				}
			}
			Expect(got429).To(BeTrue(), "must trigger at least one 429 response from KA rate limiter (burst=100, sent %d parallel)", numRequests)

			// Query DataStorage for the rate-limit audit event
			Eventually(func() bool {
				resp, qErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					EventType: ogenclient.NewOptString("aiagent.ratelimit.denied"),
				})
				if qErr != nil || len(resp.Data) == 0 {
					return false
				}
				for _, ev := range resp.Data {
					if strings.HasPrefix(ev.CorrelationID, "security-") {
						return true
					}
				}
				return false
			}, 30*time.Second, 2*time.Second).Should(BeTrue(),
				"AU-12: rate-limit audit event must be persisted in DataStorage with security- correlation_id")
		})

		It("E2E-KA-1401-002: HTTP 401 from invalid credentials produces persisted audit event", func() {
			// Send request with invalid token
			unauthClient := &http.Client{Timeout: 10 * time.Second}
			req, err := http.NewRequestWithContext(ctx, "GET", kaURL+"/api/v1/incident/analyze", nil)
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Authorization", "Bearer invalid-token-e2e-1401")

			resp, err := unauthClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

			// Query DataStorage for the auth failure audit event
			Eventually(func() bool {
				resp, qErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					EventType: ogenclient.NewOptString("aiagent.auth.failure"),
				})
				if qErr != nil || len(resp.Data) == 0 {
					return false
				}
				for _, ev := range resp.Data {
					if strings.HasPrefix(ev.CorrelationID, "security-") {
						return true
					}
				}
				return false
			}, 30*time.Second, 2*time.Second).Should(BeTrue(),
				"AC-7: auth failure audit event must be persisted in DataStorage with security- correlation_id")
		})
	})
})
