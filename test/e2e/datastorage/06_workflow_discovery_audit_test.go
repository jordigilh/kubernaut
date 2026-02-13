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
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/google/uuid"
	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// E2E-DS-017-AUDIT: Workflow Discovery Audit Events (DD-HAPI-017)
// ========================================
//
// Business Requirements:
//   - BR-AUDIT-023: Workflow discovery audit trail
//
// Design Decisions:
//   - DD-WORKFLOW-014 v3.0: Workflow Selection Audit Trail
//   - DD-HAPI-017: Three-Step Workflow Discovery Integration

var _ = Describe("E2E-DS-017-AUDIT: Workflow Discovery Audit Events (DD-WORKFLOW-014 v3.0)", Label("e2e", "datastorage", "audit", "discovery"), func() {
	var (
		testCtx          context.Context
		testCancel       context.CancelFunc
		auditWorkflowID  string
		auditWorkflowUUID uuid.UUID
		remediationID    string
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		DeferCleanup(testCancel)

		// Create a workflow for audit event tests
		testID := uuid.New().String()[:8]
		remediationID = fmt.Sprintf("rem-audit-e2e-%s", testID)
		workflowName := fmt.Sprintf("audit-discovery-e2e-%s", testID)
		content := fmt.Sprintf(`apiVersion: kubernaut.io/v1alpha1
kind: WorkflowSchema
metadata:
  workflow_id: %s
  version: "1.0.0"
  description: Audit event test workflow
labels:
  signal_type: OOMKilled
  severity: critical
  risk_tolerance: low
  component: pod
  environment: production
  priority: p0
parameters:
  - name: TARGET_RESOURCE
    type: string
    required: true
    description: Target resource
execution:
  engine: tekton
  bundle: ghcr.io/kubernaut/workflows/audit-test:v1.0.0
`, workflowName)
		contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

		createReq := dsgen.RemediationWorkflow{
			WorkflowName:    workflowName,
			ActionType:      "ScaleReplicas",
			Version:         "1.0.0",
			Name:            "Audit Discovery E2E Test",
			Description:     "Workflow for discovery audit event testing",
			Content:         content,
			ContentHash:     contentHash,
			ExecutionEngine: "tekton",
			ContainerImage:  dsgen.NewOptString("ghcr.io/kubernaut/workflows/audit-test:v1.0.0"),
			Status:          dsgen.RemediationWorkflowStatusActive,
			Labels: dsgen.MandatoryLabels{
				SignalType:  "OOMKilled",
				Severity:    dsgen.MandatoryLabelsSeverity_critical,
				Component:   "pod",
				Priority:    dsgen.MandatoryLabelsPriority_P0,
				Environment: []dsgen.MandatoryLabelsEnvironmentItem{dsgen.MandatoryLabelsEnvironmentItem("production")},
			},
		}

		resp, err := DSClient.CreateWorkflow(testCtx, &createReq)
		Expect(err).ToNot(HaveOccurred())
		workflow, ok := resp.(*dsgen.RemediationWorkflow)
		Expect(ok).To(BeTrue())
		auditWorkflowID = workflow.WorkflowID.Value.String()
		auditWorkflowUUID = workflow.WorkflowID.Value
		logger.Info("✅ Audit test workflow created", "uuid", auditWorkflowID, "remediation_id", remediationID)
	})

	// ========================================
	// E2E-DS-017-AUDIT-001: actions_listed audit event
	// ========================================
	It("E2E-DS-017-AUDIT-001: should emit workflow.catalog.actions_listed audit event", func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("E2E-DS-017-AUDIT-001: actions_listed audit event")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// ACT: Call step 1 with remediation_id
		_, err := DSClient.ListAvailableActions(testCtx, dsgen.ListAvailableActionsParams{
			Severity:      dsgen.ListAvailableActionsSeverityCritical,
			Component:     "pod",
			Environment:   "production",
			Priority:      dsgen.ListAvailableActionsPriorityP0,
			RemediationID: dsgen.NewOptString(remediationID),
			Limit:         dsgen.NewOptInt(100),
		})
		Expect(err).ToNot(HaveOccurred())

		// ASSERT: Query audit events for actions_listed
		Eventually(func() bool {
			auditResp, err := DSClient.QueryAuditEvents(testCtx, dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(remediationID),
				EventCategory: dsgen.NewOptString(dsaudit.EventCategoryWorkflow),
				EventType:     dsgen.NewOptString(dsaudit.EventTypeActionsListed),
				Limit:         dsgen.NewOptInt(10),
			})
			if err != nil {
				return false
			}
			return len(auditResp.Data) >= 1
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"Expected workflow.catalog.actions_listed audit event with remediation_id=%s", remediationID)

		logger.Info("✅ E2E-DS-017-AUDIT-001: actions_listed audit event emitted")
	})

	// ========================================
	// E2E-DS-017-AUDIT-002: workflows_listed audit event
	// ========================================
	It("E2E-DS-017-AUDIT-002: should emit workflow.catalog.workflows_listed audit event", func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("E2E-DS-017-AUDIT-002: workflows_listed audit event")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// ACT: Call step 2 with remediation_id
		_, err := DSClient.ListWorkflowsByActionType(testCtx, dsgen.ListWorkflowsByActionTypeParams{
			ActionType:    "ScaleReplicas",
			Severity:      dsgen.ListWorkflowsByActionTypeSeverityCritical,
			Component:     "pod",
			Environment:   "production",
			Priority:      dsgen.ListWorkflowsByActionTypePriorityP0,
			RemediationID: dsgen.NewOptString(remediationID),
			Limit:         dsgen.NewOptInt(100),
		})
		Expect(err).ToNot(HaveOccurred())

		// ASSERT: Query audit events for workflows_listed
		Eventually(func() bool {
			auditResp, err := DSClient.QueryAuditEvents(testCtx, dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(remediationID),
				EventCategory: dsgen.NewOptString(dsaudit.EventCategoryWorkflow),
				EventType:     dsgen.NewOptString(dsaudit.EventTypeWorkflowsListed),
				Limit:         dsgen.NewOptInt(10),
			})
			if err != nil {
				return false
			}
			return len(auditResp.Data) >= 1
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"Expected workflow.catalog.workflows_listed audit event with remediation_id=%s", remediationID)

		logger.Info("✅ E2E-DS-017-AUDIT-002: workflows_listed audit event emitted")
	})

	// ========================================
	// E2E-DS-017-AUDIT-003: workflow_retrieved audit event
	// ========================================
	It("E2E-DS-017-AUDIT-003: should emit workflow.catalog.workflow_retrieved audit event", func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("E2E-DS-017-AUDIT-003: workflow_retrieved audit event")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// ACT: Call step 3 with remediation_id and context filters
		_, err := DSClient.GetWorkflowByID(testCtx, dsgen.GetWorkflowByIDParams{
			WorkflowID:    auditWorkflowUUID,
			Severity:      dsgen.NewOptGetWorkflowByIDSeverity(dsgen.GetWorkflowByIDSeverityCritical),
			Component:     dsgen.NewOptString("pod"),
			Environment:   dsgen.NewOptString("production"),
			Priority:      dsgen.NewOptGetWorkflowByIDPriority(dsgen.GetWorkflowByIDPriorityP0),
			RemediationID: dsgen.NewOptString(remediationID),
		})
		Expect(err).ToNot(HaveOccurred())

		// ASSERT: Query audit events for workflow_retrieved
		Eventually(func() bool {
			auditResp, err := DSClient.QueryAuditEvents(testCtx, dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(remediationID),
				EventCategory: dsgen.NewOptString(dsaudit.EventCategoryWorkflow),
				EventType:     dsgen.NewOptString(dsaudit.EventTypeWorkflowRetrieved),
				Limit:         dsgen.NewOptInt(10),
			})
			if err != nil {
				return false
			}
			return len(auditResp.Data) >= 1
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"Expected workflow.catalog.workflow_retrieved audit event with remediation_id=%s", remediationID)

		logger.Info("✅ E2E-DS-017-AUDIT-003: workflow_retrieved audit event emitted")
	})

	// ========================================
	// E2E-DS-017-AUDIT-004: selection_validated audit event
	// ========================================
	It("E2E-DS-017-AUDIT-004: should emit workflow.catalog.selection_validated audit event", func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("E2E-DS-017-AUDIT-004: selection_validated audit event")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// ACT: Call GetWorkflowByID with context filters (acts as validation)
		// DD-WORKFLOW-014 v3.0: selection_validated event is emitted when context
		// filters are present (indicates post-selection validation)
		validationRemID := fmt.Sprintf("rem-validate-e2e-%s", uuid.New().String()[:8])
		_, err := DSClient.GetWorkflowByID(testCtx, dsgen.GetWorkflowByIDParams{
			WorkflowID:    auditWorkflowUUID,
			Severity:      dsgen.NewOptGetWorkflowByIDSeverity(dsgen.GetWorkflowByIDSeverityCritical),
			Component:     dsgen.NewOptString("pod"),
			Environment:   dsgen.NewOptString("production"),
			Priority:      dsgen.NewOptGetWorkflowByIDPriority(dsgen.GetWorkflowByIDPriorityP0),
			RemediationID: dsgen.NewOptString(validationRemID),
		})
		Expect(err).ToNot(HaveOccurred())

		// ASSERT: Query audit events for selection_validated
		// Note: The handler emits both workflow_retrieved AND selection_validated
		// when context filters are present.
		Eventually(func() bool {
			auditResp, err := DSClient.QueryAuditEvents(testCtx, dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(validationRemID),
				EventCategory: dsgen.NewOptString(dsaudit.EventCategoryWorkflow),
				EventType:     dsgen.NewOptString(dsaudit.EventTypeSelectionValidated),
				Limit:         dsgen.NewOptInt(10),
			})
			if err != nil {
				return false
			}
			return len(auditResp.Data) >= 1
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"Expected workflow.catalog.selection_validated audit event with remediation_id=%s", validationRemID)

		logger.Info("✅ E2E-DS-017-AUDIT-004: selection_validated audit event emitted")
	})
})
