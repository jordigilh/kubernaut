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

package conversation

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

const (
	ErrorTypeAuthenticationFailed = "https://kubernaut.ai/problems/authentication-failed"
	ErrorTypeAccessDenied         = "https://kubernaut.ai/problems/access-denied"
)

// AuthError returns an RFC 7807 error for authentication failures.
func AuthError(detail, instance string) *gwerrors.RFC7807Error {
	return &gwerrors.RFC7807Error{
		Type:     ErrorTypeAuthenticationFailed,
		Title:    "Unauthorized",
		Detail:   detail,
		Status:   http.StatusUnauthorized,
		Instance: instance,
	}
}

// ForbiddenError returns an RFC 7807 error for authorization failures.
func ForbiddenError(detail, instance string) *gwerrors.RFC7807Error {
	return &gwerrors.RFC7807Error{
		Type:     ErrorTypeAccessDenied,
		Title:    "Forbidden",
		Detail:   detail,
		Status:   http.StatusForbidden,
		Instance: instance,
	}
}

// RateLimitError returns an RFC 7807 error for rate limit violations.
func RateLimitError(detail, instance string) *gwerrors.RFC7807Error {
	return &gwerrors.RFC7807Error{
		Type:     gwerrors.ErrorTypeTooManyRequests,
		Title:    gwerrors.TitleTooManyRequests,
		Detail:   detail,
		Status:   http.StatusTooManyRequests,
		Instance: instance,
	}
}

// ConversationAuth validates tokens and checks SAR for conversation access.
type ConversationAuth struct {
	authenticator auth.Authenticator
	authorizer    auth.Authorizer
}

// NewConversationAuth creates an auth handler reusing pkg/shared/auth (DD-AUTH-014).
func NewConversationAuth(authn auth.Authenticator, authz auth.Authorizer) *ConversationAuth {
	return &ConversationAuth{authenticator: authn, authorizer: authz}
}

// ErrTokenValidation is returned when a bearer token cannot be validated.
var ErrTokenValidation = errors.New("token validation failed")

// Authenticate validates a bearer token and returns the user identity.
func (a *ConversationAuth) Authenticate(ctx context.Context, token string) (string, error) {
	userID, err := a.authenticator.ValidateToken(ctx, token)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrTokenValidation, err)
	}
	return userID, nil
}

// AuthorizeRAR checks if the user can UPDATE the given RAR (dynamic SAR) in the kubernaut.ai API group.
func (a *ConversationAuth) AuthorizeRAR(ctx context.Context, userID, namespace, rarName string) (bool, error) {
	allowed, err := a.authorizer.CheckAccessWithGroup(ctx, userID, namespace,
		"kubernaut.ai", "remediationapprovalrequests", rarName, "update")
	if err != nil {
		return false, fmt.Errorf("SAR check failed: %w", err)
	}
	return allowed, nil
}
