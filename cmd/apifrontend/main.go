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

package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	adksession "google.golang.org/adk/session"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	agentpkg "github.com/jordigilh/kubernaut-apifrontend/internal/agent"
	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/config"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ds"
	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
	"github.com/jordigilh/kubernaut-apifrontend/internal/launcher"
	"github.com/jordigilh/kubernaut-apifrontend/internal/logging"
	"github.com/jordigilh/kubernaut-apifrontend/internal/metrics"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ratelimit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/requestid"
	"github.com/jordigilh/kubernaut-apifrontend/internal/resilience"
	"github.com/jordigilh/kubernaut-apifrontend/internal/streaming"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var configPath string
	flag.StringVar(&configPath, "config", "/etc/apifrontend/config.yaml", "Path to YAML configuration file (ConfigMap mount)")
	flag.Parse()

	level := zap.NewAtomicLevelAt(zap.InfoLevel)
	logger, err := logging.NewLogger(level)
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	logger = logger.WithValues("service", "kubernaut-apifrontend")

	cfgData, err := os.ReadFile(filepath.Clean(configPath))
	if err != nil {
		return fmt.Errorf("read config %s (use --config to specify path): %w", configPath, err)
	}

	cfg, err := config.Load(cfgData)
	if err != nil {
		return fmt.Errorf("parse config %s: %w", configPath, err)
	}

	cfg.ResolveDefaults()

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	if cfgLevel, err := parseLogLevel(cfg.Logging.Level); err == nil {
		level.SetLevel(cfgLevel)
	}

	logger.Info("configuration loaded",
		"port", cfg.Server.Port,
		"mcpEnabled", cfg.MCP.Enabled,
		"agentCardURL", cfg.AgentCard.URL,
		"kaBaseURL", cfg.Agent.KABaseURL,
		"logLevel", cfg.Logging.Level,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	metricsReg := metrics.NewRegistry()

	authCfg := buildAuthConfig(cfg)
	if len(authCfg.JWT) == 0 {
		logger.Error(nil, "no JWT providers configured — all bearer tokens will be rejected unless K8s TokenReview is enabled")
	}
	validator, err := auth.NewJWTValidator(authCfg, auth.WithCBMetrics(metricsReg.CircuitBreakerState))
	if err != nil {
		return fmt.Errorf("create JWT validator: %w", err)
	}

	ipLimiter := ratelimit.NewIPLimiter(ratelimit.PerIPConfig{
		RequestsPerSecond: float64(cfg.RateLimit.IPRequestsPerSec),
		Burst:             cfg.RateLimit.IPRequestsPerSec * 2,
		CleanupInterval:   5 * time.Minute,
		MaxAge:            10 * time.Minute,
	})
	defer ipLimiter.Stop()
	maxSessions := cfg.RateLimit.MaxConcurrentSessions
	if maxSessions <= 0 {
		maxSessions = 5
	}
	toolCallsPM := cfg.RateLimit.ToolCallsPerMinute
	if toolCallsPM <= 0 {
		toolCallsPM = 60
	}
	userLimiter := ratelimit.NewUserLimiter(ratelimit.PerUserConfig{
		RequestsPerMinute:     cfg.RateLimit.UserRequestsPerSec * 60,
		MaxConcurrentSessions: maxSessions,
		ToolCallsPerMinute:    toolCallsPM,
	})

	// --- Resilience: KA client ---
	kaClient := ka.NewClient(ka.Config{
		BaseURL:            cfg.Agent.KABaseURL,
		Timeout:            cfg.Resilience.KA.RequestTimeout,
		RetryMax:           cfg.Resilience.KA.RetryMax,
		RetryInitBackoff:   cfg.Resilience.KA.RetryInitBackoff,
		RetryMaxBackoff:    cfg.Resilience.KA.RetryMaxBackoff,
		RetryableStatuses:  cfg.Resilience.KA.RetryableStatuses,
		CBMaxRequests:      cfg.Resilience.KA.CBMaxRequests,
		CBInterval:         cfg.Resilience.KA.CBInterval,
		CBTimeout:          cfg.Resilience.KA.CBTimeout,
		CBFailureThreshold: cfg.Resilience.KA.CBFailureThreshold,
	}, &ka.ClientMetrics{
		StateGauge:   metricsReg.CircuitBreakerState,
		RetryCounter: metricsReg.DownstreamRetryTotal,
		DurationHist: metricsReg.DownstreamDuration,
	})

	// --- Resilience: DS ogen client ---
	dsBaseRT := &requestid.Transport{Base: http.DefaultTransport}
	dsRetryRT := resilience.NewRetryTransport(dsBaseRT, &resilience.RetryConfig{
		MaxAttempts:       cfg.Resilience.DS.RetryMax + 1,
		InitialBackoff:    cfg.Resilience.DS.RetryInitBackoff,
		MaxBackoff:        cfg.Resilience.DS.RetryMaxBackoff,
		RetryableStatuses: cfg.Resilience.DS.RetryableStatuses,
		RetryCounter:      metricsReg.DownstreamRetryTotal,
		DependencyName:    "ds",
	})
	dsCBT := resilience.NewCircuitBreakerTransport(dsRetryRT, &resilience.CircuitBreakerConfig{
		Name:             "ds-rest",
		MaxRequests:      cfg.Resilience.DS.CBMaxRequests,
		Interval:         cfg.Resilience.DS.CBInterval,
		Timeout:          cfg.Resilience.DS.CBTimeout,
		FailureThreshold: cfg.Resilience.DS.CBFailureThreshold,
		StateGauge:       metricsReg.CircuitBreakerState,
		DurationHist:     metricsReg.DownstreamDuration,
		DependencyName:   "ds",
	})
	dsClient, err := ds.NewOgenClient(ds.OgenClientConfig{
		BaseURL:   cfg.Agent.DSBaseURL,
		Transport: dsCBT,
		Timeout:   cfg.Resilience.DS.RequestTimeout,
	})
	if err != nil {
		return fmt.Errorf("create DS client: %w", err)
	}

	// Auditor: BufferedEmitter with DS backend for durable audit storage
	bufferedAuditor := audit.NewBufferedEmitter(audit.BufferConfig{
		Writer:          dsClient,
		Logger:          logger,
		BufferSize:      4096,
		FlushInterval:   5 * time.Second,
		BatchSize:       100,
		OverflowCounter: metricsReg.AuditBufferOverflow,
		EventsCounter:   metricsReg.AuditEventsTotal,
	})
	auditor := audit.Emitter(bufferedAuditor)

	// SSE connection tracker for graceful shutdown drain
	sseTracker := streaming.NewConnectionTracker(metricsReg.SSEActiveConnections, 3*time.Second)

	// --- MCP client (Pattern B JWT delegation) ---
	mcpHTTPClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &auth.ContextJWTDelegationTransport{
			Base: &requestid.Transport{Base: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				TLSHandshakeTimeout:   5 * time.Second,
				ResponseHeaderTimeout: 15 * time.Second,
			}},
		},
	}
	mcpClient := ka.NewSDKMCPClient(cfg.Agent.KAMCPEndpoint, mcpHTTPClient, logger)

	// --- Resilience: K8s circuit breaker ---
	k8sCB := resilience.NewK8sCircuitBreaker(resilience.K8sCBConfig{
		Name:             "k8s-api",
		MaxRequests:      cfg.Resilience.K8s.CBMaxRequests,
		Interval:         cfg.Resilience.K8s.CBInterval,
		Timeout:          cfg.Resilience.K8s.CBTimeout,
		FailureThreshold: cfg.Resilience.K8s.CBFailureThreshold,
		StateGauge:       metricsReg.CircuitBreakerState,
		DependencyName:   "k8s",
	})

	// --- K8s dynamic client with circuit breaker ---
	k8sClient, err := buildK8sDynamicClient(k8sCB, logger)
	if err != nil {
		return fmt.Errorf("create K8s dynamic client: %w", err)
	}

	agentCfg := agentpkg.AgentConfig{
		GCPProject:    cfg.Agent.GCPProject,
		GCPRegion:     cfg.Agent.GCPRegion,
		Instruction:   "You are the Kubernaut API Frontend agent. Help users triage and remediate Kubernetes incidents.",
		KABaseURL:     cfg.Agent.KABaseURL,
		KAMCPEndpoint: cfg.Agent.KAMCPEndpoint,
		DSBaseURL:     cfg.Agent.DSBaseURL,
		K8sClient:     k8sClient,
		DSClient:      dsClient,
		KAClient:      kaClient,
		Auditor:       auditor,
		MCPClient:     mcpClient,
	}
	rootAgent, _, err := agentpkg.NewRootAgent(agentCfg)
	if err != nil {
		return fmt.Errorf("create root agent: %w", err)
	}
	sessionSvc := adksession.InMemoryService()

	a2aHandler, err := launcher.NewA2AHandler(launcher.A2AConfig{
		Agent:          rootAgent,
		SessionService: sessionSvc,
		AppName:        "kubernaut-apifrontend",
		Logger:         logging.NewSlogLogger(level),
		Auditor:        auditor,
	})
	if err != nil {
		return fmt.Errorf("create A2A handler: %w", err)
	}

	mcpHandler, err := handler.NewMCPHandler(handler.MCPConfig{
		ServerName:    "kubernaut-apifrontend",
		ServerVersion: "0.1.0",
		Tools:         handler.DefaultMCPTools(),
		Auditor:       auditor,
		Enabled:       cfg.MCP.Enabled,
	})
	if err != nil {
		return fmt.Errorf("create MCP handler: %w", err)
	}

	agentCardHandler, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
		Name:        "kubernaut-apifrontend",
		Description: "Kubernaut API Frontend agent for Kubernetes incident triage and remediation",
		URL:         cfg.AgentCard.URL,
		Version:     "0.1.0",
		Skills:      handler.DefaultAgentSkills(),
	})
	if err != nil {
		return fmt.Errorf("create agent card handler: %w", err)
	}

	authMiddleware := buildAuthMiddleware(validator, logger, auditor, ipLimiter, userLimiter, metricsReg)

	var draining atomic.Bool
	rootHandler, err := handler.NewRouter(handler.RouterConfig{
		MetricsRegistry:  metricsReg,
		A2AHandler:       a2aHandler,
		MCPHandler:       mcpHandler,
		AgentCardHandler: agentCardHandler,
		AuthMiddleware:   authMiddleware,
		SSETracker:       sseTracker,
		Draining:         &draining,
		ReadyChecker: handler.AllReady(
			validator.Ready,
			kaClient.Healthy,
			dsCBT.Healthy,
			k8sCB.Healthy,
		),
	})
	if err != nil {
		return fmt.Errorf("create router: %w", err)
	}

	port := strconv.Itoa(cfg.Server.Port)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           rootHandler,
		ReadTimeout:       60 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout intentionally omitted (0) for SSE/MCP streaming.
		// Per-request deadlines enforced via http.ResponseController.SetWriteDeadline().
	}

	// --- Config hot-reload ---
	// Hot-reloadable: logging.level
	// NOT hot-reloadable (require pod restart): rateLimit, auth, resilience,
	// server.port, agent endpoints, mcp.enabled, agentCard.url
	watcher, watchErr := config.NewFileWatcher(configPath, func(newContent []byte) error {
		newCfg, err := config.Load(newContent)
		if err != nil {
			return fmt.Errorf("parse: %w", err)
		}
		newCfg.ResolveDefaults()
		if err := newCfg.Validate(); err != nil {
			return fmt.Errorf("validate: %w", err)
		}
		if newLevel, err := parseLogLevel(newCfg.Logging.Level); err == nil {
			level.SetLevel(newLevel)
			logger.Info("log level updated via hot-reload", "level", newCfg.Logging.Level)
		}
		return nil
	}, config.WithAuditor(auditor))
	if watchErr != nil {
		logger.Error(watchErr, "config hot-reload disabled")
	} else {
		if startErr := watcher.Start(ctx); startErr != nil {
			logger.Error(startErr, "config watcher start failed")
		} else {
			defer watcher.Stop()
		}
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting kubernaut-apifrontend", "port", port)
		if listenErr := srv.ListenAndServe(); listenErr != nil && listenErr != http.ErrServerClosed {
			errCh <- listenErr
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server failed: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down gracefully")

	// 1. Signal draining so readyz returns 503 (LB stops new traffic)
	draining.Store(true)

	// 2. Drain active SSE/streaming connections
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 5*time.Second)
	drained := sseTracker.DrainAll(drainCtx)
	drainCancel()
	if drained > 0 {
		logger.Info("SSE connections drained", "count", drained)
	}

	// 3. Graceful HTTP drain — wait for in-flight requests to complete
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
		logger.Error(shutdownErr, "server shutdown error, forcing close")
		_ = srv.Close()
	}
	shutdownCancel()

	// 4. Flush audit buffer AFTER srv.Shutdown so late audit events
	// (e.g. from cancelled handler contexts) are captured.
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if flushErr := bufferedAuditor.Close(flushCtx); flushErr != nil {
		logger.Error(flushErr, "audit flush error during shutdown")
	}
	flushCancel()

	logger.Info("shutdown complete")

	// 5. Sync logger AFTER final log message to guarantee it's flushed
	logging.Sync()

	return nil
}

func buildAuthMiddleware(
	validator *auth.JWTValidator,
	logger logr.Logger,
	auditor audit.Emitter,
	ipLimiter *ratelimit.IPLimiter,
	userLimiter *ratelimit.UserLimiter,
	metricsReg *metrics.Registry,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		h := next
		h = ratelimit.PostAuthMiddlewareWithConfig(ratelimit.PostAuthMiddlewareConfig{
			Limiter: userLimiter,
			Auditor: auditor,
			Metrics: metricsReg.RateLimitDenied,
		})(h)
		h = auth.MiddlewareWithConfig(auth.MiddlewareConfig{
			Validator:    validator,
			Logger:       logger,
			Auditor:      auditor,
			AuthDuration: metricsReg.AuthDuration,
		})(h)
		h = ratelimit.PreAuthMiddlewareWithConfig(ratelimit.PreAuthMiddlewareConfig{
			Limiter: ipLimiter,
			Auditor: auditor,
			Metrics: metricsReg.RateLimitDenied,
		})(h)
		h = requestid.Middleware(h)
		return h
	}
}

func buildAuthConfig(cfg *config.Config) auth.Config {
	if cfg.Auth.IssuerURL == "" {
		return auth.Config{}
	}
	return auth.Config{
		JWT: []auth.ProviderConfig{{
			Issuer: auth.IssuerConfig{
				URL:       cfg.Auth.IssuerURL,
				Audiences: []string{cfg.Auth.Audience},
			},
		}},
	}
}

func parseLogLevel(s string) (zapcore.Level, error) {
	var l zapcore.Level
	err := l.UnmarshalText([]byte(s))
	return l, err
}

func buildK8sDynamicClient(cb *resilience.K8sCircuitBreaker, logger logr.Logger) (dynamic.Interface, error) {
	restCfg, err := rest.InClusterConfig()
	if err != nil {
		logger.Error(err, "in-cluster config unavailable — CRD tools will return errors until cluster access is configured")
		return nil, nil
	}

	rawClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	return resilience.NewResilientDynamicClient(rawClient, cb), nil
}
