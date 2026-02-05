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

	// Check if token is authenticated
	if !result.Status.Authenticated {
		return "", errors.New("token not authenticated")
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
//
// This method:
// 1. Creates a SubjectAccessReview request with the provided parameters
// 2. Sends the request to the Kubernetes API server
// 3. Returns true if the user is allowed, false if denied
//
// Authority: https://kubernetes.io/docs/reference/kubernetes-api/authorization-resources/subject-access-review-v1/
func (a *K8sAuthorizer) CheckAccess(ctx context.Context, user, namespace, resource, resourceName, verb string) (bool, error) {
	// Validate required parameters
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

	// Create SubjectAccessReview request
	sar := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User: user,
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: namespace,
				Resource:  resource,
				Name:      resourceName, // Optional: specific resource instance
				Verb:      verb,
			},
		},
	}

	// Call Kubernetes SubjectAccessReview API
	result, err := a.client.AuthorizationV1().SubjectAccessReviews().Create(
		ctx,
		sar,
		metav1.CreateOptions{},
	)
	if err != nil {
		return false, fmt.Errorf("authorization check failed: %w", err)
	}

	// Return authorization decision
	// Note: Denial (Allowed=false) is not an error - it's a valid authorization decision
	return result.Status.Allowed, nil
}
