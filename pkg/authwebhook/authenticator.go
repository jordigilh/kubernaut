package authwebhook

import (
	"context"

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
// TDD RED Phase: Stub implementation - tests will fail
func (a *Authenticator) ExtractUser(ctx context.Context, req *admissionv1.AdmissionRequest) (*AuthContext, error) {
	panic("implement me: ExtractUser")
}
