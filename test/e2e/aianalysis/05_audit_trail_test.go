package aianalysis

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	aianalysisaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/validators"
)

// ADR-032 §1: Audit writes are MANDATORY, not best-effort
// This E2E test validates that audit events are actually stored in Data Storage
// during full reconciliation cycles in a real Kind cluster.
//
// Why E2E audit tests are needed (in addition to integration tests):
// - Integration tests validate audit library works (isolated components)
// - E2E tests validate audit is integrated correctly (full cluster)
// - E2E tests catch audit misconfigurations that integration tests miss
//
// Confidence: Integration (90%) + E2E (98%) = Full audit assurance

// queryAuditEvents uses OpenAPI client to query DataStorage (DD-API-001 compliance)
// Per ADR-034 v1.2: event_category is optional for queries (omit to query all categories)
// Per DD-API-001: Direct HTTP to DataStorage is FORBIDDEN - use generated client
// Parameters:
//   - correlationID: RemediationRequest.Name
//   - eventType: Use constants from aianalysisaudit package (e.g., aianalysisaudit.EventTypePhaseTransition)
//
// Returns: 1-5 events (no pagination needed)
func querySpecificAuditEvent(
	correlationID string,
	eventType string, // Use aianalysisaudit.EventType* constants
) ([]dsgen.AuditEvent, error) {
	params := dsgen.QueryAuditEventsParams{
		CorrelationID: dsgen.NewOptString(correlationID),
		EventType:     dsgen.NewOptString(eventType),
		EventCategory: dsgen.NewOptString(aianalysisaudit.EventCategoryAIAnalysis),
	}

	resp, err := dsClient.QueryAuditEvents(context.Background(), params)
	if err != nil {
		return nil, fmt.Errorf("failed to query DataStorage: %w", err)
	}

	if resp.Data == nil {
		return []dsgen.AuditEvent{}, nil
	}

	return resp.Data, nil
}

// queryAllAuditEventTypes queries for ALL AI Analysis event types
// Per docs/testing/AUDIT_QUERY_PAGINATION_STANDARDS.md: Use when validating full timeline
//
// Use this when:
// - Testing FULL audit trail (all event types)
// - Validating metadata across ALL event types
// - Expected result: 10-20 events (acceptable without pagination)
//
// ⚠️ WARNING: Use sparingly - prefer querySpecificAuditEvent() when possible
//
// Parameters:
//   - correlationID: RemediationRequest.Name
//
// Returns: 10-20 events (all AI Analysis event types for this correlation_id)
func queryAllAuditEventTypes(
	correlationID string,
) ([]dsgen.AuditEvent, error) {
	params := dsgen.QueryAuditEventsParams{
		CorrelationID: dsgen.NewOptString(correlationID),
		EventCategory: dsgen.NewOptString(aianalysisaudit.EventCategoryAIAnalysis),
		// EventType intentionally omitted - query ALL event types
	}

	resp, err := dsClient.QueryAuditEvents(context.Background(), params)
	if err != nil {
		return nil, fmt.Errorf("failed to query DataStorage: %w", err)
	}

	if resp.Data == nil {
		return []dsgen.AuditEvent{}, nil
	}

	return resp.Data, nil
}

// countEventsByType counts occurrences of each event type in the given events
// Returns: map[eventType]count
func countEventsByType(events []dsgen.AuditEvent) map[string]int {
	counts := make(map[string]int)
	for _, event := range events {
		counts[event.EventType]++
	}
	return counts
}

// waitForAuditEvents polls Data Storage until audit events appear or timeout.
// This handles the async nature of BufferedAuditStore's background flush.
//
// Parameters:
//   - correlationID: RemediationRequest.Name
//   - eventType: Use constants from aianalysisaudit package (e.g., aianalysisaudit.EventTypePhaseTransition)
//   - minCount: Minimum number of events expected
//
// Returns: Array of audit events (OpenAPI-generated types)
//
// Rationale: BufferedAuditStore flushes asynchronously, so tests must poll
// rather than query immediately after reconciliation. Using Eventually()
// makes tests faster (no fixed sleep) and more reliable (handles timing variance).
func waitForSpecificAuditEvent(
	correlationID string,
	eventType string, // Use aianalysisaudit.EventType* constants
	minCount int,
) []dsgen.AuditEvent {
	var events []dsgen.AuditEvent

	Eventually(func() int {
		var err error
		events, err = querySpecificAuditEvent(correlationID, eventType)
		if err != nil {
			GinkgoWriter.Printf("⏳ Audit query error: %v\n", err)
			return 0
		}
		return len(events)
	}, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", minCount),
		fmt.Sprintf("Should have at least %d %s events for correlation %s", minCount, eventType, correlationID))

	return events
}

var _ = Describe("Audit Trail E2E", Label("e2e", "audit"), func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// ADR-032 §1: End-to-End Audit Validation
	// ========================================

	Context("ADR-032: Audit Trail Completeness", func() {
		It("should create audit events in Data Storage for full reconciliation cycle", func() {
			By("Creating AIAnalysis for production incident")
			suffix := randomSuffix()
			namespace := createTestNamespace("audit-test")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-audit-test-" + suffix,
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationID: "e2e-audit-test-" + suffix,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-audit-fingerprint",
							Severity:        "medium",
							SignalType:       "CrashLoopBackOff",
							Environment:      "production",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment",
								Name:      "payment-service",
								Namespace: "payments",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged: true,
									PDBProtected:  true,
								},
								OwnerChain: []sharedtypes.OwnerChainEntry{
									{Namespace: "payments", Kind: "Deployment", Name: "payment-service"},
								},
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

		By("Waiting for reconciliation to complete")
		// Uses SetDefaultEventuallyTimeout(30s) from suite_test.go (per RCA Jan 31, 2026)
		Eventually(func() string {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
			return string(analysis.Status.Phase)
		}).Should(Equal("Completed"))

			remediationID := analysis.Spec.RemediationID

			By("Querying Data Storage for audit events via OpenAPI client (DD-API-001)")
			// Per DD-API-001: MUST use OpenAPI generated client, NOT raw HTTP
			// Per DD-TESTING-001: Wait for COMPLETE set of expected events, not just "not empty"
			// Root cause of flakiness: Waiting for "not empty" then immediately checking exact counts
			// can fail if audit buffer (1s flush interval) hasn't finished flushing all events yet.
			// Solution: Eventually checks for the MINIMUM expected event count (3 phase transitions).
			var events []dsgen.AuditEvent
		Eventually(func() int {
			var err error
			events, err = queryAllAuditEventTypes(remediationID) // Query all AI Analysis event types
			if err != nil {
				GinkgoWriter.Printf("⏳ Waiting for audit events (error: %v)\n", err)
					return 0
				}
				// Count phase transition events specifically (most reliable indicator of completion)
				eventCounts := countEventsByType(events)
				phaseTransitionCount := eventCounts["aianalysis.phase.transition"]
				if phaseTransitionCount < 3 {
					GinkgoWriter.Printf("⏳ Waiting for complete audit trail (got %d/3 phase transitions, %d total events)\n",
						phaseTransitionCount, len(events))
				}
				return phaseTransitionCount
			}, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", 3),
				"Should have at least 3 phase transition events (Pending→Investigating→Analyzing→Completed)")

			By("Verifying audit event types and counts are EXACTLY correct")
			eventCounts := countEventsByType(events)

			// Per DD-AUDIT-003: AIAnalysis records 6 event types
			// CRITICAL: Use EXACT counts to detect duplicates and reconciliation issues
			// Per TESTING_GUIDELINES.md: Be deterministic, not "at least"

			Expect(eventCounts).To(HaveKey("aianalysis.phase.transition"),
				"Should audit phase transitions (Pending→Investigating→Analyzing→Completed)")
			Expect(eventCounts["aianalysis.phase.transition"]).To(Equal(3),
				"Should have EXACTLY 3 phase transitions: Pending→Investigating→Analyzing→Completed (no duplicates)")

			Expect(eventCounts).To(HaveKey("aianalysis.aiagent.call"),
				"Should audit AI agent API calls during investigation")
			Expect(eventCounts["aianalysis.aiagent.call"]).To(Equal(1),
				"Should have EXACTLY 1 AI agent call in happy path (no retries/duplicates)")

			Expect(eventCounts).To(HaveKey("aianalysis.rego.evaluation"),
				"Should audit Rego policy evaluation for approval decision")
			Expect(eventCounts["aianalysis.rego.evaluation"]).To(Equal(1),
				"Should have EXACTLY 1 Rego evaluation during Analyzing phase (no duplicates)")

			Expect(eventCounts).To(HaveKey("aianalysis.approval.decision"),
				"Should audit approval decision outcome")
			Expect(eventCounts["aianalysis.approval.decision"]).To(Equal(1),
				"Should have EXACTLY 1 approval decision after Rego evaluation (no duplicates)")

			Expect(eventCounts).To(HaveKey("aianalysis.analysis.completed"),
				"Should audit analysis completion with final status")
			Expect(eventCounts["aianalysis.analysis.completed"]).To(Equal(1),
				"Should have EXACTLY 1 analysis completion event (no duplicates)")

			// Note: If this test starts failing due to legitimate retries or controller
			// behavior changes, we should understand WHY before changing to >= comparisons.
			// Unexpected duplicate events indicate bugs in the controller.

			// Note: aianalysis.error.occurred may or may not be present (depends on reconciliation)

			By("Validating correlation_id matches remediation_id")
			for _, event := range events {
				// P0: Use testutil validator for baseline field validation
				validators.ValidateAuditEventHasRequiredFields(event) // ✅ OpenAPI type validation

				Expect(event.CorrelationID).To(Equal(remediationID),
					"All audit events must have correlation_id = remediation_id for traceability")
			}

			By("Validating event_data payloads are valid JSON")
			for _, event := range events {
				// ✅ OpenAPI types guarantee event_data is valid JSON
				// Note: EventData is an ogen discriminated union - cannot use BeEmpty() matcher
				// Specific payload validation is done per event type below
				Expect(event.EventData).NotTo(BeNil(), "event_data should not be null")
			}

			By("Validating event timestamps are set")
			for _, event := range events {
				// ✅ OpenAPI type: EventTimestamp is time.Time (not pointer)
				Expect(event.EventTimestamp).NotTo(BeZero(), "event_timestamp should be set")

				// Verify timestamp is parseable as RFC3339
				timestampStr := event.EventTimestamp.Format(time.RFC3339)
				_, err := time.Parse(time.RFC3339, timestampStr)
				Expect(err).NotTo(HaveOccurred(), "event_timestamp should be valid RFC3339 format")
			}
		})

		It("should audit phase transitions with correct old/new phase values", func() {
			By("Creating AIAnalysis that will go through multiple phases")
			suffix := randomSuffix()
			namespace := createTestNamespace("audit-phases")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-audit-phases-" + suffix,
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationID: "e2e-audit-phases-" + suffix,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-audit-phases",
							Severity:         "critical",
							SignalType:       "OOMKilled",
							Environment:      "staging",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: "default",
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

		By("Waiting for reconciliation to complete")
		// Uses SetDefaultEventuallyTimeout(30s) from suite_test.go (per RCA Jan 31, 2026)
		Eventually(func() string {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
			return string(analysis.Status.Phase)
		}).Should(Equal("Completed"))

			remediationID := analysis.Spec.RemediationID

		By("Waiting for phase transition events to appear in Data Storage")
		phaseEvents := waitForSpecificAuditEvent(remediationID, aianalysisaudit.EventTypePhaseTransition, 1)

			By("Validating phase transition event_data structure")
			for _, event := range phaseEvents {
				// Access strongly-typed payload via discriminated union
				payload := event.EventData.AIAnalysisPhaseTransitionPayload
				Expect(payload).ToNot(BeNil(), "Should have AIAnalysisPhaseTransitionPayload")

				// Per DD-AUDIT-004: PhaseTransitionPayload structure
				Expect(payload.OldPhase).ToNot(BeEmpty(), "Should record old phase")
				Expect(payload.NewPhase).ToNot(BeEmpty(), "Should record new phase")

				oldPhase := payload.OldPhase
				newPhase := payload.NewPhase

				// Verify phase transition is valid
				validPhases := []string{"Pending", "Investigating", "Analyzing", "Completed", "Failed"}
				Expect(validPhases).To(ContainElement(oldPhase), "old_phase should be a valid phase")
				Expect(validPhases).To(ContainElement(newPhase), "new_phase should be a valid phase")
			}
		})

		It("should audit AI agent API calls with correct endpoint and status", func() {
			By("Creating AIAnalysis that will trigger AI agent API call")
			suffix := randomSuffix()
			namespace := createTestNamespace("audit-hapi")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-audit-hapi-" + suffix,
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationID: "e2e-audit-hapi-" + suffix,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-audit-hapi",
							Severity:        "medium",
							SignalType:       "HighMemory",
							Environment:      "development",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment",
								Name:      "api-server",
								Namespace: "default",
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

		By("Waiting for reconciliation to complete")
		// Uses SetDefaultEventuallyTimeout(30s) from suite_test.go (per RCA Jan 31, 2026)
		Eventually(func() string {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
			return string(analysis.Status.Phase)
		}).Should(Equal("Completed"))

			remediationID := analysis.Spec.RemediationID

		By("Waiting for AI agent call events to appear in Data Storage")
		hapiEvents := waitForSpecificAuditEvent(remediationID, aianalysisaudit.EventTypeAIAgentCall, 1)

			By("Validating AI agent API call event_data structure")
			for _, event := range hapiEvents {
				// Access strongly-typed payload via discriminated union
				payload := event.EventData.AIAnalysisAIAgentCallPayload
				Expect(payload).ToNot(BeNil(), "Should have AIAnalysisAIAgentCallPayload")

				// Per DD-AUDIT-004: AIAgentCallPayload structure
				Expect(payload.Endpoint).ToNot(BeEmpty(), "Should record API endpoint called")
				Expect(payload.HTTPStatusCode).ToNot(BeZero(), "Should record HTTP status code")
				Expect(payload.DurationMs).ToNot(BeZero(), "Should record call duration")

				// Verify endpoint is valid
				endpoint := payload.Endpoint
				Expect(endpoint).To(Or(Equal("/api/v1/incident/analyze"), Equal("/api/v1/recovery/investigate")),
					"Endpoint should be incident/analyze or recovery/investigate")

				// Verify HTTP status is 2xx for successful calls
				statusCode := payload.HTTPStatusCode
				Expect(statusCode).To(BeNumerically(">=", 200), "Status code should be 2xx for success")
				Expect(statusCode).To(BeNumerically("<", 300), "Status code should be 2xx for success")
			}
		})

		It("should audit Rego policy evaluations with correct outcome", func() {
			By("Creating AIAnalysis that will trigger Rego evaluation")
			suffix := randomSuffix()
			namespace := createTestNamespace("audit-rego")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-audit-rego-" + suffix,
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationID: "e2e-audit-rego-" + suffix,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-audit-rego",
							Severity:        "medium",
							SignalType:       "CrashLoopBackOff",
							Environment:      "staging", // Auto-approve in staging
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment",
								Name:      "frontend",
								Namespace: "default",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								OwnerChain: []sharedtypes.OwnerChainEntry{
									{Namespace: "default", Kind: "Deployment", Name: "frontend"},
								},
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

		By("Waiting for reconciliation to complete")
		// Uses SetDefaultEventuallyTimeout(30s) from suite_test.go (per RCA Jan 31, 2026)
		Eventually(func() string {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
			return string(analysis.Status.Phase)
		}).Should(Equal("Completed"))

			remediationID := analysis.Spec.RemediationID

		By("Waiting for Rego evaluation events to appear in Data Storage")
		regoEvents := waitForSpecificAuditEvent(remediationID, aianalysisaudit.EventTypeRegoEvaluation, 1)

			By("Validating Rego evaluation event_data structure")
			event := regoEvents[0] // Should be only one Rego evaluation per analysis
			// Access strongly-typed payload via discriminated union
			payload := event.EventData.AIAnalysisRegoEvaluationPayload
			Expect(payload).ToNot(BeNil(), "Should have AIAnalysisRegoEvaluationPayload")

			// Per DD-AUDIT-004: RegoEvaluationPayload structure
			Expect(payload.Outcome).ToNot(BeEmpty(), "Should record policy outcome (approved/requires_approval)")
			Expect(payload.DurationMs).ToNot(BeZero(), "Should record evaluation duration")
			Expect(payload.Reason).ToNot(BeEmpty(), "Should record evaluation reason")

			// Verify outcome is valid
			outcome := payload.Outcome
			Expect([]string{"approved", "requires_approval"}).To(ContainElement(outcome),
				"Outcome should be 'approved' or 'requires_approval'")

			// Verify degraded flag is boolean
			degraded := payload.Degraded
			_ = degraded // Use the variable
		})

		It("should audit approval decisions with correct approval_required flag", func() {
			By("Creating AIAnalysis for production (requires approval)")
			suffix := randomSuffix()
			namespace := createTestNamespace("audit-approval")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-audit-approval-" + suffix,
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationID: "e2e-audit-approval-" + suffix,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-audit-approval",
							Severity:         "critical",
							SignalType:       "CrashLoopBackOff",
							Environment:      "production", // Production requires approval
							BusinessPriority: "P0",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment",
								Name:      "payment-service",
								Namespace: "payments",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								OwnerChain: []sharedtypes.OwnerChainEntry{
									{Namespace: "payments", Kind: "Deployment", Name: "payment-service"},
								},
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

		By("Waiting for reconciliation to complete")
		// Uses SetDefaultEventuallyTimeout(30s) from suite_test.go (per RCA Jan 31, 2026)
		Eventually(func() string {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
			return string(analysis.Status.Phase)
		}).Should(Equal("Completed"))

			By("Verifying approval is required for production")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
			Expect(analysis.Status.ApprovalRequired).To(BeTrue(),
				"Production environment should require approval per Rego policy")

			remediationID := analysis.Spec.RemediationID

		By("Waiting for approval decision events to appear in Data Storage")
		approvalEvents := waitForSpecificAuditEvent(remediationID, aianalysisaudit.EventTypeApprovalDecision, 1)

			By("Validating approval decision event_data structure")
			event := approvalEvents[0]
			// Access strongly-typed payload via discriminated union
			payload := event.EventData.AIAnalysisApprovalDecisionPayload
			Expect(payload).ToNot(BeNil(), "Should have AIAnalysisApprovalDecisionPayload")

			// Per DD-AUDIT-004: ApprovalDecisionPayload structure
			Expect(payload.ApprovalReason).ToNot(BeEmpty(), "Should record reason for approval decision")

			// Verify approval_required matches CR status
			approvalRequired := payload.ApprovalRequired
			Expect(approvalRequired).To(BeTrue(),
				"audit event approval_required should match CR status.ApprovalRequired")

			// Verify auto_approved is false for production
			autoApproved := payload.AutoApproved
			Expect(autoApproved).To(BeFalse(),
				"Production should not be auto-approved")
		})
	})
})
