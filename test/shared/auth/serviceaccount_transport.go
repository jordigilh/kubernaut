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

package auth

import (
	"fmt"
	"net/http"
)

// ServiceAccountTransport is an http.RoundTripper that injects Kubernetes ServiceAccount
// Bearer tokens into HTTP requests for E2E testing with real middleware-based authentication.
//
// Authority: DD-AUTH-014 (Middleware-Based SAR Authentication)
//
// Usage:
//
//	token, err := infrastructure.GetServiceAccountToken(ctx, namespace, "datastorage-e2e-sa", kubeconfigPath)
//	transport := auth.NewServiceAccountTransport(token)
//	client := &http.Client{Transport: transport}
//
// This injects Bearer tokens that are validated by DataStorage middleware using:
// 1. Kubernetes TokenReview API (authentication)
// 2. Kubernetes SubjectAccessReview API (authorization with SAR)
type ServiceAccountTransport struct {
	base  http.RoundTripper
	token string
}

// NewServiceAccountTransport creates a new ServiceAccountTransport with the given token.
//
// Parameters:
//   - token: Kubernetes ServiceAccount Bearer token (from Secret)
//
// Returns:
//   - *ServiceAccountTransport: HTTP transport that injects Bearer token
func NewServiceAccountTransport(token string) *ServiceAccountTransport {
	return &ServiceAccountTransport{
		base:  http.DefaultTransport,
		token: token,
	}
}

// RoundTrip implements http.RoundTripper interface.
// It clones the request and adds Authorization header with Bearer token.
//
// Authority: DD-AUTH-014 (Middleware-based authentication for E2E)
func (t *ServiceAccountTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid modifying original
	reqClone := req.Clone(req.Context())

	// Inject Bearer token (DataStorage middleware validates against Kubernetes API)
	// Authentication: TokenReview API
	// Authorization: SubjectAccessReview API # notsecret
	reqClone.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.token))

	// Execute request with injected token
	return t.base.RoundTrip(reqClone)
}
