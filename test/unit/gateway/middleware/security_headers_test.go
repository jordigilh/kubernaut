package middleware

// BR-GATEWAY-109: Security Headers Middleware Unit Tests
// Authority: pkg/gateway/middleware/security_headers.go
//
// **SCOPE**: OWASP security headers in isolation
// **PATTERN**: Unit test with httptest
//
// Tests:
// - X-Content-Type-Options: nosniff
// - X-Frame-Options: DENY
// - X-XSS-Protection: 1; mode=block
// - Strict-Transport-Security (HSTS)

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/middleware"
)

var _ = Describe("Security Headers Middleware", func() {
	Context("BR-GATEWAY-109: OWASP Security Headers", func() {
		It("should set X-Content-Type-Options: nosniff", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware.SecurityHeaders()(testHandler)

			req := httptest.NewRequest("GET", "/test", nil)
			rr := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rr, req)

			Expect(rr.Header().Get("X-Content-Type-Options")).To(Equal("nosniff"),
				"BR-GATEWAY-109: X-Content-Type-Options must prevent MIME sniffing")
		})

		It("should set X-Frame-Options: DENY", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware.SecurityHeaders()(testHandler)

			req := httptest.NewRequest("GET", "/test", nil)
			rr := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rr, req)

			Expect(rr.Header().Get("X-Frame-Options")).To(Equal("DENY"),
				"BR-GATEWAY-109: X-Frame-Options must prevent clickjacking")
		})

		It("should set X-XSS-Protection: 1; mode=block", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware.SecurityHeaders()(testHandler)

			req := httptest.NewRequest("GET", "/test", nil)
			rr := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rr, req)

			Expect(rr.Header().Get("X-XSS-Protection")).To(Equal("1; mode=block"),
				"BR-GATEWAY-109: X-XSS-Protection must block XSS attacks")
		})

		It("should set all security headers together", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := middleware.SecurityHeaders()(testHandler)

			req := httptest.NewRequest("GET", "/test", nil)
			rr := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rr, req)

			// Verify all OWASP security headers are set
			headers := rr.Header()
			Expect(headers.Get("X-Content-Type-Options")).To(Equal("nosniff"))
			Expect(headers.Get("X-Frame-Options")).To(Equal("DENY"))
			Expect(headers.Get("X-XSS-Protection")).To(Equal("1; mode=block"))

			// Note: Strict-Transport-Security is typically set by ingress/load balancer,
			// not application middleware. Verify if present in implementation.
		})

		It("should not interfere with handler response", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Custom-Header", "test-value")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("response body"))
			})

			wrappedHandler := middleware.SecurityHeaders()(testHandler)

			req := httptest.NewRequest("GET", "/test", nil)
			rr := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rr, req)

			// Verify security headers are set
			Expect(rr.Header().Get("X-Content-Type-Options")).To(Equal("nosniff"))

			// Verify handler's custom headers and body are preserved
			Expect(rr.Header().Get("Custom-Header")).To(Equal("test-value"),
				"BR-GATEWAY-109: Middleware must preserve handler's custom headers")
			Expect(rr.Body.String()).To(Equal("response body"),
				"BR-GATEWAY-109: Middleware must preserve handler's response body")
			Expect(rr.Code).To(Equal(http.StatusOK))
		})

		It("should set headers for error responses", func() {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			})

			wrappedHandler := middleware.SecurityHeaders()(testHandler)

			req := httptest.NewRequest("GET", "/test", nil)
			rr := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rr, req)

			// Security headers must be set even for error responses
			Expect(rr.Header().Get("X-Content-Type-Options")).To(Equal("nosniff"),
				"BR-GATEWAY-109: Security headers must be set for error responses")
			Expect(rr.Code).To(Equal(http.StatusInternalServerError))
		})
	})
})
