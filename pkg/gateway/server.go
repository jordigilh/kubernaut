/*
Copyright 2025 Jordi Gil.

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

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"

	"github.com/jordigilh/kubernaut/pkg/audit" // DD-AUDIT-003: Audit integration

	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/middleware" // BR-109: Request ID middleware
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	kubecors "github.com/jordigilh/kubernaut/pkg/http/cors"  // BR-HTTP-015: Shared CORS library
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization" // DD-005: Shared sanitization library
)

// Server is the main Gateway HTTP server
//
// The Gateway Server orchestrates the complete signal-to-CRD pipeline:
//
// 1. Ingestion (via adapters):
//   - Receive webhook from signal source (Prometheus, K8s Events, etc.)
//   - Parse and normalize signal data
//   - Extract metadata (labels, annotations, timestamps)
//
// 2. Processing pipeline:
//   - Deduplication: Check if signal was seen before (K8s status-based per DD-GATEWAY-011)
//   - Storm detection: Identify alert storms (rate-based, pattern-based)
//   - Note: Classification and priority removed (2025-12-06) - now owned by Signal Processing
//
// 3. CRD creation:
//   - Build RemediationRequest CRD from normalized signal
//   - Create CRD in Kubernetes
//   - Update status.deduplication for tracking (DD-GATEWAY-011)
//
// 4. HTTP response:
//   - 201 Created: New RemediationRequest CRD created
//   - 202 Accepted: Duplicate signal (deduplication metadata returned)
//   - 400 Bad Request: Invalid signal payload
//   - 500 Internal Server Error: Processing/API errors
//
// Security features:
// - Authentication: TokenReview-based bearer token validation
// - Rate limiting: Per-IP token bucket (100 req/min, burst 10)
// - Input validation: Schema validation for all signal types
//
// Observability features:
// - Prometheus metrics: 17+ metrics on /metrics endpoint
// - Health/readiness probes: /health and /ready endpoints
// - Structured logging: JSON format with trace IDs
// - Distributed tracing: OpenTelemetry integration (future)
type Server struct {
	// HTTP server
	httpServer *http.Server
	router     chi.Router // Chi router for adapter registration and route grouping

	// Core processing components
	adapterRegistry *adapters.AdapterRegistry
	crdUpdater      *processing.CRDUpdater // DD-GATEWAY-009: CRD updater for state-based deduplication
	// DD-GATEWAY-011 + DD-GATEWAY-012: Status-based deduplication and storm aggregation
	// Redis DEPRECATED - all state now in K8s RR status
	statusUpdater *processing.StatusUpdater                  // Updates RR status.deduplication and status.stormAggregation
	phaseChecker  *processing.PhaseBasedDeduplicationChecker // Phase-based deduplication logic
	crdCreator    *processing.CRDCreator

	// Infrastructure clients
	// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free
	k8sClient  *k8s.Client
	ctrlClient client.Client

	// Configuration (for storm threshold)
	stormThreshold int32 // BR-GATEWAY-182: Threshold for storm detection

	// DD-AUDIT-003: Audit store for async buffered audit event emission
	// Gateway is P0 service - MUST emit audit events per DD-AUDIT-003
	auditStore audit.AuditStore // nil if Data Storage URL not configured (graceful degradation)

	// Metrics
	metricsInstance *metrics.Metrics

	// Logger (DD-005: Unified logr.Logger interface)
	logger logr.Logger

	// Graceful shutdown flag
	// When true, readiness probe returns 503 (not ready)
	// This ensures Kubernetes removes pod from Service endpoints BEFORE
	// we stop accepting new connections via httpServer.Shutdown()
	//
	// WHY: Prevents race condition between readiness probe and listener closure
	// BENEFIT: Guaranteed endpoint removal before in-flight request completion
	//
	// Industry best practice: Google SRE, Netflix, Kubernetes community
	isShuttingDown atomic.Bool
}

// Configuration types have been moved to pkg/gateway/config/config.go
// This improves separation of concerns and allows for better testability

// NewServer creates a new Gateway server with default metrics registry
//
// This initializes:
// - Redis client with connection pooling
// - Kubernetes client (controller-runtime)
// - Processing pipeline components (deduplication, storm, CRD creation)
// - Middleware (authentication, rate limiting)
// - HTTP routes (adapters, health, metrics)
//
// Typical startup sequence:
// 1. Create server: server := NewServer(cfg, logger)
// 2. Register adapters: server.RegisterAdapter(prometheusAdapter)
// 3. Start server: server.Start(ctx)
// 4. Graceful shutdown on signal: server.Stop(ctx)
//
// For testing with isolated metrics, use NewServerWithMetrics() instead.
func NewServer(cfg *config.ServerConfig, logger logr.Logger) (*Server, error) {
	return NewServerWithMetrics(cfg, logger, nil)
}

// NewServerWithMetrics creates a new Gateway server with custom metrics instance
//
// This constructor allows tests to provide isolated Prometheus registries,
// preventing "duplicate metrics collector registration" panics when creating
// multiple Gateway servers in the same test suite.
//
// Usage in tests:
//
//	registry := prometheus.NewRegistry()
//	metricsInstance := metrics.NewMetricsWithRegistry(registry)
//	server, err := gateway.NewServerWithMetrics(cfg, logger, metricsInstance)
//
// If metricsInstance is nil, creates a new metrics instance with the default registry.
// NewServerWithK8sClient creates a Gateway server with an existing K8s client (for testing)
// This ensures the Gateway uses the same K8s client as the test, avoiding cache synchronization issues
// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free, K8s-native service
func NewServerWithK8sClient(cfg *config.ServerConfig, logger logr.Logger, metricsInstance *metrics.Metrics, ctrlClient client.Client) (*Server, error) {
	// Use provided Kubernetes client (shared with test)
	k8sClient := k8s.NewClient(ctrlClient)

	// Initialize processing pipeline components (no Redis)
	return createServerWithClients(cfg, logger, metricsInstance, ctrlClient, k8sClient)
}

func NewServerWithMetrics(cfg *config.ServerConfig, logger logr.Logger, metricsInstance *metrics.Metrics) (*Server, error) {
	// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free, K8s-native service

	// Initialize Kubernetes clients
	// Get kubeconfig with standard Kubernetes precedence:
	// 1. --kubeconfig flag
	// 2. KUBECONFIG environment variable (used in tests: ~/.kube/kind-config)
	// 3. In-cluster config (production)
	// 4. $HOME/.kube/config (default)
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	// Create scheme with RemediationRequest CRD + core K8s types
	// This is required for controller-runtime to work with custom CRDs
	scheme := k8sruntime.NewScheme()
	_ = remediationv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme) // Add core types (Namespace, Pod, etc.)

	// controller-runtime client (for CRD creation)
	ctrlClient, err := client.New(kubeConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create controller-runtime client: %w", err)
	}

	// k8s client wrapper (for CRD operations)
	k8sClient := k8s.NewClient(ctrlClient)

	return createServerWithClients(cfg, logger, metricsInstance, ctrlClient, k8sClient)
}

// createServerWithClients is the common server creation logic
// DD-005: Uses logr.Logger for unified logging interface
// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free, K8s-native service
// DD-AUDIT-003: Audit store initialization for P0 service compliance
func createServerWithClients(cfg *config.ServerConfig, logger logr.Logger, metricsInstance *metrics.Metrics, ctrlClient client.Client, k8sClient *k8s.Client) (*Server, error) {
	// Initialize processing pipeline components
	adapterRegistry := adapters.NewAdapterRegistry(logger)

	// Use provided metrics instance or create new one with default registry
	if metricsInstance == nil {
		metricsInstance = metrics.NewMetrics()
	}

	// DD-GATEWAY-009: Initialize CRD updater for duplicate alert handling
	crdUpdater := processing.NewCRDUpdater(k8sClient, logger)

	// DD-GATEWAY-012: Storm detection via Redis REMOVED
	// Storm status now tracked via status.stormAggregation (occurrence count >= threshold)
	stormThreshold := int32(cfg.Processing.Storm.BufferThreshold)
	if stormThreshold <= 0 {
		stormThreshold = 5 // Default storm threshold
	}
	logger.Info("DD-GATEWAY-012: Redis-free storm detection enabled",
		"storm_threshold", stormThreshold)

	crdCreator := processing.NewCRDCreator(k8sClient, logger, metricsInstance, cfg.Processing.CRD.FallbackNamespace, &cfg.Processing.Retry)

	// DD-GATEWAY-011 + DD-GATEWAY-012: Status-based deduplication and storm aggregation
	// All state in K8s RR status - Redis fully deprecated
	statusUpdater := processing.NewStatusUpdater(ctrlClient)
	phaseChecker := processing.NewPhaseBasedDeduplicationChecker(ctrlClient)

	// DD-AUDIT-003: Initialize audit store for P0 service compliance
	// Gateway MUST emit audit events per DD-AUDIT-003: Service Audit Trace Requirements
	var auditStore audit.AuditStore
	if cfg.Infrastructure.DataStorageURL != "" {
		httpClient := &http.Client{Timeout: 5 * time.Second}
		dsClient := audit.NewHTTPDataStorageClient(cfg.Infrastructure.DataStorageURL, httpClient)
		auditConfig := audit.RecommendedConfig("gateway") // 2x buffer for high-volume service

		var err error
		auditStore, err = audit.NewBufferedStore(dsClient, auditConfig, "gateway", logger)
		if err != nil {
			// Non-fatal: audit is important but not critical for signal processing
			logger.Error(err, "DD-AUDIT-003: Failed to initialize audit store, audit events will be dropped")
		} else {
			logger.Info("DD-AUDIT-003: Audit store initialized for P0 compliance",
				"data_storage_url", cfg.Infrastructure.DataStorageURL,
				"buffer_size", auditConfig.BufferSize)
		}
	} else {
		logger.Info("DD-AUDIT-003: Data Storage URL not configured, audit events will be dropped (WARNING)")
	}

	// Create server (Redis-free)
	server := &Server{
		adapterRegistry: adapterRegistry,
		crdUpdater:      crdUpdater,
		statusUpdater:   statusUpdater,
		phaseChecker:    phaseChecker,
		crdCreator:      crdCreator,
		k8sClient:       k8sClient,
		ctrlClient:      ctrlClient,
		stormThreshold:  stormThreshold,
		auditStore:      auditStore,
		metricsInstance: metricsInstance,
		logger:          logger,
	}

	// 6. Setup HTTP server with routes
	router := server.setupRoutes()

	// 7. Store router reference for adapter registration
	server.router = router

	// 8. Wrap with additional middleware
	// BUSINESS OUTCOME: Enable operators to trace requests across Gateway components
	// TDD GREEN: Minimal implementation to make BR-109 tests pass
	handler := server.wrapWithMiddleware(router)

	server.httpServer = &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	return server, nil
}

// setupRoutes configures all HTTP routes using chi router
//
// Routes:
// - /api/v1/signals/* : Dynamic routes for registered adapters (e.g. /api/v1/signals/prometheus)
// - /health           : Liveness probe (always returns 200)
// - /ready            : Readiness probe (checks Redis + K8s connectivity)
// - /metrics          : Prometheus metrics endpoint
//
// Chi router provides:
// - Route grouping for /api/v1/signals/*
// - HTTP method enforcement (POST only for webhooks)
// - Middleware per route group
func (s *Server) setupRoutes() chi.Router {
	r := chi.NewRouter()

	// BR-HTTP-015: CORS configuration using shared library
	// Configuration is read from environment variables:
	// - CORS_ALLOWED_ORIGINS: Comma-separated list of allowed origins (default: "*" for dev)
	// - CORS_ALLOWED_METHODS: Comma-separated list of allowed methods
	// - CORS_ALLOWED_HEADERS: Comma-separated list of allowed headers
	// - CORS_ALLOW_CREDENTIALS: "true" or "false"
	// - CORS_MAX_AGE: Preflight cache duration in seconds
	corsOpts := kubecors.FromEnvironment()
	if !corsOpts.IsProduction() {
		s.logger.Info("CORS configuration allows all origins - not recommended for production",
			"allowed_origins", corsOpts.AllowedOrigins)
	}
	r.Use(kubecors.Handler(corsOpts))

	// Global middleware
	r.Use(chimiddleware.RequestID) // Chi's built-in request ID
	r.Use(chimiddleware.RealIP)    // Extract real IP from X-Forwarded-For

	// Health endpoints
	r.Get("/health", s.healthHandler)
	r.Get("/healthz", s.healthHandler) // Kubernetes-style alias
	r.Get("/ready", s.readinessHandler)

	// Prometheus metrics
	// Expose metrics from custom registry (for test isolation)
	// If metricsInstance is nil, this will use the default registry
	var metricsHandler http.Handler
	if s.metricsInstance != nil && s.metricsInstance.Registry() != nil {
		metricsHandler = promhttp.HandlerFor(s.metricsInstance.Registry(), promhttp.HandlerOpts{})
	} else {
		metricsHandler = promhttp.Handler() // Default registry
	}
	r.Handle("/metrics", metricsHandler)

	// Note: Adapter routes will be registered dynamically when adapters are registered
	// via RegisterAdapter(). Each adapter exposes its own route (e.g. /api/v1/signals/prometheus)

	return r
}

// wrapWithMiddleware wraps the HTTP handler with middleware stack
//
// BUSINESS OUTCOME: Enable structured logging with request context (BR-109)
// TDD GREEN: Minimal middleware stack for request tracing
//
// Middleware Stack:
// 1. Request ID: Now handled by chi middleware in setupRoutes()
// 2. Performance Logging: Adds duration_ms to logs
//
// Note: Chi router handles RequestID and RealIP middleware in setupRoutes()
// This method only wraps with performance logging
func (s *Server) wrapWithMiddleware(handler http.Handler) http.Handler {
	// Wrap with performance logging middleware (BR-109)
	// Chi's RequestID middleware is already applied in setupRoutes()
	handler = s.performanceLoggingMiddleware(handler)

	return handler
}

// performanceLoggingMiddleware logs request duration and records metrics
//
// BUSINESS OUTCOME: Enable operators to analyze performance via logs and metrics (BR-109, BR-104)
// TDD GREEN: Minimal implementation to log duration_ms
// REFACTOR: Added HTTP request duration metric observation
func (s *Server) performanceLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// Call next handler
		next.ServeHTTP(ww, r)

		// Calculate duration
		duration := time.Since(start)

		// Record HTTP request duration metric (BR-104)
		s.metricsInstance.HTTPRequestDuration.WithLabelValues(
			r.URL.Path,                     // endpoint
			r.Method,                       // method
			fmt.Sprintf("%d", ww.Status()), // status
		).Observe(duration.Seconds())

		// Log request completion with duration (V(1) for health/readiness checks to reduce noise)
		logger := middleware.GetLogger(r.Context())
		if r.URL.Path == "/health" || r.URL.Path == "/healthz" || r.URL.Path == "/ready" {
			logger.V(1).Info("Request completed",
				"duration_ms", float64(duration.Milliseconds()),
			)
		} else {
			logger.Info("Request completed",
				"duration_ms", float64(duration.Milliseconds()),
			)
		}
	})
}

// Handler returns the HTTP handler for the Gateway server.
// This is useful for testing with httptest.NewServer.
//
// Example:
//
//	server := gateway.NewServer(cfg, logger)
//	testServer := httptest.NewServer(server.Handler())
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// RegisterAdapter registers a RoutableAdapter using chi router
//
// This method:
// 1. Validates adapter (checks for duplicate names/routes)
// 2. Registers adapter in registry
// 3. Creates HTTP handler that calls adapter.Parse()
// 4. Applies middleware and registers route with chi router
//
// Middleware applied:
// - Content-Type validation (BR-042)
// - Request ID (chi middleware - global)
// - Real IP extraction (chi middleware - global)
//
// Example:
//
//	prometheusAdapter := adapters.NewPrometheusAdapter(logger)
//	server.RegisterAdapter(prometheusAdapter)
//	// Now POST /api/v1/signals/prometheus is active
func (s *Server) RegisterAdapter(adapter adapters.RoutableAdapter) error {
	// Register in registry
	if err := s.adapterRegistry.Register(adapter); err != nil {
		return fmt.Errorf("failed to register adapter: %w", err)
	}

	// Create adapter HTTP handler
	handler := s.createAdapterHandler(adapter)

	// BR-042: Apply Content-Type validation middleware
	// Rejects non-JSON payloads early, before processing
	wrappedHandler := middleware.ValidateContentType(handler)

	// Register route using chi with full path
	// Chi automatically enforces POST method (returns 405 for other methods)
	// Note: chi.Router.Post() accepts http.HandlerFunc, so we use HandlerFunc wrapper
	s.router.Post(adapter.GetRoute(), wrappedHandler.ServeHTTP)

	s.logger.Info("Registered adapter route",
		"adapter", adapter.Name(),
		"route", adapter.GetRoute())

	return nil
}

// createAdapterHandler creates an HTTP handler for an adapter
//
// This handler:
// 1. Reads request body
// 2. Calls adapter.Parse() to convert to NormalizedSignal
// 3. Validates signal using adapter.Validate()
// 4. Calls ProcessSignal() to run full pipeline
// 5. Returns HTTP response (201/202/400/500)
//
// REFACTORED: Reduced cyclomatic complexity by extracting helper methods
func (s *Server) createAdapterHandler(adapter adapters.SignalAdapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only accept POST requests
		if r.Method != http.MethodPost {
			s.writeJSONError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		start := time.Now()
		ctx := r.Context()
		logger := middleware.GetLogger(ctx)

		// Read, parse, and validate signal
		signal, err := s.readParseValidateSignal(ctx, w, r, adapter, logger)
		if err != nil {
			return // Error response already sent
		}

		// Process signal through pipeline
		response, err := s.ProcessSignal(ctx, signal)
		if err != nil {
			s.handleProcessingError(w, r, err, adapter.Name(), logger)
			return
		}

		// Send success response
		s.sendSuccessResponse(w, r, response, adapter, start)
	}
}

// readParseValidateSignal reads, parses, and validates the signal from the request
// Returns nil signal and writes error response if any step fails
func (s *Server) readParseValidateSignal(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	adapter adapters.SignalAdapter,
	logger logr.Logger,
) (*types.NormalizedSignal, error) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error(err, "Failed to read request body")
		s.writeValidationError(w, r, "Failed to read request body")
		return nil, err
	}

	// Parse signal using adapter
	signal, err := adapter.Parse(ctx, body)
	if err != nil {
		logger.Info("Failed to parse signal",
			"adapter", adapter.Name(),
			"error", err)
		s.writeValidationError(w, r, fmt.Sprintf("Failed to parse signal: %v", err))
		return nil, err
	}

	// Validate signal
	if err := adapter.Validate(signal); err != nil {
		logger.Info("Signal validation failed",
			"adapter", adapter.Name(),
			"error", err)
		s.writeValidationError(w, r, fmt.Sprintf("Signal validation failed: %v", err))
		return nil, err
	}

	return signal, nil
}

// handleProcessingError handles errors from signal processing and sends appropriate HTTP response
func (s *Server) handleProcessingError(
	w http.ResponseWriter,
	r *http.Request,
	err error,
	adapterName string,
	logger logr.Logger,
) {
	logger.Error(err, "Signal processing failed",
		"adapter", adapterName)

	errMsg := err.Error()

	// Redis unavailability → HTTP 503 Service Unavailable
	if strings.Contains(errMsg, "redis unavailable") || strings.Contains(errMsg, "deduplication check failed") {
		s.writeServiceUnavailableError(w, r, "Deduplication service unavailable - Redis connection failed. Please retry after 30 seconds.", 30)
		return
	}

	// Kubernetes API errors → HTTP 500 Internal Server Error with details
	if strings.Contains(errMsg, "kubernetes") || strings.Contains(errMsg, "k8s") ||
		strings.Contains(errMsg, "failed to create RemediationRequest CRD") ||
		strings.Contains(errMsg, "namespaces") {
		s.writeInternalError(w, r, fmt.Sprintf("Kubernetes API error: %v", err))
		return
	}

	// Generic error → HTTP 500
	s.writeInternalError(w, r, "Internal server error")
}

// sendSuccessResponse sends the success HTTP response with metrics recording
func (s *Server) sendSuccessResponse(
	w http.ResponseWriter,
	r *http.Request,
	response *ProcessingResponse,
	adapter adapters.SignalAdapter,
	start time.Time,
) {
	// Determine HTTP status code based on response status
	statusCode := http.StatusCreated
	if response.Status == StatusAccepted || response.Duplicate {
		statusCode = http.StatusAccepted
	}

	// Record metrics
	duration := time.Since(start)
	route := "/unknown"
	if routableAdapter, ok := adapter.(adapters.RoutableAdapter); ok {
		route = routableAdapter.GetRoute()
	}
	s.metricsInstance.HTTPRequestDuration.WithLabelValues(
		route,
		r.Method,
		fmt.Sprintf("%d", statusCode),
	).Observe(duration.Seconds())

	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error(err, "Failed to encode JSON response")
	}
}

// Start starts the HTTP server and background health checks
//
// This method:
// 1. Starts Redis health check goroutine (BR-106)
// 2. Logs startup message
// 3. Starts HTTP server (blocking)
// 4. Returns error if server fails to start
//
// Start should be called in a goroutine:
//
//	go func() {
//	    if err := server.Start(ctx); err != nil && err != http.ErrServerClosed {
//	        log.Fatalf("Server failed: %v", err)
//	    }
//	}()
func (s *Server) Start(ctx context.Context) error {
	// DD-GATEWAY-012: Redis health monitor REMOVED - Gateway is Redis-free
	s.logger.Info("Starting Gateway server", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Stop gracefully stops the HTTP server
//
// This method implements industry best practice for graceful shutdown:
// 1. Set shutdown flag (readiness probe returns 503)
// 2. Wait 5 seconds for Kubernetes endpoint removal propagation
// 3. Shutdown HTTP server (waits for in-flight requests)
// 4. Return error if shutdown fails
//
// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free
//
// Shutdown timeout is controlled by the provided context:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	server.Stop(ctx)
//
// GRACEFUL SHUTDOWN SEQUENCE (Industry Best Practice):
//
//	T+0s:  Set isShuttingDown = true (readiness probe returns 503)
//	T+0s:  Kubernetes readiness probe fails
//	T+1s:  Kubernetes removes pod from Service endpoints
//	T+5s:  Endpoint removal propagated across cluster
//	T+5s:  httpServer.Shutdown() closes listener
//	T+5-35s: In-flight requests complete (up to 30s timeout)
//	T+35s: Pod exits cleanly
//
// WHY 5-SECOND DELAY:
// - Kubernetes takes 1-3 seconds to propagate endpoint removal
// - 5 seconds provides safety margin for large clusters
// - Prevents new traffic from arriving during shutdown
// - Industry standard: Google SRE, Netflix, Kubernetes community
//
// See: READINESS_PROBE_SHUTDOWN_ANALYSIS.md, GRACEFUL_SHUTDOWN_SEQUENCE_DIAGRAMS.md
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Stopping Gateway server")

	// STEP 1: Set shutdown flag (readiness probe will now return 503)
	// This triggers Kubernetes to remove pod from Service endpoints
	s.isShuttingDown.Store(true)
	s.logger.Info("Shutdown flag set, readiness probe will return 503")

	// STEP 2: Wait for Kubernetes to propagate endpoint removal
	// This ensures no new traffic is routed to this pod
	// Kubernetes typically takes 1-3 seconds to update endpoints
	// We wait 5 seconds to be safe (industry best practice)
	s.logger.Info("Waiting 5 seconds for Kubernetes endpoint removal propagation")
	time.Sleep(5 * time.Second)
	s.logger.Info("Endpoint removal propagation complete, proceeding with HTTP server shutdown")

	// STEP 3: Graceful HTTP server shutdown
	// Now that pod is removed from endpoints, we can safely shutdown
	// This will complete any in-flight requests that arrived before endpoint removal
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error(err, "Failed to gracefully shutdown HTTP server")
		return err
	}

	// DD-GATEWAY-012: Redis close REMOVED - Gateway is now Redis-free
	// DD-AUDIT-003: Close audit store to flush remaining events
	if s.auditStore != nil {
		if err := s.auditStore.Close(); err != nil {
			s.logger.Error(err, "DD-AUDIT-003: Failed to close audit store (potential audit data loss)")
			// Non-fatal: don't fail shutdown for audit store errors
		}
	}

	s.logger.Info("Gateway server stopped")
	return nil
}

// ProcessSignal implements adapters.SignalProcessor interface
//
// This is the main signal processing pipeline, called by adapter handlers.
//
// Pipeline stages:
// 1. Deduplication check (Redis lookup)
// 2. If duplicate: Update Redis metadata, return HTTP 202
// 3. Storm detection (rate-based + pattern-based)
// 4. CRD creation (Kubernetes API)
// 5. Store deduplication metadata (Redis)
// 6. Return HTTP 201 with CRD details
//
// Note: Environment classification and Priority assignment removed (2025-12-06)
// These are now owned by Signal Processing service per DD-CATEGORIZATION-001
//
// Performance:
// - Typical latency (new signal): p95 ~50ms, p99 ~80ms
//   - Deduplication check: ~3ms
//   - Storm detection: ~3ms
//   - CRD creation: ~30ms (Kubernetes API)
//   - Redis store: ~3ms
//
// - Typical latency (duplicate signal): p95 ~10ms, p99 ~20ms
//   - Deduplication check: ~3ms
//   - Redis update: ~3ms
//   - No CRD creation (fast path)
//
// ProcessSignal implements adapters.SignalProcessor interface
// TDD REFACTOR: Simplified by extracting helper methods
//
// This is the main signal processing pipeline orchestrator.
//
// Pipeline stages:
// 1. Deduplication check → processDuplicateSignal() if duplicate
// 2. Storm detection → processStormAggregation() if storm detected
// 3. CRD creation → createRemediationRequestCRD() for new signals
func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
	start := time.Now()
	logger := middleware.GetLogger(ctx)

	// Record ingestion metric (environment label removed - SP owns classification)
	s.metricsInstance.AlertsReceivedTotal.WithLabelValues(signal.SourceType, signal.Severity).Inc()

	// 1. Deduplication check (DD-GATEWAY-011: K8s status-based, NOT Redis)
	// BR-GATEWAY-185: Redis deprecation - use PhaseBasedDeduplicationChecker
	shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, signal.Namespace, signal.Fingerprint)
	if err != nil {
		logger.Error(err, "Deduplication check failed",
			"fingerprint", signal.Fingerprint)
		return nil, fmt.Errorf("deduplication check failed: %w", err)
	}

	if shouldDeduplicate && existingRR != nil {
		// ========================================
		// DD-GATEWAY-013: Hybrid Async Status Update Pattern
		// 📋 Design Decision: DD-GATEWAY-013 | ✅ Approved | Confidence: 85%
		// See: docs/architecture/decisions/DD-GATEWAY-013-async-status-updates.md
		// ========================================
		// - SYNC: Deduplication status (needed for accurate HTTP response)
		// - ASYNC: Storm aggregation status (non-critical, fire-and-forget)
		// ========================================

		// SYNC: Update status.deduplication (DD-GATEWAY-011)
		// Must be synchronous - HTTP response includes occurrence count
		if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
			logger.Info("Failed to update deduplication status (DD-GATEWAY-011)",
				"error", err,
				"fingerprint", signal.Fingerprint,
				"rr", existingRR.Name)
		}

		// Calculate storm threshold (needed for both async update and metrics)
		occurrenceCount := int32(1)
		if existingRR.Status.Deduplication != nil {
			occurrenceCount = existingRR.Status.Deduplication.OccurrenceCount
		}
		isThresholdReached := occurrenceCount >= s.stormThreshold

		// ASYNC: Update status.stormAggregation (DD-GATEWAY-013)
		// Fire-and-forget - storm status is informational, not critical for response
		// Captures variables for goroutine closure
		rrCopy := existingRR.DeepCopy() // Safe copy for async use
		go func() {
			asyncCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.statusUpdater.UpdateStormAggregationStatus(asyncCtx, rrCopy, isThresholdReached); err != nil {
				s.logger.Info("Failed to update storm aggregation status (async, DD-GATEWAY-013)",
					"error", err,
					"fingerprint", signal.Fingerprint,
					"rr", rrCopy.Name,
					"occurrenceCount", occurrenceCount,
					"threshold", s.stormThreshold)
			}
		}()

		// Record metrics
		s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.AlertName).Inc()
		if isThresholdReached {
			s.metricsInstance.AlertStormsDetectedTotal.WithLabelValues("rate", signal.AlertName).Inc()
		}

		logger.V(1).Info("Duplicate signal detected (K8s status-based)",
			"fingerprint", signal.Fingerprint,
			"existingRR", existingRR.Name,
			"phase", existingRR.Status.OverallPhase,
			"occurrenceCount", occurrenceCount,
			"isStorm", isThresholdReached)

		// DD-AUDIT-003: Emit audit events (BR-GATEWAY-191, BR-GATEWAY-192)
		// Fire-and-forget: audit failures don't affect business logic
		s.emitSignalDeduplicatedAudit(ctx, signal, existingRR.Name, existingRR.Namespace, occurrenceCount)
		if isThresholdReached {
			s.emitStormDetectedAudit(ctx, signal, existingRR.Name, existingRR.Namespace, occurrenceCount)
		}

		// Return duplicate response with data from existing RR
		return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
	}

	// 2. CRD creation pipeline (DD-GATEWAY-012: No storm buffering - create RR immediately)
	return s.createRemediationRequestCRD(ctx, signal, start)
}

// healthHandler handles liveness probes
//
// Endpoint: GET /health
// Response: Always 200 OK (indicates server is running)
//
// Liveness probe checks if the server process is alive.
// If this endpoint fails, Kubernetes will restart the pod.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	// Per HEALTH_CHECK_STANDARD.md: Liveness returns "healthy" not "ok"
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		s.logger.Error(err, "Failed to encode health response")
	}
}

// readinessHandler handles readiness probes
//
// Endpoint: GET /ready
// Response: 200 OK if ready, 503 Service Unavailable if not ready
//
// Readiness probe checks if the server is ready to accept traffic.
// If this endpoint fails, Kubernetes will remove the pod from load balancer.
//
// Ready conditions:
// 1. Server is not shutting down (isShuttingDown == false)
// 2. Redis is reachable (PING command succeeds)
// 3. Kubernetes API is reachable (list namespaces succeeds)
//
// Typical checks: ~10ms (Redis PING + K8s API call)
//
// GRACEFUL SHUTDOWN INTEGRATION:
// When server receives SIGTERM, isShuttingDown is set to true immediately.
// This causes readiness probe to return 503, triggering Kubernetes to remove
// the pod from Service endpoints BEFORE httpServer.Shutdown() closes the listener.
// This eliminates the race condition and guarantees zero new traffic during shutdown.
//
// Industry best practice: Google SRE, Netflix, Kubernetes community
// See: READINESS_PROBE_SHUTDOWN_ANALYSIS.md
func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
	// GRACEFUL SHUTDOWN: Return 503 immediately when shutting down
	// This ensures Kubernetes removes pod from Service endpoints BEFORE
	// we stop accepting new connections via httpServer.Shutdown()
	//
	// WHY: Prevents race condition between readiness probe and listener closure
	// BENEFIT: Guaranteed endpoint removal before in-flight request completion
	//
	// RFC 7807: Use standard Problem Details format for structured error response
	if s.isShuttingDown.Load() {
		s.logger.Info("Readiness check failed: server is shutting down")

		// Use RFC 7807 Problem Details format for structured error response
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusServiceUnavailable)

		errorResponse := gwerrors.RFC7807Error{
			Type:     gwerrors.ErrorTypeServiceUnavailable,
			Title:    gwerrors.TitleServiceUnavailable,
			Detail:   "Server is shutting down gracefully",
			Status:   http.StatusServiceUnavailable,
			Instance: r.URL.Path,
		}

		if encErr := json.NewEncoder(w).Encode(errorResponse); encErr != nil {
			s.logger.Error(encErr, "Failed to encode readiness error response")
		}
		return
	}

	// DD-GATEWAY-012: Redis check REMOVED - Gateway is now Redis-free
	// Only check Kubernetes API connectivity
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Check Kubernetes API connectivity by listing namespaces
	namespaceList := &corev1.NamespaceList{}
	if err := s.ctrlClient.List(ctx, namespaceList, client.Limit(1)); err != nil {
		s.logger.Info("Readiness check failed: Kubernetes API not reachable", "error", err)

		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusServiceUnavailable)

		errorResponse := gwerrors.RFC7807Error{
			Type:     gwerrors.ErrorTypeServiceUnavailable,
			Title:    gwerrors.TitleServiceUnavailable,
			Detail:   "Kubernetes API is not reachable",
			Status:   http.StatusServiceUnavailable,
			Instance: r.URL.Path,
		}

		if encErr := json.NewEncoder(w).Encode(errorResponse); encErr != nil {
			s.logger.Error(encErr, "Failed to encode readiness error response")
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ready"}); err != nil {
		s.logger.Error(err, "Failed to encode readiness response")
	}
}

// ProcessingResponse represents the result of signal processing
//
// Note: Environment and Priority fields removed from response (2025-12-06)
// These classifications are now owned by Signal Processing service per DD-CATEGORIZATION-001.
// AlertManager/webhook callers don't need this information - they only need to know
// if the alert was accepted (HTTP status code).
// See: docs/handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md
type ProcessingResponse struct {
	Status                      string `json:"status"` // "created", "duplicate", or "accepted"
	Message                     string `json:"message"`
	Fingerprint                 string `json:"fingerprint"`
	Duplicate                   bool   `json:"duplicate"`
	RemediationRequestName      string `json:"remediationRequestName,omitempty"`
	RemediationRequestNamespace string `json:"remediationRequestNamespace,omitempty"`
	// Note: RemediationPath removed (2025-12-06) - SP derives risk_tolerance via Rego per DD-WORKFLOW-001
	Metadata *processing.DeduplicationMetadata `json:"metadata,omitempty"` // Deduplication info only
	// Storm aggregation fields (BR-GATEWAY-016)
	IsStorm   bool   `json:"isStorm,omitempty"`   // true if alert is part of a storm
	StormType string `json:"stormType,omitempty"` // "rate" or "pattern"
	WindowID  string `json:"windowID,omitempty"`  // aggregation window identifier
}

// Processing status constants
const (
	StatusCreated   = "created"   // RemediationRequest CRD created
	StatusDuplicate = "duplicate" // Duplicate alert (deduplicated)
	StatusAccepted  = "accepted"  // Alert accepted for storm aggregation (CRD will be created later)
)

// NewDuplicateResponse creates a ProcessingResponse for duplicate signals
// TDD REFACTOR: Extracted factory function for duplicate response pattern
// Business Outcome: Consistent duplicate signal handling (BR-005)
// DEPRECATED: Use NewDuplicateResponseFromRR for DD-GATEWAY-011 status-based deduplication
// NewDuplicateResponse is DEPRECATED - use NewDuplicateResponseFromRR instead
// DD-GATEWAY-011: Redis metadata replaced with K8s status-based tracking
// Kept for backward compatibility with any external code that might use it
func NewDuplicateResponse(fingerprint string, metadata *processing.DeduplicationMetadata) *ProcessingResponse {
	return &ProcessingResponse{
		Status:      StatusDuplicate,
		Message:     "Duplicate signal (deduplication successful)",
		Fingerprint: fingerprint,
		Duplicate:   true,
		Metadata:    metadata,
	}
}

// NewDuplicateResponseFromRR creates a ProcessingResponse for duplicate signals using K8s RR data
// DD-GATEWAY-011: Status-based deduplication (Redis deprecation)
// BR-GATEWAY-185: All dedup state from K8s status, not Redis
func NewDuplicateResponseFromRR(fingerprint string, rr *remediationv1alpha1.RemediationRequest) *ProcessingResponse {
	// Build metadata from RR status (DD-GATEWAY-011: status-based tracking)
	var occurrenceCount int
	var firstOccurrence, lastOccurrence string

	if rr.Status.Deduplication != nil {
		occurrenceCount = int(rr.Status.Deduplication.OccurrenceCount)
		if rr.Status.Deduplication.FirstSeenAt != nil {
			firstOccurrence = rr.Status.Deduplication.FirstSeenAt.Time.Format(time.RFC3339)
		}
		if rr.Status.Deduplication.LastSeenAt != nil {
			lastOccurrence = rr.Status.Deduplication.LastSeenAt.Time.Format(time.RFC3339)
		}
	}

	return &ProcessingResponse{
		Status:                      StatusDuplicate,
		Message:                     "Duplicate signal (K8s status-based deduplication)",
		Fingerprint:                 fingerprint,
		Duplicate:                   true,
		RemediationRequestName:      rr.Name,
		RemediationRequestNamespace: rr.Namespace,
		Metadata: &processing.DeduplicationMetadata{
			Count:                 occurrenceCount,
			FirstOccurrence:       firstOccurrence,
			LastOccurrence:        lastOccurrence,
			RemediationRequestRef: fmt.Sprintf("%s/%s", rr.Namespace, rr.Name),
		},
	}
}

// NewStormAggregationResponse creates a ProcessingResponse for storm aggregation
// TDD REFACTOR: Extracted factory function for storm aggregation response pattern
// Business Outcome: Consistent storm aggregation handling (BR-013)
func NewStormAggregationResponse(fingerprint, windowID, stormType string, resourceCount int, isNewWindow bool) *ProcessingResponse {
	var message string
	if isNewWindow {
		message = fmt.Sprintf("Storm aggregation window started (window ID: %s, CRD will be created after 1 minute)", windowID)
	} else {
		message = fmt.Sprintf("Alert added to storm aggregation window (window ID: %s, %d resources aggregated)", windowID, resourceCount)
	}

	return &ProcessingResponse{
		Status:      StatusAccepted,
		Message:     message,
		Fingerprint: fingerprint,
		Duplicate:   false,
		IsStorm:     true,
		StormType:   stormType,
		WindowID:    windowID,
	}
}

// NewCRDCreatedResponse creates a ProcessingResponse for successful CRD creation
// TDD REFACTOR: Extracted factory function for CRD creation response pattern
// Business Outcome: Consistent CRD creation handling (BR-004)
//
// Note: Environment, Priority, and RemediationPath parameters removed (2025-12-06)
// Classification and path decision now owned by Signal Processing service
// per DD-CATEGORIZATION-001 and DD-WORKFLOW-001 (risk_tolerance in CustomLabels)
func NewCRDCreatedResponse(fingerprint, crdName, crdNamespace string) *ProcessingResponse {
	return &ProcessingResponse{
		Status:                      StatusCreated,
		Message:                     "RemediationRequest CRD created successfully",
		Fingerprint:                 fingerprint,
		Duplicate:                   false,
		RemediationRequestName:      crdName,
		RemediationRequestNamespace: crdNamespace,
	}
}

// processDuplicateSignal handles the duplicate signal fast path
// processDuplicateSignal is DEPRECATED and removed
// DD-GATEWAY-011: Replaced by inline logic in ProcessSignal using PhaseBasedDeduplicationChecker
// BR-GATEWAY-185: Redis deprecation complete for deduplication path

// parseCRDReference is DEPRECATED - no longer needed after DD-GATEWAY-011
// DD-GATEWAY-011: PhaseChecker returns the actual RR, no need to parse references
// Kept for backward compatibility with any external code
//
// DD-GATEWAY-009: Helper for parsing RemediationRequestRef
// Format: "namespace/name" (e.g., "production/rr-abc123")
//
// Returns:
// - namespace: The namespace part (empty if invalid format)
//
// - name: The name part (empty if invalid format)
//
//nolint:unused // Deprecated but kept for compatibility
func (s *Server) parseCRDReference(ref string) (namespace, name string) {
	if ref == "" {
		return "", ""
	}

	parts := strings.Split(ref, "/")
	if len(parts) != 2 {
		return "", ""
	}

	return parts[0], parts[1]
}

// DD-GATEWAY-012: processStormAggregation REMOVED - Redis-based storm buffering deprecated
// Storm detection now uses status.stormAggregation via StatusUpdater (async pattern)
// Per DD-GATEWAY-008 supersession: Create RR immediately, track storm in status

// =============================================================================
// DD-AUDIT-003: Audit Event Emission (P0 Compliance)
// =============================================================================

// emitSignalReceivedAudit emits 'gateway.signal.received' audit event (BR-GATEWAY-190)
// This is called when a NEW signal is received and RR is created
func (s *Server) emitSignalReceivedAudit(ctx context.Context, signal *types.NormalizedSignal, rrName, rrNamespace string) {
	if s.auditStore == nil {
		return // Graceful degradation: no audit store configured
	}

	ns := signal.Namespace
	event := audit.NewAuditEvent()
	event.EventType = "gateway.signal.received"
	event.EventCategory = "gateway"
	event.EventAction = "received"
	event.EventOutcome = "success"
	event.ActorType = "external"
	event.ActorID = signal.SourceType // e.g., "prometheus", "kubernetes"
	event.ResourceType = "Signal"
	event.ResourceID = signal.Fingerprint
	event.CorrelationID = rrName // Use RR name as correlation
	event.Namespace = &ns

	// Event data with Gateway-specific fields
	eventData := map[string]interface{}{
		"gateway": map[string]interface{}{
			"signal_type":          signal.SourceType,
			"alert_name":           signal.AlertName,
			"namespace":            signal.Namespace,
			"fingerprint":          signal.Fingerprint,
			"severity":             signal.Severity,
			"resource_kind":        signal.Resource.Kind,
			"resource_name":        signal.Resource.Name,
			"remediation_request":  fmt.Sprintf("%s/%s", rrNamespace, rrName),
			"deduplication_status": "new",
		},
	}
	eventDataBytes, _ := json.Marshal(eventData)
	event.EventData = eventDataBytes

	// Fire-and-forget: StoreAudit is non-blocking per DD-AUDIT-002
	if err := s.auditStore.StoreAudit(ctx, event); err != nil {
		s.logger.Info("DD-AUDIT-003: Failed to emit signal.received audit event",
			"error", err, "fingerprint", signal.Fingerprint)
	}
}

// emitSignalDeduplicatedAudit emits 'gateway.signal.deduplicated' audit event (BR-GATEWAY-191)
// This is called when a DUPLICATE signal is detected
func (s *Server) emitSignalDeduplicatedAudit(ctx context.Context, signal *types.NormalizedSignal, rrName, rrNamespace string, occurrenceCount int32) {
	if s.auditStore == nil {
		return // Graceful degradation: no audit store configured
	}

	ns := signal.Namespace
	event := audit.NewAuditEvent()
	event.EventType = "gateway.signal.deduplicated"
	event.EventCategory = "gateway"
	event.EventAction = "deduplicated"
	event.EventOutcome = "success"
	event.ActorType = "external"
	event.ActorID = signal.SourceType
	event.ResourceType = "Signal"
	event.ResourceID = signal.Fingerprint
	event.CorrelationID = rrName
	event.Namespace = &ns

	eventData := map[string]interface{}{
		"gateway": map[string]interface{}{
			"signal_type":          signal.SourceType,
			"alert_name":           signal.AlertName,
			"namespace":            signal.Namespace,
			"fingerprint":          signal.Fingerprint,
			"remediation_request":  fmt.Sprintf("%s/%s", rrNamespace, rrName),
			"deduplication_status": "duplicate",
			"occurrence_count":     occurrenceCount,
		},
	}
	eventDataBytes, _ := json.Marshal(eventData)
	event.EventData = eventDataBytes

	if err := s.auditStore.StoreAudit(ctx, event); err != nil {
		s.logger.Info("DD-AUDIT-003: Failed to emit signal.deduplicated audit event",
			"error", err, "fingerprint", signal.Fingerprint)
	}
}

// emitStormDetectedAudit emits 'gateway.storm.detected' audit event (BR-GATEWAY-192)
// This is called when storm threshold is reached (occurrence count >= threshold)
func (s *Server) emitStormDetectedAudit(ctx context.Context, signal *types.NormalizedSignal, rrName, rrNamespace string, occurrenceCount int32) {
	if s.auditStore == nil {
		return // Graceful degradation: no audit store configured
	}

	ns := signal.Namespace
	stormID := fmt.Sprintf("storm-%s-%s", signal.Fingerprint[:8], rrName[:8])
	event := audit.NewAuditEvent()
	event.EventType = "gateway.storm.detected"
	event.EventCategory = "gateway"
	event.EventAction = "detected"
	event.EventOutcome = "success"
	event.ActorType = "service"
	event.ActorID = "gateway"
	event.ResourceType = "Storm"
	event.ResourceID = stormID
	event.CorrelationID = rrName
	event.Namespace = &ns

	eventData := map[string]interface{}{
		"gateway": map[string]interface{}{
			"storm_detected":      true,
			"storm_id":            stormID,
			"alert_name":          signal.AlertName,
			"namespace":           signal.Namespace,
			"fingerprint":         signal.Fingerprint,
			"remediation_request": fmt.Sprintf("%s/%s", rrNamespace, rrName),
			"occurrence_count":    occurrenceCount,
			"storm_threshold":     s.stormThreshold,
		},
	}
	eventDataBytes, _ := json.Marshal(eventData)
	event.EventData = eventDataBytes

	if err := s.auditStore.StoreAudit(ctx, event); err != nil {
		s.logger.Info("DD-AUDIT-003: Failed to emit storm.detected audit event",
			"error", err, "fingerprint", signal.Fingerprint, "storm_id", stormID)
	}
}

// createRemediationRequestCRD handles the CRD creation pipeline
// TDD REFACTOR: Extracted from ProcessSignal for clarity
// Business Outcome: Consistent CRD creation (BR-004)
//
// Note: Environment, Priority, and RemediationPath removed from Gateway (2025-12-06)
// Signal Processing service now owns classification and path decision
// per DD-CATEGORIZATION-001 and DD-WORKFLOW-001 (risk_tolerance in CustomLabels)
// See: docs/handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md
func (s *Server) createRemediationRequestCRD(ctx context.Context, signal *types.NormalizedSignal, start time.Time) (*ProcessingResponse, error) {
	logger := middleware.GetLogger(ctx)

	// Create RemediationRequest CRD (classification and path moved to SP)
	rr, err := s.crdCreator.CreateRemediationRequest(ctx, signal)
	if err != nil {
		logger.Error(err, "Failed to create RemediationRequest CRD",
			"fingerprint", signal.Fingerprint)
		return nil, fmt.Errorf("failed to create RemediationRequest CRD: %w", err)
	}

	// DD-GATEWAY-011: Redis deduplication storage DEPRECATED
	// Deduplication now uses K8s status-based lookup (phaseChecker.ShouldDeduplicate)
	// and status updates (statusUpdater.UpdateDeduplicationStatus)
	// Redis is no longer used for deduplication state

	// DD-AUDIT-003: Emit signal.received audit event (BR-GATEWAY-190)
	// Fire-and-forget: audit failures don't affect business logic
	s.emitSignalReceivedAudit(ctx, signal, rr.Name, rr.Namespace)

	// Record processing duration
	duration := time.Since(start)
	logger.Info("Signal processed successfully",
		"fingerprint", signal.Fingerprint,
		"crdName", rr.Name,
		"duration_ms", duration.Milliseconds())

	return NewCRDCreatedResponse(signal.Fingerprint, rr.Name, rr.Namespace), nil
}

// DD-GATEWAY-012: createAggregatedCRD REMOVED - Redis-based storm aggregation deprecated
// Storm tracking now uses status.stormAggregation via StatusUpdater (async pattern)
// RRs are created immediately on first alert, storm status updated on subsequent alerts

// DD-GATEWAY-012: monitorWindowExpiration REMOVED - No Redis windows to monitor
// Storm detection threshold now based on status.deduplication.occurrenceCount

// TDD REFACTOR: RFC7807Error moved to pkg/gateway/errors package to eliminate duplication

// writeJSONError writes an RFC 7807 compliant error response
// TDD GREEN: Updated to support BR-041 (RFC 7807 error format)
// TDD REFACTOR: Now uses shared gwerrors.RFC7807Error struct and constants
// BR-109: Added request ID extraction for request tracing
// BR-GATEWAY-078: Added error message sanitization to prevent sensitive data exposure
// Business Outcome: Clients receive standards-compliant, machine-readable error responses
func (s *Server) writeJSONError(w http.ResponseWriter, r *http.Request, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)

	// BR-109: Extract request ID from context for tracing
	requestID := middleware.GetRequestID(r.Context())

	// BR-GATEWAY-078: Sanitize error message to prevent sensitive data exposure
	// DD-005: Use shared sanitization library directly
	sanitizedMessage := sanitization.SanitizeForLog(message)

	// Determine error type and title based on status code
	errorType, title := getErrorTypeAndTitle(statusCode)

	errorResponse := gwerrors.RFC7807Error{
		Type:      errorType,
		Title:     title,
		Detail:    sanitizedMessage,
		Status:    statusCode,
		Instance:  r.URL.Path,
		RequestID: requestID,
	}

	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		// Fallback to plain text if JSON encoding fails
		http.Error(w, message, statusCode)
	}
}

// getErrorTypeAndTitle returns the RFC 7807 error type URI and title for a given HTTP status code
// BR-041: RFC 7807 error format
// TDD REFACTOR: Now uses shared gwerrors constants
func getErrorTypeAndTitle(statusCode int) (string, string) {
	switch statusCode {
	case http.StatusBadRequest:
		return gwerrors.ErrorTypeValidationError, gwerrors.TitleBadRequest
	case http.StatusMethodNotAllowed:
		return gwerrors.ErrorTypeMethodNotAllowed, gwerrors.TitleMethodNotAllowed
	case http.StatusInternalServerError:
		return gwerrors.ErrorTypeInternalError, gwerrors.TitleInternalServerError
	case http.StatusServiceUnavailable:
		return gwerrors.ErrorTypeServiceUnavailable, gwerrors.TitleServiceUnavailable
	default:
		return gwerrors.ErrorTypeUnknown, gwerrors.TitleUnknown
	}
}

// writeValidationError writes a 400 Bad Request error response
// TDD REFACTOR: Extracted common validation error pattern
// BR-109: Added request parameter for request ID tracing
// Business Outcome: Consistent validation error handling (BR-001)
func (s *Server) writeValidationError(w http.ResponseWriter, r *http.Request, message string) {
	s.writeJSONError(w, r, message, http.StatusBadRequest)
}

// writeInternalError writes a 500 Internal Server Error response
// TDD REFACTOR: Extracted common internal error pattern
// BR-109: Added request parameter for request ID tracing
// Business Outcome: Consistent internal error handling (BR-001)
func (s *Server) writeInternalError(w http.ResponseWriter, r *http.Request, message string) {
	s.writeJSONError(w, r, message, http.StatusInternalServerError)
}

// writeServiceUnavailableError writes a 503 Service Unavailable error response
// TDD REFACTOR: Extracted common service unavailable pattern
// BR-109: Added request parameter for request ID tracing
// Business Outcome: Consistent service unavailability handling (BR-003)
func (s *Server) writeServiceUnavailableError(w http.ResponseWriter, r *http.Request, message string, retryAfter int) {
	w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
	s.writeJSONError(w, r, message, http.StatusServiceUnavailable)
}
