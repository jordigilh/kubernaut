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
)

// ========================================
// SCHEMA PARSER ENGINE CONFIG EXTRACTION TESTS (BR-WE-016)
// ========================================
// Authority: BR-WE-016 (EngineConfig Discriminator Pattern)
// Authority: BR-WE-015 (Ansible Execution Engine)
// Test Plan: docs/testing/45/TEST_PLAN.md
// ========================================

const ansibleSchemaBaseYAML = `apiVersion: kubernaut.ai/v1alpha1
kind: RemediationWorkflow
metadata:
  name: ansible-test-workflow
spec:
  metadata:
    workflowName: ansible-test-workflow
    version: "1.0.0"
    description:
      what: Tests ansible engine config extraction
      whenToUse: When validating ansible workflow registration
      whenNotToUse: N/A
      preconditions: None
  actionType: RestartPod
  labels:
    signalType: OOMKilled
    severity: [critical]
    component: pod
    environment: [production]
    priority: P0
  parameters:
    - name: NAMESPACE
      type: string
      description: Target namespace
      required: true
`

const validAnsibleSchemaYAML = ansibleSchemaBaseYAML + `  execution:
    engine: ansible
    bundle: https://github.com/kubernaut/playbooks.git
    bundleDigest: abc123def456
    engineConfig:
      playbookPath: playbooks/restart_pod.yml
      jobTemplateName: restart-pod
      inventoryName: production
`

const ansibleSchemaNoEngineConfigYAML = ansibleSchemaBaseYAML + `  execution:
    engine: ansible
    bundle: https://github.com/kubernaut/playbooks.git
`

const ansibleSchemaEmptyPlaybookPathYAML = ansibleSchemaBaseYAML + `  execution:
    engine: ansible
    bundle: https://github.com/kubernaut/playbooks.git
    engineConfig:
      jobTemplateName: some-template
`

const tektonSchemaWithBundleDigestYAML = ansibleSchemaBaseYAML + `  execution:
    engine: tekton
    bundle: quay.io/kubernaut/workflows/scale@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
    bundleDigest: abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
`

const tektonSchemaInlineDigestYAML = ansibleSchemaBaseYAML + `  execution:
    engine: tekton
    bundle: quay.io/kubernaut/workflows/scale@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
`

var _ = Describe("Schema Parser EngineConfig Extraction [BR-WE-016]", func() {
	var parser *schema.Parser

	BeforeEach(func() {
		parser = schema.NewParser()
	})

	Context("ExtractEngineConfig", func() {
		It("UT-WE-016-005: should extract engineConfig from ansible workflow schema", func() {
			parsedSchema, err := parser.Parse(validAnsibleSchemaYAML)
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
			parsedSchema, err := parser.Parse(tektonSchemaWithBundleDigestYAML)
			Expect(err).ToNot(HaveOccurred())

			engineConfig := parser.ExtractEngineConfig(parsedSchema)
			Expect(engineConfig).To(BeNil())
		})
	})

	Context("ExtractBundleDigest", func() {
		It("UT-WE-016-006: should extract explicit bundleDigest field", func() {
			parsedSchema, err := parser.Parse(tektonSchemaWithBundleDigestYAML)
			Expect(err).ToNot(HaveOccurred())

			digest := parser.ExtractBundleDigest(parsedSchema)
			Expect(*digest).To(Equal("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"))
		})

		It("UT-WE-016-006b: should extract digest from inline @sha256: in bundle URL", func() {
			parsedSchema, err := parser.Parse(tektonSchemaInlineDigestYAML)
			Expect(err).ToNot(HaveOccurred())

			digest := parser.ExtractBundleDigest(parsedSchema)
			Expect(*digest).To(Equal("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"))
		})

		It("UT-WE-016-006c: should extract bundleDigest for ansible schema (git commit SHA)", func() {
			parsedSchema, err := parser.Parse(validAnsibleSchemaYAML)
			Expect(err).ToNot(HaveOccurred())

			digest := parser.ExtractBundleDigest(parsedSchema)
			Expect(*digest).To(Equal("abc123def456"))
		})
	})

	Context("Ansible validation in Validate()", func() {
		It("UT-WE-016-007: should reject ansible schema without engineConfig", func() {
			_, err := parser.ParseAndValidate(ansibleSchemaNoEngineConfigYAML)
			Expect(err).To(HaveOccurred())

			var validationErr *models.SchemaValidationError
			Expect(err).To(BeAssignableToTypeOf(validationErr))
			Expect(err.Error()).To(ContainSubstring("engineConfig"))
		})

		It("UT-WE-016-007b: should reject ansible schema with empty playbookPath", func() {
			_, err := parser.ParseAndValidate(ansibleSchemaEmptyPlaybookPathYAML)
			Expect(err).To(HaveOccurred())

			var validationErr *models.SchemaValidationError
			Expect(err).To(BeAssignableToTypeOf(validationErr))
			Expect(err.Error()).To(ContainSubstring("playbookPath"))
		})

		It("UT-WE-016-007c: should accept valid ansible schema with engineConfig", func() {
			parsedSchema, err := parser.ParseAndValidate(validAnsibleSchemaYAML)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsedSchema.Execution.Engine).To(Equal("ansible"))
		})
	})
})
