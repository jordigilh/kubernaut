package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
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

const consoleOpenAPIPath = "../../../api/apifrontend/openapi/apifrontend-v1.yaml"

func loadConsoleOpenAPI() *openapi3.T {
	doc, err := openapi3.NewLoader().LoadFromFile(consoleOpenAPIPath)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, doc.Validate(context.Background())).To(Succeed())
	return doc
}

func consoleResponseSchema(doc *openapi3.T, path string, status int) *openapi3.Schema {
	item := doc.Paths.Find(path)
	ExpectWithOffset(1, item).NotTo(BeNil(), "the console path must be declared")
	ExpectWithOffset(1, item.Get).NotTo(BeNil())
	response := item.Get.Responses.Status(status)
	ExpectWithOffset(1, response).NotTo(BeNil(), "the documented status must be declared")
	content := response.Value.Content["application/json"]
	ExpectWithOffset(1, content).NotTo(BeNil())
	return content.Schema.Value
}

func decodeConsoleBody(recorder *httptest.ResponseRecorder) any {
	var body any
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

		schema := consoleResponseSchema(doc, "/api/v1/console/inventory/managed-resources", http.StatusOK)
		body := decodeConsoleBody(recorder)
		Expect(schema.VisitJSON(body)).To(Succeed(), "the served page must match the documented schema")

		// The projection is allowlisted: a field the contract does not declare
		// is a contract violation, not an extension.
		extended, ok := body.(map[string]any)
		Expect(ok).To(BeTrue())
		extended["undocumented_field"] = "leaked"
		Expect(schema.VisitJSON(extended)).NotTo(Succeed())
	})

	It("IT-AF-2584-011 [BR-API-001, FedRAMP SI-10]: the served policy-scope page and error envelope satisfy the versioned schema", func() {
		doc := loadConsoleOpenAPI()
		service, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory:  &consoleInventoryStub{page: successfulPage()},
			CurrentPolicies: &consoleOpenAPICurrentPolicyStub{},
			History:         &consoleOpenAPIHistoryStub{},
			Authorizer:      &consoleAuthorizer{allow: true},
		})
		Expect(err).NotTo(HaveOccurred())
		readHandler := handler.NewConsolePolicyReadHandler(service, logr.Discard())

		scopeRecorder := httptest.NewRecorder()
		scopeReq := httptest.NewRequest(http.MethodGet, "/api/v1/console/policies/scope?cluster_id=local", http.NoBody)
		scopeReq = scopeReq.WithContext(auth.WithUserIdentity(scopeReq.Context(), &auth.UserIdentity{Username: "alice"}))
		readHandler.ServeHTTP(scopeRecorder, scopeReq)
		Expect(scopeRecorder.Code).To(Equal(http.StatusOK))

		scopeSchema := consoleResponseSchema(doc, "/api/v1/console/policies/scope", http.StatusOK)
		Expect(scopeSchema.VisitJSON(decodeConsoleBody(scopeRecorder))).To(Succeed())

		// A malformed request must use the documented problem+json envelope.
		problemRecorder := httptest.NewRecorder()
		problemReq := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local&cluster_id=other", http.NoBody)
		problemReq = problemReq.WithContext(auth.WithUserIdentity(problemReq.Context(), &auth.UserIdentity{Username: "alice"}))
		readHandler.ServeHTTP(problemRecorder, problemReq)
		Expect(problemRecorder.Code).To(Equal(http.StatusBadRequest))

		problemItem := doc.Paths.Find("/api/v1/console/inventory/managed-resources").Get.Responses.Status(http.StatusBadRequest)
		problemSchema := problemItem.Value.Content["application/problem+json"].Schema.Value
		Expect(problemSchema).NotTo(BeNil())
		Expect(problemSchema.VisitJSON(decodeConsoleBody(problemRecorder))).To(Succeed())
	})
})
