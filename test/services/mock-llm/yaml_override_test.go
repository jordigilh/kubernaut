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

	Describe("UT-MOCK-657-001: YAML forceText:false parses to *bool(false)", func() {
		It("should parse forceText: false into a *bool pointer to false", func() {
			yamlContent := `scenarios:
  injection_configmap_read:
    workflow_id: "inject-wf-001"
    force_text: false
`
			tmpDir := GinkgoT().TempDir()
			yamlPath := filepath.Join(tmpDir, "overrides.yaml")
			Expect(os.WriteFile(yamlPath, []byte(yamlContent), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(yamlPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.Scenarios).To(HaveKey("injection_configmap_read"))

			scenario := overrides.Scenarios["injection_configmap_read"]
			Expect(scenario.ForceText).NotTo(BeNil(), "ForceText should be non-nil for explicit false")
			Expect(*scenario.ForceText).To(BeFalse())
		})
	})

	Describe("UT-MOCK-657-002: YAML toolCall config parses tool name and arguments", func() {
		It("should parse toolCall with name and arguments", func() {
			yamlContent := `scenarios:
  injection_configmap_read:
    workflow_id: "inject-wf-001"
    tool_call:
      name: "kubectl_get_yaml"
      arguments:
        kind: "ConfigMap"
        name: "poisoned-cm"
        namespace: "default"
`
			tmpDir := GinkgoT().TempDir()
			yamlPath := filepath.Join(tmpDir, "overrides.yaml")
			Expect(os.WriteFile(yamlPath, []byte(yamlContent), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(yamlPath)
			Expect(err).NotTo(HaveOccurred())

			scenario := overrides.Scenarios["injection_configmap_read"]
			Expect(scenario.ToolCall).NotTo(BeNil(), "ToolCall should be non-nil")
			Expect(scenario.ToolCall.Name).To(Equal("kubectl_get_yaml"))
			Expect(scenario.ToolCall.Arguments).To(HaveKeyWithValue("kind", "ConfigMap"))
			Expect(scenario.ToolCall.Arguments).To(HaveKeyWithValue("name", "poisoned-cm"))
			Expect(scenario.ToolCall.Arguments).To(HaveKeyWithValue("namespace", "default"))
		})
	})

	Describe("UT-MOCK-657-011: E2E ConfigMap YAML with mixed workflow + injection entries parses correctly", func() {
		It("should parse the exact YAML structure produced by deployMockLLMInNamespace", func() {
			yamlContent := `scenarios:
      oomkill-increase-memory-v1:production:
        workflow_id: "uuid-oomkill"
      crashloop-config-fix-v1:production:
        workflow_id: "uuid-crashloop"
      injection_configmap_read:
        force_text: false
        tool_call:
          name: kubectl_get_yaml
          arguments:
            kind: ConfigMap
            name: poisoned-cm
            namespace: kubernaut-agent-e2e
`
			tmpDir := GinkgoT().TempDir()
			yamlPath := filepath.Join(tmpDir, "overrides.yaml")
			Expect(os.WriteFile(yamlPath, []byte(yamlContent), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(yamlPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.Scenarios).To(HaveLen(3))

			Expect(overrides.Scenarios).To(HaveKey("oomkill-increase-memory-v1:production"))
			Expect(overrides.Scenarios["oomkill-increase-memory-v1:production"].WorkflowID).To(Equal("uuid-oomkill"))

			injection := overrides.Scenarios["injection_configmap_read"]
			Expect(injection.ForceText).NotTo(BeNil())
			Expect(*injection.ForceText).To(BeFalse())
			Expect(injection.ToolCall).NotTo(BeNil())
			Expect(injection.ToolCall.Name).To(Equal("kubectl_get_yaml"))
			Expect(injection.ToolCall.Arguments).To(HaveKeyWithValue("namespace", "kubernaut-agent-e2e"))
		})
	})

	Describe("UT-MOCK-657-012: E2E ConfigMap YAML with zero workflows + injection parses correctly", func() {
		It("should parse when injection is the only scenario entry", func() {
			yamlContent := `scenarios:
      injection_configmap_read:
        force_text: false
        tool_call:
          name: kubectl_get_yaml
          arguments:
            kind: ConfigMap
            name: poisoned-cm
            namespace: test-ns
`
			tmpDir := GinkgoT().TempDir()
			yamlPath := filepath.Join(tmpDir, "overrides.yaml")
			Expect(os.WriteFile(yamlPath, []byte(yamlContent), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(yamlPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.Scenarios).To(HaveLen(1))

			injection := overrides.Scenarios["injection_configmap_read"]
			Expect(injection.ForceText).NotTo(BeNil())
			Expect(*injection.ForceText).To(BeFalse())
			Expect(injection.ToolCall).NotTo(BeNil())
			Expect(injection.ToolCall.Name).To(Equal("kubectl_get_yaml"))
			Expect(injection.ToolCall.Arguments).To(HaveKeyWithValue("kind", "ConfigMap"))
			Expect(injection.ToolCall.Arguments).To(HaveKeyWithValue("name", "poisoned-cm"))
			Expect(injection.ToolCall.Arguments).To(HaveKeyWithValue("namespace", "test-ns"))
		})
	})

	Describe("UT-MOCK-657-005: Missing forceText in YAML leaves nil", func() {
		It("should leave ForceText nil when not specified in YAML", func() {
			yamlContent := `scenarios:
  oomkilled:
    workflow_id: "custom-uuid"
    confidence: 0.95
`
			tmpDir := GinkgoT().TempDir()
			yamlPath := filepath.Join(tmpDir, "overrides.yaml")
			Expect(os.WriteFile(yamlPath, []byte(yamlContent), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(yamlPath)
			Expect(err).NotTo(HaveOccurred())

			scenario := overrides.Scenarios["oomkilled"]
			Expect(scenario.ForceText).To(BeNil(), "ForceText should be nil when not specified")
			Expect(scenario.ToolCall).To(BeNil(), "ToolCall should be nil when not specified")
		})
	})
})
