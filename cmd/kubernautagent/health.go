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
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"

	kaapi "github.com/jordigilh/kubernaut/internal/kubernautagent/api"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	karbac "github.com/jordigilh/kubernaut/internal/kubernautagent/rbac"
	"github.com/jordigilh/kubernaut/internal/version"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

func shutdownTimeout(cfg *kaconfig.Config) time.Duration {
	if cfg.Runtime.Shutdown.DrainSeconds > 0 {
		return time.Duration(cfg.Runtime.Shutdown.DrainSeconds) * time.Second
	}
	return 30 * time.Second
}

func shutdownServer(ctx context.Context, srv *http.Server, name string, logger logr.Logger) {
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error(err, "server shutdown error", "server", name)
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// readinessHandler returns an http.HandlerFunc that performs real
// dependency checks instead of returning a static 200. Checks:
//   - shutdownFlag: set after SIGTERM/SIGINT received; makes probe fail
//     so k8s stops routing traffic during graceful shutdown.
//   - apiServerReady: set once the main API server goroutine is about to
//     start listening. Guards against the readiness probe reporting ready
//     before the main API server (auth middleware + JWKS pre-warm) has
//     finished initializing, which would otherwise let traffic hit a port
//     that isn't accepting connections yet.
//   - swappable: verifies the LLM client has a non-empty model name
//     (proxy for "LLM client was successfully initialized").
//   - ds: if non-nil, verifies the ogen client is initialized (DS is
//     a soft dependency — nil ds means DS is intentionally unconfigured).
//   - interactive: reports the interactive mode status (#891). This is
//     informational (does not fail the probe) since autonomous mode
//     continues to function even when interactive is soft-disabled.
func readinessHandler(shutdownFlag, apiServerReady *int32, swappable *llm.SwappableClient, ds *dsClients, interactive *karbac.InteractiveReadiness) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if atomic.LoadInt32(shutdownFlag) != 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "not_ready",
				"reason": "shutting_down",
			})
			return
		}

		if atomic.LoadInt32(apiServerReady) == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "not_ready",
				"reason": "api_server_starting",
			})
			return
		}

		if swappable.ModelName() == "" {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "not_ready",
				"reason": "llm_client_not_configured",
			})
			return
		}

		if ds != nil && ds.ogenClient == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "not_ready",
				"reason": "datastorage_client_unavailable",
			})
			return
		}

		resp := map[string]string{
			"status":           "ready",
			"interactive_mode": interactive.StatusString(),
		}
		if reason := interactive.Reason(); reason != "" {
			resp["interactive_reason"] = reason
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func configHandler(cfg *kaconfig.Config, swappable *llm.SwappableClient) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		model := swappable.ModelName()
		sanitized := map[string]interface{}{
			"service":     "kubernaut-agent",
			"version":     version.Version,
			"llm_model":   model,
			"session_ttl": cfg.Runtime.Session.TTL.String(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sanitized)
	}
}

// startHealthAndMetricsServers builds and starts (in background goroutines)
// the health/readiness/config/admin HTTP server and the Prometheus metrics
// server. These are started before API route setup so liveness/readiness
// probes are served even while the JWKS pre-warm (up to 15s) blocks inside
// newAuthMiddleware — otherwise the liveness probe kills the pod before the
// health server ever starts.
func startHealthAndMetricsServers(cfg *kaconfig.Config, atomicLevel zap.AtomicLevel, swappable *llm.SwappableClient, ds *dsClients, interactiveReadiness *karbac.InteractiveReadiness, shutdownFlag, apiServerReady *int32, logger logr.Logger) (*http.Server, *http.Server) {
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", healthHandler)
	healthMux.HandleFunc("/readyz", readinessHandler(shutdownFlag, apiServerReady, swappable, ds, interactiveReadiness))
	healthMux.HandleFunc("/config", configHandler(cfg, swappable))
	if !cfg.Runtime.Server.DisableAdminEndpoints {
		healthMux.Handle("/admin/loglevel", atomicLevel)
	}
	healthMux.HandleFunc("/openapi.json", kaapi.SpecHandler())
	if !cfg.Runtime.Server.DisableProfiling {
		healthMux.HandleFunc("/debug/pprof/", pprof.Index)
		healthMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		healthMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		healthMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		healthMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}
	healthServer := &http.Server{
		Addr:              cfg.Runtime.Server.HealthAddr,
		Handler:           healthMux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:              cfg.Runtime.Server.MetricsAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

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

	return healthServer, metricsServer
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
