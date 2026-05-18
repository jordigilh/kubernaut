package requestid_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/requestid"
)

func TestRequestIDSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RequestID Suite")
}

var _ = Describe("X-Request-ID Middleware", func() {
	It("UT-AF-RID-001: generates a UUID when no X-Request-ID header is present", func() {
		var capturedID string
		handler := requestid.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			capturedID = requestid.FromContext(r.Context())
		}))

		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		Expect(capturedID).NotTo(BeEmpty())
		Expect(rec.Header().Get("X-Request-ID")).To(Equal(capturedID))
	})

	It("UT-AF-RID-002: accepts a valid client-provided X-Request-ID", func() {
		var capturedID string
		handler := requestid.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			capturedID = requestid.FromContext(r.Context())
		}))

		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("X-Request-ID", "abc-123-def")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		Expect(capturedID).To(Equal("abc-123-def"))
		Expect(rec.Header().Get("X-Request-ID")).To(Equal("abc-123-def"))
	})

	It("UT-AF-RID-003: rejects X-Request-ID with control characters and generates fresh UUID", func() {
		var capturedID string
		handler := requestid.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			capturedID = requestid.FromContext(r.Context())
		}))

		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("X-Request-ID", "<script>alert(1)</script>")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		Expect(capturedID).NotTo(Equal("<script>alert(1)</script>"))
		Expect(capturedID).NotTo(BeEmpty())
	})

	It("UT-AF-RID-004: rejects oversized X-Request-ID values", func() {
		var capturedID string
		handler := requestid.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			capturedID = requestid.FromContext(r.Context())
		}))

		longID := ""
		for i := 0; i < 200; i++ {
			longID += "a"
		}
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("X-Request-ID", longID)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		Expect(capturedID).NotTo(Equal(longID))
		Expect(len(capturedID)).To(BeNumerically("<=", 128))
	})

	It("UT-AF-RID-005: FromContext returns empty string when no ID in context", func() {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		Expect(requestid.FromContext(req.Context())).To(BeEmpty())
	})
})
