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
	"net/http"
	"strings"

	"github.com/go-logr/logr"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	llmtransport "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/vertexanthropic"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// buildLLMClientFromConfig constructs an llm.Client from the static config and
// runtime config. This is a pure function used both at startup and by the
// hot-reload callback. It does not mutate either config.
func buildLLMClientFromConfig(ctx context.Context, cfg *kaconfig.Config, rt *kaconfig.LLMRuntimeConfig) (llm.Client, error) {
	switch cfg.AI.LLM.Provider {
	case "vertex_ai":
		return vertexanthropic.New(ctx,
			rt.Model, []byte(rt.APIKey),
			cfg.AI.LLM.VertexProject, cfg.AI.LLM.VertexLocation)
	default:
		return langchaingo.New(cfg.AI.LLM.Provider, rt.Endpoint, rt.Model, rt.APIKey,
			buildLLMProviderOpts(cfg, rt)...)
	}
}

// buildLLMProviderOpts returns provider-specific LangChainGo options.
func buildLLMProviderOpts(cfg *kaconfig.Config, rt *kaconfig.LLMRuntimeConfig) []langchaingo.Option {
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

	if transport := buildTransportChain(cfg, rt); transport != nil {
		opts = append(opts, langchaingo.WithHTTPClient(&http.Client{Transport: transport}))
		opts = append(opts, langchaingo.WithCloser(func() error {
			if t, ok := transport.(interface{ CloseIdleConnections() }); ok {
				t.CloseIdleConnections()
			}
			return nil
		}))
	}
	return opts
}

// buildTransportChain composes the HTTP transport stack from static (TLS CA,
// OAuth2) and runtime (CustomHeaders) config layers.
// Issue #902: When tlsCaFile is set, uses sharedtls.NewTLSTransport as the
// base instead of http.DefaultTransport.
func buildTransportChain(cfg *kaconfig.Config, rt *kaconfig.LLMRuntimeConfig) http.RoundTripper {
	var base http.RoundTripper = http.DefaultTransport
	needsCustom := false

	if cfg.AI.LLM.TLSCaFile != "" {
		tlsTransport, err := sharedtls.NewTLSTransport(cfg.AI.LLM.TLSCaFile)
		if err != nil {
			return nil
		}
		base = tlsTransport
		needsCustom = true
	}

	if cfg.AI.LLM.OAuth2.Enabled {
		base = llmtransport.NewOAuth2ClientCredentialsTransport(cfg.AI.LLM.OAuth2, base)
		needsCustom = true
	}
	if len(rt.CustomHeaders) > 0 {
		base = llmtransport.NewAuthHeadersTransport(rt.CustomHeaders, base)
		needsCustom = true
	}
	if !needsCustom {
		return nil
	}
	return base
}

// llmRuntimeReloadCallback creates a hotreload.ReloadCallback for LLM runtime
// config file changes. It parses the new content, validates it, builds a new
// LLM client, and swaps it into the SwappableClient.
//
// Since the runtime file only contains hot-reloadable fields (model, endpoint,
// apiKey, temperature, maxRetries, timeoutSeconds, customHeaders), no safety
// checks are needed — all fields are safe to change at runtime.
func llmRuntimeReloadCallback(
	staticCfg *kaconfig.Config,
	swappable *llm.SwappableClient,
	logger logr.Logger,
) func(newContent string) error {
	return func(newContent string) error {
		oldModel := swappable.ModelName()

		if strings.TrimSpace(newContent) == "" {
			logger.Info("llm_runtime_reload rejected: empty content",
				"event", "llm_runtime_reload", "status", "rejected", "reason", "empty_content")
			return fmt.Errorf("llm runtime reload rejected: empty or whitespace-only content")
		}

		rt, err := kaconfig.LoadLLMRuntime([]byte(newContent))
		if err != nil {
			logger.Error(err, "llm_runtime_reload failed: parse error",
				"event", "llm_runtime_reload", "status", "error")
			return fmt.Errorf("reload: parsing llm runtime config: %w", err)
		}

		if err := rt.Validate(staticCfg.AI.LLM.Provider); err != nil {
			logger.Info("llm_runtime_reload rejected: validation failed",
				"event", "llm_runtime_reload", "status", "rejected", "error", err)
			return fmt.Errorf("reload: validation failed: %w", err)
		}

		newClient, err := buildLLMClientFromConfig(context.Background(), staticCfg, rt)
		if err != nil {
			logger.Error(err, "llm_runtime_reload failed: client build error",
				"event", "llm_runtime_reload", "status", "error")
			return fmt.Errorf("reload: building LLM client: %w", err)
		}

		if err := swappable.Swap(newClient, rt.Model); err != nil {
			logger.Error(err, "llm_runtime_reload failed: swap error",
				"event", "llm_runtime_reload", "status", "error")
			return fmt.Errorf("reload: swapping client: %w", err)
		}

		logger.Info("llm_runtime_reload success",
			"event", "llm_runtime_reload",
			"status", "success",
			"old_model", oldModel,
			"new_model", rt.Model,
			"new_endpoint", rt.Endpoint,
		)
		return nil
	}
}
