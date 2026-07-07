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

// IT-KA-1578-001: anthropicReasoningOptions — the exact mapping used by both
// buildAnthropicNativeClient and the vertex_ai case in
// buildLLMClientFromConfig — is proven independently of the (untestable
// without a live network seam) SDK HTTP call. anthropicfamily's own
// httptest-based suite (thinking_1580_test.go, UT-KA-1578-201..204) proves
// WithReasoning's effect once applied; this proves the config->option
// mapping that used to be entirely missing (#1578 wiring-gap fix).
func TestAnthropicReasoningOptions_Wiring(t *testing.T) {
	t.Run("enabled config produces exactly one WithReasoning option", func(t *testing.T) {
		cfg := types.LLMConfig{
			Reasoning: &types.LLMReasoningConfig{Enabled: true, BudgetTokens: 8192},
		}
		opts := anthropicReasoningOptions(cfg)
		if len(opts) != 1 {
			t.Fatalf("expected exactly 1 option for reasoning.enabled=true, got %d", len(opts))
		}
	})

	t.Run("nil Reasoning produces no options (no regression for existing deployments)", func(t *testing.T) {
		cfg := types.LLMConfig{}
		opts := anthropicReasoningOptions(cfg)
		if len(opts) != 0 {
			t.Fatalf("expected 0 options when cfg.Reasoning is nil, got %d", len(opts))
		}
	})

	t.Run("reasoning.enabled=false produces no options", func(t *testing.T) {
		cfg := types.LLMConfig{
			Reasoning: &types.LLMReasoningConfig{Enabled: false, BudgetTokens: 8192},
		}
		opts := anthropicReasoningOptions(cfg)
		if len(opts) != 0 {
			t.Fatalf("expected 0 options when cfg.Reasoning.Enabled is false, got %d", len(opts))
		}
	})
}

// IT-KA-1578-002: buildAnthropicNativeClient dispatches a reasoning-enabled
// config through the real production construction path without error
// (CHECKPOINT W row: config -> client construction). Combined with
// TestAnthropicReasoningOptions_Wiring (proves the exact cfg->option
// mapping) and anthropicfamily's own httptest-based suite (proves
// WithReasoning's wire-level effect once applied), these three together
// prove the full chain: operator config -> constructed option -> applied
// client default -> outgoing "thinking" param. A live end-to-end Chat()
// call through this exact function is not exercised here because
// buildAnthropicNativeClient has no cfg-driven seam to redirect the SDK's
// base URL away from api.anthropic.com without a real network call.
func TestBuildAnthropicNativeClient_Reasoning_Wiring(t *testing.T) {
	cfg := types.LLMConfig{
		Provider:  types.LLMProviderAnthropic,
		Model:     "claude-sonnet-4-6",
		APIKey:    "sk-ant-fake-test-key",
		Reasoning: &types.LLMReasoningConfig{Enabled: true, BudgetTokens: 2048},
	}

	client, err := buildAnthropicNativeClient(cfg)
	if err != nil {
		t.Fatalf("buildAnthropicNativeClient unexpected error: %v", err)
	}
	if _, ok := client.(*anthropicfamily.Client); !ok {
		t.Fatalf("expected *anthropicfamily.Client, got %T", client)
	}
	// anthropicReasoningOptions is exercised as part of the construction
	// path above (it is unconditionally appended to opts in
	// buildAnthropicNativeClient); TestAnthropicReasoningOptions_Wiring
	// proves its mapping precisely, and anthropicfamily's own suite proves
	// WithReasoning's wire-level effect once applied to a Client.
}

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
		t.Fatalf("expected *anthropicfamily.Client, got %T — provider %q must dispatch to the native constructor",
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
			t.Fatalf("provider %q: expected *kaopenai.Client, got %T — must dispatch to the shared-core wrapper",
				provider, client)
		}
	}
}

// UT-KA-1581-002b: an unrecognized provider (Gemini native, never had a
// langchaingo case either) still falls through to the default branch and
// gets an explicit error, proving the new anthropic/openai cases above are
// additive and the removed-langchaingo default path fails loudly rather
// than silently.
func TestBuildLLMClientFromConfig_UnsupportedProvider_ReturnsError(t *testing.T) {
	cfg := types.LLMConfig{
		Provider: types.LLMProviderGemini,
		Model:    "gemini-not-yet-migrated",
	}

	// Gemini has no dedicated case in buildLLMClientFromConfig (only
	// anthropic/openai/openai-compatible are wired) and langchaingo has been
	// fully removed (#1580/#1581 M4), so the default case must return an
	// explicit "unsupported LLM provider" error rather than silently falling
	// through to a removed dependency.
	_, err := buildLLMClientFromConfig(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected an error for gemini: no langchaingo fallback exists anymore")
	}
}
