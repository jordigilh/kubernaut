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
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/cert"
	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	dsmetrics "github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/partition"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/retention"
	actiontyperepo "github.com/jordigilh/kubernaut/pkg/datastorage/repository/actiontype"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	dsmiddleware "github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"     // Issue #756: FileWatcher for cert rotation
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls" // Issue #493/#678: Conditional TLS
)

// Server is the HTTP server for Data Storage Service
// BR-STORAGE-021: REST API read endpoints
// BR-STORAGE-024: RFC 7807 error responses
//
// DD-007: Kubernetes-aware graceful shutdown with 5-step pattern (see Shutdown method)
// DD-AUTH-014: Middleware-based authentication and authorization
type Server struct {
	handler    *Handler
	db         *sql.DB
	logger     logr.Logger
	httpServer *http.Server

	// Issue #756: TLS cert hot-reload support
	certReloader *sharedtls.CertReloader // nil when TLS disabled
	certWatcher  *hotreload.FileWatcher  // nil when TLS disabled
	tlsCertDir   string                  // cert dir for FileWatcher path

	// DD-007: Graceful shutdown coordination flag
	// Thread-safe flag for readiness probe coordination during shutdown
	isShuttingDown atomic.Bool

	// endpointPropagationDelay is the time to wait for Kubernetes endpoint
	// removal to propagate. Set via ServerConfig.GetEndpointPropagationDelay()
	// (default: 5s, range: [1s, 30s]). Set to 0 in tests to avoid slow shutdown specs.
	endpointPropagationDelay time.Duration

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

	// DD-009 V1.0: DLQ retry worker (background goroutine for async persistence)
	// Processes 202 Accepted events from Redis DLQ back to PostgreSQL
	dlqRetryWorker *DLQRetryWorker

	// #1048 Phase 5 / AU-11: Retention worker (background purge)
	retentionWorker *retention.Worker

	// DD-AUTH-014: Authentication and authorization via dependency injection
	// Authenticator validates tokens (TokenReview)
	// Authorizer checks permissions (SubjectAccessReview)
	// Production: Real K8s APIs, Integration: Mocks, E2E: Real K8s APIs
	authenticator auth.Authenticator
	authorizer    auth.Authorizer
	authNamespace string // Namespace for SAR checks (dynamically determined from pod)

	// Issue #667 / BR-STORAGE-043: Maximum events per batch API request
	maxBatchSize int

	// #1048 Phase 4: OpenAPI validator created at startup (fail-hard)
	openapiValidator *dsmiddleware.OpenAPIValidator

	// #1048 Phase 4: Configurable CORS origins (ADR-030), default ["*"]
	corsAllowedOrigins []string

	// #1048 Phase 4: Max request body size in bytes (SC-5 DoS protection)
	maxBodySize int64
}

func defaultMaxBatchSize(v int) int {
	if v <= 0 {
		return 500
	}
	return v
}

// DD-007 + DD-008 graceful shutdown constants
const (
	// drainTimeout is the maximum time to wait for in-flight requests to complete
	drainTimeout = 30 * time.Second

	// dlqDrainTimeout is the maximum time to drain DLQ messages during shutdown (DD-008)
	dlqDrainTimeout = 10 * time.Second
)

// ServerDeps groups the dependencies required to create a Data Storage HTTP server.
// Replaces the previous long positional parameter list for clarity and extensibility.
type ServerDeps struct {
	DBConnStr     string             // PostgreSQL connection string
	RedisAddr     string             // Redis address for DLQ (format: "localhost:6379")
	RedisPassword string             // Redis password (from mounted secret)
	Logger        logr.Logger        // Structured logger
	AppConfig     *config.Config     // Full application configuration (includes database pool settings)
	ServerConfig  *Config            // Server-specific configuration (port, timeouts)
	DLQMaxLen     int64              // Maximum DLQ stream length for capacity monitoring (Gap 3.3)
	Authenticator auth.Authenticator // Token validator (DD-AUTH-014)
	Authorizer    auth.Authorizer    // Permission checker (DD-AUTH-014)
	AuthNamespace string             // Namespace for SAR checks (DD-AUTH-014)
	HandlerOpts   []HandlerOption    // Optional handler options (e.g. WithDependencyValidator for DD-WE-006)
}

// NewServer creates a new Data Storage HTTP server.
// BR-STORAGE-021: REST API Gateway for database access
// BR-STORAGE-001 to BR-STORAGE-020: Audit write API
// SOC2 Gap #9: PostgreSQL with custom hash chains for tamper detection
// DD-AUTH-014: Middleware-based authentication and authorization
func NewServer(deps ServerDeps) (*Server, error) {
	// DD-AUTH-014: Authenticator and authorizer are MANDATORY
	if deps.Authenticator == nil {
		return nil, fmt.Errorf("authenticator is nil - DD-AUTH-014 requires authentication (K8s in production, mock in unit tests)")
	}
	if deps.Authorizer == nil {
		return nil, fmt.Errorf("authorizer is nil - DD-AUTH-014 requires authorization (K8s in production, mock in unit tests)")
	}
	if deps.AuthNamespace == "" {
		return nil, fmt.Errorf("authNamespace is empty - DD-AUTH-014 requires namespace for SAR checks")
	}

	logger := deps.Logger
	appCfg := deps.AppConfig
	serverCfg := deps.ServerConfig

	// Connect to PostgreSQL using pgx driver (DD-010)
	// Bug fix #200: Uses OpenPostgresDB which configures QueryExecModeExec
	// to prevent stale prepared statement caches after schema migrations
	db, err := OpenPostgresDB(deps.DBConnStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// #1048 Phase 4 / RES-M2: Track resources for cleanup on late-stage errors.
	// On success the cleanup is disarmed; on failure all acquired resources are released
	// in reverse order to prevent leaks during startup.
	var cleanups []func()
	success := false
	defer func() {
		if !success {
			for i := len(cleanups) - 1; i >= 0; i-- {
				cleanups[i]()
			}
		}
	}()
	cleanups = append(cleanups, func() { _ = db.Close() })

	// Verify connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	// BR-AUDIT-029: Ensure monthly partitions for audit_events and resource_action_traces.
	// Fail-fast: if partitions cannot be created, DS must not start (writes would fail).
	// Issue #667/M5: Bound startup DDL with a 30s deadline to prevent indefinite hangs.
	clock := partition.UTCClock{}
	partitionCtx, partitionCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer partitionCancel()
	if err := partition.EnsureMonthlyPartitions(
		partitionCtx, db, clock.Now(), partition.DefaultLookaheadMonths, partition.AllTables(),
	); err != nil {
		return nil, fmt.Errorf("failed to ensure monthly partitions at startup: %w", err)
	}
	logger.Info("Monthly partitions ensured",
		"lookahead", partition.DefaultLookaheadMonths,
		"tables", []string{"audit_events", "resource_action_traces"},
	)

	// Configure connection pool from config (not hardcoded)
	db.SetMaxOpenConns(appCfg.Database.MaxOpenConns)
	db.SetMaxIdleConns(appCfg.Database.MaxIdleConns)

	connMaxLifetime, err := time.ParseDuration(appCfg.Database.ConnMaxLifetime)
	if err != nil {
		return nil, fmt.Errorf("invalid connMaxLifetime: %w", err)
	}
	db.SetConnMaxLifetime(connMaxLifetime)

	connMaxIdleTime, err := time.ParseDuration(appCfg.Database.ConnMaxIdleTime)
	if err != nil {
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
	redisOpts := &redis.Options{
		Addr:     deps.RedisAddr,
		Password: deps.RedisPassword, // ADR-030: Password from mounted secret
	}
	if appCfg.Redis.TLS.Enabled {
		redisTLS, err := appCfg.Redis.TLS.BuildTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to configure Redis TLS: %w", err)
		}
		redisOpts.TLSConfig = redisTLS
		logger.Info("Redis TLS enabled", "ca_file", appCfg.Redis.TLS.CAFile)
	}
	redisClient := redis.NewClient(redisOpts)
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	cleanups = append(cleanups, func() { _ = redisClient.Close() })

	logger.Info("Redis connection established",
		"addr", deps.RedisAddr,
	)

	// Create audit write dependencies (BR-STORAGE-001 to BR-STORAGE-020)
	logger.V(1).Info("Creating audit write dependencies...")
	repo := repository.NewNotificationAuditRepository(db, logger)
	// Gap 3.3: Use passed DLQ max length for capacity monitoring
	dlqMaxLen := deps.DLQMaxLen
	if dlqMaxLen <= 0 {
		dlqMaxLen = 10000 // Default if not configured
	}
	dlqClient, err := dlq.NewClient(redisClient, logger, dlqMaxLen)
	if err != nil {
		return nil, fmt.Errorf("failed to create DLQ client: %w", err) // cleanups run via defer
	}
	validator := validation.NewNotificationAuditValidator()

	logger.V(1).Info("Audit write dependencies created",
		"repo_nil", repo == nil,
		"dlq_client_nil", dlqClient == nil,
		"validator_nil", validator == nil)

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
		return nil, fmt.Errorf("failed to create audit store: %w", err) // cleanups run via defer
	}
	cleanups = append(cleanups, func() { _ = auditStore.Close() })

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

	// #1048 Phase 5 / AU-11: DLQ stream XADD / MAXLEN~ trim observability.
	dlqClient.SetXAddCounter(metrics.DLQStreamXAddTotal)

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

	// BR-WORKFLOW-007: ActionType taxonomy repository
	actionTypeRepo := actiontyperepo.NewRepository(sqlxDB, logger)

	// DD-WE-006: Create OCI schema extractor for execution bundle validation
	imagePuller := oci.NewCraneImagePuller(logger)
	schemaParser := schema.NewParser()
	schemaExtractor := oci.NewSchemaExtractor(imagePuller, schemaParser)

	// BR-HAPI-016: Remediation history context repository (DD-HAPI-016 v1.1)
	remHistoryRepo := repository.NewRemediationHistoryRepository(db, logger)
	remHistoryQuerier := NewRemediationHistoryRepoAdapter(remHistoryRepo)

	// Create READ API handler with logger, ADR-033 repository, workflow catalog, and audit store
	// V1.0: Embedding service removed (label-only search)
	// BR-AUDIT-006: Pass sqlDB for reconstruction queries
	// GAP-WF-1: WithWorkflowLifecycleRepository enables enable/disable/deprecate handlers
	// Build handler options: fixed options + caller-provided options (e.g. WithDependencyValidator)
	opts := []HandlerOption{
		WithLogger(logger),
		WithWorkflowRepository(workflowRepo),
		WithWorkflowLifecycleRepository(workflowRepo),
		WithWorkflowContentIntegrityRepository(workflowRepo),
		WithActionTypeValidator(actionTypeRepo),
		WithAuditStore(auditStore),
		WithSQLDB(db),
		WithSchemaExtractor(schemaExtractor),
		WithRemediationHistoryQuerier(remHistoryQuerier),
		WithActionTypeRepository(actionTypeRepo),
	}
	opts = append(opts, deps.HandlerOpts...)
	handler := NewHandler(opts...)

	// SOC2 Day 9.1: Load signing certificate for audit exports
	// BR-AUDIT-007: Digital signatures for tamper-evident audit exports
	signerCertDir := deps.AppConfig.Server.GetSignerCertDir()
	logger.V(1).Info("Loading signing certificate...", "cert_dir", signerCertDir)
	signer, err := loadSigningCertificate(logger, signerCertDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load signing certificate: %w", err) // cleanups run via defer
	}
	logger.Info("Signing certificate loaded successfully",
		"algorithm", signer.GetAlgorithm(),
		"fingerprint", signer.GetCertificateFingerprint())

	// #1048 Phase 4 / BR-STORAGE-034: Initialize OpenAPI validator at startup (fail-hard).
	// If the embedded spec is invalid the service MUST NOT start — running without
	// request validation silently accepts malformed input.
	openapiValidator, err := dsmiddleware.NewOpenAPIValidator(
		logger.WithName("openapi-validator"),
		metrics.ValidationFailures,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OpenAPI validator: %w", err) // cleanups run via defer
	}
	logger.Info("OpenAPI validator initialized at startup (fail-hard)")

	// DD-009 V1.0: Create DLQ retry worker (goroutine inside server)
	// #1048 DF-1: Pass notification repo so notification DLQ messages are persisted
	dlqWorkerConfig := DefaultDLQRetryWorkerConfig()
	dlqWorkerConfig.ConsumerName = fmt.Sprintf("worker-%d", os.Getpid())
	dlqRetryWorker := NewDLQRetryWorker(dlqClient, auditEventsRepo, repo, dlqWorkerConfig, logger, metrics.DLQValidationFailures)

	retentionWorker := retention.NewWorker(db, retention.Config{
		Enabled:              appCfg.Retention.Enabled,
		Interval:             appCfg.Retention.GetInterval(),
		BatchSize:            appCfg.Retention.GetBatchSize(),
		PartitionDropEnabled: appCfg.Retention.PartitionDropEnabled,
	}, logger)

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
			IdleTimeout:  120 * time.Second,
		},
		repository:               repo,
		dlqClient:                dlqClient,
		validator:                validator,
		auditEventsRepo:          auditEventsRepo,
		auditStore:               auditStore,
		metrics:                  metrics,
		signer:                   signer,
		dlqRetryWorker:           dlqRetryWorker,                                  // DD-009 V1.0: DLQ retry worker
		retentionWorker:          retentionWorker,                                 // #1048 Phase 5 / AU-11: retention purge
		authenticator:            deps.Authenticator,                              // DD-AUTH-014: Injected at runtime
		authorizer:               deps.Authorizer,                                 // DD-AUTH-014: Injected at runtime
		authNamespace:            deps.AuthNamespace,                              // DD-AUTH-014: Dynamic namespace for SAR checks
		maxBatchSize:             defaultMaxBatchSize(appCfg.Server.MaxBatchSize), // Issue #667 / BR-STORAGE-043
		endpointPropagationDelay: appCfg.Server.GetEndpointPropagationDelay(),
		openapiValidator:         openapiValidator,
		corsAllowedOrigins:       appCfg.Server.GetCORSAllowedOrigins(),
		maxBodySize:              appCfg.Server.GetMaxBodySize(),
	}

	// DS-FLAKY-003 FIX: Assign handler immediately so Shutdown() can work
	srv.httpServer.Handler = srv.Handler()

	if serverCfg.TLS.Enabled() {
		isTLS, reloader, tlsErr := sharedtls.ConfigureConditionalTLS(srv.httpServer, serverCfg.TLS.CertDir)
		if tlsErr != nil {
			return nil, fmt.Errorf("failed to configure TLS: %w", tlsErr) // cleanups run via defer
		}
		if isTLS {
			srv.certReloader = reloader
			srv.tlsCertDir = serverCfg.TLS.CertDir
			logger.Info("TLS configured for DataStorage server", "certDir", serverCfg.TLS.CertDir)
		}
	}

	success = true
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
	// #1048 Phase 4 / SC-5: Request body size limit (DoS protection).
	// Applied before OpenAPI validation so the body is capped before spec parsing.
	r.Use(dsmiddleware.MaxBytesReaderMiddleware(s.maxBodySize, s.logger))

	// #1048 Phase 4 / AC-4: CORS with configurable origins (ADR-030).
	// AllowedMethods includes PATCH and DELETE for workflow/action-type/legal-hold routes.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.corsAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// BR-STORAGE-034: OpenAPI validation middleware (fail-hard, initialized in NewServer)
	r.Use(s.openapiValidator.Middleware)

	// Issue #753: Health endpoints moved to dedicated :8081 server.
	// See health.NewHealthServer in cmd/datastorage/main.go.

	// BR-STORAGE-019: Prometheus metrics endpoint moved to dedicated server (Issue #283)
	// Metrics are now served on a separate port (default :9090) for standardization.
	// See cmd/datastorage/main.go for the dedicated metrics server.

	// API v1 routes
	s.logger.V(1).Info("Setting up API v1 routes",
		"handler_nil", s.handler == nil,
		"repository_nil", s.repository == nil,
		"validator_nil", s.validator == nil,
		"dlq_client_nil", s.dlqClient == nil)

	r.Route("/api/v1", func(r chi.Router) {
		// DD-AUTH-014: Authentication and authorization middleware (MANDATORY)
		// Applied to all /api/v1 routes (excludes /health, /metrics)
		// Authority: DD-AUTH-011 (SAR with verb:"create" for all audit write operations)
		// Note: authenticator/authorizer guaranteed non-nil by NewServer validation
		authMiddleware := auth.NewMiddleware(
			s.authenticator,
			s.authorizer,
			auth.MiddlewareConfig{
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

		// BR-EM-001 to BR-EM-004: On-demand effectiveness scoring (DD-017 v2.1, ADR-EM-001 Principle 5)
		s.logger.V(1).Info("Registering GET /api/v1/effectiveness/{correlation_id} handler")
		r.Get("/effectiveness/{correlation_id}", s.handleGetEffectivenessScore)

		// BR-HAPI-016: Remediation history context for LLM prompt enrichment (DD-HAPI-016 v1.1)
		s.logger.V(1).Info("Registering GET /api/v1/remediation-history/context handler")
		r.Get("/remediation-history/context", s.handler.HandleGetRemediationHistoryContext)

		// BR-STORAGE-013, BR-STORAGE-014: Workflow catalog management
		// DD-WORKFLOW-005 v1.0: Direct REST API workflow registration
		// DD-WORKFLOW-002 v3.0: UUID primary key for workflow retrieval
		s.logger.V(1).Info("Registering /api/v1/workflows handlers (BR-STORAGE-013, DD-STORAGE-008)")
		r.Post("/workflows", s.handler.HandleCreateWorkflow)
		r.Get("/workflows", s.handler.HandleListWorkflows)
		// DD-WORKFLOW-016, DD-HAPI-017: Three-step workflow discovery protocol
		// Step 1: List available action types (with signal context filters)
		r.Get("/workflows/actions", s.handler.HandleListAvailableActions)
		// Step 2: List workflows for a specific action type
		r.Get("/workflows/actions/{action_type}", s.handler.HandleListWorkflowsByActionType)
		// Step 3 + existing: Get workflow by UUID (with optional security gate via context filters)
		r.Get("/workflows/{workflowID}", s.handler.HandleGetWorkflowByID)
		// DD-WORKFLOW-012: Update mutable fields (status, metrics) - immutable fields require new version
		r.Patch("/workflows/{workflowID}", s.handler.HandleUpdateWorkflow)
		// DD-WORKFLOW-012: Convenience endpoint for disabling workflows
		r.Patch("/workflows/{workflowID}/disable", s.handler.HandleDisableWorkflow)
		// DD-WORKFLOW-017 Phase 4.4 (GAP-WF-1): Lifecycle endpoints for enable and deprecate
		r.Patch("/workflows/{workflowID}/enable", s.handler.HandleEnableWorkflow)
		r.Patch("/workflows/{workflowID}/deprecate", s.handler.HandleDeprecateWorkflow)

		// BR-WORKFLOW-007: ActionType taxonomy CRUD (ADR-059, DD-ACTIONTYPE-001)
		s.logger.V(1).Info("Registering /api/v1/action-types handlers (BR-WORKFLOW-007)")
		r.Post("/action-types", s.handler.HandleCreateActionType)
		r.Patch("/action-types/{name}", s.handler.HandleUpdateActionType)
		r.Patch("/action-types/{name}/disable", s.handler.HandleDisableActionType)
		r.Get("/action-types/{name}/workflow-count", s.handler.HandleGetActionTypeWorkflowCount)
	})

	s.logger.V(1).Info("API v1 routes configured successfully")

	return r
}

// Start starts the HTTP server, with conditional TLS (#493).
func (s *Server) Start() error {
	s.logger.Info("Starting Data Storage Service server",
		"addr", s.httpServer.Addr,
	)

	// DD-009 V1.0: Start DLQ retry worker before accepting HTTP traffic
	// Issue #667/M4: Use a server-scoped context instead of context.Background()
	s.dlqRetryWorker.Start(context.Background())
	s.retentionWorker.Start(context.Background())

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
		if err := watcher.Start(context.Background()); err != nil {
			return fmt.Errorf("failed to start cert file watcher: %w", err)
		}
		s.certWatcher = watcher
	}

	if s.httpServer.TLSConfig != nil {
		s.logger.Info("Server TLS configured", "tls.enabled", true, "tls.certDir", s.tlsCertDir)
		return s.httpServer.ListenAndServeTLS("", "")
	}
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server following DD-007 + DD-008 pattern.
//
// Steps executed (all steps run regardless of prior errors):
//  1. Set readiness flag (Kubernetes removes pod from endpoints)
//  2. Wait for endpoint removal propagation
//  3. Drain in-flight HTTP connections
//     3.5 Stop DLQ retry worker (DD-009)
//  4. Drain DLQ messages to PostgreSQL (DD-008)
//  4.5. Stop retention worker (AU-11) after DLQ drain, before closing DB
//  5. Close external resources (audit store, PostgreSQL)
//
// Returns a joined error if any step failed; individual step errors are
// logged at the point of failure. The returned error supports errors.Is/As.
func (s *Server) Shutdown(ctx context.Context) error {
	shutdownID := uuid.New().String()
	s.logger.Info("Initiating DD-007 + DD-008 Kubernetes-aware graceful shutdown with DLQ drain",
		"shutdown_id", shutdownID)

	var shutdownErrors []error

	// STEP 1: Signal Kubernetes to remove pod from endpoints
	s.shutdownStep1SetFlag(shutdownID)

	// STEP 2: Wait for endpoint removal to propagate
	s.shutdownStep2WaitForPropagation(shutdownID)

	// STEP 3: Drain in-flight HTTP connections
	// #1048 Phase 3: Never skip subsequent cleanup steps. HTTP drain failure
	// must not prevent DLQ drain or DB close — that would cause data loss
	// (BR-AUDIT-001) and leak database connections.
	if err := s.shutdownStep3DrainConnections(ctx, shutdownID); err != nil {
		s.logger.Error(err, "HTTP connection drain failed, continuing with cleanup",
			"shutdown_id", shutdownID,
			"dd", "DD-007-step-3-error-non-fatal")
		shutdownErrors = append(shutdownErrors, err)
	}

	// Issue #756: Stop cert file watcher after HTTP server is down
	if s.certWatcher != nil {
		s.certWatcher.Stop()
	}

	// STEP 3.5: Stop DLQ retry worker before draining (DD-009 V1.0)
	s.dlqRetryWorker.Stop()

	// STEP 4: Drain DLQ messages (DD-008) — ARCH-M1: surface DLQ drain errors
	if err := s.shutdownStep4DrainDLQ(ctx, shutdownID); err != nil {
		s.logger.Error(err, "DLQ drain failed during shutdown, continuing with cleanup",
			"shutdown_id", shutdownID,
			"dd", "DD-008-step-4-error-non-fatal")
		shutdownErrors = append(shutdownErrors, err)
	}

	// STEP 4.5: Stop retention worker before closing PostgreSQL (#1048 Phase 5 / AU-11)
	s.retentionWorker.Stop()

	// STEP 5: Close external resources (database)
	if err := s.shutdownStep5CloseResources(shutdownID); err != nil {
		shutdownErrors = append(shutdownErrors, err)
	}

	if len(shutdownErrors) > 0 {
		s.logger.Info("DD-007 + DD-008 graceful shutdown completed with errors",
			"shutdown_id", shutdownID,
			"error_count", len(shutdownErrors),
			"dd", "DD-007-DD-008-complete-with-errors")
		return errors.Join(shutdownErrors...)
	}

	s.logger.Info("DD-007 + DD-008 graceful shutdown complete - all resources closed, DLQ drained",
		"shutdown_id", shutdownID,
		"dd", "DD-007-DD-008-complete-success")
	return nil
}

// shutdownStep1SetFlag sets the shutdown flag to signal readiness probe
// DD-007 STEP 1: This triggers Kubernetes endpoint removal
func (s *Server) shutdownStep1SetFlag(shutdownID string) {
	s.isShuttingDown.Store(true)
	s.logger.Info("Shutdown flag set - readiness probe now returns 503",
		"shutdown_id", shutdownID,
		"effect", "kubernetes_will_remove_from_endpoints",
		"dd", "DD-007-step-1")
}

// shutdownStep2WaitForPropagation waits for Kubernetes endpoint removal to propagate
// DD-007 STEP 2: Industry best practice is 5 seconds (Kubernetes typically takes 1-3s)
func (s *Server) shutdownStep2WaitForPropagation(shutdownID string) {
	delay := s.endpointPropagationDelay
	s.logger.Info("Waiting for Kubernetes endpoint removal to propagate",
		"shutdown_id", shutdownID,
		"delay", delay,
		"dd", "DD-007-step-2")
	if delay > 0 {
		time.Sleep(delay)
	}
	s.logger.Info("Endpoint propagation complete - now draining connections",
		"shutdown_id", shutdownID,
		"dd", "DD-007-step-2-complete")
}

// shutdownStep3DrainConnections drains in-flight HTTP connections
// DD-007 STEP 3: Gracefully close HTTP connections with timeout
func (s *Server) shutdownStep3DrainConnections(ctx context.Context, shutdownID string) error {
	s.logger.Info("Draining in-flight HTTP connections",
		"shutdown_id", shutdownID,
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
			"shutdown_id", shutdownID,
			"dd", "DD-007-step-3-error")
		return fmt.Errorf("HTTP connection drain failed: %w", err)
	}

	s.logger.Info("HTTP connections drained successfully",
		"shutdown_id", shutdownID,
		"dd", "DD-007-step-3-complete")
	return nil
}

// shutdownStep4DrainDLQ drains pending DLQ messages before shutdown
// DD-008 STEP 4: Ensure audit messages in DLQ are not lost
func (s *Server) shutdownStep4DrainDLQ(ctx context.Context, shutdownID string) error {
	if s.dlqClient == nil {
		s.logger.Info("DLQ client not available, skipping DLQ drain",
			"shutdown_id", shutdownID,
			"dd", "DD-008-step-4-skipped")
		return nil
	}

	s.logger.Info("Draining DLQ messages before shutdown",
		"shutdown_id", shutdownID,
		"timeout", dlqDrainTimeout,
		"dd", "DD-008-step-4")

	// Create timeout context for DLQ drain.
	// Always start from context.Background() so the DLQ drain gets its full budget
	// even if the parent context expired during HTTP drain (ARCH-M2).
	drainCtx, cancel := context.WithTimeout(context.Background(), dlqDrainTimeout)
	defer cancel()

	// Use the parent context only if it has positive remaining time shorter than
	// the DLQ budget — this prevents an expired parent from starving the drain.
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 && remaining < dlqDrainTimeout {
			drainCtx = ctx
		}
	}

	// Drain DLQ with timeout
	stats, err := s.dlqClient.DrainWithTimeout(drainCtx, s.repository, s.auditEventsRepo)
	s.metrics.DLQDrainBatchTotal.Inc()

	if err != nil {
		s.metrics.ShutdownDLQDrainError.Inc()
		s.logger.Error(err, "Error during DLQ drain (non-fatal, continuing shutdown)",
			"shutdown_id", shutdownID,
			"dd", "DD-008-step-4-error")
		return fmt.Errorf("DLQ drain failed: %w", err)
	}

	// Log drain statistics
	s.logger.Info("DLQ drain complete",
		"shutdown_id", shutdownID,
		"notifications_processed", stats.NotificationsProcessed,
		"events_processed", stats.EventsProcessed,
		"total_processed", stats.TotalProcessed,
		"duration", stats.Duration,
		"timed_out", stats.TimedOut,
		"errors", len(stats.Errors),
		"dd", "DD-008-step-4-complete")

	// Log any errors encountered during drain (but don't fail shutdown)
	for i, drainErr := range stats.Errors {
		s.metrics.ShutdownDLQDrainError.Inc()
		s.logger.Error(drainErr, "Error during DLQ drain processing",
			"shutdown_id", shutdownID,
			"error_index", i,
			"dd", "DD-008-step-4-drain-error")
	}
	return nil
}

// shutdownStep5CloseResources closes external resources (database, audit store)
// DD-007 STEP 5 (previously step 4): Clean up database connections and flush audit events
func (s *Server) shutdownStep5CloseResources(shutdownID string) error {
	s.logger.Info("Closing external resources (PostgreSQL, audit store)",
		"shutdown_id", shutdownID,
		"dd", "DD-007-step-5")

	// BR-STORAGE-014: Flush remaining audit events before closing database
	// This ensures no audit traces are lost during graceful shutdown
	if s.auditStore != nil {
		s.logger.Info("Flushing remaining audit events (DD-STORAGE-012)",
			"shutdown_id", shutdownID,
			"dd", "DD-007-step-5-audit-flush")
		if err := s.auditStore.Close(); err != nil {
			s.logger.Error(err, "Failed to flush audit events",
				"shutdown_id", shutdownID,
				"dd", "DD-007-step-5-audit-error")
		} else {
			s.logger.Info("Audit events flushed successfully",
				"shutdown_id", shutdownID,
				"dd", "DD-007-step-5-audit-complete")
		}
	}

	// Close PostgreSQL connection
	if s.db == nil {
		s.logger.Info("No PostgreSQL connection to close — verify initialization",
			"shutdown_id", shutdownID,
			"severity", "warning",
			"dd", "DD-007-step-5-no-db")
	} else if err := s.db.Close(); err != nil {
		s.logger.Error(err, "Failed to close PostgreSQL connection",
			"shutdown_id", shutdownID,
			"dd", "DD-007-step-5-error")
		return fmt.Errorf("failed to close PostgreSQL: %w", err)
	}

	s.logger.Info("All external resources closed",
		"shutdown_id", shutdownID,
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
// Certificate files (under certDir, default /etc/certs from config):
// - tls.crt (PEM certificate)
// - tls.key (PEM private key)
//
// cert-manager Compatibility:
// - Managed by Certificate CRD (deploy/data-storage/certificate.yaml)
// - Auto-rotates 30 days before expiry
// - Self-signed via selfsigned-issuer ClusterIssuer
//
// #1048 Phase 5 / AU-9: Missing or invalid provisioning is a fatal startup error (no fallback).
func loadSigningCertificate(logger logr.Logger, certDir string) (*cert.Signer, error) {
	certFile := filepath.Join(certDir, "tls.crt")
	keyFile := filepath.Join(certDir, "tls.key")

	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("signing certificate not found at %s: cert-manager Certificate must be provisioned (AU-9)", certFile)
	}

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("signing key not found at %s: cert-manager Certificate must be provisioned (AU-9)", keyFile)
	}

	tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("signing certificate at %s is invalid or corrupt: %w", certFile, err)
	}

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
