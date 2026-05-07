package requestid_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/requestid"
)

var _ = Describe("requestid.Transport", func() {
	It("sets X-Request-ID header when context has a request ID", func() {
		var capturedHeader string
		backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			capturedHeader = r.Header.Get("X-Request-ID")
		}))
		defer backend.Close()

		transport := &requestid.Transport{Base: http.DefaultTransport}
		client := &http.Client{Transport: transport}

		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		var ctx = req.Context()
		requestid.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			ctx = r.Context()
		})).ServeHTTP(httptest.NewRecorder(), req)

		outReq, err := http.NewRequestWithContext(ctx, http.MethodGet, backend.URL, http.NoBody)
		Expect(err).NotTo(HaveOccurred())

		resp, err := client.Do(outReq)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(capturedHeader).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`))
	})

	It("does NOT set X-Request-ID header when context has no request ID", func() {
		var capturedHeader string
		backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			capturedHeader = r.Header.Get("X-Request-ID")
		}))
		defer backend.Close()

		transport := &requestid.Transport{Base: http.DefaultTransport}
		client := &http.Client{Transport: transport}

		req, err := http.NewRequest(http.MethodGet, backend.URL, http.NoBody)
		Expect(err).NotTo(HaveOccurred())

		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(capturedHeader).To(BeEmpty())
	})

	It("does not mutate the original request", func() {
		backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		defer backend.Close()

		transport := &requestid.Transport{Base: http.DefaultTransport}
		client := &http.Client{Transport: transport}

		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		var ctx = req.Context()
		requestid.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			ctx = r.Context()
		})).ServeHTTP(httptest.NewRecorder(), req)

		outReq, err := http.NewRequestWithContext(ctx, http.MethodGet, backend.URL, http.NoBody)
		Expect(err).NotTo(HaveOccurred())

		originalHeaders := outReq.Header.Clone()

		resp, err := client.Do(outReq)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(originalHeaders.Get("X-Request-ID")).To(BeEmpty())
	})
})
