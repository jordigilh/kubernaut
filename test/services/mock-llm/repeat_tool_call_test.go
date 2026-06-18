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
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Multi-Turn RepeatToolCall and Template Resolution (issue #1258)", func() {

	Describe("UT-ML-1258-001: RepeatToolCall YAML parsing", func() {
		It("should parse repeat_tool_call from YAML config", func() {
			yamlContent := `
keyword_scenarios:
  - name: "af_investigate_resume"
    keywords: ["stream the investigation"]
    match_last_only: true
    repeat_tool_call: true
    tool_call:
      name: "kubernaut_investigate"
      arguments:
        session_id: "$from_tool:kubernaut_investigate:session_id"
  - name: "af_investigate"
    keywords: ["start investigation"]
    match_last_only: true
    tool_call:
      name: "kubernaut_investigate"
`
			tmpFile := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
			Expect(os.WriteFile(tmpFile, []byte(yamlContent), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(tmpFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.KeywordScenarios).To(HaveLen(2))
			Expect(overrides.KeywordScenarios[0].RepeatToolCall).To(BeTrue())
			Expect(overrides.KeywordScenarios[1].RepeatToolCall).To(BeFalse())
		})
	})

	Describe("UT-ML-1258-002: RepeatToolCall wired into MockScenarioConfig", func() {
		It("should set RepeatToolCall on scenario config via registry", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:           "af_investigate_resume",
						Keywords:       []string{"stream the investigation"},
						ToolCall:       config.ToolCallOverride{Name: "kubernaut_investigate"},
						MatchLastOnly:  true,
						RepeatToolCall: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)

			detCtx := &scenarios.DetectionContext{
				Content:         "stream the investigation",
				AllText:         "user stream the investigation",
				LastUserContent: "stream the investigation",
			}
			result := registry.Detect(detCtx)
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("af_investigate_resume"))

			scenarioWithCfg, ok := result.Scenario.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue())
			cfg := scenarioWithCfg.Config()
			Expect(cfg.RepeatToolCall).To(BeTrue())
			Expect(cfg.ToolCallName).To(Equal("kubernaut_investigate"))
		})
	})

	Describe("UT-ML-1258-003: Gemini handler emits tool call with RepeatToolCall despite FunctionResponse", func() {
		It("should return tool call on second turn when repeat_tool_call is true", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:           "af_investigate_resume",
						Keywords:       []string{"stream the investigation"},
						ToolCall:       config.ToolCallOverride{Name: "kubernaut_investigate", Arguments: map[string]interface{}{"session_id": "sess-abc"}},
						MatchLastOnly:  true,
						RepeatToolCall: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := response.GeminiRequest{
				Contents: []response.GeminiContent{
					{Role: "user", Parts: []response.GeminiPart{{Text: "start investigation"}}},
					{Role: "model", Parts: []response.GeminiPart{{FunctionCall: &response.GeminiFunctionCall{Name: "kubernaut_investigate", Args: map[string]interface{}{"namespace": "default"}}}}},
					{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{Name: "kubernaut_investigate", Response: map[string]interface{}{"session_id": "sess-abc", "status": "started"}}}}},
					{Role: "user", Parts: []response.GeminiPart{{Text: "stream the investigation"}}},
				},
				Tools: []response.GeminiToolDecl{
					{FunctionDeclarations: []response.GeminiFunctionDecl{{Name: "kubernaut_investigate"}}},
				},
			}

			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1beta/models/gemini-1.5-pro:generateContent", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var gemResp response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&gemResp)).To(Succeed())
			Expect(gemResp.Candidates).To(HaveLen(1))

			parts := gemResp.Candidates[0].Content.Parts
			Expect(parts).To(HaveLen(1))
			Expect(parts[0].FunctionCall).NotTo(BeNil())
			Expect(parts[0].FunctionCall.Name).To(Equal("kubernaut_investigate"))
			Expect(parts[0].FunctionCall.Args).To(HaveKeyWithValue("session_id", "sess-abc"))
		})
	})

	Describe("UT-ML-1258-004: Gemini handler does NOT emit tool call without RepeatToolCall", func() {
		It("should fall through to text response on second turn without repeat_tool_call", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:          "af_investigate_resume",
						Keywords:      []string{"stream the investigation"},
						ToolCall:      config.ToolCallOverride{Name: "kubernaut_investigate", Arguments: map[string]interface{}{"session_id": "sess-abc"}},
						MatchLastOnly: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := response.GeminiRequest{
				Contents: []response.GeminiContent{
					{Role: "user", Parts: []response.GeminiPart{{Text: "start investigation"}}},
					{Role: "model", Parts: []response.GeminiPart{{FunctionCall: &response.GeminiFunctionCall{Name: "kubernaut_investigate", Args: map[string]interface{}{"namespace": "default"}}}}},
					{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{Name: "kubernaut_investigate", Response: map[string]interface{}{"session_id": "sess-abc"}}}}},
					{Role: "user", Parts: []response.GeminiPart{{Text: "stream the investigation"}}},
				},
				Tools: []response.GeminiToolDecl{
					{FunctionDeclarations: []response.GeminiFunctionDecl{{Name: "kubernaut_investigate"}}},
				},
			}

			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1beta/models/gemini-1.5-pro:generateContent", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var gemResp response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&gemResp)).To(Succeed())
			Expect(gemResp.Candidates).To(HaveLen(1))

			parts := gemResp.Candidates[0].Content.Parts
			Expect(parts).To(HaveLen(1))
			Expect(parts[0].FunctionCall).To(BeNil(), "should NOT emit tool call without RepeatToolCall")
			Expect(parts[0].Text).NotTo(BeEmpty())
		})
	})

	Describe("UT-ML-1258-005: OpenAI handler emits tool call with RepeatToolCall despite tool results", func() {
		It("should return tool call on second turn when repeat_tool_call is true", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:           "af_investigate_resume",
						Keywords:       []string{"stream the investigation"},
						ToolCall:       config.ToolCallOverride{Name: "kubernaut_investigate", Arguments: map[string]interface{}{"session_id": "sess-abc"}},
						MatchLastOnly:  true,
						RepeatToolCall: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			content := func(s string) *string { return &s }
			reqBody := openai.ChatCompletionRequest{
				Model: "gpt-4",
				Messages: []openai.Message{
					{Role: "user", Content: content("start investigation")},
					{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_1", Type: "function", Function: openai.FunctionCall{Name: "kubernaut_investigate", Arguments: `{"namespace":"default"}`}}}},
					{Role: "tool", Content: content(`{"session_id":"sess-abc","status":"started"}`)},
					{Role: "user", Content: content("stream the investigation")},
				},
				Tools: []openai.Tool{{Type: "function", Function: openai.ToolDefinition{Name: "kubernaut_investigate"}}},
			}

			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var oaiResp openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&oaiResp)).To(Succeed())
			Expect(oaiResp.Choices).To(HaveLen(1))
			Expect(oaiResp.Choices[0].Message.ToolCalls).To(HaveLen(1))
			Expect(oaiResp.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal("kubernaut_investigate"))
		})
	})

	Describe("UT-ML-1258-006: ExtractFieldFromFunctionResponse", func() {
		It("should extract a string field from matching FunctionResponse", func() {
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{
					Name:     "kubernaut_investigate",
					Response: map[string]interface{}{"session_id": "sess-xyz", "status": "started"},
				}}}},
			}
			val := response.ExtractFieldFromFunctionResponse(contents, "kubernaut_investigate", "session_id")
			Expect(val).To(Equal("sess-xyz"))
		})

		It("should return empty string when tool name does not match", func() {
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{
					Name:     "other_tool",
					Response: map[string]interface{}{"session_id": "sess-xyz"},
				}}}},
			}
			val := response.ExtractFieldFromFunctionResponse(contents, "kubernaut_investigate", "session_id")
			Expect(val).To(BeEmpty())
		})

		It("should return empty string when field is absent", func() {
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{
					Name:     "kubernaut_investigate",
					Response: map[string]interface{}{"status": "started"},
				}}}},
			}
			val := response.ExtractFieldFromFunctionResponse(contents, "kubernaut_investigate", "session_id")
			Expect(val).To(BeEmpty())
		})

		It("should handle nil response gracefully", func() {
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{
					Name:     "kubernaut_investigate",
					Response: nil,
				}}}},
			}
			val := response.ExtractFieldFromFunctionResponse(contents, "kubernaut_investigate", "session_id")
			Expect(val).To(BeEmpty())
		})
	})

	Describe("UT-ML-1258-007: ExtractFieldFromToolResult (OpenAI)", func() {
		content := func(s string) *string { return &s }

		It("should extract field from tool result matching function name", func() {
			messages := []openai.Message{
				{Role: "user", Content: content("start investigation")},
				{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_1", Type: "function", Function: openai.FunctionCall{Name: "kubernaut_investigate", Arguments: `{"ns":"default"}`}}}},
				{Role: "tool", Content: content(`{"session_id":"sess-123","status":"running"}`)},
			}
			val := response.ExtractFieldFromToolResult(messages, "kubernaut_investigate", "session_id")
			Expect(val).To(Equal("sess-123"))
		})

		It("should return empty string when function name does not match", func() {
			messages := []openai.Message{
				{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_1", Type: "function", Function: openai.FunctionCall{Name: "other_tool", Arguments: `{}`}}}},
				{Role: "tool", Content: content(`{"session_id":"sess-123"}`)},
			}
			val := response.ExtractFieldFromToolResult(messages, "kubernaut_investigate", "session_id")
			Expect(val).To(BeEmpty())
		})

		It("should return empty string when field is missing from result", func() {
			messages := []openai.Message{
				{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_1", Type: "function", Function: openai.FunctionCall{Name: "kubernaut_investigate", Arguments: `{}`}}}},
				{Role: "tool", Content: content(`{"status":"done"}`)},
			}
			val := response.ExtractFieldFromToolResult(messages, "kubernaut_investigate", "session_id")
			Expect(val).To(BeEmpty())
		})

		It("should handle multiple tool calls and match correct one", func() {
			messages := []openai.Message{
				{Role: "assistant", ToolCalls: []openai.ToolCall{
					{ID: "call_1", Type: "function", Function: openai.FunctionCall{Name: "tool_a", Arguments: `{}`}},
					{ID: "call_2", Type: "function", Function: openai.FunctionCall{Name: "kubernaut_investigate", Arguments: `{}`}},
				}},
				{Role: "tool", Content: content(`{"result":"from_a"}`)},
				{Role: "tool", Content: content(`{"session_id":"sess-multi"}`)},
			}
			val := response.ExtractFieldFromToolResult(messages, "kubernaut_investigate", "session_id")
			Expect(val).To(Equal("sess-multi"))
		})
	})

	Describe("UT-ML-1258-008: Gemini $from_tool template resolution end-to-end", func() {
		It("should resolve $from_tool template from prior FunctionResponse in multi-turn request", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:           "af_investigate_resume",
						Keywords:       []string{"stream the investigation"},
						ToolCall:       config.ToolCallOverride{Name: "kubernaut_investigate", Arguments: map[string]interface{}{"session_id": "$from_tool:kubernaut_investigate:session_id"}},
						MatchLastOnly:  true,
						RepeatToolCall: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := response.GeminiRequest{
				Contents: []response.GeminiContent{
					{Role: "user", Parts: []response.GeminiPart{{Text: "start investigation"}}},
					{Role: "model", Parts: []response.GeminiPart{{FunctionCall: &response.GeminiFunctionCall{Name: "kubernaut_investigate", Args: map[string]interface{}{"namespace": "default"}}}}},
					{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{Name: "kubernaut_investigate", Response: map[string]interface{}{"session_id": "sess-dynamic-456", "status": "started"}}}}},
					{Role: "user", Parts: []response.GeminiPart{{Text: "stream the investigation"}}},
				},
				Tools: []response.GeminiToolDecl{
					{FunctionDeclarations: []response.GeminiFunctionDecl{{Name: "kubernaut_investigate"}}},
				},
			}

			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1beta/models/gemini-1.5-pro:generateContent", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var gemResp response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&gemResp)).To(Succeed())
			Expect(gemResp.Candidates).To(HaveLen(1))

			parts := gemResp.Candidates[0].Content.Parts
			Expect(parts).To(HaveLen(1))
			Expect(parts[0].FunctionCall).NotTo(BeNil())
			Expect(parts[0].FunctionCall.Name).To(Equal("kubernaut_investigate"))
			Expect(parts[0].FunctionCall.Args).To(HaveKeyWithValue("session_id", "sess-dynamic-456"))
		})

		It("should leave unresolvable template as-is when tool result is missing", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:           "af_investigate_resume",
						Keywords:       []string{"stream the investigation"},
						ToolCall:       config.ToolCallOverride{Name: "kubernaut_investigate", Arguments: map[string]interface{}{"session_id": "$from_tool:kubernaut_investigate:session_id"}},
						MatchLastOnly:  true,
						RepeatToolCall: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := response.GeminiRequest{
				Contents: []response.GeminiContent{
					{Role: "user", Parts: []response.GeminiPart{{Text: "stream the investigation"}}},
				},
				Tools: []response.GeminiToolDecl{
					{FunctionDeclarations: []response.GeminiFunctionDecl{{Name: "kubernaut_investigate"}}},
				},
			}

			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1beta/models/gemini-1.5-pro:generateContent", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var gemResp response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&gemResp)).To(Succeed())
			Expect(gemResp.Candidates).To(HaveLen(1))

			parts := gemResp.Candidates[0].Content.Parts
			Expect(parts).To(HaveLen(1))
			Expect(parts[0].FunctionCall).NotTo(BeNil())
			Expect(parts[0].FunctionCall.Args).To(HaveKeyWithValue("session_id", "$from_tool:kubernaut_investigate:session_id"))
		})
	})

	Describe("UT-ML-1258-009: OpenAI $from_tool template resolution end-to-end", func() {
		It("should resolve $from_tool template from prior tool result in multi-turn request", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:           "af_investigate_resume",
						Keywords:       []string{"stream the investigation"},
						ToolCall:       config.ToolCallOverride{Name: "kubernaut_investigate", Arguments: map[string]interface{}{"session_id": "$from_tool:kubernaut_investigate:session_id"}},
						MatchLastOnly:  true,
						RepeatToolCall: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			content := func(s string) *string { return &s }
			reqBody := openai.ChatCompletionRequest{
				Model: "gpt-4",
				Messages: []openai.Message{
					{Role: "user", Content: content("start investigation")},
					{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_1", Type: "function", Function: openai.FunctionCall{Name: "kubernaut_investigate", Arguments: `{"namespace":"default"}`}}}},
					{Role: "tool", Content: content(`{"session_id":"sess-openai-789","status":"started"}`)},
					{Role: "user", Content: content("stream the investigation")},
				},
				Tools: []openai.Tool{{Type: "function", Function: openai.ToolDefinition{Name: "kubernaut_investigate"}}},
			}

			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var oaiResp openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&oaiResp)).To(Succeed())
			Expect(oaiResp.Choices).To(HaveLen(1))
			Expect(oaiResp.Choices[0].Message.ToolCalls).To(HaveLen(1))
			Expect(oaiResp.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal("kubernaut_investigate"))

			var args map[string]interface{}
			Expect(json.Unmarshal([]byte(oaiResp.Choices[0].Message.ToolCalls[0].Function.Arguments), &args)).To(Succeed())
			Expect(args).To(HaveKeyWithValue("session_id", "sess-openai-789"))
		})
	})

	// ===================================================================
	// FedRAMP SI-10 (Input Validation): Template parsing boundary behavior
	// Verifies that malformed or adversarial $from_tool templates are handled
	// safely without crashing, leaking state, or producing invalid tool calls.
	// ===================================================================

	Describe("UT-ML-1258-010: SI-10 malformed template with missing field segment (Gemini)", func() {
		It("should leave malformed template literal unchanged in emitted args", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:           "af_malformed",
						Keywords:       []string{"malformed test"},
						ToolCall:       config.ToolCallOverride{Name: "test_tool", Arguments: map[string]interface{}{"id": "$from_tool:onlytoolname"}},
						MatchLastOnly:  true,
						RepeatToolCall: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := response.GeminiRequest{
				Contents: []response.GeminiContent{
					{Role: "user", Parts: []response.GeminiPart{{Text: "malformed test"}}},
				},
				Tools: []response.GeminiToolDecl{
					{FunctionDeclarations: []response.GeminiFunctionDecl{{Name: "test_tool"}}},
				},
			}

			body, _ := json.Marshal(reqBody)
			resp, err := http.Post(ts.URL+"/v1beta/models/gemini-1.5-pro:generateContent", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			var gemResp response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&gemResp)).To(Succeed())
			parts := gemResp.Candidates[0].Content.Parts
			Expect(parts[0].FunctionCall).NotTo(BeNil())
			Expect(parts[0].FunctionCall.Args).To(HaveKeyWithValue("id", "$from_tool:onlytoolname"))
		})
	})

	Describe("UT-ML-1258-011: SI-10 malformed template with missing field segment (OpenAI)", func() {
		It("should leave malformed template literal unchanged in emitted args", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:           "af_malformed",
						Keywords:       []string{"malformed test"},
						ToolCall:       config.ToolCallOverride{Name: "test_tool", Arguments: map[string]interface{}{"id": "$from_tool:onlytoolname"}},
						MatchLastOnly:  true,
						RepeatToolCall: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			content := func(s string) *string { return &s }
			reqBody := openai.ChatCompletionRequest{
				Model:    "gpt-4",
				Messages: []openai.Message{{Role: "user", Content: content("malformed test")}},
				Tools:    []openai.Tool{{Type: "function", Function: openai.ToolDefinition{Name: "test_tool"}}},
			}

			body, _ := json.Marshal(reqBody)
			resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			var oaiResp openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&oaiResp)).To(Succeed())
			Expect(oaiResp.Choices[0].Message.ToolCalls).To(HaveLen(1))

			var args map[string]interface{}
			Expect(json.Unmarshal([]byte(oaiResp.Choices[0].Message.ToolCalls[0].Function.Arguments), &args)).To(Succeed())
			Expect(args).To(HaveKeyWithValue("id", "$from_tool:onlytoolname"))
		})
	})

	// ===================================================================
	// FedRAMP CM-6 (Configuration Management): Multi-tool parallel execution
	// with RepeatToolCall verifies that configuration-driven multi-tool batches
	// emit correctly on subsequent turns, preserving operational determinism.
	// ===================================================================

	Describe("UT-ML-1258-017: CM-6 MultiToolCalls + RepeatToolCall after FunctionResponse (Gemini)", func() {
		It("should emit multiple tool calls on second turn when repeat_tool_call is true", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:           "af_parallel",
						Keywords:       []string{"parallel tools"},
						ToolCall:       config.ToolCallOverride{Name: "ignored_when_multi"},
						MatchLastOnly:  true,
						RepeatToolCall: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)

			detCtx := &scenarios.DetectionContext{
				Content:         "parallel tools",
				AllText:         "user parallel tools",
				LastUserContent: "parallel tools",
			}
			result := registry.Detect(detCtx)
			Expect(result).NotTo(BeNil())
			scenarioWithCfg := result.Scenario.(scenarios.ScenarioWithConfig)
			cfg := scenarioWithCfg.Config()
			cfg.MultiToolCalls = []scenarios.MultiToolCallEntry{
				{Name: "tool_a", Arguments: map[string]interface{}{"key": "val_a"}},
				{Name: "tool_b", Arguments: map[string]interface{}{"key": "val_b"}},
			}

			router := handlers.NewRouter(scenarios.DefaultRegistryWithOverrides(&config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:           "af_parallel",
						Keywords:       []string{"parallel tools"},
						MatchLastOnly:  true,
						RepeatToolCall: true,
						ToolCall:       config.ToolCallOverride{Name: "tool_a"},
					},
				},
			}), false, "")

			Expect(cfg.RepeatToolCall).To(BeTrue())

			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := response.GeminiRequest{
				Contents: []response.GeminiContent{
					{Role: "user", Parts: []response.GeminiPart{{Text: "first turn"}}},
					{Role: "model", Parts: []response.GeminiPart{{FunctionCall: &response.GeminiFunctionCall{Name: "tool_a", Args: map[string]interface{}{}}}}},
					{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{Name: "tool_a", Response: map[string]interface{}{"done": true}}}}},
					{Role: "user", Parts: []response.GeminiPart{{Text: "parallel tools"}}},
				},
				Tools: []response.GeminiToolDecl{
					{FunctionDeclarations: []response.GeminiFunctionDecl{{Name: "tool_a"}, {Name: "tool_b"}}},
				},
			}

			body, _ := json.Marshal(reqBody)
			resp, err := http.Post(ts.URL+"/v1beta/models/gemini-1.5-pro:generateContent", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var gemResp response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&gemResp)).To(Succeed())
			parts := gemResp.Candidates[0].Content.Parts
			Expect(parts[0].FunctionCall).NotTo(BeNil(),
				"RepeatToolCall should emit tool call even after FunctionResponse present")
			Expect(parts[0].FunctionCall.Name).To(Equal("tool_a"))
		})
	})

	Describe("UT-ML-1258-018: CM-6 RepeatToolCall emits tool call on OpenAI multi-turn with prior tool results", func() {
		It("should emit tool call after tool results when repeat_tool_call is true", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:           "af_parallel",
						Keywords:       []string{"parallel tools"},
						ToolCall:       config.ToolCallOverride{Name: "tool_a", Arguments: map[string]interface{}{"key": "val"}},
						MatchLastOnly:  true,
						RepeatToolCall: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			content := func(s string) *string { return &s }
			reqBody := openai.ChatCompletionRequest{
				Model: "gpt-4",
				Messages: []openai.Message{
					{Role: "user", Content: content("first turn")},
					{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_1", Type: "function", Function: openai.FunctionCall{Name: "tool_a", Arguments: `{}`}}}},
					{Role: "tool", Content: content(`{"result":"done"}`)},
					{Role: "user", Content: content("parallel tools")},
				},
				Tools: []openai.Tool{{Type: "function", Function: openai.ToolDefinition{Name: "tool_a"}}},
			}

			body, _ := json.Marshal(reqBody)
			resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			var oaiResp openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&oaiResp)).To(Succeed())
			Expect(oaiResp.Choices[0].Message.ToolCalls).To(HaveLen(1))
			Expect(oaiResp.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal("tool_a"))
		})
	})

	// ===================================================================
	// FedRAMP SI-10 (Input Validation): Extraction resilience
	// Verifies that non-string field values, duplicate tool responses, and
	// preamble text do not cause panics or incorrect data propagation.
	// ===================================================================

	Describe("UT-ML-1258-013: SI-10 non-string field value in FunctionResponse", func() {
		It("should return empty when extracted field is numeric (not string)", func() {
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{
					Name:     "kubernaut_investigate",
					Response: map[string]interface{}{"session_id": 12345},
				}}}},
			}
			val := response.ExtractFieldFromFunctionResponse(contents, "kubernaut_investigate", "session_id")
			Expect(val).To(BeEmpty(), "numeric field should not be extracted as string")
		})
	})

	Describe("UT-ML-1258-014: SI-10 duplicate FunctionResponse for same tool takes first match", func() {
		It("should extract from the first matching FunctionResponse in iteration order", func() {
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{
					Name:     "kubernaut_investigate",
					Response: map[string]interface{}{"session_id": "first-match"},
				}}}},
				{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{
					Name:     "kubernaut_investigate",
					Response: map[string]interface{}{"session_id": "second-match"},
				}}}},
			}
			val := response.ExtractFieldFromFunctionResponse(contents, "kubernaut_investigate", "session_id")
			Expect(val).To(Equal("first-match"))
		})
	})

	Describe("UT-ML-1258-015: SI-10 OpenAI tool result with preamble text before JSON", func() {
		It("should extract field from JSON even when preceded by non-JSON text", func() {
			content := func(s string) *string { return &s }
			messages := []openai.Message{
				{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_1", Type: "function", Function: openai.FunctionCall{Name: "kubernaut_investigate", Arguments: `{}`}}}},
				{Role: "tool", Content: content(`Investigation started successfully. {"session_id":"sess-preamble","status":"active"}`)},
			}
			val := response.ExtractFieldFromToolResult(messages, "kubernaut_investigate", "session_id")
			Expect(val).To(Equal("sess-preamble"))
		})
	})

	Describe("UT-ML-1258-016: SI-10 OpenAI non-string numeric field returns empty", func() {
		It("should return empty when extracted field is not a string", func() {
			content := func(s string) *string { return &s }
			messages := []openai.Message{
				{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_1", Type: "function", Function: openai.FunctionCall{Name: "kubernaut_investigate", Arguments: `{}`}}}},
				{Role: "tool", Content: content(`{"session_id":99999}`)},
			}
			val := response.ExtractFieldFromToolResult(messages, "kubernaut_investigate", "session_id")
			Expect(val).To(BeEmpty(), "numeric session_id should not be extracted as string")
		})
	})

	// ===================================================================
	// FedRAMP AC-3 (Access Control) / SC-4 (Information in Shared Resources):
	// Template resolution must not leak resolved values across requests.
	// Verifies isolation — concurrent requests sharing the same scenario
	// singleton must not observe each other's resolved arguments.
	// ===================================================================

	Describe("UT-ML-1258-019: SC-4 template resolution does not leak state across requests", func() {
		It("should resolve template independently per request without leaking prior values", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:           "af_investigate_resume",
						Keywords:       []string{"stream the investigation"},
						ToolCall:       config.ToolCallOverride{Name: "kubernaut_investigate", Arguments: map[string]interface{}{"session_id": "$from_tool:kubernaut_investigate:session_id"}},
						MatchLastOnly:  true,
						RepeatToolCall: true,
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			makeGeminiRequest := func(sessionID string) response.GeminiResponse {
				reqBody := response.GeminiRequest{
					Contents: []response.GeminiContent{
						{Role: "model", Parts: []response.GeminiPart{{FunctionCall: &response.GeminiFunctionCall{Name: "kubernaut_investigate", Args: map[string]interface{}{}}}}},
						{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{Name: "kubernaut_investigate", Response: map[string]interface{}{"session_id": sessionID}}}}},
						{Role: "user", Parts: []response.GeminiPart{{Text: "stream the investigation"}}},
					},
					Tools: []response.GeminiToolDecl{
						{FunctionDeclarations: []response.GeminiFunctionDecl{{Name: "kubernaut_investigate"}}},
					},
				}
				body, _ := json.Marshal(reqBody)
				resp, err := http.Post(ts.URL+"/v1beta/models/gemini-1.5-pro:generateContent", "application/json", bytes.NewReader(body))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				var gemResp response.GeminiResponse
				Expect(json.NewDecoder(resp.Body).Decode(&gemResp)).To(Succeed())
				return gemResp
			}

			// Request 1: session_id = "sess-request-1"
			resp1 := makeGeminiRequest("sess-request-1")
			Expect(resp1.Candidates[0].Content.Parts[0].FunctionCall.Args).To(
				HaveKeyWithValue("session_id", "sess-request-1"))

			// Request 2: different session_id — must NOT see stale "sess-request-1"
			resp2 := makeGeminiRequest("sess-request-2")
			Expect(resp2.Candidates[0].Content.Parts[0].FunctionCall.Args).To(
				HaveKeyWithValue("session_id", "sess-request-2"))

			// Request 3: no FunctionResponse — template should remain unresolved
			reqBody3 := response.GeminiRequest{
				Contents: []response.GeminiContent{
					{Role: "user", Parts: []response.GeminiPart{{Text: "stream the investigation"}}},
				},
				Tools: []response.GeminiToolDecl{
					{FunctionDeclarations: []response.GeminiFunctionDecl{{Name: "kubernaut_investigate"}}},
				},
			}
			body3, _ := json.Marshal(reqBody3)
			resp3raw, err := http.Post(ts.URL+"/v1beta/models/gemini-1.5-pro:generateContent", "application/json", bytes.NewReader(body3))
			Expect(err).NotTo(HaveOccurred())
			defer resp3raw.Body.Close()

			var resp3 response.GeminiResponse
			Expect(json.NewDecoder(resp3raw.Body).Decode(&resp3)).To(Succeed())
			Expect(resp3.Candidates[0].Content.Parts[0].FunctionCall.Args).To(
				HaveKeyWithValue("session_id", "$from_tool:kubernaut_investigate:session_id"),
				"unresolved template must remain as literal, not leak prior request value")
		})
	})

	Describe("UT-ML-1258-020: SI-10 empty segments in $from_tool template", func() {
		It("should leave template with empty tool name unchanged", func() {
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{
					Name:     "any_tool",
					Response: map[string]interface{}{"field": "value"},
				}}}},
			}
			val := response.ExtractFieldFromFunctionResponse(contents, "", "field")
			Expect(val).To(BeEmpty(), "empty tool name should not match any response")
		})
	})
})

var _ = Describe("NextToolCall chaining (issue #1407)", func() {

	Describe("UT-ML-1407-001: NextToolCall YAML parsing", func() {
		It("should parse next_tool_call from YAML config", func() {
			yamlContent := `
keyword_scenarios:
  - name: "af_progressive_investigate"
    keywords: ["progressive investigate"]
    match_last_only: true
    tool_call:
      name: "kubernaut_investigate"
      arguments:
        namespace: "default"
    next_tool_call:
      name: "kubernaut_discover_workflows"
      arguments:
        rr_id: "kubernaut-system/rr-progressive"
`
			tmpFile := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
			Expect(os.WriteFile(tmpFile, []byte(yamlContent), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(tmpFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.KeywordScenarios).To(HaveLen(1))
			Expect(overrides.KeywordScenarios[0].NextToolCall).NotTo(BeNil())
			Expect(overrides.KeywordScenarios[0].NextToolCall.Name).To(Equal("kubernaut_discover_workflows"))
			Expect(overrides.KeywordScenarios[0].NextToolCall.Arguments).To(HaveKeyWithValue("rr_id", "kubernaut-system/rr-progressive"))
		})
	})

	Describe("UT-ML-1407-002: Gemini handler emits next_tool_call on second turn", func() {
		It("should return next_tool_call after initial tool FunctionResponse", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:          "af_progressive_investigate",
						Keywords:      []string{"progressive investigate"},
						ToolCall:      config.ToolCallOverride{Name: "kubernaut_investigate", Arguments: map[string]interface{}{"namespace": "default"}},
						MatchLastOnly: true,
						NextToolCall:  &config.ToolCallOverride{Name: "kubernaut_discover_workflows", Arguments: map[string]interface{}{"rr_id": "rr-001"}},
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := response.GeminiRequest{
				Contents: []response.GeminiContent{
					{Role: "user", Parts: []response.GeminiPart{{Text: "progressive investigate"}}},
					{Role: "model", Parts: []response.GeminiPart{{FunctionCall: &response.GeminiFunctionCall{Name: "kubernaut_investigate", Args: map[string]interface{}{"namespace": "default"}}}}},
					{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{Name: "kubernaut_investigate", Response: map[string]interface{}{"session_id": "sess-001", "rca_summary": "OOM detected"}}}}},
				},
				Tools: []response.GeminiToolDecl{
					{FunctionDeclarations: []response.GeminiFunctionDecl{
						{Name: "kubernaut_investigate"},
						{Name: "kubernaut_discover_workflows"},
					}},
				},
			}

			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1beta/models/gemini-1.5-pro:generateContent", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var gemResp response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&gemResp)).To(Succeed())
			Expect(gemResp.Candidates).To(HaveLen(1))

			parts := gemResp.Candidates[0].Content.Parts
			Expect(parts).To(HaveLen(1))
			Expect(parts[0].FunctionCall).NotTo(BeNil(), "should emit next_tool_call")
			Expect(parts[0].FunctionCall.Name).To(Equal("kubernaut_discover_workflows"),
				"chained tool call must be kubernaut_discover_workflows")
			Expect(parts[0].FunctionCall.Args).To(HaveKeyWithValue("rr_id", "rr-001"))
		})
	})

	Describe("UT-ML-1407-003: Gemini handler emits initial tool_call on first turn (no FunctionResponse)", func() {
		It("should return the initial tool_call before any chaining", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{},
				KeywordScenarios: []config.KeywordScenarioOverride{
					{
						Name:          "af_progressive_investigate",
						Keywords:      []string{"progressive investigate"},
						ToolCall:      config.ToolCallOverride{Name: "kubernaut_investigate", Arguments: map[string]interface{}{"namespace": "default"}},
						MatchLastOnly: true,
						NextToolCall:  &config.ToolCallOverride{Name: "kubernaut_discover_workflows", Arguments: map[string]interface{}{"rr_id": "rr-001"}},
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := response.GeminiRequest{
				Contents: []response.GeminiContent{
					{Role: "user", Parts: []response.GeminiPart{{Text: "progressive investigate"}}},
				},
				Tools: []response.GeminiToolDecl{
					{FunctionDeclarations: []response.GeminiFunctionDecl{
						{Name: "kubernaut_investigate"},
						{Name: "kubernaut_discover_workflows"},
					}},
				},
			}

			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1beta/models/gemini-1.5-pro:generateContent", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var gemResp response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&gemResp)).To(Succeed())
			Expect(gemResp.Candidates).To(HaveLen(1))

			parts := gemResp.Candidates[0].Content.Parts
			Expect(parts).To(HaveLen(1))
			Expect(parts[0].FunctionCall).NotTo(BeNil(), "first turn should emit initial tool_call")
			Expect(parts[0].FunctionCall.Name).To(Equal("kubernaut_investigate"),
				"first turn must use the primary tool_call, not next_tool_call")
		})
	})

	Describe("UT-ML-1407-004: OpenAI handler emits NextToolCall on second turn (CountToolResults==1)", func() {
		It("should return next_tool_call when exactly 1 tool result is present", func() {
			content := func(s string) *string { return &s }
			registry := scenarios.DefaultRegistry()
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := openai.ChatCompletionRequest{
				Model: "gpt-4",
				Messages: []openai.Message{
					{Role: "user", Content: content("brief-investigation-test signal")},
					{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_1", Type: "function", Function: openai.FunctionCall{Name: "kubectl_get", Arguments: `{"resource_type":"pod"}`}}}},
					{Role: "tool", Content: content(`{"kind":"Pod","metadata":{"name":"investigation-target"}}`)},
				},
				Tools: []openai.Tool{{Type: "function", Function: openai.ToolDefinition{Name: "kubectl_get"}}},
			}

			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var oaiResp openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&oaiResp)).To(Succeed())
			Expect(oaiResp.Choices).To(HaveLen(1))
			Expect(oaiResp.Choices[0].Message.ToolCalls).To(HaveLen(1),
				"second turn must emit NextToolCall (brief_investigation scenario)")
			Expect(oaiResp.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal("kubectl_get"),
				"NextToolCall must be kubectl_get per briefInvestigationConfig")
		})
	})

	Describe("UT-ML-1407-005: OpenAI handler does NOT emit NextToolCall on third turn (CountToolResults==2)", func() {
		It("should fall through to DAG/text when 2 tool results are present", func() {
			content := func(s string) *string { return &s }
			registry := scenarios.DefaultRegistry()
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := openai.ChatCompletionRequest{
				Model: "gpt-4",
				Messages: []openai.Message{
					{Role: "user", Content: content("brief-investigation-test signal")},
					{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_1", Type: "function", Function: openai.FunctionCall{Name: "kubectl_get", Arguments: `{"resource_type":"pod"}`}}}},
					{Role: "tool", Content: content(`{"kind":"Pod","metadata":{"name":"investigation-target"}}`)},
					{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "call_2", Type: "function", Function: openai.FunctionCall{Name: "kubectl_get", Arguments: `{"resource_type":"pod"}`}}}},
					{Role: "tool", Content: content(`{"kind":"Pod","metadata":{"name":"investigation-target-2"}}`)},
				},
				Tools: []openai.Tool{{Type: "function", Function: openai.ToolDefinition{Name: "kubectl_get"}}},
			}

			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var oaiResp openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&oaiResp)).To(Succeed())
			Expect(oaiResp.Choices).To(HaveLen(1))
			Expect(oaiResp.Choices[0].Message.ToolCalls).To(BeEmpty(),
				"third turn (2 tool results) must NOT emit NextToolCall — guard prevents infinite loops")
			Expect(oaiResp.Choices[0].Message.Content).NotTo(BeNil(),
				"third turn should produce text response via DAG final_analysis")
		})
	})

	Describe("UT-ML-1407-006: OpenAI handler emits initial ToolCallName on first turn with NextToolCall configured", func() {
		It("should return the primary ToolCallName before any chaining", func() {
			content := func(s string) *string { return &s }
			registry := scenarios.DefaultRegistry()
			router := handlers.NewRouter(registry, false, "")
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := openai.ChatCompletionRequest{
				Model: "gpt-4",
				Messages: []openai.Message{
					{Role: "user", Content: content("brief-investigation-test signal")},
				},
				Tools: []openai.Tool{{Type: "function", Function: openai.ToolDefinition{Name: "kubectl_get"}}},
			}

			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var oaiResp openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&oaiResp)).To(Succeed())
			Expect(oaiResp.Choices).To(HaveLen(1))
			Expect(oaiResp.Choices[0].Message.ToolCalls).To(HaveLen(1),
				"first turn must emit ToolCallName, not NextToolCall")
			Expect(oaiResp.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal("kubectl_get"),
				"first turn must use the primary ToolCallName from briefInvestigationConfig")
		})
	})
})
