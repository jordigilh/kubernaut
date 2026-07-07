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

package main

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

func testReloadLogger() logr.Logger {
	return logr.Discard()
}

func staticCfg() *kaconfig.Config {
	cfg := kaconfig.DefaultConfig()
	cfg.AI.LLM.Provider = "openai"
	cfg.AI.LLM.APIKey = "test-static-key" // pre-commit:allow-sensitive (test-only value)
	return cfg
}

// setupSwappable accepts testing.TB (not *testing.T) so both plain
// `testing.T`-based tests and Ginkgo specs (via GinkgoTB(), which implements
// testing.TB) can share this
// fixture.
func setupSwappable(t testing.TB) *llm.SwappableClient {
	t.Helper()
	inner := &stubLLMClient{}
	sc, err := llm.NewSwappableClient(inner, "gpt-4")
	if err != nil {
		t.Fatal(err)
	}
	return sc
}

type stubLLMClient struct{}

func (s *stubLLMClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, nil
}

func (s *stubLLMClient) StreamChat(_ context.Context, _ llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, nil
}

func (s *stubLLMClient) Close() error { return nil }

var _ = Describe("llmRuntimeReloadCallback (#783)", func() {
	It("rejects empty content and does not change the model", func() {
		sc := setupSwappable(GinkgoTB())
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil)

		err := cb("")
		Expect(err).To(HaveOccurred())
		Expect(sc.ModelName()).To(Equal("gpt-4"))
	})

	It("rejects whitespace-only content", func() {
		sc := setupSwappable(GinkgoTB())
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil)

		err := cb("   \n  \t  ")
		Expect(err).To(HaveOccurred())
	})

	It("accepts a model change", func() {
		sc := setupSwappable(GinkgoTB())
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil)

		err := cb(`model: gpt-4-turbo
endpoint: http://localhost:11434
apiKey: test-key
`)
		Expect(err).NotTo(HaveOccurred())
		Expect(sc.ModelName()).To(Equal("gpt-4-turbo"))
	})

	It("accepts an endpoint change", func() {
		sc := setupSwappable(GinkgoTB())
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil)

		err := cb(`model: gpt-4
endpoint: http://new-endpoint:8080
apiKey: test-key
`)
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects a validation failure and does not change the model", func() {
		sc := setupSwappable(GinkgoTB())
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil)

		err := cb(`model: ""
endpoint: http://localhost:11434
apiKey: test-key
`)
		Expect(err).To(HaveOccurred())
		Expect(sc.ModelName()).To(Equal("gpt-4"))
	})

	It("accepts a temperature change", func() {
		sc := setupSwappable(GinkgoTB())
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil)

		err := cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
temperature: 0.9
`)
		Expect(err).NotTo(HaveOccurred())
	})
})
