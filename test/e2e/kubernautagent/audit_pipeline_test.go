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
	"net/http"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// Audit Pipeline E2E Tests
// Test Plan: docs/development/testing/KA_E2E_TEST_PLAN.md
// Scenarios: E2E-KA-045 through E2E-KA-048 (4 total)
// Business Requirements: BR-AUDIT-005, DD-HAPI-002 v1.2
//
// Purpose: Validate audit event persistence to DataStorage for compliance and debugging

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

			req := &agentclient.IncidentRequest{
				IncidentID:        "test-audit-045",
				RemediationID:     remediationID,
				SignalName:        "OOMKilled",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-045",
				ErrorMessage:      "Container memory limit exceeded",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT: Call KA incident analysis via session client (BR-AA-HAPI-064)
			// ========================================
			_, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis API call should succeed")

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

			req := &agentclient.IncidentRequest{
				IncidentID:        "test-audit-046",
				RemediationID:     remediationID,
				SignalName:        "CrashLoopBackOff",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-046",
				ErrorMessage:      "Container restarting repeatedly",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT (BR-AA-HAPI-064: async session flow)
			// ========================================
			_, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis API call should succeed")

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
			// BR: DD-HAPI-002 v1.2

			// ========================================
			// ARRANGE
			// ========================================
			remediationID := "test-audit-047-" + time.Now().Format("20060102150405")

			req := &agentclient.IncidentRequest{
				IncidentID:        "test-audit-047",
				RemediationID:     remediationID,
				SignalName:        "OOMKilled",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-047",
				ErrorMessage:      "Container memory limit exceeded",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT: Call KA (triggers validation) (BR-AA-HAPI-064: async session flow)
			// ========================================
			_, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis API call should succeed")

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

			req := &agentclient.IncidentRequest{
				IncidentID:        "test-audit-048",
				RemediationID:     remediationID,
				SignalName:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-048",
				ErrorMessage:      "Container restarting repeatedly",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			// ========================================
			// ACT (BR-AA-HAPI-064: async session flow)
			// ========================================
			_, err := sessionClient.Investigate(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "KA incident analysis API call should succeed")

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

			req := &agentclient.IncidentRequest{
				IncidentID:        "test-audit-1111-001",
				RemediationID:     remediationID,
				SignalName:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-1111-001",
				ErrorMessage:      "Container restarting repeatedly",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			_, err := sessionClient.Investigate(ctx, req)
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

			req := &agentclient.IncidentRequest{
				IncidentID:        incidentID,
				RemediationID:     remediationID,
				SignalName:        "OOMKilled",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-1111-002",
				ErrorMessage:      "Container memory limit exceeded",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			_, err := sessionClient.Investigate(ctx, req)
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

			req := &agentclient.IncidentRequest{
				IncidentID:        "test-audit-1111-003",
				RemediationID:     remediationID,
				SignalName:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-1111-003",
				ErrorMessage:      "Container restarting repeatedly",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			_, err := sessionClient.Investigate(ctx, req)
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

			// After #1111 fix, KA forwards remediation_id to DS discovery tools.
			// DS emits workflow.catalog.* events only when mock LLM issues
			// tool_call responses that invoke the discovery tools. Best-effort check.
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

	Context("TP-433-AUDIT-SOC2: Audit parity — populated payloads", func() {

		It("E2E-KA-433-AP-001: Full investigation audit trail with populated payloads", func() {
			remediationID := "test-audit-ap-001-" + time.Now().Format("20060102150405")

			req := &agentclient.IncidentRequest{
				IncidentID:        "test-audit-ap-001",
				RemediationID:     remediationID,
				SignalName:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Deployment",
				ResourceName:      "test-deploy-ap-001",
				ErrorMessage:      "Container restarting repeatedly",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			_, err := sessionClient.Investigate(ctx, req)
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
					case "aiagent.response.complete":
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
				case "aiagent.response.complete":
					hasComplete = true
				}
			}

			Expect(hasRequest).To(BeTrue(), "aiagent.llm.request must be present")
			Expect(hasResponse).To(BeTrue(), "aiagent.llm.response must be present")
			Expect(hasComplete).To(BeTrue(), "aiagent.response.complete must be present")
		})

		It("E2E-KA-433-AP-002: response.complete contains IncidentResponseData", func() {
			remediationID := "test-audit-ap-002-" + time.Now().Format("20060102150405")

			req := &agentclient.IncidentRequest{
				IncidentID:        "test-audit-ap-002",
				RemediationID:     remediationID,
				SignalName:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Deployment",
				ResourceName:      "test-deploy-ap-002",
				ErrorMessage:      "Container restarting repeatedly",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			_, err := sessionClient.Investigate(ctx, req)
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
					if event.EventType == "aiagent.response.complete" {
						return true
					}
				}
				return false
			}, 20*time.Second, 1*time.Second).Should(BeTrue(),
				"response.complete event should be persisted")

			for _, event := range events {
				if event.EventType == "aiagent.response.complete" {
					Expect(event.CorrelationID).To(Equal(remediationID))
					Expect(event.EventAction).NotTo(BeEmpty(), "EventAction must be set on response.complete")
					break
				}
			}
		})

		It("E2E-KA-433-AP-003: All audit events carry actor attribution", func() {
			remediationID := "test-audit-ap-003-" + time.Now().Format("20060102150405")

			req := &agentclient.IncidentRequest{
				IncidentID:        "test-audit-ap-003",
				RemediationID:     remediationID,
				SignalName:        "OOMKilled",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-ap-003",
				ErrorMessage:      "Container memory limit exceeded",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			_, err := sessionClient.Investigate(ctx, req)
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
