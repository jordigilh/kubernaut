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
type auditSessionIDKey struct{}

// WithAuditSessionID attaches a session ID to the context for K8s call audit
// attribution. This is set by the MCP session handler alongside impersonation
// context so that the ImpersonatingRoundTripper can include session_id in
// audit events without importing internal packages.
func WithAuditSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, auditSessionIDKey{}, sessionID)
}

// AuditSessionIDFromContext extracts the audit session ID from context.
func AuditSessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(auditSessionIDKey{}).(string); ok {
		return v
	}
	return ""
}

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

// ImpersonateOption configures optional behaviour on ImpersonatingRoundTripper.
type ImpersonateOption func(*ImpersonatingRoundTripper)

// WithAuditor attaches a K8sCallAuditor to the round-tripper. When set,
// every impersonated K8s API call emits an audit event via the auditor
// (BR-INTERACTIVE-003, BR-AUDIT-005).
func WithAuditor(auditor K8sCallAuditor) ImpersonateOption {
	return func(t *ImpersonatingRoundTripper) {
		t.auditor = auditor
	}
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
	auditor  K8sCallAuditor
}

// NewImpersonatingRoundTripper wraps the given transport with per-request
// impersonation header injection. Options (e.g. WithAuditor) configure
// additional behaviour such as audit emission for interactive sessions.
func NewImpersonatingRoundTripper(delegate http.RoundTripper, opts ...ImpersonateOption) http.RoundTripper {
	rt := &ImpersonatingRoundTripper{delegate: delegate}
	for _, o := range opts {
		o(rt)
	}
	return rt
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

	resp, err := t.delegate.RoundTrip(clone)
	if err != nil {
		return nil, err
	}

	if t.auditor != nil {
		t.emitAudit(req.Context(), req, resp, username, groups)
	}

	return resp, nil
}

func (t *ImpersonatingRoundTripper) emitAudit(ctx context.Context, req *http.Request, resp *http.Response, username string, groups []string) {
	defer func() {
		if r := recover(); r != nil {
			// Fire-and-forget: audit must never crash the transport.
		}
	}()

	parsed := ParseK8sURL(req.Method, req.URL.Path)
	t.auditor.AuditK8sCall(ctx, K8sCallInfo{
		User:         username,
		Groups:       groups,
		Verb:         parsed.Verb,
		Resource:     parsed.Resource,
		Namespace:    parsed.Namespace,
		ResourceName: parsed.ResourceName,
		StatusCode:   resp.StatusCode,
		SessionID:    AuditSessionIDFromContext(ctx),
	})
}
