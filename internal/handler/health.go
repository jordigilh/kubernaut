package handler

import (
	"net/http"
	"sync/atomic"

	"github.com/jordigilh/kubernaut-apifrontend/internal/httputil"
)

// ReadyChecker is a function that reports whether the service is ready to
// accept traffic. Multiple checkers can be composed with AllReady.
type ReadyChecker func() bool

// AllReady composes multiple checkers into one that requires all to pass.
func AllReady(checkers ...ReadyChecker) ReadyChecker {
	return func() bool {
		for _, c := range checkers {
			if !c() {
				return false
			}
		}
		return true
	}
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// ReadyzHandlerFunc returns a handler that responds 503 when the checker reports
// not ready or when the service is draining (shutting down). This ensures
// load balancers stop sending new traffic during graceful shutdown.
func ReadyzHandlerFunc(checker func() bool, draining *atomic.Bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if draining != nil && draining.Load() {
			httputil.WriteProblem(w, http.StatusServiceUnavailable,
				"Service Unavailable", "service is shutting down")
			return
		}
		if !checker() {
			httputil.WriteProblem(w, http.StatusServiceUnavailable,
				"Service Unavailable", "one or more dependencies are not ready")
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}
