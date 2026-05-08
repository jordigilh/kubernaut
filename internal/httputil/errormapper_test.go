package httputil_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/httputil"
)

var _ = Describe("ErrorMapper", func() {
	Describe("MapToRFC7807", func() {
		It("maps authentication errors to 401", func() {
			err := fmt.Errorf("token expired: %w", httputil.ErrAuthentication)
			req := httptest.NewRequest(http.MethodGet, "/api/test", http.NoBody)
			p := httputil.MapToRFC7807(err, req)
			Expect(p.Status).To(Equal(http.StatusUnauthorized))
			Expect(p.Type).To(Equal(httputil.ProblemAuthenticationFailed))
			Expect(p.Instance).To(Equal("/api/test"))
		})

		It("maps permission denied errors to 403", func() {
			err := fmt.Errorf("no access: %w", httputil.ErrPermissionDenied)
			p := httputil.MapToRFC7807(err, nil)
			Expect(p.Status).To(Equal(http.StatusForbidden))
			Expect(p.Type).To(Equal(httputil.ProblemPermissionDenied))
		})

		It("maps rate limit errors to 429", func() {
			err := fmt.Errorf("too many requests: %w", httputil.ErrRateLimited)
			p := httputil.MapToRFC7807(err, nil)
			Expect(p.Status).To(Equal(http.StatusTooManyRequests))
		})

		It("maps upstream timeout to 504", func() {
			err := fmt.Errorf("KA timed out: %w", httputil.ErrUpstreamTimeout)
			p := httputil.MapToRFC7807(err, nil)
			Expect(p.Status).To(Equal(http.StatusGatewayTimeout))
		})

		It("maps unavailable to 503", func() {
			err := fmt.Errorf("circuit open: %w", httputil.ErrUnavailable)
			p := httputil.MapToRFC7807(err, nil)
			Expect(p.Status).To(Equal(http.StatusServiceUnavailable))
		})

		It("maps validation to 400", func() {
			err := fmt.Errorf("missing field: %w", httputil.ErrValidation)
			p := httputil.MapToRFC7807(err, nil)
			Expect(p.Status).To(Equal(http.StatusBadRequest))
		})

		It("maps payload too large to 413", func() {
			err := fmt.Errorf("body exceeds 1MB: %w", httputil.ErrPayloadTooLarge)
			p := httputil.MapToRFC7807(err, nil)
			Expect(p.Status).To(Equal(http.StatusRequestEntityTooLarge))
		})

		It("maps unknown errors to 500 with generic message", func() {
			err := fmt.Errorf("something went wrong at /internal/path:42")
			p := httputil.MapToRFC7807(err, nil)
			Expect(p.Status).To(Equal(http.StatusInternalServerError))
			Expect(p.Detail).To(Equal("an unexpected error occurred"))
			Expect(p.Detail).NotTo(ContainSubstring("/internal/path"))
		})

		It("wrapped classified error does not leak outer error string (ARCH-6)", func() {
			err := fmt.Errorf("DB query at https://internal:8443/api/secret: %w", httputil.ErrUnavailable)
			p := httputil.MapToRFC7807(err, nil)
			Expect(p.Status).To(Equal(http.StatusServiceUnavailable))
			Expect(p.Detail).To(Equal("service unavailable"))
			Expect(p.Detail).NotTo(ContainSubstring("internal:8443"))
			Expect(p.Detail).NotTo(ContainSubstring("DB query"))
			Expect(p.Detail).NotTo(ContainSubstring("https://"))
		})
	})

	Describe("WriteError", func() {
		It("writes a well-formed RFC 7807 response", func() {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/a2a/invoke", http.NoBody)
			err := fmt.Errorf("request rejected: %w", httputil.ErrRateLimited)

			httputil.WriteError(w, r, err)

			Expect(w.Code).To(Equal(http.StatusTooManyRequests))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/problem+json"))
		})
	})
})
