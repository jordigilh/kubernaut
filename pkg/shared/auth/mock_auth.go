package auth

import (
	"context"
	"errors"
	"fmt"
)

// MockAuthenticator is a test double implementation of Authenticator.
//
// This mock is intended for integration tests where we want to validate authentication
// flow without making real Kubernetes API calls. The mock allows tests to control
// which tokens are valid and which users they map to.
//
// Authority: DD-AUTH-014
//
// Security Note: This mock is ONLY for testing purposes.
// Production code (cmd/) always uses K8sAuthenticator with real Kubernetes APIs.
//
// Example usage in integration tests:
//
//	authenticator := &auth.MockAuthenticator{
//	    ValidUsers: map[string]string{
//	        "test-token-authorized": "system:serviceaccount:test:authorized-sa",
//	        "test-token-readonly":   "system:serviceaccount:test:readonly-sa",
//	    },
//	}
//
//	// Test with valid token
//	user, err := authenticator.ValidateToken(ctx, "test-token-authorized")
//	// Returns: "system:serviceaccount:test:authorized-sa", nil
//
//	// Test with invalid token
//	user, err := authenticator.ValidateToken(ctx, "invalid-token")
//	// Returns: "", error
type MockAuthenticator struct {
	// ValidUsers maps tokens to user identities.
	// Key: token string
	// Value: user identity (e.g., "system:serviceaccount:namespace:sa-name")
	ValidUsers map[string]string

	// ErrorToReturn allows tests to simulate TokenReview API failures.
	// If set, ValidateToken will return this error instead of checking ValidUsers.
	ErrorToReturn error

	// CallCount tracks how many times ValidateToken was called.
	// Useful for verifying caching behavior.
	CallCount int
}

// ValidateToken implements the Authenticator interface for testing.
func (a *MockAuthenticator) ValidateToken(ctx context.Context, token string) (string, error) {
	a.CallCount++

	// Simulate API failure if configured
	if a.ErrorToReturn != nil {
		return "", a.ErrorToReturn
	}

	// Check if token is in the valid users map
	user, ok := a.ValidUsers[token]
	if !ok {
		return "", errors.New("invalid token")
	}

	return user, nil
}

// MockAuthorizer is a test double implementation of Authorizer.
//
// This mock is intended for integration tests where we want to validate authorization
// flow without making real Kubernetes SAR API calls. The mock allows tests to control
// which users are allowed access.
//
// Authority: DD-AUTH-014
//
// Security Note: This mock is ONLY for testing purposes.
// Production code (cmd/) always uses K8sAuthorizer with real Kubernetes APIs.
//
// Example usage in integration tests:
//
//	authorizer := &auth.MockAuthorizer{
//	    AllowedUsers: map[string]bool{
//	        "system:serviceaccount:test:authorized-sa": true,
//	        "system:serviceaccount:test:readonly-sa":   false,
//	    },
//	}
//
//	// Test with authorized user
//	allowed, err := authorizer.CheckAccess(
//	    ctx,
//	    "system:serviceaccount:test:authorized-sa",
//	    "kubernaut-system",
//	    "services",
//	    "data-storage-service",
//	    "create",
//	)
//	// Returns: true, nil
//
//	// Test with unauthorized user
//	allowed, err := authorizer.CheckAccess(
//	    ctx,
//	    "system:serviceaccount:test:readonly-sa",
//	    "kubernaut-system",
//	    "services",
//	    "data-storage-service",
//	    "create",
//	)
//	// Returns: false, nil
type MockAuthorizer struct {
	// AllowedUsers maps user identities to authorization decisions.
	// Key: user identity (e.g., "system:serviceaccount:namespace:sa-name")
	// Value: true if allowed, false if denied
	//
	// If a user is not in the map, access is denied by default (secure default).
	AllowedUsers map[string]bool

	// PerResourceDecisions allows fine-grained control for tests that need
	// different authorization results based on the resource being accessed.
	// Key: "namespace/resource/resourceName/verb" (e.g., "kubernaut-system/services/data-storage-service/create")
	// Value: map of user -> allowed
	//
	// If set, PerResourceDecisions takes precedence over AllowedUsers.
	PerResourceDecisions map[string]map[string]bool

	// ErrorToReturn allows tests to simulate SAR API failures.
	// If set, CheckAccess will return this error instead of checking AllowedUsers.
	ErrorToReturn error

	// CallCount tracks how many times CheckAccess was called.
	// Useful for verifying caching behavior.
	CallCount int
}

// CheckAccess implements the Authorizer interface for testing.
func (a *MockAuthorizer) CheckAccess(ctx context.Context, user, namespace, resource, resourceName, verb string) (bool, error) {
	a.CallCount++

	// Simulate API failure if configured
	if a.ErrorToReturn != nil {
		return false, a.ErrorToReturn
	}

	// Check per-resource decisions first (more specific)
	if a.PerResourceDecisions != nil {
		key := fmt.Sprintf("%s/%s/%s/%s", namespace, resource, resourceName, verb)
		if decisions, ok := a.PerResourceDecisions[key]; ok {
			allowed, exists := decisions[user]
			if exists {
				return allowed, nil
			}
		}
	}

	// Fall back to AllowedUsers (simpler)
	if a.AllowedUsers != nil {
		allowed, exists := a.AllowedUsers[user]
		if exists {
			return allowed, nil
		}
	}

	// Default deny (secure default)
	return false, nil
}
