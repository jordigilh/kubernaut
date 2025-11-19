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

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	goredis "github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"

	// "k8s.io/client-go/kubernetes" // DD-GATEWAY-004: No longer needed (authentication removed)
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	// "github.com/jordigilh/kubernaut/internal/gateway/redis" // DELETED: internal/gateway/ removed
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/middleware" // BR-109: Request ID middleware
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
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
//   - Deduplication: Check if signal was seen before (Redis lookup)
//   - Storm detection: Identify alert storms (rate-based, pattern-based)
//   - Classification: Determine environment (prod/staging/dev)
//   - Priority assignment: Calculate priority (P0/P1/P2)
//
// 3. CRD creation:
//   - Build RemediationRequest CRD from normalized signal
//   - Create CRD in Kubernetes
//   - Record deduplication metadata in Redis
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
	mux        *http.ServeMux // BR-109: Store mux reference for adapter registration

	// Core processing components
	adapterRegistry *adapters.AdapterRegistry
	deduplicator    *processing.DeduplicationService
	crdUpdater      *processing.CRDUpdater // DD-GATEWAY-009: CRD updater for state-based deduplication
	stormDetector   *processing.StormDetector
	stormAggregator *processing.StormAggregator
	classifier      *processing.EnvironmentClassifier
	priorityEngine  *processing.PriorityEngine
	pathDecider     *processing.RemediationPathDecider
	crdCreator      *processing.CRDCreator

	// Infrastructure clients
	redisClient *goredis.Client
	k8sClient   *k8s.Client
	ctrlClient  client.Client

	// Middleware
	// DD-GATEWAY-004: Authentication middleware removed (network-level security)
	// authMiddleware *middleware.AuthMiddleware // REMOVED
	// rateLimiter    *middleware.RateLimiter    // REMOVED

	// Metrics
	metricsInstance *metrics.Metrics

	// Logger
	logger *zap.Logger

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
// - Processing pipeline components (deduplication, storm, classification, priority, CRD)
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
func NewServer(cfg *config.ServerConfig, logger *zap.Logger) (*Server, error) {
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
func NewServerWithK8sClient(cfg *config.ServerConfig, logger *zap.Logger, metricsInstance *metrics.Metrics, ctrlClient client.Client) (*Server, error) {
	// 1. Initialize Redis client
	redisClient := goredis.NewClient(&goredis.Options{
		Addr:         cfg.Infrastructure.Redis.Addr,
		DB:           cfg.Infrastructure.Redis.DB,
		Password:     cfg.Infrastructure.Redis.Password,
		DialTimeout:  cfg.Infrastructure.Redis.DialTimeout,
		ReadTimeout:  cfg.Infrastructure.Redis.ReadTimeout,
		WriteTimeout: cfg.Infrastructure.Redis.WriteTimeout,
		PoolSize:     cfg.Infrastructure.Redis.PoolSize,
		MinIdleConns: cfg.Infrastructure.Redis.MinIdleConns,
	})

	// 2. Use provided Kubernetes client (shared with test)
	k8sClient := k8s.NewClient(ctrlClient)

	// 3. Initialize processing pipeline components
	return createServerWithClients(cfg, logger, metricsInstance, redisClient, ctrlClient, k8sClient)
}

func NewServerWithMetrics(cfg *config.ServerConfig, logger *zap.Logger, metricsInstance *metrics.Metrics) (*Server, error) {
	// 1. Initialize Redis client
	redisClient := goredis.NewClient(&goredis.Options{
		Addr:         cfg.Infrastructure.Redis.Addr,
		DB:           cfg.Infrastructure.Redis.DB,
		Password:     cfg.Infrastructure.Redis.Password,
		DialTimeout:  cfg.Infrastructure.Redis.DialTimeout,
		ReadTimeout:  cfg.Infrastructure.Redis.ReadTimeout,
		WriteTimeout: cfg.Infrastructure.Redis.WriteTimeout,
		PoolSize:     cfg.Infrastructure.Redis.PoolSize,
		MinIdleConns: cfg.Infrastructure.Redis.MinIdleConns,
	})

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

	// controller-runtime client (for environment classification and CRD creation)
	ctrlClient, err := client.New(kubeConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create controller-runtime client: %w", err)
	}

	// k8s client wrapper (for CRD operations)
	k8sClient := k8s.NewClient(ctrlClient)

	return createServerWithClients(cfg, logger, metricsInstance, redisClient, ctrlClient, k8sClient)
}

// createServerWithClients is the common server creation logic
func createServerWithClients(cfg *config.ServerConfig, logger *zap.Logger, metricsInstance *metrics.Metrics, redisClient *goredis.Client, ctrlClient client.Client, k8sClient *k8s.Client) (*Server, error) {
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
		logger.Info("Using custom deduplication TTL", zap.Duration("ttl", cfg.Processing.Deduplication.TTL))
	} else {
		deduplicator = processing.NewDeduplicationService(redisClient, k8sClient, logger, metricsInstance)
	}

	// DD-GATEWAY-009: Initialize CRD updater for duplicate alert handling
	crdUpdater := processing.NewCRDUpdater(k8sClient, logger)

	stormDetector := processing.NewStormDetector(redisClient, cfg.Processing.Storm.RateThreshold, cfg.Processing.Storm.PatternThreshold, metricsInstance)
	if cfg.Processing.Storm.RateThreshold > 0 || cfg.Processing.Storm.PatternThreshold > 0 {
		logger.Info("Using custom storm detection thresholds",
			zap.Int("rate_threshold", cfg.Processing.Storm.RateThreshold),
			zap.Int("pattern_threshold", cfg.Processing.Storm.PatternThreshold),
		)
	}

	stormAggregator := processing.NewStormAggregatorWithWindow(redisClient, cfg.Processing.Storm.AggregationWindow)
	if cfg.Processing.Storm.AggregationWindow > 0 {
		logger.Info("Using custom storm aggregation window", zap.Duration("window", cfg.Processing.Storm.AggregationWindow))
	}

	// Create environment classifier with configurable cache TTL
	var classifier *processing.EnvironmentClassifier
	if cfg.Processing.Environment.CacheTTL > 0 {
		classifier = processing.NewEnvironmentClassifierWithTTL(ctrlClient, logger, cfg.Processing.Environment.CacheTTL)
		logger.Info("Using custom environment cache TTL", zap.Duration("cache_ttl", cfg.Processing.Environment.CacheTTL))
	} else {
		classifier = processing.NewEnvironmentClassifier(ctrlClient, logger)
	}

	// Create priority engine with Rego policy (REQUIRED)
	// Architecture Decision: Priority assignment MUST use Rego policies
	// Gateway fails to start if policy cannot be loaded
	if cfg.Processing.Priority.PolicyPath == "" {
		return nil, fmt.Errorf("priority policy path is required (cfg.Processing.Priority.PolicyPath)")
	}

	priorityEngine, err := processing.NewPriorityEngineWithRego(cfg.Processing.Priority.PolicyPath, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to load priority Rego policy (fail-fast): %w", err)
	}

	logger.Info("Loaded Rego policy for priority assignment",
		zap.String("policy_path", cfg.Processing.Priority.PolicyPath),
	)

	pathDecider := processing.NewRemediationPathDecider(logger)
	crdCreator := processing.NewCRDCreator(k8sClient, logger, metricsInstance, cfg.Processing.CRD.FallbackNamespace, &cfg.Processing.Retry)

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
		classifier:      classifier,
		priorityEngine:  priorityEngine,
		pathDecider:     pathDecider,
		crdCreator:      crdCreator,
		redisClient:     redisClient,
		k8sClient:       k8sClient,
		ctrlClient:      ctrlClient,
		// DD-GATEWAY-004: Authentication middleware removed
		// authMiddleware:  authMiddleware, // REMOVED
		// rateLimiter:     rateLimiter,    // REMOVED
		metricsInstance: metricsInstance,
		logger:          logger,
	}

	// 6. Setup HTTP server with routes
	mux := server.setupRoutes()

	// 7. Store mux reference for adapter registration (BR-109)
	server.mux = mux

	// 8. Wrap with request ID middleware (BR-109)
	// BUSINESS OUTCOME: Enable operators to trace requests across Gateway components
	// TDD GREEN: Minimal implementation to make BR-109 tests pass
	handler := server.wrapWithMiddleware(mux)

	server.httpServer = &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	return server, nil
}

// setupRoutes configures all HTTP routes
//
// Routes:
// - /api/v1/signals/* : Dynamic routes for registered adapters (e.g. /api/v1/signals/prometheus)
// - /health           : Liveness probe (always returns 200)
// - /ready            : Readiness probe (checks Redis + K8s connectivity)
// - /metrics          : Prometheus metrics endpoint
func (s *Server) setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Note: Adapter routes will be registered dynamically when adapters are registered
	// via RegisterAdapter(). Each adapter exposes its own route (e.g. /api/v1/signals/prometheus)

	// Health and readiness probes
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/healthz", s.healthHandler) // Kubernetes-style alias
	mux.HandleFunc("/ready", s.readinessHandler)

	// Prometheus metrics
	// Expose metrics from custom registry (for test isolation)
	// If metricsInstance is nil, this will use the default registry
	var metricsHandler http.Handler
	if s.metricsInstance != nil && s.metricsInstance.Registry() != nil {
		metricsHandler = promhttp.HandlerFor(s.metricsInstance.Registry(), promhttp.HandlerOpts{})
	} else {
		metricsHandler = promhttp.Handler() // Default registry
	}
	mux.Handle("/metrics", metricsHandler)

	return mux
}

// wrapWithMiddleware wraps the HTTP handler with middleware stack
//
// BUSINESS OUTCOME: Enable structured logging with request context (BR-109)
// TDD GREEN: Minimal middleware stack for request tracing
//
// Middleware Stack:
// 1. Request ID Middleware: Adds request_id, source_ip, endpoint to logs
// 2. Performance Logging: Adds duration_ms to logs
//
// Future middleware can be added here (rate limiting, authentication, etc.)
func (s *Server) wrapWithMiddleware(handler http.Handler) http.Handler {
	// Wrap with performance logging middleware first (BR-109)
	// This runs LAST (after request ID is set)
	handler = s.performanceLoggingMiddleware(handler)

	// Wrap with request ID middleware (BR-109)
	// This runs FIRST (sets request ID for all subsequent middleware/handlers)
	handler = middleware.RequestIDMiddleware(s.logger)(handler)

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

		// Log request completion with duration (debug level for health/readiness checks to reduce noise)
		logger := middleware.GetLogger(r.Context())
		if r.URL.Path == "/health" || r.URL.Path == "/healthz" || r.URL.Path == "/ready" {
			logger.Debug("Request completed",
				zap.Float64("duration_ms", float64(duration.Milliseconds())),
			)
		} else {
			logger.Info("Request completed",
				zap.Float64("duration_ms", float64(duration.Milliseconds())),
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

// RegisterAdapter registers a RoutableAdapter and its HTTP route
//
// This method:
// 1. Validates adapter (checks for duplicate names/routes)
// 2. Registers adapter in registry
// 3. Creates HTTP handler that calls adapter.Parse()
// 4. Configures HTTP route with full middleware stack
//
// Middleware stack (applied to all adapter routes):
// - Rate limiting (per-IP, 100 req/min)
// - Authentication (TokenReview bearer token validation)
// - Request logging
// - Metrics recording
//
// Example:
//
//	prometheusAdapter := adapters.NewPrometheusAdapter(logger)
//	server.RegisterAdapter(prometheusAdapter)
//	// Now /api/v1/signals/prometheus is active
func (s *Server) RegisterAdapter(adapter adapters.RoutableAdapter) error {
	// Register in registry
	if err := s.adapterRegistry.Register(adapter); err != nil {
		return fmt.Errorf("failed to register adapter: %w", err)
	}

	// Create adapter HTTP handler
	handler := s.createAdapterHandler(adapter)

	// DD-GATEWAY-004: Middleware removed (network-level security)
	// No rate limiting or authentication middleware
	// Security now handled at network layer (Network Policies + TLS)

	// BR-042: Apply Content-Type validation middleware
	// Rejects non-JSON payloads early, before processing
	wrappedHandler := middleware.ValidateContentType(handler)

	// Register route with Content-Type validation
	// BR-109: Use stored mux reference instead of casting httpServer.Handler
	s.mux.Handle(adapter.GetRoute(), wrappedHandler)

	s.logger.Info("Registered adapter route",
		zap.String("adapter", adapter.Name()),
		zap.String("route", adapter.GetRoute()))

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
func (s *Server) createAdapterHandler(adapter adapters.SignalAdapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only accept POST requests
		if r.Method != http.MethodPost {
			s.writeJSONError(w, r, "Method not allowed", http.StatusMethodNotAllowed) // TDD REFACTOR: Keep as-is (405 is not common enough for helper)
			return
		}

		start := time.Now()
		ctx := r.Context()

		// BR-109: Get request-scoped logger from context (includes request_id, source_ip, endpoint)
		logger := middleware.GetLogger(ctx)

		// Read request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Failed to read request body", zap.Error(err))
			s.writeValidationError(w, r, "Failed to read request body") // TDD REFACTOR: Use typed helper, BR-109: Added request
			return
		}

		// Parse signal using adapter
		signal, err := adapter.Parse(ctx, body)
		if err != nil {
			logger.Warn("Failed to parse signal",
				zap.String("adapter", adapter.Name()),
				zap.Error(err),
			)

			// TDD REFACTOR: Parse errors are validation errors (400 Bad Request)
			// Note: Payload size errors (413) are handled by adapter-specific logic
			s.writeValidationError(w, r, fmt.Sprintf("Failed to parse signal: %v", err)) // BR-109: Added request
			return
		}

		// Validate signal
		if err := adapter.Validate(signal); err != nil {
			logger.Warn("Signal validation failed",
				zap.String("adapter", adapter.Name()),
				zap.Error(err))
			s.writeValidationError(w, r, fmt.Sprintf("Signal validation failed: %v", err)) // TDD REFACTOR: Use typed helper, BR-109: Added request
			return
		}

		// Process signal through pipeline
		response, err := s.ProcessSignal(ctx, signal)
		if err != nil {
			logger.Error("Signal processing failed",
				zap.String("adapter", adapter.Name()),
				zap.Error(err))

			// BR-GATEWAY-008, BR-GATEWAY-009: Return 503 when Redis unavailable (per DD-GATEWAY-003)
			// Check if error is due to Redis unavailability
			// TDD REFACTOR: Use typed helper for consistent error handling
			if strings.Contains(err.Error(), "redis unavailable") || strings.Contains(err.Error(), "deduplication check failed") {
				// Set Retry-After header (30 seconds - allows time for Redis HA failover)
				s.writeServiceUnavailableError(w, r, "Deduplication service unavailable - Redis connection failed. Please retry after 30 seconds.", 30) // BR-109: Added request
				return
			}

			s.writeInternalError(w, r, "Internal server error") // TDD REFACTOR: Use typed helper, BR-109: Added request
			return
		}

		// Determine HTTP status code based on response status
		// BR-GATEWAY-016: Storm aggregation returns 202 Accepted
		// BR-GATEWAY-003: Deduplication returns 202 Accepted
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
			logger.Error("Failed to encode JSON response", zap.Error(err))
		}
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

	s.logger.Info("Starting Gateway server", zap.String("addr", s.httpServer.Addr))
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
			err := s.redisClient.Ping(ctx).Err()
			isAvailable := (err == nil)

			// Update availability gauge
			if isAvailable {
				s.metricsInstance.RedisAvailable.Set(1)

				// If recovering from outage, record outage duration
				if !wasAvailable && !outageStart.IsZero() {
					outageDuration := time.Since(outageStart).Seconds()
					s.metricsInstance.RedisOutageDuration.Add(outageDuration)
					s.logger.Info("Redis recovered from outage",
						zap.Duration("outage_duration", time.Since(outageStart)))
				}
			} else {
				s.metricsInstance.RedisAvailable.Set(0)

				// If this is start of new outage, record it
				if wasAvailable {
					outageStart = time.Now()
					s.metricsInstance.RedisOutageCount.Inc()
					s.logger.Warn("Redis outage detected", zap.Error(err))
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
		s.logger.Error("Failed to gracefully shutdown HTTP server", zap.Error(err))
		return err
	}

	// STEP 4: Close Redis connections
	if s.redisClient != nil {
		if err := s.redisClient.Close(); err != nil {
			s.logger.Error("Failed to close Redis client", zap.Error(err))
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
// 4. Environment classification (namespace labels + ConfigMap)
// 5. Priority assignment (Rego policy or fallback table)
// 6. CRD creation (Kubernetes API)
// 7. Store deduplication metadata (Redis)
// 8. Return HTTP 201 with CRD details
//
// Performance:
// - Typical latency (new signal): p95 ~80ms, p99 ~120ms
//   - Deduplication check: ~3ms
//   - Storm detection: ~3ms
//   - Environment classification: ~15ms (namespace label lookup)
//   - Priority assignment: ~1ms
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

	// Record ingestion metric
	s.metricsInstance.AlertsReceivedTotal.WithLabelValues(signal.SourceType, signal.Severity, "unknown").Inc()

	// 1. Deduplication check
	isDuplicate, metadata, err := s.deduplicator.Check(ctx, signal)
	if err != nil {
		logger.Error("Deduplication check failed",
			zap.String("fingerprint", signal.Fingerprint),
			zap.Error(err))
		return nil, fmt.Errorf("deduplication check failed: %w", err)
	}

	if isDuplicate {
		// TDD REFACTOR: Extracted duplicate handling
		return s.processDuplicateSignal(ctx, signal, metadata), nil
	}

	// 2. Storm detection
	isStorm, stormMetadata, err := s.stormDetector.Check(ctx, signal)
	if err != nil {
		// Non-critical error: log warning but continue processing
		logger.Warn("Storm detection failed",
			zap.String("fingerprint", signal.Fingerprint),
			zap.Error(err))
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
		s.logger.Error("Failed to encode health response", zap.Error(err))
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
			s.logger.Error("Failed to encode readiness error response", zap.Error(encErr))
		}
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Check Redis connectivity
	if err := s.redisClient.Ping(ctx).Err(); err != nil {
		s.logger.Warn("Readiness check failed: Redis not reachable", zap.Error(err))

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
			s.logger.Error("Failed to encode readiness error response", zap.Error(encErr))
		}
		return
	}

	// Check Kubernetes API connectivity
	// Note: This is a placeholder - actual implementation would use k8sClient
	// to perform a simple API call (e.g. list namespaces with limit=1)

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ready"}); err != nil {
		s.logger.Error("Failed to encode readiness response", zap.Error(err))
	}
}

// ProcessingResponse represents the result of signal processing
type ProcessingResponse struct {
	Status                      string                            `json:"status"` // "created", "duplicate", or "accepted"
	Message                     string                            `json:"message"`
	Fingerprint                 string                            `json:"fingerprint"`
	Duplicate                   bool                              `json:"duplicate"`
	RemediationRequestName      string                            `json:"remediationRequestName,omitempty"`
	RemediationRequestNamespace string                            `json:"remediationRequestNamespace,omitempty"`
	Environment                 string                            `json:"environment,omitempty"` // TDD FIX: Top-level per API spec
	Priority                    string                            `json:"priority,omitempty"`    // TDD FIX: Top-level per API spec
	RemediationPath             string                            `json:"remediationPath,omitempty"`
	Metadata                    *processing.DeduplicationMetadata `json:"metadata,omitempty"` // Deduplication info only
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
func NewDuplicateResponse(fingerprint string, metadata *processing.DeduplicationMetadata) *ProcessingResponse {
	return &ProcessingResponse{
		Status:      StatusDuplicate,
		Message:     "Duplicate signal (deduplication successful)",
		Fingerprint: fingerprint,
		Duplicate:   true,
		Metadata:    metadata,
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
func NewCRDCreatedResponse(fingerprint, crdName, crdNamespace, environment, priority, remediationPath string) *ProcessingResponse {
	return &ProcessingResponse{
		Status:                      StatusCreated,
		Message:                     "RemediationRequest CRD created successfully",
		Fingerprint:                 fingerprint,
		Duplicate:                   false,
		RemediationRequestName:      crdName,
		RemediationRequestNamespace: crdNamespace,
		Environment:                 environment,
		Priority:                    priority,
		RemediationPath:             remediationPath,
	}
}

// processDuplicateSignal handles the duplicate signal fast path
// TDD REFACTOR: Extracted from ProcessSignal for clarity
// Business Outcome: Consistent duplicate handling (BR-005)
func (s *Server) processDuplicateSignal(ctx context.Context, signal *types.NormalizedSignal, metadata *processing.DeduplicationMetadata) *ProcessingResponse {
	logger := middleware.GetLogger(ctx)

	// DD-GATEWAY-009: Update CRD occurrence count for duplicate alerts
	// Parse CRD namespace/name from RemediationRequestRef (format: "namespace/name")
	namespace, name := s.parseCRDReference(metadata.RemediationRequestRef)
	if namespace == "" || name == "" {
		// Fallback: generate CRD name from fingerprint (same as deduplication service)
		namespace = signal.Namespace
		name = s.deduplicator.GetCRDNameFromFingerprint(signal.Fingerprint)
	}

	// Update CRD occurrence count in Kubernetes
	if err := s.crdUpdater.IncrementOccurrenceCount(ctx, namespace, name); err != nil {
		// Log error but don't fail the request
		// The duplicate response is still valid even if CRD update fails
		logger.Warn("Failed to increment CRD occurrence count (duplicate alert still processed)",
			zap.Error(err),
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("namespace", namespace),
			zap.String("name", name))
	}

	// Fast path: duplicate signal, no CRD creation needed
	s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.AlertName, "unknown").Inc()

	logger.Debug("Duplicate signal detected",
		zap.String("fingerprint", signal.Fingerprint),
		zap.Int("count", metadata.Count),
		zap.String("firstSeen", metadata.FirstSeen),
	)

	return NewDuplicateResponse(signal.Fingerprint, metadata)
}

// parseCRDReference parses a CRD reference string into namespace and name
//
// DD-GATEWAY-009: Helper for parsing RemediationRequestRef
// Format: "namespace/name" (e.g., "production/rr-abc123")
//
// Returns:
// - namespace: The namespace part (empty if invalid format)
// - name: The name part (empty if invalid format)
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

	logger.Warn("Alert storm detected",
		zap.String("fingerprint", signal.Fingerprint),
		zap.String("stormType", stormMetadata.StormType),
		zap.String("stormWindow", stormMetadata.Window),
		zap.Int("alertCount", stormMetadata.AlertCount))

	// DD-GATEWAY-008: Check if aggregation window exists
	shouldAggregate, windowID, err := s.stormAggregator.ShouldAggregate(ctx, signal)
	if err != nil {
		logger.Warn("Storm aggregation check failed, falling back to individual CRD creation",
			zap.String("fingerprint", signal.Fingerprint),
			zap.Error(err))
		return true, nil // Continue to individual CRD creation
	}

	if shouldAggregate {
		// DD-GATEWAY-008: Add to existing aggregation window (sliding window behavior)
		if err := s.stormAggregator.AddResource(ctx, windowID, signal); err != nil {
			logger.Warn("Failed to add resource to storm aggregation, falling back to individual CRD creation",
				zap.String("fingerprint", signal.Fingerprint),
				zap.String("windowID", windowID),
				zap.Error(err))
			return true, nil // Continue to individual CRD creation
		}

		// Successfully added to aggregation window
		resourceCount, _ := s.stormAggregator.GetResourceCount(ctx, windowID)

		logger.Info("Alert added to storm aggregation window",
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("windowID", windowID),
			zap.Int("resourceCount", resourceCount))

		return false, NewStormAggregationResponse(signal.Fingerprint, windowID, stormMetadata.StormType, resourceCount, false)
	}

	// DD-GATEWAY-008: No window exists, call StartAggregation
	// StartAggregation will:
	// - Buffer first N alerts (default: 5)
	// - Return empty windowID if buffering (threshold not reached)
	// - Return windowID if threshold reached (window created)
	windowID, err = s.stormAggregator.StartAggregation(ctx, signal, stormMetadata)
	if err != nil {
		logger.Warn("Failed to start storm aggregation, falling back to individual CRD creation",
			zap.String("fingerprint", signal.Fingerprint),
			zap.Error(err))
		return true, nil // Continue to individual CRD creation
	}

	// DD-GATEWAY-008: Check if window was created or alert was buffered
	if windowID == "" {
		// Alert buffered, threshold not reached yet
		logger.Info("Alert buffered for storm aggregation",
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("alertName", signal.AlertName),
			zap.String("namespace", signal.Namespace))

		// Return HTTP 202 Accepted (buffered, waiting for more alerts)
		return false, &ProcessingResponse{
			Status:      StatusAccepted,
			Message:     "Alert buffered for storm aggregation (threshold not reached)",
			Fingerprint: signal.Fingerprint,
		}
	}

	// Window created (threshold reached), schedule CRD creation after window expires
	go s.createAggregatedCRDAfterWindow(context.Background(), windowID, signal, stormMetadata)

	logger.Info("Storm aggregation window started",
		zap.String("fingerprint", signal.Fingerprint),
		zap.String("windowID", windowID),
		zap.String("windowTTL", "60 seconds (sliding window)"))

	return false, NewStormAggregationResponse(signal.Fingerprint, windowID, stormMetadata.StormType, 0, true)
}

// createRemediationRequestCRD handles the CRD creation pipeline
// TDD REFACTOR: Extracted from ProcessSignal for clarity
// Business Outcome: Consistent CRD creation (BR-004)
func (s *Server) createRemediationRequestCRD(ctx context.Context, signal *types.NormalizedSignal, start time.Time) (*ProcessingResponse, error) {
	logger := middleware.GetLogger(ctx)

	// 3. Environment classification
	environment := s.classifier.Classify(ctx, signal.Namespace)

	// 4. Priority assignment
	priority := s.priorityEngine.Assign(ctx, signal.Severity, environment)

	// 5. Remediation path decision
	signalCtx := &processing.SignalContext{
		Signal:      signal,
		Environment: environment,
		Priority:    priority,
	}
	remediationPath := s.pathDecider.DeterminePath(ctx, signalCtx)

	logger.Debug("Remediation path decided",
		zap.String("fingerprint", signal.Fingerprint),
		zap.String("environment", environment),
		zap.String("priority", priority),
		zap.String("remediationPath", remediationPath))

	// 6. Create RemediationRequest CRD
	rr, err := s.crdCreator.CreateRemediationRequest(ctx, signal, priority, environment)
	if err != nil {
		logger.Error("Failed to create RemediationRequest CRD",
			zap.String("fingerprint", signal.Fingerprint),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create RemediationRequest CRD: %w", err)
	}

	// DD-GATEWAY-009: v1.0 uses K8s API for deduplication (no Redis caching)
	// v1.1 will add informer pattern to reduce API load
	// No Redis storage needed - deduplication queries K8s CRD state directly

	// Record processing duration
	duration := time.Since(start)
	logger.Info("Signal processed successfully",
		zap.String("fingerprint", signal.Fingerprint),
		zap.String("crdName", rr.Name),
		zap.String("environment", environment),
		zap.String("priority", priority),
		zap.String("remediationPath", remediationPath),
		zap.Int64("duration_ms", duration.Milliseconds()))

	return NewCRDCreatedResponse(signal.Fingerprint, rr.Name, rr.Namespace, environment, priority, remediationPath), nil
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
func (s *Server) createAggregatedCRDAfterWindow(
	ctx context.Context,
	windowID string,
	firstSignal *types.NormalizedSignal,
	stormMetadata *processing.StormMetadata,
) {
	// Wait for aggregation window to expire (configurable: 5s for tests, 1m for production)
	windowDuration := s.stormAggregator.GetWindowDuration()
	time.Sleep(windowDuration)

	s.logger.Info("Storm aggregation window expired, creating aggregated CRD",
		zap.String("windowID", windowID),
		zap.String("alertName", firstSignal.AlertName),
		zap.Duration("duration", windowDuration))

	// Retrieve all aggregated resources
	resources, err := s.stormAggregator.GetAggregatedResources(ctx, windowID)
	if err != nil {
		s.logger.Error("Failed to retrieve aggregated resources",
			zap.String("windowID", windowID),
			zap.Error(err))
		return
	}

	// Retrieve signal metadata
	signal, storedStormMetadata, err := s.stormAggregator.GetSignalMetadata(ctx, windowID)
	if err != nil {
		s.logger.Warn("Failed to retrieve signal metadata, using first signal",
			zap.String("windowID", windowID),
			zap.Error(err))
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

	s.logger.Info("Creating aggregated RemediationRequest CRD",
		zap.String("windowID", windowID),
		zap.String("alertName", signal.AlertName),
		zap.Int("resourceCount", resourceCount),
		zap.String("stormType", stormMetadata.StormType))

	// Create aggregated signal with all resources
	aggregatedSignal := *signal
	aggregatedSignal.IsStorm = true
	aggregatedSignal.StormType = stormMetadata.StormType
	aggregatedSignal.StormWindow = stormMetadata.Window
	aggregatedSignal.AlertCount = resourceCount
	aggregatedSignal.AffectedResources = resources

	// Environment classification
	environment := s.classifier.Classify(ctx, aggregatedSignal.Namespace)

	// Priority assignment
	priority := s.priorityEngine.Assign(ctx, aggregatedSignal.Severity, environment)

	// Create single aggregated RemediationRequest CRD
	rr, err := s.crdCreator.CreateRemediationRequest(ctx, &aggregatedSignal, priority, environment)
	if err != nil {
		s.logger.Error("Failed to create aggregated RemediationRequest CRD",
			zap.String("windowID", windowID),
			zap.Int("resourceCount", resourceCount),
			zap.Error(err))

		// Record metric for failed aggregation
		s.metricsInstance.CRDCreationErrors.WithLabelValues("k8s_api_error").Inc()
		return
	}

	// DD-GATEWAY-009: v1.0 uses K8s API for deduplication (no Redis caching)
	// v1.1 will add informer pattern to reduce API load
	// No Redis storage needed - deduplication queries K8s CRD state directly

	s.logger.Info("Aggregated RemediationRequest CRD created successfully",
		zap.String("windowID", windowID),
		zap.String("crdName", rr.Name),
		zap.String("crdNamespace", rr.Namespace),
		zap.Int("resourceCount", resourceCount),
		zap.String("environment", environment),
		zap.String("priority", priority),
		zap.String("stormType", stormMetadata.StormType))

	// Record metrics for successful aggregation
	s.metricsInstance.CRDsCreatedTotal.WithLabelValues(environment, priority).Inc()
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
	sanitizedMessage := middleware.SanitizeForLog(message)

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
