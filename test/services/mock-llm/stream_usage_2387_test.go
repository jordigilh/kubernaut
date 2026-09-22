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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

// Issue #2387 / BR-KA-OBSERVABILITY-001: E2E investigations stream, and the mock's SSE writer dropped
// the builders' Usage on the floor, so no streamed E2E turn ever reported
// tokens. The SSE transport must emit the response Usage as a trailing
// usage chunk (OpenAI stream_options.include_usage convention), and
// scenarios must be able to script distinctive per-scenario values via the
// YAML `usage:` override (defaults = the builders' current hardcoded values,
// so existing scenarios are unaffected). These assertions are
// control-supporting evidence: exact provider usage preserves the content
// needed for FedRAMP AU-3 records and SOC2 CC7.2 investigation monitoring;
// durable audit persistence is tested separately in the KA audit suites.
var _ = Describe("Streamed usage reporting (issue #2387) [BR-KA-OBSERVABILITY-001, AU-3, SOC2 CC7.2]", func() {
	postStreamRequest := func(ts *httptest.Server, reqBody openai.ChatCompletionRequest) string {
		body, err := json.Marshal(reqBody)
		Expect(err).NotTo(HaveOccurred())

		resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		raw, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		return string(raw)
	}

	postStream := func(ts *httptest.Server, text string, includeUsage bool) string {
		reqBody := openai.ChatCompletionRequest{
			Model:    "mock-model",
			Stream:   true,
			Messages: []openai.Message{{Role: "user", Content: strPtr(text)}},
		}
		if includeUsage {
			reqBody.StreamOptions = &openai.StreamOptions{IncludeUsage: true}
		}
		return postStreamRequest(ts, reqBody)
	}

	streamedUsage := func(raw string) openai.Usage {
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			if payload == "[DONE]" {
				continue
			}
			var chunk struct {
				Usage *openai.Usage `json:"usage"`
			}
			Expect(json.Unmarshal([]byte(payload), &chunk)).To(Succeed())
			if chunk.Usage != nil {
				return *chunk.Usage
			}
		}
		return openai.Usage{}
	}

	Describe("UT-MOCK-2387-001: stream=true emits the builder default usage as a trailing chunk", func() {
		It("should carry prompt/completion/total tokens on the wire for a default text response", func() {
			registry := scenarios.DefaultRegistry()
			ts := httptest.NewServer(handlers.NewRouter(registry, false, config.ModeInteractive))
			defer ts.Close()

			usage := streamedUsage(postStream(ts, "gibberish that matches no scenario", true))
			Expect(usage.PromptTokens).To(Equal(100))
			Expect(usage.CompletionTokens).To(Equal(50))
			Expect(usage.TotalTokens).To(Equal(150))
		})
	})

	Describe("UT-MOCK-2387-002: YAML usage override scripts distinctive streamed values", func() {
		It("should emit the overridden usage for the oomkilled scenario", func() {
			overrides := &config.Overrides{
				Scenarios: map[string]config.ScenarioOverride{
					"oomkilled": {
						Usage: &config.UsageOverride{PromptTokens: 111, CompletionTokens: 222, TotalTokens: 333},
					},
				},
			}
			registry := scenarios.DefaultRegistryWithOverrides(overrides)
			ts := httptest.NewServer(handlers.NewRouter(registry, false, config.ModeInteractive))
			defer ts.Close()

			usage := streamedUsage(postStream(ts, "- Signal Name: OOMKilled\n- Namespace: default", true))
			Expect(usage.PromptTokens).To(Equal(111))
			Expect(usage.CompletionTokens).To(Equal(222))
			Expect(usage.TotalTokens).To(Equal(333))
		})
	})

	Describe("UT-MOCK-2387-003: YAML usage block parses", func() {
		It("should parse usage prompt/completion/total tokens from a YAML overrides file", func() {
			yamlContent := `scenarios:
  oomkilled:
    usage:
      prompt_tokens: 111
      completion_tokens: 222
      total_tokens: 333
`
			tmpFile := filepath.Join(GinkgoT().TempDir(), "overrides.yaml")
			Expect(os.WriteFile(tmpFile, []byte(yamlContent), 0644)).To(Succeed())

			overrides, err := config.LoadYAMLOverrides(tmpFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(overrides.Scenarios).To(HaveKey("oomkilled"))
			usage := overrides.Scenarios["oomkilled"].Usage
			Expect(usage).NotTo(BeNil())
			Expect(usage.PromptTokens).To(Equal(111))
			Expect(usage.CompletionTokens).To(Equal(222))
			Expect(usage.TotalTokens).To(Equal(333))
		})
	})

	Describe("UT-MOCK-2387-004: usage requires the client opt-in", func() {
		It("should omit the trailing usage chunk when include_usage is false", func() {
			registry := scenarios.DefaultRegistry()
			ts := httptest.NewServer(handlers.NewRouter(registry, false, config.ModeInteractive))
			defer ts.Close()

			Expect(streamedUsage(postStream(ts, "gibberish that matches no scenario", false))).To(Equal(openai.Usage{}))
		})
	})

	Describe("UT-MOCK-2387-005: multi-tool usage override", func() {
		It("emits the scripted usage for a streamed multi-tool response", func() {
			registry := scenarios.NewRegistry()
			registry.Register(&configuredTestScenario{config: scenarios.MockScenarioConfig{
				ScenarioName: "multi-tool-usage",
				MultiToolCalls: []scenarios.MultiToolCallEntry{
					{Name: "kubectl_get_yaml"},
					{Name: "kubectl_get_by_name"},
				},
				Usage: &scenarios.MockUsage{PromptTokens: 111, CompletionTokens: 222, TotalTokens: 333},
			}})
			ts := httptest.NewServer(handlers.NewRouter(registry, false, config.ModeFull))
			defer ts.Close()

			usage := streamedUsage(postStreamRequest(ts, openai.ChatCompletionRequest{
				Model:         "mock-model",
				Stream:        true,
				StreamOptions: &openai.StreamOptions{IncludeUsage: true},
				Messages:      []openai.Message{{Role: "user", Content: strPtr("multi-tool usage")}},
				Tools: []openai.Tool{
					{Type: "function", Function: openai.ToolDefinition{Name: "kubectl_get_yaml"}},
					{Type: "function", Function: openai.ToolDefinition{Name: "kubectl_get_by_name"}},
				},
			}))

			Expect(usage).To(Equal(openai.Usage{PromptTokens: 111, CompletionTokens: 222, TotalTokens: 333}))
		})
	})

	Describe("UT-MOCK-2387-006: chained usage override", func() {
		It("emits the scripted usage for a streamed chained tool response", func() {
			registry := scenarios.NewRegistry()
			registry.Register(&configuredTestScenario{config: scenarios.MockScenarioConfig{
				ScenarioName: "chained-usage",
				ToolCallName: "kubectl_get_yaml",
				NextToolCall: &scenarios.MultiToolCallEntry{Name: "kubectl_get_by_name"},
				Usage:        &scenarios.MockUsage{PromptTokens: 444, CompletionTokens: 555, TotalTokens: 999},
			}})
			ts := httptest.NewServer(handlers.NewRouter(registry, false, config.ModeFull))
			defer ts.Close()

			messages := []openai.Message{{Role: "user", Content: strPtr("chained usage")}}
			tools := []openai.Tool{
				{Type: "function", Function: openai.ToolDefinition{Name: "kubectl_get_yaml"}},
				{Type: "function", Function: openai.ToolDefinition{Name: "kubectl_get_by_name"}},
			}
			first := postOpenAI(ts.URL, openai.ChatCompletionRequest{Model: "mock-model", Messages: messages, Tools: tools})
			appendOpenAIToolResult(&messages, first, `{"ok":true}`)

			usage := streamedUsage(postStreamRequest(ts, openai.ChatCompletionRequest{
				Model:         "mock-model",
				Stream:        true,
				StreamOptions: &openai.StreamOptions{IncludeUsage: true},
				Messages:      messages,
				Tools:         tools,
			}))

			Expect(usage).To(Equal(openai.Usage{PromptTokens: 444, CompletionTokens: 555, TotalTokens: 999}))
		})
	})
})
