package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// AuthMiddleware handles authentication using Kubernetes TokenReview API
// BR-TOOLSET-032: OAuth2/Bearer token authentication via TokenReviewer
//
// This middleware:
// 1. Extracts Bearer token from Authorization header
// 2. Validates token using Kubernetes TokenReview API
// 3. Returns 401 Unauthorized for invalid/missing tokens
// 4. Returns 500 Internal Server Error for TokenReview API failures
// 5. Passes authenticated requests to next handler
//
// Security:
// - All API endpoints (except /health, /ready) must use this middleware
// - Tokens are validated per-request (no caching)
// - ServiceAccount tokens are recommended for production
type AuthMiddleware struct {
	clientset kubernetes.Interface
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(clientset kubernetes.Interface) *AuthMiddleware {
	return &AuthMiddleware{
		clientset: clientset,
	}
}

// Middleware returns an HTTP middleware function
func (a *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract Bearer token
		token, err := a.extractToken(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("Unauthorized: %v", err), http.StatusUnauthorized)
			return
		}

		// Validate token using TokenReview API
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		authenticated, _, err := a.validateToken(ctx, token)
		if err != nil {
			// TokenReview API failure
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if !authenticated {
			http.Error(w, "Unauthorized: Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Pass to next handler
		next.ServeHTTP(w, r)
	})
}

// extractToken extracts Bearer token from Authorization header
func (a *AuthMiddleware) extractToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", fmt.Errorf("Bearer token required")
	}

	// Check for "Bearer " prefix
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", fmt.Errorf("malformed Authorization header (expected 'Bearer <token>')")
	}

	token := parts[1]
	if token == "" {
		return "", fmt.Errorf("empty token")
	}

	return token, nil
}

// validateToken validates a token using Kubernetes TokenReview API
func (a *AuthMiddleware) validateToken(ctx context.Context, token string) (bool, string, error) {
	// Create TokenReview request
	tokenReview := &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{
			Token: token,
		},
	}

	// Call TokenReview API
	result, err := a.clientset.AuthenticationV1().TokenReviews().Create(ctx, tokenReview, metav1.CreateOptions{})
	if err != nil {
		return false, "", fmt.Errorf("TokenReview API call failed: %w", err)
	}

	// Check authentication status
	if !result.Status.Authenticated {
		return false, "", nil
	}

	username := result.Status.User.Username
	return true, username, nil
}

// ExtractServiceAccount extracts namespace and service account name from username
// Expected format: system:serviceaccount:<namespace>:<sa-name>
func ExtractServiceAccount(username string) (namespace string, saName string) {
	prefix := "system:serviceaccount:"
	if !strings.HasPrefix(username, prefix) {
		return "", ""
	}

	parts := strings.Split(strings.TrimPrefix(username, prefix), ":")
	if len(parts) != 2 {
		return "", ""
	}

	return parts[0], parts[1]
}

