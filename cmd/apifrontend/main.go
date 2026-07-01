package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	adksession "google.golang.org/adk/session"
	"gopkg.in/yaml.v3"
	authorizationv1 "k8s.io/api/authorization/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/apifrontend"
	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ds"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ratelimit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/resilience"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/streaming"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tlswiring"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
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

type caWatcherEntry struct {
	name    string
	watcher *hotreload.FileWatcher
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

// backendDeps holds shared backend clients used by both the MCP and A2A handlers.
// Created once by buildBackendDeps and consumed by buildMCPHandler / buildA2AHandler.
type backendDeps struct {
	DSClient              ds.Client
	KAClient              *ka.Client
	MCPClient             ka.MCPClient
	DedicatedClient       ka.MCPClient
	Pool                  *ka.KASessionPool
	Triager               *severity.Triager
	PromClient            prom.Client
	DSResilientTransport  *resilience.CircuitBreakerTransport
	CAWatchers            []caWatcherEntry
	k8sDynClient          dynamic.Interface
	k8sTypedClient        crclient.WithWatch
	K8sCB                 *resilience.K8sCircuitBreaker
	InvestigationRegistry *tools.MonitorRegistry
	Mapper                meta.RESTMapper
}

// K8sClient returns the pod service-account scoped dynamic K8s client,
// wrapped with a circuit breaker. Returns nil if K8s API was unreachable
// at startup; callers must check for nil (tools return a clear error).
func (d *backendDeps) K8sClient() dynamic.Interface {
	return d.k8sDynClient
}

// TypedClient returns the controller-runtime typed client for all kubernaut CRDs
// (RR, RAR, EA, AIAnalysis, IS). Returns nil if K8s API was unreachable;
// callers must check for nil (#1428).
func (d *backendDeps) TypedClient() crclient.WithWatch {
	return d.k8sTypedClient
}

func buildBackendDeps(ctx context.Context, cfg *config.Config, metricsReg *metrics.Registry, auditor audit.Emitter, logger logr.Logger) (*backendDeps, error) {
	deps := &backendDeps{}

	dsTransport, dsWatcher, err := tlswiring.CAReloadableTransport(cfg.Agent.DSTLSCaFile, logger.WithName("ds-ca"))
	if err != nil {
		return nil, fmt.Errorf("DS TLS transport: %w", err)
	}
	if dsWatcher != nil {
		if err := dsWatcher.Start(ctx); err != nil {
			return nil, fmt.Errorf("DS CA watcher start: %w", err)
		}
		deps.CAWatchers = append(deps.CAWatchers, caWatcherEntry{name: "ds-ca", watcher: dsWatcher})
	}

	deps.DSResilientTransport = buildResilientTransport(dsTransport, &cfg.Resilience.DS, "ds", metricsReg, auditor)

	var dsAuthTransport http.RoundTripper = deps.DSResilientTransport
	if cfg.Agent.DSBearerTokenFile != "" {
		dsAuthTransport = &bearerTokenTransport{
			base:      deps.DSResilientTransport,
			tokenFile: cfg.Agent.DSBearerTokenFile,
		}
	}

	dsCfg := ds.OgenClientConfig{
		BaseURL:   cfg.Agent.DSBaseURL,
		Timeout:   cfg.Resilience.DS.RequestTimeout,
		Transport: dsAuthTransport,
	}
	if c, err := ds.NewOgenClient(dsCfg); err == nil {
		deps.DSClient = c
	} else {
		logger.Info("DS client unavailable, DS tools will return errors", "error", err)
	}

	kaTransport, kaWatcher, err := tlswiring.CAReloadableTransport(cfg.Agent.KATLSCaFile, logger.WithName("ka-ca"))
	if err != nil {
		return nil, fmt.Errorf("KA TLS transport: %w", err)
	}
	if kaWatcher != nil {
		if err := kaWatcher.Start(ctx); err != nil {
			return nil, fmt.Errorf("KA CA watcher start: %w", err)
		}
		deps.CAWatchers = append(deps.CAWatchers, caWatcherEntry{name: "ka-ca", watcher: kaWatcher})
	}

	kaMCPResilient := buildResilientTransport(kaTransport, &cfg.Resilience.KA, "ka-mcp", metricsReg, auditor)
	var kaMCPAuth http.RoundTripper = kaMCPResilient
	if cfg.Agent.KABearerTokenFile != "" {
		kaMCPAuth = &bearerTokenTransport{
			base:      kaMCPResilient,
			tokenFile: cfg.Agent.KABearerTokenFile,
		}
	}
	kaMCPHTTPClient := &http.Client{
		Transport: kaMCPAuth,
		Timeout:   cfg.Resilience.KA.RequestTimeout,
	}
	// #1386: Separate HTTP client for long-lived MCP sessions (SSE streams).
	// Go's http.Client.Timeout is a deadline on the entire response including
	// body reads. For persistent SSE connections, the 30s timeout kills the
	// stream after idle periods, causing "session not found" on next tool call.
	// The MCP SDK manages session lifecycle via context cancellation and
	// session.Close(); no global timeout is needed.
	kaMCPStreamClient := &http.Client{
		Transport: kaMCPAuth,
	}
	mcpClient := ka.NewSDKMCPClient(
		cfg.Agent.KAMCPEndpoint,
		kaMCPHTTPClient,
		kaMCPStreamClient,
		logger,
	)
	mcpClient.WithDownstreamDuration(metricsReg.DownstreamDuration)

	// G2: Persistent MCP sessions (#1306). The pool creates real MCP
	// connections via StreamableClientTransport. Sessions are keyed by
	// (rr_id, username) for user isolation (G9). PooledMCPClient wraps
	// the pool to implement MCPClient, auto-releasing on terminal actions.
	kaMCPEndpoint := cfg.Agent.KAMCPEndpoint
	deps.Pool = ka.NewKASessionPool(ka.PoolConfig{
		Factory: func(ctx context.Context) (ka.PoolSession, error) {
			transport := &mcp.StreamableClientTransport{
				Endpoint:   kaMCPEndpoint,
				HTTPClient: kaMCPStreamClient,
			}
			return mcpClient.ConnectSession(ctx, transport)
		},
		MaxEntries: 100,
		IdleTTL:    10 * time.Minute,
		Logger:     logger.WithName("ka-session-pool"),
	})
	deps.MCPClient = ka.NewPooledMCPClient(deps.Pool, logger)
	deps.DedicatedClient = mcpClient
	deps.InvestigationRegistry = tools.NewMonitorRegistry()

	kaRESTAuth := http.RoundTripper(kaTransport)
	if cfg.Agent.KABearerTokenFile != "" {
		kaRESTAuth = &bearerTokenTransport{
			base:      kaTransport,
			tokenFile: cfg.Agent.KABearerTokenFile,
		}
	}

	deps.KAClient = ka.NewClient(ka.Config{
		BaseURL:            cfg.Agent.KABaseURL,
		BaseTransport:      kaRESTAuth,
		Timeout:            cfg.Resilience.KA.RequestTimeout,
		CBMaxRequests:      cfg.Resilience.KA.CBMaxRequests,
		CBInterval:         cfg.Resilience.KA.CBInterval,
		CBTimeout:          cfg.Resilience.KA.CBTimeout,
		CBFailureThreshold: cfg.Resilience.KA.CBFailureThreshold,
		RetryMax:           cfg.Resilience.KA.RetryMax,
		RetryInitBackoff:   cfg.Resilience.KA.RetryInitBackoff,
		RetryMaxBackoff:    cfg.Resilience.KA.RetryMaxBackoff,
		RetryableStatuses:  cfg.Resilience.KA.RetryableStatuses,
		CBAuditFunc:        resilience.CircuitBreakerAuditFunc(auditor),
	}, &ka.ClientMetrics{
		StateGauge:   metricsReg.CircuitBreakerState,
		DurationHist: metricsReg.DownstreamDuration,
	})

	// F7+F8: Eager K8s dynamic client init with circuit breaker (WIRE-03).
	// Fail-fast: log clearly on failure instead of returning silent nil.
	restCfg, err := ctrl.GetConfig()
	if err != nil {
		logger.Error(err, "K8s dynamic client unavailable — K8s tools will return errors at runtime")
	} else {
		inner, err := dynamic.NewForConfig(restCfg)
		if err != nil {
			logger.Error(err, "K8s dynamic client creation failed — K8s tools will return errors at runtime")
		} else {
			k8sCfg := cfg.Resilience.K8s
			deps.K8sCB = resilience.NewK8sCircuitBreaker(resilience.K8sCBConfig{
				Name:             "k8s",
				MaxRequests:      k8sCfg.CBMaxRequests,
				Interval:         k8sCfg.CBInterval,
				Timeout:          k8sCfg.CBTimeout,
				FailureThreshold: k8sCfg.CBFailureThreshold,
				StateGauge:       metricsReg.CircuitBreakerState,
				DependencyName:   "k8s",
			})
			deps.k8sDynClient = resilience.NewResilientDynamicClient(inner, deps.K8sCB)
			logger.Info("K8s dynamic client initialized with circuit breaker")

			disc, discErr := discovery.NewDiscoveryClientForConfig(restCfg)
			if discErr != nil {
				logger.Error(discErr, "K8s discovery client unavailable — CRD kind resolution will use static table only")
			} else {
				deps.Mapper = restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disc))
				logger.Info("K8s RESTMapper initialized for CRD kind resolution")
			}

			typedScheme := k8sruntime.NewScheme()
			_ = eav1alpha1.AddToScheme(typedScheme)
			_ = remediationv1.AddToScheme(typedScheme)
			_ = aianalysisv1.AddToScheme(typedScheme)
			_ = isv1alpha1.AddToScheme(typedScheme)
			typedClient, tcErr := crclient.NewWithWatch(restCfg, crclient.Options{Scheme: typedScheme})
			if tcErr != nil {
				logger.Error(tcErr, "K8s typed client creation failed — CRD typed operations will fall back to dynamic")
			} else {
				deps.k8sTypedClient = typedClient
				logger.Info("K8s typed client initialized for all kubernaut CRD operations (#1428)")
			}
		}
	}

	if cfg.SeverityTriage.Enabled {
		promTransport, promWatcher, promErr := tlswiring.CAReloadableTransport(cfg.SeverityTriage.PrometheusTLSCaFile, logger.WithName("prom-ca"))
		if promErr != nil {
			return nil, fmt.Errorf("prometheus TLS transport: %w", promErr)
		}
		if promWatcher != nil {
			if err := promWatcher.Start(ctx); err != nil {
				return nil, fmt.Errorf("prometheus CA watcher start: %w", err)
			}
			deps.CAWatchers = append(deps.CAWatchers, caWatcherEntry{name: "prom-ca", watcher: promWatcher})
		}

		if cfg.Resilience.Prometheus.ConnectTimeout > 0 {
			if t, ok := promTransport.(*http.Transport); ok {
				t = t.Clone()
				t.DialContext = (&net.Dialer{Timeout: cfg.Resilience.Prometheus.ConnectTimeout}).DialContext
				promTransport = t
			}
		}
		promHTTPClient := &http.Client{
			Transport: promTransport,
			Timeout:   cfg.Resilience.Prometheus.RequestTimeout,
		}
		if cfg.SeverityTriage.PrometheusBearerTokenFile != "" {
			promHTTPClient.Transport = &bearerTokenTransport{
				base:      promTransport,
				tokenFile: cfg.SeverityTriage.PrometheusBearerTokenFile,
			}
		}

		promClient := prom.NewHTTPClient(cfg.SeverityTriage.PrometheusURL, promHTTPClient)
		deps.PromClient = promClient

		// BR-AI-1404: Resolve effective triage LLM config (independent or inherited).
		triageLLMCfg := cfg.Agent.LLM
		if cfg.SeverityTriage.LLM != nil {
			triageLLMCfg = *cfg.SeverityTriage.LLM
		}

		var llmTriager severity.LLMTriager
		if triageLLMCfg.Provider != "" {
			triager, triageErr := newLLMTriagerFromConfig(ctx, triageLLMCfg, logger.WithName("llm-triage"))
			if triageErr != nil {
				logger.Error(triageErr, "failed to create LLM triager, falling back to noop")
				llmTriager = severity.NewNoopLLMTriager(logger.WithName("llm-triage"))
			} else {
				llmTriager = triager
				logger.Info("LLM severity triage enabled",
					"provider", triageLLMCfg.Provider,
					"model", triageLLMCfg.Model,
					"source", triageLLMSource(cfg))
			}
		} else {
			llmTriager = severity.NewNoopLLMTriager(logger.WithName("llm-triage"))
			logger.Info("LLM severity triage disabled (no LLM provider configured), using noop triager")
		}

		severityCfg := severity.Config{
			Enabled:           true,
			MaxQueriesPerCall: cfg.SeverityTriage.MaxQueriesPerCall,
			MaxRulesEvaluated: cfg.SeverityTriage.MaxRulesEvaluated,
			CacheTTLSeconds:   cfg.SeverityTriage.CacheTTLSeconds,
			LLMConfidence:     cfg.SeverityTriage.LLMConfidence,
		}
		if severityCfg.MaxQueriesPerCall == 0 {
			severityCfg.MaxQueriesPerCall = 10
		}
		if severityCfg.MaxRulesEvaluated == 0 {
			severityCfg.MaxRulesEvaluated = 100
		}
		if severityCfg.CacheTTLSeconds == 0 {
			severityCfg.CacheTTLSeconds = 30
		}
		if severityCfg.LLMConfidence == 0 {
			severityCfg.LLMConfidence = 0.7
		}

		var triagerOpts []severity.TriagerOption
		triagerOpts = append(triagerOpts, severity.WithAuditor(auditor))
		if deps.k8sDynClient != nil {
			triagerOpts = append(triagerOpts, severity.WithPodResolver(
				severity.NewK8sPodResolver(deps.k8sDynClient, logger.WithName("pod-resolver")),
			))
		}

		deps.Triager = severity.NewTriager(promClient, llmTriager, severityCfg, logger.WithName("severity-triage"), triagerOpts...)
		logger.Info("severity triage enabled", "prometheusURL", cfg.SeverityTriage.PrometheusURL,
			"podResolverEnabled", deps.k8sDynClient != nil)
	}

	return deps, nil
}

// newLLMTriagerFromConfig creates a provider-aware LLMTriager based on the resolved
// LLM configuration. Routes by provider + model family:
//   - vertex_ai + claude-* model → AnthropicTriager (Anthropic SDK + Vertex)
//   - vertex_ai + other model → GenAITriager (Google GenAI SDK)
//   - gemini → GenAITriager (Gemini API)
//   - anthropic → AnthropicTriager (direct Anthropic API)
func newLLMTriagerFromConfig(ctx context.Context, llmCfg types.LLMConfig, logger logr.Logger) (severity.LLMTriager, error) {
	switch llmCfg.Provider {
	case types.LLMProviderVertexAI:
		if severity.IsAnthropicModel(llmCfg.Model) {
			return newAnthropicTriagerForVertex(ctx, llmCfg, logger)
		}
		return newGenAITriagerForVertex(ctx, llmCfg, logger)
	case types.LLMProviderGemini:
		return newGenAITriagerForGemini(ctx, llmCfg, logger)
	case types.LLMProviderAnthropic:
		return newAnthropicTriagerDirect(ctx, llmCfg, logger)
	default:
		return nil, fmt.Errorf("unsupported triage LLM provider: %q", llmCfg.Provider)
	}
}

func newGenAITriagerForVertex(ctx context.Context, llmCfg types.LLMConfig, logger logr.Logger) (severity.LLMTriager, error) {
	clientCfg := &genai.ClientConfig{
		Project:  llmCfg.VertexProject,
		Location: llmCfg.VertexLocation,
		Backend:  genai.BackendVertexAI,
	}
	if llmCfg.Endpoint != "" {
		clientCfg.HTTPOptions = genai.HTTPOptions{BaseURL: llmCfg.Endpoint}
	}
	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("vertex_ai GenAI client: %w", err)
	}
	return severity.NewGenAITriager(severity.GenAITriagerConfig{
		Client: client,
		Model:  llmCfg.Model,
		Logger: logger,
	}), nil
}

func newGenAITriagerForGemini(ctx context.Context, llmCfg types.LLMConfig, logger logr.Logger) (severity.LLMTriager, error) {
	clientCfg := &genai.ClientConfig{
		APIKey:  llmCfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	}
	if llmCfg.Endpoint != "" {
		clientCfg.HTTPOptions = genai.HTTPOptions{BaseURL: llmCfg.Endpoint}
	}
	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("gemini GenAI client: %w", err)
	}
	return severity.NewGenAITriager(severity.GenAITriagerConfig{
		Client: client,
		Model:  llmCfg.Model,
		Logger: logger,
	}), nil
}

func newAnthropicTriagerForVertex(ctx context.Context, llmCfg types.LLMConfig, logger logr.Logger) (severity.LLMTriager, error) {
	client, err := severity.NewAnthropicVertexClient(ctx, llmCfg.VertexProject, llmCfg.VertexLocation)
	if err != nil {
		return nil, fmt.Errorf("vertex_ai Anthropic client: %w", err)
	}
	return severity.NewAnthropicTriager(severity.AnthropicTriagerConfig{
		Client: client,
		Model:  llmCfg.Model,
		Logger: logger,
	}), nil
}

func newAnthropicTriagerDirect(_ context.Context, llmCfg types.LLMConfig, logger logr.Logger) (severity.LLMTriager, error) {
	client, err := severity.NewAnthropicDirectClient(llmCfg.APIKey)
	if err != nil {
		return nil, fmt.Errorf("anthropic direct client: %w", err)
	}
	return severity.NewAnthropicTriager(severity.AnthropicTriagerConfig{
		Client: client,
		Model:  llmCfg.Model,
		Logger: logger,
	}), nil
}

// triageLLMSource returns a human-readable label indicating whether the triage
// LLM config was explicitly set or inherited from the agent.
func triageLLMSource(cfg *config.Config) string {
	if cfg.SeverityTriage.LLM != nil {
		return "severityTriage.llm (explicit)"
	}
	return "agent.llm (inherited)"
}

func buildMCPHandler(d *handlerDeps) (http.Handler, func() bool, error) {
	var sessFinalizer handler.ISPhaseFinalizer
	var sessInitializer handler.ISSessionInitializer
	if d.SessInfra != nil && d.SessInfra.SessionService != nil {
		sessFinalizer = d.SessInfra.SessionService
		sessInitializer = d.SessInfra.SessionService
	}
	bridgeCfg := &handler.MCPBridgeConfig{
		K8sClient:             d.Backends.K8sClient(),
		TypedClient:           d.Backends.TypedClient(),
		Namespace:             d.Cfg.Session.Namespace,
		KAMCPClient:           d.Backends.MCPClient,
		KADedicatedClient:     d.Backends.DedicatedClient,
		InvestigationRegistry: d.Backends.InvestigationRegistry,
		DSClient:              d.Backends.DSClient,
		PromClient:            d.Backends.PromClient,
		Triager:               d.Backends.Triager,
		Authorizer:            d.Authorizer,
		Auditor:               d.Auditor,
		Logger:                d.Logger.WithName("bridge"),
		Metrics:               bridgeMetricsFrom(d.MetricsReg),
		ToolTimeout:           d.Cfg.MCP.ToolTimeout,
		ToolTimeouts:          d.Cfg.MCP.ToolTimeouts,
		MaxConcurrentTools:    10,
		UserLimiter:           d.UserLimiter,
		SessionFinalizer:      sessFinalizer,
		SessionInitializer:    sessInitializer,
		InteractiveEnabled:    d.Cfg.Interactive.Enabled,
		ActiveContextRegistry: d.ActiveCtxRegistry,
		RESTMapper:            d.Backends.Mapper,
	}

	mcpSessionTimeout := d.Cfg.MCP.SessionIdleTimeout
	if mcpSessionTimeout == 0 {
		mcpSessionTimeout = 30 * time.Minute
	}
	h, err := handler.NewMCPHandler(handler.MCPConfig{
		ServerName:     "kubernaut-apifrontend",
		ServerVersion:  version(),
		Enabled:        d.Cfg.MCP.Enabled,
		Bridge:         bridgeCfg,
		Auditor:        d.Auditor,
		SessionTimeout: mcpSessionTimeout,
	})
	if err != nil {
		return nil, nil, err
	}

	depsReady := handler.AllReady(
		d.Backends.KAClient.Healthy,
		d.Backends.DSResilientTransport.Healthy,
	)
	return h, depsReady, nil
}

// buildA2AHandler creates the A2A JSON-RPC handler when an LLM provider is
// configured. Returns a 501 stub when provider is empty, preserving backward
// compatibility for deployments that don't set it.
//
// The LLM model and transport chain are built once at startup and are NOT
// reloaded when the ConfigMap changes. Changes to agent.llm fields require
// a pod restart (consistent with KA's LLM wiring pattern).
func buildA2AHandler(ctx context.Context, d *handlerDeps) (http.Handler, error) {
	if d.Cfg.Agent.LLM.Provider == "" {
		d.Logger.Info("LLM provider not configured — A2A handler returns 501")
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "A2A not configured", http.StatusNotImplemented)
		}), nil
	}

	llmModel, err := launcher.NewModelFromConfig(ctx, d.Cfg.Agent.LLM)
	if err != nil {
		return nil, fmt.Errorf("create LLM model: %w", err)
	}

	hasCustomTransport := d.Cfg.Agent.LLM.TLSCaFile != "" || d.Cfg.Agent.LLM.OAuth2.Enabled
	if hasCustomTransport && (d.Cfg.Agent.LLM.Provider == types.LLMProviderVertexAI || d.Cfg.Agent.LLM.Provider == types.LLMProviderAnthropic) {
		d.Logger.Info("WARNING: mTLS/OAuth2 transport config is set but CANNOT be applied to " + d.Cfg.Agent.LLM.Provider +
			" — upstream ADK wrapper lacks HTTP client injection (blocked by issue #1342)")
	}

	var sessionSvcForAgent *session.CRDSessionService
	if d.SessInfra != nil {
		sessionSvcForAgent = d.SessInfra.SessionService
	}

	rootAgent, _, err := agentpkg.NewRootAgent(agentpkg.AgentConfig{
		Instruction:           agentpkg.BuildInstruction(d.Cfg.Session.Namespace),
		InstructionProvider:   agentpkg.NewInstructionProvider(d.Cfg.Session.Namespace),
		LLMModel:              llmModel,
		Namespace:             d.Cfg.Session.Namespace,
		K8sClient:             d.Backends.K8sClient(),
		TypedClient:           d.Backends.TypedClient(),
		DSClient:              d.Backends.DSClient,
		MCPClient:             d.Backends.MCPClient,
		DedicatedClient:       d.Backends.DedicatedClient,
		InvestigationRegistry: d.Backends.InvestigationRegistry,
		Pool:                  d.Backends.Pool,
		Authorizer:            d.Authorizer,
		Auditor:               d.Auditor,
		Triager:               d.Backends.Triager,
		RESTMapper:            d.Backends.Mapper,
		SessionService:        sessionSvcForAgent,
		ToolCallsTotal:        d.MetricsReg.ToolCallsTotal,
		ToolCallDuration:      d.MetricsReg.ToolCallDuration,
		UserLimiter:           d.UserLimiter,
		ActiveContextRegistry: d.ActiveCtxRegistry,
		InteractiveEnabled:    d.Cfg.Interactive.Enabled,
		PromClient:            d.Backends.PromClient,
	})
	if err != nil {
		return nil, fmt.Errorf("create root agent: %w", err)
	}

	var sessionSvc adksession.Service
	if d.SessInfra != nil && d.SessInfra.SessionService != nil {
		sessionSvc = session.NewServiceDecorator(d.SessInfra.SessionService)
	} else {
		sessionSvc = adksession.InMemoryService()
	}

	llmSemaphore := ratelimit.NewLLMSemaphore(d.Cfg.RateLimit.MaxConcurrentSessions)
	a2aCfg := launcher.A2AConfig{
		Agent:               rootAgent,
		SessionService:      sessionSvc,
		AppName:             "kubernaut-apifrontend",
		Logger:              d.Logger.WithName("a2a-launcher"),
		Auditor:             d.Auditor,
		BridgeMetrics:       d.MetricsReg,
		SessionPhaseUpdater: sessionSvcForAgent,
		SessionInterceptor: launcher.NewSessionInterceptor(
			d.ActiveCtxRegistry, d.Logger.WithName("session-interceptor"),
		),
		LLMSemaphore: llmSemaphore,
	}

	h, err := launcher.NewA2AHandler(a2aCfg)
	if err != nil {
		return nil, fmt.Errorf("create A2A handler: %w", err)
	}

	d.Logger.Info("A2A handler wired with LLM backend",
		"provider", d.Cfg.Agent.LLM.Provider,
		"model", d.Cfg.Agent.LLM.Model,
	)
	return h, nil
}

// buildResilientTransport wraps a base transport with retry + circuit breaker.
// ConnectTimeout is applied via net.Dialer on the underlying http.Transport.
// Returns the CB transport for health checking.
func buildResilientTransport(base http.RoundTripper, depCfg *config.DependencyConfig, name string, reg *metrics.Registry, auditor audit.Emitter) *resilience.CircuitBreakerTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	if depCfg.ConnectTimeout > 0 {
		if t, ok := base.(*http.Transport); ok {
			t = t.Clone()
			t.DialContext = (&net.Dialer{Timeout: depCfg.ConnectTimeout}).DialContext
			base = t
		}
	}
	retryRT := resilience.NewRetryTransport(base, &resilience.RetryConfig{
		MaxAttempts:       depCfg.RetryMax + 1,
		InitialBackoff:    depCfg.RetryInitBackoff,
		MaxBackoff:        depCfg.RetryMaxBackoff,
		RetryableStatuses: depCfg.RetryableStatuses,
		DependencyName:    name,
	})
	cbMaxReqs := depCfg.CBMaxRequests
	if cbMaxReqs == 0 {
		cbMaxReqs = 1
	}
	cbInterval := depCfg.CBInterval
	if cbInterval == 0 {
		cbInterval = 30 * time.Second
	}
	cbTimeout := depCfg.CBTimeout
	if cbTimeout == 0 {
		cbTimeout = 10 * time.Second
	}
	cbFailureThreshold := depCfg.CBFailureThreshold
	if cbFailureThreshold == 0 {
		cbFailureThreshold = 5
	}
	cbt := resilience.NewCircuitBreakerTransport(retryRT, &resilience.CircuitBreakerConfig{
		Name:             name,
		DependencyName:   name,
		MaxRequests:      cbMaxReqs,
		Interval:         cbInterval,
		Timeout:          cbTimeout,
		FailureThreshold: cbFailureThreshold,
		StateGauge:       reg.CircuitBreakerState,
		DurationHist:     reg.DownstreamDuration,
		AuditFunc:        resilience.CircuitBreakerAuditFunc(auditor),
	})
	return cbt
}

// bridgeMetricsFrom wires the global metrics registry counters into
// the bridge metrics struct — single instances shared across the process.
func bridgeMetricsFrom(reg *metrics.Registry) *handler.MCPBridgeMetrics {
	return &handler.MCPBridgeMetrics{
		ToolCallsTotal:   reg.ToolCallsTotal,
		ToolCallDuration: reg.ToolCallDuration,
	}
}

// replayCacheTTL matches (and slightly exceeds) the maximum expected token
// lifetime so replayed tokens cannot outlive their own cache entry.
const replayCacheTTL = 10 * time.Minute

// buildReplayCache constructs the jti replay-detection store selected by
// cfg (GAP-08, #1505). When cfg specifies a distributed "redis"/"valkey"
// backend, replay state is shared across all APIFrontend replicas via Valkey;
// if that backend cannot be constructed (bad address, unreadable credentials),
// it falls back to the legacy single-process in-memory cache rather than
// disabling replay protection outright — logged loudly so the HA degradation
// is observable. legacyEnable preserves the pre-GAP-08 boolean toggle for
// configs that predate the structured auth.replayCache block.
func buildReplayCache(cfg *config.ReplayCacheConfig, legacyEnable bool, logger logr.Logger) auth.ReplayCacheStore {
	if cfg == nil {
		if legacyEnable {
			return auth.NewReplayCache(replayCacheTTL)
		}
		return nil
	}
	if !cfg.IsDistributed() {
		return auth.NewReplayCache(replayCacheTTL)
	}
	rc, err := newValkeyReplayCache(cfg, logger)
	if err != nil {
		logger.Error(err, "failed to initialize valkey replay cache; falling back to in-memory cache (HA replay detection degraded)",
			"redisAddr", cfg.RedisAddr)
		return auth.NewReplayCache(replayCacheTTL)
	}
	logger.Info("auth mode: distributed replay cache (valkey)", "redisAddr", cfg.RedisAddr, "redisDB", cfg.RedisDB)
	return rc
}

// newValkeyReplayCache builds a Redis client from cfg and wraps it in a
// ValkeyReplayCache. Credentials are optional: an empty CredentialsPath
// connects without authentication (dev/test Valkey instances).
func newValkeyReplayCache(cfg *config.ReplayCacheConfig, logger logr.Logger) (*auth.ValkeyReplayCache, error) {
	password, err := loadReplayCachePassword(cfg.CredentialsPath)
	if err != nil {
		return nil, fmt.Errorf("load replay cache credentials: %w", err)
	}
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: password,
		DB:       cfg.RedisDB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping valkey at %s: %w", cfg.RedisAddr, err)
	}
	return auth.NewValkeyReplayCache(client, replayCacheTTL, logger), nil
}

// loadReplayCachePassword reads the "password" key from a YAML credentials
// file mounted from a Kubernetes Secret (same "password" key convention as
// DataStorage's valkey-secrets.yaml projection). Returns "" without error
// when path is empty (unauthenticated Valkey).
func loadReplayCachePassword(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // path from trusted, operator-controlled config
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	var secrets map[string]string
	if err := yaml.Unmarshal(data, &secrets); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	password, ok := secrets["password"]
	if !ok {
		return "", fmt.Errorf(`%s: missing required "password" key`, path)
	}
	return password, nil
}

func buildAuthMiddleware(cfg *config.Config, reg *metrics.Registry, auditor audit.Emitter, logger logr.Logger) (func(http.Handler) http.Handler, handler.ReadyChecker) {
	alwaysReady := handler.ReadyChecker(func() bool { return true })

	authCfg := buildAuthConfig(cfg)

	var validatorOpts []auth.JWTValidatorOption

	if len(authCfg.JWT) == 0 {
		restCfg, k8sErr := ctrl.GetConfig()
		if k8sErr != nil {
			logger.Error(k8sErr, "CRITICAL: no auth issuer configured and kubeconfig unavailable — denying all authenticated requests (AF-CRIT-1)")
			denyAll := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "authentication system unavailable", http.StatusServiceUnavailable)
				})
			}
			notReady := handler.ReadyChecker(func() bool { return false })
			return denyAll, notReady
		}
		k8sClient, k8sErr := kubernetes.NewForConfig(restCfg)
		if k8sErr != nil {
			logger.Error(k8sErr, "CRITICAL: failed to create kubernetes client for TokenReview — denying all authenticated requests (AF-CRIT-1)")
			denyAll := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "authentication system unavailable", http.StatusServiceUnavailable)
				})
			}
			notReady := handler.ReadyChecker(func() bool { return false })
			return denyAll, notReady
		}
		validatorOpts = append(validatorOpts, auth.WithTokenReviewer(auth.NewTokenReviewer(k8sClient)))
		logger.Info("auth mode: TokenReview (no OIDC issuer configured)")
	} else {
		if cfg.Auth.OIDCCaFile != "" {
			httpClient, err := buildOIDCHTTPClient(cfg.Auth.OIDCCaFile)
			if err != nil {
				logger.Error(err, "failed to build OIDC HTTP client with custom CA")
				return func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						http.Error(w, "authentication system unavailable", http.StatusServiceUnavailable)
					})
				}, alwaysReady
			}
			validatorOpts = append(validatorOpts, auth.WithHTTPClient(httpClient))
			logger.Info("OIDC JWKS fetcher configured with custom CA", "caFile", cfg.Auth.OIDCCaFile)
		}
		if rc := buildReplayCache(cfg.Auth.ReplayCache, cfg.Auth.EnableReplayProtection, logger); rc != nil {
			validatorOpts = append(validatorOpts, auth.WithReplayCache(rc))
		}
		logger.Info("auth mode: OIDC/JWKS", "providers", len(authCfg.JWT))
	}
	providerLimiter := ratelimit.NewProviderLimiter(ratelimit.PerProviderConfig{
		FetchIntervalSeconds: 300,
	})
	validatorOpts = append(validatorOpts, auth.WithProviderLimiter(providerLimiter))
	validatorOpts = append(validatorOpts, auth.WithCBMetrics(reg.CircuitBreakerState))
	validator, err := auth.NewJWTValidator(authCfg, validatorOpts...)
	if err != nil {
		logger.Error(err, "failed to create JWT validator — falling back to deny-all")
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "authentication system unavailable", http.StatusServiceUnavailable)
			})
		}, alwaysReady
	}

	mw := auth.MiddlewareWithConfig(auth.MiddlewareConfig{
		Validator:    validator,
		Logger:       logger,
		Auditor:      auditor,
		AuthDuration: reg.AuthDuration,
	})
	return mw, validator.Ready
}

// buildOIDCHTTPClient creates an HTTP client that trusts the system CAs plus
// the additional CA bundle at caFile. Used to reach OIDC providers whose TLS
// certificate is signed by a non-public CA (e.g., OpenShift ingress operator).
func buildOIDCHTTPClient(caFile string) (*http.Client, error) {
	caPEM, err := os.ReadFile(caFile) //nolint:gosec // path from trusted config
	if err != nil {
		return nil, fmt.Errorf("reading OIDC CA file %s: %w", caFile, err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no valid certificates found in %s", caFile)
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}, nil
}

func parseLogLevel(s string) (zapcore.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return zapcore.InfoLevel, nil
	case "debug":
		return zapcore.DebugLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("unsupported log level: %q", s)
	}
}

// buildAuthConfig maps config.AuthConfig to auth.Config.
// Priority: jwtProviders[] > legacy issuerURL > empty (TokenReview auto-detect).
func buildAuthConfig(cfg *config.Config) auth.Config {
	if len(cfg.Auth.JWTProviders) > 0 {
		providers := make([]auth.ProviderConfig, 0, len(cfg.Auth.JWTProviders))
		for _, p := range cfg.Auth.JWTProviders {
			providers = append(providers, auth.ProviderConfig{
				Issuer: auth.IssuerConfig{
					URL:       p.IssuerURL,
					JWKSURL:   p.JWKSURL,
					Audiences: p.Audiences,
				},
				ClaimMappings: auth.ClaimMappings{
					Username: p.ClaimMappings.Username,
					Groups:   p.ClaimMappings.Groups,
				},
			})
		}
		return auth.Config{
			JWT:                  providers,
			AllowInsecureIssuers: cfg.Auth.AllowInsecureIssuers,
		}
	}
	if cfg.Auth.IssuerURL != "" {
		return auth.Config{
			JWT: []auth.ProviderConfig{
				{
					Issuer: auth.IssuerConfig{
						URL:       cfg.Auth.IssuerURL,
						JWKSURL:   cfg.Auth.JWKSURL,
						Audiences: []string{cfg.Auth.Audience},
					},
				},
			},
			AllowInsecureIssuers: cfg.Auth.AllowInsecureIssuers,
		}
	}
	return auth.Config{}
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

// sessionInfra bundles the session-management components that buildSessionInfra
// produces. All fields are safe to use from multiple goroutines once built.
type sessionInfra struct {
	SessionService *session.CRDSessionService
	Reconciler     *controller.SessionCleanupReconciler
	Scheme         *k8sruntime.Scheme
	Healthy        *atomic.Bool
	StopFunc       func()
}

// buildSessionInfra creates the CRDSessionService, registers the
// InvestigationSession scheme, and instantiates the TTL reconciler.
// It creates a real ctrl.Manager, registers field indexes and reconcilers,
// and starts the manager in a goroutine.
func buildSessionInfra(cfg *config.Config, reg *metrics.Registry, auditor audit.Emitter, logger logr.Logger) (*sessionInfra, error) {
	scheme := k8sruntime.NewScheme()
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register coordination scheme: %w", err)
	}
	if err := isv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register InvestigationSession scheme: %w", err)
	}

	for _, phase := range []string{"Active", "Disconnected", "Completed", "Cancelled", "Failed"} {
		reg.SessionsActive.WithLabelValues(phase)
	}
	for _, action := range []string{"cancel", "delete"} {
		reg.SessionTTLActions.WithLabelValues(action)
	}

	restCfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("get kubeconfig: %w", err)
	}

	preflightSessionChecks(restCfg, cfg.Session.Namespace, auditor, logger)

	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				cfg.Session.Namespace: {},
			},
		},
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "",
		LeaderElection:         false,
	})
	if err != nil {
		return nil, fmt.Errorf("create session controller manager: %w", err)
	}

	if err := session.RegisterFieldIndexes(context.Background(), mgr.GetFieldIndexer()); err != nil {
		return nil, fmt.Errorf("register InvestigationSession field index: %w", err)
	}

	k8sClient := mgr.GetClient()

	svc := session.NewCRDSessionService(
		adksession.InMemoryService(),
		k8sClient,
		scheme,
		cfg.Session.Namespace,
		session.WithAuditor(auditor),
		session.WithSessionsActive(reg.SessionsActive),
		session.WithAPIReader(mgr.GetAPIReader()),
		session.WithLogger(logger.WithName("session-service")),
	)

	reconciler := controller.NewSessionCleanupReconciler(
		k8sClient,
		cfg.Session.DisconnectTTL,
		cfg.Session.RetentionTTL,
		logger.WithName("session-cleanup"),
		auditor,
		reg.SessionTTLActions,
		svc,
	)

	leaseSync := controller.NewLeaseSyncReconciler(
		k8sClient,
		cfg.Session.Namespace,
		logger.WithName("lease-sync"),
	)

	if err := reconciler.SetupWithManager(mgr); err != nil {
		return nil, fmt.Errorf("register session reconciler: %w", err)
	}
	if err := leaseSync.SetupWithManager(mgr); err != nil {
		return nil, fmt.Errorf("register lease-sync reconciler: %w", err)
	}

	healthy := &atomic.Bool{}
	mgrCtx, mgrCancel := context.WithCancel(context.Background()) //nolint:gosec // G118 false positive: mgrCancel is assigned to stopFunc below
	go func() {
		defer healthy.Store(false)
		if startErr := mgr.Start(mgrCtx); startErr != nil {
			logger.Error(startErr, "session controller manager exited with error — health degraded")
		}
	}()
	go func() {
		syncCtx, syncCancel := context.WithTimeout(mgrCtx, 60*time.Second)
		defer syncCancel()
		if mgr.GetCache().WaitForCacheSync(syncCtx) {
			healthy.Store(true)
			logger.Info("session controller cache synced")
		} else {
			logger.Error(nil, "session controller cache sync failed — session health degraded")
		}
	}()

	logger.Info("session controller manager started",
		"namespace", cfg.Session.Namespace,
		"disconnectTTL", cfg.Session.DisconnectTTL.String(),
		"retentionTTL", cfg.Session.RetentionTTL.String(),
	)

	return &sessionInfra{
		SessionService: svc,
		Reconciler:     reconciler,
		Scheme:         scheme,
		Healthy:        healthy,
		StopFunc:       mgrCancel,
	}, nil
}

// preflightSessionChecks runs diagnostic checks before starting the session
// controller manager. These are non-blocking so a misconfigured cluster still
// boots the AF; SREs can diagnose from the log output and audit trail.
func preflightSessionChecks(restCfg *rest.Config, namespace string, auditor audit.Emitter, logger logr.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	gvr := "investigationsessions.kubernaut.ai/v1alpha1"

	dc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		logger.Error(err, "pre-flight: failed to create discovery client")
		return
	}
	resources, err := dc.ServerResourcesForGroupVersion("kubernaut.ai/v1alpha1")
	crdFound := false
	if err == nil {
		for _, r := range resources.APIResources {
			if r.Name == "investigationsessions" {
				crdFound = true
				break
			}
		}
	}
	logger.Info("pre-flight CRD discovery", "gvr", gvr, "available", crdFound)
	if !crdFound {
		logger.Info("WARNING: InvestigationSession CRD not found — session controller may fail to start")
	}
	if auditor != nil {
		auditor.Emit(ctx, &audit.Event{
			Type: audit.EventPreflightCRDCheck,
			Detail: map[string]string{
				"gvr":       gvr,
				"available": fmt.Sprintf("%t", crdFound),
			},
		})
	}

	k8s, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		logger.Error(err, "pre-flight: failed to create kubernetes client for SSAR")
		return
	}

	// AC-6: Check all verbs the session controller and CRDSessionService need.
	requiredVerbs := []string{"get", "list", "watch", "create", "update", "delete"}
	allAllowed := true
	var deniedVerbs []string
	for _, verb := range requiredVerbs {
		ssar := &authorizationv1.SelfSubjectAccessReview{
			Spec: authorizationv1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Namespace: namespace,
					Verb:      verb,
					Group:     "kubernaut.ai",
					Resource:  "investigationsessions",
				},
			},
		}
		result, err := k8s.AuthorizationV1().SelfSubjectAccessReviews().Create(
			ctx, ssar, metav1.CreateOptions{},
		)
		if err != nil {
			logger.Error(err, "pre-flight RBAC check failed", "verb", verb)
			allAllowed = false
			deniedVerbs = append(deniedVerbs, verb+"(error)")
			continue
		}
		if !result.Status.Allowed {
			allAllowed = false
			deniedVerbs = append(deniedVerbs, verb)
		}
		logger.Info("pre-flight RBAC check",
			"verb", verb,
			"resource", "investigationsessions",
			"namespace", namespace,
			"allowed", result.Status.Allowed,
		)
	}
	if !allAllowed {
		logger.Info("WARNING: ServiceAccount lacks permissions on investigationsessions — session controller may fail",
			"denied_verbs", strings.Join(deniedVerbs, ","),
		)
	}
	if auditor != nil {
		auditor.Emit(ctx, &audit.Event{
			Type: audit.EventPreflightRBACCheck,
			Detail: map[string]string{
				"resource":     "investigationsessions",
				"namespace":    namespace,
				"all_allowed":  fmt.Sprintf("%t", allAllowed),
				"denied_verbs": strings.Join(deniedVerbs, ","),
			},
		})
	}
}
