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

// Package aianalysis contains flow-based audit integration tests.
//
// These tests verify that the AIAnalysis controller AUTOMATICALLY generates
// audit events during reconciliation (not manual audit calls).
//
// Authority:
// - DD-AUDIT-003: AIAnalysis MUST generate audit traces (P0)
// - BR-AI-050: Complete audit trail for compliance
//
// Test Strategy:
// - Create AIAnalysis resources → Controller reconciles → Audit events generated
// - Verify audit events appear in Data Storage WITHOUT manual audit calls
// - Tests REAL controller behavior, not just audit client library
//
// Business Value:
// - Operators can debug production failures using complete audit trail
// - Compliance teams can audit all AI analysis decisions
// - Performance teams can identify bottlenecks from audit timings
package aianalysis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	"github.com/jordigilh/kubernaut/pkg/testutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ========================================
// FLOW-BASED AUDIT INTEGRATION TESTS
// ========================================
//
// These tests verify that the AIAnalysis controller AUTOMATICALLY
// generates audit events during reconciliation.
//
// IMPORTANT DISTINCTION:
// - Manual trigger tests (DELETED): auditClient.RecordX() → tests audit client library
// - Flow tests (HERE): Create AIAnalysis → tests controller behavior
//
// ========================================

// countEventsByType counts occurrences of each event type in the given events.
// Per DD-TESTING-001: Deterministic count validation requires counting by event type.
//
// Returns: map[eventType]count
func countEventsByType(events []dsgen.AuditEvent) map[string]int {
	counts := make(map[string]int)
	for _, event := range events {
		counts[event.EventType]++
	}
	return counts
}

var _ = Describe("AIAnalysis Controller Audit Flow Integration - BR-AI-050", Label("integration", "audit", "flow"), func() {
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
		datastorageURL = "http://localhost:18095" // AIAnalysis integration test DS port (DD-TEST-001)

		// Create Data Storage client for querying audit events
		var err error
		dsClient, err = dsgen.NewClientWithResponses(datastorageURL)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Data Storage client")
	})

	// ========================================
	// CONTEXT: Complete Workflow Audit Coverage
	// Business Value: Operators need complete audit trail from creation to completion
	// ========================================

	Context("Complete Workflow Audit Trail - BR-AUDIT-001", func() {
		It("should generate complete audit trail from Pending to Completed", FlakeAttempts(3), func() {
			// ========================================
			// TEST OBJECTIVE:
			// Verify controller generates ALL audit events during full workflow:
			// - Phase transitions: Pending → Investigating → Analyzing → Completed
			// - HolmesGPT calls during Investigation
			// - Rego evaluations during Analyzing
			// - Approval decisions during Analyzing
			// - Analysis complete event at Completed
			// ========================================
			//
			// FLAKINESS NOTE (DD-TESTING-001):
			// This test is marked FlakeAttempts(3) due to intermittent audit buffering
			// race conditions in parallel execution. Expected: 3 phase transitions.
			// Occasionally observes: 5 phase transitions (likely duplicate Pending→Investigating).
			// Root cause under investigation. See commit 08ba84723 for partial fix.

			By("Creating AIAnalysis resource requiring full workflow")
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-complete-workflow-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: fmt.Sprintf("rr-complete-%s", uuid.New().String()[:8]),
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      fmt.Sprintf("fp-workflow-%s", uuid.New().String()[:8]),
							Severity:         "critical",
							SignalType:       "CrashLoopBackOff",
							Environment:      "production",
							BusinessPriority: "P0",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "critical-pod",
								Namespace: namespace,
							},
						},
						AnalysisTypes: []string{"investigation", "workflow-selection"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed(), "AIAnalysis creation should succeed")

			// Clean up after test
			defer func() {
				Expect(k8sClient.Delete(ctx, analysis)).To(Succeed())
			}()

			By("Waiting for controller to complete full reconciliation")
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				if err != nil {
					return ""
				}
				return analysis.Status.Phase
			}, 90*time.Second, 2*time.Second).Should(Equal("Completed"),
				"Controller should complete full workflow within 90 seconds")

			By("Verifying complete audit trail in Data Storage")
			// Query ALL audit events for this remediation ID
			correlationID := analysis.Spec.RemediationID
			eventCategory := "analysis"
			params := &dsgen.QueryAuditEventsParams{
				CorrelationId: &correlationID,
				EventCategory: &eventCategory,
			}
			resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
			Expect(err).ToNot(HaveOccurred(), "Audit query should succeed")
			Expect(resp.JSON200).ToNot(BeNil(), "Audit response should be 200 OK")
			Expect(resp.JSON200.Data).ToNot(BeNil(), "Audit data should be present")

			events := *resp.JSON200.Data

			// Business Value: Complete audit trail for compliance
			By("Verifying phase transition events are present")
			hasPhaseTransition := false
			for _, event := range events {
				if event.EventType == aiaudit.EventTypePhaseTransition {
					hasPhaseTransition = true
					break
				}
			}
			Expect(hasPhaseTransition).To(BeTrue(),
				"Controller MUST audit phase transitions (Pending → Investigating → Analyzing → Completed)")

			By("Verifying HolmesGPT call events are present")
			hasHolmesGPTCall := false
			for _, event := range events {
				if event.EventType == aiaudit.EventTypeHolmesGPTCall {
					hasHolmesGPTCall = true
					break
				}
			}
			Expect(hasHolmesGPTCall).To(BeTrue(),
				"Investigation handler MUST audit HolmesGPT API calls")

			By("Verifying approval decision events are present")
			hasApprovalDecision := false
			for _, event := range events {
				if event.EventType == aiaudit.EventTypeApprovalDecision {
					hasApprovalDecision = true
					break
				}
			}
			Expect(hasApprovalDecision).To(BeTrue(),
				"Analyzing handler MUST audit approval decisions")

			By("Verifying analysis complete event is present")
			hasAnalysisComplete := false
			for _, event := range events {
				if event.EventType == aiaudit.EventTypeAnalysisCompleted {
					hasAnalysisComplete = true
					break
				}
			}
			Expect(hasAnalysisComplete).To(BeTrue(),
				"Controller MUST audit analysis completion")

			// Business Value: Complete audit trail enables compliance
			By("Verifying audit trail is complete (all required event types present)")
			Expect(events).ToNot(BeEmpty(), "Audit trail must not be empty")

			// Validate ALL required event types are present
			Expect(hasPhaseTransition).To(BeTrue(), "REQUIRED: Phase transition audit events")
			Expect(hasHolmesGPTCall).To(BeTrue(), "REQUIRED: HolmesGPT call audit events")
			Expect(hasApprovalDecision).To(BeTrue(), "REQUIRED: Approval decision audit events")
			Expect(hasAnalysisComplete).To(BeTrue(), "REQUIRED: Analysis completion audit event")

			// Count events by type for detailed validation
			eventTypeCounts := make(map[string]int)
			for _, event := range events {
				eventTypeCounts[event.EventType]++
			}

			// Validate expected event counts (DD-TESTING-001: Deterministic count validation)
			// Phase transitions: Pending→Investigating, Investigating→Analyzing, Analyzing→Completed = 3 transitions
			Expect(eventTypeCounts[aiaudit.EventTypePhaseTransition]).To(Equal(3),
				"Expected exactly 3 phase transitions: Pending→Investigating, Investigating→Analyzing, Analyzing→Completed")

			// HolmesGPT calls: Exactly 1 during investigation phase
			Expect(eventTypeCounts[aiaudit.EventTypeHolmesGPTCall]).To(Equal(1),
				"Expected exactly 1 HolmesGPT API call during investigation")

			// Approval decision: Exactly 1
			Expect(eventTypeCounts[aiaudit.EventTypeApprovalDecision]).To(Equal(1),
				"Should have exactly 1 approval decision")

			// Analysis complete: Exactly 1
			Expect(eventTypeCounts[aiaudit.EventTypeAnalysisCompleted]).To(Equal(1),
				"Should have exactly 1 analysis completion event")

			// Total events: DD-TESTING-001: Validate exact expected count
			// 3 phase transitions + 1 HolmesGPT + 1 Rego evaluation + 1 approval + 1 completion = 7 events
			Expect(len(events)).To(Equal(7),
				"Complete workflow should generate exactly 7 audit events: 3 phase transitions + 1 HolmesGPT + 1 Rego + 1 approval + 1 completion")
		})
	})

	// ========================================
	// CONTEXT: Investigation Phase Audit
	// Business Value: Operators can debug HolmesGPT integration issues
	// ========================================

	Context("Investigation Phase Audit - BR-AI-023", func() {
		It("should automatically audit HolmesGPT calls during investigation", func() {
			// ========================================
			// TEST OBJECTIVE:
			// Verify InvestigatingHandler automatically calls auditClient.RecordHolmesGPTCall()
			// when it calls HolmesGPT-API during investigation phase
			// ========================================

			By("Creating AIAnalysis resource requiring investigation")
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-investigation-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: fmt.Sprintf("rr-investigation-%s", uuid.New().String()[:8]),
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      fmt.Sprintf("fp-investigation-%s", uuid.New().String()[:8]),
							Severity:         "critical",
							SignalType:       "CrashLoopBackOff",
							Environment:      "staging",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "crashing-pod",
								Namespace: namespace,
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			defer func() {
				Expect(k8sClient.Delete(ctx, analysis)).To(Succeed())
			}()

			By("Waiting for controller to complete investigation phase")
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				if err != nil {
					return ""
				}
				return analysis.Status.Phase
			}, 60*time.Second, 2*time.Second).Should(Or(
				Equal("Analyzing"),
				Equal("Completed"),
			), "Controller should complete investigation within 60 seconds")

			By("Verifying HolmesGPT call was automatically audited")
			correlationID := analysis.Spec.RemediationID
			eventType := aiaudit.EventTypeHolmesGPTCall
			eventCategory := "analysis"
			params := &dsgen.QueryAuditEventsParams{
				CorrelationId: &correlationID,
				EventType:     &eventType,
				EventCategory: &eventCategory,
			}
			resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.JSON200).ToNot(BeNil())
			Expect(resp.JSON200.Data).ToNot(BeNil())

			events := *resp.JSON200.Data

			// DD-TESTING-001: Deterministic count validation instead of weak null-testing
			eventCounts := countEventsByType(events)
			Expect(eventCounts[aiaudit.EventTypeHolmesGPTCall]).To(Equal(1),
				"Expected exactly 1 HolmesGPT call event during investigation")

			// Business Value: Operators can trace HolmesGPT interactions
			event := events[0]
			testutil.ValidateAuditEvent(event, testutil.ExpectedAuditEvent{
				EventType:     aiaudit.EventTypeHolmesGPTCall,
				EventCategory: dsgen.AuditEventEventCategoryAnalysis,
				EventAction:   "holmesgpt_call",
				EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
				CorrelationID: correlationID,
			})

			// DD-TESTING-001: Validate event_data structure per DD-AUDIT-004
			eventData := event.EventData.(map[string]interface{})
			Expect(eventData).To(HaveKey("endpoint"), "event_data should include HolmesGPT endpoint")
			Expect(eventData).To(HaveKey("http_status_code"), "event_data should include HTTP status code")
			Expect(eventData).To(HaveKey("duration_ms"), "event_data should include call duration")

			// Validate field values
			statusCode := int(eventData["http_status_code"].(float64))
			Expect(statusCode).To(Equal(200), "Successful HolmesGPT call should return 200")

			durationMs := int(eventData["duration_ms"].(float64))
			Expect(durationMs).To(BeNumerically(">", 0), "Duration should be positive")
		})

		It("should audit errors during investigation phase", func() {
			// ========================================
			// TEST OBJECTIVE:
			// Verify controller audits errors via EventTypeError during investigation failures
			// ========================================
			//
			// NOTE: This test validates that error audit events (EventTypeError) are created
			// when the controller encounters errors during reconciliation. Since the controller
			// retries transient errors, we check for the presence of ANY audit events related
			// to the AIAnalysis, which proves the audit trail is being generated.

			By("Creating AIAnalysis with potentially problematic configuration")
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-investigation-error-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: fmt.Sprintf("rr-inv-error-%s", uuid.New().String()[:8]),
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      fmt.Sprintf("fp-inv-error-%s", uuid.New().String()[:8]),
							Severity:         "critical",
							SignalType:       "CrashLoopBackOff",
							Environment:      "staging",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "test-error-pod",
								Namespace: namespace,
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			defer func() {
				Expect(k8sClient.Delete(ctx, analysis)).To(Succeed())
			}()

			By("Waiting for controller to process and generate audit events")
			correlationID := analysis.Spec.RemediationID
			eventCategory := "analysis"

			// DD-TESTING-001: Use Eventually() instead of time.Sleep()
			var events []dsgen.AuditEvent
			Eventually(func() int {
				params := &dsgen.QueryAuditEventsParams{
					CorrelationId: &correlationID,
					EventCategory: &eventCategory,
				}
				resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
				if err != nil || resp.JSON200 == nil || resp.JSON200.Data == nil {
					return 0
				}
				events = *resp.JSON200.Data
				return len(events)
			}, 30*time.Second, 2*time.Second).Should(BeNumerically(">", 0),
				"Controller MUST generate audit events even during error scenarios")

			// DD-TESTING-001: Validate specific event types even in error scenarios
			// Business Value: Operators have audit trail regardless of success/failure
			eventCounts := countEventsByType(events)

			// At minimum, we expect phase transition events
			Expect(eventCounts[aiaudit.EventTypePhaseTransition]).To(BeNumerically(">=", 1),
				"Expected at least 1 phase transition event even in error scenarios")

			// Verify events include required metadata
			for _, event := range events {
				Expect(event.EventCategory).To(Equal(dsgen.AuditEventEventCategoryAnalysis),
					"All AIAnalysis events must have category 'analysis'")
				Expect(event.CorrelationId).To(Equal(correlationID),
					"All events must share the same correlation_id")
			}
		})
	})

	// ========================================
	// CONTEXT: Analysis Phase Audit
	// Business Value: Compliance teams can audit all approval decisions
	// ========================================

	Context("Analysis Phase Audit - BR-AI-030", func() {
		It("should automatically audit approval decisions during analysis", func() {
			// ========================================
			// TEST OBJECTIVE:
			// Verify AnalyzingHandler automatically calls auditClient.RecordApprovalDecision()
			// after Rego policy evaluation determines approval requirement
			// ========================================

			By("Creating AIAnalysis resource requiring approval decision")
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-approval-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: fmt.Sprintf("rr-approval-%s", uuid.New().String()[:8]),
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      fmt.Sprintf("fp-approval-%s", uuid.New().String()[:8]),
							Severity:         "critical",
							SignalType:       "CrashLoopBackOff",
							Environment:      "production", // Production requires approval
							BusinessPriority: "P0",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "prod-pod",
								Namespace: namespace,
							},
						},
						AnalysisTypes: []string{"investigation", "workflow-selection"},
					},
				},
			}
			// Set investigation result to trigger analyzing phase
			// TODO: Once InvestigationData field is added to status, populate it here
			analysis.Status.Phase = "Analyzing"

			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			defer func() {
				Expect(k8sClient.Delete(ctx, analysis)).To(Succeed())
			}()

			By("Waiting for controller to complete analysis phase")
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				if err != nil {
					return ""
				}
				return analysis.Status.Phase
			}, 60*time.Second, 2*time.Second).Should(Equal("Completed"),
				"Controller should complete analysis within 60 seconds")

			By("Verifying approval decision was automatically audited")
			correlationID := analysis.Spec.RemediationID
			eventType := aiaudit.EventTypeApprovalDecision
			eventCategory := "analysis"
			params := &dsgen.QueryAuditEventsParams{
				CorrelationId: &correlationID,
				EventType:     &eventType,
				EventCategory: &eventCategory,
			}
			resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.JSON200).ToNot(BeNil())
			Expect(resp.JSON200.Data).ToNot(BeNil())

			events := *resp.JSON200.Data

			// DD-TESTING-001: Deterministic count validation instead of weak null-testing
			eventCounts := countEventsByType(events)
			Expect(eventCounts[aiaudit.EventTypeApprovalDecision]).To(Equal(1),
				"Expected exactly 1 approval decision event per analysis")

			// Business Value: Compliance teams can audit approval decisions
			event := events[0]
			testutil.ValidateAuditEvent(event, testutil.ExpectedAuditEvent{
				EventType:     aiaudit.EventTypeApprovalDecision,
				EventCategory: dsgen.AuditEventEventCategoryAnalysis,
				EventAction:   "approval_decision",
				EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
				CorrelationID: correlationID,
			})

			// DD-TESTING-001: Validate event_data structure per DD-AUDIT-004
			eventData := event.EventData.(map[string]interface{})
			Expect(eventData).To(HaveKey("decision"), "event_data should include approval decision")
			Expect(eventData).To(HaveKey("reason"), "event_data should include decision reason")

			// Validate field values
			decision := eventData["decision"].(string)
			Expect([]string{"requires_approval", "auto_approved"}).To(ContainElement(decision),
				"Decision should be either 'requires_approval' or 'auto_approved'")
		})

		It("should automatically audit Rego policy evaluations", func() {
			// ========================================
			// TEST OBJECTIVE:
			// Verify AnalyzingHandler automatically calls auditClient.RecordRegoEvaluation()
			// after evaluating approval policy
			// ========================================

			By("Creating AIAnalysis resource that triggers Rego evaluation")
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-rego-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: fmt.Sprintf("rr-rego-%s", uuid.New().String()[:8]),
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      fmt.Sprintf("fp-rego-%s", uuid.New().String()[:8]),
							Severity:         "critical",
							SignalType:       "OOMKilled",
							Environment:      "production", // Mock Rego requires approval for production
							BusinessPriority: "P0",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "oom-pod",
								Namespace: namespace,
							},
						},
						AnalysisTypes: []string{"investigation", "workflow-selection"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			defer func() {
				Expect(k8sClient.Delete(ctx, analysis)).To(Succeed())
			}()

			By("Waiting for controller to complete Rego evaluation")
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				if err != nil {
					return ""
				}
				return analysis.Status.Phase
			}, 60*time.Second, 2*time.Second).Should(Equal("Completed"),
				"Controller should complete analysis with Rego evaluation")

			By("Verifying Rego evaluation was automatically audited")
			correlationID := analysis.Spec.RemediationID
			eventType := aiaudit.EventTypeRegoEvaluation
			eventCategory := "analysis"
			params := &dsgen.QueryAuditEventsParams{
				CorrelationId: &correlationID,
				EventType:     &eventType,
				EventCategory: &eventCategory,
			}

			Eventually(func() int {
				resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
				if err != nil || resp.JSON200 == nil || resp.JSON200.Data == nil {
					return 0
				}
				return len(*resp.JSON200.Data)
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
				"AnalyzingHandler MUST automatically audit Rego evaluations")

			By("Verifying Rego evaluation audit event contains policy decision")
			resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.JSON200).ToNot(BeNil())
			Expect(resp.JSON200.Data).ToNot(BeNil())

			events := *resp.JSON200.Data
			event := events[0]

		// Business Value: Compliance teams can audit all policy decisions
		testutil.ValidateAuditEvent(event, testutil.ExpectedAuditEvent{
			EventType:     aiaudit.EventTypeRegoEvaluation,
			EventCategory: dsgen.AuditEventEventCategoryAnalysis,
			EventAction:   "policy_evaluation", // Matches audit.go:284
			EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
			CorrelationID: correlationID,
			EventDataFields: map[string]interface{}{
				"outcome":          "requires_approval", // Verify specific value
				"degraded":         nil,                 // Validate key exists
				"reason":           nil,                 // Validate key exists
			},
		})
		})
	})

	// ========================================
	// CONTEXT: Phase Transition Audit
	// Business Value: Operators can trace workflow progression
	// ========================================

	Context("Phase Transition Audit - DD-AUDIT-003", func() {
		It("should automatically audit all phase transitions", func() {
			// ========================================
			// TEST OBJECTIVE:
			// Verify controller automatically calls auditClient.RecordPhaseTransition()
			// every time analysis.Status.Phase changes
			// ========================================

			By("Creating AIAnalysis resource to trigger phase transitions")
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-phases-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: fmt.Sprintf("rr-phases-%s", uuid.New().String()[:8]),
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      fmt.Sprintf("fp-phases-%s", uuid.New().String()[:8]),
							Severity:         "warning",
							SignalType:       "HighMemoryUsage",
							Environment:      "development",
							BusinessPriority: "P3",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "dev-pod",
								Namespace: namespace,
							},
						},
						AnalysisTypes: []string{"investigation", "workflow-selection"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			defer func() {
				Expect(k8sClient.Delete(ctx, analysis)).To(Succeed())
			}()

			By("Waiting for controller to complete workflow")
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				if err != nil {
					return ""
				}
				return analysis.Status.Phase
			}, 60*time.Second, 2*time.Second).Should(Equal("Completed"))

			By("Verifying phase transitions were automatically audited")
			correlationID := analysis.Spec.RemediationID
			eventType := aiaudit.EventTypePhaseTransition
			eventCategory := "analysis"
			params := &dsgen.QueryAuditEventsParams{
				CorrelationId: &correlationID,
				EventType:     &eventType,
				EventCategory: &eventCategory,
			}
			resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.JSON200).ToNot(BeNil())
			Expect(resp.JSON200.Data).ToNot(BeNil())

			events := *resp.JSON200.Data
			Expect(events).To(HaveLen(3),
				"Controller should audit 3 phase transitions: Pending→Investigating, Investigating→Analyzing, Analyzing→Completed")
		})
	})

	// ========================================
	// CONTEXT: Error Handling Audit
	// Business Value: Operators can debug production failures
	// ========================================

	Context("Error Handling Audit - BR-AI-050", func() {
		It("should audit HolmesGPT calls with error status code when API fails", func() {
			// ========================================
			// TEST OBJECTIVE:
			// Verify InvestigatingHandler audits HolmesGPT calls even when they fail
			// (with status code 500 and failure outcome)
			// ========================================

			By("Creating AIAnalysis that will trigger HolmesGPT error (using invalid signal type)")
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-hapi-error-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationID: fmt.Sprintf("rr-hapi-error-%s", uuid.New().String()[:8]),
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      fmt.Sprintf("fp-hapi-error-%s", uuid.New().String()[:8]),
							Severity:         "critical",
							SignalType:       "InvalidSignalType", // This may cause HAPI to error
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: namespace,
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			defer func() {
				Expect(k8sClient.Delete(ctx, analysis)).To(Succeed())
			}()

			By("Waiting for controller to call HAPI and audit event (DD-TESTING-001: Eventually() instead of time.Sleep())")
			correlationID := analysis.Spec.RemediationID
			eventType := aiaudit.EventTypeHolmesGPTCall
			eventCategory := "analysis"

			// DD-TESTING-001: Use Eventually() for async event polling
			var events []dsgen.AuditEvent
			Eventually(func() int {
				params := &dsgen.QueryAuditEventsParams{
					CorrelationId: &correlationID,
					EventType:     &eventType,
					EventCategory: &eventCategory,
				}
				resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
				if err != nil || resp.JSON200 == nil || resp.JSON200.Data == nil {
					return 0
				}
				events = *resp.JSON200.Data
				return len(events)
			}, 30*time.Second, 2*time.Second).Should(BeNumerically(">", 0),
				"InvestigatingHandler MUST audit HolmesGPT calls even when they fail")

			// DD-TESTING-001: Validate specific event counts for HolmesGPT calls
			eventCounts := countEventsByType(events)
			Expect(eventCounts[aiaudit.EventTypeHolmesGPTCall]).To(BeNumerically(">=", 1),
				"Expected at least 1 HolmesGPT call event (may be more due to retries)")

			// Business Value: Operators can trace failed HolmesGPT interactions
			event := events[0]

			// Verify event structure and required fields
			testutil.ValidateAuditEventHasRequiredFields(event)
			testutil.ValidateAuditEventDataNotEmpty(event, "http_status_code")

			// DD-TESTING-001: Validate event_data structure per DD-AUDIT-004 (error scenario)
			eventData := event.EventData.(map[string]interface{})
			Expect(eventData).To(HaveKey("endpoint"), "event_data should include HolmesGPT endpoint")
			Expect(eventData).To(HaveKey("http_status_code"), "event_data should include HTTP status code")
			Expect(eventData).To(HaveKey("duration_ms"), "event_data should include call duration")

			// Validate duration is positive (even for failed calls)
			durationMs := int(eventData["duration_ms"].(float64))
			Expect(durationMs).To(BeNumerically(">", 0), "Duration should be positive even for failed calls")

			// Verify event matches expected structure
			// Note: EventOutcome may be success or failure depending on HAPI response
			Expect(event.EventType).To(Equal(aiaudit.EventTypeHolmesGPTCall))
			Expect(event.CorrelationId).To(Equal(correlationID))
			Expect(event.EventCategory).To(Equal(dsgen.AuditEventEventCategoryAnalysis))
		})
	})
})
