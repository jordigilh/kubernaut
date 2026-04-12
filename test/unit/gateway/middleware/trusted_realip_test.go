/*
Copyright 2025 Jordi Gil.

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

package middleware

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gwmiddleware "github.com/jordigilh/kubernaut/pkg/gateway/middleware"
)

// BR-GATEWAY-102: Trusted proxy middleware -- validates that X-Forwarded-For and related
// headers are only honoured when the immediate connection is from a trusted CIDR.
// DD-AUTH-003: Design pattern for isFromTrustedProxy.
// Issue #673 L-1: RealIP trusted proxy configuration.
var _ = Describe("Issue #673 L-1: TrustedRealIP Middleware", func() {

	Context("Trusted proxy in CIDRs list", func() {
		It("[UT-GW-673-021] should use X-Forwarded-For IP when request is from trusted proxy", func() {
			// Given: middleware configured with trusted CIDR 127.0.0.1/32
			mw := gwmiddleware.TrustedRealIP([]string{"127.0.0.1/32"})

			var capturedRemoteAddr string
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedRemoteAddr = r.RemoteAddr
			})

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.RemoteAddr = "127.0.0.1:12345"
			req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.1")

			rec := httptest.NewRecorder()
			mw(inner).ServeHTTP(rec, req)

			// Then: RemoteAddr is set to the leftmost XFF IP (client IP)
			Expect(capturedRemoteAddr).To(Equal("203.0.113.50"))
		})

		It("[UT-GW-673-022] should prefer X-Real-IP over X-Forwarded-For when both present", func() {
			mw := gwmiddleware.TrustedRealIP([]string{"127.0.0.1/32"})

			var capturedRemoteAddr string
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedRemoteAddr = r.RemoteAddr
			})

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.RemoteAddr = "127.0.0.1:12345"
			req.Header.Set("X-Real-IP", "198.51.100.10")
			req.Header.Set("X-Forwarded-For", "203.0.113.50")

			rec := httptest.NewRecorder()
			mw(inner).ServeHTTP(rec, req)

			Expect(capturedRemoteAddr).To(Equal("198.51.100.10"))
		})

		It("[UT-GW-673-023] should honour True-Client-IP from trusted proxy", func() {
			mw := gwmiddleware.TrustedRealIP([]string{"10.0.0.0/8"})

			var capturedRemoteAddr string
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedRemoteAddr = r.RemoteAddr
			})

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.RemoteAddr = "10.128.0.5:54321"
			req.Header.Set("True-Client-IP", "192.0.2.1")

			rec := httptest.NewRecorder()
			mw(inner).ServeHTTP(rec, req)

			Expect(capturedRemoteAddr).To(Equal("192.0.2.1"))
		})
	})

	Context("Untrusted source", func() {
		It("[UT-GW-673-024] should ignore X-Forwarded-For from untrusted source", func() {
			// Given: middleware configured with trusted CIDR 10.0.0.0/8; request from 192.168.1.1
			mw := gwmiddleware.TrustedRealIP([]string{"10.0.0.0/8"})

			var capturedRemoteAddr string
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedRemoteAddr = r.RemoteAddr
			})

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.RemoteAddr = "192.168.1.1:45678"
			req.Header.Set("X-Forwarded-For", "203.0.113.50")

			rec := httptest.NewRecorder()
			mw(inner).ServeHTTP(rec, req)

			// Then: RemoteAddr is NOT changed (stays as connection IP)
			Expect(capturedRemoteAddr).To(Equal("192.168.1.1:45678"))
		})

		It("[UT-GW-673-025] should ignore X-Real-IP from untrusted source", func() {
			mw := gwmiddleware.TrustedRealIP([]string{"10.0.0.0/8"})

			var capturedRemoteAddr string
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedRemoteAddr = r.RemoteAddr
			})

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.RemoteAddr = "192.168.1.1:45678"
			req.Header.Set("X-Real-IP", "203.0.113.50")

			rec := httptest.NewRecorder()
			mw(inner).ServeHTTP(rec, req)

			Expect(capturedRemoteAddr).To(Equal("192.168.1.1:45678"))
		})
	})

	Context("Empty trusted CIDRs (fail-closed)", func() {
		It("[UT-GW-673-026] should always use RemoteAddr when no CIDRs configured", func() {
			// Given: empty CIDRs -- fail-closed by default
			mw := gwmiddleware.TrustedRealIP(nil)

			var capturedRemoteAddr string
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedRemoteAddr = r.RemoteAddr
			})

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.RemoteAddr = "10.0.0.1:8080"
			req.Header.Set("X-Forwarded-For", "203.0.113.50")
			req.Header.Set("X-Real-IP", "198.51.100.10")
			req.Header.Set("True-Client-IP", "192.0.2.1")

			rec := httptest.NewRecorder()
			mw(inner).ServeHTTP(rec, req)

			// Then: RemoteAddr is unchanged
			Expect(capturedRemoteAddr).To(Equal("10.0.0.1:8080"))
		})
	})

	Context("Edge cases", func() {
		It("[UT-GW-673-027] should handle malformed CIDR in config gracefully", func() {
			// Given: one valid CIDR and one malformed -- only valid one is used
			mw := gwmiddleware.TrustedRealIP([]string{"not-a-cidr", "127.0.0.1/32"})

			var capturedRemoteAddr string
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedRemoteAddr = r.RemoteAddr
			})

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.RemoteAddr = "127.0.0.1:12345"
			req.Header.Set("X-Forwarded-For", "203.0.113.50")

			rec := httptest.NewRecorder()
			mw(inner).ServeHTTP(rec, req)

			// Then: trusted proxy match on the valid CIDR should work
			Expect(capturedRemoteAddr).To(Equal("203.0.113.50"))
		})

		It("[UT-GW-673-028] should handle invalid IP in X-Forwarded-For gracefully", func() {
			mw := gwmiddleware.TrustedRealIP([]string{"127.0.0.1/32"})

			var capturedRemoteAddr string
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedRemoteAddr = r.RemoteAddr
			})

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.RemoteAddr = "127.0.0.1:12345"
			req.Header.Set("X-Forwarded-For", "not-an-ip")

			rec := httptest.NewRecorder()
			mw(inner).ServeHTTP(rec, req)

			// Then: RemoteAddr is unchanged (invalid XFF IP is rejected)
			Expect(capturedRemoteAddr).To(Equal("127.0.0.1:12345"))
		})

		It("[UT-GW-673-029] should handle RemoteAddr without port", func() {
			mw := gwmiddleware.TrustedRealIP([]string{"127.0.0.1/32"})

			var capturedRemoteAddr string
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedRemoteAddr = r.RemoteAddr
			})

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.RemoteAddr = "127.0.0.1" // No port
			req.Header.Set("X-Forwarded-For", "203.0.113.50")

			rec := httptest.NewRecorder()
			mw(inner).ServeHTTP(rec, req)

			// Then: Should still work (parse IP directly when no port)
			Expect(capturedRemoteAddr).To(Equal("203.0.113.50"))
		})

		It("[UT-GW-673-030] should support IPv6 trusted proxy CIDR", func() {
			mw := gwmiddleware.TrustedRealIP([]string{"::1/128"})

			var capturedRemoteAddr string
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedRemoteAddr = r.RemoteAddr
			})

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.RemoteAddr = "[::1]:12345"
			req.Header.Set("X-Forwarded-For", "2001:db8::1")

			rec := httptest.NewRecorder()
			mw(inner).ServeHTTP(rec, req)

			Expect(capturedRemoteAddr).To(Equal("2001:db8::1"))
		})
	})
})
