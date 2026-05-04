package auth

import (
	"net/http"
	"strings"

	"github.com/jordigilh/kubernaut-apifrontend/internal/security"
)

// MaxBodySize is the maximum allowed request body size (1MB).
const MaxBodySize = 1 << 20

// Middleware returns an HTTP middleware that performs:
//   - L1 body size enforcement via http.MaxBytesReader
//   - L1 authorization header sanitization (no control characters)
//   - JWT validation via the provided JWTValidator
//   - UserIdentity context propagation for downstream handlers
func Middleware(validator *JWTValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, MaxBodySize)

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			if err := security.ValidateHeaderValue(authHeader); err != nil {
				http.Error(w, "invalid authorization header", http.StatusBadRequest)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == authHeader {
				http.Error(w, "authorization header must use Bearer scheme", http.StatusUnauthorized)
				return
			}

			identity, err := validator.Validate(r.Context(), token)
			if err != nil {
				http.Error(w, "authentication failed", http.StatusUnauthorized)
				return
			}

			ctx := WithUserIdentity(r.Context(), identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
