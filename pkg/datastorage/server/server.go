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

package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	dsmetrics "github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver (DD-010: Migrated from lib/pq)
)

// Server is the HTTP server for Data Storage Service
// BR-STORAGE-021: REST API read endpoints
// BR-STORAGE-024: RFC 7807 error responses
//
// DD-007: Kubernetes-aware graceful shutdown with 4-step pattern
type Server struct {
	handler    *Handler
	db         *sql.DB
	logger     *zap.Logger
	httpServer *http.Server

	// DD-007: Graceful shutdown coordination flag
	// Thread-safe flag for readiness probe coordination during shutdown
	isShuttingDown atomic.Bool

	// BR-STORAGE-001 to BR-STORAGE-020: Audit write API dependencies
	repository *repository.NotificationAuditRepository
	dlqClient  *dlq.Client
	validator  *validation.NotificationAuditValidator

	// BR-STORAGE-019: Prometheus metrics (GAP-10)
	metrics *dsmetrics.Metrics
}

// DD-007 graceful shutdown constants
const (
	// endpointRemovalPropagationDelay is the time to wait for Kubernetes to propagate
	// endpoint removal across all nodes. Industry best practice is 5 seconds.
	// Kubernetes typically takes 1-3 seconds, but we wait longer to be safe.
	endpointRemovalPropagationDelay = 5 * time.Second

	// drainTimeout is the maximum time to wait for in-flight requests to complete
	drainTimeout = 30 * time.Second
)

// Config contains server configuration
type Config struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// NewServer creates a new Data Storage HTTP server
// BR-STORAGE-021: REST API Gateway for database access
// BR-STORAGE-001 to BR-STORAGE-020: Audit write API
//
// Parameters:
// - dbConnStr: PostgreSQL connection string (format: "host=localhost port=5432 dbname=action_history user=slm_user password=xxx sslmode=disable")
// - redisAddr: Redis address for DLQ (format: "localhost:6379")
// - logger: Structured logger
// - cfg: Server configuration
func NewServer(
	dbConnStr string,
	redisAddr string,
	logger *zap.Logger,
	cfg *Config,
) (*Server, error) {
	// Connect to PostgreSQL using pgx driver (DD-010)
	db, err := sql.Open("pgx", dbConnStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		_ = db.Close() // Best effort close on failed ping
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	// Configure connection pool for production
	db.SetMaxOpenConns(25)                  // Maximum connections
	db.SetMaxIdleConns(5)                   // Idle connections
	db.SetConnMaxLifetime(5 * time.Minute)  // Connection lifetime
	db.SetConnMaxIdleTime(10 * time.Minute) // Idle connection timeout

	logger.Info("PostgreSQL connection established",
		zap.Int("max_open_conns", 25),
		zap.Int("max_idle_conns", 5),
	)

	// Connect to Redis for DLQ (DD-009)
	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		_ = db.Close() // Clean up DB connection
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Redis connection established",
		zap.String("addr", redisAddr),
	)

	// Create audit write dependencies (BR-STORAGE-001 to BR-STORAGE-020)
	repo := repository.NewNotificationAuditRepository(db, logger)
	dlqClient := dlq.NewClient(redisClient, logger)
	validator := validation.NewNotificationAuditValidator()

	// Create Prometheus metrics (BR-STORAGE-019, GAP-10)
	metrics := dsmetrics.NewMetrics("datastorage", "")

	logger.Info("Prometheus metrics initialized",
		zap.String("namespace", "datastorage"),
	)

	// Create database wrapper for READ API handlers
	dbAdapter := &DBAdapter{db: db, logger: logger}

	// Create READ API handler with logger
	handler := NewHandler(dbAdapter, WithLogger(logger))

	return &Server{
		handler:    handler,
		db:         db,
		logger:     logger,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.Port),
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		},
		repository: repo,
		dlqClient:  dlqClient,
		validator:  validator,
		metrics:    metrics,
	}, nil
}

// Handler returns the configured HTTP handler for the server
// This is useful for testing with httptest.NewServer
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID) // Add X-Request-ID
	r.Use(middleware.RealIP)    // Get real client IP
	r.Use(s.loggingMiddleware)  // Custom logging middleware
	r.Use(middleware.Recoverer) // Recover from panics
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"}, // TODO: Configure in production
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Health check endpoints (DD-007: Graceful shutdown support)
	r.Get("/health", s.handleHealth)
	r.Get("/health/ready", s.handleReadiness)
	r.Get("/health/live", s.handleLiveness)

	// BR-STORAGE-019: Prometheus metrics endpoint (GAP-10)
	// Exposes audit-specific metrics:
	// - datastorage_audit_traces_total{service,status}
	// - datastorage_audit_lag_seconds{service}
	// - datastorage_write_duration_seconds{table}
	// - datastorage_validation_failures_total{field,reason}
	r.Handle("/metrics", promhttp.Handler())

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// BR-STORAGE-021: Incident query endpoints (READ API)
		r.Get("/incidents", s.handler.ListIncidents)
		r.Get("/incidents/{id}", s.handler.GetIncident)

		// BR-STORAGE-001 to BR-STORAGE-020: Audit write endpoints (WRITE API)
		r.Post("/audit/notifications", s.handleCreateNotificationAudit)
	})

	return r
}

// Start starts the HTTP server
func (s *Server) Start() error {
	// Get configured handler (routes already set up)
	r := s.Handler()
	s.httpServer.Handler = r

	s.logger.Info("Starting Data Storage Service server",
		zap.String("addr", s.httpServer.Addr),
	)

	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server following DD-007 pattern
// DD-007: Kubernetes-Aware Graceful Shutdown (4-Step Pattern)
//
// This implements the production-proven pattern from Gateway/Context API services
// to achieve ZERO request failures during rolling updates
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

	// STEP 4: Close external resources (database)
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
	s.logger.Info("Waiting for Kubernetes endpoint removal to propagate",
		zap.Duration("delay", endpointRemovalPropagationDelay),
		zap.String("dd", "DD-007-step-2"))
	time.Sleep(endpointRemovalPropagationDelay)
	s.logger.Info("Endpoint propagation complete - now draining connections",
		zap.String("dd", "DD-007-step-2-complete"))
}

// shutdownStep3DrainConnections drains in-flight HTTP connections
// DD-007 STEP 3: Gracefully close HTTP connections with timeout
func (s *Server) shutdownStep3DrainConnections(ctx context.Context) error {
	s.logger.Info("Draining in-flight HTTP connections",
		zap.Duration("drain_timeout", drainTimeout),
		zap.String("dd", "DD-007-step-3"))

	// Create timeout context for draining
	drainCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()

	// Override parent context if it would timeout sooner
	if deadline, ok := ctx.Deadline(); ok {
		if time.Until(deadline) < drainTimeout {
			drainCtx = ctx
		}
	}

	if err := s.httpServer.Shutdown(drainCtx); err != nil {
		s.logger.Error("Error during HTTP connection drain",
			zap.Error(err),
			zap.String("dd", "DD-007-step-3-error"))
		return fmt.Errorf("HTTP connection drain failed: %w", err)
	}

	s.logger.Info("HTTP connections drained successfully",
		zap.String("dd", "DD-007-step-3-complete"))
	return nil
}

// shutdownStep4CloseResources closes external resources (database)
// DD-007 STEP 4: Clean up database connections
func (s *Server) shutdownStep4CloseResources() error {
	s.logger.Info("Closing external resources (PostgreSQL)",
		zap.String("dd", "DD-007-step-4"))

	// Close PostgreSQL connection
	if err := s.db.Close(); err != nil {
		s.logger.Error("Failed to close PostgreSQL connection",
			zap.Error(err),
			zap.String("dd", "DD-007-step-4-error"))
		return fmt.Errorf("failed to close PostgreSQL: %w", err)
	}

	s.logger.Info("All external resources closed",
		zap.String("dd", "DD-007-step-4-complete"))
	return nil
}

// Health check handlers

// handleHealth handles GET /health - overall health check
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check database connectivity
	if err := s.db.Ping(); err != nil {
		s.logger.Error("Health check failed - database unreachable",
			zap.Error(err))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, `{"status":"unhealthy","database":"unreachable","error":"%s"}`, err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, `{"status":"healthy","database":"connected"}`)
}

// handleReadiness handles GET /health/ready - readiness probe for Kubernetes
// DD-007: Returns 503 during shutdown to remove pod from endpoints
func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	// DD-007: Check shutdown flag first
	if s.isShuttingDown.Load() {
		s.logger.Debug("Readiness probe returning 503 - shutdown in progress")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, `{"status":"not_ready","reason":"shutting_down"}`)
		return
	}

	// Check database connectivity
	if err := s.db.Ping(); err != nil {
		s.logger.Warn("Readiness probe failed - database unreachable",
			zap.Error(err))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, `{"status":"not_ready","reason":"database_unreachable","error":"%s"}`, err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, `{"status":"ready"}`)
}

// handleLiveness handles GET /health/live - liveness probe for Kubernetes
func (s *Server) handleLiveness(w http.ResponseWriter, r *http.Request) {
	// Liveness is always true unless the process is completely stuck
	// Don't check database here - that's the readiness probe's job
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, `{"status":"alive"}`)
}

// loggingMiddleware logs HTTP requests with structured logging
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Get request ID from middleware.RequestID
		requestID := middleware.GetReqID(r.Context())

		// Create a response writer wrapper to capture status code
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// Call next handler
		next.ServeHTTP(ww, r)

		// Log request with timing
		duration := time.Since(start)
		s.logger.Info("HTTP request",
			zap.String("request_id", requestID),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", r.RemoteAddr),
			zap.Int("status", ww.Status()),
			zap.Int("bytes", ww.BytesWritten()),
			zap.Duration("duration", duration),
		)
	})
}

// DBAdapter adapts sql.DB to work with our Handler
// Day 3: Real database implementation using query builder
type DBAdapter struct {
	db     *sql.DB
	logger *zap.Logger
}

// Query executes a filtered query against PostgreSQL
// BR-STORAGE-021: Query database with filters and pagination
// BR-STORAGE-022: Apply dynamic filters
// BR-STORAGE-023: Pagination support
func (d *DBAdapter) Query(filters map[string]string, limit, offset int) ([]map[string]interface{}, error) {
	d.logger.Debug("DBAdapter.Query called",
		zap.Any("filters", filters),
		zap.Int("limit", limit),
		zap.Int("offset", offset),
	)

	// Build query using query builder
	builder := query.NewBuilder(query.WithLogger(d.logger))

	// Apply filters
	if ns, ok := filters["namespace"]; ok && ns != "" {
		builder = builder.WithNamespace(ns)
	}
	if alertName, ok := filters["alert_name"]; ok && alertName != "" {
		builder = builder.WithAlertName(alertName)
	}
	if sev, ok := filters["severity"]; ok && sev != "" {
		builder = builder.WithSeverity(sev)
	}
	if cluster, ok := filters["cluster"]; ok && cluster != "" {
		builder = builder.WithCluster(cluster)
	}
	if env, ok := filters["environment"]; ok && env != "" {
		builder = builder.WithEnvironment(env)
	}
	if actionType, ok := filters["action_type"]; ok && actionType != "" {
		builder = builder.WithActionType(actionType)
	}

	// Apply pagination
	builder = builder.WithLimit(limit).WithOffset(offset)

	// Build SQL query
	sqlQuery, args, err := builder.Build()
	if err != nil {
		d.logger.Error("Failed to build SQL query",
			zap.Error(err),
			zap.Any("filters", filters),
		)
		return nil, fmt.Errorf("query builder error: %w", err)
	}

	// Convert ? placeholders back to PostgreSQL $1, $2, etc.
	// (query builder uses ? for test compatibility, but PostgreSQL needs $N)
	pgQuery := convertPlaceholdersToPostgreSQL(sqlQuery, len(args))

	d.logger.Debug("Executing SQL query",
		zap.String("sql", pgQuery),
		zap.Int("arg_count", len(args)),
	)

	// Execute query
	rows, err := d.db.Query(pgQuery, args...)
	if err != nil {
		d.logger.Error("Failed to execute SQL query",
			zap.Error(err),
			zap.String("sql", pgQuery),
		)
		return nil, fmt.Errorf("database query error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		d.logger.Error("Failed to get column names",
			zap.Error(err),
		)
		return nil, fmt.Errorf("column retrieval error: %w", err)
	}

	// Scan results into map slices
	results := make([]map[string]interface{}, 0)
	for rows.Next() {
		// Create slice for scanning
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan row
		if err := rows.Scan(valuePtrs...); err != nil {
			d.logger.Error("Failed to scan row",
				zap.Error(err),
			)
			return nil, fmt.Errorf("row scan error: %w", err)
		}

		// Convert to map
		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	// Check for iteration errors
	if err := rows.Err(); err != nil {
		d.logger.Error("Row iteration error",
			zap.Error(err),
		)
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	d.logger.Info("Query executed successfully",
		zap.Int("result_count", len(results)),
		zap.Int("limit", limit),
		zap.Int("offset", offset),
	)

	return results, nil
}

// CountTotal returns the total number of records matching the filters
// 🚨 FIX: Separate COUNT(*) query for accurate pagination metadata
// This fixes the critical bug where pagination.total was set to len(array) instead of database count
// See: docs/services/stateless/data-storage/implementation/DATA-STORAGE-INTEGRATION-TEST-TRIAGE.md
func (d *DBAdapter) CountTotal(filters map[string]string) (int64, error) {
	d.logger.Debug("DBAdapter.CountTotal called",
		zap.Any("filters", filters),
	)

	// Build count query using query builder
	builder := query.NewBuilder(query.WithLogger(d.logger))

	// Apply filters (same as Query method)
	if ns, ok := filters["namespace"]; ok && ns != "" {
		builder = builder.WithNamespace(ns)
	}
	if alertName, ok := filters["alert_name"]; ok && alertName != "" {
		builder = builder.WithAlertName(alertName)
	}
	if sev, ok := filters["severity"]; ok && sev != "" {
		builder = builder.WithSeverity(sev)
	}
	if cluster, ok := filters["cluster"]; ok && cluster != "" {
		builder = builder.WithCluster(cluster)
	}
	if env, ok := filters["environment"]; ok && env != "" {
		builder = builder.WithEnvironment(env)
	}
	if actionType, ok := filters["action_type"]; ok && actionType != "" {
		builder = builder.WithActionType(actionType)
	}

	// Build SQL query for count
	sqlQuery, args, err := builder.BuildCount()
	if err != nil {
		d.logger.Error("Failed to build COUNT query",
			zap.Error(err),
			zap.Any("filters", filters),
		)
		return 0, fmt.Errorf("count query builder error: %w", err)
	}

	// Convert ? placeholders to PostgreSQL $1, $2, etc.
	pgQuery := convertPlaceholdersToPostgreSQL(sqlQuery, len(args))

	d.logger.Debug("Executing COUNT query",
		zap.String("sql", pgQuery),
		zap.Int("arg_count", len(args)),
	)

	// Execute count query
	var count int64
	err = d.db.QueryRow(pgQuery, args...).Scan(&count)
	if err != nil {
		d.logger.Error("Failed to execute COUNT query",
			zap.Error(err),
			zap.String("sql", pgQuery),
		)
		return 0, fmt.Errorf("count query error: %w", err)
	}

	d.logger.Info("COUNT query executed successfully",
		zap.Int64("total_count", count),
	)

	return count, nil
}

// Get retrieves a single incident by ID
// BR-STORAGE-021: Get incident by ID
func (d *DBAdapter) Get(id int) (map[string]interface{}, error) {
	d.logger.Debug("DBAdapter.Get called",
		zap.Int("id", id),
	)

	// Query for specific ID
	// Note: Using direct SQL here since it's a simple ID lookup
	sqlQuery := `
		SELECT *
		FROM resource_action_traces
		WHERE id = $1
		LIMIT 1
	`

	rows, err := d.db.Query(sqlQuery, id)
	if err != nil {
		d.logger.Error("Failed to execute Get query",
			zap.Error(err),
			zap.Int("id", id),
		)
		return nil, fmt.Errorf("database query error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Check if any rows returned
	if !rows.Next() {
		d.logger.Debug("No incident found with ID",
			zap.Int("id", id),
		)
		return nil, nil // Not found
	}

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		d.logger.Error("Failed to get column names",
			zap.Error(err),
		)
		return nil, fmt.Errorf("column retrieval error: %w", err)
	}

	// Create slice for scanning
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	// Scan row
	if err := rows.Scan(valuePtrs...); err != nil {
		d.logger.Error("Failed to scan row",
			zap.Error(err),
			zap.Int("id", id),
		)
		return nil, fmt.Errorf("row scan error: %w", err)
	}

	// Convert to map
	result := make(map[string]interface{})
	for i, col := range columns {
		result[col] = values[i]
	}

	d.logger.Info("Incident retrieved successfully",
		zap.Int("id", id),
	)

	return result, nil
}

// convertPlaceholdersToPostgreSQL converts ? placeholders to PostgreSQL $1, $2, etc.
func convertPlaceholdersToPostgreSQL(sql string, argCount int) string {
	result := sql
	for i := 1; i <= argCount; i++ {
		// Replace first occurrence of ? with $N
		// We need to replace in order since builder creates them in order
		result = replaceFirstOccurrence(result, "?", fmt.Sprintf("$%d", i))
	}
	return result
}

// replaceFirstOccurrence replaces the first occurrence of old with new in s
func replaceFirstOccurrence(s, old, new string) string {
	i := 0
	for {
		j := i
		for ; j < len(s); j++ {
			if j+len(old) > len(s) {
				return s
			}
			if s[j:j+len(old)] == old {
				return s[:j] + new + s[j+len(old):]
			}
		}
		if j >= len(s) {
			return s
		}
		i = j + 1
	}
}
