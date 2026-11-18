package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/requestid"
)

// consoleOpenAPICurrentPolicyStub supplies realistic digest revisions: the
// projection's revision is a sha256 digest, and the schema enforces that shape.
type consoleOpenAPICurrentPolicyStub struct{}

func (*consoleOpenAPICurrentPolicyStub) ReadCurrentPolicies(context.Context, string) ([]policyread.PolicyMetadata, error) {
	return []policyread.PolicyMetadata{{
		Type:          "aiAnalysis",
		ConfigMapName: "aianalysis-policy",
		ConfigMapKey:  "approval.rego",
		Revision:      "sha256:" + strings.Repeat("a", 64),
		Threshold:     &policyread.Threshold{Value: 0.85, Source: "runtime-config", Kind: "rego-approval"},
	}}, nil
}

// consoleOpenAPIHistoryStub keeps the policy-scope projection complete without
// manufacturing historical records: the schema check covers the envelope and
// the current-policy join.
type consoleOpenAPIHistoryStub struct{}

func (*consoleOpenAPIHistoryStub) ListAIAnalysis(context.Context) ([]aianalysisv1.AIAnalysis, error) {
	return nil, nil
}

// consoleOpenAPISpecPath resolves the committed schema from this file, so the
// spec does not depend on the test's working directory.
func consoleOpenAPISpecPath() string {
	_, file, _, ok := runtime.Caller(0)
	ExpectWithOffset(1, ok).To(BeTrue())
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "api", "apifrontend", "openapi", "apifrontend-v1.yaml")
}

func loadConsoleOpenAPI() *openapi3.T {
	doc, err := openapi3.NewLoader().LoadFromFile(consoleOpenAPISpecPath())
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, doc.Validate(context.Background())).To(Succeed())
	return doc
}

func consoleResponseSchema(doc *openapi3.T, path, mediaType string, status int) *openapi3.Schema {
	item := doc.Paths.Find(path)
	ExpectWithOffset(1, item).NotTo(BeNil(), "the console path must be declared")
	ExpectWithOffset(1, item.Get).NotTo(BeNil())
	response := item.Get.Responses.Status(status)
	ExpectWithOffset(1, response).NotTo(BeNil(), "the documented status must be declared")
	content := response.Value.Content[mediaType]
	ExpectWithOffset(1, content).NotTo(BeNil(), "the documented media type must be declared")
	ExpectWithOffset(1, content.Schema).NotTo(BeNil())
	return content.Schema.Value
}

func decodeConsoleBody(recorder *httptest.ResponseRecorder) map[string]any {
	var body map[string]any
	ExpectWithOffset(1, json.Unmarshal(recorder.Body.Bytes(), &body)).To(Succeed())
	return body
}

var _ = Describe("IT-AF-2584-011: Console read OpenAPI contract", func() {
	It("IT-AF-2584-011 [BR-API-001, FedRAMP SI-10]: the served inventory page satisfies the versioned schema and rejects undocumented fields", func() {
		doc := loadConsoleOpenAPI()
		readHandler := newConsoleReadHandler(&consoleAuthorizer{allow: true}, &consoleInventoryStub{page: successfulPage()})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)
		Expect(recorder.Code).To(Equal(http.StatusOK))

		schema := consoleResponseSchema(doc, "/api/v1/console/inventory/managed-resources", "application/json", http.StatusOK)
		body := decodeConsoleBody(recorder)
		Expect(body).To(HaveKeyWithValue("state", "success"))
		Expect(body).To(HaveKeyWithValue("completeness", "complete"))
		Expect(schema.VisitJSON(body)).To(Succeed(), "the served page must match the documented schema")

		// The projection is allowlisted (`additionalProperties: false`): a field
		// the contract does not declare is a violation, not an extension.
		body["undocumented_field"] = "leaked"
		Expect(schema.VisitJSON(body)).NotTo(Succeed())
	})

	It("IT-AF-2584-011 [BR-API-001, FedRAMP SI-10]: the served policy-scope page and problem envelope carry their documented fields", func() {
		doc := loadConsoleOpenAPI()
		service, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory:  &consoleInventoryStub{page: successfulPage()},
			CurrentPolicies: &consoleOpenAPICurrentPolicyStub{},
			History:         &consoleOpenAPIHistoryStub{},
			Authorizer:      &consoleAuthorizer{allow: true},
		})
		Expect(err).NotTo(HaveOccurred())
		readHandler := requestid.Middleware(handler.NewConsolePolicyReadHandler(service, logr.Discard()))

		scopeRecorder := httptest.NewRecorder()
		scopeReq := httptest.NewRequest(http.MethodGet, "/api/v1/console/policies/scope?cluster_id=local", http.NoBody)
		scopeReq = scopeReq.WithContext(auth.WithUserIdentity(scopeReq.Context(), &auth.UserIdentity{Username: "alice"}))
		readHandler.ServeHTTP(scopeRecorder, scopeReq)
		Expect(scopeRecorder.Code).To(Equal(http.StatusOK))

		scopeBody := decodeConsoleBody(scopeRecorder)
		Expect(scopeBody).To(HaveKeyWithValue("state", "success"))
		scopeSchema := consoleResponseSchema(doc, "/api/v1/console/policies/scope", "application/json", http.StatusOK)
		Expect(scopeSchema.VisitJSON(scopeBody)).To(Succeed())

		// A malformed request must use the documented RFC 9457 envelope. That
		// schema is intentionally extensible (RFC 9457 §3.2), so this asserts the
		// concrete members rather than unknown-member rejection.
		problemRecorder := httptest.NewRecorder()
		problemReq := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local&cluster_id=other", http.NoBody)
		problemReq.Header.Set("X-Request-ID", "req-2584-openapi")
		problemReq = problemReq.WithContext(auth.WithUserIdentity(problemReq.Context(), &auth.UserIdentity{Username: "alice"}))
		readHandler.ServeHTTP(problemRecorder, problemReq)
		Expect(problemRecorder.Code).To(Equal(http.StatusBadRequest))

		problemSchema := consoleResponseSchema(doc, "/api/v1/console/inventory/managed-resources", "application/problem+json", http.StatusBadRequest)
		problemBody := decodeConsoleBody(problemRecorder)
		Expect(problemBody).To(HaveKeyWithValue("type", "about:blank"))
		Expect(problemBody).To(HaveKeyWithValue("title", "Invalid request"))
		Expect(problemBody).To(HaveKeyWithValue("status", float64(http.StatusBadRequest)))
		Expect(problemBody).To(HaveKeyWithValue("detail", "invalid Console read filter"))
		Expect(problemBody).To(HaveKeyWithValue("request_id", "req-2584-openapi"))
		Expect(problemSchema.VisitJSON(problemBody)).To(Succeed())
	})
})
