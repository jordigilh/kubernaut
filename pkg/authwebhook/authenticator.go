package authwebhook

import (
	"context"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
)

// Authenticator extracts authenticated user identity from Kubernetes admission requests
// BR-WEBHOOK-001: SOC2 CC8.1 Attribution - captures WHO performed operator actions
type Authenticator struct{}

// NewAuthenticator creates a new authenticator instance
func NewAuthenticator() *Authenticator {
	return &Authenticator{}
}

// ExtractUser extracts authenticated user information from admission request
// Returns error if user info is missing or invalid
//
// BR-WEBHOOK-001: Extract authenticated user identity from K8s admission request
// SOC2 CC8.1: Attribution requirement - must capture WHO performed the action
func (a *Authenticator) ExtractUser(ctx context.Context, req *admissionv1.AdmissionRequest) (*AuthContext, error) {
	// Validate admission request
	if req == nil {
		return nil, fmt.Errorf("admission request cannot be nil")
	}

	// Extract username (required)
	username := req.UserInfo.Username
	if username == "" {
		return nil, fmt.Errorf("username is required for authentication")
	}

	// Extract UID (optional - not available in envtest/kubeconfig contexts)
	// Note: In production clusters, service accounts have UIDs, but test environments
	// and user kubeconfig contexts may not. Username is sufficient for SOC2 attribution.
	uid := req.UserInfo.UID

	return &AuthContext{
		Username: username,
		UID:      uid,       // May be empty in test environments - this is acceptable
		Groups:   req.UserInfo.Groups,   // AUTH-004, AUTH-009, AUTH-010: Preserve group memberships for RBAC audit
		Extra:    req.UserInfo.Extra,     // Preserve extra attributes for comprehensive audit context
	}, nil
}
