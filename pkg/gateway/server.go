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
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sony/gobreaker" // BR-GATEWAY-093: Circuit breaker detection

	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"

	"github.com/jordigilh/kubernaut/pkg/audit"                       // DD-AUDIT-003: Audit integration
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client" // Ogen generated audit types
	sharedaudit "github.com/jordigilh/kubernaut/pkg/shared/audit"    // BR-AUDIT-005 Gap #7: Standardized error details
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"              // ADR-052 Addendum 001: Exponential backoff with jitter

	coordinationv1 "k8s.io/api/coordination/v1" // BR-GATEWAY-190: Lease resources for distributed locking
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
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

// Gateway Audit Event Type Constants (from OpenAPI spec)
// See: api/openapi/data-storage-v1.yaml - GatewayAuditPayload.event_type enum
const (
	EventTypeSignalReceived     = "gateway.signal.received"     // BR-GATEWAY-190: New signal received, RR created
	EventTypeSignalDeduplicated = "gateway.signal.deduplicated" // BR-GATEWAY-191: Duplicate signal detected
	EventTypeCRDCreated         = "gateway.crd.created"         // DD-AUDIT-003: RR CRD successfully created
	EventTypeCRDFailed          = "gateway.crd.failed"          // DD-AUDIT-003: RR CRD creation failed
	CategoryGateway             = "gateway"                     // Service-level category per ADR-034
	// EventCategoryGateway is an alias for consistency with other services
	EventCategoryGateway        = CategoryGateway
)

// Gateway Audit Event Action Constants (ADR-034 event_action field)
// These are past-tense verbs describing what action was performed
const (
	ActionReceived     = "received"     // Signal was received and processed
	ActionDeduplicated = "deduplicated" // Signal was deduplicated to existing RR
	ActionCreated      = "created"      // RemediationRequest CRD was created
	ActionFailed       = "failed"       // Operation failed
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

	// Configuration
	config *config.ServerConfig // ADR-030: Service configuration (needed for middleware setup)

	// Core processing components
	adapterRegistry *adapters.AdapterRegistry
	// DD-GATEWAY-011: CRDUpdater REMOVED - replaced by StatusUpdater
	// Old: crdUpdater updated Spec.Deduplication (WRONG!)
	// New: statusUpdater updates Status.Deduplication (CORRECT!)
	// DD-GATEWAY-011 + DD-GATEWAY-012: Status-based deduplication and storm aggregation
	// Redis DEPRECATED - all state now in K8s RR status
	statusUpdater *processing.StatusUpdater                  // Updates RR status.deduplication and status.stormAggregation
	phaseChecker  *processing.PhaseBasedDeduplicationChecker // Phase-based deduplication logic
	crdCreator    *processing.CRDCreator
	lockManager   *processing.DistributedLockManager // BR-GATEWAY-190: K8s Lease-based distributed locking

	// Infrastructure clients
	// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free
	k8sClient  *k8s.Client
	ctrlClient client.Client

	// DD-AUDIT-003: Audit store for async buffered audit event emission
	// Gateway is P0 service - MUST emit audit events per DD-AUDIT-003
	auditStore audit.AuditStore // nil if Data Storage URL not configured (graceful degradation)

	// Metrics
	metricsInstance *metrics.Metrics

	// Logger (DD-005: Unified logr.Logger interface)
	logger logr.Logger

	// BR-GATEWAY-185 v1.1: Cache for field-indexed queries on spec.signalFingerprint
	k8sCache    cache.Cache        // Kubernetes cache with field index (nil for test clients)
	cacheCancel context.CancelFunc // Cancel function to stop cache on shutdown

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
// If metricsInstance is nil, automatically creates a new metrics instance with
// the default Prometheus registry (production mode).
//
// NewServerWithK8sClient creates a Gateway server with an existing K8s client (for testing)
// This ensures the Gateway uses the same K8s client as the test, avoiding cache synchronization issues
// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free, K8s-native service
// DD-STATUS-001: For tests, ctrlClient serves as both client and apiReader (direct API access)
func NewServerWithK8sClient(cfg *config.ServerConfig, logger logr.Logger, metricsInstance *metrics.Metrics, ctrlClient client.Client) (*Server, error) {
	// Use provided Kubernetes client (shared with test)
	k8sClient := k8s.NewClient(ctrlClient)

	// DD-STATUS-001: Use ctrlClient as apiReader for cache-bypassed reads
	// In test environments, this provides direct K8s API access
	return createServerWithClients(cfg, logger, metricsInstance, ctrlClient, ctrlClient, k8sClient)
}

// NewServerForTesting creates a Gateway server with injected dependencies for testing.
// This constructor allows injecting a mock audit store for unit tests.
//
// USAGE: Unit tests only - allows testing audit failure scenarios
// PRODUCTION: Use NewServer() or NewServerWithK8sClient() instead
func NewServerForTesting(cfg *config.ServerConfig, logger logr.Logger, metricsInstance *metrics.Metrics, ctrlClient client.Client, auditStore audit.AuditStore) (*Server, error) {
	// Use provided Kubernetes client
	k8sClient := k8s.NewClient(ctrlClient)

	// Initialize adapter registry
	adapterRegistry := adapters.NewAdapterRegistry(logger)

	// Create phaseChecker (for deduplication)
	// DD-GATEWAY-011: Use ctrlClient as apiReader for deduplication (test environment uses direct API access)
	// This ensures concurrent requests see each other's CRD creations immediately (GW-DEDUP-002 fix)
	phaseChecker := processing.NewPhaseBasedDeduplicationChecker(ctrlClient)

	// Create statusUpdater
	statusUpdater := processing.NewStatusUpdater(ctrlClient, ctrlClient)

	// Create CRD creator
	fallbackNamespace := "default"
	if cfg.Processing.CRD.FallbackNamespace != "" {
		fallbackNamespace = cfg.Processing.CRD.FallbackNamespace
	}
	crdCreator := processing.NewCRDCreator(k8sClient, logger, metricsInstance, fallbackNamespace, &cfg.Processing.Retry)

	// BR-GATEWAY-190: Initialize distributed lock manager for multi-replica safety (test environment)
	// Uses ctrlClient as apiReader (test clients don't have separate cache/apiReader)
	var lockManager *processing.DistributedLockManager
	podName := os.Getenv("POD_NAME")
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "default" // Test environment default
	}
	if podName != "" {
		lockManager = processing.NewDistributedLockManager(ctrlClient, namespace, podName)
	}

	// Create server
	server := &Server{
		config:          cfg,
		adapterRegistry: adapterRegistry,
		statusUpdater:   statusUpdater,
		phaseChecker:    phaseChecker,
		crdCreator:      crdCreator,
		k8sClient:       k8sClient,
		ctrlClient:      ctrlClient,
		lockManager:     lockManager, // BR-GATEWAY-190: Multi-replica deduplication safety
		auditStore:      auditStore,  // Injected for testing
		metricsInstance: metricsInstance,
		logger:          logger,
	}

	// Setup HTTP server with routes
	router := server.setupRoutes()
	server.router = router
	handler := server.wrapWithMiddleware(router)

	server.httpServer = &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	return server, nil
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
	_ = corev1.AddToScheme(scheme)         // Add core types (Namespace, Pod, etc.)
	_ = coordinationv1.AddToScheme(scheme) // BR-GATEWAY-190: Add Lease type for distributed locking

	// ========================================
	// BR-GATEWAY-185 v1.1: Create cached client with field index
	// Use spec.signalFingerprint (immutable, 64-char SHA256) instead of truncated labels
	// ========================================

	// Create cache for efficient queries
	k8sCache, err := cache.New(kubeConfig, cache.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes cache: %w", err)
	}

	// Add field index for spec.signalFingerprint (BR-GATEWAY-185 v1.1)
	// This enables O(1) lookup by fingerprint without label truncation
	ctx, cancel := context.WithCancel(context.Background())
	if err := k8sCache.IndexField(ctx, &remediationv1alpha1.RemediationRequest{},
		"spec.signalFingerprint",
		func(obj client.Object) []string {
			rr := obj.(*remediationv1alpha1.RemediationRequest)
			return []string{rr.Spec.SignalFingerprint}
		}); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create fingerprint field index: %w", err)
	}

	// Start cache in background
	go func() {
		if err := k8sCache.Start(ctx); err != nil {
			logger.Error(err, "BR-GATEWAY-185: Cache stopped unexpectedly")
		}
	}()

	// Wait for cache sync (timeout after 30s)
	syncCtx, syncCancel := context.WithTimeout(ctx, 30*time.Second)
	defer syncCancel()
	if !k8sCache.WaitForCacheSync(syncCtx) {
		cancel()
		return nil, fmt.Errorf("failed to sync Kubernetes cache (timeout)")
	}
	logger.Info("BR-GATEWAY-185: Kubernetes cache synced with spec.signalFingerprint index")

	// Create client backed by cache (reads go through cache, writes go to API)
	ctrlClient, err := client.New(kubeConfig, client.Options{
		Scheme: scheme,
		Cache: &client.CacheOptions{
			Reader: k8sCache,
		},
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create controller-runtime client: %w", err)
	}

	// DD-STATUS-001: Create UNCACHED client for fresh API reads (adopted from RO pattern)
	// This is critical for reading CRDs immediately after creation (bypasses cache sync delays)
	// RO uses mgr.GetAPIReader() which returns an uncached client - we replicate that here
	apiReader, err := client.New(kubeConfig, client.Options{
		Scheme: scheme,
		// NO Cache option = direct API server reads (no cache)
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create uncached API reader: %w", err)
	}

	// k8s client wrapper (for CRD operations)
	k8sClient := k8s.NewClient(ctrlClient)

	// DD-STATUS-001: Pass separate cached client and uncached apiReader
	// ctrlClient: Cached reads/writes for normal operations
	// apiReader: Uncached reads for fresh data (status refetch after CRD creation)
	server, err := createServerWithClients(cfg, logger, metricsInstance, ctrlClient, apiReader, k8sClient)
	if err != nil {
		cancel()
		return nil, err
	}

	// Store cache and cancel function for cleanup
	server.k8sCache = k8sCache
	server.cacheCancel = cancel

	return server, nil
}

// createServerWithClients is the common server creation logic
// DD-005: Uses logr.Logger for unified logging interface
// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free, K8s-native service
// DD-AUDIT-003: Audit store initialization for P0 service compliance
// DD-STATUS-001: apiReader parameter added for cache-bypassed status refetch (adopted from RO)
// BR-GATEWAY-190: apiReader is client.Client (not just client.Reader) for distributed locking Create/Update/Delete operations
func createServerWithClients(cfg *config.ServerConfig, logger logr.Logger, metricsInstance *metrics.Metrics, ctrlClient client.Client, apiReader client.Client, k8sClient *k8s.Client) (*Server, error) {
	// Metrics are mandatory for observability
	// If nil, create a new metrics instance with default registry (production mode)
	if metricsInstance == nil {
		metricsInstance = metrics.NewMetrics()
	}

	// Initialize processing pipeline components
	adapterRegistry := adapters.NewAdapterRegistry(logger)

	// DD-GATEWAY-011: CRDUpdater REMOVED - replaced by StatusUpdater
	// Old CRDUpdater updated Spec.Deduplication (incorrect per DD-GATEWAY-011)
	// StatusUpdater now handles status updates (status.deduplication)

	// DD-GATEWAY-015 / BR-GATEWAY-093: Circuit Breaker Integration (TDD GREEN COMPLETE)
	// ========================================
	// Circuit breaker protects Gateway from K8s API cascading failures
	//
	// Implementation:
	//   - ClientWithCircuitBreaker wraps k8sClient with fail-fast protection
	//   - CRDCreator uses k8s.ClientInterface (supports both Client and ClientWithCircuitBreaker)
	//   - Circuit breaker metrics: gateway_circuit_breaker_state, gateway_circuit_breaker_operations_total
	//
	// Behavior:
	//   - Closed (0): Normal operation, all requests pass through
	//   - Open (2): K8s API degraded, requests fail-fast (<10ms) with gobreaker.ErrOpenState
	//   - Half-Open (1): Testing recovery, limited requests allowed
	//
	// Configuration:
	//   - Threshold: 50% failure rate over 10 requests
	//   - Timeout: 30s (transitions to half-open after 30s in open state)
	//   - Max Requests: 3 (half-open state test requests)
	//
	// Business Requirements:
	//   - BR-GATEWAY-093-A: Fail-fast when K8s API unavailable
	//   - BR-GATEWAY-093-B: Prevent cascade failures during K8s API overload
	//   - BR-GATEWAY-093-C: Observable metrics for SRE response
	//
	// Design Decision: DD-GATEWAY-015 (K8s API Circuit Breaker Implementation)
	// ========================================
	cbClient := k8s.NewClientWithCircuitBreaker(k8sClient, metricsInstance)

	crdCreator := processing.NewCRDCreator(cbClient, logger, metricsInstance, cfg.Processing.CRD.FallbackNamespace, &cfg.Processing.Retry)

	// DD-GATEWAY-011: Status-based deduplication
	// All state in K8s RR status - Redis fully deprecated
	// DD-STATUS-001: Pass apiReader for cache-bypassed status refetch (adopted from RO pattern)
	statusUpdater := processing.NewStatusUpdater(ctrlClient, apiReader)
	// DD-GATEWAY-011: Use apiReader for deduplication to eliminate race conditions (cache-bypassed reads)
	// This ensures concurrent requests see each other's CRD creations immediately (GW-DEDUP-002 fix)
	phaseChecker := processing.NewPhaseBasedDeduplicationChecker(apiReader)

	// BR-GATEWAY-190: Initialize distributed lock manager for multi-replica safety
	// Uses K8s Lease resources for distributed locking (no external dependencies)
	//
	// CRITICAL: Uses apiReader (non-cached client) for immediate consistency
	// WHY: Cached client has 5-50ms sync delay → race condition → duplicate locks
	// IMPACT: 3-24 API req/sec (production load) - acceptable per impact analysis
	// See: docs/services/stateless/gateway-service/GW_API_SERVER_IMPACT_ANALYSIS_DISTRIBUTED_LOCKING_JAN18_2026.md
	var lockManager *processing.DistributedLockManager
	podName := os.Getenv("POD_NAME")
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "kubernaut-system"
	}
	if podName != "" {
		lockManager = processing.NewDistributedLockManager(apiReader, namespace, podName)
	}

	// DD-AUDIT-003: Initialize audit store for P0 service compliance
	// Gateway MUST emit audit events per DD-AUDIT-003: Service Audit Trace Requirements
	// ADR-032 §1.5: "Every alert/signal processed (SignalProcessing, Gateway)"
	// ADR-032 §3: Gateway is P0 (Business-Critical) - MUST crash if audit unavailable
	var auditStore audit.AuditStore
	if cfg.Infrastructure.DataStorageURL != "" {
		// DD-API-001: Use OpenAPI generated client (not direct HTTP)
		dsClient, err := audit.NewOpenAPIClientAdapter(cfg.Infrastructure.DataStorageURL, 5*time.Second)
		if err != nil {
			// ADR-032 §2: No fallback/recovery allowed - crash on init failure
			return nil, fmt.Errorf("FATAL: failed to create Data Storage client - audit is MANDATORY per ADR-032 §1.5 (Gateway is P0 service): %w", err)
		}
		auditConfig := audit.RecommendedConfig("gateway") // 2x buffer for high-volume service

		auditStore, err = audit.NewBufferedStore(dsClient, auditConfig, "gateway", logger)
		if err != nil {
			// ADR-032 §2: No fallback/recovery allowed - crash on init failure
			return nil, fmt.Errorf("FATAL: failed to create audit store - audit is MANDATORY per ADR-032 §1.5 (Gateway is P0 service): %w", err)
		}
		logger.Info("DD-AUDIT-003: Audit store initialized for P0 compliance (ADR-032 §1.5)",
			"data_storage_url", cfg.Infrastructure.DataStorageURL,
			"buffer_size", auditConfig.BufferSize)
	} else {
		// ADR-032 §1.5: Data Storage URL is MANDATORY for P0 services (Gateway processes alerts/signals)
		// ADR-032 §3: Gateway is P0 (Business-Critical) - MUST crash if audit unavailable
		return nil, fmt.Errorf("FATAL: Data Storage URL not configured - audit is MANDATORY per ADR-032 §1.5 (Gateway is P0 service)")
	}

	// Create server (Redis-free)
	server := &Server{
		config:          cfg,
		adapterRegistry: adapterRegistry,
		// DD-GATEWAY-011: crdUpdater field removed (replaced by statusUpdater)
		statusUpdater:   statusUpdater,
		phaseChecker:    phaseChecker,
		crdCreator:      crdCreator,
		lockManager:     lockManager,
		k8sClient:       k8sClient,
		ctrlClient:      ctrlClient,
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

	// ADR-048-ADDENDUM-001: Concurrency limiting (defense-in-depth with Nginx/HAProxy)
	// Prevents per-pod overload with chi's built-in Throttle middleware
	// - Returns HTTP 503 Service Unavailable when limit exceeded
	// - Complements cluster-wide rate limiting at Ingress/Route layer
	// - Essential for E2E tests that bypass Ingress
	if s.config.Server.MaxConcurrentRequests > 0 {
		s.logger.Info("Concurrency throttling enabled",
			"max_concurrent_requests", s.config.Server.MaxConcurrentRequests,
			"authority", "ADR-048-ADDENDUM-001")
		r.Use(chimiddleware.Throttle(s.config.Server.MaxConcurrentRequests))
	}

	// Global middleware
	r.Use(chimiddleware.RequestID) // Chi's built-in request ID
	r.Use(chimiddleware.RealIP)    // Extract real IP from X-Forwarded-For

	// BR-GATEWAY-074, BR-GATEWAY-075: Security middleware
	// Timestamp validation prevents replay attacks by rejecting old/future timestamps
	r.Use(middleware.TimestampValidator(5 * time.Minute))

	// Security headers middleware (OWASP best practices)
	// Prevents: MIME sniffing, clickjacking, XSS attacks, enforces HTTPS
	r.Use(middleware.SecurityHeaders())

	// Request ID middleware for distributed tracing
	// Ensures X-Request-ID is present for correlation across services
	r.Use(middleware.RequestIDMiddleware(s.logger))

	// HTTP metrics middleware for observability
	// Records request counts and duration metrics
	r.Use(middleware.HTTPMetrics(s.metricsInstance))

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

	// DD-GATEWAY-012: Redis error handling REMOVED - Gateway is now Redis-free
	// Deduplication check failures are now K8s API errors (phaseChecker uses K8s client)

	// Kubernetes API errors → HTTP 500 Internal Server Error with details
	if strings.Contains(errMsg, "kubernetes") || strings.Contains(errMsg, "k8s") ||
		strings.Contains(errMsg, "failed to create RemediationRequest CRD") ||
		strings.Contains(errMsg, "deduplication check failed") || // DD-GATEWAY-012: Now a K8s API error
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
	if response.Status == StatusAccepted || response.Status == StatusDeduplicated || response.Duplicate {
		statusCode = http.StatusAccepted // HTTP 202 for storm aggregation and deduplication
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

	// BR-GATEWAY-185 v1.1: Stop K8s cache
	if s.cacheCancel != nil {
		s.cacheCancel()
		s.logger.Info("BR-GATEWAY-185: Kubernetes cache stopped")
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

	// BR-GATEWAY-190: Acquire distributed lock for multi-replica safety
	// DD-GATEWAY-013: K8s Lease-based distributed locking pattern
	// ADR-052 Addendum 001 (Jan 2026): Exponential backoff with jitter (anti-thundering herd)
	if s.lockManager != nil {
		const maxRetries = 10 // 10 retries = ~2.5s total wait with exponential backoff

		// Configure shared backoff with jitter (pkg/shared/backoff)
		// ADR-052 Addendum 001: Use production-proven backoff from Notification v3.1
		backoffConfig := backoff.Config{
			BasePeriod:    100 * time.Millisecond, // Start at 100ms (proven in production)
			MaxPeriod:     1 * time.Second,        // Cap at 1s (faster than 30s lease expiry)
			Multiplier:    2.0,                    // Standard exponential (100ms → 200ms → 400ms → 800ms)
			JitterPercent: 10,                     // ±10% jitter (prevents thundering herd)
		}

		// Iterative retry loop with exponential backoff (replaces unbounded recursion)
		// ADR-052 Addendum 001: Prevents stack overflow risk from recursive retry
		for attempt := int32(1); attempt <= maxRetries; attempt++ {
			acquired, err := s.lockManager.AcquireLock(ctx, signal.Fingerprint)
			if err != nil {
				return nil, fmt.Errorf("distributed lock acquisition failed: %w", err)
			}

			if acquired {
				// Lock acquired - exit retry loop and proceed with normal flow
				break
			}

			// Lock held by another Gateway pod
			logger.V(1).Info("Lock contention, retrying with exponential backoff",
				"attempt", attempt,
				"maxRetries", maxRetries,
				"fingerprint", signal.Fingerprint)

			// Check if we've exhausted all retries (early return for failure case)
			if attempt >= maxRetries {
				// Max retries exceeded - fail immediately
				return nil, fmt.Errorf("lock acquisition timeout after %d attempts (fingerprint: %s)",
					maxRetries, signal.Fingerprint)
			}

			// Exponential backoff with jitter (shared implementation)
			backoffDuration := backoffConfig.Calculate(attempt)
			logger.V(2).Info("Backing off before retry",
				"backoff", backoffDuration,
				"attempt", attempt,
				"fingerprint", signal.Fingerprint)

			time.Sleep(backoffDuration)

			// Retry deduplication check (other pod may have created RR by now)
			shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, signal.Namespace, signal.Fingerprint)
			if err != nil {
				return nil, fmt.Errorf("deduplication check failed after lock contention: %w", err)
			}

			if shouldDeduplicate && existingRR != nil {
				// BR-GATEWAY-190: Another pod created RR during lock contention
				// Handle deduplication and return early (no need to continue retry loop)
				return s.handleDuplicateSignal(ctx, signal, existingRR)
			}

			// Still no RR - continue to next retry attempt
		}

		// Lock acquired successfully - ensure it's released after operation
		defer func() {
			if err := s.lockManager.ReleaseLock(ctx, signal.Fingerprint); err != nil {
				logger.Error(err, "Failed to release distributed lock", "fingerprint", signal.Fingerprint)
			}
		}()
	}

	// 1. Deduplication check (DD-GATEWAY-011: K8s status-based, NOT Redis)
	// BR-GATEWAY-185: Redis deprecation - use PhaseBasedDeduplicationChecker
	shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, signal.Namespace, signal.Fingerprint)
	if err != nil {
		logger.Error(err, "Deduplication check failed",
			"fingerprint", signal.Fingerprint)
		return nil, fmt.Errorf("deduplication check failed: %w", err)
	}

	if shouldDeduplicate && existingRR != nil {
		// Update status.deduplication (DD-GATEWAY-011)
		// Must be synchronous - HTTP response includes occurrence count
		if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
			logger.Info("Failed to update deduplication status (DD-GATEWAY-011)",
				"error", err,
				"fingerprint", signal.Fingerprint,
				"rr", existingRR.Name)
		}

		// Get occurrence count for metrics and logging
		occurrenceCount := int32(1)
		if existingRR.Status.Deduplication != nil {
			occurrenceCount = existingRR.Status.Deduplication.OccurrenceCount
		}

		// Record metrics
		s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.AlertName).Inc()

		// BR-GATEWAY-069: Cache hit metric (deduplication detected)
		s.metricsInstance.DeduplicationCacheHitsTotal.Inc()

		// Note: DeduplicationRate gauge is calculated on-the-fly by custom collector
		// when /metrics endpoint is scraped (see metrics.DeduplicationRateCollector)

		logger.V(1).Info("Duplicate signal detected (K8s status-based)",
			"fingerprint", signal.Fingerprint,
			"existingRR", existingRR.Name,
			"phase", existingRR.Status.OverallPhase,
			"occurrenceCount", occurrenceCount)

		// DD-AUDIT-003: Emit audit event (BR-GATEWAY-191)
		// Fire-and-forget: audit failures don't affect business logic
		s.emitSignalDeduplicatedAudit(ctx, signal, existingRR.Name, existingRR.Namespace, occurrenceCount)

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

// Processing status constants (HTTP response body status field)
// Aligned with OpenAPI enum values for consistency (no backwards compatibility needed)
const (
	StatusCreated      = "created"   // RemediationRequest CRD created
	StatusDeduplicated = "duplicate" // Signal deduplicated to existing RR (matches OpenAPI enum)
	StatusAccepted     = "accepted"  // Alert accepted for storm aggregation (CRD will be created later)
)

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
			firstOccurrence = rr.Status.Deduplication.FirstSeenAt.Format(time.RFC3339)
		}
		if rr.Status.Deduplication.LastSeenAt != nil {
			lastOccurrence = rr.Status.Deduplication.LastSeenAt.Format(time.RFC3339)
		}
	}

	return &ProcessingResponse{
		Status:                      StatusDeduplicated,
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

// =============================================================================
// DD-AUDIT-003: Audit Event Emission (P0 Compliance)
// =============================================================================

// extractRRReconstructionFields sanitizes signal fields for audit event storage
//
// ========================================
// RR RECONSTRUCTION FIELD SANITIZATION (REFACTOR PHASE)
// BR-AUDIT-005: Ensure PostgreSQL JSONB compatibility
// ========================================
//
// WHY THIS HELPER?
// - ✅ Eliminates code duplication (used by signal.received AND signal.deduplicated)
// - ✅ PostgreSQL JSONB prefers empty maps over nil values
// - ✅ Graceful handling of synthetic signals without RawPayload
// - ✅ Consistent nil handling across all Gateway audit events
//
// RETURNS:
// - labels: non-nil map[string]string (empty map if nil)
// - annotations: non-nil map[string]string (empty map if nil)
// - originalPayload: interface{} (nil if signal.RawPayload is nil)
// ========================================
func extractRRReconstructionFields(signal *types.NormalizedSignal) (
	labels map[string]string,
	annotations map[string]string,
	originalPayload map[string]interface{},
) {
	// Gap #2: Signal labels (ensure non-nil for JSONB)
	labels = signal.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	// Gap #3: Signal annotations (ensure non-nil for JSONB)
	annotations = signal.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Gap #1: Original payload (nil OK for synthetic signals)
	if len(signal.RawPayload) > 0 {
		// Unmarshal json.RawMessage to map[string]interface{}
		if err := json.Unmarshal(signal.RawPayload, &originalPayload); err != nil {
			// If unmarshal fails, leave originalPayload nil (defensive)
			originalPayload = nil
		}
	}

	return labels, annotations, originalPayload
}

// emitSignalReceivedAudit emits 'gateway.signal.received' audit event (BR-GATEWAY-190)
// This is called when a NEW signal is received and RR is created
func (s *Server) emitSignalReceivedAudit(ctx context.Context, signal *types.NormalizedSignal, rrName, rrNamespace string) {
	if s.auditStore == nil {
		// ❌ CRITICAL: This should NEVER happen if init is fixed (ADR-032 §2)
		s.logger.Error(fmt.Errorf("AuditStore is nil"), "CRITICAL: Cannot record audit event (ADR-032 §1.5 violation)")
		return
	}

	// Use OpenAPI helper functions (DD-AUDIT-002 V2.0.1)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeSignalReceived)
	audit.SetEventCategory(event, CategoryGateway)
	audit.SetEventAction(event, ActionReceived)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "external", signal.SourceType) // e.g., "prometheus", "kubernetes"
	audit.SetResource(event, "Signal", signal.Fingerprint)
	audit.SetCorrelationID(event, rrName) // Use RR name as correlation
	audit.SetNamespace(event, signal.Namespace)

	// Event data with Gateway-specific fields + RR reconstruction fields
	//
	// ========================================
	// BR-AUDIT-005: RR Reconstruction Fields (DD-AUDIT-004)
	// ========================================
	// SOC2 Compliance: Gaps #1-3 for RemediationRequest reconstruction
	// - Gap #1: original_payload (full signal payload for RR.Spec.OriginalPayload)
	// - Gap #2: signal_labels (for RR.Spec.SignalLabels)
	// - Gap #3: signal_annotations (for RR.Spec.SignalAnnotations)
	// ========================================

	// Extract RR reconstruction fields with defensive nil handling (REFACTOR phase)
	labels, annotations, originalPayload := extractRRReconstructionFields(signal)

	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.GatewayAuditPayload{
		EventType: EventTypeSignalReceived, // Required for discriminator

		// Gateway-Specific Metadata
		SignalType:  toGatewayAuditPayloadSignalType(signal.SourceType),
		AlertName:   signal.AlertName,
		Namespace:   signal.Namespace,
		Fingerprint: signal.Fingerprint,
	}

	// RR Reconstruction Fields (Root Level per DD-AUDIT-004)
	if originalPayload != nil {
		payload.OriginalPayload.SetTo(convertMapToJxRaw(originalPayload)) // Gap #1
	}
	if labels != nil {
		payload.SignalLabels.SetTo(labels) // Gap #2
	}
	if annotations != nil {
		payload.SignalAnnotations.SetTo(annotations) // Gap #3
	}

	// Optional fields
	payload.Severity = toGatewayAuditPayloadSeverity(signal.Severity) // Pass through raw severity (DD-SEVERITY-001)
	payload.ResourceKind.SetTo(signal.Resource.Kind)
	payload.ResourceName.SetTo(signal.Resource.Name)
	payload.RemediationRequest.SetTo(fmt.Sprintf("%s/%s", rrNamespace, rrName))
	payload.DeduplicationStatus.SetTo(toGatewayAuditPayloadDeduplicationStatus("new"))

	event.EventData = api.NewAuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData(payload)

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
		// ❌ CRITICAL: This should NEVER happen if init is fixed (ADR-032 §2)
		s.logger.Error(fmt.Errorf("AuditStore is nil"), "CRITICAL: Cannot record audit event (ADR-032 §1.5 violation)")
		return
	}

	// Use OpenAPI helper functions (DD-AUDIT-002 V2.0.1)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeSignalDeduplicated)
	audit.SetEventCategory(event, CategoryGateway)
	audit.SetEventAction(event, ActionDeduplicated)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "external", signal.SourceType)
	audit.SetResource(event, "Signal", signal.Fingerprint)
	audit.SetCorrelationID(event, rrName)
	audit.SetNamespace(event, signal.Namespace)

	// Event data with RR reconstruction fields (same as signal.received for consistency)
	// Extract RR reconstruction fields with defensive nil handling (REFACTOR phase)
	labels, annotations, originalPayload := extractRRReconstructionFields(signal)

	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.GatewayAuditPayload{
		EventType: EventTypeSignalDeduplicated, // Required for discriminator

		// Gateway-Specific Metadata
		SignalType:  toGatewayAuditPayloadSignalType(signal.SourceType),
		AlertName:   signal.AlertName,
		Namespace:   signal.Namespace,
		Fingerprint: signal.Fingerprint,
	}
	payload.OccurrenceCount.SetTo(occurrenceCount)

	// RR Reconstruction Fields (Root Level per DD-AUDIT-004)
	if originalPayload != nil {
		payload.OriginalPayload.SetTo(convertMapToJxRaw(originalPayload)) // Gap #1
	}
	if labels != nil {
		payload.SignalLabels.SetTo(labels) // Gap #2
	}
	if annotations != nil {
		payload.SignalAnnotations.SetTo(annotations) // Gap #3
	}

	// Optional fields
	payload.RemediationRequest.SetTo(fmt.Sprintf("%s/%s", rrNamespace, rrName))
	payload.DeduplicationStatus.SetTo(toGatewayAuditPayloadDeduplicationStatus("duplicate"))

	event.EventData = api.NewAuditEventRequestEventDataGatewaySignalDeduplicatedAuditEventRequestEventData(payload)

	if err := s.auditStore.StoreAudit(ctx, event); err != nil {
		s.logger.Info("DD-AUDIT-003: Failed to emit signal.deduplicated audit event",
			"error", err, "fingerprint", signal.Fingerprint)
	}
}

// emitCRDCreatedAudit emits 'gateway.crd.created' audit event (DD-AUDIT-003)
// This is called when a RemediationRequest CRD is successfully created
func (s *Server) emitCRDCreatedAudit(ctx context.Context, signal *types.NormalizedSignal, rrName, rrNamespace string) {
	if s.auditStore == nil {
		// ❌ CRITICAL: This should NEVER happen if init is fixed (ADR-032 §2)
		s.logger.Error(fmt.Errorf("AuditStore is nil"), "CRITICAL: Cannot record audit event (ADR-032 §1.5 violation)")
		return
	}

	// Use OpenAPI helper functions (DD-AUDIT-002 V2.0.1)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeCRDCreated)
	audit.SetEventCategory(event, CategoryGateway)
	audit.SetEventAction(event, ActionCreated)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "gateway", "crd-creator") // Gateway's CRD creator component
	audit.SetResource(event, "RemediationRequest", fmt.Sprintf("%s/%s", rrNamespace, rrName))
	audit.SetCorrelationID(event, rrName) // Use RR name as correlation
	audit.SetNamespace(event, signal.Namespace)

	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.GatewayAuditPayload{
		EventType: EventTypeCRDCreated, // Required for discriminator

		SignalType:  toGatewayAuditPayloadSignalType(signal.SourceType),
		AlertName:   signal.AlertName,
		Namespace:   signal.Namespace,
		Fingerprint: signal.Fingerprint,
	}

	// Optional fields
	payload.Severity = toGatewayAuditPayloadSeverity(signal.Severity) // Pass through raw severity (DD-SEVERITY-001)
	payload.ResourceKind.SetTo(signal.Resource.Kind)
	payload.ResourceName.SetTo(signal.Resource.Name)
	payload.RemediationRequest.SetTo(fmt.Sprintf("%s/%s", rrNamespace, rrName))
	payload.OccurrenceCount.SetTo(1) // BR-GATEWAY-056: New CRD always has OccurrenceCount=1

	event.EventData = api.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload)

	// DEBUG: Log full event structure for HTTP 400 troubleshooting
	s.logger.Info("[DEBUG] emitCRDCreatedAudit - full event",
		"event_type", event.EventType,
		"correlation_id", event.CorrelationID,
		"resource_type", event.ResourceType,
		"resource_id", event.ResourceID,
		"actor_type", event.ActorType,
		"actor_id", event.ActorID,
		"namespace", event.Namespace)
	s.logger.Info("[DEBUG] emitCRDCreatedAudit - payload",
		"event_type_discriminator", payload.EventType,
		"signal_type", payload.SignalType,
		"severity_is_set", payload.Severity.IsSet(),
		"alert_name", payload.AlertName)

	// Fire-and-forget: StoreAudit is non-blocking per DD-AUDIT-002
	if err := s.auditStore.StoreAudit(ctx, event); err != nil {
		s.logger.Info("DD-AUDIT-003: Failed to emit crd.created audit event",
			"error", err, "rrName", rrName)
	}
}

// emitCRDCreationFailedAudit emits 'gateway.crd.failed' audit event (DD-AUDIT-003)
// This is called when RemediationRequest CRD creation fails
//
// GW-INT-AUD-019 Enhancement (BR-GATEWAY-093):
// Detects circuit breaker state and includes it in error details for audit trail compliance
//
// BR-GATEWAY-058-A (Enhanced Correlation ID Pattern):
// Uses human-readable correlation ID (alertname:namespace:kind:name) instead of SHA256 hash
// for better operator experience and pattern matching capabilities.
// Fingerprint (SHA256) remains in payload for deduplication queries.
func (s *Server) emitCRDCreationFailedAudit(ctx context.Context, signal *types.NormalizedSignal, err error) {
	if s.auditStore == nil {
		// ❌ CRITICAL: This should NEVER happen if init is fixed (ADR-032 §2)
		s.logger.Error(fmt.Errorf("AuditStore is nil"), "CRITICAL: Cannot record audit event (ADR-032 §1.5 violation)")
		return
	}

	// Use OpenAPI helper functions (DD-AUDIT-002 V2.0.1)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeCRDFailed)
	audit.SetEventCategory(event, CategoryGateway)
	audit.SetEventAction(event, ActionFailed)
	audit.SetEventOutcome(event, audit.OutcomeFailure)
	audit.SetActor(event, "gateway", "crd-creator")

	// BR-GATEWAY-058-A: Use human-readable correlation ID
	// Format: "alertname:namespace:kind:name" (e.g., "HighMemoryUsage:prod:Pod:api-789")
	// Benefit: SRE can immediately understand what triggered the failure
	// Fingerprint (SHA256) still available in payload for deduplication
	correlationID := constructReadableCorrelationID(signal)
	audit.SetResource(event, "RemediationRequest", correlationID)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, signal.Namespace)

	// BR-AUDIT-005 Gap #7: Standardized error_details
	// GW-INT-AUD-019 (BR-GATEWAY-093): Detect circuit breaker errors for audit compliance
	var errorDetails *sharedaudit.ErrorDetails
	if errors.Is(err, gobreaker.ErrOpenState) {
		// Circuit breaker is open - create specialized error details
		// BR-GATEWAY-093: Circuit breaker for K8s API
		errorDetails = sharedaudit.NewErrorDetails(
			"gateway",
			"ERR_CIRCUIT_BREAKER_OPEN",
			"K8s API circuit breaker is open (fail-fast mode) - preventing cascade failure",
			true, // Retry possible once circuit breaker closes
		)
		s.logger.Info("Circuit breaker prevented K8s API request",
			"fingerprint", signal.Fingerprint,
			"circuit_breaker_state", "open")
	} else {
		// Standard K8s error handling
		errorDetails = sharedaudit.NewErrorDetailsFromK8sError("gateway", err)
	}

	// Convert shared ErrorDetails to api.ErrorDetails
	apiErrorDetails := toAPIErrorDetails(errorDetails)

	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.GatewayAuditPayload{
		EventType: EventTypeCRDFailed, // Required for discriminator

		SignalType:  toGatewayAuditPayloadSignalType(signal.SourceType),
		AlertName:   signal.AlertName,
		Namespace:   signal.Namespace,
		Fingerprint: signal.Fingerprint,
	}

	// Optional fields
	payload.Severity = toGatewayAuditPayloadSeverity(signal.Severity) // Pass through raw severity (DD-SEVERITY-001)
	payload.ResourceKind.SetTo(signal.Resource.Kind)
	payload.ResourceName.SetTo(signal.Resource.Name)
	payload.ErrorDetails.SetTo(apiErrorDetails) // Gap #7: Standardized error_details for SOC2 compliance

	event.EventData = api.NewAuditEventRequestEventDataGatewayCrdFailedAuditEventRequestEventData(payload)

	// Fire-and-forget: StoreAudit is non-blocking per DD-AUDIT-002
	if storeErr := s.auditStore.StoreAudit(ctx, event); storeErr != nil {
		s.logger.Info("DD-AUDIT-003: Failed to emit crd.creation_failed audit event",
			"error", storeErr, "fingerprint", signal.Fingerprint)
	}
}

// constructReadableCorrelationID creates human-readable correlation ID for failed CRD creation
//
// BR-GATEWAY-058-A: Enhanced Correlation ID Pattern
//
// Format: "alertname:namespace:kind:name"
// Examples:
//   - "HighMemoryUsage:prod-payment-service:Pod:payment-api-789"
//   - "NodeNotReady:default:Node:worker-node-1"
//   - "DeploymentReplicasUnavailable:prod-api:Deployment:api-server"
//
// Benefits:
//   - Human-readable: SRE can immediately identify the alert and resource
//   - Pattern matching: Query all failures for specific alert or namespace
//   - Consistency: Aligns with industry standards (OpenTelemetry semantic conventions)
//
// Fingerprint (SHA256) remains in GatewayAuditPayload.fingerprint for deduplication queries.
//
// Returns:
//   - string: Human-readable correlation ID (30-150 chars depending on names)
func constructReadableCorrelationID(signal *types.NormalizedSignal) string {
	return fmt.Sprintf("%s:%s:%s:%s",
		signal.AlertName,
		signal.Namespace,
		signal.Resource.Kind,
		signal.Resource.Name,
	)
}

// handleDuplicateSignal handles the case where another pod created a RemediationRequest during lock contention
// TDD REFACTOR: Extracted from ProcessSignal lock retry loop for clarity and testability
//
// BR-GATEWAY-190: Multi-replica deduplication safety
// ADR-052 Addendum 001: This helper is called when exponential backoff retry discovers
// that another Gateway pod successfully acquired the lock and created the RR.
//
// Business Outcome:
//   - Updates occurrence count for deduplication tracking
//   - Records metrics for alert deduplication monitoring
//   - Emits audit event for compliance and debugging
//   - Returns early from retry loop (no need to continue retrying)
//
// Returns:
//   - *ProcessingResponse: Duplicate response with existing RR reference
//   - error: Non-nil if status update or audit emission fails critically
func (s *Server) handleDuplicateSignal(ctx context.Context, signal *types.NormalizedSignal, existingRR *remediationv1alpha1.RemediationRequest) (*ProcessingResponse, error) {
	logger := middleware.GetLogger(ctx)

	// Update occurrence count for deduplication tracking
	if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
		// Non-critical: Log and continue (deduplication still succeeded)
		logger.Info("Failed to update deduplication status after lock contention",
			"error", err,
			"fingerprint", signal.Fingerprint,
			"rr", existingRR.Name)
	}

	// Get updated occurrence count for metrics and audit
	occurrenceCount := int32(1)
	if existingRR.Status.Deduplication != nil {
		occurrenceCount = existingRR.Status.Deduplication.OccurrenceCount
	}

	// Record metrics for monitoring dashboard
	s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.AlertName).Inc()
	s.metricsInstance.DeduplicationCacheHitsTotal.Inc()

	// Emit audit event for compliance (DD-AUDIT-003)
	s.emitSignalDeduplicatedAudit(ctx, signal, existingRR.Name, existingRR.Namespace, occurrenceCount)

	// Return duplicate response (early exit from retry loop)
	return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
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

		// DD-AUDIT-003: Emit crd.creation_failed audit event (DD-AUDIT-003)
		// Fire-and-forget: audit failures don't affect business logic
		s.emitCRDCreationFailedAudit(ctx, signal, err)

		return nil, fmt.Errorf("failed to create RemediationRequest CRD: %w", err)
	}

	// DD-GATEWAY-011: Initialize status.deduplication for NEW CRD
	// Gateway owns status.deduplication per DD-GATEWAY-011
	// Must initialize immediately after creation (OccurrenceCount=1, FirstSeenAt=now)
	if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, rr); err != nil {
		logger.Info("Failed to initialize deduplication status (DD-GATEWAY-011)",
			"error", err,
			"fingerprint", signal.Fingerprint,
			"rr", rr.Name)
		// Non-fatal: CRD exists, status update can be retried by RO or next duplicate
	}

	// DD-GATEWAY-011: Redis deduplication storage DEPRECATED
	// Deduplication now uses K8s status-based lookup (phaseChecker.ShouldDeduplicate)
	// and status updates (statusUpdater.UpdateDeduplicationStatus)
	// Redis is no longer used for deduplication state

	// DD-AUDIT-003: Emit signal.received audit event (BR-GATEWAY-190)
	// Fire-and-forget: audit failures don't affect business logic
	s.emitSignalReceivedAudit(ctx, signal, rr.Name, rr.Namespace)

	// DD-AUDIT-003: Emit crd.created audit event (DD-AUDIT-003)
	// Fire-and-forget: audit failures don't affect business logic
	s.emitCRDCreatedAudit(ctx, signal, rr.Name, rr.Namespace)

	// Record processing duration
	duration := time.Since(start)
	logger.Info("Signal processed successfully",
		"fingerprint", signal.Fingerprint,
		"crdName", rr.Name,
		"duration_ms", duration.Milliseconds())

	return NewCRDCreatedResponse(signal.Fingerprint, rr.Name, rr.Namespace), nil
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
