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
			discoveryWorkflowID, _ = ensureWorkflowRegistered(testCtx, DSClient, e2eTestWorkflowStubContent)
			logger.Info("✅ Discovery test workflow ready", "uuid", discoveryWorkflowID)
		})

		It("E2E-DS-017-001-001: should complete three-step discovery flow", func() {
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("E2E-DS-017-001-001: Three-step discovery happy path")
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// STEP 1: List available action types
			// Use Eventually to tolerate the same transient cache-visibility window
			// as STEP 2 below (#707) -- DataStorage's ActionType read path is now an
			// informer cache (DD-WORKFLOW-018) that can briefly lag a concurrent
			// parallel spec's ActionType create/re-seed until the watch delivers it.
			var actionTypes *dsgen.ActionTypeListResponse
			var foundActionType string
			Eventually(func() bool {
				step1Resp, listErr := DSClient.ListAvailableActions(testCtx, dsgen.ListAvailableActionsParams{
					Severity:    dsgen.ListAvailableActionsSeverityCritical,
					Component:   "v1/Pod",
					Environment: "production",
					Priority:    dsgen.ListAvailableActionsPriorityP0,
					Limit:       dsgen.NewOptInt(100),
				})
				if listErr != nil {
					return false
				}
				resp, ok := step1Resp.(*dsgen.ActionTypeListResponse)
				if !ok {
					return false
				}
				for _, at := range resp.ActionTypes {
					if at.ActionType == "ScaleReplicas" {
						actionTypes = resp
						foundActionType = at.ActionType
						return true
					}
				}
				return false
			}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(), "ScaleReplicas should be in action types")
			Expect(foundActionType).To(Equal("ScaleReplicas"), "ScaleReplicas should be in action types")
			logger.Info("✅ Step 1: Action types listed", "count", len(actionTypes.ActionTypes))

			// STEP 2: List workflows for ScaleReplicas
			// Use Eventually to tolerate transient visibility windows during parallel
			// workflow registration (#707: non-atomic supersede can create brief gaps).
			var foundWorkflowID string
			Eventually(func() string {
				step2Resp, listErr := DSClient.ListWorkflowsByActionType(testCtx, dsgen.ListWorkflowsByActionTypeParams{
					ActionType:  "ScaleReplicas",
					Severity:    dsgen.ListWorkflowsByActionTypeSeverityCritical,
					Component:   "v1/Pod",
					Environment: "production",
					Priority:    dsgen.ListWorkflowsByActionTypePriorityP0,
					Limit:       dsgen.NewOptInt(100),
				})
				if listErr != nil {
					return ""
				}
				workflows, ok := step2Resp.(*dsgen.WorkflowDiscoveryResponse)
				if !ok || len(workflows.Workflows) == 0 {
					return ""
				}
				for _, wf := range workflows.Workflows {
					if wf.WorkflowId.String() == discoveryWorkflowID {
						return wf.WorkflowId.String()
					}
				}
				return ""
			}, 30*time.Second, 2*time.Second).Should(Equal(discoveryWorkflowID),
				"Discovery test workflow should be listed")
			foundWorkflowID = discoveryWorkflowID
			logger.Info("✅ Step 2: Workflows listed", "foundID", foundWorkflowID)

			// STEP 3: Get full workflow detail with context filters (security gate)
			workflowUUID, err := uuid.Parse(discoveryWorkflowID)
			Expect(err).ToNot(HaveOccurred())

			step3Resp, err := DSClient.GetWorkflowByID(testCtx, dsgen.GetWorkflowByIDParams{
				WorkflowID:  workflowUUID,
				Severity:    dsgen.NewOptGetWorkflowByIDSeverity(dsgen.GetWorkflowByIDSeverityCritical),
				Component:   dsgen.NewOptString("v1/Pod"),
				Environment: dsgen.NewOptString("production"),
				Priority:    dsgen.NewOptGetWorkflowByIDPriority(dsgen.GetWorkflowByIDPriorityP0),
			})
			Expect(err).ToNot(HaveOccurred())

			fullWorkflow, ok := step3Resp.(*dsgen.RemediationWorkflow)
			Expect(ok).To(BeTrue(), "Expected *RemediationWorkflow from GetWorkflowByID Step 3")
			Expect(fullWorkflow.WorkflowId.Value.String()).To(Equal(discoveryWorkflowID))
			Expect(fullWorkflow.Content).ToNot(BeEmpty(), "Full workflow should include content (YAML)")
			Expect(fullWorkflow.ActionType).To(Equal("ScaleReplicas"))

			logger.Info("✅ Step 3: Full workflow detail retrieved",
				"workflow_id", fullWorkflow.WorkflowId.Value.String(),
				"action_type", fullWorkflow.ActionType)

			logger.Info("✅ E2E-DS-017-001-001: Three-step discovery happy path PASSED")
		})
	})

	// E2E-DS-017-001-002 ("disabled workflow excluded from discovery") removed:
	// #1661 Phase 55b — RemediationWorkflow.status.catalogStatus is now "Always
	// Active once admitted" (DD-WORKFLOW-018); there is no disable/enable state
	// machine for workflows anymore. Removing a workflow from the catalog means
	// deleting its CRD, not toggling a status flag.

	// ========================================
	// E2E-DS-017-001-003: Security gate 404 via E2E
	// ========================================
	Describe("Security Gate Context Filter", Label("security"), func() {
		It("E2E-DS-017-001-003: should return 404 when context filters mismatch", func() {
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("E2E-DS-017-001-003: Security gate — context mismatch → 404")
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// #1661 Phase 55b: register workflow via direct CRD creation (no
			// live AuthWebhook in this suite; DS's REST registration endpoint
			// was removed per DD-WORKFLOW-018).
			_, workflowUUID := ensureWorkflowRegistered(testCtx, DSClient, e2eTestWorkflowStubContent, "e2e-stub")

			// GetWorkflow with MISMATCHED context — should return 404
			step3Resp, err := DSClient.GetWorkflowByID(testCtx, dsgen.GetWorkflowByIDParams{
				WorkflowID:  workflowUUID,
				Severity:    dsgen.NewOptGetWorkflowByIDSeverity(dsgen.GetWorkflowByIDSeverityInfo), // mismatch: info != critical
				Component:   dsgen.NewOptString("apps/v1/StatefulSet"),                              // mismatch: StatefulSet != Pod
				Environment: dsgen.NewOptString("staging"),                                          // mismatch: staging != production
				Priority:    dsgen.NewOptGetWorkflowByIDPriority(dsgen.GetWorkflowByIDPriorityP3),   // mismatch: P3 != P0
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
			body := bytes.NewBufferString(`{"filters":{"signalName":"OOMKilled","severity":"critical","component":"v1/Pod","environment":"production","priority":"P0"}}`)

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
