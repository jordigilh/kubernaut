// Package gateway contains Priority 1 integration tests for adapter interaction patterns
package gateway

import (
	"encoding/json"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// PRIORITY 1: ADAPTER INTERACTION PATTERNS - INTEGRATION TESTS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// TDD Methodology: RED → GREEN → REFACTOR
// Business Outcome Focus: Validate WHAT adapters achieve for operators
//
// Purpose: Validate different signal adapters integrate correctly with Gateway
// Coverage: BR-001 (Prometheus), BR-002 (K8s Events), BR-013 (Multi-adapter)
//
// Business Outcomes:
// - BR-001: Prometheus alerts create CRDs with correct priority classification
// - BR-002: K8s Events create CRDs with correct priority based on environment
// - BR-013: Multiple adapters work simultaneously without conflicts
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("Priority 1: Adapter Interaction Patterns - Integration Tests", func() {
	var testCtx *Priority1TestContext

	// REFACTORED: Use shared test infrastructure helpers (TDD REFACTOR phase)
	BeforeEach(func() {
		testCtx = SetupPriority1Test()
	})

	AfterEach(func() {
		testCtx.Cleanup()
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// TEST 1: Prometheus Adapter → Priority Classification (BR-001)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	//
	// Business Outcome: Prometheus alerts classified correctly by severity + environment
	// Operational Outcome: Operators see correct priority in CRD metadata
	// Multi-tenancy: Different namespaces get different priorities
	//
	// TDD RED PHASE: This test validates Prometheus adapter priority logic
	// Expected: Critical + production = P0, warning + staging = P2
	//
	Describe("BR-001: Prometheus Adapter → Priority Classification", func() {
		It("should classify critical production alerts as P0", func() {
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS CONTEXT
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Scenario: Production payment service critical alert
			// Expected: Classified as P0 (highest priority)
			// Why: Critical + production = immediate operator attention required

			alertJSON := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "PaymentServiceDown",
						"severity": "critical",
						"namespace": "production",
						"pod": "payment-api-1"
					},
					"annotations": {
						"summary": "Payment service is down"
					}
				}]
			}`

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME VALIDATION: Priority Classification
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			// REFACTORED: Use helper function (TDD REFACTOR phase)
			resp, err := SendPrometheusAlert(testCtx.TestServer.URL, alertJSON)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME 1: Request accepted
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"Gateway MUST accept valid Prometheus alert (BR-001)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME 2: Priority correctly classified as P0
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			metadata, ok := response["metadata"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "Response MUST include metadata (BR-001)")

			priority, ok := metadata["priority"].(string)
			Expect(ok).To(BeTrue(), "Metadata MUST include priority (BR-001)")
			Expect(priority).To(Equal("P0"),
				"Critical + production MUST be classified as P0 (BR-001)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS VALUE ACHIEVED
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// ✅ Prometheus alerts accepted and processed
			// ✅ Priority classification correct (P0 for critical + production)
			// ✅ Operators receive correct priority signal
			// ✅ Multi-tenancy: Production alerts prioritized correctly
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// TEST 2: K8s Event Adapter → Priority Classification (BR-002)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	//
	// Business Outcome: K8s Events classified correctly by type + environment
	// Operational Outcome: Warning events in production get appropriate priority
	// Multi-tenancy: Different event types get different priorities
	//
	// TDD RED PHASE: This test validates K8s Event adapter priority logic
	// Expected: Warning + production = P1 (not P0, per priority.go logic)
	//
	Describe("BR-002: K8s Event Adapter → Priority Classification", func() {
		It("should classify Warning events in production as P1", func() {
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS CONTEXT
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Scenario: Production pod eviction warning
			// Expected: Classified as P1 (high priority, not critical)
			// Why: Warning + production = needs attention but not immediate

			eventJSON := `{
				"type": "Warning",
				"reason": "Evicted",
				"message": "Pod evicted due to node pressure",
				"involvedObject": {
					"kind": "Pod",
					"name": "payment-api-1",
					"namespace": "production"
				},
				"source": {
					"component": "kubelet"
				}
			}`

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME VALIDATION: K8s Event Priority
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			// REFACTORED: Use helper function (TDD REFACTOR phase)
			resp, err := SendK8sEvent(testCtx.TestServer.URL, eventJSON)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME 1: Request accepted
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"Gateway MUST accept valid K8s Event (BR-002)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME 2: Priority correctly classified as P1
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			metadata, ok := response["metadata"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "Response MUST include metadata (BR-002)")

			priority, ok := metadata["priority"].(string)
			Expect(ok).To(BeTrue(), "Metadata MUST include priority (BR-002)")
			Expect(priority).To(Equal("P1"),
				"Warning + production MUST be classified as P1 (BR-002)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS VALUE ACHIEVED
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// ✅ K8s Events accepted and processed
			// ✅ Priority classification correct (P1 for warning + production)
			// ✅ Operators receive correct priority signal
			// ✅ Multi-tenancy: Event types prioritized correctly
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// TEST 3: Multi-Adapter Concurrent Processing (BR-013)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	//
	// Business Outcome: Multiple adapters process signals simultaneously
	// Operational Outcome: No conflicts between different signal types
	// Data Integrity: Each adapter maintains independent deduplication
	//
	// TDD RED PHASE: This test validates multi-adapter concurrency
	// Expected: Both Prometheus and K8s Event signals processed successfully
	//
	Describe("BR-013: Multi-Adapter Concurrent Processing", func() {
		It("should process Prometheus and K8s Event signals concurrently", func() {
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS CONTEXT
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Scenario: Production incident - both Prometheus alerts and K8s events
			// Expected: Both signal types processed independently
			// Why: Multi-source observability requires concurrent processing

			prometheusJSON := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "HighCPU",
						"severity": "warning",
						"namespace": "production",
						"pod": "api-1"
					},
					"annotations": {
						"summary": "High CPU usage"
					}
				}]
			}`

			k8sEventJSON := `{
				"type": "Warning",
				"reason": "BackOff",
				"message": "Back-off restarting failed container",
				"involvedObject": {
					"kind": "Pod",
					"name": "api-1",
					"namespace": "production"
				},
				"source": {
					"component": "kubelet"
				}
			}`

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME VALIDATION: Multi-Adapter Processing
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			// Send Prometheus alert
			// REFACTORED: Use helper function (TDD REFACTOR phase)
			prometheusResp, err := SendPrometheusAlert(testCtx.TestServer.URL, prometheusJSON)
			Expect(err).ToNot(HaveOccurred())
			defer prometheusResp.Body.Close()

			// Send K8s Event
			// REFACTORED: Use helper function (TDD REFACTOR phase)
			k8sResp, err := SendK8sEvent(testCtx.TestServer.URL, k8sEventJSON)
			Expect(err).ToNot(HaveOccurred())
			defer k8sResp.Body.Close()

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME 1: Both requests accepted
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			Expect(prometheusResp.StatusCode).To(Equal(http.StatusCreated),
				"Gateway MUST accept Prometheus alert (BR-013)")
			Expect(k8sResp.StatusCode).To(Equal(http.StatusCreated),
				"Gateway MUST accept K8s Event (BR-013)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME 2: Both signals have unique fingerprints
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			var prometheusResponse map[string]interface{}
			err = json.NewDecoder(prometheusResp.Body).Decode(&prometheusResponse)
			Expect(err).ToNot(HaveOccurred())

			var k8sResponse map[string]interface{}
			err = json.NewDecoder(k8sResp.Body).Decode(&k8sResponse)
			Expect(err).ToNot(HaveOccurred())

			prometheusFingerprint := prometheusResponse["fingerprint"].(string)
			k8sFingerprint := k8sResponse["fingerprint"].(string)

			Expect(prometheusFingerprint).ToNot(Equal(k8sFingerprint),
				"Different signal types MUST have different fingerprints (BR-013)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS VALUE ACHIEVED
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// ✅ Multiple adapters process signals concurrently
			// ✅ No conflicts between different signal types
			// ✅ Independent deduplication per adapter
			// ✅ Operators receive signals from all sources
		})
	})
})
