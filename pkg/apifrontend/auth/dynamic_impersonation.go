package auth

import (
	"context"
	"fmt"

	"k8s.io/client-go/dynamic"
)

// DynamicClientFactory creates a dynamic.Interface appropriate for the calling
// context. ADR-022: all factories now return AF's ServiceAccount-scoped client;
// the type is retained for interface compatibility with internal tools.
type DynamicClientFactory func(ctx context.Context) (dynamic.Interface, error)

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
