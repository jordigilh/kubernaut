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
	"crypto/sha256"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	actiontyperepo "github.com/jordigilh/kubernaut/pkg/datastorage/repository/actiontype"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
)

// ========================================
// ACTIONTYPE LIFECYCLE INTEGRATION TESTS (#300)
// ========================================
//
// Authority: BR-WORKFLOW-007 (ActionType CRD Lifecycle Management)
// Test Plan: docs/testing/300/TEST_PLAN.md
//
// Tests ActionType repository CRUD operations against REAL PostgreSQL.
// Validates idempotency, re-enable, soft-disable, dependency guard,
// description updates, and schema integrity.
//
// Defense-in-Depth Strategy:
// - Unit tests (Phase 7a): Handler logic with mocked DS client
// - Integration tests (THIS FILE): Repository CRUD against real DB
// - E2E tests (Phase 7c): Full kubectl lifecycle in Kind cluster
//
// ========================================

func buildMinimalWorkflow(workflowName, actionType string, seq int) *models.RemediationWorkflow {
	content := fmt.Sprintf(`{"steps":[{"action":"test-%d"}]}`, seq)
	contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	createdBy := "integration-test"
	return &models.RemediationWorkflow{
		WorkflowName:    workflowName,
		Version:         "1.0.0",
		SchemaVersion:   "1.0",
		Name:            workflowName,
		Description:     models.StructuredDescription{What: "test workflow", WhenToUse: "during integration tests"},
		Content:         content,
		ContentHash:     contentHash,
		ActionType:      actionType,
		ExecutionEngine: models.ExecutionEngineJob,
		Labels:          models.MandatoryLabels{Severity: []string{"critical"}, Component: "pod"},
		Status:          "active",
		IsLatestVersion: true,
		CreatedBy:       &createdBy,
	}
}

var _ = Describe("ActionType Lifecycle Integration Tests (#300)", Label("integration", "actiontype"), func() {
	var (
		atRepo       *actiontyperepo.Repository
		workflowRepo *workflow.Repository
		testID       string
	)

	BeforeEach(func() {
		atRepo = actiontyperepo.NewRepository(db, logger)
		workflowRepo = workflow.NewRepository(db, logger)
		testID = generateTestID()

		processPrefix := fmt.Sprintf("AT-IT-%d-%%", GinkgoParallelProcess())
		_, err := db.ExecContext(ctx,
			"DELETE FROM action_type_taxonomy WHERE action_type LIKE $1",
			processPrefix)
		Expect(err).ToNot(HaveOccurred(), "Process-scoped cleanup should succeed")
	})

	AfterEach(func() {
		if db != nil {
			_, _ = db.ExecContext(ctx,
				"DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE $1",
				fmt.Sprintf("AT-IT-%s%%", testID))
			_, _ = db.ExecContext(ctx,
				"DELETE FROM action_type_taxonomy WHERE action_type LIKE $1",
				fmt.Sprintf("AT-IT-%s%%", testID))
		}
	})

	atName := func(suffix string) string {
		return fmt.Sprintf("AT-IT-%s-%s", testID, suffix)
	}

	baseDesc := func() models.ActionTypeDescription {
		return models.ActionTypeDescription{
			What:          "Kill and recreate one or more pods.",
			WhenToUse:     "Root cause is a transient runtime state issue.",
			Preconditions: "Evidence that the issue is transient.",
		}
	}

	// ========================================
	// IT-AT-300-001: Create action type in real PostgreSQL
	// BR-WORKFLOW-007.1
	// ========================================
	Describe("IT-AT-300-001: Create action type persisted in PostgreSQL", func() {
		It("should persist action type with description JSONB and status=active", func() {
			name := atName("create")
			desc := baseDesc()

			result, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Status).To(Equal("created"))
			Expect(result.WasReenabled).To(BeFalse())
			Expect(result.ActionType.ActionType).To(Equal(name),
				"Created result should contain the action type record")

			fetched, err := atRepo.GetByName(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetched.ActionType).To(Equal(name))
			Expect(fetched.Status).To(Equal("active"))

			var storedDesc models.ActionTypeDescription
			Expect(json.Unmarshal(fetched.Description, &storedDesc)).To(Succeed())
			Expect(storedDesc.What).To(Equal(desc.What))
			Expect(storedDesc.WhenToUse).To(Equal(desc.WhenToUse))
			Expect(storedDesc.Preconditions).To(Equal(desc.Preconditions))
		})
	})

	// ========================================
	// IT-AT-300-002: Idempotency matrix
	// BR-WORKFLOW-007.1: create/NOOP/re-enable
	// ========================================
	Describe("IT-AT-300-002: Idempotency matrix (new/active/disabled)", func() {
		It("should create on first call, NOOP on second, re-enable after disable", func() {
			name := atName("idempotent")
			desc := baseDesc()

			// First CREATE → "created"
			r1, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(r1.Status).To(Equal("created"))

			// Second CREATE → "exists" (NOOP)
			r2, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(r2.Status).To(Equal("exists"))
			Expect(r2.WasReenabled).To(BeFalse())

			// Manually disable
			disResult, err := atRepo.Disable(ctx, name, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(disResult.Disabled).To(BeTrue())

			fetched, err := atRepo.GetByName(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetched.Status).To(Equal("disabled"))

			// Third CREATE → "reenabled"
			r3, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(r3.Status).To(Equal("reenabled"))
			Expect(r3.WasReenabled).To(BeTrue())

			// Verify status restored to active
			reenabled, err := atRepo.GetByName(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			Expect(reenabled.Status).To(Equal("active"))
			Expect(reenabled.DisabledAt).To(BeNil(),
				"disabled_at should be NULL after re-enable")
			Expect(reenabled.DisabledBy).To(BeNil(),
				"disabled_by should be NULL after re-enable")
		})
	})

	// ========================================
	// IT-AT-300-003: Disable with dependency guard
	// BR-WORKFLOW-007.3
	// ========================================
	Describe("IT-AT-300-003: Disable denied when active workflows reference action type", func() {
		It("should deny disable with count and names of dependent workflows", func() {
			name := atName("dep-guard")
			desc := baseDesc()

			_, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			// Register workflow via repo (Issue #514: replaced raw SQL)
			wf := buildMinimalWorkflow(fmt.Sprintf("AT-IT-%s-wf-dep", testID), name, 1)
			Expect(workflowRepo.Create(ctx, wf)).To(Succeed(), "Registering workflow should succeed")

			// Disable should be denied
			disResult, err := atRepo.Disable(ctx, name, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(disResult.Disabled).To(BeFalse(),
				"Disable should be denied when active workflows exist")
			Expect(disResult.DependentWorkflowCount).To(Equal(1))
			Expect(disResult.DependentWorkflows).To(ContainElement(wf.WorkflowName))

			// Verify action type still active
			fetched, err := atRepo.GetByName(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetched.Status).To(Equal("active"))

			// Disable workflow via repo, then AT disable should succeed (Issue #514)
			created, err := workflowRepo.GetActiveByWorkflowName(ctx, wf.WorkflowName)
			Expect(err).ToNot(HaveOccurred())
			Expect(workflowRepo.UpdateStatus(ctx, created.WorkflowID, created.Version, "disabled", "CRD deleted", "admin@example.com")).To(Succeed())

			disResult2, err := atRepo.Disable(ctx, name, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(disResult2.Disabled).To(BeTrue(),
				"Disable should succeed after removing dependent workflows")
		})
	})

	// ========================================
	// IT-AT-300-004: Update description with old+new capture
	// BR-WORKFLOW-007.2
	// ========================================
	Describe("IT-AT-300-004: Update description captures old and new values", func() {
		It("should update description JSONB and return diff fields", func() {
			name := atName("update-desc")
			origDesc := baseDesc()

			_, err := atRepo.Create(ctx, name, origDesc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			newDesc := models.ActionTypeDescription{
				What:          "Gracefully restart pods with rolling strategy.",
				WhenToUse:     origDesc.WhenToUse,
				Preconditions: "Pod is in CrashLoopBackOff state.",
			}

			updateResult, err := atRepo.UpdateDescription(ctx, name, newDesc)
			Expect(err).ToNot(HaveOccurred())
			Expect(updateResult.OldDescription.What).To(Equal(origDesc.What))
			Expect(updateResult.NewDescription.What).To(Equal(newDesc.What))
			Expect(updateResult.UpdatedFields).To(ContainElement("what"))
			Expect(updateResult.UpdatedFields).To(ContainElement("preconditions"))
			Expect(updateResult.UpdatedFields).ToNot(ContainElement("whenToUse"),
				"whenToUse was unchanged so should not appear in diff")

			// Verify persisted
			fetched, err := atRepo.GetByName(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			var storedDesc models.ActionTypeDescription
			Expect(json.Unmarshal(fetched.Description, &storedDesc)).To(Succeed())
			Expect(storedDesc.What).To(Equal(newDesc.What))
			Expect(storedDesc.Preconditions).To(Equal(newDesc.Preconditions))
		})

		It("should return empty UpdatedFields when description is unchanged", func() {
			name := atName("update-noop")
			desc := baseDesc()

			_, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			updateResult, err := atRepo.UpdateDescription(ctx, name, desc)
			Expect(err).ToNot(HaveOccurred())
			Expect(updateResult.UpdatedFields).To(BeEmpty(),
				"No fields should be listed as updated for identical description")
		})

		It("should reject update on disabled action type", func() {
			name := atName("update-disabled")
			desc := baseDesc()

			_, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			_, err = atRepo.Disable(ctx, name, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			_, err = atRepo.UpdateDescription(ctx, name, models.ActionTypeDescription{
				What:      "New value",
				WhenToUse: "New value",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("disabled"))
		})

		It("should reject update on non-existent action type", func() {
			_, err := atRepo.UpdateDescription(ctx, "NonExistent", models.ActionTypeDescription{
				What:      "anything",
				WhenToUse: "anything",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	// ========================================
	// IT-AT-300-005: Discovery filtering — disabled excluded
	// BR-WORKFLOW-007.5 (cross-cutting)
	// ========================================
	Describe("IT-AT-300-005: Disabled action types excluded from discovery", func() {
		It("should return only active action types, excluding disabled ones", func() {
			nameA := atName("disc-A")
			nameB := atName("disc-B")
			nameC := atName("disc-C")
			desc := baseDesc()

			_, err := atRepo.Create(ctx, nameA, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			_, err = atRepo.Create(ctx, nameB, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			_, err = atRepo.Create(ctx, nameC, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			// Disable B
			_, err = atRepo.Disable(ctx, nameB, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			activeTypes, err := atRepo.ListActive(ctx)
			Expect(err).ToNot(HaveOccurred())

			activeNames := make([]string, 0, len(activeTypes))
			for _, at := range activeTypes {
				activeNames = append(activeNames, at.ActionType)
			}
			Expect(activeNames).To(ContainElement(nameA),
				"Active action type A should appear in discovery list")
			Expect(activeNames).ToNot(ContainElement(nameB),
				"Disabled action type B should NOT appear in discovery list")
			Expect(activeNames).To(ContainElement(nameC),
				"Active action type C should appear in discovery list")
		})
	})

	// ========================================
	// Disable edge cases
	// ========================================
	Describe("Disable edge cases", func() {
		It("should reject disable on non-existent action type", func() {
			_, err := atRepo.Disable(ctx, "NonExistent", "admin@example.com")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should be idempotent when disabling an already-disabled action type", func() {
			name := atName("double-disable")
			desc := baseDesc()

			_, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			result, err := atRepo.Disable(ctx, name, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Disabled).To(BeTrue())

			result, err = atRepo.Disable(ctx, name, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Disabled).To(BeTrue(), "Idempotent disable should succeed without error")
		})

		It("should set disabled_at and disabled_by when disabling", func() {
			name := atName("disable-audit")
			desc := baseDesc()

			_, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			_, err = atRepo.Disable(ctx, name, "operator@kubernaut.ai")
			Expect(err).ToNot(HaveOccurred())

			fetched, err := atRepo.GetByName(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetched.Status).To(Equal("disabled"))
			Expect(fetched.DisabledAt).ToNot(BeNil(),
				"disabled_at should be set")
			Expect(fetched.DisabledBy).To(HaveValue(Equal("operator@kubernaut.ai")),
				"disabled_by should record who performed the disable")
		})
	})

	// ========================================
	// Workflow count integration
	// ========================================
	Describe("CountActiveWorkflows with real data", func() {
		It("should count only active workflows for the specified action type", func() {
			name := atName("wf-count")
			desc := baseDesc()

			_, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			for i := 1; i <= 2; i++ {
				wf := buildMinimalWorkflow(fmt.Sprintf("AT-IT-%s-wf-count-%d", testID, i), name, i)
				Expect(workflowRepo.Create(ctx, wf)).To(Succeed())
			}

			count, names, err := atRepo.CountActiveWorkflows(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(2))
			Expect(names).To(HaveLen(2))
		})
	})

	// ========================================
	// IT-AT-512-001: ForceDisable with orphaned workflows
	// Issue #512
	// ========================================
	Describe("IT-AT-512-001: ForceDisable cleans orphaned workflows and disables action type", func() {
		It("should disable named orphaned workflows and then disable the action type", func() {
			name := atName("force-disable")
			desc := baseDesc()

			_, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			orphanWf := buildMinimalWorkflow(fmt.Sprintf("AT-IT-%s-orphan", testID), name, 1)
			Expect(workflowRepo.Create(ctx, orphanWf)).To(Succeed())

			// Normal disable should be denied
			disResult, err := atRepo.Disable(ctx, name, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(disResult.Disabled).To(BeFalse())
			Expect(disResult.DependentWorkflowCount).To(Equal(1))

			// ForceDisable with the orphan's name should succeed
			forceResult, err := atRepo.ForceDisable(ctx, name, "admin@example.com", []string{orphanWf.WorkflowName})
			Expect(err).ToNot(HaveOccurred())
			Expect(forceResult.Disabled).To(BeTrue(),
				"ForceDisable should succeed when all dependents are named as orphans")

			// Verify the action type is disabled
			fetched, err := atRepo.GetByName(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetched.Status).To(Equal("disabled"))
		})

		It("should deny when non-orphaned workflows remain", func() {
			name := atName("force-partial")
			desc := baseDesc()

			_, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			orphanWf := buildMinimalWorkflow(fmt.Sprintf("AT-IT-%s-fp-orphan", testID), name, 1)
			Expect(workflowRepo.Create(ctx, orphanWf)).To(Succeed())

			liveWf := buildMinimalWorkflow(fmt.Sprintf("AT-IT-%s-fp-live", testID), name, 2)
			Expect(workflowRepo.Create(ctx, liveWf)).To(Succeed())

			// ForceDisable naming only the orphan should still deny (live remains)
			forceResult, err := atRepo.ForceDisable(ctx, name, "admin@example.com", []string{orphanWf.WorkflowName})
			Expect(err).ToNot(HaveOccurred())
			Expect(forceResult.Disabled).To(BeFalse(),
				"ForceDisable should deny when non-orphaned workflows remain")
			Expect(forceResult.DependentWorkflowCount).To(Equal(1))
			Expect(forceResult.DependentWorkflows).To(ConsistOf(liveWf.WorkflowName))

			// Verify the action type is still active
			fetched, err := atRepo.GetByName(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetched.Status).To(Equal("active"))
		})
	})

	// ========================================
	// IT-AT-512-002: Full lifecycle via repo methods
	// Issue #512, #514: Replaces raw SQL with proper repo calls
	// ========================================
	Describe("IT-AT-512-002: Full AT+workflow lifecycle via repo methods", func() {
		It("should create AT, register workflow, disable workflow, then disable AT", func() {
			name := atName("lifecycle")
			desc := baseDesc()

			By("Creating action type")
			_, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			By("Registering workflow via workflowRepo.Create")
			wf := buildMinimalWorkflow(fmt.Sprintf("AT-IT-%s-lifecycle-wf", testID), name, 1)
			Expect(workflowRepo.Create(ctx, wf)).To(Succeed())

			By("Verifying AT disable is denied with active workflow")
			disResult, err := atRepo.Disable(ctx, name, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(disResult.Disabled).To(BeFalse())

			By("Disabling workflow via workflowRepo.UpdateStatus")
			created, err := workflowRepo.GetActiveByWorkflowName(ctx, wf.WorkflowName)
			Expect(err).ToNot(HaveOccurred())
			Expect(workflowRepo.UpdateStatus(ctx, created.WorkflowID, created.Version, "disabled", "CRD deleted", "admin@example.com")).To(Succeed())

			By("Verifying AT disable now succeeds")
			disResult, err = atRepo.Disable(ctx, name, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(disResult.Disabled).To(BeTrue(),
				"AT disable should succeed after all workflows are disabled")
		})
	})
})
