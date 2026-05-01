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
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

func testReloadLogger() logr.Logger {
	return logr.Discard()
}

func staticCfg() *kaconfig.Config {
	cfg := kaconfig.DefaultConfig()
	cfg.AI.LLM.Provider = "openai"
	return cfg
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

func (s *stubLLMClient) StreamChat(_ context.Context, _ llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, nil
}

func (s *stubLLMClient) Close() error { return nil }

func TestReloadRejectsEmptyContent(t *testing.T) {
	sc := setupSwappable(t)
	cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger())

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
	cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger())

	err := cb("   \n  \t  ")
	if err == nil {
		t.Fatal("expected error for whitespace-only content")
	}
}

func TestReloadAcceptsModelChange(t *testing.T) {
	sc := setupSwappable(t)
	cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger())

	err := cb(`model: gpt-4-turbo
endpoint: http://localhost:11434
apiKey: test-key
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
	cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger())

	err := cb(`model: gpt-4
endpoint: http://new-endpoint:8080
apiKey: test-key
`)
	if err != nil {
		t.Fatalf("endpoint change should succeed, got: %v", err)
	}
}

func TestReloadRejectsValidationFailure(t *testing.T) {
	sc := setupSwappable(t)
	cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger())

	err := cb(`model: ""
endpoint: http://localhost:11434
apiKey: test-key
`)
	if err == nil {
		t.Fatal("expected error for validation failure (empty model)")
	}
	if sc.ModelName() != "gpt-4" {
		t.Fatalf("model should not change on rejected reload, got %s", sc.ModelName())
	}
}

func TestReloadAcceptsTemperatureChange(t *testing.T) {
	sc := setupSwappable(t)
	cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger())

	err := cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
temperature: 0.9
`)
	if err != nil {
		t.Fatalf("temperature change should succeed, got: %v", err)
	}
}
