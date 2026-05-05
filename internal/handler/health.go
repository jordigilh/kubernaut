package handler

import "net/http"

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

func readyzHandler(checker func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if !checker() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}
