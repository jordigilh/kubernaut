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
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// DS-BUG-001: Duplicate Workflow Returns 500 Instead of 409
// RFC 9110 Section 15.5.10: Duplicate resources must return 409 Conflict, not 500 Internal Server Error
// This test validates the fix for HAPI team's bug report

var _ = Describe("Workflow API Integration - Duplicate Detection (DS-BUG-001)", Ordered, func() {
	Context("DS-BUG-001: Duplicate workflow creation", func() {
		It("should return 409 Conflict when creating duplicate workflow (RFC 9110 compliance)", func() {
			ctx := context.Background()

			// DD-WORKFLOW-017: Register workflow from OCI image (pullspec-only)
			_ = fmt.Sprintf("test-workflow-duplicate-%d", time.Now().UnixNano()) // for logging
			workflow := &ogenclient.CreateWorkflowFromOCIRequest{
				ContainerImage: fmt.Sprintf("%s/duplicate-test:v1.0.0", infrastructure.TestWorkflowBundleRegistry),
			}

			// DD-AUTH-014: Use shared authenticated DSClient with ogenx.ToError() for type-safe error handling
			resp1, err := DSClient.CreateWorkflow(ctx, workflow)
			Expect(err).ToNot(HaveOccurred(), "First workflow creation should not error")

			// Verify first creation succeeds (201 Created returns RemediationWorkflow)
			createdWorkflow, ok := resp1.(*ogenclient.RemediationWorkflow)
			Expect(ok).To(BeTrue(), "Expected RemediationWorkflow for 201 Created")
			Expect(createdWorkflow.WorkflowID.Set).To(BeTrue(), "Created workflow should have ID")
			createdWorkflowName := createdWorkflow.WorkflowName

			// Step 2: Attempt to create the same workflow again (should return 409 Conflict)
			GinkgoWriter.Printf("\n Creating duplicate workflow (expecting 409 Conflict)...\n")
			resp2, err := DSClient.CreateWorkflow(ctx, workflow)

			// DS-BUG-001 FIX VERIFICATION: Use ogenx.ToError() to convert 409 Conflict to error
			err = ogenx.ToError(resp2, err)
			Expect(err).To(HaveOccurred(), "Duplicate workflow creation should return error")

			// Verify 409 Conflict status code
			httpErr := ogenx.GetHTTPError(err)
			Expect(httpErr).ToNot(BeNil(), "Error should be HTTPError")
			Expect(httpErr.StatusCode).To(Equal(409),
				"Duplicate workflow creation should return 409 Conflict (RFC 9110 Section 15.5.10), not 500")

			// Step 3: Verify RFC 7807 problem details extracted by ogenx.ToError()
			Expect(httpErr.Title).To(ContainSubstring("Already Exists"),
				"Problem details 'title' should indicate workflow already exists")

			// Verify error message includes workflow name
			Expect(httpErr.Detail).To(ContainSubstring(createdWorkflowName),
				"Error detail should include workflow name")

			// Type assert the response to access RFC 7807 fields directly
			// CreateWorkflowConflict is an alias for RFC7807Problem
			conflictResp, ok := httpErr.Response.(*ogenclient.CreateWorkflowConflict)
			Expect(ok).To(BeTrue(), "Response should be CreateWorkflowConflict type")
			// Cast to RFC7807Problem to access methods
			rfc7807 := (*ogenclient.RFC7807Problem)(conflictResp)
			Expect(rfc7807.GetStatus()).To(Equal(int32(409)), "RFC 7807 status field should be 409")

			GinkgoWriter.Printf("DS-BUG-001 Fix Verified:\n")
			GinkgoWriter.Printf("   - First creation: 201 Created\n")
			GinkgoWriter.Printf("   - Duplicate attempt: 409 Conflict (RFC 9110 compliant)\n")
			GinkgoWriter.Printf("   - Error format: RFC 7807 problem details\n")
			GinkgoWriter.Printf("   - Error detail: '%s'\n", httpErr.Detail)

			// Step 4: Verify only one workflow exists in database using ListWorkflows API
			// DD-AUTH-014: Use shared authenticated DSClient from suite setup
			listResp, err := DSClient.ListWorkflows(ctx, ogenclient.ListWorkflowsParams{})
			Expect(err).ToNot(HaveOccurred())

			// Type assert the response
			listResult, ok := listResp.(*ogenclient.WorkflowListResponse)
			Expect(ok).To(BeTrue(), "Expected WorkflowListResponse")
			Expect(listResult.Workflows).ToNot(BeNil())

			// Count workflows with our unique name
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

			// DD-WORKFLOW-017: Test with empty image reference (should return 400)
			invalidWorkflow := &ogenclient.CreateWorkflowFromOCIRequest{
				ContainerImage: "",
			}

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
