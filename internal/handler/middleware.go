package handler

import (
	"net/http"
	"strconv"
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

func metricsMiddleware(reg *metrics.Registry, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(rec.status)
		path := r.URL.Path

		reg.HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
		reg.HTTPRequestDuration.WithLabelValues(r.Method, path, status).Observe(duration)
	})
}
