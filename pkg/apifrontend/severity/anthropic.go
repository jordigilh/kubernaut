package severity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/vertex"
	"github.com/go-logr/logr"
	"golang.org/x/oauth2/google"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
)

const vertexAICloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

// anthropicVertexAllowedCredentialTypes lists the GCP credential types
// NewAnthropicVertexClient accepts from an explicitly-supplied
// credentialsJSON. external_account and similar types are rejected to
// prevent loading credentials with attacker-controlled token endpoints,
// mirroring the same allow-list already enforced for Kubernaut Agent's
// equivalent Vertex path and API Frontend's own release/v1.5 fix
// (kubernaut#1731/#1870).
var anthropicVertexAllowedCredentialTypes = map[google.CredentialsType]bool{
	google.ServiceAccount: true,
	google.AuthorizedUser: true,
}

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

func (a *AnthropicTriager) classify(ctx context.Context, prompt string) (TriageResult, error) {
	params := anthropic.MessageNewParams{
		Model:     a.model,
		MaxTokens: int64(64),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	}

	resp, err := a.messager.Create(ctx, params)
	if err != nil {
		return TriageResult{}, fmt.Errorf("anthropic LLM call failed: %w", err)
	}
	if resp == nil {
		return TriageResult{}, fmt.Errorf("anthropic LLM returned nil response")
	}

	text := extractAnthropicText(resp)
	if text == "" {
		return TriageResult{}, fmt.Errorf("anthropic LLM returned empty response")
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

// NewAnthropicVertexClient creates an Anthropic SDK client configured for
// Claude on Vertex AI. When credentialsJSON is non-empty (kubernaut#1861,
// mirroring kubernaut#1870's release/v1.5 fix), it resolves explicit Google
// credentials from those bytes instead of relying on ambient Application
// Default Credentials (ADC) -- letting AF's severityTriage profile
// authenticate independently of its own agent.llm profile when both
// resolve to vertex_ai. Contrary to this package's prior assumption,
// anthropic-sdk-go's Vertex option does accept explicit *google.Credentials
// via vertex.WithCredentials, not just ambient ADC via vertex.WithGoogleAuth
// (a thin wrapper around the same). Falls back to vertex.WithGoogleAuth
// unchanged when credentialsJSON is empty, preserving today's behavior.
//
// Note: AF's own agent.llm Claude-on-Vertex connection
// (pkg/apifrontend/launcher/model.go's newVertexAnthropicModel) goes
// through the third-party adk-anthropic-go module instead of this
// function, which -- as of adk-anthropic-go v1.0.0 -- hardcodes
// vertex.WithGoogleAuth internally with no credentials override exposed.
// That path still relies on ambient ADC; this fix only closes the gap for
// severityTriage's independent Vertex connection, which is exactly the one
// the removed DD-PLATFORM-007 fail() guard used to block.
func NewAnthropicVertexClient(ctx context.Context, project, location, credentialsJSON string) (client *anthropic.Client, err error) {
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
	vertexOpt, err := resolveAnthropicVertexAuth(ctx, []byte(credentialsJSON), location, project)
	if err != nil {
		return nil, err
	}
	c := anthropic.NewClient(vertexOpt)
	return &c, nil
}

// resolveAnthropicVertexAuth resolves the Anthropic SDK's Vertex AI request
// option from explicit credential bytes when present, falling back to
// ambient ADC (vertex.WithGoogleAuth) when empty. vertex.WithGoogleAuth is
// itself a thin wrapper around vertex.WithCredentials that always resolves
// ambient ADC; this mirrors the same explicit-bytes-first technique already
// proven for Kubernaut Agent's equivalent Vertex path (kubernaut#1728) and
// API Frontend's own release/v1.5 fix (kubernaut#1870).
func resolveAnthropicVertexAuth(ctx context.Context, credentialsJSON []byte, location, project string) (option.RequestOption, error) {
	trimmed := bytes.TrimSpace(credentialsJSON)
	if len(trimmed) == 0 {
		return vertex.WithGoogleAuth(ctx, location, project, vertexAICloudPlatformScope), nil
	}
	credType, err := anthropicVertexCredentialType(trimmed)
	if err != nil {
		return nil, err
	}
	creds, err := google.CredentialsFromJSONWithType(ctx, trimmed, credType, vertexAICloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("vertex_ai: invalid credentials JSON: %w", err)
	}
	return vertex.WithCredentials(ctx, location, project, creds), nil
}

// anthropicVertexCredentialType parses the "type" field from the credential
// JSON and rejects anything outside anthropicVertexAllowedCredentialTypes.
func anthropicVertexCredentialType(jsonData []byte) (google.CredentialsType, error) {
	var f struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(jsonData, &f); err != nil {
		return "", fmt.Errorf("vertex_ai: invalid credentials JSON: %w", err)
	}
	ct := google.CredentialsType(f.Type)
	if !anthropicVertexAllowedCredentialTypes[ct] {
		return "", fmt.Errorf("vertex_ai: unsupported credential type %q; only service_account and authorized_user are accepted", f.Type)
	}
	return ct, nil
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
