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
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-logr/logr"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/credentials"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	karbac "github.com/jordigilh/kubernaut/internal/kubernautagent/rbac"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

func main() {
	var (
		configPath     string
		llmRuntimePath string
		addr           string
	)
	flag.StringVar(&configPath, "config", "/etc/kubernautagent/config.yaml", "Path to static YAML configuration file")
	flag.StringVar(&llmRuntimePath, "llm-runtime", "/etc/kubernautagent/llm-runtime.yaml", "Path to hot-reloadable LLM runtime configuration")
	flag.StringVar(&addr, "addr", "", "HTTP listen address (overrides config server.port)")
	flag.Parse()

	// Bootstrap logger at INFO for startup; replaced after config is loaded (#875).
	bootstrapLogger := kubelog.NewLogger(kubelog.Options{Level: 0, ServiceName: "kubernaut-agent"})

	cfg, llmRuntime, logger, atomicLevel := loadStartupConfig(configPath, llmRuntimePath, bootstrapLogger)
	defer kubelog.Sync(logger)

	if addr == "" {
		addr = fmt.Sprintf("%s:%d", cfg.Runtime.Server.Address, cfg.Runtime.Server.Port)
	}

	logger.Info("starting Kubernaut Agent", "addr", addr, "config", configPath)

	swappable, phaseSwappables := buildLLMClients(cfg, llmRuntime, logger)

	promptBuilder, err := prompt.NewBuilder()
	if err != nil {
		logger.Error(err, "failed to create prompt builder")
		os.Exit(1)
	}

	resultParser := parser.NewResultParser(logger.WithName("parser"))
	phaseTools := investigator.DefaultPhaseToolMap()

	// #1055: Create a single shared TokenSource for all DS-bound HTTP clients.
	// Both the ogen DS client and the audit store share this cache, so a 401 on
	// either side immediately invalidates the token for both.
	dsTokenSource := auth.NewTokenSource(cfg.Integrations.DataStorage.SATokenPath)
	dsTokenSource.SetLogger(logger.WithName("ds-token-source"))
	if _, err := os.Stat(cfg.Integrations.DataStorage.SATokenPath); err != nil {
		logger.Info("WARNING: SA token file not found at startup, DS/audit API calls will fail auth until file appears",
			"token_path", cfg.Integrations.DataStorage.SATokenPath, "error", err)
	} else {
		logger.Info("SA token source configured (shared cache with token refresh)",
			"token_path", cfg.Integrations.DataStorage.SATokenPath)
	}

	auditStore, auditCleanup := buildAuditStore(cfg, dsTokenSource, logger)
	k8sInfra := initK8sInfra(logger)

	// #1288: SSAR impersonate gate removed — KA uses its own SA for all K8s
	// API calls. Interactive readiness is no longer gated on impersonation RBAC.
	interactiveReadiness := karbac.NewInteractiveReadiness()
	var eventEmitter *karbac.EventEmitter
	if cfg.Interactive.Enabled && k8sInfra != nil {
		podName, podNS := karbac.DetectPodIdentity()
		eventEmitter = karbac.NewEventEmitter(k8sInfra.clientset, podName, podNS)
	}

	ds := initDSClients(cfg, k8sInfra, dsTokenSource, logger)
	if ds == nil {
		logger.Error(nil, "FATAL: DataStorage client initialization failed — KA cannot operate without DS (workflow discovery, audit, enrichment all require it)")
		os.Exit(1)
	}
	reg := buildToolRegistry(cfg, logger, k8sInfra, ds, auditStore)
	fleetClient, fleetToolNames := registerFleetTools(context.Background(), cfg, reg, logger)
	if len(fleetToolNames) > 0 {
		investigator.AppendFleetToolsToRCA(phaseTools, fleetToolNames)
	}
	enricher := buildEnricher(cfg, ds, k8sInfra, auditStore, logger)
	sanitizer := buildSanitizationPipeline(cfg, logger)
	anomalyDetector := buildAnomalyDetector(cfg, logger)
	sum := buildSummarizer(swappable, cfg, logger)

	instrumentedLLM := llm.NewInstrumentedClient(swappable)

	var catalogFetcher investigator.CatalogFetcher
	if ds != nil {
		catalogFetcher = newDSCatalogFetcher(ds, logger)
		logger.Info("workflow catalog fetcher enabled (per-request, DD-HAPI-002)")
	} else {
		logger.Info("workflow catalog fetcher disabled (no DataStorage — dev mode)")
	}

	effectiveLLM, effectiveReg, alignEvaluator, alignCfg := buildAlignmentStack(cfg, llmRuntime, instrumentedLLM, reg, auditStore, logger)

	stack := buildInvestigationRunner(investigationRunnerParams{
		cfg:             cfg,
		llmRuntime:      llmRuntime,
		swappable:       swappable,
		phaseSwappables: phaseSwappables,
		promptBuilder:   promptBuilder,
		resultParser:    resultParser,
		phaseTools:      phaseTools,
		enricher:        enricher,
		auditStore:      auditStore,
		effectiveLLM:    effectiveLLM,
		effectiveReg:    effectiveReg,
		alignEvaluator:  alignEvaluator,
		alignCfg:        alignCfg,
		infra:           k8sInfra,
		sanitizer:       sanitizer,
		anomalyDetector: anomalyDetector,
		catalogFetcher:  catalogFetcher,
		summarizer:      sum,
		logger:          logger,
	})
	agentMetrics := stack.agentMetrics
	instrumentedAudit := stack.instrumentedAudit
	phaseResolver := stack.phaseResolver
	inv := stack.inv
	store := stack.store
	mgr := stack.mgr
	ogenSrv := stack.ogenSrv

	r := chi.NewRouter()

	const maxRequestBodySize int64 = 1 << 20 // 1 MiB

	apiRateLimiter := kaserver.NewRateLimiter(kaserver.RateLimitConfig{
		RequestsPerSecond: cfg.Runtime.Server.RateLimit.RequestsPerSecond,
		Burst:             cfg.Runtime.Server.RateLimit.Burst,
		CleanupInterval:   cfg.Runtime.Server.RateLimit.CleanupInterval,
		MaxAge:            cfg.Runtime.Server.RateLimit.MaxAge,
		TrustedProxyCIDRs: cfg.Runtime.Server.RateLimit.TrustedProxyCIDRs,
	}, agentMetrics.HTTPRateLimitedTotal, kaserver.WithAuditStore(instrumentedAudit, logger))
	defer apiRateLimiter.Stop()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start health/metrics servers BEFORE the route setup closure so that
	// liveness/readiness probes are served even while the JWKS pre-warm
	// (up to 15 s) blocks inside newAuthMiddleware. Without this, the
	// liveness probe kills the pod before the health server ever starts.
	var shutdownFlag int32

	// apiServerReady is set to 1 only once the main API server (port 8443/8080)
	// goroutine is about to start listening. Without this gate, /readyz would
	// report ready as soon as the health server starts (immediately), while
	// the main API server is still blocked behind the JWKS pre-warm inside the
	// route-setup closure below -- causing clients to see "connection reset"
	// for requests sent right after Kubernetes reports the pod Ready.
	var apiServerReady int32

	healthServer, metricsServer := startHealthAndMetricsServers(healthServersParams{
		Config: cfg, AtomicLevel: atomicLevel, Swappable: swappable, DS: ds,
		InteractiveReadiness: interactiveReadiness, ShutdownFlag: &shutdownFlag,
		APIServerReady: &apiServerReady, Logger: logger,
	})

	sessionDrainer, authCleanup := registerAPIRoutes(r, ctx, apiRoutesParams{
		cfg: cfg, infra: k8sInfra, ds: ds, inv: inv, enricher: enricher,
		mgr: mgr, agentMetrics: agentMetrics, instrumentedAudit: instrumentedAudit,
		ogenSrv: ogenSrv, eventEmitter: eventEmitter, interactiveReadiness: interactiveReadiness,
		apiRateLimiter: apiRateLimiter, maxRequestBodySize: maxRequestBodySize, logger: logger,
	})
	// authCleanup releases the JWKS cache goroutines; deferred here in the main
	// scope so they live for the server's lifetime (registerAPIRoutes returns
	// immediately after chi route registration).
	if authCleanup != nil {
		defer authCleanup()
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout intentionally omitted: SSE/MCP streams are long-lived
		// connections that would be killed by a finite WriteTimeout.
	}

	stopHotReload := wireHotReload(ctx, cfg, httpServer, llmRuntimePath, swappable, phaseResolver, logger)
	defer stopHotReload()

	store.StartCleanupLoop(ctx, cfg.Runtime.Session.TTL/2)

	// Route setup (including the JWKS pre-warm inside newAuthMiddleware) has
	// completed by this point; only the network bind remains. Mark the API
	// server ready now so /readyz does not report ready any earlier than this.
	atomic.StoreInt32(&apiServerReady, 1)

	go func() {
		logger.Info("HTTP server listening", "addr", addr)
		var listenErr error
		if httpServer.TLSConfig != nil {
			listenErr = httpServer.ListenAndServeTLS("", "")
		} else {
			listenErr = httpServer.ListenAndServe()
		}
		if listenErr != nil && listenErr != http.ErrServerClosed {
			logger.Error(listenErr, "HTTP server error")
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	atomic.StoreInt32(&shutdownFlag, 1)
	logger.Info("shutting down...")
	mgr.Shutdown()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout(cfg))
	defer cancel()

	if sessionDrainer != nil {
		sessionDrainer.DrainSessions(shutdownCtx)
	}

	shutdownServer(shutdownCtx, httpServer, "API", logger)
	shutdownServer(shutdownCtx, healthServer, "health", logger)
	shutdownServer(shutdownCtx, metricsServer, "metrics", logger)

	eventEmitter.Shutdown()
	if fleetClient != nil {
		_ = fleetClient.Close()
		logger.Info("fleet MCP client closed")
	}
	logger.Info("flushing audit store...")
	auditCleanup()
}

// loadStartupConfig reads and validates the static config and hot-reloadable
// LLM runtime config from disk, resolves LLM API key/OAuth2 credentials, and
// builds the atomic-level-aware logger used for the remainder of the process
// lifetime. Terminates the process (os.Exit(1)) on unrecoverable failures,
// matching the original inline main() behavior.
func loadStartupConfig(configPath, llmRuntimePath string, bootstrapLogger logr.Logger) (*kaconfig.Config, *kaconfig.LLMRuntimeConfig, logr.Logger, zap.AtomicLevel) {
	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		bootstrapLogger.Error(err, "failed to read config", "path", configPath)
		os.Exit(1)
	}
	cfg, err := kaconfig.Load(cfgData)
	if err != nil {
		bootstrapLogger.Error(err, "failed to parse config")
		os.Exit(1)
	}

	atomicLevel := cfg.Runtime.Logging.NewAtomicLevel()
	logger := kubelog.NewLoggerWithAtomicLevel(kubelog.Options{
		ServiceName: "kubernaut-agent",
		Development: cfg.Runtime.Logging.IsConsoleFormat(),
	}, atomicLevel)

	logger.Info("logging configured",
		"level", cfg.Runtime.Logging.Level,
		"format", cfg.Runtime.Logging.Format,
	)

	llmRtData, err := os.ReadFile(llmRuntimePath)
	if err != nil {
		logger.Error(err, "failed to read llm runtime config", "path", llmRuntimePath)
		os.Exit(1)
	}
	llmRuntime, err := kaconfig.LoadLLMRuntime(llmRtData)
	if err != nil {
		logger.Error(err, "failed to parse llm runtime config")
		os.Exit(1)
	}

	resolveLLMCredentials(cfg, llmRuntime, logger)

	if err := cfg.Validate(); err != nil {
		logger.Error(err, "invalid configuration")
		os.Exit(1)
	}
	if err := llmRuntime.Validate(cfg.AI.LLM.Provider); err != nil {
		logger.Error(err, "invalid llm runtime configuration")
		os.Exit(1)
	}

	return cfg, llmRuntime, logger, atomicLevel
}

// resolveLLMCredentials resolves the LLM API key via static config
// apiKeyFile -> runtime apiKeyFile -> credentials dir, then resolves OAuth2
// credentials when enabled. Terminates the process (os.Exit(1)) on OAuth2
// resolution failure, matching the original inline main() behavior.
func resolveLLMCredentials(cfg *kaconfig.Config, llmRuntime *kaconfig.LLMRuntimeConfig, logger logr.Logger) {
	if err := cfg.AI.LLM.ResolveAPIKey(); err != nil {
		logger.Info("static apiKeyFile resolution failed", "error", err)
	}
	if llmRuntime.APIKeyFile != "" {
		data, readErr := os.ReadFile(llmRuntime.APIKeyFile)
		if readErr != nil {
			logger.Info("apiKeyFile resolution failed, falling back to credentials dir",
				"error", readErr, "apiKeyFile", llmRuntime.APIKeyFile)
		} else {
			llmRuntime.APIKey = strings.TrimSpace(string(data))
		}
	}
	if cfg.AI.LLM.APIKey == "" && llmRuntime.APIKey == "" {
		const credDir = "/etc/kubernaut-agent/credentials" // pre-commit:allow-sensitive (mount path)
		llmRuntime.APIKey = credentials.ResolveCredentialsFile(cfg.AI.LLM.Provider, credDir, logger)
	}

	switch cfg.AI.LLM.Provider {
	case "vertex", "vertex_ai":
		if llmRuntime.APIKey == "" {
			logger.Info("GCP provider configured without credentials — requests will use ambient ADC if available",
				"provider", cfg.AI.LLM.Provider)
		}
	}

	if cfg.AI.LLM.OAuth2.Enabled {
		if err := cfg.AI.LLM.OAuth2.ResolveOAuth2Credentials(); err != nil {
			logger.Error(err, "failed to resolve OAuth2 credentials from mounted Secret")
			os.Exit(1)
		}
		logger.Info("OAuth2 credentials resolved from mounted Secret",
			"credentialsDir", cfg.AI.LLM.OAuth2.CredentialsDir)
	}
}
