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
	"log/slog"
	"os"
	"testing"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func baseCfg() *kaconfig.Config {
	cfg := kaconfig.DefaultConfig()
	cfg.AI.LLM.Provider = "openai"
	cfg.AI.LLM.Model = "gpt-4"
	cfg.AI.LLM.Endpoint = "http://localhost:11434"
	cfg.AI.LLM.APIKey = "test-key"
	return cfg
}

func fullConfigYAML(overrides string) string {
	base := `ai:
  llm:
    provider: openai
    model: gpt-4
    endpoint: http://localhost:11434
    apiKey: test-key
  investigation:
    maxTurns: 40
`
	if overrides != "" {
		return overrides
	}
	return base
}

func setupSwappable(t *testing.T) *llm.SwappableClient {
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

func (s *stubLLMClient) Close() error { return nil }

func TestReloadRejectsEmptyContent(t *testing.T) {
	sc := setupSwappable(t)
	cb := configReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

	err := cb("")
	if err == nil {
		t.Fatal("expected error for empty content")
	}
	if sc.ModelName() != "gpt-4" {
		t.Fatalf("model should not change on rejected reload, got %s", sc.ModelName())
	}
}

func TestReloadRejectsWhitespaceContent(t *testing.T) {
	sc := setupSwappable(t)
	cb := configReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

	err := cb("   \n  \t  ")
	if err == nil {
		t.Fatal("expected error for whitespace-only content")
	}
}

func TestReloadRejectsProviderChange(t *testing.T) {
	sc := setupSwappable(t)
	cb := configReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

	err := cb(fullConfigYAML(`ai:
  llm:
    provider: anthropic
    model: claude-3
    endpoint: http://localhost:11434
    apiKey: test-key
  investigation:
    maxTurns: 40
`))
	if err == nil {
		t.Fatal("expected error for provider change")
	}
	if sc.ModelName() != "gpt-4" {
		t.Fatalf("model should not change on rejected reload, got %s", sc.ModelName())
	}
}

func TestReloadRejectsOAuth2TokenURLChange(t *testing.T) {
	currentCfg := func() *kaconfig.Config {
		c := baseCfg()
		c.AI.LLM.OAuth2 = kaconfig.OAuth2Config{
			Enabled:      true,
			TokenURL:     "https://idp.corp.com/token",
			ClientID:     "my-client",
			ClientSecret: "my-secret",
		}
		return c
	}

	sc := setupSwappable(t)
	cb := configReloadCallback("/tmp/fake.yaml", currentCfg, sc, testLogger())

	err := cb(`ai:
  llm:
    provider: openai
    model: gpt-4
    endpoint: http://localhost:11434
    apiKey: test-key
    oauth2:
      enabled: true
      tokenURL: https://evil.com/token
      credentialsDir: /tmp/oauth2
  investigation:
    maxTurns: 40
`)
	if err == nil {
		t.Fatal("expected error for OAuth2 tokenURL change")
	}
}

func TestReloadAcceptsModelChange(t *testing.T) {
	sc := setupSwappable(t)
	cb := configReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

	err := cb(`ai:
  llm:
    provider: openai
    model: gpt-4-turbo
    endpoint: http://localhost:11434
    apiKey: test-key
  investigation:
    maxTurns: 40
`)
	if err != nil {
		t.Fatalf("model change should succeed, got: %v", err)
	}
	if sc.ModelName() != "gpt-4-turbo" {
		t.Fatalf("expected model gpt-4-turbo, got %s", sc.ModelName())
	}
}

func TestReloadAcceptsEndpointChange(t *testing.T) {
	sc := setupSwappable(t)
	cb := configReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

	err := cb(`ai:
  llm:
    provider: openai
    model: gpt-4
    endpoint: http://new-endpoint:8080
    apiKey: test-key
  investigation:
    maxTurns: 40
`)
	if err != nil {
		t.Fatalf("endpoint change should succeed, got: %v", err)
	}
}

func TestReloadRejectsValidationFailure(t *testing.T) {
	sc := setupSwappable(t)
	cb := configReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

	err := cb(`ai:
  llm:
    provider: openai
    model: ""
    endpoint: http://localhost:11434
    apiKey: test-key
  investigation:
    maxTurns: 40
`)
	if err == nil {
		t.Fatal("expected error for validation failure (empty model)")
	}
	if sc.ModelName() != "gpt-4" {
		t.Fatalf("model should not change on rejected reload, got %s", sc.ModelName())
	}
}

func TestReloadRejectsTokenURLHTTPScheme(t *testing.T) {
	currentCfg := func() *kaconfig.Config {
		c := baseCfg()
		c.AI.LLM.OAuth2 = kaconfig.OAuth2Config{
			Enabled:      true,
			TokenURL:     "http://insecure.com/token",
			ClientID:     "my-client",
			ClientSecret: "my-secret",
		}
		return c
	}

	sc := setupSwappable(t)
	cb := configReloadCallback("/tmp/fake.yaml", currentCfg, sc, testLogger())

	err := cb(`ai:
  llm:
    provider: openai
    model: gpt-4
    endpoint: http://localhost:11434
    apiKey: test-key
    oauth2:
      enabled: true
      tokenURL: http://insecure.com/token
      credentialsDir: /tmp/oauth2
  investigation:
    maxTurns: 40
`)
	if err == nil {
		t.Fatal("expected error for http:// tokenURL scheme")
	}
}

func TestReloadRejectsStructuredOutputChange(t *testing.T) {
	sc := setupSwappable(t)
	cb := configReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

	err := cb(`ai:
  llm:
    provider: openai
    model: gpt-4
    endpoint: http://localhost:11434
    apiKey: test-key
    structuredOutput: true
  investigation:
    maxTurns: 40
`)
	if err == nil {
		t.Fatal("expected error for structuredOutput change")
	}
}

func TestReloadFreshCopyInvariant(t *testing.T) {
	origCfg := baseCfg()
	origModel := origCfg.AI.LLM.Model

	sc := setupSwappable(t)
	cb := configReloadCallback("/tmp/fake.yaml", func() *kaconfig.Config { return origCfg }, sc, testLogger())

	_ = cb(`ai:
  llm:
    provider: openai
    model: ""
  investigation:
    maxTurns: 40
`)

	if origCfg.AI.LLM.Model != origModel {
		t.Fatalf("live config must not be mutated on failed reload; model was %q, now %q", origModel, origCfg.AI.LLM.Model)
	}
}
