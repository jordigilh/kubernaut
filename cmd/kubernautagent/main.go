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

	"github.com/jordigilh/kubernaut/pkg/agentclient"
	fleetclient "github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/credentials"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	mcpkg "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	kametrics "github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	karbac "github.com/jordigilh/kubernaut/internal/kubernautagent/rbac"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// parseCLIFlags parses the process command-line flags and returns the
// static-config path, hot-reloadable LLM-runtime config path, and the
// optional HTTP listen address override.
func parseCLIFlags() (configPath, llmRuntimePath, addr string) {
	flag.StringVar(&configPath, "config", "/etc/kubernautagent/config.yaml", "Path to static YAML configuration file")
	flag.StringVar(&llmRuntimePath, "llm-runtime", "/etc/kubernautagent/llm-runtime.yaml", "Path to hot-reloadable LLM runtime configuration")
	flag.StringVar(&addr, "addr", "", "HTTP listen address (overrides config server.port)")
	flag.Parse()
	return configPath, llmRuntimePath, addr
}

// buildDSTokenSource creates the shared #1055 TokenSource used by every
// DataStorage-bound HTTP client (ogen DS client and audit store), logging a
// startup warning when the SA token file is not yet present.
func buildDSTokenSource(cfg *kaconfig.Config, logger logr.Logger) *auth.TokenSource {
	dsTokenSource := auth.NewTokenSource(cfg.Integrations.DataStorage.SATokenPath)
	dsTokenSource.SetLogger(logger.WithName("ds-token-source"))
	if _, err := os.Stat(cfg.Integrations.DataStorage.SATokenPath); err != nil {
		logger.Info("WARNING: SA token file not found at startup, DS/audit API calls will fail auth until file appears",
			"token_path", cfg.Integrations.DataStorage.SATokenPath, "error", err)
	} else {
		logger.Info("SA token source configured (shared cache with token refresh)",
			"token_path", cfg.Integrations.DataStorage.SATokenPath)
	}
	return dsTokenSource
}

// buildRouterAndRateLimiter constructs the chi router and the API rate
// limiter. The caller is responsible for deferring apiRateLimiter.Stop() in
// its own scope so the cleanup goroutine lives for the server's lifetime.
func buildRouterAndRateLimiter(cfg *kaconfig.Config, agentMetrics *kametrics.Metrics, instrumentedAudit audit.AuditStore, logger logr.Logger) (chi.Router, *kaserver.RateLimiter) {
	r := chi.NewRouter()
	apiRateLimiter := kaserver.NewRateLimiter(kaserver.RateLimitConfig{
		RequestsPerSecond: cfg.Runtime.Server.RateLimit.RequestsPerSecond,
		Burst:             cfg.Runtime.Server.RateLimit.Burst,
		CleanupInterval:   cfg.Runtime.Server.RateLimit.CleanupInterval,
		MaxAge:            cfg.Runtime.Server.RateLimit.MaxAge,
		TrustedProxyCIDRs: cfg.Runtime.Server.RateLimit.TrustedProxyCIDRs,
	}, agentMetrics.HTTPRateLimitedTotal, kaserver.WithAuditStore(instrumentedAudit, logger))
	return r, apiRateLimiter
}

// apiServerStartParams groups the dependencies needed to register routes,
// construct the HTTP server, wire hot-reload, and start serving. Extracted
// per AGENTS.md's 8+-param Options-pattern rule.
type apiServerStartParams struct {
	r                  chi.Router
	cfg                *kaconfig.Config
	addr               string
	llmRuntimePath     string
	swappable          *llm.SwappableClient
	phaseResolver      *investigator.DefaultPhaseResolver
	core               *coreServices
	inv                *investigator.Investigator
	mgr                *session.Manager
	store              *session.Store
	agentMetrics       *kametrics.Metrics
	instrumentedAudit  audit.AuditStore
	ogenSrv            *agentclient.Server
	apiRateLimiter     *kaserver.RateLimiter
	maxRequestBodySize int64
	apiServerReady     *int32
	logger             logr.Logger
}

// startAPIServer registers API routes, builds the HTTP server, wires
// hot-reload watchers, starts the session cleanup loop, marks the API server
// ready, and launches the listen goroutine. Extracted from main() to keep
// main() under the funlen statement budget (GO-ANTIPATTERN-AUDIT-2026-07-01
// complexity remediation, Wave C).
//
// Returns the constructed httpServer (needed for shutdown), the session
// drainer (needed for graceful drain), and a combined cleanup function that
// releases the JWKS auth cache and stops the hot-reload watchers — the
// caller must defer this in its own scope so these live for the server's
// lifetime, not just this function's call.
func startAPIServer(ctx context.Context, p apiServerStartParams) (httpServer *http.Server, sessionDrainer *mcpkg.SessionDrainer, cleanup func()) {
	sessionDrainer, authCleanup := registerAPIRoutes(p.r, ctx, apiRoutesParams{
		cfg: p.cfg, infra: p.core.infra, ds: p.core.ds, inv: p.inv, enricher: p.core.enricher,
		mgr: p.mgr, agentMetrics: p.agentMetrics, instrumentedAudit: p.instrumentedAudit,
		ogenSrv: p.ogenSrv, eventEmitter: p.core.eventEmitter, interactiveReadiness: p.core.interactiveReadiness,
		apiRateLimiter: p.apiRateLimiter, maxRequestBodySize: p.maxRequestBodySize, logger: p.logger,
	})

	httpServer = &http.Server{
		Addr:              p.addr,
		Handler:           p.r,
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout intentionally omitted: SSE/MCP streams are long-lived
		// connections that would be killed by a finite WriteTimeout.
	}

	stopHotReload := wireHotReload(ctx, p.cfg, httpServer, p.llmRuntimePath, p.swappable, p.phaseResolver, p.logger)

	p.store.StartCleanupLoop(ctx, p.cfg.Runtime.Session.TTL/2)

	// Route setup (including the JWKS pre-warm inside newAuthMiddleware) has
	// completed by this point; only the network bind remains. Mark the API
	// server ready now so /readyz does not report ready any earlier than this.
	atomic.StoreInt32(p.apiServerReady, 1)

	go func() {
		p.logger.Info("HTTP server listening", "addr", p.addr)
		var listenErr error
		if httpServer.TLSConfig != nil {
			listenErr = httpServer.ListenAndServeTLS("", "")
		} else {
			listenErr = httpServer.ListenAndServe()
		}
		if listenErr != nil && listenErr != http.ErrServerClosed {
			p.logger.Error(listenErr, "HTTP server error")
			os.Exit(1)
		}
	}()

	cleanup = func() {
		// authCleanup releases the JWKS cache goroutines.
		if authCleanup != nil {
			authCleanup()
		}
		stopHotReload()
	}
	return httpServer, sessionDrainer, cleanup
}

// initializeAgent builds the LLM clients, prompt/parser/phase-tool
// dependencies, the DS token source, and the full investigation stack
// (core services + investigator + session store/manager + ogen server).
// Extracted from main() to keep main() under the funlen line budget
// (GO-ANTIPATTERN-AUDIT-2026-07-01 complexity remediation, Wave C).
func initializeAgent(cfg *kaconfig.Config, llmRuntime *kaconfig.LLMRuntimeConfig, logger logr.Logger) (swappable *llm.SwappableClient, core *coreServices, stack *investigationStack) {
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
	dsTokenSource := buildDSTokenSource(cfg, logger)

	core = buildCoreServices(cfg, llmRuntime, swappable, dsTokenSource, phaseTools, logger)

	stack = buildInvestigationRunner(investigationRunnerParams{
		cfg:             cfg,
		llmRuntime:      llmRuntime,
		swappable:       swappable,
		phaseSwappables: phaseSwappables,
		promptBuilder:   promptBuilder,
		resultParser:    resultParser,
		phaseTools:      phaseTools,
		enricher:        core.enricher,
		auditStore:      core.auditStore,
		effectiveLLM:    core.effectiveLLM,
		effectiveReg:    core.effectiveReg,
		alignEvaluator:  core.alignEvaluator,
		alignCfg:        core.alignCfg,
		infra:           core.infra,
		sanitizer:       core.sanitizer,
		anomalyDetector: core.anomalyDetector,
		catalogFetcher:  core.catalogFetcher,
		summarizer:      core.summarizer,
		logger:          logger,
	})
	return swappable, core, stack
}

func main() {
	configPath, llmRuntimePath, addr := parseCLIFlags()

	// Bootstrap logger at INFO for startup; replaced after config is loaded (#875).
	bootstrapLogger := kubelog.NewLogger(kubelog.Options{Level: 0, ServiceName: "kubernaut-agent"})

	cfg, llmRuntime, logger, atomicLevel := loadStartupConfig(configPath, llmRuntimePath, bootstrapLogger)
	defer kubelog.Sync(logger)

	if addr == "" {
		addr = fmt.Sprintf("%s:%d", cfg.Runtime.Server.Address, cfg.Runtime.Server.Port)
	}

	logger.Info("starting Kubernaut Agent", "addr", addr, "config", configPath)

	swappable, core, stack := initializeAgent(cfg, llmRuntime, logger)

	const maxRequestBodySize int64 = 1 << 20 // 1 MiB

	r, apiRateLimiter := buildRouterAndRateLimiter(cfg, stack.agentMetrics, stack.instrumentedAudit, logger)
	defer apiRateLimiter.Stop()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// shutdownFlag/apiServerReady gate /healthz and /readyz: health/metrics
	// servers start BEFORE route setup so probes are served during the JWKS
	// pre-warm, but /readyz must not report ready until the API server is
	// actually about to listen (see startAPIServer).
	var shutdownFlag, apiServerReady int32

	healthServer, metricsServer := startHealthAndMetricsServers(healthServersParams{
		Config: cfg, AtomicLevel: atomicLevel, Swappable: swappable, DS: core.ds,
		InteractiveReadiness: core.interactiveReadiness, ShutdownFlag: &shutdownFlag,
		APIServerReady: &apiServerReady, Logger: logger,
	})

	httpServer, sessionDrainer, cleanupServers := startAPIServer(ctx, apiServerStartParams{
		r: r, cfg: cfg, addr: addr, llmRuntimePath: llmRuntimePath,
		swappable: swappable, phaseResolver: stack.phaseResolver, core: core, inv: stack.inv,
		mgr: stack.mgr, store: stack.store, agentMetrics: stack.agentMetrics, instrumentedAudit: stack.instrumentedAudit,
		ogenSrv: stack.ogenSrv, apiRateLimiter: apiRateLimiter, maxRequestBodySize: maxRequestBodySize,
		apiServerReady: &apiServerReady, logger: logger,
	})
	defer cleanupServers()

	<-ctx.Done()
	atomic.StoreInt32(&shutdownFlag, 1)
	runShutdownSequence(shutdownParams{
		cfg: cfg, mgr: stack.mgr, sessionDrainer: sessionDrainer,
		httpServer: httpServer, healthServer: healthServer, metricsServer: metricsServer,
		eventEmitter: core.eventEmitter, fleetClient: core.fleetClient, auditCleanup: core.auditCleanup,
		logger: logger,
	})
}

// shutdownParams groups the servers/dependencies that runShutdownSequence
// must drain and close on process termination. Extracted per AGENTS.md's
// 8+-param Options-pattern rule.
type shutdownParams struct {
	cfg            *kaconfig.Config
	mgr            *session.Manager
	sessionDrainer *mcpkg.SessionDrainer
	httpServer     *http.Server
	healthServer   *http.Server
	metricsServer  *http.Server
	eventEmitter   *karbac.EventEmitter
	fleetClient    *fleetclient.ResilientClient
	auditCleanup   func()
	logger         logr.Logger
}

// runShutdownSequence drains in-flight sessions, shuts down the API/health/
// metrics HTTP servers, stops the fleet MCP client, and flushes the audit
// store. Extracted from main() to keep main() under the funlen statement
// budget (GO-ANTIPATTERN-AUDIT-2026-07-01 complexity remediation, Wave C).
func runShutdownSequence(p shutdownParams) {
	p.logger.Info("shutting down...")
	p.mgr.Shutdown()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout(p.cfg))
	defer cancel()

	if p.sessionDrainer != nil {
		p.sessionDrainer.DrainSessions(shutdownCtx)
	}

	shutdownServer(shutdownCtx, p.httpServer, "API", p.logger)
	shutdownServer(shutdownCtx, p.healthServer, "health", p.logger)
	shutdownServer(shutdownCtx, p.metricsServer, "metrics", p.logger)

	p.eventEmitter.Shutdown()
	if p.fleetClient != nil {
		_ = p.fleetClient.Close()
		p.logger.Info("fleet MCP client closed")
	}
	p.logger.Info("flushing audit store...")
	p.auditCleanup()
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
