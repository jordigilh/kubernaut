/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package authwebhook

import (
	"context"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
)

// Authenticator extracts user identity from Kubernetes authentication context
// This is the CORE authentication logic for all CRD webhooks
// Implements BR-WE-013 and SOC2 CC8.1 (Attribution) requirements
type Authenticator struct{}

// NewAuthenticator creates a new Authenticator
func NewAuthenticator() *Authenticator {
	return &Authenticator{}
}

// ExtractUser extracts authenticated user from admission request
// This extracts REAL user identity from Kubernetes authentication context
// The K8s API Server has already authenticated the user via OIDC/certs/SA tokens
//
// Parameters:
//   - ctx: Request context
//   - req: Admission request containing authenticated UserInfo
//
// Returns:
//   - AuthContext: Authenticated user information
//   - error: If user information is missing or invalid
func (a *Authenticator) ExtractUser(ctx context.Context, req *admissionv1.AdmissionRequest) (*AuthContext, error) {
	// Validate username exists
	if req.UserInfo.Username == "" {
		return nil, fmt.Errorf("no user information in request")
	}

	// Validate UID exists (required for unique user identification)
	if req.UserInfo.UID == "" {
		return nil, fmt.Errorf("no user UID in request")
	}

	// Extract authenticated user information
	return &AuthContext{
		Username: req.UserInfo.Username,
		UID:      req.UserInfo.UID,
		Groups:   req.UserInfo.Groups,
		Extra:    req.UserInfo.Extra,
	}, nil
}

