package auth

import "context"

// WithUserIdentity returns a new context with the given UserIdentity attached.
func WithUserIdentity(ctx context.Context, id *UserIdentity) context.Context {
	return context.WithValue(ctx, userIdentityKey, id)
}

// UserIdentityFromContext extracts the UserIdentity from the context.
// Returns nil if not present.
func UserIdentityFromContext(ctx context.Context) *UserIdentity {
	id, _ := ctx.Value(userIdentityKey).(*UserIdentity)
	return id
}

// WithSourceIP returns a new context carrying the resolved client source key
// used for jti replay-cache source binding (#1999, BR-SECURITY-1505). The
// caller (normally MiddlewareWithConfig, via a trustedSourceResolver) is
// responsible for resolving ip in a spoof-resistant way before attaching it.
func WithSourceIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, sourceIPKey, ip)
}

// SourceIPFromContext extracts the replay-cache source key attached by
// WithSourceIP. Returns "" if not present, which Validate treats as "no
// source information available" (every such caller is treated as the same
// source — never a false-positive replay, only a potential false negative).
func SourceIPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(sourceIPKey).(string)
	return ip
}
