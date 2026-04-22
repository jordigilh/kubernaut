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

// baseCfg returns a config that simulates a live config after initial SDK merge.
// In production, main config typically has minimal LLM settings and the SDK
// fills the gaps.
func baseCfg() *kaconfig.Config {
	cfg := kaconfig.DefaultConfig()
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-4"
	cfg.LLM.Endpoint = "http://localhost:11434"
	cfg.LLM.APIKey = "test-key"
	return cfg
}

// baseCfgYAML returns main config YAML with empty LLM fields.
// In production, the main config file does NOT contain LLM fields --
// those come exclusively from the SDK config via MergeSDKConfig.
func baseCfgYAML() string {
	return `llm:
  provider: openai
investigator:
  max_turns: 15
`
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

func withReadFile(fn func(string) ([]byte, error)) func() {
	old := readFile
	readFile = fn
	return func() { readFile = old }
}

func TestReloadRejectsEmptyContent(t *testing.T) {
	sc := setupSwappable(t)
	cb := sdkReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

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
	cb := sdkReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

	err := cb("   \n  \t  ")
	if err == nil {
		t.Fatal("expected error for whitespace-only content")
	}
}

func TestReloadRejectsProviderChange(t *testing.T) {
	restore := withReadFile(func(string) ([]byte, error) {
		return []byte(baseCfgYAML()), nil
	})
	defer restore()

	sc := setupSwappable(t)
	cb := sdkReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

	sdkContent := `llm:
  provider: anthropic
  model: claude-3
`
	err := cb(sdkContent)
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
		c.LLM.OAuth2 = kaconfig.OAuth2Config{
			Enabled:      true,
			TokenURL:     "https://idp.corp.com/token",
			ClientID:     "my-client",
			ClientSecret: "my-secret",
		}
		return c
	}

	restore := withReadFile(func(string) ([]byte, error) {
		return []byte(baseCfgYAML()), nil
	})
	defer restore()

	sc := setupSwappable(t)
	cb := sdkReloadCallback("/tmp/fake.yaml", currentCfg, sc, testLogger())

	err := cb(`llm:
  model: gpt-4
  oauth2:
    enabled: true
    token_url: https://evil.com/token
    client_id: my-client
    client_secret: my-secret
`)
	if err == nil {
		t.Fatal("expected error for OAuth2 token_url change")
	}
}

func TestReloadRejectsOAuth2ClientIDChange(t *testing.T) {
	currentCfg := func() *kaconfig.Config {
		c := baseCfg()
		c.LLM.OAuth2 = kaconfig.OAuth2Config{
			Enabled:      true,
			TokenURL:     "https://idp.corp.com/token",
			ClientID:     "my-client",
			ClientSecret: "my-secret",
		}
		return c
	}

	restore := withReadFile(func(string) ([]byte, error) {
		return []byte(baseCfgYAML()), nil
	})
	defer restore()

	sc := setupSwappable(t)
	cb := sdkReloadCallback("/tmp/fake.yaml", currentCfg, sc, testLogger())

	err := cb(`llm:
  model: gpt-4
  oauth2:
    enabled: true
    token_url: https://idp.corp.com/token
    client_id: different-client
    client_secret: my-secret
`)
	if err == nil {
		t.Fatal("expected error for OAuth2 client_id change")
	}
}

func TestReloadRejectsOAuth2ClientSecretChange(t *testing.T) {
	currentCfg := func() *kaconfig.Config {
		c := baseCfg()
		c.LLM.OAuth2 = kaconfig.OAuth2Config{
			Enabled:      true,
			TokenURL:     "https://idp.corp.com/token",
			ClientID:     "my-client",
			ClientSecret: "my-secret",
		}
		return c
	}

	restore := withReadFile(func(string) ([]byte, error) {
		return []byte(baseCfgYAML()), nil
	})
	defer restore()

	sc := setupSwappable(t)
	cb := sdkReloadCallback("/tmp/fake.yaml", currentCfg, sc, testLogger())

	err := cb(`llm:
  model: gpt-4
  oauth2:
    enabled: true
    token_url: https://idp.corp.com/token
    client_id: my-client
    client_secret: different-secret
`)
	if err == nil {
		t.Fatal("expected error for OAuth2 client_secret change")
	}
}

func TestReloadAcceptsOAuth2ScopesChange(t *testing.T) {
	currentCfg := func() *kaconfig.Config {
		c := baseCfg()
		c.LLM.OAuth2 = kaconfig.OAuth2Config{
			Enabled:      true,
			TokenURL:     "https://idp.corp.com/token",
			ClientID:     "my-client",
			ClientSecret: "my-secret",
			Scopes:       []string{"read"},
		}
		return c
	}

	restore := withReadFile(func(string) ([]byte, error) {
		return []byte(baseCfgYAML()), nil
	})
	defer restore()

	sc := setupSwappable(t)
	cb := sdkReloadCallback("/tmp/fake.yaml", currentCfg, sc, testLogger())

	sdkContent := `llm:
  model: gpt-4-turbo
  endpoint: http://localhost:11434
  api_key: test-key
  oauth2:
    enabled: true
    token_url: https://idp.corp.com/token
    client_id: my-client
    client_secret: my-secret
    scopes:
      - read
      - write
`
	err := cb(sdkContent)
	if err != nil {
		t.Fatalf("scopes change should be allowed, got: %v", err)
	}
	if sc.ModelName() != "gpt-4-turbo" {
		t.Fatalf("expected model gpt-4-turbo after scopes change, got %s", sc.ModelName())
	}
}

func TestReloadAcceptsModelChange(t *testing.T) {
	restore := withReadFile(func(string) ([]byte, error) {
		return []byte(baseCfgYAML()), nil
	})
	defer restore()

	sc := setupSwappable(t)
	cb := sdkReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

	err := cb(`llm:
  model: gpt-4-turbo
  endpoint: http://localhost:11434
  api_key: test-key
`)
	if err != nil {
		t.Fatalf("model change should succeed, got: %v", err)
	}
	if sc.ModelName() != "gpt-4-turbo" {
		t.Fatalf("expected model gpt-4-turbo, got %s", sc.ModelName())
	}
}

func TestReloadAcceptsEndpointChange(t *testing.T) {
	restore := withReadFile(func(string) ([]byte, error) {
		return []byte(baseCfgYAML()), nil
	})
	defer restore()

	sc := setupSwappable(t)
	cb := sdkReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

	err := cb(`llm:
  model: gpt-4
  endpoint: http://new-endpoint:8080
  api_key: test-key
`)
	if err != nil {
		t.Fatalf("endpoint change should succeed, got: %v", err)
	}
}

func TestReloadRejectsValidationFailure(t *testing.T) {
	restore := withReadFile(func(string) ([]byte, error) {
		return []byte(`llm:
  provider: openai
  model: ""
investigator:
  max_turns: 15
`), nil
	})
	defer restore()

	sc := setupSwappable(t)
	cb := sdkReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

	err := cb(`llm:
  model: ""
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
		c.LLM.OAuth2 = kaconfig.OAuth2Config{
			Enabled:      true,
			TokenURL:     "http://insecure.com/token",
			ClientID:     "my-client",
			ClientSecret: "my-secret",
		}
		return c
	}

	restore := withReadFile(func(string) ([]byte, error) {
		return []byte(baseCfgYAML()), nil
	})
	defer restore()

	sc := setupSwappable(t)
	cb := sdkReloadCallback("/tmp/fake.yaml", currentCfg, sc, testLogger())

	err := cb(`llm:
  model: gpt-4
  oauth2:
    enabled: true
    token_url: http://insecure.com/token
    client_id: my-client
    client_secret: my-secret
`)
	if err == nil {
		t.Fatal("expected error for http:// token_url scheme")
	}
}

func TestReloadRejectsStructuredOutputChange(t *testing.T) {
	restore := withReadFile(func(string) ([]byte, error) {
		return []byte(baseCfgYAML()), nil
	})
	defer restore()

	sc := setupSwappable(t)
	cb := sdkReloadCallback("/tmp/fake.yaml", baseCfg, sc, testLogger())

	err := cb(`llm:
  structured_output: true
`)
	if err == nil {
		t.Fatal("expected error for structured_output change")
	}
}

func TestReloadFreshCopyInvariant(t *testing.T) {
	restore := withReadFile(func(string) ([]byte, error) {
		return []byte(`llm:
  provider: openai
  model: ""
investigator:
  max_turns: 15
`), nil
	})
	defer restore()

	origCfg := baseCfg()
	origModel := origCfg.LLM.Model

	sc := setupSwappable(t)
	cb := sdkReloadCallback("/tmp/fake.yaml", func() *kaconfig.Config { return origCfg }, sc, testLogger())

	// This will fail validation (empty model after merge)
	_ = cb(`llm:
  model: ""
`)

	if origCfg.LLM.Model != origModel {
		t.Fatalf("live config must not be mutated on failed reload; model was %q, now %q", origModel, origCfg.LLM.Model)
	}
}
