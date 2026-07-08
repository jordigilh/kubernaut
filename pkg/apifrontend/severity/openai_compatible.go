package severity

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
)

// ChatClient abstracts a single non-streaming Chat Completions call for
// testability, mirroring ContentGenerator's role for GenAITriager.
// *openaicompat.Client satisfies this directly.
type ChatClient interface {
	Chat(ctx context.Context, req openaicompat.Request) (*openaicompat.Response, error)
}

// OpenAICompatibleTriager implements LLMTriager against any OpenAI Chat
// Completions-compatible endpoint (OpenAI, Azure OpenAI, vLLM, Ollama,
// LlamaStack, self-hosted/custom) via the shared openaicompat client
// (#1618). Unlike GenAITriager/AnthropicTriager, there is no vendor SDK
// here — openaicompat.Client already speaks the wire protocol directly.
type OpenAICompatibleTriager struct {
	client ChatClient
	model  string
	logger logr.Logger
}

// OpenAICompatibleTriagerConfig holds construction parameters for
// OpenAICompatibleTriager. Exactly one of Client or ChatClient must be set:
// Client is the production path (a real *openaicompat.Client), ChatClient
// is the test-injection seam.
type OpenAICompatibleTriagerConfig struct {
	Client     *openaicompat.Client
	ChatClient ChatClient
	Model      string
	Logger     logr.Logger
}

// NewOpenAICompatibleTriager creates a production LLMTriager backed by an
// OpenAI-compatible endpoint. Panics if neither Client nor ChatClient is
// set, matching NewGenAITriager's fail-fast-on-misconfiguration contract.
func NewOpenAICompatibleTriager(cfg OpenAICompatibleTriagerConfig) *OpenAICompatibleTriager {
	client := cfg.ChatClient
	if client == nil {
		if cfg.Client == nil {
			panic("NewOpenAICompatibleTriager: Client or ChatClient must not be nil")
		}
		client = cfg.Client
	}
	if cfg.Logger.GetSink() == nil {
		cfg.Logger = logr.Discard()
	}
	return &OpenAICompatibleTriager{
		client: client,
		model:  cfg.Model,
		logger: cfg.Logger,
	}
}

// TriageWithRules classifies severity using LLM with matched rule context.
func (o *OpenAICompatibleTriager) TriageWithRules(ctx context.Context, rules []prom.Rule, input TriageInput) (TriageResult, error) {
	prompt := BuildTriagePrompt(input, rules)
	return o.classify(ctx, prompt)
}

// TriagePure classifies severity using LLM without rule context (pure fallback).
func (o *OpenAICompatibleTriager) TriagePure(ctx context.Context, input TriageInput) (TriageResult, error) {
	prompt := BuildTriagePrompt(input, nil)
	return o.classify(ctx, prompt)
}

func (o *OpenAICompatibleTriager) classify(ctx context.Context, prompt string) (TriageResult, error) {
	req := openaicompat.Request{
		Model:    o.model,
		Messages: []openaicompat.Message{{Role: "user", Content: prompt}},
	}
	resp, err := o.client.Chat(ctx, req)
	if err != nil {
		return TriageResult{}, fmt.Errorf("LLM call failed: %w", err)
	}
	if resp == nil {
		return TriageResult{}, fmt.Errorf("LLM returned nil response")
	}

	text := strings.TrimSpace(resp.Message.Content)
	if text == "" {
		return TriageResult{}, fmt.Errorf("LLM returned empty response")
	}

	sev := NormalizeSeverity(text)
	confidence := 1.0
	if !ValidateSeverity(strings.TrimSpace(strings.ToLower(text))) {
		confidence = 0.5
	}

	return TriageResult{
		Severity:   sev,
		Confidence: confidence,
	}, nil
}
