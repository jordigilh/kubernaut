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

package server_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
)

var _ = Describe("SSE Middleware — #823 PR4", func() {

	Describe("UT-KA-823-S05: SSE proxy headers set by middleware", func() {
		It("sets Cache-Control, Connection, and X-Accel-Buffering headers", func() {
			handler := kaserver.SSEHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/stream", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			Expect(rec.Header().Get("Cache-Control")).To(Equal("no-cache"))
			Expect(rec.Header().Get("Connection")).To(Equal("keep-alive"))
			Expect(rec.Header().Get("X-Accel-Buffering")).To(Equal("no"))
		})
	})

	Describe("UT-KA-823-S06: Auto-flush middleware flushes after each write", func() {
		It("calls Flush() after each Write()", func() {
			flushCount := 0
			spy := &spyFlusherResponseWriter{
				ResponseWriter: httptest.NewRecorder(),
				onFlush: func() {
					flushCount++
				},
			}

			fw := kaserver.NewAutoFlushWriter(spy)
			_, err := fw.Write([]byte("event 1\n"))
			Expect(err).NotTo(HaveOccurred())
			Expect(flushCount).To(Equal(1), "first write should trigger first flush")

			_, err = fw.Write([]byte("event 2\n"))
			Expect(err).NotTo(HaveOccurred())
			Expect(flushCount).To(Equal(2), "second write should trigger second flush")
		})

		It("does not panic when underlying writer does not implement Flusher", func() {
			rec := httptest.NewRecorder()
			fw := kaserver.NewAutoFlushWriter(rec)
			n, err := fw.Write([]byte("data\n"))
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(5))
		})
	})
})

type spyFlusherResponseWriter struct {
	http.ResponseWriter
	onFlush func()
}

func (s *spyFlusherResponseWriter) Flush() {
	s.onFlush()
}
