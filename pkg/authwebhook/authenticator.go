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

	// Extract username
	username := req.UserInfo.Username
	if username == "" {
		return nil, fmt.Errorf("username is required for authentication")
	}

	// Extract UID
	uid := req.UserInfo.UID
	if uid == "" {
		return nil, fmt.Errorf("UID is required for authentication")
	}

	return &AuthContext{
		Username: username,
		UID:      uid,
		Groups:   req.UserInfo.Groups,   // AUTH-004, AUTH-009, AUTH-010: Preserve group memberships for RBAC audit
		Extra:    req.UserInfo.Extra,     // Preserve extra attributes for comprehensive audit context
	}, nil
}
