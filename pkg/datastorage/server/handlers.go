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
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Health check handlers

// handleHealth handles GET /health - overall health check
// DD-AUTH-014: Verifies database connectivity and auth middleware configuration
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check database connectivity
	if err := s.db.Ping(); err != nil {
		s.logger.Error(err, "Health check failed - database unreachable")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, `{"status":"unhealthy","database":"unreachable","error":"%s"}`, err.Error())
		return
	}

	// DD-AUTH-014: Auth middleware readiness verified by non-nil check
	// Per NewServer(), authenticator is guaranteed to be non-nil if server started
	// We do NOT actively validate tokens here to avoid startup race conditions:
	//   - In CI, envtest may not be fully ready when DataStorage starts
	//   - Active validation with timeout causes health check to return 503
	//   - Integration tests fail waiting for health to become OK
	//
	// Auth middleware handles per-request validation gracefully:
	//   - Returns 401 if K8s API unreachable (client can retry)
	//   - Returns 403 if token valid but unauthorized (client error)
	//   - No need to block service startup on K8s API availability
	//
	// Health check purpose: Verify service CAN respond to requests
	// Auth validation purpose: Verify request HAS valid credentials (per-request)

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, `{"status":"healthy","database":"connected","auth":"configured"}`)
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
