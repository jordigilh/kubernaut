/*
Copyright 2026 Jordi Gil.

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

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// E2E-DS-481: ServiceAccountName REST API Persistence (#481)
// ========================================
//
// Authority: DD-WE-005 v2.0 (Per-Workflow ServiceAccount Reference)
// Business Requirement: BR-WE-007 (Service account configuration)
// Test Plan: docs/tests/481/TEST_PLAN.md
//
// These tests validate the full REST API roundtrip:
//   POST workflow with serviceAccountName -> GET/Discovery -> assert SA in response
//
// They exercise the complete chain: YAML parsing -> handler -> repository INSERT ->
// repository SELECT -> HTTP response serialization, catching bugs like the
// INSERT query omitting service_account_name.

var _ = Describe("E2E-DS-481: ServiceAccountName REST API Persistence (#481)", Label("e2e", "datastorage", "sa-persistence"), func() {
	var (
		testCtx    context.Context
		testCancel context.CancelFunc
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		DeferCleanup(testCancel)
	})

	Context("Create and retrieve workflow with ServiceAccountName", func() {

		// E2E-DS-481-001 ("persist serviceAccountName through POST -> GET
		// roundtrip") removed: #1661 Phase 55b — DS's createWorkflow REST
		// endpoint was retired (DD-WORKFLOW-018, AuthWebhook is the sole write
		// path). The POST leg of this roundtrip no longer exists; the GET leg
		// (serviceAccountName survives CRD -> workflowcache -> REST response)
		// is still exercised by E2E-DS-481-002 below via discovery.

		It("E2E-DS-481-002: should return serviceAccountName in discovery endpoint", func() {
			suffix := fmt.Sprintf("%d", time.Now().UnixNano())
			workflowName := fmt.Sprintf("e2e-sa-discovery-%s", suffix)

			By("Building a workflow CRD with serviceAccountName set")
			crd := testutil.NewTestWorkflowCRD(workflowName, "RestartPod", "job")
			crd.Spec.Description.What = "E2E SA discovery test"
			crd.Spec.Description.WhenToUse = "E2E-DS-481-002: validates serviceAccountName in discovery endpoint"
			crd.Spec.Labels.Priority = "P0"
			crd.Spec.Labels.Component = []string{"v1/Pod"}
			crd.Spec.Execution.Bundle = e2eBundleRef
			crd.Spec.Execution.ServiceAccountName = "e2e-discovery-sa"
			crd.Spec.Parameters = []models.WorkflowParameter{
				{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target resource"},
			}
			content := testutil.MarshalWorkflowCRD(crd)

			// #1661 Phase 55b: register workflow via direct CRD creation (no
			// live AuthWebhook in this suite; DS's REST registration endpoint
			// was removed per DD-WORKFLOW-018).
			By("Registering workflow via direct CRD creation")
			createdID, _ := ensureWorkflowRegistered(testCtx, DSClient, content, workflowName)
			Expect(createdID).To(HaveLen(36), "UUID should be 36 chars (8-4-4-4-12)")
			logger.Info("Workflow created for discovery test", "id", createdID, "name", workflowName)

			By("Querying discovery endpoint for RestartPod workflows")
			discResp, err := DSClient.ListWorkflowsByActionType(testCtx, dsgen.ListWorkflowsByActionTypeParams{
				ActionType:  "RestartPod",
				Severity:    dsgen.ListWorkflowsByActionTypeSeverityCritical,
				Component:   "v1/Pod",
				Environment: "production",
				Priority:    dsgen.ListWorkflowsByActionTypePriorityP0,
				Limit:       dsgen.NewOptInt(100),
			})
			Expect(err).ToNot(HaveOccurred())

			discoveryResp, ok := discResp.(*dsgen.WorkflowDiscoveryResponse)
			Expect(ok).To(BeTrue(), "Expected *WorkflowDiscoveryResponse")
			Expect(discoveryResp.Workflows).ToNot(BeEmpty(), "Should return at least 1 workflow")

			By("Finding the created workflow in discovery results and asserting SA")
			var found *dsgen.WorkflowDiscoveryEntry
			for i := range discoveryResp.Workflows {
				if discoveryResp.Workflows[i].WorkflowId.String() == createdID {
					found = &discoveryResp.Workflows[i]
					break
				}
			}
			Expect(found).ToNot(BeNil(), "Created workflow should appear in discovery results")
			Expect(found.ServiceAccountName.Set).To(BeTrue(),
				"ServiceAccountName should be present in discovery entry (Set=true)")
			Expect(found.ServiceAccountName.Value).To(Equal("e2e-discovery-sa"),
				"ServiceAccountName value should match what was submitted")

			GinkgoWriter.Printf("E2E-DS-481-002: serviceAccountName in discovery endpoint verified\n")
		})
	})
})
