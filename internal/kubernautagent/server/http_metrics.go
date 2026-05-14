/*
Copyright 2026 Jordi Gil.

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
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	kametrics "github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
)

// HTTPMetricsMiddleware records request duration and in-flight count.
// DD-3: Excludes /stream endpoints from the histogram (SSE connections are
// long-lived and would skew P99 to minutes).
func HTTPMetricsMiddleware(m *kametrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if m == nil {
				next.ServeHTTP(w, r)
				return
			}

			m.HTTPRequestsInFlight.Inc()
			defer m.HTTPRequestsInFlight.Dec()

			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			if strings.Contains(r.URL.Path, "/stream") {
				return
			}

			duration := time.Since(start).Seconds()
			m.HTTPRequestDurationSeconds.WithLabelValues(
				normalizeEndpoint(r.URL.Path),
				r.Method,
				strconv.Itoa(ww.Status()),
			).Observe(duration)
		})
	}
}

// normalizeEndpoint collapses path parameters to prevent cardinality explosion.
// "/incident/session/abc-123/result" -> "/incident/session/{id}/result"
func normalizeEndpoint(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if i > 0 && parts[i-1] == "session" && part != "" &&
			part != "result" && part != "cancel" && part != "snapshot" && part != "stream" {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}
