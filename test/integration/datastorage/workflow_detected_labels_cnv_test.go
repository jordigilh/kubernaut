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
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// ========================================
// CNV DETECTED LABELS INTEGRATION TESTS (#1378)
// ========================================
//
// Authority: BR-WORKFLOW-018 (CNV workload detection)
// Authority: BR-WORKFLOW-004 (Workflow schema + discovery filters)
// Test Plan: docs/tests/1378/TEST_PLAN.md
//
// Test IDs:
// - IT-DS-1378-001: ListWorkflowsByActionType with virtualMachine=true
// - IT-DS-1378-002: ListWorkflowsByActionType with storageBackend=odf-ceph
// - IT-DS-1378-003: CNV fixture roundtrip + discovery query
//
// Uses REAL PostgreSQL database (not mocks) per no-mocks policy.
//
// ========================================

var _ = Describe("CNV DetectedLabels Integration (#1378)", Label("it", "ds", "cnv", "1378", "detected-labels"), func() {
	var (
		workflowRepo  *workflow.Repository
		schemaParser  *schema.Parser
		testID        string
		cnvActionType string
	)

	BeforeEach(func() {
		workflowRepo = workflow.NewRepository(db, logger)
		schemaParser = schema.NewParser()
		testID = generateTestID()
		cnvActionType = fmt.Sprintf("RestartPod-cnv-%s", testID)
	})

	AfterEach(func() {
		if db != nil {
			_, _ = db.ExecContext(ctx,
				"DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE $1",
				fmt.Sprintf("wf-cnv-%s%%", testID))
		}
	})

	cnvMandatoryLabels := models.MandatoryLabels{
		Severity:    []string{"critical", "high"},
		Component:   []string{"kubevirt.io/v1/VirtualMachine"},
		Environment: []string{"production", "staging"},
		Priority:    "P1",
	}

	cnvDiscoveryContext := func(dl *models.DetectedLabels) *models.WorkflowDiscoveryFilters {
		return &models.WorkflowDiscoveryFilters{
			Severity:       "critical",
			Component:      "kubevirt.io/v1/VirtualMachine",
			Environment:    "production",
			Priority:       "P1",
			DetectedLabels: dl,
		}
	}

	createCNVWorkflow := func(name, actionType string, labels models.MandatoryLabels, dl models.DetectedLabels) *models.RemediationWorkflow {
		content := fmt.Sprintf(`{"steps":[{"action":"%s","name":"%s"}]}`, actionType, name)
		contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

		wf := &models.RemediationWorkflow{
			WorkflowName: fmt.Sprintf("wf-cnv-%s-%s", testID, name),
			ActionType:   actionType,
			Version:      "v1.0",
			SchemaVersion: "1.0",
			Name:         name,
			Description: models.StructuredDescription{
				What:      fmt.Sprintf("CNV test workflow %s", name),
				WhenToUse: "CNV detectedLabels integration testing",
			},
			Content:         content,
			ContentHash:     contentHash,
			Labels:          labels,
			CustomLabels:    models.CustomLabels{},
			DetectedLabels:  dl,
			Status:          "Active",
			ExecutionEngine: "tekton",
			IsLatestVersion: true,
		}

		err := workflowRepo.Create(ctx, wf)
		Expect(err).ToNot(HaveOccurred(), "workflow %s should be created", name)
		Expect(wf.WorkflowID).ToNot(BeEmpty())

		return wf
	}

	buildWorkflowFromParsedSchema := func(parsed *models.WorkflowSchema, rawContent, workflowName string) *models.RemediationWorkflow {
		labelsJSON, err := schemaParser.ExtractLabels(parsed)
		Expect(err).ToNot(HaveOccurred())

		detectedLabels, err := schemaParser.ExtractDetectedLabels(parsed)
		Expect(err).ToNot(HaveOccurred())

		wf := &models.RemediationWorkflow{
			WorkflowName:    workflowName,
			Version:         parsed.Version,
			SchemaVersion:   parsed.SchemaVersion,
			Name:            parsed.WorkflowName,
			Description: models.StructuredDescription{
				What:          parsed.Description.What,
				WhenToUse:     parsed.Description.WhenToUse,
				WhenNotToUse:  parsed.Description.WhenNotToUse,
				Preconditions: parsed.Description.Preconditions,
			},
			Content:         rawContent,
			ContentHash:     fmt.Sprintf("%x", sha256.Sum256([]byte(rawContent))),
			ActionType:      parsed.ActionType,
			Status:          "Active",
			IsLatestVersion: true,
			ExecutionEngine: models.ExecutionEngine(schemaParser.ExtractExecutionEngine(parsed)),
			DetectedLabels:  *detectedLabels,
		}

		err = json.Unmarshal(labelsJSON, &wf.Labels)
		Expect(err).ToNot(HaveOccurred())

		return wf
	}

	Describe("ListWorkflowsByActionType — CNV filters", func() {
		It("IT-DS-1378-001: virtualMachine=true returns VM workflows and ranks CNV matches first [BR-WORKFLOW-004]", func() {
			fullCNV := models.DetectedLabels{
				VirtualMachine: true,
				LiveMigratable: true,
				CDIManaged:     true,
				StorageBackend: "odf-ceph",
			}
			cnvWF := createCNVWorkflow("cnv-full-match", cnvActionType, cnvMandatoryLabels, fullCNV)
			createCNVWorkflow("generic-kubevirt", cnvActionType, cnvMandatoryLabels, models.DetectedLabels{})

			podLabels := models.MandatoryLabels{
				Severity:    []string{"critical"},
				Component:   []string{"v1/Pod"},
				Environment: []string{"production"},
				Priority:    "P1",
			}
			createCNVWorkflow("pod-non-vm", cnvActionType, podLabels, models.DetectedLabels{})

			filters := cnvDiscoveryContext(&models.DetectedLabels{VirtualMachine: true})
			results, totalCount, err := workflowRepo.ListWorkflowsByActionType(ctx, cnvActionType, filters, 0, 10)

			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(2), "only kubevirt workflows should match VM discovery context")
			Expect(results).To(HaveLen(2))
			Expect(results[0].WorkflowName).To(Equal(cnvWF.WorkflowName),
				"CNV workflow with virtualMachine=true should rank first due to detected label boost")
			Expect(results[0].DetectedLabels.VirtualMachine).To(BeTrue())
			Expect(results[1].DetectedLabels.IsEmpty()).To(BeTrue(),
				"generic kubevirt workflow should remain discoverable without CNV requirements")
		})

		It("IT-DS-1378-002: storageBackend=odf-ceph matches exact and wildcard with correct ranking [BR-WORKFLOW-004]", func() {
			exactWF := createCNVWorkflow("exact-odf-ceph", cnvActionType, cnvMandatoryLabels, models.DetectedLabels{
				VirtualMachine: true,
				StorageBackend: "odf-ceph",
			})
			createCNVWorkflow("wildcard-storage", cnvActionType, cnvMandatoryLabels, models.DetectedLabels{
				VirtualMachine: true,
				StorageBackend: "*",
			})
			createCNVWorkflow("lvms-storage", cnvActionType, cnvMandatoryLabels, models.DetectedLabels{
				VirtualMachine: true,
				StorageBackend: "lvms",
			})

			filters := cnvDiscoveryContext(&models.DetectedLabels{StorageBackend: "odf-ceph"})
			results, totalCount, err := workflowRepo.ListWorkflowsByActionType(ctx, cnvActionType, filters, 0, 10)

			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(2), "exact and wildcard storageBackend workflows should match odf-ceph filter")
			Expect(results).To(HaveLen(2))
			Expect(results[0].WorkflowName).To(Equal(exactWF.WorkflowName),
				"exact storageBackend=odf-ceph workflow should outrank wildcard workflow")
			Expect(results[0].DetectedLabels.StorageBackend).To(Equal("odf-ceph"))
			Expect(results[1].DetectedLabels.StorageBackend).To(Equal("*"))
		})
	})

	Describe("CNV fixture roundtrip and discovery", func() {
		It("IT-DS-1378-003: fixture parse → serialize → extract → query preserves all 4 CNV fields [BR-WORKFLOW-004]", func() {
			rawFixture := testutil.LoadWorkflowFixture("cnv-vm-boot-failure")

			parsedSchema, err := schemaParser.ParseAndValidate(rawFixture)
			Expect(err).ToNot(HaveOccurred(), "CNV fixture should pass schema validation")

			extracted, err := schemaParser.ExtractDetectedLabels(parsedSchema)
			Expect(err).ToNot(HaveOccurred())
			Expect(extracted.VirtualMachine).To(BeTrue())
			Expect(extracted.LiveMigratable).To(BeTrue())
			Expect(extracted.CDIManaged).To(BeTrue())
			Expect(extracted.StorageBackend).To(Equal("odf-ceph"))

			serialized, err := extracted.SerializeLabels()
			Expect(err).ToNot(HaveOccurred())

			var roundtripped models.DetectedLabels
			err = json.Unmarshal(serialized, &roundtripped)
			Expect(err).ToNot(HaveOccurred())
			Expect(roundtripped.VirtualMachine).To(Equal(extracted.VirtualMachine))
			Expect(roundtripped.LiveMigratable).To(Equal(extracted.LiveMigratable))
			Expect(roundtripped.CDIManaged).To(Equal(extracted.CDIManaged))
			Expect(roundtripped.StorageBackend).To(Equal(extracted.StorageBackend))

			workflowName := fmt.Sprintf("wf-cnv-%s-fixture", testID)
			wf := buildWorkflowFromParsedSchema(parsedSchema, rawFixture, workflowName)
			err = workflowRepo.Create(ctx, wf)
			Expect(err).ToNot(HaveOccurred())

			filters := cnvDiscoveryContext(&models.DetectedLabels{
				VirtualMachine: true,
				LiveMigratable: true,
				CDIManaged:     true,
				StorageBackend: "odf-ceph",
			})
			results, totalCount, err := workflowRepo.ListWorkflowsByActionType(ctx, parsedSchema.ActionType, filters, 0, 10)

			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(1))
			Expect(results).To(HaveLen(1))
			Expect(results[0].WorkflowName).To(Equal(workflowName))
			Expect(results[0].DetectedLabels.VirtualMachine).To(BeTrue())
			Expect(results[0].DetectedLabels.LiveMigratable).To(BeTrue())
			Expect(results[0].DetectedLabels.CDIManaged).To(BeTrue())
			Expect(results[0].DetectedLabels.StorageBackend).To(Equal("odf-ceph"))
		})
	})
})
