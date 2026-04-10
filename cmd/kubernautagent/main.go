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
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	llmtransport "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
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

	slogHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	slogger := slog.New(slogHandler)
	logrLogger := logr.FromSlogHandler(slogHandler)

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
		cfg.LLM.APIKey = resolveCredentialsFile(cfg.LLM.Provider, slogger)
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

	llmClient, err := langchaingo.New(cfg.LLM.Provider, cfg.LLM.Endpoint, cfg.LLM.Model, cfg.LLM.APIKey,
		buildLLMProviderOptions(cfg)...)
	if err != nil {
		slogger.Error("failed to create LLM client", "provider", cfg.LLM.Provider, "error", err)
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

	resultParser := parser.NewResultParser()
	phaseTools := investigator.DefaultPhaseToolMap()

	k8sInfra := initK8sInfra(slogger)
	ds := initDSClients(cfg, k8sInfra, slogger)
	auditStore, auditCleanup := buildAuditStore(cfg, slogger, logrLogger)
	reg := buildToolRegistry(cfg, slogger, k8sInfra, ds)
	enricher := buildEnricher(ds, k8sInfra, auditStore, slogger)
	sanitizer := buildSanitizationPipeline(cfg, slogger)
	anomalyDetector := buildAnomalyDetector(cfg, slogger)
	sum := buildSummarizer(llmClient, cfg, slogger)

	instrumentedLLM := llm.NewInstrumentedClient(llmClient)

	var catalogFetcher investigator.CatalogFetcher
	if ds != nil {
		catalogFetcher = newDSCatalogFetcher(ds, slogger)
		slogger.Info("workflow catalog fetcher enabled (per-request, DD-HAPI-002)")
	} else {
		slogger.Info("workflow catalog fetcher disabled (no DataStorage — dev mode)")
	}

	inv := investigator.New(investigator.Config{
		Client:       instrumentedLLM,
		Builder:      promptBuilder,
		ResultParser: resultParser,
		Enricher:     enricher,
		AuditStore:   auditStore,
		Logger:       slogger,
		MaxTurns:     cfg.Investigator.MaxTurns,
		PhaseTools:   phaseTools,
		Registry:     reg,
		ModelName:    cfg.LLM.Model,
		Pipeline: investigator.Pipeline{
			Sanitizer:       sanitizer,
			AnomalyDetector: anomalyDetector,
			CatalogFetcher:  catalogFetcher,
			Summarizer:      sum,
		},
	})

	store := session.NewStore(cfg.Session.TTL)
	mgr := session.NewManager(store, slogger)

	handler := kaserver.NewHandler(mgr, inv, slogger)

	ogenSrv, err := agentclient.NewServer(handler)
	if err != nil {
		slogger.Error("failed to create ogen server", "error", err)
		os.Exit(1)
	}

	r := chi.NewRouter()

	r.Get("/health", healthHandler)
	r.Get("/ready", readyHandler)
	r.Get("/config", configHandler(cfg))
	r.Handle("/metrics", promhttp.Handler())

	r.Route("/api/v1", func(r chi.Router) {
		authMw := newAuthMiddleware(cfg, logrLogger)
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

		r.Handle("/*", ogenSrv)
	})

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store.StartCleanupLoop(ctx, cfg.Session.TTL/2)

	go func() {
		slogger.Info("HTTP server listening", "addr", addr)
		if listenErr := httpServer.ListenAndServe(); listenErr != nil && listenErr != http.ErrServerClosed {
			slogger.Error("HTTP server error", "error", listenErr)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slogger.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if shutdownErr := httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
		fmt.Fprintf(os.Stderr, "shutdown error: %v\n", shutdownErr)
	}

	slogger.Info("flushing audit store...")
	auditCleanup()
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func readyHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func configHandler(cfg *kaconfig.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		sanitized := map[string]interface{}{
			"service":     "kubernaut-agent",
			"version":     "v1.3",
			"llm_model":   cfg.LLM.Model,
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

	var opts []ogenclient.ClientOption
	if tokenData, err := os.ReadFile(cfg.DataStorage.SATokenPath); err == nil && len(tokenData) > 0 {
		token := string(tokenData)
		opts = append(opts, ogenclient.WithClient(&http.Client{
			Transport: &bearerTransport{base: http.DefaultTransport, token: token},
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
func buildEnricher(ds *dsClients, infra *k8sInfra, auditStore audit.AuditStore, logger *slog.Logger) *enrichment.Enricher {
	if ds == nil {
		return nil
	}
	e := enrichment.NewEnricher(ds.k8sAdapter, ds.dsAdapter, auditStore, logger)
	if infra != nil && infra.dynClient != nil {
		e.WithLabelDetector(enrichment.NewLabelDetector(infra.dynClient))
		logger.Info("label detector enabled (ADR-056)")
	}
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

// buildLLMProviderOptions returns provider-specific LangChainGo options based on config.
func buildLLMProviderOptions(cfg *kaconfig.Config) []langchaingo.Option {
	var opts []langchaingo.Option
	if cfg.LLM.AzureAPIVersion != "" {
		opts = append(opts, langchaingo.WithAzureAPIVersion(cfg.LLM.AzureAPIVersion))
	}
	if cfg.LLM.VertexProject != "" {
		opts = append(opts, langchaingo.WithVertexProject(cfg.LLM.VertexProject))
	}
	if cfg.LLM.VertexLocation != "" {
		opts = append(opts, langchaingo.WithVertexLocation(cfg.LLM.VertexLocation))
	}
	if cfg.LLM.BedrockRegion != "" {
		opts = append(opts, langchaingo.WithBedrockRegion(cfg.LLM.BedrockRegion))
	}

	if rt := buildTransportChain(cfg); rt != nil {
		opts = append(opts, langchaingo.WithHTTPClient(&http.Client{Transport: rt}))
	}
	return opts
}

// buildTransportChain composes the HTTP transport stack for the LLM client.
// Layers are applied inside-out: the innermost transport (DefaultTransport)
// handles the actual HTTP call, and outer layers intercept/decorate.
//
// Chain: StructuredOutputTransport? → AuthHeadersTransport? → OAuth2Transport? → http.DefaultTransport
//
// Returns nil when no custom transports are needed (caller uses provider defaults).
func buildTransportChain(cfg *kaconfig.Config) http.RoundTripper {
	var base http.RoundTripper = http.DefaultTransport
	needsCustom := false

	if cfg.LLM.OAuth2.Enabled {
		base = llmtransport.NewOAuth2ClientCredentialsTransport(cfg.LLM.OAuth2, base)
		needsCustom = true
	}

	if len(cfg.LLM.CustomHeaders) > 0 {
		base = llmtransport.NewAuthHeadersTransport(cfg.LLM.CustomHeaders, base)
		needsCustom = true
	}

	if cfg.LLM.StructuredOutput {
		base = llmtransport.NewStructuredOutputTransport(
			parser.InvestigationResultSchema(),
			base,
		)
		needsCustom = true
	}

	if !needsCustom {
		return nil
	}
	return base
}

// resolveCredentialsFile reads the LLM API key from the Helm-mounted credentials
// directory (/etc/kubernaut-agent/credentials/). The Helm chart mounts the
// credentialsSecretName as a volume; each secret key becomes a file.
// Providers use different env-var names (OPENAI_API_KEY, ANTHROPIC_API_KEY, etc.),
// so we try the provider-specific key first, then fall back to any single file.
func resolveCredentialsFile(provider string, logger *slog.Logger) string {
	const credDir = "/etc/kubernaut-agent/credentials"

	providerKeyFiles := map[string]string{
		"openai":     "OPENAI_API_KEY",
		"anthropic":  "ANTHROPIC_API_KEY",
		"mistral":    "MISTRAL_API_KEY",
		"huggingface": "HUGGINGFACEHUB_API_TOKEN",
	}

	if keyFile, ok := providerKeyFiles[provider]; ok {
		path := filepath.Join(credDir, keyFile)
		if data, err := os.ReadFile(path); err == nil {
			key := strings.TrimSpace(string(data))
			if key != "" {
				logger.Info("resolved LLM API key from credentials file", "path", path)
				return key
			}
		}
	}

	entries, err := os.ReadDir(credDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(credDir, e.Name())
		if data, readErr := os.ReadFile(path); readErr == nil {
			key := strings.TrimSpace(string(data))
			if key != "" {
				logger.Info("resolved LLM API key from credentials file (fallback)", "path", path)
				return key
			}
		}
	}
	return ""
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
	var transport http.RoundTripper
	if tokenData, err := os.ReadFile(cfg.DataStorage.SATokenPath); err == nil && len(tokenData) > 0 {
		transport = &bearerTransport{base: http.DefaultTransport, token: string(tokenData)}
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
func buildSummarizer(llmClient llm.Client, cfg *kaconfig.Config, logger *slog.Logger) *summarizer.Summarizer {
	if cfg.Summarizer.Threshold <= 0 {
		logger.Info("summarizer disabled (threshold <= 0)")
		return nil
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
	}
	logger.Info("anomaly detector enabled",
		"maxToolCallsPerTool", ac.MaxToolCallsPerTool,
		"maxTotalToolCalls", ac.MaxTotalToolCalls,
		"maxRepeatedFailures", ac.MaxRepeatedFailures,
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
		promClient, promErr := promtools.NewClient(promtools.ClientConfig{
			URL:       cfg.Tools.Prometheus.URL,
			Timeout:   cfg.Tools.Prometheus.Timeout,
			SizeLimit: cfg.Tools.Prometheus.SizeLimit,
		})
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

// newAuthMiddleware creates the DD-AUTH-014 auth middleware using in-cluster K8s config.
func newAuthMiddleware(_ *kaconfig.Config, logger logr.Logger) *auth.Middleware {
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		logger.Info("K8s config not available, auth middleware disabled", "error", err)
		return nil
	}

	k8sClientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		logger.Error(err, "failed to create K8s clientset for auth")
		return nil
	}

	authenticator := auth.NewK8sAuthenticator(k8sClientset)
	authorizer := auth.NewK8sAuthorizer(k8sClientset)

	namespace := detectNamespace()

	return auth.NewMiddleware(authenticator, authorizer, auth.MiddlewareConfig{
		Namespace:    namespace,
		Resource:     "services",
		ResourceName: "kubernaut-agent",
		Verb:         "create",
	}, logger)
}
