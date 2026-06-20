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
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	internaltransport "github.com/jordigilh/kubernaut/internal/kubernautagent/llm/transport"
	llmtransport "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/vertexanthropic"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

// buildLLMClientFromConfig constructs an llm.Client from the static config and
// runtime config. This is a pure function used both at startup and by the
// hot-reload callback. It does not mutate either config.
func buildLLMClientFromConfig(ctx context.Context, cfg *kaconfig.Config, rt *kaconfig.LLMRuntimeConfig) (llm.Client, error) {
	switch cfg.AI.LLM.Provider {
	case "vertex_ai":
		var vertexOpts []vertexanthropic.Option
		timeout := 120 * time.Second
		if rt.TimeoutSeconds > 0 {
			timeout = time.Duration(rt.TimeoutSeconds) * time.Second
		}
		vertexOpts = append(vertexOpts, vertexanthropic.WithHTTPTimeout(timeout))

		chain, err := buildTransportChain(cfg, rt)
		if err != nil {
			return nil, fmt.Errorf("vertex_ai transport chain: %w", err)
		}
		if chain != nil {
			vertexOpts = append(vertexOpts, vertexanthropic.WithBaseTransport(chain))
		}

		return vertexanthropic.New(ctx,
			rt.Model, []byte(rt.APIKey),
			cfg.AI.LLM.VertexProject, cfg.AI.LLM.VertexLocation,
			vertexOpts...)
	default:
		providerOpts, err := buildLLMProviderOpts(cfg, rt)
		if err != nil {
			return nil, err
		}
		return langchaingo.New(cfg.AI.LLM.Provider, rt.Endpoint, rt.Model, rt.APIKey,
			providerOpts...)
	}
}

// buildLLMProviderOpts returns provider-specific LangChainGo options.
func buildLLMProviderOpts(cfg *kaconfig.Config, rt *kaconfig.LLMRuntimeConfig) ([]langchaingo.Option, error) {
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

	const defaultLLMClientTimeout = 120 * time.Second

	timeout := defaultLLMClientTimeout
	if rt.TimeoutSeconds > 0 {
		timeout = time.Duration(rt.TimeoutSeconds) * time.Second
	}

	customTransport, err := buildTransportChain(cfg, rt)
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

// buildTransportChain composes the HTTP transport stack from static (TLS CA,
// OAuth2) and runtime (CustomHeaders) config layers, optionally wrapped with
// a circuit breaker (OPS-2).
//
// Chain order (outermost first): CircuitBreaker -> Auth/Headers -> OAuth2 -> TLS/base
//
// Issue #902: When tlsCaFile is set, uses sharedtls.NewTLSTransport as the
// base instead of http.DefaultTransport.
func buildTransportChain(cfg *kaconfig.Config, rt *kaconfig.LLMRuntimeConfig) (http.RoundTripper, error) {
	var base = http.DefaultTransport
	needsCustom := false

	if cfg.AI.LLM.TLSCaFile != "" {
		var tlsOpts []sharedtls.TLSTransportOption
		if cfg.AI.LLM.TLSCertFile != "" {
			tlsOpts = append(tlsOpts, sharedtls.WithClientCert(cfg.AI.LLM.TLSCertFile, cfg.AI.LLM.TLSKeyFile))
		}
		tlsTransport, err := sharedtls.NewTLSTransport(cfg.AI.LLM.TLSCaFile, tlsOpts...)
		if err != nil {
			return nil, fmt.Errorf("llm TLS transport: %w", err)
		}
		base = tlsTransport
		needsCustom = true
	}

	if cfg.AI.LLM.OAuth2.Enabled {
		base = internaltransport.NewOAuth2ClientCredentialsTransport(cfg.AI.LLM.OAuth2, base)
		needsCustom = true
	}
	if len(rt.CustomHeaders) > 0 {
		base = llmtransport.NewAuthHeadersTransport(rt.CustomHeaders, base)
		needsCustom = true
	}

	if cfg.AI.LLM.CircuitBreaker.Enabled {
		cb := cfg.AI.LLM.CircuitBreaker
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

		if rt.APIKey == "" {
			const credDir = "/etc/kubernaut-agent/credentials" // pre-commit:allow-sensitive (mount path)
			rt.APIKey = credentials.ResolveCredentialsFile(staticCfg.AI.LLM.Provider, credDir, logger)
		}

		newClient, err := buildLLMClientFromConfig(context.Background(), staticCfg, rt)
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

		phaseCfg := *staticCfg
		phaseCfg.AI.LLM = phaseLLM

		phaseClient, err := buildLLMClientFromConfig(context.Background(), &phaseCfg, &phaseRT)
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
