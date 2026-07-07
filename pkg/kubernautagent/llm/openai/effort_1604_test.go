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
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
)

// Issue #1604 / BR-AI-086: unified Effort knob wiring for KA's
// OpenAI-Chat-Completions-compatible wrapper. Mirrors the
// anthropicfamily.WithReasoning construction-time-default pattern
// (thinking_1580_test.go, UT-KA-1578-201..204) for symmetry across the two
// client families.
var _ = Describe("kubernautagent/llm/openai.Client Effort knob wiring — #1604", func() {
	var (
		server       *httptest.Server
		receivedBody map[string]interface{}
	)

	BeforeEach(func() {
		receivedBody = nil
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	newTestServer := func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			receivedBody = nil
			_ = json.Unmarshal(body, &receivedBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
		}))
	}

	It("UT-KA-1604-301: WithReasoning sends the configured Effort as reasoning_effort for a real OpenAI reasoning model", func() {
		newTestServer()
		client := kaopenai.New("gpt-5", server.URL, "test-key",
			kaopenai.WithReasoning(llm.ReasoningRequest{Enabled: true, Effort: "high"}))
		_, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "hello"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedBody["reasoning_effort"]).To(Equal("high"))
	})

	It("UT-KA-1604-302: a per-call Options.Reasoning overrides the construction-time default", func() {
		newTestServer()
		client := kaopenai.New("gpt-5", server.URL, "test-key",
			kaopenai.WithReasoning(llm.ReasoningRequest{Enabled: true, Effort: "low"}))
		_, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "hello"}},
			Options:  llm.ChatOptions{Reasoning: &llm.ReasoningRequest{Enabled: true, Effort: "xhigh"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedBody["reasoning_effort"]).To(Equal("xhigh"))
	})

	It("UT-KA-1604-303: WithReasoning has no effect on a non-reasoning model (no effort dialect recognized)", func() {
		newTestServer()
		client := kaopenai.New("gpt-4o", server.URL, "test-key",
			kaopenai.WithReasoning(llm.ReasoningRequest{Enabled: true, Effort: "high"}))
		_, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "hello"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedBody).NotTo(HaveKey("reasoning_effort"))
	})

	It("UT-KA-1604-304: WithReasoning(Enabled:false) never sends reasoning_effort, even with Effort set", func() {
		newTestServer()
		client := kaopenai.New("gpt-5", server.URL, "test-key",
			kaopenai.WithReasoning(llm.ReasoningRequest{Enabled: false, Effort: "high"}))
		_, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "hello"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedBody).NotTo(HaveKey("reasoning_effort"))
	})

	It("UT-KA-1604-305: downscales to DeepSeek's dialect for a deepseek-v4-pro model", func() {
		newTestServer()
		client := kaopenai.New("deepseek-v4-pro", server.URL, "test-key",
			kaopenai.WithReasoning(llm.ReasoningRequest{Enabled: true, Effort: "xhigh"}))
		_, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "hello"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedBody["reasoning_effort"]).To(Equal("max"))
		thinking := receivedBody["thinking"].(map[string]interface{})
		Expect(thinking["type"]).To(Equal("enabled"))
	})

	It("UT-KA-1604-306: without WithReasoning, no effort field is ever sent (no regression)", func() {
		newTestServer()
		client := kaopenai.New("gpt-5", server.URL, "test-key")
		_, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "hello"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedBody).NotTo(HaveKey("reasoning_effort"))
	})
})
