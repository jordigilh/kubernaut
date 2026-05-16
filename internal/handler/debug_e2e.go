//go:build e2e

package handler

import "net/http"

func init() {
	registerDebugEndpoints = registerE2EDebugEndpoints
}

func registerE2EDebugEndpoints(mux *http.ServeMux) {
	mux.HandleFunc("POST /debug/panic", func(_ http.ResponseWriter, _ *http.Request) {
		panic("e2e-test-trigger")
	})
}
