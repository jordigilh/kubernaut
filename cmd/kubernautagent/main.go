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
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"

	hapiclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/investigation"
	k8stools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	promtools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/prometheus"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/summarizer"
	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
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

	// Build tool registry with all available tools.
	reg := registry.New()

	// llm_summarize post-processing for large-output tools (DD-HAPI-019-002)
	llmSummarizer := summarizer.New(llmClient, 30000)

	// K8s tools (requires in-cluster config)
	kubeConfig, kubeErr := ctrl.GetConfig()
	if kubeErr == nil {
		k8sClientset, clientErr := kubernetes.NewForConfig(kubeConfig)
		if clientErr == nil {
			for _, t := range k8stools.NewAllTools(k8sClientset) {
				reg.Register(summarizer.Wrap(t, llmSummarizer))
			}
			slogger.Info("registered K8s tools", "count", len(k8stools.AllToolNames))
		} else {
			slogger.Warn("K8s clientset creation failed, K8s tools unavailable", "error", clientErr)
		}
	} else {
		slogger.Warn("K8s config not available, K8s tools unavailable", "error", kubeErr)
	}

	// Prometheus tools
	if cfg.Tools.Prometheus.URL != "" {
		promClient, promErr := promtools.NewClient(promtools.ClientConfig{
			URL:       cfg.Tools.Prometheus.URL,
			Timeout:   cfg.Tools.Prometheus.Timeout,
			SizeLimit: cfg.Tools.Prometheus.SizeLimit,
		})
		if promErr == nil {
			for _, t := range promtools.NewAllTools(promClient) {
				reg.Register(t)
			}
			slogger.Info("registered Prometheus tools", "count", len(promtools.AllToolNames))
		} else {
			slogger.Warn("Prometheus client creation failed, Prometheus tools unavailable", "error", promErr)
		}
	}

	// TodoWrite — available in all investigation phases
	reg.Register(investigation.NewTodoWriteTool())
	slogger.Info("registered TodoWrite tool")

	inv := investigator.New(
		llmClient, promptBuilder, resultParser,
		nil, // enricher — requires DataStorage adapter; wired in Phase 1B
		reg,
		auditStore, slogger,
		cfg.Investigator.MaxTurns, phaseTools,
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

	// Public endpoints (no auth) — same pattern as Gateway/DataStorage
	r.Get("/health", healthHandler)
	r.Get("/ready", readyHandler)
	r.Get("/config", configHandler(cfg))
	r.Handle("/metrics", promhttp.Handler())

	// DD-AUTH-014: Business endpoints with auth middleware
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
// Falls back to "kubernaut-system" when running outside a cluster.
func detectNamespace() string {
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err == nil && len(data) > 0 {
		return string(data)
	}
	return "kubernaut-system"
}

// newAuthMiddleware creates the DD-AUTH-014 auth middleware using in-cluster K8s config.
// Returns nil when running outside a cluster (local dev, unit tests).
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
