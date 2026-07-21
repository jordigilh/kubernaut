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
// Test IDs: IT-DS-595-001, IT-DS-595-002, IT-DS-595-004 through IT-DS-595-006
//
// These tests validate that workflow discovery correctly matches labels
// regardless of case (e.g., "Production" query matches ["production"] label),
// via the cache-backed discovery path.
//
// #1661 Phase F: migrated off workflowRepo.Create (Postgres, zero production
// callers post-Phase-B) to seedWorkflowCRD -- DD-WORKFLOW-018 (etcd sole
// source of truth).
//
// IT-DS-595-003 ("lowercase priority query matches uppercase array label") was
// removed as obsolete rather than migrated: it exercised a raw
// `UPDATE ... SET labels = jsonb_set(labels, '{priority}', '["P0","P1"]')` SQL
// statement to force priority into a JSONB *array* representation -- a
// Postgres-JSONB-column storage quirk. Every CRD-native/cache-backed
// representation of priority (models.WorkflowSchemaLabels.Priority,
// models.MandatoryLabels.Priority) is a scalar string by construction; there
// is no code path, and no raw-SQL equivalent, that can produce an array-typed
// priority for a RemediationWorkflow CRD, so the scenario has no reachable
// CRD-native counterpart.
// ========================================

var _ = Describe("Workflow Discovery: Case-Insensitive Label Matching (#595)", Serial, func() {
	var (
		workflowRepo *workflow.Repository
		testID       string
	)

	BeforeEach(func() {
		workflowRepo = newCachedWorkflowRepo()
		testID = generateTestID()
		// #1661: this Describe asserts unscoped/global counts -- close the
		// cross-process cache lag race, see waitForWorkflowCacheConverged's
		// doc comment (workflow_crd_seeding_helper_test.go).
		waitForWorkflowCacheConverged()
	})

	createWorkflow := func(name, actionType string, severity []string, component string, environment []string, priority string) string {
		return seedWorkflowCRD(workflowCRDSpec{
			Name:        testID + "-" + name,
			ActionType:  actionType,
			Severity:    severity,
			Component:   []string{component},
			Environment: environment,
			Priority:    priority,
		})
	}

	// ========================================
	// IT-DS-595-001: Environment PascalCase query matches lowercase label
	// ========================================
	Describe("ListActions - Environment Case Mismatch (#595)", func() {
		Context("IT-DS-595-001: PascalCase environment query matches lowercase label", func() {
			It("should find workflow with environment=['production'] when queried with 'Production'", func() {
				createWorkflow("env-case", "ScaleReplicas",
					[]string{"critical"}, "v1/Pod", []string{"production"}, "P0")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "v1/Pod",
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
					[]string{"critical"}, "v1/Pod", []string{"production"}, "P0")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "Critical",
					Component:   "v1/Pod",
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
	// IT-DS-595-004: Full Issue #595 reproduction -- all labels case-mismatched
	// ========================================
	Describe("Full Reproduction (#595)", func() {
		Context("IT-DS-595-004: PascalCase filters on all 4 mandatory labels", func() {
			It("should find workflow when all labels have case mismatches", func() {
				createWorkflow("full-repro", "ScaleReplicas",
					[]string{"critical"}, "apps/v1/Deployment", []string{"production"}, "P0")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "Critical",
					Component:   "Apps/V1/Deployment",
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
					[]string{"*"}, "v1/Pod", []string{"*"}, "P0")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "Critical",
					Component:   "v1/Pod",
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
				workflowID := createWorkflow("gate-case", "ScaleReplicas",
					[]string{"critical"}, "v1/Pod", []string{"production"}, "P0")

				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "v1/Pod",
					Environment: "Production",
					Priority:    "P0",
				}
				result, err := workflowRepo.GetWorkflowWithContextFilters(ctx, workflowID, filters)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil(),
					"IT-DS-595-006: security gate must pass with case-insensitive environment matching")
				Expect(result.WorkflowID).To(Equal(workflowID))
			})
		})
	})
})
