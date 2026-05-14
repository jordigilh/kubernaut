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
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"

	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/config"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ds"
	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
	"github.com/jordigilh/kubernaut-apifrontend/internal/metrics"
	prom "github.com/jordigilh/kubernaut-apifrontend/internal/prometheus"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ratelimit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/resilience"
	"github.com/jordigilh/kubernaut-apifrontend/internal/severity"
	"github.com/jordigilh/kubernaut-apifrontend/internal/streaming"
	"github.com/jordigilh/kubernaut-apifrontend/internal/tlswiring"
)

const (
	configPath     = "/etc/apifrontend/config.yaml"
	rbacRolesPath  = "/etc/apifrontend/rbac_roles.yaml"
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
	cfg.ResolveDefaults()

	logLevel, _ := parseLogLevel(cfg.Logging.Level)
	zapLogger := newZapLogger(logLevel)
	defer func() { _ = zapLogger.Sync() }()
	logger := zapr.NewLogger(zapLogger).WithName("apifrontend")

	if err := cfg.Validate(); err != nil {
		logger.Error(err, "invalid config")
		return 1
	}

	rbacRoles, err := loadRBACRoles()
	if err != nil {
		logger.Error(err, "failed to load RBAC roles", "path", rbacRolesPath)
		return 1
	}

	metricsReg := metrics.NewRegistry()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// F-004 + DP-01: Wire audit emitter to DS-backed writer with CA-pinned transport.
	var auditWriter audit.Writer = &logAuditWriter{logger: logger.WithName("audit-writer")}
	if cfg.Agent.DSBaseURL != "" {
		auditDSTransport, auditDSWatcher, err := tlswiring.CAReloadableTransport(cfg.Agent.DSTLSCaFile, logger.WithName("ds-audit-ca"))
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
		dsCfg := ds.OgenClientConfig{
			BaseURL:   cfg.Agent.DSBaseURL,
			Timeout:   cfg.Resilience.DS.RequestTimeout,
			Transport: auditDSTransport,
		}
		dsAuditClient, err := ds.NewOgenClient(dsCfg)
		if err != nil {
			logger.Error(err, "DS audit client creation failed — refusing to start")
			return 1
		}
		auditWriter = &dsAuditWriterAdapter{client: dsAuditClient, logger: logger.WithName("ds-audit")}
		logger.Info("audit trail wired to Data Store backend", "dsURL", cfg.Agent.DSBaseURL)
	}

	auditor := audit.NewBufferedEmitter(audit.BufferConfig{
		Writer:          auditWriter,
		Logger:          logger,
		OverflowCounter: metricsReg.AuditBufferOverflow,
		EventsCounter:   metricsReg.AuditEventsTotal,
	})
	auditor.Start()

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

	mcpHandler, caWatchers, depsReady, err := buildMCPHandler(ctx, cfg, metricsReg, rbacRoles, auditor, logger, userLimiter)
	if err != nil {
		logger.Error(err, "failed to create MCP handler")
		return 1
	}
	defer func() {
		for _, w := range caWatchers {
			w.watcher.Stop()
		}
	}()

	// CM-02: Wire config file watcher for drift detection + audit trail.
	cfgWatcher, err := config.NewFileWatcher(configPath, func(newContent []byte) error {
		var newCfg config.Config
		if err := yaml.Unmarshal(newContent, &newCfg); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
		newCfg.ResolveDefaults()
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

	agentCardHandler, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
		Name:         "kubernaut-apifrontend",
		Description:  "Kubernaut AI-driven remediation API Frontend",
		URL:          cfg.AgentCard.URL,
		Version:      version(),
		Skills:       handler.DefaultAgentSkills(),
		RBACRoles:    handler.RBACRoles(rbacRoles),
		GroupMapping: handler.GroupMapping(cfg.RBAC.GroupMapping),
	})
	if err != nil {
		logger.Error(err, "failed to create agent card handler")
		return 1
	}

	a2aHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "A2A not configured", http.StatusNotImplemented)
	})

	// F-001: Wire JWT auth middleware (fall back to noop only when auth is unconfigured).
	authMiddleware := buildAuthMiddleware(cfg, metricsReg, auditor, logger)
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
		ReadyChecker:       handler.AllReady(func() bool { return !draining.Load() }, depsReady),
		SSETracker:         streaming.NewConnectionTracker(metricsReg.SSEActiveConnections, 5*time.Second),
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

	healthMux := buildHealthMux(depsReady, draining)
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
	shutdownServer(shutCtx, httpServer, "API", logger)
	shutdownServer(shutCtx, healthServer, "health", logger)
	shutdownServer(shutCtx, metricsServer, "metrics", logger)

	// F-008: Drain audit buffer before exit to prevent event loss.
	if err := auditor.Close(shutCtx); err != nil {
		logger.Error(err, "failed to flush audit buffer on shutdown")
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

type rbacFile struct {
	Roles map[string][]string `yaml:"roles"`
}

func loadRBACRoles() (map[string][]string, error) {
	data, err := os.ReadFile(rbacRolesPath)
	if err != nil {
		if os.IsNotExist(err) {
			// F-002: Fail startup when rbac_roles.yaml is missing.
			// Wildcard defaults are unsafe for production.
			return nil, fmt.Errorf("rbac_roles.yaml not found at %s — RBAC policy is required", rbacRolesPath)
		}
		return nil, err
	}
	var rf rbacFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parsing rbac_roles.yaml: %w", err)
	}
	if len(rf.Roles) == 0 {
		return nil, fmt.Errorf("rbac_roles.yaml must define at least one role")
	}
	return rf.Roles, nil
}

type caWatcherEntry struct {
	name    string
	watcher *hotreload.FileWatcher
}

func buildMCPHandler(ctx context.Context, cfg *config.Config, metricsReg *metrics.Registry, rbacRoles map[string][]string, auditor audit.Emitter, logger logr.Logger, userLimiter *ratelimit.UserLimiter) (http.Handler, []caWatcherEntry, func() bool, error) {
	var caWatchers []caWatcherEntry

	dsTransport, dsWatcher, err := tlswiring.CAReloadableTransport(cfg.Agent.DSTLSCaFile, logger.WithName("ds-ca"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("DS TLS transport: %w", err)
	}
	if dsWatcher != nil {
		if err := dsWatcher.Start(ctx); err != nil {
			return nil, nil, nil, fmt.Errorf("DS CA watcher start: %w", err)
		}
		caWatchers = append(caWatchers, caWatcherEntry{name: "ds-ca", watcher: dsWatcher})
	}

	// F-014: Wrap DS transport with retry + circuit breaker for resilience.
	dsResilientTransport := buildResilientTransport(dsTransport, &cfg.Resilience.DS, "ds", metricsReg)

	var dsClient ds.Client
	dsCfg := ds.OgenClientConfig{
		BaseURL:   cfg.Agent.DSBaseURL,
		Timeout:   cfg.Resilience.DS.RequestTimeout,
		Transport: dsResilientTransport,
	}
	if c, err := ds.NewOgenClient(dsCfg); err == nil {
		dsClient = c
	} else {
		logger.Info("DS client unavailable, DS tools will return errors", "error", err)
	}

	kaTransport, kaWatcher, err := tlswiring.CAReloadableTransport(cfg.Agent.KATLSCaFile, logger.WithName("ka-ca"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("KA TLS transport: %w", err)
	}
	if kaWatcher != nil {
		if err := kaWatcher.Start(ctx); err != nil {
			return nil, nil, nil, fmt.Errorf("KA CA watcher start: %w", err)
		}
		caWatchers = append(caWatchers, caWatcherEntry{name: "ka-ca", watcher: kaWatcher})
	}

	kaMCPHTTPClient := &http.Client{Transport: &auth.ContextJWTDelegationTransport{}}
	if kaTransport != nil {
		kaMCPHTTPClient.Transport = &auth.ContextJWTDelegationTransport{Base: kaTransport}
	}
	kaMCPClient := ka.NewSDKMCPClient(
		cfg.Agent.KAMCPEndpoint,
		kaMCPHTTPClient,
		logger,
	)

	var triager *severity.Triager
	if cfg.SeverityTriage.Enabled {
		promTransport, promWatcher, promErr := tlswiring.CAReloadableTransport(cfg.SeverityTriage.PrometheusTLSCaFile, logger.WithName("prom-ca"))
		if promErr != nil {
			return nil, nil, nil, fmt.Errorf("prometheus TLS transport: %w", promErr)
		}
		if promWatcher != nil {
			if err := promWatcher.Start(ctx); err != nil {
				return nil, nil, nil, fmt.Errorf("prometheus CA watcher start: %w", err)
			}
			caWatchers = append(caWatchers, caWatcherEntry{name: "prom-ca", watcher: promWatcher})
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

		triager = severity.NewTriager(promClient, llmTriager, severityCfg, logger.WithName("severity-triage"), &severity.TriagerMetrics{
			Total:    metricsReg.SeverityTriageTotal,
			Duration: metricsReg.SeverityTriageDuration,
			Errors:   metricsReg.SeverityTriageErrorsTotal,
		})
		logger.Info("severity triage enabled", "prometheusURL", cfg.SeverityTriage.PrometheusURL)
	}

	kaClient := ka.NewClient(ka.Config{
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
	}, &ka.ClientMetrics{
		StateGauge:   metricsReg.CircuitBreakerState,
		DurationHist: metricsReg.DownstreamDuration,
		RetryCounter: metricsReg.DownstreamRetryTotal,
	})

	bridgeCfg := &handler.MCPBridgeConfig{
		DynFactory:         buildDynFactory(),
		KAClient:           kaClient,
		KAMCPClient:        kaMCPClient,
		DSClient:           dsClient,
		Triager:            triager,
		RBACRoles:          rbacRoles,
		Auditor:            auditor,
		Logger:             logger.WithName("bridge"),
		Metrics:            bridgeMetricsFrom(metricsReg),
		ToolTimeout:        30 * time.Second,
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
		return nil, nil, nil, err
	}

	// CM-04: Build composite readiness checker from dependency health.
	depsReady := handler.AllReady(
		kaClient.Healthy,
		dsResilientTransport.Healthy,
	)
	return h, caWatchers, depsReady, nil
}

// buildResilientTransport wraps a base transport with retry + circuit breaker.
// Returns the CB transport for health checking.
func buildResilientTransport(base http.RoundTripper, depCfg *config.DependencyConfig, name string, reg *metrics.Registry) *resilience.CircuitBreakerTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	retryRT := resilience.NewRetryTransport(base, &resilience.RetryConfig{
		MaxAttempts:       depCfg.RetryMax + 1,
		InitialBackoff:    depCfg.RetryInitBackoff,
		MaxBackoff:        depCfg.RetryMaxBackoff,
		RetryableStatuses: depCfg.RetryableStatuses,
		RetryCounter:      reg.DownstreamRetryTotal,
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
	})
	return cbt
}

// bridgeMetricsFrom wires the global metrics registry counters into
// the bridge metrics struct — single instances shared across the process.
func bridgeMetricsFrom(reg *metrics.Registry) *handler.MCPBridgeMetrics {
	return &handler.MCPBridgeMetrics{
		ToolCallsTotal:   reg.ToolCallsTotal,
		ToolCallDuration: reg.ToolCallDuration,
		RBACDeniedTotal:  reg.MCPRBACDeniedTotal,
	}
}

func buildDynFactory() auth.DynamicClientFactory {
	return func(ctx context.Context) (dynamic.Interface, error) {
		restCfg, err := ctrl.GetConfig()
		if err != nil {
			return nil, fmt.Errorf("kubernetes config unavailable: %w", err)
		}
		identity := auth.UserIdentityFromContext(ctx)
		if identity == nil {
			// F-003: Reject requests without validated identity.
			// Using the pod ServiceAccount for user-scoped operations violates least privilege.
			return nil, fmt.Errorf("authenticated user identity required for kubernetes operations")
		}
		return auth.NewImpersonatingDynamicFactory(restCfg)(ctx)
	}
}

func buildAuthMiddleware(cfg *config.Config, reg *metrics.Registry, auditor audit.Emitter, logger logr.Logger) func(http.Handler) http.Handler {
	ac := buildAuthConfig(cfg)
	if len(ac.JWT) == 0 || ac.JWT[0].Issuer.URL == "" {
		logger.Info("WARNING: no auth issuer configured — using pass-through auth (not suitable for production)")
		return func(next http.Handler) http.Handler { return next }
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
		}
	}

	return auth.MiddlewareWithConfig(auth.MiddlewareConfig{
		Validator:    validator,
		Logger:       logger,
		Auditor:      auditor,
		AuthDuration: reg.AuthDuration,
	})
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
	token, err := os.ReadFile(t.tokenFile) // #nosec G304 -- path from operator-controlled config
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

// logAuditWriter implements audit.Writer by logging events via logr.
// Used as fallback when DS is unavailable.
type logAuditWriter struct {
	logger logr.Logger
}

func (w *logAuditWriter) WriteAuditEvents(_ context.Context, events []*audit.Event) error {
	for _, ev := range events {
		w.logger.Info("audit event", "type", ev.Type, "user", ev.UserID, "detail", ev.Detail)
	}
	return nil
}

// dsAuditWriterAdapter wraps a DS client to satisfy audit.Writer for FedRAMP-compliant
// durable, centralized audit storage.
type dsAuditWriterAdapter struct {
	client *ds.OgenClient
	logger logr.Logger
}

func (w *dsAuditWriterAdapter) WriteAuditEvents(ctx context.Context, events []*audit.Event) error {
	if err := w.client.WriteAuditEvents(ctx, events); err != nil {
		w.logger.Error(err, "DS audit write failed, events logged as fallback", "count", len(events))
		for _, ev := range events {
			w.logger.Info("audit event (DS fallback)", "type", ev.Type, "user", ev.UserID, "detail", ev.Detail)
		}
		return err
	}
	return nil
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
