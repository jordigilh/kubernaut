package auth

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
)

// ErrTokenExpiredDelegation is returned when outbound delegation is attempted with an expired token.
var ErrTokenExpiredDelegation = fmt.Errorf("token expired: refusing outbound delegation")

// ContextJWTDelegationTransport wraps an http.RoundTripper to inject the
// authenticated user's JWT from request context into outbound requests.
// Used for Pattern B JWT delegation (DD-AUTH-MCP-001 v2.0): the API Frontend
// forwards the user's Keycloak JWT to KA's MCP endpoint, which validates it
// via JWKS.
type ContextJWTDelegationTransport struct {
	Base http.RoundTripper
}

// RoundTrip extracts the raw JWT token from the request context (stored by
// auth middleware) and sets it as the Authorization header. If the token has
// expired, the request is rejected (fail-closed) to prevent forwarding
// invalid credentials.
func (t *ContextJWTDelegationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	identity := UserIdentityFromContext(req.Context())
	if identity != nil {
		if !identity.ExpiresAt.IsZero() && time.Now().After(identity.ExpiresAt) {
			return nil, ErrTokenExpiredDelegation
		}
		if identity.RawToken != "" {
			req = req.Clone(req.Context())
			req.Header.Set("Authorization", "Bearer "+identity.RawToken)
		}
	}

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// AuditingJWTDelegationTransport wraps ContextJWTDelegationTransport to emit
// a jwt.delegation audit event on each outbound delegated request.
type AuditingJWTDelegationTransport struct {
	Base    http.RoundTripper
	Auditor audit.Emitter
}

// RoundTrip delegates to Base and emits a jwt.delegation audit event.
func (t *AuditingJWTDelegationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Auditor != nil {
		username := ""
		if identity := UserIdentityFromContext(req.Context()); identity != nil {
			username = identity.Username
		}
		t.Auditor.Emit(req.Context(), &audit.Event{
			Type:   audit.EventJWTDelegation,
			UserID: username,
			Detail: map[string]string{
				"target_host": req.URL.Scheme + "://" + req.URL.Host,
				"method":      req.Method,
			},
		})
	}

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
