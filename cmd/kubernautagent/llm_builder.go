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
	"time"

	"github.com/go-logr/logr"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/credentials"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	internaltransport "github.com/jordigilh/kubernaut/internal/kubernautagent/llm/transport"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	llmtransport "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/vertexanthropic"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// buildLLMClientFromConfig constructs an llm.Client from a merged LLMConfig
// that contains both static and runtime fields. This is a pure function used
// both at startup and by the hot-reload callback.
func buildLLMClientFromConfig(ctx context.Context, cfg types.LLMConfig) (llm.Client, error) {
	switch cfg.Provider {
	case types.LLMProviderVertexAI:
		var vertexOpts []vertexanthropic.Option
		timeout := 120 * time.Second
		if cfg.TimeoutSeconds > 0 {
			timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
		}
		vertexOpts = append(vertexOpts, vertexanthropic.WithHTTPTimeout(timeout))

		chain, err := buildTransportChain(cfg)
		if err != nil {
			return nil, fmt.Errorf("vertex_ai transport chain: %w", err)
		}
		if chain != nil {
			vertexOpts = append(vertexOpts, vertexanthropic.WithBaseTransport(chain))
		}

		return vertexanthropic.New(ctx,
			cfg.Model, []byte(cfg.APIKey),
			cfg.VertexProject, cfg.VertexLocation,
			vertexOpts...)
	default:
		providerOpts, err := buildLLMProviderOpts(cfg)
		if err != nil {
			return nil, err
		}
		return langchaingo.New(cfg.Provider, cfg.Endpoint, cfg.Model, cfg.APIKey,
			providerOpts...)
	}
}

// buildLLMProviderOpts returns provider-specific LangChainGo options.
func buildLLMProviderOpts(cfg types.LLMConfig) ([]langchaingo.Option, error) {
	var opts []langchaingo.Option
	if cfg.AzureAPIVersion != "" {
		opts = append(opts, langchaingo.WithAzureAPIVersion(cfg.AzureAPIVersion))
	}
	if cfg.VertexProject != "" {
		opts = append(opts, langchaingo.WithVertexProject(cfg.VertexProject))
	}
	if cfg.VertexLocation != "" {
		opts = append(opts, langchaingo.WithVertexLocation(cfg.VertexLocation))
	}
	if cfg.BedrockRegion != "" {
		opts = append(opts, langchaingo.WithBedrockRegion(cfg.BedrockRegion))
	}

	const defaultLLMClientTimeout = 120 * time.Second

	timeout := defaultLLMClientTimeout
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}

	customTransport, err := buildTransportChain(cfg)
	if err != nil {
		return nil, fmt.Errorf("llm transport chain: %w", err)
	}
	httpClient := &http.Client{Timeout: timeout}
	if customTransport != nil {
		httpClient.Transport = customTransport
	}
	opts = append(opts, langchaingo.WithHTTPClient(httpClient))
	if customTransport != nil {
		opts = append(opts, langchaingo.WithCloser(func() error {
			if t, ok := customTransport.(interface{ CloseIdleConnections() }); ok {
				t.CloseIdleConnections()
			}
			return nil
		}))
	}
	return opts, nil
}

// buildTransportChain composes the HTTP transport stack from the merged config,
// optionally wrapped with a circuit breaker (OPS-2).
//
// Chain order (outermost first): CircuitBreaker -> Auth/Headers -> OAuth2 -> TLS/base
//
// Issue #902: When tlsCaFile is set, uses sharedtls.NewTLSTransport as the
// base instead of http.DefaultTransport.
func buildTransportChain(cfg types.LLMConfig) (http.RoundTripper, error) {
	var base = http.DefaultTransport
	needsCustom := false

	if cfg.TLSCaFile != "" {
		var tlsOpts []sharedtls.TLSTransportOption
		if cfg.TLSCertFile != "" {
			tlsOpts = append(tlsOpts, sharedtls.WithClientCert(cfg.TLSCertFile, cfg.TLSKeyFile))
		}
		tlsTransport, err := sharedtls.NewTLSTransport(cfg.TLSCaFile, tlsOpts...)
		if err != nil {
			return nil, fmt.Errorf("llm TLS transport: %w", err)
		}
		base = tlsTransport
		needsCustom = true
	}

	if cfg.OAuth2.Enabled {
		base = internaltransport.NewOAuth2ClientCredentialsTransport(cfg.OAuth2, base)
		needsCustom = true
	}
	if len(cfg.CustomHeaders) > 0 {
		base = llmtransport.NewAuthHeadersTransport(cfg.CustomHeaders, base)
		needsCustom = true
	}

	if cfg.CircuitBreaker.Enabled {
		cb := cfg.CircuitBreaker
		base = transport.NewCircuitBreakerTransport(base, transport.CircuitBreakerConfig{
			Enabled:          true,
			Name:             "llm",
			MaxRequests:      cb.MaxRequests,
			Interval:         cb.Interval,
			Timeout:          cb.Timeout,
			FailureThreshold: cb.FailureThreshold,
			FailureRatio:     cb.FailureRatio,
		})
		needsCustom = true
	}

	if !needsCustom {
		return nil, nil
	}
	return base, nil
}

// mergeLLMConfig merges runtime (hot-reloadable) fields from an LLMRuntimeConfig
// into a base types.LLMConfig. Static fields (provider, TLS, OAuth2, CB) come from
// the base; runtime fields (model, endpoint, apiKeyFile, temperature, etc.) come from rt.
func mergeLLMConfig(base types.LLMConfig, rt *kaconfig.LLMRuntimeConfig) types.LLMConfig {
	merged := base
	merged.Model = rt.Model
	merged.Endpoint = rt.Endpoint
	if rt.APIKeyFile != "" {
		merged.APIKeyFile = rt.APIKeyFile
	}
	if rt.APIKey != "" {
		merged.APIKey = rt.APIKey
	}
	merged.TimeoutSeconds = rt.TimeoutSeconds
	merged.CustomHeaders = rt.CustomHeaders
	if rt.Temperature != 0 {
		t := rt.Temperature
		merged.Temperature = &t
	}
	if rt.MaxRetries != 0 {
		r := rt.MaxRetries
		merged.MaxRetries = &r
	}
	return merged
}

// llmRuntimeReloadCallback creates a hotreload.ReloadCallback for LLM runtime
// config file changes. It parses the new content, validates it, merges with
// static config, builds a new LLM client, and swaps it into the SwappableClient.
func llmRuntimeReloadCallback(
	staticCfg *kaconfig.Config,
	swappable *llm.SwappableClient,
	logger logr.Logger,
	phaseResolver *investigator.DefaultPhaseResolver,
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
			return fmt.Errorf("reload: parsing llm runtime config: %w", err)
		}

		if err := rt.Validate(staticCfg.AI.LLM.Provider); err != nil {
			logger.Info("llm_runtime_reload rejected: validation failed",
				"event", "llm_runtime_reload", "status", "rejected", "error", err)
			return fmt.Errorf("reload: validation failed: %w", err)
		}

		merged := mergeLLMConfig(staticCfg.AI.LLM, rt)
		resolveAPIKeyForReload(&merged, staticCfg.AI.LLM.Provider, logger)

		newClient, err := buildLLMClientFromConfig(context.Background(), merged)
		if err != nil {
			return fmt.Errorf("reload: building LLM client: %w", err)
		}

		if err := swappable.Swap(newClient, rt.Model, llm.RuntimeParams{
			Temperature:    rt.Temperature,
			TimeoutSeconds: rt.TimeoutSeconds,
			MaxRetries:     rt.MaxRetries,
		}); err != nil {
			return fmt.Errorf("reload: swapping client: %w", err)
		}

		logger.Info("llm_runtime_reload success",
			"event", "llm_runtime_reload",
			"status", "success",
			"old_model", oldModel,
			"new_model", rt.Model,
			"new_endpoint", rt.Endpoint,
		)

		if phaseResolver != nil {
			reloadPhaseClients(staticCfg, rt, phaseResolver, logger)
		}
		return nil
	}
}

// resolveAPIKeyForReload fills in merged.APIKey when the runtime reload
// payload did not carry one directly, falling back first to apiKeyFile and
// then to the Helm-mounted credentials directory. Mutates merged in place.
func resolveAPIKeyForReload(merged *types.LLMConfig, provider string, logger logr.Logger) {
	if merged.APIKey != "" {
		return
	}
	if merged.APIKeyFile != "" {
		if err := merged.ResolveAPIKey(); err != nil {
			logger.Info("llm_runtime_reload: apiKeyFile resolution failed, falling back to credentials dir",
				"error", err, "apiKeyFile", merged.APIKeyFile)
		}
	}
	if merged.APIKey == "" {
		const credDir = "/etc/kubernaut-agent/credentials" // pre-commit:allow-sensitive (mount path)
		merged.APIKey = credentials.ResolveCredentialsFile(provider, credDir, logger)
	}
}

// reloadPhaseClients synchronizes the phase resolver with the new config.
// It adds/updates phase-specific SwappableClients for each phase in PhaseModels
// and removes phases that are no longer configured.
func reloadPhaseClients(
	staticCfg *kaconfig.Config,
	rt *kaconfig.LLMRuntimeConfig,
	resolver *investigator.DefaultPhaseResolver,
	logger logr.Logger,
) {
	configuredPhases := make(map[katypes.Phase]bool)

	for phaseName, override := range rt.PhaseModels {
		phase := katypes.Phase(phaseName)
		configuredPhases[phase] = true

		phaseLLM, phaseRT := rt.EffectivePhaseConfig(phaseName, staticCfg.AI.LLM, *rt)
		merged := mergeLLMConfig(phaseLLM, &phaseRT)

		phaseClient, err := buildLLMClientFromConfig(context.Background(), merged)
		if err != nil {
			logger.Error(err, "reload: failed to build phase LLM client",
				"phase", phaseName, "model", override.Model)
			continue
		}

		existing := resolver.PhaseSwappable(phase)
		if existing != nil {
			oldModel := existing.ModelName()
			if swapErr := existing.Swap(phaseClient, phaseRT.Model, llm.RuntimeParams{
				Temperature:    phaseRT.Temperature,
				TimeoutSeconds: phaseRT.TimeoutSeconds,
				MaxRetries:     phaseRT.MaxRetries,
			}); swapErr != nil {
				logger.Error(swapErr, "reload: failed to swap phase client",
					"phase", phaseName)
				continue
			}
			logger.Info("llm_runtime_reload phase_model updated",
				"event", "llm_runtime_reload",
				"phase", phaseName,
				"old_model", oldModel,
				"new_model", phaseRT.Model,
			)
		} else {
			newSw, swErr := llm.NewSwappableClient(phaseClient, phaseRT.Model, llm.RuntimeParams{
				Temperature:    phaseRT.Temperature,
				TimeoutSeconds: phaseRT.TimeoutSeconds,
				MaxRetries:     phaseRT.MaxRetries,
			})
			if swErr != nil {
				logger.Error(swErr, "reload: failed to create phase SwappableClient",
					"phase", phaseName)
				continue
			}
			resolver.SetPhaseSwappable(phase, newSw)
			logger.Info("llm_runtime_reload phase_model added",
				"event", "llm_runtime_reload",
				"phase", phaseName,
				"model", phaseRT.Model,
			)
		}
	}

	for _, existingPhase := range resolver.Phases() {
		if !configuredPhases[existingPhase] {
			resolver.RemovePhaseSwappable(existingPhase)
			logger.Info("llm_runtime_reload phase_model removed",
				"event", "llm_runtime_reload",
				"phase", string(existingPhase),
			)
		}
	}
}
