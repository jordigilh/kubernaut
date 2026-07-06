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
	"net/http/httptest"
	"testing"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/anthropicfamily"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// IT-KA-1580-001: buildLLMClientFromConfig dispatches provider "anthropic"
// (LLMProviderAnthropic, native API-key auth) to anthropicfamily.NewWithAPIKey
// through the actual production switch statement — not just a direct call to
// the constructor in a package-local test (CHECKPOINT W, Wiring Manifest row 1).
func TestBuildLLMClientFromConfig_AnthropicNative_Wiring(t *testing.T) {
	cfg := types.LLMConfig{
		Provider: types.LLMProviderAnthropic,
		Model:    "claude-sonnet-4-6",
		APIKey:   "sk-ant-fake-test-key",
	}

	client, err := buildLLMClientFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("buildLLMClientFromConfig(anthropic) unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("buildLLMClientFromConfig(anthropic) returned nil client")
	}
	if _, ok := client.(*anthropicfamily.Client); !ok {
		t.Fatalf("expected *anthropicfamily.Client, got %T — provider %q must dispatch to the native constructor, not langchaingo",
			client, types.LLMProviderAnthropic)
	}
}

// UT-KA-1580-001b: the anthropic provider case fails fast with no API key,
// matching NewWithAPIKey's own validation (production dispatch must not
// silently construct a client that will fail on first use).
func TestBuildLLMClientFromConfig_AnthropicNative_RequiresAPIKey(t *testing.T) {
	cfg := types.LLMConfig{
		Provider: types.LLMProviderAnthropic,
		Model:    "claude-sonnet-4-6",
	}

	_, err := buildLLMClientFromConfig(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected an error when provider=anthropic has no apiKey configured")
	}
}

// IT-KA-1581-002: buildLLMClientFromConfig dispatches providers "openai" and
// "openai_compatible" to the shared-core-backed kaopenai.Client through the
// actual production switch statement (CHECKPOINT W, Wiring Manifest row 3).
func TestBuildLLMClientFromConfig_OpenAICompat_Wiring(t *testing.T) {
	server := httptest.NewServer(nil)
	defer server.Close()

	for _, provider := range []string{types.LLMProviderOpenAI, types.LLMProviderOpenAICompatible} {
		cfg := types.LLMConfig{
			Provider: provider,
			Model:    "gpt-4o",
			Endpoint: server.URL,
			APIKey:   "sk-fake-test-key",
		}

		client, err := buildLLMClientFromConfig(context.Background(), cfg)
		if err != nil {
			t.Fatalf("buildLLMClientFromConfig(%s) unexpected error: %v", provider, err)
		}
		if client == nil {
			t.Fatalf("buildLLMClientFromConfig(%s) returned nil client", provider)
		}
		if _, ok := client.(*kaopenai.Client); !ok {
			t.Fatalf("provider %q: expected *kaopenai.Client, got %T — must dispatch to the shared-core wrapper, not langchaingo",
				provider, client)
		}
	}
}

// UT-KA-1581-002b: an unrecognized provider (Gemini native, not yet migrated
// off langchaingo) still falls through to the default branch, proving the
// new cases above are additive and do not regress the fallback path.
func TestBuildLLMClientFromConfig_UnmigratedProvider_FallsThroughToDefault(t *testing.T) {
	cfg := types.LLMConfig{
		Provider: types.LLMProviderGemini,
		Model:    "gemini-not-yet-migrated",
	}

	_, err := buildLLMClientFromConfig(context.Background(), cfg)
	// langchaingo has no "gemini" case (only "openai"/"ollama"/"azure"/
	// "vertex"/"anthropic"/"bedrock"/"huggingface"/"mistral") — the point of
	// this test is that Gemini reaches langchaingo.New and fails there
	// ("unsupported LLM provider"), not that this call succeeds.
	if err == nil {
		t.Fatal("expected an error for gemini via the (pre-existing, unrelated) langchaingo fallback path")
	}
}
