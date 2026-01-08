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
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
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
var _ = PDescribe("BR-AUDIT-005 Gap #4: Hybrid Provider Data Capture", Serial, Label("integration", "audit", "hybrid", "soc2"), func() {
	var (
		ctx            context.Context
		namespace      string
		datastorageURL string
		dsClient       *dsgen.ClientWithResponses
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"

		// Data Storage URL for audit event queries
		datastorageURL = "http://127.0.0.1:18095" // AIAnalysis integration test DS port (DD-TEST-001)

		// Create Data Storage client for querying audit events
		var err error
		dsClient, err = dsgen.NewClientWithResponses(datastorageURL)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Data Storage client")
	})

	// ========================================
	// SHARED HELPER FUNCTIONS (DD-TESTING-001 Compliance)
	// ========================================

	// queryAuditEvents queries Data Storage for audit events using OpenAPI client (DD-API-001).
	// Returns: Array of audit events (OpenAPI-generated types)
	queryAuditEvents := func(correlationID string, eventType *string) ([]dsgen.AuditEvent, error) {
		limit := 100
		params := &dsgen.QueryAuditEventsParams{
			CorrelationId: &correlationID,
			Limit:         &limit,
		}
		if eventType != nil {
			params.EventType = eventType
		}

		resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("failed to query DataStorage: %w", err)
		}
		if resp.JSON200 == nil {
			return nil, fmt.Errorf("DataStorage returned non-200: %d", resp.StatusCode())
		}
		if resp.JSON200.Data == nil {
			return []dsgen.AuditEvent{}, nil
		}
		return *resp.JSON200.Data, nil
	}

	// waitForAuditEvents polls Data Storage until exact expected count appears (DD-TESTING-001 §256-300).
	// MANDATORY: Uses Equal() for deterministic count validation, not BeNumerically(">=")
	waitForAuditEvents := func(correlationID string, eventType string, expectedCount int) []dsgen.AuditEvent {
		var events []dsgen.AuditEvent
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
	countEventsByType := func(events []dsgen.AuditEvent) map[string]int {
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

			// ========================================
			// STEP 1: Verify HAPI audit event (provider perspective)
			// ========================================
			By("Waiting for HAPI audit event (holmesgpt.response.complete)")
			hapiEvents := waitForAuditEvents(correlationID, "holmesgpt.response.complete", 1)
			hapiEvent := hapiEvents[0]

			By("Validating HAPI event metadata with testutil")
			actorID := "holmesgpt-api"
			testutil.ValidateAuditEvent(hapiEvent, testutil.ExpectedAuditEvent{
				EventType:     "holmesgpt.response.complete",
				EventCategory: dsgen.AuditEventEventCategoryAnalysis,
				EventAction:   "response_sent",
				EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
				CorrelationID: correlationID,
				ActorID:       &actorID,
			})

		By("Validating HAPI event_data structure (provider perspective - full response)")
		testutil.ValidateAuditEventHasRequiredFields(hapiEvent)
		hapiEventData, ok := hapiEvent.EventData.(map[string]interface{})
		Expect(ok).To(BeTrue(), "HAPI event_data should be a map")

		// DD-AUDIT-005: Validate response_data contains complete IncidentResponse
		Expect(hapiEventData).To(HaveKey("response_data"), "EventData should contain response_data")
		responseData, ok := hapiEventData["response_data"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "response_data should be a map")
		Expect(responseData).ToNot(BeEmpty(), "response_data should not be empty")

			// Validate complete IncidentResponse structure (for RR reconstruction)
			Expect(responseData).To(HaveKey("incident_id"), "Should have incident_id")
			Expect(responseData).To(HaveKey("analysis"), "Should have analysis text")
			Expect(responseData).To(HaveKey("root_cause_analysis"), "Should have root_cause_analysis")
			Expect(responseData).To(HaveKey("selected_workflow"), "Should have selected_workflow")
			Expect(responseData).To(HaveKey("confidence"), "Should have confidence score")
			Expect(responseData).To(HaveKey("timestamp"), "Should have timestamp")
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
				EventCategory: dsgen.AuditEventEventCategoryAnalysis,
				EventAction:   "analysis_complete",
				EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
				CorrelationID: correlationID,
				ActorID:       &aaActorID,
			})

		By("Validating AA event_data structure (consumer perspective - summary + business context)")
		testutil.ValidateAuditEventHasRequiredFields(aaEvent)
		
		// DD-AUDIT-005: Validate provider_response_summary and business context fields
		aaEventData := aaEvent.EventData.(map[string]interface{})
		Expect(aaEventData).To(HaveKey("provider_response_summary"), "EventData should contain provider_response_summary")
		Expect(aaEventData).To(HaveKey("phase"), "EventData should contain phase")
		Expect(aaEventData).To(HaveKey("approval_required"), "EventData should contain approval_required")
		Expect(aaEventData).To(HaveKey("degraded_mode"), "EventData should contain degraded_mode")
		Expect(aaEventData).To(HaveKey("warnings_count"), "EventData should contain warnings_count")
			summary := aaEventData["provider_response_summary"].(map[string]interface{})

			// Validate provider_response_summary structure
			Expect(summary).To(HaveKey("incident_id"), "Summary should have incident_id")
			Expect(summary).To(HaveKey("analysis_preview"), "Summary should have analysis_preview")
			Expect(summary).To(HaveKey("needs_human_review"), "Summary should have needs_human_review")
			Expect(summary).To(HaveKey("warnings_count"), "Summary should have warnings_count")
			GinkgoWriter.Println("✅ AA event contains provider_response_summary")

			// Validate AA business context (not in HAPI event)
			Expect(aaEventData["phase"]).To(Equal("Completed"), "Phase should be Completed")
			GinkgoWriter.Println("✅ AA event contains business context fields")

			// ========================================
			// STEP 3: Validate hybrid approach benefits
			// ========================================
			By("Validating hybrid approach benefits")

			// Benefit 1: Both events share correlation_id for linkage
			Expect(hapiEvent.CorrelationId).To(Equal(aaEvent.CorrelationId),
				"Both events should share correlation_id")

			// Benefit 2: HAPI has authoritative full response
			Expect(responseData).To(HaveKey("alternative_workflows"),
				"HAPI should have complete response including alternatives")

			// Benefit 3: AA has business context not in HAPI
			_, hapiHasPhase := hapiEventData["phase"]
			Expect(hapiHasPhase).To(BeFalse(), "HAPI should not have 'phase' (business context)")
			_, aaHasPhase := aaEventData["phase"]
			Expect(aaHasPhase).To(BeTrue(), "AA should have 'phase' (business context)")

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

			By("Querying HAPI event for RR reconstruction validation")
			hapiEventType := "holmesgpt.response.complete"
			hapiResp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
				CorrelationId: &correlationID,
				EventType:     &hapiEventType,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(hapiResp.JSON200.Data).ToNot(BeNil())
			hapiEvents := *hapiResp.JSON200.Data
			// DD-TESTING-001 §256-300: MANDATORY deterministic count validation
			Expect(len(hapiEvents)).To(Equal(1),
				"DD-TESTING-001 violation: Should have EXACTLY 1 HAPI event")

			// Extract response_data
			hapiEventData := hapiEvents[0].EventData.(map[string]interface{})
			responseData := hapiEventData["response_data"].(map[string]interface{})

			By("Validating COMPLETE IncidentResponse structure for RR reconstruction")

			// Core analysis fields
			Expect(responseData).To(HaveKey("incident_id"), "Required: incident_id")
			Expect(responseData).To(HaveKey("analysis"), "Required: analysis text")
			Expect(responseData).To(HaveKey("timestamp"), "Required: timestamp")
			Expect(responseData).To(HaveKey("confidence"), "Required: confidence score")

			// Root cause analysis (structured)
			Expect(responseData).To(HaveKey("root_cause_analysis"), "Required: root_cause_analysis")
			rca, ok := responseData["root_cause_analysis"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "root_cause_analysis should be a map")
			Expect(rca).To(HaveKey("summary"), "RCA should have summary")
			Expect(rca).To(HaveKey("severity"), "RCA should have severity")

			// Selected workflow (for execution)
			Expect(responseData).To(HaveKey("selected_workflow"), "Required: selected_workflow")
			if responseData["selected_workflow"] != nil {
				workflow, ok := responseData["selected_workflow"].(map[string]interface{})
				Expect(ok).To(BeTrue(), "selected_workflow should be a map")
				Expect(workflow).To(HaveKey("workflow_id"), "Workflow should have workflow_id")
				Expect(workflow).To(HaveKey("containerImage"), "Workflow should have containerImage")
				Expect(workflow).To(HaveKey("confidence"), "Workflow should have confidence")
				Expect(workflow).To(HaveKey("parameters"), "Workflow should have parameters")
			}

			// Alternative workflows (audit trail)
			Expect(responseData).To(HaveKey("alternative_workflows"), "Required: alternative_workflows")

			// Decision metadata
			Expect(responseData).To(HaveKey("needs_human_review"), "Required: needs_human_review flag")
			Expect(responseData).To(HaveKey("warnings"), "Required: warnings array")
			Expect(responseData).To(HaveKey("target_in_owner_chain"), "Required: target_in_owner_chain flag")

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

			By("Querying ALL events by correlation_id")
			allResp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
				CorrelationId: &correlationID,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(allResp.JSON200.Data).ToNot(BeNil())
			allEvents := *allResp.JSON200.Data

			By("Counting events by type")
			eventCounts := countEventsByType(allEvents)

			// DD-TESTING-001 §256-300: MANDATORY deterministic count validation
			hapiCount := eventCounts["holmesgpt.response.complete"]
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
				Expect(event.CorrelationId).To(Equal(correlationID),
					fmt.Sprintf("Event type %s should have correlation_id %s", event.EventType, correlationID))
			}

			By("Validating both hybrid events present")
			var foundHAPI, foundAA bool
			for _, event := range allEvents {
				if event.EventType == "holmesgpt.response.complete" {
					foundHAPI = true
				}
				if event.EventType == "aianalysis.analysis.completed" {
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
