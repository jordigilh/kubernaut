package severity

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/vertex"
	"github.com/go-logr/logr"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
)

// Messager abstracts the Anthropic Messages.New call for testability.
type Messager interface {
	Create(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error)
}

// sdkMessager adapts the real Anthropic SDK client to the Messager interface.
type sdkMessager struct {
	client *anthropic.Client
}

func (s *sdkMessager) Create(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	return s.client.Messages.New(ctx, params)
}

// AnthropicTriager implements LLMTriager using the Anthropic SDK (Messages API).
// Supports both direct Anthropic API and Claude on Vertex AI.
type AnthropicTriager struct {
	messager Messager
	model    string
	logger   logr.Logger
}

// AnthropicTriagerConfig holds construction parameters for AnthropicTriager.
type AnthropicTriagerConfig struct {
	Client   *anthropic.Client
	Messager Messager
	Model    string
	Logger   logr.Logger
}

// NewAnthropicTriager creates an LLMTriager backed by the Anthropic SDK.
// If Messager is set, it is used directly; otherwise Client.Messages is wrapped.
func NewAnthropicTriager(cfg AnthropicTriagerConfig) *AnthropicTriager {
	var m Messager
	if cfg.Messager != nil {
		m = cfg.Messager
	} else {
		if cfg.Client == nil {
			panic("NewAnthropicTriager: Client or Messager must not be nil")
		}
		m = &sdkMessager{client: cfg.Client}
	}
	if cfg.Model == "" {
		cfg.Model = "claude-sonnet-4-6"
	}
	if cfg.Logger.GetSink() == nil {
		cfg.Logger = logr.Discard()
	}
	return &AnthropicTriager{
		messager: m,
		model:    cfg.Model,
		logger:   cfg.Logger,
	}
}

// TriageWithRules classifies severity using Anthropic LLM with matched rule context.
func (a *AnthropicTriager) TriageWithRules(ctx context.Context, rules []prom.Rule, input TriageInput) (TriageResult, error) {
	prompt := BuildTriagePrompt(input, rules)
	return a.classify(ctx, prompt)
}

// TriagePure classifies severity using Anthropic LLM without rule context.
func (a *AnthropicTriager) TriagePure(ctx context.Context, input TriageInput) (TriageResult, error) {
	prompt := BuildTriagePrompt(input, nil)
	return a.classify(ctx, prompt)
}

func (a *AnthropicTriager) classify(ctx context.Context, prompt string) (TriageResult, error) {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: int64(64),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	}

	resp, err := a.messager.Create(ctx, params)
	if err != nil {
		return TriageResult{}, fmt.Errorf("Anthropic LLM call failed: %w", err)
	}
	if resp == nil {
		return TriageResult{}, fmt.Errorf("Anthropic LLM returned nil response")
	}

	text := extractAnthropicText(resp)
	if text == "" {
		return TriageResult{}, fmt.Errorf("Anthropic LLM returned empty response")
	}

	severity := NormalizeSeverity(text)
	confidence := 1.0
	if !ValidateSeverity(strings.TrimSpace(strings.ToLower(text))) {
		confidence = 0.5
	}

	return TriageResult{
		Severity:   severity,
		Confidence: confidence,
	}, nil
}

func extractAnthropicText(resp *anthropic.Message) string {
	if resp == nil || len(resp.Content) == 0 {
		return ""
	}
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			return strings.TrimSpace(block.Text)
		}
	}
	return ""
}

// IsAnthropicModel returns true if the model name indicates an Anthropic model
// (Claude family) that requires the Anthropic SDK rather than the Google GenAI SDK.
func IsAnthropicModel(model string) bool {
	return strings.HasPrefix(model, "claude-")
}

// NewAnthropicVertexClient creates an Anthropic SDK client configured for
// Claude on Vertex AI using Google Cloud ADC (Application Default Credentials).
func NewAnthropicVertexClient(ctx context.Context, project, location string) (client *anthropic.Client, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("GCP ADC unavailable for Anthropic Vertex client: %v", r)
		}
	}()
	if project == "" {
		return nil, fmt.Errorf("vertexProject is required for Anthropic on Vertex AI")
	}
	if location == "" {
		location = "us-central1"
	}
	vertexOpt := vertex.WithGoogleAuth(ctx, location, project,
		"https://www.googleapis.com/auth/cloud-platform")
	c := anthropic.NewClient(vertexOpt)
	return &c, nil
}

// NewAnthropicDirectClient creates an Anthropic SDK client configured for
// direct Anthropic API access using an API key.
func NewAnthropicDirectClient(apiKey string) (*anthropic.Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("apiKey is required for direct Anthropic API access")
	}
	c := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &c, nil
}
