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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// ENGINE CONFIG DISCRIMINATOR TESTS (BR-WE-016)
// ========================================
// Authority: BR-WE-016 (EngineConfig Discriminator Pattern)
// Authority: BR-WE-015 (Ansible Execution Engine)
// Test Plan: docs/testing/45/TEST_PLAN.md
// ========================================

var _ = Describe("ParseEngineConfig Discriminator [BR-WE-016]", func() {

	Context("ansible engine", func() {
		It("UT-WE-016-001: should return correctly populated AnsibleEngineConfig for engine=ansible", func() {
			raw := json.RawMessage(`{
				"playbookPath": "playbooks/restart_pod.yml",
				"jobTemplateName": "restart-pod-template",
				"inventoryName": "production"
			}`)

			result, err := models.ParseEngineConfig("ansible", raw)
			Expect(err).ToNot(HaveOccurred())

			cfg, ok := result.(*models.AnsibleEngineConfig)
			Expect(ok).To(BeTrue(), "expected *AnsibleEngineConfig, got %T", result)
			Expect(cfg.PlaybookPath).To(Equal("playbooks/restart_pod.yml"))
			Expect(cfg.JobTemplateName).To(Equal("restart-pod-template"))
			Expect(cfg.InventoryName).To(Equal("production"))
		})

		It("UT-WE-016-001b: should accept AnsibleEngineConfig with only required playbookPath", func() {
			raw := json.RawMessage(`{"playbookPath": "playbooks/scale.yml"}`)

			result, err := models.ParseEngineConfig("ansible", raw)
			Expect(err).ToNot(HaveOccurred())

			cfg, ok := result.(*models.AnsibleEngineConfig)
			Expect(ok).To(BeTrue())
			Expect(cfg.PlaybookPath).To(Equal("playbooks/scale.yml"))
			Expect(cfg.JobTemplateName).To(BeEmpty())
			Expect(cfg.InventoryName).To(BeEmpty())
		})
	})

	Context("tekton and job engines", func() {
		It("UT-WE-016-002: should return nil config for engine=tekton", func() {
			raw := json.RawMessage(`{"someField": "value"}`)

			result, err := models.ParseEngineConfig("tekton", raw)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("UT-WE-016-002b: should return nil config for engine=job", func() {
			raw := json.RawMessage(`{"someField": "value"}`)

			result, err := models.ParseEngineConfig("job", raw)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("UT-WE-016-002c: should return nil for tekton with empty engineConfig", func() {
			result, err := models.ParseEngineConfig("tekton", nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})

	Context("unknown engine", func() {
		It("UT-WE-016-003: should return descriptive error for unknown engine", func() {
			raw := json.RawMessage(`{"playbookPath": "test.yml"}`)

			_, err := models.ParseEngineConfig("lambda", raw)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("lambda"))
			Expect(err.Error()).To(ContainSubstring("unknown engine"))
		})
	})

	Context("AnsibleEngineConfig validation", func() {
		It("UT-WE-016-004: should reject ansible config with empty playbookPath", func() {
			raw := json.RawMessage(`{"jobTemplateName": "some-template"}`)

			_, err := models.ParseEngineConfig("ansible", raw)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("playbookPath"))
		})

		It("UT-WE-016-004b: should reject ansible config with invalid JSON", func() {
			raw := json.RawMessage(`{invalid json}`)

			_, err := models.ParseEngineConfig("ansible", raw)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ansible"))
		})

		It("UT-WE-016-004c: should return nil for ansible with empty engineConfig", func() {
			result, err := models.ParseEngineConfig("ansible", nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})
})
