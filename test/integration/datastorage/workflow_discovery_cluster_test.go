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
	"crypto/sha256"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
)

// ========================================
// WORKFLOW DISCOVERY: CLUSTER CLASSIFICATION FILTER DIMENSION
// ========================================
//
// Authority: BR-FLEET-003, DD-FLEET-002, issue #1511
// Test Plan: docs/tests/1511/TEST_PLAN.md
// Test IDs: IT-DS-1511-001 through IT-DS-1511-003
//
// These tests validate that the optional `cluster` filter dimension executes
// correctly against real PostgreSQL: exact match, exclusion of unlabeled
// workflows once the filter is active, and zero behavioral change when no
// `cluster` param is supplied (non-fleet deployments, backward compatible).
// ========================================

var _ = Describe("Workflow Discovery: Cluster Classification Filter (BR-FLEET-003, #1511)", Serial, func() {
	var (
		workflowRepo *workflow.Repository
		testID       string
	)

	BeforeEach(func() {
		workflowRepo = workflow.NewRepository(db, logger)
		testID = generateTestID()

		_, err := db.ExecContext(ctx, "TRUNCATE TABLE remediation_workflow_catalog")
		Expect(err).ToNot(HaveOccurred(), "Workflow catalog truncation should succeed")
	})

	AfterEach(func() {
		if db != nil {
			_, _ = db.ExecContext(ctx,
				"DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE $1",
				fmt.Sprintf("wf-1511-%s%%", testID))
		}
	})

	createWorkflow := func(name, actionType string, cluster []string) *models.RemediationWorkflow {
		content := fmt.Sprintf("apiVersion: v1\nkind: Workflow\nname: %s", name)
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

		wf := &models.RemediationWorkflow{
			WorkflowName:  fmt.Sprintf("wf-1511-%s-%s", testID, name),
			Version:       "v1.0.0",
			SchemaVersion: "1.0",
			Name:          name,
			Description: models.StructuredDescription{
				What:      fmt.Sprintf("Test workflow %s for cluster classification filtering", name),
				WhenToUse: "Testing",
			},
			Content:     content,
			ContentHash: hash,
			Labels: models.MandatoryLabels{
				Severity:    []string{"critical"},
				Component:   []string{"v1/Pod"},
				Environment: []string{"production"},
				Priority:    "P0",
				Cluster:     cluster,
			},
			ExecutionEngine: models.ExecutionEngineTekton,
			Status:          "Active",
			IsLatestVersion: true,
			ActionType:      actionType,
		}

		err := workflowRepo.Create(ctx, wf)
		Expect(err).ToNot(HaveOccurred(), "Workflow creation should succeed for %s", name)
		Expect(wf.WorkflowID).ToNot(BeEmpty(), "Workflow ID should be generated")
		return wf
	}

	// ========================================
	// IT-DS-1511-001: cluster filter executes against real PostgreSQL, exact match (AC-4)
	// ========================================
	Describe("IT-DS-1511-001: cluster filter, exact match", func() {
		It("returns only the workflow labeled for the requested cluster classification", func() {
			createWorkflow("prod-only", "ScaleReplicas", []string{"production"})
			createWorkflow("staging-only", "ScaleReplicas", []string{"staging"})

			filters := &models.WorkflowDiscoveryFilters{
				Severity:    "critical",
				Component:   "v1/Pod",
				Environment: "production",
				Priority:    "P0",
				Cluster:     "production",
			}
			result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1),
				"IT-DS-1511-001: only the 'production'-labeled workflow must match cluster=production")
			Expect(result).To(HaveLen(1))

			workflows, wfCount, err := workflowRepo.ListWorkflowsByActionType(ctx, "ScaleReplicas", filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
			Expect(wfCount).To(Equal(1),
				"IT-DS-1511-001: ListWorkflowsByActionType must also filter by cluster classification")
			Expect(workflows).To(HaveLen(1))
		})
	})

	// ========================================
	// IT-DS-1511-002: cluster filter excludes unlabeled workflows once active (SC-7)
	// ========================================
	Describe("IT-DS-1511-002: cluster filter excludes unlabeled workflows", func() {
		It("does not return a workflow with no cluster label once a cluster filter is active", func() {
			createWorkflow("no-cluster-label", "RestartPod", nil)

			filters := &models.WorkflowDiscoveryFilters{
				Severity:    "critical",
				Component:   "v1/Pod",
				Environment: "production",
				Priority:    "P0",
				Cluster:     "production",
			}
			result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(0),
				"IT-DS-1511-002: workflow with no cluster label must be excluded once cluster filter is active")
			Expect(result).To(BeEmpty())
		})

		It("returns a workflow labeled cluster:['*'] regardless of the requested classification", func() {
			createWorkflow("wildcard-cluster", "RestartPod", []string{"*"})

			filters := &models.WorkflowDiscoveryFilters{
				Severity:    "critical",
				Component:   "v1/Pod",
				Environment: "production",
				Priority:    "P0",
				Cluster:     "staging-eu",
			}
			result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1),
				"IT-DS-1511-002: cluster:['*'] must match any concrete cluster filter value")
			Expect(result).To(HaveLen(1))
		})
	})

	// ========================================
	// IT-DS-1511-003: no cluster param -> identical result set to pre-#1511 behavior (SC-7, regression)
	// ========================================
	Describe("IT-DS-1511-003: no cluster param is a zero behavioral change (regression)", func() {
		It("returns all workflows matching other filters regardless of cluster label presence", func() {
			createWorkflow("labeled", "ScaleReplicas", []string{"production"})
			createWorkflow("unlabeled", "ScaleReplicas", nil)

			filters := &models.WorkflowDiscoveryFilters{
				Severity:    "critical",
				Component:   "v1/Pod",
				Environment: "production",
				Priority:    "P0",
				// Cluster intentionally omitted -- simulates a non-fleet deployment.
			}
			result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1),
				"IT-DS-1511-003: with no cluster param, both labeled and unlabeled workflows count toward the same ScaleReplicas action type")
			Expect(result).To(HaveLen(1))
			Expect(result[0].WorkflowCount).To(Equal(2),
				"IT-DS-1511-003: no cluster filter applied -- both the labeled and unlabeled workflow must be counted")
		})
	})
})
