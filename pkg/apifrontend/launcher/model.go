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
	"github.com/anthropics/anthropic-sdk-go"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/genai"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	internaltransport "github.com/jordigilh/kubernaut/internal/kubernautagent/llm/transport"
	pkgconfig "github.com/jordigilh/kubernaut/pkg/kubernautagent/config"
	llmtransport "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

// NewModelFromConfig constructs an ADK model.LLM from the AF LLM configuration.
// It builds the appropriate transport chain (TLS CA, OAuth2, custom headers,
// circuit breaker) and creates the provider-specific model client.
func NewModelFromConfig(ctx context.Context, cfg config.LLMConfig) (model.LLM, error) {
	switch cfg.Provider {
	case config.LLMProviderVertexAI:
		return newVertexAIModel(ctx, cfg)
	case config.LLMProviderGemini:
		return newGeminiModel(ctx, cfg)
	case config.LLMProviderAnthropic:
		return newAnthropicModel(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %q", cfg.Provider)
	}
}

func newVertexAIModel(ctx context.Context, cfg config.LLMConfig) (model.LLM, error) {
	adkCfg := &adkanthropic.Config{
		Variant:         adkanthropic.VariantVertexAI,
		VertexProjectID: cfg.VertexProject,
		VertexLocation:  cfg.VertexLocation,
	}
	if cfg.Endpoint != "" {
		adkCfg.BaseURL = cfg.Endpoint
	}
	return adkanthropic.NewModel(ctx, anthropic.Model(cfg.Model), adkCfg)
}

func newGeminiModel(ctx context.Context, cfg config.LLMConfig) (model.LLM, error) {
	clientCfg := &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	}
	if cfg.Endpoint != "" {
		clientCfg.HTTPOptions = genai.HTTPOptions{
			BaseURL: cfg.Endpoint,
		}
	}

	httpClient, err := buildLLMHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("build HTTP client: %w", err)
	}
	if httpClient != nil {
		clientCfg.HTTPClient = httpClient
	}

	return gemini.NewModel(ctx, cfg.Model, clientCfg)
}

func newAnthropicModel(ctx context.Context, cfg config.LLMConfig) (model.LLM, error) {
	adkCfg := &adkanthropic.Config{
		Variant: adkanthropic.VariantAnthropicAPI,
		APIKey:  cfg.APIKey,
	}
	if cfg.Endpoint != "" {
		adkCfg.BaseURL = cfg.Endpoint
	}
	return adkanthropic.NewModel(ctx, anthropic.Model(cfg.Model), adkCfg)
}

// buildLLMHTTPClient constructs an HTTP client with the transport chain
// (TLS CA, OAuth2, custom headers, circuit breaker) when any auth/resilience
// options are configured. Returns (nil, nil) when no custom transport is needed.
func buildLLMHTTPClient(cfg config.LLMConfig) (*http.Client, error) {
	rt, err := buildTransportChain(cfg)
	if err != nil {
		return nil, err
	}
	if rt == nil {
		return nil, nil
	}
	timeout := time.Duration(config.DefaultLLMTimeoutSeconds) * time.Second
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
// NOTE: This transport chain is currently only applied to the Gemini provider
// (via buildLLMHTTPClient). Vertex AI uses GCP-managed auth (ADC) and the
// Anthropic ADK wrapper does not expose HTTP client injection. If support is
// added for Anthropic transport in the future, validateLLM() must be updated
// to allow it, and newAnthropicModel must call buildLLMHTTPClient.
func buildTransportChain(cfg config.LLMConfig) (http.RoundTripper, error) {
	base := http.DefaultTransport
	needsCustom := false

	if cfg.TLSCaFile != "" {
		tlsTransport, err := sharedtls.NewTLSTransport(cfg.TLSCaFile)
		if err != nil {
			return nil, fmt.Errorf("load TLS CA %q: %w", cfg.TLSCaFile, err)
		}
		base = tlsTransport
		needsCustom = true
	}

	if cfg.OAuth2.Enabled {
		oauth2Cfg := kaconfig.OAuth2Config{
			Enabled:        cfg.OAuth2.Enabled,
			TokenURL:       cfg.OAuth2.TokenURL,
			Scopes:         cfg.OAuth2.Scopes,
			CredentialsDir: cfg.OAuth2.CredentialsDir,
		}
		if err := resolveOAuth2Secrets(&oauth2Cfg); err != nil {
			return nil, fmt.Errorf("resolve OAuth2 secrets: %w", err)
		}
		base = internaltransport.NewOAuth2ClientCredentialsTransport(oauth2Cfg, base)
		needsCustom = true
	}

	if len(cfg.CustomHeaders) > 0 {
		headers := make([]pkgconfig.HeaderDefinition, 0, len(cfg.CustomHeaders))
		for _, h := range cfg.CustomHeaders {
			headers = append(headers, pkgconfig.HeaderDefinition{
				Name:     h.Name,
				Value:    h.Value,
				FilePath: h.FilePath,
			})
		}
		base = llmtransport.NewAuthHeadersTransport(headers, base)
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

	if !needsCustom {
		return nil, nil
	}
	return base, nil
}

// resolveOAuth2Secrets reads client-id and client-secret from the mounted
// secrets directory (same layout as KA: <credentialsDir>/client-id, client-secret).
func resolveOAuth2Secrets(cfg *kaconfig.OAuth2Config) error {
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
