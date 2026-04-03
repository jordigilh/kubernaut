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

	hapiclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/investigation"
	k8stools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	logtools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/logs"
	promtools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/prometheus"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

func main() {
	var (
		configPath string
		addr       string
	)
	flag.StringVar(&configPath, "config", "/etc/kubernautagent/config.yaml", "Path to YAML configuration file")
	flag.StringVar(&addr, "addr", ":8080", "HTTP listen address")
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
	if err := cfg.Validate(); err != nil {
		slogger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	slogger.Info("starting Kubernaut Agent", "addr", addr, "config", configPath)

	llmClient, err := langchaingo.New(cfg.LLM.Provider, cfg.LLM.Endpoint, cfg.LLM.Model, cfg.LLM.APIKey)
	if err != nil {
		slogger.Error("failed to create LLM client", "provider", cfg.LLM.Provider, "error", err)
		os.Exit(1)
	}

	promptBuilder, err := prompt.NewBuilder()
	if err != nil {
		slogger.Error("failed to create prompt builder", "error", err)
		os.Exit(1)
	}

	resultParser := parser.NewResultParser()
	auditStore := audit.NopAuditStore{}
	phaseTools := investigator.DefaultPhaseToolMap()

	reg := buildToolRegistry(cfg, slogger)

	inv := investigator.New(
		llmClient, promptBuilder, resultParser,
		nil, // enricher — requires DataStorage adapter; wired when adapters are ready
		auditStore, slogger,
		cfg.Investigator.MaxTurns, phaseTools,
		reg,
	)

	store := session.NewStore(cfg.Session.TTL)
	mgr := session.NewManager(store, slogger)

	handler := kaserver.NewHandler(mgr, inv, slogger)

	ogenSrv, err := hapiclient.NewServer(handler)
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

// buildToolRegistry creates and populates the tool registry with all available tool sets.
func buildToolRegistry(cfg *kaconfig.Config, logger *slog.Logger) *registry.Registry {
	reg := registry.New()

	if err := registerK8sTools(reg, logger); err != nil {
		logger.Warn("K8s tools registration failed", "error", err)
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

	reg.Register(investigation.NewTodoWriteTool())
	logger.Info("registered TodoWrite tool")

	logger.Info("tool registry ready", "total_tools", len(reg.All()))
	return reg
}

func registerK8sTools(reg *registry.Registry, logger *slog.Logger) error {
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("K8s config not available: %w", err)
	}

	k8sClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("creating K8s client: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	cachedDisc := memory.NewMemCacheClient(k8sClient.Discovery())
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedDisc)
	kindIndex, err := k8stools.BuildKindIndex(k8sClient.Discovery())
	if err != nil {
		logger.Warn("failed to build kind index, using empty index", "error", err)
		kindIndex = make(map[string]schema.GroupKind)
	}
	resolver := k8stools.NewDynamicResolver(dynClient, mapper, kindIndex)

	for _, t := range k8stools.NewAllTools(k8sClient, resolver) {
		reg.Register(t)
	}
	logger.Info("registered K8s tools", "count", len(k8stools.AllToolNames))

	reg.Register(logtools.NewFetchPodLogsTool(k8sClient))
	logger.Info("registered fetch_pod_logs tool")

	mc, mcErr := metricsclient.NewForConfig(kubeConfig)
	if mcErr != nil {
		logger.Error("failed to create metrics client, metrics tools will not be registered", "error", mcErr)
	} else {
		for _, t := range k8stools.NewMetricsTools(k8stools.NewMetricsClient(mc)) {
			reg.Register(t)
		}
		logger.Info("registered metrics tools", "count", len(k8stools.MetricsToolNames))
	}

	return nil
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
