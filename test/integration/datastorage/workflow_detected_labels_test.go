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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
)

// ========================================
// WORKFLOW DETECTED LABELS INTEGRATION TESTS (ADR-043 v1.3)
// ========================================
//
// Authority: ADR-043 v1.3 (detectedLabels schema field)
// Authority: DD-WORKFLOW-001 v2.0 (DetectedLabels architecture)
// Test Plan: docs/testing/ADR-043/TEST_PLAN.md
//
// Tests cover:
// - IT-DS-043-001 through IT-DS-043-003, IT-DS-043-006
// - JSONB round-trip fidelity for DetectedLabels, via the cache-backed
//   discovery path (RemediationWorkflow CRD -> workflowcache.Cache ->
//   crdDetectedLabelsToModel).
//
// #1661 Phase F: migrated off workflowRepo.Create (Postgres, zero production
// callers post-Phase-B) to seedWorkflowCRD -- DD-WORKFLOW-018 (etcd sole
// source of truth). Retrieval swapped from the dying GetLatestVersion (no
// cache equivalent) to GetByID.
//
// IT-DS-043-007 ("version update with changed detectedLabels stores new
// values") was removed: it exercised two coexisting versions of the same
// workflow_name superseding one another, a concept that no longer exists
// once metadata.name is the workflow's sole identity (DD-WORKFLOW-018) --
// there is no "version" dimension left to update independently of the CRD
// itself.
// ========================================

var _ = Describe("Workflow DetectedLabels Integration (ADR-043 v1.3)", func() {
	var (
		workflowRepo *workflow.Repository
		testID       string
	)

	BeforeEach(func() {
		workflowRepo = newCachedWorkflowRepo()
		testID = generateTestID()
	})

	seedWorkflow := func(name string, dl models.DetectedLabels) string {
		return seedWorkflowCRD(workflowCRDSpec{
			Name:           testID + "-" + name,
			ActionType:     "RestartPod",
			Priority:       "P0",
			DetectedLabels: &dl,
		})
	}

	Describe("JSONB Round-Trip Fidelity", func() {
		It("IT-DS-043-001: workflow with detectedLabels is stored accurately in catalog", func() {
			dl := models.DetectedLabels{
				GitOpsManaged:   true,
				GitOpsTool:      "argocd",
				PDBProtected:    true,
				HPAEnabled:      true,
				Stateful:        true,
				HelmManaged:     true,
				NetworkIsolated: true,
				ServiceMesh:     "istio",
			}
			workflowID := seedWorkflow("all-fields", dl)

			retrieved, err := workflowRepo.GetByID(ctx, workflowID)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved.DetectedLabels.GitOpsManaged).To(BeTrue())
			Expect(retrieved.DetectedLabels.GitOpsTool).To(Equal("argocd"))
			Expect(retrieved.DetectedLabels.PDBProtected).To(BeTrue())
			Expect(retrieved.DetectedLabels.HPAEnabled).To(BeTrue())
			Expect(retrieved.DetectedLabels.Stateful).To(BeTrue())
			Expect(retrieved.DetectedLabels.HelmManaged).To(BeTrue())
			Expect(retrieved.DetectedLabels.NetworkIsolated).To(BeTrue())
			Expect(retrieved.DetectedLabels.ServiceMesh).To(Equal("istio"))
		})

		It("IT-DS-043-002: workflow retrieved returns exact detectedLabels registered", func() {
			dl := models.DetectedLabels{
				HPAEnabled: true,
				GitOpsTool: "*",
			}
			workflowID := seedWorkflow("partial-fields", dl)

			retrieved, err := workflowRepo.GetByID(ctx, workflowID)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved.DetectedLabels.HPAEnabled).To(BeTrue())
			Expect(retrieved.DetectedLabels.GitOpsTool).To(Equal("*"))
			Expect(retrieved.DetectedLabels.PDBProtected).To(BeFalse(),
				"unset boolean should be false after round-trip")
			Expect(retrieved.DetectedLabels.ServiceMesh).To(BeEmpty(),
				"unset string should be empty after round-trip")
		})

		It("IT-DS-043-003: workflow without detectedLabels has empty DetectedLabels in catalog", func() {
			workflowID := seedWorkflow("no-detected", models.DetectedLabels{})

			retrieved, err := workflowRepo.GetByID(ctx, workflowID)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved.DetectedLabels.IsEmpty()).To(BeTrue(),
				"empty DetectedLabels should round-trip as empty, not null")
		})
	})

	Describe("Full Schema Round-Trip", func() {
		It("IT-DS-043-006: all fields preserved alongside detectedLabels", func() {
			dl := models.DetectedLabels{
				PDBProtected: true,
				GitOpsTool:   "flux",
			}
			workflowID := seedWorkflowCRD(workflowCRDSpec{
				Name:           testID + "-full-roundtrip",
				ActionType:     "RestartPod",
				Priority:       "P0",
				Severity:       []string{"critical", "high"},
				CustomLabels:   map[string]string{"team": "platform"},
				DetectedLabels: &dl,
			})

			retrieved, err := workflowRepo.GetByID(ctx, workflowID)
			Expect(err).ToNot(HaveOccurred())

			Expect(retrieved.Labels.Severity).To(ConsistOf("critical", "high"))
			Expect(retrieved.CustomLabels).To(HaveKey("team"))
			Expect(retrieved.DetectedLabels.PDBProtected).To(BeTrue())
			Expect(retrieved.DetectedLabels.GitOpsTool).To(Equal("flux"))
			Expect(retrieved.Description.What).To(ContainSubstring(testID + "-full-roundtrip"))
		})
	})
})
