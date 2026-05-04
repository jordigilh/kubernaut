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
