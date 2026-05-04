package auth

// UserIdentity represents an authenticated user's identity extracted from a JWT.
type UserIdentity struct {
	Username string
	Groups   []string
	Issuer   string
	RawToken string
}

// contextKey is an unexported type for context keys in this package.
type contextKey struct{}

// userIdentityKey is the context key for UserIdentity.
var userIdentityKey = contextKey{}
