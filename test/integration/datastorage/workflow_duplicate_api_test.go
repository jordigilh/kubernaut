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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dsgen ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// DS-BUG-001: Duplicate Workflow Returns 500 Instead of 409
// RFC 9110 Section 15.5.10: Duplicate resources must return 409 Conflict, not 500 Internal Server Error
// This test validates the fix for HAPI team's bug report

const (
	// Test constants for workflow creation
	testContainerImage  = "test:v1.0.0"
	testContainerDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000001"
)

var _ = Describe("Workflow API Integration - Duplicate Detection (DS-BUG-001)",  Ordered, func() {
	var (
		httpClient *http.Client
	)

	BeforeAll(func() {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	})

	Context("DS-BUG-001: Duplicate workflow creation", func() {
		It("should return 409 Conflict when creating duplicate workflow (RFC 9110 compliance)", func() {
			ctx := context.Background()

			// Step 1: Create a unique workflow (should succeed with 201)
			uniqueWorkflowName := fmt.Sprintf("test-workflow-duplicate-%d", time.Now().UnixNano())
			workflow := createTestWorkflowRequest(uniqueWorkflowName, "1.0.0")

			resp1, err := createWorkflowHTTP(httpClient, datastorageURL, workflow)
			Expect(err).ToNot(HaveOccurred(), "First workflow creation should not error")
			defer func() { _ = resp1.Body.Close() }()

			// Verify first creation succeeds
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated),
				"First workflow creation should return 201 Created")

			var createdWorkflow dsgen.RemediationWorkflow
			err = json.NewDecoder(resp1.Body).Decode(&createdWorkflow)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")
			Expect(createdWorkflow.WorkflowId).ToNot(BeNil(), "Created workflow should have ID")

			// Step 2: Attempt to create the same workflow again (should return 409 Conflict)
			GinkgoWriter.Printf("\n🔄 Creating duplicate workflow (expecting 409 Conflict)...\n")
			resp2, err := createWorkflowHTTP(httpClient, datastorageURL, workflow)
			Expect(err).ToNot(HaveOccurred(), "Second workflow creation should not error at HTTP level")
			defer func() { _ = resp2.Body.Close() }()

			// DS-BUG-001 FIX VERIFICATION: Should return 409, not 500
			Expect(resp2.StatusCode).To(Equal(http.StatusConflict),
				"Duplicate workflow creation should return 409 Conflict (RFC 9110 Section 15.5.10), not 500")

			// Step 3: Verify RFC 7807 problem details format
			Expect(resp2.Header.Get("Content-Type")).To(ContainSubstring("application/problem+json"),
				"Error response should use RFC 7807 problem details format")

			var problemDetails map[string]interface{}
			err = json.NewDecoder(resp2.Body).Decode(&problemDetails)
			Expect(err).ToNot(HaveOccurred(), "Problem details should be valid JSON")

			// Verify RFC 7807 fields
			Expect(problemDetails["type"]).To(ContainSubstring("conflict"),
				"Problem details 'type' should indicate conflict")
			Expect(problemDetails["title"]).To(ContainSubstring("Already Exists"),
				"Problem details 'title' should indicate workflow already exists")
			Expect(problemDetails["status"]).To(Equal(float64(409)),
				"Problem details 'status' should be 409")

			// Verify error message includes workflow name and version
			detail, ok := problemDetails["detail"].(string)
			Expect(ok).To(BeTrue(), "Problem details 'detail' should be a string")
			Expect(detail).To(ContainSubstring(uniqueWorkflowName),
				"Error detail should include workflow name")
			Expect(detail).To(ContainSubstring("1.0.0"),
				"Error detail should include workflow version")

			GinkgoWriter.Printf("✅ DS-BUG-001 Fix Verified:\n")
			GinkgoWriter.Printf("   - First creation: 201 Created\n")
			GinkgoWriter.Printf("   - Duplicate attempt: 409 Conflict (RFC 9110 compliant)\n")
			GinkgoWriter.Printf("   - Error format: RFC 7807 problem details\n")
			GinkgoWriter.Printf("   - Error detail: '%s'\n", detail)

			// Step 4: Verify only one workflow exists in database using ListWorkflows API
			listClient, err := dsgen.NewClientWithResponses(datastorageURL)
			Expect(err).ToNot(HaveOccurred())

			listResp, err := listClient.ListWorkflowsWithResponse(ctx, &dsgen.ListWorkflowsParams{})
			Expect(err).ToNot(HaveOccurred())
			Expect(listResp.StatusCode()).To(Equal(200))
			Expect(listResp.JSON200).ToNot(BeNil())
			Expect(listResp.JSON200.Workflows).ToNot(BeNil())

			// Count workflows with our unique name
			matchingWorkflows := 0
			for _, wf := range *listResp.JSON200.Workflows {
				if wf.WorkflowName == uniqueWorkflowName {
					matchingWorkflows++
				}
			}
			Expect(matchingWorkflows).To(Equal(1),
				"Duplicate creation should not create additional records")
		})

		It("should return 500 for other database errors (not duplicate-related)", func() {
			// This test ensures we didn't break general error handling
			// We'll test with an invalid workflow that triggers a different database error

			// Create a workflow with extremely long name (exceeds database column limit)
			invalidWorkflow := createTestWorkflowRequest(
				string(make([]byte, 1000)), // 1000 characters - exceeds typical VARCHAR limits
				"1.0.0",
			)

			resp, err := createWorkflowHTTP(httpClient, datastorageURL, invalidWorkflow)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Should return 500 for non-duplicate database errors
			// (or 400 if validation catches it first, which is also acceptable)
			Expect(resp.StatusCode).To(Or(
				Equal(http.StatusBadRequest),
				Equal(http.StatusInternalServerError),
			), "Non-duplicate database errors should return 400 or 500, not 409")

			// If 500, verify it's the generic internal error, not conflict
			if resp.StatusCode == http.StatusInternalServerError {
				var problemDetails map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&problemDetails)
				Expect(err).ToNot(HaveOccurred())
				Expect(problemDetails["type"]).To(ContainSubstring("internal-error"),
					"Non-duplicate errors should use 'internal-error' type")
			}
		})
	})
})

// Helper function to create a test workflow request
func createTestWorkflowRequest(workflowName, version string) *dsgen.RemediationWorkflow {
	name := "Test Duplicate Workflow"
	description := "Test workflow for duplicate detection"
	content := "apiVersion: kubernaut.io/v1alpha1\nkind: WorkflowSchema\nmetadata:\n  name: test"
	contentHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	executionEngine := string(models.ExecutionEngineTekton)
	status := dsgen.RemediationWorkflowStatusActive
	containerImage := testContainerImage
	containerDigest := testContainerDigest

	// Create mandatory labels (per ADR-033)
	labels := dsgen.MandatoryLabels{
		Component:   "pod",
		Environment: "test",
		Priority:    "P2",
		Severity:    "medium",
		SignalType:  "OOMKilled",
	}

	return &dsgen.RemediationWorkflow{
		WorkflowName:    workflowName,
		Version:         version,
		Name:            name,
		Description:     description,
		Content:         content,
		ContentHash:     contentHash,
		Labels:          labels,
		CustomLabels:    &dsgen.CustomLabels{},
		DetectedLabels:  &dsgen.DetectedLabels{},
		ExecutionEngine: executionEngine,
		Status:          status,
		ContainerImage:  &containerImage,
		ContainerDigest: &containerDigest,
	}
}

// Helper function to create a workflow via HTTP
func createWorkflowHTTP(client *http.Client, baseURL string, workflow *dsgen.RemediationWorkflow) (*http.Response, error) {
	body, err := json.Marshal(workflow)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal workflow: %w", err)
	}

	req, err := http.NewRequest("POST", baseURL+"/api/v1/workflows", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	return client.Do(req)
}
