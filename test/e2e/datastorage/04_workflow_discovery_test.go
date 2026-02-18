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
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// E2E-DS-017-001: Three-Step Workflow Discovery (DD-HAPI-017)
// ========================================
//
// Business Requirements:
//   - BR-HAPI-017-001: Three-step tool implementation
//   - BR-HAPI-017-003: Context filter security gate
//   - BR-HAPI-017-006: Old search endpoint removed
//
// Design Decisions:
//   - DD-WORKFLOW-016: Action-Type Workflow Catalog Indexing
//   - DD-HAPI-017: Three-Step Workflow Discovery Integration

var _ = Describe("E2E-DS-017-001: Three-Step Workflow Discovery (DD-HAPI-017)", Label("e2e", "datastorage", "discovery"), func() {
	var (
		testCtx    context.Context
		testCancel context.CancelFunc
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		DeferCleanup(testCancel)
	})

	// ========================================
	// E2E-DS-017-001-001: Three-step endpoints happy path
	// ========================================
	Describe("Three-Step Discovery Happy Path", Label("happy-path"), func() {
		var discoveryWorkflowID string

		BeforeEach(func() {
			// DD-WORKFLOW-017: Register workflow from OCI image (pullspec-only)
			// The image at quay.io contains /workflow-schema.yaml with ScaleReplicas action type,
			// OOMKilled signal type, and production labels for discovery E2E tests.
			createReq := dsgen.CreateWorkflowFromOCIRequest{
				SchemaImage: fmt.Sprintf("%s/discovery-test:v1.0.0", infrastructure.TestWorkflowBundleRegistry),
			}

			resp, err := DSClient.CreateWorkflow(testCtx, &createReq)
			Expect(err).ToNot(HaveOccurred())

			workflow, ok := resp.(*dsgen.RemediationWorkflow)
			Expect(ok).To(BeTrue(), "Expected *RemediationWorkflow response")
			discoveryWorkflowID = workflow.WorkflowId.Value.String()
			logger.Info("✅ Discovery test workflow created", "uuid", discoveryWorkflowID)
		})

		It("E2E-DS-017-001-001: should complete three-step discovery flow", func() {
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("E2E-DS-017-001-001: Three-step discovery happy path")
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// STEP 1: List available action types
			step1Resp, err := DSClient.ListAvailableActions(testCtx, dsgen.ListAvailableActionsParams{
				Severity:    dsgen.ListAvailableActionsSeverityCritical,
				Component:   "pod",
				Environment: "production",
				Priority:    dsgen.ListAvailableActionsPriorityP0,
				Limit:       dsgen.NewOptInt(100),
			})
			Expect(err).ToNot(HaveOccurred())

			actionTypes, ok := step1Resp.(*dsgen.ActionTypeListResponse)
			Expect(ok).To(BeTrue(), "Expected *ActionTypeListResponse")
			Expect(actionTypes.ActionTypes).ToNot(BeEmpty(), "Should return at least 1 action type")

			// Find ScaleReplicas in the results
			var foundActionType string
			for _, at := range actionTypes.ActionTypes {
				if at.ActionType == "ScaleReplicas" {
					foundActionType = at.ActionType
					break
				}
			}
			Expect(foundActionType).To(Equal("ScaleReplicas"), "ScaleReplicas should be in action types")
			logger.Info("✅ Step 1: Action types listed", "count", len(actionTypes.ActionTypes))

			// STEP 2: List workflows for ScaleReplicas
			step2Resp, err := DSClient.ListWorkflowsByActionType(testCtx, dsgen.ListWorkflowsByActionTypeParams{
				ActionType:  "ScaleReplicas",
				Severity:    dsgen.ListWorkflowsByActionTypeSeverityCritical,
				Component:   "pod",
				Environment: "production",
				Priority:    dsgen.ListWorkflowsByActionTypePriorityP0,
				Limit:       dsgen.NewOptInt(100),
			})
			Expect(err).ToNot(HaveOccurred())

			workflows, ok := step2Resp.(*dsgen.WorkflowDiscoveryResponse)
			Expect(ok).To(BeTrue(), "Expected *WorkflowDiscoveryResponse")
			Expect(workflows.Workflows).ToNot(BeEmpty(), "Should return at least 1 workflow")

			// Find our workflow in the results
			var foundWorkflowID string
			for _, wf := range workflows.Workflows {
				if wf.WorkflowId.String() == discoveryWorkflowID {
					foundWorkflowID = wf.WorkflowId.String()
					break
				}
			}
			Expect(foundWorkflowID).To(Equal(discoveryWorkflowID), "Discovery test workflow should be listed")
			logger.Info("✅ Step 2: Workflows listed", "count", len(workflows.Workflows))

			// STEP 3: Get full workflow detail with context filters (security gate)
			workflowUUID, err := uuid.Parse(discoveryWorkflowID)
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
			Expect(fullWorkflow.WorkflowId.Value.String()).To(Equal(discoveryWorkflowID))
			Expect(fullWorkflow.Content).ToNot(BeEmpty(), "Full workflow should include content (YAML)")
			Expect(fullWorkflow.ActionType).To(Equal("ScaleReplicas"))

			logger.Info("✅ Step 3: Full workflow detail retrieved",
				"workflow_id", fullWorkflow.WorkflowId.Value.String(),
				"action_type", fullWorkflow.ActionType)

			logger.Info("✅ E2E-DS-017-001-001: Three-step discovery happy path PASSED")
		})
	})

	// ========================================
	// E2E-DS-017-001-002: Disabled workflow excluded from discovery
	// ========================================
	Describe("Disabled Workflow Exclusion", Label("security"), func() {
		It("E2E-DS-017-001-002: should exclude disabled workflows from discovery results", func() {
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("E2E-DS-017-001-002: Disabled workflow excluded from discovery")
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// Create ACTIVE workflow via OCI pullspec (DD-WORKFLOW-017)
			activeReq := dsgen.CreateWorkflowFromOCIRequest{
				SchemaImage: fmt.Sprintf("%s/rollback-deployment-test:v1.0.0", infrastructure.TestWorkflowBundleRegistry),
			}
			activeResp, err := DSClient.CreateWorkflow(testCtx, &activeReq)
			Expect(err).ToNot(HaveOccurred())
			activeWorkflow, ok := activeResp.(*dsgen.RemediationWorkflow)
			Expect(ok).To(BeTrue())
			activeUUID := activeWorkflow.WorkflowId.Value.String()

			// Create a second workflow from OCI, then disable it via PATCH (DD-WORKFLOW-017)
			disabledReq := dsgen.CreateWorkflowFromOCIRequest{
				SchemaImage: fmt.Sprintf("%s/rollback-deployment-disabled-test:v1.0.0", infrastructure.TestWorkflowBundleRegistry),
			}
			disabledResp, err := DSClient.CreateWorkflow(testCtx, &disabledReq)
			Expect(err).ToNot(HaveOccurred())
			disabledWorkflow, ok := disabledResp.(*dsgen.RemediationWorkflow)
			Expect(ok).To(BeTrue())
			disabledUUID := disabledWorkflow.WorkflowId.Value

			// Disable the workflow via PATCH endpoint (GAP-WF-5: reason mandatory)
			disableReq := &dsgen.WorkflowLifecycleRequest{
				Reason: "E2E test: exclude disabled from discovery",
			}
			_, err = DSClient.DisableWorkflow(testCtx, disableReq, dsgen.DisableWorkflowParams{
				WorkflowID: disabledUUID,
			})
			Expect(err).ToNot(HaveOccurred())

			// Query discovery — disabled workflow should NOT appear
			step2Resp, err := DSClient.ListWorkflowsByActionType(testCtx, dsgen.ListWorkflowsByActionTypeParams{
				ActionType:  "RollbackDeployment",
				Severity:    dsgen.ListWorkflowsByActionTypeSeverityHigh,
				Component:   "deployment",
				Environment: "staging",
				Priority:    dsgen.ListWorkflowsByActionTypePriorityP1,
				Limit:       dsgen.NewOptInt(100),
			})
			Expect(err).ToNot(HaveOccurred())

			workflows, ok := step2Resp.(*dsgen.WorkflowDiscoveryResponse)
			Expect(ok).To(BeTrue())

			// Verify active workflow IS present, disabled IS NOT
			var foundActive, foundDisabled bool
			for _, wf := range workflows.Workflows {
				if wf.WorkflowId.String() == activeUUID {
					foundActive = true
				}
				if wf.WorkflowId == disabledUUID {
					foundDisabled = true
				}
			}
			Expect(foundActive).To(BeTrue(), "Active workflow should appear in discovery")
			Expect(foundDisabled).To(BeFalse(), "Disabled workflow should NOT appear in discovery")

			logger.Info("✅ E2E-DS-017-001-002: Disabled workflow correctly excluded")
		})
	})

	// ========================================
	// E2E-DS-017-001-003: Security gate 404 via E2E
	// ========================================
	Describe("Security Gate Context Filter", Label("security"), func() {
		It("E2E-DS-017-001-003: should return 404 when context filters mismatch", func() {
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("E2E-DS-017-001-003: Security gate — context mismatch → 404")
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// DD-WORKFLOW-017: Register workflow from OCI image for security gate test
			createReq := dsgen.CreateWorkflowFromOCIRequest{
				SchemaImage: fmt.Sprintf("%s/security-gate-test:v1.0.0", infrastructure.TestWorkflowBundleRegistry),
			}

			resp, err := DSClient.CreateWorkflow(testCtx, &createReq)
			Expect(err).ToNot(HaveOccurred())
			workflow, ok := resp.(*dsgen.RemediationWorkflow)
			Expect(ok).To(BeTrue())
			workflowUUID := workflow.WorkflowId.Value

			// GetWorkflow with MISMATCHED context — should return 404
			step3Resp, err := DSClient.GetWorkflowByID(testCtx, dsgen.GetWorkflowByIDParams{
				WorkflowID:  workflowUUID,
				Severity:    dsgen.NewOptGetWorkflowByIDSeverity(dsgen.GetWorkflowByIDSeverityLow),   // mismatch: low != critical
				Component:   dsgen.NewOptString("statefulset"),                                       // mismatch: statefulset != pod
				Environment: dsgen.NewOptString("staging"),                                           // mismatch: staging != production
				Priority:    dsgen.NewOptGetWorkflowByIDPriority(dsgen.GetWorkflowByIDPriorityP3),    // mismatch: P3 != P0
			})
			Expect(err).ToNot(HaveOccurred())

			// Security gate should return 404 (Not Found)
			_, isNotFound := step3Resp.(*dsgen.GetWorkflowByIDNotFound)
			Expect(isNotFound).To(BeTrue(), "Security gate should return 404 for context mismatch")

			logger.Info("✅ E2E-DS-017-001-003: Security gate correctly returned 404 on context mismatch")
		})
	})

	// ========================================
	// E2E-DS-017-006-001: Old search endpoint removed
	// ========================================
	Describe("Old Search Endpoint Removed", Label("security"), func() {
		It("E2E-DS-017-006-001: should return 404/405 for POST /api/v1/workflows/search", func() {
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("E2E-DS-017-006-001: Old search endpoint removed")
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// Use AuthHTTPClient for raw HTTP request (endpoint no longer in ogen client)
			searchURL := fmt.Sprintf("%s/api/v1/workflows/search", dataStorageURL)
			body := bytes.NewBufferString(`{"filters":{"signalType":"OOMKilled","severity":"critical","component":"pod","environment":"production","priority":"P0"}}`)

			req, err := http.NewRequestWithContext(testCtx, http.MethodPost, searchURL, body)
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := AuthHTTPClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() {
				_, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
			}()

			// Should be 404 (Not Found) or 405 (Method Not Allowed)
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusNotFound),
				Equal(http.StatusMethodNotAllowed),
			), "POST /search should return 404 or 405 (endpoint removed per DD-HAPI-017)")

			logger.Info("✅ E2E-DS-017-006-001: Old search endpoint correctly removed",
				"status_code", resp.StatusCode)
		})
	})
})
