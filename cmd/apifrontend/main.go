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
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ratelimit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/streaming"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tlswiring"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

const configPath = "/etc/apifrontend/config.yaml"

func main() { os.Exit(run()) }

func run() int {
	cfg, logger, zapLogger, err := setupConfigAndLogger()
	if err != nil {
		if zapLogger != nil {
			_ = zapLogger.Sync()
		}
		return 1
	}
	defer func() { _ = zapLogger.Sync() }()
	ctrl.SetLogger(logger.WithName("controller-runtime"))

	sarChecker, err := buildSARClient(logger, cfg.RBAC.SARCacheTTL)
	if err != nil {
		return 1
	}

	metricsReg := metrics.NewRegistry()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	auditor, stopAuditWiring, err := buildAuditWiring(ctx, cfg, logger)
	if err != nil {
		return 1
	}
	defer stopAuditWiring()

	ipLimiter, userLimiter := buildRateLimiters(cfg)

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
	defer stopBackendDeps(deps, logger)

	// AF-HIGH-2: Schedule periodic idle session eviction to prevent pool growth.
	evictStop := startIdleSessionEviction(deps.Pool, logger)

	hDeps := buildHandlerDeps(buildHandlerDepsParams{
		Cfg: cfg, Deps: deps, SessInfra: sessInfra, MetricsReg: metricsReg,
		SARChecker: sarChecker, Auditor: auditor, UserLimiter: userLimiter, Logger: logger,
	})

	mcpHandler, depsReady, agentCardHandler, a2aHandler, stopConfigWatcher, err := buildMCPAndAgentHandlers(ctx, cfg, hDeps, auditor, logger)
	if err != nil {
		return 1
	}
	defer stopConfigWatcher()

	rs, draining, stopTLSWatchers, err := buildAndStartRouter(ctx, buildAndStartRouterParams{
		Cfg: cfg, HDeps: hDeps, MCPHandler: mcpHandler, DepsReady: depsReady,
		AgentCardHandler: agentCardHandler, A2AHandler: a2aHandler,
		MetricsReg: metricsReg, Auditor: auditor,
		IPLimiter: ipLimiter, UserLimiter: userLimiter, Logger: logger,
	})
	if err != nil {
		// startRouterAndServers already logged the specific failure.
		return 1
	}
	defer stopTLSWatchers()

	<-ctx.Done()
	draining.Store(true)
	sessInfra.StopFunc()
	runShutdown(runShutdownParams{
		RS: rs, Cfg: cfg, Deps: deps, Auditor: auditor, EvictStop: evictStop,
		IPLimiter: ipLimiter, UserLimiter: userLimiter, Logger: logger,
	})
	return 0
}

// buildAndStartRouterParams bundles the inputs needed to wire the auth/rate-limit
// middlewares and build+start the router and its HTTP servers.
type buildAndStartRouterParams struct {
	Cfg              *config.Config
	HDeps            *handlerDeps
	MCPHandler       http.Handler
	DepsReady        func() bool
	AgentCardHandler http.Handler
	A2AHandler       http.Handler
	MetricsReg       *metrics.Registry
	Auditor          audit.Emitter
	IPLimiter        *ratelimit.IPLimiter
	UserLimiter      *ratelimit.UserLimiter
	Logger           logr.Logger
}

// buildAndStartRouter wires the JWT auth middleware (F-001, fall back to noop
// only when auth is unconfigured) and rate-limit middlewares, then builds and
// starts the router and its HTTP servers.
func buildAndStartRouter(ctx context.Context, p buildAndStartRouterParams) (*routerAndServers, *atomic.Bool, func(), error) {
	authMiddleware, authReady := buildAuthMiddleware(p.Cfg, p.MetricsReg, p.Auditor, p.Logger)
	preAuthMW, postAuthMW := buildRateLimitMiddlewares(p.MetricsReg, p.Auditor, p.IPLimiter, p.UserLimiter)

	return startRouterAndServers(ctx, startRouterParams{
		HDeps: p.HDeps, MCPHandler: p.MCPHandler, DepsReady: p.DepsReady,
		AgentCardHandler: p.AgentCardHandler, A2AHandler: p.A2AHandler,
		AuthMiddleware: authMiddleware, AuthReady: authReady,
		PreAuthMW: preAuthMW, PostAuthMW: postAuthMW,
	}, p.Cfg, p.Logger)
}

// startRouterParams bundles the handler/middleware inputs needed to build
// and start the router and its HTTP servers.
type startRouterParams struct {
	HDeps            *handlerDeps
	MCPHandler       http.Handler
	DepsReady        func() bool
	AgentCardHandler http.Handler
	A2AHandler       http.Handler
	AuthMiddleware   func(http.Handler) http.Handler
	AuthReady        handler.ReadyChecker
	PreAuthMW        func(http.Handler) http.Handler
	PostAuthMW       func(http.Handler) http.Handler
}

// startRouterAndServers builds the router/HTTP servers and starts them in
// background goroutines. The returned atomic.Bool is the draining flag that
// the caller must set to true before initiating graceful shutdown.
func startRouterAndServers(ctx context.Context, p startRouterParams, cfg *config.Config, logger logr.Logger) (*routerAndServers, *atomic.Bool, func(), error) {
	draining := &atomic.Bool{}
	rs, stopTLSWatchers, err := buildRouterAndServers(ctx, routerBuildParams{
		HDeps:            p.HDeps,
		MCPHandler:       p.MCPHandler,
		DepsReady:        p.DepsReady,
		AgentCardHandler: p.AgentCardHandler,
		A2AHandler:       p.A2AHandler,
		AuthMiddleware:   p.AuthMiddleware,
		AuthReady:        p.AuthReady,
		PreAuthMW:        p.PreAuthMW,
		PostAuthMW:       p.PostAuthMW,
		Draining:         draining,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	startServers(rs, cfg, logger)
	return rs, draining, stopTLSWatchers, nil
}

// runShutdownParams bundles the running server/backend state runShutdown
// needs to assemble shutdownDeps.
type runShutdownParams struct {
	RS          *routerAndServers
	Cfg         *config.Config
	Deps        *backendDeps
	Auditor     audit.ClosableEmitter
	EvictStop   chan struct{}
	IPLimiter   *ratelimit.IPLimiter
	UserLimiter *ratelimit.UserLimiter
	Logger      logr.Logger
}

// runShutdown assembles shutdownDeps from the running server/backend state
// and drives graceful shutdown.
func runShutdown(p runShutdownParams) {
	gracefulShutdown(shutdownDeps{
		Cfg:           p.Cfg,
		Logger:        p.Logger,
		Deps:          p.Deps,
		SSETracker:    p.RS.RouterCfg.SSETracker,
		HTTPServer:    p.RS.HTTPServer,
		HealthServer:  p.RS.HealthServer,
		MetricsServer: p.RS.MetricsServer,
		Auditor:       p.Auditor,
		EvictStop:     p.EvictStop,
		IPLimiter:     p.IPLimiter,
		UserLimiter:   p.UserLimiter,
	})
}

// stopBackendDeps stops the CA file watchers and closes the Fleet resilient
// MCP client, logging any close error. Safe to call via defer unconditionally.
func stopBackendDeps(deps *backendDeps, logger logr.Logger) {
	for _, w := range deps.CAWatchers {
		w.watcher.Stop()
	}
	if deps.fleetReadinessGate != nil {
		deps.fleetReadinessGate.Stop()
	}
	if fc := deps.FleetResilientClient(); fc != nil {
		logger.Info("Closing fleet MCP Gateway connection")
		if err := fc.Close(); err != nil {
			logger.Error(err, "failed to close fleet MCP client gracefully")
		}
	}
}

// startServers launches the API, health, and metrics HTTP servers in
// background goroutines and logs the started-up banner.
func startServers(rs *routerAndServers, cfg *config.Config, logger logr.Logger) {
	go startServerTLS(rs.HTTPServer, rs.TLSEnabled, "API", logger)
	go startServer(rs.HealthServer, "health", logger)
	go startServer(rs.MetricsServer, "metrics", logger)

	logger.Info("kubernaut-apifrontend started",
		"addr", rs.Addr, "tls", rs.TLSEnabled, "mcp_enabled", cfg.MCP.Enabled, "tools", 20)
}

// buildHandlerDepsParams bundles the inputs buildHandlerDeps needs to
// assemble the shared handlerDeps bundle.
type buildHandlerDepsParams struct {
	Cfg         *config.Config
	Deps        *backendDeps
	SessInfra   *sessionInfra
	MetricsReg  *metrics.Registry
	SARChecker  auth.ToolAuthorizer
	Auditor     audit.Emitter
	UserLimiter *ratelimit.UserLimiter
	Logger      logr.Logger
}

// buildHandlerDeps assembles the shared handlerDeps bundle passed to the MCP,
// A2A, and router builders.
func buildHandlerDeps(p buildHandlerDepsParams) *handlerDeps {
	return &handlerDeps{
		Cfg:               p.Cfg,
		Backends:          p.Deps,
		SessInfra:         p.SessInfra,
		MetricsReg:        p.MetricsReg,
		Authorizer:        p.SARChecker,
		Auditor:           p.Auditor,
		Logger:            p.Logger,
		UserLimiter:       p.UserLimiter,
		ActiveCtxRegistry: launcher.NewActiveContextRegistry(launcher.DefaultRegistryTTL, launcher.DefaultRegistryIdleTimeout),
	}
}

// buildMCPAndAgentHandlers wires the MCP handler, the config file watcher
// (CM-02: drift detection + audit trail), and the agent-card/A2A handlers.
// On error, the caller should return without deferring stopConfigWatcher.
func buildMCPAndAgentHandlers(ctx context.Context, cfg *config.Config, hDeps *handlerDeps, auditor audit.Emitter, logger logr.Logger) (mcpHandler http.Handler, depsReady func() bool, agentCardHandler, a2aHandler http.Handler, stopConfigWatcher func(), err error) {
	mcpHandler, depsReady, err = buildMCPHandler(hDeps)
	if err != nil {
		logger.Error(err, "failed to create MCP handler")
		return nil, nil, nil, nil, nil, err
	}

	stopConfigWatcher = startConfigWatcher(ctx, cfg, auditor, logger)

	agentCardHandler, a2aHandler, err = buildCardAndA2AHandler(ctx, cfg, hDeps, auditor)
	if err != nil {
		logger.Error(err, "failed to create agent card/A2A handler")
		stopConfigWatcher()
		return nil, nil, nil, nil, nil, err
	}

	return mcpHandler, depsReady, agentCardHandler, a2aHandler, stopConfigWatcher, nil
}

// buildCardAndA2AHandler wires the audited agent-card handler and the A2A
// (Agent-to-Agent) handler used by the router.
func buildCardAndA2AHandler(ctx context.Context, cfg *config.Config, hDeps *handlerDeps, auditor audit.Emitter) (http.Handler, http.Handler, error) {
	agentCardBase, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
		Name:        "Kubernaut Agent",
		Description: "Kubernaut AI-driven remediation agent",
		URL:         cfg.AgentCard.URL,
		Version:     version(),
		Skills:      handler.DefaultAgentSkills(cfg.Interactive.Enabled),
	})
	if err != nil {
		return nil, nil, err
	}
	agentCardHandler := handler.WithAgentCardAudit(agentCardBase, auditor)

	a2aHandler, err := buildA2AHandler(ctx, hDeps)
	if err != nil {
		return nil, nil, err
	}
	return agentCardHandler, a2aHandler, nil
}

// buildRateLimitMiddlewares constructs the pre-auth (per-IP) and post-auth
// (per-user) rate-limiting middlewares (F-005 / SM-01 / SM-02).
func buildRateLimitMiddlewares(metricsReg *metrics.Registry, auditor audit.Emitter, ipLimiter *ratelimit.IPLimiter, userLimiter *ratelimit.UserLimiter) (func(http.Handler) http.Handler, func(http.Handler) http.Handler) {
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
	return preAuthMW, postAuthMW
}

// setupConfigAndLogger loads and validates the AF config, resolves the
// operational namespace and interactive timeouts, and constructs the
// zap-backed logr.Logger. On failure, the returned error has already been
// logged with a best-effort logger (the real one may not exist yet); the
// returned *zap.Logger is non-nil as soon as it has been constructed, so
// callers can still flush it (Sync) even on a later failure (e.g. Validate).
func setupConfigAndLogger() (*config.Config, logr.Logger, *zap.Logger, error) {
	cfg, err := loadConfig()
	if err != nil {
		z, _ := zap.NewProduction()
		z.Error("failed to load config", zap.Error(err))
		return nil, logr.Logger{}, nil, err
	}
	origPort := cfg.Server.Port
	config.ApplyPortEnvOverride(cfg)
	if err := cfg.ResolveDefaults(); err != nil {
		z, _ := zap.NewProduction()
		z.Error("failed to resolve config defaults", zap.Error(err))
		return nil, logr.Logger{}, nil, err
	}

	logLevel, _ := parseLogLevel(cfg.Logging.Level)
	zapLogger := newZapLogger(logLevel)
	logger := zapr.NewLogger(zapLogger).WithName("apifrontend")

	if cfg.Server.Port != origPort {
		logger.Info("PORT env override applied", "original", origPort, "effective", cfg.Server.Port)
	} else if p := os.Getenv("PORT"); p != "" {
		logger.Info("PORT env var ignored (invalid or out-of-range)", "value", p)
	}

	if err := cfg.Validate(); err != nil {
		logger.Error(err, "invalid config")
		return nil, logr.Logger{}, zapLogger, err
	}

	cfg.Session.Namespace = agentpkg.ResolveNamespace(cfg.Session.Namespace, agentpkg.DefaultNamespaceFile)
	logger.Info("operational namespace resolved", "namespace", cfg.Session.Namespace)

	if cfg.Interactive.AwaitSessionTimeout > 0 {
		tools.AwaitSessionTimeout = cfg.Interactive.AwaitSessionTimeout
	}
	if cfg.Interactive.BridgeInactivityTimeout > 0 {
		tools.BridgeInactivityTimeout = cfg.Interactive.BridgeInactivityTimeout
	}

	return cfg, logger, zapLogger, nil
}

// buildSARClient wires the SelfSubjectAccessReview-based authorizer used for
// K8s-tool RBAC gating.
func buildSARClient(logger logr.Logger, sarCacheTTL time.Duration) (auth.ToolAuthorizer, error) {
	restCfg, err := ctrl.GetConfig()
	if err != nil {
		logger.Error(err, "failed to get in-cluster config for SAR client")
		return nil, err
	}
	k8sClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		logger.Error(err, "failed to create kubernetes client for SAR")
		return nil, err
	}
	return auth.NewSARChecker(k8sClient, sarCacheTTL, logger.WithName("sar")), nil
}

// buildAuditWiring wires AF's audit trail to the shared BufferedAuditStore
// (#1156), including the optional CA-reloadable DS audit transport. Always
// returns a non-nil cleanup func (a no-op if no watcher was started);
// callers should defer it unconditionally.
func buildAuditWiring(ctx context.Context, cfg *config.Config, logger logr.Logger) (audit.ClosableEmitter, func(), error) {
	auditDSTransport, cleanup, err := buildAuditDSTransport(ctx, cfg, logger)
	if err != nil {
		return nil, cleanup, err
	}
	dsAuditClient, err := sharedaudit.NewOpenAPIClientAdapterWithTransport(
		cfg.Agent.DSBaseURL, cfg.Resilience.DS.RequestTimeout, auditDSTransport)
	if err != nil {
		logger.Error(err, "DS audit client creation failed — refusing to start")
		return nil, cleanup, err
	}
	auditStore, err := sharedaudit.NewBufferedStore(dsAuditClient, sharedaudit.DefaultConfig(), "apifrontend", logger)
	if err != nil {
		logger.Error(err, "failed to create buffered audit store")
		return nil, cleanup, err
	}
	auditor := audit.NewStoreAdapter(auditStore, logger)
	logger.Info("audit trail wired to shared BufferedAuditStore", "dsURL", cfg.Agent.DSBaseURL)
	return auditor, cleanup, nil
}

// buildAuditDSTransport constructs the (optional) CA-reloadable, optionally
// bearer-token-wrapped HTTP transport used by the DS audit client. Returns a
// nil transport (and a no-op cleanup) when no DS base URL is configured.
func buildAuditDSTransport(ctx context.Context, cfg *config.Config, logger logr.Logger) (http.RoundTripper, func(), error) {
	noopCleanup := func() {}
	if cfg.Agent.DSBaseURL == "" {
		return nil, noopCleanup, nil
	}

	transport, auditDSWatcher, err := tlswiring.CAReloadableTransport(cfg.Agent.DSTLSCaFile, logger.WithName("ds-audit-ca"))
	if err != nil {
		logger.Error(err, "DS audit CA transport failed — refusing to start with broken TLS")
		return nil, noopCleanup, err
	}
	cleanup := noopCleanup
	if auditDSWatcher != nil {
		if err := auditDSWatcher.Start(ctx); err != nil {
			logger.Error(err, "DS audit CA watcher failed to start")
			return nil, noopCleanup, err
		}
		cleanup = auditDSWatcher.Stop
	}
	if cfg.Agent.DSBearerTokenFile == "" {
		return transport, cleanup, nil
	}
	return &bearerTokenTransport{base: transport, tokenFile: cfg.Agent.DSBearerTokenFile}, cleanup, nil
}

// buildRateLimiters constructs the per-IP and per-user rate limiters from
// cfg.RateLimit (F-005 + SM-01/SM-02/CFG-01).
func buildRateLimiters(cfg *config.Config) (*ratelimit.IPLimiter, *ratelimit.UserLimiter) {
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
	return ratelimit.NewIPLimiter(rlCfg.PerIP), ratelimit.NewUserLimiter(rlCfg.PerUser)
}

// startIdleSessionEviction schedules periodic idle KA session eviction
// (AF-HIGH-2) to prevent unbounded pool growth. No-op (returns an unused,
// never-closed-by-anyone-but-caller channel) when pool is nil.
func startIdleSessionEviction(pool *ka.KASessionPool, logger logr.Logger) chan struct{} {
	evictStop := make(chan struct{})
	if pool == nil {
		return evictStop
	}
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if n := pool.EvictIdle(); n > 0 {
					logger.V(1).Info("evicted idle KA sessions", "count", n)
				}
			case <-evictStop:
				return
			}
		}
	}()
	return evictStop
}

// startConfigWatcher wires CM-02 config file drift detection + audit trail.
// Returns a cleanup func to stop the watcher; safe to call unconditionally
// even if the watcher was never started.
func startConfigWatcher(ctx context.Context, cfg *config.Config, auditor audit.Emitter, logger logr.Logger) func() {
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
		return func() {}
	}
	if err := cfgWatcher.Start(ctx); err != nil {
		logger.Info("config file watcher start failed", "error", err)
		return func() {}
	}
	return cfgWatcher.Stop
}

// shutdownDeps groups the dependencies needed by gracefulShutdown. Extracted
// per AGENTS.md's 8+-param Options-pattern rule (GO-ANTIPATTERN-AUDIT-2026-07-01
// Phase 4g), same pattern as routerBuildParams/handlerDeps in this file.
type shutdownDeps struct {
	Cfg           *config.Config
	Logger        logr.Logger
	Deps          *backendDeps
	SSETracker    *streaming.ConnectionTracker
	HTTPServer    *http.Server
	HealthServer  *http.Server
	MetricsServer *http.Server
	Auditor       audit.ClosableEmitter
	EvictStop     chan struct{}
	IPLimiter     *ratelimit.IPLimiter
	UserLimiter   *ratelimit.UserLimiter
}

// gracefulShutdown runs AF's shutdown sequence: drain in-flight SSE/investigation/
// pool sessions, stop the HTTP/health/metrics servers, flush the audit store,
// and stop rate-limiter background goroutines (WIRE-16).
func gracefulShutdown(d shutdownDeps) {
	d.Logger.Info("shutting down...")

	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout(d.Cfg))
	defer cancel()

	if d.SSETracker != nil {
		d.SSETracker.DrainAll(shutCtx)
	}
	if d.Deps.InvestigationRegistry != nil {
		d.Deps.InvestigationRegistry.StopAll()
		d.Logger.Info("stopped active investigation sessions")
	}
	if d.Deps.Pool != nil {
		if err := d.Deps.Pool.DrainAll(shutCtx); err != nil {
			d.Logger.Error(err, "failed to drain KA session pool on shutdown")
		}
	}
	shutdownServer(shutCtx, d.HTTPServer, "API", d.Logger)
	shutdownServer(shutCtx, d.HealthServer, "health", d.Logger)
	shutdownServer(shutCtx, d.MetricsServer, "metrics", d.Logger)

	// Issue #1156: Drain shared audit store before exit to prevent event loss.
	if err := d.Auditor.Close(shutCtx); err != nil {
		d.Logger.Error(err, "failed to flush audit store on shutdown")
	}

	// WIRE-16: Stop background goroutines in limiters to prevent leaks.
	close(d.EvictStop)
	d.IPLimiter.Stop()
	d.UserLimiter.Stop()

	d.Logger.Info("shutdown complete")
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

	routerCfg, router, err := buildRouterConfig(p)
	if err != nil {
		logger.Error(err, "failed to create router")
		return nil, nil, err
	}

	addr, httpServer, healthServer, metricsServer := buildHTTPServers(p, router)

	tlsEnabled, certReloader, err := configureServerTLS(httpServer, cfg, logger)
	if err != nil {
		return nil, nil, err
	}

	stopWatchers, err := startTLSWatchers(ctx, cfg.Server.TLS.CertDir, certReloader, logger)
	if err != nil {
		return nil, nil, err
	}

	return &routerAndServers{
		RouterCfg:     routerCfg,
		HTTPServer:    httpServer,
		HealthServer:  healthServer,
		MetricsServer: metricsServer,
		TLSEnabled:    tlsEnabled,
		Addr:          addr,
	}, stopWatchers, nil
}

// buildRouterConfig assembles handler.RouterConfig from routerBuildParams and
// constructs the chi router.
func buildRouterConfig(p routerBuildParams) (handler.RouterConfig, http.Handler, error) {
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
		return routerCfg, nil, err
	}
	return routerCfg, router, nil
}

// buildHTTPServers constructs the three *http.Server instances (API/health/
// metrics) sharing the router built by buildRouterConfig.
func buildHTTPServers(p routerBuildParams, router http.Handler) (addr string, httpServer, healthServer, metricsServer *http.Server) {
	cfg := p.HDeps.Cfg
	metricsReg := p.HDeps.MetricsReg
	sessInfra := p.HDeps.SessInfra

	addr = fmt.Sprintf(":%d", cfg.Server.Port)
	httpServer = &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	healthMux := buildHealthMux(handler.AllReady(p.DepsReady, p.AuthReady, sessInfra.Healthy.Load), p.Draining)
	healthServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.HealthPort),
		Handler:           healthMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", metricsReg.Handler())
	metricsServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.MetricsPort),
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return addr, httpServer, healthServer, metricsServer
}

// configureServerTLS wires hot-reloadable TLS certs onto httpServer if
// certificate material is present, warning loudly (F-006) when TLS ends up
// disabled and failing fast if TLS is explicitly required but unavailable.
func configureServerTLS(httpServer *http.Server, cfg *config.Config, logger logr.Logger) (tlsEnabled bool, certReloader *sharedtls.CertReloader, err error) {
	tlsEnabled, certReloader, err = tlswiring.ConfigureServer(httpServer, cfg.Server.TLS.CertDir)
	if err != nil {
		logger.Error(err, "failed to configure TLS")
		return false, nil, err
	}
	if tlsEnabled {
		logger.Info("TLS enabled with hot-reloadable certificates", "certDir", cfg.Server.TLS.CertDir)
		return tlsEnabled, certReloader, nil
	}

	// F-006: Warn loudly when TLS is disabled; production deployments must use
	// either application TLS or document mesh/ingress TLS as compensating control.
	if warn := tlswiring.CheckPartialTLSMaterial(cfg.Server.TLS.CertDir); warn != "" {
		logger.Info("WARNING: "+warn, "certDir", cfg.Server.TLS.CertDir)
	}
	if cfg.Server.TLS.Required {
		reqErr := fmt.Errorf("TLS required but no certificates found")
		logger.Error(reqErr, "server.tls.required is true but certDir is empty or missing certs")
		return false, nil, reqErr
	}
	logger.Info("WARNING: TLS disabled, serving plain HTTP — not suitable for FedRAMP production")
	return tlsEnabled, certReloader, nil
}

// startTLSWatchers starts the cert and CA file watchers (if configured) and
// returns a single cleanup func that stops whichever ones were started.
func startTLSWatchers(ctx context.Context, certDir string, certReloader *sharedtls.CertReloader, logger logr.Logger) (func(), error) {
	var stoppers []func()

	certWatcher, err := tlswiring.StartCertFileWatcher(ctx, certDir, certReloader, logger)
	if err != nil {
		logger.Error(err, "failed to start certificate file watcher")
		return nil, err
	}
	if certWatcher != nil {
		stoppers = append(stoppers, certWatcher.Stop)
	}

	caWatcher, err := tlswiring.StartCAFileWatcher(ctx, logger)
	if err != nil {
		logger.Error(err, "failed to start CA file watcher")
		return nil, err
	}
	if caWatcher != nil {
		stoppers = append(stoppers, caWatcher.Stop)
	}

	return func() {
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
