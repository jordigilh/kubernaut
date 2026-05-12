package severity

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"google.golang.org/genai"

	prom "github.com/jordigilh/kubernaut-apifrontend/internal/prometheus"
)

// ContentGenerator abstracts the LLM content generation call for testability.
type ContentGenerator interface {
	GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
}

// genaiModels adapts genai.Models to the ContentGenerator interface.
type genaiModels struct {
	models genai.Models
}

func (g *genaiModels) GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return g.models.GenerateContent(ctx, model, contents, config)
}

// GenAITriager implements LLMTriager using Google GenAI (Vertex AI).
type GenAITriager struct {
	generator ContentGenerator
	model     string
	logger    logr.Logger
}

// GenAITriagerConfig holds construction parameters for GenAITriager.
type GenAITriagerConfig struct {
	Client    *genai.Client
	Generator ContentGenerator
	Model     string
	Logger    logr.Logger
}

// NewGenAITriager creates a production LLMTriager backed by Google GenAI.
// If Generator is set, it is used directly; otherwise Client.Models is wrapped.
func NewGenAITriager(cfg GenAITriagerConfig) *GenAITriager {
	var gen ContentGenerator
	if cfg.Generator != nil {
		gen = cfg.Generator
	} else {
		if cfg.Client == nil {
			panic("NewGenAITriager: Client or Generator must not be nil")
		}
		gen = &genaiModels{models: *cfg.Client.Models}
	}
	if cfg.Model == "" {
		cfg.Model = "gemini-2.0-flash"
	}
	if cfg.Logger.GetSink() == nil {
		cfg.Logger = logr.Discard()
	}
	return &GenAITriager{
		generator: gen,
		model:     cfg.Model,
		logger:    cfg.Logger,
	}
}

// TriageWithRules classifies severity using LLM with matched rule context.
func (g *GenAITriager) TriageWithRules(ctx context.Context, rules []prom.Rule, input TriageInput) (TriageResult, error) {
	prompt := BuildTriagePrompt(input, rules)
	return g.classify(ctx, prompt)
}

// TriagePure classifies severity using LLM without rule context (pure fallback).
func (g *GenAITriager) TriagePure(ctx context.Context, input TriageInput) (TriageResult, error) {
	prompt := BuildTriagePrompt(input, nil)
	return g.classify(ctx, prompt)
}

func (g *GenAITriager) classify(ctx context.Context, prompt string) (TriageResult, error) {
	resp, err := g.generator.GenerateContent(ctx, g.model, genai.Text(prompt), nil)
	if err != nil {
		return TriageResult{}, fmt.Errorf("LLM call failed: %w", err)
	}
	if resp == nil {
		return TriageResult{}, fmt.Errorf("LLM returned nil response")
	}

	text := extractText(resp)
	if text == "" {
		return TriageResult{}, fmt.Errorf("LLM returned empty response")
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

func extractText(resp *genai.GenerateContentResponse) string {
	if resp == nil || len(resp.Candidates) == 0 {
		return ""
	}
	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return ""
	}
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			return strings.TrimSpace(part.Text)
		}
	}
	return ""
}

// NoopLLMTriager is a stub LLMTriager for dev/CI environments without LLM credentials.
// It always returns "medium" severity with full confidence.
type NoopLLMTriager struct {
	logger logr.Logger
}

// NewNoopLLMTriager creates a no-op triager that satisfies the non-nil LLMTriager
// requirement. It logs a warning at construction time.
func NewNoopLLMTriager(logger logr.Logger) *NoopLLMTriager {
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	logger.Info("WARNING: using NoopLLMTriager — all LLM triage calls will return 'medium'")
	return &NoopLLMTriager{logger: logger}
}

// TriageWithRules returns a static "medium" result (noop implementation).
func (n *NoopLLMTriager) TriageWithRules(_ context.Context, _ []prom.Rule, _ TriageInput) (TriageResult, error) {
	return TriageResult{
		Severity:   "medium",
		Confidence: 1.0,
	}, nil
}

// TriagePure returns a static "medium" result (noop implementation).
func (n *NoopLLMTriager) TriagePure(_ context.Context, _ TriageInput) (TriageResult, error) {
	return TriageResult{
		Severity:   "medium",
		Confidence: 1.0,
	}, nil
}
