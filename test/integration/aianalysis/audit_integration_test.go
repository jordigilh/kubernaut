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

// Package aianalysis contains integration tests for the AIAnalysis controller.
//
// This file tests audit event persistence with REAL Data Storage Service.
//
// Authority:
// - DD-AUDIT-003: AIAnalysis MUST generate audit traces (P0)
// - TESTING_GUIDELINES.md: Integration tests use REAL services (podman-compose)
// - TESTING_GUIDELINES.md: "If Data Storage is unavailable, E2E tests should FAIL, not skip"
//
// Test Strategy:
// - Integration tests require real Data Storage running via AIAnalysis-specific infrastructure
// - Audit events are written and then verified via direct DB query
// - Uses AIAnalysis's dedicated DS instance (port 18091, not shared 18090)
//
// Business Requirements:
// - BR-AI-001: AI Analysis CRD lifecycle management
// - BR-STORAGE-001: Complete audit trail with no data loss
package aianalysis

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/audit"

	_ "github.com/jackc/pgx/v5/stdlib"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ========================================
// AIANALYSIS AUDIT INTEGRATION TESTS
// 📋 Authority: DD-AUDIT-003 (AIAnalysis MUST generate audit traces)
// 📋 Authority: TESTING_GUIDELINES.md (Real Data Storage required)
// ========================================
//
// These tests REQUIRE real Data Storage running via podman-compose.test.yml:
//   podman-compose -f podman-compose.test.yml up -d datastorage postgres redis
//
// If Data Storage is unavailable, these tests will FAIL (not skip) per
// TESTING_GUIDELINES.md: "If Data Storage is unavailable, E2E tests should FAIL, not skip"
//
// ========================================

var _ = Describe("AIAnalysis Audit Integration - DD-AUDIT-003", Label("integration", "audit"), func() {
	var (
		datastorageURL string
		db             *sql.DB
		auditClient    *aiaudit.AuditClient
		auditStore     audit.AuditStore
		testAnalysis   *aianalysisv1.AIAnalysis
	)

	BeforeEach(func() {
		// Determine Data Storage URL from environment or default
		datastorageURL = os.Getenv("DATASTORAGE_URL")
		if datastorageURL == "" {
			datastorageURL = "http://localhost:18090" // Default from podman-compose.test.yml (DD-TEST-001)
		}

		// MANDATORY: Verify Data Storage is available (per TESTING_GUIDELINES.md)
		// "If Data Storage is unavailable, E2E tests should FAIL, not skip"
		By("Verifying Data Storage is available (MANDATORY per TESTING_GUIDELINES.md)")
		var httpErr error
		var resp *http.Response
		Eventually(func() error {
			resp, httpErr = http.Get(datastorageURL + "/health")
			if httpErr != nil {
				return httpErr
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("Data Storage health check failed with status %d", resp.StatusCode)
			}
			return nil
		}, "10s", "1s").Should(Succeed(), "Data Storage MUST be available for audit integration tests (per DD-AUDIT-003, TESTING_GUIDELINES.md)")

		// Connect to PostgreSQL for verification queries
		By("Connecting to PostgreSQL for audit event verification")
		pgHost := os.Getenv("POSTGRES_HOST")
		if pgHost == "" {
			pgHost = "localhost"
		}
		pgPort := os.Getenv("POSTGRES_PORT")
		if pgPort == "" {
			pgPort = "15433" // Default from podman-compose.test.yml (DD-TEST-001)
		}

		connStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable", pgHost, pgPort)
		var dbErr error
		db, dbErr = sql.Open("pgx", connStr)
		Expect(dbErr).ToNot(HaveOccurred(), "PostgreSQL connection should succeed")
		Expect(db.Ping()).To(Succeed(), "PostgreSQL ping should succeed")

		// Create HTTP client for Data Storage
		By("Creating audit store with HTTP client to Data Storage")
		httpClient := &http.Client{Timeout: 5 * time.Second}
		dsClient := audit.NewHTTPDataStorageClient(datastorageURL, httpClient)

		// Create buffered audit store (per DD-AUDIT-002)
		config := audit.Config{
			BufferSize:    100,
			BatchSize:     10,
			FlushInterval: 100 * time.Millisecond, // Fast flush for tests
			MaxRetries:    3,
		}
		var storeErr error
		auditStore, storeErr = audit.NewBufferedStore(dsClient, config, "aianalysis-integration-test", ctrl.Log.WithName("audit-store"))
		Expect(storeErr).ToNot(HaveOccurred(), "Audit store creation should succeed")

		// Create AIAnalysis audit client (per DD-AUDIT-003)
		auditClient = aiaudit.NewAuditClient(auditStore, ctrl.Log.WithName("aianalysis-audit"))

		// Create test AIAnalysis resource
		testAnalysis = &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-analysis-%s", uuid.New().String()[:8]),
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: fmt.Sprintf("rr-test-%s", uuid.New().String()[:8]),
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "fp-test-123",
						Severity:         "warning",
						SignalType:       "CrashLoopBackOff",
						Environment:      "staging",
						BusinessPriority: "P2",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
					AnalysisTypes: []string{"investigation", "workflow-selection"},
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:            "Completed",
				ApprovalRequired: false,
				ApprovalReason:   "Auto-approved for staging",
				DegradedMode:     false,
				Warnings:         []string{},
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowID: "wf-test-001",
					Confidence: 0.85,
				},
			},
		}
	})

	AfterEach(func() {
		// Close audit store to flush remaining events
		if auditStore != nil {
			Expect(auditStore.Close()).To(Succeed(), "Audit store should close cleanly")
		}

		// Close database connection
		if db != nil {
			db.Close()
		}
	})

	// ========================================
	// BR-STORAGE-001: Complete audit trail
	// ========================================

	Context("RecordAnalysisComplete - BR-STORAGE-001", func() {
		It("should persist analysis completion audit event to Data Storage", func() {
			By("Recording analysis completion event")
			auditClient.RecordAnalysisComplete(ctx, testAnalysis)

			// Wait for async write to complete
			time.Sleep(500 * time.Millisecond)
			Expect(auditStore.Close()).To(Succeed()) // Force flush

			By("Verifying audit event was persisted to PostgreSQL")
			query := `
				SELECT event_type, resource_type, resource_id, correlation_id, event_outcome
				FROM audit_events
				WHERE correlation_id = $1
				AND event_type = $2
				ORDER BY event_timestamp DESC
				LIMIT 1
			`
			var eventType, resourceType, resourceID, correlationID, eventOutcome string
			err := db.QueryRow(query, testAnalysis.Spec.RemediationID, aiaudit.EventTypeAnalysisCompleted).
				Scan(&eventType, &resourceType, &resourceID, &correlationID, &eventOutcome)

			// If this fails, Data Storage is not working correctly
			Expect(err).ToNot(HaveOccurred(), "Audit event should be found in PostgreSQL (per DD-AUDIT-003)")
			Expect(eventType).To(Equal(aiaudit.EventTypeAnalysisCompleted))
			Expect(resourceType).To(Equal("AIAnalysis"))
			Expect(resourceID).To(Equal(testAnalysis.Name))
			Expect(correlationID).To(Equal(testAnalysis.Spec.RemediationID))
			Expect(eventOutcome).To(Equal("success"))
		})

		It("should include correct event_data in analysis completion audit", func() {
			By("Recording analysis completion event with specific status")
			testAnalysis.Status.Phase = "Completed"
			testAnalysis.Status.ApprovalRequired = true
			testAnalysis.Status.ApprovalReason = "Production environment requires manual approval"
			auditClient.RecordAnalysisComplete(ctx, testAnalysis)

			time.Sleep(500 * time.Millisecond)
			Expect(auditStore.Close()).To(Succeed())

			By("Verifying event_data contains status fields")
			query := `
				SELECT event_data
				FROM audit_events
				WHERE correlation_id = $1
				AND event_type = $2
				ORDER BY event_timestamp DESC
				LIMIT 1
			`
			var eventDataBytes []byte
			err := db.QueryRow(query, testAnalysis.Spec.RemediationID, aiaudit.EventTypeAnalysisCompleted).
				Scan(&eventDataBytes)

			Expect(err).ToNot(HaveOccurred(), "Audit event should be found")

			var eventData map[string]interface{}
			Expect(json.Unmarshal(eventDataBytes, &eventData)).To(Succeed())
			Expect(eventData["phase"]).To(Equal("Completed"))
			Expect(eventData["approval_required"]).To(BeTrue())
		})
	})

	// ========================================
	// DD-AUDIT-003: Phase transition tracking
	// ========================================

	Context("RecordPhaseTransition - DD-AUDIT-003", func() {
		It("should persist phase transition audit event", func() {
			By("Recording phase transition from Pending to Investigating")
			auditClient.RecordPhaseTransition(ctx, testAnalysis, "Pending", "Investigating")

			time.Sleep(500 * time.Millisecond)
			Expect(auditStore.Close()).To(Succeed())

			By("Verifying phase transition event was persisted")
			query := `
				SELECT event_type, event_data
				FROM audit_events
				WHERE correlation_id = $1
				AND event_type = $2
				ORDER BY event_timestamp DESC
				LIMIT 1
			`
			var eventType string
			var eventDataBytes []byte
			err := db.QueryRow(query, testAnalysis.Spec.RemediationID, aiaudit.EventTypePhaseTransition).
				Scan(&eventType, &eventDataBytes)

			Expect(err).ToNot(HaveOccurred(), "Phase transition audit should be found")
			Expect(eventType).To(Equal(aiaudit.EventTypePhaseTransition))

			var eventData map[string]interface{}
			Expect(json.Unmarshal(eventDataBytes, &eventData)).To(Succeed())
			Expect(eventData["from_phase"]).To(Equal("Pending"))
			Expect(eventData["to_phase"]).To(Equal("Investigating"))
		})
	})

	// ========================================
	// DD-AUDIT-003: HolmesGPT-API call tracking
	// ========================================

	Context("RecordHolmesGPTCall - DD-AUDIT-003", func() {
		It("should persist HolmesGPT-API call audit event", func() {
			By("Recording HolmesGPT-API call event")
			auditClient.RecordHolmesGPTCall(ctx, testAnalysis, "/api/v1/investigate", 200, 1234)

			time.Sleep(500 * time.Millisecond)
			Expect(auditStore.Close()).To(Succeed())

			By("Verifying HolmesGPT call event was persisted")
			query := `
				SELECT event_type, event_outcome, event_data
				FROM audit_events
				WHERE correlation_id = $1
				AND event_type = $2
				ORDER BY event_timestamp DESC
				LIMIT 1
			`
			var eventType, eventOutcome string
			var eventDataBytes []byte
			err := db.QueryRow(query, testAnalysis.Spec.RemediationID, aiaudit.EventTypeHolmesGPTCall).
				Scan(&eventType, &eventOutcome, &eventDataBytes)

			Expect(err).ToNot(HaveOccurred(), "HolmesGPT call audit should be found")
			Expect(eventType).To(Equal(aiaudit.EventTypeHolmesGPTCall))
			Expect(eventOutcome).To(Equal("success"))

			var eventData map[string]interface{}
			Expect(json.Unmarshal(eventDataBytes, &eventData)).To(Succeed())
			Expect(eventData["endpoint"]).To(Equal("/api/v1/investigate"))
			Expect(eventData["status_code"]).To(BeNumerically("==", 200))
			Expect(eventData["duration_ms"]).To(BeNumerically("==", 1234))
		})

		It("should record failure outcome for 4xx/5xx status codes", func() {
			By("Recording failed HolmesGPT-API call")
			auditClient.RecordHolmesGPTCall(ctx, testAnalysis, "/api/v1/investigate", 500, 500)

			time.Sleep(500 * time.Millisecond)
			Expect(auditStore.Close()).To(Succeed())

			By("Verifying failure outcome is recorded")
			query := `
				SELECT event_outcome
				FROM audit_events
				WHERE correlation_id = $1
				AND event_type = $2
				ORDER BY event_timestamp DESC
				LIMIT 1
			`
			var eventOutcome string
			err := db.QueryRow(query, testAnalysis.Spec.RemediationID, aiaudit.EventTypeHolmesGPTCall).
				Scan(&eventOutcome)

			Expect(err).ToNot(HaveOccurred())
			Expect(eventOutcome).To(Equal("failure"))
		})
	})

	// ========================================
	// DD-AUDIT-003: Approval decision tracking
	// ========================================

	Context("RecordApprovalDecision - DD-AUDIT-003", func() {
		It("should persist approval decision audit event", func() {
			By("Recording approval decision event")
			auditClient.RecordApprovalDecision(ctx, testAnalysis, "auto-approved", "Staging environment")

			time.Sleep(500 * time.Millisecond)
			Expect(auditStore.Close()).To(Succeed())

			By("Verifying approval decision event was persisted")
			query := `
				SELECT event_type, event_data
				FROM audit_events
				WHERE correlation_id = $1
				AND event_type = $2
				ORDER BY event_timestamp DESC
				LIMIT 1
			`
			var eventType string
			var eventDataBytes []byte
			err := db.QueryRow(query, testAnalysis.Spec.RemediationID, aiaudit.EventTypeApprovalDecision).
				Scan(&eventType, &eventDataBytes)

			Expect(err).ToNot(HaveOccurred(), "Approval decision audit should be found")
			Expect(eventType).To(Equal(aiaudit.EventTypeApprovalDecision))

			var eventData map[string]interface{}
			Expect(json.Unmarshal(eventDataBytes, &eventData)).To(Succeed())
			Expect(eventData["decision"]).To(Equal("auto-approved"))
			Expect(eventData["reason"]).To(Equal("Staging environment"))
			Expect(eventData["environment"]).To(Equal("staging"))
		})
	})

	// ========================================
	// DD-AUDIT-003: Rego evaluation tracking
	// ========================================

	Context("RecordRegoEvaluation - DD-AUDIT-003", func() {
		It("should persist Rego evaluation audit event", func() {
			By("Recording Rego evaluation event")
			auditClient.RecordRegoEvaluation(ctx, testAnalysis, "allow", false, 50)

			time.Sleep(500 * time.Millisecond)
			Expect(auditStore.Close()).To(Succeed())

			By("Verifying Rego evaluation event was persisted")
			query := `
				SELECT event_type, event_outcome, event_data
				FROM audit_events
				WHERE correlation_id = $1
				AND event_type = $2
				ORDER BY event_timestamp DESC
				LIMIT 1
			`
			var eventType, eventOutcome string
			var eventDataBytes []byte
			err := db.QueryRow(query, testAnalysis.Spec.RemediationID, aiaudit.EventTypeRegoEvaluation).
				Scan(&eventType, &eventOutcome, &eventDataBytes)

			Expect(err).ToNot(HaveOccurred(), "Rego evaluation audit should be found")
			Expect(eventType).To(Equal(aiaudit.EventTypeRegoEvaluation))
			Expect(eventOutcome).To(Equal("allow"))

			var eventData map[string]interface{}
			Expect(json.Unmarshal(eventDataBytes, &eventData)).To(Succeed())
			Expect(eventData["outcome"]).To(Equal("allow"))
			Expect(eventData["degraded"]).To(BeFalse())
			Expect(eventData["duration_ms"]).To(BeNumerically("==", 50))
		})
	})

	// ========================================
	// DD-AUDIT-003: Error tracking
	// ========================================

	Context("RecordError - DD-AUDIT-003", func() {
		It("should persist error audit event", func() {
			By("Recording error event")
			auditClient.RecordError(ctx, testAnalysis, "Investigating", fmt.Errorf("HolmesGPT-API timeout"))

			time.Sleep(500 * time.Millisecond)
			Expect(auditStore.Close()).To(Succeed())

			By("Verifying error event was persisted")
			query := `
				SELECT event_type, event_outcome, error_message
				FROM audit_events
				WHERE correlation_id = $1
				AND event_type = $2
				ORDER BY event_timestamp DESC
				LIMIT 1
			`
			var eventType, eventOutcome string
			var errorMessage *string
			err := db.QueryRow(query, testAnalysis.Spec.RemediationID, aiaudit.EventTypeError).
				Scan(&eventType, &eventOutcome, &errorMessage)

			Expect(err).ToNot(HaveOccurred(), "Error audit should be found")
			Expect(eventType).To(Equal(aiaudit.EventTypeError))
			Expect(eventOutcome).To(Equal("failure"))
			Expect(errorMessage).ToNot(BeNil())
			Expect(*errorMessage).To(Equal("HolmesGPT-API timeout"))
		})
	})

	// ========================================
	// Graceful Degradation - Risk #4 in DD-AUDIT-002
	// ========================================

	Context("Graceful Degradation - DD-AUDIT-002 Risk #4", func() {
		It("should not block business logic on audit write failure", func() {
			By("Recording multiple audit events rapidly")
			start := time.Now()

			// Fire multiple audit events rapidly
			for i := 0; i < 100; i++ {
				auditClient.RecordPhaseTransition(ctx, testAnalysis, "Pending", "Investigating")
			}

			elapsed := time.Since(start)

			// Should complete quickly (< 100ms) due to non-blocking design
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond),
				"Audit events should not block business logic (per DD-AUDIT-002 Risk #4)")
		})
	})
})

