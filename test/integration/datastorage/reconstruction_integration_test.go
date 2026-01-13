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

package datastorage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// ========================================
// RECONSTRUCTION ENDPOINT INTEGRATION TESTS
// ========================================
//
// Purpose: Test reconstruction REST API endpoint against REAL PostgreSQL database
// to validate end-to-end reconstruction workflow.
//
// Business Requirements:
// - BR-AUDIT-006: RemediationRequest Reconstruction from Audit Traces
//
// Test Strategy:
// - Uses REAL PostgreSQL database (not mocks)
// - Seeds audit events for gateway and orchestrator
// - Calls reconstruction HTTP endpoint
// - Validates YAML output and validation results
//
// Test Plan Reference:
// - docs/development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md
// - Test Tier: Integration Tests (Tier 2)
// - Gap Coverage: Gaps #1-3, #8 (E2E reconstruction flow)
//
// ========================================

var _ = Describe("Reconstruction Integration Tests (BR-AUDIT-006)", func() {
	var (
		testServer    *server.Server
		auditRepo     *repository.AuditEventsRepository
		testID        string
		correlationID string
	)

	BeforeEach(func() {
		// Create audit events repository
		auditRepo = repository.NewAuditEventsRepository(db.DB, logger)

		// Generate unique test ID for isolation
		testID = generateTestID()
		correlationID = fmt.Sprintf("test-reconstruction-%s", testID)

		// Create test server with real database
		var err error
		testServer, err = server.NewServer(
			db.DB,
			redisClient,
			logger,
			server.WithAddress(":0"), // Random port for testing
		)
		Expect(err).ToNot(HaveOccurred())

		// Clean up test data
		_, err = db.ExecContext(ctx,
			"DELETE FROM audit_events WHERE correlation_id = $1",
			correlationID)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Clean up test data
		if db != nil {
			_, _ = db.ExecContext(ctx,
				"DELETE FROM audit_events WHERE correlation_id = $1",
				correlationID)
		}
	})

	// ========================================
	// SUCCESSFUL RECONSTRUCTION TESTS
	// ========================================
	Context("INTEGRATION-01: Complete audit trail", func() {
		It("should reconstruct RR from gateway and orchestrator events", func() {
			// ARRANGE: Seed audit events
			gatewayEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().Add(-10 * time.Second).UTC(),
				EventType:      "gateway.signal.received",
				EventCategory:  "gateway",
				EventAction:    "received",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "Signal",
				ResourceID:     "signal-123",
				EventData: map[string]interface{}{
					"signal_name": "HighCPU",
					"signal_type": "prometheus-alert",
					"signal_labels": map[string]interface{}{
						"alertname": "HighCPU",
						"severity":  "critical",
					},
					"signal_annotations": map[string]interface{}{
						"summary": "CPU usage is high",
					},
					"original_payload": map[string]interface{}{
						"alert": "data",
					},
				},
			}

			_, err := auditRepo.Create(ctx, gatewayEvent)
			Expect(err).ToNot(HaveOccurred())

			orchestratorEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
				EventType:      "orchestrator.lifecycle.created",
				EventCategory:  "orchestrator",
				EventAction:    "created",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "RemediationRequest",
				ResourceID:     "rr-123",
				EventData: map[string]interface{}{
					"timeout_config": map[string]interface{}{
						"global":     "1h",
						"processing": "10m",
						"analyzing":  "15m",
					},
				},
			}

			_, err = auditRepo.Create(ctx, orchestratorEvent)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Call reconstruction endpoint
			req := httptest.NewRequest(http.MethodPost,
				fmt.Sprintf("/api/v1/audit/remediation-requests/%s/reconstruct", correlationID),
				nil)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			testServer.ServeHTTP(rr, req)

			// ASSERT: HTTP 200 OK
			Expect(rr.Code).To(Equal(http.StatusOK), "Expected HTTP 200 OK")

			// ASSERT: Valid JSON response
			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

			// ASSERT: Response structure
			Expect(response).To(HaveKey("remediation_request_yaml"))
			Expect(response).To(HaveKey("validation"))
			Expect(response).To(HaveKey("reconstructed_at"))
			Expect(response).To(HaveKey("correlation_id"))
			Expect(response["correlation_id"]).To(Equal(correlationID))

			// ASSERT: YAML contains K8s structure
			yamlContent := response["remediation_request_yaml"].(string)
			Expect(yamlContent).To(ContainSubstring("apiVersion: remediation.kubernaut.ai/v1alpha1"))
			Expect(yamlContent).To(ContainSubstring("kind: RemediationRequest"))
			Expect(yamlContent).To(ContainSubstring("metadata:"))
			Expect(yamlContent).To(ContainSubstring("spec:"))
			Expect(yamlContent).To(ContainSubstring("status:"))

			// ASSERT: Validation results
			validation := response["validation"].(map[string]interface{})
			Expect(validation["is_valid"]).To(BeTrue())
			Expect(validation["completeness"]).To(BeNumerically(">=", 50))
			Expect(validation["errors"]).To(BeEmpty())

			// ASSERT: YAML can be parsed as RemediationRequest
			var reconstructedRR remediationv1.RemediationRequest
			err = yaml.Unmarshal([]byte(yamlContent), &reconstructedRR)
			Expect(err).ToNot(HaveOccurred(), "YAML should be valid RemediationRequest")

			// ASSERT: Reconstructed fields match input
			Expect(reconstructedRR.Spec.SignalName).To(Equal("HighCPU"))
			Expect(reconstructedRR.Spec.SignalType).To(Equal("prometheus-alert"))
			Expect(reconstructedRR.Spec.SignalLabels).To(HaveKeyWithValue("alertname", "HighCPU"))
			Expect(reconstructedRR.Spec.SignalLabels).To(HaveKeyWithValue("severity", "critical"))
			Expect(reconstructedRR.Status.TimeoutConfig.Global.Duration).To(Equal(time.Hour))
		})
	})

	// ========================================
	// ERROR HANDLING TESTS
	// ========================================
	Context("INTEGRATION-02: Missing correlation ID", func() {
		It("should return HTTP 404 for non-existent correlation ID", func() {
			// ACT: Call reconstruction with non-existent correlation ID
			nonExistentID := fmt.Sprintf("nonexistent-%s", testID)
			req := httptest.NewRequest(http.MethodPost,
				fmt.Sprintf("/api/v1/audit/remediation-requests/%s/reconstruct", nonExistentID),
				nil)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			testServer.ServeHTTP(rr, req)

			// ASSERT: HTTP 404 Not Found
			Expect(rr.Code).To(Equal(http.StatusNotFound), "Expected HTTP 404 Not Found")

			// ASSERT: RFC 7807 error response
			var errorResponse map[string]interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResponse).To(HaveKey("type"))
			Expect(errorResponse).To(HaveKey("title"))
			Expect(errorResponse).To(HaveKey("status"))
			Expect(errorResponse["status"]).To(Equal(float64(404)))
		})
	})

	Context("INTEGRATION-03: Missing gateway event", func() {
		It("should return HTTP 400 when only orchestrator event exists", func() {
			// ARRANGE: Seed only orchestrator event (no gateway event)
			orchestratorEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().UTC(),
				EventType:      "orchestrator.lifecycle.created",
				EventCategory:  "orchestrator",
				EventAction:    "created",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "RemediationRequest",
				ResourceID:     "rr-123",
				EventData: map[string]interface{}{
					"timeout_config": map[string]interface{}{
						"global": "1h",
					},
				},
			}

			_, err := auditRepo.Create(ctx, orchestratorEvent)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Call reconstruction endpoint
			req := httptest.NewRequest(http.MethodPost,
				fmt.Sprintf("/api/v1/audit/remediation-requests/%s/reconstruct", correlationID),
				nil)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			testServer.ServeHTTP(rr, req)

			// ASSERT: HTTP 400 Bad Request
			Expect(rr.Code).To(Equal(http.StatusBadRequest), "Expected HTTP 400 Bad Request")

			// ASSERT: RFC 7807 error with gateway event message
			var errorResponse map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &errorResponse)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResponse["type"].(string)).To(ContainSubstring("missing-gateway-event"))
			Expect(errorResponse["detail"].(string)).To(ContainSubstring("gateway.signal.received"))
		})
	})

	// NOTE: Additional integration tests for partial reconstruction,
	// malformed events, and performance benchmarks can be added here
})
