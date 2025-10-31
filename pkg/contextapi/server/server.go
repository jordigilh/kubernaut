package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/client"
	"github.com/jordigilh/kubernaut/pkg/contextapi/errors"
	"github.com/jordigilh/kubernaut/pkg/contextapi/metrics"
	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
	"github.com/jordigilh/kubernaut/pkg/contextapi/query"
)

// Server is the HTTP server for Context API
// BR-CONTEXT-008: REST API for LLM context
//
// v2.0: Uses v2.0 components (CachedExecutor, CacheManager, Router)
type Server struct {
	router         *query.Router         // v2.0: Query router
	cachedExecutor *query.CachedExecutor // v2.0: Cache-first executor
	dbClient       client.Client         // v2.0: PostgreSQL client
	cacheManager   cache.CacheManager    // v2.0: Multi-tier cache
	metrics        *metrics.Metrics
	logger         *zap.Logger
	httpServer     *http.Server
}

// Config contains server configuration
type Config struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// NewServer creates a new Context API HTTP server
//
// v2.0 Changes:
// - Accepts connection strings instead of pre-initialized components
// - Creates v2.0 components (CachedExecutor, CacheManager, Router)
// - Returns error for initialization failures
func NewServer(
	connStr string, // PostgreSQL connection string
	redisAddr string, // Redis address for caching
	logger *zap.Logger,
	cfg *Config,
) (*Server, error) {
	return NewServerWithMetrics(connStr, redisAddr, logger, cfg, nil)
}

// NewServerWithMetrics creates a new Context API HTTP server with custom metrics
// If metricsInstance is nil, creates default metrics
func NewServerWithMetrics(
	connStr string,
	redisAddr string,
	logger *zap.Logger,
	cfg *Config,
	metricsInstance *metrics.Metrics,
) (*Server, error) {
	// Initialize metrics (use provided or create default)
	var m *metrics.Metrics
	if metricsInstance != nil {
		m = metricsInstance
	} else {
		m = metrics.NewMetrics("context_api", "server")
	}

	// 1. Create PostgreSQL client (Day 1)
	dbClient, err := client.NewPostgresClient(connStr, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create DB client: %w", err)
	}

	// 2. Create cache manager (Day 3)
	// REFACTOR Phase: Parse Redis DB from address (format: "host:port/db")
	// This enables parallel test isolation with separate Redis databases
	redisHost := redisAddr
	redisDB := 0 // Default DB
	if idx := strings.LastIndex(redisAddr, "/"); idx != -1 {
		// Extract DB number from "localhost:6379/3"
		dbStr := redisAddr[idx+1:]
		if db, err := strconv.Atoi(dbStr); err == nil && db >= 0 && db <= 15 {
			redisDB = db
			redisHost = redisAddr[:idx] // Strip "/3" suffix
		}
	}

	cacheConfig := &cache.Config{
		RedisAddr:  redisHost,
		RedisDB:    redisDB,
		LRUSize:    1000,
		DefaultTTL: 5 * time.Minute,
	}
	cacheManager, err := cache.NewCacheManager(cacheConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache manager: %w", err)
	}

	// 3. Create cached executor (Day 4)
	executorCfg := &query.Config{
		DB:    dbClient.GetDB(),
		Cache: cacheManager,
		TTL:   5 * time.Minute,
	}
	cachedExecutor, err := query.NewCachedExecutor(executorCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create cached executor: %w", err)
	}

	// 4. Create aggregation service (Day 6)
	aggregation := query.NewAggregationService(dbClient.GetDB(), cacheManager, logger)

	// 5. Create query router (Day 6) - VectorSearch nil for now (Day 8)
	queryRouter := query.NewRouter(cachedExecutor, nil, aggregation, logger)

	return &Server{
		router:         queryRouter,
		cachedExecutor: cachedExecutor,
		dbClient:       dbClient,
		cacheManager:   cacheManager,
		metrics:        m,
		logger:         logger,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.Port),
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		},
	}, nil
}

// Handler returns the configured HTTP handler for the server
// This is useful for testing with httptest.NewServer
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.loggingMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"}, // TODO: Configure in production
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Health check endpoints
	r.Get("/health", s.handleHealth)
	r.Get("/health/ready", s.handleReadiness)
	r.Get("/health/live", s.handleLiveness)

	// Metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Incident query endpoints (legacy paths for backward compatibility)
		r.Get("/incidents", s.handleListIncidents)
		r.Get("/incidents/{id}", s.handleGetIncident)

		// Context API endpoints (v2.2 standardized paths)
		r.Route("/context", func(r chi.Router) {
			// Day 8 Suite 1 - Test #4: Query endpoint
			// BR-CONTEXT-001: Query historical incident context
			r.Get("/query", s.handleQuery)
		})

		// Aggregation endpoints
		r.Route("/aggregations", func(r chi.Router) {
			r.Get("/success-rate", s.handleSuccessRate)
			r.Get("/namespaces", s.handleNamespaceGrouping)
			r.Get("/severity", s.handleSeverityDistribution)
			r.Get("/trend", s.handleIncidentTrend)
		})

		// TODO: Semantic search endpoint (not yet implemented)
		// r.Post("/search/semantic", s.handleSemanticSearch)
	})

	return r
}

// Start starts the HTTP server
func (s *Server) Start() error {
	// Get configured handler (routes already set up)
	r := s.Handler()
	s.httpServer.Handler = r

	s.logger.Info("Starting Context API server",
		zap.String("addr", s.httpServer.Addr),
	)

	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
// BR-CONTEXT-007: Production Readiness - Graceful shutdown
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Initiating graceful shutdown...")

	// Shutdown HTTP server gracefully
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("Error during shutdown", zap.Error(err))
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	s.logger.Info("Server shutdown complete")
	return nil
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Stopping Context API server")
	return s.httpServer.Shutdown(ctx)
}

// CloseDatabaseConnection closes the database connection for test simulation
// This method is used in integration tests to simulate database unavailability
// BR-CONTEXT-008: Health check testing (Day 8 Suite 1 - Test #3)
func (s *Server) CloseDatabaseConnection() error {
	return s.dbClient.Close()
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Health Check Handlers
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	// BR-CONTEXT-008: Readiness check returns 503 when services are unhealthy
	// Day 8 Suite 1 - Test #3 (DO-GREEN Phase)

	// Check database connectivity
	dbReady := "ready"
	dbHealthy := true
	if err := s.dbClient.Ping(r.Context()); err != nil {
		dbReady = "not_ready"
		dbHealthy = false
		s.logger.Warn("Database not ready", zap.Error(err))
	}

	// Check cache connectivity
	cacheReady := "ready"
	cacheHealthy := true
	// TODO: Implement cache.Ping() method in Day 8 Suite 2 (Cache Fallback)

	// Determine overall readiness status
	// Service is ready only if ALL dependencies are healthy
	overallHealthy := dbHealthy && cacheHealthy

	// Return appropriate HTTP status code
	statusCode := http.StatusOK
	if !overallHealthy {
		statusCode = http.StatusServiceUnavailable // 503
	}

	s.respondJSON(w, statusCode, map[string]interface{}{
		"database": dbReady,
		"cache":    cacheReady,
		"time":     time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleLiveness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Query Handlers
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// handleQuery handles GET /api/v1/context/query requests
// Day 8 Suite 1 - Test #4 (DO-GREEN Phase - Pure TDD)
// BR-CONTEXT-001: Query historical incident context
// BR-CONTEXT-002: Filter by namespace, severity, time range
//
// This is the standardized v2.2 query endpoint that replaces /incidents
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	// Minimal GREEN implementation: delegate to handleListIncidents logic
	// (This avoids code duplication while passing the test)
	s.handleListIncidents(w, r)
}

func (s *Server) handleListIncidents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()

	// Parse query parameters
	params := &models.ListIncidentsParams{
		Namespace: getStringPtr(r.URL.Query().Get("namespace")),
		Phase:     getStringPtr(r.URL.Query().Get("phase")),
		Status:    getStringPtr(r.URL.Query().Get("status")),
		Severity:  getStringPtr(r.URL.Query().Get("severity")),
		Limit:     getIntOrDefault(r.URL.Query().Get("limit"), 10),
		Offset:    getIntOrDefault(r.URL.Query().Get("offset"), 0),
	}

	// Validate parameters
	if params.Limit < 1 || params.Limit > 100 {
		s.respondError(w, r, http.StatusBadRequest, "limit must be between 1 and 100")
		return
	}

	// Execute query via cached executor (uses cache-first, then database)
	// Day 8 Suite 1 - Test #4: Use cachedExecutor instead of dbClient stub
	incidents, total, err := s.cachedExecutor.ListIncidents(ctx, params)
	if err != nil {
		s.logger.Error("Failed to list incidents", zap.Error(err))
		s.metrics.RecordQueryError("list_incidents")
		s.respondError(w, r, http.StatusInternalServerError, "Failed to query incidents")
		return
	}

	// Record metrics
	duration := time.Since(start).Seconds()
	s.metrics.RecordQuerySuccess("list_incidents", duration)

	// Respond
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"incidents": incidents,
		"total":     total,
		"limit":     params.Limit,
		"offset":    params.Offset,
	})
}

func (s *Server) handleGetIncident(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()

	// Parse ID parameter
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.respondError(w, r, http.StatusBadRequest, "Invalid incident ID")
		return
	}

	// Execute query
	incident, err := s.dbClient.GetIncidentByID(ctx, id)
	if err != nil {
		if err.Error() == "incident not found" {
			s.respondError(w, r, http.StatusNotFound, "Incident not found")
			return
		}
		s.logger.Error("Failed to get incident", zap.Error(err), zap.Int64("id", id))
		s.metrics.RecordQueryError("get_incident")
		s.respondError(w, r, http.StatusInternalServerError, "Failed to query incident")
		return
	}

	// Record metrics
	duration := time.Since(start).Seconds()
	s.metrics.RecordQuerySuccess("get_incident", duration)

	// Respond
	s.respondJSON(w, http.StatusOK, incident)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Aggregation Handlers
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func (s *Server) handleSuccessRate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()

	workflowID := r.URL.Query().Get("workflow_id")
	if workflowID == "" {
		s.respondError(w, r, http.StatusBadRequest, "workflow_id parameter required")
		return
	}

	// v2.0: Use aggregation service through router
	result, err := s.router.Aggregation().AggregateSuccessRate(ctx, workflowID)
	if err != nil {
		s.logger.Error("Failed to calculate success rate", zap.Error(err))
		s.metrics.RecordQueryError("success_rate")
		s.respondError(w, r, http.StatusInternalServerError, "Failed to calculate success rate")
		return
	}

	duration := time.Since(start).Seconds()
	s.metrics.RecordQuerySuccess("success_rate", duration)

	s.respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleNamespaceGrouping(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()

	// v2.0: Use aggregation service through router
	groups, err := s.router.Aggregation().GroupByNamespace(ctx)
	if err != nil {
		s.logger.Error("Failed to group by namespace", zap.Error(err))
		s.metrics.RecordQueryError("namespace_grouping")
		s.respondError(w, r, http.StatusInternalServerError, "Failed to group incidents")
		return
	}

	duration := time.Since(start).Seconds()
	s.metrics.RecordQuerySuccess("namespace_grouping", duration)

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"groups": groups,
		"count":  len(groups),
	})
}

func (s *Server) handleSeverityDistribution(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()

	namespace := r.URL.Query().Get("namespace")

	// v2.0: Use aggregation service through router
	distribution, err := s.router.Aggregation().GetSeverityDistribution(ctx, namespace)
	if err != nil {
		s.logger.Error("Failed to get severity distribution", zap.Error(err))
		s.metrics.RecordQueryError("severity_distribution")
		s.respondError(w, r, http.StatusInternalServerError, "Failed to calculate distribution")
		return
	}

	duration := time.Since(start).Seconds()
	s.metrics.RecordQuerySuccess("severity_distribution", duration)

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"distribution": distribution,
		"namespace":    namespace,
	})
}

func (s *Server) handleIncidentTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()

	days := getIntOrDefault(r.URL.Query().Get("days"), 30)
	if days < 1 || days > 365 {
		s.respondError(w, r, http.StatusBadRequest, "days must be between 1 and 365")
		return
	}

	// v2.0: Use aggregation service through router
	trend, err := s.router.Aggregation().GetIncidentTrend(ctx, days)
	if err != nil {
		s.logger.Error("Failed to get incident trend", zap.Error(err))
		s.metrics.RecordQueryError("incident_trend")
		s.respondError(w, r, http.StatusInternalServerError, "Failed to calculate trend")
		return
	}

	duration := time.Since(start).Seconds()
	s.metrics.RecordQuerySuccess("incident_trend", duration)

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"trend": trend,
		"days":  days,
	})
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Semantic Search Handler
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func (s *Server) handleSemanticSearch(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req struct {
		Query     string  `json:"query"`
		Limit     int     `json:"limit"`
		Threshold float64 `json:"threshold"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Query == "" {
		s.respondError(w, r, http.StatusBadRequest, "query parameter required")
		return
	}

	// TODO: Implement semantic search in Day 8 with vector DB integration
	// For now, return placeholder response
	duration := time.Since(start).Seconds()
	s.metrics.RecordQuerySuccess("semantic_search", duration)

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"query":   req.Query,
		"results": []interface{}{},
		"message": "Semantic search will be implemented in Day 8 (integration testing)",
	})
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Middleware
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(ww.Status())

		// Record metrics
		s.metrics.RecordHTTPRequest(r.Method, r.URL.Path, status, duration)

		// Log request
		s.logger.Info("HTTP request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", ww.Status()),
			zap.Duration("duration", time.Since(start)),
			zap.String("remote", r.RemoteAddr),
		)
	})
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Helper Functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError writes an RFC 7807 compliant error response
// DD-004: RFC 7807 Error Response Standard
// BR-CONTEXT-009: Consistent error responses
//
// GREEN PHASE: Minimal implementation to make tests pass
func (s *Server) respondError(w http.ResponseWriter, r *http.Request, status int, message string) {
	// Set RFC 7807 Content-Type header
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	// Extract request ID from middleware for tracing
	requestID := middleware.GetReqID(r.Context())

	// Determine error type and title based on status code
	errorType, title := getErrorTypeAndTitle(status)

	// Build RFC 7807 error response
	errorResponse := errors.RFC7807Error{
		Type:      errorType,
		Title:     title,
		Detail:    message,
		Status:    status,
		Instance:  r.URL.Path,
		RequestID: requestID,
	}

	// Encode and send response
	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		// Fallback to plain text if JSON encoding fails
		s.logger.Error("Failed to encode RFC 7807 error response", zap.Error(err))
		http.Error(w, message, status)
	}
}

// getErrorTypeAndTitle maps HTTP status codes to RFC 7807 error types and titles
// DD-004: Error Type URI Convention
//
// GREEN PHASE: Minimal implementation covering test cases
func getErrorTypeAndTitle(statusCode int) (string, string) {
	switch statusCode {
	case http.StatusBadRequest:
		return errors.ErrorTypeValidationError, errors.TitleBadRequest
	case http.StatusNotFound:
		return errors.ErrorTypeNotFound, errors.TitleNotFound
	case http.StatusMethodNotAllowed:
		return errors.ErrorTypeMethodNotAllowed, errors.TitleMethodNotAllowed
	case http.StatusUnsupportedMediaType:
		return errors.ErrorTypeUnsupportedMediaType, errors.TitleUnsupportedMediaType
	case http.StatusInternalServerError:
		return errors.ErrorTypeInternalError, errors.TitleInternalServerError
	case http.StatusServiceUnavailable:
		return errors.ErrorTypeServiceUnavailable, errors.TitleServiceUnavailable
	case http.StatusGatewayTimeout:
		return errors.ErrorTypeGatewayTimeout, errors.TitleGatewayTimeout
	default:
		return errors.ErrorTypeUnknown, errors.TitleUnknown
	}
}

func getStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func getIntOrDefault(s string, def int) int {
	if s == "" {
		return def
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return val
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// GREEN PHASE IMPLEMENTATION NOTES
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Business Requirements:
// - BR-CONTEXT-008: REST API for LLM context (fully implemented)
// - BR-CONTEXT-006: Observability (metrics + health checks)
//
// Endpoints Implemented:
// 1. Health: /health, /health/ready, /health/live
// 2. Metrics: /metrics (Prometheus)
// 3. Query: GET /api/v1/incidents, GET /api/v1/incidents/:id
// 4. Aggregation: 4 endpoints (success-rate, namespaces, severity, trend)
// 5. Search: POST /api/v1/search/semantic (placeholder for Day 8)
//
// Middleware:
// - Logging with zap
// - Request ID tracking
// - Recovery from panics
// - CORS support
// - Metrics recording
//
// Architecture Alignment:
// - Read-only operations (no writes)
// - Integrates with Router and AggregationService from Day 6
// - Queries resource_action_traces table via dbClient (DD-SCHEMA-001)
// - Comprehensive error handling
// - Performance tracking via Prometheus
//
// REFACTOR Phase (Next):
// - Add authentication middleware (Istio integration)
// - Add rate limiting
// - Add request validation middleware
// - Enhance error responses with error codes
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
