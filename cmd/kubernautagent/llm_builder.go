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
	"os"
	"strings"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	llmtransport "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/vertexanthropic"
)

// buildLLMClientFromConfig constructs an llm.Client from the given config.
// This is a pure function used both at startup and by the SDK hot-reload
// callback. It does not mutate cfg.
func buildLLMClientFromConfig(ctx context.Context, cfg *kaconfig.Config) (llm.Client, error) {
	switch cfg.LLM.Provider {
	case "vertex_ai":
		return vertexanthropic.New(ctx,
			cfg.LLM.Model, []byte(cfg.LLM.APIKey),
			cfg.LLM.VertexProject, cfg.LLM.VertexLocation)
	default:
		return langchaingo.New(cfg.LLM.Provider, cfg.LLM.Endpoint, cfg.LLM.Model, cfg.LLM.APIKey,
			buildLLMProviderOpts(cfg)...)
	}
}

// buildLLMProviderOpts returns provider-specific LangChainGo options based on
// config. Extracted from buildLLMProviderOptions for reuse.
func buildLLMProviderOpts(cfg *kaconfig.Config) []langchaingo.Option {
	var opts []langchaingo.Option
	if cfg.LLM.AzureAPIVersion != "" {
		opts = append(opts, langchaingo.WithAzureAPIVersion(cfg.LLM.AzureAPIVersion))
	}
	if cfg.LLM.VertexProject != "" {
		opts = append(opts, langchaingo.WithVertexProject(cfg.LLM.VertexProject))
	}
	if cfg.LLM.VertexLocation != "" {
		opts = append(opts, langchaingo.WithVertexLocation(cfg.LLM.VertexLocation))
	}
	if cfg.LLM.BedrockRegion != "" {
		opts = append(opts, langchaingo.WithBedrockRegion(cfg.LLM.BedrockRegion))
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

	if cfg.LLM.OAuth2.Enabled {
		base = llmtransport.NewOAuth2ClientCredentialsTransport(cfg.LLM.OAuth2, base)
		needsCustom = true
	}
	if len(cfg.LLM.CustomHeaders) > 0 {
		base = llmtransport.NewAuthHeadersTransport(cfg.LLM.CustomHeaders, base)
		needsCustom = true
	}
	if cfg.LLM.StructuredOutput {
		base = llmtransport.NewStructuredOutputTransport(nil, base)
		needsCustom = true
	}
	if !needsCustom {
		return nil
	}
	return base
}

// sdkReloadCallback creates a hotreload.ReloadCallback for SDK config changes.
// It re-reads the main config from disk, merges the new SDK content, validates,
// builds a new client, and swaps it into the SwappableClient.
//
// Security invariants (per #783 adversarial review):
//   - Provider changes are rejected
//   - OAuth2 credential changes (token_url, client_id, client_secret) are rejected
//   - Empty/whitespace SDK content is rejected
//   - token_url must use https:// scheme
//   - structured_output changes are rejected
//   - Fresh config copy is used (live cfg never mutated on failure)
func sdkReloadCallback(
	mainConfigPath string,
	currentCfg func() *kaconfig.Config,
	swappable *llm.SwappableClient,
	logger *slog.Logger,
) func(newContent string) error {
	return func(newContent string) error {
		oldModel := swappable.ModelName()

		if strings.TrimSpace(newContent) == "" {
			logger.Warn("sdk_config_reload rejected: empty content",
				"event", "sdk_config_reload", "status", "rejected", "reason", "empty_content")
			return fmt.Errorf("SDK config reload rejected: empty or whitespace-only content")
		}

		freshCfg, err := loadFreshConfig(mainConfigPath)
		if err != nil {
			logger.Error("sdk_config_reload failed: cannot re-read main config",
				"event", "sdk_config_reload", "status", "error", "reason", "main_config_read", "error", err)
			return fmt.Errorf("reload: re-reading main config: %w", err)
		}

		if err := freshCfg.MergeSDKConfig([]byte(newContent)); err != nil {
			logger.Error("sdk_config_reload failed: merge error",
				"event", "sdk_config_reload", "status", "error", "reason", "merge_failed", "error", err)
			return fmt.Errorf("reload: merging SDK config: %w", err)
		}

		cur := currentCfg()

		if err := validateReloadSafety(cur, freshCfg); err != nil {
			logger.Warn("sdk_config_reload rejected",
				"event", "sdk_config_reload", "status", "rejected", "reason", err.Error())
			return err
		}

		if err := freshCfg.Validate(); err != nil {
			logger.Warn("sdk_config_reload rejected: validation failed",
				"event", "sdk_config_reload", "status", "rejected", "reason", "validation_failed", "error", err)
			return fmt.Errorf("reload: validation failed: %w", err)
		}

		newClient, err := buildLLMClientFromConfig(context.Background(), freshCfg)
		if err != nil {
			logger.Error("sdk_config_reload failed: client build error",
				"event", "sdk_config_reload", "status", "error", "reason", "client_build_failed", "error", err)
			return fmt.Errorf("reload: building LLM client: %w", err)
		}

		if err := swappable.Swap(newClient, freshCfg.LLM.Model); err != nil {
			logger.Error("sdk_config_reload failed: swap error",
				"event", "sdk_config_reload", "status", "error", "reason", "swap_failed", "error", err)
			return fmt.Errorf("reload: swapping client: %w", err)
		}

		logger.Info("sdk_config_reload success",
			"event", "sdk_config_reload",
			"status", "success",
			"old_model", oldModel,
			"new_model", freshCfg.LLM.Model,
			"old_endpoint", cur.LLM.Endpoint,
			"new_endpoint", freshCfg.LLM.Endpoint,
		)
		return nil
	}
}

// loadFreshConfig re-reads the main config file from disk and returns a fresh
// Config struct. This ensures the reload callback never mutates the live config.
func loadFreshConfig(path string) (*kaconfig.Config, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	return kaconfig.Load(data)
}

// readFile is a package-level variable for test injection.
var readFile = os.ReadFile

// validateReloadSafety checks that the new config does not violate hot-reload
// safety constraints.
func validateReloadSafety(current, proposed *kaconfig.Config) error {
	if current.LLM.Provider != proposed.LLM.Provider {
		return fmt.Errorf("SDK config reload rejected: provider change from %q to %q requires pod restart",
			current.LLM.Provider, proposed.LLM.Provider)
	}

	if current.LLM.StructuredOutput != proposed.LLM.StructuredOutput {
		return fmt.Errorf("SDK config reload rejected: structured_output change requires pod restart")
	}

	if err := validateOAuth2Safety(current.LLM.OAuth2, proposed.LLM.OAuth2); err != nil {
		return err
	}

	if proposed.LLM.OAuth2.Enabled && proposed.LLM.OAuth2.TokenURL != "" {
		if !strings.HasPrefix(strings.ToLower(proposed.LLM.OAuth2.TokenURL), "https://") {
			return fmt.Errorf("SDK config reload rejected: oauth2.token_url must use https:// scheme, got %q",
				proposed.LLM.OAuth2.TokenURL)
		}
	}

	return nil
}

func validateOAuth2Safety(current, proposed kaconfig.OAuth2Config) error {
	if !current.Enabled && !proposed.Enabled {
		return nil
	}

	if current.TokenURL != proposed.TokenURL {
		return fmt.Errorf("SDK config reload rejected: oauth2.token_url change requires pod restart")
	}
	if current.ClientID != proposed.ClientID {
		return fmt.Errorf("SDK config reload rejected: oauth2.client_id change requires pod restart")
	}
	if current.ClientSecret != proposed.ClientSecret {
		return fmt.Errorf("SDK config reload rejected: oauth2.client_secret change requires pod restart")
	}
	return nil
}
