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
	"encoding/json"
	"fmt"
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
	dsmodels "github.com/jordigilh/kubernaut/pkg/datastorage/models"
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

		authMw, authCleanup := newAuthMiddleware(p.infra, p.cfg.Interactive, p.logger) //nolint:contextcheck // newAuthMiddleware wires JWT authenticator construction once at startup; no parent request context exists yet
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
			sessionDrainer = mountInteractiveMCPRoute(r, ctx, p, authMw)
		}

		r.Handle("/*", kaserver.SSEHeadersMiddleware(p.ogenSrv))
	})

	return sessionDrainer, authCleanupRef
}

// mountInteractiveMCPRoute builds the MCP interactive handler and, on
// success, mounts it under /api/v1/mcp with per-user rate limiting (SEC-02),
// signaling interactive readiness accordingly. On handler construction
// failure, marks interactive readiness as soft-disabled instead of mounting
// the route. Returns the session drainer for graceful shutdown (nil when
// construction failed).
func mountInteractiveMCPRoute(r chi.Router, ctx context.Context, p apiRoutesParams, authMw *auth.Middleware) *mcpkg.SessionDrainer {
	mcpHandler, sessionDrainer := buildMCPHandler(ctx, mcpHandlerParams{
		cfg: p.cfg, infra: p.infra, ds: p.ds, inv: p.inv, enricher: p.enricher,
		autoMgr: p.mgr, authMw: authMw, agentMetrics: p.agentMetrics,
		auditStore: p.instrumentedAudit, logger: p.logger,
	})
	if mcpHandler == nil {
		p.interactiveReadiness.SetSoftDisabled("handler construction failed (check preceding errors)")
		if p.eventEmitter != nil {
			p.eventEmitter.EmitInteractiveSoftDisabled("MCP handler construction failed")
		}
		p.logger.Error(nil, "MCP interactive mode enabled but handler construction failed (check preceding errors)")
		return nil
	}

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
	return sessionDrainer
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

// resolveContextReconstructor returns the DS-backed ContextReconstructor
// when DS is available, or a no-op fail-open implementation otherwise.
func resolveContextReconstructor(ds *dsClients, logger logr.Logger) mcpkg.ContextReconstructor {
	if ds == nil {
		logger.Info("MCP interactive mode: DS unavailable — context reconstruction disabled")
		return &noopReconstructor{}
	}
	return mcpkg.NewDSContextReconstructor(ds.ogenClient, 10*time.Second, logger)
}

// resolveWorkflowQuerier returns the DS-backed WorkflowQuerier when DS is
// available, or a no-op fail-open implementation otherwise (mirrors
// resolveContextReconstructor's fail-open pattern).
func resolveWorkflowQuerier(ds *dsClients, logger logr.Logger) wfclient.WorkflowQuerier {
	if ds == nil {
		logger.Info("MCP interactive mode: DS unavailable — workflow catalog lookups disabled")
		return &noopWorkflowQuerier{}
	}
	return wfclient.NewOgenWorkflowQuerier(ds.ogenClient)
}

// resolveMCPTimeouts applies the DefaultMCPKeepAlive/DefaultMCPSessionTimeout
// fallbacks for any zero-valued interactive config timeouts.
func resolveMCPTimeouts(cfg *kaconfig.Config) (keepAlive, sessionTimeout time.Duration) {
	keepAlive = cfg.Interactive.MCPKeepAlive
	if keepAlive == 0 {
		keepAlive = kaconfig.DefaultMCPKeepAlive
	}
	sessionTimeout = cfg.Interactive.MCPSessionTimeout
	if sessionTimeout == 0 {
		sessionTimeout = kaconfig.DefaultMCPSessionTimeout
	}
	return keepAlive, sessionTimeout
}

// mcpCoreDeps groups the session/lease/timeout/reconstruction/disconnect
// infrastructure built once at MCP interactive-handler construction time by
// buildMCPCoreDeps, and consumed by the remainder of buildMCPHandler.
type mcpCoreDeps struct {
	ctrlCli           ctrlclient.Client
	namespace         string
	leaseMgr          *mcpkg.LeaseSessionManager
	recon             mcpkg.ContextReconstructor
	eventStore        *mcpkg.DelegatingEventStore
	timeoutMgr        *mcpkg.TimeoutManager
	disconnectHandler *mcpkg.GracefulSessionClosedHandler
}

// buildMCPCoreDeps wires the controller-runtime client (SEC-07), the
// K8s-Lease-backed session manager, context reconstruction, the SDK event
// store, the inactivity TimeoutManager (SEC-04, HARM-03/04), and the
// GracefulSessionClosedHandler (BR-INTERACTIVE-001) — including its
// reconnect callback (#1442) and background Run goroutine. Returns an error
// when the controller-runtime client cannot be constructed.
func buildMCPCoreDeps(ctx context.Context, p mcpHandlerParams) (*mcpCoreDeps, error) {
	cfg, infra, ds, inv, autoMgr, agentMetrics, auditStore, logger :=
		p.cfg, p.infra, p.ds, p.inv, p.autoMgr, p.agentMetrics, p.auditStore, p.logger

	// SEC-07: Build controller-runtime client with MCP-specific timeouts.
	// Scheme includes remediationv1 for RR existence validation (HARM-004)
	// and future NL signal intake (#714).
	ctrlCli, err := buildMCPControllerClient(infra)
	if err != nil {
		return nil, err
	}

	namespace := detectNamespace()

	// emitDisconnectAudit emits interactive.completed for non-tool session endings
	// (disconnect, inactivity timeout, TTL expiry). M1: ensures all session-ending
	// paths produce an audit trail, not just action=complete/cancel through InvestigateTool.
	emitDisconnectAudit := newDisconnectAuditEmitter(auditStore, logger) //nolint:contextcheck // disconnect audit emitter fires asynchronously on session disconnect events, not tied to any single request

	// Session management via K8s Leases (single-driver guarantee).
	leaseMgr := buildMCPLeaseManager(ctrlCli, namespace, cfg, autoMgr, agentMetrics, logger, emitDisconnectAudit) //nolint:contextcheck // buildMCPLeaseManager wires session lease management once at startup; no parent request context exists yet

	// Context reconstruction from DS audit events (best-effort).
	recon := resolveContextReconstructor(ds, logger)

	// DelegatingEventStore: bridges MCP SDK session lifecycle to our disconnect
	// handler. Wraps SDK's MemoryEventStore for stream resumption support.
	eventStore := mcpkg.NewDelegatingEventStore()

	// TimeoutManager: fires onExpire when a session goes inactive (SEC-04, HARM-03/04).
	timeoutMgr := buildMCPTimeoutManager(cfg, autoMgr, leaseMgr, agentMetrics, logger, emitDisconnectAudit) //nolint:contextcheck // session Release must succeed on its own bounded context regardless of the caller's (drain/timeout/disconnect) context state

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
	disconnectHandler := buildMCPDisconnectHandler(mcpDisconnectHandlerDeps{ //nolint:contextcheck // session Release must succeed on its own bounded context regardless of the caller's (drain/timeout/disconnect) context state
		eventStore:          eventStore,
		timeoutMgr:          timeoutMgr,
		leaseMgr:            leaseMgr,
		autoMgr:             autoMgr,
		reconSpawner:        reconSpawner,
		agentMetrics:        agentMetrics,
		logger:              logger,
		emitDisconnectAudit: emitDisconnectAudit,
		gracePeriod:         disconnectGracePeriod,
	})

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

	return &mcpCoreDeps{
		ctrlCli: ctrlCli, namespace: namespace, leaseMgr: leaseMgr, recon: recon,
		eventStore: eventStore, timeoutMgr: timeoutMgr, disconnectHandler: disconnectHandler,
	}, nil
}

// buildAndRegisterMCPTools constructs the InvestigateTool, SelectWorkflowTool,
// and CompleteNoActionTool (with their shared rate-limiter, RR-existence
// checker, signal-context resolver, and workflow-catalog adapter), then
// registers them with the MCP SDK server. Returns the registered ToolDeps
// and the SessionNotifier (needed later for the session drainer).
func buildAndRegisterMCPTools(core *mcpCoreDeps, p mcpHandlerParams) (mcpkg.ToolDeps, *mcpkg.SessionNotifier) {
	cfg, ds, inv, enricher, autoMgr, agentMetrics, auditStore, logger :=
		p.cfg, p.ds, p.inv, p.enricher, p.autoMgr, p.agentMetrics, p.auditStore, p.logger

	// Build the InvestigatorRunner adapter.
	investigatorRunner := mcpadapters.NewInvestigatorRunnerAdapter(inv)

	// SEC-HIGH-01: Per-session message rate limiter (maxMessageSize = 64KB).
	sessionRateLimiter := mcpkg.NewSessionRateLimiter(cfg.Interactive.RateLimitPerUser, 64*1024)

	// UX-01/02: Session notifier delivers timeout warnings to MCP clients.
	sessionNotifier := mcpkg.NewSessionNotifier()

	// HARM-004: Validate RR existence before creating interactive Leases.
	rrChecker := mcptools.NewK8sRRExistenceChecker(core.ctrlCli, core.namespace)

	// Signal context resolver: reads the SignalContext stored on the session
	// from the original AA IncidentRequest payload. Falls back to reading
	// the RR CRD for sessions without stored signal (e.g. interactive sessions
	// started directly via MCP without an AA payload).
	signalResolver := mcpadapters.NewSessionSignalContextResolver(autoMgr, core.ctrlCli, core.namespace)

	// Build the WorkflowCatalog adapter (shared between InvestigateTool and SelectWorkflowTool).
	// DS is optional at startup (same fail-open contract as recon above and
	// buildToolRegistry/readinessHandler elsewhere in this package): when
	// unavailable, catalog lookups fail per-call with a clear error instead
	// of panicking the whole MCP interactive-mode handler at construction.
	wfQuerier := resolveWorkflowQuerier(ds, logger)
	catalogAdapter := mcpadapters.NewWorkflowCatalogAdapter(wfQuerier)

	investigateTool, selectWfTool, completeNoActionTool := buildMCPTools(mcpToolsDeps{
		leaseMgr:           core.leaseMgr,
		investigatorRunner: investigatorRunner,
		recon:              core.recon,
		autoMgr:            autoMgr,
		agentMetrics:       agentMetrics,
		sessionRateLimiter: sessionRateLimiter,
		timeoutMgr:         core.timeoutMgr,
		sessionNotifier:    sessionNotifier,
		rrChecker:          rrChecker,
		auditStore:         auditStore,
		logger:             logger,
		signalResolver:     signalResolver,
		catalogAdapter:     catalogAdapter,
		enricher:           enricher,
	})

	// Register tools with the MCP SDK server.
	toolDeps := mcpkg.ToolDeps{}
	toolDeps.Investigate = mcptools.InvestigateRegistration(investigateTool, core.eventStore, sessionNotifier, logger)
	toolDeps.SelectWorkflow = mcptools.SelectWorkflowRegistration(selectWfTool, logger)
	toolDeps.CompleteNoAction = mcptools.CompleteNoActionRegistration(completeNoActionTool, logger)
	return toolDeps, sessionNotifier
}

// buildMCPHandler constructs the fully-wired MCP interactive handler with all
// tools registered. Returns nil if prerequisites are missing (K8s infra or DS).
// PR6a: Production wiring for MCP interactive mode (BR-INTERACTIVE-001..008).
func buildMCPHandler(ctx context.Context, p mcpHandlerParams) (http.Handler, *mcpkg.SessionDrainer) {
	cfg, authMw, auditStore, logger := p.cfg, p.authMw, p.auditStore, p.logger

	if !checkMCPPrerequisites(p) {
		return nil, nil
	}

	core, err := buildMCPCoreDeps(ctx, p)
	if err != nil {
		logger.Error(err, "MCP interactive mode: failed to create controller-runtime client")
		return nil, nil
	}

	toolDeps, sessionNotifier := buildAndRegisterMCPTools(core, p)

	mcpKeepAlive, mcpSessionTimeout := resolveMCPTimeouts(cfg)
	mcpHandler, _ := mcpkg.BootstrapMCP(mcpkg.MCPDeps{
		AuthMiddleware: func(next http.Handler) http.Handler {
			return kaserver.AuditAuthMiddleware(authMw.Handler(next), auditStore, logger)
		},
		Tools:          toolDeps,
		EventStore:     core.eventStore,
		KeepAlive:      mcpKeepAlive,
		SessionTimeout: mcpSessionTimeout,
	})

	drainer := mcpkg.NewSessionDrainer(core.leaseMgr, sessionNotifier, logger.WithName("session-drainer"))

	logger.Info("MCP interactive mode fully wired",
		"investigate", true,
		"select_workflow", true,
		"complete_no_action", true,
		"enrichment_in_select_workflow", p.enricher != nil,
		"event_store", true,
		"timeout_manager", true,
		"disconnect_handler", true,
		"reconstruction_spawner", true,
		"notification_bus", true,
		"session_drainer", true,
	)

	return mcpHandler, drainer
}

// checkMCPPrerequisites validates the guards required before MCP interactive
// mode construction can proceed, logging the specific reason for each
// failure. Returns false when K8s infrastructure, the auth middleware
// (DD-AUTH-MCP-001), or the investigator (SEC-05) are unavailable.
func checkMCPPrerequisites(p mcpHandlerParams) bool {
	if p.infra == nil || p.infra.kubeConfig == nil {
		p.logger.Error(nil, "MCP interactive mode: K8s infrastructure unavailable")
		return false
	}
	if p.authMw == nil {
		p.logger.Error(nil, "MCP interactive mode: auth middleware unavailable (DD-AUTH-MCP-001)")
		return false
	}
	// SEC-05: Investigator is required for the core investigate tool.
	if p.inv == nil {
		p.logger.Error(nil, "MCP interactive mode: investigator unavailable")
		return false
	}
	return true
}

// buildMCPControllerClient constructs the controller-runtime client used by
// MCP interactive mode, with its own scheme (remediationv1 for RR existence
// validation — HARM-004 — and future NL signal intake, #714) and
// MCP-specific timeout/QPS/burst tuning (SEC-07), independent from the
// primary manager's client.
func buildMCPControllerClient(infra *k8sInfra) (ctrlclient.Client, error) {
	mcpScheme := k8sruntime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(mcpScheme))
	utilruntime.Must(remediationv1.AddToScheme(mcpScheme))

	mcpRestConfig := *infra.kubeConfig
	mcpRestConfig.Timeout = 10 * time.Second
	mcpRestConfig.QPS = 20
	mcpRestConfig.Burst = 40

	return ctrlclient.New(&mcpRestConfig, ctrlclient.Options{Scheme: mcpScheme})
}

// newDisconnectAuditEmitter returns a closure that emits interactive.completed
// audit events for non-tool session endings (disconnect, inactivity timeout,
// TTL expiry). M1: ensures all session-ending paths produce an audit trail,
// not just action=complete/cancel through InvestigateTool. Extracted as its
// own factory (rather than an inline closure) so the three later callbacks
// that reference it — lease-expiry, inactivity-timeout, and disconnect — can
// each receive it as an explicit parameter instead of relying on lexical
// capture across a helper-function boundary.
func newDisconnectAuditEmitter(auditStore audit.AuditStore, logger logr.Logger) func(sessionID, correlationID, reason string) {
	return func(sessionID, correlationID, reason string) {
		event := audit.NewEvent(audit.EventTypeInteractiveCompleted, correlationID,
			audit.WithSessionID(sessionID),
		)
		event.EventAction = audit.ActionInteractiveCompleted
		event.EventOutcome = audit.OutcomeSuccess
		event.Data["reason"] = reason
		audit.StoreBestEffort(context.Background(), auditStore, event, logger.WithName("mcp-audit"))
	}
}

// buildMCPLeaseManager constructs the K8s-Lease-backed session manager
// (single-driver guarantee) and reclaims any orphaned Leases left over from a
// previous process instance.
func buildMCPLeaseManager(ctrlCli ctrlclient.Client, namespace string, cfg *kaconfig.Config, autoMgr *session.Manager, agentMetrics *kametrics.Metrics, logger logr.Logger, emitDisconnectAudit func(string, string, string)) *mcpkg.LeaseSessionManager {
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
	return leaseMgr
}

// buildMCPTimeoutManager constructs the TimeoutManager that fires onExpire
// when a session goes inactive (SEC-04, HARM-03/04): it snapshots the
// correlation ID before releasing the lease, resolves the HTTP session so AA
// stops polling user_driving, and emits the disconnect audit trail.
func buildMCPTimeoutManager(cfg *kaconfig.Config, autoMgr *session.Manager, leaseMgr *mcpkg.LeaseSessionManager, agentMetrics *kametrics.Metrics, logger logr.Logger, emitDisconnectAudit func(string, string, string)) *mcpkg.TimeoutManager {
	return mcpkg.NewTimeoutManager(
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
}

// spawnReconstruction runs the background context-reconstruction +
// autonomous-investigation spawn (INT-06, BR-INTERACTIVE-008) after an
// interactive session ends. All values that only exist at callback-runtime
// (rrID, interactiveSessionID, signalMeta) are threaded in as explicit
// parameters rather than captured lexically, per the closure-capture map
// produced during the Wave 5 preflight spike. Intended to be invoked via
// `go spawnReconstruction(...)`; runs with its own bounded timeout,
// independent of the caller's context.
func spawnReconstruction(reconSpawner *mcpkg.ReconstructionSpawner, logger logr.Logger, rrID, interactiveSessionID string, signalMeta map[string]string) {
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
}

// mcpDisconnectHandlerDeps groups the dependencies needed to construct the
// MCP disconnect handler's onClose callback. Kept as a config struct (rather
// than individual parameters) per the Go Anti-Pattern Checklist's 8+-param
// rule.
type mcpDisconnectHandlerDeps struct {
	eventStore          *mcpkg.DelegatingEventStore
	timeoutMgr          *mcpkg.TimeoutManager
	leaseMgr            *mcpkg.LeaseSessionManager
	autoMgr             *session.Manager
	reconSpawner        *mcpkg.ReconstructionSpawner
	agentMetrics        *kametrics.Metrics
	logger              logr.Logger
	emitDisconnectAudit func(string, string, string)
	gracePeriod         time.Duration
}

// buildMCPDisconnectHandler constructs the GracefulSessionClosedHandler that
// processes MCP disconnect events with a configurable grace period before
// releasing the interactive lease (BR-INTERACTIVE-001): if a client
// reconnects during the grace period, the lease is preserved. The returned
// handler must be wired onward by the caller (SetReconnectCallback and
// `go disconnectHandler.Run(ctx)`), since both depend on the constructed
// value being available at call time.
func buildMCPDisconnectHandler(d mcpDisconnectHandlerDeps) *mcpkg.GracefulSessionClosedHandler {
	return mcpkg.NewGracefulSessionClosedHandler(d.eventStore, func(mcpSessionID string) {
		interactiveSessionID, ok := d.eventStore.LookupInteractiveSession(mcpSessionID)
		if !ok {
			d.logger.V(1).Info("MCP session closed without interactive mapping (autonomous or already released)",
				"mcp_session_id", mcpSessionID)
			return
		}

		d.eventStore.DeleteMCPSession(mcpSessionID)

		d.timeoutMgr.StopTracking(interactiveSessionID)

		// T1-1: Snapshot session info BEFORE Release deletes the entry.
		rrID, signalMeta := d.leaseMgr.GetSessionInfo(interactiveSessionID)

		// #1438: Emit terminal event BEFORE closing the HTTP session so the
		// EventLogBridge can forward it to AF before the channel closes.
		d.autoMgr.EmitSessionEndedByRR(rrID, "disconnect")

		if err := d.leaseMgr.Release(interactiveSessionID, "disconnect"); err != nil {
			d.logger.Info("failed to release disconnected session",
				"session_id", interactiveSessionID,
				"error", err.Error())
			return
		}

		// KA-CRIT-2: Resolve the HTTP session so AA stops polling user_driving.
		mcptools.CompleteHTTPSession(d.autoMgr, rrID, nil, d.logger, "disconnect")

		d.emitDisconnectAudit(interactiveSessionID, rrID, "disconnect")

		// T1-4: Decrement gauge on disconnect to prevent drift.
		d.agentMetrics.RecordInteractiveSessionEnded()

		// Spawn reconstruction in background (best-effort, BR-INTERACTIVE-008).
		go spawnReconstruction(d.reconSpawner, d.logger, rrID, interactiveSessionID, signalMeta)
	}, d.gracePeriod, d.logger)
}

// mcpToolsDeps groups the dependencies needed to construct the three MCP
// tools (investigate, select_workflow, complete_no_action). Kept as a config
// struct (rather than individual parameters) per the Go Anti-Pattern
// Checklist's 8+-param rule.
type mcpToolsDeps struct {
	leaseMgr           *mcpkg.LeaseSessionManager
	investigatorRunner mcptools.InvestigatorRunner
	recon              mcpkg.ContextReconstructor
	autoMgr            *session.Manager
	agentMetrics       *kametrics.Metrics
	sessionRateLimiter *mcpkg.SessionRateLimiter
	timeoutMgr         *mcpkg.TimeoutManager
	sessionNotifier    *mcpkg.SessionNotifier
	rrChecker          *mcptools.K8sRRExistenceChecker
	auditStore         audit.AuditStore
	logger             logr.Logger
	signalResolver     *mcpadapters.SessionSignalContextResolver
	catalogAdapter     *mcpadapters.WorkflowCatalogAdapter
	enricher           *enrichment.Enricher
}

// buildMCPTools constructs the InvestigateTool, SelectWorkflowTool, and
// CompleteNoActionTool with their shared dependencies (lease manager,
// catalog adapter) and per-tool options.
func buildMCPTools(d mcpToolsDeps) (*mcptools.InvestigateTool, *mcptools.SelectWorkflowTool, *mcptools.CompleteNoActionTool) {
	// Build the InvestigateTool with optional dependencies.
	investigateOpts := []mcptools.InvestigateOption{
		mcptools.WithToolMetrics(d.agentMetrics),
		mcptools.WithRateLimiter(d.sessionRateLimiter),
		mcptools.WithTimeoutTracker(d.timeoutMgr),
		mcptools.WithNotifyFunc(d.sessionNotifier.Notify),
		mcptools.WithRRExistenceChecker(d.rrChecker),
		mcptools.WithHTTPCompleter(d.autoMgr),
		mcptools.WithAuditStore(d.auditStore, d.logger.WithName("mcp-audit")),
		mcptools.WithSignalContextResolver(d.signalResolver),
		mcptools.WithWorkflowCatalog(d.catalogAdapter),
	}
	investigateTool := mcptools.NewInvestigateTool(d.leaseMgr, d.investigatorRunner, d.recon, d.autoMgr, investigateOpts...)

	// Build SelectWorkflowTool (reuses the same catalogAdapter).
	// #1012: enrichment is now internalized into select_workflow via WithEnrichmentRunner.
	swOpts := []mcptools.SelectWorkflowOption{
		mcptools.WithLogger(d.logger.WithName("select-workflow")),
		mcptools.WithHTTPSessionCompleter(d.autoMgr),
		mcptools.WithMutexProvider(investigateTool),
	}
	if d.enricher != nil {
		swOpts = append(swOpts, mcptools.WithEnrichmentRunner(d.enricher))
	}
	selectWfTool := mcptools.NewSelectWorkflowTool(d.catalogAdapter, d.leaseMgr, swOpts...)

	// Build the CompleteNoActionTool.
	completeNoActionTool := mcptools.NewCompleteNoActionTool(d.leaseMgr,
		mcptools.WithCompleteNoActionLogger(d.logger.WithName("complete-no-action")),
		mcptools.WithCompleteNoActionHTTPCompleter(d.autoMgr),
		mcptools.WithCompleteNoActionMutexProvider(investigateTool),
	)

	return investigateTool, selectWfTool, completeNoActionTool
}

// noopReconstructor is a no-op ContextReconstructor used when DS is unavailable.
type noopReconstructor struct{}

func (n *noopReconstructor) Reconstruct(_ context.Context, _ string, _ string) ([]mcpkg.ConversationTurn, error) {
	return nil, nil
}

// errDSUnavailable is returned by every noopWorkflowQuerier method. Bug fix
// (Wave 5 follow-up): buildMCPHandler previously dereferenced ds.ogenClient
// unconditionally when building the workflow-catalog querier, panicking the
// whole MCP interactive-mode handler when DS was nil, even though every
// other DS-optional dependency in this package (recon above, plus
// buildToolRegistry/readinessHandler) already fails open. A wrapped
// "unavailable" error surfaces through WorkflowCatalogAdapter.GetWorkflowByID
// and is returned by SelectWorkflowTool.Handle as "workflow catalog lookup
// failed: %w" — a normal tool error the caller can act on, not a crash.
var errDSUnavailable = fmt.Errorf("workflow catalog unavailable: DataStorage integration not configured")

// noopWorkflowQuerier is a no-op wfclient.WorkflowQuerier used when DS is
// unavailable, mirroring noopReconstructor's fail-open pattern.
type noopWorkflowQuerier struct{}

func (n *noopWorkflowQuerier) GetWorkflowDependencies(_ context.Context, _ string) (*dsmodels.WorkflowDependencies, error) {
	return nil, errDSUnavailable
}

func (n *noopWorkflowQuerier) GetWorkflowEngineConfig(_ context.Context, _ string) (json.RawMessage, error) {
	return nil, errDSUnavailable
}

func (n *noopWorkflowQuerier) GetWorkflowExecutionEngine(_ context.Context, _ string) (string, string, error) {
	return "", "", errDSUnavailable
}

func (n *noopWorkflowQuerier) GetWorkflowExecutionBundle(_ context.Context, _ string) (string, string, error) {
	return "", "", errDSUnavailable
}

func (n *noopWorkflowQuerier) ResolveWorkflowCatalogMetadata(_ context.Context, _ string) (*wfclient.WorkflowCatalogMetadata, error) {
	return nil, errDSUnavailable
}

func (n *noopWorkflowQuerier) GetWorkflowSchemaMetadata(_ context.Context, _ string) (*wfclient.SchemaMetadata, error) {
	return nil, errDSUnavailable
}

// Compile-time interface compliance check.
var _ wfclient.WorkflowQuerier = (*noopWorkflowQuerier)(nil)
