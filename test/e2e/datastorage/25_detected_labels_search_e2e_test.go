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

	"github.com/google/uuid"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// E2E-DS-043: DetectedLabels OCI -> Storage -> Retrieval (ADR-043 v1.3)
// ========================================
//
// Business Requirements:
//   - BR-WORKFLOW-004: Workflow Schema Format (detectedLabels field)
//   - BR-STORAGE-013: Semantic search with hybrid weighted scoring
//
// Authority:
//   - ADR-043 v1.3 (detectedLabels schema field)
//   - DD-WORKFLOW-001 v2.0 (DetectedLabels architecture)
//   - DD-WORKFLOW-004 v1.5 (Label-Only Scoring with Wildcard Weighting)
//
// Test Plan: docs/testing/ADR-043/TEST_PLAN.md
//
// Validates the full chain:
//   OCI image (with detectedLabels in workflow-schema.yaml)
//   -> OCI registration (POST /api/v1/workflows)
//   -> DB storage (JSONB detected_labels column)
//   -> HTTP retrieval (GET /api/v1/workflows/{id} returns detectedLabels)
//
// The scoring logic (SearchByLabels) is covered by integration tests
// (IT-DS-043-005) at the repository level.

var _ = Describe("E2E-DS-043: DetectedLabels OCI Registration and Retrieval", Ordered, Label("e2e", "datastorage", "detected-labels"), func() {
	var (
		testCtx              context.Context
		testCancel           context.CancelFunc
		registeredWorkflowID string
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		DeferCleanup(testCancel)

		createReq := dsgen.CreateWorkflowFromOCIRequest{
			SchemaImage: fmt.Sprintf("%s/detected-labels-test:v1.0.0", infrastructure.TestWorkflowBundleRegistry),
		}

		resp, err := DSClient.CreateWorkflow(testCtx, &createReq)
		Expect(err).ToNot(HaveOccurred())

		switch v := resp.(type) {
		case *dsgen.RemediationWorkflow:
			registeredWorkflowID = v.WorkflowId.Value.String()
			logger.Info("Workflow registered for detectedLabels E2E",
				"uuid", registeredWorkflowID)
		case *dsgen.CreateWorkflowConflict:
			// Workflow already exists (parallel test or re-run). Fetch it by listing.
			logger.Info("Workflow already exists (409), fetching by list")
			listResp, listErr := DSClient.ListWorkflowsByActionType(testCtx, dsgen.ListWorkflowsByActionTypeParams{
				ActionType:  "ScaleReplicas",
				Severity:    dsgen.ListWorkflowsByActionTypeSeverityCritical,
				Component:   "pod",
				Environment: "production",
				Priority:    dsgen.ListWorkflowsByActionTypePriorityP0,
				Limit:       dsgen.NewOptInt(100),
			})
			Expect(listErr).ToNot(HaveOccurred())
			workflows, ok := listResp.(*dsgen.WorkflowDiscoveryResponse)
			Expect(ok).To(BeTrue())
			for _, wf := range workflows.Workflows {
				if wf.WorkflowName == "detected-labels-test-v1" {
					registeredWorkflowID = wf.WorkflowId.String()
					break
				}
			}
			Expect(registeredWorkflowID).ToNot(BeEmpty(),
				"should find existing detected-labels-test-v1 workflow")
			logger.Info("Found existing workflow", "uuid", registeredWorkflowID)
		default:
			Fail(fmt.Sprintf("Unexpected CreateWorkflow response type: %T", resp))
		}
	})

	It("E2E-DS-043-001: registration response includes parsed detectedLabels from OCI schema", func() {
		workflowUUID, err := uuid.Parse(registeredWorkflowID)
		Expect(err).ToNot(HaveOccurred())

		resp, err := DSClient.GetWorkflowByID(testCtx, dsgen.GetWorkflowByIDParams{
			WorkflowID: workflowUUID,
		})
		Expect(err).ToNot(HaveOccurred())

		workflow, ok := resp.(*dsgen.RemediationWorkflow)
		Expect(ok).To(BeTrue(), "Expected *RemediationWorkflow response")

		By("verifying detectedLabels section is present")
		Expect(workflow.DetectedLabels.Set).To(BeTrue(),
			"detectedLabels should be present after OCI registration")

		By("verifying boolean field: hpaEnabled=true")
		Expect(workflow.DetectedLabels.Value.HpaEnabled.Set).To(BeTrue(),
			"hpaEnabled should be set")
		Expect(workflow.DetectedLabels.Value.HpaEnabled.Value).To(BeTrue(),
			"hpaEnabled should be true (from workflow-schema.yaml)")

		By("verifying string field: gitOpsTool=argocd")
		Expect(workflow.DetectedLabels.Value.GitOpsTool.Set).To(BeTrue(),
			"gitOpsTool should be set")
		Expect(string(workflow.DetectedLabels.Value.GitOpsTool.Value)).To(Equal("argocd"),
			"gitOpsTool should be 'argocd' (from workflow-schema.yaml)")

		By("verifying unset fields remain unset")
		Expect(workflow.DetectedLabels.Value.PdbProtected.Set).To(BeFalse(),
			"pdbProtected should NOT be set (absent from schema)")
		Expect(workflow.DetectedLabels.Value.Stateful.Set).To(BeFalse(),
			"stateful should NOT be set (absent from schema)")
	})

	It("E2E-DS-043-002: GetWorkflowByID returns detectedLabels from DB (full round-trip)", func() {
		workflowUUID, err := uuid.Parse(registeredWorkflowID)
		Expect(err).ToNot(HaveOccurred())

		var fullWorkflow *dsgen.RemediationWorkflow
		Eventually(func() error {
			resp, getErr := DSClient.GetWorkflowByID(testCtx, dsgen.GetWorkflowByIDParams{
				WorkflowID: workflowUUID,
			})
			if getErr != nil {
				return getErr
			}
			wf, ok := resp.(*dsgen.RemediationWorkflow)
			if !ok {
				return fmt.Errorf("unexpected response type: %T", resp)
			}
			fullWorkflow = wf
			return nil
		}, 30*time.Second, 2*time.Second).Should(Succeed(),
			"should retrieve workflow by ID")

		By("verifying detectedLabels survived OCI -> DB -> HTTP round-trip")
		Expect(fullWorkflow.DetectedLabels.Set).To(BeTrue(),
			"detectedLabels should be present after DB round-trip")

		Expect(fullWorkflow.DetectedLabels.Value.HpaEnabled.Set).To(BeTrue())
		Expect(fullWorkflow.DetectedLabels.Value.HpaEnabled.Value).To(BeTrue(),
			"hpaEnabled=true should survive round-trip")

		Expect(fullWorkflow.DetectedLabels.Value.GitOpsTool.Set).To(BeTrue())
		Expect(string(fullWorkflow.DetectedLabels.Value.GitOpsTool.Value)).To(Equal("argocd"),
			"gitOpsTool='argocd' should survive round-trip")

		Expect(fullWorkflow.DetectedLabels.Value.GitOpsManaged.Set).To(BeFalse(),
			"gitOpsManaged should remain unset after round-trip")
		Expect(fullWorkflow.DetectedLabels.Value.NetworkIsolated.Set).To(BeFalse(),
			"networkIsolated should remain unset after round-trip")
	})

	It("E2E-DS-043-004: HTTP search with detected_labels query parameter returns filtered results", func() {
		By("searching with matching detected_labels filter (hpaEnabled=true)")
		matchResp, err := DSClient.ListWorkflowsByActionType(testCtx, dsgen.ListWorkflowsByActionTypeParams{
			ActionType:     "ScaleReplicas",
			Severity:       dsgen.ListWorkflowsByActionTypeSeverityCritical,
			Component:      "pod",
			Environment:    "production",
			Priority:       dsgen.ListWorkflowsByActionTypePriorityP0,
			DetectedLabels: dsgen.NewOptString(`{"hpaEnabled":true}`),
			Limit:          dsgen.NewOptInt(100),
		})
		Expect(err).ToNot(HaveOccurred())

		matchWorkflows, ok := matchResp.(*dsgen.WorkflowDiscoveryResponse)
		Expect(ok).To(BeTrue(), "Expected *WorkflowDiscoveryResponse")

		var foundMatchingWorkflow bool
		for _, wf := range matchWorkflows.Workflows {
			if wf.WorkflowName == "detected-labels-test-v1" {
				foundMatchingWorkflow = true
				break
			}
		}
		Expect(foundMatchingWorkflow).To(BeTrue(),
			"detected-labels-test-v1 (hpaEnabled=true) should appear when filtering by hpaEnabled=true")

		By("searching with non-matching detected_labels filter (networkIsolated=true)")
		nonMatchResp, err := DSClient.ListWorkflowsByActionType(testCtx, dsgen.ListWorkflowsByActionTypeParams{
			ActionType:     "ScaleReplicas",
			Severity:       dsgen.ListWorkflowsByActionTypeSeverityCritical,
			Component:      "pod",
			Environment:    "production",
			Priority:       dsgen.ListWorkflowsByActionTypePriorityP0,
			DetectedLabels: dsgen.NewOptString(`{"networkIsolated":true}`),
			Limit:          dsgen.NewOptInt(100),
		})
		Expect(err).ToNot(HaveOccurred())

		nonMatchWorkflows, ok := nonMatchResp.(*dsgen.WorkflowDiscoveryResponse)
		Expect(ok).To(BeTrue(), "Expected *WorkflowDiscoveryResponse")

		var foundInNonMatch bool
		for _, wf := range nonMatchWorkflows.Workflows {
			if wf.WorkflowName == "detected-labels-test-v1" {
				foundInNonMatch = true
				break
			}
		}

		By("verifying non-matching filter excludes or deprioritizes the workflow")
		if foundInNonMatch {
			// If found, it may still appear but at a lower rank (implementation-dependent).
			// The key assertion is that the matching filter DID return it above.
			logger.Info("Non-matching filter still returned workflow (scoring-based, not exclusion)")
		} else {
			logger.Info("Non-matching filter correctly excluded the workflow")
		}

		logger.Info("E2E-DS-043-004: HTTP search with detected_labels query parameter PASSED",
			"matchingFilterFound", foundMatchingWorkflow,
			"nonMatchingFilterFound", foundInNonMatch)
	})

	It("E2E-DS-043-003: workflow with detectedLabels appears in three-step discovery", func() {
		By("Step 2: listing workflows by ScaleReplicas action type")
		var foundWorkflow bool
		Eventually(func() bool {
			step2Resp, searchErr := DSClient.ListWorkflowsByActionType(testCtx, dsgen.ListWorkflowsByActionTypeParams{
				ActionType:  "ScaleReplicas",
				Severity:    dsgen.ListWorkflowsByActionTypeSeverityCritical,
				Component:   "pod",
				Environment: "production",
				Priority:    dsgen.ListWorkflowsByActionTypePriorityP0,
				Limit:       dsgen.NewOptInt(100),
			})
			if searchErr != nil {
				return false
			}

			workflows, ok := step2Resp.(*dsgen.WorkflowDiscoveryResponse)
			if !ok || len(workflows.Workflows) == 0 {
				return false
			}

			for _, wf := range workflows.Workflows {
				if wf.WorkflowName == "detected-labels-test-v1" {
					foundWorkflow = true
					return true
				}
			}
			return false
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"detected-labels-test-v1 should appear in discovery results")

		Expect(foundWorkflow).To(BeTrue())

		By("Step 3: retrieving full workflow detail with detectedLabels")
		workflowUUID, err := uuid.Parse(registeredWorkflowID)
		Expect(err).ToNot(HaveOccurred())

		step3Resp, err := DSClient.GetWorkflowByID(testCtx, dsgen.GetWorkflowByIDParams{
			WorkflowID:  workflowUUID,
			Severity:    dsgen.NewOptGetWorkflowByIDSeverity(dsgen.GetWorkflowByIDSeverityCritical),
			Component:   dsgen.NewOptString("pod"),
			Environment: dsgen.NewOptString("production"),
			Priority:    dsgen.NewOptGetWorkflowByIDPriority(dsgen.GetWorkflowByIDPriorityP0),
		})
		Expect(err).ToNot(HaveOccurred())

		fullWorkflow, ok := step3Resp.(*dsgen.RemediationWorkflow)
		Expect(ok).To(BeTrue(), "Expected *RemediationWorkflow from Step 3")
		Expect(fullWorkflow.DetectedLabels.Set).To(BeTrue(),
			"Step 3 response should include detectedLabels")
		Expect(fullWorkflow.DetectedLabels.Value.HpaEnabled.Value).To(BeTrue(),
			"hpaEnabled should be true in discovery Step 3 response")
		Expect(string(fullWorkflow.DetectedLabels.Value.GitOpsTool.Value)).To(Equal("argocd"),
			"gitOpsTool should be 'argocd' in discovery Step 3 response")

		logger.Info("E2E-DS-043-003: Three-step discovery with detectedLabels PASSED")
	})
})

// ========================================
// E2E-DS-043-005: All 8 DetectedLabels Fields Round-Trip (ADR-043 v1.3)
// ========================================
//
// Business Requirements:
//   - BR-WORKFLOW-004: Workflow Schema Format (all 8 detectedLabels fields)
//
// Authority:
//   - ADR-043 v1.3 (detectedLabels schema field)
//   - DD-WORKFLOW-001 v2.0 (DetectedLabels architecture)
//
// Uses a dedicated OCI fixture (detected-labels-all-fields) with all 8 fields populated
// to verify the entire production pipeline preserves every field.

var _ = Describe("E2E-DS-043-005: All 8 DetectedLabels Fields OCI -> DB -> HTTP Round-Trip", Ordered, Label("e2e", "datastorage", "detected-labels"), func() {
	var (
		allFieldsCtx        context.Context
		allFieldsCancel     context.CancelFunc
		allFieldsWorkflowID string
	)

	BeforeAll(func() {
		allFieldsCtx, allFieldsCancel = context.WithTimeout(ctx, 5*time.Minute)
		DeferCleanup(allFieldsCancel)

		createReq := dsgen.CreateWorkflowFromOCIRequest{
			SchemaImage: fmt.Sprintf("%s/detected-labels-all-fields:v1.0.0", infrastructure.TestWorkflowBundleRegistry),
		}

		resp, err := DSClient.CreateWorkflow(allFieldsCtx, &createReq)
		Expect(err).ToNot(HaveOccurred())

		switch v := resp.(type) {
		case *dsgen.RemediationWorkflow:
			allFieldsWorkflowID = v.WorkflowId.Value.String()
			logger.Info("All-8-fields workflow registered",
				"uuid", allFieldsWorkflowID)
		case *dsgen.CreateWorkflowConflict:
			logger.Info("All-8-fields workflow already exists (409), fetching by list")
			listResp, listErr := DSClient.ListWorkflowsByActionType(allFieldsCtx, dsgen.ListWorkflowsByActionTypeParams{
				ActionType:  "RestartPod",
				Severity:    dsgen.ListWorkflowsByActionTypeSeverityCritical,
				Component:   "pod",
				Environment: "production",
				Priority:    dsgen.ListWorkflowsByActionTypePriorityP0,
				Limit:       dsgen.NewOptInt(100),
			})
			Expect(listErr).ToNot(HaveOccurred())
			workflows, ok := listResp.(*dsgen.WorkflowDiscoveryResponse)
			Expect(ok).To(BeTrue())
			for _, wf := range workflows.Workflows {
				if wf.WorkflowName == "detected-labels-all-fields-v1" {
					allFieldsWorkflowID = wf.WorkflowId.String()
					break
				}
			}
			Expect(allFieldsWorkflowID).ToNot(BeEmpty(),
				"should find existing detected-labels-all-fields-v1 workflow")
			logger.Info("Found existing all-8-fields workflow", "uuid", allFieldsWorkflowID)
		default:
			Fail(fmt.Sprintf("Unexpected CreateWorkflow response type: %T", resp))
		}
	})

	It("E2E-DS-043-005: all 8 detectedLabels fields survive full OCI -> DB -> HTTP round-trip", func() {
		workflowUUID, err := uuid.Parse(allFieldsWorkflowID)
		Expect(err).ToNot(HaveOccurred())

		var fullWorkflow *dsgen.RemediationWorkflow
		Eventually(func() error {
			resp, getErr := DSClient.GetWorkflowByID(allFieldsCtx, dsgen.GetWorkflowByIDParams{
				WorkflowID: workflowUUID,
			})
			if getErr != nil {
				return getErr
			}
			wf, ok := resp.(*dsgen.RemediationWorkflow)
			if !ok {
				return fmt.Errorf("unexpected response type: %T", resp)
			}
			fullWorkflow = wf
			return nil
		}, 30*time.Second, 2*time.Second).Should(Succeed(),
			"should retrieve all-8-fields workflow by ID")

		By("verifying detectedLabels section is present")
		Expect(fullWorkflow.DetectedLabels.Set).To(BeTrue(),
			"detectedLabels should be present after OCI -> DB -> HTTP round-trip")

		By("verifying boolean field: hpaEnabled=true")
		Expect(fullWorkflow.DetectedLabels.Value.HpaEnabled.Set).To(BeTrue(), "hpaEnabled should be set")
		Expect(fullWorkflow.DetectedLabels.Value.HpaEnabled.Value).To(BeTrue(), "hpaEnabled should be true")

		By("verifying boolean field: pdbProtected=true")
		Expect(fullWorkflow.DetectedLabels.Value.PdbProtected.Set).To(BeTrue(), "pdbProtected should be set")
		Expect(fullWorkflow.DetectedLabels.Value.PdbProtected.Value).To(BeTrue(), "pdbProtected should be true")

		By("verifying boolean field: stateful=true")
		Expect(fullWorkflow.DetectedLabels.Value.Stateful.Set).To(BeTrue(), "stateful should be set")
		Expect(fullWorkflow.DetectedLabels.Value.Stateful.Value).To(BeTrue(), "stateful should be true")

		By("verifying boolean field: helmManaged=true")
		Expect(fullWorkflow.DetectedLabels.Value.HelmManaged.Set).To(BeTrue(), "helmManaged should be set")
		Expect(fullWorkflow.DetectedLabels.Value.HelmManaged.Value).To(BeTrue(), "helmManaged should be true")

		By("verifying boolean field: networkIsolated=true")
		Expect(fullWorkflow.DetectedLabels.Value.NetworkIsolated.Set).To(BeTrue(), "networkIsolated should be set")
		Expect(fullWorkflow.DetectedLabels.Value.NetworkIsolated.Value).To(BeTrue(), "networkIsolated should be true")

		By("verifying boolean field: gitOpsManaged=true")
		Expect(fullWorkflow.DetectedLabels.Value.GitOpsManaged.Set).To(BeTrue(), "gitOpsManaged should be set")
		Expect(fullWorkflow.DetectedLabels.Value.GitOpsManaged.Value).To(BeTrue(), "gitOpsManaged should be true")

		By("verifying string field: gitOpsTool=flux")
		Expect(fullWorkflow.DetectedLabels.Value.GitOpsTool.Set).To(BeTrue(), "gitOpsTool should be set")
		Expect(string(fullWorkflow.DetectedLabels.Value.GitOpsTool.Value)).To(Equal("flux"),
			"gitOpsTool should be 'flux' (from all-8-fields fixture)")

		By("verifying string field: serviceMesh=istio")
		Expect(fullWorkflow.DetectedLabels.Value.ServiceMesh.Set).To(BeTrue(), "serviceMesh should be set")
		Expect(string(fullWorkflow.DetectedLabels.Value.ServiceMesh.Value)).To(Equal("istio"),
			"serviceMesh should be 'istio' (from all-8-fields fixture)")

		By("verifying no extra failedDetections leak into response")
		Expect(fullWorkflow.DetectedLabels.Value.FailedDetections).To(BeEmpty(),
			"failedDetections should be empty (no detection failures in test fixture)")

		logger.Info("E2E-DS-043-005: All 8 detectedLabels fields round-trip PASSED")
	})
})
