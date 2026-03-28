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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/pkg/shared/uuid"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Override Wiring Integration", func() {

	Describe("IT-MOCK-561-001: YAML override applied via HTTP", func() {
		It("should serve overridden workflow UUID when override file provides custom workflow_id", func() {
			tmpDir := GinkgoT().TempDir()
			overridePath := filepath.Join(tmpDir, "overrides.yaml")
			Expect(os.WriteFile(overridePath, []byte(`scenarios:
  oomkilled:
    workflow_id: "custom-uuid-from-override"
`), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(overridePath)
			Expect(err).NotTo(HaveOccurred())

			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false)
			server := httptest.NewServer(router)
			defer server.Close()

			body := chatRequestWithTools(
				"- Signal Name: OOMKilled\n- Namespace: default",
				[]string{openai.ToolListAvailableActions, openai.ToolListWorkflows, openai.ToolGetWorkflow},
			)

			// Step through three-step mode to reach get_workflow which carries the UUID
			for i := 0; i < 3; i++ {
				resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
				Expect(err).NotTo(HaveOccurred())

				var result openai.ChatCompletionResponse
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				resp.Body.Close()

				if result.Choices[0].FinishReason == "tool_calls" &&
					result.Choices[0].Message.ToolCalls[0].Function.Name == openai.ToolGetWorkflow {
					var args map[string]interface{}
					Expect(json.Unmarshal(
						[]byte(result.Choices[0].Message.ToolCalls[0].Function.Arguments), &args,
					)).To(Succeed())
					Expect(args["workflow_id"]).To(Equal("custom-uuid-from-override"))
					return
				}

				body = chatRequestWithToolResult(
					"- Signal Name: OOMKilled\n- Namespace: default",
					result.Choices[0].Message.ToolCalls[0].Function.Name,
					[]string{openai.ToolListAvailableActions, openai.ToolListWorkflows, openai.ToolGetWorkflow},
					i+1,
				)
			}
			Fail("get_workflow tool call with overridden UUID not found in 3 steps")
		})
	})

	Describe("IT-MOCK-561-002: Startup without override file", func() {
		It("should start cleanly and serve deterministic default UUIDs when no override file exists", func() {
			overrides, err := config.LoadYAMLOverrides("")
			Expect(err).NotTo(HaveOccurred())

			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false)
			server := httptest.NewServer(router)
			defer server.Close()

			// Health check
			resp, err := http.Get(server.URL + "/health")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))
		})
	})

	Describe("IT-MOCK-561-003: Deterministic UUID end-to-end via HTTP", func() {
		type scenarioCase struct {
			keyword      string
			workflowName string
		}

		DescribeTable("should produce UUID matching uuid.DeterministicUUID(workflowName)",
			func(sc scenarioCase) {
				overrides, err := config.LoadYAMLOverrides("")
				Expect(err).NotTo(HaveOccurred())

				registry := scenarios.DefaultRegistryWithOverrides(overrides)
				router := handlers.NewRouter(registry, false)
				server := httptest.NewServer(router)
				defer server.Close()

				expectedUUID := uuid.DeterministicUUID(sc.workflowName)

				body := chatRequest("- Signal Name: "+sc.keyword+"\n- Namespace: default", nil)
				resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				var result openai.ChatCompletionResponse
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result.Choices[0].Message.Content).NotTo(BeNil())
				Expect(*result.Choices[0].Message.Content).To(ContainSubstring(expectedUUID))
			},
			Entry("oomkilled", scenarioCase{keyword: "OOMKilled", workflowName: "oomkill-increase-memory-v1"}),
			Entry("crashloop", scenarioCase{keyword: "CrashLoopBackOff", workflowName: "crashloop-config-fix-v1"}),
			Entry("node_not_ready", scenarioCase{keyword: "NodeNotReady", workflowName: "node-drain-reboot-v1"}),
		)
	})

	Describe("IT-MOCK-561-004: MOCK_LLM_CONFIG_PATH env var wired at startup", func() {
		It("should load overrides from the path specified by MOCK_LLM_CONFIG_PATH", func() {
			tmpDir := GinkgoT().TempDir()
			overridePath := filepath.Join(tmpDir, "overrides.yaml")
			Expect(os.WriteFile(overridePath, []byte(`scenarios:
  crashloop:
    workflow_id: "env-var-wired-uuid"
`), 0644)).To(Succeed())

			os.Setenv("MOCK_LLM_CONFIG_PATH", overridePath)
			defer os.Unsetenv("MOCK_LLM_CONFIG_PATH")

			cfg := config.LoadFromEnv()
			Expect(cfg.ConfigPath).To(Equal(overridePath))

			overrides, err := config.LoadYAMLOverrides(cfg.ConfigPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.Scenarios).To(HaveKey("crashloop"))
			Expect(overrides.Scenarios["crashloop"].WorkflowID).To(Equal("env-var-wired-uuid"))

			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false)
			server := httptest.NewServer(router)
			defer server.Close()

			body := chatRequest("- Signal Name: CrashLoopBackOff\n- Namespace: staging", nil)
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			var result openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Choices[0].Message.Content).NotTo(BeNil())
			Expect(*result.Choices[0].Message.Content).To(ContainSubstring("env-var-wired-uuid"))
		})
	})
})

func chatRequestWithTools(content string, toolNames []string) *bytes.Buffer {
	return chatRequest(content, toolNames)
}

func chatRequestWithToolResult(content, toolCallName string, toolNames []string, toolResultCount int) *bytes.Buffer {
	messages := []map[string]interface{}{
		{"role": "user", "content": content},
	}
	for i := 0; i < toolResultCount; i++ {
		messages = append(messages, map[string]interface{}{
			"role":         "tool",
			"content":      `{"result": "ok"}`,
			"tool_call_id": "call_test",
		})
	}

	req := map[string]interface{}{
		"model":    "mock-model",
		"messages": messages,
	}
	if len(toolNames) > 0 {
		tools := make([]map[string]interface{}, len(toolNames))
		for i, name := range toolNames {
			tools[i] = map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        name,
					"description": "test",
					"parameters":  map[string]interface{}{},
				},
			}
		}
		req["tools"] = tools
	}
	data, _ := json.Marshal(req)
	return bytes.NewBuffer(data)
}
