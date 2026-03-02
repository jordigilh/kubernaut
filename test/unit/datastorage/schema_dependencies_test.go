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

	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
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

// validSchemaWithDependencies is a workflow schema that declares dependencies
const validSchemaWithDependencies = `metadata:
  workflowId: fix-certificate-gitops-v1
  version: "1.0.0"
  description:
    what: Reverts a bad Git commit that broke a cert-manager ClusterIssuer
    whenToUse: When a GitOps-managed cert-manager Certificate is stuck NotReady
actionType: GitRevertCommit
labels:
  signalName: CertManagerCertNotReady
  severity: [critical, high]
  environment: ["*"]
  component: certificate
  priority: "*"
execution:
  engine: job
  bundle: quay.io/kubernaut/fix-cert:v1.0.0@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
dependencies:
  secrets:
    - name: gitea-repo-creds
  configMaps:
    - name: remediation-config
parameters:
  - name: GIT_REPO_URL
    type: string
    required: true
    description: URL of the Git repository (without credentials)
  - name: TARGET_NAMESPACE
    type: string
    required: true
    description: Namespace of the affected Certificate
`

var _ = Describe("Schema-Declared Dependencies (DD-WE-006)", func() {

	Context("Parsing dependencies from workflow-schema.yaml", func() {
		var parser *schema.Parser

		BeforeEach(func() {
			parser = schema.NewParser()
		})

		It("UT-DS-006-001: should parse dependencies section with secrets and configMaps", func() {
			parsedSchema, err := parser.ParseAndValidate(validSchemaWithDependencies)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Dependencies.Secrets).To(HaveLen(1))
			Expect(parsedSchema.Dependencies.Secrets[0].Name).To(Equal("gitea-repo-creds"))
			Expect(parsedSchema.Dependencies.ConfigMaps).To(HaveLen(1))
			Expect(parsedSchema.Dependencies.ConfigMaps[0].Name).To(Equal("remediation-config"))
		})

		It("UT-DS-006-002: should accept schema without dependencies section (backward compatible)", func() {
			parsedSchema, err := parser.ParseAndValidate(validWorkflowSchemaYAML)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Dependencies).To(BeNil(),
				"absence of dependencies in YAML means no infrastructure dependencies")
		})

		It("UT-DS-006-003: should accept schema with empty dependencies section", func() {
			yamlContent := validWorkflowSchemaYAML + `dependencies: {}
`
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Dependencies.Secrets).To(BeEmpty())
			Expect(parsedSchema.Dependencies.ConfigMaps).To(BeEmpty())
		})

		It("UT-DS-006-004: should accept schema with only secrets, no configMaps", func() {
			yamlContent := validWorkflowSchemaYAML + `dependencies:
  secrets:
    - name: gitea-repo-creds
`
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Dependencies.Secrets).To(HaveLen(1))
			Expect(parsedSchema.Dependencies.ConfigMaps).To(BeEmpty())
		})

		It("UT-DS-006-005: should accept schema with only configMaps, no secrets", func() {
			yamlContent := validWorkflowSchemaYAML + `dependencies:
  configMaps:
    - name: remediation-config
`
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Dependencies.Secrets).To(BeEmpty())
			Expect(parsedSchema.Dependencies.ConfigMaps).To(HaveLen(1))
		})

		It("UT-DS-006-006: should parse multiple secrets and configMaps", func() {
			yamlContent := validWorkflowSchemaYAML + `dependencies:
  secrets:
    - name: gitea-repo-creds
    - name: tls-certificates
  configMaps:
    - name: remediation-config
    - name: alert-thresholds
`
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
			yamlContent := validWorkflowSchemaYAML + `dependencies:
  secrets:
    - name: ""
`
			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dependencies.secrets"))
			Expect(err.Error()).To(ContainSubstring("name"),
				"error should tell the author that name is required")
		})

		It("UT-DS-006-011: should reject configMap with empty name", func() {
			yamlContent := validWorkflowSchemaYAML + `dependencies:
  configMaps:
    - name: ""
`
			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dependencies.configMaps"))
			Expect(err.Error()).To(ContainSubstring("name"),
				"error should tell the author that name is required")
		})

		It("UT-DS-006-012: should reject duplicate secret names", func() {
			yamlContent := validWorkflowSchemaYAML + `dependencies:
  secrets:
    - name: gitea-repo-creds
    - name: gitea-repo-creds
`
			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dependencies.secrets"))
			Expect(err.Error()).To(ContainSubstring("duplicate"),
				"error should tell the author about the duplicate")
			Expect(err.Error()).To(ContainSubstring("gitea-repo-creds"))
		})

		It("UT-DS-006-013: should reject duplicate configMap names", func() {
			yamlContent := validWorkflowSchemaYAML + `dependencies:
  configMaps:
    - name: remediation-config
    - name: remediation-config
`
			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dependencies.configMaps"))
			Expect(err.Error()).To(ContainSubstring("duplicate"),
				"error should tell the author about the duplicate")
			Expect(err.Error()).To(ContainSubstring("remediation-config"))
		})

		It("UT-DS-006-014: should allow same name in different categories (secret vs configMap)", func() {
			yamlContent := validWorkflowSchemaYAML + `dependencies:
  secrets:
    - name: shared-name
  configMaps:
    - name: shared-name
`
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
			parsedSchema, err := parser.ParseAndValidate(validSchemaWithDependencies)
			Expect(err).ToNot(HaveOccurred())

			deps := parser.ExtractDependencies(parsedSchema)
			Expect(deps.Secrets).To(HaveLen(1))
			Expect(deps.Secrets[0].Name).To(Equal("gitea-repo-creds"))
			Expect(deps.ConfigMaps).To(HaveLen(1))
			Expect(deps.ConfigMaps[0].Name).To(Equal("remediation-config"))
		})

		It("UT-DS-006-021: should return nil when schema has no dependencies", func() {
			parsedSchema, err := parser.ParseAndValidate(validWorkflowSchemaYAML)
			Expect(err).ToNot(HaveOccurred())

			deps := parser.ExtractDependencies(parsedSchema)
			Expect(deps).To(BeNil(),
				"ExtractDependencies should return nil for schemas without dependencies")
		})
	})
})
