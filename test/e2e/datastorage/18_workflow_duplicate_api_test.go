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

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/ogenx"
)

// DS-BUG-001: Duplicate Workflow Returns 500 Instead of 409
// RFC 9110 Section 15.5.10: Duplicate resources must return 409 Conflict, not 500 Internal Server Error
// This test validates the fix for HAPI team's bug report

const (
	// Test constants for workflow creation
	testContainerImage  = "test:v1.0.0"
	testContainerDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000001"
)

var _ = Describe("Workflow API Integration - Duplicate Detection (DS-BUG-001)", Ordered, func() {
	Context("DS-BUG-001: Duplicate workflow creation", func() {
		It("should return 409 Conflict when creating duplicate workflow (RFC 9110 compliance)", func() {
			ctx := context.Background()

			// Step 1: Create a unique workflow (should succeed with 201)
			uniqueWorkflowName := fmt.Sprintf("test-workflow-duplicate-%d", time.Now().UnixNano())
			workflow := createTestWorkflowRequest(uniqueWorkflowName, "1.0.0")

			// DD-AUTH-014: Use shared authenticated DSClient with ogenx.ToError() for type-safe error handling
			resp1, err := DSClient.CreateWorkflow(ctx, workflow)
			Expect(err).ToNot(HaveOccurred(), "First workflow creation should not error")

			// Verify first creation succeeds (201 Created returns RemediationWorkflow)
			createdWorkflow, ok := resp1.(*ogenclient.RemediationWorkflow)
			Expect(ok).To(BeTrue(), "Expected RemediationWorkflow for 201 Created")
			Expect(createdWorkflow.WorkflowID.Set).To(BeTrue(), "Created workflow should have ID")

			// Step 2: Attempt to create the same workflow again (should return 409 Conflict)
			GinkgoWriter.Printf("\n🔄 Creating duplicate workflow (expecting 409 Conflict)...\n")
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

			// Verify error message includes workflow name and version
			Expect(httpErr.Detail).To(ContainSubstring(uniqueWorkflowName),
				"Error detail should include workflow name")
			Expect(httpErr.Detail).To(ContainSubstring("1.0.0"),
				"Error detail should include workflow version")

			// Type assert the response to access RFC 7807 fields directly
			// CreateWorkflowConflict is an alias for RFC7807Problem
			conflictResp, ok := httpErr.Response.(*ogenclient.CreateWorkflowConflict)
			Expect(ok).To(BeTrue(), "Response should be CreateWorkflowConflict type")
			// Cast to RFC7807Problem to access methods
			rfc7807 := (*ogenclient.RFC7807Problem)(conflictResp)
			Expect(rfc7807.GetStatus()).To(Equal(int32(409)), "RFC 7807 status field should be 409")

			GinkgoWriter.Printf("✅ DS-BUG-001 Fix Verified:\n")
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
				if wf.WorkflowName == uniqueWorkflowName {
					matchingWorkflows++
				}
			}
			Expect(matchingWorkflows).To(Equal(1),
				"Duplicate creation should not create additional records")
		})

		It("should return 500 for other database errors (not duplicate-related)", func() {
			ctx := context.Background()

			// This test ensures we didn't break general error handling
			// We'll test with an invalid workflow that triggers a different database error

			// Create a workflow with extremely long name (exceeds database column limit)
			invalidWorkflow := createTestWorkflowRequest(
				string(make([]byte, 1000)), // 1000 characters - exceeds typical VARCHAR limits
				"1.0.0",
			)

			resp, err := DSClient.CreateWorkflow(ctx, invalidWorkflow)
			err = ogenx.ToError(resp, err)
			Expect(err).To(HaveOccurred(), "Invalid workflow should return error")

			httpErr := ogenx.GetHTTPError(err)
			Expect(httpErr).ToNot(BeNil(), "Error should be HTTPError")

			// Should return 500 for non-duplicate database errors
			// (or 400 if validation catches it first, which is also acceptable)
			Expect(httpErr.StatusCode).To(Or(
				Equal(400),
				Equal(500),
			), "Non-duplicate database errors should return 400 or 500, not 409")

			// Verify it's NOT a 409 Conflict
			Expect(httpErr.StatusCode).ToNot(Equal(409),
				"Non-duplicate errors should not return 409 Conflict")
		})
	})
})

// Helper function to create a test workflow request
func createTestWorkflowRequest(workflowName, version string) *ogenclient.RemediationWorkflow {
	name := "Test Duplicate Workflow"
	description := "Test workflow for duplicate detection"
	content := "apiVersion: kubernaut.io/v1alpha1\nkind: WorkflowSchema\nmetadata:\n  name: test"
	contentHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	executionEngine := string(models.ExecutionEngineTekton)
	status := ogenclient.RemediationWorkflowStatusActive
	containerImage := testContainerImage
	containerDigest := testContainerDigest

	// Create mandatory labels (per ADR-033)
	labels := ogenclient.MandatoryLabels{
		Component:   "pod",
		Environment: []ogenclient.MandatoryLabelsEnvironmentItem{ogenclient.MandatoryLabelsEnvironmentItem("test")},
		Priority:    "P2",
		Severity:    "medium",
		SignalType:  "OOMKilled",
	}

	return &ogenclient.RemediationWorkflow{
		WorkflowName:    workflowName,
		Version:         version,
		Name:            name,
		Description:     description,
		Content:         content,
		ContentHash:     contentHash,
		Labels:          labels,
		CustomLabels:    ogenclient.OptCustomLabels{},
		DetectedLabels:  ogenclient.OptDetectedLabels{},
		ExecutionEngine: executionEngine,
		Status:          status,
		ContainerImage:  ogenclient.NewOptString(containerImage),
		ContainerDigest: ogenclient.NewOptString(containerDigest),
	}
}

// NOTE: Previously used createWorkflowHTTP() helper for raw HTTP calls
// Now using DSClient.CreateWorkflow() with ogenx.ToError() for type-safe error handling (DD-API-001)
