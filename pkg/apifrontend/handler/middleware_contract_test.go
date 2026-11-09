package handler_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
)

var _ = Describe("HTTP metric route normalization", func() {
	It("UT-AF-2614-010: gives the public contract a fixed route label", func() {
		router, err := handler.NewRouter(backendContractRouterConfig(testBackendIncarnation))
		Expect(err).NotTo(HaveOccurred())

		contract := httptest.NewRecorder()
		router.ServeHTTP(contract, httptest.NewRequest(http.MethodGet, "/.well-known/kubernaut-console-contract.json", http.NoBody))
		arbitrary := httptest.NewRecorder()
		router.ServeHTTP(arbitrary, httptest.NewRequest(http.MethodGet, "/arbitrary/token-like/path", http.NoBody))
		metrics := httptest.NewRecorder()
		router.ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))

		body := metrics.Body.String()
		Expect(body).To(ContainSubstring(`path="/.well-known/kubernaut-console-contract.json"`))
		Expect(body).To(ContainSubstring(`path="unknown"`))
		Expect(body).NotTo(ContainSubstring(testBackendIncarnation))
	})
})
