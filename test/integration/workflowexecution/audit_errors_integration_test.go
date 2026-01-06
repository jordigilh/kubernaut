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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// =============================================================================
// BR-AUDIT-005 Gap #7: Workflow Execution Error Details Standardization
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
// - Integration tier: Requires envtest + real Tekton for error scenarios
// - OpenAPI client MANDATORY for all audit queries (DD-API-001)
// - Eventually() MANDATORY for async operations (NO time.Sleep())
//
// Error Scenarios Tested:
// - Scenario 1: Tekton pipeline failure (ERR_PIPELINE_FAILED)
// - Scenario 2: Workflow not found (ERR_WORKFLOW_NOT_FOUND)
//
// To run these tests:
//   make test-integration-workflowexecution
//
// =============================================================================

var _ = Describe("BR-AUDIT-005 Gap #7: WorkflowExecution Error Audit Standardization", func() {
	Context("Gap #7 Scenario 1: Tekton Pipeline Failure", func() {
		It("should emit standardized error_details on pipeline failure", func() {
			// Given: WorkflowExecution CRD with workflow that will fail
			testID := fmt.Sprintf("pipeline-fail-%d", time.Now().Unix())
			wfe := createUniqueWFE(testID, "test-pod-1")

			// Override workflow ref to point to a failing workflow
			// TODO: Create a test workflow that always fails
			wfe.Spec.WorkflowRef = workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID:     "test-failing-workflow",
				Version:        "v1.0.0",
				ContainerImage: "ghcr.io/kubernaut/workflows/failing-test@sha256:abc123",
			}

			correlationID := wfe.Spec.RemediationRequestRef.Name

			// When: Create WorkflowExecution CRD (controller will create PipelineRun that fails)
			err := k8sClient.Create(ctx, wfe)
			Expect(err).ToNot(HaveOccurred())

			// Cleanup
			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

			Fail("IMPLEMENTATION REQUIRED: Need mechanism to trigger Tekton pipeline failure\n" +
				"  Per TESTING_GUIDELINES.md: Tests MUST fail to show missing infrastructure\n" +
				"  Next step: Create test workflow container that always fails execution")

			// Then: Should emit workflow.failed with error_details (enhanced from existing event)
			eventType := "workflow.failed"

			// Wait for error event
			Eventually(func() int {
				resp, _ := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
					EventType:     &eventType,
					CorrelationId: &correlationID,
				})
				if resp.JSON200 == nil {
					return 0
				}
				return *resp.JSON200.Pagination.Total
			}, 120*time.Second, 2*time.Second).Should(Equal(1),
				"Should find exactly 1 workflow.failed event")

			// Validate Gap #7: error_details (WILL FAIL - not standardized yet)
			// resp, _ := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
			// 	EventType:     &eventType,
			// 	CorrelationId: &correlationID,
			// })
			// events := *resp.JSON200.Data
			// Expect(len(events)).To(Equal(1))
			//
			// eventData := events[0].EventData.(map[string]interface{})
			// Expect(eventData).To(HaveKey("error_details"))
			//
			// errorDetails := eventData["error_details"].(map[string]interface{})
			// Expect(errorDetails).To(HaveKey("message"))
			// Expect(errorDetails["message"]).To(ContainSubstring("pipeline"))
			// Expect(errorDetails).To(HaveKey("code"))
			// Expect(errorDetails["code"]).To(Equal("ERR_PIPELINE_FAILED"))
			// Expect(errorDetails).To(HaveKey("component"))
			// Expect(errorDetails["component"]).To(Equal("workflowexecution"))
			// Expect(errorDetails).To(HaveKey("retry_possible"))
			// // Pipeline failures may be retryable depending on the error
		})
	})

	Context("Gap #7 Scenario 2: Workflow Not Found", func() {
		It("should emit standardized error_details on workflow not found", func() {
			// Given: WorkflowExecution CRD with non-existent workflow reference
			testID := fmt.Sprintf("wf-notfound-%d", time.Now().Unix())
			wfe := createUniqueWFE(testID, "test-pod-2")

			// Set workflow ref to non-existent workflow
			wfe.Spec.WorkflowRef = workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID:     "non-existent-workflow-12345",
				Version:        "v99.99.99",
				ContainerImage: "ghcr.io/kubernaut/workflows/nonexistent@sha256:invalid",
			}

			correlationID := wfe.Spec.RemediationRequestRef.Name

			// When: Create WorkflowExecution CRD (controller will fail to find workflow)
			err := k8sClient.Create(ctx, wfe)
			Expect(err).ToNot(HaveOccurred())

			// Cleanup
			defer func() {
				_ = k8sClient.Delete(ctx, wfe)
			}()

			Fail("IMPLEMENTATION REQUIRED: Need to determine workflow validation behavior\n" +
				"  Per TESTING_GUIDELINES.md: Tests MUST fail to show missing functionality\n" +
				"  Next step: Verify controller emits failure audit when workflow not found")

			// Then: Should emit workflow.failed with error_details
			eventType := "workflow.failed"

			// Wait for error event
			Eventually(func() int {
				resp, _ := dsClient.QueryAuditEventsWithResponse(ctx, &dsgen.QueryAuditEventsParams{
					EventType:     &eventType,
					CorrelationId: &correlationID,
				})
				if resp.JSON200 == nil {
					return 0
				}
				return *resp.JSON200.Pagination.Total
			}, 60*time.Second, 2*time.Second).Should(Equal(1),
				"Should find exactly 1 workflow.failed event for workflow not found")

			// Validate Gap #7: error_details (WILL FAIL - not standardized yet)
			// errorDetails := eventData["error_details"].(map[string]interface{})
			// Expect(errorDetails["message"]).To(ContainSubstring("not found"))
			// Expect(errorDetails["code"]).To(Equal("ERR_WORKFLOW_NOT_FOUND"))
			// Expect(errorDetails["component"]).To(Equal("workflowexecution"))
			// Expect(errorDetails["retry_possible"]).To(BeFalse()) // Workflow not found is permanent
		})
	})
})

