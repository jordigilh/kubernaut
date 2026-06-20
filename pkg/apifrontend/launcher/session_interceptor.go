package launcher

import (
	"context"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/go-logr/logr"
	adksession "google.golang.org/adk/session"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// StaleSessionValidator determines whether a context_id references a valid
// in-memory session. Used by SessionInterceptor to detect stale contexts
// after pod restarts (issue #1472, BR-SESS-025).
type StaleSessionValidator interface {
	// IsContextValid returns true if the context_id has a backing session
	// in memory OR if an error prevents determination (fail-open, SC-5).
	IsContextValid(ctx context.Context, contextID, username string) bool
}

// SessionInterceptor implements a2asrv.CallInterceptor to provide multi-turn
// conversation continuity (BR-SESS-020) and stable user identity (BR-SESS-021).
//
// Before each A2A request it:
//  1. Sets callCtx.User to the authenticated username (AC-2 compliance).
//  2. Validates explicit context_ids against the session store (#1472, BR-SESS-025).
//  3. Overrides msg.ContextID to the active investigation's context if one
//     exists for the user, routing the message into the same ADK session.
type SessionInterceptor struct {
	a2asrv.PassthroughCallInterceptor
	registry  *ActiveContextRegistry
	validator StaleSessionValidator
	logger    logr.Logger
}

// SessionInterceptorOption configures optional behavior for SessionInterceptor.
type SessionInterceptorOption func(*SessionInterceptor)

// WithStaleSessionValidator attaches a validator that checks whether an
// explicit context_id still has a backing in-memory session (#1472).
func WithStaleSessionValidator(v StaleSessionValidator) SessionInterceptorOption {
	return func(s *SessionInterceptor) {
		s.validator = v
	}
}

// NewSessionInterceptor creates an interceptor backed by the given registry.
func NewSessionInterceptor(registry *ActiveContextRegistry, logger logr.Logger, opts ...SessionInterceptorOption) *SessionInterceptor {
	s := &SessionInterceptor{
		registry: registry,
		logger:   logger.WithName("session-interceptor"),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Before sets callCtx.User from the auth context and conditionally overrides
// the message ContextID when an active investigation exists for the user.
func (s *SessionInterceptor) Before(ctx context.Context, callCtx *a2asrv.CallContext, req *a2asrv.Request) (context.Context, error) {
	identity := auth.UserIdentityFromContext(ctx)
	if identity == nil || identity.Username == "" {
		return ctx, nil
	}

	callCtx.User = &a2asrv.AuthenticatedUser{UserName: identity.Username}

	params, ok := req.Payload.(*a2a.MessageSendParams)
	if !ok || params == nil || params.Message == nil {
		return ctx, nil
	}

	msg := params.Message

	// BR-SESS-025 (#1472): When an explicit context_id is provided, validate
	// that it still has a backing session in memory. After a pod restart, the
	// in-memory session store is empty (hydration deferred to #1451), making
	// any carried-over context_id stale. Clearing it forces ADK to generate a
	// fresh session ID, preventing the misleading "reconnecting" UX.
	if msg.ContextID != "" {
		if s.validator != nil && !s.validator.IsContextValid(ctx, msg.ContextID, identity.Username) {
			s.logger.Info("clearing stale context_id — no backing session in memory (post-restart, #1472)",
				"user", identity.Username,
				"stale_context_id", msg.ContextID,
			)
			msg.ContextID = ""
			// Fall through to registry check below (which will also find
			// nothing post-restart, resulting in a fresh conversation).
		} else {
			return ctx, nil
		}
	}

	hadEntry := s.registry.HasEntry(identity.Username)
	activeCtx, found := s.registry.Get(identity.Username)
	if !found {
		if hadEntry {
			s.logger.Info("clearing stale context — idle-expired session will not redirect (#1446)",
				"user", identity.Username,
			)
		}
		return ctx, nil
	}

	s.logger.Info("overriding context_id for session continuity",
		"user", identity.Username,
		"original_context_id", msg.ContextID,
		"target_context_id", activeCtx,
	)
	msg.ContextID = activeCtx

	return ctx, nil
}

// inMemorySessionValidator implements StaleSessionValidator by probing the
// ADK session service's in-memory store. If the session exists, the context
// is valid. If not found, it's stale (post-restart). On unexpected errors,
// returns true (fail-open, SC-5).
type inMemorySessionValidator struct {
	sessionService adksession.Service
	appName        string
	logger         logr.Logger
}

// NewInMemorySessionValidator creates a validator that checks context validity
// by probing the ADK session service. The appName is required by the session
// service's Get() method.
func NewInMemorySessionValidator(sessionService adksession.Service, appName string, logger logr.Logger) StaleSessionValidator {
	return &inMemorySessionValidator{
		sessionService: sessionService,
		appName:        appName,
		logger:         logger.WithName("stale-session-validator"),
	}
}

// IsContextValid probes the in-memory session store. Returns false only when
// the session is definitively not found. For any other error (unexpected
// failure), returns true to preserve availability (fail-open, SC-5).
func (v *inMemorySessionValidator) IsContextValid(ctx context.Context, contextID, username string) bool {
	_, err := v.sessionService.Get(ctx, &adksession.GetRequest{
		AppName:   v.appName,
		UserID:    username,
		SessionID: contextID,
	})
	if err == nil {
		return true
	}
	if strings.Contains(err.Error(), "not found") {
		return false
	}
	v.logger.Info("session lookup returned unexpected error — fail-open (SC-5)",
		"context_id", contextID,
		"user", username,
		"error", err.Error(),
	)
	return true
}
