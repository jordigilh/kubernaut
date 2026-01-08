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

package aianalysis

import (
	"context"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// Recovery Endpoint Integration Tests
//
// BR-AI-082: RecoveryRequest Implementation
// DD-RECOVERY-002: Direct recovery flow
//
// Testing Strategy (per TESTING_GUIDELINES.md + user clarification):
// - Integration tests use REAL HAPI service via AIAnalysis-specific infrastructure
// - HAPI runs with MOCK_LLM_ENABLED=true (cost constraint)
// - Tests verify contract compliance with /api/v1/recovery/analyze endpoint
//
// Infrastructure Required (AIAnalysis-Specific):
//   podman-compose -f test/integration/aianalysis/podman-compose.yml up -d
//
//   This starts AIAnalysis's dedicated infrastructure stack:
//   - PostgreSQL (:15438)
//   - Redis (:16384)
//   - DataStorage API (:18095)
//   - HolmesGPT API (:18120) with MOCK_LLM_MODE=true
//
// Environment Variables:
//   HOLMESGPT_URL: Override default HAPI URL (default: http://localhost:18120)
//
// Port Allocation (DD-TEST-001):
//   - HAPI: 18120 (AIAnalysis integration range: 18120-18129)
//   - No collisions with other services (DataStorage uses 18090, Gateway uses 50001-60000)

// SERIAL EXECUTION: AA integration suite runs serially for 100% reliability.
// See audit_flow_integration_test.go for detailed rationale.
var _ = PDescribe("Recovery Endpoint Integration", Serial, Label("integration", "recovery", "hapi"), func() {
	var (
		hapiClient *client.HolmesGPTClient
		hapiURL    string
		testCtx    context.Context
		cancelFunc context.CancelFunc
	)

	BeforeEach(func() {
		// Get HAPI URL from environment or use default from podman-compose.test.yml
		hapiURL = os.Getenv("HOLMESGPT_URL")
		if hapiURL == "" {
			// DD-TEST-001: HAPI integration port range 18120-18129
			hapiURL = "http://localhost:18120"
		}

		// Create real HAPI client (DD-HAPI-003: Generated OpenAPI client)
		var err error
		hapiClient, err = client.NewHolmesGPTClient(client.Config{
			BaseURL: hapiURL,
			Timeout: 60 * time.Second,
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create HolmesGPT-API client")

		testCtx, cancelFunc = context.WithTimeout(context.Background(), 90*time.Second)

		// Verify HAPI is available
		// Skip if HAPI is not running (allows running test suite without infrastructure)
		// Per TESTING_GUIDELINES.md: Integration tests SHOULD fail if real services unavailable
		// But we use Skip to allow partial test runs during development
		By("Verifying HAPI availability")
		// Use HTTP health endpoint instead of Investigate() for better reliability
		// The /health endpoint is always available and doesn't require complex request setup
		healthURL := hapiURL + "/health"
		healthReq, err := http.NewRequest("GET", healthURL, nil)
		Expect(err).ToNot(HaveOccurred(), "Failed to create health check request")

		healthCtx, healthCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer healthCancel()
		healthReq = healthReq.WithContext(healthCtx)

		httpClient := &http.Client{Timeout: 10 * time.Second}
		healthResp, err := httpClient.Do(healthReq)
		if err != nil {
			// Per TESTING_GUIDELINES.md: Skip() is ABSOLUTELY FORBIDDEN
			// Integration tests MUST fail when required services are unavailable
			Fail("REQUIRED: HAPI not available at " + hapiURL + "\n" +
				"  Health check failed: " + err.Error() + "\n" +
				"  Per TESTING_GUIDELINES.md: Integration tests MUST use real services\n" +
				"  Start with: podman-compose -f podman-compose.test.yml up -d holmesgpt-api")
		}
		defer func() { _ = healthResp.Body.Close() }()

		if healthResp.StatusCode != 200 {
			Fail("REQUIRED: HAPI health check returned non-200 status: " + healthResp.Status + "\n" +
				"  URL: " + healthURL + "\n" +
				"  Per TESTING_GUIDELINES.md: Integration tests MUST use real services")
		}
	})

	AfterEach(func() {
		cancelFunc()
	})

	// ========================================
	// BR-AI-082: RecoveryRequest Schema Compliance
	// ========================================
	Context("Recovery Endpoint - BR-AI-082", func() {
		// PENDING: Recovery endpoint (/api/v1/recovery/analyze) not yet implemented in HolmesGPT-API
		// These tests validate BR-AI-082 (Recovery flow) which is a future feature
		// Reference: docs/handoff/AA_COMPLETE_TEST_TRIAGE_DEC_28_2025.md
		It("should accept valid RecoveryRequest with all required fields", func() {
			recoveryReq := &client.RecoveryRequest{
				// REQUIRED fields
				IncidentID:    "test-recovery-int-001",
				RemediationID: "req-2025-12-10-int001",

				// Recovery-specific fields
				IsRecoveryAttempt:     client.NewOptBool(true),
				RecoveryAttemptNumber: client.NewOptNilInt(1),

				// Previous execution context
				PreviousExecution: client.NewOptNilPreviousExecution(client.PreviousExecution{
					WorkflowExecutionRef: "we-failed-001",
					OriginalRca: client.OriginalRCA{
						Summary:             "Initial OOM analysis from integration test",
						SignalType:          "OOMKilled",
						Severity:            "critical",
						ContributingFactors: []string{"memory limit too low", "traffic spike"},
					},
					SelectedWorkflow: client.SelectedWorkflowSummary{
						WorkflowID:     "memory-fix-v1",
						Version:        "v1.0.0",
						ContainerImage: "kubernaut/memory-fix:v1.0.0",
						Rationale:      "Selected for OOM remediation",
					},
					Failure: client.ExecutionFailure{
						FailedStepIndex: 1,
						FailedStepName:  "apply-memory-limit",
						Reason:          "DeadlineExceeded",
						Message:         "Step timed out after 30s",
						FailedAt:        "2025-12-10T10:00:00Z",
						ExecutionTime:   "30s",
					},
				}),

				// Optional signal context
				SignalType:        client.NewOptNilString("CrashLoopBackOff"),
				Severity:          client.NewOptNilString("warning"),
				ResourceNamespace: client.NewOptNilString("test-ns"),
				ResourceKind:      client.NewOptNilString("Deployment"),
				ResourceName:      client.NewOptNilString("test-app"),

				// Default values
				Environment:      client.NewOptString("staging"),
				Priority:         client.NewOptString("P2"),
				RiskTolerance:    client.NewOptString("medium"),
				BusinessCategory: client.NewOptString("standard"),
			}

			resp, err := hapiClient.InvestigateRecovery(testCtx, recoveryReq)

			// Contract validation
			Expect(err).ToNot(HaveOccurred(), "Recovery request should succeed")
			Expect(resp).ToNot(BeNil(), "Response should not be nil")
			Expect(resp.IncidentID).ToNot(BeEmpty(), "IncidentID should be returned")
			// Note: With mock LLM, response may vary but should be valid JSON
		})

		It("should reject request without required remediation_id", func() {
			recoveryReq := &client.RecoveryRequest{
				IncidentID:        "test-no-remediation",
				RemediationID:     "", // EMPTY - violates DD-WORKFLOW-002
				IsRecoveryAttempt: client.NewOptBool(true),
				Environment:       client.NewOptString("test"),
				Priority:          client.NewOptString("P2"),
				RiskTolerance:     client.NewOptString("medium"),
				BusinessCategory:  client.NewOptString("standard"),
			}

			_, err := hapiClient.InvestigateRecovery(testCtx, recoveryReq)

			// Should return validation error (HAPI returns 400 for validation errors)
			Expect(err).To(HaveOccurred(), "Request without remediation_id should fail")

			apiErr, ok := err.(*client.APIError)
			Expect(ok).To(BeTrue(), "Error should be *client.APIError, got %T: %v", err, err)
			// HAPI returns HTTP 400 for Pydantic validation errors (not 422)
			Expect(apiErr.StatusCode).To(Equal(400), "Should return 400 for validation error (HAPI actual behavior)")
		})

		It("should handle recovery attempt number correctly", func() {
			for attemptNum := 1; attemptNum <= 3; attemptNum++ {
				recoveryReq := &client.RecoveryRequest{
					IncidentID:            "test-attempt-tracking",
					RemediationID:         "req-attempt-test",
					IsRecoveryAttempt:     client.NewOptBool(true),
					RecoveryAttemptNumber: client.NewOptNilInt(attemptNum),
					Environment:           client.NewOptString("test"),
					Priority:              client.NewOptString("P2"),
					RiskTolerance:         client.NewOptString("medium"),
					BusinessCategory:      client.NewOptString("standard"),
				}

				resp, err := hapiClient.InvestigateRecovery(testCtx, recoveryReq)
				Expect(err).ToNot(HaveOccurred(), "Recovery attempt %d should succeed", attemptNum)
				Expect(resp).ToNot(BeNil())
			}
		})
	})

	// ========================================
	// BR-AI-083: Incident vs Recovery Endpoint Selection
	// ========================================
	Context("Endpoint Selection - BR-AI-083", func() {
		It("should call incident endpoint for initial analysis", func() {
			incidentReq := &client.IncidentRequest{
				IncidentID:        "test-incident-initial",
				RemediationID:     "req-initial-001",
				SignalType:        "OOMKilled",
				Severity:          "critical",
				SignalSource:      "kubernaut",
				ResourceNamespace: "production",
				ResourceKind:      "Pod",
				ResourceName:      "app-pod-xyz",
				ErrorMessage:      "Container killed due to OOM",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "low",
				BusinessCategory:  "critical",
				ClusterName:       "prod-cluster",
			}

			resp, err := hapiClient.Investigate(testCtx, incidentReq)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp).ToNot(BeNil())
			Expect(resp.IncidentID).ToNot(BeEmpty())
		})

		It("should call recovery endpoint for failed workflow attempts", func() {
			recoveryReq := &client.RecoveryRequest{
				IncidentID:            "test-recovery-after-failure",
				RemediationID:         "req-recovery-001",
				IsRecoveryAttempt:     client.NewOptBool(true),
				RecoveryAttemptNumber: client.NewOptNilInt(1),
				PreviousExecution: client.NewOptNilPreviousExecution(client.PreviousExecution{
					WorkflowExecutionRef: "we-xyz-failed",
					OriginalRca: client.OriginalRCA{
						Summary:    "Memory leak detected",
						SignalType: "OOMKilled",
						Severity:   "critical",
					},
					SelectedWorkflow: client.SelectedWorkflowSummary{
						WorkflowID:     "restart-pod-v1",
						Version:        "1.0.0",
						ContainerImage: "kubernaut/restart:v1",
						Rationale:      "Selected based on OOMKilled signal type",
					},
					Failure: client.ExecutionFailure{
						FailedStepIndex: 0,
						FailedStepName:  "restart-container",
						Reason:          "ContainerCreating",
						Message:         "Image pull failed",
						FailedAt:        "2025-12-10T11:00:00Z",
						ExecutionTime:   "60s",
					},
				}),
				Environment:      client.NewOptString("staging"),
				Priority:         client.NewOptString("P2"),
				RiskTolerance:    client.NewOptString("medium"),
				BusinessCategory: client.NewOptString("standard"),
			}

			resp, err := hapiClient.InvestigateRecovery(testCtx, recoveryReq)

			Expect(err).ToNot(HaveOccurred())
			Expect(resp).ToNot(BeNil())
		})
	})

	// ========================================
	// DD-RECOVERY-003: Previous Execution Context
	// ========================================
	Context("Previous Execution Context - DD-RECOVERY-003", func() {
		It("should accept PreviousExecution with full failure details", func() {
			exitCode := int32(137)
			recoveryReq := &client.RecoveryRequest{
				IncidentID:            "test-full-context",
				RemediationID:         "req-context-001",
				IsRecoveryAttempt:     client.NewOptBool(true),
				RecoveryAttemptNumber: client.NewOptNilInt(2),
				PreviousExecution: client.NewOptNilPreviousExecution(client.PreviousExecution{
					WorkflowExecutionRef: "we-full-context-001",
					OriginalRca: client.OriginalRCA{
						Summary:             "Database connection pool exhausted",
						SignalType:          "ConnectionTimeout",
						Severity:            "high",
						ContributingFactors: []string{"high traffic", "slow queries", "connection leak"},
					},
					SelectedWorkflow: client.SelectedWorkflowSummary{
						WorkflowID:     "db-pool-fix-v2",
						Version:        "v2.1.0",
						ContainerImage: "kubernaut/db-fix:v2.1.0",
						Parameters:     client.NewOptSelectedWorkflowSummaryParameters(map[string]string{"MAX_CONNECTIONS": "100", "TIMEOUT": "30s"}),
						Rationale:      "Selected based on connection pool symptoms",
					},
					Failure: client.ExecutionFailure{
						FailedStepIndex: 2,
						FailedStepName:  "apply-connection-config",
						Reason:          "OOMKilled",
						Message:         "Container killed - exit code 137",
						ExitCode:        client.NewOptNilInt(int(exitCode)),
						FailedAt:        "2025-12-10T12:00:00Z",
						ExecutionTime:   "2m34s",
					},
				}),
				Environment:      client.NewOptString("production"),
				Priority:         client.NewOptString("P1"),
				RiskTolerance:    client.NewOptString("low"),
				BusinessCategory: client.NewOptString("critical"),
			}

			resp, err := hapiClient.InvestigateRecovery(testCtx, recoveryReq)

			Expect(err).ToNot(HaveOccurred(), "Full context recovery request should succeed")
			Expect(resp).ToNot(BeNil())
		})

		It("should handle multiple previous attempts context", func() {
			// Test that 3rd recovery attempt works (system should learn from 2 failures)
			recoveryReq := &client.RecoveryRequest{
				IncidentID:            "test-multi-attempt",
				RemediationID:         "req-multi-001",
				IsRecoveryAttempt:     client.NewOptBool(true),
				RecoveryAttemptNumber: client.NewOptNilInt(3), // Third attempt
				PreviousExecution: client.NewOptNilPreviousExecution(client.PreviousExecution{
					WorkflowExecutionRef: "we-attempt-2-failed",
					OriginalRca: client.OriginalRCA{
						Summary:    "Persistent memory issue",
						SignalType: "OOMKilled",
						Severity:   "critical",
					},
					SelectedWorkflow: client.SelectedWorkflowSummary{
						WorkflowID:     "memory-scale-v2",
						ContainerImage: "kubernaut/memory-scale:v2",
						Rationale:      "Second attempt after restart failed",
					},
					Failure: client.ExecutionFailure{
						FailedStepIndex: 0,
						FailedStepName:  "scale-memory",
						Reason:          "ResourceQuota",
						Message:         "Exceeded namespace memory quota",
						FailedAt:        "2025-12-10T13:00:00Z",
						ExecutionTime:   "5s",
					},
				}),
				Environment:      client.NewOptString("staging"),
				Priority:         client.NewOptString("P2"),
				RiskTolerance:    client.NewOptString("medium"),
				BusinessCategory: client.NewOptString("standard"),
			}

			resp, err := hapiClient.InvestigateRecovery(testCtx, recoveryReq)

			Expect(err).ToNot(HaveOccurred(), "Third recovery attempt should succeed")
			Expect(resp).ToNot(BeNil())
		})
	})

	// ========================================
	// Error Handling
	// ========================================
	Context("Error Handling", func() {
		It("should return APIError for transient failures", func() {
			// Create client with very short timeout to simulate timeout (DD-HAPI-003)
			shortClient, err := client.NewHolmesGPTClient(client.Config{
				BaseURL: hapiURL,
				Timeout: 1 * time.Nanosecond, // Effectively instant timeout
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to create short-timeout HAPI client")

			recoveryReq := &client.RecoveryRequest{
				IncidentID:        "test-timeout",
				RemediationID:     "req-timeout-001",
				IsRecoveryAttempt: client.NewOptBool(true),
				Environment:       client.NewOptString("test"),
				Priority:          client.NewOptString("P2"),
				RiskTolerance:     client.NewOptString("medium"),
				BusinessCategory:  client.NewOptString("standard"),
			}

			_, err = shortClient.InvestigateRecovery(testCtx, recoveryReq)

			Expect(err).To(HaveOccurred(), "Should fail with timeout")
		})
	})
})

// Helper function for optional string pointers
// NOTE: Currently unused - kept for potential future use (Dec 29, 2025)
// Uncomment if needed for creating optional string pointers in recovery tests
/*
func strPtr(s string) *string {
	return &s
}
*/
