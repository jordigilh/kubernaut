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
// Pattern: Real PostgreSQL database (no mocks)
// ========================================

var _ = Describe("EngineConfig Workflow Catalog Integration (BR-WE-016)", func() {
	var (
		workflowRepo *workflow.Repository
		testID       string
	)

	BeforeEach(func() {
		workflowRepo = workflow.NewRepository(db, logger)
		testID = generateTestID()
	})

	Context("Create and retrieve with engineConfig", func() {
		It("IT-WE-016-001: should store and retrieve engineConfig in catalog", func() {
			workflowName := fmt.Sprintf("ansible-it-engineconfig-%s-%d", testID, time.Now().UnixNano())
			content := "test-workflow-content"
			contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

			ansibleConfig := map[string]interface{}{
				"playbookPath":    "playbooks/restart.yml",
				"jobTemplateName": "restart-pod",
				"inventoryName":   "production",
			}
			engineConfigJSON, err := json.Marshal(ansibleConfig)
			Expect(err).ToNot(HaveOccurred())
			rawEngineConfig := json.RawMessage(engineConfigJSON)

			labels := models.MandatoryLabels{
				Severity:    []string{"critical"},
				Component:   []string{"pod"},
				Environment: []string{"production"},
				Priority:    "P0",
			}

			testWorkflow := &models.RemediationWorkflow{
				WorkflowName:    workflowName,
				Version:         "v1.0.0",
				SchemaVersion:   "1.0",
				Name:            workflowName,
				Description:     models.StructuredDescription{What: "Ansible workflow", WhenToUse: "Testing engineConfig"},
				Content:         content,
				ContentHash:     contentHash,
				Labels:          labels,
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "Active",
				ExecutionEngine: models.ExecutionEngineAnsible,
				EngineConfig:    &rawEngineConfig,
				IsLatestVersion: true,
				ActionType:      "RestartPod",
			}

			err = workflowRepo.Create(ctx, testWorkflow)
			Expect(err).ToNot(HaveOccurred(), "Should persist workflow with engineConfig")

			defer func() {
				_, _ = db.ExecContext(ctx, `DELETE FROM remediation_workflow_catalog WHERE workflow_name = $1`, workflowName)
			}()

			retrieved, err := workflowRepo.GetByNameAndVersion(ctx, workflowName, "v1.0.0")
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
			content := "test-tekton-content"
			contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

			labels := models.MandatoryLabels{
				Severity:    []string{"high"},
				Component:   []string{"pod"},
				Environment: []string{"production"},
				Priority:    "P1",
			}

			testWorkflow := &models.RemediationWorkflow{
				WorkflowName:    workflowName,
				Version:         "v1.0.0",
				SchemaVersion:   "1.0",
				Name:            workflowName,
				Description:     models.StructuredDescription{What: "Tekton workflow", WhenToUse: "Testing no engineConfig"},
				Content:         content,
				ContentHash:     contentHash,
				Labels:          labels,
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "Active",
				ExecutionEngine: models.ExecutionEngineTekton,
				IsLatestVersion: true,
				ActionType:      "RestartPod",
			}

			err := workflowRepo.Create(ctx, testWorkflow)
			Expect(err).ToNot(HaveOccurred())

			defer func() {
				_, _ = db.ExecContext(ctx, `DELETE FROM remediation_workflow_catalog WHERE workflow_name = $1`, workflowName)
			}()

			retrieved, err := workflowRepo.GetByNameAndVersion(ctx, workflowName, "v1.0.0")
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved.ExecutionEngine).To(Equal(models.ExecutionEngineTekton))
			Expect(retrieved.EngineConfig).To(BeNil(), "Tekton workflows should have nil engineConfig")

			GinkgoWriter.Printf("✅ IT-WE-016-001b: tekton workflow without engineConfig verified\n")
		})
	})

	Context("List and search with engineConfig", func() {
		It("IT-WE-016-002: should return engineConfig in List results", func() {
			workflowName := fmt.Sprintf("ansible-it-list-%s-%d", testID, time.Now().UnixNano())
			content := "test-list-content"
			contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

			ansibleConfig := map[string]interface{}{
				"playbookPath":    "playbooks/scale-down.yml",
				"jobTemplateName": "scale-down-svc",
			}
			engineConfigJSON, err := json.Marshal(ansibleConfig)
			Expect(err).ToNot(HaveOccurred())
			rawEngineConfig := json.RawMessage(engineConfigJSON)

			testWorkflow := &models.RemediationWorkflow{
				WorkflowName:    workflowName,
				Version:         "v1.0.0",
				SchemaVersion:   "1.0",
				Name:            workflowName,
				Description:     models.StructuredDescription{What: "List test", WhenToUse: "IT-WE-016-002"},
				Content:         content,
				ContentHash:     contentHash,
				Labels:          models.MandatoryLabels{Severity: []string{"critical"}, Component: []string{"pod"}, Environment: []string{"production"}, Priority: "P0"},
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "Active",
				ExecutionEngine: models.ExecutionEngineAnsible,
				EngineConfig:    &rawEngineConfig,
				IsLatestVersion: true,
				ActionType:      "ScaleReplicas",
			}

			err = workflowRepo.Create(ctx, testWorkflow)
			Expect(err).ToNot(HaveOccurred())

			defer func() {
				_, _ = db.ExecContext(ctx, `DELETE FROM remediation_workflow_catalog WHERE workflow_name = $1`, workflowName)
			}()

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
			content := "test-float-content"
			contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

			min := 0.1
			max := 99.9
			params := []models.WorkflowParameter{
				{
					Name:        "cpu_threshold",
					Type:        "float",
					Description: "CPU threshold percentage",
					Required:    true,
					Minimum:     &min,
					Maximum:     &max,
				},
				{
					Name:        "timeout",
					Type:        "integer",
					Description: "Timeout in seconds",
					Required:    false,
				},
			}

			paramsJSON, err := json.Marshal(params)
			Expect(err).ToNot(HaveOccurred())
			rawParams := json.RawMessage(paramsJSON)

			testWorkflow := &models.RemediationWorkflow{
				WorkflowName:    workflowName,
				Version:         "v1.0.0",
				SchemaVersion:   "1.0",
				Name:            workflowName,
				Description:     models.StructuredDescription{What: "Float params test", WhenToUse: "IT-WF-005-001"},
				Content:         content,
				ContentHash:     contentHash,
				Labels:          models.MandatoryLabels{Severity: []string{"high"}, Component: []string{"pod"}, Environment: []string{"production"}, Priority: "P1"},
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "Active",
				ExecutionEngine: models.ExecutionEngineTekton,
				Parameters:      &rawParams,
				IsLatestVersion: true,
				ActionType:      "IncreaseCPULimits",
			}

			err = workflowRepo.Create(ctx, testWorkflow)
			Expect(err).ToNot(HaveOccurred(), "Should persist workflow with float parameters")

			defer func() {
				_, _ = db.ExecContext(ctx, `DELETE FROM remediation_workflow_catalog WHERE workflow_name = $1`, workflowName)
			}()

			retrieved, err := workflowRepo.GetByNameAndVersion(ctx, workflowName, "v1.0.0")
			Expect(err).ToNot(HaveOccurred())
			var retrievedParams []models.WorkflowParameter
			err = json.Unmarshal(*retrieved.Parameters, &retrievedParams)
			Expect(err).ToNot(HaveOccurred())
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
