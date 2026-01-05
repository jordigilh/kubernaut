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
	"os/exec"
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
var _ = Describe("BR-AUDIT-005 Gap #4: Hybrid Provider Data Capture", Serial, Label("integration", "audit", "hybrid", "soc2"), func() {
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
			By("Querying Data Storage for HAPI audit event (holmesgpt.response.complete)")
			hapiEventType := "holmesgpt.response.complete"
			hapiResp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
				CorrelationId: &correlationID,
				EventType:     &hapiEventType,
			})
			Expect(err).ToNot(HaveOccurred(), "HAPI event query should succeed")
			Expect(hapiResp.JSON200).ToNot(BeNil(), "Should receive 200 response")
			Expect(hapiResp.JSON200.Data).ToNot(BeNil(), "Response should contain data array")

		hapiEvents := *hapiResp.JSON200.Data
		if len(hapiEvents) == 0 {
			// DIAGNOSTIC: Print HAPI container logs to debug why no events
			GinkgoWriter.Println("🚨 NO HAPI EVENTS FOUND - Printing HAPI container logs for diagnosis...")
			cmd := exec.Command("podman", "logs", "aianalysis_hapi_1", "--tail", "100")
			output, err := cmd.CombinedOutput()
			if err != nil {
				GinkgoWriter.Printf("⚠️  Failed to get HAPI logs: %v\n", err)
			} else {
				GinkgoWriter.Printf("=== HAPI Container Logs (last 100 lines) ===\n%s\n=== End HAPI Logs ===\n", string(output))
			}
		}
		Expect(hapiEvents).To(HaveLen(1), "Should have exactly 1 HAPI event (provider perspective)")
		GinkgoWriter.Printf("✅ Found HAPI audit event: holmesgpt.response.complete\n")

			// Validate HAPI event metadata
			hapiEvent := hapiEvents[0]
			Expect(string(hapiEvent.EventCategory)).To(Equal("analysis"),
				"HAPI event_category should be 'analysis'")
			Expect(hapiEvent.ActorId).ToNot(BeNil(), "HAPI actor_id should be set")
			Expect(*hapiEvent.ActorId).To(Equal("holmesgpt-api"),
				"HAPI actor_id should be 'holmesgpt-api'")
			Expect(hapiEvent.CorrelationId).To(Equal(correlationID),
				"HAPI correlation_id should match RemediationID")
			Expect(string(hapiEvent.EventOutcome)).To(Equal("success"),
				"HAPI event_outcome should be 'success'")

			// Validate HAPI event data structure (provider perspective - full response)
			Expect(hapiEvent.EventData).ToNot(BeNil(), "HAPI event_data should be set")
			hapiEventData, ok := hapiEvent.EventData.(map[string]interface{})
			Expect(ok).To(BeTrue(), "HAPI event_data should be a map")

			Expect(hapiEventData).To(HaveKey("response_data"),
				"HAPI event_data should contain 'response_data' field")
			responseData, ok := hapiEventData["response_data"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "response_data should be a map")

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
			By("Querying Data Storage for AA audit event (aianalysis.analysis.completed)")
			aaEventType := "aianalysis.analysis.completed"
			aaResp, err := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
				CorrelationId: &correlationID,
				EventType:     &aaEventType,
			})
			Expect(err).ToNot(HaveOccurred(), "AA event query should succeed")
			Expect(aaResp.JSON200).ToNot(BeNil(), "Should receive 200 response")
			Expect(aaResp.JSON200.Data).ToNot(BeNil(), "Response should contain data array")

			aaEvents := *aaResp.JSON200.Data
			Expect(aaEvents).To(HaveLen(1), "Should have exactly 1 AA event (consumer perspective)")
			GinkgoWriter.Printf("✅ Found AA audit event: aianalysis.analysis.completed\n")

			// Validate AA event metadata
			aaEvent := aaEvents[0]
			Expect(string(aaEvent.EventCategory)).To(Equal("analysis"),
				"AA event_category should be 'analysis'")
			Expect(aaEvent.ActorId).ToNot(BeNil(), "AA actor_id should be set")
			Expect(*aaEvent.ActorId).To(Equal("aianalysis-controller"),
				"AA actor_id should be 'aianalysis-controller'")
			Expect(aaEvent.CorrelationId).To(Equal(correlationID),
				"AA correlation_id should match RemediationID")
			Expect(string(aaEvent.EventOutcome)).To(Equal("success"),
				"AA event_outcome should be 'success'")

			// Validate AA event data structure (consumer perspective - summary + business context)
			Expect(aaEvent.EventData).ToNot(BeNil(), "AA event_data should be set")
			aaEventData, ok := aaEvent.EventData.(map[string]interface{})
			Expect(ok).To(BeTrue(), "AA event_data should be a map")

			// Validate provider_response_summary (consumer perspective on provider data)
			Expect(aaEventData).To(HaveKey("provider_response_summary"),
				"AA event_data should contain 'provider_response_summary' field")
			summary, ok := aaEventData["provider_response_summary"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "provider_response_summary should be a map")

			Expect(summary).To(HaveKey("incident_id"), "Summary should have incident_id")
			Expect(summary).To(HaveKey("analysis_preview"), "Summary should have analysis_preview")
			Expect(summary).To(HaveKey("needs_human_review"), "Summary should have needs_human_review")
			Expect(summary).To(HaveKey("warnings_count"), "Summary should have warnings_count")
			GinkgoWriter.Println("✅ AA event contains provider_response_summary")

			// Validate AA business context (not in HAPI event)
			Expect(aaEventData).To(HaveKey("phase"), "AA should have phase")
			Expect(aaEventData).To(HaveKey("approval_required"), "AA should have approval_required")
			Expect(aaEventData).To(HaveKey("degraded_mode"), "AA should have degraded_mode")
			Expect(aaEventData).To(HaveKey("warnings_count"), "AA should have warnings_count")
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
			Expect(hapiEvents).To(HaveLen(1))

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

			// Should have at least these 2 hybrid audit events
			hapiCount := eventCounts["holmesgpt.response.complete"]
			aaCompletedCount := eventCounts["aianalysis.analysis.completed"]

			Expect(hapiCount).To(Equal(1), "Should have exactly 1 HAPI event")
			Expect(aaCompletedCount).To(Equal(1), "Should have exactly 1 AA completion event")

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

