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
	"google.golang.org/adk/model/gemini"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"
	"gopkg.in/yaml.v3"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
	agentpkg "github.com/jordigilh/kubernaut-apifrontend/internal/agent"
	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/config"
	"github.com/jordigilh/kubernaut-apifrontend/internal/controller"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ds"
	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
	"github.com/jordigilh/kubernaut-apifrontend/internal/launcher"
	"github.com/jordigilh/kubernaut-apifrontend/internal/metrics"
	prom "github.com/jordigilh/kubernaut-apifrontend/internal/prometheus"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ratelimit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/resilience"
	"github.com/jordigilh/kubernaut-apifrontend/internal/session"
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

	sessInfra := buildSessionInfra(cfg, metricsReg, auditor, logger)
	defer sessInfra.StopFunc()

	deps, err := buildBackendDeps(ctx, cfg, metricsReg, logger)
	if err != nil {
		logger.Error(err, "failed to create backend dependencies")
		return 1
	}
	defer func() {
		for _, w := range deps.CAWatchers {
			w.watcher.Stop()
		}
	}()

	mcpHandler, depsReady, err := buildMCPHandler(cfg, deps, metricsReg, rbacRoles, auditor, logger, userLimiter)
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
		Name:        "kubernaut-apifrontend",
		Description: "Kubernaut AI-driven remediation API Frontend",
		URL:         cfg.AgentCard.URL,
		Version:     version(),
		Skills:      handler.DefaultAgentSkills(),
	})
	if err != nil {
		logger.Error(err, "failed to create agent card handler")
		return 1
	}

	a2aHandler, err := buildA2AHandler(ctx, cfg, deps, sessInfra, metricsReg, auditor, logger)
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

// backendDeps holds shared backend clients used by both the MCP and A2A handlers.
// Created once by buildBackendDeps and consumed by buildMCPHandler / buildA2AHandler.
type backendDeps struct {
	DSClient             ds.Client
	KAClient             *ka.Client
	MCPClient            ka.MCPClient
	DynFactory           auth.DynamicClientFactory
	Triager              *severity.Triager
	DSResilientTransport *resilience.CircuitBreakerTransport
	CAWatchers           []caWatcherEntry
	k8sDynClient         dynamic.Interface
}

// K8sClient returns the pod service-account scoped dynamic K8s client.
// Created lazily on first call via the in-cluster config.
func (d *backendDeps) K8sClient() dynamic.Interface {
	if d.k8sDynClient != nil {
		return d.k8sDynClient
	}
	restCfg, err := ctrl.GetConfig()
	if err != nil {
		return nil
	}
	c, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil
	}
	d.k8sDynClient = c
	return d.k8sDynClient
}

func buildBackendDeps(ctx context.Context, cfg *config.Config, metricsReg *metrics.Registry, logger logr.Logger) (*backendDeps, error) {
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

	deps.DSResilientTransport = buildResilientTransport(dsTransport, &cfg.Resilience.DS, "ds", metricsReg)

	dsCfg := ds.OgenClientConfig{
		BaseURL:   cfg.Agent.DSBaseURL,
		Timeout:   cfg.Resilience.DS.RequestTimeout,
		Transport: deps.DSResilientTransport,
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

	kaMCPHTTPClient := &http.Client{Transport: &auth.ContextJWTDelegationTransport{}}
	if kaTransport != nil {
		kaMCPHTTPClient.Transport = &auth.ContextJWTDelegationTransport{Base: kaTransport}
	}
	deps.MCPClient = ka.NewSDKMCPClient(
		cfg.Agent.KAMCPEndpoint,
		kaMCPHTTPClient,
		logger,
	)

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

		deps.Triager = severity.NewTriager(promClient, llmTriager, severityCfg, logger.WithName("severity-triage"), &severity.TriagerMetrics{
			Total:    metricsReg.SeverityTriageTotal,
			Duration: metricsReg.SeverityTriageDuration,
			Errors:   metricsReg.SeverityTriageErrorsTotal,
		})
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
	}, &ka.ClientMetrics{
		StateGauge:   metricsReg.CircuitBreakerState,
		DurationHist: metricsReg.DownstreamDuration,
		RetryCounter: metricsReg.DownstreamRetryTotal,
	})

	deps.DynFactory = buildDynFactory()

	return deps, nil
}

func buildMCPHandler(cfg *config.Config, deps *backendDeps, metricsReg *metrics.Registry, rbacRoles map[string][]string, auditor audit.Emitter, logger logr.Logger, userLimiter *ratelimit.UserLimiter) (http.Handler, func() bool, error) {
	bridgeCfg := &handler.MCPBridgeConfig{
		DynFactory:         deps.DynFactory,
		KAClient:           deps.KAClient,
		KAMCPClient:        deps.MCPClient,
		DSClient:           deps.DSClient,
		Triager:            deps.Triager,
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
		return nil, nil, err
	}

	depsReady := handler.AllReady(
		deps.KAClient.Healthy,
		deps.DSResilientTransport.Healthy,
	)
	return h, depsReady, nil
}

// buildA2AHandler creates the A2A JSON-RPC handler when an LLM endpoint is
// configured. Returns a 501 stub when LLMEndpoint is empty, preserving backward
// compatibility for deployments that don't set it.
func buildA2AHandler(ctx context.Context, cfg *config.Config, deps *backendDeps, sessInfra *sessionInfra, metricsReg *metrics.Registry, auditor audit.Emitter, logger logr.Logger) (http.Handler, error) {
	if cfg.Agent.LLMEndpoint == "" {
		logger.Info("LLM endpoint not configured — A2A handler returns 501")
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "A2A not configured", http.StatusNotImplemented)
		}), nil
	}

	llmModel, err := gemini.NewModel(ctx, cfg.Agent.LLMModel, &genai.ClientConfig{
		APIKey:  cfg.Agent.LLMAPIKey,
		Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{
			BaseURL: cfg.Agent.LLMEndpoint,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create LLM model: %w", err)
	}

	rootAgent, _, err := agentpkg.NewRootAgent(agentpkg.AgentConfig{
		Instruction:                agentpkg.DefaultTestConfig().Instruction,
		LLMModel:                   llmModel,
		K8sClient:                  deps.K8sClient(),
		KAClient:                   deps.KAClient,
		DSClient:                   deps.DSClient,
		MCPClient:                  deps.MCPClient,
		ImpersonatingClientFactory: deps.DynFactory,
		Auditor:                    auditor,
		ToolCallsTotal:             metricsReg.ToolCallsTotal,
		ToolCallDuration:           metricsReg.ToolCallDuration,
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
	}

	h, err := launcher.NewA2AHandler(a2aCfg)
	if err != nil {
		return nil, fmt.Errorf("create A2A handler: %w", err)
	}

	logger.Info("A2A handler wired with LLM backend",
		"endpoint", cfg.Agent.LLMEndpoint,
		"model", cfg.Agent.LLMModel,
	)
	return h, nil
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
				reg.SessionTTLActionsTotal,
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
		reg.SessionTTLActionsTotal,
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
