package auth

import (
	"context"
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// DynamicClientFactory creates a dynamic.Interface appropriate for the calling
// user's identity. Read-only triage tools use this to enforce least-privilege:
// the user only sees what their RBAC permits, not the AF ServiceAccount's broader view.
type DynamicClientFactory func(ctx context.Context) (dynamic.Interface, error)

// NewImpersonatingDynamicFactory returns a DynamicClientFactory that creates an
// impersonated dynamic.Interface based on the UserIdentity in the context.
// If no identity is present, it returns an error (fail-closed).
// baseCfg is the AF ServiceAccount's rest.Config (never mutated).
func NewImpersonatingDynamicFactory(baseCfg *rest.Config) DynamicClientFactory {
	return func(ctx context.Context) (dynamic.Interface, error) {
		identity := UserIdentityFromContext(ctx)
		if identity == nil || identity.Username == "" {
			return nil, fmt.Errorf("impersonation requires authenticated user identity in context")
		}

		impCfg, err := NewImpersonatedConfig(baseCfg, identity.Username, identity.Groups)
		if err != nil {
			return nil, fmt.Errorf("creating impersonated config: %w", err)
		}

		client, err := dynamic.NewForConfig(impCfg)
		if err != nil {
			return nil, fmt.Errorf("creating impersonated dynamic client: %w", err)
		}

		return client, nil
	}
}

// StaticDynamicFactory returns a DynamicClientFactory that always returns the
// same client. Used for AF ServiceAccount-scoped tools and testing.
func StaticDynamicFactory(client dynamic.Interface) DynamicClientFactory {
	return func(_ context.Context) (dynamic.Interface, error) {
		if client == nil {
			return nil, fmt.Errorf("kubernetes cluster is not available — contact your administrator")
		}
		return client, nil
	}
}
