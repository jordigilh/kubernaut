package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ratelimit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/streaming"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tlswiring"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
)

const configPath = "/etc/apifrontend/config.yaml"

func main() { os.Exit(run()) }

func run() int {
	cfg, err := loadConfig()
	if err != nil {
		z, _ := zap.NewProduction()
		z.Error("failed to load config", zap.Error(err))
		return 1
	}
	origPort := cfg.Server.Port
	config.ApplyPortEnvOverride(cfg)
	if err := cfg.ResolveDefaults(); err != nil {
		z, _ := zap.NewProduction()
		z.Error("failed to resolve config defaults", zap.Error(err))
		return 1
	}

	logLevel, _ := parseLogLevel(cfg.Logging.Level)
	zapLogger := newZapLogger(logLevel)
	defer func() { _ = zapLogger.Sync() }()
	logger := zapr.NewLogger(zapLogger).WithName("apifrontend")
	ctrl.SetLogger(logger.WithName("controller-runtime"))

	if cfg.Server.Port != origPort {
		logger.Info("PORT env override applied", "original", origPort, "effective", cfg.Server.Port)
	} else if p := os.Getenv("PORT"); p != "" {
		logger.Info("PORT env var ignored (invalid or out-of-range)", "value", p)
	}

	if err := cfg.Validate(); err != nil {
		logger.Error(err, "invalid config")
		return 1
	}

	cfg.Session.Namespace = agentpkg.ResolveNamespace(cfg.Session.Namespace, agentpkg.DefaultNamespaceFile)
	logger.Info("operational namespace resolved", "namespace", cfg.Session.Namespace)

	if cfg.Interactive.AwaitSessionTimeout > 0 {
		tools.AwaitSessionTimeout = cfg.Interactive.AwaitSessionTimeout
	}
	if cfg.Interactive.BridgeInactivityTimeout > 0 {
		tools.BridgeInactivityTimeout = cfg.Interactive.BridgeInactivityTimeout
	}

	restCfg, err := ctrl.GetConfig()
	if err != nil {
		logger.Error(err, "failed to get in-cluster config for SAR client")
		return 1
	}
	k8sClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		logger.Error(err, "failed to create kubernetes client for SAR")
		return 1
	}
	sarChecker := auth.NewSARChecker(k8sClient, cfg.RBAC.SARCacheTTL, logger.WithName("sar"))

	metricsReg := metrics.NewRegistry()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Issue #1156: Wire audit to shared pkg/audit.BufferedAuditStore + StoreAdapter.
	var auditDSTransport http.RoundTripper
	if cfg.Agent.DSBaseURL != "" {
		transport, auditDSWatcher, err := tlswiring.CAReloadableTransport(cfg.Agent.DSTLSCaFile, logger.WithName("ds-audit-ca"))
		if err != nil {
			logger.Error(err, "DS audit CA transport failed — refusing to start with broken TLS")
			return 1
		}
		if auditDSWatcher != nil {
			if err := auditDSWatcher.Start(ctx); err != nil {
				logger.Error(err, "DS audit CA watcher failed to start")
				return 1
			}
			defer auditDSWatcher.Stop()
		}
		auditDSTransport = transport
		if cfg.Agent.DSBearerTokenFile != "" {
			auditDSTransport = &bearerTokenTransport{
				base:      transport,
				tokenFile: cfg.Agent.DSBearerTokenFile,
			}
		}
	}
	dsAuditClient, err := sharedaudit.NewOpenAPIClientAdapterWithTransport(
		cfg.Agent.DSBaseURL, cfg.Resilience.DS.RequestTimeout, auditDSTransport)
	if err != nil {
		logger.Error(err, "DS audit client creation failed — refusing to start")
		return 1
	}
	auditStore, err := sharedaudit.NewBufferedStore(dsAuditClient, sharedaudit.DefaultConfig(), "apifrontend", logger)
	if err != nil {
		logger.Error(err, "failed to create buffered audit store")
		return 1
	}
	auditor := audit.NewStoreAdapter(auditStore, logger)
	logger.Info("audit trail wired to shared BufferedAuditStore", "dsURL", cfg.Agent.DSBaseURL)

	// F-005 + SM-01/SM-02/CFG-01: Wire all rate limit config fields.
	rlCfg := ratelimit.DefaultConfig()
	rlCfg.PerIP.RequestsPerSecond = float64(cfg.RateLimit.IPRequestsPerSec)
	rlCfg.PerIP.Burst = cfg.RateLimit.IPRequestsPerSec * 2
	if cfg.RateLimit.UserRequestsPerSec > 0 {
		rlCfg.PerUser.RequestsPerMinute = cfg.RateLimit.UserRequestsPerSec * 60
	}
	if cfg.RateLimit.MaxConcurrentSessions > 0 {
		rlCfg.PerUser.MaxConcurrentSessions = cfg.RateLimit.MaxConcurrentSessions
	}
	if cfg.RateLimit.ToolCallsPerMinute > 0 {
		rlCfg.PerUser.ToolCallsPerMinute = cfg.RateLimit.ToolCallsPerMinute
	}
	ipLimiter := ratelimit.NewIPLimiter(rlCfg.PerIP)
	userLimiter := ratelimit.NewUserLimiter(rlCfg.PerUser)

	sessInfra, err := buildSessionInfra(cfg, metricsReg, auditor, logger)
	if err != nil {
		logger.Error(err, "session infrastructure failed to initialize")
		return 1
	}

	deps, err := buildBackendDeps(ctx, cfg, metricsReg, auditor, logger)
	if err != nil {
		logger.Error(err, "failed to create backend dependencies")
		return 1
	}
	defer func() {
		for _, w := range deps.CAWatchers {
			w.watcher.Stop()
		}
	}()

	// AF-HIGH-2: Schedule periodic idle session eviction to prevent pool growth.
	evictStop := make(chan struct{})
	if deps.Pool != nil {
		go func() {
			ticker := time.NewTicker(2 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if n := deps.Pool.EvictIdle(); n > 0 {
						logger.V(1).Info("evicted idle KA sessions", "count", n)
					}
				case <-evictStop:
					return
				}
			}
		}()
	}

	hDeps := &handlerDeps{
		Cfg:               cfg,
		Backends:          deps,
		SessInfra:         sessInfra,
		MetricsReg:        metricsReg,
		Authorizer:        sarChecker,
		Auditor:           auditor,
		Logger:            logger,
		UserLimiter:       userLimiter,
		ActiveCtxRegistry: launcher.NewActiveContextRegistry(launcher.DefaultRegistryTTL, launcher.DefaultRegistryIdleTimeout),
	}

	mcpHandler, depsReady, err := buildMCPHandler(hDeps)
	if err != nil {
		logger.Error(err, "failed to create MCP handler")
		return 1
	}

	// CM-02: Wire config file watcher for drift detection + audit trail.
	cfgWatcher, err := config.NewFileWatcher(configPath, func(newContent []byte) error {
		var newCfg config.Config
		if err := yaml.Unmarshal(newContent, &newCfg); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
		if err := newCfg.ResolveDefaults(); err != nil {
			return fmt.Errorf("resolve defaults: %w", err)
		}
		return newCfg.Validate()
	}, config.WithLogger(logger.WithName("config-watcher")), config.WithAuditor(auditor))
	if err != nil {
		logger.Info("config file watcher unavailable", "error", err)
	} else {
		if err := cfgWatcher.Start(ctx); err != nil {
			logger.Info("config file watcher start failed", "error", err)
		} else {
			defer cfgWatcher.Stop()
		}
	}

	agentCardBase, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
		Name:        "Kubernaut Agent",
		Description: "Kubernaut AI-driven remediation agent",
		URL:         cfg.AgentCard.URL,
		Version:     version(),
		Skills:      handler.DefaultAgentSkills(cfg.Interactive.Enabled),
	})
	if err != nil {
		logger.Error(err, "failed to create agent card handler")
		return 1
	}
	agentCardHandler := handler.WithAgentCardAudit(agentCardBase, auditor)

	a2aHandler, err := buildA2AHandler(ctx, hDeps)
	if err != nil {
		logger.Error(err, "failed to create A2A handler")
		return 1
	}

	// F-001: Wire JWT auth middleware (fall back to noop only when auth is unconfigured).
	authMiddleware, authReady := buildAuthMiddleware(cfg, metricsReg, auditor, logger)
	preAuthMW := ratelimit.PreAuthMiddlewareWithConfig(ratelimit.PreAuthMiddlewareConfig{
		Limiter: ipLimiter,
		Auditor: auditor,
		Metrics: metricsReg.RateLimitDenied,
	})
	postAuthMW := ratelimit.PostAuthMiddlewareWithConfig(ratelimit.PostAuthMiddlewareConfig{
		Limiter: userLimiter,
		Auditor: auditor,
		Metrics: metricsReg.RateLimitDenied,
	})

	draining := &atomic.Bool{}
	rs, stopTLSWatchers, err := buildRouterAndServers(ctx, routerBuildParams{
		HDeps:            hDeps,
		MCPHandler:       mcpHandler,
		DepsReady:        depsReady,
		AgentCardHandler: agentCardHandler,
		A2AHandler:       a2aHandler,
		AuthMiddleware:   authMiddleware,
		AuthReady:        authReady,
		PreAuthMW:        preAuthMW,
		PostAuthMW:       postAuthMW,
		Draining:         draining,
	})
	if err != nil {
		// buildRouterAndServers already logged the specific failure.
		return 1
	}
	defer stopTLSWatchers()
	routerCfg := rs.RouterCfg
	httpServer := rs.HTTPServer
	healthServer := rs.HealthServer
	metricsServer := rs.MetricsServer

	go startServerTLS(httpServer, rs.TLSEnabled, "API", logger)
	go startServer(healthServer, "health", logger)
	go startServer(metricsServer, "metrics", logger)

	logger.Info("kubernaut-apifrontend started",
		"addr", rs.Addr, "tls", rs.TLSEnabled, "mcp_enabled", cfg.MCP.Enabled, "tools", 20)

	<-ctx.Done()
	draining.Store(true)
	sessInfra.StopFunc()
	logger.Info("shutting down...")

	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout(cfg))
	defer cancel()

	if tracker := routerCfg.SSETracker; tracker != nil {
		tracker.DrainAll(shutCtx)
	}
	if deps.InvestigationRegistry != nil {
		deps.InvestigationRegistry.StopAll()
		logger.Info("stopped active investigation sessions")
	}
	if deps.Pool != nil {
		if err := deps.Pool.DrainAll(shutCtx); err != nil {
			logger.Error(err, "failed to drain KA session pool on shutdown")
		}
	}
	shutdownServer(shutCtx, httpServer, "API", logger)
	shutdownServer(shutCtx, healthServer, "health", logger)
	shutdownServer(shutCtx, metricsServer, "metrics", logger)

	// Issue #1156: Drain shared audit store before exit to prevent event loss.
	if err := auditor.Close(shutCtx); err != nil {
		logger.Error(err, "failed to flush audit store on shutdown")
	}

	// WIRE-16: Stop background goroutines in limiters to prevent leaks.
	close(evictStop)
	ipLimiter.Stop()
	userLimiter.Stop()

	logger.Info("shutdown complete")
	return 0
}

// routerBuildParams groups the dependencies needed to build the router and
// the three HTTP servers (API/health/metrics). Extracted per AGENTS.md's
// 8+-param Options-pattern rule (GO-ANTIPATTERN-AUDIT-2026-07-01 Phase 4g).
type routerBuildParams struct {
	HDeps            *handlerDeps
	MCPHandler       http.Handler
	DepsReady        func() bool
	AgentCardHandler http.Handler
	A2AHandler       http.Handler
	AuthMiddleware   func(http.Handler) http.Handler
	AuthReady        func() bool
	PreAuthMW        func(http.Handler) http.Handler
	PostAuthMW       func(http.Handler) http.Handler
	Draining         *atomic.Bool
}

// routerAndServers groups the router config and the three HTTP servers
// (API/health/metrics) constructed by buildRouterAndServers. run() keeps
// RouterCfg.SSETracker to drain in-flight SSE connections at shutdown.
type routerAndServers struct {
	RouterCfg     handler.RouterConfig
	HTTPServer    *http.Server
	HealthServer  *http.Server
	MetricsServer *http.Server
	TLSEnabled    bool
	Addr          string
}

// buildRouterAndServers constructs the chi router, wires TLS (with
// hot-reloadable certs), and builds the API/health/metrics *http.Server
// instances. On any wiring failure it logs the specific cause itself
// (identical messages to the original inline run() code) and returns a
// non-nil error; callers should simply `return 1` without re-logging.
//
// Returns a cleanup function that stops the cert and CA file watchers, if
// started. The caller must defer it in run()'s own scope so the watchers
// live for the server's lifetime, not just this function's call
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Phase 4g — same pattern as
// watchLoopState.stopEAWatcher in Phase 4e / wireHotReload in KA's Phase 4f).
func buildRouterAndServers(ctx context.Context, p routerBuildParams) (*routerAndServers, func(), error) {
	cfg := p.HDeps.Cfg
	logger := p.HDeps.Logger
	metricsReg := p.HDeps.MetricsReg
	sessInfra := p.HDeps.SessInfra

	var statusHandler http.Handler
	if p.HDeps.Backends.TypedClient() != nil {
		statusHandler = handler.NewStatusHandler(p.HDeps.Backends.TypedClient(), cfg.Session.Namespace, logger)
	}

	routerCfg := handler.RouterConfig{
		MetricsRegistry:    metricsReg,
		Logger:             logger,
		A2AHandler:         p.A2AHandler,
		MCPHandler:         p.MCPHandler,
		AgentCardHandler:   p.AgentCardHandler,
		AuthMiddleware:     p.AuthMiddleware,
		PreAuthMiddleware:  p.PreAuthMW,
		PostAuthMiddleware: p.PostAuthMW,
		ReadyChecker:       handler.AllReady(func() bool { return !p.Draining.Load() }, p.DepsReady, p.AuthReady, sessInfra.Healthy.Load),
		SSETracker:         buildSSETracker(cfg, metricsReg),
		StatusHandler:      statusHandler,
		Draining:           p.Draining,
	}
	router, err := handler.NewRouter(routerCfg)
	if err != nil {
		logger.Error(err, "failed to create router")
		return nil, nil, err
	}

	addr := fmt.Sprintf(":%d", cfg.Server.Port)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	healthAddr := fmt.Sprintf(":%d", cfg.Server.HealthPort)
	metricsAddr := fmt.Sprintf(":%d", cfg.Server.MetricsPort)

	healthMux := buildHealthMux(handler.AllReady(p.DepsReady, p.AuthReady, sessInfra.Healthy.Load), p.Draining)
	healthServer := &http.Server{
		Addr:              healthAddr,
		Handler:           healthMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", metricsReg.Handler())
	metricsServer := &http.Server{
		Addr:              metricsAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	tlsEnabled, certReloader, err := tlswiring.ConfigureServer(httpServer, cfg.Server.TLS.CertDir)
	if err != nil {
		logger.Error(err, "failed to configure TLS")
		return nil, nil, err
	}
	if tlsEnabled {
		logger.Info("TLS enabled with hot-reloadable certificates", "certDir", cfg.Server.TLS.CertDir)
	} else {
		// F-006: Warn loudly when TLS is disabled; production deployments must use
		// either application TLS or document mesh/ingress TLS as compensating control.
		if warn := tlswiring.CheckPartialTLSMaterial(cfg.Server.TLS.CertDir); warn != "" {
			logger.Info("WARNING: "+warn, "certDir", cfg.Server.TLS.CertDir)
		}
		if cfg.Server.TLS.Required {
			reqErr := fmt.Errorf("TLS required but no certificates found")
			logger.Error(reqErr, "server.tls.required is true but certDir is empty or missing certs")
			return nil, nil, reqErr
		}
		logger.Info("WARNING: TLS disabled, serving plain HTTP — not suitable for FedRAMP production")
	}

	var stoppers []func()

	certWatcher, err := tlswiring.StartCertFileWatcher(ctx, cfg.Server.TLS.CertDir, certReloader, logger)
	if err != nil {
		logger.Error(err, "failed to start certificate file watcher")
		return nil, nil, err
	}
	if certWatcher != nil {
		stoppers = append(stoppers, certWatcher.Stop)
	}

	caWatcher, err := tlswiring.StartCAFileWatcher(ctx, logger)
	if err != nil {
		logger.Error(err, "failed to start CA file watcher")
		return nil, nil, err
	}
	if caWatcher != nil {
		stoppers = append(stoppers, caWatcher.Stop)
	}

	return &routerAndServers{
			RouterCfg:     routerCfg,
			HTTPServer:    httpServer,
			HealthServer:  healthServer,
			MetricsServer: metricsServer,
			TLSEnabled:    tlsEnabled,
			Addr:          addr,
		}, func() {
			for _, stop := range stoppers {
				stop()
			}
		}, nil
}

func newZapLogger(level zapcore.Level) *zap.Logger {
	zapCfg := zap.NewProductionConfig()
	zapCfg.EncoderConfig.TimeKey = "ts"
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapCfg.Level = zap.NewAtomicLevelAt(level)

	zapLogger, err := zapCfg.Build()
	if err != nil {
		return zap.NewNop()
	}
	return zapLogger
}

func startServer(srv *http.Server, name string, logger logr.Logger) {
	logger.Info("server listening", "name", name, "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error(err, "server error", "name", name)
		os.Exit(1)
	}
}

func startServerTLS(srv *http.Server, tlsEnabled bool, name string, logger logr.Logger) {
	if !tlsEnabled {
		startServer(srv, name, logger)
		return
	}
	logger.Info("server listening (TLS)", "name", name, "addr", srv.Addr)
	if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		logger.Error(err, "server TLS error", "name", name)
		os.Exit(1)
	}
}

func shutdownServer(ctx context.Context, srv *http.Server, name string, logger logr.Logger) {
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error(err, "shutdown error", "name", name)
	}
}

func loadConfig() (*config.Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// CFG-02: Fail when config file is missing — prevent unsafe defaults in production.
			return nil, fmt.Errorf("config file not found at %s — explicit configuration required", configPath)
		}
		return nil, err
	}
	return config.Load(data)
}

// handlerDeps groups the shared dependencies consumed by both buildMCPHandler
// and buildA2AHandler. Constructed once in run() to avoid excessive positional
// parameters on the builder functions.
type handlerDeps struct {
	Cfg               *config.Config
	Backends          *backendDeps
	SessInfra         *sessionInfra
	MetricsReg        *metrics.Registry
	Authorizer        auth.ToolAuthorizer
	Auditor           audit.Emitter
	Logger            logr.Logger
	UserLimiter       *ratelimit.UserLimiter
	ActiveCtxRegistry *launcher.ActiveContextRegistry
}

// Build-time metadata set via -ldflags.
var (
	Version   = "v0.1.0-dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func version() string {
	return Version
}

// bearerTokenTransport wraps an http.RoundTripper to inject an Authorization
// header with a bearer token read from a file (e.g. ServiceAccount token).
type bearerTokenTransport struct {
	base      http.RoundTripper
	tokenFile string
}

func (t *bearerTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := os.ReadFile(t.tokenFile) //nolint:gosec // G304/G703 -- path from operator-controlled config
	if err != nil {
		return nil, fmt.Errorf("reading bearer token: %w", err)
	}
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(r)
}

// buildHealthMux constructs the health server mux with dependency-aware readyz.
// WIRE-01: /readyz must check depsReady, not just draining.
func buildHealthMux(depsReady handler.ReadyChecker, draining *atomic.Bool) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"healthy"}`)
	})
	checker := depsReady
	if checker == nil {
		checker = func() bool { return true }
	}
	mux.Handle("/readyz", handler.ReadyzHandlerFunc(checker, draining))
	return mux
}

// shutdownTimeout returns the configured drain timeout or a sensible default.
// WIRE-07: must honour cfg.Shutdown.DrainSeconds instead of hardcoded 15s.
func shutdownTimeout(cfg *config.Config) time.Duration {
	if cfg.Shutdown.DrainSeconds > 0 {
		return time.Duration(cfg.Shutdown.DrainSeconds) * time.Second
	}
	return 15 * time.Second
}

func buildSSETracker(cfg *config.Config, metricsReg *metrics.Registry) *streaming.ConnectionTracker {
	tracker := streaming.NewConnectionTracker(metricsReg.SSEActiveConnections, 5*time.Second)
	if cfg.Server.MaxSSEConnections > 0 {
		tracker.MaxConnections = cfg.Server.MaxSSEConnections
	}
	return tracker
}
