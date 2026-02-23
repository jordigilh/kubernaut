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
	"fmt"
	"time"

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
// - IT-DS-043-001 through IT-DS-043-007
// - JSONB round-trip fidelity for DetectedLabels
// - Workflow search/discovery filtering by DetectedLabels
// - Version update with changed DetectedLabels
//
// Uses REAL PostgreSQL database (not mocks) per no-mocks policy.
//
// ========================================

var _ = Describe("Workflow DetectedLabels Integration (ADR-043 v1.3)", func() {
	var (
		workflowRepo *workflow.Repository
		testID       string
	)

	BeforeEach(func() {
		workflowRepo = workflow.NewRepository(db, logger)
		testID = generateTestID()
	})

	AfterEach(func() {
		if db != nil {
			_, _ = db.ExecContext(ctx,
				"DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE $1",
				fmt.Sprintf("wf-dl-%s%%", testID))
		}
	})

	baseWorkflow := func(name string, dl models.DetectedLabels) *models.RemediationWorkflow {
		content := `{"steps":[{"action":"remediate"}]}`
		contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
		return &models.RemediationWorkflow{
			WorkflowName: fmt.Sprintf("wf-dl-%s-%s", testID, name),
			ActionType:   "RestartPod",
			Version:      "v1.0",
			Name:         name,
			Description:  models.StructuredDescription{What: "Test workflow", WhenToUse: "Testing"},
			Content:      content,
			ContentHash:  contentHash,
			Labels: models.MandatoryLabels{
				SignalName:  "CrashLoopBackOff",
				Severity:    []string{"critical"},
				Component:   "pod",
				Environment: []string{"production"},
				Priority:    "P0",
			},
			CustomLabels:    models.CustomLabels{},
			DetectedLabels:  dl,
			Status:          "active",
			ExecutionEngine: "tekton",
			IsLatestVersion: true,
		}
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
			wf := baseWorkflow("all-fields", dl)

			err := workflowRepo.Create(ctx, wf)
			Expect(err).ToNot(HaveOccurred(), "workflow with detectedLabels should be created")

			retrieved, err := workflowRepo.GetLatestVersion(ctx, wf.WorkflowName)
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
			wf := baseWorkflow("partial-fields", dl)

			err := workflowRepo.Create(ctx, wf)
			Expect(err).ToNot(HaveOccurred())

			retrieved, err := workflowRepo.GetLatestVersion(ctx, wf.WorkflowName)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved.DetectedLabels.HPAEnabled).To(BeTrue())
			Expect(retrieved.DetectedLabels.GitOpsTool).To(Equal("*"))
			Expect(retrieved.DetectedLabels.PDBProtected).To(BeFalse(),
				"unset boolean should be false after JSONB round-trip")
			Expect(retrieved.DetectedLabels.ServiceMesh).To(BeEmpty(),
				"unset string should be empty after JSONB round-trip")
		})

		It("IT-DS-043-003: workflow without detectedLabels has empty DetectedLabels in catalog", func() {
			wf := baseWorkflow("no-detected", models.DetectedLabels{})

			err := workflowRepo.Create(ctx, wf)
			Expect(err).ToNot(HaveOccurred())

			retrieved, err := workflowRepo.GetLatestVersion(ctx, wf.WorkflowName)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved.DetectedLabels.IsEmpty()).To(BeTrue(),
				"empty DetectedLabels should round-trip as empty, not null")
		})
	})

	Describe("Workflow Discovery by DetectedLabels", func() {
		It("IT-DS-043-005: search filters correctly by detectedLabels", func() {
			hpaWorkflow := baseWorkflow("with-hpa", models.DetectedLabels{HPAEnabled: true})
			plainWorkflow := baseWorkflow("no-hpa", models.DetectedLabels{})

			err := workflowRepo.Create(ctx, hpaWorkflow)
			Expect(err).ToNot(HaveOccurred())
			err = workflowRepo.Create(ctx, plainWorkflow)
			Expect(err).ToNot(HaveOccurred())

			searchRequest := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalName:  "CrashLoopBackOff",
					Severity:    "critical",
					Component:   "pod",
					Environment: "production",
					Priority:    "P0",
					DetectedLabels: models.DetectedLabels{
						HPAEnabled: true,
					},
				},
				TopK: 10,
			}

			var response *models.WorkflowSearchResponse
			Eventually(func() bool {
				var searchErr error
				response, searchErr = workflowRepo.SearchByLabels(ctx, searchRequest)
				return searchErr == nil && len(response.Workflows) > 0
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"search should return results")

			var hpaFound bool
			for _, r := range response.Workflows {
				if r.Title == hpaWorkflow.Name {
					hpaFound = true
				}
			}
			Expect(hpaFound).To(BeTrue(),
				"workflow with matching detectedLabels should appear in search results")
		})
	})

	Describe("Full Schema Round-Trip", func() {
		It("IT-DS-043-006: all fields preserved alongside detectedLabels", func() {
			dl := models.DetectedLabels{
				PDBProtected: true,
				GitOpsTool:   "flux",
			}
			wf := baseWorkflow("full-roundtrip", dl)
			wf.Labels.SignalName = "NodeNotReady"
			wf.Labels.Severity = []string{"critical", "high"}
			wf.CustomLabels = models.CustomLabels{
				"team": []string{"platform"},
			}

			err := workflowRepo.Create(ctx, wf)
			Expect(err).ToNot(HaveOccurred())

			retrieved, err := workflowRepo.GetLatestVersion(ctx, wf.WorkflowName)
			Expect(err).ToNot(HaveOccurred())

			Expect(retrieved.Labels.SignalName).To(Equal("NodeNotReady"))
			Expect(retrieved.Labels.Severity).To(ConsistOf("critical", "high"))
			Expect(retrieved.CustomLabels).To(HaveKey("team"))
			Expect(retrieved.DetectedLabels.PDBProtected).To(BeTrue())
			Expect(retrieved.DetectedLabels.GitOpsTool).To(Equal("flux"))
			Expect(retrieved.Description.What).To(Equal("Test workflow"))
		})
	})

	Describe("Version Update", func() {
		It("IT-DS-043-007: version update with changed detectedLabels stores new values", func() {
			v1 := baseWorkflow("versioned", models.DetectedLabels{HPAEnabled: true})
			err := workflowRepo.Create(ctx, v1)
			Expect(err).ToNot(HaveOccurred())

			v2 := baseWorkflow("versioned", models.DetectedLabels{
				HPAEnabled:   true,
				PDBProtected: true,
				GitOpsTool:   "argocd",
			})
			v2.Version = "v2.0"
			err = workflowRepo.Create(ctx, v2)
			Expect(err).ToNot(HaveOccurred())

			retrieved, err := workflowRepo.GetLatestVersion(ctx, v2.WorkflowName)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved.Version).To(Equal("v2.0"))
			Expect(retrieved.DetectedLabels.HPAEnabled).To(BeTrue())
			Expect(retrieved.DetectedLabels.PDBProtected).To(BeTrue(),
				"v2 detectedLabels should include PDBProtected")
			Expect(retrieved.DetectedLabels.GitOpsTool).To(Equal("argocd"),
				"v2 detectedLabels should include GitOpsTool")
		})
	})
})
