package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/httputil"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/logging"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/requestid"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
)

// MaxBodySize is the maximum allowed request body size (1MB).
const MaxBodySize = 1 << 20

// MiddlewareConfig holds dependencies for the auth middleware.
type MiddlewareConfig struct {
	Validator    *JWTValidator
	Logger       logr.Logger
	Auditor      audit.Emitter
	AuthDuration *prometheus.HistogramVec
}

// MiddlewareWithConfig returns auth middleware with full observability support.
// Performs L1 body size enforcement, authorization header sanitization,
// JWT validation, structured logging (OPS-3), audit event emission (SEC-2),
// and UserIdentity context propagation.
func MiddlewareWithConfig(cfg MiddlewareConfig) func(http.Handler) http.Handler {
	logger := cfg.Logger
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	logger = logger.WithName("auth")

	authMethod := AuthMethodJWT
	if cfg.Validator != nil {
		authMethod = cfg.Validator.AuthMethod()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			stripImpersonationHeaders(r)
			start := time.Now()
			r.Body = http.MaxBytesReader(w, r.Body, MaxBodySize)

			reqLogger := logger.WithValues(
				"component", "auth",
				"source_ip", httputil.ExtractClientIP(r),
				"request_id", requestid.FromContext(r.Context()),
			)
			ctx := logging.WithLogger(r.Context(), reqLogger)

			token, ok := extractBearerToken(w, r, ctx, cfg.Auditor, reqLogger, authMethod)
			if !ok {
				return
			}

			identity, err := cfg.Validator.Validate(r.Context(), token)
			if err != nil {
				reqLogger.V(1).Info("auth failed: token validation", "error", err)
				observeAuthDuration(cfg.AuthDuration, start, "failure")
				emitAuthFailure(ctx, cfg.Auditor, "", httputil.ExtractClientIP(r), classifyAuthError(err), authMethod)
				httputil.WriteProblem(w, http.StatusUnauthorized,
					"Authentication Failed", "The provided token could not be validated.")
				return
			}

			observeAuthDuration(cfg.AuthDuration, start, "success")
			reqLogger.V(1).Info("auth success",
				"user_id", identity.Username,
				"issuer", identity.Issuer,
				"duration", time.Since(start).String(),
			)
			emitAuthSuccess(ctx, cfg.Auditor, identity, httputil.ExtractClientIP(r))

			ctx = WithUserIdentity(ctx, identity)
			ctx = logging.WithUserID(ctx, identity.Username)

			ctx, cancel := applyTokenExpiryDeadline(ctx, identity)
			if cancel != nil {
				defer cancel()
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractBearerToken validates the Authorization header is present,
// control-character-free, and uses the Bearer scheme, writing the
// appropriate problem response (and emitting an auth-failure audit event) and
// returning ok=false on the first violation.
func extractBearerToken(w http.ResponseWriter, r *http.Request, ctx context.Context, auditor audit.Emitter, reqLogger logr.Logger, authMethod string) (string, bool) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		reqLogger.V(1).Info("auth failed: missing authorization header")
		emitAuthFailure(ctx, auditor, "", httputil.ExtractClientIP(r), "missing_header", authMethod)
		httputil.WriteProblem(w, http.StatusUnauthorized,
			"Missing Authorization", "The Authorization header is required.")
		return "", false
	}

	if err := security.ValidateHeaderValue(authHeader); err != nil {
		reqLogger.V(1).Info("auth failed: invalid authorization header", "error", err)
		emitAuthFailure(ctx, auditor, "", httputil.ExtractClientIP(r), "control_chars", authMethod)
		httputil.WriteProblem(w, http.StatusBadRequest,
			"Invalid Authorization Header", "The Authorization header contains invalid characters.")
		return "", false
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == authHeader {
		reqLogger.V(1).Info("auth failed: non-bearer scheme")
		emitAuthFailure(ctx, auditor, "", httputil.ExtractClientIP(r), "non_bearer", authMethod)
		httputil.WriteProblem(w, http.StatusUnauthorized,
			"Invalid Scheme", "The Authorization header must use the Bearer scheme.")
		return "", false
	}
	return token, true
}

// applyTokenExpiryDeadline derives a context deadline from the token's expiry
// so streaming handlers terminate before the token becomes invalid. Jitter
// prevents a timing oracle on token expiry. Returns a nil cancel func when
// identity has no expiry (deadline not applicable).
func applyTokenExpiryDeadline(ctx context.Context, identity *UserIdentity) (context.Context, context.CancelFunc) {
	if identity.ExpiresAt.IsZero() {
		return ctx, nil
	}
	jitter := time.Duration(25+cryptoRandIntn(10)) * time.Second
	deadline := identity.ExpiresAt.Add(-jitter)
	return context.WithDeadline(ctx, deadline)
}

func authMethodFromIdentity(identity *UserIdentity) string {
	if identity.Issuer != "" {
		return AuthMethodJWT
	}
	return AuthMethodTokenReview
}

func emitAuthSuccess(ctx context.Context, emitter audit.Emitter, identity *UserIdentity, sourceIP string) {
	if emitter == nil {
		return
	}
	emitter.Emit(ctx, &audit.Event{
		Type:     audit.EventAuthSuccess,
		UserID:   identity.Username,
		SourceIP: sourceIP,
		Detail: map[string]string{
			"auth_method": authMethodFromIdentity(identity),
			"issuer":      identity.Issuer,
		},
	})
}

func emitAuthFailure(ctx context.Context, emitter audit.Emitter, userID, sourceIP, reason, authMethod string) {
	if emitter == nil {
		return
	}
	emitter.Emit(ctx, &audit.Event{
		Type:     audit.EventAuthFailure,
		UserID:   userID,
		SourceIP: sourceIP,
		Detail: map[string]string{
			"auth_method":    authMethod,
			"failure_reason": reason,
		},
	})
}

func classifyAuthError(err error) string {
	switch {
	case errors.Is(err, ErrTokenReplayed):
		return "token_replayed"
	case errors.Is(err, ErrTokenExpired):
		return "token_expired"
	case errors.Is(err, ErrNotYetValid):
		return "not_yet_valid"
	case errors.Is(err, ErrInvalidAudience):
		return "invalid_audience"
	case errors.Is(err, ErrUnknownIssuer):
		return "unknown_issuer"
	case errors.Is(err, ErrMalformedToken):
		return "malformed_token"
	case errors.Is(err, ErrCircuitOpen):
		return "circuit_open"
	case errors.Is(err, ErrCELValidation):
		return "cel_rule_failed"
	case errors.Is(err, ErrMissingExpiry):
		return "missing_expiry"
	default:
		return "validation_failed"
	}
}

func observeAuthDuration(hist *prometheus.HistogramVec, start time.Time, result string) {
	if hist != nil {
		hist.WithLabelValues(result).Observe(time.Since(start).Seconds())
	}
}

// cryptoRandIntn returns a cryptographically random int in [0, n).
func cryptoRandIntn(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

// stripImpersonationHeaders removes K8s impersonation headers from inbound
// requests (SEC-12 / ADR-022). AF no longer performs impersonation; stale
// headers from a proxy or malicious client must be dropped before processing.
func stripImpersonationHeaders(r *http.Request) {
	r.Header.Del("Impersonate-User")
	r.Header.Del("Impersonate-Group")
	r.Header.Del("Impersonate-Uid")
	for key := range r.Header {
		if strings.HasPrefix(strings.ToLower(key), "impersonate-extra-") {
			r.Header.Del(key)
		}
	}
}
