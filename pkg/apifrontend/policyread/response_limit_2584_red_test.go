package policyread_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
)

var _ = Describe("BR-SAFETY-001: HTTPS response-body containment", func() {
	It("UT-AF-2584-HTTPS-BODY-001: fails before retaining an oversized HTTPS response body", func() {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			flusher, ok := w.(http.Flusher)
			Expect(ok).To(BeTrue())
			for range 4 {
				_, err := io.WriteString(w, strings.Repeat("x", 32))
				Expect(err).NotTo(HaveOccurred())
				flusher.Flush()
			}
		}))
		DeferCleanup(server.Close)

		client := server.Client()
		client.Transport = policyread.NewResponseLimitedTransport(client.Transport, 64)
		response, err := client.Get(server.URL)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(response.Body.Close)

		_, err = io.ReadAll(response.Body)
		Expect(errors.Is(err, policyread.ErrResponseBodyLimitExceeded)).To(BeTrue())
	})
})
