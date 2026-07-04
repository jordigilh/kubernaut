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
	"net/http"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr" // BR-GATEWAY-093: Circuit breaker detection

	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"

	"github.com/jordigilh/kubernaut/pkg/audit" // DD-AUDIT-003: Audit integration
	// Ogen generated audit types
	// BR-AUDIT-005 Gap #7: Standardized error details
	"github.com/jordigilh/kubernaut/pkg/shared/auth" // BR-GATEWAY-036/037: Shared auth middleware
	// ADR-052 Addendum 001: Exponential backoff with jitter
	// Issue #753: Dedicated health server
	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"      // #1553: fail-closed Fleet readiness gate
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"     // Issue #756: FileWatcher for cert rotation
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls" // Issue #493/#678: Conditional TLS

	// BR-GATEWAY-190: Lease resources for distributed locking
	corev1 "k8s.io/api/core/v1" // BR-GATEWAY-036/037: K8s clientset for TokenReview/SAR
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// ADR-068: Federated scope checking factory
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/middleware" // BR-109: Request ID middleware
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	kubecors "github.com/jordigilh/kubernaut/pkg/http/cors" // BR-HTTP-015: Shared CORS library

	// DD-005: Shared sanitization library
	"github.com/jordigilh/kubernaut/pkg/shared/scope" // BR-SCOPE-002: Resource scope management
)

// Gateway Audit Event Type Constants (from OpenAPI spec)
// See: api/openapi/data-storage-v1.yaml - GatewayAuditPayload.event_type enum
const (
	EventTypeSignalReceived     = "gateway.signal.received"     // BR-GATEWAY-190: New signal received, RR created
	EventTypeSignalDeduplicated = "gateway.signal.deduplicated" // BR-GATEWAY-191: Duplicate signal detected
	EventTypeCRDCreated         = "gateway.crd.created"         // DD-AUDIT-003: RR CRD successfully created
	EventTypeCRDFailed          = "gateway.crd.failed"          // DD-AUDIT-003: RR CRD creation failed
	EventTypeConfigReloaded     = "gateway.config.reloaded"     // GAP-11 (Issue #1505): Hot-reload accepted (log level, CA cert)
	EventTypeConfigRejected     = "gateway.config.rejected"     // GAP-11 (Issue #1505): Hot-reload rejected, previous config kept
	CategoryGateway             = "gateway"                     // Service-level category per ADR-034
	// EventCategoryGateway is an alias for consistency with other services
	EventCategoryGateway = CategoryGateway
)

// Gateway Audit Event Action Constants (ADR-034 event_action field)
// These are past-tense verbs describing what action was performed
const (
	ActionReceived     = "received"     // Signal was received and processed
	ActionDeduplicated = "deduplicated" // Signal was deduplicated to existing RR
	ActionCreated      = "created"      // RemediationRequest CRD was created
	ActionFailed       = "failed"       // Operation failed
	ActionReloaded     = "reloaded"     // GAP-11: Hot-reloadable config accepted
	ActionRejected     = "rejected"     // GAP-11: Hot-reloadable config rejected
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
// - Rate limiting: chi.Throttle concurrent-request limiter; per-IP rate limiting should be enforced at the Ingress/Route layer
// - Input validation: Schema validation for all signal types
//
// Observability features:
// - Prometheus metrics: 17+ metrics on /metrics endpoint
// - Health/readiness probes: /health and /ready endpoints
// - Structured logging: JSON format with trace IDs
// - Distributed tracing: OpenTelemetry integration (future)
type Server struct {
	// HTTP servers (Issue #753: 3-port standard — API :8080, Health :8081, Metrics :9090)
	httpServer    *http.Server
	healthServer  *http.Server            // Issue #753: dedicated health probe server (/healthz, /readyz)
	metricsServer *http.Server            // Issue #753: dedicated metrics server (/metrics)
	router        chi.Router              // Chi router for adapter registration and route grouping
	certReloader  *sharedtls.CertReloader // Issue #756: nil when TLS disabled
	certWatcher   *hotreload.FileWatcher  // Issue #756: nil when TLS disabled
	tlsCertDir    string                  // Issue #756: cert dir for FileWatcher path

	// Configuration
	config *config.ServerConfig // ADR-030: Service configuration (needed for middleware setup)

	// Core processing components
	adapterRegistry *adapters.AdapterRegistry
	// DD-GATEWAY-011 + DD-GATEWAY-012: Status-based deduplication
	// Redis DEPRECATED - all state now in K8s RR status
	statusUpdater *processing.StatusUpdater                  // Updates RR status.deduplication
	phaseChecker  *processing.PhaseBasedDeduplicationChecker // Phase-based deduplication logic
	crdCreator    *processing.CRDCreator
	lockManager   *processing.DistributedLockManager // BR-GATEWAY-190: K8s Lease-based distributed locking
	scopeChecker  scope.ScopeChecker                 // BR-SCOPE-002 + ADR-068: nil = no scope filtering (backward compat)
	// fleetReadinessGate reflects Fleet dependency (MCP Gateway / scope-check
	// backend) reachability (#1553, ADR-068, BR-INTEGRATION-065). nil when
	// Fleet is disabled, in which case readinessHandler skips the check
	// entirely (fleet was never part of the readiness contract before).
	fleetReadinessGate *readiness.Gate

	// ADR-057: Controller namespace where all CRDs are created and queried
	controllerNamespace string

	// Infrastructure clients
	// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free
	k8sClient  *k8s.Client
	ctrlClient client.Client
	// apiReader: Uncached client for cache-bypassed reads (readiness K8s check, dedup, status refetch)
	// ADR-057 fix: Readiness List(NamespaceList) must use apiReader—ctrlClient's cache only watches
	// RemediationRequest in controllerNS; NamespaceList may fail when served from restricted cache.
	apiReader client.Reader

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

	// BR-GATEWAY-036/037: Auth middleware (nil = auth disabled for backward-compat test migration)
	// Production: always non-nil (K8sAuthenticator + K8sAuthorizer)
	authMiddleware *auth.Middleware

	// BR-GATEWAY-185 / Issue #852: Informer cache sync gate
	// Zero-value (false) = fail-closed: readiness rejects until WaitForCacheSync completes.
	// Set to true ONLY after cache.WaitForCacheSync succeeds in production startup.
	cacheReady atomic.Bool

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

// LivenessHandler returns the liveness probe handler for use with the
// dedicated health server (Issue #753: port 8081, /healthz).
func (s *Server) LivenessHandler() http.HandlerFunc {
	return s.healthHandler
}

// ReadinessHandler returns the readiness probe handler for use with the
// dedicated health server (Issue #753: port 8081, /readyz).
func (s *Server) ReadinessHandler() http.HandlerFunc {
	return s.readinessHandler
}

// ScopeChecker returns the server's scope.ScopeChecker (nil if scope
// filtering is disabled). Exposed so production wiring code (cmd/gateway/
// main.go) can reach the federated remote backend for readiness probing
// without duplicating scope-checker construction (#1553).
func (s *Server) ScopeChecker() scope.ScopeChecker {
	return s.scopeChecker
}

// SetFleetReadinessGate wires the Fleet dependency readiness gate (#1553).
// Production code (cmd/gateway/main.go) calls this once at startup, after
// starting the gate, when Fleet is enabled. Must be called before the HTTP
// server starts accepting readiness probes.
func (s *Server) SetFleetReadinessGate(gate *readiness.Gate) {
	s.fleetReadinessGate = gate
}

// MarkCacheReady signals that the informer cache has completed initial sync.
// Call this ONCE from production startup after cache.WaitForCacheSync succeeds.
// The readiness handler returns 503 until this is called (fail-closed design).
func (s *Server) MarkCacheReady() {
	s.cacheReady.Store(true)
	s.logger.Info("Informer cache sync complete, readiness probe unblocked")
}

// GetMetrics returns the metrics instance for wiring into adapters.
func (s *Server) GetMetrics() *metrics.Metrics {
	return s.metricsInstance
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

	// BR-HTTP-015 + Issue #1215: CORS from config YAML with env-var fallback.
	corsOpts := kubecors.FromConfig(&kubecors.Options{
		AllowedOrigins:   s.config.CORS.AllowedOrigins,
		AllowedMethods:   s.config.CORS.AllowedMethods,
		AllowedHeaders:   s.config.CORS.AllowedHeaders,
		ExposedHeaders:   s.config.CORS.ExposedHeaders,
		AllowCredentials: s.config.CORS.AllowCredentials,
		MaxAge:           s.config.CORS.MaxAge,
	})
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

	// Issue #673 L-1: Trusted proxy-aware RealIP extraction.
	// Replaces chimiddleware.RealIP which unconditionally trusts proxy headers.
	// Fail-closed: empty CIDRs = proxy headers never trusted.
	trustedCIDRs := s.config.Middleware.TrustedProxyCIDRs
	if len(trustedCIDRs) > 0 {
		s.logger.Info("Trusted proxy RealIP enabled",
			"trusted_cidrs", trustedCIDRs,
			"authority", "Issue #673 L-1")
	} else {
		s.logger.Info("Trusted proxy RealIP: fail-closed (no CIDRs configured, proxy headers ignored)")
	}
	r.Use(middleware.TrustedRealIP(trustedCIDRs))

	// BR-GATEWAY-074, BR-GATEWAY-075: Replay prevention middleware
	// Moved from global to per-adapter in RegisterAdapter() via ReplayValidator().
	// Each adapter declares its own replay prevention strategy:
	// - Prometheus: header-based (X-Timestamp via TimestampValidator)
	// - K8s Events: body-based (event timestamps via EventFreshnessValidator)

	// Security headers middleware (OWASP best practices)
	// Prevents: MIME sniffing, clickjacking, XSS attacks, enforces HTTPS
	r.Use(middleware.SecurityHeaders())

	// Request ID middleware for distributed tracing
	// Ensures X-Request-ID is present for correlation across services
	r.Use(middleware.RequestIDMiddleware(s.logger))

	// HTTP metrics middleware for observability
	// Records request counts and duration metrics
	r.Use(middleware.HTTPMetrics(s.metricsInstance))

	// Issue #753: Health and metrics routes moved to dedicated servers (:8081, :9090).
	// API routes are registered dynamically when adapters call RegisterAdapter().
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

		// HTTPRequestDuration is already observed by middleware.HTTPMetrics (http_metrics.go).
		// Duplicate observation removed — see Phase 3a of FedRAMP remediation.

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

// GetCachedClient returns the controller-runtime cached client used by the Gateway.
// This client uses metadata-only informers (PartialObjectMetadata) for resource lookups.
//
// Used by:
//   - K8sOwnerResolver (BR-GATEWAY-004): owner chain resolution for K8s event deduplication
//
// Note: scope.Manager uses ctrlClient (informer-backed) — see createServerWithClients.
func (s *Server) GetCachedClient() client.Client {
	return s.ctrlClient
}

// GetAPIReader returns the uncached client for direct API server reads.
// Used by K8sOwnerResolver as a fallback when the informer cache misses
// newly created resources (e.g., pods after a rollout restart). (#282)
func (s *Server) GetAPIReader() client.Reader {
	return s.apiReader
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
	s.logger.Info("Starting Gateway server",
		"api_addr", s.httpServer.Addr,
		"health_addr", s.healthServer.Addr,
		"metrics_addr", s.metricsServer.Addr)

	// Issue #756: Start cert file watcher for hot-reload before accepting connections
	if s.certReloader != nil {
		watcher, err := hotreload.NewFileWatcher(
			filepath.Join(s.tlsCertDir, "tls.crt"),
			s.certReloader.ReloadCallback,
			s.logger.WithName("cert-reloader"),
		)
		if err != nil {
			return fmt.Errorf("failed to create cert file watcher: %w", err)
		}
		if err := watcher.Start(ctx); err != nil {
			return fmt.Errorf("failed to start cert file watcher: %w", err)
		}
		s.certWatcher = watcher
	}

	// Issue #753: Start dedicated health and metrics servers (plain HTTP, never TLS)
	go func() {
		s.logger.Info("Starting dedicated health server", "addr", s.healthServer.Addr)
		if err := s.healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error(err, "Health server failed")
		}
	}()
	go func() {
		s.logger.Info("Starting dedicated metrics server", "addr", s.metricsServer.Addr)
		if err := s.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error(err, "Metrics server failed")
		}
	}()

	// Issue #493: Conditional TLS — serve HTTPS when TLSConfig is set
	if s.httpServer.TLSConfig != nil {
		s.logger.Info("TLS enabled, starting HTTPS server")
		return s.httpServer.ListenAndServeTLS("", "")
	}
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
	s.logger.Info("Waiting for Kubernetes endpoint removal propagation (up to 5s)")
	select {
	case <-ctx.Done():
		s.logger.Info("Shutdown context cancelled during endpoint propagation wait")
	case <-time.After(5 * time.Second):
		s.logger.Info("Endpoint removal propagation complete, proceeding with HTTP server shutdown")
	}

	// STEP 3: Graceful HTTP server shutdown
	// Now that pod is removed from endpoints, we can safely shutdown
	// This will complete any in-flight requests that arrived before endpoint removal
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error(err, "Failed to gracefully shutdown HTTP server")
		return err
	}

	// Issue #753: Shutdown dedicated health and metrics servers
	if s.healthServer != nil {
		if err := s.healthServer.Shutdown(ctx); err != nil {
			s.logger.Error(err, "Failed to shutdown health server")
		}
	}
	if s.metricsServer != nil {
		if err := s.metricsServer.Shutdown(ctx); err != nil {
			s.logger.Error(err, "Failed to shutdown metrics server")
		}
	}

	// Issue #756: Stop cert file watcher after HTTP server is down
	if s.certWatcher != nil {
		s.certWatcher.Stop()
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
// 2. Informer cache has completed initial sync (cacheReady == true)
// 3. Kubernetes API is reachable (list namespaces succeeds)
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
	// Shutdown takes highest priority — Kubernetes must remove pod from
	// Service endpoints BEFORE httpServer.Shutdown closes the listener.
	if s.isShuttingDown.Load() {
		s.writeReadinessUnavailable(w, r, "server is shutting down",
			"Server is shutting down gracefully")
		return
	}

	// BR-GATEWAY-185 / Issue #852: Gate on informer cache sync.
	// Prevents stale-data responses during startup or cache resync.
	if !s.cacheReady.Load() {
		s.writeReadinessUnavailable(w, r, "informer cache not synced",
			"Informer cache has not completed initial sync")
		return
	}

	// K8s API connectivity check (ADR-057: uses apiReader to bypass restricted cache).
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	reader := s.apiReader
	if reader == nil {
		reader = s.ctrlClient
	}
	namespaceList := &corev1.NamespaceList{}
	if err := reader.List(ctx, namespaceList, client.Limit(1)); err != nil {
		s.writeReadinessUnavailable(w, r, "Kubernetes API not reachable",
			"Kubernetes API is not reachable")
		return
	}

	// #1553 / ADR-068 / BR-INTEGRATION-065: when Fleet is enabled, an
	// unreachable MCP Gateway or scope-check backend fails the whole pod's
	// readiness (not just the fleet-specific code path), so Kubernetes
	// removes it from Service endpoints until the dependency recovers.
	if s.fleetReadinessGate != nil {
		if err := s.fleetReadinessGate.Check(r); err != nil {
			s.writeReadinessUnavailable(w, r, "fleet dependency unreachable: "+err.Error(),
				"Fleet dependency unreachable: "+err.Error())
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ready"}); err != nil {
		s.logger.Error(err, "Failed to encode readiness response")
	}
}

// writeReadinessUnavailable writes a 503 RFC 7807 response for the readiness probe.
// logReason appears in the structured log; detail appears in the response body.
func (s *Server) writeReadinessUnavailable(w http.ResponseWriter, r *http.Request, logReason, detail string) {
	s.logger.Info("Readiness check failed: " + logReason)

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusServiceUnavailable)

	resp := gwerrors.RFC7807Error{
		Type:     gwerrors.ErrorTypeServiceUnavailable,
		Title:    gwerrors.TitleServiceUnavailable,
		Detail:   detail,
		Status:   http.StatusServiceUnavailable,
		Instance: r.URL.Path,
	}
	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		s.logger.Error(encErr, "Failed to encode readiness error response")
	}
}
