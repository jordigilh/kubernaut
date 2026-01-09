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

// Package workflowexecution contains integration tests for Gap 5-6.
//
// Business Requirements:
// - BR-AUDIT-005 v2.0 (Gap 5-6 - Workflow References)
// - BR-WE-013 (Audit-tracked workflow execution)
//
// Test Strategy:
// This test validates that WorkflowExecution controller emits 2 audit events:
// 1. workflow.selection.completed - When workflow is selected
// 2. execution.workflow.started - When PipelineRun is created
//
// Both events share the same correlation_id (WorkflowExecution CRD name)
// for complete RR reconstruction (SOC2 compliance).
//
// Infrastructure:
// - EnvTest (simulated K8s API server)
// - PostgreSQL: Persistence
// - Redis: Caching
// - Data Storage: Audit trail (REAL service, not mocked, uses shared dataStorageBaseURL)
// - WorkflowExecution Controller: Real controller with real audit client
//
// Test Pattern: Follows audit_flow_integration_test.go (proven, anti-pattern-free)
package workflowexecution

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// GAP 5-6: WORKFLOW SELECTION & EXECUTION REFS
// BR-AUDIT-005, BR-WE-013
// ========================================
//
// These tests validate the 2-event audit pattern for workflow lifecycle:
// 1. workflow.selection.completed (Gap #5)
// 2. execution.workflow.started (Gap #6)
//
// Execution: Serial (for reliability, follows audit_flow_integration_test.go pattern)
// Infrastructure: Uses existing WE integration test infrastructure (auto-started)
//
// ========================================
var _ = Describe("BR-AUDIT-005 Gap 5-6: Workflow Selection & Execution", Label("integration", "audit", "workflow", "soc2"), func() {
	var (
		ctx       context.Context
		namespace string
		dsClient  *dsgen.ClientWithResponses
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Verify Data Storage is available
		// Per TESTING_GUIDELINES.md: Skip() is ABSOLUTELY FORBIDDEN - tests MUST fail
		// Per DD-AUDIT-003: WorkflowExecution REQUIRES audit capability
		httpClient := &http.Client{Timeout: 5 * time.Second}
		resp, err := httpClient.Get(dataStorageBaseURL + "/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			Fail(fmt.Sprintf(
				"REQUIRED: Data Storage not available at %s\n"+
					"  Per DD-AUDIT-003: WorkflowExecution MUST have audit capability\n"+
					"  Per TESTING_GUIDELINES.md: Integration tests MUST use real services\n"+
					"  Per TESTING_GUIDELINES.md: Skip() is FORBIDDEN - tests must FAIL\n\n"+
					"  Health check error: %v\n"+
					"  Start infrastructure: make test-integration-workflowexecution\n",
				dataStorageBaseURL, err))
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}

		// Create test namespace
		namespace = fmt.Sprintf("we-gap56-test-%d", time.Now().Unix())
		testNs := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		Expect(k8sClient.Create(ctx, testNs)).To(Succeed())

		// Data Storage client (use shared infrastructure port from suite_test.go)
		dsClient, err = dsgen.NewClientWithResponses(dataStorageBaseURL)
		Expect(err).ToNot(HaveOccurred(), "Failed to create DataStorage OpenAPI client")
	})

	AfterEach(func() {
		// Cleanup namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		_ = k8sClient.Delete(ctx, ns)
	})

	// ========================================
	// TEST 1: Happy Path - Both Events Emitted
	// ========================================
	Context("when workflow is selected and execution starts", func() {
		It("should emit both workflow.selection.completed and execution.workflow.started events", func() {
			// BUSINESS SCENARIO:
			// 1. WorkflowExecution CRD created by Remediation Orchestrator
			// 2. Controller selects workflow from spec.WorkflowRef
			// 3. Controller creates Tekton PipelineRun
			// 4. MUST emit 2 audit events for SOC2 compliance

			By("1. Creating WorkflowExecution CRD (BUSINESS LOGIC TRIGGER)")
			wfeName := fmt.Sprintf("gap56-happy-%s", uuid.New().String()[:8])
			rrName := "test-rr-" + wfeName
			// DD-AUDIT-CORRELATION-001: Correlation ID = RemediationRequest name
			correlationID := rrName

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:       wfeName,
					Namespace:  namespace,
					Generation: 1,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       rrName, // Correlation ID source!
						Namespace:  namespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "k8s-restart-pod-v1", // Label-safe: no slashes
						Version:        "v1.0.0",
						ContainerImage: "ghcr.io/kubernaut/workflows/restart-pod@sha256:abc123",
					},
					TargetResource: fmt.Sprintf("%s/deployment/test-app", namespace),
					Parameters: map[string]string{
						"pod_name":  "test-pod-123",
						"namespace": namespace,
					},
				},
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

		By("2. Wait for Gap 5-6 events to appear (CRD controller async)")
		// CRD controllers are async - use Eventually with 60s timeout
		// DD-TESTING-001: Deterministic event count per event type
		// Flush INSIDE Eventually to ensure controller has reconciled and buffered events first
		Eventually(func() bool {
			flushAuditBuffer() // Flush on each poll attempt
			// Query all audit events for this correlation_id
			events, err := queryAuditEvents(dsClient, correlationID, nil)
			if err != nil {
				GinkgoWriter.Printf("⚠️  Query error: %v\n", err)
				return false
			}
			GinkgoWriter.Printf("📊 Query result: %d events found\n", len(events))

				// Count by type to check if Gap 5-6 events are present
				eventCounts := countEventsByType(events)
				hasSelection := eventCounts["workflow.selection.completed"] >= 1
				hasExecution := eventCounts["execution.workflow.started"] >= 1
				return hasSelection && hasExecution
			}, 60*time.Second, 1*time.Second).Should(BeTrue(),
				"Should have workflow.selection.completed and execution.workflow.started events (Gap 5-6)")

			By("3. Validate exact event counts per type (DD-TESTING-001)")
			allEvents, err := queryAuditEvents(dsClient, correlationID, nil)
			Expect(err).ToNot(HaveOccurred())

			// Count events by type (DD-TESTING-001: Deterministic validation)
			eventCounts := countEventsByType(allEvents)

			// Gap 5-6: Validate exactly 1 of each required event type
			Expect(eventCounts["workflow.selection.completed"]).To(Equal(1),
				"Gap 5: Should have exactly 1 workflow.selection.completed event")
			Expect(eventCounts["execution.workflow.started"]).To(Equal(1),
				"Gap 6: Should have exactly 1 execution.workflow.started event")

			// Workflow may complete during test - if so, validate exactly 1 completion event
			if completionCount, exists := eventCounts["workflow.completed"]; exists {
				Expect(completionCount).To(Equal(1),
					"If workflow completed, should have exactly 1 workflow.completed event")
			}

			By("4. Validate workflow.selection.completed event structure")
			selectionEvents := filterEventsByType(allEvents, "workflow.selection.completed")
			Expect(len(selectionEvents)).To(Equal(1), "Should have exactly 1 selection event")

		selectionEvent := selectionEvents[0]
		validateEventMetadata(selectionEvent, "workflow", correlationID)
		Expect(selectionEvent.ActorID.Value).To(Equal("workflowexecution-controller"))
		Expect(string(selectionEvent.EventOutcome)).To(Equal("success"))

		// Validate event_data structure (Gap #5) - OGEN-MIGRATION
		// Per Q4 Answer: Flat structure, no nested selected_workflow_ref
		eventData := selectionEvent.EventData.GetWorkflowExecutionAuditPayload()
		Expect(eventData.Nil).To(BeFalse(), "EventData should be WorkflowExecutionAuditPayload")

		// Access flat fields directly
		Expect(eventData.Value.WorkflowID).To(Equal("k8s-restart-pod-v1"))
		Expect(eventData.Value.WorkflowVersion).To(Equal("v1.0.0"))
		Expect(eventData.Value.ContainerImage).ToNot(BeEmpty())
		Expect(eventData.Value.Phase).To(Equal("Pending"))

		By("5. Validate execution.workflow.started event structure")
		executionEvents := filterEventsByType(allEvents, "execution.workflow.started")
		Expect(len(executionEvents)).To(Equal(1), "Should have exactly 1 execution event")

		executionEvent := executionEvents[0]
		validateEventMetadata(executionEvent, "execution", correlationID)
		Expect(executionEvent.ActorID.Value).To(Equal("workflowexecution-controller"))
		Expect(string(executionEvent.EventOutcome)).To(Equal("success"))

		// Validate event_data structure (Gap #6) - OGEN-MIGRATION
		// Per Q4 Answer: Flat structure with PipelineRunName field
		execEventData := executionEvent.EventData.GetWorkflowExecutionAuditPayload()
		Expect(execEventData.Nil).To(BeFalse(), "EventData should be WorkflowExecutionAuditPayload")

		// Access flat fields directly
		Expect(execEventData.Value.WorkflowID).To(Equal("k8s-restart-pod-v1"))
		Expect(execEventData.Value.PipelineRunName.IsSet()).To(BeTrue(), "PipelineRun name should be set")
		Expect(execEventData.Value.PipelineRunName.Value).ToNot(BeEmpty())
		})
	})

	// ========================================
	// TEST 2: Selection Only - Execution Not Started
	// ========================================
	Context("when workflow is selected but execution hasn't started yet", func() {
		It("should emit workflow.selection.completed event immediately", func() {
			// BUSINESS SCENARIO:
			// Testing CRD controller async behavior - workflow selection
			// happens before PipelineRun creation (timing validation)

			By("1. Creating WorkflowExecution CRD")
			wfeName := fmt.Sprintf("gap56-selection-%s", uuid.New().String()[:8])
			rrName := "test-rr-" + wfeName
			// DD-AUDIT-CORRELATION-001: Correlation ID = RemediationRequest name
			correlationID := rrName

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:       wfeName,
					Namespace:  namespace,
					Generation: 1,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       rrName, // Correlation ID source!
						Namespace:  namespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "k8s-scale-deployment-v1", // Label-safe: no slashes
						Version:        "v1.0.0",
						ContainerImage: "ghcr.io/kubernaut/workflows/scale@sha256:def456",
					},
					TargetResource: fmt.Sprintf("%s/deployment/api-server", namespace),
				},
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

		By("2. Wait for workflow.selection.completed event (fast path)")
		// DD-TESTING-001: Deterministic event count (exactly 1 event)
		// Flush INSIDE Eventually to ensure controller has reconciled and buffered event first
		Eventually(func() int {
			flushAuditBuffer() // Flush on each poll attempt
			selectionType := "workflow.selection.completed"
			events, err := queryAuditEvents(dsClient, correlationID, &selectionType)
			if err != nil {
				return 0
			}
			return len(events)
		}, 30*time.Second, 1*time.Second).Should(Equal(1),
			"Should have exactly 1 workflow.selection.completed event")

			By("3. Validate selection event is present")
			selectionType := "workflow.selection.completed"
			selectionEvents, err := queryAuditEvents(dsClient, correlationID, &selectionType)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(selectionEvents)).To(Equal(1), "Should have exactly 1 selection event")

		// Validate event structure
		selectionEvent := selectionEvents[0]
		validateEventMetadata(selectionEvent, "workflow", correlationID)

		// Validate event_data structure - OGEN-MIGRATION
		// Per Q4 Answer: Flat structure, no nested selected_workflow_ref
		eventData := selectionEvent.EventData.GetWorkflowExecutionAuditPayload()
		Expect(eventData.Nil).To(BeFalse(), "EventData should be WorkflowExecutionAuditPayload")

		// Access flat fields directly
		Expect(eventData.Value.WorkflowID).To(Equal("k8s-scale-deployment-v1"))
		})
	})
})

// ========================================
// HELPER FUNCTIONS (DD-TESTING-001 Compliant)
// Reused from existing audit_flow_integration_test.go
// ========================================

// queryAuditEvents queries Data Storage for audit events
// DD-API-001: Uses OpenAPI client
// DD-TESTING-001: Type-safe query with optional event type filter
func queryAuditEvents(
	client *dsgen.ClientWithResponses,
	correlationID string,
	eventType *string,
) ([]dsgen.AuditEvent, error) {
	limit := 100
	params := &dsgen.QueryAuditEventsParams{
		CorrelationId: &correlationID,
		EventType:     eventType,
		Limit:         &limit,
	}

	resp, err := client.QueryAuditEventsWithResponse(context.Background(), params)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil {
		return []dsgen.AuditEvent{}, nil
	}

	return *resp.JSON200.Data, nil
}

// countEventsByType groups events by type and returns counts
// DD-TESTING-001: Deterministic event count validation
func countEventsByType(events []dsgen.AuditEvent) map[string]int {
	counts := make(map[string]int)
	for _, event := range events {
		counts[event.EventType]++
	}
	return counts
}

// filterEventsByType returns events of specific type
func filterEventsByType(events []dsgen.AuditEvent, eventType string) []dsgen.AuditEvent {
	var filtered []dsgen.AuditEvent
	for _, event := range events {
		if event.EventType == eventType {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// validateEventMetadata validates common event metadata fields
// DD-TESTING-001: Standard metadata validation
func validateEventMetadata(event dsgen.AuditEvent, category, correlationID string) {
	Expect(event.EventType).ToNot(BeEmpty())
	Expect(string(event.EventCategory)).To(Equal(category))
	Expect(event.CorrelationId).To(Equal(correlationID))
	Expect(string(event.EventOutcome)).ToNot(BeEmpty())
	Expect(event.ActorId).ToNot(BeNil())
}
