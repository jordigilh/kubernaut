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

// bootRuntimeFor builds the boot-time LLMRuntimeConfig snapshot passed to
// llmRuntimeReloadCallback (#1599). Tests that don't exercise PhaseModels
// only need the boot Model to match the SwappableClient's initial model.
// bootRuntimeFor is also called from reload_callback_1470_test.go and
// reload_callback_1616_test.go in this package.
//
//nolint:unparam // model always receives "gpt-4" in this file, but hardcoding it would require touching the other two call-site files above, which are out of scope for this edit
func bootRuntimeFor(model string) *kaconfig.LLMRuntimeConfig {
	return &kaconfig.LLMRuntimeConfig{Model: model}
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
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil, bootRuntimeFor("gpt-4"))

		err := cb("")
		Expect(err).To(HaveOccurred())
		Expect(sc.ModelName()).To(Equal("gpt-4"))
	})

	It("rejects whitespace-only content", func() {
		sc := setupSwappable(GinkgoTB())
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil, bootRuntimeFor("gpt-4"))

		err := cb("   \n  \t  ")
		Expect(err).To(HaveOccurred())
	})

	// IT-KA-1599-001: base Model is immutable after boot; a hot-reload
	// attempt to change it must be rejected and the previously-active
	// client must keep serving (restart required per #1599).
	It("rejects a model change and does not change the model", func() {
		sc := setupSwappable(GinkgoTB())
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil, bootRuntimeFor("gpt-4"))

		err := cb(`model: gpt-4-turbo
endpoint: http://localhost:11434
apiKey: test-key
`)
		Expect(err).To(HaveOccurred())
		Expect(sc.ModelName()).To(Equal("gpt-4"))
	})

	It("accepts an endpoint change", func() {
		sc := setupSwappable(GinkgoTB())
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil, bootRuntimeFor("gpt-4"))

		err := cb(`model: gpt-4
endpoint: http://new-endpoint:8080
apiKey: test-key
`)
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects a validation failure and does not change the model", func() {
		sc := setupSwappable(GinkgoTB())
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil, bootRuntimeFor("gpt-4"))

		err := cb(`model: ""
endpoint: http://localhost:11434
apiKey: test-key
`)
		Expect(err).To(HaveOccurred())
		Expect(sc.ModelName()).To(Equal("gpt-4"))
	})

	It("accepts a temperature change", func() {
		sc := setupSwappable(GinkgoTB())
		cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil, bootRuntimeFor("gpt-4"))

		err := cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
temperature: 0.9
`)
		Expect(err).NotTo(HaveOccurred())
	})
})
