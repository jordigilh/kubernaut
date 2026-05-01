/*
Copyright 2026 Jordi Gil.

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

package transport

import (
	"context"
	"net/http"
)

type impersonationContextKey struct{}

type impersonationIdentity struct {
	username string
	groups   []string
}

// WithImpersonatedUser returns a context carrying the impersonated user identity.
// When this context reaches the ImpersonatingRoundTripper, the transport will
// inject Impersonate-User and Impersonate-Group headers into the K8s API request,
// causing the API server to evaluate RBAC as the specified user (#703, DD-AUTH-MCP-001).
func WithImpersonatedUser(ctx context.Context, username string, groups []string) context.Context {
	return context.WithValue(ctx, impersonationContextKey{}, &impersonationIdentity{
		username: username,
		groups:   groups,
	})
}

// ImpersonatedUserFromContext extracts the impersonated user identity from context.
// Returns empty username and nil groups if no impersonation is set.
func ImpersonatedUserFromContext(ctx context.Context) (username string, groups []string) {
	val := ctx.Value(impersonationContextKey{})
	if val == nil {
		return "", nil
	}
	id := val.(*impersonationIdentity)
	return id.username, id.groups
}

// ImpersonatingRoundTripper is an http.RoundTripper that conditionally injects
// Kubernetes impersonation headers based on context values. When context carries
// an impersonated user (via WithImpersonatedUser), the transport adds
// Impersonate-User and Impersonate-Group headers so the API server enforces the
// user's RBAC rather than the service account's.
//
// When context has no impersonation identity, the request passes through unchanged
// (autonomous mode operates as the KA service account).
type ImpersonatingRoundTripper struct {
	delegate http.RoundTripper
}

// NewImpersonatingRoundTripper wraps the given transport with per-request
// impersonation header injection.
func NewImpersonatingRoundTripper(delegate http.RoundTripper) http.RoundTripper {
	return &ImpersonatingRoundTripper{delegate: delegate}
}

func (t *ImpersonatingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	username, groups := ImpersonatedUserFromContext(req.Context())
	if username == "" {
		return t.delegate.RoundTrip(req)
	}

	clone := req.Clone(req.Context())
	clone.Header.Set("Impersonate-User", username)
	for _, g := range groups {
		clone.Header.Add("Impersonate-Group", g)
	}

	return t.delegate.RoundTrip(clone)
}
