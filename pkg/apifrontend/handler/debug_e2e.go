//go:build e2e

package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/streaming"
)

func init() {
	registerDebugEndpoints = registerE2EDebugEndpoints
}

func registerE2EDebugEndpoints(mux *http.ServeMux, tracker *streaming.ConnectionTracker) {
	mux.HandleFunc("POST /debug/panic", func(_ http.ResponseWriter, _ *http.Request) {
		panic("e2e-test-trigger")
	})

	// /debug/slow-sse holds an SSE connection open (periodic keepalive
	// comments) until the client disconnects, without invoking the agent/LLM
	// pipeline at all. Wired through trackSSEConnection so it occupies a real
	// ConnectionTracker slot and is subject to the identical cap-enforcement
	// (503 "too many concurrent connections") as /a2a/invoke, /mcp, and
	// /a2a/status — letting TC-E2E-SSE-CAP-01 smoke-test the deployed cap
	// without competing with sibling specs for the separate, scarcer
	// LLM-concurrency semaphore (issue #1544).
	mux.Handle("POST /debug/slow-sse", trackSSEConnection(tracker, http.HandlerFunc(handleSlowSSE)))
}

func handleSlowSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, canFlush := w.(http.Flusher)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		}
	}
}
