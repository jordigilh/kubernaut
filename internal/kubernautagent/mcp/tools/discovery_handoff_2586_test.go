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

package tools_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	mockconfig "github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

const discoveryHandoff2586WorkflowID = "ab14b63c-edf0-52db-885a-807b591e4168"

var _ = Describe("Issue #2586 discovery-to-selection handoff", func() {
	It("IT-KA-2586-001 [BR-KA-017, BR-AI-086]: preserves the exact YAML selector workflow and parameters through submit, parsing, and extraction", func() {
		selectorYAML := `
scenario_selectors:
  - name: "ka_gitops_workflow_discovery_2390"
    caller: "ka"
    phase: "workflow_discovery"
    signal_patterns: ["FullPipelineA2ASeverityGrounding_fp_int_"]
    workflow_id: "` + discoveryHandoff2586WorkflowID + `"
    action_type: "IncreaseMemoryLimits"
    parameters:
      MEMORY_LIMIT_NEW: "512Mi"
    tool_call:
      name: "list_available_actions"
`
		selectorPath := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
		Expect(os.WriteFile(selectorPath, []byte(selectorYAML), 0644)).To(Succeed())
		overrides, err := mockconfig.LoadYAMLOverrides(selectorPath)
		Expect(err).NotTo(HaveOccurred())
		registry := scenarios.DefaultRegistryFull(overrides, "")
		server := httptest.NewServer(handlers.NewRouter(registry, false, mockconfig.ModeFull))
		defer server.Close()

		messages := []openai.Message{{
			Role:    "user",
			Content: discoveryHandoff2586StringPtr("Signal Name: FullPipelineA2ASeverityGrounding_fp_int_3e56c0e0"),
		}}
		tools := discoveryHandoff2586Tools()
		for _, step := range []struct {
			tool   string
			result string
		}{
			{openai.ToolListAvailableActions, `{"actions":[]}`},
			{openai.ToolListWorkflows, `{"workflows":[{"workflow_id":"` + discoveryHandoff2586WorkflowID + `"}]}`},
			{openai.ToolGetWorkflow, `{"workflow_id":"` + discoveryHandoff2586WorkflowID + `"}`},
		} {
			response := postDiscoveryHandoff2586(server.URL, openai.ChatCompletionRequest{
				Model: "mock-model", Messages: messages, Tools: tools,
			})
			Expect(response.Choices[0].Message.ToolCalls).To(HaveLen(1))
			Expect(response.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal(step.tool))
			appendDiscoveryHandoff2586ToolResult(&messages, response, step.result)
		}

		final := postDiscoveryHandoff2586(server.URL, openai.ChatCompletionRequest{
			Model: "mock-model", Messages: messages, Tools: tools,
		})
		Expect(final.Choices[0].Message.ToolCalls).To(HaveLen(1))
		call := final.Choices[0].Message.ToolCalls[0]
		Expect(call.Function.Name).To(Equal(openai.ToolSubmitResultWithWorkflow))

		var payload map[string]interface{}
		Expect(json.Unmarshal([]byte(call.Function.Arguments), &payload)).To(Succeed())
		selected, ok := payload["selected_workflow"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(selected).To(HaveKeyWithValue("workflow_id", discoveryHandoff2586WorkflowID))
		Expect(selected).To(HaveKeyWithValue("parameters", map[string]interface{}{"MEMORY_LIMIT_NEW": "512Mi"}))

		result, err := parser.NewResultParser().Parse(call.Function.Arguments)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.WorkflowID).To(Equal(discoveryHandoff2586WorkflowID))
		Expect(result.Parameters).To(HaveKeyWithValue("MEMORY_LIMIT_NEW", "512Mi"))

		discovery := mcptools.ExtractDiscoveryResult(result)
		Expect(discovery.Recommended).NotTo(BeNil())
		Expect(discovery.Recommended.WorkflowID).To(Equal(discoveryHandoff2586WorkflowID))
		Expect(discovery.Recommended.Parameters).To(HaveKeyWithValue("MEMORY_LIMIT_NEW", "512Mi"))
	})
})

func postDiscoveryHandoff2586(serverURL string, request openai.ChatCompletionRequest) openai.ChatCompletionResponse {
	body, err := json.Marshal(request)
	Expect(err).NotTo(HaveOccurred())
	response, err := http.Post(serverURL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	defer response.Body.Close()
	Expect(response.StatusCode).To(Equal(http.StatusOK))

	var result openai.ChatCompletionResponse
	Expect(json.NewDecoder(response.Body).Decode(&result)).To(Succeed())
	return result
}

func appendDiscoveryHandoff2586ToolResult(messages *[]openai.Message, result openai.ChatCompletionResponse, payload string) {
	assistant := result.Choices[0].Message
	*messages = append(*messages, assistant, openai.Message{
		Role:       "tool",
		ToolCallID: assistant.ToolCalls[0].ID,
		Content:    discoveryHandoff2586StringPtr(payload),
	})
}

func discoveryHandoff2586StringPtr(value string) *string {
	return &value
}

func discoveryHandoff2586Tools() []openai.Tool {
	return []openai.Tool{
		{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolListAvailableActions}},
		{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolListWorkflows}},
		{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolGetWorkflow}},
		{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolSubmitResultWithWorkflow}},
	}
}
