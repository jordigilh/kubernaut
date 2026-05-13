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
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Health check handlers

// LivenessHandler returns the liveness probe handler for use with the
// dedicated health server (Issue #753: port 8081, /healthz).
func (s *Server) LivenessHandler() http.HandlerFunc {
	return s.handleLiveness
}

// ReadinessHandler returns the readiness probe handler for use with the
// dedicated health server (Issue #753: port 8081, /readyz).
func (s *Server) ReadinessHandler() http.HandlerFunc {
	return s.handleReadiness
}

// handleReadiness handles GET /readyz - readiness probe for Kubernetes (port 8081)
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
		s.logger.Error(err, "Readiness probe failed - database unreachable")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, `{"status":"not_ready","reason":"database_unreachable"}`)
		return
	}

	// #1088 Phase 7.3: Check Redis connectivity
	if s.dlqClient != nil {
		if err := s.dlqClient.HealthCheck(r.Context()); err != nil {
			s.logger.Error(err, "Readiness probe failed - Redis unreachable")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, `{"status":"not_ready","reason":"redis_unreachable"}`)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, `{"status":"ready"}`)
}

// handleLiveness handles GET /healthz - liveness probe for Kubernetes.
// Issue #753 H-3: Standardized response across all stateless services.
func (s *Server) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
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
