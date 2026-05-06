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
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"go.uber.org/zap"
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

	logger.Info("configuration loaded",
		"port", cfg.Server.Port,
		"mcpEnabled", cfg.MCP.Enabled,
		"agentCardURL", cfg.AgentCard.URL,
		"kaBaseURL", cfg.Agent.KABaseURL,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	metricsReg := metrics.NewRegistry()
	auditor := audit.NewLogEmitter(logger)

	authCfg := auth.Config{}
	if len(authCfg.JWT) == 0 {
		logger.Error(nil, "no JWT providers configured — all bearer tokens will be rejected unless K8s TokenReview is enabled")
	}
	validator, err := auth.NewJWTValidator(authCfg, auth.WithCBMetrics(metricsReg.CircuitBreakerState))
	if err != nil {
		return fmt.Errorf("create JWT validator: %w", err)
	}

	rlCfg := ratelimit.DefaultConfig()
	ipLimiter := ratelimit.NewIPLimiter(rlCfg.PerIP)
	defer ipLimiter.Stop()
	userLimiter := ratelimit.NewUserLimiter(rlCfg.PerUser)
	defer userLimiter.Stop()

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
	dsRetryRT := resilience.NewRetryTransport(http.DefaultTransport, &resilience.RetryConfig{
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
		Logger:         slog.Default(),
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

	rootHandler, err := handler.NewRouter(handler.RouterConfig{
		MetricsRegistry:  metricsReg,
		A2AHandler:       a2aHandler,
		MCPHandler:       mcpHandler,
		AgentCardHandler: agentCardHandler,
		AuthMiddleware:   authMiddleware,
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
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
		logger.Error(shutdownErr, "server shutdown error")
	}

	logger.Info("shutdown complete")
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
			Validator: validator,
			Logger:    logger,
			Auditor:   auditor,
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
