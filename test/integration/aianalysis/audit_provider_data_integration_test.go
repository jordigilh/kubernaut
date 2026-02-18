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

// Package aianalysis contains integration tests for hybrid provider data audit capture.
//
// Business Requirements:
// - BR-AUDIT-005 v2.0 (Gap #4 - AI Provider Data)
// - DD-AUDIT-005: Hybrid Provider Data Capture
//
// Test Strategy:
// This test validates the HYBRID audit approach where BOTH HolmesAPI and AI Analysis
// emit audit events for defense-in-depth SOC2 compliance:
//
// 1. HolmesAPI emits: aiagent.response.complete (provider perspective, full response)
// 2. AI Analysis emits: aianalysis.analysis.completed (consumer perspective, summary + business context)
//
// Infrastructure:
// - PostgreSQL (port 15438): Persistence
// - Redis (port 16384): Caching
// - Data Storage (port 18095): Audit trail
// - HolmesAPI (port 18120): REAL service with MOCK_LLM_MODE=true
// - AIAnalysis Controller: Real controller with real audit client
//
// Test Pattern:
// - Create AIAnalysis CRD → Controller reconciles → HAPI is called → Both services emit audit events
// - Query Data Storage API for BOTH event types
// - Validate complete audit trail for SOC2 compliance
package aianalysis

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	aianalysisaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/validators"
)

// ========================================
// HYBRID PROVIDER DATA AUDIT TESTS
// BR-AUDIT-005 Gap #4, DD-AUDIT-005
// ========================================
//
// These tests validate the hybrid audit approach where BOTH HolmesAPI and AI Analysis
// emit audit events to provide defense-in-depth auditing for SOC2 compliance.
//
// Execution: Serial (for reliability, follows audit_flow_integration_test.go pattern)
// Infrastructure: Uses existing AIAnalysis integration test infrastructure (auto-started)
//
// ========================================
var _ = Describe("BR-AUDIT-005 Gap #4: Hybrid Provider Data Capture", Label("integration", "audit", "hybrid", "soc2"), func() {
	var (
		ctx            context.Context
		namespace      string
		datastorageURL string
		dsClient       *ogenclient.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"

		// Data Storage URL for audit event queries
		datastorageURL = "http://127.0.0.1:18095" // AIAnalysis integration test DS port (DD-TEST-001)

		// CI FIX: Ensure DataStorage is reachable before test starts
		// Rationale: CI environment may have resource contention causing DataStorage to be temporarily unavailable
		// even though SynchronizedBeforeSuite health check passed. Add defensive health check.
		By("Verifying DataStorage connectivity before test")
		Eventually(func() error {
		healthURL := datastorageURL + "/health"
		resp, err := http.Get(healthURL)
		if err != nil {
			return fmt.Errorf("health check failed: %w", err)
		}
		defer func() { _ = resp.Body.Close() }() // Explicitly ignore - test cleanup
		if resp.StatusCode != 200 {
			return fmt.Errorf("health check returned status %d", resp.StatusCode)
		}
		return nil
		}, 30*time.Second, 2*time.Second).Should(Succeed(),
			"DataStorage must be healthy before test starts (CI resource contention mitigation)")
		GinkgoWriter.Println("✅ DataStorage health check passed")

		// DD-AUTH-014: Use authenticated OpenAPI client from suite setup
		// FIX: Creating unauthenticated client here caused HTTP 401 errors when querying audit events
		// dsClients is created in SynchronizedBeforeSuite with ServiceAccount token
		// Pattern follows notification/controller_audit_emission_test.go:70
		dsClient = dsClients.OpenAPIClient
	})

	// ========================================
	// SHARED HELPER FUNCTIONS (DD-TESTING-001 Compliance)
	// ========================================

	// queryAuditEvents queries Data Storage for audit events using OpenAPI client (DD-API-001).
	// Returns: Array of audit events (OpenAPI-generated types)
	queryAuditEvents := func(correlationID string, eventType *string) ([]ogenclient.AuditEvent, error) {
		limit := 100
		params := ogenclient.QueryAuditEventsParams{
			CorrelationID: ogenclient.NewOptString(correlationID),
			Limit:         ogenclient.NewOptInt(limit),
		}
		if eventType != nil {
			params.EventType = ogenclient.NewOptString(*eventType)
		}

		resp, err := dsClient.QueryAuditEvents(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("failed to query DataStorage: %w", err)
		}
		if resp.Data == nil {
			return []ogenclient.AuditEvent{}, nil
		}
		return resp.Data, nil
	}

	// waitForAuditEvents polls Data Storage until exact expected count appears (DD-TESTING-001 §256-300).
	// MANDATORY: Uses Equal() for deterministic count validation, not BeNumerically(">=")
	// NT Pattern: Flushes audit buffer on EACH retry to ensure events are written to DataStorage
	// CI FIX: Increased timeout from 60s to 90s for CI environment resource contention
	waitForAuditEvents := func(correlationID string, eventType string, expectedCount int) []ogenclient.AuditEvent {
		var events []ogenclient.AuditEvent
		Eventually(func() int {
			// NT Pattern: Flush on each retry to catch events buffered since last check
			_ = auditStore.Flush(ctx)

			var err error
			events, err = queryAuditEvents(correlationID, &eventType)
			if err != nil {
				GinkgoWriter.Printf("⏳ Audit query error: %v\n", err)
				return 0
			}
			return len(events)
		}, 90*time.Second, 500*time.Millisecond).Should(Equal(expectedCount),
			fmt.Sprintf("DD-TESTING-001 violation: Should have EXACTLY %d %s events (controller idempotency)", expectedCount, eventType))
		return events
	}

	// countEventsByType counts occurrences of each event type (DD-TESTING-001 helper pattern).
	// Returns: map[eventType]count for deterministic validation
	countEventsByType := func(events []ogenclient.AuditEvent) map[string]int {
		counts := make(map[string]int)
		for _, event := range events {
			counts[event.EventType]++
		}
		return counts
	}

	// ========================================
	// TEST 1: Hybrid Capture Validation
	// Validates that BOTH HAPI and AA emit audit events
	// ========================================
	Context("Hybrid Audit Event Emission", func() {
		It("should capture Holmes response in BOTH HAPI and AA audit events", func() {
			// ========================================
			// TEST OBJECTIVE:
			// Verify that BOTH services emit audit events with correct structure:
			// 1. HolmesAPI emits aiagent.response.complete (provider perspective)
			// 2. AI Analysis emits aianalysis.analysis.completed (consumer perspective)
			// 3. Both events share the same correlation_id
			// 4. Both events contain appropriate data for their perspective
			//
			// BUG FIX (Jan 15, 2026): Made RemediationRequestRef.Name unique per test run
			// to prevent cross-test audit event pollution in shared DataStorage database.
			// Previously hard-coded "test-rr" caused query to retrieve events from ALL
			// test runs, leading to false failures (expected 1, got 2+).
			// ========================================

			By("Creating AIAnalysis resource for hybrid audit validation")
			testID := uuid.New().String()[:8]
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-hybrid-audit-%s", testID),
					Namespace: namespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: fmt.Sprintf("rr-hybrid-%s", testID),
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       fmt.Sprintf("test-rr-%s", testID), // ✅ UNIQUE per test run
						Namespace:  namespace,
					},
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      fmt.Sprintf("fp-hybrid-%s", uuid.New().String()[:8]),
							Severity:         "critical",
							SignalType:       "CrashLoopBackOff", // HAPI mock will return deterministic response
							Environment:      "production",
							BusinessPriority: "P0",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "payment-service",
								Namespace: namespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged: true,
								},
							},
						},
						// Single analysis type for hybrid audit test (focused on HAPI+AA event validation)
						AnalysisTypes: []string{"investigation"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed(), "AIAnalysis creation should succeed")

			// Clean up after test
			defer func() {
				_ = k8sClient.Delete(ctx, analysis)
			}()

			By("Waiting for controller to complete analysis (calls HAPI)")
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				if err != nil {
					return ""
				}
				return analysis.Status.Phase
			}, 90*time.Second, 2*time.Second).Should(Equal("Completed"),
				"Controller should complete analysis within 90 seconds")

			// DD-AUDIT-CORRELATION-001: Use RemediationRequestRef.Name as correlation_id
		// This matches the correlation_id that AIAnalysis audit client records
		correlationID := analysis.Spec.RemediationRequestRef.Name
			GinkgoWriter.Printf("📋 Testing hybrid audit for correlation_id: %s\n", correlationID)

			// ========================================
			// STEP 1: Verify HAPI audit event (provider perspective)
			// ========================================
			// NT Pattern: No preliminary flush needed - waitForAuditEvents helper flushes on each retry
			By("Waiting for HAPI audit event (aiagent.response.complete)")
			hapiEvents := waitForAuditEvents(correlationID, string(ogenclient.AIAgentResponsePayloadAuditEventEventData), 1)
			hapiEvent := hapiEvents[0]

		By("Validating HAPI event metadata with testutil")
		actorID := "holmesgpt-api"
		validators.ValidateAuditEvent(hapiEvent, validators.ExpectedAuditEvent{
			EventType:     string(ogenclient.AIAgentResponsePayloadAuditEventEventData),
			EventCategory: ogenclient.AuditEventEventCategoryAiagent, // ADR-034 v1.6: HolmesGPT API uses "aiagent" category
			EventAction:   "response_sent",
			EventOutcome:  validators.EventOutcomePtr(ogenclient.AuditEventEventOutcomeSuccess),
			CorrelationID: correlationID,
			ActorID:       &actorID,
		})

			By("Validating HAPI event_data structure (provider perspective - full response)")
			validators.ValidateAuditEventHasRequiredFields(hapiEvent)

			// DD-AUDIT-004: Use strongly-typed payload (no map[string]interface{})
			hapiPayload := hapiEvent.EventData.AIAgentResponsePayload
			Expect(hapiPayload.EventID).ToNot(BeEmpty(), "EventData should have event_id")
			Expect(hapiPayload.IncidentID).ToNot(BeEmpty(), "EventData should have incident_id")

			// DD-AUDIT-005: Validate response_data contains complete IncidentResponse
			responseData := hapiPayload.ResponseData
			Expect(responseData.IncidentId).ToNot(BeEmpty(), "response_data should have incident_id")
			Expect(responseData.Analysis).ToNot(BeEmpty(), "response_data should have analysis text")
			Expect(responseData.RootCauseAnalysis.Summary).ToNot(BeEmpty(), "Should have root_cause_analysis.summary")
			Expect(responseData.Confidence).To(BeNumerically(">", 0), "Should have confidence > 0")
			Expect(responseData.Timestamp).ToNot(BeZero(), "Should have timestamp")
			GinkgoWriter.Println("✅ HAPI event contains complete IncidentResponse structure")

			// ========================================
			// STEP 2: Verify AA audit event (consumer perspective)
			// ========================================
			By("Waiting for AA audit event (aianalysis.analysis.completed)")
			aaEvents := waitForAuditEvents(correlationID, "aianalysis.analysis.completed", 1)
			aaEvent := aaEvents[0]

			By("Validating AA event metadata with testutil")
			aaActorID := "aianalysis-controller"
			validators.ValidateAuditEvent(aaEvent, validators.ExpectedAuditEvent{
				EventType:     "aianalysis.analysis.completed",
				EventCategory: ogenclient.AuditEventEventCategoryAnalysis,
				EventAction:   "analysis_complete",
				EventOutcome:  validators.EventOutcomePtr(ogenclient.AuditEventEventOutcomeSuccess),
				CorrelationID: correlationID,
				ActorID:       &aaActorID,
			})

			By("Validating AA event structure and business context")

			// ✅ CORRECT: Use testutil per TESTING_GUIDELINES.md (line 1823-1846)
			validators.ValidateAuditEvent(aaEvent, validators.ExpectedAuditEvent{
				EventType:     "aianalysis.analysis.completed",
				EventCategory: ogenclient.AuditEventEventCategoryAnalysis,
				EventAction:   "analysis_complete",
				EventOutcome:  validators.EventOutcomePtr(ogenclient.AuditEventEventOutcomeSuccess),
				CorrelationID: correlationID,
				ActorID:       &aaActorID,
			})

			// Validate strongly-typed payload fields (DD-AUDIT-004)
			aaPayload := aaEvent.EventData.AIAnalysisAuditPayload
			Expect(aaPayload.AnalysisName).ToNot(BeEmpty(), "Should have analysis_name in business context")
			Expect(aaPayload.Namespace).ToNot(BeEmpty(), "Should have namespace in business context")
			Expect(string(aaPayload.Phase)).To(Equal("Completed"), "Should have Completed phase")
			GinkgoWriter.Println("✅ AA event contains business context fields")

			// ========================================
			// STEP 3: Validate hybrid approach benefits
			// ========================================
			By("Validating hybrid approach benefits")

			// Benefit 1: Both events share correlation_id for linkage
			Expect(hapiEvent.CorrelationID).To(Equal(aaEvent.CorrelationID),
				"Both events should share correlation_id")

			// Benefit 2: HAPI has authoritative full response
			Expect(responseData.Analysis).ToNot(BeEmpty(),
				"HAPI should have complete analysis response")

			// Benefit 3: AA has business context not in HAPI
			Expect(aaPayload.Phase).ToNot(BeZero(), "AA should have 'phase' (business context)")
			GinkgoWriter.Println("✅ Hybrid approach validated: HAPI has full response, AA has business context")

			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("✅ HYBRID AUDIT VALIDATION PASSED")
			GinkgoWriter.Printf("   • HAPI event: Provider perspective (full response)\n")
			GinkgoWriter.Printf("   • AA event: Consumer perspective (summary + business context)\n")
			GinkgoWriter.Printf("   • Correlation: Both linked via %s\n", correlationID)
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})
	})

	// ========================================
	// TEST 2: RR Reconstruction Completeness
	// Validates that HAPI event contains complete data for RR reconstruction
	// ========================================
	Context("RemediationRequest Reconstruction Capability", func() {
		It("should capture complete IncidentResponse in HAPI event for RR reconstruction", func() {
			// ========================================
			// TEST OBJECTIVE:
			// Verify that HAPI audit event contains COMPLETE IncidentResponse
			// structure with all fields required for RemediationRequest reconstruction
			// per SOC2 Type II compliance requirements.
			// ========================================

			By("Creating AIAnalysis resource for RR reconstruction validation")
			testID := uuid.New().String()[:8]
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-rr-recon-%s", testID),
					Namespace: namespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: fmt.Sprintf("rr-recon-%s", testID),
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       fmt.Sprintf("test-rr-recon-%s", testID), // ✅ UNIQUE per test run
						Namespace:  namespace,
					},
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      fmt.Sprintf("fp-recon-%s", uuid.New().String()[:8]),
							Severity:         "critical",
							SignalType:       "OOMKilled", // Different signal type for variety
							Environment:      "staging",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "data-processor",
								Namespace: namespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged: false,
								},
							},
						},
						// Single analysis type for RR reconstruction test (focused on data structure validation)
						AnalysisTypes: []string{"investigation"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Waiting for analysis completion")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return analysis.Status.Phase
			}, 90*time.Second, 2*time.Second).Should(Equal("Completed"))

			// DD-AUDIT-CORRELATION-001: Use RemediationRequestRef.Name as correlation_id
		// This matches the correlation_id that AIAnalysis audit client records
		correlationID := analysis.Spec.RemediationRequestRef.Name

			// FIX: AA-INT-HAPI-001 - Use standardized waitForAuditEvents pattern
			// ROOT CAUSE: Inline Eventually() used stricter query params (EventCategory="analysis")
			// and shorter timeout (30s vs 90s) compared to successful tests.
			// HAPI Python audit buffer can be slow to flush under load (batch_size triggers, not timer).
			// SOLUTION: Use same pattern as successful "Hybrid Audit Event Emission" test (line 259)
			By("Querying HAPI event for RR reconstruction validation (with Eventually for async buffer)")
			hapiEventType := string(ogenclient.AIAgentResponsePayloadAuditEventEventData)
			hapiEvents := waitForAuditEvents(correlationID, hapiEventType, 1)

			// Extract strongly-typed response_data (DD-AUDIT-004)
			hapiPayload := hapiEvents[0].EventData.AIAgentResponsePayload
			responseData := hapiPayload.ResponseData

			By("Validating COMPLETE IncidentResponse structure for RR reconstruction")

			// Core analysis fields (strongly-typed)
			Expect(responseData.IncidentId).ToNot(BeEmpty(), "Required: incident_id")
			Expect(responseData.Analysis).ToNot(BeEmpty(), "Required: analysis text")
			Expect(responseData.Timestamp).ToNot(BeZero(), "Required: timestamp")
			Expect(responseData.Confidence).To(BeNumerically(">", 0), "Required: confidence score")

			// Root cause analysis (strongly-typed structured data)
			Expect(responseData.RootCauseAnalysis.Summary).ToNot(BeEmpty(), "RCA should have summary")
			Expect(responseData.RootCauseAnalysis.Severity).ToNot(BeEmpty(), "RCA should have severity")

			// Selected workflow (for execution) - strongly-typed optional field
			if responseData.SelectedWorkflow.IsSet() {
				workflow := responseData.SelectedWorkflow.Value
				// Workflow ID is mandatory if workflow is selected
				Expect(workflow.WorkflowId.IsSet()).To(BeTrue(), "Workflow should have workflow_id")
				if workflow.WorkflowId.IsSet() {
					Expect(workflow.WorkflowId.Value).ToNot(BeEmpty(), "Workflow ID should not be empty")
				}
				// Other fields are truly optional and may not be set in all scenarios
				GinkgoWriter.Printf("✅ Selected workflow present: workflow_id=%v\n", workflow.WorkflowId)
			}

			// Alternative workflows (audit trail) - strongly-typed array
			Expect(responseData.AlternativeWorkflows).ToNot(BeNil(), "Required: alternative_workflows")

			// Decision metadata - strongly-typed optional fields
			// needs_human_review defaults to false, so just check field exists
			Expect(responseData.Warnings).ToNot(BeNil(), "Required: warnings array")
			// ADR-055: target_in_owner_chain removed, replaced by affected_resource in RCA

			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("✅ RR RECONSTRUCTION VALIDATION PASSED")
			GinkgoWriter.Printf("   • Complete IncidentResponse captured in HAPI event\n")
			GinkgoWriter.Printf("   • All fields required for RR reconstruction present\n")
			GinkgoWriter.Printf("   • SOC2 Type II compliance: PASS\n")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})
	})

	// ========================================
	// TEST 3: Correlation ID Consistency
	// Validates that both events use same correlation_id
	// ========================================
	Context("Audit Event Correlation", func() {
		It("should use same correlation_id in both HAPI and AA events for audit trail linkage", func() {
			// ========================================
			// TEST OBJECTIVE:
			// Verify that BOTH audit events use the SAME correlation_id
			// to enable audit trail reconstruction and defense-in-depth validation.
			// ========================================

			By("Creating AIAnalysis resource for correlation validation")
			// DD-AUDIT-CORRELATION-001: RemediationID must match RemediationRequestRef.Name
			// for HAPI and AA audit events to use same correlation_id
			rrName := fmt.Sprintf("test-rr-corr-%s", uuid.New().String()[:8])
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-correlation-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: rrName, // Must match RemediationRequestRef.Name
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       rrName, // Same value as RemediationID
						Namespace:  namespace,
					},
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      fmt.Sprintf("fp-corr-%s", uuid.New().String()[:8]),
							Severity:         "medium", // DD-SEVERITY-001: Use normalized severity enum
							SignalType:       "ImagePullBackOff",
							Environment:      "development",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: namespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged: false,
								},
							},
						},
						// DD-AIANALYSIS-005: v1.x single analysis type only
						AnalysisTypes: []string{"investigation"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, analysis) }()

			By("Waiting for analysis completion")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return analysis.Status.Phase
			}, 90*time.Second, 2*time.Second).Should(Equal("Completed"))

			// DD-AUDIT-CORRELATION-001: Use RemediationRequestRef.Name as correlation_id
		// This matches the correlation_id that AIAnalysis audit client records
		correlationID := analysis.Spec.RemediationRequestRef.Name

			// NT Pattern: Use Eventually() with Flush() to wait for HAPI events to be written
			By("Querying ALL events by correlation_id and waiting for HAPI events")
			var allEvents []ogenclient.AuditEvent
			Eventually(func() int {
				// NT Pattern: Flush audit buffer on each retry
				_ = auditStore.Flush(ctx)

				allResp, err := dsClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(correlationID),
				})
				if err != nil {
					GinkgoWriter.Printf("⏳ Audit query error: %v\n", err)
					return 0
				}
				if allResp.Data == nil {
					return 0
				}
				allEvents = allResp.Data
				
				// Count HAPI events specifically - don't exit until we have at least 1
				hapiEventType := ogenclient.AIAgentResponsePayloadAuditEventEventData
				hapiCount := 0
				for _, event := range allEvents {
					if event.EventData.Type == hapiEventType {
						hapiCount++
					}
				}
				if hapiCount == 0 {
					GinkgoWriter.Printf("⏳ Waiting for HAPI events (found %d total events, 0 HAPI)\n", len(allEvents))
				}
				return hapiCount
			}, 90*time.Second, 500*time.Millisecond).Should(Equal(1),
				"Should find exactly 1 HAPI audit event after buffer flush")

			By("Counting events by type")
			eventCounts := countEventsByType(allEvents)

			// DD-TESTING-001 §256-300: MANDATORY deterministic count validation
			hapiCount := eventCounts[string(ogenclient.AIAgentResponsePayloadAuditEventEventData)]
			aaCompletedCount := eventCounts["aianalysis.analysis.completed"]

			Expect(hapiCount).To(Equal(1),
				"DD-TESTING-001 violation: Should have EXACTLY 1 HAPI event (controller idempotency)")
			Expect(aaCompletedCount).To(Equal(1),
				"DD-TESTING-001 violation: Should have EXACTLY 1 AA completion event (controller idempotency)")

			GinkgoWriter.Printf("📊 Event counts for correlation_id %s:\n", correlationID)
			for eventType, count := range eventCounts {
				GinkgoWriter.Printf("   • %s: %d\n", eventType, count)
			}

			By("Validating correlation_id consistency across all events")
			for _, event := range allEvents {
				Expect(event.CorrelationID).To(Equal(correlationID),
					fmt.Sprintf("Event type %s should have correlation_id %s", event.EventType, correlationID))
			}

			By("Validating both hybrid events present")
			var foundHAPI, foundAA bool
			for _, event := range allEvents {
				if event.EventType == string(ogenclient.AIAgentResponsePayloadAuditEventEventData) {
					foundHAPI = true
				}
				if event.EventType == aianalysisaudit.EventTypeAnalysisCompleted {
					foundAA = true
				}
			}
			Expect(foundHAPI).To(BeTrue(), "Should find HAPI hybrid event")
			Expect(foundAA).To(BeTrue(), "Should find AA hybrid event")

			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("✅ CORRELATION VALIDATION PASSED")
			GinkgoWriter.Printf("   • Both HAPI and AA events use correlation_id: %s\n", correlationID)
			GinkgoWriter.Printf("   • Audit trail linkage: VERIFIED\n")
			GinkgoWriter.Printf("   • Defense-in-depth validation: ENABLED\n")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})
	})
})
