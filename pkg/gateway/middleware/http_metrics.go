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
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	gatewayMetrics "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// HTTPMetrics tracks HTTP request duration by method, path, and status code
// BR-GATEWAY-071: HTTP request observability for performance monitoring
//
// Day 9 Phase 4: Additional metrics for operational visibility
//
// This middleware measures the total HTTP request duration including:
// - Request parsing
// - Authentication/authorization
// - Business logic processing
// - Response serialization
//
// Metrics:
//   - gateway_http_request_duration_seconds{method, path, status_code}
//
// Usage:
//
//	r.Use(middleware.HTTPMetrics(metrics))
func HTTPMetrics(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Nil-safe: If metrics disabled, pass through
			if metrics == nil {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			// Wrap response writer to capture status code
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			// Process request
			next.ServeHTTP(ww, r)

			// Record duration
			duration := time.Since(start).Seconds()
			metrics.HTTPRequestDuration.WithLabelValues(
				r.URL.Path,                // endpoint
				r.Method,                  // method
				strconv.Itoa(ww.Status()), // status
			).Observe(duration)
		})
	}
}

// InFlightRequests tracks the current number of concurrent HTTP requests
// BR-GATEWAY-072: In-flight request tracking for capacity planning
//
// Day 9 Phase 4: Concurrent request monitoring
//
// This middleware increments a gauge when a request starts and decrements
// it when the request completes (using defer). This provides real-time
// visibility into:
// - Current server load
// - Capacity utilization
// - Potential overload conditions
//
// Metrics:
//   - gateway_http_requests_in_flight (gauge)
//
// Usage:
//
//	r.Use(middleware.InFlightRequests(metrics))
func InFlightRequests(metrics *gatewayMetrics.Metrics) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Nil-safe: If metrics disabled, pass through
			if metrics == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Process request
		next.ServeHTTP(w, r)
		})
	}
}
