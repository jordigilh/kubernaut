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

package signalprocessing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	"github.com/jordigilh/kubernaut/pkg/testutil"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// =============================================================================
// BR-SP-090: SignalProcessing → Data Storage Audit Integration Tests
// =============================================================================
//
// Business Requirements:
// - BR-SP-090: All categorization decisions MUST generate audit events
// - BR-AUDIT-001: All service operations MUST generate audit events
// - BR-AUDIT-002: Audit events MUST be persisted to Data Storage
//
// Test Strategy:
// - Per TESTING_GUIDELINES.md: Integration tests use REAL infrastructure
// - SignalProcessing connects to REAL Data Storage (via podman-compose)
// - Tests verify audit events appear in Data Storage database
// - Tests validate audit event content matches controller operations
//
// Audit Events Tested:
// - signalprocessing.signal.processed: Main completion event
// - signalprocessing.phase.transition: Phase change events
// - signalprocessing.classification.decision: Classification results
// - signalprocessing.enrichment.completed: K8s enrichment completion
// - signalprocessing.error.occurred: Error tracking
//
// To run these tests:
//   go test ./test/integration/signalprocessing/... --ginkgo.focus="Audit Integration"
//
// =============================================================================

var _ = Describe("BR-SP-090: SignalProcessing → Data Storage Audit Integration", func() {
	var (
		dataStorageURL string
	)

	BeforeEach(func() {
		// DataStorage URL from suite's shared infrastructure (port 18094)
		dataStorageURL = fmt.Sprintf("http://localhost:%d", infrastructure.SignalProcessingIntegrationDataStoragePort)

		// Verify Data Storage is running
		healthResp, err := http.Get(dataStorageURL + "/health")
		if err != nil {
			Fail(fmt.Sprintf(
				"REQUIRED: Data Storage not available at %s\n"+
					"  Per BR-SP-090: SignalProcessing MUST have audit capability\n"+
					"  Per TESTING_GUIDELINES.md: Integration tests MUST use real services\n\n"+
					"  Start with: podman-compose -f test/integration/signalprocessing/podman-compose.signalprocessing.test.yml up -d\n\n"+
					"  Error: %v", dataStorageURL, err))
		}
		defer healthResp.Body.Close()
		if healthResp.StatusCode != http.StatusOK {
			Fail(fmt.Sprintf(
				"REQUIRED: Data Storage health check failed at %s\n"+
					"  Status: %d\n"+
					"  Expected: 200 OK", dataStorageURL, healthResp.StatusCode))
		}
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// SIGNAL PROCESSING COMPLETION AUDITING (BR-SP-090)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when signal processing completes successfully (BR-SP-090)", func() {
		It("should create 'signalprocessing.signal.processed' audit event in Data Storage", func() {
			// BUSINESS SCENARIO:
			// When SignalProcessing controller completes signal enrichment and classification:
			// 1. Environment classification (production/staging/dev)
			// 2. Priority assignment (P0/P1/P2/P3)
			// 3. Business classification (criticality, SLA)
			// 4. MUST emit audit event to Data Storage for compliance tracking
			//
			// COMPLIANCE: SOC2, HIPAA require audit trails for all categorization decisions

			By("1. Creating production namespace with environment label")
			ns := createTestNamespaceWithLabels("audit-test-prod", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("2. Creating test pod")
			podLabels := map[string]string{"app": "payment-service"}
			_ = createTestPod(ns, "payment-pod-audit-01", podLabels, nil)

			By("3. Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "payment-pod-audit-01",
				Namespace: ns,
			}
			rrName := "audit-test-rr-01"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["audit-001"], "critical", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Use RR name as correlation ID (production pattern)
			correlationID := rrName

			By("4. Creating SignalProcessing CR with parent RR")
			sp := CreateTestSignalProcessingWithParent("audit-test-sp-01", ns, rr, ValidTestFingerprints["audit-001"], targetResource)
			sp.Spec.Signal.Severity = "critical"
			sp.Spec.Signal.Name = "HighMemoryUsage"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			By("5. Wait for processing to complete")
			Eventually(func() signalprocessingv1alpha1.SignalProcessingPhase {
				var updated signalprocessingv1alpha1.SignalProcessing
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(signalprocessingv1alpha1.PhaseCompleted))

			By("6. Query Data Storage for 'signal.processed' audit event via OpenAPI client")
			// V1.0 MANDATORY: Use OpenAPI client instead of raw HTTP (per V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
			auditClient, err := dsgen.NewClientWithResponses(dataStorageURL)
			Expect(err).ToNot(HaveOccurred(), "Failed to create OpenAPI audit client")

			eventCategory := "signalprocessing"
			var auditEvents []dsgen.AuditEvent
			// WORKAROUND: 90s timeout for DataStorage buffer flush bug
			// Expected: 2-5s for audit events to appear (1s flush interval)
			// Actual: 60-90s due to timer not firing in pkg/audit/store.go backgroundWriter
			// See: DATASTORAGE_AUDIT_BUFFER_FLUSH_TIMING_ISSUE.md
			// This timeout matches RemediationOrchestrator pattern and proves bug affects SignalProcessing
			Eventually(func() int {
				resp, err := auditClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
					EventCategory: &eventCategory,
					CorrelationId: &correlationID,
				})
				if err != nil {
					GinkgoWriter.Printf("Failed to query audit events: %v\n", err)
					return 0
				}

				if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
					GinkgoWriter.Printf("Audit query returned status %d\n", resp.StatusCode())
					return 0
				}

				if resp.JSON200.Data != nil {
					auditEvents = *resp.JSON200.Data
				}
				if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
					return *resp.JSON200.Pagination.Total
				}
				return 0
			}, 90*time.Second, 500*time.Millisecond).Should(Equal(1),
				"BR-SP-090: SignalProcessing MUST emit exactly 1 signal.processed event per processing completion")

			By("7. Find 'signal.processed' audit event")
			var processedEvent *dsgen.AuditEvent
			for i := range auditEvents {
				if auditEvents[i].EventType == "signalprocessing.signal.processed" {
					processedEvent = &auditEvents[i]
					break
				}
			}
			Expect(processedEvent).ToNot(BeNil(), "Should have 'signal.processed' audit event")

			By("8. Validate audit event using testutil.ValidateAuditEvent (V1.0 MANDATORY)")
			// V1.0 MANDATORY: Use testutil.ValidateAuditEvent for type-safe validation
			actorType := "service"
			actorID := "signalprocessing-controller"
			testutil.ValidateAuditEvent(*processedEvent, testutil.ExpectedAuditEvent{
				EventType:     "signalprocessing.signal.processed",
				EventCategory: dsgen.AuditEventEventCategorySignalprocessing,
				EventAction:   "processed",
				EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
				CorrelationID: correlationID,
				ActorType:     &actorType,
				ActorID:       &actorID,
				EventDataFields: map[string]interface{}{
					"environment": "production",
					"priority":    "P0",
				},
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// CLASSIFICATION DECISION AUDITING (BR-SP-090)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when classification decision is made (BR-SP-090)", func() {
		It("should create 'classification.decision' audit event with all categorization results", func() {
			// BUSINESS SCENARIO:
			// Classification decision includes:
			// - Environment: production/staging/development
			// - Priority: P0/P1/P2/P3
			// - Business criticality, SLA requirements
			// Each decision MUST be audited with confidence scores

			By("1. Creating staging namespace")
			ns := createTestNamespaceWithLabels("audit-test-staging", map[string]string{
				"kubernaut.ai/environment": "staging",
			})
			defer deleteTestNamespace(ns)

			By("2. Creating test deployment")
			depLabels := map[string]string{"app": "api-service"}
			_ = createTestDeployment(ns, "api-deployment-audit-02", depLabels)

			By("3. Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "api-deployment-audit-02",
				Namespace: ns,
			}
			rrName := "audit-test-rr-02"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["audit-002"], "warning", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			correlationID := rrName

			By("4. Creating SignalProcessing CR")
			sp := CreateTestSignalProcessingWithParent("audit-test-sp-02", ns, rr, ValidTestFingerprints["audit-002"], targetResource)
			sp.Spec.Signal.Severity = "warning"
			sp.Spec.Signal.Name = "HighCPUUsage"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			By("5. Wait for processing to complete")
			Eventually(func() signalprocessingv1alpha1.SignalProcessingPhase {
				var updated signalprocessingv1alpha1.SignalProcessing
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(signalprocessingv1alpha1.PhaseCompleted))

			By("6. Query Data Storage for 'classification.decision' audit event via OpenAPI client")
			// V1.0 MANDATORY: Use OpenAPI client instead of raw HTTP
			auditClient, err := dsgen.NewClientWithResponses(dataStorageURL)
			Expect(err).ToNot(HaveOccurred())

			eventType := "signalprocessing.classification.decision"
			var auditEvents []dsgen.AuditEvent
			// WORKAROUND: 90s timeout for DataStorage buffer flush bug (see DATASTORAGE_AUDIT_BUFFER_FLUSH_TIMING_ISSUE.md)
			Eventually(func() int {
				resp, err := auditClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
					EventType:     &eventType,
					CorrelationId: &correlationID,
				})
				if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
					return 0
				}

				if resp.JSON200.Data != nil {
					auditEvents = *resp.JSON200.Data
				}
				if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
					return *resp.JSON200.Pagination.Total
				}
				return 0
			}, 90*time.Second, 500*time.Millisecond).Should(Equal(1),
				"BR-SP-090: SignalProcessing MUST emit exactly 1 classification.decision event per classification")

			By("7. Validate classification audit event using testutil.ValidateAuditEvent")
			Expect(len(auditEvents)).To(Equal(1), "Should have exactly 1 classification event")
			testutil.ValidateAuditEvent(auditEvents[0], testutil.ExpectedAuditEvent{
				EventType:     "signalprocessing.classification.decision",
				EventCategory: dsgen.AuditEventEventCategorySignalprocessing,
				EventAction:   "classification",
				EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
				CorrelationID: correlationID,
				EventDataFields: map[string]interface{}{
					"environment": "staging",
					"priority":    "P2",
				},
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BUSINESS CLASSIFICATION AUDITING (AUDIT-06, BR-SP-002)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when business classification is applied (AUDIT-06, BR-SP-002)", func() {
		It("should create 'business.classified' audit event with criticality and SLA", func() {
			// BUSINESS SCENARIO:
			// Business classification assigns:
			// - BusinessUnit: Team ownership (e.g., payments, platform)
			// - Criticality: Business impact level
			// - SLA: Service level agreement requirements
			// Each business classification MUST be audited for compliance
			//
			// INTEGRATION PLAN: AUDIT-06 per integration-test-plan.md v1.1.0

			By("1. Creating namespace with business classification labels")
			ns := createTestNamespaceWithLabels("audit-test-business", map[string]string{
				"kubernaut.ai/environment": "production",
				"kubernaut.ai/team":        "payments",
			})
			defer deleteTestNamespace(ns)

			By("2. Creating test deployment")
			depLabels := map[string]string{"app": "payment-gateway"}
			_ = createTestDeployment(ns, "payment-deploy-audit-06", depLabels)

			By("3. Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "payment-deploy-audit-06",
				Namespace: ns,
			}
			rrName := "audit-test-rr-06"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["audit-006"], "critical", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			correlationID := rrName

			By("4. Creating SignalProcessing CR")
			sp := CreateTestSignalProcessingWithParent("audit-test-sp-06", ns, rr, ValidTestFingerprints["audit-006"], targetResource)
			sp.Spec.Signal.Severity = "critical"
			sp.Spec.Signal.Name = "PaymentServiceDown"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			By("5. Wait for processing to complete")
			Eventually(func() signalprocessingv1alpha1.SignalProcessingPhase {
				var updated signalprocessingv1alpha1.SignalProcessing
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(signalprocessingv1alpha1.PhaseCompleted))

		By("6. Query Data Storage for 'business.classified' audit event via OpenAPI client")
		// V1.0 MANDATORY: Use OpenAPI client instead of raw HTTP
		auditClient, err := dsgen.NewClientWithResponses(dataStorageURL)
		Expect(err).ToNot(HaveOccurred())

		eventType := "signalprocessing.business.classified"
		var auditEvents []dsgen.AuditEvent
		// WORKAROUND: 90s timeout for DataStorage buffer flush bug (see DATASTORAGE_AUDIT_BUFFER_FLUSH_TIMING_ISSUE.md)
		Eventually(func() int {
			resp, err := auditClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
				EventType:     &eventType,
				CorrelationId: &correlationID,
			})
			if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
				return 0
			}

			if resp.JSON200.Data != nil {
				auditEvents = *resp.JSON200.Data
			}
			if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
				return *resp.JSON200.Pagination.Total
			}
			return 0
		}, 90*time.Second, 500*time.Millisecond).Should(Equal(1),
			"AUDIT-06: SignalProcessing MUST emit exactly 1 business.classified event per business classification")

			By("7. Validate business classification audit event using testutil.ValidateAuditEvent")
			Expect(len(auditEvents)).To(Equal(1), "Should have exactly 1 business classification event")
			testutil.ValidateAuditEvent(auditEvents[0], testutil.ExpectedAuditEvent{
				EventType:     "signalprocessing.business.classified",
				EventCategory: dsgen.AuditEventEventCategorySignalprocessing,
				EventAction:   "classification",
				EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
				CorrelationID: correlationID,
				EventDataFields: map[string]interface{}{
					"business_unit": "payments",
				},
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// ENRICHMENT COMPLETION AUDITING (BR-SP-090)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when K8s enrichment completes (BR-SP-090)", func() {
		It("should create 'enrichment.completed' audit event with enrichment details", func() {
			// BUSINESS SCENARIO:
			// K8s enrichment gathers:
			// - Namespace details
			// - Pod/Deployment details
			// - Owner chain information
			// - Detected labels (PDB, HPA, etc.)
			// Completion MUST be audited with performance metrics

			By("1. Creating development namespace")
			ns := createTestNamespaceWithLabels("audit-test-dev", map[string]string{
				"kubernaut.ai/environment": "development",
			})
			defer deleteTestNamespace(ns)

			By("2. Creating test pod with owner chain")
			podLabels := map[string]string{"app": "worker"}
			deployment := createTestDeployment(ns, "worker-deployment", podLabels)

			// Create ReplicaSet (owner chain: Pod → ReplicaSet → Deployment)
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker-rs-xyz",
					Namespace: ns,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       deployment.Name,
							UID:        deployment.UID,
						},
					},
				},
				Spec: appsv1.ReplicaSetSpec{
					Selector: deployment.Spec.Selector,
					Template: deployment.Spec.Template,
				},
			}
			Expect(k8sClient.Create(ctx, rs)).To(Succeed())

			// Create Pod with ReplicaSet owner
			pod := createTestPod(ns, "worker-pod-audit-03", podLabels, []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       rs.Name,
					UID:        rs.UID,
				},
			})

			By("3. Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      pod.Name,
				Namespace: ns,
			}
			rrName := "audit-test-rr-03"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["audit-003"], "info", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			correlationID := rrName

			By("4. Creating SignalProcessing CR")
			sp := CreateTestSignalProcessingWithParent("audit-test-sp-03", ns, rr, ValidTestFingerprints["audit-003"], targetResource)
			sp.Spec.Signal.Severity = "info"
			sp.Spec.Signal.Name = "PodRestart"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			By("5. Wait for processing to complete")
			Eventually(func() signalprocessingv1alpha1.SignalProcessingPhase {
				var updated signalprocessingv1alpha1.SignalProcessing
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(signalprocessingv1alpha1.PhaseCompleted))

			By("6. Query Data Storage for 'enrichment.completed' audit event via OpenAPI client")
			// V1.0 MANDATORY: Use OpenAPI client instead of raw HTTP
			auditClient, err := dsgen.NewClientWithResponses(dataStorageURL)
			Expect(err).ToNot(HaveOccurred())

			eventType := "signalprocessing.enrichment.completed"
			var auditEvents []dsgen.AuditEvent
			// WORKAROUND: 90s timeout for DataStorage buffer flush bug (see DATASTORAGE_AUDIT_BUFFER_FLUSH_TIMING_ISSUE.md)
			Eventually(func() int {
				resp, err := auditClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
					EventType:     &eventType,
					CorrelationId: &correlationID,
				})
				if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
					return 0
				}

				if resp.JSON200.Data != nil {
					auditEvents = *resp.JSON200.Data
				}
				if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
					return *resp.JSON200.Pagination.Total
				}
				return 0
			}, 90*time.Second, 500*time.Millisecond).Should(Equal(1),
				"BR-SP-090: SignalProcessing MUST emit exactly 1 enrichment.completed event per enrichment operation")

			By("7. Validate enrichment audit event using testutil.ValidateAuditEvent")
			Expect(len(auditEvents)).To(Equal(1), "Should have exactly 1 enrichment audit event")

			// Debug: Print found events
			GinkgoWriter.Printf("\n📊 Found %d audit events\n", len(auditEvents))

			// V1.0 MANDATORY: Use testutil.ValidateAuditEvent for type-safe validation
			testutil.ValidateAuditEvent(auditEvents[0], testutil.ExpectedAuditEvent{
				EventType:     "signalprocessing.enrichment.completed",
				EventCategory: dsgen.AuditEventEventCategorySignalprocessing,
				EventAction:   "enrichment",
				EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
				CorrelationID: correlationID,
				EventDataFields: map[string]interface{}{
					"has_namespace": true,
					"has_pod":       true,
					"degraded_mode": false,
				},
			})

			// Additional assertions for enrichment-specific fields
			Expect(auditEvents[0].DurationMs).ToNot(BeNil(),
				"Should capture enrichment duration for performance tracking")
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PHASE TRANSITION AUDITING (BR-SP-090)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when phase transitions occur (BR-SP-090)", func() {
		It("should create 'phase.transition' audit events for each phase change", func() {
			// BUSINESS SCENARIO:
			// SignalProcessing goes through phases:
			// Pending → Enriching → Classifying → Categorizing → Completed
			// Each transition MUST be audited for workflow tracking

			By("1. Creating test namespace")
			ns := createTestNamespaceWithLabels("audit-test-phase", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("2. Creating test pod")
			podLabels := map[string]string{"app": "test"}
			_ = createTestPod(ns, "test-pod-audit-04", podLabels, nil)

			By("3. Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "test-pod-audit-04",
				Namespace: ns,
			}
			rrName := "audit-test-rr-04"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["audit-004"], "warning", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			correlationID := rrName

			By("4. Creating SignalProcessing CR")
			sp := CreateTestSignalProcessingWithParent("audit-test-sp-04", ns, rr, ValidTestFingerprints["audit-004"], targetResource)
			sp.Spec.Signal.Severity = "warning"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			By("5. Wait for processing to complete")
			Eventually(func() signalprocessingv1alpha1.SignalProcessingPhase {
				var updated signalprocessingv1alpha1.SignalProcessing
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(signalprocessingv1alpha1.PhaseCompleted))

			By("6. Query Data Storage for 'phase.transition' audit events via OpenAPI client")
			// V1.0 MANDATORY: Use OpenAPI client instead of raw HTTP
			auditClient, err := dsgen.NewClientWithResponses(dataStorageURL)
			Expect(err).ToNot(HaveOccurred())

			eventType := "signalprocessing.phase.transition"
			var auditEvents []dsgen.AuditEvent
			// WORKAROUND: 90s timeout for DataStorage buffer flush bug (see DATASTORAGE_AUDIT_BUFFER_FLUSH_TIMING_ISSUE.md)
			Eventually(func() int {
				resp, err := auditClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
					EventType:     &eventType,
					CorrelationId: &correlationID,
				})
				if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
					return 0
				}

				if resp.JSON200.Data != nil {
					auditEvents = *resp.JSON200.Data
				}
				if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
					return *resp.JSON200.Pagination.Total
				}
				return 0
			}, 90*time.Second, 500*time.Millisecond).Should(Equal(4),
				"BR-SP-090: SignalProcessing MUST emit exactly 4 phase.transition events: Pending→Enriching, Enriching→Classifying, Classifying→Categorizing, Categorizing→Completed")

			By("7. Validate phase transition audit events using testutil.ValidateAuditEvent")
			Expect(len(auditEvents)).To(Equal(4), "Should have exactly 4 phase transition events")

			// V1.0 MANDATORY: Use testutil.ValidateAuditEvent for type-safe validation
			testutil.ValidateAuditEvent(auditEvents[0], testutil.ExpectedAuditEvent{
				EventType:     "signalprocessing.phase.transition",
				EventCategory: dsgen.AuditEventEventCategorySignalprocessing,
				EventAction:   "phase_transition",
				EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
				CorrelationID: correlationID,
			})

			// Verify event_data contains phase information
			testutil.ValidateAuditEventDataNotEmpty(auditEvents[0])
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// ERROR AUDITING (BR-SP-090, ADR-038)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("when errors occur during processing (BR-SP-090, ADR-038)", func() {
		It("should create 'error.occurred' audit event with error details", func() {
			// BUSINESS SCENARIO:
			// Errors during signal processing:
			// - K8s enrichment failures (API unavailable, RBAC denied)
			// - Classification failures (Rego policy errors)
			// - Phase transition failures
			// Errors MUST be audited for debugging and incident response
			//
			// ADR-038: Audit failures MUST NOT block reconciliation

			By("1. Creating test namespace")
			ns := createTestNamespaceWithLabels("audit-test-error", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("2. Creating parent RemediationRequest")
			// Target non-existent pod to trigger enrichment error
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "non-existent-pod-audit-05",
				Namespace: ns,
			}
			rrName := "audit-test-rr-05"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["audit-005"], "critical", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			correlationID := rrName

			By("3. Creating SignalProcessing CR with non-existent target")
			sp := CreateTestSignalProcessingWithParent("audit-test-sp-05", ns, rr, ValidTestFingerprints["audit-005"], targetResource)
			sp.Spec.Signal.Severity = "critical"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			By("4. Wait for processing attempt to reach degraded mode or failed phase")
			Eventually(func() signalprocessingv1alpha1.SignalProcessingPhase {
				var updated signalprocessingv1alpha1.SignalProcessing
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).ShouldNot(Equal(signalprocessingv1alpha1.PhasePending),
				"SignalProcessing should leave Pending phase even with errors (degraded mode)")

			By("5. Query Data Storage for error audit events via OpenAPI client")
			// V1.0 MANDATORY: Use OpenAPI client instead of raw HTTP
			auditClient, err := dsgen.NewClientWithResponses(dataStorageURL)
			Expect(err).ToNot(HaveOccurred())

			eventCategory := "signalprocessing"
			var auditEvents []dsgen.AuditEvent
			// WORKAROUND: 90s timeout for DataStorage buffer flush bug (see DATASTORAGE_AUDIT_BUFFER_FLUSH_TIMING_ISSUE.md)
			Eventually(func() int {
				resp, err := auditClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
					EventCategory: &eventCategory,
					CorrelationId: &correlationID,
				})
				if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
					return 0
				}

				if resp.JSON200.Data != nil {
					auditEvents = *resp.JSON200.Data
				}
				if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
					return *resp.JSON200.Pagination.Total
				}
				return 0
			}, 90*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
				"Should have audit events even with errors (degraded mode processing)")

			By("6. Verify audit events captured error handling")
			// Should have either error event OR completion event with degraded mode
			foundAudit := false
			for _, event := range auditEvents {
				if event.EventType == "signalprocessing.error.occurred" {
					// Explicit error event
					foundAudit = true
					Expect(event.EventOutcome).To(Equal(dsgen.AuditEventEventOutcomeFailure))
					Expect(event.EventData).ToNot(BeNil(),
						"Error event should contain event data with error details")
					break
				} else if event.EventType == "signalprocessing.signal.processed" {
					// Completion event (degraded mode)
					foundAudit = true
					GinkgoWriter.Printf("Processed in degraded mode: %v\n", event.EventData)
					break
				}
			}
			Expect(foundAudit).To(BeTrue(),
				"Should have either error audit or degraded mode completion audit")

			By("7. Verify ADR-038: Reconciliation was not blocked by audit")
			// SignalProcessing should still have updated status (not stuck in Pending)
			var finalSP signalprocessingv1alpha1.SignalProcessing
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      sp.Name,
				Namespace: sp.Namespace,
			}, &finalSP)
			Expect(err).ToNot(HaveOccurred())
			Expect(finalSP.Status.Phase).ToNot(Equal(signalprocessingv1alpha1.PhasePending),
				"ADR-038: Audit failures must not block reconciliation progress")
		})
	})
})

// Note: Helper functions (CreateTestRemediationRequest, CreateTestSignalProcessingWithParent, ValidTestFingerprints)
// are defined in test_helpers.go to avoid duplication across test files.
