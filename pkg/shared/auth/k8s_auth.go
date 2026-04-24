package auth

import (
	"context"
	"errors"
	"fmt"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// K8sAuthenticator implements Authenticator using Kubernetes TokenReview API.
//
// This implementation validates ServiceAccount tokens by making TokenReview API calls
// to the Kubernetes API server. It is suitable for production and E2E environments.
//
// Authority: DD-AUTH-014
type K8sAuthenticator struct {
	client kubernetes.Interface
}

// NewK8sAuthenticator creates a new Kubernetes-based authenticator.
//
// Example:
//
//	config, _ := rest.InClusterConfig()
//	clientset, _ := kubernetes.NewForConfig(config)
//	authenticator := auth.NewK8sAuthenticator(clientset)
func NewK8sAuthenticator(client kubernetes.Interface) *K8sAuthenticator {
	return &K8sAuthenticator{
		client: client,
	}
}

// ValidateToken validates a ServiceAccount token using Kubernetes TokenReview API.
//
// This method:
// 1. Creates a TokenReview request with the provided token
// 2. Sends the request to the Kubernetes API server
// 3. Returns the authenticated user identity if valid
//
// Authority: https://kubernetes.io/docs/reference/kubernetes-api/authentication-resources/token-review-v1/
func (a *K8sAuthenticator) ValidateToken(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", errors.New("token cannot be empty")
	}

	// Create TokenReview request
	review := &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{
			Token: token,
		},
	}

	// Call Kubernetes TokenReview API
	result, err := a.client.AuthenticationV1().TokenReviews().Create(
		ctx,
		review,
		metav1.CreateOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("token validation failed: %w", err)
	}

	if !result.Status.Authenticated {
		return "", fmt.Errorf("%w: token rejected by API server", ErrTokenInvalid)
	}

	// Check if user info is present
	if result.Status.User.Username == "" {
		return "", errors.New("token authenticated but user identity is empty")
	}

	return result.Status.User.Username, nil
}

// K8sAuthorizer implements Authorizer using Kubernetes SubjectAccessReview (SAR) API.
//
// This implementation checks RBAC permissions by making SAR API calls to the
// Kubernetes API server. It is suitable for production and E2E environments.
//
// Authority: DD-AUTH-014
type K8sAuthorizer struct {
	client kubernetes.Interface
}

// NewK8sAuthorizer creates a new Kubernetes-based authorizer.
//
// Example:
//
//	config, _ := rest.InClusterConfig()
//	clientset, _ := kubernetes.NewForConfig(config)
//	authorizer := auth.NewK8sAuthorizer(clientset)
func NewK8sAuthorizer(client kubernetes.Interface) *K8sAuthorizer {
	return &K8sAuthorizer{
		client: client,
	}
}

// CheckAccess checks if a user has permission using Kubernetes SubjectAccessReview API.
// Delegates to CheckAccessWithGroup with an empty API group (core resources).
//
// Authority: https://kubernetes.io/docs/reference/kubernetes-api/authorization-resources/subject-access-review-v1/
func (a *K8sAuthorizer) CheckAccess(ctx context.Context, user, namespace, resource, resourceName, verb string) (bool, error) {
	return a.CheckAccessWithGroup(ctx, user, namespace, "", resource, resourceName, verb)
}

// CheckAccessWithGroup checks if a user has permission on a resource in a specific API group.
//
// Parameters:
//   - apiGroup: Kubernetes API group (e.g., "kubernaut.ai" for CRDs, "" for core resources)
//
// Authority: https://kubernetes.io/docs/reference/kubernetes-api/authorization-resources/subject-access-review-v1/
func (a *K8sAuthorizer) CheckAccessWithGroup(ctx context.Context, user, namespace, apiGroup, resource, resourceName, verb string) (bool, error) {
	if user == "" {
		return false, errors.New("user cannot be empty")
	}
	if namespace == "" {
		return false, errors.New("namespace cannot be empty")
	}
	if resource == "" {
		return false, errors.New("resource cannot be empty")
	}
	if verb == "" {
		return false, errors.New("verb cannot be empty")
	}

	sar := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User: user,
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: namespace,
				Group:     apiGroup,
				Resource:  resource,
				Name:      resourceName,
				Verb:      verb,
			},
		},
	}

	result, err := a.client.AuthorizationV1().SubjectAccessReviews().Create(
		ctx,
		sar,
		metav1.CreateOptions{},
	)
	if err != nil {
		return false, fmt.Errorf("authorization check failed: %w", err)
	}

	return result.Status.Allowed, nil
}
