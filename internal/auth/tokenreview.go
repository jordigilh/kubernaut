package auth

import (
	"context"
	"fmt"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// TokenReviewer validates ServiceAccount tokens via the K8s TokenReview API.
type TokenReviewer struct {
	client kubernetes.Interface
}

// NewTokenReviewer creates a new TokenReviewer using the given K8s client.
func NewTokenReviewer(client kubernetes.Interface) *TokenReviewer {
	return &TokenReviewer{client: client}
}

// Validate validates a token via the K8s TokenReview API and returns the identity.
func (r *TokenReviewer) Validate(ctx context.Context, token string) (*UserIdentity, error) {
	if r.client == nil {
		return nil, fmt.Errorf("token review not configured")
	}

	review := &authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token: token,
		},
	}

	result, err := r.client.AuthenticationV1().TokenReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("token review request failed: %w", err)
	}

	if !result.Status.Authenticated {
		return nil, fmt.Errorf("%w: token review: not authenticated", ErrMalformedToken)
	}

	return &UserIdentity{
		Username: result.Status.User.Username,
		Groups:   result.Status.User.Groups,
		RawToken: token,
	}, nil
}
