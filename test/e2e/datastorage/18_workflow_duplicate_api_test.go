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
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/ogenx"
)

// DS-BUG-001: Duplicate Workflow Returns 500 Instead of 409
// RFC 9110 Section 15.5.10: Duplicate resources must return 409 Conflict, not 500 Internal Server Error
// This test validates the fix for HAPI team's bug report

var _ = Describe("Workflow API Integration - Duplicate Detection (DS-BUG-001)", Ordered, func() {
	Context("DS-BUG-001: Duplicate workflow creation", func() {
		It("should return 409 Conflict when creating duplicate workflow (RFC 9110 compliance)", func() {
			ctx := context.Background()

			// Step 1: Create the initial workflow
			testID := fmt.Sprintf("dup-%d", time.Now().UnixNano())
			uniqueName := fmt.Sprintf("e2e-dup-%s", testID)
			content1 := generateWorkflowContent(uniqueName, "1.0.0")
			workflow := &ogenclient.CreateWorkflowInlineRequest{Content: content1}
			workflow.Source.SetTo("e2e-test")

			resp1, err := DSClient.CreateWorkflow(ctx, workflow)
			Expect(err).ToNot(HaveOccurred(), "First workflow creation should not error")

			var createdWorkflow *ogenclient.RemediationWorkflow
			switch v := resp1.(type) {
			case *ogenclient.CreateWorkflowCreated:
				createdWorkflow = (*ogenclient.RemediationWorkflow)(v)
			case *ogenclient.CreateWorkflowOK:
				createdWorkflow = (*ogenclient.RemediationWorkflow)(v)
			default:
				Fail(fmt.Sprintf("Expected CreateWorkflowCreated or CreateWorkflowOK, got: %T", resp1))
			}
			Expect(createdWorkflow.WorkflowId.Set).To(BeTrue(), "Created workflow should have ID")
			createdWorkflowName := createdWorkflow.WorkflowName

			// Step 2: Create a DIFFERENT workflow with same name+version but altered content
			// (different description produces a different content hash → triggers 409 Conflict)
			content2 := fmt.Sprintf(`apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: %[1]s
spec:
  metadata:
    workflowName: %[1]s
    version: "1.0.0"
    description:
      what: "DUPLICATE: altered content to trigger 409 Conflict"
      whenToUse: "DS-BUG-001 duplicate detection E2E test"
  actionType: ScaleReplicas
  labels:
    severity: [critical]
    environment: [production]
    component: pod
    priority: P0
  execution:
    engine: tekton
    bundle: quay.io/kubernaut-cicd/test-workflows/placeholder-execution:v1.0.0@sha256:adfc09ea45a5b627550c6a73fe75d50efe1c80fa43359fcc4908c9c5b0639ac3
  parameters:
    - name: TARGET_RESOURCE
      type: string
      required: true
      description: "Target resource for remediation"
`, uniqueName)

			GinkgoWriter.Printf("\n Creating duplicate workflow (expecting 409 Conflict)...\n")
			dupWorkflow := &ogenclient.CreateWorkflowInlineRequest{Content: content2}
			dupWorkflow.Source.SetTo("e2e-test")
			resp2, err := DSClient.CreateWorkflow(ctx, dupWorkflow)

			// DS-BUG-001 FIX VERIFICATION: Use ogenx.ToError() to convert 409 Conflict to error
			err = ogenx.ToError(resp2, err)
			Expect(err).To(HaveOccurred(), "Duplicate workflow creation should return error")

			httpErr := ogenx.GetHTTPError(err)
			Expect(httpErr).ToNot(BeNil(), "Error should be HTTPError")
			Expect(httpErr.StatusCode).To(Equal(409),
				"Duplicate workflow creation should return 409 Conflict (RFC 9110 Section 15.5.10), not 500")

			Expect(httpErr.Title).To(ContainSubstring("Already Exists"),
				"Problem details 'title' should indicate workflow already exists")

			Expect(httpErr.Detail).To(ContainSubstring(createdWorkflowName),
				"Error detail should include workflow name")

			conflictResp, ok := httpErr.Response.(*ogenclient.CreateWorkflowConflict)
			Expect(ok).To(BeTrue(), "Response should be CreateWorkflowConflict type")
			rfc7807 := (*ogenclient.RFC7807Problem)(conflictResp)
			Expect(rfc7807.GetStatus()).To(Equal(int32(409)), "RFC 7807 status field should be 409")

			GinkgoWriter.Printf("DS-BUG-001 Fix Verified:\n")
			GinkgoWriter.Printf("   - First creation: 201 Created\n")
			GinkgoWriter.Printf("   - Duplicate attempt: 409 Conflict (RFC 9110 compliant)\n")
			GinkgoWriter.Printf("   - Error format: RFC 7807 problem details\n")
			GinkgoWriter.Printf("   - Error detail: '%s'\n", httpErr.Detail)

			// Step 3: Verify only one workflow exists with this name
			listResp, err := DSClient.ListWorkflows(ctx, ogenclient.ListWorkflowsParams{})
			Expect(err).ToNot(HaveOccurred())

			listResult, ok := listResp.(*ogenclient.WorkflowListResponse)
			Expect(ok).To(BeTrue(), "Expected WorkflowListResponse")
			Expect(len(listResult.Workflows)).To(BeNumerically(">=", 1),
				"Workflow list should contain at least the duplicate-tested workflow")

			matchingWorkflows := 0
			for _, wf := range listResult.Workflows {
				if wf.WorkflowName == createdWorkflowName {
					matchingWorkflows++
				}
			}
			Expect(matchingWorkflows).To(Equal(1),
				"Duplicate creation should not create additional records")
		})

		It("should return error for invalid OCI image reference", func() {
			ctx := context.Background()

			// DD-WORKFLOW-017: Test with empty content (should return 400)
			invalidWorkflow := &ogenclient.CreateWorkflowInlineRequest{
				Content: "",
			}
			invalidWorkflow.Source.SetTo("e2e-test")

			resp, err := DSClient.CreateWorkflow(ctx, invalidWorkflow)
			err = ogenx.ToError(resp, err)
			Expect(err).To(HaveOccurred(), "Invalid workflow should return error")

			httpErr := ogenx.GetHTTPError(err)
			Expect(httpErr).ToNot(BeNil(), "Error should be HTTPError")

			// Should return 400 for empty image reference
			Expect(httpErr.StatusCode).To(Equal(400),
				"Empty image reference should return 400 Bad Request")

			// Verify it's NOT a 409 Conflict
			Expect(httpErr.StatusCode).ToNot(Equal(409),
				"Non-duplicate errors should not return 409 Conflict")
		})
	})
})
