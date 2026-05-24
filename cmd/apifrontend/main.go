package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	adksession "google.golang.org/adk/session"
	"gopkg.in/yaml.v3"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/internal/controller/apifrontend"
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
)

const (
	configPath     = "/etc/apifrontend/config.yaml"
	defaultHealthz = ":8081"
	defaultMetrics = ":9090"
)

func main() { os.Exit(run()) }

func run() int {
	cfg, err := loadConfig()
	if err != nil {
		z, _ := zap.NewProduction()
		z.Error("failed to load config", zap.Error(err))
		return 1
	}
	if err := cfg.ResolveDefaults(); err != nil {
		z, _ := zap.NewProduction()
		z.Error("failed to resolve config defaults", zap.Error(err))
		return 1
	}

	logLevel, _ := parseLogLevel(cfg.Logging.Level)
	zapLogger := newZapLogger(logLevel)
	defer func() { _ = zapLogger.Sync() }()
	logger := zapr.NewLogger(zapLogger).WithName("apifrontend")

	if err := cfg.Validate(); err != nil {
		logger.Error(err, "invalid config")
		return 1
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

	sessInfra := buildSessionInfra(cfg, metricsReg, auditor, logger)
	defer sessInfra.StopFunc()

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

	mcpHandler, depsReady, err := buildMCPHandler(cfg, deps, metricsReg, sarChecker, auditor, logger, userLimiter)
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
	}, config.WithAuditor(auditor))
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
		Name:        cfg.AgentCard.Name,
		Description: "Kubernaut AI-driven remediation API Frontend",
		URL:         cfg.AgentCard.URL,
		Version:     version(),
		Skills:      handler.DefaultAgentSkills(),
	})
	if err != nil {
		logger.Error(err, "failed to create agent card handler")
		return 1
	}
	agentCardHandler := handler.WithAgentCardAudit(agentCardBase, auditor)

	a2aHandler, err := buildA2AHandler(ctx, cfg, deps, sessInfra, metricsReg, sarChecker, auditor, logger, userLimiter)
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
	routerCfg := handler.RouterConfig{
		MetricsRegistry:    metricsReg,
		Logger:             logger,
		A2AHandler:         a2aHandler,
		MCPHandler:         mcpHandler,
		AgentCardHandler:   agentCardHandler,
		AuthMiddleware:     authMiddleware,
		PreAuthMiddleware:  preAuthMW,
		PostAuthMiddleware: postAuthMW,
		ReadyChecker:       handler.AllReady(func() bool { return !draining.Load() }, depsReady, authReady),
		SSETracker:         buildSSETracker(cfg, metricsReg),
		Draining:           draining,
	}
	router, err := handler.NewRouter(routerCfg)
	if err != nil {
		logger.Error(err, "failed to create router")
		return 1
	}

	addr := fmt.Sprintf(":%d", cfg.Server.Port)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}

	healthMux := buildHealthMux(handler.AllReady(depsReady, authReady), draining)
	healthServer := &http.Server{
		Addr:              defaultHealthz,
		Handler:           healthMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", metricsReg.Handler())
	metricsServer := &http.Server{
		Addr:              defaultMetrics,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	tlsEnabled, certReloader, err := tlswiring.ConfigureServer(httpServer, cfg.Server.TLS.CertDir)
	if err != nil {
		logger.Error(err, "failed to configure TLS")
		return 1
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
			logger.Error(fmt.Errorf("TLS required but no certificates found"), "server.tls.required is true but certDir is empty or missing certs")
			return 1
		}
		logger.Info("WARNING: TLS disabled, serving plain HTTP — not suitable for FedRAMP production")
	}

	certWatcher, err := tlswiring.StartCertFileWatcher(ctx, cfg.Server.TLS.CertDir, certReloader, logger)
	if err != nil {
		logger.Error(err, "failed to start certificate file watcher")
		return 1
	}
	if certWatcher != nil {
		defer certWatcher.Stop()
	}

	caWatcher, err := tlswiring.StartCAFileWatcher(ctx, logger)
	if err != nil {
		logger.Error(err, "failed to start CA file watcher")
		return 1
	}
	if caWatcher != nil {
		defer caWatcher.Stop()
	}

	go startServerTLS(httpServer, tlsEnabled, "API", logger)
	go startServer(healthServer, "health", logger)
	go startServer(metricsServer, "metrics", logger)

	logger.Info("kubernaut-apifrontend started",
		"addr", addr, "tls", tlsEnabled, "mcp_enabled", cfg.MCP.Enabled, "tools", 20)

	<-ctx.Done()
	draining.Store(true)
	logger.Info("shutting down...")

	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout(cfg))
	defer cancel()

	if tracker := routerCfg.SSETracker; tracker != nil {
		tracker.DrainAll(shutCtx)
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
	ipLimiter.Stop()
	userLimiter.Stop()

	logger.Info("shutdown complete")
	return 0
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

// backendDeps holds shared backend clients used by both the MCP and A2A handlers.
// Created once by buildBackendDeps and consumed by buildMCPHandler / buildA2AHandler.
type backendDeps struct {
	DSClient             ds.Client
	KAClient             *ka.Client
	MCPClient            ka.MCPClient
	Pool                 *ka.KASessionPool
	Triager              *severity.Triager
	DSResilientTransport *resilience.CircuitBreakerTransport
	CAWatchers           []caWatcherEntry
	k8sDynClient         dynamic.Interface
	k8sOnce              sync.Once
}

// K8sClient returns the pod service-account scoped dynamic K8s client.
// Created lazily on first call via the in-cluster config. Thread-safe via sync.Once.
func (d *backendDeps) K8sClient() dynamic.Interface {
	d.k8sOnce.Do(func() {
		restCfg, err := ctrl.GetConfig()
		if err != nil {
			return
		}
		c, err := dynamic.NewForConfig(restCfg)
		if err != nil {
			return
		}
		d.k8sDynClient = c
	})
	return d.k8sDynClient
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
	var jwtDelegation http.RoundTripper = &auth.ContextJWTDelegationTransport{Base: kaMCPResilient}
	jwtDelegation = &auth.AuditingJWTDelegationTransport{Base: jwtDelegation, Auditor: auditor}
	kaMCPHTTPClient := &http.Client{Transport: jwtDelegation}
	mcpClient := ka.NewSDKMCPClient(
		cfg.Agent.KAMCPEndpoint,
		kaMCPHTTPClient,
		logger,
	)
	mcpClient.WithDownstreamDuration(metricsReg.DownstreamDuration)
	deps.MCPClient = mcpClient

	// G2 (deferred): Pool is constructed for shutdown wiring (DrainAll) but
	// interactive tools currently use session-per-call via SDKMCPClient.
	// The factory is a placeholder until G2 persistent sessions are
	// implemented. Calling Pool.Acquire will fail until a real factory
	// is provided. See pkg/apifrontend/ka/mcp_sdk_client.go for the
	// session-per-call rationale (P2 Architect finding, DD-AUTH-MCP-001 v2.0).
	deps.Pool = ka.NewKASessionPool(ka.PoolConfig{
		Factory: func(ctx context.Context) (ka.PoolSession, error) {
			return nil, fmt.Errorf("pool session factory not yet configured (G2 deferred)")
		},
		MaxEntries: 100,
		IdleTTL:    10 * time.Minute,
		Logger:     logger.WithName("ka-session-pool"),
	})

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

		promHTTPClient := &http.Client{Transport: promTransport}
		if cfg.SeverityTriage.PrometheusBearerTokenFile != "" {
			promHTTPClient.Transport = &bearerTokenTransport{
				base:      promTransport,
				tokenFile: cfg.SeverityTriage.PrometheusBearerTokenFile,
			}
		}

		promClient := prom.NewHTTPClient(cfg.SeverityTriage.PrometheusURL, promHTTPClient)

		llmTriager := severity.LLMTriager(severity.NewNoopLLMTriager(logger.WithName("llm-triage")))

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

		deps.Triager = severity.NewTriager(promClient, llmTriager, severityCfg, logger.WithName("severity-triage"))
		logger.Info("severity triage enabled", "prometheusURL", cfg.SeverityTriage.PrometheusURL)
	}

	deps.KAClient = ka.NewClient(ka.Config{
		BaseURL:            cfg.Agent.KABaseURL,
		BaseTransport:      kaTransport,
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

	return deps, nil
}

func buildMCPHandler(cfg *config.Config, deps *backendDeps, metricsReg *metrics.Registry, authorizer auth.ToolAuthorizer, auditor audit.Emitter, logger logr.Logger, userLimiter *ratelimit.UserLimiter) (http.Handler, func() bool, error) {
	bridgeCfg := &handler.MCPBridgeConfig{
		K8sClient:          deps.K8sClient(),
		KAClient:           deps.KAClient,
		KAMCPClient:        deps.MCPClient,
		Pool:               deps.Pool,
		DSClient:           deps.DSClient,
		Triager:            deps.Triager,
		Authorizer:         authorizer,
		Auditor:            auditor,
		Logger:             logger.WithName("bridge"),
		Metrics:            bridgeMetricsFrom(metricsReg),
		ToolTimeout:        cfg.MCP.ToolTimeout,
		ToolTimeouts:       cfg.MCP.ToolTimeouts,
		MaxConcurrentTools: 10,
		UserLimiter:        userLimiter,
	}

	mcpSessionTimeout := cfg.MCP.SessionIdleTimeout
	if mcpSessionTimeout == 0 {
		mcpSessionTimeout = 30 * time.Minute
	}
	h, err := handler.NewMCPHandler(handler.MCPConfig{
		ServerName:     "kubernaut-apifrontend",
		ServerVersion:  version(),
		Enabled:        cfg.MCP.Enabled,
		Bridge:         bridgeCfg,
		Auditor:        auditor,
		SessionTimeout: mcpSessionTimeout,
	})
	if err != nil {
		return nil, nil, err
	}

	depsReady := handler.AllReady(
		deps.KAClient.Healthy,
		deps.DSResilientTransport.Healthy,
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
func buildA2AHandler(ctx context.Context, cfg *config.Config, deps *backendDeps, sessInfra *sessionInfra, metricsReg *metrics.Registry, authorizer auth.ToolAuthorizer, auditor audit.Emitter, logger logr.Logger, userLimiter *ratelimit.UserLimiter) (http.Handler, error) {
	if cfg.Agent.LLM.Provider == "" {
		logger.Info("LLM provider not configured — A2A handler returns 501")
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "A2A not configured", http.StatusNotImplemented)
		}), nil
	}

	llmModel, err := launcher.NewModelFromConfig(ctx, cfg.Agent.LLM)
	if err != nil {
		return nil, fmt.Errorf("create LLM model: %w", err)
	}

	var sessionSvcForAgent *session.CRDSessionService
	if sessInfra != nil {
		sessionSvcForAgent = sessInfra.SessionService
	}
	rootAgent, _, err := agentpkg.NewRootAgent(agentpkg.AgentConfig{
		Instruction:      agentpkg.DefaultTestConfig().Instruction,
		LLMModel:         llmModel,
		K8sClient:        deps.K8sClient(),
		KAClient:         deps.KAClient,
		DSClient:         deps.DSClient,
		MCPClient:        deps.MCPClient,
		Authorizer:       authorizer,
		Auditor:          auditor,
		Triager:          deps.Triager,
		SessionService:   sessionSvcForAgent,
		ToolCallsTotal:   metricsReg.ToolCallsTotal,
		ToolCallDuration: metricsReg.ToolCallDuration,
		UserLimiter:      userLimiter,
	})
	if err != nil {
		return nil, fmt.Errorf("create root agent: %w", err)
	}

	var sessionSvc adksession.Service
	if sessInfra != nil && sessInfra.SessionService != nil {
		sessionSvc = session.NewServiceDecorator(sessInfra.SessionService)
	} else {
		sessionSvc = adksession.InMemoryService()
	}

	a2aCfg := launcher.A2AConfig{
		Agent:          rootAgent,
		SessionService: sessionSvc,
		AppName:        "kubernaut-apifrontend",
		Auditor:        auditor,
		BridgeMetrics:  metricsReg,
	}

	h, err := launcher.NewA2AHandler(a2aCfg)
	if err != nil {
		return nil, fmt.Errorf("create A2A handler: %w", err)
	}

	logger.Info("A2A handler wired with LLM backend",
		"provider", cfg.Agent.LLM.Provider,
		"model", cfg.Agent.LLM.Model,
	)
	return h, nil
}

// buildResilientTransport wraps a base transport with retry + circuit breaker.
// Returns the CB transport for health checking.
func buildResilientTransport(base http.RoundTripper, depCfg *config.DependencyConfig, name string, reg *metrics.Registry, auditor audit.Emitter) *resilience.CircuitBreakerTransport {
	if base == nil {
		base = http.DefaultTransport
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

func buildAuthMiddleware(cfg *config.Config, reg *metrics.Registry, auditor audit.Emitter, logger logr.Logger) (func(http.Handler) http.Handler, handler.ReadyChecker) {
	alwaysReady := handler.ReadyChecker(func() bool { return true })

	ac := buildAuthConfig(cfg)
	if len(ac.JWT) == 0 || ac.JWT[0].Issuer.URL == "" {
		logger.Info("WARNING: no auth issuer configured — using pass-through auth (not suitable for production)")
		return func(next http.Handler) http.Handler { return next }, alwaysReady
	}

	authCfg := auth.Config{
		JWT:                  make([]auth.ProviderConfig, 0, len(ac.JWT)),
		AllowInsecureIssuers: cfg.Auth.AllowInsecureIssuers,
	}
	for _, jp := range ac.JWT {
		authCfg.JWT = append(authCfg.JWT, auth.ProviderConfig{
			Issuer: auth.IssuerConfig{
				URL:       jp.Issuer.URL,
				JWKSURL:   jp.Issuer.JWKSURL,
				Audiences: jp.Issuer.Audiences,
			},
		})
	}

	var validatorOpts []auth.JWTValidatorOption
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
	if cfg.Auth.EnableReplayProtection {
		validatorOpts = append(validatorOpts, auth.WithReplayCache(auth.NewReplayCache(10*time.Minute)))
	}
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

// authConfig holds JWT auth provider configuration for the auth middleware.
type authConfig struct {
	JWT []jwtProvider
}

type jwtProvider struct {
	Issuer jwtIssuer
}

type jwtIssuer struct {
	URL       string
	JWKSURL   string
	Audiences []string
}

func buildAuthConfig(cfg *config.Config) authConfig {
	if cfg.Auth.IssuerURL == "" {
		return authConfig{}
	}
	return authConfig{
		JWT: []jwtProvider{
			{
				Issuer: jwtIssuer{
					URL:       cfg.Auth.IssuerURL,
					JWKSURL:   cfg.Auth.JWKSURL,
					Audiences: []string{cfg.Auth.Audience},
				},
			},
		},
	}
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
	StopFunc       func()
}

// buildSessionInfra creates the CRDSessionService, registers the
// InvestigationSession scheme, and instantiates the TTL reconciler.
// When a kubeconfig is available (in-cluster or KUBECONFIG env), it creates a
// real ctrl.Manager, registers the reconciler, and starts it in a goroutine.
// When no kubeconfig is available (unit tests), it falls back to a fake client.
func buildSessionInfra(cfg *config.Config, reg *metrics.Registry, auditor audit.Emitter, logger logr.Logger) *sessionInfra {
	scheme := k8sruntime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		logger.Error(err, "failed to register InvestigationSession scheme — session features will be unavailable")
	}

	for _, phase := range []string{"Active", "Disconnected", "Completed", "Cancelled", "Failed"} {
		reg.SessionsActive.WithLabelValues(phase)
	}

	var k8sClient client.Client
	var stopFunc func()

	restCfg, err := ctrl.GetConfig()
	if err == nil {
		mgr, mgrErr := ctrl.NewManager(restCfg, ctrl.Options{
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
		if mgrErr != nil {
			logger.Error(mgrErr, "failed to create session controller manager — falling back to in-memory")
			k8sClient, stopFunc = buildFakeSessionClient(scheme)
		} else {
			k8sClient = mgr.GetClient()

			svc := session.NewCRDSessionService(
				adksession.InMemoryService(),
				k8sClient,
				scheme,
				cfg.Session.Namespace,
				session.WithAuditor(auditor),
				session.WithSessionsActive(reg.SessionsActive),
				session.WithAPIReader(mgr.GetAPIReader()),
			)

			reconciler := controller.NewSessionCleanupReconciler(
				k8sClient,
				cfg.Session.DisconnectTTL,
				cfg.Session.RetentionTTL,
				auditor,
				nil,
				svc,
			)

			if setupErr := reconciler.SetupWithManager(mgr); setupErr != nil {
				logger.Error(setupErr, "failed to register session reconciler with manager")
				k8sClient, stopFunc = buildFakeSessionClient(scheme)
			} else {
				mgrCtx, mgrCancel := context.WithCancel(context.Background()) //nolint:gosec // G118 false positive: mgrCancel is assigned to stopFunc below
				go func() {
					if startErr := mgr.Start(mgrCtx); startErr != nil {
						logger.Error(startErr, "session controller manager exited with error")
					}
				}()
				stopFunc = mgrCancel
				logger.Info("session controller manager started",
					"namespace", cfg.Session.Namespace,
					"disconnectTTL", cfg.Session.DisconnectTTL.String(),
					"retentionTTL", cfg.Session.RetentionTTL.String(),
				)

				return &sessionInfra{
					SessionService: svc,
					Reconciler:     reconciler,
					Scheme:         scheme,
					StopFunc:       stopFunc,
				}
			}
		}
	} else {
		logger.Info("no kubeconfig available — session CRDs will use in-memory client",
			"reason", err.Error())
		k8sClient, stopFunc = buildFakeSessionClient(scheme)
	}

	svc := session.NewCRDSessionService(
		adksession.InMemoryService(),
		k8sClient,
		scheme,
		cfg.Session.Namespace,
		session.WithAuditor(auditor),
		session.WithSessionsActive(reg.SessionsActive),
	)

	reconciler := controller.NewSessionCleanupReconciler(
		k8sClient,
		cfg.Session.DisconnectTTL,
		cfg.Session.RetentionTTL,
		auditor,
		nil,
		svc,
	)

	return &sessionInfra{
		SessionService: svc,
		Reconciler:     reconciler,
		Scheme:         scheme,
		StopFunc:       stopFunc,
	}
}

func buildFakeSessionClient(scheme *k8sruntime.Scheme) (c client.Client, cleanup func()) {
	c = k8sfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.InvestigationSession{}).
		Build()
	cleanup = func() {}
	return c, cleanup
}
