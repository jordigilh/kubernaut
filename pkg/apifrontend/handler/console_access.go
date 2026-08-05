package handler

import (
	"context"
	"net/http"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/httputil"
)

// consoleAccessHandler serves GET /a2a/access (#1919): a lightweight,
// pre-flight check the console/chat client can call once at load time to
// decide whether to render its UI at all, backed by the same
// kubernaut.ai/console SAR grant that checkRBAC and newRBACGuard also
// enforce directly at every tool-call site (this endpoint is advisory only
// -- it is not itself the security boundary, see #1919 design notes).
type consoleAccessHandler struct {
	checker auth.ConsoleAuthorizer
	auditor audit.Emitter
	logger  logr.Logger
}

// NewConsoleAccessHandler constructs the GET /a2a/access handler. auditor
// may be nil (no audit emission); logger may be the zero value (discarded).
func NewConsoleAccessHandler(checker auth.ConsoleAuthorizer, auditor audit.Emitter, logger logr.Logger) http.Handler {
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	return &consoleAccessHandler{
		checker: checker,
		auditor: auditor,
		logger:  logger.WithName("console-access"),
	}
}

func (h *consoleAccessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := auth.UserIdentityFromContext(ctx)
	if user == nil {
		httputil.WriteProblem(w, http.StatusUnauthorized, "Unauthorized", "authentication required")
		return
	}

	allowed, err := h.checker.CheckConsoleAccess(ctx, user.Username, user.Groups)
	if err != nil {
		h.logger.Error(err, "console access check failed", "user", user.Username)
		h.deny(ctx, w, user, "console_authorizer_error")
		return
	}
	if !allowed {
		h.deny(ctx, w, user, "console_access_denied")
		return
	}

	w.WriteHeader(http.StatusOK)
}

// deny emits an EventAuthAccessDenied audit event (AU-12) and writes the
// RFC 7807 403 response, mirroring checkRBAC's denyRBAC (mcp_bridge.go).
func (h *consoleAccessHandler) deny(ctx context.Context, w http.ResponseWriter, user *auth.UserIdentity, reason string) {
	if h.auditor != nil {
		h.auditor.Emit(ctx, &audit.Event{
			Type:   audit.EventAuthAccessDenied,
			UserID: user.Username,
			Detail: map[string]string{
				"endpoint": "console",
				"reason":   reason,
			},
		})
	}
	httputil.WriteProblem(w, http.StatusForbidden, "Forbidden", "not authorized to access the console")
}
