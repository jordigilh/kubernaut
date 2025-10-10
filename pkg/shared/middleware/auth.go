<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Authenticator defines the interface for Kubernetes/OpenShift authentication providers
// Currently only OAuth2/JWT with Kubernetes TokenReview API is supported
type Authenticator interface {
	Authenticate(ctx context.Context, r *http.Request) (*AuthenticationResult, error)
	GetType() string
}

// AuthenticationResult contains the result of authentication
type AuthenticationResult struct {
	Authenticated bool
	Username      string
	Groups        []string
	Namespace     string
	Errors        []string
	Metadata      map[string]string // Additional auth metadata
}

// AuthenticationFilter provides HTTP middleware for Kubernetes/OpenShift authentication
type AuthenticationFilter struct {
	authenticator Authenticator
	logger        *logrus.Logger
	config        AuthFilterConfig
}

// AuthFilterConfig holds configuration for the authentication filter
type AuthFilterConfig struct {
	// SkipPaths are paths that bypass authentication (e.g., /health, /metrics)
	SkipPaths []string
	// LogSuccessfulAuth determines if successful authentications are logged
	LogSuccessfulAuth bool
	// LogFailedAuth determines if failed authentications are logged
	LogFailedAuth bool
	// CustomErrorHandler allows custom error response handling
	CustomErrorHandler func(w http.ResponseWriter, r *http.Request, authResult *AuthenticationResult, err error)
}

// NewAuthenticationFilter creates a new authentication filter
func NewAuthenticationFilter(authenticator Authenticator, logger *logrus.Logger, config AuthFilterConfig) *AuthenticationFilter {
	return &AuthenticationFilter{
		authenticator: authenticator,
		logger:        logger,
		config:        config,
	}
}

// Middleware returns the HTTP middleware function
func (af *AuthenticationFilter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip authentication for configured paths
		if af.shouldSkipAuthentication(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Skip if no authenticator is configured (allows optional authentication)
		if af.authenticator == nil {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		// Perform authentication
		authResult, err := af.authenticator.Authenticate(r.Context(), r)
		duration := time.Since(start)

		// Handle authentication failure
		if err != nil || (authResult != nil && !authResult.Authenticated) {
			af.handleAuthenticationFailure(w, r, authResult, err, duration)
			return
		}

		// Handle successful authentication
		af.handleAuthenticationSuccess(r, authResult, duration)

		// Add authentication context to request
		ctx := af.addAuthContextToRequest(r.Context(), authResult)
		r = r.WithContext(ctx)

		// Continue to next handler
		next.ServeHTTP(w, r)
	})
}

// shouldSkipAuthentication checks if the path should skip authentication
func (af *AuthenticationFilter) shouldSkipAuthentication(path string) bool {
	for _, skipPath := range af.config.SkipPaths {
		if path == skipPath || strings.HasPrefix(path, skipPath) {
			return true
		}
	}
	return false
}

// handleAuthenticationFailure handles authentication failures
func (af *AuthenticationFilter) handleAuthenticationFailure(w http.ResponseWriter, r *http.Request, authResult *AuthenticationResult, err error, duration time.Duration) {
	// Determine error message and status code
	errorMsg := "authentication failed"
	statusCode := http.StatusUnauthorized

	if err != nil {
		errorMsg = err.Error()
	} else if authResult != nil && len(authResult.Errors) > 0 {
		errorMsg = strings.Join(authResult.Errors, "; ")
	}

	// Determine appropriate status code based on error type
	if strings.Contains(errorMsg, "namespace mismatch") || strings.Contains(errorMsg, "ServiceAccount mismatch") {
		statusCode = http.StatusForbidden
	} else if strings.Contains(errorMsg, "configuration incomplete") || strings.Contains(errorMsg, "configuration error") {
		statusCode = http.StatusInternalServerError
	}

	// Log authentication failure
	if af.config.LogFailedAuth {
		af.logger.WithFields(logrus.Fields{
			"auth_type":   af.getAuthType(),
			"remote_ip":   r.RemoteAddr,
			"user_agent":  r.UserAgent(),
			"path":        r.URL.Path,
			"method":      r.Method,
			"error":       errorMsg,
			"status_code": statusCode,
			"duration":    duration,
			"component":   "auth_filter",
		}).Warn("Authentication failed")
	}

	// Use custom error handler if provided
	if af.config.CustomErrorHandler != nil {
		af.config.CustomErrorHandler(w, r, authResult, err)
		return
	}

	// Default error response
	af.sendErrorResponse(w, statusCode, errorMsg)
}

// handleAuthenticationSuccess handles successful authentication
func (af *AuthenticationFilter) handleAuthenticationSuccess(r *http.Request, authResult *AuthenticationResult, duration time.Duration) {
	if af.config.LogSuccessfulAuth {
		af.logger.WithFields(logrus.Fields{
			"auth_type":  af.getAuthType(),
			"username":   authResult.Username,
			"namespace":  authResult.Namespace,
			"groups":     authResult.Groups,
			"remote_ip":  r.RemoteAddr,
			"user_agent": r.UserAgent(),
			"path":       r.URL.Path,
			"method":     r.Method,
			"duration":   duration,
			"component":  "auth_filter",
		}).Info("Authentication successful")
	}
}

// addAuthContextToRequest adds authentication information to the request context
func (af *AuthenticationFilter) addAuthContextToRequest(ctx context.Context, authResult *AuthenticationResult) context.Context {
	if authResult == nil {
		return ctx
	}

	// Add authentication result to context for downstream handlers
	ctx = context.WithValue(ctx, "auth_result", authResult)
	ctx = context.WithValue(ctx, "auth_username", authResult.Username)
	ctx = context.WithValue(ctx, "auth_namespace", authResult.Namespace)
	ctx = context.WithValue(ctx, "auth_groups", authResult.Groups)

	return ctx
}

// sendErrorResponse sends a JSON error response
func (af *AuthenticationFilter) sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	// Simple JSON error response
	errorResponse := `{"status":"error","error":"` + message + `","message":"` + message + `"}`
	if _, err := w.Write([]byte(errorResponse)); err != nil {
		af.logger.WithError(err).Error("Failed to write error response")
	}
}

// getAuthType returns the authenticator type, handling nil case
func (af *AuthenticationFilter) getAuthType() string {
	if af.authenticator == nil {
		return "none"
	}
	return af.authenticator.GetType()
}

// AuthContextHelper provides helper functions to extract auth info from request context
type AuthContextHelper struct{}

// GetAuthResult extracts the AuthenticationResult from request context
func (AuthContextHelper) GetAuthResult(ctx context.Context) (*AuthenticationResult, bool) {
	result, ok := ctx.Value("auth_result").(*AuthenticationResult)
	return result, ok
}

// GetUsername extracts the authenticated username from request context
func (AuthContextHelper) GetUsername(ctx context.Context) (string, bool) {
	username, ok := ctx.Value("auth_username").(string)
	return username, ok
}

// GetNamespace extracts the authenticated namespace from request context
func (AuthContextHelper) GetNamespace(ctx context.Context) (string, bool) {
	namespace, ok := ctx.Value("auth_namespace").(string)
	return namespace, ok
}

// GetGroups extracts the authenticated groups from request context
func (AuthContextHelper) GetGroups(ctx context.Context) ([]string, bool) {
	groups, ok := ctx.Value("auth_groups").([]string)
	return groups, ok
}

// Global helper instance
var AuthContext = AuthContextHelper{}
