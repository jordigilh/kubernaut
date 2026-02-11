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

// Package aianalysis contains integration tests for AIAnalysis error handling.
//
// Business Requirements:
// - BR-AI-009: Error handling and recovery
// - BR-AI-050: Error audit trail completeness
//
// Test Strategy:
// - Tests BUSINESS LOGIC that emits audit events (not direct audit store calls)
// - Uses real DataStorage API for audit verification
// - Follows TESTING_GUIDELINES.md anti-pattern avoidance
//
// Authority:
// - docs/services/crd-controllers/02-aianalysis/implementation/appendices/APPENDIX_B_ERROR_HANDLING_PHILOSOPHY.md
// - docs/development/business-requirements/TESTING_GUIDELINES.md
package aianalysis

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// ERROR HANDLING INTEGRATION TESTS
// ========================================
//
// These tests validate AIAnalysis error handling and audit trail completeness
// by triggering BUSINESS OPERATIONS that result in errors, then verifying
// audit events as side effects.
//
// ✅ CORRECT PATTERN: Create CRD → Wait for failure → Verify audit events
// ❌ WRONG PATTERN: Directly call auditStore.StoreAudit() (infrastructure testing)
//
// Reference: TESTING_GUIDELINES.md - "Anti-Pattern: Direct Audit Infrastructure Testing"

var _ = Describe("AIAnalysis Error Handling Integration", func() {
	var (
		testCtx    context.Context
		testCancel context.CancelFunc
		dsClient   *ogenclient.Client
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 120*time.Second)

		// DD-AUTH-014: Use authenticated OpenAPI client from suite setup
		// FIX: Creating unauthenticated client here caused HTTP 401 errors when querying audit events
		// dsClients is created in SynchronizedBeforeSuite with ServiceAccount token
		// Pattern follows notification/controller_audit_emission_test.go:70
		dsClient = dsClients.OpenAPIClient
	})

	AfterEach(func() {
		testCancel()
	})

	// ========================================
	// Scenario 1: Terminal Failure State Audit Trail
	// Business Requirement: BR-AI-050 (Error audit trail completeness)
	// Coverage Target: RecordAnalysisFailed() (audit.go:461) - currently 0.0%
	// ========================================
	Context("Terminal failure auditing - BR-AI-050", func() {
		It("should record analysis.failed audit event when investigation fails permanently", func() {
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Given: AIAnalysis with configuration that causes permanent failure
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			testID := fmt.Sprintf("terminal-failure-%d", time.Now().UnixNano())
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testID,
					Namespace: testNamespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      fmt.Sprintf("test-rr-%s", testID),
						Namespace: testNamespace,
					},
					RemediationID: fmt.Sprintf("test-rr-%s", testID),
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							// Use Mock LLM's no_workflow_found scenario
							// This triggers HAPI to return needs_human_review=true with
							// human_review_reason="workflow_not_found"
							SignalType:       "MOCK_NO_WORKFLOW_FOUND",
							Severity:         "critical",
							Environment:      "production",
							BusinessPriority: "P0",
							Fingerprint:      "test-fingerprint-" + testID,
							TargetResource: aianalysisv1.TargetResource{
								Namespace: testNamespace,
								Kind:      "Pod",
								Name:      "test-pod",
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			GinkgoWriter.Printf("📝 Creating AIAnalysis %s to trigger terminal failure\n", analysis.Name)
			Expect(k8sClient.Create(testCtx, analysis)).To(Succeed())

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// When: Controller processes and reaches terminal state
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			GinkgoWriter.Printf("⏳ Waiting for AIAnalysis to reach Failed state...\n")

			// Per BR-HAPI-197: needs_human_review=true with human_review_reason="no_matching_workflows"
			// should transition to Failed phase with WorkflowResolutionFailed reason
			// Authority: RESPONSE_AIANALYSIS_NEEDS_HUMAN_REVIEW.md
			Eventually(func() bool {
				var updated aianalysisv1.AIAnalysis
				if err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(analysis), &updated); err != nil {
					return false
				}

				GinkgoWriter.Printf("  Current phase: %s, Reason: %s, SubReason: %s\n",
					updated.Status.Phase, updated.Status.Reason, updated.Status.SubReason)

				// Terminal state: Failed + WorkflowResolutionFailed + NoMatchingWorkflows
				return updated.Status.Phase == aianalysisv1.PhaseFailed &&
					updated.Status.Reason == "WorkflowResolutionFailed" &&
					updated.Status.SubReason == "NoMatchingWorkflows"
			}, 90*time.Second, 2*time.Second).Should(BeTrue(),
				"AIAnalysis should reach Failed phase with WorkflowResolutionFailed/NoMatchingWorkflows when no workflow found")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Then: Audit trail includes appropriate audit events
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			correlationID := analysis.Spec.RemediationRequestRef.Name
			GinkgoWriter.Printf("🔍 Querying audit events for correlation_id=%s\n", correlationID)

		// Verify aiagent.response.complete event exists (HAPI returned needs_human_review=true)
		Eventually(func() bool {
			// NT Pattern: Flush audit buffer on each retry
			_ = auditStore.Flush(testCtx)
			
			// Query audit events via OpenAPI client (DD-API-001)
			eventCategory := ogenclient.NewOptString("aiagent") // ADR-034 v1.6: HAPI events use "aiagent" category
			eventType := ogenclient.NewOptString(string(ogenclient.AIAgentResponsePayloadAuditEventEventData))

				resp, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(correlationID),
					EventCategory: eventCategory,
					EventType:     eventType,
				})

				if err != nil {
					GinkgoWriter.Printf("  ⚠️  Audit query failed: %v\n", err)
					return false
				}

				// Extract audit events from response
				if resp.Data == nil || len(resp.Data) == 0 {
					GinkgoWriter.Printf("  ⏳ No aiagent.response.complete events yet\n")
					return false
				}

				GinkgoWriter.Printf("  ✅ Found %d aiagent.response.complete event(s)\n", len(resp.Data))

				// Verify event outcome (should be success - HTTP 200 from HAPI perspective)
				// Per holmesgpt-api/src/audit/events.py:413 - HAPI always returns outcome="success"
				// because the API request succeeded (HTTP 200), even when needs_human_review=true.
				// The "failure" is captured in the AIAnalysis audit event (analysis.failed), not the HAPI event.
				for _, event := range resp.Data {
					GinkgoWriter.Printf("    Event ID: %s, Outcome: %s\n",
						event.EventID, event.EventOutcome)

					// Verify event outcome uses OpenAPI constant
					Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeSuccess),
						"aiagent.response.complete should have outcome=success (HTTP 200 from provider perspective)")
				}

				return true
			}, 90*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Controller should emit aiagent.response.complete audit event when HAPI returns needs_human_review=true")

			// Verify analysis.failed event exists (AIAnalysis failed due to workflow resolution failure)
			Eventually(func() bool {
				eventCategory := ogenclient.NewOptString("analysis")
				eventType := ogenclient.NewOptString("aianalysis.analysis.failed")

				resp, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(correlationID),
					EventCategory: eventCategory,
					EventType:     eventType,
				})

				if err != nil {
					GinkgoWriter.Printf("  ⚠️  Audit query failed: %v\n", err)
					return false
				}

				if resp.Data == nil || len(resp.Data) == 0 {
					GinkgoWriter.Printf("  ⏳ No analysis.failed events yet\n")
					return false
				}

				GinkgoWriter.Printf("  ✅ Found %d analysis.failed event(s)\n", len(resp.Data))

				// Verify event data contains expected fields
				for _, event := range resp.Data {
					// Verify event uses OpenAPI discriminated union for event_data
					if !event.EventData.IsAIAnalysisAuditPayload() {
						GinkgoWriter.Printf("    ⚠️  Event data is not AIAnalysisAuditPayload\n")
						continue
					}

					payload, ok := event.EventData.GetAIAnalysisAuditPayload()
					Expect(ok).To(BeTrue(), "Should be able to extract AIAnalysisAuditPayload")

					GinkgoWriter.Printf("    Analysis: %s, Phase: %s, Reason: %s\n",
						payload.AnalysisName, payload.Phase, payload.Reason.Value)

					// Verify phase is Failed with WorkflowResolutionFailed reason
					Expect(string(payload.Phase)).To(Equal("Failed"),
						"Analysis should be in Failed phase")
					Expect(payload.Reason.Value).To(Equal("WorkflowResolutionFailed"),
						"Failure reason should be WorkflowResolutionFailed")
				}

				return true
			}, 30*time.Second, 2*time.Second).Should(BeTrue(),
				"Controller should emit analysis.failed audit event after workflow resolution fails")

			GinkgoWriter.Printf("✅ Terminal failure auditing test complete\n")
		})
	})

	// ========================================
	// NOTE: Generic error auditing moved to unit tests
	// See: pkg/aianalysis/audit/audit_test.go
	// Rationale: Integration tests cannot reliably trigger RecordError() call paths.
	// Unit tests with mock audit store provide better coverage and control.
	// Business Requirement: BR-AI-050 (Error audit trail completeness)
	// ========================================

	// ========================================
	// Scenario 3: Problem Resolved Path (No Workflow Needed)
	// Business Requirement: BR-HAPI-200 (Problem self-resolved)
	// Coverage Target: handleProblemResolvedFromIncident() (response_processor.go:401) - currently 0.0%
	// ========================================
	Context("Problem resolved path - BR-HAPI-200", func() {
		It("should handle problem_resolved from HAPI (no workflow needed)", func() {
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Given: AIAnalysis with signal that triggers problem_resolved response
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			testID := fmt.Sprintf("problem-resolved-%d", time.Now().UnixNano())
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testID,
					Namespace: testNamespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      fmt.Sprintf("test-rr-%s", testID),
						Namespace: testNamespace,
					},
					RemediationID: fmt.Sprintf("test-rr-%s", testID),
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							// Use Mock LLM's problem_resolved scenario
							// This triggers HAPI to return investigation_outcome="resolved"
							// with confidence >= 0.7 and selected_workflow=null
							SignalType:       "MOCK_PROBLEM_RESOLVED",
							Severity:         "low", // DD-SEVERITY-001 v1.1: Use normalized severity (critical, high, medium, low, unknown)
							Environment:      "production",
							BusinessPriority: "P2",
							Fingerprint:      "test-fingerprint-" + testID,
							TargetResource: aianalysisv1.TargetResource{
								Namespace: testNamespace,
								Kind:      "Pod",
								Name:      "recovered-pod",
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			GinkgoWriter.Printf("📝 Creating AIAnalysis %s to trigger problem_resolved path\n", analysis.Name)
			Expect(k8sClient.Create(testCtx, analysis)).To(Succeed())

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// When: Controller processes and HAPI returns problem_resolved
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			GinkgoWriter.Printf("⏳ Waiting for AIAnalysis to reach Completed phase...\n")

			// The AIAnalysis should complete without creating a workflow
			Eventually(func() bool {
				var updated aianalysisv1.AIAnalysis
				if err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(analysis), &updated); err != nil {
					return false
				}

				GinkgoWriter.Printf("  Current phase: %s, Reason: %s, SubReason: %s\n",
					updated.Status.Phase, updated.Status.Reason, updated.Status.SubReason)

				// Terminal state: Completed + WorkflowNotNeeded + ProblemResolved
				return updated.Status.Phase == aianalysisv1.PhaseCompleted &&
					updated.Status.Reason == "WorkflowNotNeeded" &&
					updated.Status.SubReason == "ProblemResolved"
			}, 90*time.Second, 2*time.Second).Should(BeTrue(),
				"AIAnalysis should reach Completed phase with WorkflowNotNeeded/ProblemResolved")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Then: Verify status fields and audit trail
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			var finalAnalysis aianalysisv1.AIAnalysis
			Expect(k8sClient.Get(testCtx, client.ObjectKeyFromObject(analysis), &finalAnalysis)).To(Succeed())

			// Verify status fields (BR-HAPI-200 Outcome A)
			Expect(finalAnalysis.Status.Phase).To(Equal(aianalysisv1.PhaseCompleted),
				"Phase should be Completed")
			Expect(finalAnalysis.Status.Reason).To(Equal("WorkflowNotNeeded"),
				"Reason should be WorkflowNotNeeded")
			Expect(finalAnalysis.Status.SubReason).To(Equal("ProblemResolved"),
				"SubReason should be ProblemResolved")
			Expect(finalAnalysis.Status.SelectedWorkflow).To(BeNil(),
				"SelectedWorkflow should be nil (no workflow needed)")
			Expect(finalAnalysis.Status.ApprovalRequired).To(BeFalse(),
				"ApprovalRequired should be false (no human review needed)")
			Expect(finalAnalysis.Status.CompletedAt).ToNot(BeNil(),
				"CompletedAt should be set")

			// Verify message indicates self-resolution
			Expect(finalAnalysis.Status.Message).To(ContainSubstring("self-resolved"),
				"Message should indicate problem self-resolved")

			correlationID := analysis.Spec.RemediationRequestRef.Name
			GinkgoWriter.Printf("🔍 Querying audit events for correlation_id=%s\n", correlationID)

		// Verify aiagent.response.complete event exists with success outcome
		Eventually(func() bool {
			// NT Pattern: Flush audit buffer on each retry
			_ = auditStore.Flush(testCtx)
			
			eventCategory := ogenclient.NewOptString("aiagent") // ADR-034 v1.6: HAPI events use "aiagent" category
			eventType := ogenclient.NewOptString(string(ogenclient.AIAgentResponsePayloadAuditEventEventData))

				resp, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(correlationID),
					EventCategory: eventCategory,
					EventType:     eventType,
				})

				if err != nil {
					GinkgoWriter.Printf("  ⚠️  Audit query failed: %v\n", err)
					return false
				}

				if resp.Data == nil || len(resp.Data) == 0 {
					GinkgoWriter.Printf("  ⏳ No aiagent.response.complete events yet\n")
					return false
				}

				GinkgoWriter.Printf("  ✅ Found %d aiagent.response.complete event(s)\n", len(resp.Data))

				// Verify event outcome (should be success for problem resolved)
				for _, event := range resp.Data {
					GinkgoWriter.Printf("    Event ID: %s, Outcome: %s\n",
						event.EventID, event.EventOutcome)

					// Verify event outcome uses OpenAPI constant
					Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeSuccess),
						"aiagent.response.complete should have outcome=success when problem resolved")
				}

				return true
			}, 90*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Controller should emit aiagent.response.complete audit event when HAPI returns problem_resolved")

			// Verify analysis.complete event exists
			Eventually(func() bool {
				eventCategory := ogenclient.NewOptString("analysis")
				eventType := ogenclient.NewOptString("aianalysis.analysis.completed")

				resp, err := dsClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(correlationID),
					EventCategory: eventCategory,
					EventType:     eventType,
				})

				if err != nil {
					GinkgoWriter.Printf("  ⚠️  Audit query failed: %v\n", err)
					return false
				}

				if resp.Data == nil || len(resp.Data) == 0 {
					GinkgoWriter.Printf("  ⏳ No analysis.completed events yet\n")
					return false
				}

				GinkgoWriter.Printf("  ✅ Found %d analysis.completed event(s)\n", len(resp.Data))

				// Verify event data contains expected fields
				for _, event := range resp.Data {
					if !event.EventData.IsAIAnalysisAuditPayload() {
						GinkgoWriter.Printf("    ⚠️  Event data is not AIAnalysisAuditPayload\n")
						continue
					}

					payload, ok := event.EventData.GetAIAnalysisAuditPayload()
					Expect(ok).To(BeTrue(), "Should be able to extract AIAnalysisAuditPayload")

					GinkgoWriter.Printf("    Analysis: %s, Phase: %s\n",
						payload.AnalysisName, payload.Phase)

					// Verify phase is Completed
					Expect(string(payload.Phase)).To(Equal("Completed"),
						"Analysis should be in Completed phase")
				}

				return true
			}, 30*time.Second, 2*time.Second).Should(BeTrue(),
				"Controller should emit analysis.completed audit event after problem_resolved path")

			GinkgoWriter.Printf("✅ Problem resolved path test complete\n")
		})
	})
})
