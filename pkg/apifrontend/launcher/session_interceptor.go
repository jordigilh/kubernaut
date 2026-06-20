package launcher

import (
	"context"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// SessionInterceptor implements a2asrv.CallInterceptor to provide multi-turn
// conversation continuity (BR-SESS-020) and stable user identity (BR-SESS-021).
//
// Before each A2A request it:
//  1. Sets callCtx.User to the authenticated username (AC-2 compliance).
//  2. Overrides msg.ContextID to the active investigation's context if one
//     exists for the user, routing the message into the same ADK session.
type SessionInterceptor struct {
	a2asrv.PassthroughCallInterceptor
	registry *ActiveContextRegistry
	logger   logr.Logger
}

// NewSessionInterceptor creates an interceptor backed by the given registry.
func NewSessionInterceptor(registry *ActiveContextRegistry, logger logr.Logger) *SessionInterceptor {
	return &SessionInterceptor{
		registry: registry,
		logger:   logger.WithName("session-interceptor"),
	}
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

	if msg.ContextID != "" {
		return ctx, nil
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
