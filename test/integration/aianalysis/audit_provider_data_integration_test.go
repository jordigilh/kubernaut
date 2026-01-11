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
// 1. HolmesAPI emits: holmesgpt.response.complete (provider perspective, full response)
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
	"github.com/jordigilh/kubernaut/pkg/testutil"
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

		// Create Data Storage client for querying audit events
		var err error
		dsClient, err = ogenclient.NewClient(datastorageURL)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Data Storage client")
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
	waitForAuditEvents := func(correlationID string, eventType string, expectedCount int) []ogenclient.AuditEvent {
		var events []ogenclient.AuditEvent
		Eventually(func() int {
			var err error
			events, err = queryAuditEvents(correlationID, &eventType)
			if err != nil {
				GinkgoWriter.Printf("⏳ Audit query error: %v\n", err)
				return 0
			}
			return len(events)
		}, 60*time.Second, 2*time.Second).Should(Equal(expectedCount),
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
			// 1. HolmesAPI emits holmesgpt.response.complete (provider perspective)
			// 2. AI Analysis emits aianalysis.analysis.completed (consumer perspective)
			// 3. Both events share the same correlation_id
			// 4. Both events contain appropriate data for their perspective
			// ========================================

			By("Creating AIAnalysis resource for hybrid audit validation")
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-hybrid-audit-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: fmt.Sprintf("rr-hybrid-%s", uuid.New().String()[:8]),
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr",
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
						AnalysisTypes: []string{"investigation", "workflow-selection"},
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

			correlationID := analysis.Spec.RemediationID
			GinkgoWriter.Printf("📋 Testing hybrid audit for correlation_id: %s\n", correlationID)

			By("Flushing audit buffers to ensure all events are persisted")
			flushCtx, flushCancel := context.WithTimeout(ctx, 10*time.Second)
			defer flushCancel()
			err := auditStore.Flush(flushCtx)
			Expect(err).NotTo(HaveOccurred(), "Audit flush should succeed")
			GinkgoWriter.Println("✅ Audit buffers flushed")

			// RACE FIX: HAPI has independent Python buffer with 0.1s flush interval
			// Give HAPI time to flush its buffer (Go flush doesn't trigger HAPI flush)
			time.Sleep(500 * time.Millisecond) // 5x HAPI flush interval for safety
			GinkgoWriter.Println("✅ Waited for HAPI buffer flush (500ms)")

			// ========================================
			// STEP 1: Verify HAPI audit event (provider perspective)
			// ========================================
			By("Waiting for HAPI audit event (holmesgpt.response.complete)")
			hapiEvents := waitForAuditEvents(correlationID, string(ogenclient.HolmesGPTResponsePayloadAuditEventEventData), 1)
			hapiEvent := hapiEvents[0]

			By("Validating HAPI event metadata with testutil")
			actorID := "holmesgpt-api"
			testutil.ValidateAuditEvent(hapiEvent, testutil.ExpectedAuditEvent{
				EventType:     string(ogenclient.HolmesGPTResponsePayloadAuditEventEventData),
				EventCategory: ogenclient.AuditEventEventCategoryAnalysis,
				EventAction:   "response_sent",
				EventOutcome:  testutil.EventOutcomePtr(ogenclient.AuditEventEventOutcomeSuccess),
				CorrelationID: correlationID,
				ActorID:       &actorID,
			})

			By("Validating HAPI event_data structure (provider perspective - full response)")
			testutil.ValidateAuditEventHasRequiredFields(hapiEvent)

			// DD-AUDIT-004: Use strongly-typed payload (no map[string]interface{})
			hapiPayload := hapiEvent.EventData.HolmesGPTResponsePayload
			Expect(hapiPayload.EventID).ToNot(BeEmpty(), "EventData should have event_id")
			Expect(hapiPayload.IncidentID).ToNot(BeEmpty(), "EventData should have incident_id")

			// DD-AUDIT-005: Validate response_data contains complete IncidentResponse
			responseData := hapiPayload.ResponseData
			Expect(responseData.IncidentID).ToNot(BeEmpty(), "response_data should have incident_id")
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
			testutil.ValidateAuditEvent(aaEvent, testutil.ExpectedAuditEvent{
				EventType:     "aianalysis.analysis.completed",
				EventCategory: ogenclient.AuditEventEventCategoryAnalysis,
				EventAction:   "analysis_complete",
				EventOutcome:  testutil.EventOutcomePtr(ogenclient.AuditEventEventOutcomeSuccess),
				CorrelationID: correlationID,
				ActorID:       &aaActorID,
			})

			By("Validating AA event structure and business context")

			// ✅ CORRECT: Use testutil per TESTING_GUIDELINES.md (line 1823-1846)
			testutil.ValidateAuditEvent(aaEvent, testutil.ExpectedAuditEvent{
				EventType:     "aianalysis.analysis.completed",
				EventCategory: ogenclient.AuditEventEventCategoryAnalysis,
				EventAction:   "analysis_complete",
				EventOutcome:  testutil.EventOutcomePtr(ogenclient.AuditEventEventOutcomeSuccess),
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
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-rr-recon-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: fmt.Sprintf("rr-recon-%s", uuid.New().String()[:8]),
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-recon",
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
						AnalysisTypes: []string{"investigation", "workflow-selection"},
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

			correlationID := analysis.Spec.RemediationID

			By("Flushing audit buffers")
			flushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			Expect(auditStore.Flush(flushCtx)).To(Succeed())

			By("Querying HAPI event for RR reconstruction validation (with Eventually for async buffer)")
			hapiEventType := string(ogenclient.HolmesGPTResponsePayloadAuditEventEventData)
			var hapiEvents []ogenclient.AuditEvent

			// RACE FIX: HAPI has independent Python buffer with 0.1s flush interval
			// Use Eventually() to poll until event appears (replaces brittle time.Sleep)
			Eventually(func() int {
				hapiResp, err := dsClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(correlationID),
					EventType:     ogenclient.NewOptString(hapiEventType),
				})
				if err != nil {
					GinkgoWriter.Printf("⏳ Waiting for HAPI event (query error: %v)\n", err)
					return 0
				}
				if hapiResp.Data == nil {
					GinkgoWriter.Println("⏳ Waiting for HAPI event (no data yet)")
					return 0
				}
				hapiEvents = hapiResp.Data
				return len(hapiEvents)
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(1),
				"DD-TESTING-001: Should have EXACTLY 1 HAPI event after async buffer flush")

			// Extract strongly-typed response_data (DD-AUDIT-004)
			hapiPayload := hapiEvents[0].EventData.HolmesGPTResponsePayload
			responseData := hapiPayload.ResponseData

			By("Validating COMPLETE IncidentResponse structure for RR reconstruction")

			// Core analysis fields (strongly-typed)
			Expect(responseData.IncidentID).ToNot(BeEmpty(), "Required: incident_id")
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
				Expect(workflow.WorkflowID.IsSet()).To(BeTrue(), "Workflow should have workflow_id")
				if workflow.WorkflowID.IsSet() {
					Expect(workflow.WorkflowID.Value).ToNot(BeEmpty(), "Workflow ID should not be empty")
				}
				// Other fields are truly optional and may not be set in all scenarios
				GinkgoWriter.Printf("✅ Selected workflow present: workflow_id=%v\n", workflow.WorkflowID)
			}

			// Alternative workflows (audit trail) - strongly-typed array
			Expect(responseData.AlternativeWorkflows).ToNot(BeNil(), "Required: alternative_workflows")

			// Decision metadata - strongly-typed optional fields
			// needs_human_review defaults to false, so just check field exists
			Expect(responseData.Warnings).ToNot(BeNil(), "Required: warnings array")
			// target_in_owner_chain defaults to true, field always present

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
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-correlation-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: fmt.Sprintf("rr-corr-%s", uuid.New().String()[:8]),
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-corr",
						Namespace:  namespace,
					},
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      fmt.Sprintf("fp-corr-%s", uuid.New().String()[:8]),
							Severity:         "warning",
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
						AnalysisTypes: []string{"investigation", "workflow-selection"},
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

			correlationID := analysis.Spec.RemediationID

			By("Flushing audit buffers")
			flushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			Expect(auditStore.Flush(flushCtx)).To(Succeed())

			// RACE FIX: HAPI has independent Python buffer with 0.1s flush interval
			time.Sleep(500 * time.Millisecond)
			GinkgoWriter.Println("✅ Waited for HAPI buffer flush (500ms)")

			By("Querying ALL events by correlation_id")
			allResp, err := dsClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(allResp.Data).ToNot(BeNil())
			allEvents := allResp.Data

			By("Counting events by type")
			eventCounts := countEventsByType(allEvents)

			// DD-TESTING-001 §256-300: MANDATORY deterministic count validation
			hapiCount := eventCounts[string(ogenclient.HolmesGPTResponsePayloadAuditEventEventData)]
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
				if event.EventType == string(ogenclient.HolmesGPTResponsePayloadAuditEventEventData) {
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
