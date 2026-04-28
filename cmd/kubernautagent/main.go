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
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	sharedhealth "github.com/jordigilh/kubernaut/pkg/shared/health"

	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	alignprompt "github.com/jordigilh/kubernaut/internal/kubernautagent/alignment/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/credentials"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

func main() {
	var (
		configPath    string
		sdkConfigPath string
		addr          string
	)
	flag.StringVar(&configPath, "config", "/etc/kubernautagent/config.yaml", "Path to YAML configuration file")
	flag.StringVar(&sdkConfigPath, "sdk-config", "/etc/kubernaut-agent/sdk/sdk-config.yaml", "Path to SDK configuration file (LLM provider, model, auth, transport)")
	flag.StringVar(&addr, "addr", "", "HTTP listen address (overrides config server.port)")
	flag.Parse()

	// Bootstrap logger at INFO for startup; replaced after config is loaded (#875).
	bootHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	slogger := slog.New(bootHandler)

	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		slogger.Error("failed to read config", "path", configPath, "error", err)
		os.Exit(1)
	}
	cfg, err := kaconfig.Load(cfgData)
	if err != nil {
		slogger.Error("failed to parse config", "error", err)
		os.Exit(1)
	}

	// Re-create logger with configured level (#875).
	slogHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.Logging.SlogLevel()})
	slogger = slog.New(slogHandler)
	logrLogger := logr.FromSlogHandler(slogHandler)

	slogger.Info("log level configured", "level", cfg.Logging.Level)

	if sdkData, sdkErr := os.ReadFile(sdkConfigPath); sdkErr == nil {
		if mergeErr := cfg.MergeSDKConfig(sdkData); mergeErr != nil {
			slogger.Warn("failed to parse SDK config, continuing with main config only",
				"path", sdkConfigPath, "error", mergeErr)
		} else {
			slogger.Info("merged SDK config", "path", sdkConfigPath)
		}
	} else {
		slogger.Info("SDK config not found, using main config only", "path", sdkConfigPath)
	}

	if cfg.LLM.APIKey == "" {
		const credDir = "/etc/kubernaut-agent/credentials" // pre-commit:allow-sensitive (mount path)
		cfg.LLM.APIKey = credentials.ResolveCredentialsFile(cfg.LLM.Provider, credDir, slogger)
	}

	switch cfg.LLM.Provider {
	case "vertex", "vertex_ai":
		if cfg.LLM.APIKey == "" {
			slogger.Warn("GCP provider configured without credentials — requests will use ambient ADC if available",
				"provider", cfg.LLM.Provider)
		}
	}

	if cfg.LLM.OAuth2.Enabled {
		if v := os.Getenv("OAUTH2_CLIENT_ID"); v != "" {
			cfg.LLM.OAuth2.ClientID = v
		}
		if v := os.Getenv("OAUTH2_CLIENT_SECRET"); v != "" {
			cfg.LLM.OAuth2.ClientSecret = v
		}
	}

	if err := cfg.Validate(); err != nil {
		slogger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	if addr == "" {
		addr = fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port)
	}

	slogger.Info("starting Kubernaut Agent", "addr", addr, "config", configPath)

	llmClient, err := buildLLMClientFromConfig(context.Background(), cfg)
	if err != nil {
		slogger.Error("failed to create LLM client", "provider", cfg.LLM.Provider, "error", err)
		os.Exit(1)
	}

	swappable, err := llm.NewSwappableClient(llmClient, cfg.LLM.Model)
	if err != nil {
		slogger.Error("failed to create swappable LLM client", "error", err)
		os.Exit(1)
	}

	var promptOpts []prompt.BuilderOption
	if cfg.LLM.StructuredOutput {
		promptOpts = append(promptOpts, prompt.WithStructuredOutput(true))
	}
	promptBuilder, err := prompt.NewBuilder(promptOpts...)
	if err != nil {
		slogger.Error("failed to create prompt builder", "error", err)
		os.Exit(1)
	}

	resultParser := parser.NewResultParser(logrLogger.WithName("parser"))
	phaseTools := investigator.DefaultPhaseToolMap()

	k8sInfra := initK8sInfra(slogger)
	ds := initDSClients(cfg, k8sInfra, slogger)
	auditStore, auditCleanup := buildAuditStore(cfg, slogger, logrLogger)
	reg := buildToolRegistry(cfg, slogger, k8sInfra, ds)
	enricher := buildEnricher(cfg, ds, k8sInfra, auditStore, slogger)
	sanitizer := buildSanitizationPipeline(cfg, slogger)
	anomalyDetector := buildAnomalyDetector(cfg, slogger)
	sum := buildSummarizer(swappable, cfg, slogger)

	instrumentedLLM := llm.NewInstrumentedClient(swappable)

	var catalogFetcher investigator.CatalogFetcher
	if ds != nil {
		catalogFetcher = newDSCatalogFetcher(ds, slogger)
		slogger.Info("workflow catalog fetcher enabled (per-request, DD-HAPI-002)")
	} else {
		slogger.Info("workflow catalog fetcher disabled (no DataStorage — dev mode)")
	}

	var effectiveLLM llm.Client = instrumentedLLM
	var effectiveReg registry.ToolRegistry = reg
	var alignEvaluator *alignment.Evaluator

	if cfg.AlignmentCheck.Enabled {
		var shadowClient llm.Client
		if cfg.AlignmentCheck.LLM == nil {
			shadowClient = instrumentedLLM
			slogger.Info("shadow agent shares investigation LLM client")
		} else {
			alignLLMCfg := cfg.AlignmentCheck.EffectiveLLM(cfg.LLM)
			alignCfgMerge := *cfg
			alignCfgMerge.LLM = alignLLMCfg
			raw, alignErr := langchaingo.New(
				alignLLMCfg.Provider, alignLLMCfg.Endpoint, alignLLMCfg.Model, alignLLMCfg.APIKey,
				buildLLMProviderOpts(&alignCfgMerge)...)
			if alignErr != nil {
				slogger.Error("alignment check LLM client failed (fail-closed): alignment is enabled but shadow client unavailable", "error", alignErr)
				os.Exit(1)
			} else {
				shadowClient = llm.NewInstrumentedClient(raw)
				slogger.Info("shadow agent using dedicated LLM client", "model", alignLLMCfg.Model)
			}
		}
		if shadowClient != nil {
			alignEvaluator = alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
				Timeout:       cfg.AlignmentCheck.Timeout,
				MaxStepTokens: cfg.AlignmentCheck.MaxStepTokens,
				MaxRetries:    1,
			}, alignprompt.SystemPrompt())
			effectiveLLM = alignment.NewLLMProxy(instrumentedLLM)
			effectiveReg = alignment.NewToolProxy(reg)
			slogger.Info("shadow agent alignment check enabled")
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
		Logger:        slogger,
		MaxTurns:      cfg.Investigator.MaxTurns,
		PhaseTools:    phaseTools,
		Registry:      effectiveReg,
		ModelName:     cfg.LLM.Model,
		Swappable:     swappable,
		ScopeResolver: scopeResolver,
		Metrics:       agentMetrics,
		Pipeline: investigator.Pipeline{
			Sanitizer:         sanitizer,
			AnomalyDetector:   anomalyDetector,
			CatalogFetcher:    catalogFetcher,
			Summarizer:        sum,
			MaxToolOutputSize: cfg.Summarizer.MaxToolOutputSize,
		},
	})

	var investigationRunner kaserver.InvestigationRunner = inv
	if alignEvaluator != nil {
		investigationRunner = alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
			Inner:          inv,
			Evaluator:      alignEvaluator,
			VerdictTimeout: 30 * time.Second,
			AuditStore:     instrumentedAudit,
			Logger:         slogger,
		})
	}

	store := session.NewStore(cfg.Session.TTL)
	mgr := session.NewManager(store, slogger, instrumentedAudit, agentMetrics)

	handler := kaserver.NewHandler(mgr, investigationRunner, slogger, agentMetrics)

	ogenSrv, err := agentclient.NewServer(handler)
	if err != nil {
		slogger.Error("failed to create ogen server", "error", err)
		os.Exit(1)
	}

	r := chi.NewRouter()

	// Issue #753: /config remains on API port; health, readiness and metrics move to dedicated ports
	r.Get("/config", configHandler(cfg, swappable))

	apiRateLimiter := kaserver.NewRateLimiter(kaserver.DefaultRateLimitConfig(), agentMetrics.HTTPRateLimitedTotal)
	defer apiRateLimiter.Stop()

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(kaserver.HTTPMetricsMiddleware(agentMetrics))
		r.Use(apiRateLimiter.Middleware)

		authMw := newAuthMiddleware(k8sInfra, logrLogger)
		if authMw != nil {
			r.Use(authMw.Handler)
			slogger.Info("auth middleware enabled (DD-AUTH-014)",
				"resource", "services",
				"resourceName", "kubernaut-agent",
				"verb", "create",
			)
		} else {
			slogger.Info("auth middleware DISABLED (no in-cluster K8s config)")
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
	if cfg.Server.TLS.Enabled() {
		isTLS, reloader, tlsErr := sharedtls.ConfigureConditionalTLS(httpServer, cfg.Server.TLS.CertDir)
		if tlsErr != nil {
			slogger.Error("Failed to configure TLS", "error", tlsErr)
			os.Exit(1)
		}
		if isTLS {
			certReloader = reloader
			slogger.Info("TLS configured for HTTP server", "certDir", cfg.Server.TLS.CertDir)
		}
	}

	// Issue #753: Dedicated health and metrics servers (plain HTTP, never TLS)
	healthServer := sharedhealth.NewHealthServer(cfg.Server.HealthAddr, healthHandler, readyHandler)
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:              cfg.Server.MetricsAddr,
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
			filepath.Join(cfg.Server.TLS.CertDir, "tls.crt"),
			certReloader.ReloadCallback,
			logr.FromSlogHandler(slogger.Handler()).WithName("cert-reloader"),
		)
		if watchErr != nil {
			slogger.Error("Failed to create cert file watcher", "error", watchErr)
			os.Exit(1)
		}
		if err := certWatcher.Start(ctx); err != nil {
			slogger.Error("Failed to start cert file watcher", "error", err)
			os.Exit(1)
		}
		defer certWatcher.Stop()
	}

	// Issue #783: Wire FileWatcher for SDK config hot-reload
	sdkCallback := sdkReloadCallback(configPath, func() *kaconfig.Config { return cfg }, swappable, slogger)
	sdkWatcher, sdkWatchErr := hotreload.NewFileWatcher(
		sdkConfigPath,
		sdkCallback,
		logr.FromSlogHandler(slogger.Handler()).WithName("sdk-config-reloader"),
	)
	if sdkWatchErr != nil {
		slogger.Warn("SDK config file watcher not started (file may not exist yet)", "error", sdkWatchErr)
	} else {
		if err := sdkWatcher.Start(ctx); err != nil {
			slogger.Warn("SDK config file watcher failed to start", "error", err)
		} else {
			defer sdkWatcher.Stop()
			slogger.Info("SDK config hot-reload enabled (#783)", "path", sdkConfigPath)
		}
	}

	// Issue #748: Load OCP TLS security profile from config before any TLS setup
	if err := sharedtls.SetDefaultSecurityProfileFromConfig(cfg.TLSProfile); err != nil {
		slogger.Error("Invalid TLS security profile in config, using default TLS 1.2", "error", err)
	} else if cfg.TLSProfile != "" {
		slogger.Info("TLS security profile active", "profile", cfg.TLSProfile)
	}

	// Issue #756: Start CA file watcher for client-side TLS hot-reload
	caWatcher, caWatchErr := sharedtls.StartCAFileWatcher(ctx, logr.FromSlogHandler(slogger.Handler()))
	if caWatchErr != nil {
		slogger.Error("Failed to start CA file watcher", "error", caWatchErr)
		os.Exit(1)
	}
	if caWatcher != nil {
		defer caWatcher.Stop()
	}

	store.StartCleanupLoop(ctx, cfg.Session.TTL/2)

	// Issue #753: Start dedicated health and metrics servers
	go func() {
		slogger.Info("health server listening", "addr", cfg.Server.HealthAddr)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slogger.Error("health server error", "error", err)
		}
	}()
	go func() {
		slogger.Info("metrics server listening", "addr", cfg.Server.MetricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slogger.Error("metrics server error", "error", err)
		}
	}()

	go func() {
		slogger.Info("HTTP server listening", "addr", addr)
		var listenErr error
		if httpServer.TLSConfig != nil {
			listenErr = httpServer.ListenAndServeTLS("", "")
		} else {
			listenErr = httpServer.ListenAndServe()
		}
		if listenErr != nil && listenErr != http.ErrServerClosed {
			slogger.Error("HTTP server error", "error", listenErr)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slogger.Info("shutting down...")
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

	slogger.Info("flushing audit store...")
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
		model := cfg.LLM.Model
		if swappable != nil {
			model = swappable.ModelName()
		}
		sanitized := map[string]interface{}{
			"service":     "kubernaut-agent",
			"version":     "v1.3",
			"llm_model":   model,
			"session_ttl": cfg.Session.TTL.String(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sanitized)
	}
}

// detectNamespace reads the pod's namespace from the mounted ServiceAccount.
func detectNamespace() string {
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err == nil && len(data) > 0 {
		return string(data)
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
func initK8sInfra(logger *slog.Logger) *k8sInfra {
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		logger.Warn("K8s config not available, K8s tools and enricher disabled", "error", err)
		return nil
	}
	k8sClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		logger.Error("failed to create K8s clientset", "error", err)
		return nil
	}
	dynClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		logger.Error("failed to create dynamic client", "error", err)
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
func initDSClients(cfg *kaconfig.Config, infra *k8sInfra, logger *slog.Logger) *dsClients {
	if cfg.DataStorage.URL == "" {
		logger.Info("DataStorage URL not configured, DS adapters disabled")
		return nil
	}
	if infra == nil {
		logger.Warn("K8s infrastructure unavailable, DS adapters disabled")
		return nil
	}

	// Issue #853: Wrapped with RetryTransport for transient failure resilience.
	dsBase, tlsErr := sharedtls.DefaultBaseTransportWithRetry()
	if tlsErr != nil {
		logger.Error("failed to create TLS-aware transport for DS client", "error", tlsErr)
		return nil
	}

	var opts []ogenclient.ClientOption
	if tokenData, err := os.ReadFile(cfg.DataStorage.SATokenPath); err == nil && len(tokenData) > 0 {
		token := string(tokenData)
		opts = append(opts, ogenclient.WithClient(&http.Client{
			Transport: &bearerTransport{base: dsBase, token: token},
		}))
		logger.Info("DS client auth configured (DD-AUTH-014)", "token_path", cfg.DataStorage.SATokenPath)
	} else {
		logger.Warn("SA token not available for DS client — DS API calls may fail auth",
			"path", cfg.DataStorage.SATokenPath, "error", err)
	}

	ogenClient, err := ogenclient.NewClient(cfg.DataStorage.URL, opts...)
	if err != nil {
		logger.Error("failed to create DataStorage ogen client", "url", cfg.DataStorage.URL, "error", err)
		return nil
	}
	logger.Info("DataStorage clients initialized", "url", cfg.DataStorage.URL)
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
func buildEnricher(cfg *kaconfig.Config, ds *dsClients, infra *k8sInfra, auditStore audit.AuditStore, logger *slog.Logger) *enrichment.Enricher {
	if ds == nil {
		return nil
	}
	e := enrichment.NewEnricher(ds.k8sAdapter, ds.dsAdapter, auditStore, logger)
	if infra != nil && infra.dynClient != nil {
		e.WithLabelDetector(enrichment.NewLabelDetector(infra.dynClient, infra.mapper))
		logger.Info("label detector enabled (ADR-056)")
	}
	e.WithRetryConfig(enrichment.RetryConfig{
		MaxRetries:  cfg.Enrichment.MaxRetries,
		BaseBackoff: cfg.Enrichment.BaseBackoff,
	})
	logger.Info("enrichment retry config wired (#704)",
		slog.Int("max_retries", cfg.Enrichment.MaxRetries),
		slog.Duration("base_backoff", cfg.Enrichment.BaseBackoff),
	)
	return e
}

// buildSanitizationPipeline creates the G4 + I1 sanitization pipeline
// per DD-HAPI-019-003. Returns nil when both stages are disabled.
func buildSanitizationPipeline(cfg *kaconfig.Config, logger *slog.Logger) *sanitization.Pipeline {
	var stages []sanitization.Stage
	if cfg.Sanitization.CredentialScrubEnabled {
		stages = append(stages, sanitization.NewCredentialSanitizer())
	}
	if cfg.Sanitization.InjectionPatternsEnabled {
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
func buildAuditStore(cfg *kaconfig.Config, slogger *slog.Logger, logrLog logr.Logger) (audit.AuditStore, func()) {
	nop := func() {}
	if !cfg.Audit.Enabled || cfg.DataStorage.URL == "" {
		slogger.Info("audit store disabled (nop)")
		return audit.NopAuditStore{}, nop
	}

	// Use the same auth transport as initDSClients: read SA token from the
	// configured path and inject as Bearer header on every request.
	auditBase, tlsErr := sharedtls.DefaultBaseTransport()
	if tlsErr != nil {
		slogger.Error("failed to create TLS-aware transport for audit store", "error", tlsErr)
		return audit.NopAuditStore{}, nop
	}

	var transport http.RoundTripper
	if tokenData, err := os.ReadFile(cfg.DataStorage.SATokenPath); err == nil && len(tokenData) > 0 {
		transport = &bearerTransport{base: auditBase, token: string(tokenData)}
		slogger.Info("audit store auth configured (same SA token as DS client)",
			"token_path", cfg.DataStorage.SATokenPath)
	} else {
		slogger.Warn("SA token not available for audit store — batch writes may fail auth",
			"path", cfg.DataStorage.SATokenPath, "error", err)
	}

	dsClient, err := sharedaudit.NewOpenAPIClientAdapterWithTransport(
		cfg.DataStorage.URL, 5*time.Second, transport,
	)
	if err != nil {
		slogger.Error("failed to create DS audit client, falling back to nop", "error", err)
		return audit.NopAuditStore{}, nop
	}

	var storeOpts []audit.BufferedDSAuditStoreOption
	if cfg.Audit.FlushIntervalSeconds > 0 {
		storeOpts = append(storeOpts, audit.WithFlushInterval(
			time.Duration(cfg.Audit.FlushIntervalSeconds*float64(time.Second))))
	}
	if cfg.Audit.BufferSize > 0 {
		storeOpts = append(storeOpts, audit.WithBufferSize(cfg.Audit.BufferSize))
	}
	if cfg.Audit.BatchSize > 0 {
		storeOpts = append(storeOpts, audit.WithBatchSize(cfg.Audit.BatchSize))
	}

	store, err := audit.NewBufferedDSAuditStore(dsClient, logrLog, storeOpts...)
	if err != nil {
		slogger.Error("failed to create buffered audit store, falling back to nop", "error", err)
		return audit.NopAuditStore{}, nop
	}
	slogger.Info("audit store enabled (buffered, DD-AUDIT-002 aligned)",
		"ds_url", cfg.DataStorage.URL)
	return store, func() {
		if closeErr := store.Close(); closeErr != nil {
			slogger.Error("audit store close error", "error", closeErr)
		}
	}
}

// buildSummarizer creates a tool output summarizer when the threshold is positive.
// When MaxToolOutputSize is configured, it enables pre-truncation to prevent
// the summarizer's own LLM call from exceeding context window limits (#752).
func buildSummarizer(llmClient llm.Client, cfg *kaconfig.Config, logger *slog.Logger) *summarizer.Summarizer {
	if cfg.Summarizer.Threshold <= 0 {
		logger.Info("summarizer disabled (threshold <= 0)")
		return nil
	}
	if cfg.Summarizer.MaxToolOutputSize > 0 {
		logger.Info("summarizer enabled with pre-truncation",
			"threshold", cfg.Summarizer.Threshold,
			"max_tool_output_size", cfg.Summarizer.MaxToolOutputSize)
		return summarizer.NewWithMaxInput(llmClient, cfg.Summarizer.Threshold, cfg.Summarizer.MaxToolOutputSize)
	}
	logger.Info("summarizer enabled", "threshold", cfg.Summarizer.Threshold)
	return summarizer.New(llmClient, cfg.Summarizer.Threshold)
}

// buildAnomalyDetector creates the I7 anomaly detector from config thresholds.
func buildAnomalyDetector(cfg *kaconfig.Config, logger *slog.Logger) *investigator.AnomalyDetector {
	ac := investigator.AnomalyConfig{
		MaxToolCallsPerTool: cfg.Anomaly.MaxToolCallsPerTool,
		MaxTotalToolCalls:   cfg.Anomaly.MaxTotalToolCalls,
		MaxRepeatedFailures: cfg.Anomaly.MaxRepeatedFailures,
		ExemptPrefixes:      cfg.Anomaly.ExemptPrefixes,
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
func buildToolRegistry(cfg *kaconfig.Config, logger *slog.Logger, infra *k8sInfra, ds *dsClients) *registry.Registry {
	reg := registry.New()

	if infra != nil {
		registerK8sTools(reg, infra, logger)
	}

	if cfg.Tools.Prometheus.URL != "" {
		promCfg := promtools.ClientConfig{
			URL:       cfg.Tools.Prometheus.URL,
			Timeout:   cfg.Tools.Prometheus.Timeout,
			SizeLimit: cfg.Tools.Prometheus.SizeLimit,
		}
		if cfg.Tools.Prometheus.TLSCaFile != "" {
			promBase, promTLSErr := sharedtls.NewTLSTransport(cfg.Tools.Prometheus.TLSCaFile)
			if promTLSErr != nil {
				logger.Error("failed to create Prometheus TLS transport", "error", promTLSErr, "ca_file", cfg.Tools.Prometheus.TLSCaFile)
			} else {
				promCfg.Transport = auth.NewServiceAccountTransportWithBase(promBase)
				logger.Info("Prometheus client configured with TLS + SA bearer auth", "ca_file", cfg.Tools.Prometheus.TLSCaFile)
			}
		}
		promClient, promErr := promtools.NewClient(promCfg)
		if promErr != nil {
			logger.Error("failed to create Prometheus client", "error", promErr)
		} else {
			for _, t := range promtools.NewAllTools(promClient) {
				reg.Register(t)
			}
			logger.Info("registered Prometheus tools", "count", len(promtools.AllToolNames))
		}
	}

	if ds != nil {
		custom.RegisterAll(reg, ds.ogenClient, ds.dsAdapter, ds.k8sAdapter)
		logger.Info("registered custom tools", "count", len(custom.AllToolNames))
	}

	reg.Register(investigation.NewTodoWriteTool())
	logger.Info("registered TodoWrite tool")

	logger.Info("tool registry ready", "total_tools", len(reg.All()))
	return reg
}

func registerK8sTools(reg *registry.Registry, infra *k8sInfra, logger *slog.Logger) {
	kindIndex, err := k8stools.BuildKindIndex(infra.clientset.Discovery())
	if err != nil {
		logger.Warn("failed to build kind index, using empty index", "error", err)
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
		logger.Error("failed to create metrics client, metrics tools will not be registered", "error", mcErr)
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
	logger *slog.Logger
}

func newDSCatalogFetcher(ds *dsClients, logger *slog.Logger) *dsCatalogFetcher {
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

