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

package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// CompositeAuthenticator routes tokens to the appropriate authenticator based
// on token shape: JWT-shaped tokens (3-dot structure) are tried against the
// JWTAuthenticator first; opaque tokens go directly to K8sAuthenticator.
//
// Fail-closed semantics: if a JWT routes to a known issuer but fails signature
// verification, the error is returned immediately without fallback. Fallback to
// K8s only occurs when the issuer is unknown (ErrIssuerNotFound) or the token
// cannot be parsed as a JWT at all.
//
// DD-AUTH-MCP-001 v2.0: Pattern A (SA token) and Pattern B (JWT) coexist.
type CompositeAuthenticator struct {
	jwtAuth *JWTAuthenticator
	k8sAuth Authenticator
}

// NewCompositeAuthenticator creates a CompositeAuthenticator that routes between
// JWT and K8s authenticators. If jwtAuth is nil, all tokens go to k8sAuth.
func NewCompositeAuthenticator(jwtAuth *JWTAuthenticator, k8sAuth Authenticator) *CompositeAuthenticator {
	return &CompositeAuthenticator{
		jwtAuth: jwtAuth,
		k8sAuth: k8sAuth,
	}
}

// ValidateToken implements Authenticator.ValidateToken.
func (c *CompositeAuthenticator) ValidateToken(ctx context.Context, token string) (string, error) {
	info, err := c.ValidateTokenFull(ctx, token)
	if err != nil {
		return "", err
	}
	return info.Username, nil
}

// ValidateTokenFull implements Authenticator.ValidateTokenFull with composite routing.
func (c *CompositeAuthenticator) ValidateTokenFull(ctx context.Context, token string) (UserInfo, error) {
	if token == "" {
		return UserInfo{}, fmt.Errorf("%w: empty token", ErrTokenInvalid)
	}

	if c.jwtAuth != nil && looksLikeJWT(token) {
		userInfo, err := c.jwtAuth.ValidateTokenFull(ctx, token)
		if err == nil {
			return userInfo, nil
		}

		if errors.Is(err, ErrIssuerNotFound) || errors.Is(err, ErrTokenMalformed) {
			return c.k8sAuth.ValidateTokenFull(ctx, token)
		}

		return UserInfo{}, err
	}

	return c.k8sAuth.ValidateTokenFull(ctx, token)
}

// looksLikeJWT checks if a token has the 3-part dot-separated structure of a JWT.
// This is a fast heuristic — actual JWT parsing and validation happens in
// JWTAuthenticator.
func looksLikeJWT(token string) bool {
	return strings.Count(token, ".") == 2
}

var _ Authenticator = (*CompositeAuthenticator)(nil)
