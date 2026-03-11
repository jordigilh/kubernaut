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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// ========================================
// SCHEMA PARSER ENGINE CONFIG EXTRACTION TESTS (BR-WE-016)
// ========================================
// Authority: BR-WE-016 (EngineConfig Discriminator Pattern)
// Authority: BR-WE-015 (Ansible Execution Engine)
// Test Plan: docs/testing/45/TEST_PLAN.md
// ========================================

// ansibleTestCRD returns a CRD with ansible execution and engineConfig.
func ansibleTestCRD() *models.WorkflowSchemaCRD {
	crd := testutil.NewTestWorkflowCRD("ansible-test-workflow", "RestartPod", "ansible")
	crd.Spec.Execution.Bundle = "https://github.com/kubernaut/playbooks.git"
	crd.Spec.Execution.BundleDigest = "abc123def456"
	crd.Spec.Execution.EngineConfig = map[string]interface{}{
		"playbookPath":    "playbooks/restart_pod.yml",
		"jobTemplateName": "restart-pod",
		"inventoryName":   "production",
	}
	return crd
}

// tektonWithBundleDigestCRD returns a CRD with tekton execution and explicit bundleDigest.
func tektonWithBundleDigestCRD() *models.WorkflowSchemaCRD {
	crd := testutil.NewTestWorkflowCRD("tekton-digest-test", "RestartPod", "tekton")
	crd.Spec.Execution.Bundle = "quay.io/kubernaut/workflows/scale@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	crd.Spec.Execution.BundleDigest = "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	return crd
}

// tektonInlineDigestCRD returns a CRD with tekton execution and inline digest in bundle URL.
func tektonInlineDigestCRD() *models.WorkflowSchemaCRD {
	crd := testutil.NewTestWorkflowCRD("tekton-inline-digest-test", "RestartPod", "tekton")
	crd.Spec.Execution.Bundle = "quay.io/kubernaut/workflows/scale@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	return crd
}

var _ = Describe("Schema Parser EngineConfig Extraction [BR-WE-016]", func() {
	var parser *schema.Parser

	BeforeEach(func() {
		parser = schema.NewParser()
	})

	Context("ExtractEngineConfig", func() {
		It("UT-WE-016-005: should extract engineConfig from ansible workflow schema", func() {
			yamlContent := testutil.MarshalWorkflowCRD(ansibleTestCRD())
			parsedSchema, err := parser.Parse(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			engineConfig := parser.ExtractEngineConfig(parsedSchema)
			Expect(string(*engineConfig)).To(ContainSubstring("playbookPath"), "engineConfig should contain ansible-specific fields")

			cfg, err := models.ParseEngineConfig("ansible", *engineConfig)
			Expect(err).ToNot(HaveOccurred())
			ansibleCfg, ok := cfg.(*models.AnsibleEngineConfig)
			Expect(ok).To(BeTrue())
			Expect(ansibleCfg.PlaybookPath).To(Equal("playbooks/restart_pod.yml"))
			Expect(ansibleCfg.JobTemplateName).To(Equal("restart-pod"))
			Expect(ansibleCfg.InventoryName).To(Equal("production"))
		})

		It("UT-WE-016-005b: should return nil engineConfig for tekton schema", func() {
			yamlContent := testutil.MarshalWorkflowCRD(tektonWithBundleDigestCRD())
			parsedSchema, err := parser.Parse(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			engineConfig := parser.ExtractEngineConfig(parsedSchema)
			Expect(engineConfig).To(BeNil())
		})
	})

	Context("ExtractBundleDigest", func() {
		It("UT-WE-016-006: should extract explicit bundleDigest field", func() {
			yamlContent := testutil.MarshalWorkflowCRD(tektonWithBundleDigestCRD())
			parsedSchema, err := parser.Parse(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			digest := parser.ExtractBundleDigest(parsedSchema)
			Expect(*digest).To(Equal("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"))
		})

		It("UT-WE-016-006b: should extract digest from inline @sha256: in bundle URL", func() {
			yamlContent := testutil.MarshalWorkflowCRD(tektonInlineDigestCRD())
			parsedSchema, err := parser.Parse(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			digest := parser.ExtractBundleDigest(parsedSchema)
			Expect(*digest).To(Equal("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"))
		})

		It("UT-WE-016-006c: should extract bundleDigest for ansible schema (git commit SHA)", func() {
			yamlContent := testutil.MarshalWorkflowCRD(ansibleTestCRD())
			parsedSchema, err := parser.Parse(yamlContent)
			Expect(err).ToNot(HaveOccurred())

			digest := parser.ExtractBundleDigest(parsedSchema)
			Expect(*digest).To(Equal("abc123def456"))
		})
	})

	Context("Ansible validation in Validate()", func() {
		It("UT-WE-016-007: should reject ansible schema without engineConfig", func() {
			crd := ansibleTestCRD()
			crd.Spec.Execution.EngineConfig = nil
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())

			var validationErr *models.SchemaValidationError
			Expect(err).To(BeAssignableToTypeOf(validationErr))
			Expect(err.Error()).To(ContainSubstring("engineConfig"))
		})

		It("UT-WE-016-007b: should reject ansible schema with empty playbookPath", func() {
			crd := ansibleTestCRD()
			crd.Spec.Execution.EngineConfig = map[string]interface{}{
				"jobTemplateName": "some-template",
			}
			yamlContent := testutil.MarshalWorkflowCRD(crd)

			_, err := parser.ParseAndValidate(yamlContent)
			Expect(err).To(HaveOccurred())

			var validationErr *models.SchemaValidationError
			Expect(err).To(BeAssignableToTypeOf(validationErr))
			Expect(err.Error()).To(ContainSubstring("playbookPath"))
		})

		It("UT-WE-016-007c: should accept valid ansible schema with engineConfig", func() {
			yamlContent := testutil.MarshalWorkflowCRD(ansibleTestCRD())
			parsedSchema, err := parser.ParseAndValidate(yamlContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Execution.Engine).To(Equal("ansible"))
		})
	})
})
