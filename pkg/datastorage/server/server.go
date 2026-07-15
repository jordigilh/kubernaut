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
	"database/sql"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/cert"
	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	dsmetrics "github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/retention"
	dsmiddleware "github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
	"github.com/jordigilh/kubernaut/pkg/datastorage/workflowcache"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls" // Issue #493/#678: Conditional TLS
)

// Server is the HTTP server for Data Storage Service
// BR-STORAGE-021: REST API read endpoints
// BR-STORAGE-024: RFC 7807 error responses
//
// DD-007: Kubernetes-aware graceful shutdown with 5-step pattern (see Shutdown method)
// DD-AUTH-014: Middleware-based authentication and authorization
//
// File layout (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3 split, pure code
// motion, no behavior change):
//   - server.go (this file): Server struct, ServerDeps, startupCleanups, constants
//   - server_construction.go: NewServer and its dependency-wiring helpers
//   - server_routes.go: Handler() chi route table
//   - server_lifecycle.go: Start, Shutdown, and loadSigningCertificate
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

	// GAP-09 (Issue #1505) / SC-5: Optional per-IP rate limiter for the HTTP API.
	// nil when disabled (default) — see appCfg.Server.RateLimit.Enabled.
	ipLimiter *dsmiddleware.IPLimiter

	// Issue #1661 Phase 29 / DD-WORKFLOW-018: cancel func for the workflow
	// cache's informers, stopped during graceful shutdown. nil when
	// ServerDeps.K8sRestConfig was not provided (e.g. most unit/integration
	// tests that don't exercise workflow discovery).
	cancelWorkflowCache func()
}

// WorkflowCache returns the informer-backed RemediationWorkflow/ActionType
// CRD cache (Issue #1661 Phase 29 / DD-WORKFLOW-018), or nil if
// ServerDeps.K8sRestConfig was not supplied when the server was built.
func (s *Server) WorkflowCache() *workflowcache.Cache {
	return s.handler.workflowCache
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
	HandlerOpts   []HandlerOption    // Optional handler options (e.g. WithSchemaExtractor)

	// K8sRestConfig is the Kubernetes API server config used to build the
	// Issue #1661 / DD-WORKFLOW-018 workflow cache (informer-backed view of
	// RemediationWorkflow/ActionType CRDs). Optional: when nil, the workflow
	// cache is not built and Server.WorkflowCache() returns nil -- existing
	// callers (most unit/integration tests) are unaffected. cmd/datastorage/
	// main.go always supplies this in production (buildK8sAuthDeps already
	// builds the same rest.Config for DD-AUTH-014's auth middleware).
	K8sRestConfig *rest.Config
}

// startupCleanups accumulates resource-release closures during NewServer so
// that a late-stage startup failure can unwind all previously-acquired
// resources in reverse order (#1048 Phase 4 / RES-M2).
type startupCleanups struct {
	fns []func()
}

func (c *startupCleanups) add(fn func()) {
	c.fns = append(c.fns, fn)
}

func (c *startupCleanups) runAll() {
	for i := len(c.fns) - 1; i >= 0; i-- {
		c.fns[i]()
	}
}
