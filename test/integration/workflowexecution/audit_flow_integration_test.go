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
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// WorkflowExecution Audit Flow Integration Tests
//
// COMPLIANCE: BR-WE-005 - Audit events for execution lifecycle
// PATTERN: Flow-based testing (not infrastructure testing)
//
// These tests verify that the WorkflowExecution controller:
// 1. Emits audit events during reconciliation (BUSINESS LOGIC)
// 2. Audit events are persisted to DataStorage (SIDE EFFECT)
// 3. Audit event content is correct and complete (VALIDATION)
//
// CORRECT PATTERN (per AUDIT_INFRASTRUCTURE_TESTING_ANTI_PATTERN_TRIAGE_DEC_26_2025.md):
// - ✅ Create WorkflowExecution CRD (business logic trigger)
// - ✅ Wait for controller to process (business logic execution)
// - ✅ Verify audit event exists in DataStorage (side effect validation)
// - ✅ Validate audit event content (business requirement verification)
//
// REFERENCES:
// - Best Practice: test/integration/signalprocessing/audit_integration_test.go
// - Best Practice: test/integration/gateway/audit_integration_test.go
// - Triage Doc: docs/handoff/AUDIT_INFRASTRUCTURE_TESTING_ANTI_PATTERN_TRIAGE_DEC_26_2025.md
// - DD-API-001: OpenAPI client mandatory for REST API communication

var _ = Describe("WorkflowExecution Audit Flow Integration Tests", Label("audit", "flow"), func() {
	// Data Storage service URL from WE integration infrastructure (DD-TEST-002)
	// Port 18097 per DD-TEST-001 v1.9 (unique port, parallel with HAPI)
	dataStorageURL := fmt.Sprintf("http://localhost:%d", infrastructure.WEIntegrationDataStoragePort)

	var dsClient *dsgen.ClientWithResponses

	BeforeEach(func() {
		// Verify Data Storage is available
		// Per TESTING_GUIDELINES.md: Integration tests MUST use real services
		httpClient := &http.Client{Timeout: 5 * time.Second}
		resp, err := httpClient.Get(dataStorageURL + "/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			Skip(fmt.Sprintf(
				"Data Storage not available at %s - skipping audit flow tests\n"+
					"Start WE infrastructure: make test-integration-workflowexecution",
				dataStorageURL))
		}
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}

		// ✅ DD-API-001: Use OpenAPI generated client (MANDATORY)
		dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
		Expect(err).ToNot(HaveOccurred(), "Failed to create DataStorage OpenAPI client")
	})

	Context("when workflow execution starts (BR-WE-005)", func() {
		It("should emit 'workflow.started' audit event to Data Storage", func() {
			// BUSINESS SCENARIO:
			// When WorkflowExecution controller creates a PipelineRun:
			// 1. Validates workflow configuration
			// 2. Creates Tekton PipelineRun
			// 3. MUST emit audit event for compliance tracking
			//
			// COMPLIANCE: BR-WE-005 - Audit trail for execution lifecycle

			By("1. Creating a test namespace")
			testNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("we-audit-test-%d", time.Now().Unix()),
				},
			}
			Expect(k8sClient.Create(ctx, testNs)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, testNs)
			}()

			By("2. Creating WorkflowExecution CRD (BUSINESS LOGIC TRIGGER)")
			wfeName := fmt.Sprintf("audit-test-wfe-%d", time.Now().Unix())
			targetResource := fmt.Sprintf("%s/deployment/test-app", testNs.Name)
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      wfeName,
					Namespace: testNs.Name,
					Generation: 1, // K8s increments on create/update
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + wfeName,
						Namespace:  testNs.Name,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-workflow",
						Version:        "v1.0.0",
						ContainerImage: "ghcr.io/kubernaut/workflows/test@sha256:abc123",
					},
					TargetResource: targetResource,
				},
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

			// Use WFE name as correlation ID (production pattern)
			correlationID := wfeName

			By("3. Wait for controller to process (BUSINESS LOGIC)")
			Eventually(func() string {
				var updated workflowexecutionv1alpha1.WorkflowExecution
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe.Name,
					Namespace: wfe.Namespace,
				}, &updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).ShouldNot(BeEmpty(),
				"Controller should start processing workflow execution")

			By("4. Query Data Storage for 'workflow.started' audit event (SIDE EFFECT)")
			// ✅ DD-API-001: Use OpenAPI client with type-safe parameters
			eventType := "workflow.started"
			eventCategory := "workflow"
			var auditEvents []dsgen.AuditEvent
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
					EventType:     &eventType,
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
			}, 20*time.Second, 1*time.Second).Should(Equal(1),
				"BR-WE-005: WorkflowExecution MUST emit exactly 1 workflow.started audit event (DD-TESTING-001)")

			By("5. Validate audit event content")
			var startedEvent *dsgen.AuditEvent
			for i := range auditEvents {
				if auditEvents[i].EventType == "workflow.started" {
					startedEvent = &auditEvents[i]
					break
				}
			}
			Expect(startedEvent).ToNot(BeNil(), "Should have 'workflow.started' audit event")

			// Validate key fields
			Expect(startedEvent.EventCategory).To(Equal(dsgen.AuditEventEventCategoryWorkflow))
			Expect(startedEvent.EventAction).To(Equal("started"))
			Expect(startedEvent.CorrelationId).To(Equal(correlationID))
			Expect(startedEvent.ResourceType).ToNot(BeNil())
			Expect(*startedEvent.ResourceType).To(Equal("WorkflowExecution"))

			GinkgoWriter.Printf("✅ workflow.started audit event validated: %s\n", startedEvent.EventId)
		})
	})

	Context("when workflow execution completes (BR-WE-005)", func() {
		It("should track workflow lifecycle through audit events", func() {
			// BUSINESS SCENARIO:
			// When WorkflowExecution progresses through phases:
			// 1. Pending → Running (workflow.started)
			// 2. Running → Completed/Failed (workflow.completed/workflow.failed)
			// 3. Each transition MUST emit audit event
			//
			// COMPLIANCE: BR-WE-005 - Complete audit trail

			By("1. Creating a test namespace")
			testNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("we-audit-lifecycle-%d", time.Now().Unix()),
				},
			}
			Expect(k8sClient.Create(ctx, testNs)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, testNs)
			}()

			By("2. Creating WorkflowExecution CRD")
			wfeName := fmt.Sprintf("audit-lifecycle-wfe-%d", time.Now().Unix())
			targetResource := fmt.Sprintf("%s/deployment/test-app", testNs.Name)
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      wfeName,
					Namespace: testNs.Name,
					Generation: 1, // K8s increments on create/update
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + wfeName,
						Namespace:  testNs.Name,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-workflow",
						Version:        "v1.0.0",
						ContainerImage: "ghcr.io/kubernaut/workflows/test@sha256:abc123",
					},
					TargetResource: targetResource,
				},
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

			correlationID := wfeName

			By("3. Wait for controller to process and emit workflow.started event")
			// DD-TESTING-001: Use Eventually() instead of time.Sleep()
			eventCategory := "workflow"
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
					EventCategory: &eventCategory,
					CorrelationId: &correlationID,
				})
				if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
					return 0
				}
				if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
					return *resp.JSON200.Pagination.Total
				}
				return 0
			}, 20*time.Second, 1*time.Second).Should(BeNumerically(">=", 1),
				"Controller should emit at least workflow.started event")

			By("4. Fetch all workflow audit events for detailed validation")
			// ✅ DD-API-001: Use OpenAPI client
			var auditEvents []dsgen.AuditEvent
			resp, err := dsClient.QueryAuditEventsWithResponse(context.Background(), &dsgen.QueryAuditEventsParams{
				EventCategory: &eventCategory,
				CorrelationId: &correlationID,
			})
			Expect(err).ToNot(HaveOccurred(), "Should successfully query audit events")
			Expect(resp.StatusCode()).To(Equal(http.StatusOK), "Query should return 200")
			Expect(resp.JSON200).ToNot(BeNil(), "Response should have JSON body")
			if resp.JSON200.Data != nil {
				auditEvents = *resp.JSON200.Data
			}
			// DD-TESTING-001: Deterministic validation - we always expect workflow.started
			// May also have workflow.completed/failed if Tekton is available
			Expect(len(auditEvents)).To(BeNumerically(">=", 1),
				"Should have at least workflow.started event")

			By("5. Verify workflow lifecycle events")
			eventTypes := make(map[string]bool)
			for _, event := range auditEvents {
				eventTypes[event.EventType] = true
				GinkgoWriter.Printf("  Found audit event: %s (correlation: %s)\n",
					event.EventType, event.CorrelationId)
			}

			// Should have at minimum workflow.started
			Expect(eventTypes).To(HaveKey("workflow.started"),
				"Expected workflow.started event in lifecycle")

			// May have workflow.completed or workflow.failed depending on Tekton availability
			if eventTypes["workflow.completed"] || eventTypes["workflow.failed"] {
				GinkgoWriter.Println("✅ Complete workflow lifecycle tracked in audit trail")
			} else {
				GinkgoWriter.Println("⚠️  Only workflow.started event found (expected in test env without full Tekton)")
			}
		})
	})
})

