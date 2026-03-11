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
	"github.com/jordigilh/kubernaut/test/testutil"
)

// ========================================
// SCHEMA VERSION VALIDATION TESTS (#255)
// ========================================
// Authority: BR-WORKFLOW-004 v1.1 (Workflow Schema Format Specification)
// Business Requirement: BR-WORKFLOW-004 (schemaVersion field)
// Enables: DD-WE-005 (Workflow-Scoped RBAC in v1.1)
// ========================================

var _ = Describe("Schema Version Validation [BR-WORKFLOW-004] (#255)", func() {

	var parser *schema.Parser

	BeforeEach(func() {
		parser = schema.NewParser()
	})

	Context("apiVersion-derived schemaVersion parsing", func() {

		It("UT-DS-255-001: should derive schemaVersion from apiVersion", func() {
			crd := testutil.NewTestWorkflowCRD("schema-version-test", "RestartPod", "tekton")
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.SchemaVersion).To(Equal("1.0"),
				"schemaVersion should be derived from apiVersion kubernaut.ai/v1alpha1")
		})

		It("UT-DS-255-002: should reject schema with missing apiVersion", func() {
			// Tier 3: Raw YAML — missing apiVersion cannot be expressed via builder
			// since WorkflowSchemaCRD always marshals the apiVersion field.
			schemaVersionMissingYAML := `kind: RemediationWorkflow
metadata:
  name: schema-version-missing
spec:
  version: "1.0.0"
  description:
    what: Tests missing apiVersion
    whenToUse: When validating schema format versioning
  actionType: RestartPod
  labels:
    severity: [critical]
    environment: [production]
    component: pod
    priority: P0
  execution:
    engine: tekton
    bundle: quay.io/kubernaut/test@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
  parameters:
    - name: NAMESPACE
      type: string
      required: true
      description: Target namespace
`
			_, err := parser.ParseAndValidate(schemaVersionMissingYAML)
			Expect(err).To(HaveOccurred())

			var validationErr *models.SchemaValidationError
			Expect(err).To(BeAssignableToTypeOf(validationErr))
			Expect(err.Error()).To(ContainSubstring("apiVersion"))
		})

		It("UT-DS-255-003: should reject schema with unsupported apiVersion", func() {
			crd := testutil.NewTestWorkflowCRD("schema-version-invalid", "RestartPod", "tekton")
			crd.APIVersion = "kubernaut.ai/v2"
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())

			var validationErr *models.SchemaValidationError
			Expect(err).To(BeAssignableToTypeOf(validationErr))
			Expect(err.Error()).To(ContainSubstring("apiVersion"))
		})
	})
})
