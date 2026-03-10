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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
)

// ContextKey is the type for context keys used in the auth middleware.
type ContextKey string

const (
	// UserContextKey is the context key for the authenticated user identity.
	UserContextKey ContextKey = "user"
)

// Middleware provides authentication and authorization for HTTP requests.
//
// Authority: DD-AUTH-014 (Middleware-Based SAR Authentication)
//
// This middleware implements a secure, testable auth framework using dependency injection:
// 1. Extracts Bearer token from Authorization header
// 2. Validates token using Authenticator interface (TokenReview)
// 3. Checks authorization using Authorizer interface (SAR)
// 4. Injects user identity into request context (for audit logging)
// 5. Injects X-Auth-Request-User header (for SOC2 user attribution)
//
// Security: No runtime disable flags — auth is always enforced via interface implementations.
//
// Used by: DataStorage service (BR-DS-*), Gateway service (BR-GATEWAY-036/037)
type Middleware struct {
	authenticator Authenticator
	authorizer    Authorizer
	config        MiddlewareConfig
	logger        logr.Logger
}

// MiddlewareConfig contains the SAR configuration for authorization checks.
type MiddlewareConfig struct {
	// Namespace is the Kubernetes namespace for the SAR check
	Namespace string

	// Resource is the Kubernetes resource type (e.g., "services", "pods")
	Resource string

	// ResourceName is the specific resource name (e.g., "data-storage-service", "gateway-service")
	ResourceName string

	// Verb is the RBAC verb to check (e.g., "create", "get", "list", "update", "delete")
	// Authority: DD-AUTH-011 (Granular RBAC with SAR verb mapping)
	Verb string
}

// NewMiddleware creates a new authentication middleware with dependency injection.
//
// Example (Production):
//
//	k8sClient, _ := kubernetes.NewForConfig(config)
//	authenticator := auth.NewK8sAuthenticator(k8sClient)
//	authorizer := auth.NewK8sAuthorizer(k8sClient)
//	mw := auth.NewMiddleware(
//	    authenticator,
//	    authorizer,
//	    auth.MiddlewareConfig{
//	        Namespace:    "kubernaut-system",
//	        Resource:     "services",
//	        ResourceName: "gateway-service",
//	        Verb:         "create",
//	    },
//	    logger,
//	)
func NewMiddleware(
	authenticator Authenticator,
	authorizer Authorizer,
	config MiddlewareConfig,
	logger logr.Logger,
) *Middleware {
	return &Middleware{
		authenticator: authenticator,
		authorizer:    authorizer,
		config:        config,
		logger:        logger.WithName("auth-middleware"),
	}
}

// Handler returns a chi-compatible middleware handler.
//
// HTTP Status Codes (Authority: DD-AUTH-013):
// - 401 Unauthorized: Missing/invalid/expired token
// - 403 Forbidden: Valid token but insufficient RBAC permissions
// - 500 Internal Server Error: TokenReview/SAR API call failures
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			m.writeError(w, http.StatusUnauthorized, "Unauthorized", "Missing Authorization header")
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			m.writeError(w, http.StatusUnauthorized, "Unauthorized", "Invalid Authorization header format (expected 'Bearer <token>')")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			m.writeError(w, http.StatusUnauthorized, "Unauthorized", "Empty Bearer token")
			return
		}

		user, err := m.authenticator.ValidateToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, ErrTokenInvalid) {
				m.writeError(w, http.StatusUnauthorized, "Unauthorized", "Invalid or expired token")
				return
			}
			m.logger.Error(err, "Token validation failed",
				"path", r.URL.Path,
				"method", r.Method,
			)
			m.writeError(w, http.StatusInternalServerError, "Internal Server Error", fmt.Sprintf("Token validation failed: %v", err))
			return
		}

		m.logger.V(2).Info("Token validated",
			"user", user,
			"path", r.URL.Path,
		)

		allowed, err := m.authorizer.CheckAccess(
			r.Context(),
			user,
			m.config.Namespace,
			m.config.Resource,
			m.config.ResourceName,
			m.config.Verb,
		)
		if err != nil {
			m.logger.Error(err, "Authorization check failed",
				"user", user,
				"path", r.URL.Path,
			)
			m.writeError(w, http.StatusInternalServerError, "Internal Server Error", fmt.Sprintf("Authorization check failed: %v", err))
			return
		}

		if !allowed {
			m.logger.Info("Authorization denied",
				"user", user,
				"path", r.URL.Path,
				"resource", m.config.Resource,
				"resourceName", m.config.ResourceName,
				"verb", m.config.Verb,
			)
			m.writeError(w, http.StatusForbidden, "Forbidden", fmt.Sprintf("Insufficient RBAC permissions: %s verb:%s on %s/%s", user, m.config.Verb, m.config.Resource, m.config.ResourceName))
			return
		}

		m.logger.V(2).Info("Authorization granted",
			"user", user,
			"path", r.URL.Path,
		)

		ctx := context.WithValue(r.Context(), UserContextKey, user)
		r.Header.Set("X-Auth-Request-User", user)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserFromContext extracts the authenticated user identity from the request context.
// Returns empty string if user is not in context.
func GetUserFromContext(ctx context.Context) string {
	if user, ok := ctx.Value(UserContextKey).(string); ok {
		return user
	}
	return ""
}

// writeError writes an RFC 7807 Problem Details JSON error response.
func (m *Middleware) writeError(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	problem := map[string]any{
		"type":   "about:blank",
		"title":  title,
		"status": status,
		"detail": detail,
	}

	_ = json.NewEncoder(w).Encode(problem)
}
