package handler

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut-apifrontend/internal/httputil"
	"github.com/jordigilh/kubernaut-apifrontend/internal/metrics"
)

// RecoverMiddleware returns middleware that recovers from panics in HTTP handlers.
// On panic it logs the stack trace via logr, increments af_http_panics_total,
// and returns HTTP 500 with RFC 7807 problem+json.
func RecoverMiddleware(reg *metrics.Registry, logger logr.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rv := recover(); rv != nil {
					logger.Error(fmt.Errorf("panic: %v", rv),
						"HTTP handler panic recovered",
						"method", r.Method,
						"path", r.URL.Path,
						"stack", string(debug.Stack()))

					if reg != nil && reg.HTTPPanicsTotal != nil {
						reg.HTTPPanicsTotal.Inc()
					}

					httputil.WriteProblem(w, http.StatusInternalServerError,
						"Internal Server Error",
						fmt.Sprintf("unexpected panic: %v", rv))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
