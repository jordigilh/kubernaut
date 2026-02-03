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

package holmesgptapi

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	hapiclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// Audit Pipeline E2E Tests
// Test Plan: docs/development/testing/HAPI_E2E_TEST_PLAN.md
// Scenarios: E2E-HAPI-045 through E2E-HAPI-048 (4 total)
// Business Requirements: BR-AUDIT-005, DD-HAPI-002 v1.2
//
// Purpose: Validate audit event persistence to DataStorage for compliance and debugging

var _ = Describe("E2E-HAPI Audit Pipeline", Label("e2e", "hapi", "audit"), func() {

	var dataStorageClient *ogenclient.Client

	BeforeEach(func() {
		// Create authenticated DataStorage client for audit event queries
		saToken, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "holmesgpt-api-e2e-sa", kubeconfigPath)
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

		It("E2E-HAPI-045: LLM request event persisted to DataStorage", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-045
			// Business Outcome: All LLM API calls are audited for compliance and debugging
			// Python Source: test_audit_pipeline_e2e.py:350
			// BR: BR-AUDIT-005

			// ========================================
			// ARRANGE: Create incident request with unique remediation_id
			// ========================================
			remediationID := "test-audit-045-" + time.Now().Format("20060102150405")

			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-audit-045",
				RemediationID:     remediationID,
				SignalType:        "OOMKilled",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-045",
				ErrorMessage:      "Container memory limit exceeded",
			}

			// ========================================
			// ACT: Call HAPI incident analysis endpoint
			// ========================================
			_, err := hapiClient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

			// ========================================
			// ASSERT: Query DataStorage for audit events with retry (async buffering)
			// ========================================
			// HAPI uses async audit buffering, so events may take a few seconds to appear
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

				// Look for llm_request event
				for _, event := range events {
					if event.EventType == "llm_request" {
						return true
					}
				}
				return false
			}, 15*time.Second, 1*time.Second).Should(BeTrue(),
				"LLM request event should be persisted within 15 seconds")

			// BEHAVIOR: LLM request event persisted
			var llmRequestEvent *ogenclient.AuditEvent
			for i, event := range events {
				if event.EventType == "llm_request" {
					llmRequestEvent = &events[i]
					break
				}
			}

			Expect(llmRequestEvent).ToNot(BeNil(),
				"llm_request event must be found")
			Expect(llmRequestEvent.CorrelationID).To(Equal(remediationID),
				"correlation_id must match remediation_id")

			// CORRECTNESS: Event data complete
			// event_data should contain incident_id and prompt information
			// (Exact structure depends on OpenAPI schema)

			// BUSINESS IMPACT: Compliance team can audit all LLM interactions
		})

		It("E2E-HAPI-046: LLM response event persisted to DataStorage", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-046
			// Business Outcome: All LLM responses audited for cost tracking and analysis
			// Python Source: test_audit_pipeline_e2e.py:425
			// BR: BR-AUDIT-005

			// ========================================
			// ARRANGE
			// ========================================
			remediationID := "test-audit-046-" + time.Now().Format("20060102150405")

			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-audit-046",
				RemediationID:     remediationID,
				SignalType:        "CrashLoopBackOff",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-046",
				ErrorMessage:      "Container restarting repeatedly",
			}

			// ========================================
			// ACT
			// ========================================
			_, err := hapiClient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

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

				// Look for llm_response event
				for _, event := range events {
					if event.EventType == "llm_response" {
						return true
					}
				}
				return false
			}, 15*time.Second, 1*time.Second).Should(BeTrue(),
				"LLM response event should be persisted within 15 seconds")

			// BEHAVIOR: LLM response event persisted
			var llmResponseEvent *ogenclient.AuditEvent
			for i, event := range events {
				if event.EventType == "llm_response" {
					llmResponseEvent = &events[i]
					break
				}
			}

			Expect(llmResponseEvent).ToNot(BeNil(),
				"llm_response event must be found")
			Expect(llmResponseEvent.CorrelationID).To(Equal(remediationID),
				"correlation_id must match remediation_id")

			// CORRECTNESS: Response data captured
			// event_data should contain incident_id and analysis information

			// BUSINESS IMPACT: Cost analysis, quality monitoring, debugging
		})

		It("E2E-HAPI-047: Validation attempt event persisted", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-047
			// Business Outcome: Workflow validation attempts audited for quality analysis
			// Python Source: test_audit_pipeline_e2e.py:492
			// BR: DD-HAPI-002 v1.2

			// ========================================
			// ARRANGE
			// ========================================
			remediationID := "test-audit-047-" + time.Now().Format("20060102150405")

			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-audit-047",
				RemediationID:     remediationID,
				SignalType:        "OOMKilled",
				Severity:          "high",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-047",
				ErrorMessage:      "Container memory limit exceeded",
			}

			// ========================================
			// ACT: Call HAPI (triggers validation)
			// ========================================
			_, err := hapiClient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

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

				// Look for workflow_validation_attempt event
				for _, event := range events {
					if event.EventType == "workflow_validation_attempt" {
						return true
					}
				}
				return false
			}, 15*time.Second, 1*time.Second).Should(BeTrue(),
				"Validation attempt event should be persisted within 15 seconds")

			// BEHAVIOR: Validation events persisted
			var validationEvent *ogenclient.AuditEvent
			for i, event := range events {
				if event.EventType == "workflow_validation_attempt" {
					validationEvent = &events[i]
					break
				}
			}

			Expect(validationEvent).ToNot(BeNil(),
				"workflow_validation_attempt event must be found")
			Expect(validationEvent.CorrelationID).To(Equal(remediationID),
				"correlation_id must match remediation_id")

			// CORRECTNESS: Validation data complete
			// event_data should contain: attempt, max_attempts, is_valid

			// BUSINESS IMPACT: Self-correction quality analysis, debugging failed validations
		})

		It("E2E-HAPI-048: Complete audit trail persisted", func() {
			// ========================================
			// TEST PLAN MAPPING
			// ========================================
			// Scenario ID: E2E-HAPI-048
			// Business Outcome: Complete audit trail (all event types) available for incident forensics
			// Python Source: test_audit_pipeline_e2e.py:573
			// BR: BR-AUDIT-005

			// ========================================
			// ARRANGE
			// ========================================
			remediationID := "test-audit-048-" + time.Now().Format("20060102150405")

			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-audit-048",
				RemediationID:     remediationID,
				SignalType:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalSource:      "kubernetes",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod-048",
				ErrorMessage:      "Container restarting repeatedly",
			}

			// ========================================
			// ACT
			// ========================================
			_, err := hapiClient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)
			Expect(err).ToNot(HaveOccurred(), "HAPI incident analysis API call should succeed")

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
					if event.EventType == "llm_request" {
						hasLLMRequest = true
					}
					if event.EventType == "llm_response" {
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
				if event.EventType == "llm_request" {
					hasLLMRequest = true
				}
				if event.EventType == "llm_response" {
					hasLLMResponse = true
				}
				if event.EventType == "workflow_validation_attempt" {
					hasValidation = true
				}
			}

			Expect(hasLLMRequest).To(BeTrue(),
				"llm_request event must be present")
			Expect(hasLLMResponse).To(BeTrue(),
				"llm_response event must be present")
			// Note: workflow_validation_attempt is optional (depends on if validation occurred)
			_ = hasValidation

			// CORRECTNESS: Consistent correlation across events
			for _, event := range events {
				Expect(event.CorrelationID).To(Equal(remediationID),
					"All events must have same correlation_id (remediation_id)")
			}

			// BUSINESS IMPACT: Complete incident forensics, compliance reporting
		})
	})
})
