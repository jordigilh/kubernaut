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
// WORKFLOW DISCOVERY: CASE-INSENSITIVE LABEL MATCHING
// ========================================
//
// Authority: DD-WORKFLOW-001 v2.9 (case-insensitive JSONB array matching)
// Bug report: Issue #595 (SP produces PascalCase environment, DS stores lowercase)
// Test Plan: docs/tests/595/TEST_PLAN.md
// Test IDs: IT-DS-595-001 through IT-DS-595-006
//
// These tests validate that workflow discovery correctly matches labels
// regardless of case (e.g., "Production" query matches ["production"] label).
// ========================================

var _ = Describe("Workflow Discovery: Case-Insensitive Label Matching (#595)", Serial, func() {
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
				fmt.Sprintf("wf-595-%s%%", testID))
		}
	})

	createWorkflow := func(name, actionType string, severity []string, component string, environment []string, priority string) *models.RemediationWorkflow {
		content := fmt.Sprintf("apiVersion: v1\nkind: Workflow\nname: %s", name)
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

		wf := &models.RemediationWorkflow{
			WorkflowName:  fmt.Sprintf("wf-595-%s-%s", testID, name),
			Version:       "v1.0.0",
			SchemaVersion: "1.0",
			Name:          name,
			Description: models.StructuredDescription{
				What:      fmt.Sprintf("Test workflow %s for case-insensitive matching", name),
				WhenToUse: "Testing",
			},
			Content:         content,
			ContentHash:     hash,
			Labels: models.MandatoryLabels{
				Severity:    severity,
				Component:   []string{component},
				Environment: environment,
				Priority:    priority,
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
	// IT-DS-595-001: Environment PascalCase query matches lowercase label
	// ========================================
	Describe("ListActions - Environment Case Mismatch (#595)", func() {
		Context("IT-DS-595-001: PascalCase environment query matches lowercase label", func() {
			It("should find workflow with environment=['production'] when queried with 'Production'", func() {
				createWorkflow("env-case", "ScaleReplicas",
					[]string{"critical"}, "pod", []string{"production"}, "P0")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   []string{"pod"},
					Environment: "Production",
					Priority:    "P0",
				}
				result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1),
					"IT-DS-595-001: PascalCase 'Production' must match lowercase label ['production']")
				Expect(result).To(HaveLen(1))
				Expect(result[0].ActionType).To(Equal("ScaleReplicas"))
			})
		})
	})

	// ========================================
	// IT-DS-595-002: Severity PascalCase query matches lowercase label
	// ========================================
	Describe("ListActions - Severity Case Mismatch (#595)", func() {
		Context("IT-DS-595-002: PascalCase severity query matches lowercase label", func() {
			It("should find workflow with severity=['critical'] when queried with 'Critical'", func() {
				createWorkflow("sev-case", "RestartPod",
					[]string{"critical"}, "pod", []string{"production"}, "P0")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "Critical",
					Component:   []string{"pod"},
					Environment: "production",
					Priority:    "P0",
				}
				result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1),
					"IT-DS-595-002: PascalCase 'Critical' must match lowercase label ['critical']")
				Expect(result).To(HaveLen(1))
				Expect(result[0].ActionType).To(Equal("RestartPod"))
			})
		})
	})

	// ========================================
	// IT-DS-595-003: Priority array branch with lowercase query
	// ========================================
	Describe("ListActions - Priority Array Case Mismatch (#595)", func() {
		Context("IT-DS-595-003: lowercase priority query matches uppercase array label", func() {
			It("should find workflow with priority=['P0','P1'] (raw SQL array) when queried with 'p0'", func() {
				wf := createWorkflow("pri-case", "ScaleReplicas",
					[]string{"critical"}, "pod", []string{"production"}, "P0")

				// Raw SQL UPDATE to store priority as a JSONB array
				// (Go MandatoryLabels.Priority is string, so Create() stores scalar)
				_, err := db.ExecContext(ctx,
					`UPDATE remediation_workflow_catalog
					 SET labels = jsonb_set(labels, '{priority}', '["P0","P1"]')
					 WHERE workflow_id = $1`, wf.WorkflowID)
				Expect(err).ToNot(HaveOccurred(), "Raw SQL UPDATE for array priority should succeed")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   []string{"pod"},
					Environment: "production",
					Priority:    "p0",
				}
				result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1),
					"IT-DS-595-003: lowercase 'p0' must match array label ['P0','P1'] via case-insensitive matching")
				Expect(result).To(HaveLen(1))
			})
		})
	})

	// ========================================
	// IT-DS-595-004: Full Issue #595 reproduction -- all labels case-mismatched
	// ========================================
	Describe("Full Reproduction (#595)", func() {
		Context("IT-DS-595-004: PascalCase filters on all 4 mandatory labels", func() {
			It("should find workflow when all labels have case mismatches", func() {
				createWorkflow("full-repro", "ScaleReplicas",
					[]string{"critical"}, "deployment", []string{"production"}, "P0")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "Critical",
					Component:   []string{"Deployment"},
					Environment: "Production",
					Priority:    "P0",
				}

				result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)
				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1),
					"IT-DS-595-004: all PascalCase filters must match lowercase labels")
				Expect(result).To(HaveLen(1))

				results, totalCount2, err := workflowRepo.ListWorkflowsByActionType(ctx, "ScaleReplicas", filters, 0, 10)
				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount2).To(Equal(1),
					"IT-DS-595-004: ListWorkflowsByActionType must also match case-insensitively")
				Expect(results).To(HaveLen(1))
			})
		})
	})

	// ========================================
	// IT-DS-595-005: Wildcard labels still work with PascalCase queries
	// ========================================
	Describe("Wildcard Compatibility (#595)", func() {
		Context("IT-DS-595-005: wildcard labels match PascalCase queries", func() {
			It("should find workflow with severity=['*'], environment=['*'] when queried with PascalCase", func() {
				createWorkflow("wc-case", "RestartPod",
					[]string{"*"}, "pod", []string{"*"}, "P0")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "Critical",
					Component:   []string{"pod"},
					Environment: "Production",
					Priority:    "P0",
				}
				result, totalCount, err := workflowRepo.ListActions(ctx, filters, 0, 10)

				Expect(err).ToNot(HaveOccurred())
				Expect(totalCount).To(Equal(1),
					"IT-DS-595-005: wildcard labels must still match PascalCase queries")
				Expect(result).To(HaveLen(1))
			})
		})
	})

	// ========================================
	// IT-DS-595-006: GetWorkflowWithContextFilters security gate passes
	// ========================================
	Describe("GetWorkflowWithContextFilters - Case-Insensitive Security Gate (#595)", func() {
		Context("IT-DS-595-006: security gate passes with case-mismatched environment", func() {
			It("should return non-nil workflow when environment='Production' queries ['production'] label", func() {
				wf := createWorkflow("gate-case", "ScaleReplicas",
					[]string{"critical"}, "pod", []string{"production"}, "P0")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   []string{"pod"},
					Environment: "Production",
					Priority:    "P0",
				}
				result, err := workflowRepo.GetWorkflowWithContextFilters(ctx, wf.WorkflowID, filters)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil(),
					"IT-DS-595-006: security gate must pass with case-insensitive environment matching")
				Expect(result.WorkflowID).To(Equal(wf.WorkflowID))
			})
		})
	})
})
