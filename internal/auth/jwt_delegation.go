package auth

import "net/http"

// ContextJWTDelegationTransport wraps an http.RoundTripper to inject the
// authenticated user's JWT from request context into outbound requests.
// Used for Pattern B JWT delegation (DD-AUTH-MCP-001 v2.0): the API Frontend
// forwards the user's Keycloak JWT to KA's MCP endpoint, which validates it
// via JWKS.
type ContextJWTDelegationTransport struct {
	Base http.RoundTripper
}

// RoundTrip extracts the raw JWT token from the request context (stored by
// auth middleware) and sets it as the Authorization header. If no token is
// found in context, the request is forwarded without modification.
func (t *ContextJWTDelegationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	identity := UserIdentityFromContext(req.Context())
	if identity != nil && identity.RawToken != "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+identity.RawToken)
	}

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
