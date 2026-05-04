package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/httputil"
	"github.com/jordigilh/kubernaut-apifrontend/internal/logging"
	"github.com/jordigilh/kubernaut-apifrontend/internal/requestid"
	"github.com/jordigilh/kubernaut-apifrontend/internal/security"
)

// MaxBodySize is the maximum allowed request body size (1MB).
const MaxBodySize = 1 << 20

// MiddlewareConfig holds dependencies for the auth middleware.
type MiddlewareConfig struct {
	Validator *JWTValidator
	Logger    logr.Logger
	Auditor   audit.Emitter
}

// Middleware returns an HTTP middleware that performs:
//   - L1 body size enforcement via http.MaxBytesReader
//   - L1 authorization header sanitization (no control characters)
//   - JWT validation via the provided JWTValidator
//   - Structured logging of auth decisions (OPS-3)
//   - Audit event emission (SEC-2)
//   - UserIdentity context propagation for downstream handlers
func Middleware(validator *JWTValidator) func(http.Handler) http.Handler {
	return MiddlewareWithConfig(MiddlewareConfig{Validator: validator})
}

// MiddlewareWithConfig returns auth middleware with full observability support.
func MiddlewareWithConfig(cfg MiddlewareConfig) func(http.Handler) http.Handler {
	logger := cfg.Logger
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	logger = logger.WithName("auth")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			r.Body = http.MaxBytesReader(w, r.Body, MaxBodySize)

			reqLogger := logger.WithValues(
				"component", "auth",
				"source_ip", extractClientIP(r),
				"request_id", requestid.FromContext(r.Context()),
			)
			ctx := logging.WithLogger(r.Context(), reqLogger)

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				reqLogger.V(1).Info("auth failed: missing authorization header")
				emitAuthFailure(ctx, cfg.Auditor, "", extractClientIP(r), "missing_header")
				httputil.WriteProblem(w, http.StatusUnauthorized,
					"Missing Authorization", "The Authorization header is required.")
				return
			}

			if err := security.ValidateHeaderValue(authHeader); err != nil {
				reqLogger.V(1).Info("auth failed: invalid authorization header", "error", err)
				emitAuthFailure(ctx, cfg.Auditor, "", extractClientIP(r), "control_chars")
				httputil.WriteProblem(w, http.StatusBadRequest,
					"Invalid Authorization Header", "The Authorization header contains invalid characters.")
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == authHeader {
				reqLogger.V(1).Info("auth failed: non-bearer scheme")
				emitAuthFailure(ctx, cfg.Auditor, "", extractClientIP(r), "non_bearer")
				httputil.WriteProblem(w, http.StatusUnauthorized,
					"Invalid Scheme", "The Authorization header must use the Bearer scheme.")
				return
			}

			identity, err := cfg.Validator.Validate(r.Context(), token)
			if err != nil {
				reqLogger.V(1).Info("auth failed: token validation", "error", err)
				emitAuthFailure(ctx, cfg.Auditor, "", extractClientIP(r), err.Error())
				httputil.WriteProblem(w, http.StatusUnauthorized,
					"Authentication Failed", "The provided token could not be validated.")
				return
			}

			reqLogger.V(1).Info("auth success",
				"user_id", identity.Username,
				"issuer", identity.Issuer,
				"duration", time.Since(start).String(),
			)
			emitAuthSuccess(ctx, cfg.Auditor, identity, extractClientIP(r))

			ctx = WithUserIdentity(ctx, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return xff
	}
	return r.RemoteAddr
}

func emitAuthSuccess(ctx context.Context, emitter audit.Emitter, identity *UserIdentity, sourceIP string) {
	if emitter == nil {
		return
	}
	emitter.Emit(ctx, audit.Event{
		Type:     audit.EventAuthSuccess,
		UserID:   identity.Username,
		SourceIP: sourceIP,
		Detail: map[string]string{
			"issuer": identity.Issuer,
		},
	})
}

func emitAuthFailure(ctx context.Context, emitter audit.Emitter, userID, sourceIP, reason string) {
	if emitter == nil {
		return
	}
	emitter.Emit(ctx, audit.Event{
		Type:     audit.EventAuthFailure,
		UserID:   userID,
		SourceIP: sourceIP,
		Detail: map[string]string{
			"reason": reason,
		},
	})
}
