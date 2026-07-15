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
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/cert"
	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	dsmetrics "github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/partition"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	actiontyperepo "github.com/jordigilh/kubernaut/pkg/datastorage/repository/actiontype"
	"github.com/jordigilh/kubernaut/pkg/datastorage/retention"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	dsmiddleware "github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
	"github.com/jordigilh/kubernaut/pkg/datastorage/workflowcache"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls" // Issue #493/#678: Conditional TLS
)

// NewServer construction path: dependency wiring for Data Storage's
// PostgreSQL/Redis connections, audit-write and workflow-catalog
// repositories, and the assembled Server. Split from server.go
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3, pure code motion, no behavior
// change); see server.go for the Server struct/config and
// server_routes.go / server_lifecycle.go for routing and Start/Shutdown.

// validateServerDeps enforces the DD-AUTH-014 mandatory dependencies before
// any resource acquisition begins.
func validateServerDeps(deps ServerDeps) error {
	if deps.Authenticator == nil {
		return fmt.Errorf("authenticator is nil - DD-AUTH-014 requires authentication (K8s in production, mock in unit tests)")
	}
	if deps.Authorizer == nil {
		return fmt.Errorf("authorizer is nil - DD-AUTH-014 requires authorization (K8s in production, mock in unit tests)")
	}
	if deps.AuthNamespace == "" {
		return fmt.Errorf("authNamespace is empty - DD-AUTH-014 requires namespace for SAR checks")
	}
	return nil
}

// connectAndPreparePostgres opens the PostgreSQL connection (DD-010 pgx
// driver), verifies connectivity, ensures the monthly partitions required
// for BR-AUDIT-029, and configures the connection pool from appCfg.
func connectAndPreparePostgres(deps ServerDeps, appCfg *config.Config, logger logr.Logger, cleanups *startupCleanups) (*sql.DB, error) {
	// Bug fix #200: Uses OpenPostgresDB which configures QueryExecModeExec
	// to prevent stale prepared statement caches after schema migrations
	db, err := OpenPostgresDB(deps.DBConnStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	cleanups.add(func() { _ = db.Close() })

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

	return db, nil
}

// connectServerRedis connects to Redis for the DLQ (DD-009), configuring TLS
// when enabled in appCfg.
func connectServerRedis(deps ServerDeps, appCfg *config.Config, logger logr.Logger, cleanups *startupCleanups) (*redis.Client, error) {
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
	cleanups.add(func() { _ = redisClient.Close() })

	logger.Info("Redis connection established", "addr", deps.RedisAddr)
	return redisClient, nil
}

// auditWriteDeps groups the BR-STORAGE-001..033 audit write dependencies
// constructed by buildAuditWriteDependencies.
type auditWriteDeps struct {
	repo            *repository.NotificationAuditRepository
	dlqClient       *dlq.Client
	validator       *validation.NotificationAuditValidator
	auditEventsRepo *repository.AuditEventsRepository
	auditStore      audit.AuditStore
	metrics         *dsmetrics.Metrics
}

// buildAuditWriteDependencies constructs the notification audit repository,
// DLQ client, ADR-034 unified audit events repository (SOC2 Gap #9 hash
// chains), self-auditing buffered store (DD-STORAGE-012), and Prometheus
// metrics (GAP-10).
func buildAuditWriteDependencies(db *sql.DB, redisClient *redis.Client, deps ServerDeps, appCfg *config.Config, logger logr.Logger, cleanups *startupCleanups) (*auditWriteDeps, error) {
	logger.V(1).Info("Creating audit write dependencies...")
	repo := repository.NewNotificationAuditRepository(db, logger)

	// Gap 3.3: Use passed DLQ max length for capacity monitoring
	dlqMaxLen := deps.DLQMaxLen
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

	// Create BR-STORAGE-033: Unified audit events repository (ADR-034)
	// SOC2 Gap #9: PostgreSQL with custom hash chains for tamper detection
	logger.V(1).Info("Creating ADR-034 unified audit events repository (PostgreSQL)...")
	// GAP-05 (Issue #1505): Enable keyed HMAC-SHA256 hash chaining when an HMAC
	// key has been provisioned; otherwise fall back to the legacy unkeyed
	// SHA256 algorithm (backward-compatible default).
	hashChainAlgorithm := repository.HashAlgorithmSHA256Unkeyed
	if len(appCfg.Audit.HMACKey) > 0 {
		hashChainAlgorithm = repository.HashAlgorithmHMACSHA256
	}
	auditEventsRepo := repository.NewAuditEventsRepository(db, logger, repository.WithHMACKey(appCfg.Audit.HMACKey))
	logger.V(1).Info("ADR-034 audit events repository created (PostgreSQL-backed, SOC2 Gap #9)",
		"audit_events_repo_nil", auditEventsRepo == nil,
		"hash_chain_algorithm", hashChainAlgorithm)

	// Create BR-STORAGE-012: Self-auditing audit store (DD-STORAGE-012)
	// Uses InternalAuditClient to avoid circular dependency (cannot call own REST API)
	logger.V(1).Info("Creating self-auditing audit store (DD-STORAGE-012)...")
	internalClient := audit.NewInternalAuditClient(db)
	auditStore, err := audit.NewBufferedStore(
		internalClient,
		audit.RecommendedConfig("datastorage"), // DD-AUDIT-004: HIGH tier (50K buffer)
		"datastorage",                          // service name
		logger,                                 // Use logr.Logger directly (DD-005 v2.0)
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit store: %w", err)
	}
	cleanups.add(func() { _ = auditStore.Close() })

	logger.Info("Self-auditing audit store initialized (DD-STORAGE-012)",
		"buffer_size", audit.DefaultConfig().BufferSize,
		"batch_size", audit.DefaultConfig().BatchSize,
		"flush_interval", audit.DefaultConfig().FlushInterval,
		"max_retries", audit.DefaultConfig().MaxRetries,
	)

	// Create Prometheus metrics (BR-STORAGE-019, GAP-10)
	// Note: NewMetrics always returns a valid *Metrics (never nil)
	metrics := dsmetrics.NewMetrics("datastorage", "")
	logger.Info("Prometheus metrics initialized", "namespace", "datastorage")

	// #1048 Phase 5 / AU-11: DLQ stream XADD / MAXLEN~ trim observability.
	dlqClient.SetXAddCounter(metrics.DLQStreamXAddTotal)

	return &auditWriteDeps{
		repo:            repo,
		dlqClient:       dlqClient,
		validator:       validator,
		auditEventsRepo: auditEventsRepo,
		auditStore:      auditStore,
		metrics:         metrics,
	}, nil
}

// workflowCatalogDeps groups the BR-STORAGE-013/014 and BR-WORKFLOW-007
// workflow catalog dependencies constructed by buildWorkflowCatalogDependencies.
type workflowCatalogDeps struct {
	workflowRepo      *repository.WorkflowRepository
	actionTypeRepo    *actiontyperepo.Repository
	schemaExtractor   *oci.SchemaExtractor
	remHistoryQuerier RemediationHistoryQuerier
}

// buildWorkflowCatalogDependencies constructs the workflow repository,
// action-type taxonomy repository, OCI schema extractor (DD-WE-006), and
// remediation history querier (DD-HAPI-016 v1.1).
//
// V1.0: Embedding service removed (label-only search); see
// CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md (92% confidence).
func buildWorkflowCatalogDependencies(db *sql.DB, logger logr.Logger) *workflowCatalogDeps {
	logger.V(1).Info("Creating workflow catalog dependencies...")
	sqlxDB := sqlx.NewDb(db, "pgx") // Wrap *sql.DB with sqlx for workflow repository

	workflowRepo := repository.NewWorkflowRepository(sqlxDB, logger)
	logger.V(1).Info("Workflow catalog dependencies created (label-only search)",
		"workflow_repo_nil", workflowRepo == nil)

	actionTypeRepo := actiontyperepo.NewRepository(sqlxDB, logger)

	imagePuller := oci.NewCraneImagePuller(logger)
	schemaParser := schema.NewParser()
	schemaExtractor := oci.NewSchemaExtractor(imagePuller, schemaParser)

	remHistoryRepo := repository.NewRemediationHistoryRepository(db, logger)
	remHistoryQuerier := NewRemediationHistoryRepoAdapter(remHistoryRepo)

	return &workflowCatalogDeps{
		workflowRepo:      workflowRepo,
		actionTypeRepo:    actionTypeRepo,
		schemaExtractor:   schemaExtractor,
		remHistoryQuerier: remHistoryQuerier,
	}
}

// buildWorkflowCache constructs the Issue #1661 Phase 29 / DD-WORKFLOW-018
// informer-backed RemediationWorkflow/ActionType CRD cache when
// deps.K8sRestConfig is supplied, blocking until the initial sync completes.
// Returns (nil, nil, nil) when K8sRestConfig is nil -- preserves existing
// behavior for the many unit/integration tests that build a Server without
// a Kubernetes dependency. cmd/datastorage/main.go always supplies
// K8sRestConfig in production, so DS fails fast (like Postgres/Redis above)
// if etcd is unreachable at startup, rather than silently running with a
// stale/empty catalog.
//
// The returned cancel func is registered with cleanups (unwound if a later
// startup step fails) AND must also be stored on Server for the graceful
// Shutdown() path, since cleanups only run on startup failure.
func buildWorkflowCache(deps ServerDeps, logger logr.Logger, cleanups *startupCleanups) (*workflowcache.Cache, func(), error) {
	if deps.K8sRestConfig == nil {
		logger.Info("Workflow cache disabled (no K8sRestConfig supplied)")
		return nil, nil, nil
	}

	scheme, err := workflowcache.NewScheme()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build workflow cache scheme: %w", err)
	}

	wfCache, cancel, err := workflowcache.NewInformerCache(deps.K8sRestConfig, scheme, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build workflow cache: %w", err)
	}
	cleanups.add(cancel)

	logger.Info("Workflow cache synced (Issue #1661 Phase 29, DD-WORKFLOW-018)")
	return wfCache, cancel, nil
}

// buildRESTHandler assembles the READ API Handler from the audit write and
// workflow catalog dependencies plus any caller-provided options (e.g.
// WithSchemaExtractor).
//
// BR-AUDIT-006: Pass sqlDB for reconstruction queries.
// GAP-WF-1: WithWorkflowLifecycleRepository enables enable/disable/deprecate handlers.
// Issue #1661 Phase 29: wfCache is nil when ServerDeps.K8sRestConfig was not supplied.
func buildRESTHandler(deps ServerDeps, db *sql.DB, logger logr.Logger, auditDeps *auditWriteDeps, catalogDeps *workflowCatalogDeps, wfCache *workflowcache.Cache) *Handler {
	opts := make([]HandlerOption, 0, 11+len(deps.HandlerOpts))
	opts = append(opts,
		WithLogger(logger),
		WithWorkflowRepository(catalogDeps.workflowRepo),
		WithWorkflowLifecycleRepository(catalogDeps.workflowRepo),
		WithWorkflowContentIntegrityRepository(catalogDeps.workflowRepo),
		WithActionTypeValidator(catalogDeps.actionTypeRepo),
		WithAuditStore(auditDeps.auditStore),
		WithSQLDB(db),
		WithSchemaExtractor(catalogDeps.schemaExtractor),
		WithRemediationHistoryQuerier(catalogDeps.remHistoryQuerier),
		WithActionTypeRepository(catalogDeps.actionTypeRepo),
		WithWorkflowCache(wfCache),
	)
	opts = append(opts, deps.HandlerOpts...)
	return NewHandler(opts...)
}

// buildIPRateLimiter constructs the optional per-IP rate limiter
// (GAP-09 / Issue #1505 / SC-5), returning nil when disabled (default).
func buildIPRateLimiter(appCfg *config.Config, logger logr.Logger, cleanups *startupCleanups) *dsmiddleware.IPLimiter {
	if !appCfg.Server.RateLimit.Enabled {
		return nil
	}

	ipLimiter := dsmiddleware.NewIPLimiter(dsmiddleware.IPLimiterConfig{
		RequestsPerSecond: appCfg.Server.RateLimit.GetRequestsPerSecond(),
		Burst:             appCfg.Server.RateLimit.GetBurst(),
	})
	cleanups.add(func() { ipLimiter.Stop() })
	logger.Info("Per-IP rate limiting enabled (GAP-09, SC-5)",
		"requestsPerSecond", appCfg.Server.RateLimit.GetRequestsPerSecond(),
		"burst", appCfg.Server.RateLimit.GetBurst())
	return ipLimiter
}

// initSignerAndOpenAPIValidator loads the audit-export signing certificate
// (SOC2 Day 9.1 / BR-AUDIT-007) and initializes the OpenAPI request validator
// at startup, fail-hard (#1048 Phase 4 / BR-STORAGE-034): an invalid embedded
// spec MUST NOT allow the service to start, since that would silently accept
// malformed input. Extracted from NewServer (Wave 6 6f GREEN: funlen
// remediation) — pure code motion, no behavior change.
func initSignerAndOpenAPIValidator(deps ServerDeps, auditDeps *auditWriteDeps, logger logr.Logger) (*cert.Signer, *dsmiddleware.OpenAPIValidator, error) {
	// SOC2 Day 9.1: Load signing certificate for audit exports
	// BR-AUDIT-007: Digital signatures for tamper-evident audit exports
	signerCertDir := deps.AppConfig.Server.GetSignerCertDir()
	logger.V(1).Info("Loading signing certificate...", "cert_dir", signerCertDir)
	signer, err := loadSigningCertificate(logger, signerCertDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load signing certificate: %w", err) // cleanups run via defer
	}
	logger.Info("Signing certificate loaded successfully",
		"algorithm", signer.GetAlgorithm(),
		"fingerprint", signer.GetCertificateFingerprint())

	// #1048 Phase 4 / BR-STORAGE-034: Initialize OpenAPI validator at startup (fail-hard).
	// If the embedded spec is invalid the service MUST NOT start — running without
	// request validation silently accepts malformed input.
	openapiValidator, err := dsmiddleware.NewOpenAPIValidator(
		logger.WithName("openapi-validator"),
		auditDeps.metrics.ValidationFailures,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize OpenAPI validator: %w", err) // cleanups run via defer
	}
	logger.Info("OpenAPI validator initialized at startup (fail-hard)")

	return signer, openapiValidator, nil
}

// buildServerBackgroundWorkers constructs the server's background workers:
// the DD-009 DLQ retry worker, the #1048 Phase 5 / AU-11 retention purge
// worker, and the optional GAP-09 (Issue #1505) / SC-5 per-IP rate limiter.
// Extracted from NewServer (Wave 6 6f GREEN: funlen remediation) — pure code
// motion, no behavior change.
func buildServerBackgroundWorkers(db *sql.DB, appCfg *config.Config, auditDeps *auditWriteDeps, logger logr.Logger, cleanups *startupCleanups) (*DLQRetryWorker, *retention.Worker, *dsmiddleware.IPLimiter) {
	// DD-009 V1.0: Create DLQ retry worker (goroutine inside server)
	// #1048 DF-1: Pass notification repo so notification DLQ messages are persisted
	dlqWorkerConfig := DefaultDLQRetryWorkerConfig()
	dlqWorkerConfig.ConsumerName = fmt.Sprintf("worker-%d", os.Getpid())
	dlqRetryWorker := NewDLQRetryWorker(auditDeps.dlqClient, auditDeps.auditEventsRepo, auditDeps.repo, dlqWorkerConfig, logger, auditDeps.metrics.DLQValidationFailures)

	retentionWorker := retention.NewWorker(db, retention.Config{
		Enabled:              appCfg.Retention.Enabled,
		Interval:             appCfg.Retention.GetInterval(),
		BatchSize:            appCfg.Retention.GetBatchSize(),
		DefaultDays:          appCfg.Retention.GetDefaultDays(),
		PartitionDropEnabled: appCfg.Retention.PartitionDropEnabled,
	}, logger)

	// GAP-09 (Issue #1505) / SC-5: Optional per-IP rate limiter, disabled by default.
	ipLimiter := buildIPRateLimiter(appCfg, logger, cleanups)

	return dlqRetryWorker, retentionWorker, ipLimiter
}

// serverBackgroundWorkers groups the background workers produced by
// buildServerBackgroundWorkers, so assembleServer can take them as a single
// argument (100go.co anti-pattern: functions with 8+ parameters).
type serverBackgroundWorkers struct {
	dlqRetryWorker  *DLQRetryWorker
	retentionWorker *retention.Worker
	ipLimiter       *dsmiddleware.IPLimiter
}

// assembleServer builds the Server struct from its constructed dependencies.
// DS-FLAKY-003 FIX: handler is assigned directly on httpServer here (not
// deferred to Start()) so graceful shutdown works in both Start() and
// httptest scenarios. Extracted from NewServer (Wave 6 6f GREEN: funlen
// remediation) — pure code motion, no behavior change.
func assembleServer(
	deps ServerDeps,
	db *sql.DB,
	handler *Handler,
	auditDeps *auditWriteDeps,
	signer *cert.Signer,
	openapiValidator *dsmiddleware.OpenAPIValidator,
	workers serverBackgroundWorkers,
) *Server {
	appCfg := deps.AppConfig
	serverCfg := deps.ServerConfig
	return &Server{
		handler: handler,
		db:      db,
		logger:  deps.Logger,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", serverCfg.Port),
			ReadTimeout:  serverCfg.ReadTimeout,
			WriteTimeout: serverCfg.WriteTimeout,
			IdleTimeout:  120 * time.Second,
		},
		repository:               auditDeps.repo,
		dlqClient:                auditDeps.dlqClient,
		validator:                auditDeps.validator,
		auditEventsRepo:          auditDeps.auditEventsRepo,
		auditStore:               auditDeps.auditStore,
		metrics:                  auditDeps.metrics,
		signer:                   signer,
		dlqRetryWorker:           workers.dlqRetryWorker,                          // DD-009 V1.0: DLQ retry worker
		retentionWorker:          workers.retentionWorker,                         // #1048 Phase 5 / AU-11: retention purge
		authenticator:            deps.Authenticator,                              // DD-AUTH-014: Injected at runtime
		authorizer:               deps.Authorizer,                                 // DD-AUTH-014: Injected at runtime
		authNamespace:            deps.AuthNamespace,                              // DD-AUTH-014: Dynamic namespace for SAR checks
		maxBatchSize:             defaultMaxBatchSize(appCfg.Server.MaxBatchSize), // Issue #667 / BR-STORAGE-043
		endpointPropagationDelay: appCfg.Server.GetEndpointPropagationDelay(),
		openapiValidator:         openapiValidator,
		corsAllowedOrigins:       appCfg.Server.GetCORSAllowedOrigins(),
		maxBodySize:              appCfg.Server.GetMaxBodySize(),
		ipLimiter:                workers.ipLimiter, // GAP-09 (Issue #1505): nil when disabled
	}
}

// NewServer creates a new Data Storage HTTP server.
// BR-STORAGE-021: REST API Gateway for database access
// BR-STORAGE-001 to BR-STORAGE-020: Audit write API
// SOC2 Gap #9: PostgreSQL with custom hash chains for tamper detection
// DD-AUTH-014: Middleware-based authentication and authorization
func NewServer(deps ServerDeps) (*Server, error) {
	if err := validateServerDeps(deps); err != nil {
		return nil, err
	}

	logger := deps.Logger
	appCfg := deps.AppConfig
	serverCfg := deps.ServerConfig

	// #1048 Phase 4 / RES-M2: Track resources for cleanup on late-stage errors.
	// On success the cleanup is disarmed; on failure all acquired resources are released
	// in reverse order to prevent leaks during startup.
	cleanups := &startupCleanups{}
	success := false
	defer func() {
		if !success {
			cleanups.runAll()
		}
	}()

	db, err := connectAndPreparePostgres(deps, appCfg, logger, cleanups)
	if err != nil {
		return nil, err
	}

	redisClient, err := connectServerRedis(deps, appCfg, logger, cleanups)
	if err != nil {
		return nil, err
	}

	auditDeps, err := buildAuditWriteDependencies(db, redisClient, deps, appCfg, logger, cleanups)
	if err != nil {
		return nil, err
	}

	catalogDeps := buildWorkflowCatalogDependencies(db, logger)

	wfCache, cancelWorkflowCache, err := buildWorkflowCache(deps, logger, cleanups)
	if err != nil {
		return nil, err
	}
	if wfCache != nil {
		// Issue #1661 Change 6 (DD-WORKFLOW-018): switches ListActions/
		// ListWorkflowsByActionType from Postgres to the informer-backed CRD
		// cache. GetWorkflowWithContextFilters/GetByID (Step 3) are unaffected
		// -- deferred per Phase 31 scope decision.
		catalogDeps.workflowRepo.SetCache(wfCache)
	}

	handler := buildRESTHandler(deps, db, logger, auditDeps, catalogDeps, wfCache)

	signer, openapiValidator, err := initSignerAndOpenAPIValidator(deps, auditDeps, logger)
	if err != nil {
		return nil, err
	}

	dlqRetryWorker, retentionWorker, ipLimiter := buildServerBackgroundWorkers(db, appCfg, auditDeps, logger, cleanups)

	srv := assembleServer(deps, db, handler, auditDeps, signer, openapiValidator, serverBackgroundWorkers{
		dlqRetryWorker:  dlqRetryWorker,
		retentionWorker: retentionWorker,
		ipLimiter:       ipLimiter,
	})
	srv.cancelWorkflowCache = cancelWorkflowCache

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
