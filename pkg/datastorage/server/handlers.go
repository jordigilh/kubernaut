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

package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Health check handlers

// handleHealth handles GET /health - overall health check
// DD-AUTH-014: Now includes auth middleware readiness check
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check database connectivity
	if err := s.db.Ping(); err != nil {
		s.logger.Error(err, "Health check failed - database unreachable")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, `{"status":"unhealthy","database":"unreachable","error":"%s"}`, err.Error())
		return
	}

	// DD-AUTH-014: Check auth middleware readiness (authenticator guaranteed non-nil by NewServer)
	// Use DataStorage's own ServiceAccount token to test auth
	// This ensures envtest/K8s API is reachable and TokenReview is working
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Read DataStorage's ServiceAccount token
	// Production: /var/run/secrets/kubernetes.io/serviceaccount/token
	// Integration tests: May not exist, use dummy token as fallback
	var token string
	if tokenBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		token = string(tokenBytes)
	} else {
		// Fallback for environments without ServiceAccount mount (local dev)
		token = "health-check-test-token"
	}

	// Validate the token to ensure K8s API is reachable
	_, err := s.authenticator.ValidateToken(ctx, token)
	if err != nil {
		// Check if it's a network/connectivity error (auth not ready)
		if isNetworkError(err) {
			s.logger.V(1).Info("Health check failed - auth middleware not ready",
				"error", err.Error())
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, `{"status":"unhealthy","database":"connected","auth":"not_ready","error":"K8s API unreachable"}`)
			return
		}
		// Auth errors (like "Unauthorized") mean K8s API IS working
		// This can happen if DataStorage SA doesn't have TokenReview permissions (misconfiguration)
		// Still consider service healthy - middleware will handle per-request auth
	}

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, `{"status":"healthy","database":"connected","auth":"ready"}`)
}

// handleReadiness handles GET /health/ready - readiness probe for Kubernetes
// DD-007: Returns 503 during shutdown to remove pod from endpoints
func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	// DD-007: Check shutdown flag first
	if s.isShuttingDown.Load() {
		s.logger.V(1).Info("Readiness probe returning 503 - shutdown in progress")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, `{"status":"not_ready","reason":"shutting_down"}`)
		return
	}

	// Check database connectivity
	if err := s.db.Ping(); err != nil {
		s.logger.Info("Readiness probe failed - database unreachable",
			"error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, `{"status":"not_ready","reason":"database_unreachable","error":"%s"}`, err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, `{"status":"ready"}`)
}

// handleLiveness handles GET /health/live - liveness probe for Kubernetes
func (s *Server) handleLiveness(w http.ResponseWriter, r *http.Request) {
	// Liveness is always true unless the process is completely stuck
	// Don't check database here - that's the readiness probe's job
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, `{"status":"alive"}`)
}

// Middleware

// panicRecoveryMiddleware catches panics and logs detailed information
func (s *Server) panicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				requestID := middleware.GetReqID(r.Context())

				// Log the panic with full details
				s.logger.Error(fmt.Errorf("panic: %v", err), "🚨 PANIC RECOVERED",
					"request_id", requestID,
					"method", r.Method,
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr,
					"stack_trace", string(debug.Stack()),
				)

				// Let chi's Recoverer handle the response
				panic(err)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// isNetworkError checks if an error is a network connectivity issue
// Returns true for connection refused, timeouts, DNS failures, etc.
// Returns false for authentication/authorization errors (which mean auth IS working)
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check for context timeout/cancellation
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true // Timeout, temporary errors, etc.
	}

	// Check for connection refused (syscall error)
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, syscall.ECONNREFUSED) {
			return true
		}
		// Other network operation errors (DNS lookup, etc.)
		return true
	}

	// Check for DNS errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// Not a network error - likely an auth error (which means auth IS working)
	return false
}

// loggingMiddleware logs HTTP requests with structured logging
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Get request ID from middleware.RequestID
		requestID := middleware.GetReqID(r.Context())

		// Create a response writer wrapper to capture status code
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// Call next handler
		next.ServeHTTP(ww, r)

		// Log request with timing
		duration := time.Since(start)
		s.logger.Info("HTTP request",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration", duration,
		)
	})
}
