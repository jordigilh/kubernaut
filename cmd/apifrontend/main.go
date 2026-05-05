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
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"go.uber.org/zap"
	adksession "google.golang.org/adk/session"

	agentpkg "github.com/jordigilh/kubernaut-apifrontend/internal/agent"
	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
	"github.com/jordigilh/kubernaut-apifrontend/internal/launcher"
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

	// --- Agent + Session ---
	agentCfg := agentpkg.AgentConfig{
		GCPProject:    envOr("GCP_PROJECT", ""),
		GCPRegion:     envOr("GCP_REGION", "us-central1"),
		Instruction:   "You are the Kubernaut API Frontend agent. Help users triage and remediate Kubernetes incidents.",
		KABaseURL:     envOr("KA_BASE_URL", "http://localhost:8080"),
		KAMCPEndpoint: envOr("KA_MCP_ENDPOINT", "http://localhost:8080/api/v1/mcp/"),
		DSBaseURL:     envOr("DS_BASE_URL", "http://localhost:9090"),
	}
	rootAgent, _, err := agentpkg.NewRootAgent(agentCfg)
	if err != nil {
		return fmt.Errorf("create root agent: %w", err)
	}
	sessionSvc := adksession.InMemoryService()

	// --- A2A Handler ---
	a2aHandler, err := launcher.NewA2AHandler(launcher.A2AConfig{
		Agent:          rootAgent,
		SessionService: sessionSvc,
		AppName:        "kubernaut-apifrontend",
		Logger:         slog.Default(),
	})
	if err != nil {
		return fmt.Errorf("create A2A handler: %w", err)
	}

	// --- MCP Handler ---
	mcpHandler, err := handler.NewMCPHandler(handler.MCPConfig{
		ServerName:    "kubernaut-apifrontend",
		ServerVersion: "0.1.0",
		Tools:         handler.DefaultMCPTools(),
	})
	if err != nil {
		return fmt.Errorf("create MCP handler: %w", err)
	}

	// --- Agent Card ---
	agentCardHandler, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
		Name:        "kubernaut-apifrontend",
		Description: "Kubernaut API Frontend agent for Kubernetes incident triage and remediation",
		URL:         fmt.Sprintf("https://localhost:%s", envOr("PORT", "8443")),
		Version:     "0.1.0",
		Skills:      handler.DefaultAgentSkills(),
	})
	if err != nil {
		return fmt.Errorf("create agent card handler: %w", err)
	}

	// --- Auth Middleware Chain ---
	authMiddleware := buildAuthMiddleware(validator, logger, auditor, ipLimiter, userLimiter, metricsReg)

	// --- Router ---
	rootHandler, err := handler.NewRouter(handler.RouterConfig{
		MetricsRegistry:  metricsReg,
		A2AHandler:       a2aHandler,
		MCPHandler:       mcpHandler,
		AgentCardHandler: agentCardHandler,
		AuthMiddleware:   authMiddleware,
		ReadyChecker:     validator.Ready,
	})
	if err != nil {
		return fmt.Errorf("create router: %w", err)
	}

	port := envOr("PORT", "8443")
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
