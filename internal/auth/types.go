package auth

import "fmt"

// UserIdentity represents an authenticated user's identity extracted from a JWT.
type UserIdentity struct {
	Username string   `json:"username"`
	Groups   []string `json:"groups,omitempty"`
	Issuer   string   `json:"issuer"`
	RawToken string   `json:"-"`
}

// String returns a safe representation that redacts the raw token (SEC-3).
func (u *UserIdentity) String() string {
	if u == nil {
		return "<nil>"
	}
	return fmt.Sprintf("UserIdentity{Username:%q, Groups:%v, Issuer:%q, RawToken:REDACTED}", u.Username, u.Groups, u.Issuer)
}

// contextKey is an unexported type for context keys in this package.
type contextKey struct{}

// userIdentityKey is the context key for UserIdentity.
var userIdentityKey = contextKey{}
