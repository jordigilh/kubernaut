//go:build e2e

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("E2E slow SSE debug handler", func() {
	It("UT-AF-E2E-1544-001: flushes headers before the keepalive ticker", func() {
		server := httptest.NewServer(http.HandlerFunc(handleSlowSSE))
		defer server.Close()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
		Expect(err).NotTo(HaveOccurred())

		resp, err := server.Client().Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(Equal("text/event-stream"))
		Expect(resp.Body.Close()).To(Succeed())
	})
})
