package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
)

var _ = Describe("backend incarnation contract", func() {
	It("UT-AF-2614-004: accepts only a bounded canonical lowercase UUIDv4", func() {
		Expect(handler.ValidateBackendIncarnation("01234567-89ab-4cde-8123-456789abcdef")).To(Succeed())
	})

	DescribeTable("UT-AF-2614-003: rejects unsafe or non-canonical values",
		func(value string) {
			Expect(handler.ValidateBackendIncarnation(value)).To(MatchError(ContainSubstring("backend incarnation")))
		},
		Entry("empty", ""),
		Entry("whitespace", " 01234567-89ab-4cde-8123-456789abcdef"),
		Entry("uppercase", "01234567-89AB-4CDE-8123-456789ABCDEF"),
		Entry("wrong version", "01234567-89ab-3cde-8123-456789abcdef"),
		Entry("wrong variant", "01234567-89ab-4cde-7123-456789abcdef"),
		Entry("carriage return", "01234567-89ab-4cde-8123-456789abcde\r"),
		Entry("line feed", "01234567-89ab-4cde-8123-456789abcde\n"),
		Entry("unicode", "01234567-89ab-4cde-8123-456789abcdeé"),
		Entry("oversized", "01234567-89ab-4cde-8123-456789abcdef-extra"),
	)

	It("UT-AF-2614-001/002: writes the exact sanitized public JSON shape", func() {
		const incarnation = "01234567-89ab-4cde-8123-456789abcdef"
		router, err := handler.NewRouter(backendContractRouterConfig(incarnation))
		Expect(err).NotTo(HaveOccurred())
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/.well-known/kubernaut-console-contract.json", http.NoBody))

		Expect(recorder.Code).To(Equal(http.StatusOK))
		Expect(recorder.Header().Get("Content-Type")).To(Equal("application/json"))
		Expect(recorder.Header().Get("Cache-Control")).To(Equal("no-store"))
		Expect(recorder.Header()).NotTo(HaveKey("X-Kubernaut-Backend-Incarnation"))
		Expect(recorder.Body.String()).To(Equal(`{"backend_incarnation":"01234567-89ab-4cde-8123-456789abcdef"}`))

		var payload map[string]string
		Expect(json.Unmarshal(recorder.Body.Bytes(), &payload)).To(Succeed())
		Expect(payload).To(HaveLen(1))
		Expect(payload).To(HaveKeyWithValue("backend_incarnation", incarnation))
	})

	It("UT-AF-2614-002: does not serialize request credentials or identity", func() {
		const incarnation = "01234567-89ab-4cde-8123-456789abcdef"
		router, err := handler.NewRouter(backendContractRouterConfig(incarnation))
		Expect(err).NotTo(HaveOccurred())
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/.well-known/kubernaut-console-contract.json", http.NoBody)
		req.Header.Set("Authorization", "Bearer credential-fixture")
		req.Header.Set("Cookie", "identity=identity-fixture")
		router.ServeHTTP(recorder, req)

		body := recorder.Body.String()
		Expect(body).NotTo(ContainSubstring("Bearer"))
		Expect(body).NotTo(ContainSubstring("Authorization"))
		Expect(body).NotTo(ContainSubstring("Cookie"))
		Expect(body).NotTo(ContainSubstring("sre-user@example.com"))
	})
})
