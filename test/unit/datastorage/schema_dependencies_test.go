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
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// ========================================
// SCHEMA-DECLARED DEPENDENCIES TESTS (DD-WE-006)
// ========================================
// Authority: DD-WE-006 (Schema-Declared Infrastructure Dependencies)
// Authority: BR-WORKFLOW-004 (Workflow Schema Format Specification)
// Business Requirement: BR-WE-014 (Kubernetes Job Execution Backend)
// ========================================
//
// Tests cover:
// - Parsing dependencies section from workflow-schema.yaml
// - Structural validation (non-empty names, uniqueness)
// - Backward compatibility (no dependencies section)
// - ExtractDependencies helper
//
// ========================================

// depsTestCRD returns a CRD with dependencies pre-configured.
func depsTestCRD() *models.WorkflowSchemaCRD {
	crd := testutil.NewTestWorkflowCRD("fix-certificate-gitops-v1", "GitRevertCommit", "job")
	crd.Spec.Description = sharedtypes.StructuredDescription{
		What:      "Reverts a bad Git commit that broke a cert-manager ClusterIssuer",
		WhenToUse: "When a GitOps-managed cert-manager Certificate is stuck NotReady",
	}
	crd.Spec.Labels = models.WorkflowSchemaLabels{
		Severity:    []string{"critical", "high"},
		Environment: []string{"*"},
		Component:   "certificate",
		Priority:    "*",
	}
	crd.Spec.Dependencies = &models.WorkflowDependencies{
		Secrets:    []models.ResourceDependency{{Name: "gitea-repo-creds"}},
		ConfigMaps: []models.ResourceDependency{{Name: "remediation-config"}},
	}
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "GIT_REPO_URL", Type: "string", Required: true, Description: "URL of the Git repository (without credentials)"},
		{Name: "TARGET_NAMESPACE", Type: "string", Required: true, Description: "Namespace of the affected Certificate"},
	}
	return crd
}

var _ = Describe("Schema-Declared Dependencies (DD-WE-006)", func() {

	Context("Parsing dependencies from workflow-schema.yaml", func() {
		var parser *schema.Parser

		BeforeEach(func() {
			parser = schema.NewParser()
		})

		It("UT-DS-006-001: should parse dependencies section with secrets and configMaps", func() {
			yamlContent := testutil.MarshalWorkflowCRD(depsTestCRD())
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Dependencies.Secrets).To(HaveLen(1))
			Expect(parsedSchema.Dependencies.Secrets[0].Name).To(Equal("gitea-repo-creds"))
			Expect(parsedSchema.Dependencies.ConfigMaps).To(HaveLen(1))
			Expect(parsedSchema.Dependencies.ConfigMaps[0].Name).To(Equal("remediation-config"))
		})

		It("UT-DS-006-002: should accept schema without dependencies section (backward compatible)", func() {
			yamlContent := testutil.MarshalWorkflowCRD(baseOCITestCRD())
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Dependencies).To(BeNil(),
				"absence of dependencies in YAML means no infrastructure dependencies")
		})

		It("UT-DS-006-003: should accept schema with empty dependencies section", func() {
			crd := baseOCITestCRD()
			crd.Spec.Dependencies = &models.WorkflowDependencies{}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Dependencies.Secrets).To(BeEmpty())
			Expect(parsedSchema.Dependencies.ConfigMaps).To(BeEmpty())
		})

		It("UT-DS-006-004: should accept schema with only secrets, no configMaps", func() {
			crd := baseOCITestCRD()
			crd.Spec.Dependencies = &models.WorkflowDependencies{
				Secrets: []models.ResourceDependency{{Name: "gitea-repo-creds"}},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Dependencies.Secrets).To(HaveLen(1))
			Expect(parsedSchema.Dependencies.ConfigMaps).To(BeEmpty())
		})

		It("UT-DS-006-005: should accept schema with only configMaps, no secrets", func() {
			crd := baseOCITestCRD()
			crd.Spec.Dependencies = &models.WorkflowDependencies{
				ConfigMaps: []models.ResourceDependency{{Name: "remediation-config"}},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Dependencies.Secrets).To(BeEmpty())
			Expect(parsedSchema.Dependencies.ConfigMaps).To(HaveLen(1))
		})

		It("UT-DS-006-006: should parse multiple secrets and configMaps", func() {
			crd := baseOCITestCRD()
			crd.Spec.Dependencies = &models.WorkflowDependencies{
				Secrets: []models.ResourceDependency{
					{Name: "gitea-repo-creds"},
					{Name: "tls-certificates"},
				},
				ConfigMaps: []models.ResourceDependency{
					{Name: "remediation-config"},
					{Name: "alert-thresholds"},
				},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Dependencies.Secrets).To(HaveLen(2))
			Expect(parsedSchema.Dependencies.ConfigMaps).To(HaveLen(2))
		})
	})

	Context("Structural validation of dependencies", func() {
		var parser *schema.Parser

		BeforeEach(func() {
			parser = schema.NewParser()
		})

		It("UT-DS-006-010: should reject secret with empty name", func() {
			crd := baseOCITestCRD()
			crd.Spec.Dependencies = &models.WorkflowDependencies{
				Secrets: []models.ResourceDependency{{Name: ""}},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dependencies.secrets"))
			Expect(err.Error()).To(ContainSubstring("name"),
				"error should tell the author that name is required")
		})

		It("UT-DS-006-011: should reject configMap with empty name", func() {
			crd := baseOCITestCRD()
			crd.Spec.Dependencies = &models.WorkflowDependencies{
				ConfigMaps: []models.ResourceDependency{{Name: ""}},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dependencies.configMaps"))
			Expect(err.Error()).To(ContainSubstring("name"),
				"error should tell the author that name is required")
		})

		It("UT-DS-006-012: should reject duplicate secret names", func() {
			crd := baseOCITestCRD()
			crd.Spec.Dependencies = &models.WorkflowDependencies{
				Secrets: []models.ResourceDependency{
					{Name: "gitea-repo-creds"},
					{Name: "gitea-repo-creds"},
				},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dependencies.secrets"))
			Expect(err.Error()).To(ContainSubstring("duplicate"),
				"error should tell the author about the duplicate")
			Expect(err.Error()).To(ContainSubstring("gitea-repo-creds"))
		})

		It("UT-DS-006-013: should reject duplicate configMap names", func() {
			crd := baseOCITestCRD()
			crd.Spec.Dependencies = &models.WorkflowDependencies{
				ConfigMaps: []models.ResourceDependency{
					{Name: "remediation-config"},
					{Name: "remediation-config"},
				},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dependencies.configMaps"))
			Expect(err.Error()).To(ContainSubstring("duplicate"),
				"error should tell the author about the duplicate")
			Expect(err.Error()).To(ContainSubstring("remediation-config"))
		})

		It("UT-DS-006-014: should allow same name in different categories (secret vs configMap)", func() {
			crd := baseOCITestCRD()
			crd.Spec.Dependencies = &models.WorkflowDependencies{
				Secrets:    []models.ResourceDependency{{Name: "shared-name"}},
				ConfigMaps: []models.ResourceDependency{{Name: "shared-name"}},
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred(),
				"same name in different categories should be allowed")
			Expect(parsedSchema.Dependencies.Secrets[0].Name).To(Equal("shared-name"))
			Expect(parsedSchema.Dependencies.ConfigMaps[0].Name).To(Equal("shared-name"))
		})
	})

	Context("ExtractDependencies helper", func() {
		var parser *schema.Parser

		BeforeEach(func() {
			parser = schema.NewParser()
		})

		It("UT-DS-006-020: should extract dependencies from schema with deps", func() {
			yamlContent := testutil.MarshalWorkflowCRD(depsTestCRD())
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			deps := parser.ExtractDependencies(parsedSchema)
			Expect(deps.Secrets).To(HaveLen(1))
			Expect(deps.Secrets[0].Name).To(Equal("gitea-repo-creds"))
			Expect(deps.ConfigMaps).To(HaveLen(1))
			Expect(deps.ConfigMaps[0].Name).To(Equal("remediation-config"))
		})

		It("UT-DS-006-021: should return nil when schema has no dependencies", func() {
			yamlContent := testutil.MarshalWorkflowCRD(baseOCITestCRD())
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			deps := parser.ExtractDependencies(parsedSchema)
			Expect(deps).To(BeNil(),
				"ExtractDependencies should return nil for schemas without dependencies")
		})
	})
})
