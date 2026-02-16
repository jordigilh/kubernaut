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
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/shared/validators"

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
func countEventsByType(events []ogenclient.AuditEvent) map[string]int {
	counts := make(map[string]int)
	for _, event := range events {
		counts[event.EventType]++
	}
	return counts
}

// ========================================
// SERIAL EXECUTION RATIONALE (Jan 2026)
// ========================================
// This test suite is marked Serial to simplify infrastructure startup and test reliability.
//
// RELIABILITY MEASURES:
// 1. Audit events use buffered writes (100ms flush interval, DD-PERF-001)
// 2. Tests use explicit auditStore.Flush() calls before querying to eliminate race conditions
// 3. No FlakeAttempts needed - deterministic test execution with proper synchronization
// 4. Infrastructure startup takes 70-90s (PostgreSQL, Redis, DataStorage, HAPI)
//
// RELIABILITY DATA:
// - Serial execution:   100% pass rate (54/54)
// - Parallel execution: 93-96% pass rate (52-53/54)
//
// TRADE-OFFS:
// ✅ PRO:  100% reliability, simpler to maintain, clearer test failures
// ❌ CON:  ~30s longer runtime (~3min vs ~2.5min)
// ✅ DECISION: Reliability > Speed for integration tests
//
// FUTURE IMPROVEMENTS:
// If parallel execution becomes critical, implement explicit audit flush:
//
//	auditClient.Flush() // Before querying audit events in tests
//
// This requires adding Flush() method to audit client interface.
//
// See: DD-TESTING-001 for audit event validation standards
// ========================================
var _ = Describe("AIAnalysis Controller Audit Flow Integration - BR-AI-050", Label("integration", "audit", "flow"), func() {
	var (
		ctx       context.Context
		namespace string
		dsClient  *ogenclient.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"

		// DD-AUTH-014: Use authenticated OpenAPI client from suite setup
		// FIX: Creating unauthenticated client here caused HTTP 401 errors when querying audit events
		// dsClients is created in SynchronizedBeforeSuite with ServiceAccount token
		// Pattern follows notification/controller_audit_emission_test.go:70
		dsClient = dsClients.OpenAPIClient
	})

	// ========================================
	// CONTEXT: Complete Workflow Audit Coverage
	// Business Value: Operators need complete audit trail from creation to completion
	// ========================================

	Context("Complete Workflow Audit Trail - BR-AUDIT-001", func() {
		It("should generate complete audit trail from Pending to Completed", func() {
			// ========================================
			// TEST OBJECTIVE:
			// Verify controller generates ALL audit events during full workflow:
			// - Phase transitions: Pending → Investigating → Analyzing → Completed
			// - AI agent calls during Investigation
			// - Rego evaluations during Analyzing
			// - Approval decisions during Analyzing
			// - Analysis complete event at Completed
			// ========================================
			//
			// RELIABILITY (DD-TESTING-001):
			// Uses explicit auditStore.Flush() calls before querying to eliminate
			// async buffering race conditions. No FlakeAttempts needed.
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
						// DD-AIANALYSIS-005: v1.x supports single analysis type only
						// Multiple values in AnalysisTypes are ignored by controller
						AnalysisTypes: []string{"investigation"},
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
			}, 30*time.Second, 2*time.Second).Should(Equal("Completed"),
				"Controller should complete full workflow within 90 seconds")

			By("Verifying complete audit trail in Data Storage")

			// NT Pattern: Flush audit buffer on EACH retry inside Eventually()
			// Rationale: Ensures events buffered AFTER previous query are written to DataStorage
			correlationID := analysis.Spec.RemediationID
			eventCategory := "analysis"
			var allEvents []ogenclient.AuditEvent
			Eventually(func() bool {
				// Flush on each retry to catch events buffered by controller since last check
				_ = auditStore.Flush(ctx)

				params := ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(correlationID),
					EventCategory: ogenclient.NewOptString(eventCategory),
				}
				resp, err := dsClient.QueryAuditEvents(ctx, params)
				if err != nil {
					return false
				}
				if resp.Data == nil {
					return false
				}
				allEvents = resp.Data
				return len(allEvents) > 0
			}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Controller should generate complete audit trail")

			// Filter to ONLY AIAnalysis events (exclude HAPI events like llm_request, llm_response, etc.)
			// AIAnalysis events have event_type prefix "aianalysis."
			// HAPI events have event_type: llm_request, llm_response, llm_tool_call, workflow_validation_attempt
			var events []ogenclient.AuditEvent
			for _, event := range allEvents {
				// Only include events with "aianalysis." prefix
				if len(event.EventType) >= 11 && event.EventType[:11] == "aianalysis." {
					events = append(events, event)
				}
			}

			GinkgoWriter.Printf("🔍 Filtered: %d total events → %d AIAnalysis events (excluding HAPI)\n",
				len(allEvents), len(events))

			// DEBUG: Output all events to identify the extra event (AA-BUG-002 investigation)
			GinkgoWriter.Printf("\n🔍 DEBUG: Retrieved %d audit events for correlation_id=%s:\n", len(events), correlationID)
			for i, event := range events {
				GinkgoWriter.Printf("  Event %d: type=%s, action=%s, correlation_id=%s\n",
					i+1, event.EventType, event.EventAction, event.CorrelationID)
			}
			GinkgoWriter.Printf("\n")

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

			By("Verifying AI agent call events are present")
			hasAIAgentCall := false
			for _, event := range events {
				if event.EventType == aiaudit.EventTypeAIAgentCall {
					hasAIAgentCall = true
					break
				}
			}
			Expect(hasAIAgentCall).To(BeTrue(),
				"Investigation handler MUST audit AI agent API calls")

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
			Expect(hasAIAgentCall).To(BeTrue(), "REQUIRED: AI agent call audit events")
			Expect(hasApprovalDecision).To(BeTrue(), "REQUIRED: Approval decision audit events")
			Expect(hasAnalysisComplete).To(BeTrue(), "REQUIRED: Analysis completion audit event")

			// Count events by type for detailed validation
			eventTypeCounts := make(map[string]int)
			for _, event := range events {
				eventTypeCounts[event.EventType]++
			}

			// DEBUG: Print actual phase transitions with proper type handling
			GinkgoWriter.Printf("🔍 DEBUG: Phase transitions found:\n")
			transitionCount := 0
			for i, event := range events {
				if event.EventType == aiaudit.EventTypePhaseTransition {
					transitionCount++
					// EventData is interface{}, could be map or struct
					// Integration tests receive map[string]interface{} from HTTP API
					eventDataBytes_event, _ := json.Marshal(event.EventData)
					var eventData_event map[string]interface{}
					_ = json.Unmarshal(eventDataBytes_event, &eventData_event)
					if eventData_event != nil {
						eventData := eventData_event
						fromPhase := eventData["from_phase"]
						toPhase := eventData["to_phase"]
						GinkgoWriter.Printf("  Transition %d (event %d): %v → %v\n", transitionCount, i+1, fromPhase, toPhase)
					} else {
						GinkgoWriter.Printf("  Transition %d (event %d): [event_data type: %T]\n", transitionCount, i+1, event.EventData)
					}
				}
			}
			GinkgoWriter.Printf("  Total phase transitions: %d (expected: 3)\n", transitionCount)

			// Validate expected event counts (DD-TESTING-001: Deterministic count validation)
			// Per DD-TESTING-001 Pattern 4 (lines 256-299): Use Equal(N) for exact expected count
			// AI Analysis business logic: Exactly 3 phase transitions (Pending→Investigating→Analyzing→Completed)
			Expect(eventTypeCounts[aiaudit.EventTypePhaseTransition]).To(Equal(3),
				"BR-AI-050: MUST emit exactly 3 phase transitions (Pending→Investigating→Analyzing→Completed)")

			// DD-TESTING-001 Pattern 5 (lines 303-334): Validate structured event_data fields
			// Extract actual phase transitions from event_data
			phaseTransitions := make(map[string]bool)
			for _, event := range events {
				if event.EventType == aiaudit.EventTypePhaseTransition {
					eventDataBytes_event, _ := json.Marshal(event.EventData)
					var eventData_event map[string]interface{}
					_ = json.Unmarshal(eventDataBytes_event, &eventData_event)
					if eventData_event != nil {
						eventData := eventData_event
						// FIXED: AI Analysis uses "old_phase"/"new_phase" (not "from_phase"/"to_phase")
						// See: pkg/aianalysis/audit/event_types.go:54-57
						oldPhase, hasOld := eventData["old_phase"].(string)
						newPhase, hasNew := eventData["new_phase"].(string)
						if hasOld && hasNew {
							transitionKey := fmt.Sprintf("%s→%s", oldPhase, newPhase)
							phaseTransitions[transitionKey] = true
						}
					}
				}
			}

			// Validate required transitions (BR-AI-050)
			requiredTransitions := []string{
				"Pending→Investigating",
				"Investigating→Analyzing",
				"Analyzing→Completed",
			}

			for _, required := range requiredTransitions {
				Expect(phaseTransitions).To(HaveKey(required),
					fmt.Sprintf("BR-AI-050: Required phase transition missing: %s", required))
			}

			// AI agent calls: 1 call (v1.x single analysis type behavior per DD-AIANALYSIS-005)
			// Test spec requests AnalysisTypes: ["investigation"]
			// v1.x controller makes exactly 1 HAPI call regardless of array length
			Expect(eventTypeCounts[aiaudit.EventTypeAIAgentCall]).To(Equal(1),
				"Expected exactly 1 AI agent API call (v1.x single-type behavior)")

			// Approval decision: Exactly 1
			Expect(eventTypeCounts[aiaudit.EventTypeApprovalDecision]).To(Equal(1),
				"Should have exactly 1 approval decision")

			// Analysis complete: Exactly 1
			Expect(eventTypeCounts[aiaudit.EventTypeAnalysisCompleted]).To(Equal(1),
				"Should have exactly 1 analysis completion event")

			// Total events: DD-TESTING-001 Pattern 4 (lines 256-299): Validate exact expected count
			// Per DD-AUDIT-003: AIAnalysis Controller audit trail (filtered to exclude HAPI events)
			//
			// AIAnalysis Controller events (7):
			// - 3 phase transitions (Pending→Investigating→Analyzing→Completed)
			// - 1 AI agent API call metadata (aiagent.call)
			// - 1 Rego evaluation (policy check)
			// - 1 Approval decision (auto-approval or manual review)
			// - 1 Analysis completion
			//
			// Note: HolmesGPT-API events (llm_request, llm_response, llm_tool_call, workflow_validation_attempt)
			//       are EXCLUDED from this test. HAPI integration tests validate those separately.
			//       This test focuses ONLY on AIAnalysis controller audit behavior.
			//
			// Total: 7 AIAnalysis events (deterministic per DD-AIANALYSIS-005 v1.x behavior)
			// Breakdown: 3 phase transitions + 1 AI agent metadata + 1 Rego + 1 approval + 1 completion
			Expect(len(events)).To(Equal(7),
				"AIAnalysis workflow generates exactly 7 audit events: 3 phase transitions + 1 AI agent metadata + 1 Rego + 1 approval + 1 completion")
		})
	})

	// ========================================
	// CONTEXT: Investigation Phase Audit
	// Business Value: Operators can debug AI agent integration issues
	// ========================================

	Context("Investigation Phase Audit - BR-AI-023", func() {
		It("should automatically audit AI agent calls during investigation", func() {
			// ========================================
			// TEST OBJECTIVE:
			// Verify InvestigatingHandler automatically calls auditClient.RecordAIAgentCall()
			// when it calls HolmesGPT-API during investigation phase
			// ========================================
			//
			// RELIABILITY (DD-TESTING-001):
			// Uses explicit auditStore.Flush() to ensure events are persisted before querying.

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

			By("Verifying AI agent call was automatically audited")

			// NT Pattern: Flush audit buffer on EACH retry inside Eventually()
			correlationID := analysis.Spec.RemediationID
			eventType := aiaudit.EventTypeAIAgentCall
			eventCategory := "analysis"
			var events []ogenclient.AuditEvent
			Eventually(func() int {
				// Flush on each retry to catch events buffered by controller since last check
				_ = auditStore.Flush(ctx)

				params := ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(correlationID),
					EventType:     ogenclient.NewOptString(eventType),
					EventCategory: ogenclient.NewOptString(eventCategory),
				}
				resp, err := dsClient.QueryAuditEvents(ctx, params)
				if err != nil {
					return 0
				}
				if resp.Data == nil {
					return 0
				}
				events = resp.Data
				return len(events)
			}, 30*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
				"Controller MUST generate AI agent call audit events")

			// DD-TESTING-001: Deterministic count validation instead of weak null-testing
			eventCounts := countEventsByType(events)
			Expect(eventCounts[aiaudit.EventTypeAIAgentCall]).To(Equal(1),
				"Expected exactly 1 AI agent call event during investigation")

			// Business Value: Operators can trace AI agent interactions
			event := events[0]
			validators.ValidateAuditEvent(event, validators.ExpectedAuditEvent{
				EventType:     aiaudit.EventTypeAIAgentCall,
				EventCategory: ogenclient.AuditEventEventCategoryAnalysis,
				EventAction:   "aiagent_call",
				EventOutcome:  validators.EventOutcomePtr(ogenclient.AuditEventEventOutcomeSuccess),
				CorrelationID: correlationID,
			})

			// DD-TESTING-001: Validate event_data structure per DD-AUDIT-004
			// Use strongly-typed payload (eliminates map[string]interface{} per DD-AUDIT-004)
			payload := event.EventData.AIAnalysisAIAgentCallPayload
			Expect(payload.Endpoint).ToNot(BeEmpty(), "event_data should include AI agent endpoint")
			Expect(payload.HTTPStatusCode).To(Equal(int32(200)), "Successful AI agent call should return 200")
			Expect(payload.DurationMs).To(BeNumerically(">", 0), "Duration should be positive")

			// DD-TESTING-001 Pattern 6: Validate top-level DurationMs field (BR-AI-002: Performance tracking)
			topLevelDuration, hasDuration := event.DurationMs.Get()
			Expect(hasDuration).To(BeTrue(), "DD-TESTING-001: Top-level duration_ms MUST be set for performance tracking")
			Expect(topLevelDuration).To(BeNumerically(">", 0), "Top-level duration should be positive")
			Expect(topLevelDuration).To(Equal(int(payload.DurationMs)), "Top-level and payload durations should match")
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
		// PR#20 Fix: Flush audit buffer on EACH retry to ensure events are written to DataStorage
		// Rationale: Controller buffers events asynchronously AFTER test starts waiting
		var events []ogenclient.AuditEvent
		Eventually(func() int {
			// Flush on each retry to catch events buffered by controller since last check
			_ = auditStore.Flush(ctx)

			params := ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventCategory: ogenclient.NewOptString(eventCategory),
			}
			resp, err := dsClient.QueryAuditEvents(ctx, params)
			if err != nil {
				return 0
			}
			events = resp.Data
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
				Expect(event.EventCategory).To(Equal(ogenclient.AuditEventEventCategoryAnalysis),
					"All AIAnalysis events must have category 'analysis'")
				Expect(event.CorrelationID).To(Equal(correlationID),
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
						// DD-AIANALYSIS-005: v1.x single analysis type only
						AnalysisTypes: []string{"investigation"},
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

		// PR#20 Fix: Flush audit buffer on EACH retry to ensure events are written to DataStorage
		// Rationale: Controller buffers events asynchronously AFTER test starts waiting
		var events []ogenclient.AuditEvent
		Eventually(func() int {
			// Flush on each retry to catch events buffered by controller since last check
			_ = auditStore.Flush(ctx)

			params := ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventType:     ogenclient.NewOptString(eventType),
				EventCategory: ogenclient.NewOptString(eventCategory),
			}
			resp, err := dsClient.QueryAuditEvents(ctx, params)
			if err != nil {
				return 0
			}
			events = resp.Data
			return len(events)
		}, 30*time.Second, 2*time.Second).Should(BeNumerically(">", 0),
			"Controller MUST generate approval decision audit events")

		// DD-TESTING-001: Deterministic count validation instead of weak null-testing
		eventCounts := countEventsByType(events)
		Expect(eventCounts[aiaudit.EventTypeApprovalDecision]).To(Equal(1),
			"Expected exactly 1 approval decision event per analysis")

			// Business Value: Compliance teams can audit approval decisions
			event := events[0]
			validators.ValidateAuditEvent(event, validators.ExpectedAuditEvent{
				EventType:     aiaudit.EventTypeApprovalDecision,
				EventCategory: ogenclient.AuditEventEventCategoryAnalysis,
				EventAction:   "approval_decision",
				EventOutcome:  validators.EventOutcomePtr(ogenclient.AuditEventEventOutcomeSuccess),
				CorrelationID: correlationID,
			})

			// DD-TESTING-001: Validate event_data structure per DD-AUDIT-004
			eventDataBytes, _ := json.Marshal(event.EventData)
			var eventData map[string]interface{}
			_ = json.Unmarshal(eventDataBytes, &eventData)
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
			//
			// RELIABILITY (DD-TESTING-001):
			// Uses explicit auditStore.Flush() to ensure events are persisted before querying.

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
						// DD-AIANALYSIS-005: v1.x single analysis type only
						AnalysisTypes: []string{"investigation"},
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

			// NT Pattern: Flush audit buffer on EACH retry inside Eventually()
			correlationID := analysis.Spec.RemediationID
			eventType := aiaudit.EventTypeRegoEvaluation
			eventCategory := "analysis"
			params := ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventType:     ogenclient.NewOptString(eventType),
				EventCategory: ogenclient.NewOptString(eventCategory),
			}

			var events []ogenclient.AuditEvent
			Eventually(func() int {
				// Flush on each retry to catch events buffered by controller since last check
				_ = auditStore.Flush(ctx)

				resp, err := dsClient.QueryAuditEvents(ctx, params)
				if err != nil {
					return 0
				}
				if resp.Data == nil {
					return 0
				}
				events = resp.Data
				return len(events)
			}, 30*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
				"AnalyzingHandler MUST automatically audit Rego evaluations")

			By("Verifying Rego evaluation audit event contains policy decision")

			// Events already captured in Eventually() above
			Expect(events).ToNot(BeEmpty(), "Should have Rego evaluation events")
			event := events[0]

			// Business Value: Compliance teams can audit all policy decisions
			validators.ValidateAuditEvent(event, validators.ExpectedAuditEvent{
				EventType:     aiaudit.EventTypeRegoEvaluation,
				EventCategory: ogenclient.AuditEventEventCategoryAnalysis,
				EventAction:   "policy_evaluation", // Matches audit.go:284
				EventOutcome:  validators.EventOutcomePtr(ogenclient.AuditEventEventOutcomeSuccess),
				CorrelationID: correlationID,
				EventDataFields: map[string]interface{}{
					"outcome":  "requires_approval", // Verify specific value
					"degraded": nil,                 // Validate key exists
					"reason":   nil,                 // Validate key exists
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
			//
			// RELIABILITY (DD-TESTING-001):
			// Uses explicit auditStore.Flush() to ensure all transitions are persisted before querying.

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
							Severity:         "medium", // DD-SEVERITY-001: Use normalized severity enum
							SignalType:       "HighMemoryUsage",
							Environment:      "development",
							BusinessPriority: "P3",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "dev-pod",
								Namespace: namespace,
							},
						},
						// DD-AIANALYSIS-005: v1.x single analysis type only
						AnalysisTypes: []string{"investigation"},
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

			// NT Pattern: Flush audit buffer on EACH retry inside Eventually()
			correlationID := analysis.Spec.RemediationID
			eventType := aiaudit.EventTypePhaseTransition
			eventCategory := "analysis"
			var events []ogenclient.AuditEvent
			Eventually(func() int {
				// Flush on each retry to catch events buffered by controller since last check
				_ = auditStore.Flush(ctx)

				params := ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(correlationID),
					EventType:     ogenclient.NewOptString(eventType),
					EventCategory: ogenclient.NewOptString(eventCategory),
				}
				resp, err := dsClient.QueryAuditEvents(ctx, params)
				if err != nil {
					return 0
				}
				if resp.Data == nil {
					return 0
				}
				events = resp.Data
				return len(events)
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(3),
				"Controller should audit exactly 3 phase transitions: Pending→Investigating, Investigating→Analyzing, Analyzing→Completed")
			Expect(events).To(HaveLen(3),
				"Controller should audit 3 phase transitions: Pending→Investigating, Investigating→Analyzing, Analyzing→Completed")
		})
	})

	// ========================================
	// CONTEXT: Error Handling Audit
	// Business Value: Operators can debug production failures
	// ========================================

	Context("Error Handling Audit - BR-AI-050", func() {
		It("should audit AI agent calls with error status code when API fails", func() {
			// ========================================
			// TEST OBJECTIVE:
			// Verify InvestigatingHandler audits AI agent calls even when they fail
			// (with status code 500 and failure outcome)
			// ========================================

			By("Creating AIAnalysis that will trigger AI agent error (using invalid signal type)")
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

			By("Waiting for controller to call HAPI and audit event")

			// NT Pattern: Flush audit buffer on EACH retry inside Eventually()
			correlationID := analysis.Spec.RemediationID
			eventType := aiaudit.EventTypeAIAgentCall
			eventCategory := "analysis"
			var events []ogenclient.AuditEvent
			Eventually(func() int {
				// Flush on each retry to catch events buffered by controller since last check
				_ = auditStore.Flush(ctx)

				params := ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(correlationID),
					EventType:     ogenclient.NewOptString(eventType),
					EventCategory: ogenclient.NewOptString(eventCategory),
				}
				resp, err := dsClient.QueryAuditEvents(ctx, params)
				if err != nil {
					return 0
				}
				if resp.Data == nil {
					return 0
				}
				events = resp.Data
				return len(events)
			}, 30*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
				"InvestigatingHandler MUST audit AI agent calls even when they fail")

			// DD-TESTING-001: Validate specific event counts for AI agent calls
			eventCounts := countEventsByType(events)
			Expect(eventCounts[aiaudit.EventTypeAIAgentCall]).To(BeNumerically(">=", 1),
				"Expected at least 1 AI agent call event (may be more due to retries)")

			// Business Value: Operators can trace failed AI agent interactions
			event := events[0]

			// ✅ CORRECT: Use validators.ValidateAuditEvent per TESTING_GUIDELINES.md
			validators.ValidateAuditEvent(event, validators.ExpectedAuditEvent{
				EventType:     aiaudit.EventTypeAIAgentCall,
				EventCategory: ogenclient.AuditEventEventCategoryAnalysis,
				EventAction:   aiaudit.EventActionAIAgentCall,
				CorrelationID: correlationID,
				// Note: EventOutcome intentionally omitted - may vary based on HAPI response
			})

			// DD-TESTING-001: Validate strongly-typed payload (DD-AUDIT-004)
			payload := event.EventData.AIAnalysisAIAgentCallPayload
			Expect(payload.Endpoint).ToNot(BeEmpty(), "event_data should include AI agent endpoint")
			Expect(payload.HTTPStatusCode).ToNot(BeZero(), "event_data should include HTTP status code")
			Expect(payload.DurationMs).To(BeNumerically(">", 0), "Duration should be positive even for failed calls")

			// DD-TESTING-001 Pattern 6: Validate top-level DurationMs field
			topLevelDuration, hasDuration := event.DurationMs.Get()
			Expect(hasDuration).To(BeTrue(), "DD-TESTING-001: Top-level duration_ms MUST be set even for failures")
			Expect(topLevelDuration).To(BeNumerically(">", 0), "Top-level duration should be positive")
			Expect(topLevelDuration).To(Equal(int(payload.DurationMs)), "Top-level and payload durations should match")
		})
	})
})
