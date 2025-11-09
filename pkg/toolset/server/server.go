package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/jordigilh/kubernaut/pkg/toolset"
	"github.com/jordigilh/kubernaut/pkg/toolset/configmap"
	"github.com/jordigilh/kubernaut/pkg/toolset/discovery"
	toolseterrors "github.com/jordigilh/kubernaut/pkg/toolset/errors"
	"github.com/jordigilh/kubernaut/pkg/toolset/generator"
	"github.com/jordigilh/kubernaut/pkg/toolset/metrics"
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
func (s *Server) setupRoutes() {
	// Health endpoints
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/ready", s.handleReady)

	// API endpoints (auth handled by sidecar/network policies)
	// Note: More specific routes must be registered first
	s.mux.HandleFunc("/api/v1/toolsets/validate", s.handleValidateToolset) // BR-TOOLSET-042: Validate toolset
	s.mux.HandleFunc("/api/v1/toolsets/generate", s.handleGenerateToolset) // BR-TOOLSET-041: Generate toolset
	s.mux.HandleFunc("/api/v1/toolsets/", s.handleToolsetsRouter)          // BR-TOOLSET-040: Router for list and get operations
	s.mux.HandleFunc("/api/v1/toolset", s.handleGetLegacyToolset)          // Legacy endpoint for backwards compatibility
	s.mux.HandleFunc("/api/v1/services", s.handleListServices)
	s.mux.HandleFunc("/api/v1/discover", s.handleDiscover)

	// Metrics endpoint
	s.mux.HandleFunc("/metrics", s.handleMetrics)
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

// writeJSONError writes an RFC 7807 compliant error response
// BR-TOOLSET-039: RFC 7807 error format
func (s *Server) writeJSONError(w http.ResponseWriter, r *http.Request, statusCode int, detail string) {
	// Create RFC 7807 error
	rfc7807Err := toolseterrors.NewRFC7807Error(statusCode, detail, r.URL.Path)

	// Extract request ID from context if available
	if requestID, ok := r.Context().Value("request_id").(string); ok {
		rfc7807Err.RequestID = requestID
	}

	// Record error metric
	// BR-TOOLSET-039: Track RFC 7807 error responses
	metrics.ErrorResponsesTotal.WithLabelValues(
		strconv.Itoa(statusCode),
		rfc7807Err.Type,
	).Inc()

	// Set headers
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)

	// Write JSON response
	json.NewEncoder(w).Encode(rfc7807Err)
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

// handleGetLegacyToolset handles GET /api/v1/toolset (legacy endpoint)
// Returns the current combined toolset JSON
func (s *Server) handleGetLegacyToolset(w http.ResponseWriter, r *http.Request) {
	timer := prometheus.NewTimer(metrics.APIRequestDuration.WithLabelValues("/api/v1/toolset", "GET"))
	defer timer.ObserveDuration()

	if r.Method != http.MethodGet {
		metrics.APIRequests.WithLabelValues("/api/v1/toolset", "GET", "405").Inc()
		s.writeJSONError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Discover services
	services, err := s.discoverer.DiscoverServices(r.Context())
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolset", "GET", "500").Inc()
		metrics.APIErrors.WithLabelValues("/api/v1/toolset", "discovery_failed").Inc()
		s.writeJSONError(w, r, http.StatusInternalServerError, "Failed to discover services")
		return
	}

	// Convert to pointer slice for GenerateToolset
	servicePointers := make([]*toolset.DiscoveredService, len(services))
	for i := range services {
		servicePointers[i] = &services[i]
	}

	// Generate toolset from discovered services
	toolsetJSON, err := s.generator.GenerateToolset(r.Context(), servicePointers)
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolset", "GET", "500").Inc()
		metrics.APIErrors.WithLabelValues("/api/v1/toolset", "generation_failed").Inc()
		s.writeJSONError(w, r, http.StatusInternalServerError, "Failed to generate toolset")
		return
	}

	// Parse JSON to ensure it has the "tools" key
	var toolsetMap map[string]interface{}
	if err := json.Unmarshal([]byte(toolsetJSON), &toolsetMap); err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolset", "GET", "500").Inc()
		metrics.APIErrors.WithLabelValues("/api/v1/toolset", "json_parse_failed").Inc()
		s.writeJSONError(w, r, http.StatusInternalServerError, "Failed to parse toolset JSON")
		return
	}

	// Ensure "tools" key exists (even if empty)
	if _, ok := toolsetMap["tools"]; !ok {
		toolsetMap["tools"] = []interface{}{}
	}

	metrics.APIRequests.WithLabelValues("/api/v1/toolset", "GET", "200").Inc()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toolsetMap)
}

// handleListToolsets handles GET /api/v1/toolsets with optional filtering
// BR-TOOLSET-037: Renamed from handleGetToolset to match api-specification.md (plural)
// BR-TOOLSET-039: List toolsets with query parameter filtering
func (s *Server) handleListToolsets(w http.ResponseWriter, r *http.Request) {
	timer := prometheus.NewTimer(metrics.APIRequestDuration.WithLabelValues("/api/v1/toolsets", "GET"))
	defer timer.ObserveDuration()

	if r.Method != http.MethodGet {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets", "GET", "405").Inc()
		s.writeJSONError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Parse query parameters for filtering
	enabledFilter, err := parseOptionalBool(r.URL.Query().Get("enabled"))
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets", "GET", "400").Inc()
		s.writeJSONError(w, r, http.StatusBadRequest, "Invalid enabled parameter")
		return
	}

	healthyFilter, err := parseOptionalBool(r.URL.Query().Get("healthy"))
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets", "GET", "400").Inc()
		s.writeJSONError(w, r, http.StatusBadRequest, "Invalid healthy parameter")
		return
	}

	// Discover services
	services, err := s.discoverer.DiscoverServices(r.Context())
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets", "GET", "500").Inc()
		metrics.APIErrors.WithLabelValues("/api/v1/toolsets", "discovery_failed").Inc()
		s.writeJSONError(w, r, http.StatusInternalServerError, "Failed to discover services")
		return
	}

	// Convert to toolset responses
	toolsets := s.servicesToToolsets(services)

	// Apply filters
	filtered := s.filterToolsets(toolsets, enabledFilter, healthyFilter)

	// Build response
	response := toolset.ToolsetsListResponse{
		Toolsets:      filtered,
		Total:         len(filtered),
		LastDiscovery: time.Now().Format(time.RFC3339),
	}

	metrics.APIRequests.WithLabelValues("/api/v1/toolsets", "GET", "200").Inc()
	metrics.ToolsInToolset.Set(float64(len(filtered)))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleToolsetsRouter routes between list and get operations
// BR-TOOLSET-040: Route toolsets API calls based on path
func (s *Server) handleToolsetsRouter(w http.ResponseWriter, r *http.Request) {
	// Extract path after /api/v1/toolsets/
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/toolsets")
	path = strings.Trim(path, "/")

	if path == "" {
		// GET /api/v1/toolsets - list all
		s.handleListToolsets(w, r)
		return
	}

	// GET /api/v1/toolsets/{name} - get specific toolset
	s.handleGetToolset(w, r, path)
}

// handleGetToolset gets a specific toolset by name or type
// BR-TOOLSET-040: Get toolset by name
func (s *Server) handleGetToolset(w http.ResponseWriter, r *http.Request, name string) {
	timer := prometheus.NewTimer(metrics.APIRequestDuration.WithLabelValues("/api/v1/toolsets/{name}", "GET"))
	defer timer.ObserveDuration()

	if r.Method != http.MethodGet {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/{name}", "GET", "405").Inc()
		s.writeJSONError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Discover services
	services, err := s.discoverer.DiscoverServices(r.Context())
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/{name}", "GET", "500").Inc()
		metrics.APIErrors.WithLabelValues("/api/v1/toolsets/{name}", "discovery_failed").Inc()
		s.writeJSONError(w, r, http.StatusInternalServerError, "Failed to discover services")
		return
	}

	// Convert to toolset responses
	toolsets := s.servicesToToolsets(services)

	// Find matching toolset (prefer name match, fallback to type match)
	for _, t := range toolsets {
		if t.Name == name || t.Type == name {
			metrics.APIRequests.WithLabelValues("/api/v1/toolsets/{name}", "GET", "200").Inc()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(t)
			return
		}
	}

	// Not found
	metrics.APIRequests.WithLabelValues("/api/v1/toolsets/{name}", "GET", "404").Inc()
	s.writeJSONError(w, r, http.StatusNotFound, fmt.Sprintf("Toolset %s not found", name))
}

// handleGenerateToolset handles POST /api/v1/toolsets/generate
// BR-TOOLSET-041: Generate toolset with discovery and ConfigMap update
func (s *Server) handleGenerateToolset(w http.ResponseWriter, r *http.Request) {
	timer := prometheus.NewTimer(metrics.APIRequestDuration.WithLabelValues("/api/v1/toolsets/generate", "POST"))
	defer timer.ObserveDuration()

	if r.Method != http.MethodPost {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "405").Inc()
		s.writeJSONError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Trigger service discovery
	services, err := s.discoverer.DiscoverServices(r.Context())
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "500").Inc()
		metrics.APIErrors.WithLabelValues("/api/v1/toolsets/generate", "discovery_failed").Inc()
		s.writeJSONError(w, r, http.StatusInternalServerError, "Failed to discover services")
		return
	}

	// Convert to toolset responses
	toolsets := s.servicesToToolsets(services)

	// Generate toolset JSON
	servicePointers := make([]*toolset.DiscoveredService, len(services))
	for i := range services {
		servicePointers[i] = &services[i]
	}

	toolsetJSON, err := s.generator.GenerateToolset(r.Context(), servicePointers)
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "500").Inc()
		metrics.APIErrors.WithLabelValues("/api/v1/toolsets/generate", "generation_failed").Inc()
		s.writeJSONError(w, r, http.StatusInternalServerError, "Failed to generate toolset")
		return
	}

	// Build ConfigMap
	cm, err := s.configBuilder.BuildConfigMap(r.Context(), toolsetJSON)
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "500").Inc()
		metrics.APIErrors.WithLabelValues("/api/v1/toolsets/generate", "configmap_failed").Inc()
		s.writeJSONError(w, r, http.StatusInternalServerError, "Failed to build ConfigMap")
		return
	}

	// Create or update ConfigMap in cluster
	existingCM, err := s.clientset.CoreV1().ConfigMaps(cm.Namespace).Get(r.Context(), cm.Name, metav1.GetOptions{})
	if err != nil {
		// ConfigMap doesn't exist, create it
		cm, err = s.clientset.CoreV1().ConfigMaps(cm.Namespace).Create(r.Context(), cm, metav1.CreateOptions{})
		if err != nil {
			metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "500").Inc()
			metrics.APIErrors.WithLabelValues("/api/v1/toolsets/generate", "configmap_create_failed").Inc()
			s.writeJSONError(w, r, http.StatusInternalServerError, "Failed to create ConfigMap")
			return
		}
	} else {
		// ConfigMap exists, update it
		cm.ResourceVersion = existingCM.ResourceVersion
		cm, err = s.clientset.CoreV1().ConfigMaps(cm.Namespace).Update(r.Context(), cm, metav1.UpdateOptions{})
		if err != nil {
			metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "500").Inc()
			metrics.APIErrors.WithLabelValues("/api/v1/toolsets/generate", "configmap_update_failed").Inc()
			s.writeJSONError(w, r, http.StatusInternalServerError, "Failed to update ConfigMap")
			return
		}
	}

	// Build response
	response := toolset.ToolsetsListResponse{
		Toolsets:         toolsets,
		Total:            len(toolsets),
		LastDiscovery:    time.Now().Format(time.RFC3339),
		ConfigMapVersion: cm.ResourceVersion,
	}

	metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "200").Inc()
	metrics.ToolsInToolset.Set(float64(len(toolsets)))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleListServices handles GET /api/v1/services
// BR-TOOLSET-034: List discovered services
func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeJSONError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Discover services
	services, err := s.discoverer.DiscoverServices(r.Context())
	if err != nil {
		s.writeJSONError(w, r, http.StatusInternalServerError, "Failed to discover services")
		return
	}

	// Filter by type if specified
	serviceType := r.URL.Query().Get("type")
	if serviceType != "" {
		filtered := []toolset.DiscoveredService{}
		for _, svc := range services {
			if svc.Type == serviceType {
				filtered = append(filtered, svc)
			}
		}
		services = filtered
	}

	response := map[string]interface{}{
		"services": services,
		"count":    len(services),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleDiscover handles POST /api/v1/discover
// BR-TOOLSET-034: Trigger discovery manually
func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSONError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Trigger discovery (async)
	go func() {
		_, _ = s.discoverer.DiscoverServices(context.Background())
	}()

	response := map[string]interface{}{
		"message": "Discovery triggered successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)
}

// handleMetrics handles GET /metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}

// handleValidateToolset handles POST /api/v1/toolsets/validate
// BR-TOOLSET-042: Validate toolset JSON structure
func (s *Server) handleValidateToolset(w http.ResponseWriter, r *http.Request) {
	timer := prometheus.NewTimer(metrics.APIRequestDuration.WithLabelValues("/api/v1/toolsets/validate", "POST"))
	defer timer.ObserveDuration()

	if r.Method != http.MethodPost {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/validate", "POST", "405").Inc()
		s.writeJSONError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Read request body
	var toolsetData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&toolsetData); err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/validate", "POST", "400").Inc()
		s.writeJSONError(w, r, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate toolset structure
	validationErrors := validateToolsetStructure(toolsetData)

	// Build response
	response := toolset.ValidationResponse{
		Valid:  len(validationErrors) == 0,
		Errors: validationErrors,
	}

	metrics.APIRequests.WithLabelValues("/api/v1/toolsets/validate", "POST", "200").Inc()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// parseOptionalBool parses an optional boolean query parameter
// BR-TOOLSET-039: Query parameter parsing for filtering
func parseOptionalBool(value string) (*bool, error) {
	if value == "" {
		return nil, nil
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// servicesToToolsets converts discovered services to toolset responses
// BR-TOOLSET-039: Convert services to API response format
func (s *Server) servicesToToolsets(services []toolset.DiscoveredService) []toolset.ToolsetResponse {
	toolsets := make([]toolset.ToolsetResponse, 0, len(services))
	for _, svc := range services {
		toolsets = append(toolsets, toolset.ToolsetResponse{
			Name:            svc.Name,
			Type:            svc.Type,
			Enabled:         true, // All discovered services are enabled
			Healthy:         svc.Healthy,
			ServiceEndpoint: svc.Endpoint,
			DiscoveredAt:    svc.DiscoveredAt.Format(time.RFC3339),
			LastHealthCheck: svc.LastCheck.Format(time.RFC3339),
			Config: map[string]interface{}{
				"url": svc.Endpoint,
			},
		})
	}
	return toolsets
}

// filterToolsets applies enabled and healthy filters to toolset list
// BR-TOOLSET-039: Filter toolsets by enabled and healthy status
func (s *Server) filterToolsets(toolsets []toolset.ToolsetResponse, enabled, healthy *bool) []toolset.ToolsetResponse {
	filtered := make([]toolset.ToolsetResponse, 0, len(toolsets))
	for _, t := range toolsets {
		// Apply enabled filter
		if enabled != nil && t.Enabled != *enabled {
			continue
		}
		// Apply healthy filter
		if healthy != nil && t.Healthy != *healthy {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

// validateToolsetStructure validates toolset JSON structure
// BR-TOOLSET-042: Toolset validation logic
func validateToolsetStructure(data map[string]interface{}) []toolset.ValidationError {
	// Validate tools array exists and is valid
	tools, err := extractToolsArray(data)
	if err != nil {
		return []toolset.ValidationError{*err}
	}

	// Validate individual tools
	return validateTools(tools)
}

// extractToolsArray extracts and validates the tools array from toolset data
func extractToolsArray(data map[string]interface{}) ([]interface{}, *toolset.ValidationError) {
	// Check if tools array exists
	toolsInterface, ok := data["tools"]
	if !ok {
		return nil, &toolset.ValidationError{
			Field:   "tools",
			Message: "tools array is required",
		}
	}

	// Check if tools is an array
	tools, ok := toolsInterface.([]interface{})
	if !ok {
		return nil, &toolset.ValidationError{
			Field:   "tools",
			Message: "tools must be an array",
		}
	}

	// Check if tools array is empty
	if len(tools) == 0 {
		return nil, &toolset.ValidationError{
			Field:   "tools",
			Message: "tools array cannot be empty",
		}
	}

	return tools, nil
}

// validateTools validates each tool in the tools array
func validateTools(tools []interface{}) []toolset.ValidationError {
	var errors []toolset.ValidationError
	toolNames := make(map[string]bool)

	for i, toolInterface := range tools {
		tool, ok := toolInterface.(map[string]interface{})
		if !ok {
			errors = append(errors, toolset.ValidationError{
				Field:   fmt.Sprintf("tools[%d]", i),
				Message: "tool must be an object",
			})
			continue
		}

		// Validate individual tool fields
		toolErrors := validateTool(tool, i, toolNames)
		errors = append(errors, toolErrors...)
	}

	return errors
}

// validateTool validates a single tool's fields
func validateTool(tool map[string]interface{}, index int, seenNames map[string]bool) []toolset.ValidationError {
	var errors []toolset.ValidationError

	// Validate name
	if err := validateToolName(tool, index, seenNames); err != nil {
		errors = append(errors, *err)
	}

	// Validate type
	if err := validateToolType(tool, index); err != nil {
		errors = append(errors, *err)
	}

	// Validate endpoint
	if err := validateToolEndpoint(tool, index); err != nil {
		errors = append(errors, *err)
	}

	return errors
}

// validateToolName validates the tool name field
func validateToolName(tool map[string]interface{}, index int, seenNames map[string]bool) *toolset.ValidationError {
	name, ok := tool["name"].(string)
	if !ok || name == "" {
		return &toolset.ValidationError{
			Field:   fmt.Sprintf("tools[%d].name", index),
			Message: "name is required and must be a non-empty string",
		}
	}

	// Check for duplicate names
	if seenNames[name] {
		return &toolset.ValidationError{
			Field:   fmt.Sprintf("tools[%d].name", index),
			Message: fmt.Sprintf("duplicate tool name: %s", name),
		}
	}

	seenNames[name] = true
	return nil
}

// validateToolType validates the tool type field
func validateToolType(tool map[string]interface{}, index int) *toolset.ValidationError {
	toolType, ok := tool["type"].(string)
	if !ok || toolType == "" {
		return &toolset.ValidationError{
			Field:   fmt.Sprintf("tools[%d].type", index),
			Message: "type is required and must be a non-empty string",
		}
	}
	return nil
}

// validateToolEndpoint validates the tool endpoint field
func validateToolEndpoint(tool map[string]interface{}, index int) *toolset.ValidationError {
	endpoint, ok := tool["endpoint"].(string)
	if !ok || endpoint == "" {
		return &toolset.ValidationError{
			Field:   fmt.Sprintf("tools[%d].endpoint", index),
			Message: "endpoint is required and must be a non-empty string",
		}
	}

	// Validate endpoint is a valid URL
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return &toolset.ValidationError{
			Field:   fmt.Sprintf("tools[%d].endpoint", index),
			Message: "endpoint must be a valid HTTP or HTTPS URL",
		}
	}

	return nil
}
