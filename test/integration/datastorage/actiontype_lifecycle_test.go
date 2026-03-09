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

			// Insert a fake active workflow referencing this action type
			wfName := fmt.Sprintf("AT-IT-%s-wf-dep", testID)
			content := `{"steps":[{"action":"test"}]}`
			contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
			labelsJSON, _ := json.Marshal(map[string]interface{}{
				"severity": []string{"critical"}, "component": "pod",
				"priority": "P0", "environment": []string{"production"},
			})
			descJSON, _ := json.Marshal(map[string]string{
				"what": "test workflow", "whenToUse": "during tests",
			})

			_, err = db.ExecContext(ctx,
				`INSERT INTO remediation_workflow_catalog
				 (workflow_name, version, name, is_latest_version, status, content, content_hash,
				  action_type, labels, description)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
				wfName, "1.0.0", wfName, true, "active", content, contentHash,
				name, labelsJSON, descJSON)
			Expect(err).ToNot(HaveOccurred(), "Inserting fake workflow should succeed")

			// Disable should be denied
			disResult, err := atRepo.Disable(ctx, name, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())
			Expect(disResult.Disabled).To(BeFalse(),
				"Disable should be denied when active workflows exist")
			Expect(disResult.DependentWorkflowCount).To(Equal(1))
			Expect(disResult.DependentWorkflows).To(ContainElement(wfName))

			// Verify action type still active
			fetched, err := atRepo.GetByName(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetched.Status).To(Equal("active"))

			// Remove workflow, then disable should succeed
			_, err = db.ExecContext(ctx,
				"DELETE FROM remediation_workflow_catalog WHERE workflow_name = $1", wfName)
			Expect(err).ToNot(HaveOccurred())

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
		It("should not count disabled action type as having active workflows", func() {
			name := atName("discovery")
			desc := baseDesc()

			_, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			// No workflows referencing it
			count, names, err := atRepo.CountActiveWorkflows(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(0))
			Expect(names).To(BeEmpty())
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

		It("should reject disable on already-disabled action type", func() {
			name := atName("double-disable")
			desc := baseDesc()

			_, err := atRepo.Create(ctx, name, desc, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			_, err = atRepo.Disable(ctx, name, "admin@example.com")
			Expect(err).ToNot(HaveOccurred())

			_, err = atRepo.Disable(ctx, name, "admin@example.com")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("disabled"))
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

			// Insert 2 active workflows
			for i := 1; i <= 2; i++ {
				wfName := fmt.Sprintf("AT-IT-%s-wf-count-%d", testID, i)
				content := fmt.Sprintf(`{"steps":[{"action":"test-%d"}]}`, i)
				contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
				labelsJSON, _ := json.Marshal(map[string]interface{}{
					"severity": []string{"critical"}, "component": "pod",
				})
				descJSON, _ := json.Marshal(map[string]string{
					"what": "test", "whenToUse": "test",
				})

				_, insertErr := db.ExecContext(ctx,
					`INSERT INTO remediation_workflow_catalog
					 (workflow_name, version, name, is_latest_version, status, content, content_hash,
					  action_type, labels, description)
					 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
					wfName, "1.0.0", wfName, true, "active", content, contentHash,
					name, labelsJSON, descJSON)
				Expect(insertErr).ToNot(HaveOccurred())
			}

			count, names, err := atRepo.CountActiveWorkflows(ctx, name)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(2))
			Expect(names).To(HaveLen(2))

			_ = workflowRepo
		})
	})
})
