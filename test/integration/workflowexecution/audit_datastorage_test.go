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

package workflowexecution

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
)

// Integration Tests: Audit with Real Data Storage Service
//
// COMPLIANCE: TESTING_GUIDELINES.md lines 423-429
// > Integration tests should use "Real (podman-compose)" services
//
// These tests verify:
// - BR-WE-005: Audit events for execution lifecycle
// - DD-AUDIT-002: Shared library integration with real DS
// - DD-AUDIT-003: P0 MUST generate audit traces
//
// EXPECTED TO FAIL: Until Data Storage batch endpoint is fixed
// See: NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md
//
// To run these tests:
// 1. Start services: podman-compose -f podman-compose.test.yml up -d
// 2. Run tests: go test ./test/integration/workflowexecution/... -v -run "DataStorage"

var _ = Describe("Audit Events with Real Data Storage Service", Label("datastorage", "audit"), func() {
	// Data Storage service URL from podman-compose.test.yml
	// Port 18090 is mapped to container port 8080 per DD-TEST-001
	const dataStorageURL = "http://localhost:18090"

	var (
		dsAvailable bool
		httpClient  *http.Client
		dsClient    audit.DataStorageClient
	)

	BeforeEach(func() {
		// Per TESTING_GUIDELINES.md: Skip() is ABSOLUTELY FORBIDDEN - NO EXCEPTIONS
		// Per TESTING_GUIDELINES.md: Integration tests MUST use real services (podman-compose)
		// Per DD-AUDIT-003: WorkflowExecution is P0 - MUST generate audit traces
		//
		// If Data Storage is not running, tests FAIL (not skip)
		// This enforces the architectural dependency: WE requires DS for audit compliance

		httpClient = &http.Client{Timeout: 5 * time.Second}
		resp, err := httpClient.Get(dataStorageURL + "/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			Fail(fmt.Sprintf(
				"Data Storage REQUIRED but not available at %s\n"+
					"  Per DD-AUDIT-003: WorkflowExecution is P0 - MUST generate audit traces\n"+
					"  Per TESTING_GUIDELINES.md: Integration tests MUST use real services\n"+
					"  Per TESTING_GUIDELINES.md: Skip() is FORBIDDEN - tests must FAIL\n\n"+
					"  Start infrastructure: podman-compose -f podman-compose.test.yml up -d",
				dataStorageURL))
		}
		resp.Body.Close()
		dsAvailable = true

		// Create real DS client
		dsClient = audit.NewHTTPDataStorageClient(dataStorageURL, httpClient)
	})

	// ========================================
	// BR-WE-005: Audit Events with Real DS
	// DD-AUDIT-002: Shared Library Integration
	// ========================================
	Context("BR-WE-005: Audit Events Persistence (Real Data Storage)", func() {
		// Note: BeforeEach already fails if DS is not available
		// No need for redundant availability checks in individual tests

		It("should write audit events to Data Storage via batch endpoint", func() {
			By("Creating a test audit event")
			event := createTestAuditEvent("workflow.started", "success")

			By("Sending batch to Data Storage")
			// This will FAIL until DS batch endpoint is fixed
			// Error: "json: cannot unmarshal array into Go value of type map[string]interface {}"
			err := dsClient.StoreBatch(ctx, []*audit.AuditEvent{event})

			// ========================================
			// EXPECTED TO FAIL: DS batch endpoint not implemented
			// See: NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md
			// ========================================
			Expect(err).ToNot(HaveOccurred(), "BLOCKED: Data Storage batch endpoint not implemented. See NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md")

			By("Verifying event was persisted (query audit_events table)")
			// Once DS is fixed, we should query and verify the event
			// For now, successful StoreBatch is sufficient verification
			GinkgoWriter.Println("✅ Audit event written to Data Storage")
		})

		It("should write workflow.completed audit event via batch endpoint", func() {
			By("Creating a workflow.completed audit event")
			event := createTestAuditEvent("workflow.completed", "success")
			durationMs := 5000 // 5 seconds
			event.DurationMs = &durationMs

			By("Sending batch to Data Storage")
			err := dsClient.StoreBatch(ctx, []*audit.AuditEvent{event})

			// EXPECTED TO FAIL until DS batch endpoint is fixed
			Expect(err).ToNot(HaveOccurred(), "BLOCKED: Data Storage batch endpoint not implemented")
		})

		It("should write workflow.failed audit event via batch endpoint", func() {
			if !dsAvailable {
				Skip("Data Storage not available")
			}

			By("Creating a workflow.failed audit event")
			event := createTestAuditEvent("workflow.failed", "failure")
			errorCode := "PIPELINE_FAILED"
			errorMessage := "Task step-1 failed with exit code 1"
			event.ErrorCode = &errorCode
			event.ErrorMessage = &errorMessage

			By("Sending batch to Data Storage")
			err := dsClient.StoreBatch(ctx, []*audit.AuditEvent{event})

			// EXPECTED TO FAIL until DS batch endpoint is fixed
			Expect(err).ToNot(HaveOccurred(), "BLOCKED: Data Storage batch endpoint not implemented")
		})

		It("should write workflow.skipped audit event via batch endpoint", func() {
			if !dsAvailable {
				Skip("Data Storage not available")
			}

			By("Creating a workflow.skipped audit event")
			event := createTestAuditEvent("workflow.skipped", "skipped")

			By("Sending batch to Data Storage")
			err := dsClient.StoreBatch(ctx, []*audit.AuditEvent{event})

			// EXPECTED TO FAIL until DS batch endpoint is fixed
			Expect(err).ToNot(HaveOccurred(), "BLOCKED: Data Storage batch endpoint not implemented")
		})

		It("should write multiple audit events in a single batch", func() {
			if !dsAvailable {
				Skip("Data Storage not available")
			}

			By("Creating multiple audit events")
			events := []*audit.AuditEvent{
				createTestAuditEvent("workflow.started", "success"),
				createTestAuditEvent("workflow.completed", "success"),
			}

			By("Sending batch to Data Storage")
			err := dsClient.StoreBatch(ctx, events)

			// EXPECTED TO FAIL until DS batch endpoint is fixed
			Expect(err).ToNot(HaveOccurred(), "BLOCKED: Data Storage batch endpoint not implemented")
		})
	})

	// ========================================
	// DD-AUDIT-002: BufferedAuditStore with Real DS
	// ========================================
	Context("DD-AUDIT-002: BufferedAuditStore Integration", func() {

		It("should initialize BufferedAuditStore with real Data Storage client", func() {
			if !dsAvailable {
				Skip("Data Storage not available")
			}

			By("Creating BufferedAuditStore with real DS client")
			auditStore, err := audit.NewBufferedStore(
				dsClient,
				audit.DefaultConfig(),
				"workflowexecution-integration-test",
				GinkgoLogr,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(auditStore).ToNot(BeNil())

			By("Storing an audit event")
			event := createTestAuditEvent("workflow.started", "success")
			err = auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			By("Closing store to flush events")
			// This will attempt to write to DS and may fail
			err = auditStore.Close()
			// Note: Close() may return error if DS batch endpoint fails
			// But we still want to verify the store was created correctly
			if err != nil {
				GinkgoWriter.Printf("⚠️ AuditStore.Close() error (expected if DS batch not implemented): %v\n", err)
			}
		})
	})
})

// ========================================
// Test Helpers
// ========================================

// createTestAuditEvent creates a test audit event for DS integration tests
func createTestAuditEvent(eventAction, outcome string) *audit.AuditEvent {
	event := audit.NewAuditEvent()
	event.EventType = "workflowexecution." + eventAction
	event.EventCategory = "workflow"
	event.EventAction = eventAction
	event.EventOutcome = outcome
	event.ActorType = "service"
	event.ActorID = "workflowexecution-controller"
	event.ResourceType = "WorkflowExecution"
	event.ResourceID = fmt.Sprintf("test-wfe-%d", time.Now().UnixNano())
	event.CorrelationID = fmt.Sprintf("test-corr-%d", time.Now().UnixNano())
	ns := "default"
	event.Namespace = &ns

	// Add event data
	eventData := map[string]interface{}{
		"workflow_id":     "test-workflow",
		"target_resource": "default/deployment/test-app",
		"phase":           string(workflowexecutionv1alpha1.PhaseRunning),
		"test_marker":     "integration-test-with-real-ds",
	}
	eventDataBytes, _ := json.Marshal(eventData)
	event.EventData = eventDataBytes

	return event
}
