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

package openaicompat_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
)

// Issue #1604 / BR-AI-086: a unified, provider-agnostic reasoning-effort
// knob for the OpenAI-Chat-Completions-compatible client family. langchaingo
// (tmc/langchaingo#1394, #1499) took roughly a year of incremental PRs to
// reach the same "reasoning_effort on Chat Completions" scope — this closes
// only that scope, deliberately excluding the OpenAI Responses API (#1604
// tracks that separately; it requires a new SDK dependency and its own DD).
var _ = Describe("openaicompat effort knob — #1604", func() {
	Describe("DetectEffortDialect — model-aware detection of which effort wire dialect a model speaks", func() {
		DescribeTable("real OpenAI reasoning models detect to the OpenAI dialect",
			func(model string) {
				Expect(openaicompat.DetectEffortDialect(model)).To(Equal(openaicompat.EffortDialectOpenAI))
			},
			Entry("o1", "o1"),
			Entry("o1-mini", "o1-mini"),
			Entry("o1-preview", "o1-preview"),
			Entry("o3", "o3"),
			Entry("o3-mini", "o3-mini"),
			Entry("o3-pro", "o3-pro"),
			Entry("o4-mini", "o4-mini"),
			Entry("gpt-5", "gpt-5"),
			Entry("gpt-5-mini", "gpt-5-mini"),
			Entry("gpt-5.1", "gpt-5.1"),
			Entry("gpt-5-codex", "gpt-5-codex"),
		)

		DescribeTable("DeepSeek reasoning models detect to the DeepSeek dialect",
			func(model string) {
				Expect(openaicompat.DetectEffortDialect(model)).To(Equal(openaicompat.EffortDialectDeepSeek))
			},
			Entry("deepseek-reasoner (legacy)", "deepseek-reasoner"),
			Entry("deepseek-r1", "deepseek-r1"),
			Entry("deepseek-v4-pro", "deepseek-v4-pro"),
			Entry("deepseek-v4-flash", "deepseek-v4-flash"),
		)

		DescribeTable("every other model detects to no dialect (no effort parameter is ever sent)",
			func(model string) {
				Expect(openaicompat.DetectEffortDialect(model)).To(Equal(openaicompat.EffortDialectNone))
			},
			Entry("gpt-4o", "gpt-4o"),
			Entry("gpt-3.5-turbo", "gpt-3.5-turbo"),
			Entry("llama3 (self-hosted)", "llama3"),
			Entry("unrecognized future model", "some-future-model-9000"),
		)
	})

	Describe("wire-level effort mapping — real OpenAI dialect", func() {
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
		newTestClient := func(model string) *openaicompat.Client {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				receivedBody = nil
				_ = json.Unmarshal(body, &receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
			}))
			return openaicompat.New(model, server.URL, "test-key")
		}

		DescribeTable("passes the canonical effort value through verbatim as reasoning_effort",
			func(effort string) {
				client := newTestClient("gpt-5")
				_, err := client.Chat(context.Background(), openaicompat.Request{
					Messages:      []openaicompat.Message{{Role: "user", Content: "hi"}},
					Effort:        effort,
					EffortDialect: openaicompat.EffortDialectOpenAI,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(receivedBody["reasoning_effort"]).To(Equal(effort))
			},
			Entry("none", "none"),
			Entry("minimal", "minimal"),
			Entry("low", "low"),
			Entry("medium", "medium"),
			Entry("high", "high"),
			Entry("xhigh", "xhigh"),
		)

		It("UT-KA-1604-001: sends no reasoning_effort field when Effort is empty (vendor default applies)", func() {
			client := newTestClient("gpt-5")
			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages:      []openaicompat.Message{{Role: "user", Content: "hi"}},
				EffortDialect: openaicompat.EffortDialectOpenAI,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).NotTo(HaveKey("reasoning_effort"))
		})

		It("UT-KA-1604-002: sends no reasoning_effort field when EffortDialect is none, even if Effort is set", func() {
			client := newTestClient("gpt-4o")
			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages:      []openaicompat.Message{{Role: "user", Content: "hi"}},
				Effort:        "high",
				EffortDialect: openaicompat.EffortDialectNone,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).NotTo(HaveKey("reasoning_effort"))
		})

		It("UT-KA-1604-003: never sends a DeepSeek-specific thinking field for the OpenAI dialect", func() {
			client := newTestClient("gpt-5")
			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages:      []openaicompat.Message{{Role: "user", Content: "hi"}},
				Effort:        "high",
				EffortDialect: openaicompat.EffortDialectOpenAI,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).NotTo(HaveKey("thinking"))
		})
	})

	Describe("wire-level effort mapping — DeepSeek dialect (downscaled two-tier + explicit toggle)", func() {
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
		newTestClient := func(model string) *openaicompat.Client {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				receivedBody = nil
				_ = json.Unmarshal(body, &receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
			}))
			return openaicompat.New(model, server.URL, "test-key")
		}

		DescribeTable("downscales the canonical effort value to DeepSeek's high/max dialect and enables thinking",
			func(effort, wantWireEffort string) {
				client := newTestClient("deepseek-v4-pro")
				_, err := client.Chat(context.Background(), openaicompat.Request{
					Messages:      []openaicompat.Message{{Role: "user", Content: "hi"}},
					Effort:        effort,
					EffortDialect: openaicompat.EffortDialectDeepSeek,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(receivedBody["reasoning_effort"]).To(Equal(wantWireEffort))
				thinking, ok := receivedBody["thinking"].(map[string]interface{})
				Expect(ok).To(BeTrue(), "expected a thinking object in the request body")
				Expect(thinking["type"]).To(Equal("enabled"))
			},
			Entry("minimal maps up to DeepSeek's floor (high)", "minimal", "high"),
			Entry("low maps up to DeepSeek's floor (high)", "low", "high"),
			Entry("medium maps up to DeepSeek's floor (high)", "medium", "high"),
			Entry("high maps to high", "high", "high"),
			Entry("xhigh maps to DeepSeek's ceiling (max)", "xhigh", "max"),
		)

		It("UT-KA-1604-004: effort: none disables thinking entirely rather than mapping to a tier", func() {
			client := newTestClient("deepseek-v4-pro")
			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages:      []openaicompat.Message{{Role: "user", Content: "hi"}},
				Effort:        "none",
				EffortDialect: openaicompat.EffortDialectDeepSeek,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).NotTo(HaveKey("reasoning_effort"))
			thinking, ok := receivedBody["thinking"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(thinking["type"]).To(Equal("disabled"))
		})

		It("UT-KA-1604-005: sends no effort/thinking fields at all when Effort is empty (vendor default applies)", func() {
			client := newTestClient("deepseek-v4-pro")
			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages:      []openaicompat.Message{{Role: "user", Content: "hi"}},
				EffortDialect: openaicompat.EffortDialectDeepSeek,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).NotTo(HaveKey("reasoning_effort"))
			Expect(receivedBody).NotTo(HaveKey("thinking"))
		})
	})
})
