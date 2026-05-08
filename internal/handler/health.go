package handler

import (
	"net/http"

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

// readyzHandler returns a handler that responds 503 when the checker reports
// not ready. The response includes "not ready" to match monitoring expectations.
// TODO(PR7+): consider ReadyChecker returning (bool, string) for specific reasons.
func readyzHandler(checker func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if !checker() {
			httputil.WriteProblem(w, http.StatusServiceUnavailable,
				"Service Unavailable", "one or more dependencies are not ready")
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}
