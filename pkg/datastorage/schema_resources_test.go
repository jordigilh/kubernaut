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

package datastorage_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// ========================================
// PER-WORKFLOW JOB RESOURCES TESTS (DD-WE-008 / BR-WE-019)
// ========================================
// Authority: DD-WE-008 (Job Resource Governance and Transient-Failure Tolerance)
// Authority: BR-WE-019 (Job Resource Governance and Transient-Failure Tolerance)
// Test Plan: docs/tests/1572/TEST_PLAN.md (TP-1572-v1.5)
//
// Tests cover:
// - Parsing execution.resources (quoted/unquoted quantities, requests-only/limits-only)
// - Registration-time fail-fast validation (invalid quantity, requests > limits)
// - Engine-scoping enforcement (job only)
// - ExtractResources helper (absent section -> nil, nil)
// ========================================

// resourcesTestCRD returns a CRD with engine "job" and no resources set by default.
func resourcesTestCRD() *models.WorkflowSchemaCRD {
	crd := testutil.NewTestWorkflowCRD("scale-up-deployment", "ScaleReplicas", "job")
	crd.Spec.Execution.Bundle = "ghcr.io/test/bundle:latest@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	crd.Spec.Labels.Component = []string{"apps/v1/Deployment"}
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "REPLICAS", Type: "integer", Required: true, Description: "Target replica count"},
	}
	return crd
}

var _ = Describe("Per-Workflow Job Resources (DD-WE-008, BR-WE-019)", func() {

	Context("Parsing execution.resources from workflow-schema.yaml", func() {
		var parser *schema.Parser

		BeforeEach(func() {
			parser = schema.NewParser()
		})

		It("UT-DS-008-001: should parse quoted and unquoted quantities into a typed corev1.ResourceRequirements", func() {
			crd := resourcesTestCRD()
			crd.Spec.Execution.Resources = &models.ResourcesSchema{
				Requests: map[string]string{"cpu": "100m", "memory": "512Mi"},
				Limits:   map[string]string{"cpu": "1", "memory": "1Gi"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			resources, err := parser.ExtractResources(parsedSchema)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).ToNot(BeNil())
			Expect(resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("100m")))
			Expect(resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("512Mi")))
			Expect(resources.Limits[corev1.ResourceCPU]).To(Equal(resource.MustParse("1")))
			Expect(resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("1Gi")))
		})

		It("UT-DS-008-002: should reject an invalid resource quantity at registration time", func() {
			crd := resourcesTestCRD()
			crd.Spec.Execution.Resources = &models.ResourcesSchema{
				Requests: map[string]string{"cpu": "not-a-number"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("execution.resources"))
		})

		It("UT-DS-008-003: should return (nil, nil) when execution.resources is absent (backward compatible)", func() {
			yamlContent := testutil.MarshalWorkflowCRD(resourcesTestCRD())
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			resources, err := parser.ExtractResources(parsedSchema)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).To(BeNil(), "absence of execution.resources means no override -- BestEffort QoS preserved")
		})

		It("UT-DS-008-004: should parse requests-only and limits-only declarations independently", func() {
			crd := resourcesTestCRD()
			crd.Spec.Execution.Resources = &models.ResourcesSchema{
				Requests: map[string]string{"cpu": "250m"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			resources, err := parser.ExtractResources(parsedSchema)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("250m")))
			Expect(resources.Limits).To(BeEmpty())

			crd2 := resourcesTestCRD()
			crd2.Spec.Execution.Resources = &models.ResourcesSchema{
				Limits: map[string]string{"memory": "256Mi"},
			}
			yamlContent2 := testutil.MarshalWorkflowCRD(crd2)
			parsedSchema2, err := parser.ParseAndValidate(yamlContent2)
			Expect(err).ToNot(HaveOccurred())

			resources2, err := parser.ExtractResources(parsedSchema2)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources2.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("256Mi")))
			Expect(resources2.Requests).To(BeEmpty())
		})

		It("UT-DS-008-009: should parse a valid mixed cpu/memory requests+limits declaration", func() {
			crd := resourcesTestCRD()
			crd.Spec.Execution.Resources = &models.ResourcesSchema{
				Requests: map[string]string{"cpu": "100m", "memory": "128Mi"},
				Limits:   map[string]string{"cpu": "500m", "memory": "256Mi"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			resources, err := parser.ExtractResources(parsedSchema)
			Expect(err).ToNot(HaveOccurred())
			Expect(resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("100m")))
			Expect(resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("128Mi")))
			Expect(resources.Limits[corev1.ResourceCPU]).To(Equal(resource.MustParse("500m")))
			Expect(resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("256Mi")))
		})
	})

	Context("Engine-scoping enforcement (AC2)", func() {
		It("UT-DS-008-005: should reject execution.resources declared with engine: tekton", func() {
			crd := testutil.NewTestWorkflowCRD("tekton-with-resources", "RestartPod", "tekton")
			crd.Spec.Execution.Bundle = "ghcr.io/test/bundle:latest@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
			crd.Spec.Parameters = []models.WorkflowParameter{
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "ns"},
			}
			crd.Spec.Execution.Resources = &models.ResourcesSchema{
				Requests: map[string]string{"cpu": "100m"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parser := schema.NewParser()
			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("execution.resources"))
			Expect(err.Error()).To(ContainSubstring("tekton"))
		})

		It("UT-DS-008-006: should reject execution.resources declared with engine: ansible", func() {
			crd := testutil.NewTestWorkflowCRD("ansible-with-resources", "RunPlaybook", "ansible")
			crd.Spec.Execution.Bundle = "https://github.com/example/playbooks.git"
			crd.Spec.Execution.EngineConfig = map[string]interface{}{
				"playbookPath": "playbooks/restart.yml",
			}
			crd.Spec.Parameters = []models.WorkflowParameter{
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "ns"},
			}
			crd.Spec.Execution.Resources = &models.ResourcesSchema{
				Requests: map[string]string{"cpu": "100m"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parser := schema.NewParser()
			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("execution.resources"))
			Expect(err.Error()).To(ContainSubstring("ansible"))
		})
	})

	Context("requests <= limits fail-fast validation (AC3)", func() {
		var parser *schema.Parser

		BeforeEach(func() {
			parser = schema.NewParser()
		})

		It("UT-DS-008-007: should reject requests.cpu exceeding limits.cpu", func() {
			crd := resourcesTestCRD()
			crd.Spec.Execution.Resources = &models.ResourcesSchema{
				Requests: map[string]string{"cpu": "2"},
				Limits:   map[string]string{"cpu": "1"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("execution.resources"))
			Expect(err.Error()).To(ContainSubstring("cpu"))
		})

		It("UT-DS-008-008: should reject requests.memory exceeding limits.memory (independent of cpu)", func() {
			crd := resourcesTestCRD()
			crd.Spec.Execution.Resources = &models.ResourcesSchema{
				Requests: map[string]string{"cpu": "100m", "memory": "1Gi"},
				Limits:   map[string]string{"cpu": "500m", "memory": "512Mi"},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("execution.resources"))
			Expect(err.Error()).To(ContainSubstring("memory"))
		})
	})
})
