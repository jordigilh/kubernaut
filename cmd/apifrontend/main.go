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
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/logging"
	"github.com/jordigilh/kubernaut-apifrontend/internal/metrics"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ratelimit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/requestid"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	level := zap.NewAtomicLevelAt(zap.InfoLevel)
	logger, err := logging.NewLogger(level)
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	logger = logger.WithValues("service", "kubernaut-apifrontend")

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

	// Authenticated API routes
	apiMux := http.NewServeMux()

	// Middleware chain: RequestID → PreAuth(IP) → Auth(JWT) → PostAuth(User) → Handler
	var apiHandler http.Handler = apiMux
	apiHandler = ratelimit.PostAuthMiddlewareWithConfig(ratelimit.PostAuthMiddlewareConfig{
		Limiter: userLimiter,
		Auditor: auditor,
		Metrics: metricsReg.RateLimitDenied,
	})(apiHandler)
	apiHandler = auth.MiddlewareWithConfig(auth.MiddlewareConfig{
		Validator: validator,
		Logger:    logger,
		Auditor:   auditor,
	})(apiHandler)
	apiHandler = ratelimit.PreAuthMiddlewareWithConfig(ratelimit.PreAuthMiddlewareConfig{
		Limiter: ipLimiter,
		Auditor: auditor,
		Metrics: metricsReg.RateLimitDenied,
	})(apiHandler)
	apiHandler = requestid.Middleware(apiHandler)

	// Top-level mux: infra endpoints are unauthenticated, API routes go through auth chain
	rootMux := http.NewServeMux()
	rootMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	rootMux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		// JWKS provider health
		if !validator.Ready() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("JWKS circuit breaker open"))
			return
		}
		// TODO(PR5+): add K8s API reachability check when controller-runtime
		// manager is integrated (P3 OPS-1 from PR#78 review).
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	rootMux.Handle("/metrics", metricsReg.Handler())
	rootMux.Handle("/", apiHandler)

	// Port 8443 serves plaintext HTTP. TLS termination is handled by the
	// Kubernetes ingress controller or service mesh sidecar (Envoy/Istio).
	// Override with PORT env var if needed (e.g., PORT=8080 for local dev).
	port := os.Getenv("PORT")
	if port == "" {
		port = "8443"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           rootMux,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting kubernaut-apifrontend", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
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

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error(err, "server shutdown error")
	}

	logger.Info("shutdown complete")
	return nil
}
