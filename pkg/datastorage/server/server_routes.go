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
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	dsmiddleware "github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// Handler returns the configured HTTP handler for the server
// This is useful for testing with httptest.NewServer
//
// Split from server.go (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3, pure code
// motion, no behavior change); see server_construction.go for NewServer's
// dependency wiring and server_lifecycle.go for Start/Shutdown.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	s.registerGlobalMiddleware(r)

	// API v1 routes
	s.logger.V(1).Info("Setting up API v1 routes",
		"handler_nil", s.handler == nil,
		"repository_nil", s.repository == nil,
		"validator_nil", s.validator == nil,
		"dlq_client_nil", s.dlqClient == nil)

	r.Route("/api/v1", func(r chi.Router) {
		writeAuthMiddleware, mutateAuthMiddleware := s.registerAPIV1AuthMiddleware(r)
		s.registerAuditRoutes(r, writeAuthMiddleware, mutateAuthMiddleware)
		s.registerWorkflowRoutes(r)
	})

	s.logger.V(1).Info("API v1 routes configured successfully")

	return r
}

// registerGlobalMiddleware wires the router-wide middleware chain: request
// ID/real-IP, logging, panic recovery, request-body size limiting (SC-5),
// the optional per-IP rate limiter (GAP-09 / Issue #1505), and CORS
// (AC-4 / ADR-030). Extracted from Handler (Wave 6 6f GREEN: funlen
// remediation) — pure code motion, no behavior change.
func (s *Server) registerGlobalMiddleware(r chi.Router) {
	r.Use(middleware.RequestID)      // Add X-Request-ID
	r.Use(middleware.RealIP)         // Get real client IP
	r.Use(s.loggingMiddleware)       // Custom logging middleware
	r.Use(s.panicRecoveryMiddleware) // Enhanced panic recovery with logging
	// #1048 Phase 4 / SC-5: Request body size limit (DoS protection).
	// Applied before OpenAPI validation so the body is capped before spec parsing.
	r.Use(dsmiddleware.MaxBytesReaderMiddleware(s.maxBodySize, s.logger))

	// GAP-09 (Issue #1505) / SC-5: Optional per-IP rate limiting, pre-authentication.
	// Placed before auth so an unauthenticated flood does not reach TokenReview/SAR calls.
	if s.ipLimiter != nil {
		r.Use(dsmiddleware.IPRateLimitMiddleware(dsmiddleware.IPRateLimitMiddlewareConfig{
			Limiter: s.ipLimiter,
			Logger:  s.logger,
			AuditFunc: func(ctx context.Context, sourceIP, path, method string) {
				if s.auditStore == nil {
					return
				}
				auditCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				if err := s.auditStore.StoreAudit(auditCtx, dsaudit.NewRatelimitDeniedAuditEvent(sourceIP, path, method)); err != nil {
					s.logger.Error(err, "Failed to persist rate-limit denial audit event",
						"source_ip", sourceIP, "path", path, "method", method)
				}
			},
		}))
	}

	// AC-4: CORS with configurable origins (ADR-030).
	// SEC-C1: go-chi/cors treats empty AllowedOrigins as "allow all".
	// Use AllowOriginFunc to deny all cross-origin when list is empty.
	corsOpts := cors.Options{
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}
	if len(s.corsAllowedOrigins) > 0 {
		corsOpts.AllowedOrigins = s.corsAllowedOrigins
	} else {
		corsOpts.AllowOriginFunc = func(_ *http.Request, _ string) bool { return false }
	}
	r.Use(cors.Handler(corsOpts))
}

// registerAPIV1AuthMiddleware wires the /api/v1 auth chain (SEC-H2: auth
// before OpenAPI validation, DD-AUTH-011 tiered SAR verbs) and returns the
// write ("create") and mutate ("update") middlewares for route-level use.
// The base "get" middleware and OpenAPI validator are applied router-wide
// within this /api/v1 group. Extracted from Handler (Wave 6 6f GREEN:
// funlen remediation) — pure code motion, no behavior change.
func (s *Server) registerAPIV1AuthMiddleware(r chi.Router) (writeAuthMiddleware, mutateAuthMiddleware *auth.Middleware) {
	// SEC-H2: Auth runs BEFORE OpenAPI validation so unauthenticated
	// requests get 401, not a 400 from spec validation.
	// DD-AUTH-011: Base middleware requires SAR verb "get" — read access to DS.
	baseAuthMiddleware := auth.NewMiddleware(
		s.authenticator,
		s.authorizer,
		auth.MiddlewareConfig{
			Namespace:    s.authNamespace,
			Resource:     "services",
			ResourceName: "data-storage-service",
			Verb:         "get",
		},
		s.logger,
	)
	r.Use(baseAuthMiddleware.Handler)

	// BR-STORAGE-034: OpenAPI validation after auth (SEC-H2)
	r.Use(s.openapiValidator.Middleware)
	s.logger.Info("Auth middleware enabled (DD-AUTH-014, SEC-1 tiered)",
		"namespace", s.authNamespace,
		"resource", "services",
		"resourceName", "data-storage-service",
	)

	// DD-AUTH-011: Write middleware requires SAR verb "create" — mutating operations.
	// Layered on top of base "get" so callers need both read + write access.
	writeAuthMiddleware = auth.NewMiddleware(
		s.authenticator,
		s.authorizer,
		auth.MiddlewareConfig{
			Namespace:    s.authNamespace,
			Resource:     "services",
			ResourceName: "data-storage-service",
			Verb:         "create",
		},
		s.logger,
	)

	// DD-AUTH-011: Mutate middleware requires SAR verb "update" — PATCH operations
	// and admin-only endpoints (legal hold management).
	mutateAuthMiddleware = auth.NewMiddleware(
		s.authenticator,
		s.authorizer,
		auth.MiddlewareConfig{
			Namespace:    s.authNamespace,
			Resource:     "services",
			ResourceName: "data-storage-service",
			Verb:         "update",
		},
		s.logger,
	)

	return writeAuthMiddleware, mutateAuthMiddleware
}

// registerAuditRoutes registers the audit-trail endpoints (write API,
// unified audit events, batch, verify-chain, legal hold, export,
// reconstruction, and effectiveness scoring). Extracted from Handler
// (Wave 6 6f GREEN: funlen remediation) — pure code motion, no behavior
// change.
func (s *Server) registerAuditRoutes(r chi.Router, writeAuthMiddleware, mutateAuthMiddleware *auth.Middleware) {
	// BR-STORAGE-001 to BR-STORAGE-020: Audit write endpoints (WRITE API)
	s.logger.V(1).Info("Registering POST /api/v1/audit/notifications handler")
	r.With(writeAuthMiddleware.Handler).Post("/audit/notifications", s.handleCreateNotificationAudit)

	// BR-STORAGE-033: Unified audit events API (ADR-034)
	// DD-STORAGE-010: Query API with offset-based pagination
	s.logger.V(1).Info("Registering /api/v1/audit/events handlers (ADR-034, DD-STORAGE-010)")
	r.With(writeAuthMiddleware.Handler).Post("/audit/events", s.handleCreateAuditEvent)
	r.Get("/audit/events", s.handleQueryAuditEvents)

	// DD-AUDIT-002: Batch audit events API for HTTPDataStorageClient.StoreBatch()
	// BR-AUDIT-001: Complete audit trail with no data loss
	s.logger.V(1).Info("Registering /api/v1/audit/events/batch handler (DD-AUDIT-002)")
	r.With(writeAuthMiddleware.Handler).Post("/audit/events/batch", s.handleCreateAuditEventsBatch)

	// SOC2 Gap #9: Tamper detection verification API (PostgreSQL-based)
	// BR-AUDIT-005: Enterprise-Grade Audit Integrity
	s.logger.V(1).Info("Registering /api/v1/audit/verify-chain handler (SOC2 Gap #9)")
	r.With(writeAuthMiddleware.Handler).Post("/audit/verify-chain", s.HandleVerifyChain)

	// SOC2 Gap #8: Legal Hold & Retention Policies
	// BR-AUDIT-008: Legal hold capability for Sarbanes-Oxley and HIPAA compliance
	// SEC-1/AC-6: GET uses base "get" verb; POST/DELETE require "update" (admin-only)
	s.logger.V(1).Info("Registering /api/v1/audit/legal-hold handlers (SOC2 Gap #8, SEC-1 admin tier)")
	r.Get("/audit/legal-hold", s.HandleListLegalHolds)
	r.With(mutateAuthMiddleware.Handler).Post("/audit/legal-hold", s.HandlePlaceLegalHold)
	r.With(mutateAuthMiddleware.Handler).Delete("/audit/legal-hold/{correlation_id}", s.HandleReleaseLegalHold)

	// SOC2 Day 9: Signed Audit Export
	// BR-AUDIT-007: Audit export with digital signatures for compliance verification
	s.logger.V(1).Info("Registering /api/v1/audit/export handler (SOC2 Day 9)")
	r.Get("/audit/export", s.HandleExportAuditEvents)

	// BR-RR-RECON-001: RemediationRequest Reconstruction from Audit Traces
	// SOC2 compliance: Reconstruct complete RR CRDs from audit trail
	s.logger.V(1).Info("Registering /api/v1/audit/remediation-requests/{correlation_id}/reconstruct handler")
	r.With(writeAuthMiddleware.Handler).Post("/audit/remediation-requests/{correlation_id}/reconstruct", s.handleReconstructRemediationRequestWrapper)

	// BR-EM-001 to BR-EM-004: On-demand effectiveness scoring (DD-017 v2.1, ADR-EM-001 Principle 5)
	s.logger.V(1).Info("Registering GET /api/v1/effectiveness/{correlation_id} handler")
	r.Get("/effectiveness/{correlation_id}", s.handleGetEffectivenessScore)

	// BR-HAPI-016: Remediation history context for LLM prompt enrichment (DD-HAPI-016 v1.1)
	s.logger.V(1).Info("Registering GET /api/v1/remediation-history/context handler")
	r.Get("/remediation-history/context", s.handler.HandleGetRemediationHistoryContext)
}

// registerWorkflowRoutes registers the workflow catalog and action-type
// taxonomy read endpoints (three-step workflow discovery protocol).
// Extracted from Handler (Wave 6 6f GREEN: funlen remediation) — pure code
// motion, no behavior change.
//
// #1661 Phase B: the workflow mutation routes (POST /workflows, PATCH
// .../{disable,enable,deprecate,update}) were removed -- AuthWebhook
// admission now owns the RemediationWorkflow CRD lifecycle entirely locally
// (DD-WORKFLOW-018), mirroring Phase A3's ActionType-side removal. Every
// remaining route here is a read (GET), so this function no longer needs
// writeAuthMiddleware/mutateAuthMiddleware.
func (s *Server) registerWorkflowRoutes(r chi.Router) {
	// BR-STORAGE-014: Workflow catalog management
	// DD-WORKFLOW-002 v3.0: UUID primary key for workflow retrieval
	s.logger.V(1).Info("Registering /api/v1/workflows handlers (BR-STORAGE-014)")
	r.Get("/workflows", s.handler.HandleListWorkflows)
	// DD-WORKFLOW-016, DD-HAPI-017: Three-step workflow discovery protocol
	// Step 1: List available action types (with signal context filters)
	r.Get("/workflows/actions", s.handler.HandleListAvailableActions)
	// Step 2: List workflows for a specific action type
	r.Get("/workflows/actions/{action_type}", s.handler.HandleListWorkflowsByActionType)
	// Step 3 + existing: Get workflow by UUID (with optional security gate via context filters)
	r.Get("/workflows/{workflowID}", s.handler.HandleGetWorkflowByID)

	// BR-WORKFLOW-007: ActionType workflow-count query (ADR-059, DD-ACTIONTYPE-001).
	// #1661 Phase A3: createActionType/updateActionType/disableActionType were
	// removed -- AuthWebhook admission now owns the ActionType CRD lifecycle
	// entirely locally (DD-WORKFLOW-018); there is no DS-side mutation path.
	s.logger.V(1).Info("Registering /api/v1/action-types handlers (BR-WORKFLOW-007)")
	r.Get("/action-types/{name}/workflow-count", s.handler.HandleGetActionTypeWorkflowCount)
}
