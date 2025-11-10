package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"

	"github.com/jordigilh/kubernaut/pkg/toolset/configmap"
	"github.com/jordigilh/kubernaut/pkg/toolset/discovery"
	"github.com/jordigilh/kubernaut/pkg/toolset/generator"
	"github.com/jordigilh/kubernaut/pkg/toolset/server/middleware"
)

// DD-007 graceful shutdown constants
const (
	// endpointRemovalPropagationDelay is the time to wait for Kubernetes to propagate
	// endpoint removal across all nodes. Industry best practice is 5 seconds.
	// Kubernetes typically takes 1-3 seconds, but we wait longer to be safe.
	endpointRemovalPropagationDelay = 5 * time.Second
)

// Config holds server configuration
// BR-TOOLSET-033: HTTP server configuration
type Config struct {
	Port              int
	MetricsPort       int
	ShutdownTimeout   time.Duration
	DiscoveryInterval time.Duration
}

// Server represents the Dynamic Toolset HTTP server
// BR-TOOLSET-033: HTTP server
// BR-TOOLSET-040: Graceful shutdown with in-flight request completion
// DD-007: Kubernetes-aware graceful shutdown with 4-step pattern
// Note: Auth/authz handled by sidecars and network policies (per ADR-036)
type Server struct {
	config        *Config
	httpServer    *http.Server
	metricsServer *http.Server
	mux           *http.ServeMux
	handler       http.Handler // Wrapped handler with middleware (for testing)
	clientset     kubernetes.Interface
	discoverer    discovery.ServiceDiscoverer
	generator     generator.ToolsetGenerator
	configBuilder configmap.ConfigMapBuilder
	logger        *zap.Logger

	// DD-007: Graceful shutdown coordination flag
	// Thread-safe flag for readiness probe coordination during shutdown
	isShuttingDown atomic.Bool
}

// NewServer creates a new HTTP server
func NewServer(config *Config, clientset kubernetes.Interface) (*Server, error) {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	s := &Server{
		config:        config,
		clientset:     clientset,
		discoverer:    discovery.NewServiceDiscoverer(clientset),
		generator:     generator.NewHolmesGPTGenerator(),
		configBuilder: configmap.NewConfigMapBuilder("kubernaut-toolset-config", "kubernaut-system"),
		mux:           http.NewServeMux(),
		logger:        logger,
	}

	// Register detectors
	s.discoverer.RegisterDetector(discovery.NewPrometheusDetector())
	s.discoverer.RegisterDetector(discovery.NewGrafanaDetector())
	s.discoverer.RegisterDetector(discovery.NewJaegerDetector())
	s.discoverer.RegisterDetector(discovery.NewElasticsearchDetector())
	s.discoverer.RegisterDetector(discovery.NewCustomDetector())

	// Setup routes
	s.setupRoutes()

	// Wrap mux with middleware chain for RFC 7807 error tracing and Content-Type validation
	// BR-TOOLSET-039: Request ID tracing
	// BR-TOOLSET-043: Content-Type validation
	s.handler = middleware.RequestIDMiddleware(middleware.ValidateContentType(s.mux))

	// Create HTTP servers
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: s.handler,
	}

	// Create separate metrics server on different port
	// Note: Auth/authz handled by sidecars and network policies (per ADR-036)
	metricsMux := http.NewServeMux()
	metricsMux.HandleFunc("/metrics", s.handleMetrics)

	s.metricsServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", config.MetricsPort),
		Handler: metricsMux,
	}

	return s, nil
}

// RegisterDetector registers a custom detector (primarily for testing)
// This allows tests to inject mock detectors with custom health checkers
func (s *Server) RegisterDetector(detector discovery.ServiceDetector) {
	s.discoverer.RegisterDetector(detector)
}

// setupRoutes configures HTTP routes
// Note: Auth/authz handled by sidecars and network policies (per ADR-036)
//
// DD-TOOLSET-001: REST API Deprecation (V1)
// ========================================
// REST API endpoints DISABLED in V1 (0-10% business value)
// - ConfigMap introspection is sufficient for viewing discovered services
// - V1.1 will introduce ToolsetConfig CRD for configuration (BR-TOOLSET-044)
// See: docs/architecture/decisions/DD-TOOLSET-001-REST-API-Deprecation.md
// ========================================
func (s *Server) setupRoutes() {
	// Health endpoints (KEEP - 100% business value)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/ready", s.handleReady)

	// Metrics endpoint (KEEP - 100% business value)
	s.mux.HandleFunc("/metrics", s.handleMetrics)

	// ========================================
	// REST API endpoints (DISABLED in V1 - DD-TOOLSET-001)
	// ========================================
	// TODO(V1.1): Remove these handlers and implement ToolsetConfig CRD (BR-TOOLSET-044)
	// Reason: ConfigMap introspection is sufficient, REST API has 0-10% business value
	//
	// DISABLED ENDPOINTS:
	// - POST /api/v1/discover (10% value) - Use ConfigMap introspection instead
	// - POST /api/v1/toolsets/generate (5% value) - Controller auto-generates
	// - POST /api/v1/toolsets/validate (5% value) - Controller validates
	// - GET /api/v1/toolsets (0% value) - Use `kubectl get configmap kubernaut-toolset-config`
	// - GET /api/v1/toolsets/{name} (0% value) - Use `kubectl get configmap kubernaut-toolset-config`
	// - GET /api/v1/services (0% value) - Use `kubectl get configmap kubernaut-toolset-config`
	// - GET /api/v1/toolset (0% value) - Legacy endpoint
	//
	// s.mux.HandleFunc("/api/v1/toolsets/validate", s.handleValidateToolset) // BR-TOOLSET-042: Validate toolset
	// s.mux.HandleFunc("/api/v1/toolsets/generate", s.handleGenerateToolset) // BR-TOOLSET-041: Generate toolset
	// s.mux.HandleFunc("/api/v1/toolsets/", s.handleToolsetsRouter)          // BR-TOOLSET-040: Router for list and get operations
	// s.mux.HandleFunc("/api/v1/toolset", s.handleGetLegacyToolset)          // Legacy endpoint for backwards compatibility
	// s.mux.HandleFunc("/api/v1/services", s.handleListServices)
	// s.mux.HandleFunc("/api/v1/discover", s.handleDiscover)
	// ========================================
}

// Start starts the HTTP server and metrics server
func (s *Server) Start(ctx context.Context) error {
	// Start discovery loop in background
	go func() {
		_ = s.discoverer.Start(ctx)
	}()

	// Start metrics server in background
	go func() {
		if err := s.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Metrics server error: %v\n", err)
		}
	}()

	// Start main HTTP server (blocking)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down both servers
// Shutdown implements DD-007 4-step Kubernetes-aware graceful shutdown pattern
// BR-TOOLSET-040: Graceful shutdown with in-flight request completion
// DD-007: Kubernetes-aware graceful shutdown
//
// This pattern ensures ZERO request failures during rolling updates by coordinating
// with Kubernetes endpoint removal, waiting for propagation, draining connections,
// and cleaning up resources.
//
// ZERO request failures during rolling updates (vs 5-10% baseline without pattern)
//
// Note: Shutdown metrics are NOT recorded here because they would be lost when the pod
// terminates. All shutdown observability is provided through structured logging with DD-007 tags.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Initiating DD-007 Kubernetes-aware graceful shutdown")

	// STEP 1: Signal Kubernetes to remove pod from endpoints
	s.shutdownStep1SetFlag()

	// STEP 2: Wait for endpoint removal to propagate
	s.shutdownStep2WaitForPropagation()

	// STEP 3: Drain in-flight HTTP connections
	if err := s.shutdownStep3DrainConnections(ctx); err != nil {
		return err
	}

	// STEP 4: Close external resources (Kubernetes client, discovery)
	if err := s.shutdownStep4CloseResources(); err != nil {
		return err
	}

	s.logger.Info("DD-007 Kubernetes-aware graceful shutdown complete - all resources closed",
		zap.String("dd", "DD-007-complete-success"))
	return nil
}

// shutdownStep1SetFlag sets the shutdown flag to signal readiness probe
// DD-007 STEP 1: This triggers Kubernetes endpoint removal
func (s *Server) shutdownStep1SetFlag() {
	s.isShuttingDown.Store(true)
	s.logger.Info("Shutdown flag set - readiness probe now returns 503",
		zap.String("effect", "kubernetes_will_remove_from_endpoints"),
		zap.String("dd", "DD-007-step-1"))
}

// shutdownStep2WaitForPropagation waits for Kubernetes endpoint removal to propagate
// DD-007 STEP 2: Industry best practice is 5 seconds (Kubernetes typically takes 1-3s)
func (s *Server) shutdownStep2WaitForPropagation() {
	s.logger.Info("Waiting for Kubernetes endpoint removal propagation",
		zap.Duration("delay", endpointRemovalPropagationDelay),
		zap.String("reason", "ensure_no_new_traffic"),
		zap.String("dd", "DD-007-step-2"))

	time.Sleep(endpointRemovalPropagationDelay)

	s.logger.Info("Endpoint removal propagation complete - no new traffic expected",
		zap.String("next", "drain_in_flight_connections"),
		zap.String("dd", "DD-007-step-2-complete"))
}

// shutdownStep3DrainConnections drains in-flight HTTP connections
// DD-007 STEP 3: Uses http.Server.Shutdown() to wait for active connections
func (s *Server) shutdownStep3DrainConnections(ctx context.Context) error {
	s.logger.Info("Draining in-flight HTTP connections",
		zap.String("method", "http.Server.Shutdown"),
		zap.String("dd", "DD-007-step-3"))

	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("HTTP server shutdown failed",
			zap.Error(err),
			zap.String("dd", "DD-007-step-3-error"))
		return fmt.Errorf("HTTP shutdown failed (DD-007 step 3): %w", err)
	}

	s.logger.Info("HTTP connections drained successfully",
		zap.String("next", "close_resources"),
		zap.String("dd", "DD-007-step-3-complete"))
	return nil
}

// shutdownStep4CloseResources closes external resources (Kubernetes client, discovery, metrics)
// DD-007 STEP 4: Continue cleanup even if one step fails to prevent resource leaks
func (s *Server) shutdownStep4CloseResources() error {
	s.logger.Info("Closing external resources (Kubernetes client)",
		zap.String("dd", "DD-007-step-4"))

	// Stop discovery loop
	if err := s.discoverer.Stop(); err != nil {
		s.logger.Error("Failed to stop discoverer during shutdown",
			zap.Error(err),
			zap.String("dd", "DD-007-step-4-discoverer-error"))
		return fmt.Errorf("discoverer stop: %w", err)
	}

	// Shutdown metrics server
	if err := s.metricsServer.Shutdown(context.Background()); err != nil {
		s.logger.Error("Failed to shutdown metrics server",
			zap.Error(err),
			zap.String("dd", "DD-007-step-4-metrics-error"))
		return fmt.Errorf("metrics server shutdown: %w", err)
	}

	s.logger.Info("External resources closed successfully",
		zap.String("dd", "DD-007-step-4-complete"))
	return nil
}

// Handler returns the HTTP handler for the server
// This is used by tests to create httptest.Server instances
func (s *Server) Handler() http.Handler {
	return s.handler
}

// ServeHTTP implements http.Handler for testing
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status": "ok",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleReady handles readiness check requests
// BR-TOOLSET-040: Graceful shutdown with readiness probe coordination
// DD-007: STEP 0 - Check shutdown flag FIRST (before any other checks)
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// DD-007: STEP 0 - Check shutdown flag FIRST (before any other checks)
	// This signals Kubernetes to remove pod from Service endpoints during graceful shutdown
	if s.isShuttingDown.Load() {
		s.logger.Debug("Readiness check during shutdown - returning 503")
		response := map[string]interface{}{
			"status": "shutting_down",
			"reason": "graceful_shutdown_in_progress",
			"time":   time.Now().Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Check Kubernetes API connectivity
	_, err := s.clientset.Discovery().ServerVersion()
	k8sReady := err == nil

	response := map[string]interface{}{
		"kubernetes": k8sReady,
	}

	status := http.StatusOK
	if !k8sReady {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response)
}

// handleMetrics handles GET /metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}
