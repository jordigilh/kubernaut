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
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	llmtransport "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/vertexanthropic"
)

// buildLLMClientFromConfig constructs an llm.Client from the given config.
// This is a pure function used both at startup and by the config hot-reload
// callback. It does not mutate cfg.
func buildLLMClientFromConfig(ctx context.Context, cfg *kaconfig.Config) (llm.Client, error) {
	switch cfg.AI.LLM.Provider {
	case "vertex_ai":
		return vertexanthropic.New(ctx,
			cfg.AI.LLM.Model, []byte(cfg.AI.LLM.APIKey),
			cfg.AI.LLM.VertexProject, cfg.AI.LLM.VertexLocation)
	default:
		return langchaingo.New(cfg.AI.LLM.Provider, cfg.AI.LLM.Endpoint, cfg.AI.LLM.Model, cfg.AI.LLM.APIKey,
			buildLLMProviderOpts(cfg)...)
	}
}

// buildLLMProviderOpts returns provider-specific LangChainGo options based on
// config. Extracted from buildLLMProviderOptions for reuse.
func buildLLMProviderOpts(cfg *kaconfig.Config) []langchaingo.Option {
	var opts []langchaingo.Option
	if cfg.AI.LLM.AzureAPIVersion != "" {
		opts = append(opts, langchaingo.WithAzureAPIVersion(cfg.AI.LLM.AzureAPIVersion))
	}
	if cfg.AI.LLM.VertexProject != "" {
		opts = append(opts, langchaingo.WithVertexProject(cfg.AI.LLM.VertexProject))
	}
	if cfg.AI.LLM.VertexLocation != "" {
		opts = append(opts, langchaingo.WithVertexLocation(cfg.AI.LLM.VertexLocation))
	}
	if cfg.AI.LLM.BedrockRegion != "" {
		opts = append(opts, langchaingo.WithBedrockRegion(cfg.AI.LLM.BedrockRegion))
	}

	if rt := buildTransportChainFromConfig(cfg); rt != nil {
		opts = append(opts, langchaingo.WithHTTPClient(&http.Client{Transport: rt}))
		opts = append(opts, langchaingo.WithCloser(func() error {
			if t, ok := rt.(interface{ CloseIdleConnections() }); ok {
				t.CloseIdleConnections()
			}
			return nil
		}))
	}
	return opts
}

// buildTransportChainFromConfig composes the HTTP transport stack. Extracted
// from buildTransportChain for reuse by the reload callback.
func buildTransportChainFromConfig(cfg *kaconfig.Config) http.RoundTripper {
	var base http.RoundTripper = http.DefaultTransport
	needsCustom := false

	if cfg.AI.LLM.OAuth2.Enabled {
		base = llmtransport.NewOAuth2ClientCredentialsTransport(cfg.AI.LLM.OAuth2, base)
		needsCustom = true
	}
	if len(cfg.AI.LLM.CustomHeaders) > 0 {
		base = llmtransport.NewAuthHeadersTransport(cfg.AI.LLM.CustomHeaders, base)
		needsCustom = true
	}
	if cfg.AI.LLM.StructuredOutput {
		base = llmtransport.NewStructuredOutputTransport(nil, base)
		needsCustom = true
	}
	if !needsCustom {
		return nil
	}
	return base
}

// configReloadCallback creates a hotreload.ReloadCallback for main config
// file changes. It re-parses the new config content, validates safety
// constraints, builds a new LLM client, and swaps it into the SwappableClient.
//
// Security invariants (per #783 adversarial review):
//   - Provider changes are rejected (requires pod restart)
//   - OAuth2 tokenURL changes are rejected (requires pod restart)
//   - structured_output changes are rejected (requires pod restart)
//   - Empty/whitespace content is rejected
//   - token_url must use https:// scheme
//   - Fresh config copy is used (live cfg never mutated on failure)
func configReloadCallback(
	configPath string,
	currentCfg func() *kaconfig.Config,
	swappable *llm.SwappableClient,
	logger *slog.Logger,
) func(newContent string) error {
	return func(newContent string) error {
		oldModel := swappable.ModelName()

		if strings.TrimSpace(newContent) == "" {
			logger.Warn("config_reload rejected: empty content",
				"event", "config_reload", "status", "rejected", "reason", "empty_content")
			return fmt.Errorf("config reload rejected: empty or whitespace-only content")
		}

		freshCfg, err := kaconfig.Load([]byte(newContent))
		if err != nil {
			logger.Error("config_reload failed: parse error",
				"event", "config_reload", "status", "error", "reason", "parse_failed", "error", err)
			return fmt.Errorf("reload: parsing config: %w", err)
		}

		cur := currentCfg()

		if err := validateReloadSafety(cur, freshCfg); err != nil {
			logger.Warn("config_reload rejected",
				"event", "config_reload", "status", "rejected", "reason", err.Error())
			return err
		}

		if err := freshCfg.Validate(); err != nil {
			logger.Warn("config_reload rejected: validation failed",
				"event", "config_reload", "status", "rejected", "reason", "validation_failed", "error", err)
			return fmt.Errorf("reload: validation failed: %w", err)
		}

		newClient, err := buildLLMClientFromConfig(context.Background(), freshCfg)
		if err != nil {
			logger.Error("config_reload failed: client build error",
				"event", "config_reload", "status", "error", "reason", "client_build_failed", "error", err)
			return fmt.Errorf("reload: building LLM client: %w", err)
		}

		if err := swappable.Swap(newClient, freshCfg.AI.LLM.Model); err != nil {
			logger.Error("config_reload failed: swap error",
				"event", "config_reload", "status", "error", "reason", "swap_failed", "error", err)
			return fmt.Errorf("reload: swapping client: %w", err)
		}

		logger.Info("config_reload success",
			"event", "config_reload",
			"status", "success",
			"old_model", oldModel,
			"new_model", freshCfg.AI.LLM.Model,
			"old_endpoint", cur.AI.LLM.Endpoint,
			"new_endpoint", freshCfg.AI.LLM.Endpoint,
		)
		return nil
	}
}

// validateReloadSafety checks that the new config does not violate hot-reload
// safety constraints. Safe to change at runtime: model, endpoint, apiKey,
// temperature, maxRetries, timeoutSeconds. Requires restart: provider,
// structuredOutput, oauth2.tokenURL.
func validateReloadSafety(current, proposed *kaconfig.Config) error {
	if current.AI.LLM.Provider != proposed.AI.LLM.Provider {
		return fmt.Errorf("config reload rejected: provider change from %q to %q requires pod restart",
			current.AI.LLM.Provider, proposed.AI.LLM.Provider)
	}

	if current.AI.LLM.StructuredOutput != proposed.AI.LLM.StructuredOutput {
		return fmt.Errorf("config reload rejected: structured_output change requires pod restart")
	}

	if err := validateOAuth2Safety(current.AI.LLM.OAuth2, proposed.AI.LLM.OAuth2); err != nil {
		return err
	}

	if proposed.AI.LLM.OAuth2.Enabled && proposed.AI.LLM.OAuth2.TokenURL != "" {
		if !strings.HasPrefix(strings.ToLower(proposed.AI.LLM.OAuth2.TokenURL), "https://") {
			return fmt.Errorf("config reload rejected: oauth2.tokenURL must use https:// scheme, got %q",
				proposed.AI.LLM.OAuth2.TokenURL)
		}
	}

	return nil
}

func validateOAuth2Safety(current, proposed kaconfig.OAuth2Config) error {
	if !current.Enabled && !proposed.Enabled {
		return nil
	}

	if current.TokenURL != proposed.TokenURL {
		return fmt.Errorf("config reload rejected: oauth2.tokenURL change requires pod restart")
	}
	return nil
}
