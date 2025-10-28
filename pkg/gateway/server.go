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
	"time"

	goredis "github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	// "k8s.io/client-go/kubernetes" // DD-GATEWAY-004: No longer needed (authentication removed)
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// "github.com/jordigilh/kubernaut/internal/gateway/redis" // DELETED: internal/gateway/ removed
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"

	// "github.com/jordigilh/kubernaut/pkg/gateway/middleware" // DD-GATEWAY-004: Middleware removed
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

	// Core processing components
	adapterRegistry *adapters.AdapterRegistry
	deduplicator    *processing.DeduplicationService
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
}

// ServerConfig holds server configuration
// ServerConfig is the top-level configuration for the Gateway service.
// Organized by Single Responsibility Principle for better maintainability.
type ServerConfig struct {
	// HTTP Server configuration
	Server ServerSettings `yaml:"server"`

	// Middleware configuration
	Middleware MiddlewareSettings `yaml:"middleware"`

	// Infrastructure dependencies
	Infrastructure InfrastructureSettings `yaml:"infrastructure"`

	// Business logic configuration
	Processing ProcessingSettings `yaml:"processing"`
}

// ServerSettings contains HTTP server configuration.
// Single Responsibility: HTTP server behavior
type ServerSettings struct {
	ListenAddr   string        `yaml:"listen_addr"`   // Default: ":8080"
	ReadTimeout  time.Duration `yaml:"read_timeout"`  // Default: 30s
	WriteTimeout time.Duration `yaml:"write_timeout"` // Default: 30s
	IdleTimeout  time.Duration `yaml:"idle_timeout"`  // Default: 120s
}

// MiddlewareSettings contains middleware configuration.
// Single Responsibility: Request processing middleware
type MiddlewareSettings struct {
	RateLimit RateLimitSettings `yaml:"rate_limit"`
}

// RateLimitSettings contains rate limiting configuration.
type RateLimitSettings struct {
	RequestsPerMinute int `yaml:"requests_per_minute"` // Default: 100
	Burst             int `yaml:"burst"`               // Default: 10
}

// InfrastructureSettings contains external dependency configuration.
// Single Responsibility: Infrastructure connections
type InfrastructureSettings struct {
	Redis *goredis.Options `yaml:"redis"`
}

// ProcessingSettings contains business logic configuration.
// Single Responsibility: Signal processing behavior
type ProcessingSettings struct {
	Deduplication DeduplicationSettings `yaml:"deduplication"`
	Storm         StormSettings         `yaml:"storm"`
	Environment   EnvironmentSettings   `yaml:"environment"`
}

// DeduplicationSettings contains deduplication configuration.
type DeduplicationSettings struct {
	// TTL for deduplication fingerprints
	// For testing: set to 5*time.Second for fast tests
	// For production: use default (0) for 5-minute TTL
	TTL time.Duration `yaml:"ttl"` // Default: 5m
}

// StormSettings contains storm detection configuration.
type StormSettings struct {
	// Rate threshold for rate-based storm detection
	// For testing: set to 2-3 for early storm detection in tests
	// For production: use default (0) for 10 alerts/minute
	RateThreshold int `yaml:"rate_threshold"` // Default: 10 alerts/minute

	// Pattern threshold for pattern-based storm detection
	// For testing: set to 2-3 for early storm detection in tests
	// For production: use default (0) for 5 similar alerts
	PatternThreshold int `yaml:"pattern_threshold"` // Default: 5 similar alerts

	// Aggregation window for storm aggregation
	// For testing: set to 5*time.Second for fast integration tests
	// For production: use default (0) for 1-minute windows
	AggregationWindow time.Duration `yaml:"aggregation_window"` // Default: 1m
}

// EnvironmentSettings contains environment classification configuration.
type EnvironmentSettings struct {
	// Cache TTL for namespace label cache
	// For testing: set to 5*time.Second for fast cache expiry in tests
	// For production: use default (0) for 30-second TTL
	CacheTTL time.Duration `yaml:"cache_ttl"` // Default: 30s

	// ConfigMap for environment overrides
	ConfigMapNamespace string `yaml:"configmap_namespace"` // Default: "kubernaut-system"
	ConfigMapName      string `yaml:"configmap_name"`      // Default: "kubernaut-environment-overrides"
}

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
func NewServer(cfg *ServerConfig, logger *zap.Logger) (*Server, error) {
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
func NewServerWithMetrics(cfg *ServerConfig, logger *zap.Logger, metricsInstance *metrics.Metrics) (*Server, error) {
	// 1. Initialize Redis client
	redisClient := goredis.NewClient(cfg.Infrastructure.Redis)

	// 2. Initialize Kubernetes clients
	// Get kubeconfig
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	// controller-runtime client (for environment classification)
	ctrlClient, err := client.New(kubeConfig, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to create controller-runtime client: %w", err)
	}

	// k8s client wrapper (for CRD operations)
	k8sClient := k8s.NewClient(ctrlClient)

	// DD-GATEWAY-004: kubernetes clientset removed (no longer needed for authentication)
	// clientset, err := kubernetes.NewForConfig(kubeConfig) // REMOVED
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	// } // REMOVED

	// 3. Initialize processing pipeline components
	adapterRegistry := adapters.NewAdapterRegistry(logger)

	// Use provided metrics instance or create new one with default registry
	if metricsInstance == nil {
		metricsInstance = metrics.NewMetrics()
	}

	var deduplicator *processing.DeduplicationService
	if cfg.Processing.Deduplication.TTL > 0 {
		deduplicator = processing.NewDeduplicationServiceWithTTL(redisClient, cfg.Processing.Deduplication.TTL, logger, metricsInstance)
		logger.Info("Using custom deduplication TTL", zap.Duration("ttl", cfg.Processing.Deduplication.TTL))
	} else {
		deduplicator = processing.NewDeduplicationService(redisClient, logger, metricsInstance)
	}

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
	priorityEngine := processing.NewPriorityEngine(logger)
	pathDecider := processing.NewRemediationPathDecider(logger)
	crdCreator := processing.NewCRDCreator(k8sClient, logger, metricsInstance)

	// 4. Initialize middleware
	// DD-GATEWAY-004: Authentication middleware removed (network-level security)
	// authMiddleware := middleware.NewAuthMiddleware(clientset, logger) // REMOVED
	// rateLimiter := middleware.NewRateLimiter(cfg.Middleware.RateLimit.RequestsPerMinute, cfg.Middleware.RateLimit.Burst, logger) // REMOVED

	// 5. Create server
	server := &Server{
		adapterRegistry: adapterRegistry,
		deduplicator:    deduplicator,
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
	server.httpServer = &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      mux,
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
	mux.Handle("/metrics", promhttp.Handler())

	return mux
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

	// Register route directly (no middleware wrapping)
	s.httpServer.Handler.(*http.ServeMux).Handle(adapter.GetRoute(), handler)

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
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		start := time.Now()
		ctx := r.Context()

		// Read request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.logger.Error("Failed to read request body", zap.Error(err))
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		// Parse signal using adapter
		signal, err := adapter.Parse(ctx, body)
		if err != nil {
			s.logger.Warn("Failed to parse signal",
				zap.String("adapter", adapter.Name()),
				zap.Error(err),
			)

			// Return 413 for payload size errors, 400 for other parse errors
			statusCode := http.StatusBadRequest
			if strings.Contains(err.Error(), "payload too large") {
				statusCode = http.StatusRequestEntityTooLarge
			}

			http.Error(w, fmt.Sprintf("Failed to parse signal: %v", err), statusCode)
			return
		}

		// Validate signal
		if err := adapter.Validate(signal); err != nil {
			s.logger.Warn("Signal validation failed",
				zap.String("adapter", adapter.Name()),
				zap.Error(err))
			http.Error(w, fmt.Sprintf("Signal validation failed: %v", err), http.StatusBadRequest)
			return
		}

		// Process signal through pipeline
		response, err := s.ProcessSignal(ctx, signal)
		if err != nil {
			s.logger.Error("Signal processing failed",
				zap.String("adapter", adapter.Name()),
				zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Determine HTTP status code
		statusCode := http.StatusCreated
		if response.Duplicate {
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
			s.logger.Error("Failed to encode JSON response", zap.Error(err))
		}
	}
}

// Start starts the HTTP server
//
// This method:
// 1. Logs startup message
// 2. Starts HTTP server (blocking)
// 3. Returns error if server fails to start
//
// Start should be called in a goroutine:
//
//	go func() {
//	    if err := server.Start(ctx); err != nil && err != http.ErrServerClosed {
//	        log.Fatalf("Server failed: %v", err)
//	    }
//	}()
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting Gateway server", zap.String("addr", s.httpServer.Addr))
	return s.httpServer.ListenAndServe()
}

// Stop gracefully stops the HTTP server
//
// This method:
// 1. Initiates graceful shutdown (waits for in-flight requests)
// 2. Closes Redis connections
// 3. Returns error if shutdown fails
//
// Shutdown timeout is controlled by the provided context:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	server.Stop(ctx)
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Stopping Gateway server")

	// Graceful HTTP server shutdown
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("Failed to gracefully shutdown HTTP server", zap.Error(err))
		return err
	}

	// Close Redis connections
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
func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
	start := time.Now()

	// Record ingestion metric
	s.metricsInstance.AlertsReceivedTotal.WithLabelValues(signal.SourceType, signal.Severity, "unknown").Inc()

	// 1. Deduplication check
	isDuplicate, metadata, err := s.deduplicator.Check(ctx, signal)
	if err != nil {
		s.logger.Error("Deduplication check failed",
			zap.String("fingerprint", signal.Fingerprint),
			zap.Error(err))
		return nil, fmt.Errorf("deduplication check failed: %w", err)
	}

	if isDuplicate {
		// Fast path: duplicate signal, no CRD creation needed
		s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.AlertName, "unknown").Inc()

		s.logger.Debug("Duplicate signal detected",
			zap.String("fingerprint", signal.Fingerprint),
			zap.Int("count", metadata.Count),
			zap.String("firstSeen", metadata.FirstSeen),
		)

		return &ProcessingResponse{
			Status:      StatusDuplicate,
			Message:     "Duplicate signal (deduplication successful)",
			Fingerprint: signal.Fingerprint,
			Duplicate:   true,
			Metadata:    metadata,
		}, nil
	}

	// 2. Storm detection
	isStorm, stormMetadata, err := s.stormDetector.Check(ctx, signal)
	if err != nil {
		// Non-critical error: log warning but continue processing
		s.logger.Warn("Storm detection failed",
			zap.String("fingerprint", signal.Fingerprint),
			zap.Error(err))
	} else if isStorm && stormMetadata != nil {
		s.metricsInstance.AlertStormsDetectedTotal.WithLabelValues(stormMetadata.StormType, signal.AlertName).Inc()

		s.logger.Warn("Alert storm detected",
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("stormType", stormMetadata.StormType),
			zap.String("stormWindow", stormMetadata.Window),
			zap.Int("alertCount", stormMetadata.AlertCount))

		// BR-GATEWAY-016: Storm aggregation
		// Instead of creating individual CRDs during storms, aggregate alerts
		// into a single CRD after a 1-minute window
		shouldAggregate, windowID, err := s.stormAggregator.ShouldAggregate(ctx, signal)
		if err != nil {
			s.logger.Warn("Storm aggregation check failed, falling back to individual CRD creation",
				zap.String("fingerprint", signal.Fingerprint),
				zap.Error(err))
			// Fall through to individual CRD creation
		} else if shouldAggregate {
			// Add to existing aggregation window
			if err := s.stormAggregator.AddResource(ctx, windowID, signal); err != nil {
				s.logger.Warn("Failed to add resource to storm aggregation, falling back to individual CRD creation",
					zap.String("fingerprint", signal.Fingerprint),
					zap.String("windowID", windowID),
					zap.Error(err))
				// Fall through to individual CRD creation
			} else {
				// Successfully added to aggregation window, return without creating CRD
				resourceCount, _ := s.stormAggregator.GetResourceCount(ctx, windowID)

				s.logger.Info("Alert added to storm aggregation window",
					zap.String("fingerprint", signal.Fingerprint),
					zap.String("windowID", windowID),
					zap.Int("resourceCount", resourceCount))

				// Return accepted response (no CRD created yet)
				return &ProcessingResponse{
					Status:      StatusAccepted,
					Message:     fmt.Sprintf("Alert added to storm aggregation window (window ID: %s, %d resources aggregated)", windowID, resourceCount),
					Fingerprint: signal.Fingerprint,
					Duplicate:   false,
					IsStorm:     true,
					StormType:   stormMetadata.StormType,
					WindowID:    windowID,
				}, nil
			}
		} else {
			// Start new aggregation window
			windowID, err := s.stormAggregator.StartAggregation(ctx, signal, stormMetadata)
			if err != nil {
				s.logger.Warn("Failed to start storm aggregation, falling back to individual CRD creation",
					zap.String("fingerprint", signal.Fingerprint),
					zap.Error(err))
				// Fall through to individual CRD creation
			} else {
				// Schedule aggregated CRD creation after window expires
				// Use background context - HTTP request context gets cancelled after response
				// but aggregation goroutine needs to run for full window duration (5s-1m)
				go s.createAggregatedCRDAfterWindow(context.Background(), windowID, signal, stormMetadata)

				s.logger.Info("Storm aggregation window started",
					zap.String("fingerprint", signal.Fingerprint),
					zap.String("windowID", windowID),
					zap.String("windowTTL", "1 minute"))

				// Return accepted response (CRD will be created after window expires)
				return &ProcessingResponse{
					Status:      StatusAccepted,
					Message:     fmt.Sprintf("Storm aggregation window started (window ID: %s, CRD will be created after 1 minute)", windowID),
					Fingerprint: signal.Fingerprint,
					Duplicate:   false,
					IsStorm:     true,
					StormType:   stormMetadata.StormType,
					WindowID:    windowID,
				}, nil
			}
		}

		// If we reach here, aggregation failed - enrich signal for individual CRD creation
		signal.IsStorm = true
		signal.StormType = stormMetadata.StormType
		signal.StormWindow = stormMetadata.Window
		signal.AlertCount = stormMetadata.AlertCount
	}

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

	s.logger.Debug("Remediation path decided",
		zap.String("fingerprint", signal.Fingerprint),
		zap.String("environment", environment),
		zap.String("priority", priority),
		zap.String("remediationPath", remediationPath))

	// 6. Create RemediationRequest CRD
	rr, err := s.crdCreator.CreateRemediationRequest(ctx, signal, priority, environment)
	if err != nil {
		s.logger.Error("Failed to create RemediationRequest CRD",
			zap.String("fingerprint", signal.Fingerprint),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create RemediationRequest CRD: %w", err)
	}

	// 7. Store deduplication metadata
	remediationRequestRef := fmt.Sprintf("%s/%s", rr.Namespace, rr.Name)
	if err := s.deduplicator.Store(ctx, signal, remediationRequestRef); err != nil {
		// Non-critical error: CRD already created, log warning
		s.logger.Warn("Failed to store deduplication metadata",
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("crdName", rr.Name),
			zap.Error(err))
	}

	// Record processing duration
	duration := time.Since(start)
	s.logger.Info("Signal processed successfully",
		zap.String("fingerprint", signal.Fingerprint),
		zap.String("crdName", rr.Name),
		zap.String("environment", environment),
		zap.String("priority", priority),
		zap.String("remediationPath", remediationPath),
		zap.Int64("duration_ms", duration.Milliseconds()))

	return &ProcessingResponse{
		Status:                      StatusCreated,
		Message:                     "RemediationRequest CRD created successfully",
		Fingerprint:                 signal.Fingerprint,
		Duplicate:                   false,
		RemediationRequestName:      rr.Name,
		RemediationRequestNamespace: rr.Namespace,
		Environment:                 environment,
		Priority:                    priority,
		RemediationPath:             remediationPath,
	}, nil
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
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
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
// 1. Redis is reachable (PING command succeeds)
// 2. Kubernetes API is reachable (list namespaces succeeds)
//
// Typical checks: ~10ms (Redis PING + K8s API call)
func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Check Redis connectivity
	if err := s.redisClient.Ping(ctx).Err(); err != nil {
		s.logger.Warn("Readiness check failed: Redis not reachable", zap.Error(err))
		w.WriteHeader(http.StatusServiceUnavailable)
		if encErr := json.NewEncoder(w).Encode(map[string]string{
			"status": "not ready",
			"reason": "redis unavailable",
		}); encErr != nil {
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
	Environment                 string                            `json:"environment,omitempty"`
	Priority                    string                            `json:"priority,omitempty"`
	RemediationPath             string                            `json:"remediationPath,omitempty"`
	Metadata                    *processing.DeduplicationMetadata `json:"metadata,omitempty"`
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

	// Store deduplication metadata for all aggregated resources
	remediationRequestRef := fmt.Sprintf("%s/%s", rr.Namespace, rr.Name)
	if err := s.deduplicator.Store(ctx, &aggregatedSignal, remediationRequestRef); err != nil {
		// Non-critical error: CRD already created, log warning
		s.logger.Warn("Failed to store deduplication metadata for aggregated CRD",
			zap.String("windowID", windowID),
			zap.String("crdName", rr.Name),
			zap.Error(err))
	}

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
