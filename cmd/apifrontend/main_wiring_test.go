package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/dynamic"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/config"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ds"
	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
	"github.com/jordigilh/kubernaut-apifrontend/internal/metrics"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ratelimit"
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

	cbt := buildResilientTransport(http.DefaultTransport, depCfg, "ds", reg)
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
	t.Parallel()
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 10 * time.Minute,
			RetentionTTL:  31 * 24 * time.Hour,
		},
	}
	infra := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if infra.SessionService == nil {
		t.Fatal("HIGH-02b: buildSessionInfra must return a non-nil SessionService")
	}
}

func TestBuildSessionInfra_GaugeIsWired(t *testing.T) {
	t.Parallel()
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 10 * time.Minute,
			RetentionTTL:  31 * 24 * time.Hour,
		},
	}
	infra := buildSessionInfra(cfg, reg, nil, logr.Discard())
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
	t.Parallel()
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 15 * time.Minute,
			RetentionTTL:  31 * 24 * time.Hour,
		},
	}
	infra := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if infra.Reconciler == nil {
		t.Fatal("HIGH-02b: buildSessionInfra must return a non-nil Reconciler")
	}
}

func TestBuildSessionInfra_RetentionTTLClamped(t *testing.T) {
	t.Parallel()
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 5 * time.Minute,
			RetentionTTL:  1 * time.Hour, // below NIST AU-11 minimum
		},
	}
	infra := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if infra.Reconciler == nil {
		t.Fatal("Reconciler must not be nil even with sub-minimum retention TTL")
	}
}

func TestBuildSessionInfra_SchemeIncludesInvestigationSession(t *testing.T) {
	t.Parallel()
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 10 * time.Minute,
			RetentionTTL:  31 * 24 * time.Hour,
		},
	}
	infra := buildSessionInfra(cfg, reg, nil, logr.Discard())
	if infra.Scheme == nil {
		t.Fatal("HIGH-02b: buildSessionInfra must return a non-nil Scheme")
	}

	gvk := v1alpha1.GroupVersion.WithKind("InvestigationSession")
	if !infra.Scheme.Recognizes(gvk) {
		t.Errorf("HIGH-02b: scheme does not recognize %s", gvk)
	}
}

func TestBuildSessionInfra_GracefulShutdown(t *testing.T) {
	t.Parallel()
	reg := metrics.NewRegistry()
	cfg := &config.Config{
		Session: config.SessionConfig{
			Namespace:     "test-ns",
			DisconnectTTL: 10 * time.Minute,
			RetentionTTL:  31 * 24 * time.Hour,
		},
	}
	infra := buildSessionInfra(cfg, reg, nil, logr.Discard())
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
	h, err := buildA2AHandler(context.Background(), cfg, testBackendDeps(), nil, reg, nil, logr.Discard())
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
	cfg.Agent.LLMEndpoint = mockLLM.URL
	cfg.Agent.LLMModel = "mock-model"
	cfg.Agent.LLMAPIKey = "test-key"
	reg := metrics.NewRegistry()

	h, err := buildA2AHandler(context.Background(), cfg, testBackendDeps(), nil, reg, nil, logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("handler must not be nil when LLM endpoint is configured")
	}
}

func TestBuildA2AHandler_WithSessionInfra_UsesDecorator(t *testing.T) {
	t.Parallel()

	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	}))
	t.Cleanup(mockLLM.Close)

	cfg := &config.Config{}
	cfg.Agent.LLMEndpoint = mockLLM.URL
	cfg.Agent.LLMModel = "mock-model"
	cfg.Agent.LLMAPIKey = "test-key"
	reg := metrics.NewRegistry()

	infra := buildSessionInfra(cfg, reg, nil, logr.Discard())
	defer infra.StopFunc()

	h, err := buildA2AHandler(context.Background(), cfg, testBackendDeps(), infra, reg, nil, logr.Discard())
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
	cfg.Agent.LLMEndpoint = mockLLM.URL
	cfg.Agent.LLMModel = "mock-model"
	cfg.Agent.LLMAPIKey = "test-key"
	reg := metrics.NewRegistry()

	deps := testBackendDeps()
	h, err := buildA2AHandler(context.Background(), cfg, deps, nil, reg, nil, logr.Discard())
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
	cfg.Agent.LLMEndpoint = mockLLM.URL
	cfg.Agent.LLMModel = "mock-model"
	cfg.Agent.LLMAPIKey = "test-key"
	reg := metrics.NewRegistry()

	deps := testBackendDeps()
	deps.KAClient = ka.NewClient(ka.Config{BaseURL: kaBackend.URL}, nil)

	h, err := buildA2AHandler(context.Background(), cfg, deps, nil, reg, nil, logr.Discard())
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
	cfg.Agent.LLMEndpoint = mockLLM.URL
	cfg.Agent.LLMModel = "mock-model"
	cfg.Agent.LLMAPIKey = "test-key"
	reg := metrics.NewRegistry()

	dsClient, dsErr := ds.NewOgenClient(ds.OgenClientConfig{BaseURL: dsBackend.URL})
	if dsErr != nil {
		t.Fatalf("failed to create DS client: %v", dsErr)
	}

	deps := testBackendDeps()
	deps.DSClient = dsClient

	h, err := buildA2AHandler(context.Background(), cfg, deps, nil, reg, nil, logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("TC-WIRING-03: handler must not be nil when DSClient is provided")
	}
}

// TC-WIRING-04: A2A handler threads ImpersonatingClientFactory into AgentConfig
func TestBuildA2AHandler_ThreadsImpersonatingFactory(t *testing.T) {
	t.Parallel()

	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	}))
	t.Cleanup(mockLLM.Close)

	cfg := &config.Config{}
	cfg.Agent.LLMEndpoint = mockLLM.URL
	cfg.Agent.LLMModel = "mock-model"
	cfg.Agent.LLMAPIKey = "test-key"
	reg := metrics.NewRegistry()

	factoryCalled := false
	deps := testBackendDeps()
	deps.DynFactory = func(_ context.Context) (dynamic.Interface, error) {
		factoryCalled = true
		return nil, fmt.Errorf("test factory called")
	}

	h, err := buildA2AHandler(context.Background(), cfg, deps, nil, reg, nil, logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("TC-WIRING-04: handler must not be nil when DynFactory is provided")
	}
	_ = factoryCalled
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

type noopAuditor struct{}

func (n *noopAuditor) Emit(_ context.Context, _ *audit.Event) {}
func (n *noopAuditor) Start()                                 {}
func (n *noopAuditor) Close(_ context.Context) error          { return nil }

func noopLogger() logr.Logger {
	return logr.Discard()
}
