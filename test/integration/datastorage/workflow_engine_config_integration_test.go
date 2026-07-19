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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
)

// ========================================
// ENGINE CONFIG INTEGRATION TESTS (BR-WE-016, BR-WORKFLOW-005)
// ========================================
// Authority: BR-WE-016 (EngineConfig Discriminator Pattern)
// Authority: BR-WORKFLOW-005 (Float Parameter Type)
// Test Plan: docs/testing/45/TEST_PLAN.md
// Pattern: CRD-native seeding + shared informer cache (DD-WORKFLOW-018).
//
// #1661 Phase F: migrated off workflowRepo.Create (Postgres, zero production
// callers post-Phase-B) to seedWorkflowCRD -- workflows are RemediationWorkflow
// CRDs, read back via the cache-backed GetByID/List, not GetByNameAndVersion
// (a dying Postgres-only method with no cache equivalent: DD-WORKFLOW-018
// makes etcd metadata.name the sole identity, so "get by name+version" no
// longer has a coexisting-versions concept to disambiguate).
// ========================================

// unwrapParameters extracts the flat parameter list from the
// {"schema":{"parameters":[...]}} envelope that GetByID/List's cache-backed
// path wraps parameters in (wrapCRDParameters, cache_convert.go) -- mirrors
// the wire format ogen clients decode (DD-API-001).
func unwrapParameters(raw *json.RawMessage) []models.WorkflowParameter {
	var wrapper struct {
		Schema struct {
			Parameters []models.WorkflowParameter `json:"parameters"`
		} `json:"schema"`
	}
	Expect(json.Unmarshal(*raw, &wrapper)).To(Succeed())
	return wrapper.Schema.Parameters
}

var _ = Describe("EngineConfig Workflow Catalog Integration (BR-WE-016)", func() {
	var (
		workflowRepo *workflow.Repository
		testID       string
	)

	BeforeEach(func() {
		workflowRepo = newCachedWorkflowRepo()
		testID = generateTestID()
	})

	Context("Create and retrieve with engineConfig", func() {
		It("IT-WE-016-001: should store and retrieve engineConfig in catalog", func() {
			workflowName := fmt.Sprintf("ansible-it-engineconfig-%s-%d", testID, time.Now().UnixNano())

			ansibleConfig := map[string]interface{}{
				"playbookPath":    "playbooks/restart.yml",
				"jobTemplateName": "restart-pod",
				"inventoryName":   "production",
			}

			workflowID := seedWorkflowCRD(workflowCRDSpec{
				Name:         workflowName,
				ActionType:   "RestartPod",
				Engine:       "ansible",
				EngineConfig: ansibleConfig,
			})

			retrieved, err := workflowRepo.GetByID(ctx, workflowID)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved.ExecutionEngine).To(Equal(models.ExecutionEngineAnsible))
			Expect(retrieved.EngineConfig).ToNot(BeNil(), "EngineConfig should be preserved after roundtrip")

			var parsedConfig map[string]interface{}
			err = json.Unmarshal(*retrieved.EngineConfig, &parsedConfig)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedConfig["playbookPath"]).To(Equal("playbooks/restart.yml"))
			Expect(parsedConfig["jobTemplateName"]).To(Equal("restart-pod"))
			Expect(parsedConfig["inventoryName"]).To(Equal("production"))

			GinkgoWriter.Printf("✅ IT-WE-016-001: engineConfig roundtrip verified\n")
		})

		It("IT-WE-016-001b: should store tekton workflow without engineConfig", func() {
			workflowName := fmt.Sprintf("tekton-it-no-engineconfig-%s-%d", testID, time.Now().UnixNano())

			workflowID := seedWorkflowCRD(workflowCRDSpec{
				Name:       workflowName,
				ActionType: "RestartPod",
				Engine:     "tekton",
			})

			retrieved, err := workflowRepo.GetByID(ctx, workflowID)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved.ExecutionEngine).To(Equal(models.ExecutionEngineTekton))
			Expect(retrieved.EngineConfig).To(BeNil(), "Tekton workflows should have nil engineConfig")

			GinkgoWriter.Printf("✅ IT-WE-016-001b: tekton workflow without engineConfig verified\n")
		})
	})

	Context("List and search with engineConfig", func() {
		It("IT-WE-016-002: should return engineConfig in List results", func() {
			workflowName := fmt.Sprintf("ansible-it-list-%s-%d", testID, time.Now().UnixNano())

			ansibleConfig := map[string]interface{}{
				"playbookPath":    "playbooks/scale-down.yml",
				"jobTemplateName": "scale-down-svc",
			}

			seedWorkflowCRD(workflowCRDSpec{
				Name:         workflowName,
				ActionType:   "ScaleReplicas",
				Engine:       "ansible",
				EngineConfig: ansibleConfig,
			})

			filters := &models.WorkflowSearchFilters{WorkflowName: workflowName}
			results, total, err := workflowRepo.List(ctx, filters, 50, 0)
			Expect(err).ToNot(HaveOccurred())
			Expect(total).To(Equal(1))
			Expect(results).To(HaveLen(1))
			Expect(results[0].EngineConfig).ToNot(BeNil(), "EngineConfig must be present in List results")

			var parsedConfig map[string]interface{}
			err = json.Unmarshal(*results[0].EngineConfig, &parsedConfig)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedConfig["playbookPath"]).To(Equal("playbooks/scale-down.yml"))

			GinkgoWriter.Printf("✅ IT-WE-016-002: engineConfig returned in List results\n")
		})
	})

	Context("Float parameter persistence", func() {
		It("IT-WF-005-001: should store and retrieve workflow with float parameters", func() {
			workflowName := fmt.Sprintf("float-param-it-%s-%d", testID, time.Now().UnixNano())

			minVal := 0.1
			maxVal := 99.9
			params := []models.WorkflowParameter{
				{
					Name:        "cpu_threshold",
					Type:        "float",
					Description: "CPU threshold percentage",
					Required:    true,
					Minimum:     &minVal,
					Maximum:     &maxVal,
				},
				{
					Name:        "timeout",
					Type:        "integer",
					Description: "Timeout in seconds",
					Required:    false,
				},
			}

			workflowID := seedWorkflowCRD(workflowCRDSpec{
				Name:       workflowName,
				ActionType: "IncreaseCPULimits",
				Engine:     "tekton",
				Parameters: params,
			})

			retrieved, err := workflowRepo.GetByID(ctx, workflowID)
			Expect(err).ToNot(HaveOccurred())
			retrievedParams := unwrapParameters(retrieved.Parameters)
			Expect(retrievedParams).To(HaveLen(2))

			cpuParam := retrievedParams[0]
			Expect(cpuParam.Name).To(Equal("cpu_threshold"))
			Expect(cpuParam.Type).To(Equal("float"))
			Expect(*cpuParam.Minimum).To(BeNumerically("~", 0.1, 0.001))
			Expect(*cpuParam.Maximum).To(BeNumerically("~", 99.9, 0.001))

			GinkgoWriter.Printf("✅ IT-WF-005-001: float parameter persistence verified\n")
		})
	})
})
