// Package auth provides authentication and authorization interfaces and implementations
// for Kubernetes-based REST API services.
//
// Authority: DD-AUTH-014 (Middleware-Based SAR Authentication)
//
// This package implements a secure, testable authentication/authorization framework using
// dependency injection. The interfaces allow for:
// - Production: Real Kubernetes TokenReview + SubjectAccessReview (SAR)
// - Integration tests: Mock implementations (auth still enforced)
// - E2E tests: Real Kubernetes APIs in Kind clusters
//
// Security: No runtime disable flags - auth is always enforced via interface implementations.
package auth

import "context"

// Authenticator validates tokens and returns user identity.
//
// Implementations:
//   - K8sAuthenticator: Uses Kubernetes TokenReview API (production/E2E)
//   - MockAuthenticator: Test double for integration tests
//
// Example usage:
//
//	authenticator := auth.NewK8sAuthenticator(k8sClient)
//	user, err := authenticator.ValidateToken(ctx, "Bearer eyJhbGc...")
//	if err != nil {
//	    // Return 401 Unauthorized
//	}
type Authenticator interface {
	// ValidateToken checks if the token is valid and returns the user identity.
	//
	// Parameters:
	//   - ctx: Request context (may include timeout)
	//   - token: Bearer token string (without "Bearer " prefix)
	//
	// Returns:
	//   - string: User identity (e.g., "system:serviceaccount:namespace:sa-name")
	//   - error: Token validation failure
	//
	// Errors:
	//   - Token is invalid or expired
	//   - Token cannot be authenticated
	//   - Kubernetes API call fails
	ValidateToken(ctx context.Context, token string) (string, error)
}

// Authorizer checks if a user has permission to perform an action on a resource.
//
// Implementations:
//   - K8sAuthorizer: Uses Kubernetes SubjectAccessReview (SAR) API (production/E2E)
//   - MockAuthorizer: Test double for integration tests
//
// Example usage:
//
//	authorizer := auth.NewK8sAuthorizer(k8sClient)
//	allowed, err := authorizer.CheckAccess(
//	    ctx,
//	    "system:serviceaccount:kubernaut-system:datastorage",
//	    "kubernaut-system",     // namespace
//	    "services",             // resource
//	    "data-storage-service", // resourceName
//	    "create",               // verb
//	)
//	if err != nil || !allowed {
//	    // Return 403 Forbidden
//	}
type Authorizer interface {
	// CheckAccess verifies if the user has the required permissions.
	//
	// This method performs a Kubernetes SubjectAccessReview (SAR) check to determine
	// if the specified user can perform the given verb on the specified resource.
	//
	// Parameters:
	//   - ctx: Request context (may include timeout)
	//   - user: User identity from token validation (e.g., "system:serviceaccount:ns:sa")
	//   - namespace: Kubernetes namespace for the resource
	//   - resource: Resource type (e.g., "services", "pods", "deployments")
	//   - resourceName: Specific resource name (e.g., "data-storage-service")
	//   - verb: RBAC verb (e.g., "create", "get", "list", "update", "delete")
	//
	// Returns:
	//   - bool: true if access is allowed, false if denied
	//   - error: SAR API call failure (not authorization denial)
	//
	// Errors:
	//   - Kubernetes API call fails
	//   - Invalid parameters
	//   - Network timeout
	//
	// Note: Authorization denial (user lacks permissions) returns (false, nil), not an error.
	CheckAccess(ctx context.Context, user, namespace, resource, resourceName, verb string) (bool, error)
}
