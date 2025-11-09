package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// RequestIDMiddleware adds a unique request ID to each request
// BR-TOOLSET-039: Request tracing for RFC 7807 errors
//
// This middleware:
// 1. Extracts X-Request-ID header if present
// 2. Generates a new UUID if header is missing
// 3. Adds request ID to response header
// 4. Stores request ID in request context
//
// Reference: Gateway Service request ID middleware pattern
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract or generate request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Add to response header
		w.Header().Set("X-Request-ID", requestID)

		// Add to context
		ctx := context.WithValue(r.Context(), "request_id", requestID)

		// Call next handler with updated context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

