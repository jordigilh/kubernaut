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

package remediationorchestrator

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
)

// =============================================================================
// BR-AUDIT-005 Gap #7: Remediation Orchestrator Error Details Standardization
// =============================================================================
//
// Business Requirements:
// - BR-AUDIT-005 v2.0 Gap #7: Standardized error details across all services
// - SOC2 Type II: Comprehensive error audit trail for compliance
// - RR Reconstruction: Reliable `.status.error` field reconstruction
//
// Authority Documents:
// - DD-AUDIT-003 v1.4: Service audit trace requirements
// - ADR-034: Unified audit table design
// - SOC2_AUDIT_IMPLEMENTATION_PLAN.md: Day 4 - Error Details Standardization
//
// Test Strategy (per TESTING_GUIDELINES.md):
// - Integration tier: Requires envtest for CRD lifecycle error scenarios
// - OpenAPI client MANDATORY for all audit queries (DD-API-001)
// - Eventually() MANDATORY for async operations (NO time.Sleep())
//
// Error Scenarios Tested:
// - Scenario 1: Timeout configuration error (ERR_INVALID_TIMEOUT_CONFIG)
// - Scenario 2: Child CRD creation failure (ERR_K8S_CREATE_FAILED)
//
// To run these tests:
//   make test-integration-remediationorchestrator
//
// =============================================================================

var _ = Describe("BR-AUDIT-005 Gap #7: RemediationOrchestrator Error Audit Standardization", func() {
	var (
		dsClient *ogenclient.Client
	)

	BeforeEach(func() {
		// DD-AUTH-014: Use authenticated OpenAPI client from shared setup
		// dsClients is created in SynchronizedBeforeSuite with ServiceAccount token
		// Creating a new client here would bypass authentication!
		dsClient = dsClients.OpenAPIClient
	})

	Context("Gap #7 Scenario 1: Timeout Configuration Error", func() {
		It("should emit standardized error_details on invalid timeout configuration", func() {
			// Given: RemediationRequest CRD with invalid timeout configuration
			testNamespace := createTestNamespace("error-audit")
			defer func() {
				// Async namespace cleanup
				go func() {
					deleteTestNamespace(testNamespace)
				}()
			}()

			fingerprint := GenerateTestFingerprint(testNamespace, "timeout-error")
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-timeout-error",
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "IntegrationTestSignal",
					Severity:          "warning",
					SignalType:        "prometheus",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: "default",
					},
					FiringTime:   now,
					ReceivedTime: now,
				},
				// NOTE: Status field intentionally omitted - Kubernetes ignores Status on CRD creation
				// We'll set it after creation via status update (simulates operator error or webhook bypass)
			}

		// When: Create RemediationRequest CRD
		err := k8sClient.Create(ctx, rr)
		Expect(err).ToNot(HaveOccurred())

		// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) for audit event queries
		correlationID := rr.Name

		// Wait for controller to initialize status.timeoutConfig with defaults
			Eventually(func() bool {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.TimeoutConfig != nil
			}, timeout, interval).Should(BeTrue(), "Controller should initialize status.timeoutConfig")

			// Now inject invalid timeout via status update (simulates operator error or webhook bypass)
			// Gap #7: Tests controller detection of invalid configuration
			rr.Status.TimeoutConfig.Global = &metav1.Duration{Duration: -100 * time.Second} // Invalid: negative
			err = k8sClient.Status().Update(ctx, rr)
			Expect(err).ToNot(HaveOccurred())

		// Then: Controller should detect invalid config and transition to Failed
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed), "RR should transition to Failed on invalid timeout")

		// Flush buffered audit events to DataStorage before querying
		err = auditStore.Flush(ctx)
		Expect(err).ToNot(HaveOccurred(), "Failed to flush audit events to DataStorage")

		// Query for orchestrator.lifecycle.completed (failure) audit event
			eventType := roaudit.EventTypeLifecycleCompleted
			var events []ogenclient.AuditEvent
			Eventually(func() int {
				var failureEvents []ogenclient.AuditEvent
				allEvents := queryAuditEventsOpenAPI(dsClient, correlationID, eventType)
				// Filter for failure outcome
				for _, e := range allEvents {
					if string(e.EventOutcome) == "failure" {
						failureEvents = append(failureEvents, e)
					}
				}
				events = failureEvents
				return len(events)
			}, "10s", "500ms").Should(Equal(1), "Should find exactly 1 orchestrator.lifecycle.completed (failure) event")

			// Validate Gap #7: error_details
			event := events[0]
			payload := event.EventData.RemediationOrchestratorAuditPayload
			Expect(payload.ErrorDetails.IsSet()).To(BeTrue(), "error_details should be present")
			errorDetails := payload.ErrorDetails.Value

			Expect(errorDetails.Code).To(ContainSubstring("ERR_INVALID_TIMEOUT_CONFIG"))
			Expect(errorDetails.Message).To(ContainSubstring("timeout"))
			Expect(errorDetails.Component).To(Equal(ogenclient.ErrorDetailsComponentRemediationorchestrator))
			Expect(errorDetails.RetryPossible).To(BeFalse(), "Invalid config is permanent error")
		})
	})

	// Gap #7 Scenario 2: REMOVED - Moved to unit tests (DD-TEST-008)
	// Rationale: Error code mapping is pure business logic, best tested in unit tests
	// Integration Scenario 1 already validates end-to-end error_details flow
	// See: test/unit/remediationorchestrator/audit/manager_test.go - "Gap #7: Error Code Mapping Logic"
	// See: docs/handoff/RO_GAP7_SCENARIO2_TIER_ANALYSIS_JAN10.md
})
