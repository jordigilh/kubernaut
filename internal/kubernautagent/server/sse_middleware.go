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

import "net/http"

// SSEHeadersMiddleware sets proxy anti-buffering headers required for
// Server-Sent Events to work through reverse proxies (nginx, envoy).
// Matches the pattern in conversation/sse.go (SSEContentType et al.).
func SSEHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		next.ServeHTTP(w, r)
	})
}

// AutoFlushWriter wraps an http.ResponseWriter to call Flush() after
// each Write(), ensuring SSE events are delivered immediately to the
// client rather than being buffered by Go's HTTP server.
type AutoFlushWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewAutoFlushWriter creates an AutoFlushWriter. If the underlying writer
// supports http.Flusher (most do), Flush() is called after every Write().
func NewAutoFlushWriter(w http.ResponseWriter) *AutoFlushWriter {
	f, _ := w.(http.Flusher)
	return &AutoFlushWriter{w: w, flusher: f}
}

// Write delegates to the underlying ResponseWriter and flushes immediately.
func (fw *AutoFlushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if err == nil && fw.flusher != nil {
		fw.flusher.Flush()
	}
	return n, err
}
