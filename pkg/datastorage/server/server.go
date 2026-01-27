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
	"crypto/tls"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-logr/logr"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/cert"
	"github.com/jordigilh/kubernaut/pkg/datastorage/adapter"
	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	dsmetrics "github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	dsmiddleware "github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver (DD-010: Migrated from lib/pq)
)

// Server is the HTTP server for Data Storage Service
// BR-STORAGE-021: REST API read endpoints
// BR-STORAGE-024: RFC 7807 error responses
//
// DD-007: Kubernetes-aware graceful shutdown with 4-step pattern
// DD-AUTH-014: Middleware-based authentication and authorization
type Server struct {
	handler    *Handler
	db         *sql.DB
	logger     logr.Logger
	httpServer *http.Server

	// DD-007: Graceful shutdown coordination flag
	// Thread-safe flag for readiness probe coordination during shutdown
	isShuttingDown atomic.Bool

	// BR-STORAGE-001 to BR-STORAGE-020: Audit write API dependencies
	repository *repository.NotificationAuditRepository
	dlqClient  *dlq.Client
	validator  *validation.NotificationAuditValidator

	// BR-STORAGE-033: Unified audit events API (ADR-034)
	// SOC2 Gap #9: PostgreSQL-based with custom hash chains for tamper detection
	auditEventsRepo *repository.AuditEventsRepository

	// BR-STORAGE-012: Self-auditing (DD-STORAGE-012)
	// Uses InternalAuditClient to avoid circular dependency
	auditStore audit.AuditStore

	// BR-STORAGE-019: Prometheus metrics (GAP-10)
	metrics *dsmetrics.Metrics

	// SOC2 Day 9.1: Digital signature for audit exports
	// BR-AUDIT-007: Signed exports for tamper detection
	signer *cert.Signer

	// DD-AUTH-014: Authentication and authorization via dependency injection
	// Authenticator validates tokens (TokenReview)
	// Authorizer checks permissions (SubjectAccessReview)
	// Production: Real K8s APIs, Integration: Mocks, E2E: Real K8s APIs
	authenticator auth.Authenticator
	authorizer    auth.Authorizer
	authNamespace string // Namespace for SAR checks (dynamically determined from pod)
}

// DD-007 + DD-008 graceful shutdown constants
const (
	// endpointRemovalPropagationDelay is the time to wait for Kubernetes to propagate
	// endpoint removal across all nodes. Industry best practice is 5 seconds.
	// Kubernetes typically takes 1-3 seconds, but we wait longer to be safe.
	endpointRemovalPropagationDelay = 5 * time.Second

	// drainTimeout is the maximum time to wait for in-flight requests to complete
	drainTimeout = 30 * time.Second

	// dlqDrainTimeout is the maximum time to drain DLQ messages during shutdown (DD-008)
	// This ensures audit messages in the DLQ are persisted before shutdown
	dlqDrainTimeout = 10 * time.Second
)

// NewServer creates a new Data Storage HTTP server
// BR-STORAGE-021: REST API Gateway for database access
// BR-STORAGE-001 to BR-STORAGE-020: Audit write API
// SOC2 Gap #9: PostgreSQL with custom hash chains for tamper detection
// DD-AUTH-014: Middleware-based authentication and authorization
//
// Parameters:
// - dbConnStr: PostgreSQL connection string (format: "host=localhost port=5432 dbname=action_history user=slm_user password=xxx sslmode=disable")
// - redisAddr: Redis address for DLQ (format: "localhost:6379")
// - redisPassword: Redis password (from mounted secret)
// - logger: Structured logger
// - appCfg: Full application configuration (includes database pool settings)
// - serverCfg: Server-specific configuration (port, timeouts)
// - dlqMaxLen: Maximum DLQ stream length for capacity monitoring (Gap 3.3)
// - authenticator: Token validator (DD-AUTH-014) - K8s implementation for production/E2E, mock for integration tests
// - authorizer: Permission checker (DD-AUTH-014) - K8s implementation for production/E2E, mock for integration tests
// - authNamespace: Namespace for SAR checks (DD-AUTH-014) - dynamically determined from pod's ServiceAccount
func NewServer(
	dbConnStr string,
	redisAddr string,
	redisPassword string,
	logger logr.Logger,
	appCfg *config.Config,
	serverCfg *Config,
	dlqMaxLen int64,
	authenticator auth.Authenticator,
	authorizer auth.Authorizer,
	authNamespace string,
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

	// Configure connection pool from config (not hardcoded)
	// Bug fix: Use appCfg.Database values instead of hardcoded 25/5
	// Issue discovered: 2026-01-14 - Integration tests with 12 parallel processes
	// were bottlenecked by hardcoded max_open_conns=25
	db.SetMaxOpenConns(appCfg.Database.MaxOpenConns)
	db.SetMaxIdleConns(appCfg.Database.MaxIdleConns)

	// Parse duration strings from config
	connMaxLifetime, err := time.ParseDuration(appCfg.Database.ConnMaxLifetime)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("invalid connMaxLifetime: %w", err)
	}
	db.SetConnMaxLifetime(connMaxLifetime)

	connMaxIdleTime, err := time.ParseDuration(appCfg.Database.ConnMaxIdleTime)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("invalid connMaxIdleTime: %w", err)
	}
	db.SetConnMaxIdleTime(connMaxIdleTime)

	logger.Info("PostgreSQL connection established",
		"maxOpenConns", appCfg.Database.MaxOpenConns,
		"maxIdleConns", appCfg.Database.MaxIdleConns,
		"connMaxLifetime", appCfg.Database.ConnMaxLifetime,
		"connMaxIdleTime", appCfg.Database.ConnMaxIdleTime,
	)

	// Connect to Redis for DLQ (DD-009)
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword, // ADR-030: Password from mounted secret
	})
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		_ = db.Close() // Clean up DB connection
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Redis connection established",
		"addr", redisAddr,
	)

	// Create audit write dependencies (BR-STORAGE-001 to BR-STORAGE-020)
	logger.V(1).Info("Creating audit write dependencies...")
	repo := repository.NewNotificationAuditRepository(db, logger)
	// Gap 3.3: Use passed DLQ max length for capacity monitoring
	if dlqMaxLen <= 0 {
		dlqMaxLen = 10000 // Default if not configured
	}
	dlqClient, err := dlq.NewClient(redisClient, logger, dlqMaxLen)
	if err != nil {
		return nil, fmt.Errorf("failed to create DLQ client: %w", err)
	}
	validator := validation.NewNotificationAuditValidator()

	logger.V(1).Info("Audit write dependencies created",
		"repo_nil", repo == nil,
		"dlq_client_nil", dlqClient == nil,
		"validator_nil", validator == nil)

	// Create ADR-033 action trace repository (BR-STORAGE-031-01, BR-STORAGE-031-02)
	logger.V(1).Info("Creating ADR-033 action trace repository...")
	actionTraceRepo := repository.NewActionTraceRepository(db, logger)
	logger.V(1).Info("ADR-033 action trace repository created",
		"action_trace_repo_nil", actionTraceRepo == nil)

	// Create BR-STORAGE-033: Unified audit events repository (ADR-034)
	// SOC2 Gap #9: PostgreSQL with custom hash chains for tamper detection
	logger.V(1).Info("Creating ADR-034 unified audit events repository (PostgreSQL)...")
	auditEventsRepo := repository.NewAuditEventsRepository(db, logger)
	logger.V(1).Info("ADR-034 audit events repository created (PostgreSQL-backed, SOC2 Gap #9)",
		"audit_events_repo_nil", auditEventsRepo == nil)

	// Create BR-STORAGE-012: Self-auditing audit store (DD-STORAGE-012)
	// Uses InternalAuditClient to avoid circular dependency (cannot call own REST API)
	logger.V(1).Info("Creating self-auditing audit store (DD-STORAGE-012)...")
	internalClient := audit.NewInternalAuditClient(db)

	// Create audit store with logr logger (DD-005 v2.0: Unified logging interface)
	auditStore, err := audit.NewBufferedStore(
		internalClient,
		audit.RecommendedConfig("datastorage"), // DD-AUDIT-004: HIGH tier (50K buffer)
		"datastorage",                          // service name
		logger,                                 // Use logr.Logger directly (DD-005 v2.0)
	)
	if err != nil {
		_ = db.Close() // Clean up DB connection
		return nil, fmt.Errorf("failed to create audit store: %w", err)
	}

	logger.Info("Self-auditing audit store initialized (DD-STORAGE-012)",
		"buffer_size", audit.DefaultConfig().BufferSize,
		"batch_size", audit.DefaultConfig().BatchSize,
		"flush_interval", audit.DefaultConfig().FlushInterval,
		"max_retries", audit.DefaultConfig().MaxRetries,
	)

	// Create Prometheus metrics (BR-STORAGE-019, GAP-10)
	// Note: NewMetrics always returns a valid *Metrics (never nil)
	metrics := dsmetrics.NewMetrics("datastorage", "")

	logger.Info("Prometheus metrics initialized",
		"namespace", "datastorage",
	)

	// BR-STORAGE-013, BR-STORAGE-014: Create workflow catalog dependencies
	logger.V(1).Info("Creating workflow catalog dependencies...")
	sqlxDB := sqlx.NewDb(db, "pgx") // Wrap *sql.DB with sqlx for workflow repository

	// V1.0: Embedding service removed (label-only search)
	// Authority: CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md (92% confidence)
	// Workflow repository no longer requires embedding client
	// V1.0: Label-only search (embedding client removed)
	workflowRepo := repository.NewWorkflowRepository(sqlxDB, logger)

	logger.V(1).Info("Workflow catalog dependencies created (label-only search)",
		"workflow_repo_nil", workflowRepo == nil)

	// Create database adapter for READ API handlers
	dbAdapter := adapter.NewDBAdapter(db, logger)

	// Create READ API handler with logger, ADR-033 repository, workflow catalog, and audit store
	// V1.0: Embedding service removed (label-only search)
	// BR-AUDIT-006: Pass sqlDB for reconstruction queries
	handler := NewHandler(dbAdapter,
		WithLogger(logger),
		WithActionTraceRepository(actionTraceRepo),
		WithWorkflowRepository(workflowRepo),
		WithAuditStore(auditStore),
		WithSQLDB(db))

	// SOC2 Day 9.1: Load signing certificate for audit exports
	// BR-AUDIT-007: Digital signatures for tamper-evident audit exports
	logger.V(1).Info("Loading signing certificate from /etc/certs...")
	signer, err := loadSigningCertificate(logger)
	if err != nil {
		_ = db.Close() // Clean up DB connection
		return nil, fmt.Errorf("failed to load signing certificate: %w", err)
	}
	logger.Info("Signing certificate loaded successfully",
		"algorithm", signer.GetAlgorithm(),
		"fingerprint", signer.GetCertificateFingerprint())

	// DS-FLAKY-003 FIX: Create server with handler assigned to httpServer
	// This allows graceful shutdown to work in both Start() and httptest scenarios
	// Previously, handler was only assigned in Start(), causing Shutdown() to hang in tests
	srv := &Server{
		handler: handler,
		db:      db,
		logger:  logger,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", serverCfg.Port),
			ReadTimeout:  serverCfg.ReadTimeout,
			WriteTimeout: serverCfg.WriteTimeout,
		},
		repository:      repo,
		dlqClient:       dlqClient,
		validator:       validator,
		auditEventsRepo: auditEventsRepo,
		auditStore:      auditStore,
		metrics:         metrics,
		signer:          signer,
		authenticator:   authenticator,   // DD-AUTH-014: Injected at runtime
		authorizer:      authorizer,      // DD-AUTH-014: Injected at runtime
		authNamespace:   authNamespace,   // DD-AUTH-014: Dynamic namespace for SAR checks
	}

	// DS-FLAKY-003 FIX: Assign handler immediately so Shutdown() can work
	srv.httpServer.Handler = srv.Handler()

	return srv, nil
}

// Handler returns the configured HTTP handler for the server
// This is useful for testing with httptest.NewServer
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)      // Add X-Request-ID
	r.Use(middleware.RealIP)         // Get real client IP
	r.Use(s.loggingMiddleware)       // Custom logging middleware
	r.Use(s.panicRecoveryMiddleware) // Enhanced panic recovery with logging
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"}, // V1.0: Permissive CORS (configure via env var in production)
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// BR-STORAGE-034: OpenAPI validation middleware
	// BR-STORAGE-019: Prometheus metrics for validation failures
	// DD-API-002: OpenAPI spec embedded in binary (zero-config deployment)
	//
	// Automatically validates all API requests against OpenAPI spec:
	// - Validates required fields (including empty strings via minLength: 1)
	// - Validates enum values, types, formats
	// - Returns RFC 7807 errors for validation failures
	// - Emits Prometheus metrics for observability
	// Routes not in spec (/health, /metrics) pass through without validation
	// Metrics are validated at constructor time (non-nil guaranteed)
	validationMetrics := s.metrics.ValidationFailures
	openapiValidator, err := dsmiddleware.NewOpenAPIValidator(
		s.logger.WithName("openapi-validator"),
		validationMetrics,
	)
	if err != nil {
		s.logger.Error(err, "Failed to initialize OpenAPI validator - continuing without validation")
	} else {
		r.Use(openapiValidator.Middleware)
		s.logger.Info("OpenAPI validation middleware enabled")
	}

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
	s.logger.V(1).Info("Setting up API v1 routes",
		"handler_nil", s.handler == nil,
		"repository_nil", s.repository == nil,
		"validator_nil", s.validator == nil,
		"dlq_client_nil", s.dlqClient == nil)

	r.Route("/api/v1", func(r chi.Router) {
		// DD-AUTH-014: Authentication and authorization middleware
		// Applied to all /api/v1 routes (excludes /health, /metrics)
		// Authority: DD-AUTH-011 (SAR with verb:"create" for all audit write operations)
		if s.authenticator != nil && s.authorizer != nil {
			authMiddleware := dsmiddleware.NewAuthMiddleware(
				s.authenticator,
				s.authorizer,
				dsmiddleware.AuthConfig{
					Namespace:    s.authNamespace,
					Resource:     "services",
					ResourceName: "data-storage-service",
					Verb:         "create", // DD-AUTH-014: All services need audit write permissions
				},
				s.logger,
			)
			r.Use(authMiddleware.Handler)
			s.logger.Info("Auth middleware enabled (DD-AUTH-014)",
				"namespace", s.authNamespace,
				"resource", "services",
				"resourceName", "data-storage-service",
				"verb", "create",
			)
		} else {
			s.logger.Info("Auth middleware SKIPPED - authenticator or authorizer is nil (test environment)")
		}

		// BR-STORAGE-021: Incident query endpoints (READ API)
		r.Get("/incidents", s.handler.ListIncidents)
		r.Get("/incidents/{id}", s.handler.GetIncident)

		// BR-STORAGE-030: Aggregation endpoints (READ API)
		r.Get("/incidents/aggregate/success-rate", s.handler.AggregateSuccessRate)
		r.Get("/incidents/aggregate/by-namespace", s.handler.AggregateByNamespace)
		r.Get("/incidents/aggregate/by-severity", s.handler.AggregateBySeverity)
		r.Get("/incidents/aggregate/trend", s.handler.AggregateIncidentTrend)

		// BR-STORAGE-031-01, BR-STORAGE-031-02, BR-STORAGE-031-05: ADR-033 Multi-dimensional Success Tracking (READ API)
		r.Get("/success-rate/incident-type", s.handler.HandleGetSuccessRateByIncidentType)
		r.Get("/success-rate/workflow", s.handler.HandleGetSuccessRateByWorkflow)
		r.Get("/success-rate/multi-dimensional", s.handler.HandleGetSuccessRateMultiDimensional)

		// BR-STORAGE-001 to BR-STORAGE-020: Audit write endpoints (WRITE API)
		s.logger.V(1).Info("Registering POST /api/v1/audit/notifications handler")
		r.Post("/audit/notifications", s.handleCreateNotificationAudit)

		// BR-STORAGE-033: Unified audit events API (ADR-034)
		// DD-STORAGE-010: Query API with offset-based pagination
		s.logger.V(1).Info("Registering /api/v1/audit/events handlers (ADR-034, DD-STORAGE-010)")
		r.Post("/audit/events", s.handleCreateAuditEvent)
		r.Get("/audit/events", s.handleQueryAuditEvents)

		// DD-AUDIT-002: Batch audit events API for HTTPDataStorageClient.StoreBatch()
		// BR-AUDIT-001: Complete audit trail with no data loss
		s.logger.V(1).Info("Registering /api/v1/audit/events/batch handler (DD-AUDIT-002)")
		r.Post("/audit/events/batch", s.handleCreateAuditEventsBatch)

		// SOC2 Gap #9: Tamper detection verification API (PostgreSQL-based)
		// BR-AUDIT-005: Enterprise-Grade Audit Integrity
		s.logger.V(1).Info("Registering /api/v1/audit/verify-chain handler (SOC2 Gap #9)")
		r.Post("/audit/verify-chain", s.HandleVerifyChain)

		// SOC2 Gap #8: Legal Hold & Retention Policies
		// BR-AUDIT-006: Legal hold capability for Sarbanes-Oxley and HIPAA compliance
		s.logger.V(1).Info("Registering /api/v1/audit/legal-hold handlers (SOC2 Gap #8)")
		r.Post("/audit/legal-hold", s.HandlePlaceLegalHold)
		r.Delete("/audit/legal-hold/{correlation_id}", s.HandleReleaseLegalHold)
		r.Get("/audit/legal-hold", s.HandleListLegalHolds)

		// SOC2 Day 9: Signed Audit Export
		// BR-AUDIT-007: Audit export with digital signatures for compliance verification
		s.logger.V(1).Info("Registering /api/v1/audit/export handler (SOC2 Day 9)")
		r.Get("/audit/export", s.HandleExportAuditEvents)

		// BR-AUDIT-006: RemediationRequest Reconstruction from Audit Traces
		// SOC2 compliance: Reconstruct complete RR CRDs from audit trail
		s.logger.V(1).Info("Registering /api/v1/audit/remediation-requests/{correlation_id}/reconstruct handler (BR-AUDIT-006)")
		r.Post("/audit/remediation-requests/{correlation_id}/reconstruct", s.handleReconstructRemediationRequestWrapper)

		// BR-STORAGE-013: Semantic search for remediation workflows
		// BR-STORAGE-014: Workflow catalog management
		// DD-STORAGE-008: Workflow catalog schema
		// DD-WORKFLOW-005 v1.0: Direct REST API workflow registration
		// DD-WORKFLOW-002 v3.0: UUID primary key for workflow retrieval
		s.logger.V(1).Info("Registering /api/v1/workflows handlers (BR-STORAGE-013, DD-STORAGE-008)")
		r.Post("/workflows", s.handler.HandleCreateWorkflow)
		r.Post("/workflows/search", s.handler.HandleWorkflowSearch)
		r.Get("/workflows", s.handler.HandleListWorkflows)
		r.Get("/workflows/{workflowID}", s.handler.HandleGetWorkflowByID)
		// DD-WORKFLOW-012: Update mutable fields (status, metrics) - immutable fields require new version
		r.Patch("/workflows/{workflowID}", s.handler.HandleUpdateWorkflow)
		// DD-WORKFLOW-012: Convenience endpoint for disabling workflows
		r.Patch("/workflows/{workflowID}/disable", s.handler.HandleDisableWorkflow)
		// DD-WORKFLOW-002 v3.0: List all versions by workflow_name
		r.Get("/workflows/by-name/{workflowName}/versions", s.handler.HandleListWorkflowVersions)
	})

	s.logger.V(1).Info("API v1 routes configured successfully")

	return r
}

// Start starts the HTTP server
func (s *Server) Start() error {
	// DS-FLAKY-003 FIX: Handler is now assigned in NewServer(), so just start the server
	// Previously: Handler was assigned here, causing httptest tests to fail graceful shutdown
	s.logger.Info("Starting Data Storage Service server",
		"addr", s.httpServer.Addr,
	)

	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server following DD-007 pattern
// DD-007: Kubernetes-Aware Graceful Shutdown (4-Step Pattern)
//
// This implements the production-proven pattern from Gateway/Context API services
// to achieve ZERO request failures during rolling updates
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Initiating DD-007 + DD-008 Kubernetes-aware graceful shutdown with DLQ drain")

	// STEP 1: Signal Kubernetes to remove pod from endpoints
	s.shutdownStep1SetFlag()

	// STEP 2: Wait for endpoint removal to propagate
	s.shutdownStep2WaitForPropagation()

	// STEP 3: Drain in-flight HTTP connections
	if err := s.shutdownStep3DrainConnections(ctx); err != nil {
		return err
	}

	// STEP 4: Drain DLQ messages (DD-008)
	s.shutdownStep4DrainDLQ(ctx)

	// STEP 5: Close external resources (database)
	if err := s.shutdownStep5CloseResources(); err != nil {
		return err
	}

	s.logger.Info("DD-007 + DD-008 graceful shutdown complete - all resources closed, DLQ drained",
		"dd", "DD-007-DD-008-complete-success")
	return nil
}

// shutdownStep1SetFlag sets the shutdown flag to signal readiness probe
// DD-007 STEP 1: This triggers Kubernetes endpoint removal
func (s *Server) shutdownStep1SetFlag() {
	s.isShuttingDown.Store(true)
	s.logger.Info("Shutdown flag set - readiness probe now returns 503",
		"effect", "kubernetes_will_remove_from_endpoints",
		"dd", "DD-007-step-1")
}

// shutdownStep2WaitForPropagation waits for Kubernetes endpoint removal to propagate
// DD-007 STEP 2: Industry best practice is 5 seconds (Kubernetes typically takes 1-3s)
func (s *Server) shutdownStep2WaitForPropagation() {
	s.logger.Info("Waiting for Kubernetes endpoint removal to propagate",
		"delay", endpointRemovalPropagationDelay,
		"dd", "DD-007-step-2")
	time.Sleep(endpointRemovalPropagationDelay)
	s.logger.Info("Endpoint propagation complete - now draining connections",
		"dd", "DD-007-step-2-complete")
}

// shutdownStep3DrainConnections drains in-flight HTTP connections
// DD-007 STEP 3: Gracefully close HTTP connections with timeout
func (s *Server) shutdownStep3DrainConnections(ctx context.Context) error {
	s.logger.Info("Draining in-flight HTTP connections",
		"drain_timeout", drainTimeout,
		"dd", "DD-007-step-3")

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
		s.logger.Error(err, "Error during HTTP connection drain",
			"dd", "DD-007-step-3-error")
		return fmt.Errorf("HTTP connection drain failed: %w", err)
	}

	s.logger.Info("HTTP connections drained successfully",
		"dd", "DD-007-step-3-complete")
	return nil
}

// shutdownStep4DrainDLQ drains pending DLQ messages before shutdown
// DD-008 STEP 4: Ensure audit messages in DLQ are not lost
func (s *Server) shutdownStep4DrainDLQ(ctx context.Context) {
	if s.dlqClient == nil {
		s.logger.Info("DLQ client not available, skipping DLQ drain",
			"dd", "DD-008-step-4-skipped")
		return
	}

	s.logger.Info("Draining DLQ messages before shutdown",
		"timeout", dlqDrainTimeout,
		"dd", "DD-008-step-4")

	// Create timeout context for DLQ drain
	drainCtx, cancel := context.WithTimeout(context.Background(), dlqDrainTimeout)
	defer cancel()

	// Override parent context if it would timeout sooner
	if deadline, ok := ctx.Deadline(); ok {
		if time.Until(deadline) < dlqDrainTimeout {
			drainCtx = ctx
		}
	}

	// Drain DLQ with timeout
	stats, err := s.dlqClient.DrainWithTimeout(drainCtx, s.repository, s.auditEventsRepo)
	if err != nil {
		s.logger.Error(err, "Error during DLQ drain (non-fatal, continuing shutdown)",
			"dd", "DD-008-step-4-error")
		// Continue with shutdown even if DLQ drain fails
		// (DLQ failures should not block graceful shutdown)
		return
	}

	// Log drain statistics
	s.logger.Info("DLQ drain complete",
		"notifications_processed", stats.NotificationsProcessed,
		"events_processed", stats.EventsProcessed,
		"total_processed", stats.TotalProcessed,
		"duration", stats.Duration,
		"timed_out", stats.TimedOut,
		"errors", len(stats.Errors),
		"dd", "DD-008-step-4-complete")

	// Log any errors encountered during drain (but don't fail shutdown)
	for i, drainErr := range stats.Errors {
		s.logger.Error(drainErr, "Error during DLQ drain processing",
			"error_index", i,
			"dd", "DD-008-step-4-drain-error")
	}
}

// shutdownStep5CloseResources closes external resources (database, audit store)
// DD-007 STEP 5 (previously step 4): Clean up database connections and flush audit events
func (s *Server) shutdownStep5CloseResources() error {
	s.logger.Info("Closing external resources (PostgreSQL, audit store)",
		"dd", "DD-007-step-5")

	// BR-STORAGE-014: Flush remaining audit events before closing database
	// This ensures no audit traces are lost during graceful shutdown
	if s.auditStore != nil {
		s.logger.Info("Flushing remaining audit events (DD-STORAGE-012)",
			"dd", "DD-007-step-5-audit-flush")
		if err := s.auditStore.Close(); err != nil {
			s.logger.Error(err, "Failed to flush audit events",
				"dd", "DD-007-step-5-audit-error")
			// Continue with shutdown even if audit flush fails
			// (audit failures should not block graceful shutdown)
		} else {
			s.logger.Info("Audit events flushed successfully",
				"dd", "DD-007-step-5-audit-complete")
		}
	}

	// Close PostgreSQL connection
	if err := s.db.Close(); err != nil {
		s.logger.Error(err, "Failed to close PostgreSQL connection",
			"dd", "DD-007-step-5-error")
		return fmt.Errorf("failed to close PostgreSQL: %w", err)
	}

	s.logger.Info("All external resources closed",
		"dd", "DD-007-step-5-complete")
	return nil
}

// GetDLQClient returns the DLQ client for testing purposes
// This allows integration tests to verify DD-008 DLQ drain behavior
func (s *Server) GetDLQClient() *dlq.Client {
	return s.dlqClient
}

// loadSigningCertificate loads the signing certificate from cert-manager managed Secret
// SOC2 Day 9.1: Digital signatures for audit exports
// BR-AUDIT-007: Tamper-evident audit logs
//
// Certificate Location: /etc/certs/ (mounted from datastorage-signing-cert Secret)
// - /etc/certs/tls.crt (PEM certificate)
// - /etc/certs/tls.key (PEM private key)
//
// cert-manager Compatibility:
// - Managed by Certificate CRD (deploy/data-storage/certificate.yaml)
// - Auto-rotates 30 days before expiry
// - Self-signed via selfsigned-issuer ClusterIssuer
//
// Fallback: If cert-manager not available, generate self-signed cert
func loadSigningCertificate(logger logr.Logger) (*cert.Signer, error) {
	certFile := "/etc/certs/tls.crt"
	keyFile := "/etc/certs/tls.key"

	// Check if cert-manager provided certificate exists
	_, statErr := os.Stat(certFile)
	if os.IsNotExist(statErr) {
		logger.Info("cert-manager certificate not found, generating self-signed certificate",
			"cert_file", certFile)
		return generateFallbackCertificate(logger)
	}

	// Check if key file exists
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		logger.Info("cert-manager key file not found, generating self-signed certificate",
			"key_file", keyFile)
		return generateFallbackCertificate(logger)
	}

	// Load certificate from cert-manager Secret
	tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		// Fallback to self-signed if cert-manager cert is invalid/corrupt
		logger.Info("cert-manager certificate invalid or corrupt, generating self-signed certificate",
			"cert_file", certFile,
			"key_file", keyFile,
			"load_error", err.Error())
		return generateFallbackCertificate(logger)
	}

	// Create signer from TLS certificate
	signer, err := cert.NewSignerFromTLSCertificate(&tlsCert)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer from certificate: %w", err)
	}

	logger.V(1).Info("Loaded signing certificate from cert-manager",
		"cert_file", certFile,
		"algorithm", signer.GetAlgorithm(),
		"fingerprint", signer.GetCertificateFingerprint())

	return signer, nil
}

// generateFallbackCertificate generates a self-signed certificate if cert-manager is unavailable
// Used in development or when cert-manager is not yet installed
func generateFallbackCertificate(logger logr.Logger) (*cert.Signer, error) {
	logger.Info("Generating fallback self-signed certificate",
		"validity", "1 year",
		"key_size", "2048-bit RSA")

	// Generate self-signed certificate
	certPair, err := cert.GenerateSelfSigned(cert.CertificateOptions{
		CommonName:       "data-storage-service",
		Organization:     "Kubernaut",
		DNSNames:         []string{"data-storage-service", "data-storage-service.kubernaut-system.svc.cluster.local"},
		ValidityDuration: 8760 * time.Hour, // 1 year (cert-manager default)
		KeySize:          2048,             // cert-manager default
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate fallback certificate: %w", err)
	}

	// Create signer from generated certificate
	signer, err := cert.NewSignerFromPEM(certPair.CertPEM, certPair.KeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer from generated certificate: %w", err)
	}

	logger.Info("Fallback certificate generated successfully",
		"algorithm", signer.GetAlgorithm(),
		"fingerprint", signer.GetCertificateFingerprint(),
		"not_before", certPair.NotBefore,
		"not_after", certPair.NotAfter)

	return signer, nil
}
