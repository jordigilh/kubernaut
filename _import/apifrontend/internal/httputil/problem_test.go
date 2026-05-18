package httputil_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/httputil"
)

func TestHTTPUtilSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTPUtil Suite")
}

var _ = Describe("RFC 7807 Problem Details", func() {
	It("UT-AF-HTTP-001: WriteProblem sets application/problem+json content type", func() {
		rec := httptest.NewRecorder()
		httputil.WriteProblem(rec, 401, "Unauthorized", "Token expired")

		Expect(rec.Code).To(Equal(401))
		Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))
	})

	It("UT-AF-HTTP-002: WriteProblem produces valid JSON with required fields", func() {
		rec := httptest.NewRecorder()
		httputil.WriteProblem(rec, 429, "Rate Limit", "Too many requests")

		var problem httputil.ProblemDetail
		Expect(json.NewDecoder(rec.Body).Decode(&problem)).To(Succeed())
		Expect(problem.Type).To(Equal("about:blank"))
		Expect(problem.Title).To(Equal("Rate Limit"))
		Expect(problem.Status).To(Equal(429))
		Expect(problem.Detail).To(Equal("Too many requests"))
	})

	It("UT-AF-HTTP-003: WriteProblemWithType includes custom type URI and instance", func() {
		rec := httptest.NewRecorder()
		httputil.WriteProblemWithType(rec, 403, "https://kubernaut.io/errors/forbidden",
			"Forbidden", "No access", "/api/v1/pods")

		var problem httputil.ProblemDetail
		Expect(json.NewDecoder(rec.Body).Decode(&problem)).To(Succeed())
		Expect(problem.Type).To(Equal("https://kubernaut.io/errors/forbidden"))
		Expect(problem.Instance).To(Equal("/api/v1/pods"))
	})

	It("UT-AF-HTTP-004: WriteProblem omits empty optional fields", func() {
		rec := httptest.NewRecorder()
		httputil.WriteProblem(rec, 400, "Bad Request", "")

		var raw map[string]interface{}
		Expect(json.NewDecoder(rec.Body).Decode(&raw)).To(Succeed())
		Expect(raw).NotTo(HaveKey("instance"))
	})
})
