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
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
)

var _ = Describe("Security Audit Correlation ID — #1401", func() {

	Describe("UT-KA-1401-001: Rate-limit audit event has non-empty correlation_id", func() {
		It("should produce a non-empty correlation_id when rate limit triggers", func() {
			spy := &spyAuditStore1401{}
			cfg := kaserver.RateLimitConfig{
				RequestsPerSecond: 1,
				Burst:             1,
				CleanupInterval:   time.Hour,
				MaxAge:            time.Hour,
			}
			rl := kaserver.NewRateLimiter(cfg, nil, kaserver.WithAuditStore(spy, logr.Discard()))
			defer rl.Stop()

			handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req1 := httptest.NewRequest("GET", "/api/v1/incident/analyze", nil)
			req1.RemoteAddr = "10.0.0.99:12345"
			handler.ServeHTTP(httptest.NewRecorder(), req1)

			req2 := httptest.NewRequest("GET", "/api/v1/incident/analyze", nil)
			req2.RemoteAddr = "10.0.0.99:12345"
			handler.ServeHTTP(httptest.NewRecorder(), req2)

			Expect(spy.events).To(HaveLen(1), "exactly one rate-limit event expected")
			evt := spy.events[0]
			Expect(evt.CorrelationID).NotTo(BeEmpty(),
				"AU-12: correlation_id must not be empty for security audit events")
		})
	})

	Describe("UT-KA-1401-002: Auth failure audit event has non-empty correlation_id", func() {
		It("should produce a non-empty correlation_id on 401 response", func() {
			spy := &spyAuditStore1401{}
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			})
			handler := kaserver.AuditAuthMiddleware(inner, spy, logr.Discard())

			req := httptest.NewRequest("GET", "/api/v1/mcp", nil)
			req.RemoteAddr = "10.0.0.5:9999"
			handler.ServeHTTP(httptest.NewRecorder(), req)

			Expect(spy.events).To(HaveLen(1))
			Expect(spy.events[0].CorrelationID).NotTo(BeEmpty(),
				"AU-12: auth failure event must have non-empty correlation_id")
		})
	})

	Describe("UT-KA-1401-003: Auth denied audit event has non-empty correlation_id", func() {
		It("should produce a non-empty correlation_id on 403 response", func() {
			spy := &spyAuditStore1401{}
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			})
			handler := kaserver.AuditAuthMiddleware(inner, spy, logr.Discard())

			req := httptest.NewRequest("GET", "/api/v1/mcp", nil)
			req.RemoteAddr = "10.0.0.6:9999"
			handler.ServeHTTP(httptest.NewRecorder(), req)

			Expect(spy.events).To(HaveLen(1))
			Expect(spy.events[0].CorrelationID).NotTo(BeEmpty(),
				"AU-12: auth denied event must have non-empty correlation_id")
		})
	})

	Describe("UT-KA-1401-004: correlation_id has security- prefix for traceability", func() {
		It("should prefix correlation_id with security-", func() {
			spy := &spyAuditStore1401{}
			cfg := kaserver.RateLimitConfig{
				RequestsPerSecond: 1,
				Burst:             1,
				CleanupInterval:   time.Hour,
				MaxAge:            time.Hour,
			}
			rl := kaserver.NewRateLimiter(cfg, nil, kaserver.WithAuditStore(spy, logr.Discard()))
			defer rl.Stop()

			handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req1 := httptest.NewRequest("GET", "/stream", nil)
			req1.RemoteAddr = "10.0.0.77:12345"
			handler.ServeHTTP(httptest.NewRecorder(), req1)

			req2 := httptest.NewRequest("GET", "/stream", nil)
			req2.RemoteAddr = "10.0.0.77:12345"
			handler.ServeHTTP(httptest.NewRecorder(), req2)

			Expect(spy.events).To(HaveLen(1))
			Expect(spy.events[0].CorrelationID).To(HavePrefix("security-"),
				"AU-3: correlation_id must start with 'security-' for filtering")
		})
	})

	Describe("UT-KA-1401-005: correlation_id suffix is valid UUID v4", func() {
		It("should have a parseable UUID after the security- prefix", func() {
			spy := &spyAuditStore1401{}
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			})
			handler := kaserver.AuditAuthMiddleware(inner, spy, logr.Discard())

			req := httptest.NewRequest("POST", "/api/v1/incident/analyze", nil)
			req.RemoteAddr = "10.0.0.8:9999"
			handler.ServeHTTP(httptest.NewRecorder(), req)

			Expect(spy.events).To(HaveLen(1))
			corrID := spy.events[0].CorrelationID
			Expect(corrID).To(HavePrefix("security-"))

			suffix := strings.TrimPrefix(corrID, "security-")
			_, err := uuid.Parse(suffix)
			Expect(err).NotTo(HaveOccurred(),
				"AU-3: correlation_id suffix must be a valid UUID, got: %s", suffix)
		})
	})
})

type spyAuditStore1401 struct {
	events []*audit.AuditEvent
}

func (s *spyAuditStore1401) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.events = append(s.events, event)
	return nil
}
