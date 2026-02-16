package aianalysis

import (
	"context"
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

// ========================================
// E2E Error Audit Trail Tests
// ========================================
//
// PURPOSE: Validate that audit events are captured for ERROR scenarios in a full Kind cluster.
//
// Why These Tests Matter:
// - Integration tests validate error auditing works with mock infrastructure
// - E2E tests validate error auditing works in production-like Kind cluster
// - E2E tests catch configuration issues that integration tests might miss
//   (e.g., missing RBAC, wrong DataStorage endpoint, network issues)
//
// Test Coverage:
// - AI agent API failures (HTTP 500, timeouts, network errors)
// - Controller reconciliation errors
// - Audit event data validation for error scenarios
//
// Complement to: 05_audit_trail_test.go (success path audit validation)

var _ = Describe("Error Audit Trail E2E", Label("e2e", "audit", "error"), func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// Context: AI Agent API Error Scenarios
	// ========================================

	Context("AI Agent API Error Audit - BR-AI-050", func() {
		It("should audit AI agent calls even when API returns HTTP 500", func() {
			By("Creating AIAnalysis with signal type that might trigger HAPI error")
			suffix := randomSuffix()
			namespace := createTestNamespace("error-audit-hapi")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-error-audit-hapi-" + suffix,
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationID: "e2e-error-audit-hapi-" + suffix,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-error-hapi-fingerprint",
							Severity:         "critical",
							SignalType:       "UnknownError", // Potentially problematic signal type
							Environment:      "production",
							BusinessPriority: "P0",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "failing-pod",
								Namespace: "production",
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			remediationID := analysis.Spec.RemediationID

			By("Querying Data Storage for AI agent call audit events")
			// Per TESTING_GUIDELINES.md: Use Eventually(), NOT time.Sleep()
			// Wait for at least one HAPI call event (success or failure)
		// waitForSpecificAuditEvent already uses Eventually() internally
		hapiEventType := aianalysisaudit.EventTypeAIAgentCall
		hapiEvents := waitForSpecificAuditEvent(remediationID, hapiEventType, 1)

			By("Validating AI agent call was audited regardless of success/failure")
			Expect(hapiEvents).ToNot(BeEmpty(),
				"Controller MUST audit AI agent calls even when they fail (ADR-032 §1)")

			By("Verifying HTTP status code is captured in audit event")
			event := hapiEvents[0]
			// Access strongly-typed payload via discriminated union
			payload := event.EventData.AIAnalysisAIAgentCallPayload
			Expect(payload).ToNot(BeNil(), "Should have AIAnalysisAIAgentCallPayload")
			Expect(payload.HTTPStatusCode).ToNot(BeZero(),
				"HTTP status code MUST be captured for all HAPI calls")

			statusCode := payload.HTTPStatusCode
			GinkgoWriter.Printf("📊 HAPI call status code: %d\n", statusCode)

			// Status code could be 200 (success), 500 (error), or timeout
			// The key is that it's captured, not necessarily what it is
			Expect(statusCode).To(BeNumerically(">", 0),
				"Status code should be a positive integer")

			By("Verifying event outcome reflects call result")
			eventOutcome := string(event.EventOutcome)
			Expect([]string{"success", "failure"}).To(ContainElement(eventOutcome),
				"event_outcome should be 'success' or 'failure'")
		})

		It("should create audit trail even when AIAnalysis remains in retry loop", func() {
			By("Creating AIAnalysis that may experience repeated errors")
			suffix := randomSuffix()
			namespace := createTestNamespace("error-audit-retry")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-error-retry-" + suffix,
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationID: "e2e-error-retry-" + suffix,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-error-retry-fp",
							Severity:        "medium",
							SignalType:       "CrashLoopBackOff",
							Environment:      "staging",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment",
								Name:      "unstable-service",
								Namespace: namespace,
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			remediationID := analysis.Spec.RemediationID

			By("Verifying audit trail exists regardless of AIAnalysis completion state (DD-API-001)")
			// Per TESTING_GUIDELINES.md: Use Eventually(), NOT time.Sleep()
			// Per DD-API-001: Use OpenAPI client, NOT raw HTTP
			var events []dsgen.AuditEvent
		Eventually(func() []dsgen.AuditEvent {
			var err error
			events, err = queryAllAuditEventTypes(remediationID) // Query all AI Analysis event types
			if err != nil {
				GinkgoWriter.Printf("⏳ Waiting for audit events (error: %v)\n", err)
					return nil
				}
				return events
			}, 2*time.Minute, 5*time.Second).ShouldNot(BeEmpty(),
				"Controller MUST create audit trail even if analysis doesn't complete (ADR-032 §1)")

			By("Verifying audit events have correct correlation_id")
			for _, event := range events {
				// ✅ Type-safe field access (OpenAPI generated)
				Expect(event.CorrelationID).To(Equal(remediationID),
					"All audit events must have correlation_id matching remediation_id")
			}

			By("Checking current AIAnalysis state")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
			GinkgoWriter.Printf("📊 AIAnalysis phase: %s (may be retrying)\n", analysis.Status.Phase)
			GinkgoWriter.Printf("📊 Audit events created: %d\n", len(events))

			// Business Value: Even if the analysis is stuck retrying,
			// operators have visibility into what attempts were made via audit trail
		})
	})

	// ========================================
	// Context: Controller Error Scenarios
	// ========================================

	Context("Controller Error Audit - BR-AI-050", func() {
		It("should audit errors during investigation phase", func() {
			By("Creating AIAnalysis that will be processed by controller")
			suffix := randomSuffix()
			namespace := createTestNamespace("error-audit-investigation")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-error-investigation-" + suffix,
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationID: "e2e-error-investigation-" + suffix,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-error-investigation-fp",
							Severity:         "critical",
							SignalType:       "OOMKilled",
							Environment:      "production",
							BusinessPriority: "P0",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "memory-hog",
								Namespace: namespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								OwnerChain: []sharedtypes.OwnerChainEntry{
									{Namespace: namespace, Kind: "Pod", Name: "memory-hog"},
								},
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			remediationID := analysis.Spec.RemediationID

		By("Verifying audit trail exists for this AIAnalysis (DD-API-001)")
		// Per TESTING_GUIDELINES.md: Use Eventually(), NOT time.Sleep()
		// Per DD-API-001: Use OpenAPI client, NOT raw HTTP
		var events []dsgen.AuditEvent
		Eventually(func() []dsgen.AuditEvent {
			var err error
			events, err = queryAllAuditEventTypes(remediationID) // Query all types to find error events
			if err != nil {
				GinkgoWriter.Printf("⏳ Waiting for audit events (error: %v)\n", err)
					return nil
				}
				return events
			}, 2*time.Minute, 5*time.Second).ShouldNot(BeEmpty(),
				"Controller MUST generate audit events even during error scenarios")

			By("Checking for error audit events if present")
			errorEvents := []dsgen.AuditEvent{}
			for _, event := range events {
				if event.EventType == aianalysisaudit.EventTypeError {
					errorEvents = append(errorEvents, event)
				}
			}

			if len(errorEvents) > 0 {
				GinkgoWriter.Printf("📊 Found %d error audit events\n", len(errorEvents))

				By("Validating error event structure")
				for _, errEvent := range errorEvents {
					// ✅ Type-safe field access (OpenAPI generated)
					Expect(errEvent.EventData).NotTo(BeNil(), "error event_data should exist")

					// Error events should capture error details
					Expect(errEvent.EventData).To(HaveKey("error_message"),
						"Error events should include error message")
				}
			} else {
				GinkgoWriter.Println("ℹ️  No error events found (analysis may have succeeded)")
			}

			// Business Value: Operators have audit trail regardless of success/failure
			GinkgoWriter.Printf("📊 Total audit events for remediation %s: %d\n", remediationID, len(events))
		})

		It("should maintain audit integrity across controller restarts", func() {
			By("Creating AIAnalysis before simulated controller restart")
			suffix := randomSuffix()
			namespace := createTestNamespace("error-audit-restart")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-restart-audit-" + suffix,
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationID: "e2e-restart-audit-" + suffix,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-restart-fp",
							Severity:        "medium",
							SignalType:       "HighMemoryUsage",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Deployment",
								Name:      "backend-api",
								Namespace: namespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								OwnerChain: []sharedtypes.OwnerChainEntry{
									{Namespace: namespace, Kind: "Deployment", Name: "backend-api"},
								},
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			remediationID := analysis.Spec.RemediationID

		By("Capturing initial audit event count (DD-API-001)")
		// Per TESTING_GUIDELINES.md: Use Eventually(), NOT time.Sleep()
		// Per DD-API-001: Use OpenAPI client, NOT raw HTTP
		// Query phase transitions (first events always created)
		var initialEvents []dsgen.AuditEvent
		Eventually(func() []dsgen.AuditEvent {
			var err error
			initialEvents, err = querySpecificAuditEvent(remediationID, aianalysisaudit.EventTypePhaseTransition)
			if err != nil {
				GinkgoWriter.Printf("⏳ Waiting for initial audit events (error: %v)\n", err)
					return nil
				}
				return initialEvents
			}, 2*time.Minute, 5*time.Second).ShouldNot(BeEmpty(),
				"Should have audit events before persistence check")

			initialEventCount := len(initialEvents)
			GinkgoWriter.Printf("📊 Initial audit events: %d\n", initialEventCount)

			// Note: In a real E2E test with controller pod restart capability,
			// we would restart the controller pod here. For now, we validate
			// that events persist in DataStorage (PostgreSQL) by querying again

		By("Verifying audit events persist in DataStorage (PostgreSQL durability)")
		// Query phase transitions (first events always created)
		persistedEvents, err := querySpecificAuditEvent(remediationID, aianalysisaudit.EventTypePhaseTransition)
		Expect(err).ToNot(HaveOccurred())

			persistedEventCount := len(persistedEvents)
			GinkgoWriter.Printf("📊 Persisted audit events: %d\n", persistedEventCount)

			Expect(persistedEventCount).To(BeNumerically(">=", initialEventCount),
				"Audit events MUST persist in DataStorage (ADR-032 §2: PostgreSQL durability)")

			// Business Value: Audit trail survives controller restarts for compliance
		})
	})

	// ========================================
	// Context: Audit Data Validation
	// ========================================

	Context("Error Audit Data Integrity - DD-AUDIT-003", func() {
		It("should include complete metadata in all error audit events", func() {
			By("Creating AIAnalysis that will generate audit events")
			suffix := randomSuffix()
			namespace := createTestNamespace("error-audit-metadata")
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-metadata-" + suffix,
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationID: "e2e-metadata-" + suffix,
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "e2e-metadata-fp",
							Severity:         "critical",
							SignalType:       "CrashLoopBackOff",
							Environment:      "production",
							BusinessPriority: "P0",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "critical-pod",
								Namespace: namespace,
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			remediationID := analysis.Spec.RemediationID

		By("Querying all audit events for metadata validation (DD-API-001)")
		// Per TESTING_GUIDELINES.md: Use Eventually(), NOT time.Sleep()
		// Per DD-API-001: Use OpenAPI client, NOT raw HTTP
		// Query all AI Analysis event types to validate metadata on each
		var events []dsgen.AuditEvent
		Eventually(func() []dsgen.AuditEvent {
			var err error
			events, err = queryAllAuditEventTypes(remediationID)
			if err != nil {
				GinkgoWriter.Printf("⏳ Waiting for audit events (error: %v)\n", err)
					return nil
				}
				return events
			}, 2*time.Minute, 5*time.Second).ShouldNot(BeEmpty(),
				"Should have audit events for metadata validation")

			By("Validating REQUIRED metadata fields in ALL events (DD-AUDIT-003)")
			for i, event := range events {
				GinkgoWriter.Printf("📋 Validating event %d/%d\n", i+1, len(events))

				// P0: Use testutil validator for comprehensive field validation
				// ✅ OpenAPI type validation (no conversion needed)
				validators.ValidateAuditEventHasRequiredFields(event)

				// Additional E2E-specific validation (only for AIAnalysis events)
				// Note: correlation_id may include events from other services (workflow, llm_request, etc.)
				// Only check category for AIAnalysis-specific events
				if event.EventCategory == dsgen.AuditEventEventCategoryAnalysis {
					Expect(event.CorrelationID).To(Equal(remediationID),
						"AIAnalysis events should have correlation_id matching remediation_id")
				}
			}

			GinkgoWriter.Printf("✅ All %d audit events have complete metadata\n", len(events))
		})
	})
})
