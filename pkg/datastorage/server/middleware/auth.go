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

package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// ContextKey is the type for context keys used in the auth middleware
type ContextKey string

const (
	// UserContextKey is the context key for the authenticated user identity
	UserContextKey ContextKey = "user"
)

// AuthMiddleware provides authentication and authorization for HTTP requests.
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
// The X-Auth-Request-User header mimics oauth-proxy behavior, enabling handlers
// (like ExportAuditEvents) to track user attribution for SOC2 compliance (CC8.1).
//
// Security: No runtime disable flags - auth is always enforced via interface implementations.
//
// Testing:
// - Production: Inject K8sAuthenticator + K8sAuthorizer (real Kubernetes APIs)
// - Integration: Inject MockAuthenticator + MockAuthorizer (test doubles)
// - E2E: Inject K8sAuthenticator + K8sAuthorizer (real APIs in Kind)
type AuthMiddleware struct {
	authenticator auth.Authenticator
	authorizer    auth.Authorizer
	config        AuthConfig
	logger        logr.Logger
}

// AuthConfig contains the SAR configuration for authorization checks.
type AuthConfig struct {
	// Namespace is the Kubernetes namespace for the SAR check
	Namespace string

	// Resource is the Kubernetes resource type (e.g., "services", "pods")
	Resource string

	// ResourceName is the specific resource name (e.g., "data-storage-service")
	// Can be empty for list/create operations on resource types
	ResourceName string

	// Verb is the RBAC verb to check (e.g., "create", "get", "list", "update", "delete")
	// Authority: DD-AUTH-011 (Granular RBAC with SAR verb mapping)
	//   - "create": Write operations (audit events, workflows)
	//   - "get": Read operations (query, retrieve)
	//   - "list": List operations (search, list all)
	//   - "update": Update operations (patch, modify)
	//   - "delete": Delete operations (remove)
	Verb string
}

// NewAuthMiddleware creates a new authentication middleware with dependency injection.
//
// Example (Production):
//
//	k8sClient, _ := kubernetes.NewForConfig(config)
//	authenticator := auth.NewK8sAuthenticator(k8sClient)
//	authorizer := auth.NewK8sAuthorizer(k8sClient)
//	authMiddleware := middleware.NewAuthMiddleware(
//	    authenticator,
//	    authorizer,
//	    middleware.AuthConfig{
//	        Namespace:    "kubernaut-system",
//	        Resource:     "services",
//	        ResourceName: "data-storage-service",
//	        Verb:         "create",
//	    },
//	    logger,
//	)
//
// Example (Integration Tests):
//
//	authenticator := &auth.MockAuthenticator{
//	    ValidUsers: map[string]string{"test-token": "system:serviceaccount:test:sa"},
//	}
//	authorizer := &auth.MockAuthorizer{
//	    AllowedUsers: map[string]bool{"system:serviceaccount:test:sa": true},
//	}
//	authMiddleware := middleware.NewAuthMiddleware(authenticator, authorizer, config, logger)
func NewAuthMiddleware(
	authenticator auth.Authenticator,
	authorizer auth.Authorizer,
	config AuthConfig,
	logger logr.Logger,
) *AuthMiddleware {
	return &AuthMiddleware{
		authenticator: authenticator,
		authorizer:    authorizer,
		config:        config,
		logger:        logger.WithName("auth-middleware"),
	}
}

// Handler returns a chi-compatible middleware handler.
//
// Request Flow:
// 1. Extract Bearer token from "Authorization" header
// 2. Authenticate token using Kubernetes TokenReview API
// 3. Authorize user using Kubernetes SubjectAccessReview (SAR) API
// 4. Inject user identity into request context (available for audit logging)
// 5. Inject X-Auth-Request-User header (for SOC2 user attribution)
// 6. Pass request to next handler
//
// HTTP Status Codes (Authority: DD-AUTH-013):
// - 401 Unauthorized: Missing/invalid/expired token
// - 403 Forbidden: Valid token but insufficient RBAC permissions
// - 500 Internal Server Error: TokenReview/SAR API call failures
func (m *AuthMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Step 1: Extract Bearer token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			m.logger.Error(nil, "DD-AUTH-014 DEBUG: Authentication failed - missing Authorization header",
				"path", r.URL.Path,
				"method", r.Method,
			)
			m.writeError(w, http.StatusUnauthorized, "Unauthorized", "Missing Authorization header")
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			m.logger.Error(nil, "DD-AUTH-014 DEBUG: Authentication failed - invalid Authorization header format",
				"path", r.URL.Path,
				"method", r.Method,
				"auth_header_prefix", authHeader[:min(30, len(authHeader))],
			)
			m.writeError(w, http.StatusUnauthorized, "Unauthorized", "Invalid Authorization header format (expected 'Bearer <token>')")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			m.logger.Error(nil, "DD-AUTH-014 DEBUG: Authentication failed - empty Bearer token",
				"path", r.URL.Path,
				"method", r.Method,
			)
			m.writeError(w, http.StatusUnauthorized, "Unauthorized", "Empty Bearer token")
			return
		}

		// DD-AUTH-014 DEBUG: Log token validation attempt
		m.logger.Info("DD-AUTH-014 DEBUG: Attempting TokenReview",
			"path", r.URL.Path,
			"method", r.Method,
			"token_length", len(token),
			"token_prefix", token[:min(30, len(token))],
		)

		// Step 2: Authenticate token using TokenReview API (via injected Authenticator)
		user, err := m.authenticator.ValidateToken(r.Context(), token)
		if err != nil {
			m.logger.Error(err, "DD-AUTH-014 DEBUG: Authentication failed - token validation failed",
				"path", r.URL.Path,
				"method", r.Method,
			)
			m.writeError(w, http.StatusUnauthorized, "Unauthorized", fmt.Sprintf("Token validation failed: %v", err))
			return
		}

		m.logger.Info("DD-AUTH-014 DEBUG: Token validated successfully",
			"user", user,
			"path", r.URL.Path,
			"method", r.Method,
		)

		m.logger.V(2).Info("Token validated successfully",
			"user", user,
			"path", r.URL.Path,
			"method", r.Method,
		)

		// DD-AUTH-014 DEBUG: Log SAR check attempt
		m.logger.Info("DD-AUTH-014 DEBUG: Attempting SAR check",
			"user", user,
			"namespace", m.config.Namespace,
			"resource", m.config.Resource,
			"resourceName", m.config.ResourceName,
			"verb", m.config.Verb,
			"path", r.URL.Path,
			"method", r.Method,
		)

		// Step 3: Authorize user using SubjectAccessReview API (via injected Authorizer)
		allowed, err := m.authorizer.CheckAccess(
			r.Context(),
			user,
			m.config.Namespace,
			m.config.Resource,
			m.config.ResourceName,
			m.config.Verb,
		)
		if err != nil {
			// SAR API call failed (not authorization denial)
			m.logger.Error(err, "DD-AUTH-014 DEBUG: Authorization check failed - SAR API error",
				"user", user,
				"path", r.URL.Path,
				"method", r.Method,
				"namespace", m.config.Namespace,
				"resource", m.config.Resource,
				"resourceName", m.config.ResourceName,
				"verb", m.config.Verb,
			)
			m.writeError(w, http.StatusInternalServerError, "Internal Server Error", fmt.Sprintf("Authorization check failed: %v", err))
			return
		}

		if !allowed {
			// User is authenticated but not authorized (RBAC denial)
			m.logger.Error(nil, "DD-AUTH-014 DEBUG: Authorization denied - insufficient RBAC permissions",
				"user", user,
				"path", r.URL.Path,
				"method", r.Method,
				"namespace", m.config.Namespace,
				"resource", m.config.Resource,
				"resourceName", m.config.ResourceName,
				"verb", m.config.Verb,
			)
			m.writeError(w, http.StatusForbidden, "Forbidden", fmt.Sprintf("Insufficient RBAC permissions: %s verb:%s on %s/%s", user, m.config.Verb, m.config.Resource, m.config.ResourceName))
			return
		}

		// DD-AUTH-014 DEBUG: Log successful authorization
		m.logger.Info("DD-AUTH-014 DEBUG: Authorization successful",
			"user", user,
			"namespace", m.config.Namespace,
			"resource", m.config.Resource,
			"resourceName", m.config.ResourceName,
			"verb", m.config.Verb,
		)

		m.logger.V(2).Info("Authorization granted",
			"user", user,
			"path", r.URL.Path,
			"method", r.Method,
		)

		// Step 4: Inject user identity into request context (for audit logging)
		// Authority: DD-AUTH-009 (Workflow Attribution), SOC2 CC8.1 (User tracking)
		ctx := context.WithValue(r.Context(), UserContextKey, user)

		// Step 5: Inject X-Auth-Request-User header (for handlers that require it)
		// This mimics oauth-proxy behavior, enabling handlers like ExportAuditEvents
		// to track user attribution for SOC2 compliance (CC8.1)
		// Authority: DD-AUTH-014 (Middleware-based authentication replaces oauth-proxy)
		r.Header.Set("X-Auth-Request-User", user)

		// Step 6: Pass request to next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserFromContext extracts the authenticated user identity from the request context.
//
// This is a helper function for handlers that need to access the user identity
// for audit logging or user attribution.
//
// Example:
//
//	func (h *Handler) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
//	    user := middleware.GetUserFromContext(r.Context())
//	    // Log audit event with user attribution
//	    h.auditStore.LogEvent(ctx, "workflow.created", user, ...)
//	}
//
// Returns empty string if user is not in context (should not happen if middleware is applied).
func GetUserFromContext(ctx context.Context) string {
	if user, ok := ctx.Value(UserContextKey).(string); ok {
		return user
	}
	return ""
}

// writeError writes an RFC 7807 Problem Details JSON error response.
//
// This ensures error responses are properly formatted JSON that the OpenAPI client can parse.
//
// Authority: RFC 7807 (Problem Details for HTTP APIs)
func (m *AuthMiddleware) writeError(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	
	problem := map[string]interface{}{
		"type":   "about:blank",
		"title":  title,
		"status": status,
		"detail": detail,
	}
	
	json.NewEncoder(w).Encode(problem)
}
