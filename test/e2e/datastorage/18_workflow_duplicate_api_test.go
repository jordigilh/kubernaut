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
		//   active + same hash       → 200 OK (idempotent, no DB writes)
		//   active + diff hash       → 409 Conflict (must bump version — Issue #773)
		//   cross-version (any hash) → supersede old → create new → 201 Created
		// All are meaningful responses, not 500.

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

		// Issue #773: Same (name, version) + different content → 409 Conflict.
		// Version-locked content immutability: must bump the version to register new content.
		It("should reject same-version content change with 409 Conflict (Issue #773)", func() {
			ctx := context.Background()

			testID := fmt.Sprintf("rej-%d", time.Now().UnixNano())
			uniqueName := fmt.Sprintf("e2e-rej-%s", testID)
			content1 := generateWorkflowContent(uniqueName, "1.0.0")

			workflow := &ogenclient.CreateWorkflowInlineRequest{Content: content1}
			workflow.Source.SetTo("e2e-test")
			resp1, err := DSClient.CreateWorkflow(ctx, workflow)
			Expect(err).ToNot(HaveOccurred())

			switch resp1.(type) {
			case *ogenclient.CreateWorkflowCreated, *ogenclient.CreateWorkflowOK:
			default:
				Fail(fmt.Sprintf("Expected CreateWorkflowCreated or CreateWorkflowOK, got: %T", resp1))
			}

			supersedeCRD := testutil.NewTestWorkflowCRD(uniqueName, "ScaleReplicas", "tekton")
			supersedeCRD.Spec.Description.What = "Different content, same version — should be rejected"
			supersedeCRD.Spec.Description.WhenToUse = "Issue #773: content-integrity-violation E2E test"
			supersedeCRD.Spec.Labels.Priority = "P0"
			supersedeCRD.Spec.Execution.Bundle = e2eBundleRef
			supersedeCRD.Spec.Parameters = []models.WorkflowParameter{
				{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target resource for remediation"},
			}
			content2 := testutil.MarshalWorkflowCRD(supersedeCRD)

			GinkgoWriter.Printf("\n Creating workflow with different content, same version (expecting 409 Conflict)...\n")
			reject := &ogenclient.CreateWorkflowInlineRequest{Content: content2}
			reject.Source.SetTo("e2e-test")
			resp2, err := DSClient.CreateWorkflow(ctx, reject)
			err = ogenx.ToError(resp2, err)
			Expect(err).To(HaveOccurred(), "Same version + different content should return error")

			httpErr := ogenx.GetHTTPError(err)
			Expect(httpErr).ToNot(BeNil(), "Error should be HTTPError")
			Expect(httpErr.StatusCode).To(Equal(409),
				"Same version + different content should return 409 Conflict")

			GinkgoWriter.Printf("Issue #773 Content Integrity Verified:\n")
			GinkgoWriter.Printf("   - Same version + different content → 409 Conflict\n")
		})

		// Issue #371: Cross-version supersede still works — version bump creates new workflow
		// and marks old as superseded.
		It("should supersede active workflow on cross-version update (BR-WORKFLOW-006)", func() {
			ctx := context.Background()

			testID := fmt.Sprintf("sup-%d", time.Now().UnixNano())
			uniqueName := fmt.Sprintf("e2e-sup-%s", testID)
			content1 := generateWorkflowContent(uniqueName, "1.0.0")

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

			content2 := generateWorkflowContent(uniqueName, "2.0.0")

			GinkgoWriter.Printf("\n Creating workflow with version bump (expecting 201 Created - supersede)...\n")
			supersede := &ogenclient.CreateWorkflowInlineRequest{Content: content2}
			supersede.Source.SetTo("e2e-test")
			resp2, err := DSClient.CreateWorkflow(ctx, supersede)
			Expect(err).ToNot(HaveOccurred(), "Cross-version supersede should not return error")

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
				"Cross-version supersede should create a new workflow with a different ID")

			GinkgoWriter.Printf("BR-WORKFLOW-006 Cross-Version Supersede Verified:\n")
			GinkgoWriter.Printf("   - v1.0.0: %s\n", firstWorkflowID)
			GinkgoWriter.Printf("   - v2.0.0: %s (new ID)\n", supersededWorkflow.WorkflowId.Value.String())
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
