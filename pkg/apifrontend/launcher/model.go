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
	"bytes"
	"context"
	"fmt"
	"iter"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/auth/httptransport"
	adkanthropic "github.com/Alcova-AI/adk-anthropic-go/v2"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/model/gemini"
	"google.golang.org/genai"

	internaltransport "github.com/jordigilh/kubernaut/internal/kubernautagent/llm/transport"
	openaimodel "github.com/jordigilh/kubernaut/pkg/apifrontend/launcher/openai"
	llmtransport "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// gcpAuthScope is the OAuth2 scope requested for Vertex AI credentials,
// matching kubernautagent/llm/geminifamily and anthropicfamily's own Vertex
// auth resolution (DD-LLM-010).
const gcpAuthScope = "https://www.googleapis.com/auth/cloud-platform"

// NewModelFromConfig constructs an ADK model.LLM from the AF LLM configuration.
// It builds the appropriate transport chain (TLS CA, OAuth2, custom headers,
// circuit breaker) and creates the provider-specific model client.
//
// The Anthropic/Vertex cases below use adk-anthropic-go's model.LLM wrapper
// rather than kubernautagent's anthropicfamily.Client: AF's launcher is
// entirely ADK-based (session/event/tool semantics all speak ADK's
// model.LLM contract), while anthropicfamily implements KA's own,
// deliberately framework-independent llm.Client interface (DD-KA-019-001).
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

// newVertexAIModel dispatches provider: vertex_ai to the correct
// model.LLM implementation for the configured model family. vertex_ai can
// host either Claude or Gemini models (#1778, #1792) — previously this
// unconditionally assumed Claude, silently mis-constructing an
// adk-anthropic-go model for a gemini-* model. Disambiguated here using
// the shared types.IsAnthropicModel/IsGeminiModel detectors, the same
// ones KA's llm_builder.go uses for the identical ambiguity. A model
// matching neither family fails fast here instead of silently falling
// through to Gemini and failing later with a confusing SDK-level error
// (found in the #1778/#1792 GA readiness audit).
func newVertexAIModel(ctx context.Context, cfg types.LLMConfig) (model.LLM, error) {
	if types.IsAnthropicModel(cfg.Model) {
		return newVertexAnthropicModel(ctx, cfg)
	}
	if types.IsGeminiModel(cfg.Model) {
		return newVertexGeminiModel(ctx, cfg) //nolint:contextcheck // LLM transport chain lazily builds an OAuth2 client-credentials token source shared across future requests
	}
	return nil, fmt.Errorf("vertex_ai: unrecognized model family for model %q (expected a claude-* or gemini-* model)", cfg.Model)
}

// newVertexAnthropicModel constructs the adk-anthropic-go Vertex AI model
// for a Claude-family model. Extracted, unchanged in behavior, from the
// original newVertexAIModel so that function could become the vertex_ai
// dispatch point above (#1778, #1792).
func newVertexAnthropicModel(ctx context.Context, cfg types.LLMConfig) (m model.LLM, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("vertex_ai: GCP ADC unavailable — set GOOGLE_APPLICATION_CREDENTIALS or provide credentials: %v", r)
		}
	}()
	if err := InjectAmbientGoogleCredentials(cfg); err != nil {
		return nil, fmt.Errorf("vertex_ai: %w", err)
	}
	adkCfg := &adkanthropic.Config{
		Variant:         adkanthropic.VariantVertexAI,
		VertexProjectID: cfg.VertexProject,
		VertexLocation:  cfg.VertexLocation,
	}
	if cfg.Endpoint != "" {
		adkCfg.BaseURL = cfg.Endpoint
	}
	llm, err := adkanthropic.NewModel(ctx, cfg.Model, adkCfg)
	if err != nil {
		return nil, err
	}
	return wrapWithTimeout(llm, cfg), nil
}

// newVertexGeminiModel constructs an ADK model.LLM for a Gemini model
// hosted on Vertex AI (BR-AI-087, #1778, #1792).
//
// Deliberately distinct from newGeminiModel (native Gemini API, apiKey
// auth against generativelanguage.googleapis.com): Vertex AI authenticates
// with GCP credentials against a project/location-scoped
// aiplatform.googleapis.com endpoint. genai.ClientConfig has no
// "credentials JSON bytes" field — only a resolved *auth.Credentials or an
// already-authenticated *http.Client — so credentials must be resolved
// explicitly here (credentials.DetectDefault, mirroring
// kubernautagent/llm/geminifamily.New, DD-LLM-010) rather than left to
// genai's own Vertex auto-ADC fallback, which only activates when no
// HTTPClient is set at all and would otherwise bypass AF's transport chain
// (TLS CA, custom headers, circuit breaker) entirely.
//
// cfg.APIKey carries the mounted credentials.json's content here (resolved
// generically by pkg/apifrontend/config's resolveLLMKey from
// cfg.APIKeyFile) — as of kubernaut#1801, this is real service-account
// bytes rather than always-empty, since the Helm chart now renders
// apiKeyFile for vertex_ai too. No env var is touched on this path, ever,
// matching Kubernaut Agent's geminifamily.New.
func newVertexGeminiModel(ctx context.Context, cfg types.LLMConfig) (model.LLM, error) {
	cred, err := credentials.DetectDefault(&credentials.DetectOptions{
		CredentialsJSON: bytes.TrimSpace([]byte(cfg.APIKey)),
		Scopes:          []string{gcpAuthScope},
	})
	if err != nil {
		return nil, fmt.Errorf("vertex_ai (gemini): resolving credentials: %w", err)
	}

	base, err := buildTransportChain(cfg) //nolint:contextcheck // LLM transport chain lazily builds an OAuth2 client-credentials token source shared across future requests
	if err != nil {
		return nil, fmt.Errorf("vertex_ai (gemini) transport chain: %w", err)
	}

	httpClient, err := httptransport.NewClient(&httptransport.Options{
		Credentials:      cred,
		BaseRoundTripper: base,
	})
	if err != nil {
		return nil, fmt.Errorf("vertex_ai (gemini): building HTTP client: %w", err)
	}
	httpClient.Timeout = time.Duration(types.DefaultLLMTimeoutSeconds) * time.Second
	if cfg.TimeoutSeconds > 0 {
		httpClient.Timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}

	clientCfg := &genai.ClientConfig{
		Backend:    genai.BackendVertexAI,
		Project:    cfg.VertexProject,
		Location:   cfg.VertexLocation,
		HTTPClient: httpClient,
	}
	if cfg.Endpoint != "" {
		clientCfg.HTTPOptions = genai.HTTPOptions{BaseURL: cfg.Endpoint}
	}

	return gemini.NewModel(ctx, cfg.Model, clientCfg)
}

// InjectAmbientGoogleCredentials sets GOOGLE_APPLICATION_CREDENTIALS
// in-process to cfg.APIKeyFile, for provider: vertex_ai model families
// whose upstream SDK has no explicit-credentials-bytes option and can only
// discover credentials via ambient ADC (ADC = Google's Application Default
// Credentials lookup chain, which checks this env var first).
//
// As of kubernaut#1861, this is used by exactly one call site: AF's own
// agent.llm Claude-on-Vertex connection (newVertexAnthropicModel below),
// via adk-anthropic-go v1.0.0, which hardcodes
// anthropic-sdk-go/vertex.WithGoogleAuth internally with no credentials
// override exposed. Every other vertex_ai path in this package/AF now
// resolves credentials explicitly from cfg.APIKey instead:
// newVertexGeminiModel (this file, since #1801) and both of
// cmd/apifrontend's severityTriage constructors, newAnthropicTriagerForVertex
// and newGenAITriagerForVertex (since #1861, mirroring release/v1.5's
// #1870) -- the latter two used to call this function too, which is
// exactly what previously required the DD-PLATFORM-007 Helm-chart fail()
// guard blocking AF's main and severityTriage profiles from both resolving
// to vertex_ai with different credentialsSecretName values. That guard has
// been removed: since severityTriage no longer touches this env var at
// all, there is no longer a shared-mutable-state collision to prevent,
// even though this one remaining call site still relies on ambient ADC.
//
// This performs the env var assignment here, in Go, immediately before
// construction, rather than declaring it statically in the Helm Deployment
// manifest (kubernaut#1801) -- mirroring the same runtime-injection
// pattern already used elsewhere in Kubernaut (Kubernaut Agent never
// touches this env var at all, passing credential bytes explicitly
// instead; the HolmesGPT API predecessor used an analogous
// _inject_runtime_env() at startup) to avoid exposing credential-adjacent
// config statically in the pod spec, where it's visible via `kubectl get
// pod -o yaml` to anyone with pod-read RBAC.
//
// No-ops when cfg.APIKeyFile is empty (non-vertex_ai providers, or a
// vertex_ai profile that -- unexpectedly -- has no resolved credentials
// file), leaving any pre-existing ambient ADC state untouched.
func InjectAmbientGoogleCredentials(cfg types.LLMConfig) error {
	if cfg.APIKeyFile == "" {
		return nil
	}
	if err := os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", cfg.APIKeyFile); err != nil {
		return fmt.Errorf("set GOOGLE_APPLICATION_CREDENTIALS: %w", err)
	}
	return nil
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
	llm, err := adkanthropic.NewModel(ctx, cfg.Model, adkCfg)
	if err != nil {
		return nil, err
	}
	return wrapWithTimeout(llm, cfg), nil
}

// timeoutModel wraps a model.LLM so every GenerateContent call is bounded
// by a context deadline, working around adk-anthropic-go not exposing
// HTTP client injection (#1342) — the only two providers (direct Anthropic
// API and Vertex-hosted Claude) still missing any client-side timeout
// after #1955's audit. The deadline is applied to the ctx that flows into
// the (lazily-evaluated) returned iterator, so it bounds the real,
// deferred SDK call — adk-anthropic-go's own GenerateContent/generateStream
// return closures that don't touch the network until ranged over, and
// anthropic-sdk-go builds its HTTP request via http.NewRequestWithContext,
// which binds connect, header-read, and body/SSE read to ctx — not just
// this function's own (instant) return.
type timeoutModel struct {
	inner   model.LLM
	timeout time.Duration
}

func (m *timeoutModel) Name() string { return m.inner.Name() }

func (m *timeoutModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		callCtx, cancel := context.WithTimeout(ctx, m.timeout)
		defer cancel()
		for resp, err := range m.inner.GenerateContent(callCtx, req, stream) {
			if !yield(resp, err) {
				return
			}
		}
	}
}

// wrapWithTimeout resolves the configured LLM call timeout (falling back
// to DefaultLLMTimeoutSeconds, mirroring BuildLLMHTTPClient's identical
// resolution for the Gemini/OpenAI-compatible/Vertex-Gemini paths) and
// wraps inner with a timeoutModel enforcing it at the context layer
// instead of the HTTP client layer.
func wrapWithTimeout(inner model.LLM, cfg types.LLMConfig) model.LLM {
	timeout := time.Duration(types.DefaultLLMTimeoutSeconds) * time.Second
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	return &timeoutModel{inner: inner, timeout: timeout}
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
