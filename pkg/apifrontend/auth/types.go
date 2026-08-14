package auth

import (
	"fmt"
	"time"
)

// UserIdentity represents an authenticated user's identity extracted from a JWT.
type UserIdentity struct {
	Username         string    `json:"username"`
	Groups           []string  `json:"groups,omitempty"`
	Issuer           string    `json:"issuer"`
	RawToken         string    `json:"-"`
	ExpiresAt        time.Time `json:"expiresAt,omitempty"`
	IsServiceAccount bool      `json:"isServiceAccount,omitempty"` // BR-INTERACTIVE-010: true for SA tokens (TokenReview path)
}

// String returns a safe representation that redacts the raw token (SEC-3).
func (u *UserIdentity) String() string {
	if u == nil {
		return "<nil>"
	}
	return fmt.Sprintf("UserIdentity{Username:%q, Groups:%v, Issuer:%q, IsServiceAccount:%t, RawToken:REDACTED}",
		u.Username, u.Groups, u.Issuer, u.IsServiceAccount)
}

// contextKey is an unexported type for context keys in this package. Backed
// by string (not struct{}) so distinct keys are actually distinct: every
// struct{} value compares equal to every other, which would silently
// collide two different context keys of that type.
type contextKey string

// userIdentityKey is the context key for UserIdentity.
var userIdentityKey = contextKey("userIdentity")

// sourceIPKey is the context key for the replay-cache source-binding key
// (#1999, BR-SECURITY-1505).
var sourceIPKey = contextKey("sourceIP")
