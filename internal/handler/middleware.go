package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut-apifrontend/internal/metrics"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) { //nolint:revive // implements http.ResponseWriter
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher, required for MCP SSE streaming through
// the metrics middleware wrapper.
func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// normalizePath maps request paths to a fixed set of known route labels,
// preventing unbounded Prometheus cardinality from arbitrary request paths.
func normalizePath(p string) string {
	switch {
	case p == "/healthz", p == "/readyz", p == "/metrics":
		return p
	case strings.HasPrefix(p, "/a2a/"):
		return "/a2a/invoke"
	case p == "/mcp":
		return "/mcp"
	case p == "/.well-known/agent-card.json":
		return "/.well-known/agent-card.json"
	default:
		return "unknown"
	}
}

func metricsMiddleware(reg *metrics.Registry, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(rec.status)
		path := normalizePath(r.URL.Path)

		reg.HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
		reg.HTTPRequestDuration.WithLabelValues(r.Method, path, status).Observe(duration)
	})
}

// securityHeadersMiddleware adds defense-in-depth HTTP security headers.
// TLS termination is expected at ingress/mesh; these headers provide additional
// protection in case responses reach browsers or non-standard clients.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}
