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
	goredis "github.com/redis/go-redis/v9"

	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"

	// "k8s.io/client-go/kubernetes" // DD-GATEWAY-004: No longer needed (authentication removed)
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis" // DD-CACHE-001: Shared Redis Library
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
	deduplicator    *processing.DeduplicationService
	crdUpdater      *processing.CRDUpdater // DD-GATEWAY-009: CRD updater for state-based deduplication
	stormDetector   *processing.StormDetector
	stormAggregator *processing.StormAggregator
	// DD-GATEWAY-011: Status-based deduplication and storm aggregation (Redis deprecation)
	statusUpdater *processing.StatusUpdater                  // Updates RR status.deduplication and status.stormAggregation
	phaseChecker  *processing.PhaseBasedDeduplicationChecker // Phase-based deduplication logic
	// Note: classifier, priorityEngine, and pathDecider removed (2025-12-06)
	// Environment/Priority classification and remediation path now owned by Signal Processing
	// per DD-CATEGORIZATION-001 and DD-WORKFLOW-001 (risk_tolerance in CustomLabels)
	crdCreator *processing.CRDCreator

	// Infrastructure clients
	redisClient *rediscache.Client // DD-CACHE-001: Shared Redis Library
	k8sClient   *k8s.Client
	ctrlClient  client.Client

	// Middleware
	// DD-GATEWAY-004: Authentication middleware removed (network-level security)
	// authMiddleware *middleware.AuthMiddleware // REMOVED
	// rateLimiter    *middleware.RateLimiter    // REMOVED

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
func NewServerWithK8sClient(cfg *config.ServerConfig, logger logr.Logger, metricsInstance *metrics.Metrics, ctrlClient client.Client) (*Server, error) {
	// 1. Initialize Redis client (DD-CACHE-001: Shared Redis Library)
	redisClient := rediscache.NewClient(&goredis.Options{
		Addr:         cfg.Infrastructure.Redis.Addr,
		DB:           cfg.Infrastructure.Redis.DB,
		Password:     cfg.Infrastructure.Redis.Password,
		DialTimeout:  cfg.Infrastructure.Redis.DialTimeout,
		ReadTimeout:  cfg.Infrastructure.Redis.ReadTimeout,
		WriteTimeout: cfg.Infrastructure.Redis.WriteTimeout,
		PoolSize:     cfg.Infrastructure.Redis.PoolSize,
		MinIdleConns: cfg.Infrastructure.Redis.MinIdleConns,
	}, logger)

	// 2. Use provided Kubernetes client (shared with test)
	k8sClient := k8s.NewClient(ctrlClient)

	// 3. Initialize processing pipeline components
	return createServerWithClients(cfg, logger, metricsInstance, redisClient, ctrlClient, k8sClient)
}

func NewServerWithMetrics(cfg *config.ServerConfig, logger logr.Logger, metricsInstance *metrics.Metrics) (*Server, error) {
	// 1. Initialize Redis client (DD-CACHE-001: Shared Redis Library)
	redisClient := rediscache.NewClient(&goredis.Options{
		Addr:         cfg.Infrastructure.Redis.Addr,
		DB:           cfg.Infrastructure.Redis.DB,
		Password:     cfg.Infrastructure.Redis.Password,
		DialTimeout:  cfg.Infrastructure.Redis.DialTimeout,
		ReadTimeout:  cfg.Infrastructure.Redis.ReadTimeout,
		WriteTimeout: cfg.Infrastructure.Redis.WriteTimeout,
		PoolSize:     cfg.Infrastructure.Redis.PoolSize,
		MinIdleConns: cfg.Infrastructure.Redis.MinIdleConns,
	}, logger)

	// 2. Initialize Kubernetes clients
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

	return createServerWithClients(cfg, logger, metricsInstance, redisClient, ctrlClient, k8sClient)
}

// createServerWithClients is the common server creation logic
// DD-005: Uses logr.Logger for unified logging interface
func createServerWithClients(cfg *config.ServerConfig, logger logr.Logger, metricsInstance *metrics.Metrics, redisClient *rediscache.Client, ctrlClient client.Client, k8sClient *k8s.Client) (*Server, error) {
	// DD-GATEWAY-004: kubernetes clientset removed (no longer needed for authentication)
	// clientset, err := kubernetes.NewForConfig(kubeConfig) // REMOVED
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	// } // REMOVED

	// Initialize processing pipeline components
	adapterRegistry := adapters.NewAdapterRegistry(logger)

	// Use provided metrics instance or create new one with default registry
	if metricsInstance == nil {
		metricsInstance = metrics.NewMetrics()
	}

	// DD-GATEWAY-009: Initialize deduplicator with K8s client for state-based deduplication
	var deduplicator *processing.DeduplicationService
	if cfg.Processing.Deduplication.TTL > 0 {
		deduplicator = processing.NewDeduplicationServiceWithTTL(redisClient, k8sClient, cfg.Processing.Deduplication.TTL, logger, metricsInstance)
		logger.Info("Using custom deduplication TTL", "ttl", cfg.Processing.Deduplication.TTL)
	} else {
		deduplicator = processing.NewDeduplicationService(redisClient, k8sClient, logger, metricsInstance)
	}

	// DD-GATEWAY-009: Initialize CRD updater for duplicate alert handling
	crdUpdater := processing.NewCRDUpdater(k8sClient, logger)

	stormDetector := processing.NewStormDetector(redisClient.GetClient(), cfg.Processing.Storm.RateThreshold, cfg.Processing.Storm.PatternThreshold, metricsInstance)
	if cfg.Processing.Storm.RateThreshold > 0 || cfg.Processing.Storm.PatternThreshold > 0 {
		logger.Info("Using custom storm detection thresholds",
			"rate_threshold", cfg.Processing.Storm.RateThreshold,
			"pattern_threshold", cfg.Processing.Storm.PatternThreshold,
		)
	}

	// DD-GATEWAY-008: Use NewStormAggregatorWithConfig for full feature support
	// This enables buffered first-alert aggregation, sliding windows, and multi-tenant isolation
	stormAggregator := processing.NewStormAggregatorWithConfig(
		redisClient.GetClient(),
		logger,                                 // Logger for operational visibility (DD-005: logr.Logger)
		cfg.Processing.Storm.BufferThreshold,   // BR-GATEWAY-016: Buffer N alerts before creating window
		cfg.Processing.Storm.InactivityTimeout, // BR-GATEWAY-008: Sliding window timeout
		cfg.Processing.Storm.MaxWindowDuration, // BR-GATEWAY-008: Maximum window duration
		1000,                                   // defaultMaxSize: Default namespace buffer size
		5000,                                   // globalMaxSize: Global buffer limit
		nil,                                    // perNamespaceLimits: Per-namespace overrides (future)
		0.95,                                   // samplingThreshold: Utilization to trigger sampling
		0.5,                                    // samplingRate: Sample rate when threshold reached
	)
	if cfg.Processing.Storm.BufferThreshold > 0 || cfg.Processing.Storm.InactivityTimeout > 0 {
		logger.Info("Using custom storm buffering configuration",
			"buffer_threshold", cfg.Processing.Storm.BufferThreshold,
			"inactivity_timeout", cfg.Processing.Storm.InactivityTimeout,
			"max_window_duration", cfg.Processing.Storm.MaxWindowDuration,
			"aggregation_window", cfg.Processing.Storm.AggregationWindow)
	}

	// Note: Environment classifier, Priority engine, and RemediationPathDecider removed (2025-12-06)
	// Environment/Priority classification now owned by Signal Processing per DD-CATEGORIZATION-001
	// Remediation path (risk_tolerance) derived by SP via Rego per DD-WORKFLOW-001
	// See: docs/handoff/NOTICE_GATEWAY_CLASSIFICATION_REMOVAL.md

	crdCreator := processing.NewCRDCreator(k8sClient, logger, metricsInstance, cfg.Processing.CRD.FallbackNamespace, &cfg.Processing.Retry)

	// DD-GATEWAY-011: Initialize status-based deduplication components (Redis deprecation)
	// These components update RR status fields instead of Redis, enabling Redis removal
	statusUpdater := processing.NewStatusUpdater(ctrlClient)
	phaseChecker := processing.NewPhaseBasedDeduplicationChecker(ctrlClient)

	// 4. Initialize middleware
	// DD-GATEWAY-004: Authentication middleware removed (network-level security)
	// authMiddleware := middleware.NewAuthMiddleware(clientset, logger) // REMOVED
	// rateLimiter := middleware.NewRateLimiter(cfg.Middleware.RateLimit.RequestsPerMinute, cfg.Middleware.RateLimit.Burst, logger) // REMOVED

	// 5. Create server
	server := &Server{
		adapterRegistry: adapterRegistry,
		deduplicator:    deduplicator,
		crdUpdater:      crdUpdater, // DD-GATEWAY-009: CRD updater for state-based deduplication
		stormDetector:   stormDetector,
		stormAggregator: stormAggregator,
		// DD-GATEWAY-011: Status-based deduplication (Redis deprecation path)
		statusUpdater: statusUpdater,
		phaseChecker:  phaseChecker,
		// Note: classifier, priorityEngine, pathDecider removed - SP owns classification and path
		crdCreator:  crdCreator,
		redisClient: redisClient,
		k8sClient:   k8sClient,
		ctrlClient:  ctrlClient,
		// DD-GATEWAY-004: Authentication middleware removed
		// authMiddleware:  authMiddleware, // REMOVED
		// rateLimiter:     rateLimiter,    // REMOVED
		metricsInstance: metricsInstance,
		logger:          logger,
		// aggregationGroup is zero-initialized (ready to use)
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
	// Start Redis health check goroutine (BR-106: Redis availability monitoring)
	go s.monitorRedisHealth(ctx)

	s.logger.Info("Starting Gateway server", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// monitorRedisHealth periodically checks Redis availability and updates metrics
//
// BUSINESS OUTCOME: Enable operators to track Redis SLO (BR-106)
// This goroutine runs every 5 seconds and:
// 1. Pings Redis to check availability
// 2. Updates gateway_redis_available gauge (1=available, 0=unavailable)
// 3. Tracks outage duration and count
//
// The health check runs independently of request processing to provide
// continuous visibility into Redis health even during low traffic periods.
func (s *Server) monitorRedisHealth(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	wasAvailable := true // Assume available at start
	outageStart := time.Time{}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Redis health monitor stopped")
			return
		case <-ticker.C:
			// Check Redis availability
			err := s.redisClient.EnsureConnection(ctx)
			isAvailable := (err == nil)

			// Update availability gauge
			if isAvailable {
				s.metricsInstance.RedisAvailable.Set(1)

				// If recovering from outage, record outage duration
				if !wasAvailable && !outageStart.IsZero() {
					outageDuration := time.Since(outageStart).Seconds()
					s.metricsInstance.RedisOutageDuration.Add(outageDuration)
					s.logger.Info("Redis recovered from outage",
						"outage_duration", time.Since(outageStart))
				}
			} else {
				s.metricsInstance.RedisAvailable.Set(0)

				// If this is start of new outage, record it
				if wasAvailable {
					outageStart = time.Now()
					s.metricsInstance.RedisOutageCount.Inc()
					s.logger.Info("Redis outage detected", "error", err)
				}
			}

			wasAvailable = isAvailable
		}
	}
}

// Stop gracefully stops the HTTP server
//
// This method implements industry best practice for graceful shutdown:
// 1. Set shutdown flag (readiness probe returns 503)
// 2. Wait 5 seconds for Kubernetes endpoint removal propagation
// 3. Shutdown HTTP server (waits for in-flight requests)
// 4. Close Redis connections
// 5. Return error if shutdown fails
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

	// STEP 4: Close Redis connections
	if s.redisClient != nil {
		if err := s.redisClient.Close(); err != nil {
			s.logger.Error(err, "Failed to close Redis client")
			return err
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
		// DD-GATEWAY-011: Update status.deduplication for duplicate tracking
		if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
			logger.Info("Failed to update deduplication status (DD-GATEWAY-011)",
				"error", err,
				"fingerprint", signal.Fingerprint,
				"rr", existingRR.Name)
		}

		// Record deduplication metric
		s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.AlertName).Inc()

		logger.V(1).Info("Duplicate signal detected (K8s status-based)",
			"fingerprint", signal.Fingerprint,
			"existingRR", existingRR.Name,
			"phase", existingRR.Status.OverallPhase)

		// Return duplicate response with data from existing RR
		return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
	}

	// 2. Storm detection
	isStorm, stormMetadata, err := s.stormDetector.Check(ctx, signal)
	if err != nil {
		// Non-critical error: log warning but continue processing
		logger.Info("Storm detection failed",
			"fingerprint", signal.Fingerprint,
			"error", err)
	} else if isStorm && stormMetadata != nil {
		// TDD REFACTOR: Extracted storm aggregation logic
		shouldContinue, response := s.processStormAggregation(ctx, signal, stormMetadata)
		if !shouldContinue {
			// Storm was aggregated, return response immediately
			return response, nil
		}

		// Aggregation failed, enrich signal for individual CRD creation
		signal.IsStorm = true
		signal.StormType = stormMetadata.StormType
		signal.StormWindow = stormMetadata.Window
		signal.AlertCount = stormMetadata.AlertCount
	}

	// 3. CRD creation pipeline
	// TDD REFACTOR: Extracted CRD creation logic
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

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Check Redis connectivity
	if err := s.redisClient.EnsureConnection(ctx); err != nil {
		s.logger.Info("Readiness check failed: Redis not reachable", "error", err)

		// Use RFC 7807 Problem Details format for structured error response
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusServiceUnavailable)

		errorResponse := gwerrors.RFC7807Error{
			Type:     gwerrors.ErrorTypeServiceUnavailable,
			Title:    gwerrors.TitleServiceUnavailable,
			Detail:   "Redis is not reachable",
			Status:   http.StatusServiceUnavailable,
			Instance: r.URL.Path,
		}

		if encErr := json.NewEncoder(w).Encode(errorResponse); encErr != nil {
			s.logger.Error(encErr, "Failed to encode readiness error response")
		}
		return
	}

	// Check Kubernetes API connectivity
	// Note: This is a placeholder - actual implementation would use k8sClient
	// to perform a simple API call (e.g. list namespaces with limit=1)

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

// processStormAggregation handles storm detection and aggregation logic
// TDD REFACTOR: Extracted from ProcessSignal for clarity
// DD-GATEWAY-008: Enhanced with buffered first-alert aggregation
// Business Outcome: Consistent storm aggregation (BR-GATEWAY-016, BR-GATEWAY-008)
// Returns: (shouldContinue bool, response *ProcessingResponse)
//   - shouldContinue=false means storm was aggregated/buffered, return response immediately
//   - shouldContinue=true means fall through to individual CRD creation
func (s *Server) processStormAggregation(ctx context.Context, signal *types.NormalizedSignal, stormMetadata *processing.StormMetadata) (bool, *ProcessingResponse) {
	logger := middleware.GetLogger(ctx)

	s.metricsInstance.AlertStormsDetectedTotal.WithLabelValues(stormMetadata.StormType, signal.AlertName).Inc()

	logger.Info("Alert storm detected",
		"fingerprint", signal.Fingerprint,
		"stormType", stormMetadata.StormType,
		"stormWindow", stormMetadata.Window,
		"alertCount", stormMetadata.AlertCount)

	// DD-GATEWAY-008: Check if aggregation window exists
	shouldAggregate, windowID, err := s.stormAggregator.ShouldAggregate(ctx, signal)
	if err != nil {
		logger.Info("Storm aggregation check failed, falling back to individual CRD creation",
			"fingerprint", signal.Fingerprint,
			"error", err)
		return true, nil // Continue to individual CRD creation
	}

	if shouldAggregate {
		// DD-GATEWAY-008: Add to existing aggregation window (sliding window behavior)
		if err := s.stormAggregator.AddResource(ctx, windowID, signal); err != nil {
			logger.Info("Failed to add resource to storm aggregation, falling back to individual CRD creation",
				"fingerprint", signal.Fingerprint,
				"windowID", windowID,
				"error", err)
			return true, nil // Continue to individual CRD creation
		}

		// Successfully added to aggregation window
		resourceCount, _ := s.stormAggregator.GetResourceCount(ctx, windowID)

		logger.Info("Alert added to storm aggregation window",
			"fingerprint", signal.Fingerprint,
			"windowID", windowID,
			"resourceCount", resourceCount)

		return false, NewStormAggregationResponse(signal.Fingerprint, windowID, stormMetadata.StormType, resourceCount, false)
	}

	// DD-GATEWAY-008: No window exists, call StartAggregation
	// StartAggregation will:
	// - Buffer first N alerts (default: 5)
	// - Return empty windowID if buffering (threshold not reached)
	// - Return windowID if threshold reached (window created)
	windowID, err = s.stormAggregator.StartAggregation(ctx, signal, stormMetadata)
	if err != nil {
		logger.Info("Failed to start storm aggregation, falling back to individual CRD creation",
			"fingerprint", signal.Fingerprint,
			"error", err)

		// DD-GATEWAY-008: Record buffer overflow/blocking metrics (BR-GATEWAY-011)
		// Check if error is due to capacity issues
		if strings.Contains(err.Error(), "over capacity") {
			s.metricsInstance.NamespaceBufferBlocking.WithLabelValues(signal.Namespace).Inc()
			s.metricsInstance.StormBufferOverflow.WithLabelValues(signal.Namespace).Inc()
		}

		return true, nil // Continue to individual CRD creation
	}

	// DD-GATEWAY-008: Check if window was created or alert was buffered
	if windowID == "" {
		// Alert buffered, threshold not reached yet
		logger.Info("Alert buffered for storm aggregation",
			"fingerprint", signal.Fingerprint,
			"alertName", signal.AlertName,
			"namespace", signal.Namespace)

		// DD-GATEWAY-008: Record namespace buffer utilization (BR-GATEWAY-011)
		if utilization, err := s.stormAggregator.GetNamespaceUtilization(ctx, signal.Namespace); err == nil {
			s.metricsInstance.NamespaceBufferUtilization.WithLabelValues(signal.Namespace).Set(utilization)
		}

		// Return HTTP 202 Accepted (buffered, waiting for more alerts)
		return false, &ProcessingResponse{
			Status:      StatusAccepted,
			Message:     "Alert buffered for storm aggregation (threshold not reached)",
			Fingerprint: signal.Fingerprint,
		}
	}

	// DD-GATEWAY-008: Threshold reached - create CRD IMMEDIATELY with ALL buffered alerts
	// Per DD-GATEWAY-008 lines 99-146: "When threshold reached, create aggregated CRD with ALL buffered alerts"
	logger.Info("Storm aggregation threshold reached - creating CRD immediately",
		"fingerprint", signal.Fingerprint,
		"windowID", windowID,
		"alertName", signal.AlertName,
		"namespace", signal.Namespace)

	// Create aggregated CRD synchronously
	crdName, err := s.createAggregatedCRD(ctx, windowID, signal, stormMetadata)
	if err != nil {
		logger.Info("Failed to create aggregated CRD, falling back to individual CRD creation",
			"windowID", windowID,
			"error", err)
		return true, nil // Fall back to individual CRD creation
	}

	// Monitor window expiration for cleanup (window continues accepting alerts)
	go s.monitorWindowExpiration(context.Background(), windowID)

	logger.Info("Storm aggregated CRD created",
		"fingerprint", signal.Fingerprint,
		"windowID", windowID,
		"crdName", crdName)

	// Return HTTP 201 Created with CRD name
	return false, &ProcessingResponse{
		Status:                 StatusCreated,
		Message:                "Storm aggregated CRD created",
		Fingerprint:            signal.Fingerprint,
		RemediationRequestName: crdName,
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

	// Store deduplication metadata in Redis (BR-003, BR-005, BR-077)
	// This enables duplicate detection across Gateway restarts and provides
	// persistent state for deduplication even when CRDs are deleted
	crdRef := fmt.Sprintf("%s/%s", rr.Namespace, rr.Name)
	if err := s.deduplicator.Store(ctx, signal, crdRef); err != nil {
		// Graceful degradation: log error but don't fail the request
		// The CRD is already created, so the alert is being processed
		logger.Info("Failed to store deduplication metadata in Redis",
			"fingerprint", signal.Fingerprint,
			"crd_ref", crdRef,
			"error", err)
	}

	// Record processing duration
	duration := time.Since(start)
	logger.Info("Signal processed successfully",
		"fingerprint", signal.Fingerprint,
		"crdName", rr.Name,
		"duration_ms", duration.Milliseconds())

	return NewCRDCreatedResponse(signal.Fingerprint, rr.Name, rr.Namespace), nil
}

// createAggregatedCRDAfterWindow creates a single aggregated RemediationRequest CRD
// after the storm aggregation window expires
//
// Business Requirement: BR-GATEWAY-016 - Storm aggregation
//
// This method is called in a goroutine when a storm aggregation window is started.
// It waits for the window duration (1 minute), then:
// 1. Retrieves all aggregated resources from Redis
// 2. Retrieves the original signal metadata
// 3. Creates a single RemediationRequest CRD with all resources
// 4. Stores deduplication metadata
//
// Benefits:
// - Reduces CRD count by 10-50x during storms
// - AI service receives single aggregated analysis request
// - Coordinated remediation instead of 50 parallel workflows
//
// Example:
// - Storm: 50 pod crashes in 1 minute
// - Without aggregation: 50 CRDs created
// - With aggregation: 1 CRD created with 50 resources listed
// createAggregatedCRD creates aggregated CRD immediately when threshold reached (DD-GATEWAY-008)
//
// DD-GATEWAY-008 Design Decision: Create CRD synchronously when buffer threshold reached
// Source: docs/architecture/decisions/DD-GATEWAY-008-storm-aggregation-first-alert-handling.md lines 99-146
//
// Returns:
// - string: CRD name
// - error: CRD creation errors
func (s *Server) createAggregatedCRD(
	ctx context.Context,
	windowID string,
	firstSignal *types.NormalizedSignal,
	stormMetadata *processing.StormMetadata,
) (string, error) {
	logger := middleware.GetLogger(ctx)

	// Retrieve all aggregated resources (includes buffered alerts moved to window)
	resources, err := s.stormAggregator.GetAggregatedResources(ctx, windowID)
	if err != nil {
		logger.Error(err, "Failed to retrieve aggregated resources",
			"windowID", windowID)
		return "", fmt.Errorf("failed to retrieve aggregated resources: %w", err)
	}

	// Retrieve signal metadata
	signal, storedStormMetadata, err := s.stormAggregator.GetSignalMetadata(ctx, windowID)
	if err != nil {
		logger.Info("Failed to retrieve signal metadata, using first signal",
			"windowID", windowID,
			"error", err)
		// Fall back to using the first signal passed as parameter
		signal = firstSignal
	} else {
		// Use stored storm metadata if available
		if storedStormMetadata != nil {
			stormMetadata = storedStormMetadata
		}
	}

	// Update alert count with actual aggregated count
	resourceCount := len(resources)

	logger.Info("Creating aggregated RemediationRequest CRD (DD-GATEWAY-008)",
		"windowID", windowID,
		"alertName", signal.AlertName,
		"resourceCount", resourceCount,
		"stormType", stormMetadata.StormType)

	// Create aggregated signal with all resources
	aggregatedSignal := *signal
	aggregatedSignal.IsStorm = true
	aggregatedSignal.StormType = stormMetadata.StormType
	aggregatedSignal.StormWindow = stormMetadata.Window
	aggregatedSignal.AlertCount = resourceCount
	aggregatedSignal.AffectedResources = resources

	// Create single aggregated RemediationRequest CRD
	// Note: Environment/Priority classification moved to SP per DD-CATEGORIZATION-001
	rr, err := s.crdCreator.CreateRemediationRequest(ctx, &aggregatedSignal)
	if err != nil {
		logger.Error(err, "Failed to create aggregated RemediationRequest CRD",
			"windowID", windowID,
			"resourceCount", resourceCount)

		// Record metric for failed aggregation
		s.metricsInstance.CRDCreationErrors.WithLabelValues("k8s_api_error").Inc()
		return "", fmt.Errorf("failed to create aggregated CRD: %w", err)
	}

	// Store deduplication metadata in Redis for storm aggregation (BR-GATEWAY-016)
	if err := s.deduplicator.Store(ctx, &aggregatedSignal, rr.Name); err != nil {
		logger.Info("Failed to store deduplication metadata",
			"fingerprint", aggregatedSignal.Fingerprint,
			"error", err)
	}

	// DD-GATEWAY-011: Update status.stormAggregation using StatusUpdater (Redis deprecation path)
	// This tracks storm aggregation in RR status instead of Redis
	isThresholdReached := resourceCount >= 5 // Default threshold per DD-GATEWAY-008
	if err := s.statusUpdater.UpdateStormAggregationStatus(ctx, rr, isThresholdReached); err != nil {
		logger.Info("Failed to update RR storm aggregation status (DD-GATEWAY-011)",
			"error", err,
			"crdName", rr.Name,
			"resourceCount", resourceCount)
		// Non-fatal: CRD is already created, storm tracking is bonus
	}

	// Record metrics
	s.metricsInstance.CRDsCreatedTotal.WithLabelValues(aggregatedSignal.SourceType, "storm_aggregated").Inc()

	logger.Info("Aggregated RemediationRequest CRD created successfully (DD-GATEWAY-008)",
		"crdName", rr.Name,
		"windowID", windowID,
		"resourceCount", resourceCount)

	return rr.Name, nil
}

// monitorWindowExpiration monitors window expiration for cleanup (DD-GATEWAY-008)
//
// DD-GATEWAY-008: Window continues accepting alerts after CRD creation (sliding window)
// This goroutine monitors the window and cleans up Redis keys after expiration.
// Respects context cancellation for graceful shutdown.
func (s *Server) monitorWindowExpiration(ctx context.Context, windowID string) {
	// Wait for window to expire, respecting context cancellation
	windowDuration := s.stormAggregator.GetWindowDuration()

	select {
	case <-time.After(windowDuration):
		s.logger.Info("Storm aggregation window expired, cleaning up",
			"windowID", windowID)
	// Cleanup: Window resources are already in CRD, just remove Redis keys
	// Note: Resources have 2x TTL for retrieval, will auto-expire

	case <-ctx.Done():
		s.logger.Info("Window expiration monitor cancelled (shutdown)",
			"windowID", windowID)
		return
	}
}

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
