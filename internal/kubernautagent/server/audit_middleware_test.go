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
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
)

var _ = Describe("AuditAuthMiddleware — SSE Flusher support", func() {

	Describe("UT-KA-SSE-FLUSH-001: statusRecorder preserves http.Flusher for SSE streams", func() {
		It("allows http.NewResponseController to flush through the audit wrapper [UT-KA-SSE-FLUSH-001]", func() {
			var flushErr atomic.Value
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				rc := http.NewResponseController(w)
				if err := rc.Flush(); err != nil {
					flushErr.Store(err)
				}
			})

			store := &noopAuditStore{}
			handler := kaserver.AuditAuthMiddleware(inner, store, logr.Discard())

			srv := httptest.NewServer(handler)
			defer srv.Close()

			resp, err := http.Get(srv.URL + "/api/v1/mcp")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(flushErr.Load()).To(BeNil(), "Flush must succeed through audit middleware wrapper")
		})
	})

	Describe("UT-KA-SSE-FLUSH-002: statusRecorder implements Unwrap for ResponseController", func() {
		It("exposes Unwrap() so http.NewResponseController can access the underlying writer [UT-KA-SSE-FLUSH-002]", func() {
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				type unwrapper interface {
					Unwrap() http.ResponseWriter
				}
				_, ok := w.(unwrapper)
				Expect(ok).To(BeTrue(), "statusRecorder must implement Unwrap() http.ResponseWriter")
				w.WriteHeader(http.StatusOK)
			})

			store := &noopAuditStore{}
			handler := kaserver.AuditAuthMiddleware(inner, store, logr.Discard())

			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})
})

type noopAuditStore struct{}

func (s *noopAuditStore) StoreAudit(_ context.Context, _ *audit.AuditEvent) error { return nil }
