package aianalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil"
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

// waitForAuditEvents polls Data Storage until audit events appear or timeout.
// This handles the async nature of BufferedAuditStore's background flush.
//
// Parameters:
//   - httpClient: HTTP client for Data Storage API
//   - remediationID: Correlation ID to filter events
//   - eventType: Event type to query (e.g., "aianalysis.phase.transition")
//   - minCount: Minimum number of events expected
//
// Returns: Array of audit events (as map[string]interface{})
//
// Rationale: BufferedAuditStore flushes asynchronously, so tests must poll
// rather than query immediately after reconciliation. Using Eventually()
// makes tests faster (no fixed sleep) and more reliable (handles timing variance).
func waitForAuditEvents(
	httpClient *http.Client,
	remediationID string,
	eventType string,
	minCount int,
) []map[string]interface{} {
	var events []map[string]interface{}

	Eventually(func() int {
		resp, err := httpClient.Get(fmt.Sprintf(
			"http://localhost:8091/api/v1/audit/events?correlation_id=%s&event_type=%s",
			remediationID, eventType,
		))
		if err != nil {
			return 0
		}
		defer resp.Body.Close()

		var auditResponse struct {
			Data []map[string]interface{} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&auditResponse); err != nil {
			return 0
		}

		events = auditResponse.Data
		return len(events)
	}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", minCount),
		fmt.Sprintf("Should have at least %d %s events for remediation %s", minCount, eventType, remediationID))

	return events
}

var _ = Describe("Audit Trail E2E", Label("e2e", "audit"), func() {
	var (
		httpClient *http.Client
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		httpClient = &http.Client{Timeout: 10 * time.Second}
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
							Severity:         "warning",
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
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal("Completed"))

			remediationID := analysis.Spec.RemediationID

		By("Querying Data Storage for audit events via NodePort")
		// Data Storage NodePort: 30081 -> host port 8091 (per kind-aianalysis-config.yaml)
		// NOTE: Using Eventually to handle async audit buffer flush (1s interval)
		var events []map[string]interface{}
		Eventually(func() []map[string]interface{} {
			resp, err := httpClient.Get(fmt.Sprintf(
				"http://localhost:8091/api/v1/audit/events?correlation_id=%s",
				remediationID,
			))
			if err != nil {
				return nil
			}
			defer resp.Body.Close()

			var auditResponse struct {
				Data []map[string]interface{} `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&auditResponse); err != nil {
				return nil
			}
			events = auditResponse.Data
			return events
		}, 3*time.Second, 200*time.Millisecond).ShouldNot(BeEmpty(), "Should have at least one audit event for completed analysis")

			By("Verifying audit event types are present")
			eventTypes := make(map[string]int)
			for _, event := range events {
				eventType, ok := event["event_type"].(string)
				Expect(ok).To(BeTrue(), "event_type should be a string")
				eventTypes[eventType]++
			}

			// Per DD-AUDIT-003: AIAnalysis records 6 event types
			Expect(eventTypes).To(HaveKey("aianalysis.phase.transition"),
				"Should audit phase transitions (Pending→Investigating→Analyzing→Completed)")
			Expect(eventTypes).To(HaveKey("aianalysis.holmesgpt.call"),
				"Should audit HolmesGPT-API calls during investigation")
			Expect(eventTypes).To(HaveKey("aianalysis.rego.evaluation"),
				"Should audit Rego policy evaluation for approval decision")
			Expect(eventTypes).To(HaveKey("aianalysis.approval.decision"),
				"Should audit approval decision outcome")
			Expect(eventTypes).To(HaveKey("aianalysis.analysis.completed"),
				"Should audit analysis completion with final status")

			// Note: aianalysis.error.occurred may or may not be present (depends on reconciliation)

			By("Validating correlation_id matches remediation_id")
			for _, event := range events {
				// P0: Use testutil validator for baseline field validation
				typedEvent := convertJSONToAuditEvent(event)
				testutil.ValidateAuditEventHasRequiredFields(typedEvent)

				correlationID, ok := event["correlation_id"].(string)
				Expect(ok).To(BeTrue(), "correlation_id should be a string")
				Expect(correlationID).To(Equal(remediationID),
					"All audit events must have correlation_id = remediation_id for traceability")
			}

			By("Validating event_data payloads are valid JSON")
			for _, event := range events {
				eventData, ok := event["event_data"]
				Expect(ok).To(BeTrue(), "event_data field should exist")
				Expect(eventData).NotTo(BeNil(), "event_data should not be null")

				// event_data should be a JSON object (map)
				eventDataMap, ok := eventData.(map[string]interface{})
				Expect(ok).To(BeTrue(), "event_data should be a JSON object")
				Expect(eventDataMap).NotTo(BeEmpty(), "event_data should not be an empty object")
			}

			By("Validating event timestamps are set")
			for _, event := range events {
				timestamp, ok := event["event_timestamp"].(string)
				Expect(ok).To(BeTrue(), "event_timestamp should be a string")
				Expect(timestamp).NotTo(BeEmpty(), "event_timestamp should not be empty")

				// Verify timestamp is parseable as RFC3339
				_, err := time.Parse(time.RFC3339, timestamp)
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
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal("Completed"))

		remediationID := analysis.Spec.RemediationID

		By("Waiting for phase transition events to appear in Data Storage")
		phaseEvents := waitForAuditEvents(httpClient, remediationID, "aianalysis.phase.transition", 1)

		By("Validating phase transition event_data structure")
			for _, event := range phaseEvents {
				eventData, ok := event["event_data"].(map[string]interface{})
				Expect(ok).To(BeTrue())

				// Per DD-AUDIT-004: PhaseTransitionPayload structure
				Expect(eventData).To(HaveKey("old_phase"), "Should record old phase")
				Expect(eventData).To(HaveKey("new_phase"), "Should record new phase")

				oldPhase := eventData["old_phase"].(string)
				newPhase := eventData["new_phase"].(string)

				// Verify phase transition is valid
				validPhases := []string{"Pending", "Investigating", "Analyzing", "Completed", "Failed"}
				Expect(validPhases).To(ContainElement(oldPhase), "old_phase should be a valid phase")
				Expect(validPhases).To(ContainElement(newPhase), "new_phase should be a valid phase")
			}
		})

		It("should audit HolmesGPT-API calls with correct endpoint and status", func() {
			By("Creating AIAnalysis that will trigger HolmesGPT-API call")
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
							Severity:         "warning",
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
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal("Completed"))

		remediationID := analysis.Spec.RemediationID

		By("Waiting for HolmesGPT-API call events to appear in Data Storage")
		hapiEvents := waitForAuditEvents(httpClient, remediationID, "aianalysis.holmesgpt.call", 1)

		By("Validating HolmesGPT-API call event_data structure")
			for _, event := range hapiEvents {
				eventData, ok := event["event_data"].(map[string]interface{})
				Expect(ok).To(BeTrue())

				// Per DD-AUDIT-004: HolmesGPTCallPayload structure
				Expect(eventData).To(HaveKey("endpoint"), "Should record API endpoint called")
				Expect(eventData).To(HaveKey("http_status_code"), "Should record HTTP status code")
				Expect(eventData).To(HaveKey("duration_ms"), "Should record call duration")

			// Verify endpoint is valid
			endpoint := eventData["endpoint"].(string)
			Expect(endpoint).To(Or(Equal("/api/v1/incident/analyze"), Equal("/api/v1/recovery/investigate")),
				"Endpoint should be incident/analyze or recovery/investigate")

				// Verify HTTP status is 2xx for successful calls
				statusCode := int(eventData["http_status_code"].(float64))
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
							Severity:         "warning",
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
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal("Completed"))

		remediationID := analysis.Spec.RemediationID

		By("Waiting for Rego evaluation events to appear in Data Storage")
		regoEvents := waitForAuditEvents(httpClient, remediationID, "aianalysis.rego.evaluation", 1)

		By("Validating Rego evaluation event_data structure")
			event := regoEvents[0] // Should be only one Rego evaluation per analysis
			eventData, ok := event["event_data"].(map[string]interface{})
			Expect(ok).To(BeTrue())

		// Per DD-AUDIT-004: RegoEvaluationPayload structure
		Expect(eventData).To(HaveKey("outcome"), "Should record policy outcome (approved/requires_approval)")
		Expect(eventData).To(HaveKey("degraded"), "Should record if policy ran in degraded mode")
		Expect(eventData).To(HaveKey("duration_ms"), "Should record evaluation duration")
		Expect(eventData).To(HaveKey("reason"), "Should record evaluation reason")

		// Verify outcome is valid
		outcome := eventData["outcome"].(string)
		Expect([]string{"approved", "requires_approval"}).To(ContainElement(outcome),
			"Outcome should be 'approved' or 'requires_approval'")

			// Verify degraded flag is boolean
			degraded, ok := eventData["degraded"].(bool)
			Expect(ok).To(BeTrue(), "degraded should be a boolean")
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
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal("Completed"))

			By("Verifying approval is required for production")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
			Expect(analysis.Status.ApprovalRequired).To(BeTrue(),
				"Production environment should require approval per Rego policy")

		remediationID := analysis.Spec.RemediationID

		By("Waiting for approval decision events to appear in Data Storage")
		approvalEvents := waitForAuditEvents(httpClient, remediationID, "aianalysis.approval.decision", 1)

			By("Validating approval decision event_data structure")
			event := approvalEvents[0]
			eventData, ok := event["event_data"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			// Per DD-AUDIT-004: ApprovalDecisionPayload structure
			Expect(eventData).To(HaveKey("approval_required"), "Should record if approval is required")
			Expect(eventData).To(HaveKey("approval_reason"), "Should record reason for approval decision")
			Expect(eventData).To(HaveKey("auto_approved"), "Should record if auto-approved")

			// Verify approval_required matches CR status
			approvalRequired, ok := eventData["approval_required"].(bool)
			Expect(ok).To(BeTrue(), "approval_required should be a boolean")
			Expect(approvalRequired).To(BeTrue(),
				"audit event approval_required should match CR status.ApprovalRequired")

			// Verify auto_approved is false for production
			autoApproved, ok := eventData["auto_approved"].(bool)
			Expect(ok).To(BeTrue(), "auto_approved should be a boolean")
			Expect(autoApproved).To(BeFalse(),
				"Production should not be auto-approved")
		})
	})
})
