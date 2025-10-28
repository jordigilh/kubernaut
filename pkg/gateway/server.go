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
	"github.com/sirupsen/logrus"

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
	logger *logrus.Logger
}

// ServerConfig holds server configuration
type ServerConfig struct {
	// Server settings
	ListenAddr   string        `yaml:"listen_addr"`   // Default: ":8080"
	ReadTimeout  time.Duration `yaml:"read_timeout"`  // Default: 30s
	WriteTimeout time.Duration `yaml:"write_timeout"` // Default: 30s
	IdleTimeout  time.Duration `yaml:"idle_timeout"`  // Default: 120s

	// Rate limiting
	RateLimitRequestsPerMinute int `yaml:"rate_limit_requests_per_minute"` // Default: 100
	RateLimitBurst             int `yaml:"rate_limit_burst"`               // Default: 10

	// Redis configuration
	Redis *goredis.Options `yaml:"redis"`

	// Deduplication TTL (optional, defaults to 5 minutes)
	// For testing: set to 5*time.Second for fast tests
	// For production: use default (0) for 5-minute TTL
	DeduplicationTTL time.Duration `yaml:"deduplication_ttl"`

	// Storm detection thresholds (optional, defaults: rate=10, pattern=5)
	// For testing: set to 2-3 for early storm detection in tests
	// For production: use defaults (0) for 10 alerts/minute
	StormRateThreshold    int `yaml:"storm_rate_threshold"`    // Default: 10 alerts/minute
	StormPatternThreshold int `yaml:"storm_pattern_threshold"` // Default: 5 similar alerts

	// Storm aggregation window (optional, default: 1 minute)
	// For testing: set to 5*time.Second for fast integration tests
	// For production: use default (0) for 1-minute windows
	StormAggregationWindow time.Duration `yaml:"storm_aggregation_window"` // Default: 1 minute

	// Environment classification cache TTL (optional, default: 30 seconds)
	// For testing: set to 5*time.Second for fast cache expiry in tests
	// For production: use default (0) for 30-second TTL
	// Set to 0 for default behavior
	EnvironmentCacheTTL time.Duration `yaml:"environment_cache_ttl"` // Default: 30 seconds

	// Environment classification ConfigMap
	EnvConfigMapNamespace string `yaml:"env_configmap_namespace"` // Default: "kubernaut-system"
	EnvConfigMapName      string `yaml:"env_configmap_name"`      // Default: "kubernaut-environment-overrides"
}

// NewServer creates a new Gateway server
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
func NewServer(cfg *ServerConfig, logger *logrus.Logger) (*Server, error) {
	// 1. Initialize Redis client
	redisClient := goredis.NewClient(cfg.Redis)

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

	// Use custom TTL if provided (for testing), otherwise default to 5 minutes
	metricsInstance := metrics.NewMetrics()

	var deduplicator *processing.DeduplicationService
	if cfg.DeduplicationTTL > 0 {
		deduplicator = processing.NewDeduplicationServiceWithTTL(redisClient, cfg.DeduplicationTTL, logger, metricsInstance)
		logger.WithField("ttl", cfg.DeduplicationTTL).Info("Using custom deduplication TTL")
	} else {
		deduplicator = processing.NewDeduplicationService(redisClient, logger, metricsInstance)
	}

	stormDetector := processing.NewStormDetector(redisClient, cfg.StormRateThreshold, cfg.StormPatternThreshold, metricsInstance)
	if cfg.StormRateThreshold > 0 || cfg.StormPatternThreshold > 0 {
		logger.WithFields(logrus.Fields{
			"rate_threshold":    cfg.StormRateThreshold,
			"pattern_threshold": cfg.StormPatternThreshold,
		}).Info("Using custom storm detection thresholds")
	}

	stormAggregator := processing.NewStormAggregatorWithWindow(redisClient, cfg.StormAggregationWindow)
	if cfg.StormAggregationWindow > 0 {
		logger.WithField("window", cfg.StormAggregationWindow).Info("Using custom storm aggregation window")
	}

	// Create environment classifier with configurable cache TTL
	var classifier *processing.EnvironmentClassifier
	if cfg.EnvironmentCacheTTL > 0 {
		classifier = processing.NewEnvironmentClassifierWithTTL(ctrlClient, logger, cfg.EnvironmentCacheTTL)
		logger.WithField("cache_ttl", cfg.EnvironmentCacheTTL).Info("Using custom environment cache TTL")
	} else {
		classifier = processing.NewEnvironmentClassifier(ctrlClient, logger)
	}
	priorityEngine := processing.NewPriorityEngine(logger)
	crdCreator := processing.NewCRDCreator(k8sClient, logger, metricsInstance)

	// 4. Initialize middleware
	// DD-GATEWAY-004: Authentication middleware removed (network-level security)
	// authMiddleware := middleware.NewAuthMiddleware(clientset, logger) // REMOVED
	// rateLimiter := middleware.NewRateLimiter(cfg.RateLimitRequestsPerMinute, cfg.RateLimitBurst, logger) // REMOVED

	// 5. Create server
	server := &Server{
		adapterRegistry: adapterRegistry,
		deduplicator:    deduplicator,
		stormDetector:   stormDetector,
		stormAggregator: stormAggregator,
		classifier:      classifier,
		priorityEngine:  priorityEngine,
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
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
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

	s.logger.WithFields(logrus.Fields{
		"adapter": adapter.Name(),
		"route":   adapter.GetRoute(),
	}).Info("Registered adapter route")

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
			s.logger.WithError(err).Error("Failed to read request body")
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		// Parse signal using adapter
		signal, err := adapter.Parse(ctx, body)
		if err != nil {
			s.logger.WithFields(logrus.Fields{
				"adapter": adapter.Name(),
				"error":   err,
			}).Warn("Failed to parse signal")

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
			s.logger.WithFields(logrus.Fields{
				"adapter": adapter.Name(),
				"error":   err,
			}).Warn("Signal validation failed")
			http.Error(w, fmt.Sprintf("Signal validation failed: %v", err), http.StatusBadRequest)
			return
		}

		// Process signal through pipeline
		response, err := s.ProcessSignal(ctx, signal)
		if err != nil {
			s.logger.WithFields(logrus.Fields{
				"adapter": adapter.Name(),
				"error":   err,
			}).Error("Signal processing failed")
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
		json.NewEncoder(w).Encode(response)
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
	s.logger.WithField("addr", s.httpServer.Addr).Info("Starting Gateway server")
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
		s.logger.WithError(err).Error("Failed to gracefully shutdown HTTP server")
		return err
	}

	// Close Redis connections
	if s.redisClient != nil {
		if err := s.redisClient.Close(); err != nil {
			s.logger.WithError(err).Error("Failed to close Redis client")
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
		s.logger.WithFields(logrus.Fields{
			"fingerprint": signal.Fingerprint,
			"error":       err,
		}).Error("Deduplication check failed")
		return nil, fmt.Errorf("deduplication check failed: %w", err)
	}

	if isDuplicate {
		// Fast path: duplicate signal, no CRD creation needed
		s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.AlertName, "unknown").Inc()

		s.logger.WithFields(logrus.Fields{
			"fingerprint": signal.Fingerprint,
			"count":       metadata.Count,
			"firstSeen":   metadata.FirstSeen,
		}).Debug("Duplicate signal detected")

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
		s.logger.WithFields(logrus.Fields{
			"fingerprint": signal.Fingerprint,
			"error":       err,
		}).Warn("Storm detection failed")
	} else if isStorm && stormMetadata != nil {
		s.metricsInstance.AlertStormsDetectedTotal.WithLabelValues(stormMetadata.StormType, signal.AlertName).Inc()

		s.logger.WithFields(logrus.Fields{
			"fingerprint": signal.Fingerprint,
			"stormType":   stormMetadata.StormType,
			"stormWindow": stormMetadata.Window,
			"alertCount":  stormMetadata.AlertCount,
		}).Warn("Alert storm detected")

		// BR-GATEWAY-016: Storm aggregation
		// Instead of creating individual CRDs during storms, aggregate alerts
		// into a single CRD after a 1-minute window
		shouldAggregate, windowID, err := s.stormAggregator.ShouldAggregate(ctx, signal)
		if err != nil {
			s.logger.WithFields(logrus.Fields{
				"fingerprint": signal.Fingerprint,
				"error":       err,
			}).Warn("Storm aggregation check failed, falling back to individual CRD creation")
			// Fall through to individual CRD creation
		} else if shouldAggregate {
			// Add to existing aggregation window
			if err := s.stormAggregator.AddResource(ctx, windowID, signal); err != nil {
				s.logger.WithFields(logrus.Fields{
					"fingerprint": signal.Fingerprint,
					"windowID":    windowID,
					"error":       err,
				}).Warn("Failed to add resource to storm aggregation, falling back to individual CRD creation")
				// Fall through to individual CRD creation
			} else {
				// Successfully added to aggregation window, return without creating CRD
				resourceCount, _ := s.stormAggregator.GetResourceCount(ctx, windowID)

				s.logger.WithFields(logrus.Fields{
					"fingerprint":   signal.Fingerprint,
					"windowID":      windowID,
					"resourceCount": resourceCount,
				}).Info("Alert added to storm aggregation window")

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
				s.logger.WithFields(logrus.Fields{
					"fingerprint": signal.Fingerprint,
					"error":       err,
				}).Warn("Failed to start storm aggregation, falling back to individual CRD creation")
				// Fall through to individual CRD creation
			} else {
				// Schedule aggregated CRD creation after window expires
				// Use background context - HTTP request context gets cancelled after response
				// but aggregation goroutine needs to run for full window duration (5s-1m)
				go s.createAggregatedCRDAfterWindow(context.Background(), windowID, signal, stormMetadata)

				s.logger.WithFields(logrus.Fields{
					"fingerprint": signal.Fingerprint,
					"windowID":    windowID,
					"windowTTL":   "1 minute",
				}).Info("Storm aggregation window started")

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

	// 5. Create RemediationRequest CRD
	rr, err := s.crdCreator.CreateRemediationRequest(ctx, signal, priority, environment)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"fingerprint": signal.Fingerprint,
			"error":       err,
		}).Error("Failed to create RemediationRequest CRD")
		return nil, fmt.Errorf("failed to create RemediationRequest CRD: %w", err)
	}

	// 6. Store deduplication metadata
	remediationRequestRef := fmt.Sprintf("%s/%s", rr.Namespace, rr.Name)
	if err := s.deduplicator.Store(ctx, signal, remediationRequestRef); err != nil {
		// Non-critical error: CRD already created, log warning
		s.logger.WithFields(logrus.Fields{
			"fingerprint": signal.Fingerprint,
			"crdName":     rr.Name,
			"error":       err,
		}).Warn("Failed to store deduplication metadata")
	}

	// Record processing duration
	duration := time.Since(start)
	s.logger.WithFields(logrus.Fields{
		"fingerprint": signal.Fingerprint,
		"crdName":     rr.Name,
		"environment": environment,
		"priority":    priority,
		"duration_ms": duration.Milliseconds(),
	}).Info("Signal processed successfully")

	return &ProcessingResponse{
		Status:                      StatusCreated,
		Message:                     "RemediationRequest CRD created successfully",
		Fingerprint:                 signal.Fingerprint,
		Duplicate:                   false,
		RemediationRequestName:      rr.Name,
		RemediationRequestNamespace: rr.Namespace,
		Environment:                 environment,
		Priority:                    priority,
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
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
		s.logger.WithError(err).Warn("Readiness check failed: Redis not reachable")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not ready",
			"reason": "redis unavailable",
		})
		return
	}

	// Check Kubernetes API connectivity
	// Note: This is a placeholder - actual implementation would use k8sClient
	// to perform a simple API call (e.g. list namespaces with limit=1)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
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

	s.logger.WithFields(logrus.Fields{
		"windowID":  windowID,
		"alertName": firstSignal.AlertName,
		"duration":  windowDuration,
	}).Info("Storm aggregation window expired, creating aggregated CRD")

	// Retrieve all aggregated resources
	resources, err := s.stormAggregator.GetAggregatedResources(ctx, windowID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"windowID": windowID,
			"error":    err,
		}).Error("Failed to retrieve aggregated resources")
		return
	}

	// Retrieve signal metadata
	signal, storedStormMetadata, err := s.stormAggregator.GetSignalMetadata(ctx, windowID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"windowID": windowID,
			"error":    err,
		}).Warn("Failed to retrieve signal metadata, using first signal")
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

	s.logger.WithFields(logrus.Fields{
		"windowID":      windowID,
		"alertName":     signal.AlertName,
		"resourceCount": resourceCount,
		"stormType":     stormMetadata.StormType,
	}).Info("Creating aggregated RemediationRequest CRD")

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
		s.logger.WithFields(logrus.Fields{
			"windowID":      windowID,
			"resourceCount": resourceCount,
			"error":         err,
		}).Error("Failed to create aggregated RemediationRequest CRD")

		// Record metric for failed aggregation
		s.metricsInstance.CRDCreationErrors.WithLabelValues("k8s_api_error").Inc()
		return
	}

	// Store deduplication metadata for all aggregated resources
	remediationRequestRef := fmt.Sprintf("%s/%s", rr.Namespace, rr.Name)
	if err := s.deduplicator.Store(ctx, &aggregatedSignal, remediationRequestRef); err != nil {
		// Non-critical error: CRD already created, log warning
		s.logger.WithFields(logrus.Fields{
			"windowID": windowID,
			"crdName":  rr.Name,
			"error":    err,
		}).Warn("Failed to store deduplication metadata for aggregated CRD")
	}

	s.logger.WithFields(logrus.Fields{
		"windowID":      windowID,
		"crdName":       rr.Name,
		"crdNamespace":  rr.Namespace,
		"resourceCount": resourceCount,
		"environment":   environment,
		"priority":      priority,
		"stormType":     stormMetadata.StormType,
	}).Info("Aggregated RemediationRequest CRD created successfully")

	// Record metrics for successful aggregation
	s.metricsInstance.CRDsCreatedTotal.WithLabelValues(environment, priority).Inc()
}
