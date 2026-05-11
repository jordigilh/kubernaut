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
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

func main() {
	zapLogger := newZapLogger()
	defer func() { _ = zapLogger.Sync() }()
	logger := zapr.NewLogger(zapLogger).WithName("apifrontend")

	cfg, err := loadConfig()
	if err != nil {
		logger.Error(err, "failed to load config")
		os.Exit(1) //nolint:gocritic // exitAfterDefer: deferred Sync is best-effort
	}
	cfg.ResolveDefaults()
	if err := cfg.Validate(); err != nil {
		logger.Error(err, "invalid config")
		os.Exit(1)
	}

	rbacRoles, err := loadRBACRoles()
	if err != nil {
		logger.Error(err, "failed to load RBAC roles", "path", rbacRolesPath)
		os.Exit(1)
	}

	metricsReg := metrics.NewRegistry()

	auditor := audit.NewBufferedEmitter(audit.BufferConfig{
		Writer:          &logAuditWriter{logger: logger.WithName("audit-writer")},
		Logger:          logger,
		OverflowCounter: metricsReg.AuditBufferOverflow,
		EventsCounter:   metricsReg.AuditEventsTotal,
	})
	auditor.Start()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mcpHandler, caWatchers, err := buildMCPHandler(ctx, cfg, metricsReg, rbacRoles, auditor, logger)
	if err != nil {
		logger.Error(err, "failed to create MCP handler")
		os.Exit(1)
	}
	defer func() {
		for _, w := range caWatchers {
			w.watcher.Stop()
		}
	}()

	agentCardHandler, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
		Name:        "kubernaut-apifrontend",
		Description: "Kubernaut AI-driven remediation API Frontend",
		URL:         cfg.AgentCard.URL,
		Version:     version(),
		Skills:      handler.DefaultAgentSkills(),
	})
	if err != nil {
		logger.Error(err, "failed to create agent card handler")
		os.Exit(1)
	}

	a2aHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "A2A not configured", http.StatusNotImplemented)
	})

	draining := &atomic.Bool{}
	routerCfg := handler.RouterConfig{
		MetricsRegistry:  metricsReg,
		A2AHandler:       a2aHandler,
		MCPHandler:       mcpHandler,
		AgentCardHandler: agentCardHandler,
		AuthMiddleware:   noopMiddleware,
		ReadyChecker:     func() bool { return !draining.Load() },
		SSETracker:       streaming.NewConnectionTracker(metricsReg.SSEActiveConnections, 5*time.Second),
		Draining:         draining,
	}
	router, err := handler.NewRouter(routerCfg)
	if err != nil {
		logger.Error(err, "failed to create router")
		os.Exit(1)
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

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"healthy"}`)
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if draining.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, `{"status":"draining"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"ready"}`)
	})
	healthServer := &http.Server{
		Addr:              defaultHealthz,
		Handler:           healthMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:              defaultMetrics,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	tlsEnabled, certReloader, err := tlswiring.ConfigureServer(httpServer, cfg.Server.TLS.CertDir)
	if err != nil {
		logger.Error(err, "failed to configure TLS")
		os.Exit(1)
	}
	if tlsEnabled {
		logger.Info("TLS enabled with hot-reloadable certificates", "certDir", cfg.Server.TLS.CertDir)
	} else {
		if warn := tlswiring.CheckPartialTLSMaterial(cfg.Server.TLS.CertDir); warn != "" {
			logger.Info("WARNING: "+warn, "certDir", cfg.Server.TLS.CertDir)
		}
		logger.Info("TLS disabled, serving plain HTTP")
	}

	certWatcher, err := tlswiring.StartCertFileWatcher(ctx, cfg.Server.TLS.CertDir, certReloader, logger)
	if err != nil {
		logger.Error(err, "failed to start certificate file watcher")
		os.Exit(1)
	}
	if certWatcher != nil {
		defer certWatcher.Stop()
	}

	caWatcher, err := tlswiring.StartCAFileWatcher(ctx, logger)
	if err != nil {
		logger.Error(err, "failed to start CA file watcher")
		os.Exit(1)
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

	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	shutdownServer(shutCtx, httpServer, "API", logger)
	shutdownServer(shutCtx, healthServer, "health", logger)
	shutdownServer(shutCtx, metricsServer, "metrics", logger)

	logger.Info("shutdown complete")
}

func newZapLogger() *zap.Logger {
	zapCfg := zap.NewProductionConfig()
	zapCfg.EncoderConfig.TimeKey = "ts"
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)

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
			return config.DefaultConfig(), nil
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
			return map[string][]string{"*": {"*"}}, nil
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

func buildMCPHandler(ctx context.Context, cfg *config.Config, metricsReg *metrics.Registry, rbacRoles map[string][]string, auditor audit.Emitter, logger logr.Logger) (http.Handler, []caWatcherEntry, error) {
	var caWatchers []caWatcherEntry

	dsTransport, dsWatcher, err := tlswiring.CAReloadableTransport(cfg.Agent.DSTLSCaFile, logger.WithName("ds-ca"))
	if err != nil {
		return nil, nil, fmt.Errorf("DS TLS transport: %w", err)
	}
	if dsWatcher != nil {
		if err := dsWatcher.Start(ctx); err != nil {
			return nil, nil, fmt.Errorf("DS CA watcher start: %w", err)
		}
		caWatchers = append(caWatchers, caWatcherEntry{name: "ds-ca", watcher: dsWatcher})
	}

	var dsClient ds.Client
	dsCfg := ds.OgenClientConfig{
		BaseURL:   cfg.Agent.DSBaseURL,
		Timeout:   cfg.Resilience.DS.RequestTimeout,
		Transport: dsTransport,
	}
	if c, err := ds.NewOgenClient(dsCfg); err == nil {
		dsClient = c
	} else {
		logger.Info("DS client unavailable, DS tools will return errors", "error", err)
	}

	kaTransport, kaWatcher, err := tlswiring.CAReloadableTransport(cfg.Agent.KATLSCaFile, logger.WithName("ka-ca"))
	if err != nil {
		return nil, nil, fmt.Errorf("KA TLS transport: %w", err)
	}
	if kaWatcher != nil {
		if err := kaWatcher.Start(ctx); err != nil {
			return nil, nil, fmt.Errorf("KA CA watcher start: %w", err)
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
			return nil, nil, fmt.Errorf("prometheus TLS transport: %w", promErr)
		}
		if promWatcher != nil {
			if err := promWatcher.Start(ctx); err != nil {
				return nil, nil, fmt.Errorf("prometheus CA watcher start: %w", err)
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

		triager = severity.NewTriager(promClient, llmTriager, severityCfg, logger.WithName("severity-triage"))
		logger.Info("severity triage enabled", "prometheusURL", cfg.SeverityTriage.PrometheusURL)
	}

	bridgeCfg := &handler.MCPBridgeConfig{
		DynFactory:         buildDynFactory(),
		KAClient:           ka.NewClient(ka.Config{BaseURL: cfg.Agent.KABaseURL, BaseTransport: kaTransport}),
		KAMCPClient:        kaMCPClient,
		DSClient:           dsClient,
		Triager:            triager,
		RBACRoles:          rbacRoles,
		Auditor:            auditor,
		Logger:             logger.WithName("bridge"),
		Metrics:            bridgeMetricsFrom(metricsReg),
		ToolTimeout:        30 * time.Second,
		MaxConcurrentTools: 10,
	}

	h, err := handler.NewMCPHandler(handler.MCPConfig{
		ServerName:    "kubernaut-apifrontend",
		ServerVersion: version(),
		Enabled:       cfg.MCP.Enabled,
		Bridge:        bridgeCfg,
		Auditor:       auditor,
	})
	if err != nil {
		return nil, nil, err
	}
	return h, caWatchers, nil
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
		if identity != nil {
			return auth.NewImpersonatingDynamicFactory(restCfg)(ctx)
		}
		return dynamic.NewForConfig(restCfg)
	}
}

func noopMiddleware(next http.Handler) http.Handler { return next }

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
// In production, replace with a DS-backed writer.
type logAuditWriter struct {
	logger logr.Logger
}

func (w *logAuditWriter) WriteAuditEvents(_ context.Context, events []*audit.Event) error {
	for _, ev := range events {
		w.logger.Info("audit event", "type", ev.Type, "user", ev.UserID, "detail", ev.Detail)
	}
	return nil
}
