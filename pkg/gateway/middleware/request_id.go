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
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// REQUEST ID MIDDLEWARE (BR-109)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// BUSINESS OUTCOME: Enable operators to trace requests across Gateway components
// using a unique request_id in all log entries.
//
// TDD GREEN: Minimal implementation to make BR-109 tests pass.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// RequestIDKey is the context key for request ID
	RequestIDKey contextKey = "request_id"
	// LoggerKey is the context key for request-scoped logger
	LoggerKey contextKey = "logger"
)

// RequestIDMiddleware adds a unique request ID to each request
//
// BUSINESS OUTCOME: Operators can trace requests across Gateway components
// by searching logs for the same request_id.
//
// TDD GREEN: Minimal implementation:
// 1. Generate UUID for each request
// 2. Store in request context
// 3. Add to response header (X-Request-ID)
// 4. Create request-scoped logger with request_id field
func RequestIDMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Generate unique request ID
			requestID := uuid.New().String()

			// Add request ID to response headers for client tracing
			w.Header().Set("X-Request-ID", requestID)

			// Create request-scoped logger with structured fields
			requestLogger := logger.With(
				zap.String("request_id", requestID),
				zap.String("source_ip", getSourceIP(r)),
				zap.String("endpoint", r.URL.Path),
				zap.String("method", r.Method),
			)

		// Store request ID and logger in context
		ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
		ctx = context.WithValue(ctx, LoggerKey, requestLogger)

		// Log incoming request (debug level for health/readiness checks to reduce noise)
		if r.URL.Path == "/health" || r.URL.Path == "/healthz" || r.URL.Path == "/ready" {
			requestLogger.Debug("Incoming request",
				zap.String("user_agent", r.UserAgent()),
				zap.String("content_type", r.Header.Get("Content-Type")),
			)
		} else {
			requestLogger.Info("Incoming request",
				zap.String("user_agent", r.UserAgent()),
				zap.String("content_type", r.Header.Get("Content-Type")),
			)
		}

		// Pass to next handler
		next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// getSourceIP extracts the source IP from the request
//
// BUSINESS OUTCOME: Enable security auditing by logging webhook sources.
//
// TDD GREEN: Minimal implementation - extract from RemoteAddr.
// REFACTOR: Could enhance to handle X-Forwarded-For, X-Real-IP headers.
func getSourceIP(r *http.Request) string {
	// Check X-Forwarded-For header (proxy/load balancer)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-IP header (nginx)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fallback to RemoteAddr
	return r.RemoteAddr
}

// GetRequestID retrieves the request ID from context
//
// BUSINESS OUTCOME: Enable handlers to access request ID for logging.
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return "unknown"
}

// GetLogger retrieves the request-scoped logger from context
//
// BUSINESS OUTCOME: Enable handlers to use logger with request context.
func GetLogger(ctx context.Context) *zap.Logger {
	if logger, ok := ctx.Value(LoggerKey).(*zap.Logger); ok {
		return logger
	}
	// Fallback to no-op logger if not found
	return zap.NewNop()
}
