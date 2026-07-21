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
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/credentials"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	internaltransport "github.com/jordigilh/kubernaut/internal/kubernautagent/llm/transport"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/anthropicfamily"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
	llmtransport "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
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
		var vertexOpts []anthropicfamily.Option
		timeout := 120 * time.Second
		if cfg.TimeoutSeconds > 0 {
			timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
		}
		vertexOpts = append(vertexOpts, anthropicfamily.WithHTTPTimeout(timeout))
		vertexOpts = append(vertexOpts, anthropicReasoningOptions(cfg)...)

		chain, err := buildTransportChain(cfg) //nolint:contextcheck // LLM transport chain lazily builds an OAuth2 client-credentials token source shared across future requests
		if err != nil {
			return nil, fmt.Errorf("vertex_ai transport chain: %w", err)
		}
		if chain != nil {
			vertexOpts = append(vertexOpts, anthropicfamily.WithBaseTransport(chain))
		}

		return anthropicfamily.New(ctx,
			cfg.Model, []byte(cfg.APIKey),
			cfg.VertexProject, cfg.VertexLocation,
			vertexOpts...)
	case types.LLMProviderAnthropic:
		return buildAnthropicNativeClient(cfg) //nolint:contextcheck // LLM transport chain lazily builds an OAuth2 client-credentials token source shared across future requests
	case types.LLMProviderOpenAI, types.LLMProviderOpenAICompatible:
		return buildOpenAICompatClient(cfg) //nolint:contextcheck // LLM transport chain lazily builds an OAuth2 client-credentials token source shared across future requests
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %q", cfg.Provider)
	}
}

// buildAnthropicNativeClient constructs an anthropicfamily.Client for the
// native Anthropic API (api.anthropic.com), issue #1580. Distinct from the
// LLMProviderVertexAI case above: no GCP project/location is required or
// consulted.
func buildAnthropicNativeClient(cfg types.LLMConfig) (llm.Client, error) {
	var opts []anthropicfamily.Option
	timeout := 120 * time.Second
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	opts = append(opts, anthropicfamily.WithHTTPTimeout(timeout))
	opts = append(opts, anthropicReasoningOptions(cfg)...)

	chain, err := buildTransportChain(cfg)
	if err != nil {
		return nil, fmt.Errorf("anthropic transport chain: %w", err)
	}
	if chain != nil {
		opts = append(opts, anthropicfamily.WithBaseTransport(chain))
	}

	return anthropicfamily.NewWithAPIKey(cfg.APIKey, cfg.Model, opts...)
}

// anthropicReasoningOptions maps the operator's ai.llm.reasoning config into
// an anthropicfamily.WithReasoning construction option, resolved once here
// rather than threaded per-call from investigator business logic
// (DD-HAPI-019, llm.ReasoningRequest doc contract).
//
// #1578 wiring-gap fix: before this, cfg.Reasoning.Enabled/BudgetTokens were
// parsed and validated but never read anywhere in this file (only
// cfg.Reasoning.CapabilityOverride was, for the unrelated OpenAI-compatible
// replay-mode auto-detection below) — so ai.llm.reasoning.enabled: true had
// no effect on any real Anthropic/Vertex call. anthropicfamily.Client now
// applies this default whenever a per-call req.Options.Reasoning is nil.
func anthropicReasoningOptions(cfg types.LLMConfig) []anthropicfamily.Option {
	if cfg.Reasoning == nil || !cfg.Reasoning.Enabled {
		return nil
	}
	return []anthropicfamily.Option{anthropicfamily.WithReasoning(llm.ReasoningRequest{
		Enabled:      true,
		BudgetTokens: cfg.Reasoning.BudgetTokens,
		Effort:       cfg.Reasoning.Effort,
	})}
}

// buildOpenAICompatClient constructs the shared-core-backed kaopenai.Client
// for any OpenAI-Chat-Completions-compatible endpoint (OpenAI itself, Azure
// OpenAI, Ollama, vLLM, LlamaStack, Mistral, HuggingFace TGI, DeepSeek —
// issue #1581, DD-LLM-005).
func buildOpenAICompatClient(cfg types.LLMConfig) (llm.Client, error) {
	timeout := 120 * time.Second
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	httpClient := &http.Client{Timeout: timeout}

	chain, err := buildTransportChain(cfg)
	if err != nil {
		return nil, fmt.Errorf("openai transport chain: %w", err)
	}
	if chain != nil {
		httpClient.Transport = chain
	}

	opts := []kaopenai.Option{kaopenai.WithHTTPClient(httpClient)}
	// CapabilityOverride only controls DetectReasoningMode's replay-mode
	// auto-detection — reasoning-content CAPTURE for this family is
	// unconditional and model-driven (DeepSeek-reasoner-class models always
	// emit reasoning_content; see openaicompat's AC3 "always capture"
	// behavior), so Enabled/BudgetTokens are never read for that purpose.
	// BudgetTokens has no OpenAI-Chat-Completions equivalent (no
	// token-budget concept in "reasoning_effort") and stays Anthropic-only.
	// Effort DOES have a real wire equivalent here (#1604's
	// reasoning_effort request-side control) — see openaiReasoningOptions.
	if cfg.Reasoning != nil && cfg.Reasoning.CapabilityOverride != "" {
		opts = append(opts, kaopenai.WithCapabilityOverride(cfg.Reasoning.CapabilityOverride))
	}
	// AzureAPIVersion is the sole detection signal for Azure OpenAI (#1600)
	// — there is no separate "azure" provider enum value, matching the
	// pre-langchaingo-removal behavior this restores: Azure is layered on
	// top of provider: openai/openai_compatible, not a distinct provider.
	if cfg.AzureAPIVersion != "" {
		opts = append(opts, kaopenai.WithAzureAPIVersion(cfg.AzureAPIVersion))
	}
	opts = append(opts, openaiReasoningOptions(cfg)...)

	return kaopenai.New(cfg.Model, cfg.Endpoint, cfg.APIKey, opts...), nil
}

// openaiReasoningOptions maps the operator's ai.llm.reasoning.effort config
// into a kaopenai.WithReasoning construction option (#1604), resolved once
// here rather than threaded per-call from investigator business logic
// (DD-HAPI-019), mirroring anthropicReasoningOptions above. Unlike that
// function, BudgetTokens is not passed through: it has no
// OpenAI-Chat-Completions wire equivalent.
func openaiReasoningOptions(cfg types.LLMConfig) []kaopenai.Option {
	if cfg.Reasoning == nil || !cfg.Reasoning.Enabled {
		return nil
	}
	return []kaopenai.Option{kaopenai.WithReasoning(llm.ReasoningRequest{
		Enabled: true,
		Effort:  cfg.Reasoning.Effort,
	})}
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

	// nolint:nilnil // intentional "no custom transport" sentinel, not an
	// error — all 3 callers already guard with `if chain != nil` before use
	// (Issue #1546 Tier 2).
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
//
// #1599 / DD-LLM-008: LLM identity (provider+model, at both base and
// per-phase-override level) is immutable after process start — a
// cross-provider hot-swap risks replaying one provider's opaque reasoning
// signature (e.g. Anthropic's thinking-block Signature bytes) against a
// different provider/model, which that provider never issued. Any attempt
// to change identity is rejected in full (the whole candidate reload, base
// and phase tuning alike) and requires a restart; bootRuntime is the frozen
// LLM runtime snapshot captured once at boot and is the source of truth for
// "what identity is this process currently running", independent of
// anything a later reload attempted.
func llmRuntimeReloadCallback(
	staticCfg *kaconfig.Config,
	swappable *llm.SwappableClient,
	logger logr.Logger,
	phaseResolver *investigator.DefaultPhaseResolver,
	bootRuntime *kaconfig.LLMRuntimeConfig,
) func(newContent string) error {
	return func(newContent string) error {
		oldModel := swappable.ModelName()

		rt, err := parseAndAuthorizeReload(staticCfg, phaseResolver, bootRuntime, newContent, logger)
		if err != nil {
			return err
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

// parseAndAuthorizeReload parses, validates, and applies the #1599
// restart-required identity lock to a candidate llm-runtime reload payload.
// Returns the parsed *kaconfig.LLMRuntimeConfig on success (safe to merge and
// apply), or an error if the content is empty, malformed, fails field
// validation, or would change base or per-phase LLM identity. Extracted from
// llmRuntimeReloadCallback to keep that function's cognitive complexity
// within the AGENTS.md Go Anti-Pattern Checklist budget.
func parseAndAuthorizeReload(
	staticCfg *kaconfig.Config,
	phaseResolver *investigator.DefaultPhaseResolver,
	bootRuntime *kaconfig.LLMRuntimeConfig,
	newContent string,
	logger logr.Logger,
) (*kaconfig.LLMRuntimeConfig, error) {
	if bootRuntime == nil {
		return nil, fmt.Errorf("reload: internal error: boot runtime snapshot not configured")
	}

	if strings.TrimSpace(newContent) == "" {
		logger.Info("llm_runtime_reload rejected: empty content",
			"event", "llm_runtime_reload", "status", "rejected", "reason", "empty_content")
		return nil, fmt.Errorf("llm runtime reload rejected: empty or whitespace-only content")
	}

	rt, err := kaconfig.LoadLLMRuntime([]byte(newContent))
	if err != nil {
		return nil, fmt.Errorf("reload: parsing llm runtime config: %w", err)
	}

	if err := rt.Validate(staticCfg.AI.LLM.Provider); err != nil {
		logger.Info("llm_runtime_reload rejected: validation failed",
			"event", "llm_runtime_reload", "status", "rejected", "error", err)
		return nil, fmt.Errorf("reload: validation failed: %w", err)
	}

	if rt.Model != bootRuntime.Model {
		logger.Info("llm_runtime_reload rejected: model change requires restart",
			"event", "llm_runtime_reload", "status", "rejected", "reason", "identity_change",
			"boot_model", bootRuntime.Model, "requested_model", rt.Model)
		return nil, fmt.Errorf("reload: model change from %q to %q requires a process restart (see #1599)", bootRuntime.Model, rt.Model)
	}

	if phaseResolver != nil {
		if err := validatePhaseIdentity(staticCfg, bootRuntime, rt); err != nil {
			logger.Info("llm_runtime_reload rejected: phase identity change requires restart",
				"event", "llm_runtime_reload", "status", "rejected", "error", err)
			return nil, fmt.Errorf("reload: %w", err)
		}
	}

	return rt, nil
}

// validatePhaseIdentity checks that no phase's effective LLM identity
// (provider+model) differs between the frozen boot-time snapshot and the
// reload candidate. Per #1599, phase identity is subject to the same
// restart-required rule as base identity; hot-reload may only alter
// non-identity tuning fields (endpoint, temperature, timeouts, headers,
// custom auth) within a phase override, whether that override is new,
// existing, or being removed. Returns a single error naming every phase
// that would require a restart, or nil if the candidate is safe to apply.
func validatePhaseIdentity(staticCfg *kaconfig.Config, bootRuntime, rt *kaconfig.LLMRuntimeConfig) error {
	phases := make(map[string]struct{}, len(bootRuntime.PhaseModels)+len(rt.PhaseModels))
	for name := range bootRuntime.PhaseModels {
		phases[name] = struct{}{}
	}
	for name := range rt.PhaseModels {
		phases[name] = struct{}{}
	}

	violations := make([]string, 0, len(phases))
	for name := range phases {
		bootLLM, bootRT := bootRuntime.EffectivePhaseConfig(name, staticCfg.AI.LLM, *bootRuntime)
		newLLM, newRT := rt.EffectivePhaseConfig(name, staticCfg.AI.LLM, *rt)
		if bootLLM.Provider != newLLM.Provider || bootRT.Model != newRT.Model {
			violations = append(violations, fmt.Sprintf("%s: %s/%s -> %s/%s",
				name, bootLLM.Provider, bootRT.Model, newLLM.Provider, newRT.Model))
		}
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("phase identity change requires a process restart (see #1599): %s", strings.Join(violations, "; "))
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

// swapExistingPhaseClient hot-swaps an already-registered phase SwappableClient
// with the newly-built phaseClient/phaseRT, logging the model transition.
func swapExistingPhaseClient(existing *llm.SwappableClient, phaseName string, phaseClient llm.Client, phaseRT kaconfig.LLMRuntimeConfig, logger logr.Logger) {
	oldModel := existing.ModelName()
	if swapErr := existing.Swap(phaseClient, phaseRT.Model, llm.RuntimeParams{
		Temperature:    phaseRT.Temperature,
		TimeoutSeconds: phaseRT.TimeoutSeconds,
		MaxRetries:     phaseRT.MaxRetries,
	}); swapErr != nil {
		logger.Error(swapErr, "reload: failed to swap phase client",
			"phase", phaseName)
		return
	}
	logger.Info("llm_runtime_reload phase_model updated",
		"event", "llm_runtime_reload",
		"phase", phaseName,
		"old_model", oldModel,
		"new_model", phaseRT.Model,
	)
}

// registerNewPhaseClient creates and registers a SwappableClient for a phase
// that does not yet have one on the resolver.
func registerNewPhaseClient(resolver *investigator.DefaultPhaseResolver, phase katypes.Phase, phaseName string, phaseClient llm.Client, phaseRT kaconfig.LLMRuntimeConfig, logger logr.Logger) {
	newSw, swErr := llm.NewSwappableClient(phaseClient, phaseRT.Model, llm.RuntimeParams{
		Temperature:    phaseRT.Temperature,
		TimeoutSeconds: phaseRT.TimeoutSeconds,
		MaxRetries:     phaseRT.MaxRetries,
	})
	if swErr != nil {
		logger.Error(swErr, "reload: failed to create phase SwappableClient",
			"phase", phaseName)
		return
	}
	resolver.SetPhaseSwappable(phase, newSw)
	logger.Info("llm_runtime_reload phase_model added",
		"event", "llm_runtime_reload",
		"phase", phaseName,
		"model", phaseRT.Model,
	)
}

// reloadSinglePhaseClient rebuilds the LLM client for one phase override and
// either hot-swaps the existing phase SwappableClient or registers a new one.
func reloadSinglePhaseClient(
	staticCfg *kaconfig.Config,
	rt *kaconfig.LLMRuntimeConfig,
	resolver *investigator.DefaultPhaseResolver,
	phaseName string,
	override *kaconfig.LLMOverrideConfig,
	logger logr.Logger,
) {
	phase := katypes.Phase(phaseName)
	phaseLLM, phaseRT := rt.EffectivePhaseConfig(phaseName, staticCfg.AI.LLM, *rt)
	merged := mergeLLMConfig(phaseLLM, &phaseRT)

	phaseClient, err := buildLLMClientFromConfig(context.Background(), merged)
	if err != nil {
		logger.Error(err, "reload: failed to build phase LLM client",
			"phase", phaseName, "model", override.Model)
		return
	}

	if existing := resolver.PhaseSwappable(phase); existing != nil {
		swapExistingPhaseClient(existing, phaseName, phaseClient, phaseRT, logger)
	} else {
		registerNewPhaseClient(resolver, phase, phaseName, phaseClient, phaseRT, logger)
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
		configuredPhases[katypes.Phase(phaseName)] = true
		reloadSinglePhaseClient(staticCfg, rt, resolver, phaseName, override, logger)
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
