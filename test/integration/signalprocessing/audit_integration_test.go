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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	spaudit "github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
	// HTTP Anti-Pattern Phase 2: Removed ogenclient import (HTTP client)
	// HTTP Anti-Pattern Phase 2: Removed fmt, net/http imports (HTTP health check)
	// HTTP Anti-Pattern Phase 2: Removed infrastructure import (dataStorageURL)
	// Now using testDB for direct PostgreSQL queries (from suite_test.go)
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
	// HTTP Anti-Pattern Phase 2: Removed dataStorageURL and HTTP health check
	// Now using testDB (direct PostgreSQL connection from suite_test.go)
	// PostgreSQL connection is verified in SynchronizedBeforeSuite (testDB.Ping)

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

		By("6. Query PostgreSQL for 'signal.processed' audit event (HTTP Anti-Pattern Phase 2)")
		// HTTP Anti-Pattern Phase 2: Replaced HTTP client with direct PostgreSQL query
		// Now querying audit_events table directly via testDB (from suite_test.go)
		
		// Wait for 'signal.processed' event to appear in PostgreSQL
		var eventCount int
		Eventually(func() int {
			err := testDB.QueryRow(`
				SELECT COUNT(*)
				FROM audit_events
				WHERE event_type = $1
				  AND correlation_id = $2
				  AND service_name = 'SignalProcessing'
			`, spaudit.EventTypeSignalProcessed, correlationID).Scan(&eventCount)
			if err != nil {
				GinkgoWriter.Printf("Failed to query audit events: %v\n", err)
				return 0
			}
			return eventCount
		}, 120*time.Second, 500*time.Millisecond).Should(Equal(1),
			"BR-SP-090: SignalProcessing MUST emit exactly 1 'signal.processed' audit event")

		By("7. Fetch and validate 'signal.processed' audit event from PostgreSQL")
		var (
			eventID        string
			eventTimestamp time.Time
			eventCategory  string
			eventAction    string
			eventOutcome   string
			actorType      string
			actorID        string
			eventData      []byte // JSONB stored as bytes
		)
		err := testDB.QueryRow(`
			SELECT event_id, event_timestamp, event_category, event_action, 
			       COALESCE(event_outcome, ''), COALESCE(actor_type, ''), COALESCE(actor_id, ''),
			       event_data
			FROM audit_events
			WHERE event_type = $1
			  AND correlation_id = $2
			  AND service_name = 'SignalProcessing'
			ORDER BY event_timestamp DESC
			LIMIT 1
		`, spaudit.EventTypeSignalProcessed, correlationID).Scan(
			&eventID, &eventTimestamp, &eventCategory, &eventAction,
			&eventOutcome, &actorType, &actorID, &eventData,
		)
		Expect(err).ToNot(HaveOccurred(), "Should find 'signal.processed' audit event in PostgreSQL")

		By("8. Validate audit event fields from PostgreSQL row")
		Expect(eventCategory).To(Equal("signalprocessing"), "Event category must match")
		Expect(eventAction).To(Equal("processed"), "Event action must match")
		Expect(eventOutcome).To(Equal("success"), "Event outcome must be success")
		Expect(actorType).To(Equal("service"), "Actor type must be service")
		Expect(actorID).To(Equal("signalprocessing-controller"), "Actor ID must match controller")

		By("9. Validate event_data JSONB fields")
		var eventDataMap map[string]interface{}
		err = json.Unmarshal(eventData, &eventDataMap)
		Expect(err).ToNot(HaveOccurred(), "event_data should be valid JSON")
		Expect(eventDataMap["environment"]).To(Equal("production"), "Environment must match")
		Expect(eventDataMap["priority"]).To(Equal("P0"), "Priority must match")
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

		By("6. Query PostgreSQL for 'classification.decision' audit event (HTTP Anti-Pattern Phase 2)")
		eventType := "signalprocessing.classification.decision"
		
		var eventCount int
		Eventually(func() int {
			err := testDB.QueryRow(`
				SELECT COUNT(*)
				FROM audit_events
				WHERE event_type = $1
				  AND correlation_id = $2
				  AND service_name = 'SignalProcessing'
			`, eventType, correlationID).Scan(&eventCount)
			if err != nil {
				return 0
			}
			return eventCount
		}, 120*time.Second, 500*time.Millisecond).Should(Equal(1),
			"BR-SP-090: SignalProcessing MUST emit exactly 1 classification.decision event per classification")

		By("7. Validate classification audit event from PostgreSQL")
		var (
			eventCategory string
			eventAction   string
			eventOutcome  string
			eventData     []byte
		)
		err := testDB.QueryRow(`
			SELECT event_category, event_action, COALESCE(event_outcome, ''), event_data
			FROM audit_events
			WHERE event_type = $1
			  AND correlation_id = $2
			  AND service_name = 'SignalProcessing'
			ORDER BY event_timestamp DESC
			LIMIT 1
		`, eventType, correlationID).Scan(&eventCategory, &eventAction, &eventOutcome, &eventData)
		Expect(err).ToNot(HaveOccurred())

		Expect(eventCategory).To(Equal("signalprocessing"))
		Expect(eventAction).To(Equal("classification"))
		Expect(eventOutcome).To(Equal("success"))

		var eventDataMap map[string]interface{}
		err = json.Unmarshal(eventData, &eventDataMap)
		Expect(err).ToNot(HaveOccurred())
		Expect(eventDataMap["environment"]).To(Equal("staging"))
		Expect(eventDataMap["priority"]).To(Equal("P2"))
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

		By("6. Query PostgreSQL for 'business.classified' audit event (HTTP Anti-Pattern Phase 2)")
		eventType := "signalprocessing.business.classified"
		
		var eventCount int
		Eventually(func() int {
			err := testDB.QueryRow(`
				SELECT COUNT(*)
				FROM audit_events
				WHERE event_type = $1
				  AND correlation_id = $2
				  AND service_name = 'SignalProcessing'
			`, eventType, correlationID).Scan(&eventCount)
			if err != nil {
				return 0
			}
			return eventCount
		}, 120*time.Second, 500*time.Millisecond).Should(Equal(1),
			"AUDIT-06: SignalProcessing MUST emit exactly 1 business.classified event per business classification")

		By("7. Validate business classification audit event from PostgreSQL")
		var (
			eventCategory string
			eventAction   string
			eventOutcome  string
			eventData     []byte
		)
		err := testDB.QueryRow(`
			SELECT event_category, event_action, COALESCE(event_outcome, ''), event_data
			FROM audit_events
			WHERE event_type = $1
			  AND correlation_id = $2
			  AND service_name = 'SignalProcessing'
			ORDER BY event_timestamp DESC
			LIMIT 1
		`, eventType, correlationID).Scan(&eventCategory, &eventAction, &eventOutcome, &eventData)
		Expect(err).ToNot(HaveOccurred())

		Expect(eventCategory).To(Equal("signalprocessing"))
		Expect(eventAction).To(Equal("classification"))
		Expect(eventOutcome).To(Equal("success"))

		var eventDataMap map[string]interface{}
		err = json.Unmarshal(eventData, &eventDataMap)
		Expect(err).ToNot(HaveOccurred())
		Expect(eventDataMap["business_unit"]).To(Equal("payments"))
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

		By("6. Query PostgreSQL for 'enrichment.completed' audit event (HTTP Anti-Pattern Phase 2)")
		eventType := "signalprocessing.enrichment.completed"
		
		var eventCount int
		Eventually(func() int {
			err := testDB.QueryRow(`
				SELECT COUNT(*)
				FROM audit_events
				WHERE event_type = $1
				  AND correlation_id = $2
				  AND service_name = 'SignalProcessing'
			`, eventType, correlationID).Scan(&eventCount)
			if err != nil {
				return 0
			}
			return eventCount
		}, 120*time.Second, 500*time.Millisecond).Should(Equal(1),
			"BR-SP-090: SignalProcessing MUST emit exactly 1 enrichment.completed event per enrichment operation")

		By("7. Validate enrichment audit event from PostgreSQL")
		var (
			eventCategory  string
			eventAction    string
			eventOutcome   string
			eventData      []byte
			durationMs     *int64 // Optional field
		)
		err := testDB.QueryRow(`
			SELECT event_category, event_action, COALESCE(event_outcome, ''), 
			       event_data, duration_ms
			FROM audit_events
			WHERE event_type = $1
			  AND correlation_id = $2
			  AND service_name = 'SignalProcessing'
			ORDER BY event_timestamp DESC
			LIMIT 1
		`, eventType, correlationID).Scan(&eventCategory, &eventAction, &eventOutcome, &eventData, &durationMs)
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Printf("\n📊 Found 1 enrichment audit event\n")

		Expect(eventCategory).To(Equal("signalprocessing"))
		Expect(eventAction).To(Equal("enrichment"))
		Expect(eventOutcome).To(Equal("success"))

		var eventDataMap map[string]interface{}
		err = json.Unmarshal(eventData, &eventDataMap)
		Expect(err).ToNot(HaveOccurred())
		Expect(eventDataMap["has_namespace"]).To(BeTrue())
		Expect(eventDataMap["has_pod"]).To(BeTrue())
		Expect(eventDataMap["degraded_mode"]).To(BeFalse())

		// Additional assertions for enrichment-specific fields
		Expect(durationMs).ToNot(BeNil(),
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

		By("6. Query PostgreSQL for ALL signalprocessing audit events (HTTP Anti-Pattern Phase 2)")
		// HTTP Anti-Pattern Phase 2: Replaced HTTP client with direct PostgreSQL query
		// Querying all events for this correlation_id to count phase transitions
		
		var totalCount int
		Eventually(func() int {
			err := testDB.QueryRow(`
				SELECT COUNT(*)
				FROM audit_events
				WHERE event_category = 'signalprocessing'
				  AND correlation_id = $1
				  AND service_name = 'SignalProcessing'
			`, correlationID).Scan(&totalCount)
			if err != nil {
				return 0
			}
			return totalCount
		}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
			"BR-SP-090: SignalProcessing MUST emit audit events")

		By("7. Count phase.transition events (DD-TESTING-001 deterministic validation)")
		var phaseTransitionCount int
		err := testDB.QueryRow(`
			SELECT COUNT(*)
			FROM audit_events
			WHERE event_type = $1
			  AND correlation_id = $2
			  AND service_name = 'SignalProcessing'
		`, spaudit.EventTypePhaseTransition, correlationID).Scan(&phaseTransitionCount)
		Expect(err).ToNot(HaveOccurred())

		By("8. Validate exact event count for 'phase.transition' (DD-TESTING-001 compliance)")
		// Business requirement: SP has 5 phases (Pending→Enriching→Classifying→Categorizing→Completed)
		// Therefore: Exactly 4 phase transitions per successful processing
		Expect(phaseTransitionCount).To(Equal(4),
			"BR-SP-090: MUST emit exactly 4 phase transitions: Pending→Enriching→Classifying→Categorizing→Completed")

		By("9. Fetch first 'phase.transition' event for detailed validation")
		var (
			eventCategory string
			eventAction   string
			eventOutcome  string
			eventData     []byte
		)
		err = testDB.QueryRow(`
			SELECT event_category, event_action, COALESCE(event_outcome, ''), event_data
			FROM audit_events
			WHERE event_type = $1
			  AND correlation_id = $2
			  AND service_name = 'SignalProcessing'
			ORDER BY event_timestamp ASC
			LIMIT 1
		`, spaudit.EventTypePhaseTransition, correlationID).Scan(&eventCategory, &eventAction, &eventOutcome, &eventData)
		Expect(err).ToNot(HaveOccurred(), "Should find at least one phase.transition audit event")

		By("10. Validate phase transition event structure from PostgreSQL")
		Expect(eventCategory).To(Equal("signalprocessing"))
		Expect(eventAction).To(Equal("phase_transition"))
		Expect(eventOutcome).To(Equal("success"))

		// Verify event_data contains phase information
		Expect(eventData).ToNot(BeNil(), "EventData should not be nil")
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

		By("5. Query PostgreSQL for error audit events (HTTP Anti-Pattern Phase 2)")
		// HTTP Anti-Pattern Phase 2: Replaced HTTP client with direct PostgreSQL query
		
		var totalCount int
		Eventually(func() int {
			err := testDB.QueryRow(`
				SELECT COUNT(*)
				FROM audit_events
				WHERE event_category = 'signalprocessing'
				  AND correlation_id = $1
				  AND service_name = 'SignalProcessing'
			`, correlationID).Scan(&totalCount)
			if err != nil {
				return 0
			}
			return totalCount
		}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
			"Should have audit events even with errors (degraded mode processing)")

		By("6. Count events by event_type (DD-TESTING-001 deterministic validation)")
		var errorCount, completionCount int
		err := testDB.QueryRow(`
			SELECT 
				COUNT(CASE WHEN event_type = $1 THEN 1 END) as error_count,
				COUNT(CASE WHEN event_type = $2 THEN 1 END) as completion_count
			FROM audit_events
			WHERE correlation_id = $3
			  AND service_name = 'SignalProcessing'
		`, spaudit.EventTypeError, spaudit.EventTypeSignalProcessed, correlationID).Scan(&errorCount, &completionCount)
		Expect(err).ToNot(HaveOccurred())

		By("7. Validate error handling produced expected audit event (DD-TESTING-001 compliance)")
		// Business logic: In error scenarios, SP emits either:
		// - Option A: Explicit error event (signalprocessing.error.occurred) with EventOutcome=Failure
		// - Option B: Completion in degraded mode (signalprocessing.signal.processed) with degraded=true
		hasErrorEvent := errorCount > 0
		hasCompletionEvent := completionCount > 0

		Expect(hasErrorEvent || hasCompletionEvent).To(BeTrue(),
			"BR-SP-090: MUST emit either error event OR degraded mode completion event")

		// Validate exactly 1 of the expected event types (deterministic)
		if hasErrorEvent {
			Expect(errorCount).To(Equal(1),
				"BR-SP-090: Should emit exactly 1 error event per error occurrence")

			By("8. Validate error event structure from PostgreSQL (DD-TESTING-001)")
			var (
				eventOutcome string
				eventData    []byte
			)
			err = testDB.QueryRow(`
				SELECT COALESCE(event_outcome, ''), event_data
				FROM audit_events
				WHERE event_type = $1
				  AND correlation_id = $2
				  AND service_name = 'SignalProcessing'
				ORDER BY event_timestamp DESC
				LIMIT 1
			`, spaudit.EventTypeError, correlationID).Scan(&eventOutcome, &eventData)
			Expect(err).ToNot(HaveOccurred())

			// Validate event_outcome is Failure
			Expect(eventOutcome).To(Equal("failure"),
				"Error events MUST have EventOutcome=Failure")

			// DD-TESTING-001 MANDATORY: Validate structured event_data fields
			var eventDataMap map[string]interface{}
			err = json.Unmarshal(eventData, &eventDataMap)
			Expect(err).ToNot(HaveOccurred(), "event_data should be marshallable")

			// Per DD-AUDIT-004: Error events should contain structured error information
			Expect(eventDataMap).To(HaveKey("error_message"),
				"Error event should contain error_message field")

			errorMessage := eventDataMap["error_message"].(string)
			Expect(errorMessage).ToNot(BeEmpty(),
				"Error message should not be empty")
		} else {
			Expect(completionCount).To(Equal(1),
				"BR-SP-090: Should emit exactly 1 completion event (degraded mode)")
			GinkgoWriter.Printf("✅ Processed in degraded mode (no explicit error event)\n")
		}

			By("9. Verify ADR-038: Reconciliation was not blocked by audit")
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

		// SP-BUG-003/004: Test fatal enrichment error path (namespace not found)
		// This test validates that error.occurred audit events are emitted for fatal enrichment errors
		// (as opposed to degraded mode for missing target resources)
		It("should emit 'error.occurred' event for fatal enrichment errors (namespace not found)", func() {
			// BUSINESS SCENARIO:
			// Fatal enrichment errors (namespace not found, API timeouts, RBAC denials)
			// MUST emit error.occurred audit events before stopping reconciliation
			//
			// This is DIFFERENT from degraded mode (missing target Pod):
			// - Missing Pod → degraded mode → continue processing → signal.processed
			// - Missing namespace → fatal error → stop processing → error.occurred
			//
			// SP-BUG-003: Controller now emits error audit events before returning
			// SP-BUG-004: Enricher properly propagates fatal errors (not silent success)

			By("1. Creating parent RemediationRequest in EXISTING namespace")
			existingNs := createTestNamespaceWithLabels("audit-test-fatal-error", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(existingNs)

			rrName := "audit-test-rr-fatal-06"
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "test-pod-fatal",
				Namespace: "non-existent-namespace-fatal", // This namespace does NOT exist
			}
			rr := CreateTestRemediationRequest(rrName, existingNs, ValidTestFingerprints["audit-006"], "critical", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			correlationID := rrName

			By("2. Creating SignalProcessing CR targeting NON-EXISTENT namespace")
			sp := CreateTestSignalProcessingWithParent("audit-test-sp-fatal-06", existingNs, rr, ValidTestFingerprints["audit-006"], targetResource)
			sp.Spec.Signal.Severity = "critical"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			By("3. Wait for enrichment to fail (namespace not found is fatal)")
			// Controller should attempt reconciliation and fail during enrichment
			time.Sleep(5 * time.Second)

		By("4. Query PostgreSQL for audit events (HTTP Anti-Pattern Phase 2)")
		var totalCount int
		Eventually(func() int {
			err := testDB.QueryRow(`
				SELECT COUNT(*)
				FROM audit_events
				WHERE event_category = 'signalprocessing'
				  AND correlation_id = $1
				  AND service_name = 'SignalProcessing'
			`, correlationID).Scan(&totalCount)
			if err != nil {
				return 0
			}
			return totalCount
		}, 120*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
			"Should have error.occurred audit event for fatal enrichment error")

		By("5. Validate error.occurred event was emitted (DD-TESTING-001)")
		var errorCount int
		err := testDB.QueryRow(`
			SELECT COUNT(*)
			FROM audit_events
			WHERE event_type = $1
			  AND correlation_id = $2
			  AND service_name = 'SignalProcessing'
		`, spaudit.EventTypeError, correlationID).Scan(&errorCount)
		Expect(err).ToNot(HaveOccurred())

		// For fatal errors, we MUST have error.occurred (not signal.processed)
		Expect(errorCount).To(BeNumerically(">=", 1),
			"BR-SP-090: MUST emit error.occurred for fatal enrichment errors")

	By("6. Fetch error event structure from PostgreSQL")
		var (
			eventOutcome string
			eventData    []byte
		)
		err = testDB.QueryRow(`
			SELECT COALESCE(event_outcome, ''), event_data
			FROM audit_events
			WHERE event_type = $1
			  AND correlation_id = $2
			  AND service_name = 'SignalProcessing'
			ORDER BY event_timestamp DESC
			LIMIT 1
		`, spaudit.EventTypeError, correlationID).Scan(&eventOutcome, &eventData)
		Expect(err).ToNot(HaveOccurred())

		// Validate EventOutcome is Failure
		Expect(eventOutcome).To(Equal("failure"),
			"Fatal errors MUST have EventOutcome=Failure")

		// Validate error_data contains namespace error information
		var eventDataMap map[string]interface{}
		err = json.Unmarshal(eventData, &eventDataMap)
		Expect(err).ToNot(HaveOccurred(), "event_data should be marshallable")

		Expect(eventDataMap).To(HaveKey("phase"), "Error event should contain phase")
		Expect(eventDataMap["phase"]).To(Equal("Enriching"), "Error should occur during Enriching phase")

		Expect(eventDataMap).To(HaveKey("error"), "Error event should contain error message")
		errorMsg := eventDataMap["error"].(string)
		Expect(errorMsg).To(ContainSubstring("non-existent-namespace-fatal"),
			"Error message should reference the missing namespace")

		GinkgoWriter.Printf("✅ Fatal enrichment error correctly emitted error.occurred audit event\n")
		GinkgoWriter.Printf("   Error: %s\n", errorMsg)
		})
	})
})

// Note: Helper functions (CreateTestRemediationRequest, CreateTestSignalProcessingWithParent, ValidTestFingerprints)
// are defined in test_helpers.go to avoid duplication across test files.
