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

package launcher

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	adkanthropic "github.com/Alcova-AI/adk-anthropic-go"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/genai"

	internaltransport "github.com/jordigilh/kubernaut/internal/kubernautagent/llm/transport"
	openaimodel "github.com/jordigilh/kubernaut/pkg/apifrontend/launcher/openai"
	llmtransport "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// NewModelFromConfig constructs an ADK model.LLM from the AF LLM configuration.
// It builds the appropriate transport chain (TLS CA, OAuth2, custom headers,
// circuit breaker) and creates the provider-specific model client.
//
// The Anthropic/Vertex cases below use adk-anthropic-go's model.LLM wrapper
// rather than kubernautagent's anthropicfamily.Client: AF's launcher is
// entirely ADK-based (session/event/tool semantics all speak ADK's
// model.LLM contract), while anthropicfamily implements KA's own,
// deliberately framework-independent llm.Client interface (DD-HAPI-019-001).
// This is an intentional architectural boundary, not an inconsistency to
// converge — see DD-LLM-007. Note the Anthropic/Vertex cases below still
// don't thread cfg.Reasoning: AF has no reasoning/thinking-token support on
// that path today (unlike its OpenAI-compatible path, which gained
// reasoning-content capture for free via the shared openaicompat core,
// DD-LLM-004, and now also threads cfg.Reasoning.Effort, #1604, via
// newOpenAICompatibleModel below) — the Anthropic/Vertex gap remains
// tracked in DD-LLM-007, not fixed by it.
func NewModelFromConfig(ctx context.Context, cfg types.LLMConfig) (model.LLM, error) {
	switch cfg.Provider {
	case types.LLMProviderVertexAI:
		return newVertexAIModel(ctx, cfg)
	case types.LLMProviderGemini:
		return newGeminiModel(ctx, cfg)
	case types.LLMProviderAnthropic:
		return newAnthropicModel(ctx, cfg)
	case types.LLMProviderOpenAI, types.LLMProviderOpenAICompatible:
		return newOpenAICompatibleModel(cfg) //nolint:contextcheck // LLM transport chain lazily builds an OAuth2 client-credentials token source shared across future requests
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %q", cfg.Provider)
	}
}

func newVertexAIModel(ctx context.Context, cfg types.LLMConfig) (m model.LLM, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("vertex_ai: GCP ADC unavailable — set GOOGLE_APPLICATION_CREDENTIALS or provide credentials: %v", r)
		}
	}()
	adkCfg := &adkanthropic.Config{
		Variant:         adkanthropic.VariantVertexAI,
		VertexProjectID: cfg.VertexProject,
		VertexLocation:  cfg.VertexLocation,
	}
	if cfg.Endpoint != "" {
		adkCfg.BaseURL = cfg.Endpoint
	}
	return adkanthropic.NewModel(ctx, cfg.Model, adkCfg)
}

func newGeminiModel(ctx context.Context, cfg types.LLMConfig) (model.LLM, error) {
	clientCfg := &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	}
	if cfg.Endpoint != "" {
		clientCfg.HTTPOptions = genai.HTTPOptions{
			BaseURL: cfg.Endpoint,
		}
	}

	httpClient, err := BuildLLMHTTPClient(cfg) //nolint:contextcheck // LLM transport chain lazily builds an OAuth2 client-credentials token source shared across future requests
	if err != nil {
		return nil, fmt.Errorf("build HTTP client: %w", err)
	}
	if httpClient != nil {
		clientCfg.HTTPClient = httpClient
	}

	return gemini.NewModel(ctx, cfg.Model, clientCfg)
}

func newAnthropicModel(ctx context.Context, cfg types.LLMConfig) (model.LLM, error) {
	adkCfg := &adkanthropic.Config{
		Variant: adkanthropic.VariantAnthropicAPI,
		APIKey:  cfg.APIKey,
	}
	if cfg.Endpoint != "" {
		adkCfg.BaseURL = cfg.Endpoint
	}
	return adkanthropic.NewModel(ctx, cfg.Model, adkCfg)
}

// BuildLLMHTTPClient constructs an HTTP client with the transport chain
// (TLS CA, OAuth2, custom headers, circuit breaker) when any auth/resilience
// options are configured. Returns (nil, nil) when no custom transport is
// needed. Exported (rather than kept package-private) because it's reused
// by cmd/apifrontend's OpenAI-compatible severity-triage wiring (#1618), in
// addition to this package's own OpenAI-compatible/Gemini model
// construction — not just a testing-only need.
func BuildLLMHTTPClient(cfg types.LLMConfig) (*http.Client, error) {
	rt, err := buildTransportChain(cfg)
	if err != nil {
		return nil, err
	}
	// nolint:nilnil // intentional "no custom transport" sentinel, not an
	// error — all 3 callers already guard with `if httpClient != nil` before
	// use, and http.Client falls back to DefaultTransport when Transport is
	// nil (Issue #1546 Tier 2).
	if rt == nil {
		return nil, nil
	}
	timeout := time.Duration(types.DefaultLLMTimeoutSeconds) * time.Second
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	return &http.Client{
		Transport: rt,
		Timeout:   timeout,
	}, nil
}

// buildTransportChain composes the HTTP transport stack from config.
// Chain order (outermost first): CircuitBreaker -> CustomHeaders -> OAuth2 -> TLS/base
// Returns (nil, nil) when no custom transport is needed.
//
// Issue #1342: This transport chain is applied to the Gemini provider (via
// BuildLLMHTTPClient). Vertex AI and Anthropic providers cannot receive a custom
// transport yet because the ADK wrapper (adk-anthropic-go) does not expose HTTP
// client injection. An upstream PR adding BaseTransport to Config is pending;
// once merged, newVertexAIModel and newAnthropicModel should call
// BuildLLMHTTPClient. The AF validation gate for these providers has been
// removed (Phase 3) to allow transport config in preparation.
func buildTransportChain(cfg types.LLMConfig) (http.RoundTripper, error) {
	base := http.DefaultTransport
	needsCustom := false

	if cfg.TLSCaFile != "" {
		var tlsOpts []sharedtls.TLSTransportOption
		if cfg.TLSCertFile != "" {
			tlsOpts = append(tlsOpts, sharedtls.WithClientCert(cfg.TLSCertFile, cfg.TLSKeyFile))
		}
		tlsTransport, err := sharedtls.NewTLSTransport(cfg.TLSCaFile, tlsOpts...)
		if err != nil {
			return nil, fmt.Errorf("load TLS CA %q: %w", cfg.TLSCaFile, err)
		}
		base = tlsTransport
		needsCustom = true
	}

	if cfg.OAuth2.Enabled {
		oauth2Cfg := cfg.OAuth2
		if err := resolveOAuth2Secrets(&oauth2Cfg); err != nil {
			return nil, fmt.Errorf("resolve OAuth2 secrets: %w", err)
		}
		base = internaltransport.NewOAuth2ClientCredentialsTransport(oauth2Cfg, base)
		needsCustom = true
	}

	if len(cfg.CustomHeaders) > 0 {
		base = llmtransport.NewAuthHeadersTransport(cfg.CustomHeaders, base)
		needsCustom = true
	}

	if cfg.CircuitBreaker.Enabled {
		base = transport.NewCircuitBreakerTransport(base, transport.CircuitBreakerConfig{
			Enabled:          true,
			Name:             "af-llm",
			MaxRequests:      cfg.CircuitBreaker.MaxRequests,
			Interval:         cfg.CircuitBreaker.Interval,
			Timeout:          cfg.CircuitBreaker.Timeout,
			FailureThreshold: cfg.CircuitBreaker.FailureThreshold,
		})
		needsCustom = true
	}

	// nolint:nilnil // intentional "no custom transport" sentinel, not an
	// error — already documented above ("Returns (nil, nil) when no custom
	// transport is needed"); sole caller (BuildLLMHTTPClient) already guards
	// with `if rt == nil` (Issue #1546 Tier 2).
	if !needsCustom {
		return nil, nil
	}
	return base, nil
}

func newOpenAICompatibleModel(cfg types.LLMConfig) (model.LLM, error) {
	var opts []openaimodel.Option

	httpClient, err := BuildLLMHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("build HTTP client: %w", err)
	}
	if httpClient != nil {
		opts = append(opts, openaimodel.WithHTTPClient(httpClient))
	}
	// AzureAPIVersion is the sole detection signal for Azure OpenAI (#1600)
	// — there is no separate "azure" provider enum value; Azure is layered
	// on top of provider: openai/openai_compatible, matching KA's dispatch
	// (cmd/kubernautagent/llm_builder.go's buildOpenAICompatClient). Net-new
	// capability for AF — it never had Azure support before.
	if cfg.AzureAPIVersion != "" {
		opts = append(opts, openaimodel.WithAzureAPIVersion(cfg.AzureAPIVersion))
	}
	if cfg.Reasoning != nil && cfg.Reasoning.Enabled {
		opts = append(opts, openaimodel.WithReasoningEffort(cfg.Reasoning.Effort))
	}

	return openaimodel.NewModel(cfg.Model, cfg.Endpoint, cfg.APIKey, opts...), nil
}

// resolveOAuth2Secrets reads client-id and client-secret from the mounted
// secrets directory (same layout as KA: <credentialsDir>/client-id, client-secret).
func resolveOAuth2Secrets(cfg *types.LLMOAuth2Config) error {
	if cfg.CredentialsDir == "" {
		return nil
	}
	data, err := os.ReadFile(cfg.CredentialsDir + "/client-id")
	if err != nil {
		return fmt.Errorf("read oauth2 client-id from %s: %w", cfg.CredentialsDir, err)
	}
	cfg.ClientID = strings.TrimSpace(string(data))

	data, err = os.ReadFile(cfg.CredentialsDir + "/client-secret")
	if err != nil {
		return fmt.Errorf("read oauth2 client-secret from %s: %w", cfg.CredentialsDir, err)
	}
	cfg.ClientSecret = strings.TrimSpace(string(data))
	return nil
}
