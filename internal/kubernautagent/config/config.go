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

package config

import (
	"fmt"
	"path/filepath"
	"time"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	pkgconfig "github.com/jordigilh/kubernaut/pkg/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"gopkg.in/yaml.v3"
)

// Load parses configuration from YAML bytes and applies defaults.
func Load(data []byte) (*Config, error) {
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

// LoadLLMRuntime parses LLM runtime configuration from YAML bytes.
func LoadLLMRuntime(data []byte) (*LLMRuntimeConfig, error) {
	var rt LLMRuntimeConfig
	if err := yaml.Unmarshal(data, &rt); err != nil {
		return nil, fmt.Errorf("parsing llm runtime config: %w", err)
	}
	return &rt, nil
}

// Validate checks that the LLM runtime config has the minimum required fields.
// providersWithoutEndpointRequirement lists providers that resolve their
// endpoint implicitly (via SDK defaults or region config), so an explicit
// runtime.endpoint is not required for them.
//
// #2258: "bedrock", "huggingface", and "openai" were removed from this map.
// bedrock/huggingface are not supported providers at all (rejected
// downstream regardless of endpoint -- dead entries left over from the
// pre-#1598 langchaingo dispatch removal). openai IS a supported provider,
// but the openaicompat client has no default base URL -- an empty endpoint
// previously passed validation here and only failed at request time, not
// startup; this now fails closed at startup like every other non-exempt
// provider. "vertex" is intentionally retained: though rejected downstream
// like bedrock/huggingface, it is still documented and tested as an
// endpoint-optional legacy provider string (configuration-reference.md
// §5.1, UT-KA-684-103b).
var providersWithoutEndpointRequirement = map[string]bool{
	"anthropic": true, "vertex": true, "vertex_ai": true,
}

func (r *LLMRuntimeConfig) Validate(provider string) error {
	if r.Model == "" {
		return fmt.Errorf("model is required")
	}
	if !providersWithoutEndpointRequirement[provider] && r.Endpoint == "" {
		return fmt.Errorf("endpoint is required for provider %q", provider)
	}
	if len(r.CustomHeaders) > 0 {
		if err := pkgconfig.ValidateHeaderSources(r.CustomHeaders); err != nil {
			return fmt.Errorf("customHeaders: %w", err)
		}
	}
	for phase, override := range r.PhaseModels {
		if !ValidPhaseNames[phase] {
			return fmt.Errorf("phaseModels: unknown phase %q", phase)
		}
		if isEmptyPhaseOverride(override) {
			return fmt.Errorf("phaseModels[%q]: at least one override field must be set", phase)
		}
		if override.Reasoning != nil {
			effectiveProvider := override.Provider
			if effectiveProvider == "" {
				effectiveProvider = provider
			}
			if err := types.ValidateReasoningConfig(fmt.Sprintf("phaseModels[%q]", phase), override.Reasoning, effectiveProvider); err != nil {
				return err
			}
		}
	}
	return nil
}

// isEmptyPhaseOverride reports whether a phase-model override sets none of
// its fields (or is nil), which is rejected as a no-op configuration error.
// Reasoning counts as a real (non-empty) field per #1616/BR-AI-086: a
// phase override that tunes only reasoning is a legitimate, non-trivial
// configuration, not an accidental no-op.
func isEmptyPhaseOverride(override *LLMOverrideConfig) bool {
	return override == nil || (override.Provider == "" && override.Endpoint == "" &&
		override.Model == "" && override.APIKeyFile == "" &&
		override.AzureAPIVersion == "" && override.VertexProject == "" &&
		override.VertexLocation == "" &&
		override.Reasoning == nil && override.Temperature == nil)
}

// Validate checks required fields and value constraints for the static config.
// Runtime LLM fields are validated separately via LLMRuntimeConfig.Validate().
//
// Decomposed into per-section validators (GO-ANTIPATTERN-AUDIT-2026-07-01
// Phase 4b) purely for readability/complexity — behavior, order, and error
// messages are unchanged from the pre-decomposition implementation.
func (c *Config) Validate() error {
	if err := c.validateRuntime(); err != nil {
		return err
	}
	if err := c.validateAI(); err != nil {
		return err
	}
	if err := c.validateLLMOAuth2AndTLS(); err != nil {
		return err
	}
	if err := c.validateFleetIntegration(); err != nil {
		return err
	}
	if c.Runtime.Server.RateLimit.RequestsPerSecond <= 0 {
		return fmt.Errorf("runtime.server.rateLimit.requestsPerSecond must be positive, got %v", c.Runtime.Server.RateLimit.RequestsPerSecond)
	}
	if c.Runtime.Server.RateLimit.Burst <= 0 {
		return fmt.Errorf("runtime.server.rateLimit.burst must be positive, got %d", c.Runtime.Server.RateLimit.Burst)
	}
	if err := c.validateAlignmentCheck(); err != nil {
		return err
	}
	return c.validateInteractive()
}

// validateRuntime checks the runtime server/session/audit/shutdown sections.
func (c *Config) validateRuntime() error {
	if err := c.Runtime.Logging.Validate(); err != nil {
		return err
	}

	if c.Runtime.Server.Port < 1 || c.Runtime.Server.Port > 65535 {
		return fmt.Errorf("runtime.server.port must be 1-65535, got %d", c.Runtime.Server.Port)
	}
	if c.Runtime.Server.MaxConcurrentRequests < 0 {
		return fmt.Errorf("runtime.server.maxConcurrentRequests must be non-negative, got %d", c.Runtime.Server.MaxConcurrentRequests)
	}
	if c.Runtime.Session.TTL <= 0 {
		return fmt.Errorf("runtime.session.ttl must be positive, got %v", c.Runtime.Session.TTL)
	}
	if c.Runtime.Session.MaxConcurrentInvestigations <= 0 {
		return fmt.Errorf("runtime.session.maxConcurrentInvestigations must be positive, got %d", c.Runtime.Session.MaxConcurrentInvestigations)
	}
	if c.Runtime.Audit.Enabled {
		if c.Runtime.Audit.BufferSize <= 0 {
			return fmt.Errorf("runtime.audit.bufferSize must be positive when audit enabled, got %d", c.Runtime.Audit.BufferSize)
		}
		if c.Runtime.Audit.BatchSize <= 0 {
			return fmt.Errorf("runtime.audit.batchSize must be positive when audit enabled, got %d", c.Runtime.Audit.BatchSize)
		}
	}
	switch c.Runtime.Audit.Verbosity {
	case "full", "standard", "minimal", "":
	default:
		return fmt.Errorf("runtime.audit.verbosity must be full, standard, or minimal, got %q", c.Runtime.Audit.Verbosity)
	}
	if c.Runtime.Shutdown.DrainSeconds <= 0 {
		return fmt.Errorf("runtime.shutdown.drainSeconds must be positive, got %d; "+
			"the operator default is 30s", c.Runtime.Shutdown.DrainSeconds)
	}
	if c.Runtime.Shutdown.DrainSeconds > maxDrainSeconds {
		return fmt.Errorf("runtime.shutdown.drainSeconds must not exceed %d, got %d; "+
			"the operator enforces this upper bound via CRD validation",
			maxDrainSeconds, c.Runtime.Shutdown.DrainSeconds)
	}
	return nil
}

// validateAI checks the AI investigation/summarizer/enrichment/anomaly-safety sections.
func (c *Config) validateAI() error {
	if c.AI.Investigation.MaxTurns <= 0 {
		return fmt.Errorf("ai.investigation.maxTurns must be positive, got %d", c.AI.Investigation.MaxTurns)
	}
	if err := c.AI.Investigation.validateConfidenceBands(); err != nil {
		return err
	}
	if c.AI.Summarizer.MaxToolOutputSize <= 0 {
		return fmt.Errorf("ai.summarizer.maxToolOutputSize must be positive, got %d", c.AI.Summarizer.MaxToolOutputSize)
	}
	if c.AI.Enrichment.MaxRetries < 0 {
		return fmt.Errorf("ai.enrichment.maxRetries must be non-negative, got %d", c.AI.Enrichment.MaxRetries)
	}
	if c.AI.Safety.Anomaly.MaxToolCallsPerTool <= 0 {
		return fmt.Errorf("ai.safety.anomaly.maxToolCallsPerTool must be positive, got %d", c.AI.Safety.Anomaly.MaxToolCallsPerTool)
	}
	if c.AI.Safety.Anomaly.MaxTotalToolCalls <= 0 {
		return fmt.Errorf("ai.safety.anomaly.maxTotalToolCalls must be positive, got %d", c.AI.Safety.Anomaly.MaxTotalToolCalls)
	}
	if c.AI.Safety.Anomaly.MaxRepeatedFailures <= 0 {
		return fmt.Errorf("ai.safety.anomaly.maxRepeatedFailures must be positive, got %d", c.AI.Safety.Anomaly.MaxRepeatedFailures)
	}
	return nil
}

// validateConfidenceBands checks the BR-KA-213 / Issue #1826 investigation-outcome
// confidence bands. Unlike AIAnalysis's pointer-typed RegoConfig knobs (where nil
// distinguishes "unset" from "explicitly zero"), InvestigationConfig's fields are
// plain float64: Load() always starts from DefaultConfig()'s 0.7/0.5 before
// unmarshaling, so by the time Validate() runs the field already holds either the
// default or an operator-supplied value — there is no "unset" state left to special
// case, matching this struct's existing MaxTurns <= 0 always-reject convention.
func (ic *InvestigationConfig) validateConfidenceBands() error {
	if err := validateConfidenceUnitInterval(ic.ResolvedConfidenceThreshold, "ai.investigation.resolvedConfidenceThreshold"); err != nil {
		return err
	}
	if err := validateConfidenceUnitInterval(ic.InconclusiveConfidenceThreshold, "ai.investigation.inconclusiveConfidenceThreshold"); err != nil {
		return err
	}
	if ic.InconclusiveConfidenceThreshold >= ic.ResolvedConfidenceThreshold {
		return fmt.Errorf("ai.investigation.inconclusiveConfidenceThreshold (%v) must be less than resolvedConfidenceThreshold (%v)",
			ic.InconclusiveConfidenceThreshold, ic.ResolvedConfidenceThreshold)
	}
	return nil
}

// validateConfidenceUnitInterval checks that a confidence-style field falls in
// range (0.0, 1.0].
func validateConfidenceUnitInterval(v float64, fieldName string) error {
	if v <= 0 || v > 1.0 {
		return fmt.Errorf("%s must be in range (0.0, 1.0], got %v", fieldName, v)
	}
	return nil
}

// validateLLMOAuth2AndTLS checks the AI.LLM OAuth2 and mTLS settings (SC-8).
func (c *Config) validateLLMOAuth2AndTLS() error {
	if c.AI.LLM.OAuth2.Enabled {
		if c.AI.LLM.OAuth2.TokenURL == "" {
			return fmt.Errorf("ai.llm.oauth2.tokenURL is required when oauth2.enabled=true")
		}
		if c.AI.LLM.OAuth2.CredentialsDir == "" {
			return fmt.Errorf("ai.llm.oauth2.credentialsDir is required when oauth2.enabled=true")
		}
	}
	return validateTLSCertPair("ai.llm", c.AI.LLM.TLSCertFile, c.AI.LLM.TLSKeyFile, c.AI.LLM.TLSCaFile)
}

// validateFleetIntegration checks the Fleet OAuth2 integration settings and
// the Endpoint/GatewayType pairing.
//
// #1553: registerFleetTools (cmd/kubernautagent/toolregistry.go) silently
// treats fleet as disabled when either Endpoint or GatewayType is empty.
// Without this check, an operator who set one but not the other got no
// startup error -- KA just ran with fleet silently off instead of failing
// closed on the misconfiguration.
func (c *Config) validateFleetIntegration() error {
	endpoint := c.Integrations.Fleet.Endpoint
	gatewayType := c.Integrations.Fleet.GatewayType

	if endpoint != "" && gatewayType == "" {
		return fmt.Errorf("integrations.fleet.gatewayType is required when integrations.fleet.endpoint is set")
	}
	if gatewayType != "" && endpoint == "" {
		return fmt.Errorf("integrations.fleet.endpoint is required when integrations.fleet.gatewayType is set")
	}

	// Issue #1984 Phase D (ADR-068 gap closure, IA-2/AC-3/AC-17): ADR-068
	// mandates OAuth2 authentication for every service-to-MCP-Gateway
	// connection, but oauth2.enabled defaulted to false with no guard
	// requiring it true -- KA could reach the MCP Gateway completely
	// unauthenticated whenever integrations.fleet.endpoint was set.
	// Mirrors FleetMetadataCache's existing unconditional oauth2
	// requirement.
	if endpoint != "" && !c.Integrations.Fleet.OAuth2.Enabled {
		return fmt.Errorf("integrations.fleet.oauth2.enabled must be true when integrations.fleet.endpoint is set (ADR-068 mandates OAuth2 authentication for all MCP Gateway connections)")
	}

	if c.Integrations.Fleet.OAuth2.Enabled {
		if c.Integrations.Fleet.OAuth2.TokenURL == "" {
			return fmt.Errorf("integrations.fleet.oauth2.tokenURL is required when oauth2.enabled=true")
		}
		if c.Integrations.Fleet.OAuth2.CredentialsSecretRef == "" {
			return fmt.Errorf("integrations.fleet.oauth2.credentialsSecretRef is required when oauth2.enabled=true")
		}
	}

	return nil
}

// validateAlignmentCheck checks the AI alignment-check (shadow evaluator) settings.
func (c *Config) validateAlignmentCheck() error {
	if !c.AI.AlignmentCheck.Enabled {
		return nil
	}
	if c.AI.AlignmentCheck.Timeout <= 0 {
		return fmt.Errorf("ai.alignmentCheck.timeout must be positive when enabled, got %v", c.AI.AlignmentCheck.Timeout)
	}
	if c.AI.AlignmentCheck.MaxStepTokens <= 0 {
		return fmt.Errorf("ai.alignmentCheck.maxStepTokens must be positive when enabled, got %d", c.AI.AlignmentCheck.MaxStepTokens)
	}
	if c.AI.AlignmentCheck.VerdictTimeout <= 0 {
		return fmt.Errorf("ai.alignmentCheck.verdictTimeout must be positive when enabled, got %v", c.AI.AlignmentCheck.VerdictTimeout)
	}
	if c.AI.AlignmentCheck.MaxRetries < 0 {
		return fmt.Errorf("ai.alignmentCheck.maxRetries must be non-negative when enabled, got %d", c.AI.AlignmentCheck.MaxRetries)
	}
	switch c.AI.AlignmentCheck.Mode {
	case AlignmentModeEnforce, AlignmentModeMonitor:
	default:
		return fmt.Errorf("ai.alignmentCheck.mode must be 'enforce' or 'monitor', got %q", c.AI.AlignmentCheck.Mode)
	}
	if c.AI.AlignmentCheck.GroundingReview.Enabled {
		if c.AI.AlignmentCheck.GroundingReview.Timeout <= 0 {
			return fmt.Errorf("ai.alignmentCheck.groundingReview.timeout must be positive when enabled, got %v", c.AI.AlignmentCheck.GroundingReview.Timeout)
		}
		if c.AI.AlignmentCheck.GroundingReview.MaxConversationTokens <= 0 {
			return fmt.Errorf("ai.alignmentCheck.groundingReview.maxConversationTokens must be positive when enabled, got %d", c.AI.AlignmentCheck.GroundingReview.MaxConversationTokens)
		}
	}
	return nil
}

// validateInteractive checks the interactive-session settings.
func (c *Config) validateInteractive() error {
	if !c.Interactive.Enabled {
		return nil
	}
	if c.Interactive.SessionTTL <= 0 {
		return fmt.Errorf("interactive.sessionTTL must be positive when enabled, got %v", c.Interactive.SessionTTL)
	}
	if c.Interactive.SessionTTL > time.Hour {
		return fmt.Errorf("interactive.sessionTTL must not exceed 1h, got %v", c.Interactive.SessionTTL)
	}
	if c.Interactive.InactivityTimeout <= 0 {
		return fmt.Errorf("interactive.inactivityTimeout must be positive when enabled, got %v", c.Interactive.InactivityTimeout)
	}
	if c.Interactive.InactivityTimeout > 30*time.Minute {
		return fmt.Errorf("interactive.inactivityTimeout must not exceed 30m, got %v", c.Interactive.InactivityTimeout)
	}
	if c.Interactive.MaxConcurrentSessions <= 0 {
		return fmt.Errorf("interactive.maxConcurrentSessions must be positive when enabled, got %d", c.Interactive.MaxConcurrentSessions)
	}
	if c.Interactive.MaxConcurrentSessions > 100 {
		return fmt.Errorf("interactive.maxConcurrentSessions must not exceed 100, got %d", c.Interactive.MaxConcurrentSessions)
	}
	if c.Interactive.RateLimitPerUser <= 0 {
		c.Interactive.RateLimitPerUser = 10
	}
	if c.Interactive.RateLimitPerUser > 100 {
		return fmt.Errorf("interactive.rateLimitPerUser must not exceed 100, got %d", c.Interactive.RateLimitPerUser)
	}
	return c.Interactive.validateJWTProviders()
}

// defaultRuntimeConfig returns the production-default RuntimeConfig (server,
// session, audit, shutdown). Extracted from DefaultConfig per the
// GO-ANTIPATTERN-AUDIT-2026-07-01 complexity remediation (Wave C).
func defaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		Logging: internalconfig.DefaultLoggingConfig(),
		Debug:   internalconfig.DefaultDebugConfig(),
		Server: ServerConfig{
			Address: "0.0.0.0", Port: 8443, HealthAddr: ":8081", MetricsAddr: ":9090",
			DisableAdminEndpoints: true,
			MaxConcurrentRequests: 100,
			RateLimit: RateLimitConfig{
				RequestsPerSecond: 5,
				Burst:             10,
				CleanupInterval:   5 * time.Minute,
				MaxAge:            10 * time.Minute,
			},
		},
		Session: SessionConfig{TTL: 30 * time.Minute, MaxConcurrentInvestigations: 10},
		Audit: AuditConfig{
			Enabled:    true,
			BufferSize: 100,
			BatchSize:  10,
			Verbosity:  "full",
		},
		Shutdown: ShutdownConfig{DrainSeconds: 30},
	}
}

// defaultAIConfig returns the production-default AIConfig (LLM, investigation,
// summarizer, enrichment, alignment-check, safety). Extracted from
// DefaultConfig per the GO-ANTIPATTERN-AUDIT-2026-07-01 complexity
// remediation (Wave C).
// V1.0 defaults for the investigation-outcome confidence bands (BR-KA-213,
// Issue #1826), mirroring the values previously hardcoded directly in
// phase3_workflow_selection.tmpl.
const (
	defaultResolvedConfidenceThreshold     = 0.7
	defaultInconclusiveConfidenceThreshold = 0.5
)

func defaultAIConfig() AIConfig {
	return AIConfig{
		LLM: types.LLMConfig{Provider: "openai"},
		Investigation: InvestigationConfig{
			MaxTurns:                        40,
			ResolvedConfidenceThreshold:     defaultResolvedConfidenceThreshold,
			InconclusiveConfidenceThreshold: defaultInconclusiveConfidenceThreshold,
		},
		Summarizer: SummarizerConfig{
			Threshold:         8000,
			MaxToolOutputSize: DefaultMaxToolOutputSize,
		},
		Enrichment: EnrichmentConfig{
			MaxRetries:  3,
			BaseBackoff: 1 * time.Second,
		},
		AlignmentCheck: AlignmentCheckConfig{
			Enabled:        false,
			Mode:           AlignmentModeEnforce,
			Timeout:        10 * time.Second,
			MaxStepTokens:  500,
			MaxRetries:     1,
			VerdictTimeout: 30 * time.Second,
			Canary:         CanaryConfig{ForceEscalation: true},
			GroundingReview: GroundingReviewConfig{
				Enabled:               false,
				Timeout:               30 * time.Second,
				MaxConversationTokens: 32000,
			},
		},
		Safety: SafetyConfig{
			Sanitization: SanitizationConfig{
				InjectionPatternsEnabled: true,
				CredentialScrubEnabled:   true,
				SecretRedactionEnabled:   true,
			},
			Anomaly: AnomalyConfig{
				MaxToolCallsPerTool: 10,
				MaxTotalToolCalls:   30,
				MaxRepeatedFailures: 3,
				ExemptPrefixes:      []string{"todo_"},
			},
			ToolCallTimeout: 60 * time.Second,
		},
	}
}

// DefaultConfig returns a Config with production defaults applied.
func DefaultConfig() *Config {
	return &Config{
		Runtime: defaultRuntimeConfig(),
		AI:      defaultAIConfig(),
		Integrations: IntegrationsConfig{
			DataStorage: DataStorageConfig{SATokenPath: "/var/run/secrets/kubernetes.io/serviceaccount/token"},
		},
	}
}

// DefaultLLMRuntime returns an LLMRuntimeConfig with sensible production defaults.
//
// Temperature is deliberately left nil (#1749): not every model/provider
// accepts the parameter (e.g. claude-opus-4-8 rejects it with a 400), so
// the safe default is to omit it from the wire request entirely unless an
// operator explicitly configures one for their model.
func DefaultLLMRuntime() *LLMRuntimeConfig {
	return &LLMRuntimeConfig{
		MaxRetries:     3,
		TimeoutSeconds: 120,
	}
}

// validateTLSCertPair validates that tlsCertFile and tlsKeyFile are set
// together, use absolute paths, and require tlsCaFile (server verification
// remains mandatory per SC-8).
func validateTLSCertPair(prefix, certFile, keyFile, caFile string) error {
	hasCert := certFile != ""
	hasKey := keyFile != ""
	if hasCert != hasKey {
		return fmt.Errorf("%s.tlsCertFile and %s.tlsKeyFile must both be set or both be empty", prefix, prefix)
	}
	if !hasCert {
		return nil
	}
	if !filepath.IsAbs(certFile) {
		return fmt.Errorf("%s.tlsCertFile must be an absolute path, got %q", prefix, certFile)
	}
	if !filepath.IsAbs(keyFile) {
		return fmt.Errorf("%s.tlsKeyFile must be an absolute path, got %q", prefix, keyFile)
	}
	if caFile == "" {
		return fmt.Errorf("%s.tlsCaFile is required when client certificates are configured (server verification is mandatory)", prefix)
	}
	return nil
}
