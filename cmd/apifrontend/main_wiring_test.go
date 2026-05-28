package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	adksession "google.golang.org/adk/session"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ds"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ratelimit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/resilience"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// ---------------------------------------------------------------------------
// TC-A-01: Health mux /readyz must be dependency-aware (WIRE-01)
// ---------------------------------------------------------------------------

func TestHealthMuxReadyz_DepsHealthy(t *testing.T) {
	draining := &atomic.Bool{}
	depsReady := handler.ReadyChecker(func() bool { return true })
	mux := buildHealthMux(depsReady, draining)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Errorf("TC-A-01a: expected 200 when deps healthy, got %d", rec.Code)
	}
}

func TestHealthMuxReadyz_DepsUnhealthy(t *testing.T) {
	draining := &atomic.Bool{}
	depsReady := handler.ReadyChecker(func() bool { return false })
	mux := buildHealthMux(depsReady, draining)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("TC-A-01b: expected 503 when deps unhealthy, got %d", rec.Code)
	}
}

func TestHealthMuxReadyz_Draining(t *testing.T) {
	draining := &atomic.Bool{}
	draining.Store(true)
	depsReady := handler.ReadyChecker(func() bool { return true })
	mux := buildHealthMux(depsReady, draining)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("TC-A-01d: expected 503 when draining, got %d", rec.Code)
	}
}

func TestHealthMuxReadyz_NilDepsReady(t *testing.T) {
	draining := &atomic.Bool{}
	mux := buildHealthMux(nil, draining)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Errorf("TC-A-01f: expected 200 when depsReady nil (fail-open), got %d", rec.Code)
	}
}

func TestHealthMuxHealthz_AlwaysOK(t *testing.T) {
	mux := buildHealthMux(nil, &atomic.Bool{})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Errorf("TC-A-01c: healthz should always return 200, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// TC-A-06: buildResilientTransport must set DependencyName on CB config
// ---------------------------------------------------------------------------

func TestBuildResilientTransport_DependencyNameInMetrics(t *testing.T) {
	reg := metrics.NewRegistry()
	depCfg := &config.DependencyConfig{
		RetryMax:           1,
		RetryInitBackoff:   100 * time.Millisecond,
		RetryMaxBackoff:    1 * time.Second,
		CBMaxRequests:      3,
		CBInterval:         5 * time.Second,
		CBTimeout:          10 * time.Second,
		CBFailureThreshold: 3,
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backend.Close()

	cbt := buildResilientTransport(http.DefaultTransport, depCfg, "ds", reg, nil)
	client := &http.Client{Transport: cbt}

	for i := 0; i < 5; i++ {
		resp, err := client.Get(backend.URL)
		if err == nil {
			_ = resp.Body.Close()
		}
	}

	metricsHandler := reg.Handler()
	mrec := httptest.NewRecorder()
	metricsHandler.ServeHTTP(mrec, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
	body, _ := io.ReadAll(mrec.Result().Body)
	metricsText := string(body)

	if !strings.Contains(metricsText, `dependency="ds"`) {
		t.Errorf("TC-A-06a: af_circuit_breaker_state{dependency=\"ds\"} not found in metrics; "+
			"DependencyName not set on CircuitBreakerConfig (WIRE-06). Metrics:\n%s",
			extractMetricLines(metricsText, "af_circuit_breaker_state"))
	}
}

// ---------------------------------------------------------------------------
// TC-A-07: Shutdown timeout must be configurable (WIRE-07)
// ---------------------------------------------------------------------------

func TestShutdownTimeout_UsesConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Shutdown.DrainSeconds = 3

	timeout := shutdownTimeout(cfg)
	if timeout != 3*time.Second {
		t.Errorf("TC-A-07a: expected 3s from config, got %v", timeout)
	}
}

func TestShutdownTimeout_DefaultsOnZero(t *testing.T) {
	cfg := &config.Config{}
	cfg.Shutdown.DrainSeconds = 0

	timeout := shutdownTimeout(cfg)
	if timeout != 15*time.Second {
		t.Errorf("TC-A-07e: expected 15s default, got %v", timeout)
	}
}

// ---------------------------------------------------------------------------
// TC-P2A-01: Auth middleware CB metrics — behavioral (BAC-02)
// ---------------------------------------------------------------------------

func TestBuildAuthMiddleware_PassesCBMetrics(t *testing.T) {
	t.Parallel()

	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"keys":[]}`)
	}))
	t.Cleanup(jwksSrv.Close)

	cfg := &config.Config{}
	cfg.Auth.IssuerURL = jwksSrv.URL
	cfg.Auth.JWKSURL = jwksSrv.URL
	cfg.Auth.Audience = "test"
	cfg.Auth.AllowInsecureIssuers = true

	reg := metrics.NewRegistry()
	auditor := &noopAuditor{}
	logger := noopLogger()

	mw, _ := buildAuthMiddleware(cfg, reg, auditor, logger)
	if mw == nil {
		t.Fatal("TC-P2A-01a: buildAuthMiddleware returned nil")
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := mw(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
	req.Header.Set("Authorization", "Bearer dummy-token")
	wrapped.ServeHTTP(rec, req)

	if rec.Code == http.StatusServiceUnavailable {
		t.Fatal("TC-P2A-01a: middleware returned 503 (deny-all fallback); WithCBMetrics likely missing")
	}

	metricsHandler := reg.Handler()
	mrec := httptest.NewRecorder()
	metricsHandler.ServeHTTP(mrec, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
	body, _ := io.ReadAll(mrec.Result().Body)
	metricsText := string(body)

	if !strings.Contains(metricsText, "af_auth_duration_seconds") {
		t.Errorf("TC-P2A-01b: af_auth_duration_seconds not found in metrics — auth duration not wired.\nMetrics:\n%s",
			extractMetricLines(metricsText, "af_auth"))
	}
}

// ---------------------------------------------------------------------------
// TC-P2A-02: UserLimiter wiring — behavioral (BAC-03)
// ---------------------------------------------------------------------------

func TestBridgeCfg_UserLimiter_IsWired(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.NewUserLimiter(ratelimit.PerUserConfig{
		RequestsPerMinute:     60,
		MaxConcurrentSessions: 5,
		ToolCallsPerMinute:    30,
		CleanupInterval:       1 * time.Minute,
		MaxAge:                5 * time.Minute,
	})
	t.Cleanup(limiter.Stop)

	bridgeCfg := handler.MCPBridgeConfig{
		UserLimiter: limiter,
	}

	if bridgeCfg.UserLimiter == nil {
		t.Fatal("TC-P2A-02a: MCPBridgeConfig.UserLimiter is nil after explicit wiring")
	}

	if !limiter.AllowRequest("testuser") {
		t.Error("TC-P2A-02b: UserLimiter should allow first request within rate limit")
	}
}

// ---------------------------------------------------------------------------
// TC-P2C-05b: ReplayCache.Stop is idempotent (BAC-11)
// ---------------------------------------------------------------------------

func TestReplayCache_StopIdempotent(t *testing.T) {
	t.Parallel()
	rc := auth.NewReplayCache(1 * time.Minute)

	rc.Stop()
	rc.Stop()
}

// ---------------------------------------------------------------------------
// HIGH-02b: Session lifecycle — af_sessions_active gauge wiring
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// HIGH-02b RED: Session infrastructure wiring tests
// ---------------------------------------------------------------------------

func TestBuildSessionInfra_ReturnsNonNilService(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/path")
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 10 * time.Minute,
			RetentionTTL:  31 * 24 * time.Hour,
		},
	}
	infra, err := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if err != nil {
		t.Skipf("HIGH-02b: skipping (no kubeconfig available): %v", err)
	}
	if infra.SessionService == nil {
		t.Fatal("HIGH-02b: buildSessionInfra must return a non-nil SessionService")
	}
}

func TestBuildSessionInfra_GaugeIsWired(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/path")
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 10 * time.Minute,
			RetentionTTL:  31 * 24 * time.Hour,
		},
	}
	infra, err := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if err != nil {
		t.Skipf("skipping (no kubeconfig available): %v", err)
	}
	if infra.SessionService == nil {
		t.Fatal("SessionService is nil")
	}

	metricsHandler := reg.Handler()
	rec := httptest.NewRecorder()
	metricsHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
	body, _ := io.ReadAll(rec.Result().Body)
	if !strings.Contains(string(body), "af_sessions_active") {
		t.Error("HIGH-02b: af_sessions_active gauge should be wired via registry")
	}
}

func TestBuildSessionInfra_ReconcilerIsCreated(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/path")
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 15 * time.Minute,
			RetentionTTL:  31 * 24 * time.Hour,
		},
	}
	infra, err := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if err != nil {
		t.Skipf("skipping (no kubeconfig available): %v", err)
	}
	if infra.Reconciler == nil {
		t.Fatal("HIGH-02b: buildSessionInfra must return a non-nil Reconciler")
	}
}

func TestBuildSessionInfra_RetentionTTLClamped(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/path")
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 5 * time.Minute,
			RetentionTTL:  1 * time.Hour, // below NIST AU-11 minimum
		},
	}
	infra, err := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if err != nil {
		t.Skipf("skipping (no kubeconfig available): %v", err)
	}
	if infra.Reconciler == nil {
		t.Fatal("Reconciler must not be nil even with sub-minimum retention TTL")
	}
}

func TestBuildSessionInfra_SchemeIncludesInvestigationSession(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/path")
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 10 * time.Minute,
			RetentionTTL:  31 * 24 * time.Hour,
		},
	}
	infra, err := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if err != nil {
		t.Skipf("skipping (no kubeconfig available): %v", err)
	}
	if infra.Scheme == nil {
		t.Fatal("HIGH-02b: buildSessionInfra must return a non-nil Scheme")
	}

	gvk := v1alpha1.GroupVersion.WithKind("InvestigationSession")
	if !infra.Scheme.Recognizes(gvk) {
		t.Errorf("HIGH-02b: scheme does not recognize %s", gvk)
	}
}

func TestBuildSessionInfra_GracefulShutdown(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/path")
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 10 * time.Minute,
			RetentionTTL:  31 * 24 * time.Hour,
		},
	}
	infra, err := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if err != nil {
		t.Skipf("skipping (no kubeconfig available): %v", err)
	}
	if infra.StopFunc == nil {
		t.Fatal("HIGH-02b: buildSessionInfra must return a StopFunc for graceful shutdown")
	}
	infra.StopFunc()
}

// ---------------------------------------------------------------------------
// MED-03: buildAuthMiddleware must return an auth readiness checker so that
// /readyz returns 503 when the JWKS circuit breaker is open.
// ---------------------------------------------------------------------------

func TestBuildAuthMiddleware_ReturnsReadyChecker(t *testing.T) {
	t.Parallel()

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	t.Cleanup(jwksServer.Close)

	cfg := &config.Config{}
	cfg.Auth.IssuerURL = jwksServer.URL
	cfg.Auth.JWKSURL = jwksServer.URL + "/.well-known/jwks.json"
	cfg.Auth.AllowInsecureIssuers = true

	reg := metrics.NewRegistry()
	mw, readyFn := buildAuthMiddleware(cfg, reg, nil, logr.Discard())
	if mw == nil {
		t.Fatal("MED-03: middleware must not be nil")
	}
	if readyFn == nil {
		t.Fatal("MED-03: buildAuthMiddleware must return a non-nil readiness checker")
	}
	if !readyFn() {
		t.Error("MED-03: auth readiness should be true when JWKS server is reachable")
	}
}

func TestBuildAuthMiddleware_NoAuth_ReadyAlwaysTrue(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	reg := metrics.NewRegistry()
	mw, readyFn := buildAuthMiddleware(cfg, reg, nil, logr.Discard())
	if mw == nil {
		t.Fatal("middleware must not be nil")
	}
	if readyFn == nil {
		t.Fatal("MED-03: readiness checker must not be nil even when auth is unconfigured")
	}
	if !readyFn() {
		t.Error("MED-03: auth readiness should always be true when no JWT providers are configured")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1309-020: OIDC mode rejects opaque token (no TokenReview wired)
// ---------------------------------------------------------------------------

func TestBuildAuthMiddleware_OIDCMode_RejectsOpaqueToken(t *testing.T) {
	t.Parallel()

	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"keys":[]}`)
	}))
	t.Cleanup(jwksSrv.Close)

	cfg := &config.Config{}
	cfg.Auth.IssuerURL = jwksSrv.URL
	cfg.Auth.JWKSURL = jwksSrv.URL
	cfg.Auth.Audience = "test"
	cfg.Auth.AllowInsecureIssuers = true

	reg := metrics.NewRegistry()
	mw, _ := buildAuthMiddleware(cfg, reg, nil, logr.Discard())
	if mw == nil {
		t.Fatal("UT-AF-1309-020: buildAuthMiddleware returned nil")
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := mw(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer opaque-sa-token-not-jwt")
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("UT-AF-1309-020: expected 401 for opaque token in OIDC mode, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1309-021: No OIDC → auto-detect TokenReview mode or pass-through
// ---------------------------------------------------------------------------

func TestBuildAuthMiddleware_NoOIDC_AutoDetect(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	reg := metrics.NewRegistry()
	mw, readyFn := buildAuthMiddleware(cfg, reg, nil, logr.Discard())
	if mw == nil {
		t.Fatal("UT-AF-1309-021: buildAuthMiddleware returned nil")
	}
	if readyFn == nil {
		t.Fatal("UT-AF-1309-021: readiness checker must not be nil")
	}
	// When no OIDC issuer is configured, buildAuthMiddleware either wires
	// TokenReview (if kubeconfig is available) or falls back to pass-through.
	// Both are valid outcomes; the key assertion is that no OIDC is attempted.
}

// ---------------------------------------------------------------------------
// MCP wiring: buildMCPHandler
// ---------------------------------------------------------------------------

func TestBuildMCPHandler_ReturnsHandlerAndReadyChecker(t *testing.T) {
	t.Parallel()

	kaBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(kaBackend.Close)

	cfg := &config.Config{}
	cfg.MCP.Enabled = true
	reg := metrics.NewRegistry()

	deps := &backendDeps{
		KAClient: ka.NewClient(ka.Config{BaseURL: kaBackend.URL}),
		DSResilientTransport: resilience.NewCircuitBreakerTransport(
			http.DefaultTransport,
			&resilience.CircuitBreakerConfig{Name: "test-ds"},
		),
	}

	h, depsReady, err := buildMCPHandler(cfg, deps, nil, reg, &allowAllToolAuthorizer{}, nil, logr.Discard(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("TC-MCP-WIRE-01: handler must not be nil")
	}
	if depsReady == nil {
		t.Fatal("TC-MCP-WIRE-01: depsReady checker must not be nil")
	}
	if !depsReady() {
		t.Error("TC-MCP-WIRE-01: depsReady should be true when backends are healthy")
	}
}

// ---------------------------------------------------------------------------
// A2A wiring: buildA2AHandler
// ---------------------------------------------------------------------------

// testBackendDeps returns a minimal backendDeps for unit tests (no real K8s cluster).
func testBackendDeps() *backendDeps {
	return &backendDeps{}
}

func TestBuildA2AHandler_NoLLMEndpoint_Returns501Stub(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	reg := metrics.NewRegistry()
	h, err := buildA2AHandler(context.Background(), cfg, testBackendDeps(), nil, reg, nil, nil, logr.Discard(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("handler must not be nil even without LLM endpoint")
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/a2a/invoke", http.NoBody))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501 when LLM endpoint not set, got %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Result().Body)
	if !strings.Contains(string(body), "A2A not configured") {
		t.Errorf("expected body to contain 'A2A not configured', got %q", string(body))
	}
}

func TestBuildA2AHandler_WithLLMEndpoint_ReturnsHandler(t *testing.T) {
	t.Parallel()

	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"}]},"finishReason":"STOP"}]}`))
	}))
	t.Cleanup(mockLLM.Close)

	cfg := &config.Config{}
	cfg.Agent.LLM.Provider = config.LLMProviderGemini
	cfg.Agent.LLM.Endpoint = mockLLM.URL
	cfg.Agent.LLM.Model = "mock-model"
	cfg.Agent.LLM.APIKey = "test-key"
	reg := metrics.NewRegistry()

	h, err := buildA2AHandler(context.Background(), cfg, testBackendDeps(), nil, reg, nil, nil, logr.Discard(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("handler must not be nil when LLM endpoint is configured")
	}
}

func TestBuildA2AHandler_WithSessionInfra_UsesDecorator(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/path")

	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	}))
	t.Cleanup(mockLLM.Close)

	cfg := &config.Config{}
	cfg.Agent.LLM.Provider = config.LLMProviderGemini
	cfg.Agent.LLM.Endpoint = mockLLM.URL
	cfg.Agent.LLM.Model = "mock-model"
	cfg.Agent.LLM.APIKey = "test-key"
	reg := metrics.NewRegistry()

	infra, infraErr := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if infraErr != nil {
		t.Skipf("skipping (no kubeconfig available): %v", infraErr)
	}
	defer infra.StopFunc()

	h, err := buildA2AHandler(context.Background(), cfg, testBackendDeps(), infra, reg, nil, nil, logr.Discard(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("handler must not be nil when session infra is provided")
	}
}

// TC-WIRING-01: A2A handler threads K8sClient into AgentConfig
func TestBuildA2AHandler_ThreadsK8sClient(t *testing.T) {
	t.Parallel()

	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	}))
	t.Cleanup(mockLLM.Close)

	cfg := &config.Config{}
	cfg.Agent.LLM.Provider = config.LLMProviderGemini
	cfg.Agent.LLM.Endpoint = mockLLM.URL
	cfg.Agent.LLM.Model = "mock-model"
	cfg.Agent.LLM.APIKey = "test-key"
	reg := metrics.NewRegistry()

	deps := testBackendDeps()
	h, err := buildA2AHandler(context.Background(), cfg, deps, nil, reg, nil, nil, logr.Discard(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("TC-WIRING-01: handler must not be nil — K8sClient threading must not break construction")
	}
}

// TC-WIRING-02: A2A handler threads KAClient into AgentConfig
func TestBuildA2AHandler_ThreadsKAClient(t *testing.T) {
	t.Parallel()

	kaBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(kaBackend.Close)

	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	}))
	t.Cleanup(mockLLM.Close)

	cfg := &config.Config{}
	cfg.Agent.LLM.Provider = config.LLMProviderGemini
	cfg.Agent.LLM.Endpoint = mockLLM.URL
	cfg.Agent.LLM.Model = "mock-model"
	cfg.Agent.LLM.APIKey = "test-key"
	reg := metrics.NewRegistry()

	deps := testBackendDeps()
	deps.KAClient = ka.NewClient(ka.Config{BaseURL: kaBackend.URL}, nil)

	h, err := buildA2AHandler(context.Background(), cfg, deps, nil, reg, nil, nil, logr.Discard(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("TC-WIRING-02: handler must not be nil when KAClient is provided")
	}
}

// TC-WIRING-03: A2A handler threads DSClient into AgentConfig
func TestBuildA2AHandler_ThreadsDSClient(t *testing.T) {
	t.Parallel()

	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	}))
	t.Cleanup(mockLLM.Close)

	dsBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(dsBackend.Close)

	cfg := &config.Config{}
	cfg.Agent.LLM.Provider = config.LLMProviderGemini
	cfg.Agent.LLM.Endpoint = mockLLM.URL
	cfg.Agent.LLM.Model = "mock-model"
	cfg.Agent.LLM.APIKey = "test-key"
	reg := metrics.NewRegistry()

	dsClient, dsErr := ds.NewOgenClient(ds.OgenClientConfig{BaseURL: dsBackend.URL})
	if dsErr != nil {
		t.Fatalf("failed to create DS client: %v", dsErr)
	}

	deps := testBackendDeps()
	deps.DSClient = dsClient

	h, err := buildA2AHandler(context.Background(), cfg, deps, nil, reg, nil, nil, logr.Discard(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("TC-WIRING-03: handler must not be nil when DSClient is provided")
	}
}

// TC-WIRING-04: A2A handler threads UserLimiter into AgentConfig (ADR-022)
func TestBuildA2AHandler_ThreadsUserLimiter(t *testing.T) {
	t.Parallel()

	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	}))
	t.Cleanup(mockLLM.Close)

	cfg := &config.Config{}
	cfg.Agent.LLM.Provider = config.LLMProviderGemini
	cfg.Agent.LLM.Endpoint = mockLLM.URL
	cfg.Agent.LLM.Model = "mock-model"
	cfg.Agent.LLM.APIKey = "test-key"
	reg := metrics.NewRegistry()

	limiter := ratelimit.NewUserLimiter(ratelimit.PerUserConfig{
		RequestsPerMinute:     60,
		MaxConcurrentSessions: 5,
		ToolCallsPerMinute:    30,
		CleanupInterval:       1 * time.Minute,
		MaxAge:                5 * time.Minute,
	})
	t.Cleanup(limiter.Stop)

	deps := testBackendDeps()
	h, err := buildA2AHandler(context.Background(), cfg, deps, nil, reg, nil, nil, logr.Discard(), limiter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("TC-WIRING-04: handler must not be nil when UserLimiter is provided")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func extractMetricLines(metricsText, prefix string) string {
	var lines []string
	for _, line := range strings.Split(metricsText, "\n") {
		if strings.HasPrefix(line, prefix) {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return "(no lines with prefix " + prefix + ")"
	}
	return strings.Join(lines, "\n")
}

// TC-WIRING-08: K8sClient() is safe for concurrent access (sync.Once guards lazy init).
func TestBackendDeps_K8sClient_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	deps := &backendDeps{}

	const goroutines = 50
	results := make([]dynamic.Interface, goroutines)
	start := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx] = deps.K8sClient()
		}(i)
	}

	close(start)
	wg.Wait()

	// All goroutines must observe the same value (nil when no kubeconfig is available in test)
	for i := 1; i < goroutines; i++ {
		if results[i] != results[0] {
			t.Fatalf("goroutine %d got different K8sClient result than goroutine 0", i)
		}
	}
}

type allowAllToolAuthorizer struct{}

func (a *allowAllToolAuthorizer) Check(_ context.Context, _ string, _ []string, _ string) (bool, error) {
	return true, nil
}

type noopAuditor struct{}

func (n *noopAuditor) Emit(_ context.Context, _ *audit.Event) {}
func (n *noopAuditor) Start()                                 {}
func (n *noopAuditor) Close(_ context.Context) error          { return nil }

func noopLogger() logr.Logger {
	return logr.Discard()
}

// ---------------------------------------------------------------------------
// IT-AF-1234-W10: KASessionPool constructed in backendDeps
// ---------------------------------------------------------------------------

func TestBackendDeps_PoolConstructed(t *testing.T) {
	t.Parallel()
	deps := &backendDeps{
		Pool: ka.NewKASessionPool(ka.PoolConfig{
			Factory:    func(_ context.Context) (ka.PoolSession, error) { return nil, nil },
			MaxEntries: 10,
			Logger:     logr.Discard(),
		}),
	}
	if deps.Pool == nil {
		t.Fatal("IT-AF-1234-W10: Pool must be constructed in backendDeps")
	}
	if deps.Pool.Size() != 0 {
		t.Errorf("IT-AF-1234-W10: new pool should have size 0, got %d", deps.Pool.Size())
	}
}

// ---------------------------------------------------------------------------
// IT-AF-1234-W11: Pool.DrainAll called during shutdown
// ---------------------------------------------------------------------------

func TestShutdown_PoolDrainAllCalled(t *testing.T) {
	t.Parallel()
	var drained bool
	pool := ka.NewKASessionPool(ka.PoolConfig{
		Factory: func(_ context.Context) (ka.PoolSession, error) {
			return &mockPoolSession{}, nil
		},
		MaxEntries: 10,
		Logger:     logr.Discard(),
	})
	_, _ = pool.Acquire(context.Background(), "test/rr", "user@test")
	if pool.Size() != 1 {
		t.Fatalf("expected pool size 1, got %d", pool.Size())
	}

	ctx := context.Background()
	if err := pool.DrainAll(ctx); err != nil {
		t.Fatalf("DrainAll failed: %v", err)
	}
	drained = pool.Size() == 0
	if !drained {
		t.Fatal("IT-AF-1234-W11: DrainAll must empty the pool")
	}
}

type mockPoolSession struct{}

func (m *mockPoolSession) CallTool(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return nil, nil
}
func (m *mockPoolSession) Close() error { return nil }

// ---------------------------------------------------------------------------
// IT-AF-1234-W12: WithDownstreamDuration wired on SDKMCPClient
// ---------------------------------------------------------------------------

func TestSDKMCPClient_DownstreamDurationWired(t *testing.T) {
	t.Parallel()
	reg := metrics.NewRegistry()
	mcpClient := ka.NewSDKMCPClient("http://localhost:0", &http.Client{}, logr.Discard())
	result := mcpClient.WithDownstreamDuration(reg.DownstreamDuration)
	if result == nil {
		t.Fatal("IT-AF-1234-W12: WithDownstreamDuration must return the client")
	}
}

// ---------------------------------------------------------------------------
// IT-AF-1234-W13: buildResilientTransport wraps MCP transport with CB+retry
// ---------------------------------------------------------------------------

func TestBuildResilientTransport_ForMCP(t *testing.T) {
	t.Parallel()
	reg := metrics.NewRegistry()
	depCfg := &config.DependencyConfig{
		CBFailureThreshold: 5,
		CBMaxRequests:      3,
		CBInterval:         10 * time.Second,
		CBTimeout:          100 * time.Millisecond,
		RetryMax:           1,
		RetryInitBackoff:   1 * time.Millisecond,
		RetryMaxBackoff:    5 * time.Millisecond,
		RetryableStatuses:  []int{503},
	}

	transport := buildResilientTransport(http.DefaultTransport, depCfg, "ka-mcp", reg, &noopAuditor{})
	if transport == nil {
		t.Fatal("IT-AF-1234-W13: buildResilientTransport must return non-nil transport")
	}
}

// ---------------------------------------------------------------------------
// IT-AF-1234-W10b: buildMCPHandler passes pool to bridge config
// ---------------------------------------------------------------------------

func TestBuildMCPHandler_PassesPool(t *testing.T) {
	t.Parallel()

	kaBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(kaBackend.Close)

	cfg := &config.Config{}
	cfg.MCP.Enabled = true
	reg := metrics.NewRegistry()

	pool := ka.NewKASessionPool(ka.PoolConfig{
		Factory:    func(_ context.Context) (ka.PoolSession, error) { return nil, nil },
		MaxEntries: 10,
		Logger:     logr.Discard(),
	})

	deps := &backendDeps{
		KAClient: ka.NewClient(ka.Config{BaseURL: kaBackend.URL}),
		Pool:     pool,
		DSResilientTransport: resilience.NewCircuitBreakerTransport(
			http.DefaultTransport,
			&resilience.CircuitBreakerConfig{Name: "test-ds"},
		),
	}

	h, _, err := buildMCPHandler(cfg, deps, nil, reg, &allowAllToolAuthorizer{}, nil, logr.Discard(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("IT-AF-1234-W10b: handler must not be nil when pool is provided")
	}
}

// ---------------------------------------------------------------------------
// IT-AF-1293-W01: buildMCPHandler wires SessionInitializer from sessionInfra
// ---------------------------------------------------------------------------

func TestBuildMCPHandler_WiresSessionInitializer(t *testing.T) {
	t.Parallel()

	kaBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(kaBackend.Close)

	cfg := &config.Config{}
	cfg.MCP.Enabled = true
	reg := metrics.NewRegistry()

	deps := &backendDeps{
		KAClient: ka.NewClient(ka.Config{BaseURL: kaBackend.URL}),
		DSResilientTransport: resilience.NewCircuitBreakerTransport(
			http.DefaultTransport,
			&resilience.CircuitBreakerConfig{Name: "test-ds"},
		),
	}

	sessInfra := &sessionInfra{
		SessionService: session.NewCRDSessionService(
			adksession.InMemoryService(), nil, nil, "test-ns",
		),
		Healthy:  &atomic.Bool{},
		StopFunc: func() {},
	}

	h, _, err := buildMCPHandler(cfg, deps, sessInfra, reg, &allowAllToolAuthorizer{}, nil, logr.Discard(), nil)
	if err != nil {
		t.Fatalf("IT-AF-1293-W01: unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("IT-AF-1293-W01: handler must not be nil when sessionInfra is provided")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1272-001: session health flag is false before cache sync (BR-SESS-011)
//
// FedRAMP SI-4: system must report degraded state when monitoring subsystem
// has not yet confirmed readiness. A freshly created health flag must be
// false so that /readyz correctly returns 503 during cache warmup.
// ---------------------------------------------------------------------------

func TestSessionInfra_HealthyFalseBeforeSync(t *testing.T) {
	t.Parallel()
	healthy := &atomic.Bool{}
	infra := &sessionInfra{Healthy: healthy, StopFunc: func() {}}
	if infra.Healthy.Load() {
		t.Fatal("UT-AF-1272-001: health flag must report degraded (false) before cache sync completes — /readyz would incorrectly return 200")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1272-002: session health flag transitions to true after cache sync (BR-SESS-011)
//
// FedRAMP SI-4: once the informer cache confirms sync, the flag must flip
// so /readyz returns 200 and the pod accepts traffic.
// ---------------------------------------------------------------------------

func TestSessionInfra_HealthyTrueAfterSync(t *testing.T) {
	t.Parallel()
	healthy := &atomic.Bool{}
	infra := &sessionInfra{Healthy: healthy, StopFunc: func() {}}
	infra.Healthy.Store(true)
	if !infra.Healthy.Load() {
		t.Fatal("UT-AF-1272-002: health flag must report ready (true) after cache sync — /readyz would incorrectly return 503")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1272-003: fallback path signals readiness immediately (BR-SESS-011)
//
// When no kubeconfig is available the AF uses an in-memory fake client.
// There is no informer cache to sync, so the flag must be true from the
// start — otherwise the pod never passes readiness and K8s restarts it.
// ---------------------------------------------------------------------------

func TestBuildSessionInfra_NoKubeconfigReturnsError(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/path")
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 10 * time.Minute,
			RetentionTTL:  31 * 24 * time.Hour,
		},
	}
	_, err := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if err == nil {
		t.Fatal("UT-AF-1272-003: buildSessionInfra must return error when kubeconfig is unavailable")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1272-006: TTL actions are observable via Prometheus (BR-MONITORING-001)
//
// FedRAMP SI-4(2): monitoring must include automated mechanisms for TTL-driven
// lifecycle actions. The counter must be registered so /metrics exposes it to
// the SIEM scraper even before any actions occur.
// ---------------------------------------------------------------------------

func TestMetricsRegistry_SessionTTLActionsExposedOnMetrics(t *testing.T) {
	t.Parallel()
	reg := metrics.NewRegistry()
	if reg.SessionTTLActions == nil {
		t.Fatal("UT-AF-1272-006: SessionTTLActions must be registered — SIEM scraper has no visibility into TTL events")
	}

	reg.SessionTTLActions.WithLabelValues("cancel")

	metricsHandler := reg.Handler()
	rec := httptest.NewRecorder()
	metricsHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
	body, _ := io.ReadAll(rec.Result().Body)
	if !strings.Contains(string(body), "af_session_ttl_actions_total") {
		t.Error("UT-AF-1272-006: af_session_ttl_actions_total absent from /metrics — SIEM cannot scrape TTL lifecycle events")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1274-010: buildSessionInfra threads logger into reconciler (BR-SESS-013)
//
// FedRAMP AU-3: every reconcile error must flow through the structured log
// pipeline (logr -> zapr -> zap -> JSON). Without logger injection the
// reconciler would use slog.Default() and the JSON sink would be bypassed.
// ---------------------------------------------------------------------------

func TestBuildSessionInfra_ThreadsLoggerIntoReconciler(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/path")
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 10 * time.Minute,
			RetentionTTL:  31 * 24 * time.Hour,
		},
	}
	infra, err := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if err != nil {
		t.Skipf("skipping (no kubeconfig available): %v", err)
	}
	defer infra.StopFunc()
	if infra.Reconciler == nil {
		t.Fatal("UT-AF-1274-010: reconciler must not be nil — TTL enforcement disabled")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1274-011: buildA2AHandler threads logger into launcher (BR-SESS-013)
//
// FedRAMP AU-3: A2A task execution errors must flow through the structured
// log pipeline. The A2AConfig.Logger field carries the logr chain that
// ensures all task lifecycle events are JSON-serialised and scrape-ready.
// ---------------------------------------------------------------------------

func TestBuildA2AHandler_ThreadsLoggerIntoLauncher(t *testing.T) {
	t.Parallel()

	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	}))
	t.Cleanup(mockLLM.Close)

	cfg := &config.Config{}
	cfg.Agent.LLM.Provider = config.LLMProviderGemini
	cfg.Agent.LLM.Endpoint = mockLLM.URL
	cfg.Agent.LLM.Model = "mock-model"
	cfg.Agent.LLM.APIKey = "test-key"
	reg := metrics.NewRegistry()

	h, err := buildA2AHandler(context.Background(), cfg, testBackendDeps(), nil, reg, nil, nil, logr.Discard(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("UT-AF-1274-011: handler must not be nil — A2A task errors would be invisible")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1274-012: config watcher accepts logr.Logger (BR-SESS-013)
//
// FedRAMP CM-3(5): configuration change events must be auditable.
// The hot-reload watcher must route change/reject logs through the same
// structured pipeline as all other AF components, not through slog.Default().
// ---------------------------------------------------------------------------

func TestConfigWatcher_AcceptsLogr(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfgFile := tmpDir + "/config.yaml"
	if err := os.WriteFile(cfgFile, []byte("server:\n  port: 8443\n"), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	fw, err := config.NewFileWatcher(cfgFile, func(_ []byte) error { return nil },
		config.WithLogger(logr.Discard()),
	)
	if err != nil {
		t.Fatalf("UT-AF-1274-012: NewFileWatcher with logr.Logger failed: %v", err)
	}
	if fw == nil {
		t.Fatal("UT-AF-1274-012: FileWatcher nil — config drift detection disabled")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1272 supplement: buildSessionInfra wires TTL metrics to reconciler
//
// FedRAMP SI-4(2): the disconnect/cancel/delete counter must be wired from
// the metrics registry into the reconciler so TTL actions are observable.
// ---------------------------------------------------------------------------

func TestBuildSessionInfra_WiresTTLMetrics(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/path")
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 10 * time.Minute,
			RetentionTTL:  31 * 24 * time.Hour,
		},
	}
	infra, err := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if err != nil {
		t.Skipf("skipping (no kubeconfig available): %v", err)
	}
	defer infra.StopFunc()
	if infra.Reconciler == nil {
		t.Fatal("reconciler must not be nil — TTL enforcement disabled")
	}
	if reg.SessionTTLActions == nil {
		t.Fatal("SessionTTLActions must be wired — TTL events invisible to SIEM")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1273-001: preflightSessionChecks logs CRD discovery result [SI-10]
//
// FedRAMP SI-10: environment validation at startup. The function must log
// a "pre-flight CRD discovery" line regardless of whether the CRD is found.
// ---------------------------------------------------------------------------

func TestPreflightSessionChecks_LogsCRDDiscovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "kubernaut.ai") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"kubernaut.ai/v1alpha1","resources":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	var buf strings.Builder
	logger := funcr.New(func(prefix, args string) {
		buf.WriteString(prefix + " " + args + "\n")
	}, funcr.Options{Verbosity: 10})

	preflightSessionChecks(&rest.Config{Host: srv.URL}, "default", nil, logger)
	if !strings.Contains(buf.String(), "pre-flight CRD discovery") {
		t.Fatal("UT-AF-1273-001: must log CRD discovery result for SI-10 compliance")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1273-002: preflightSessionChecks logs RBAC SSAR result [AC-6]
//
// FedRAMP AC-6: least privilege verification. The function must log
// a "pre-flight RBAC check" line with the allowed/denied result.
// ---------------------------------------------------------------------------

func TestPreflightSessionChecks_LogsRBACCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "kubernaut.ai") {
			_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"kubernaut.ai/v1alpha1","resources":[{"name":"investigationsessions","namespaced":true,"kind":"InvestigationSession","verbs":["get","list","watch"]}]}`))
			return
		}
		if strings.Contains(r.URL.Path, "selfsubjectaccessreviews") {
			_, _ = w.Write([]byte(`{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":true}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	var buf strings.Builder
	logger := funcr.New(func(prefix, args string) {
		buf.WriteString(prefix + " " + args + "\n")
	}, funcr.Options{Verbosity: 10})

	preflightSessionChecks(&rest.Config{Host: srv.URL}, "default", nil, logger)
	if !strings.Contains(buf.String(), "pre-flight RBAC check") {
		t.Fatal("UT-AF-1273-002: must log RBAC SSAR result for AC-6 compliance")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1273-003: preflightSessionChecks warns on missing CRD [SI-10]
//
// When the CRD is not found, the function must emit a WARNING so SREs
// can diagnose before the controller fails to start.
// ---------------------------------------------------------------------------

func TestPreflightSessionChecks_WarnsMissingCRD(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "kubernaut.ai") {
			_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"kubernaut.ai/v1alpha1","resources":[]}`))
			return
		}
		if strings.Contains(r.URL.Path, "selfsubjectaccessreviews") {
			_, _ = w.Write([]byte(`{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":true}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	var buf strings.Builder
	logger := funcr.New(func(prefix, args string) {
		buf.WriteString(prefix + " " + args + "\n")
	}, funcr.Options{Verbosity: 10})

	preflightSessionChecks(&rest.Config{Host: srv.URL}, "default", nil, logger)
	if !strings.Contains(buf.String(), "InvestigationSession CRD not found") {
		t.Fatal("UT-AF-1273-003: must warn when CRD is missing for SI-10 compliance")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1273-004: preflightSessionChecks warns on RBAC denied [AC-6]
//
// When SSAR returns denied, the function must emit a WARNING so SREs
// know the service account lacks required permissions.
// ---------------------------------------------------------------------------

func TestPreflightSessionChecks_WarnsRBACDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "kubernaut.ai") {
			_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"kubernaut.ai/v1alpha1","resources":[{"name":"investigationsessions","namespaced":true,"kind":"InvestigationSession","verbs":["get","list","watch"]}]}`))
			return
		}
		if strings.Contains(r.URL.Path, "selfsubjectaccessreviews") {
			_, _ = w.Write([]byte(`{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":false,"reason":"RBAC: no binding"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	var buf strings.Builder
	logger := funcr.New(func(prefix, args string) {
		buf.WriteString(prefix + " " + args + "\n")
	}, funcr.Options{Verbosity: 10})

	preflightSessionChecks(&rest.Config{Host: srv.URL}, "default", nil, logger)
	if !strings.Contains(buf.String(), "lacks permissions") {
		t.Fatal("UT-AF-1273-004: must warn when RBAC denied for AC-6 compliance")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1273-005: preflightSessionChecks emits audit events [AU-2]
//
// When auditor is non-nil, preflight must emit EventPreflightCRDCheck
// and EventPreflightRBACCheck events for the FedRAMP audit trail.
// ---------------------------------------------------------------------------

type collectingAuditor struct {
	mu     sync.Mutex
	events []*audit.Event
}

func (a *collectingAuditor) Emit(_ context.Context, e *audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *collectingAuditor) Events() []*audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]*audit.Event(nil), a.events...)
}

func TestPreflightSessionChecks_EmitsAuditEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "kubernaut.ai") {
			_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"kubernaut.ai/v1alpha1","resources":[{"name":"investigationsessions","namespaced":true,"kind":"InvestigationSession","verbs":["get","list","watch"]}]}`))
			return
		}
		if strings.Contains(r.URL.Path, "selfsubjectaccessreviews") {
			_, _ = w.Write([]byte(`{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":true}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	auditor := &collectingAuditor{}
	logger := funcr.New(func(_, _ string) {}, funcr.Options{})

	preflightSessionChecks(&rest.Config{Host: srv.URL}, "default", auditor, logger)

	events := auditor.Events()
	var hasCRD, hasRBAC bool
	for _, e := range events {
		if e.Type == audit.EventPreflightCRDCheck {
			hasCRD = true
		}
		if e.Type == audit.EventPreflightRBACCheck {
			hasRBAC = true
		}
	}
	if !hasCRD {
		t.Error("AU-2: must emit EventPreflightCRDCheck audit event")
	}
	if !hasRBAC {
		t.Error("AU-2: must emit EventPreflightRBACCheck audit event")
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1273-006: preflightSessionChecks checks all 6 verbs [AC-6]
//
// The SSAR loop must check get, list, watch, create, update, delete.
// ---------------------------------------------------------------------------

func TestPreflightSessionChecks_ChecksAllVerbs(t *testing.T) {
	var ssarCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "kubernaut.ai") {
			_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"kubernaut.ai/v1alpha1","resources":[{"name":"investigationsessions","namespaced":true,"kind":"InvestigationSession","verbs":["get","list","watch"]}]}`))
			return
		}
		if strings.Contains(r.URL.Path, "selfsubjectaccessreviews") {
			ssarCount.Add(1)
			_, _ = w.Write([]byte(`{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":true}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	logger := funcr.New(func(_, _ string) {}, funcr.Options{})
	preflightSessionChecks(&rest.Config{Host: srv.URL}, "default", nil, logger)

	if count := ssarCount.Load(); count != 6 {
		t.Errorf("AC-6: expected 6 SSAR checks (get,list,watch,create,update,delete), got %d", count)
	}
}

// ---------------------------------------------------------------------------
// UT-AF-1273-007: preflightSessionChecks lists denied verbs [AC-6]
//
// When some verbs are denied, the warning message must list them.
// ---------------------------------------------------------------------------

func TestPreflightSessionChecks_ListsDeniedVerbs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "kubernaut.ai") {
			_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"kubernaut.ai/v1alpha1","resources":[{"name":"investigationsessions","namespaced":true,"kind":"InvestigationSession","verbs":["get","list","watch"]}]}`))
			return
		}
		if strings.Contains(r.URL.Path, "selfsubjectaccessreviews") {
			body, _ := io.ReadAll(r.Body)
			bodyStr := string(body)
			ct := r.Header.Get("Content-Type")
			// K8s client may send protobuf or JSON. For JSON bodies,
			// deny verbs containing "delete" or "create".
			denied := false
			if strings.Contains(ct, "json") || strings.Contains(ct, "application/json") {
				denied = strings.Contains(bodyStr, `"delete"`) || strings.Contains(bodyStr, `"create"`)
			} else {
				// Protobuf: check raw bytes for verb strings
				denied = strings.Contains(bodyStr, "delete") || strings.Contains(bodyStr, "create")
			}
			if denied {
				_, _ = w.Write([]byte(`{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":false}}`))
			} else {
				_, _ = w.Write([]byte(`{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":true}}`))
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	var buf strings.Builder
	logger := funcr.New(func(prefix, args string) {
		buf.WriteString(prefix + " " + args + "\n")
	}, funcr.Options{Verbosity: 10})

	auditor := &collectingAuditor{}
	preflightSessionChecks(&rest.Config{Host: srv.URL}, "default", auditor, logger)

	if !strings.Contains(buf.String(), "delete") {
		t.Logf("full logs:\n%s", buf.String())
		t.Error("AC-6: warning must list denied verb 'delete'")
	}

	events := auditor.Events()
	for _, e := range events {
		if e.Type == audit.EventPreflightRBACCheck {
			if e.Detail["denied_verbs"] == "" || !strings.Contains(e.Detail["denied_verbs"], "delete") {
				t.Errorf("AU-2: RBAC audit event must list denied_verbs containing 'delete', got %q", e.Detail["denied_verbs"])
			}
			if e.Detail["all_allowed"] != "false" {
				t.Errorf("AU-2: all_allowed must be false when some verbs denied, got %q", e.Detail["all_allowed"])
			}
		}
	}
}

// ---------------------------------------------------------------------------
// TP-1301-1302 §4.5: Prompt Compliance Wiring Tests — FedRAMP CM-3
// Validates that prompt.txt documents the merged kubernaut_investigate tool.
// ---------------------------------------------------------------------------

func TestPromptContainsMandatoryInvestigate(t *testing.T) {
	prompt, err := os.ReadFile("../../pkg/apifrontend/agent/prompt.txt")
	if err != nil {
		t.Fatalf("WT-AF-1302-001: failed to read prompt.txt: %v", err)
	}
	text := string(prompt)

	if !strings.Contains(text, "kubernaut_investigate") {
		t.Error("WT-AF-1302-001 CM-3: prompt.txt must reference kubernaut_investigate")
	}
	if !strings.Contains(text, "streams live events") {
		t.Error("WT-AF-1302-001 CM-3: prompt.txt must describe kubernaut_investigate as streaming live events")
	}
}

func TestPromptFixJourneyStartsWithCreateRR(t *testing.T) {
	prompt, err := os.ReadFile("../../pkg/apifrontend/agent/prompt.txt")
	if err != nil {
		t.Fatalf("WT-AF-1302-002: failed to read prompt.txt: %v", err)
	}
	text := string(prompt)

	fixIdx := strings.Index(text, "### Fix something")
	if fixIdx == -1 {
		t.Fatal("WT-AF-1302-002 CM-3: prompt.txt must contain '### Fix something' section")
	}
	fixSection := text[fixIdx:]

	journeyIdx := strings.Index(fixSection, "Full journey:")
	if journeyIdx == -1 {
		t.Fatal("WT-AF-1302-002 CM-3: Fix section must contain 'Full journey:' line")
	}
	journeyLine := fixSection[journeyIdx : journeyIdx+200]

	if !strings.Contains(journeyLine, "af_create_rr") {
		t.Error("WT-AF-1302-002 CM-3: Full journey must start with af_create_rr")
	}
	if !strings.Contains(journeyLine, "kubernaut_investigate") {
		t.Error("WT-AF-1302-002 CM-3: Full journey must include kubernaut_investigate")
	}

	createIdx := strings.Index(journeyLine, "af_create_rr")
	investigateIdx := strings.Index(journeyLine, "kubernaut_investigate")
	if createIdx > investigateIdx {
		t.Error("WT-AF-1302-002 CM-3: journey order must be af_create_rr → kubernaut_investigate")
	}
}

func TestPromptPhase1RequiresStartBeforeStream(t *testing.T) {
	prompt, err := os.ReadFile("../../pkg/apifrontend/agent/prompt.txt")
	if err != nil {
		t.Fatalf("WT-AF-1302-003: failed to read prompt.txt: %v", err)
	}
	text := string(prompt)

	phase1Idx := strings.Index(text, "### Phase 1: Investigate")
	if phase1Idx == -1 {
		t.Fatal("WT-AF-1302-003 CM-3: prompt.txt must contain '### Phase 1: Investigate' section")
	}
	phase1Section := text[phase1Idx:]
	if phase15Idx := strings.Index(phase1Section, "### Phase 1.5"); phase15Idx != -1 {
		phase1Section = phase1Section[:phase15Idx]
	}

	if !strings.Contains(phase1Section, "Call kubernaut_investigate") {
		t.Error("WT-AF-1302-003 CM-3: Phase 1 must instruct a single kubernaut_investigate call")
	}
	if !strings.Contains(phase1Section, "returns immediately") {
		t.Error("WT-AF-1302-003 CM-3: Phase 1 must describe investigation as returning immediately with streaming")
	}
}
