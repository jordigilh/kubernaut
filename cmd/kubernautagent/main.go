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
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	sharedhealth "github.com/jordigilh/kubernaut/pkg/shared/health"

	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	sharedtransport "github.com/jordigilh/kubernaut/pkg/shared/transport"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	alignprompt "github.com/jordigilh/kubernaut/internal/kubernautagent/alignment/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/credentials"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	mcpkg "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcpadapters "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/adapters"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	kametrics "github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/investigation"
	k8stools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	logtools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/logs"
	promtools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/prometheus"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/sanitization"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/summarizer"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	wfclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
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
	logger := kubelog.NewLogger(kubelog.Options{Level: 0, ServiceName: "kubernaut-agent"})

	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		logger.Error(err, "failed to read config", "path", configPath)
		os.Exit(1)
	}
	cfg, err := kaconfig.Load(cfgData)
	if err != nil {
		logger.Error(err, "failed to parse config")
		os.Exit(1)
	}

	atomicLevel := cfg.Runtime.Logging.NewAtomicLevel()
	logger = kubelog.NewLoggerWithAtomicLevel(kubelog.Options{ServiceName: "kubernaut-agent"}, atomicLevel)
	defer kubelog.Sync(logger)

	logger.Info("log level configured", "level", cfg.Runtime.Logging.Level)

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

	if llmRuntime.APIKey == "" {
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

	if err := cfg.Validate(); err != nil {
		logger.Error(err, "invalid configuration")
		os.Exit(1)
	}
	if err := llmRuntime.Validate(cfg.AI.LLM.Provider); err != nil {
		logger.Error(err, "invalid llm runtime configuration")
		os.Exit(1)
	}

	if addr == "" {
		addr = fmt.Sprintf("%s:%d", cfg.Runtime.Server.Address, cfg.Runtime.Server.Port)
	}

	logger.Info("starting Kubernaut Agent", "addr", addr, "config", configPath)

	llmClient, err := buildLLMClientFromConfig(context.Background(), cfg, llmRuntime)
	if err != nil {
		logger.Error(err, "failed to create LLM client", "provider", cfg.AI.LLM.Provider)
		os.Exit(1)
	}

	swappable, err := llm.NewSwappableClient(llmClient, llmRuntime.Model)
	if err != nil {
		logger.Error(err, "failed to create swappable LLM client")
		os.Exit(1)
	}

	promptBuilder, err := prompt.NewBuilder()
	if err != nil {
		logger.Error(err, "failed to create prompt builder")
		os.Exit(1)
	}

	resultParser := parser.NewResultParser(logger.WithName("parser"))
	phaseTools := investigator.DefaultPhaseToolMap()

	k8sInfra := initK8sInfra(logger)
	ds := initDSClients(cfg, k8sInfra, logger)
	auditStore, auditCleanup := buildAuditStore(cfg, logger)
	reg := buildToolRegistry(cfg, logger, k8sInfra, ds)
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

	var effectiveLLM llm.Client = instrumentedLLM
	var effectiveReg registry.ToolRegistry = reg
	var alignEvaluator *alignment.Evaluator

	if cfg.AI.AlignmentCheck.Enabled {
		var shadowClient llm.Client
		if cfg.AI.AlignmentCheck.LLM == nil {
			shadowClient = instrumentedLLM
			logger.Info("shadow agent shares investigation LLM client")
		} else {
			alignStaticCfg, alignRtCfg := cfg.AI.AlignmentCheck.EffectiveLLM(cfg.AI.LLM, *llmRuntime)
			alignCfgMerge := *cfg
			alignCfgMerge.AI.LLM = alignStaticCfg
			raw, alignErr := buildLLMClientFromConfig(context.Background(), &alignCfgMerge, &alignRtCfg)
			if alignErr != nil {
				logger.Error(alignErr, "alignment check LLM client failed (fail-closed): alignment is enabled but shadow client unavailable")
				os.Exit(1)
			} else {
				shadowClient = llm.NewInstrumentedClient(raw)
				logger.Info("shadow agent using dedicated LLM client", "model", alignRtCfg.Model)
			}
		}
		if shadowClient != nil {
			alignEvaluator = alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
				Timeout:       cfg.AI.AlignmentCheck.Timeout,
				MaxStepTokens: cfg.AI.AlignmentCheck.MaxStepTokens,
				MaxRetries:    cfg.AI.AlignmentCheck.MaxRetries,
			}, alignprompt.SystemPrompt(), alignment.WithLogger(logger))
			effectiveLLM = alignment.NewLLMProxy(instrumentedLLM)
			effectiveReg = alignment.NewToolProxy(reg)
			logger.Info("shadow agent alignment check enabled")
		}
	}

	agentMetrics := kametrics.NewMetrics()

	instrumentedAudit := audit.NewInstrumentedAuditStore(auditStore, agentMetrics.RecordAuditEventEmitted)

	var scopeResolver investigator.ScopeResolver
	if k8sInfra != nil {
		scopeResolver = investigator.NewMapperScopeResolver(k8sInfra.mapper)
	}

	inv := investigator.New(investigator.Config{
		Client:        effectiveLLM,
		Builder:       promptBuilder,
		ResultParser:  resultParser,
		Enricher:      enricher,
		AuditStore:    instrumentedAudit,
		Logger:        logger,
		MaxTurns:      cfg.AI.Investigation.MaxTurns,
		PhaseTools:    phaseTools,
		Registry:      effectiveReg,
		ModelName:     llmRuntime.Model,
		Swappable:     swappable,
		ScopeResolver: scopeResolver,
		Metrics:       agentMetrics,
		Pipeline: investigator.Pipeline{
			Sanitizer:         sanitizer,
			AnomalyDetector:   anomalyDetector,
			CatalogFetcher:    catalogFetcher,
			Summarizer:        sum,
			MaxToolOutputSize: cfg.AI.Summarizer.MaxToolOutputSize,
		},
	})

	var investigationRunner kaserver.InvestigationRunner = inv
	if alignEvaluator != nil {
		investigationRunner = alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
			Inner:                 inv,
			Evaluator:             alignEvaluator,
			VerdictTimeout:        cfg.AI.AlignmentCheck.VerdictTimeout,
			AuditStore:            instrumentedAudit,
			Logger:                logger,
			Mode:                  cfg.AI.AlignmentCheck.Mode,
			CanaryForceEscalation: cfg.AI.AlignmentCheck.Canary.ForceEscalation,
		})
	}

	store := session.NewStore(cfg.Runtime.Session.TTL)
	mgr := session.NewManager(store, logger, instrumentedAudit, agentMetrics)

	handler := kaserver.NewHandler(mgr, investigationRunner, logger, agentMetrics)

	ogenSrv, err := agentclient.NewServer(handler)
	if err != nil {
		logger.Error(err, "failed to create ogen server")
		os.Exit(1)
	}

	r := chi.NewRouter()

	// Issue #753: /config remains on API port; health, readiness and metrics move to dedicated ports
	r.Get("/config", configHandler(cfg, swappable))

	apiRateLimiter := kaserver.NewRateLimiter(kaserver.RateLimitConfig{
		RequestsPerSecond: cfg.Runtime.Server.RateLimit.RequestsPerSecond,
		Burst:             cfg.Runtime.Server.RateLimit.Burst,
		CleanupInterval:   cfg.Runtime.Server.RateLimit.CleanupInterval,
		MaxAge:            cfg.Runtime.Server.RateLimit.MaxAge,
	}, agentMetrics.HTTPRateLimitedTotal)
	defer apiRateLimiter.Stop()

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(kaserver.HTTPMetricsMiddleware(agentMetrics))
		r.Use(apiRateLimiter.Middleware)

		authMw := newAuthMiddleware(k8sInfra, logger)
		if authMw != nil {
			r.Use(authMw.Handler)
			logger.Info("auth middleware enabled (DD-AUTH-014)",
				"resource", "services",
				"resourceName", "kubernaut-agent",
				"verb", "create",
			)
		} else {
			logger.Info("auth middleware DISABLED (no in-cluster K8s config)")
		}

		if cfg.Interactive.Enabled {
			mcpHandler := buildMCPHandler(cfg, k8sInfra, ds, inv, enricher, mgr, authMw, agentMetrics, logger)
			if mcpHandler != nil {
				// SEC-02: Per-user rate limiting for MCP interactive endpoint.
				userRL := kaserver.NewUserRateLimiter(
					kaserver.DefaultUserRateLimitConfig(cfg.Interactive.RateLimitPerUser),
					agentMetrics.HTTPRateLimitedTotal,
				)
				defer userRL.Stop()

				r.Route("/mcp", func(mcpRouter chi.Router) {
					mcpRouter.Use(userRL.Middleware)
					mcpRouter.Handle("/", kaserver.SSEHeadersMiddleware(mcpHandler))
					mcpRouter.Handle("/*", kaserver.SSEHeadersMiddleware(mcpHandler))
				})
				logger.Info("MCP interactive route mounted",
					"path", "/api/v1/mcp",
					"rateLimitPerUser", cfg.Interactive.RateLimitPerUser,
				)
			} else {
				logger.Error(nil, "MCP interactive mode enabled but handler construction failed (check preceding errors)")
			}
		}

		r.Handle("/*", kaserver.SSEHeadersMiddleware(ogenSrv))
	})

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout intentionally omitted: SSE streams are long-lived
		// connections that would be killed by a finite WriteTimeout.
	}

	// Issue #493: Conditional TLS for the HTTP server
	// Issue #756: CertReloader enables hot-reload of server certificates
	var certReloader *sharedtls.CertReloader
	if cfg.Runtime.Server.TLS.Enabled() {
		isTLS, reloader, tlsErr := sharedtls.ConfigureConditionalTLS(httpServer, cfg.Runtime.Server.TLS.CertDir)
		if tlsErr != nil {
			logger.Error(tlsErr, "Failed to configure TLS")
			os.Exit(1)
		}
		if isTLS {
			certReloader = reloader
			logger.Info("TLS configured for HTTP server", "certDir", cfg.Runtime.Server.TLS.CertDir)
		}
	}

	// Issue #753: Dedicated health and metrics servers (plain HTTP, never TLS)
	healthServer := sharedhealth.NewHealthServer(cfg.Runtime.Server.HealthAddr, healthHandler, readyHandler)
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:              cfg.Runtime.Server.MetricsAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Issue #756: Wire FileWatcher for server cert hot-reload
	if certReloader != nil {
		certWatcher, watchErr := hotreload.NewFileWatcher(
			filepath.Join(cfg.Runtime.Server.TLS.CertDir, "tls.crt"),
			certReloader.ReloadCallback,
			logger.WithName("cert-reloader"),
		)
		if watchErr != nil {
			logger.Error(watchErr, "Failed to create cert file watcher")
			os.Exit(1)
		}
		if err := certWatcher.Start(ctx); err != nil {
			logger.Error(err, "Failed to start cert file watcher")
			os.Exit(1)
		}
		defer certWatcher.Stop()
	}

	// Issue #916: Wire FileWatcher for LLM runtime config hot-reload
	rtCallback := llmRuntimeReloadCallback(cfg, swappable, logger)
	rtWatcher, rtWatchErr := hotreload.NewFileWatcher(
		llmRuntimePath,
		rtCallback,
		logger.WithName("llm-runtime-reloader"),
	)
	if rtWatchErr != nil {
		logger.Info("llm runtime file watcher not started", "error", rtWatchErr)
	} else {
		if err := rtWatcher.Start(ctx); err != nil {
			logger.Info("llm runtime file watcher failed to start", "error", err)
		} else {
			defer rtWatcher.Stop()
			logger.Info("llm runtime hot-reload enabled (#916)", "path", llmRuntimePath)
		}
	}

	// Issue #748: Load OCP TLS security profile from config before any TLS setup
	if err := sharedtls.SetDefaultSecurityProfileFromConfig(cfg.Runtime.Server.TLSProfile); err != nil {
		logger.Error(err, "Invalid TLS security profile in config, using default TLS 1.2")
	} else if cfg.Runtime.Server.TLSProfile != "" {
		logger.Info("TLS security profile active", "profile", cfg.Runtime.Server.TLSProfile)
	}

	// Issue #756: Start CA file watcher for client-side TLS hot-reload
	caWatcher, caWatchErr := sharedtls.StartCAFileWatcher(ctx, logger)
	if caWatchErr != nil {
		logger.Error(caWatchErr, "Failed to start CA file watcher")
		os.Exit(1)
	}
	if caWatcher != nil {
		defer caWatcher.Stop()
	}

	store.StartCleanupLoop(ctx, cfg.Runtime.Session.TTL/2)

	// Issue #753: Start dedicated health and metrics servers
	go func() {
		logger.Info("health server listening", "addr", cfg.Runtime.Server.HealthAddr)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(err, "health server error")
		}
	}()
	go func() {
		logger.Info("metrics server listening", "addr", cfg.Runtime.Server.MetricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(err, "metrics server error")
		}
	}()

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
	logger.Info("shutting down...")
	mgr.Shutdown()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if shutdownErr := httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
		fmt.Fprintf(os.Stderr, "API server shutdown error: %v\n", shutdownErr)
	}
	if shutdownErr := healthServer.Shutdown(shutdownCtx); shutdownErr != nil {
		fmt.Fprintf(os.Stderr, "health server shutdown error: %v\n", shutdownErr)
	}
	if shutdownErr := metricsServer.Shutdown(shutdownCtx); shutdownErr != nil {
		fmt.Fprintf(os.Stderr, "metrics server shutdown error: %v\n", shutdownErr)
	}

	logger.Info("flushing audit store...")
	auditCleanup()
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func readyHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func configHandler(cfg *kaconfig.Config, swappable *llm.SwappableClient) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		model := swappable.ModelName()
		sanitized := map[string]interface{}{
			"service":     "kubernaut-agent",
			"version":     "v1.4",
			"llm_model":   model,
			"session_ttl": cfg.Runtime.Session.TTL.String(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sanitized)
	}
}

// detectNamespace reads the pod's namespace from the mounted ServiceAccount.
func detectNamespace() string {
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err == nil && len(data) > 0 {
		return strings.TrimSpace(string(data))
	}
	return "kubernaut-system"
}

// k8sInfra holds shared Kubernetes clients created once and reused by
// the tool registry, enricher, and custom tools.
type k8sInfra struct {
	kubeConfig *rest.Config
	clientset  *kubernetes.Clientset
	dynClient  dynamic.Interface
	mapper     meta.RESTMapper
}

// initK8sInfra creates the shared Kubernetes clients. Returns nil when
// running outside a cluster (e.g. local development).
func initK8sInfra(logger logr.Logger) *k8sInfra {
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		logger.Info("K8s config not available, K8s tools and enricher disabled", "error", err)
		return nil
	}

	// SEC-06 (#703): Wrap transport so interactive-mode tool calls impersonate
	// the authenticated user. In autonomous mode (no user in context), the
	// wrapper is a no-op and KA SA credentials are used directly.
	kubeConfig.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return sharedtransport.NewImpersonatingRoundTripper(rt)
	}

	k8sClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		logger.Error(err, "failed to create K8s clientset")
		return nil
	}
	dynClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		logger.Error(err, "failed to create dynamic client")
		return nil
	}
	cachedDisc := memory.NewMemCacheClient(k8sClient.Discovery())
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedDisc)
	return &k8sInfra{kubeConfig: kubeConfig, clientset: k8sClient, dynClient: dynClient, mapper: mapper}
}

// dsClients holds DataStorage client instances created once and shared
// between the enricher and the custom tool registry.
type dsClients struct {
	ogenClient *ogenclient.Client
	dsAdapter  *enrichment.DSAdapter
	k8sAdapter *enrichment.K8sAdapter
}

// initDSClients creates the DataStorage adapter clients. Returns nil when
// DataStorage URL is empty or K8s infrastructure is unavailable.
//
// DD-AUTH-014: When a ServiceAccount token is available (sa_token_path config
// or default /var/run/secrets/kubernetes.io/serviceaccount/token), the ogen
// client is configured with a Bearer token transport so that all DS API calls
// (including ListWorkflows for the workflow validator) pass authentication.
func initDSClients(cfg *kaconfig.Config, infra *k8sInfra, logger logr.Logger) *dsClients {
	if cfg.Integrations.DataStorage.URL == "" {
		logger.Info("DataStorage URL not configured, DS adapters disabled")
		return nil
	}
	if infra == nil {
		logger.Info("K8s infrastructure unavailable, DS adapters disabled")
		return nil
	}

	// Issue #853: Wrapped with RetryTransport for transient failure resilience.
	dsBase, tlsErr := sharedtls.DefaultBaseTransportWithRetry()
	if tlsErr != nil {
		logger.Error(tlsErr, "failed to create TLS-aware transport for DS client")
		return nil
	}

	var opts []ogenclient.ClientOption
	if tokenData, err := os.ReadFile(cfg.Integrations.DataStorage.SATokenPath); err == nil && len(tokenData) > 0 {
		token := string(tokenData)
		opts = append(opts, ogenclient.WithClient(&http.Client{
			Transport: &bearerTransport{base: dsBase, token: token},
		}))
		logger.Info("DS client auth configured (DD-AUTH-014)", "token_path", cfg.Integrations.DataStorage.SATokenPath)
	} else {
		logger.Info("SA token not available for DS client — DS API calls may fail auth",
			"path", cfg.Integrations.DataStorage.SATokenPath, "error", err)
	}

	ogenClient, err := ogenclient.NewClient(cfg.Integrations.DataStorage.URL, opts...)
	if err != nil {
		logger.Error(err, "failed to create DataStorage ogen client", "url", cfg.Integrations.DataStorage.URL)
		return nil
	}
	logger.Info("DataStorage clients initialized", "url", cfg.Integrations.DataStorage.URL)
	return &dsClients{
		ogenClient: ogenClient,
		dsAdapter:  enrichment.NewDSAdapter(ogenClient),
		k8sAdapter: enrichment.NewK8sAdapter(infra.dynClient, infra.mapper),
	}
}

// bearerTransport injects a Kubernetes ServiceAccount Bearer token into
// every outbound HTTP request (DD-AUTH-014).
type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(r)
}

// buildEnricher creates the enrichment.Enricher when DS clients are available.
// ADR-056: attaches LabelDetector so detected_labels are populated during enrichment.
// #704: wires RetryConfig from config for HAPI-aligned owner chain retry+fail-hard.
func buildEnricher(cfg *kaconfig.Config, ds *dsClients, infra *k8sInfra, auditStore audit.AuditStore, logger logr.Logger) *enrichment.Enricher {
	if ds == nil {
		return nil
	}
	e := enrichment.NewEnricher(ds.k8sAdapter, ds.dsAdapter, auditStore, logger)
	if infra != nil && infra.dynClient != nil {
		e.WithLabelDetector(enrichment.NewLabelDetector(infra.dynClient, infra.mapper, logger.WithName("label-detector")))
		logger.Info("label detector enabled (ADR-056)")
	}
	e.WithRetryConfig(enrichment.RetryConfig{
		MaxRetries:  cfg.AI.Enrichment.MaxRetries,
		BaseBackoff: cfg.AI.Enrichment.BaseBackoff,
	})
	logger.Info("enrichment retry config wired (#704)",
		"max_retries", cfg.AI.Enrichment.MaxRetries,
		"base_backoff", cfg.AI.Enrichment.BaseBackoff,
	)
	return e
}

// buildSanitizationPipeline creates the G4 + I1 sanitization pipeline
// per DD-HAPI-019-003. Returns nil when both stages are disabled.
func buildSanitizationPipeline(cfg *kaconfig.Config, logger logr.Logger) *sanitization.Pipeline {
	var stages []sanitization.Stage
	if cfg.AI.Safety.Sanitization.CredentialScrubEnabled {
		stages = append(stages, sanitization.NewCredentialSanitizer())
	}
	if cfg.AI.Safety.Sanitization.InjectionPatternsEnabled {
		stages = append(stages, sanitization.NewInjectionSanitizer(nil))
	}
	if len(stages) == 0 {
		logger.Info("sanitization pipeline disabled")
		return nil
	}
	logger.Info("sanitization pipeline enabled", "stages", len(stages))
	return sanitization.NewPipeline(stages...)
}

// buildAuditStore creates a BufferedDSAuditStore (DD-AUDIT-002 aligned) when audit
// is enabled and DS is available, falling back to NopAuditStore otherwise.
// Uses the same OpenAPIClientAdapter + BufferedAuditStore stack as every other
// platform service. Auth transport is shared with initDSClients (same SA token)
// to guarantee identical authentication behavior.
func buildAuditStore(cfg *kaconfig.Config, logger logr.Logger) (audit.AuditStore, func()) {
	nop := func() {}
	if !cfg.Runtime.Audit.Enabled || cfg.Integrations.DataStorage.URL == "" {
		logger.Info("audit store disabled (nop)")
		return audit.NopAuditStore{}, nop
	}

	// Use the same auth transport as initDSClients: read SA token from the
	// configured path and inject as Bearer header on every request.
	auditBase, tlsErr := sharedtls.DefaultBaseTransport()
	if tlsErr != nil {
		logger.Error(tlsErr, "failed to create TLS-aware transport for audit store")
		return audit.NopAuditStore{}, nop
	}

	var transport http.RoundTripper
	if tokenData, err := os.ReadFile(cfg.Integrations.DataStorage.SATokenPath); err == nil && len(tokenData) > 0 {
		transport = &bearerTransport{base: auditBase, token: string(tokenData)}
		logger.Info("audit store auth configured (same SA token as DS client)",
			"token_path", cfg.Integrations.DataStorage.SATokenPath)
	} else {
		logger.Info("SA token not available for audit store — batch writes may fail auth",
			"path", cfg.Integrations.DataStorage.SATokenPath, "error", err)
	}

	dsClient, err := sharedaudit.NewOpenAPIClientAdapterWithTransport(
		cfg.Integrations.DataStorage.URL, 5*time.Second, transport,
	)
	if err != nil {
		logger.Error(err, "failed to create DS audit client, falling back to nop")
		return audit.NopAuditStore{}, nop
	}

	var storeOpts []audit.BufferedDSAuditStoreOption
	if cfg.Runtime.Audit.FlushIntervalSeconds > 0 {
		storeOpts = append(storeOpts, audit.WithFlushInterval(
			time.Duration(cfg.Runtime.Audit.FlushIntervalSeconds*float64(time.Second))))
	}
	if cfg.Runtime.Audit.BufferSize > 0 {
		storeOpts = append(storeOpts, audit.WithBufferSize(cfg.Runtime.Audit.BufferSize))
	}
	if cfg.Runtime.Audit.BatchSize > 0 {
		storeOpts = append(storeOpts, audit.WithBatchSize(cfg.Runtime.Audit.BatchSize))
	}

	store, err := audit.NewBufferedDSAuditStore(dsClient, logger, storeOpts...)
	if err != nil {
		logger.Error(err, "failed to create buffered audit store, falling back to nop")
		return audit.NopAuditStore{}, nop
	}
	logger.Info("audit store enabled (buffered, DD-AUDIT-002 aligned)",
		"ds_url", cfg.Integrations.DataStorage.URL)
	return store, func() {
		if closeErr := store.Close(); closeErr != nil {
			logger.Error(closeErr, "audit store close error")
		}
	}
}

// buildSummarizer creates a tool output summarizer when the threshold is positive.
// When MaxToolOutputSize is configured, it enables pre-truncation to prevent
// the summarizer's own LLM call from exceeding context window limits (#752).
func buildSummarizer(llmClient llm.Client, cfg *kaconfig.Config, logger logr.Logger) *summarizer.Summarizer {
	if cfg.AI.Summarizer.Threshold <= 0 {
		logger.Info("summarizer disabled (threshold <= 0)")
		return nil
	}
	if cfg.AI.Summarizer.MaxToolOutputSize > 0 {
		logger.Info("summarizer enabled with pre-truncation",
			"threshold", cfg.AI.Summarizer.Threshold,
			"max_tool_output_size", cfg.AI.Summarizer.MaxToolOutputSize)
		return summarizer.NewWithMaxInput(llmClient, cfg.AI.Summarizer.Threshold, cfg.AI.Summarizer.MaxToolOutputSize)
	}
	logger.Info("summarizer enabled", "threshold", cfg.AI.Summarizer.Threshold)
	return summarizer.New(llmClient, cfg.AI.Summarizer.Threshold)
}

// buildAnomalyDetector creates the I7 anomaly detector from config thresholds.
func buildAnomalyDetector(cfg *kaconfig.Config, logger logr.Logger) *investigator.AnomalyDetector {
	ac := investigator.AnomalyConfig{
		MaxToolCallsPerTool: cfg.AI.Safety.Anomaly.MaxToolCallsPerTool,
		MaxTotalToolCalls:   cfg.AI.Safety.Anomaly.MaxTotalToolCalls,
		MaxRepeatedFailures: cfg.AI.Safety.Anomaly.MaxRepeatedFailures,
		ExemptPrefixes:      cfg.AI.Safety.Anomaly.ExemptPrefixes,
	}
	logger.Info("anomaly detector enabled",
		"maxToolCallsPerTool", ac.MaxToolCallsPerTool,
		"maxTotalToolCalls", ac.MaxTotalToolCalls,
		"maxRepeatedFailures", ac.MaxRepeatedFailures,
		"exemptPrefixes", ac.ExemptPrefixes,
	)
	return investigator.NewAnomalyDetector(ac, nil)
}

// buildToolRegistry creates and populates the tool registry with all available tool sets.
func buildToolRegistry(cfg *kaconfig.Config, logger logr.Logger, infra *k8sInfra, ds *dsClients) *registry.Registry {
	reg := registry.New()

	if infra != nil {
		registerK8sTools(reg, infra, logger)
	}

	if cfg.Integrations.Tools.Prometheus.URL != "" {
		promCfg := promtools.ClientConfig{
			URL:       cfg.Integrations.Tools.Prometheus.URL,
			Timeout:   cfg.Integrations.Tools.Prometheus.Timeout,
			SizeLimit: cfg.Integrations.Tools.Prometheus.SizeLimit,
		}
		if cfg.Integrations.Tools.Prometheus.TLSCaFile != "" {
			promBase, promTLSErr := sharedtls.NewTLSTransport(cfg.Integrations.Tools.Prometheus.TLSCaFile)
			if promTLSErr != nil {
				logger.Error(promTLSErr, "failed to create Prometheus TLS transport", "ca_file", cfg.Integrations.Tools.Prometheus.TLSCaFile)
			} else {
				promCfg.Transport = auth.NewServiceAccountTransportWithBase(promBase)
				logger.Info("Prometheus client configured with TLS + SA bearer auth", "ca_file", cfg.Integrations.Tools.Prometheus.TLSCaFile)
			}
		}
		promClient, promErr := promtools.NewClient(promCfg)
		if promErr != nil {
			logger.Error(promErr, "failed to create Prometheus client")
		} else {
			for _, t := range promtools.NewAllTools(promClient) {
				reg.Register(t)
			}
			logger.Info("registered Prometheus tools", "count", len(promtools.AllToolNames))
		}
	}

	if ds != nil {
		custom.RegisterAll(reg, ds.ogenClient, ds.dsAdapter, ds.k8sAdapter, logger)
		logger.Info("registered custom tools", "count", len(custom.AllToolNames))
	}

	reg.Register(investigation.NewTodoWriteTool())
	logger.Info("registered TodoWrite tool")

	logger.Info("tool registry ready", "total_tools", len(reg.All()))
	return reg
}

func registerK8sTools(reg *registry.Registry, infra *k8sInfra, logger logr.Logger) {
	kindIndex, err := k8stools.BuildKindIndex(infra.clientset.Discovery())
	if err != nil {
		logger.Info("failed to build kind index, using empty index", "error", err)
		kindIndex = make(map[string]schema.GroupKind)
	}
	resolver := k8stools.NewDynamicResolver(infra.dynClient, infra.mapper, kindIndex)

	for _, t := range k8stools.NewAllTools(infra.clientset, resolver) {
		reg.Register(t)
	}
	logger.Info("registered K8s tools", "count", len(k8stools.AllToolNames))

	reg.Register(logtools.NewFetchPodLogsTool(infra.clientset))
	logger.Info("registered fetch_pod_logs tool")

	mc, mcErr := metricsclient.NewForConfig(infra.kubeConfig)
	if mcErr != nil {
		logger.Error(mcErr, "failed to create metrics client, metrics tools will not be registered")
	} else {
		for _, t := range k8stools.NewMetricsTools(k8stools.NewMetricsClient(mc)) {
			reg.Register(t)
		}
		logger.Info("registered metrics tools", "count", len(k8stools.MetricsToolNames))
	}
}

// dsCatalogFetcher implements investigator.CatalogFetcher by querying
// DataStorage on every call. This removes the boot-time blocking fetch
// that caused #665 (CrashLoopBackOff when the catalog was not yet seeded).
//
// Per DD-HAPI-002 (v1.1+), KA is the sole workflow validator. The catalog
// is fetched per-request so KA always validates against the current catalog
// without needing a restart when workflows are added/removed.
type dsCatalogFetcher struct {
	ds     *dsClients
	logger logr.Logger
}

func newDSCatalogFetcher(ds *dsClients, logger logr.Logger) *dsCatalogFetcher {
	return &dsCatalogFetcher{ds: ds, logger: logger}
}

func (f *dsCatalogFetcher) FetchValidator(ctx context.Context) (*parser.Validator, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := f.ds.ogenClient.ListWorkflows(fetchCtx, ogenclient.ListWorkflowsParams{})
	if err != nil {
		return nil, fmt.Errorf("ListWorkflows call failed: %w", err)
	}

	wlr, ok := resp.(*ogenclient.WorkflowListResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected ListWorkflows response type %T", resp)
	}

	ids := make([]string, 0, len(wlr.Workflows))
	for _, w := range wlr.Workflows {
		if w.WorkflowId.Set {
			ids = append(ids, w.WorkflowId.Value.String())
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("workflow catalog returned 0 workflows")
	}

	validator := parser.NewValidator(ids)
	for _, w := range wlr.Workflows {
		if !w.WorkflowId.Set {
			continue
		}
		wfID := w.WorkflowId.Value.String()
		meta := parser.WorkflowMeta{
			ExecutionEngine: w.ExecutionEngine,
			Version:         w.Version,
			Component:       append([]string(nil), w.Labels.GetComponent()...),
		}
		if w.ExecutionBundle.Set {
			meta.ExecutionBundle = w.ExecutionBundle.Value
		}
		if w.ExecutionBundleDigest.Set {
			meta.ExecutionBundleDigest = w.ExecutionBundleDigest.Value
		}
		if w.ServiceAccountName.Set {
			meta.ServiceAccountName = w.ServiceAccountName.Value
		}
		validator.SetWorkflowMeta(wfID, meta)
	}

	f.logger.Info("workflow catalog fetched (DD-HAPI-002: per-request validation)",
		"allowed_workflows", len(ids))
	return validator, nil
}

// newAuthMiddleware creates the DD-AUTH-014 auth middleware using the shared k8sInfra clientset.
func newAuthMiddleware(infra *k8sInfra, logger logr.Logger) *auth.Middleware {
	if infra == nil || infra.clientset == nil {
		logger.Info("K8s infrastructure not available, auth middleware disabled")
		return nil
	}

	authenticator := auth.NewK8sAuthenticator(infra.clientset)
	authorizer := auth.NewK8sAuthorizer(infra.clientset)

	namespace := detectNamespace()

	return auth.NewMiddleware(authenticator, authorizer, auth.MiddlewareConfig{
		Namespace:    namespace,
		Resource:     "services",
		ResourceName: "kubernaut-agent",
		Verb:         "create",
	}, logger)
}


// buildMCPHandler constructs the fully-wired MCP interactive handler with all
// tools registered. Returns nil if prerequisites are missing (K8s infra or DS).
// PR6a: Production wiring for MCP interactive mode (BR-INTERACTIVE-001..008).
func buildMCPHandler(
	cfg *kaconfig.Config,
	infra *k8sInfra,
	ds *dsClients,
	inv *investigator.Investigator,
	enricher *enrichment.Enricher,
	autoMgr *session.Manager,
	authMw *auth.Middleware,
	agentMetrics *kametrics.Metrics,
	logger logr.Logger,
) http.Handler {
	if infra == nil || infra.kubeConfig == nil {
		logger.Error(nil, "MCP interactive mode: K8s infrastructure unavailable")
		return nil
	}
	if authMw == nil {
		logger.Error(nil, "MCP interactive mode: auth middleware unavailable (DD-AUTH-MCP-001)")
		return nil
	}
	// SEC-05: Investigator is required for the core investigate tool.
	if inv == nil {
		logger.Error(nil, "MCP interactive mode: investigator unavailable")
		return nil
	}

	// Bridge: MCP pkg functions still accept *slog.Logger (pending their own logr migration).
	slogBridge := slog.New(logr.ToSlogHandler(logger))

	// SEC-07: Build controller-runtime client with MCP-specific timeouts.
	mcpRestConfig := *infra.kubeConfig
	mcpRestConfig.Timeout = 10 * time.Second
	mcpRestConfig.QPS = 20
	mcpRestConfig.Burst = 40

	ctrlCli, err := ctrlclient.New(&mcpRestConfig, ctrlclient.Options{})
	if err != nil {
		logger.Error(err, "MCP interactive mode: failed to create controller-runtime client")
		return nil
	}

	namespace := detectNamespace()

	// Session management via K8s Leases (single-driver guarantee).
	leaseOpts := []mcpkg.LeaseOption{
		mcpkg.WithSessionTTL(cfg.Interactive.SessionTTL),
		mcpkg.WithInactivityTimeout(cfg.Interactive.InactivityTimeout),
		mcpkg.WithMaxConcurrentSessions(cfg.Interactive.MaxConcurrentSessions),
	}
	leaseMgr := mcpkg.NewLeaseSessionManagerConcrete(ctrlCli, namespace, slogBridge, leaseOpts...)

	// Context reconstruction from DS audit events (best-effort).
	var recon mcpkg.ContextReconstructor
	if ds != nil {
		recon = mcpkg.NewDSContextReconstructor(ds.ogenClient, 10*time.Second, slogBridge)
	} else {
		recon = &noopReconstructor{}
		logger.Info("MCP interactive mode: DS unavailable — context reconstruction disabled")
	}

	// DelegatingEventStore: bridges MCP SDK session lifecycle to our disconnect
	// handler. Wraps SDK's MemoryEventStore for stream resumption support.
	eventStore := mcpkg.NewDelegatingEventStore()

	// NotificationBus: in-memory pub/sub for audit event delivery (DD-INTERACTIVE-002).
	notifBus := mcpkg.NewInMemoryNotificationBus(32)
	_ = notifBus // Available for future tool-level publish/subscribe wiring.

	// TimeoutManager: fires onExpire when a session goes inactive (SEC-04, HARM-03/04).
	timeoutMgr := mcpkg.NewTimeoutManager(
		cfg.Interactive.InactivityTimeout,
		[]time.Duration{cfg.Interactive.InactivityTimeout - 2*time.Minute, cfg.Interactive.InactivityTimeout - 30*time.Second},
		func(sessionID string) {
			logger.Info("interactive session expired due to inactivity",
				"session_id", sessionID)
			if err := leaseMgr.Release(sessionID, "inactivity_timeout"); err != nil {
				logger.Error(err, "failed to release expired session",
					"session_id", sessionID)
				return
			}
			// T1-4: Decrement gauge on timeout expiry to prevent drift.
			agentMetrics.RecordInteractiveSessionEnded()
		},
	)

	// ReconstructionSpawner: rebuilds context and spawns autonomous investigation
	// after an interactive session ends (INT-06, BR-INTERACTIVE-008).
	reconRunner := mcpadapters.NewReconRunnerAdapter(inv)
	reconSpawner := mcpkg.NewReconstructionSpawner(reconRunner, recon, slogBridge)

	// SessionClosedHandler: processes MCP disconnect events → release + reconstruct.
	disconnectHandler := mcpkg.NewSessionClosedHandler(eventStore, func(mcpSessionID string) {
		interactiveSessionID, ok := eventStore.LookupInteractiveSession(mcpSessionID)
		if !ok {
			logger.V(1).Info("MCP session closed without interactive mapping (autonomous or already released)",
				"mcp_session_id", mcpSessionID)
			return
		}

		timeoutMgr.StopTracking(interactiveSessionID)

		// T1-1: Snapshot session info BEFORE Release deletes the entry.
		rrID, signalMeta := leaseMgr.GetSessionInfo(interactiveSessionID)

		if err := leaseMgr.Release(interactiveSessionID, "disconnect"); err != nil {
			logger.Info("failed to release disconnected session",
				"session_id", interactiveSessionID,
				"error", err.Error())
			return
		}

		// T1-4: Decrement gauge on disconnect to prevent drift.
		agentMetrics.RecordInteractiveSessionEnded()

		// Spawn reconstruction in background (best-effort, BR-INTERACTIVE-008).
		go func() {
			reconCtx, reconCancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer reconCancel()
			_ = reconSpawner.SpawnReconstruct(reconCtx, &mcpkg.ReconstructionContext{
				CorrelationID: rrID,
				SessionID:     interactiveSessionID,
				SignalMeta:    signalMeta,
			})
		}()
	}, slogBridge)

	// Start disconnect handler goroutine (lifecycle tied to server context).
	go disconnectHandler.Run(context.Background())

	// Build the InvestigatorRunner adapter.
	investigatorRunner := mcpadapters.NewInvestigatorRunnerAdapter(inv)

	// SEC-HIGH-01: Per-session message rate limiter (maxMessageSize = 64KB).
	sessionRateLimiter := mcpkg.NewSessionRateLimiter(cfg.Interactive.RateLimitPerUser, 64*1024)

	// UX-01/02: Session notifier delivers timeout warnings to MCP clients.
	sessionNotifier := mcpkg.NewSessionNotifier()

	// Build the InvestigateTool with optional dependencies.
	investigateOpts := []mcptools.InvestigateOption{
		mcptools.WithToolMetrics(agentMetrics),
		mcptools.WithRateLimiter(sessionRateLimiter),
		mcptools.WithTimeoutTracker(timeoutMgr),
		mcptools.WithNotifyFunc(sessionNotifier.Notify),
	}
	if autoMgr != nil {
		investigateOpts = append(investigateOpts, mcptools.WithAutonomousManager(autoMgr))
	}
	investigateTool := mcptools.NewInvestigateTool(leaseMgr, investigatorRunner, recon, investigateOpts...)

	// Build the EnrichTool (enricher can be nil in dev mode).
	var enrichTool *mcptools.EnrichTool
	if enricher != nil {
		enrichTool = mcptools.NewEnrichTool(enricher, leaseMgr)
	}

	// Build the WorkflowCatalog adapter and SelectWorkflowTool.
	var selectWfTool *mcptools.SelectWorkflowTool
	if ds != nil {
		wfQuerier := wfclient.NewOgenWorkflowQuerier(ds.ogenClient)
		catalogAdapter := mcpadapters.NewWorkflowCatalogAdapter(wfQuerier)
		selectWfTool = mcptools.NewSelectWorkflowTool(catalogAdapter, leaseMgr)
	}

	// Register tools with the MCP SDK server.
	toolDeps := mcpkg.ToolDeps{}
	toolDeps.Investigate = mcptools.InvestigateRegistration(investigateTool, eventStore, sessionNotifier)
	if enrichTool != nil {
		toolDeps.Enrich = mcptools.EnrichRegistration(enrichTool)
	}
	if selectWfTool != nil {
		toolDeps.SelectWorkflow = mcptools.SelectWorkflowRegistration(selectWfTool)
	}

	mcpHandler, _ := mcpkg.BootstrapMCP(mcpkg.MCPDeps{
		AuthMiddleware: authMw.Handler,
		Tools:          toolDeps,
		EventStore:     eventStore,
	})

	logger.Info("MCP interactive mode fully wired",
		"investigate", true,
		"enrich", enrichTool != nil,
		"select_workflow", selectWfTool != nil,
		"event_store", true,
		"timeout_manager", true,
		"disconnect_handler", true,
		"reconstruction_spawner", true,
		"notification_bus", true,
	)

	return mcpHandler
}

// noopReconstructor is a no-op ContextReconstructor used when DS is unavailable.
type noopReconstructor struct{}

func (n *noopReconstructor) Reconstruct(_ context.Context, _ string, _ string) ([]mcpkg.ConversationTurn, error) {
	return nil, nil
}
