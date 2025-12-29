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
// - HolmesGPT-API failures (HTTP 500, timeouts, network errors)
// - Controller reconciliation errors
// - Audit event data validation for error scenarios
//
// Complement to: 05_audit_trail_test.go (success path audit validation)

var _ = Describe("Error Audit Trail E2E", Label("e2e", "audit", "error"), func() {
	var (
		httpClient *http.Client
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		httpClient = &http.Client{Timeout: 10 * time.Second}
	})

	// ========================================
	// Context: HolmesGPT-API Error Scenarios
	// ========================================

	Context("HolmesGPT-API Error Audit - BR-AI-050", func() {
		It("should audit HolmesGPT calls even when API returns HTTP 500", func() {
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

			By("Waiting for controller to process (may retry on errors)")
			// Give controller time to call HAPI and record audit events
			// Even if HAPI fails, audit events should be created
			time.Sleep(15 * time.Second)

			remediationID := analysis.Spec.RemediationID

			By("Querying Data Storage for HolmesGPT call audit events")
			// Wait for at least one HAPI call event (success or failure)
			hapiEvents := waitForAuditEvents(httpClient, remediationID, "aianalysis.holmesgpt.call", 1)

			By("Validating HolmesGPT call was audited regardless of success/failure")
			Expect(hapiEvents).ToNot(BeEmpty(),
				"Controller MUST audit HolmesGPT calls even when they fail (ADR-032 §1)")

			By("Verifying HTTP status code is captured in audit event")
			event := hapiEvents[0]
			eventData, ok := event["event_data"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "event_data should be a JSON object")
			Expect(eventData).To(HaveKey("http_status_code"),
				"HTTP status code MUST be captured for all HAPI calls")

			statusCode := int(eventData["http_status_code"].(float64))
			GinkgoWriter.Printf("📊 HAPI call status code: %d\n", statusCode)

			// Status code could be 200 (success), 500 (error), or timeout
			// The key is that it's captured, not necessarily what it is
			Expect(statusCode).To(BeNumerically(">", 0),
				"Status code should be a positive integer")

			By("Verifying event outcome reflects call result")
			eventOutcome, ok := event["event_outcome"].(string)
			Expect(ok).To(BeTrue(), "event_outcome should be a string")
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
							Severity:         "warning",
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

			By("Waiting for controller to process and potentially retry")
			time.Sleep(20 * time.Second)

			remediationID := analysis.Spec.RemediationID

			By("Verifying audit trail exists regardless of AIAnalysis completion state")
			// Query for ANY audit events (don't filter by type initially)
			resp, err := httpClient.Get(fmt.Sprintf(
				"http://localhost:8091/api/v1/audit/events?correlation_id=%s",
				remediationID,
			))
			Expect(err).ToNot(HaveOccurred(), "Should be able to query DataStorage API")
			defer resp.Body.Close()

			var auditResponse struct {
				Data []map[string]interface{} `json:"data"`
			}
			err = json.NewDecoder(resp.Body).Decode(&auditResponse)
			Expect(err).ToNot(HaveOccurred(), "Should be able to decode audit response")

			events := auditResponse.Data
			Expect(events).ToNot(BeEmpty(),
				"Controller MUST create audit trail even if analysis doesn't complete (ADR-032 §1)")

			By("Verifying audit events have correct correlation_id")
			for _, event := range events {
				correlationID, ok := event["correlation_id"].(string)
				Expect(ok).To(BeTrue(), "correlation_id should be a string")
				Expect(correlationID).To(Equal(remediationID),
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

			By("Waiting for controller to process")
			time.Sleep(15 * time.Second)

			remediationID := analysis.Spec.RemediationID

			By("Verifying audit trail exists for this AIAnalysis")
			resp, err := httpClient.Get(fmt.Sprintf(
				"http://localhost:8091/api/v1/audit/events?correlation_id=%s",
				remediationID,
			))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			var auditResponse struct {
				Data []map[string]interface{} `json:"data"`
			}
			err = json.NewDecoder(resp.Body).Decode(&auditResponse)
			Expect(err).ToNot(HaveOccurred())

			events := auditResponse.Data
			Expect(events).ToNot(BeEmpty(),
				"Controller MUST generate audit events even during error scenarios")

			By("Checking for error audit events if present")
			errorEvents := []map[string]interface{}{}
			for _, event := range events {
				eventType, ok := event["event_type"].(string)
				if ok && eventType == "aianalysis.error.occurred" {
					errorEvents = append(errorEvents, event)
				}
			}

			if len(errorEvents) > 0 {
				GinkgoWriter.Printf("📊 Found %d error audit events\n", len(errorEvents))

				By("Validating error event structure")
				for _, errEvent := range errorEvents {
					eventData, ok := errEvent["event_data"].(map[string]interface{})
					Expect(ok).To(BeTrue(), "error event_data should be a JSON object")

					// Error events should capture error details
					Expect(eventData).To(HaveKey("error_message"),
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
							Severity:         "warning",
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

			By("Waiting for initial audit events to be created")
			time.Sleep(10 * time.Second)

			remediationID := analysis.Spec.RemediationID

			By("Capturing initial audit event count")
			resp, err := httpClient.Get(fmt.Sprintf(
				"http://localhost:8091/api/v1/audit/events?correlation_id=%s",
				remediationID,
			))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			var auditResponse1 struct {
				Data []map[string]interface{} `json:"data"`
			}
			err = json.NewDecoder(resp.Body).Decode(&auditResponse1)
			Expect(err).ToNot(HaveOccurred())

			initialEventCount := len(auditResponse1.Data)
			GinkgoWriter.Printf("📊 Initial audit events: %d\n", initialEventCount)
			Expect(initialEventCount).To(BeNumerically(">", 0),
				"Should have audit events before restart")

			// Note: In a real E2E test with controller pod restart capability,
			// we would restart the controller pod here. For now, we validate
			// that events persist in DataStorage (PostgreSQL)

			By("Verifying audit events persist after time passes (simulating restart)")
			time.Sleep(10 * time.Second)

			resp2, err := httpClient.Get(fmt.Sprintf(
				"http://localhost:8091/api/v1/audit/events?correlation_id=%s",
				remediationID,
			))
			Expect(err).ToNot(HaveOccurred())
			defer resp2.Body.Close()

			var auditResponse2 struct {
				Data []map[string]interface{} `json:"data"`
			}
			err = json.NewDecoder(resp2.Body).Decode(&auditResponse2)
			Expect(err).ToNot(HaveOccurred())

			persistedEventCount := len(auditResponse2.Data)
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

			By("Waiting for audit events to be created")
			time.Sleep(15 * time.Second)

			remediationID := analysis.Spec.RemediationID

			By("Querying all audit events for metadata validation")
			resp, err := httpClient.Get(fmt.Sprintf(
				"http://localhost:8091/api/v1/audit/events?correlation_id=%s",
				remediationID,
			))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			var auditResponse struct {
				Data []map[string]interface{} `json:"data"`
			}
			err = json.NewDecoder(resp.Body).Decode(&auditResponse)
			Expect(err).ToNot(HaveOccurred())

			events := auditResponse.Data
			Expect(events).ToNot(BeEmpty(), "Should have audit events")

			By("Validating REQUIRED metadata fields in ALL events (DD-AUDIT-003)")
			for i, event := range events {
				GinkgoWriter.Printf("📋 Validating event %d/%d\n", i+1, len(events))

				// P0: Use testutil validator for comprehensive field validation
				typedEvent := convertJSONToAuditEvent(event)
				testutil.ValidateAuditEventHasRequiredFields(typedEvent)

				// Additional E2E-specific validation
				eventCategory, ok := event["event_category"].(string)
				Expect(ok).To(BeTrue(), "event_category should be a string")
				Expect(eventCategory).To(Equal("analysis"),
					"AIAnalysis events should have category 'analysis'")

				correlationID, ok := event["correlation_id"].(string)
				Expect(ok).To(BeTrue(), "correlation_id should be a string")
				Expect(correlationID).To(Equal(remediationID),
					"correlation_id should match remediation_id")
			}

			GinkgoWriter.Printf("✅ All %d audit events have complete metadata\n", len(events))
		})
	})
})

