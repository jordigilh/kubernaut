package auth

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

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

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			r.Body = http.MaxBytesReader(w, r.Body, MaxBodySize)

			reqLogger := logger.WithValues(
				"component", "auth",
				"source_ip", httputil.ExtractClientIP(r),
				"request_id", requestid.FromContext(r.Context()),
			)
			ctx := logging.WithLogger(r.Context(), reqLogger)

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				reqLogger.V(1).Info("auth failed: missing authorization header")
				emitAuthFailure(ctx, cfg.Auditor, "", httputil.ExtractClientIP(r), "missing_header")
				httputil.WriteProblem(w, http.StatusUnauthorized,
					"Missing Authorization", "The Authorization header is required.")
				return
			}

			if err := security.ValidateHeaderValue(authHeader); err != nil {
				reqLogger.V(1).Info("auth failed: invalid authorization header", "error", err)
				emitAuthFailure(ctx, cfg.Auditor, "", httputil.ExtractClientIP(r), "control_chars")
				httputil.WriteProblem(w, http.StatusBadRequest,
					"Invalid Authorization Header", "The Authorization header contains invalid characters.")
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == authHeader {
				reqLogger.V(1).Info("auth failed: non-bearer scheme")
				emitAuthFailure(ctx, cfg.Auditor, "", httputil.ExtractClientIP(r), "non_bearer")
				httputil.WriteProblem(w, http.StatusUnauthorized,
					"Invalid Scheme", "The Authorization header must use the Bearer scheme.")
				return
			}

			identity, err := cfg.Validator.Validate(r.Context(), token)
			if err != nil {
				reqLogger.V(1).Info("auth failed: token validation", "error", err)
				observeAuthDuration(cfg.AuthDuration, start, "failure")
				emitAuthFailure(ctx, cfg.Auditor, "", httputil.ExtractClientIP(r), classifyAuthError(err))
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

			// Derive deadline from token expiry so streaming handlers terminate
			// before the token becomes invalid. Jitter prevents timing oracle.
			if !identity.ExpiresAt.IsZero() {
				jitter := time.Duration(25+cryptoRandIntn(10)) * time.Second
				deadline := identity.ExpiresAt.Add(-jitter)
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, deadline)
				defer cancel()
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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
			"issuer": identity.Issuer,
		},
	})
}

func emitAuthFailure(ctx context.Context, emitter audit.Emitter, userID, sourceIP, reason string) {
	if emitter == nil {
		return
	}
	emitter.Emit(ctx, &audit.Event{
		Type:     audit.EventAuthFailure,
		UserID:   userID,
		SourceIP: sourceIP,
		Detail: map[string]string{
			"reason": reason,
		},
	})
}

func classifyAuthError(err error) string {
	switch {
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
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	v := int(binary.LittleEndian.Uint64(buf[:]))
	if v < 0 {
		v = -v
	}
	return v % n
}
