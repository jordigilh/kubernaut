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

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// Issue #2694 (BR-SECURITY-016, FedRAMP AU-2): DataStorage's
// registerAPIV1AuthMiddleware must route the shared auth middleware's
// security_event records onto the dedicated unsampled, non-level-gated
// logger derived from the host logger.
//
// The host logger is a real zap core at INFO level writing to an in-memory
// buffer. The level matters for discrimination: if the wiring were missing,
// emitSecurityEvent's fallback would write the Info security_event record
// into this host buffer and the spec would fail. With the wiring in place
// the record goes to the dedicated logger instead. The WARN-gate and
// sampler behavior of the dedicated logger is proven in pkg/log
// (UT-LOG-2694-002/003) and pkg/shared/auth (UT-AUTH-2694-003/004).
// (Registered on the existing Server Shutdown Suite in shutdown_test.go:
// Ginkgo permits one RunSpecs per test binary.)

var _ = Describe("DS auth security_event dedicated logger wiring [Issue #2694]", func() {
	It("IT-DS-2694-001 [BR-SECURITY-016, AU-2]: registerAPIV1AuthMiddleware routes security events off the host logger", func() {
		hostBuf := &syncBufferDS{}
		hostLogger := zapr.NewLogger(zap.New(zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), hostBuf, zapcore.InfoLevel)))

		srv := &Server{
			logger:        hostLogger,
			authenticator: &auth.MockAuthenticator{},
			authorizer:    &auth.MockAuthorizer{},
			authNamespace: "kubernaut-system",
			// openapiValidator stays nil deliberately: auth runs before
			// OpenAPI validation (SEC-H2), so an unauthenticated request is
			// rejected before the validator's middleware is ever invoked.
		}

		r := chi.NewRouter()
		srv.registerAPIV1AuthMiddleware(r)
		// A concrete route is needed: chi only runs the Use-registered auth
		// middleware for matched routes. The handler must never be reached.
		r.Get("/api/v1/audit/events", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/audit/events", nil))
		Expect(recorder.Code).To(Equal(http.StatusUnauthorized))

		var securityEvents int
		for _, line := range strings.Split(strings.TrimSpace(hostBuf.String()), "\n") {
			if line == "" {
				continue
			}
			var entry map[string]any
			Expect(json.Unmarshal([]byte(line), &entry)).To(Succeed())
			if entry["msg"] == "security_event" {
				securityEvents++
			}
		}
		Expect(securityEvents).To(Equal(0),
			"the security_event record must leave the host logger's level-gated path: "+
				"registerAPIV1AuthMiddleware wires the dedicated unsampled security-event logger "+
				"for the base 'get', 'create' and 'update' middlewares")
	})
})

// syncBufferDS is a concurrency-safe zapcore.WriteSyncer.
type syncBufferDS struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBufferDS) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBufferDS) Sync() error { return nil }

func (b *syncBufferDS) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
