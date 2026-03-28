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
package mockllm_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
)

var _ = Describe("YAML Scenario Overrides", func() {

	Describe("UT-MOCK-033-001: YAML override merges on top of deterministic defaults", func() {
		It("should apply overrides from YAML file, leaving non-overridden scenarios unchanged", func() {
			yamlContent := `scenarios:
  oomkilled:
    workflow_id: "custom-uuid-from-yaml"
    confidence: 0.99
`
			tmpDir := GinkgoT().TempDir()
			yamlPath := filepath.Join(tmpDir, "overrides.yaml")
			Expect(os.WriteFile(yamlPath, []byte(yamlContent), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(yamlPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.Scenarios).To(HaveKey("oomkilled"))
			Expect(overrides.Scenarios["oomkilled"].WorkflowID).To(Equal("custom-uuid-from-yaml"))
			Expect(*overrides.Scenarios["oomkilled"].Confidence).To(BeNumerically("~", 0.99))

			Expect(overrides.Scenarios).NotTo(HaveKey("crashloop_config_fix"),
				"Non-overridden scenarios should not appear in overrides map")
		})
	})

	Describe("UT-MOCK-033-002: Missing YAML file falls back gracefully", func() {
		It("should return empty overrides and no error for non-existent file", func() {
			overrides, err := config.LoadYAMLOverrides("/nonexistent/path/overrides.yaml")
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.Scenarios).To(BeEmpty())
		})

		It("should return empty overrides and no error for empty path", func() {
			overrides, err := config.LoadYAMLOverrides("")
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.Scenarios).To(BeEmpty())
		})
	})
})
