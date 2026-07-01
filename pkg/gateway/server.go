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
	"github.com/jordigilh/kubernaut/pkg/shared/auth"    // BR-GATEWAY-036/037: Shared auth middleware
	"github.com/jordigilh/kubernaut/pkg/shared/backoff" // ADR-052 Addendum 001: Exponential backoff with jitter
	// Issue #753: Dedicated health server
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"     // Issue #756: FileWatcher for cert rotation
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls" // Issue #493/#678: Conditional TLS

	// BR-GATEWAY-190: Lease resources for distributed locking
	corev1 "k8s.io/api/core/v1" // BR-GATEWAY-036/037: K8s clientset for TokenReview/SAR
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1" // ADR-068: Federated scope checking factory
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/middleware" // BR-109: Request ID middleware
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
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

// RegisterAdapter registers a RoutableAdapter using chi router
//
// This method:
// 1. Validates adapter (checks for duplicate names/routes)
// 2. Registers adapter in registry
// 3. Creates HTTP handler (batch-aware for BatchParser adapters, single-signal otherwise)
// 4. Applies middleware and registers route with chi router
//
// Middleware applied:
// - Content-Type validation (BR-042)
// - Request ID (chi middleware - global)
// - Real IP extraction (chi middleware - global)
//
// Example:
//
//	prometheusAdapter := adapters.NewPrometheusAdapter(ownerResolver, registry)
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

	// BR-GATEWAY-074, BR-GATEWAY-075: Apply adapter-specific replay prevention middleware
	// Each adapter declares its own strategy via ReplayValidator():
	// - Header-based (e.g., Prometheus): middleware.TimestampValidator (X-Timestamp header)
	// - Body-based (e.g., K8s Events): middleware.EventFreshnessValidator (event timestamp)
	finalHandler := adapter.ReplayValidator(5 * time.Minute)(wrappedHandler)

	// BR-GATEWAY-036/037: Apply auth middleware (outermost layer)
	// Auth is checked before any content-type validation or replay prevention
	if s.authMiddleware != nil {
		finalHandler = s.authMiddleware.Handler(finalHandler)
	}

	// Register route using chi with full path
	// Chi automatically enforces POST method (returns 405 for other methods)
	// Note: chi.Router.Post() accepts http.HandlerFunc, so we use HandlerFunc wrapper
	s.router.Post(adapter.GetRoute(), finalHandler.ServeHTTP)

	s.logger.Info("Registered adapter route",
		"adapter", adapter.Name(),
		"route", adapter.GetRoute())

	return nil
}

// createAdapterHandler creates an HTTP handler for an adapter
//
// This handler:
// For single-signal adapters:
// 1. Reads request body
// 2. Calls adapter.Parse() to convert to NormalizedSignal
// 3. Validates signal using adapter.Validate()
// 4. Calls ProcessSignal() to run full pipeline
// 5. Returns HTTP response (201/202/400/500)
//
// For BatchParser adapters (e.g., Prometheus):
// Delegates to handleBatchRequest which processes each signal independently
// and returns HTTP 207 Multi-Status with per-alert results.
//
// REFACTORED: Reduced cyclomatic complexity by extracting helper methods
func (s *Server) createAdapterHandler(adapter adapters.SignalAdapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only accept POST requests
		if r.Method != http.MethodPost {
			s.writeJSONError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx := r.Context()
		logger := middleware.GetLogger(ctx)

		// Check if adapter supports batch parsing (e.g., Prometheus AlertManager)
		if batchAdapter, ok := adapter.(adapters.BatchParser); ok {
			s.handleBatchRequest(ctx, w, r, adapter, batchAdapter, logger)
			return
		}

		start := time.Now()

		// Read, parse, and validate signal
		signal, err := s.readParseValidateSignal(ctx, w, r, adapter, logger)
		if err != nil {
			return // Error response already sent
		}

		// BR-GATEWAY-102: Enforce per-handler timeout on K8s API operations.
		// Must be < WriteTimeout to allow writing 504 JSON before the server kills the connection.
		k8sTimeout := s.config.Server.K8sRequestTimeout
		if k8sTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, k8sTimeout)
			defer cancel()
		}

		// Process signal through pipeline
		response, err := s.ProcessSignal(ctx, signal)
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				logger.Error(err, "K8s request timeout exceeded",
					"adapter", adapter.Name(),
					"timeout", k8sTimeout)
				s.writeJSONError(w, r, "Request processing timed out", http.StatusGatewayTimeout)
				return
			}
			s.handleProcessingError(w, r, err, adapter.Name(), logger)
			return
		}

		// Send success response
		s.sendSuccessResponse(w, r, response, adapter, start)
	}
}

// handleBatchRequest processes a batch payload where each signal is handled independently.
//
// Routing strategy:
//   - Single-alert batches (len == 1): delegate to the standard single-signal pipeline
//     so that existing HTTP status contracts (201, 202, 500, 504) are preserved.
//     AlertManager retries on 5xx, so returning 207 for a single failed alert
//     would silently swallow the failure.
//   - Multi-alert batches (len > 1): return HTTP 207 Multi-Status with per-alert
//     results and an aggregate summary (#1036).
func (s *Server) handleBatchRequest(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	adapter adapters.SignalAdapter,
	batchAdapter adapters.BatchParser,
	logger logr.Logger,
) {
	start := time.Now()

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			logger.Info("Request body too large", "limit", maxRequestBodySize)
			s.writeJSONError(w, r, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		logger.Error(err, "Failed to read request body")
		s.writeJSONError(w, r, "Failed to read request body", http.StatusBadRequest)
		return
	}

	signals, err := batchAdapter.ParseBatch(ctx, body)
	if err != nil {
		logger.Info("Batch parse failed", "adapter", adapter.Name(), "error", err)
		s.writeJSONError(w, r, "Failed to parse batch payload", http.StatusBadRequest)
		return
	}

	if len(signals) == 0 {
		resp := BatchProcessingResponse{
			Results: []ProcessingResult{},
			Summary: BatchSummary{Total: 0},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMultiStatus)
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// Single-alert batch: use standard single-signal pipeline to preserve
	// HTTP status contracts (201/202/500/504) for backward compatibility.
	if len(signals) == 1 {
		s.processSingleSignal(ctx, w, r, adapter, signals[0], logger, start)
		return
	}

	// Multi-alert batch: process each signal independently, return 207.
	s.processMultiSignalBatch(ctx, w, r, adapter, signals, logger)
}

// processSingleSignal handles a single signal through the standard pipeline,
// preserving existing HTTP status code contracts.
func (s *Server) processSingleSignal(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	adapter adapters.SignalAdapter,
	signal *types.NormalizedSignal,
	logger logr.Logger,
	start time.Time,
) {
	if valErr := adapter.Validate(signal); valErr != nil {
		logger.Info("Signal validation failed",
			"adapter", adapter.Name(),
			"error", valErr)
		s.writeValidationError(w, r, fmt.Sprintf("Signal validation failed: %v", valErr))
		return
	}

	k8sTimeout := s.config.Server.K8sRequestTimeout
	if k8sTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, k8sTimeout)
		defer cancel()
	}

	response, procErr := s.ProcessSignal(ctx, signal)
	if procErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logger.Error(procErr, "K8s request timeout exceeded",
				"adapter", adapter.Name(),
				"timeout", k8sTimeout)
			s.writeJSONError(w, r, "Request processing timed out", http.StatusGatewayTimeout)
			return
		}
		s.handleProcessingError(w, r, procErr, adapter.Name(), logger)
		return
	}

	s.sendSuccessResponse(w, r, response, adapter, start)
}

// processMultiSignalBatch processes multiple signals independently and returns
// HTTP 207 Multi-Status with per-alert results (#1036).
func (s *Server) processMultiSignalBatch(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	adapter adapters.SignalAdapter,
	signals []*types.NormalizedSignal,
	logger logr.Logger,
) {
	k8sTimeout := s.config.Server.K8sRequestTimeout
	if k8sTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, k8sTimeout)
		defer cancel()
	}

	results := make([]ProcessingResult, 0, len(signals))
	var summary BatchSummary
	summary.Total = len(signals)

	for _, signal := range signals {
		if err := ctx.Err(); err != nil {
			results = append(results, ProcessingResult{
				Status:      "failed",
				Fingerprint: signal.Fingerprint,
				Error:       "request timeout exceeded",
			})
			summary.Failed++
			continue
		}

		if valErr := adapter.Validate(signal); valErr != nil {
			logger.Info("Signal validation failed in batch",
				"fingerprint", signal.Fingerprint,
				"error", valErr)
			results = append(results, ProcessingResult{
				Status:      "rejected",
				Fingerprint: signal.Fingerprint,
				Error:       fmt.Sprintf("Signal validation failed: %s", valErr.Error()),
			})
			summary.Rejected++
			continue
		}

		response, procErr := s.ProcessSignal(ctx, signal)
		if procErr != nil {
			logger.Error(procErr, "Signal processing failed in batch",
				"fingerprint", signal.Fingerprint)
			results = append(results, ProcessingResult{
				Status:      "failed",
				Fingerprint: signal.Fingerprint,
				Error:       "Processing failed",
			})
			summary.Failed++
			continue
		}

		result := ProcessingResult{
			Status:      response.Status,
			Fingerprint: response.Fingerprint,
			Message:     response.Message,
		}
		results = append(results, result)

		switch response.Status {
		case StatusCreated:
			summary.Created++
		case StatusDeduplicated:
			summary.Deduplicated++
		case StatusRejected:
			summary.Rejected++
		default:
			summary.Created++
		}
	}

	resp := BatchProcessingResponse{
		Results: results,
		Summary: summary,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusMultiStatus)
	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		logger.Error(encErr, "Failed to encode batch response")
	}
}

// maxRequestBodySize references the shared constant from middleware.
// Issue #673 C-1 + C-ADV-1: Single source of truth for the body cap.
const maxRequestBodySize = middleware.MaxRequestBodySize

// readParseValidateSignal reads, parses, and validates the signal from the request
// Returns nil signal and writes error response if any step fails
func (s *Server) readParseValidateSignal(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	adapter adapters.SignalAdapter,
	logger logr.Logger,
) (*types.NormalizedSignal, error) {
	// Issue #673 C-1: Limit request body size before reading into memory
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			logger.Info("Request body too large", "limit", maxRequestBodySize)
			s.writeJSONError(w, r, "Request body too large", http.StatusRequestEntityTooLarge)
			return nil, err
		}
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
		s.writeValidationError(w, r, "Failed to parse signal")
		return nil, err
	}

	// Validate signal
	if err := adapter.Validate(signal); err != nil {
		logger.Info("Signal validation failed",
			"adapter", adapter.Name(),
			"error", err)
		s.writeValidationError(w, r, "Signal validation failed")
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

	// Issue #673 C-ADV-2: All processing errors return a generic message.
	// Internal details (K8s API addresses, CRD names, namespace names) are
	// already logged at line 958 via logger.Error -- no observability lost.
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
	if response.Status == StatusRejected {
		statusCode = http.StatusOK // HTTP 200 for scope rejection (not an error, just informational)
	} else if response.Status == StatusDeduplicated || response.Duplicate {
		statusCode = http.StatusAccepted // HTTP 202 for deduplication
	}

	// HTTPRequestDuration is already observed by middleware.HTTPMetrics (http_metrics.go).
	// Duplicate observation removed — see Phase 3a of FedRAMP remediation.

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

// validateScope checks whether the signal's target resource is within Kubernaut's
// management scope. Returns (nil, nil) if managed, (*ProcessingResponse, nil) for
// a clean rejection, or (nil, error) on scope infrastructure failure.
//
// BR-SCOPE-002: Label-based resource opt-in with 2-level hierarchy.
// BR-SCOPE-013: Deny-by-default when scope checker is not initialized.
func (s *Server) validateScope(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
	logger := middleware.GetLogger(ctx)

	if s.scopeChecker == nil {
		logger.Error(nil, "Scope checker not initialized — rejecting signal (deny-by-default)",
			"namespace", signal.Namespace,
			"kind", signal.Resource.Kind,
			"name", signal.Resource.Name,
			"fingerprint", signal.Fingerprint)
		s.metricsInstance.SignalsRejectedTotal.WithLabelValues(RejectionReasonScopeCheckerNotInitialized).Inc()
		return NewRejectedResponse(signal.Namespace, signal.Resource.Kind, signal.Resource.Name), nil
	}

	managed, err := s.scopeChecker.IsManagedResource(ctx, scope.ResourceIdentity{
		ClusterID: signal.ClusterID,
		Kind:      signal.Resource.Kind,
		Namespace: signal.Namespace,
		Name:      signal.Resource.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("scope validation failed: %w", err)
	}

	if managed {
		logger.V(1).Info("Scope check passed: resource is managed",
			"namespace", signal.Namespace,
			"kind", signal.Resource.Kind,
			"name", signal.Resource.Name)
		return nil, nil
	}

	s.metricsInstance.SignalsRejectedTotal.WithLabelValues(RejectionReasonUnmanagedResource).Inc()
	logger.Info("Signal rejected: resource not managed by Kubernaut",
		"namespace", signal.Namespace,
		"kind", signal.Resource.Kind,
		"name", signal.Resource.Name,
		"reason", RejectionReasonUnmanagedResource,
		"fingerprint", signal.Fingerprint)

	return NewRejectedResponse(signal.Namespace, signal.Resource.Kind, signal.Resource.Name), nil
}

// ProcessSignal implements adapters.SignalProcessor interface.
//
// Main signal processing pipeline orchestrator, called by adapter handlers.
// TDD REFACTOR: Simplified by extracting helper methods.
//
// Pipeline stages:
//  1. Scope validation → validateScope() rejects unmanaged resources
//  2. Optional distributed lock (DD-GATEWAY-013) for multi-replica safety
//  3. Deduplication check → K8s status lookup (DD-GATEWAY-011); if duplicate,
//     update status.deduplication on the existing RemediationRequest and return HTTP 202
//  4. CRD creation → createRemediationRequestCRD() for new signals; return HTTP 201
//
// Note: Environment classification and Priority assignment removed (2025-12-06).
// These are now owned by Signal Processing service per DD-CATEGORIZATION-001.
//
// Performance (order-of-magnitude; varies by cluster and API load):
// - New signal: p95 often ~50-80ms — K8s dedup check, CRD creation (Kubernetes API).
// - Duplicate: p95 often lower — K8s dedup check and status patch; no new CRD.
func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
	start := time.Now()
	logger := middleware.GetLogger(ctx)

	// Record ingestion metric (environment label removed - SP owns classification)
	s.metricsInstance.AlertsReceivedTotal.WithLabelValues(signal.Source, signal.Severity).Inc()

	// BR-SCOPE-002: Validate resource is within Kubernaut's management scope
	if rejection, err := s.validateScope(ctx, signal); err != nil {
		return nil, err
	} else if rejection != nil {
		return rejection, nil
	}

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
			// Issue #195: Use controllerNamespace — RRs live in controller NS per ADR-057
			shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, s.controllerNamespace, signal.Fingerprint)
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
	// Issue #195: Use controllerNamespace — RRs live in controller NS per ADR-057
	shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, s.controllerNamespace, signal.Fingerprint)
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
		s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.SignalName).Inc()

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

	// 2. CRD creation pipeline
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
	s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.SignalName).Inc()

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
