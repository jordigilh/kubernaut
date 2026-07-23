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
	"encoding/json"
	"fmt"
	"strings"

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
// #1661 Phase F: migrated off workflowRepo.Create (Postgres, zero production
// callers post-Phase-B) to seedWorkflowCRD -- DD-WORKFLOW-018 (etcd sole
// source of truth). Discovery queries now exercise the cache-backed
// ListWorkflowsByActionType path (pkg/datastorage/repository/workflow/discovery_cache.go).
// ========================================

var _ = Describe("CNV DetectedLabels Integration (#1378)", Label("it", "ds", "cnv", "1378", "detected-labels"), func() {
	var (
		workflowRepo *workflow.Repository
		schemaParser *schema.Parser
		testID       string
	)

	BeforeEach(func() {
		workflowRepo = newCachedWorkflowRepo()
		schemaParser = schema.NewParser()
		testID = generateTestID()
	})

	cnvDiscoveryContext := func(dl *models.DetectedLabels) *models.WorkflowDiscoveryFilters {
		return &models.WorkflowDiscoveryFilters{
			Severity:       "critical",
			Component:      "kubevirt.io/v1/VirtualMachine",
			Environment:    "production",
			Priority:       "P1",
			DetectedLabels: dl,
		}
	}

	createCNVWorkflow := func(name, actionType string, spec workflowCRDSpec, dl models.DetectedLabels) string {
		spec.Name = fmt.Sprintf("wf-cnv-%s-%s", testID, name)
		spec.ActionType = actionType
		spec.DetectedLabels = &dl
		return seedWorkflowCRD(spec)
	}

	cnvSpec := func() workflowCRDSpec {
		return workflowCRDSpec{
			Severity:    []string{"critical", "high"},
			Component:   []string{"kubevirt.io/v1/VirtualMachine"},
			Environment: []string{"production", "staging"},
			Priority:    "P1",
		}
	}

	podSpec := func() workflowCRDSpec {
		return workflowCRDSpec{
			Severity:    []string{"critical"},
			Component:   []string{"v1/Pod"},
			Environment: []string{"production"},
			Priority:    "P1",
		}
	}

	filterOurs := func(results []models.RemediationWorkflow, prefix string) []models.RemediationWorkflow {
		var filtered []models.RemediationWorkflow
		for _, r := range results {
			if strings.HasPrefix(r.WorkflowName, prefix) {
				filtered = append(filtered, r)
			}
		}
		return filtered
	}

	Describe("ListWorkflowsByActionType — CNV filters", func() {
		It("IT-DS-1378-001: virtualMachine=true returns VM workflows and ranks CNV matches first [BR-WORKFLOW-004]", func() {
			prefix := fmt.Sprintf("wf-cnv-%s-", testID)
			fullCNV := models.DetectedLabels{
				VirtualMachine: true,
				LiveMigratable: true,
				CDIManaged:     true,
				StorageBackend: "odf-ceph",
			}
			cnvWorkflowName := fmt.Sprintf("wf-cnv-%s-cnv-full-match", testID)
			createCNVWorkflow("cnv-full-match", "RestartPod", cnvSpec(), fullCNV)
			createCNVWorkflow("generic-kubevirt", "RestartPod", cnvSpec(), models.DetectedLabels{})
			createCNVWorkflow("pod-non-vm", "RestartPod", podSpec(), models.DetectedLabels{})

			filters := cnvDiscoveryContext(&models.DetectedLabels{VirtualMachine: true})
			results, _, err := workflowRepo.ListWorkflowsByActionType(ctx, "RestartPod", filters, 0, 100)

			Expect(err).ToNot(HaveOccurred())
			ours := filterOurs(results, prefix)
			Expect(ours).To(HaveLen(2), "only 2 of our kubevirt workflows should match VM discovery context")
			Expect(ours[0].WorkflowName).To(Equal(cnvWorkflowName),
				"CNV workflow with virtualMachine=true should rank first due to detected label boost")
			Expect(ours[0].DetectedLabels.VirtualMachine).To(BeTrue())
			Expect(ours[1].DetectedLabels.IsEmpty()).To(BeTrue(),
				"generic kubevirt workflow should remain discoverable without CNV requirements")
		})

		It("IT-DS-1378-002: storageBackend=odf-ceph matches exact and wildcard with correct ranking [BR-WORKFLOW-004]", func() {
			prefix := fmt.Sprintf("wf-cnv-%s-", testID)
			exactWorkflowName := fmt.Sprintf("wf-cnv-%s-exact-odf-ceph", testID)
			createCNVWorkflow("exact-odf-ceph", "RestartPod", cnvSpec(), models.DetectedLabels{
				VirtualMachine: true,
				StorageBackend: "odf-ceph",
			})
			createCNVWorkflow("wildcard-storage", "RestartPod", cnvSpec(), models.DetectedLabels{
				VirtualMachine: true,
				StorageBackend: "*",
			})
			createCNVWorkflow("lvms-storage", "RestartPod", cnvSpec(), models.DetectedLabels{
				VirtualMachine: true,
				StorageBackend: "lvms",
			})

			filters := cnvDiscoveryContext(&models.DetectedLabels{StorageBackend: "odf-ceph"})
			results, _, err := workflowRepo.ListWorkflowsByActionType(ctx, "RestartPod", filters, 0, 100)

			Expect(err).ToNot(HaveOccurred())
			ours := filterOurs(results, prefix)
			Expect(ours).To(HaveLen(2), "exact and wildcard storageBackend workflows should match odf-ceph filter")
			Expect(ours[0].WorkflowName).To(Equal(exactWorkflowName),
				"exact storageBackend=odf-ceph workflow should outrank wildcard workflow")
			Expect(ours[0].DetectedLabels.StorageBackend).To(Equal("odf-ceph"))
			Expect(ours[1].DetectedLabels.StorageBackend).To(Equal("*"))
		})
	})

	Describe("CNV fixture roundtrip and discovery", func() {
		It("IT-DS-1378-003: fixture parse → serialize → extract → query preserves all 4 CNV fields [BR-WORKFLOW-004]", func() {
			prefix := fmt.Sprintf("wf-cnv-%s-", testID)
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
			seedWorkflowCRD(workflowCRDSpec{
				Name:        workflowName,
				ActionType:  parsedSchema.ActionType,
				Engine:      "job",
				Severity:    []string{"critical", "high"},
				Component:   []string{"kubevirt.io/v1/VirtualMachine"},
				Environment: []string{"production", "staging"},
				Priority:    "P1",
				DetectedLabels: &models.DetectedLabels{
					VirtualMachine: extracted.VirtualMachine,
					LiveMigratable: extracted.LiveMigratable,
					CDIManaged:     extracted.CDIManaged,
					StorageBackend: extracted.StorageBackend,
				},
			})

			filters := cnvDiscoveryContext(&models.DetectedLabels{
				VirtualMachine: true,
				LiveMigratable: true,
				CDIManaged:     true,
				StorageBackend: "odf-ceph",
			})
			results, _, err := workflowRepo.ListWorkflowsByActionType(ctx, parsedSchema.ActionType, filters, 0, 100)

			Expect(err).ToNot(HaveOccurred())
			ours := filterOurs(results, prefix)
			Expect(ours).To(HaveLen(1))
			Expect(ours[0].WorkflowName).To(Equal(workflowName))
			Expect(ours[0].DetectedLabels.VirtualMachine).To(BeTrue())
			Expect(ours[0].DetectedLabels.LiveMigratable).To(BeTrue())
			Expect(ours[0].DetectedLabels.CDIManaged).To(BeTrue())
			Expect(ours[0].DetectedLabels.StorageBackend).To(Equal("odf-ceph"))
		})
	})
})
