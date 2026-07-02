/*
Copyright 2026 Jordi Gil.

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

package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"
	wfclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	mcpkg "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcpadapters "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/adapters"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	kametrics "github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	karbac "github.com/jordigilh/kubernaut/internal/kubernautagent/rbac"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

// newAuthMiddleware creates the DD-AUTH-014 auth middleware using the shared k8sInfra clientset.
// DD-AUTH-MCP-001 v2.0: When JWT providers are configured, wraps K8sAuthenticator
// in a CompositeAuthenticator for Pattern A + Pattern B coexistence.
// Returns a cleanup function that stops JWKS background goroutines (nil if no JWT).
func newAuthMiddleware(infra *k8sInfra, interactiveCfg kaconfig.InteractiveConfig, logger logr.Logger) (*auth.Middleware, func()) {
	if infra == nil || infra.clientset == nil {
		logger.Info("K8s infrastructure not available, auth middleware disabled")
		return nil, nil
	}

	k8sAuth := auth.NewK8sAuthenticator(infra.clientset)
	authorizer := auth.NewK8sAuthorizer(infra.clientset)
	namespace := detectNamespace()

	var authenticator auth.Authenticator = k8sAuth
	var cleanup func()

	if interactiveCfg.Enabled && len(interactiveCfg.JWTProviders) > 0 {
		entries := make([]auth.JWTProviderEntry, len(interactiveCfg.JWTProviders))
		for i, p := range interactiveCfg.JWTProviders {
			entries[i] = auth.JWTProviderEntry{
				Issuer:        p.Issuer,
				JWKSURL:       p.JWKSURL,
				Audience:      p.Audience,
				UsernameClaim: p.ClaimMappings.Username,
				GroupsClaim:   p.ClaimMappings.Groups,
				TLSCAFile:     p.TLSCaFile,
			}
		}

		for _, e := range entries {
			if strings.HasPrefix(e.JWKSURL, "http://") {
				logger.Info("WARNING: JWKS URL uses plain HTTP — vulnerable to MITM in production; enforce HTTPS via kubernaut-operator admission webhook (kubernaut-operator#46)",
					"provider", e.Issuer, "jwksURL", e.JWKSURL)
			}
		}

		jwtAuth, err := auth.NewJWTAuthenticator(entries, logger.WithName("jwt-auth"))
		if err != nil {
			logger.Error(err, "failed to create JWTAuthenticator; Pattern B disabled, Pattern A active")
		} else {
			authenticator = auth.NewCompositeAuthenticator(jwtAuth, k8sAuth)
			cleanup = func() {
				jwtAuth.Close()
				logger.Info("JWTAuthenticator JWKS caches stopped")
			}
			logger.Info("CompositeAuthenticator enabled (Pattern A + Pattern B)",
				"jwtProviders", len(entries),
			)
		}
	}

	mw := auth.NewMiddleware(authenticator, authorizer, auth.MiddlewareConfig{
		Namespace:    namespace,
		Resource:     "services",
		ResourceName: "kubernaut-agent",
		Verb:         "create",
	}, logger)
	return mw, cleanup
}

// apiRoutesParams groups the dependencies needed to register the /api/v1
// route tree (body-size limiting, throttling, metrics, rate limiting, auth,
// the optional MCP interactive endpoint, and the ogen-generated REST API).
type apiRoutesParams struct {
	cfg                  *kaconfig.Config
	infra                *k8sInfra
	ds                   *dsClients
	inv                  *investigator.Investigator
	enricher             *enrichment.Enricher
	mgr                  *session.Manager
	agentMetrics         *kametrics.Metrics
	instrumentedAudit    audit.AuditStore
	ogenSrv              http.Handler
	eventEmitter         *karbac.EventEmitter
	interactiveReadiness *karbac.InteractiveReadiness
	apiRateLimiter       *kaserver.RateLimiter
	maxRequestBodySize   int64
	logger               logr.Logger
}

// registerAPIRoutes mounts the /api/v1 route tree onto r: request body-size
// limiting, optional concurrency throttling, HTTP metrics, API rate limiting,
// auth middleware (DD-AUTH-014), the optional MCP interactive endpoint, and
// the ogen-generated REST API as the catch-all. Returns the MCP session
// drainer (nil when interactive mode is disabled or handler construction
// failed) and the auth middleware's JWKS cache cleanup func (nil when auth
// is disabled), both intended to be handled by the caller after this
// function returns.
func registerAPIRoutes(r chi.Router, ctx context.Context, p apiRoutesParams) (*mcpkg.SessionDrainer, func()) {
	var sessionDrainer *mcpkg.SessionDrainer
	var authCleanupRef func()

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				req.Body = http.MaxBytesReader(w, req.Body, p.maxRequestBodySize)
				next.ServeHTTP(w, req)
			})
		})
		if p.cfg.Runtime.Server.MaxConcurrentRequests > 0 {
			p.logger.Info("concurrency throttling enabled",
				"max_concurrent_requests", p.cfg.Runtime.Server.MaxConcurrentRequests)
			r.Use(chimiddleware.Throttle(p.cfg.Runtime.Server.MaxConcurrentRequests))
		}
		r.Use(kaserver.HTTPMetricsMiddleware(p.agentMetrics))
		r.Use(p.apiRateLimiter.Middleware)

		authMw, authCleanup := newAuthMiddleware(p.infra, p.cfg.Interactive, p.logger)
		if authCleanup != nil {
			// authCleanupRef is returned to the caller and deferred in main()'s
			// scope (not here — this is a chi route-setup closure that returns
			// immediately).
			authCleanupRef = authCleanup
		}
		if authMw != nil {
			r.Use(func(next http.Handler) http.Handler {
				return kaserver.AuditAuthMiddleware(authMw.Handler(next), p.instrumentedAudit, p.logger)
			})
			p.logger.Info("auth middleware enabled (DD-AUTH-014)",
				"resource", "services",
				"resourceName", "kubernaut-agent",
				"verb", "create",
			)
		} else {
			p.logger.Info("auth middleware DISABLED (no in-cluster K8s config)")
		}

		if p.cfg.Interactive.Enabled {
			var mcpHandler http.Handler
			mcpHandler, sessionDrainer = buildMCPHandler(ctx, mcpHandlerParams{
				cfg: p.cfg, infra: p.infra, ds: p.ds, inv: p.inv, enricher: p.enricher,
				autoMgr: p.mgr, authMw: authMw, agentMetrics: p.agentMetrics,
				auditStore: p.instrumentedAudit, logger: p.logger,
			})
			if mcpHandler != nil {
				// SEC-02: Per-user rate limiting for MCP interactive endpoint.
				userRL := kaserver.NewUserRateLimiter(
					kaserver.DefaultUserRateLimitConfig(p.cfg.Interactive.RateLimitPerUser),
					p.agentMetrics.HTTPRateLimitedTotal,
				)
				defer userRL.Stop()

				r.Route("/mcp", func(mcpRouter chi.Router) {
					mcpRouter.Use(userRL.Middleware)
					mcpRouter.Handle("/", kaserver.SSEHeadersMiddleware(mcpHandler))
					mcpRouter.Handle("/*", kaserver.SSEHeadersMiddleware(mcpHandler))
				})
				p.interactiveReadiness.SetEnabled()
				if p.eventEmitter != nil {
					p.eventEmitter.EmitInteractiveEnabled()
				}
				p.logger.Info("MCP interactive route mounted",
					"path", "/api/v1/mcp",
					"rateLimitPerUser", p.cfg.Interactive.RateLimitPerUser,
				)
			} else {
				p.interactiveReadiness.SetSoftDisabled("handler construction failed (check preceding errors)")
				if p.eventEmitter != nil {
					p.eventEmitter.EmitInteractiveSoftDisabled("MCP handler construction failed")
				}
				p.logger.Error(nil, "MCP interactive mode enabled but handler construction failed (check preceding errors)")
			}
		}

		r.Handle("/*", kaserver.SSEHeadersMiddleware(p.ogenSrv))
	})

	return sessionDrainer, authCleanupRef
}

// mcpHandlerParams groups the dependencies needed to build the MCP
// interactive-mode HTTP handler. Extracted per AGENTS.md's 8+-param
// Options-pattern rule.
type mcpHandlerParams struct {
	cfg          *kaconfig.Config
	infra        *k8sInfra
	ds           *dsClients
	inv          *investigator.Investigator
	enricher     *enrichment.Enricher
	autoMgr      *session.Manager
	authMw       *auth.Middleware
	agentMetrics *kametrics.Metrics
	auditStore   audit.AuditStore
	logger       logr.Logger
}

// buildMCPHandler constructs the fully-wired MCP interactive handler with all
// tools registered. Returns nil if prerequisites are missing (K8s infra or DS).
// PR6a: Production wiring for MCP interactive mode (BR-INTERACTIVE-001..008).
func buildMCPHandler(ctx context.Context, p mcpHandlerParams) (http.Handler, *mcpkg.SessionDrainer) {
	cfg, infra, ds, inv, enricher, autoMgr, authMw, agentMetrics, auditStore, logger :=
		p.cfg, p.infra, p.ds, p.inv, p.enricher, p.autoMgr, p.authMw, p.agentMetrics, p.auditStore, p.logger

	if infra == nil || infra.kubeConfig == nil {
		logger.Error(nil, "MCP interactive mode: K8s infrastructure unavailable")
		return nil, nil
	}
	if authMw == nil {
		logger.Error(nil, "MCP interactive mode: auth middleware unavailable (DD-AUTH-MCP-001)")
		return nil, nil
	}
	// SEC-05: Investigator is required for the core investigate tool.
	if inv == nil {
		logger.Error(nil, "MCP interactive mode: investigator unavailable")
		return nil, nil
	}

	// SEC-07: Build controller-runtime client with MCP-specific timeouts.
	// Scheme includes remediationv1 for RR existence validation (HARM-004)
	// and future NL signal intake (#714).
	mcpScheme := k8sruntime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(mcpScheme))
	utilruntime.Must(remediationv1.AddToScheme(mcpScheme))

	mcpRestConfig := *infra.kubeConfig
	mcpRestConfig.Timeout = 10 * time.Second
	mcpRestConfig.QPS = 20
	mcpRestConfig.Burst = 40

	ctrlCli, err := ctrlclient.New(&mcpRestConfig, ctrlclient.Options{Scheme: mcpScheme})
	if err != nil {
		logger.Error(err, "MCP interactive mode: failed to create controller-runtime client")
		return nil, nil
	}

	namespace := detectNamespace()

	// emitDisconnectAudit emits interactive.completed for non-tool session endings
	// (disconnect, inactivity timeout, TTL expiry). M1: ensures all session-ending
	// paths produce an audit trail, not just action=complete/cancel through InvestigateTool.
	emitDisconnectAudit := func(sessionID, correlationID, reason string) {
		event := audit.NewEvent(audit.EventTypeInteractiveCompleted, correlationID,
			audit.WithSessionID(sessionID),
		)
		event.EventAction = audit.ActionInteractiveCompleted
		event.EventOutcome = audit.OutcomeSuccess
		event.Data["reason"] = reason
		audit.StoreBestEffort(context.Background(), auditStore, event, logger.WithName("mcp-audit"))
	}

	// Session management via K8s Leases (single-driver guarantee).
	leaseOpts := []mcpkg.LeaseOption{
		mcpkg.WithSessionTTL(cfg.Interactive.SessionTTL),
		mcpkg.WithInactivityTimeout(cfg.Interactive.InactivityTimeout),
		mcpkg.WithMaxConcurrentSessions(cfg.Interactive.MaxConcurrentSessions),
		mcpkg.WithSessionExpiredCallback(func(sessionID, rrID, reason string) {
			// #1438: Emit terminal event BEFORE completing the HTTP session so
			// EventLogBridge can forward it to AF before the channel closes.
			autoMgr.EmitSessionEndedByRR(rrID, reason)
			mcptools.CompleteHTTPSession(autoMgr, rrID, nil, logger, reason)
			emitDisconnectAudit(sessionID, rrID, reason)
			agentMetrics.RecordInteractiveSessionEnded()
		}),
	}
	leaseMgr := mcpkg.NewLeaseSessionManagerConcrete(ctrlCli, namespace, logger, leaseOpts...)

	if n := leaseMgr.ReconcileOrphanedLeases(context.Background()); n > 0 {
		logger.Info("startup: reclaimed orphaned interactive Leases", "count", n)
	}

	// Context reconstruction from DS audit events (best-effort).
	var recon mcpkg.ContextReconstructor
	if ds != nil {
		recon = mcpkg.NewDSContextReconstructor(ds.ogenClient, 10*time.Second, logger)
	} else {
		recon = &noopReconstructor{}
		logger.Info("MCP interactive mode: DS unavailable — context reconstruction disabled")
	}

	// DelegatingEventStore: bridges MCP SDK session lifecycle to our disconnect
	// handler. Wraps SDK's MemoryEventStore for stream resumption support.
	eventStore := mcpkg.NewDelegatingEventStore()

	// TimeoutManager: fires onExpire when a session goes inactive (SEC-04, HARM-03/04).
	timeoutMgr := mcpkg.NewTimeoutManager(
		cfg.Interactive.InactivityTimeout,
		[]time.Duration{cfg.Interactive.InactivityTimeout - 2*time.Minute, cfg.Interactive.InactivityTimeout - 30*time.Second},
		func(sessionID string) {
			logger.Info("interactive session expired due to inactivity",
				"session_id", sessionID)
			// Snapshot correlationID before Release deletes the entry.
			rrID, _ := leaseMgr.GetSessionInfo(sessionID)
			// #1438: Emit terminal event BEFORE closing the HTTP session so the
			// EventLogBridge can forward it to AF before the channel closes.
			autoMgr.EmitSessionEndedByRR(rrID, "inactivity_timeout")
			if err := leaseMgr.Release(sessionID, "inactivity_timeout"); err != nil {
				logger.Error(err, "failed to release expired session",
					"session_id", sessionID)
				return
			}
			// KA-CRIT-2: Resolve the HTTP session so AA stops polling user_driving.
			mcptools.CompleteHTTPSession(autoMgr, rrID, nil, logger, "inactivity_timeout")
			emitDisconnectAudit(sessionID, rrID, "inactivity_timeout")
			// T1-4: Decrement gauge on timeout expiry to prevent drift.
			agentMetrics.RecordInteractiveSessionEnded()
		},
	)

	// ReconstructionSpawner: rebuilds context and spawns autonomous investigation
	// after an interactive session ends (INT-06, BR-INTERACTIVE-008).
	reconRunner := mcpadapters.NewReconRunnerAdapter(inv)
	reconSpawner := mcpkg.NewReconstructionSpawner(reconRunner, recon, logger)

	// GracefulSessionClosedHandler: processes MCP disconnect events with a
	// configurable grace period before releasing the interactive lease.
	// BR-INTERACTIVE-001: decouples MCP transport lifecycle from lease lifecycle.
	// If a client reconnects during the grace period, the lease is preserved.
	disconnectGracePeriod := cfg.Interactive.DisconnectGracePeriod
	if disconnectGracePeriod <= 0 {
		disconnectGracePeriod = 60 * time.Second
	}
	disconnectHandler := mcpkg.NewGracefulSessionClosedHandler(eventStore, func(mcpSessionID string) {
		interactiveSessionID, ok := eventStore.LookupInteractiveSession(mcpSessionID)
		if !ok {
			logger.V(1).Info("MCP session closed without interactive mapping (autonomous or already released)",
				"mcp_session_id", mcpSessionID)
			return
		}

		eventStore.DeleteMCPSession(mcpSessionID)

		timeoutMgr.StopTracking(interactiveSessionID)

		// T1-1: Snapshot session info BEFORE Release deletes the entry.
		rrID, signalMeta := leaseMgr.GetSessionInfo(interactiveSessionID)

		// #1438: Emit terminal event BEFORE closing the HTTP session so the
		// EventLogBridge can forward it to AF before the channel closes.
		autoMgr.EmitSessionEndedByRR(rrID, "disconnect")

		if err := leaseMgr.Release(interactiveSessionID, "disconnect"); err != nil {
			logger.Info("failed to release disconnected session",
				"session_id", interactiveSessionID,
				"error", err.Error())
			return
		}

		// KA-CRIT-2: Resolve the HTTP session so AA stops polling user_driving.
		mcptools.CompleteHTTPSession(autoMgr, rrID, nil, logger, "disconnect")

		emitDisconnectAudit(interactiveSessionID, rrID, "disconnect")

		// T1-4: Decrement gauge on disconnect to prevent drift.
		agentMetrics.RecordInteractiveSessionEnded()

		// Spawn reconstruction in background (best-effort, BR-INTERACTIVE-008).
		go func() {
			reconCtx, reconCancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer reconCancel()
			if err := reconSpawner.SpawnReconstruct(reconCtx, &mcpkg.ReconstructionContext{
				CorrelationID: rrID,
				SessionID:     interactiveSessionID,
				SignalMeta:    signalMeta,
			}); err != nil {
				logger.Error(err, "background reconstruction failed",
					"correlationID", rrID, "sessionID", interactiveSessionID)
			}
		}()
	}, disconnectGracePeriod, logger)

	// Wire reconnect callback: when Takeover detects a same-user reconnect,
	// cancel the pending graceful release (BR-INTERACTIVE-001, #1442).
	leaseMgr.SetReconnectCallback(func(sessionID string) {
		if disconnectHandler.CancelPendingRelease(sessionID) {
			logger.Info("pending disconnect release cancelled (client reconnected)",
				"session_id", sessionID)
		}
	})

	// Start disconnect handler goroutine.
	go disconnectHandler.Run(ctx)

	// Build the InvestigatorRunner adapter.
	investigatorRunner := mcpadapters.NewInvestigatorRunnerAdapter(inv)

	// SEC-HIGH-01: Per-session message rate limiter (maxMessageSize = 64KB).
	sessionRateLimiter := mcpkg.NewSessionRateLimiter(cfg.Interactive.RateLimitPerUser, 64*1024)

	// UX-01/02: Session notifier delivers timeout warnings to MCP clients.
	sessionNotifier := mcpkg.NewSessionNotifier()

	// HARM-004: Validate RR existence before creating interactive Leases.
	rrChecker := mcptools.NewK8sRRExistenceChecker(ctrlCli, namespace)

	// Signal context resolver: reads the SignalContext stored on the session
	// from the original AA IncidentRequest payload. Falls back to reading
	// the RR CRD for sessions without stored signal (e.g. interactive sessions
	// started directly via MCP without an AA payload).
	signalResolver := mcpadapters.NewSessionSignalContextResolver(autoMgr, ctrlCli, namespace)

	// Build the WorkflowCatalog adapter (shared between InvestigateTool and SelectWorkflowTool).
	wfQuerier := wfclient.NewOgenWorkflowQuerier(ds.ogenClient)
	catalogAdapter := mcpadapters.NewWorkflowCatalogAdapter(wfQuerier)

	// Build the InvestigateTool with optional dependencies.
	investigateOpts := []mcptools.InvestigateOption{
		mcptools.WithToolMetrics(agentMetrics),
		mcptools.WithRateLimiter(sessionRateLimiter),
		mcptools.WithTimeoutTracker(timeoutMgr),
		mcptools.WithNotifyFunc(sessionNotifier.Notify),
		mcptools.WithRRExistenceChecker(rrChecker),
		mcptools.WithHTTPCompleter(autoMgr),
		mcptools.WithAuditStore(auditStore, logger.WithName("mcp-audit")),
		mcptools.WithSignalContextResolver(signalResolver),
		mcptools.WithWorkflowCatalog(catalogAdapter),
	}
	investigateTool := mcptools.NewInvestigateTool(leaseMgr, investigatorRunner, recon, autoMgr, investigateOpts...)

	// Build SelectWorkflowTool (reuses the same catalogAdapter).
	// #1012: enrichment is now internalized into select_workflow via WithEnrichmentRunner.
	swOpts := []mcptools.SelectWorkflowOption{
		mcptools.WithLogger(logger.WithName("select-workflow")),
		mcptools.WithHTTPSessionCompleter(autoMgr),
		mcptools.WithMutexProvider(investigateTool),
	}
	if enricher != nil {
		swOpts = append(swOpts, mcptools.WithEnrichmentRunner(enricher))
	}
	selectWfTool := mcptools.NewSelectWorkflowTool(catalogAdapter, leaseMgr, swOpts...)

	// Build the CompleteNoActionTool.
	completeNoActionTool := mcptools.NewCompleteNoActionTool(leaseMgr,
		mcptools.WithCompleteNoActionLogger(logger.WithName("complete-no-action")),
		mcptools.WithCompleteNoActionHTTPCompleter(autoMgr),
		mcptools.WithCompleteNoActionMutexProvider(investigateTool),
	)

	// Register tools with the MCP SDK server.
	toolDeps := mcpkg.ToolDeps{}
	toolDeps.Investigate = mcptools.InvestigateRegistration(investigateTool, eventStore, sessionNotifier, logger)
	toolDeps.SelectWorkflow = mcptools.SelectWorkflowRegistration(selectWfTool, logger)
	toolDeps.CompleteNoAction = mcptools.CompleteNoActionRegistration(completeNoActionTool, logger)

	mcpKeepAlive := cfg.Interactive.MCPKeepAlive
	if mcpKeepAlive == 0 {
		mcpKeepAlive = kaconfig.DefaultMCPKeepAlive
	}
	mcpSessionTimeout := cfg.Interactive.MCPSessionTimeout
	if mcpSessionTimeout == 0 {
		mcpSessionTimeout = kaconfig.DefaultMCPSessionTimeout
	}
	mcpHandler, _ := mcpkg.BootstrapMCP(mcpkg.MCPDeps{
		AuthMiddleware: func(next http.Handler) http.Handler {
			return kaserver.AuditAuthMiddleware(authMw.Handler(next), auditStore, logger)
		},
		Tools:          toolDeps,
		EventStore:     eventStore,
		KeepAlive:      mcpKeepAlive,
		SessionTimeout: mcpSessionTimeout,
	})

	drainer := mcpkg.NewSessionDrainer(leaseMgr, sessionNotifier, logger.WithName("session-drainer"))

	logger.Info("MCP interactive mode fully wired",
		"investigate", true,
		"select_workflow", true,
		"complete_no_action", true,
		"enrichment_in_select_workflow", enricher != nil,
		"event_store", true,
		"timeout_manager", true,
		"disconnect_handler", true,
		"reconstruction_spawner", true,
		"notification_bus", true,
		"session_drainer", true,
	)

	return mcpHandler, drainer
}

// noopReconstructor is a no-op ContextReconstructor used when DS is unavailable.
type noopReconstructor struct{}

func (n *noopReconstructor) Reconstruct(_ context.Context, _ string, _ string) ([]mcpkg.ConversationTurn, error) {
	return nil, nil
}
