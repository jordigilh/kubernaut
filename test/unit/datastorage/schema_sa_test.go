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
	"github.com/jordigilh/kubernaut/test/testutil"
)

// ========================================
// PER-WORKFLOW SERVICE ACCOUNT TESTS (#481)
// ========================================
// Authority: DD-WE-005 v2.0 (Per-Workflow ServiceAccount Reference)
// Business Requirement: BR-WE-007 (Service account configuration)
// ========================================

var _ = Describe("Per-Workflow ServiceAccount [DD-WE-005] (#481)", func() {

	var parser *schema.Parser

	BeforeEach(func() {
		parser = schema.NewParser()
	})

	Context("schema parsing with serviceAccountName", func() {

		It("UT-DS-481-001: should parse serviceAccountName from execution section", func() {
			crd := testutil.NewTestWorkflowCRD("sa-test-workflow", "PatchHPA", "job")
			crd.Spec.Execution.ServiceAccountName = "hpa-workflow-sa"
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Execution.ServiceAccountName).To(Equal("hpa-workflow-sa"))
			Expect(parsedSchema.Execution.Engine).To(Equal("job"))
			Expect(parsedSchema.Execution.Bundle).To(Equal(testutil.ValidBundleRef))
		})

		It("UT-DS-481-002: should accept schema without serviceAccountName (optional field)", func() {
			crd := testutil.NewTestWorkflowCRD("sa-optional-workflow", "RestartPod", "tekton")
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Execution.ServiceAccountName).To(BeEmpty())
		})
	})

	Context("ExtractServiceAccountName", func() {

		It("UT-DS-481-003: should return nil when schema has no serviceAccountName", func() {
			crd := testutil.NewTestWorkflowCRD("sa-absent-workflow", "RestartPod", "tekton")
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			result := parser.ExtractServiceAccountName(parsedSchema)
			Expect(result).To(BeNil())
		})

		It("UT-DS-481-004: should return pointer to SA name when present", func() {
			crd := testutil.NewTestWorkflowCRD("sa-present-workflow", "PatchHPA", "job")
			crd.Spec.Execution.ServiceAccountName = "hpa-workflow-sa"
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			result := parser.ExtractServiceAccountName(parsedSchema)
			Expect(*result).To(Equal("hpa-workflow-sa"))
		})
	})
})
