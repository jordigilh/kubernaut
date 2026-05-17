package integration_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
)

var _ = Describe("Graceful Drain — In-process (IT-DRAIN)", func() {

	It("IT-DRAIN-001: /readyz returns 503 when draining flag is set", func() {
		draining := &atomic.Bool{}
		readyz := handler.ReadyzHandlerFunc(func() bool { return true }, draining)

		req := httptest.NewRequest("GET", "/readyz", http.NoBody)
		rec := httptest.NewRecorder()
		readyz.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusOK))

		draining.Store(true)

		req = httptest.NewRequest("GET", "/readyz", http.NoBody)
		rec = httptest.NewRecorder()
		readyz.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusServiceUnavailable),
			"/readyz should return 503 once draining is set (simulates SIGTERM signal handler)")
	})

	It("IT-DRAIN-002: /readyz returns 200 while dependencies are healthy and not draining", func() {
		draining := &atomic.Bool{}
		readyz := handler.ReadyzHandlerFunc(func() bool { return true }, draining)

		req := httptest.NewRequest("GET", "/readyz", http.NoBody)
		rec := httptest.NewRecorder()
		readyz.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(Equal("ok"))
	})
})
