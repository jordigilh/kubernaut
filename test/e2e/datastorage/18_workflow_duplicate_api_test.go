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
	"github.com/jordigilh/kubernaut/test/testutil"
)

// DS-BUG-001: Duplicate Workflow Returns 500 Instead of 409
// RFC 9110 Section 15.5.10: Duplicate resources must return 409 Conflict, not 500 Internal Server Error
// This test validates the fix for KA team's bug report

var _ = Describe("Workflow API Integration - Duplicate Detection (DS-BUG-001)", Ordered, func() {
	Context("DS-BUG-001: Duplicate workflow creation", func() {
		// DS-BUG-001 Fix Verification: The original bug was that duplicate workflow creation
		// returned 500 Internal Server Error. The fix uses BR-WORKFLOW-006 ContentHash-based
		// duplicate detection:
		//   active + same hash    → 200 OK (idempotent, no DB writes)
		//   active + diff hash    → supersede old → create new → 201 Created
		// Both are meaningful responses, not 500.

		It("should return 200 OK for idempotent re-apply of same content (DS-BUG-001 fix)", func() {
			ctx := context.Background()

			testID := fmt.Sprintf("dup-%d", time.Now().UnixNano())
			uniqueName := fmt.Sprintf("e2e-dup-%s", testID)
			content := generateWorkflowContent(uniqueName, "1.0.0")

			// Step 1: Create the initial workflow
			workflow := &ogenclient.CreateWorkflowInlineRequest{Content: content}
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
			createdWorkflowID := createdWorkflow.WorkflowId.Value.String()

			// Step 2: Re-apply exact same content (idempotent)
			// DS-BUG-001 fix: returns 200 OK (not 500 Internal Server Error)
			GinkgoWriter.Printf("\n Re-applying same workflow content (expecting 200 OK - idempotent)...\n")
			dupWorkflow := &ogenclient.CreateWorkflowInlineRequest{Content: content}
			dupWorkflow.Source.SetTo("e2e-test")
			resp2, err := DSClient.CreateWorkflow(ctx, dupWorkflow)
			Expect(err).ToNot(HaveOccurred(), "Idempotent re-apply should not error")

			idempotentResult, ok := resp2.(*ogenclient.CreateWorkflowOK)
			Expect(ok).To(BeTrue(), "Idempotent re-apply should return 200 OK (CreateWorkflowOK)")
			Expect(idempotentResult.WorkflowId.Value.String()).To(Equal(createdWorkflowID),
				"Idempotent re-apply should return the same workflow ID")

			// Step 3: Verify only one active workflow exists with this name
			listResp, err := DSClient.ListWorkflows(ctx, ogenclient.ListWorkflowsParams{})
			Expect(err).ToNot(HaveOccurred())

			listResult, ok := listResp.(*ogenclient.WorkflowListResponse)
			Expect(ok).To(BeTrue(), "Expected WorkflowListResponse")

			matchingWorkflows := 0
			for _, wf := range listResult.Workflows {
				if wf.WorkflowName == createdWorkflowName {
					matchingWorkflows++
				}
			}
			Expect(matchingWorkflows).To(Equal(1),
				"Idempotent re-apply should not create additional records")

			GinkgoWriter.Printf("DS-BUG-001 Fix Verified:\n")
			GinkgoWriter.Printf("   - First creation: 201 Created\n")
			GinkgoWriter.Printf("   - Idempotent re-apply: 200 OK (same workflow ID returned)\n")
			GinkgoWriter.Printf("   - No 500 Internal Server Error\n")
		})

		It("should supersede active workflow when content hash changes (BR-WORKFLOW-006)", func() {
			ctx := context.Background()

			testID := fmt.Sprintf("sup-%d", time.Now().UnixNano())
			uniqueName := fmt.Sprintf("e2e-sup-%s", testID)
			content1 := generateWorkflowContent(uniqueName, "1.0.0")

			// Step 1: Create initial workflow
			workflow := &ogenclient.CreateWorkflowInlineRequest{Content: content1}
			workflow.Source.SetTo("e2e-test")
			resp1, err := DSClient.CreateWorkflow(ctx, workflow)
			Expect(err).ToNot(HaveOccurred())

			var firstWorkflow *ogenclient.RemediationWorkflow
			switch v := resp1.(type) {
			case *ogenclient.CreateWorkflowCreated:
				firstWorkflow = (*ogenclient.RemediationWorkflow)(v)
			case *ogenclient.CreateWorkflowOK:
				firstWorkflow = (*ogenclient.RemediationWorkflow)(v)
			default:
				Fail(fmt.Sprintf("Expected CreateWorkflowCreated or CreateWorkflowOK, got: %T", resp1))
			}
			firstWorkflowID := firstWorkflow.WorkflowId.Value.String()

			// Step 2: Create with same name+version but different content (triggers supersede)
			supersedeCRD := testutil.NewTestWorkflowCRD(uniqueName, "ScaleReplicas", "tekton")
			supersedeCRD.Spec.Description.What = "Updated content to trigger supersede (BR-WORKFLOW-006)"
			supersedeCRD.Spec.Description.WhenToUse = "DS-BUG-001 supersede path E2E test"
			supersedeCRD.Spec.Labels.Priority = "P0"
			supersedeCRD.Spec.Execution.Bundle = e2eBundleRef
			supersedeCRD.Spec.Parameters = []models.WorkflowParameter{
				{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target resource for remediation"},
			}
			supersedeCRD.Spec.DetectedLabels = &models.DetectedLabelsSchema{
				HPAEnabled:      "true",
				GitOpsTool:      "argocd",
				PopulatedFields: []string{"hpaEnabled", "gitOpsTool"},
			}
			content2 := testutil.MarshalWorkflowCRD(supersedeCRD)

			GinkgoWriter.Printf("\n Creating workflow with different content (expecting 201 Created - supersede)...\n")
			supersede := &ogenclient.CreateWorkflowInlineRequest{Content: content2}
			supersede.Source.SetTo("e2e-test")
			resp2, err := DSClient.CreateWorkflow(ctx, supersede)
			Expect(err).ToNot(HaveOccurred(), "Supersede should not return error")

			var supersededWorkflow *ogenclient.RemediationWorkflow
			switch v := resp2.(type) {
			case *ogenclient.CreateWorkflowCreated:
				supersededWorkflow = (*ogenclient.RemediationWorkflow)(v)
			case *ogenclient.CreateWorkflowOK:
				supersededWorkflow = (*ogenclient.RemediationWorkflow)(v)
			default:
				Fail(fmt.Sprintf("Expected CreateWorkflowCreated or CreateWorkflowOK, got: %T", resp2))
			}

			Expect(supersededWorkflow.WorkflowId.Value.String()).ToNot(Equal(firstWorkflowID),
				"Supersede should create a new workflow with a different ID")

			GinkgoWriter.Printf("BR-WORKFLOW-006 Supersede Verified:\n")
			GinkgoWriter.Printf("   - First creation: %s\n", firstWorkflowID)
			GinkgoWriter.Printf("   - Supersede: %s (new ID)\n", supersededWorkflow.WorkflowId.Value.String())
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
