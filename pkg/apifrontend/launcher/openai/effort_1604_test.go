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

package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	openaimodel "github.com/jordigilh/kubernaut/pkg/apifrontend/launcher/openai"
)

// Issue #1604 / BR-AI-086: unified Effort knob wiring for AF's ADK-facing
// OpenAI-Chat-Completions-compatible adapter. Mirrors
// pkg/kubernautagent/llm/openai's WithReasoning construction-time-default
// pattern (effort_1604_test.go, UT-KA-1604-301..306) for cross-family
// symmetry (DD-LLM-005) — AF has no per-call reasoning override the way
// KA's Options.Reasoning does, since ADK's model.LLMRequest carries no
// such field, so the construction-time default is the only knob here.
var _ = Describe("apifrontend/launcher/openai.Model Effort knob wiring — #1604", func() {
	var (
		server       *httptest.Server
		receivedBody map[string]any
	)

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	newTestServer := func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedBody = nil
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(chatCompletionResponse("ok"))
		}))
	}

	simpleRequest := func() *model.LLMRequest {
		return &model.LLMRequest{
			Contents: []*genai.Content{
				{Role: "user", Parts: []*genai.Part{{Text: "hello"}}},
			},
		}
	}

	// UT-AF-1604-301 [BR-AI-086 AC8]: WithReasoningEffort sends the configured
	// Effort as reasoning_effort for a real OpenAI reasoning model.
	It("UT-AF-1604-301: WithReasoningEffort sends the configured Effort as reasoning_effort for a real OpenAI reasoning model", func() {
		newTestServer()
		m := openaimodel.NewModel("gpt-5", server.URL, "test-key", openaimodel.WithReasoningEffort("high"))
		for resp, err := range m.GenerateContent(context.Background(), simpleRequest(), false) {
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		}
		Expect(receivedBody["reasoning_effort"]).To(Equal("high"))
	})

	// UT-AF-1604-302 [BR-AI-086 AC8]: WithReasoningEffort has no effect on a
	// non-reasoning model (no effort dialect recognized) — compatibility floor.
	It("UT-AF-1604-302: WithReasoningEffort has no effect on a non-reasoning model", func() {
		newTestServer()
		m := openaimodel.NewModel("gpt-4o", server.URL, "test-key", openaimodel.WithReasoningEffort("high"))
		for resp, err := range m.GenerateContent(context.Background(), simpleRequest(), false) {
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		}
		Expect(receivedBody).NotTo(HaveKey("reasoning_effort"))
	})

	// UT-AF-1604-303 [BR-AI-086 AC8]: downscales to DeepSeek's own dialect for
	// a deepseek-v4-pro model, same mapping as KA's wrapper (shared openaicompat).
	It("UT-AF-1604-303: downscales to DeepSeek's dialect for a deepseek-v4-pro model", func() {
		newTestServer()
		m := openaimodel.NewModel("deepseek-v4-pro", server.URL, "test-key", openaimodel.WithReasoningEffort("xhigh"))
		for resp, err := range m.GenerateContent(context.Background(), simpleRequest(), false) {
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		}
		Expect(receivedBody["reasoning_effort"]).To(Equal("max"))
		thinking, ok := receivedBody["thinking"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(thinking["type"]).To(Equal("enabled"))
	})

	// UT-AF-1604-304 [BR-AI-086 AC8]: without WithReasoningEffort, no effort
	// field is ever sent (zero-regression default).
	It("UT-AF-1604-304: without WithReasoningEffort, no effort field is ever sent", func() {
		newTestServer()
		m := openaimodel.NewModel("gpt-5", server.URL, "test-key")
		for resp, err := range m.GenerateContent(context.Background(), simpleRequest(), false) {
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		}
		Expect(receivedBody).NotTo(HaveKey("reasoning_effort"))
	})

	// UT-AF-1604-305 [BR-AI-086 AC8]: an empty Effort ("" — vendor default) is
	// a no-op even on a recognized reasoning model, mirroring KA's behavior.
	It("UT-AF-1604-305: an empty Effort sends no reasoning_effort even on a recognized model", func() {
		newTestServer()
		m := openaimodel.NewModel("gpt-5", server.URL, "test-key", openaimodel.WithReasoningEffort(""))
		for resp, err := range m.GenerateContent(context.Background(), simpleRequest(), false) {
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		}
		Expect(receivedBody).NotTo(HaveKey("reasoning_effort"))
	})
})
