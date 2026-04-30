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

package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

var (
	// ErrMissingAuthorization indicates no Bearer token was provided.
	ErrMissingAuthorization = errors.New("missing or invalid authorization header")

	// ErrImpersonationDenied indicates the SA lacks impersonate RBAC for the target user.
	ErrImpersonationDenied = errors.New("impersonation denied")
)

// ExtractEffectiveUser resolves the acting user identity from the HTTP request using
// one of two patterns:
//
//   - Pattern A (direct): The Bearer token belongs to a real user. ValidateTokenFull
//     returns their identity directly.
//   - Pattern B (delegated): The Bearer token belongs to a ServiceAccount that is
//     authorized to impersonate on behalf of the real user. The real user identity
//     comes from Impersonate-User/Impersonate-Group headers, and a SAR check confirms
//     the SA has the "impersonate" verb on the target user.
//
// Pattern B is detected when: (1) the authenticated identity has the
// "system:serviceaccount:" prefix, AND (2) the request contains an Impersonate-User header.
func ExtractEffectiveUser(ctx context.Context, req *http.Request, authn auth.Authenticator, authz auth.Authorizer) (*UserInfo, error) {
	token := extractBearerToken(req)
	if token == "" {
		return nil, ErrMissingAuthorization
	}

	tokenInfo, err := authn.ValidateTokenFull(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	impersonateUser := req.Header.Get("Impersonate-User")

	if isServiceAccount(tokenInfo.Username) && impersonateUser != "" {
		return resolvePatternB(ctx, tokenInfo.Username, impersonateUser, req, authz)
	}

	return &UserInfo{
		Username: tokenInfo.Username,
		Groups:   tokenInfo.Groups,
	}, nil
}

func resolvePatternB(ctx context.Context, saUser, targetUser string, req *http.Request, authz auth.Authorizer) (*UserInfo, error) {
	allowed, err := authz.CheckAccessWithGroup(ctx, saUser, "", "", "users", targetUser, "impersonate")
	if err != nil {
		return nil, fmt.Errorf("impersonate SAR check failed for %q -> %q: %w", saUser, targetUser, err)
	}
	if !allowed {
		return nil, fmt.Errorf("%w: service account %q cannot impersonate user %q", ErrImpersonationDenied, saUser, targetUser)
	}

	groups := req.Header.Values("Impersonate-Group")

	return &UserInfo{
		Username: targetUser,
		Groups:   groups,
	}, nil
}

func extractBearerToken(req *http.Request) string {
	header := req.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(header, "Bearer ")
}

func isServiceAccount(username string) bool {
	return strings.HasPrefix(username, "system:serviceaccount:")
}
