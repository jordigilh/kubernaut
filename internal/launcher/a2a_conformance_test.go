package launcher_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/adk/agent"
	adksession "google.golang.org/adk/session"

	agentpkg "github.com/jordigilh/kubernaut-apifrontend/internal/agent"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/launcher"
)

var _ = Describe("A2A Protocol Conformance", func() {
	var (
		rootAgent  agent.Agent
		sessionSvc adksession.Service
		handler    http.Handler
	)

	BeforeEach(func() {
		var err error
		rootAgent, _, err = agentpkg.NewRootAgent(agentpkg.AgentConfig{
			Instruction: "Test agent for A2A conformance.",
			SkipTools:   false,
		})
		Expect(err).NotTo(HaveOccurred())
		sessionSvc = adksession.InMemoryService()

		handler, err = launcher.NewA2AHandler(launcher.A2AConfig{
			Agent:          rootAgent,
			SessionService: sessionSvc,
			AppName:        "kubernaut-apifrontend",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-AF-042-016: POST /a2a/invoke returns JSON-RPC result with id", func() {
		body := `{"jsonrpc":"2.0","id":"conformance-016","method":"message/send","params":{"message":{"messageId":"msg-016","role":"user","parts":[{"kind":"text","text":"hello"}]}}}`
		req := httptest.NewRequest("POST", "/a2a/invoke", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
			Username: "conformance-user",
			Groups:   []string{"sre"},
			Issuer:   "test-issuer",
			RawToken: "test-token",
		})
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))

		var response map[string]interface{}
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		Expect(err).NotTo(HaveOccurred(), "response must be valid JSON")
		Expect(response).To(HaveKey("jsonrpc"))
		Expect(response["jsonrpc"]).To(Equal("2.0"))
		Expect(response).To(HaveKey("id"))
		Expect(response["id"]).To(Equal("conformance-016"))
		Expect(response).To(HaveKey("result"))
	})

	It("UT-AF-042-017: message/send response contains task with id and status", func() {
		body := `{"jsonrpc":"2.0","id":"conformance-017","method":"message/send","params":{"message":{"messageId":"msg-017","role":"user","parts":[{"kind":"text","text":"what pods are failing?"}]}}}`
		req := httptest.NewRequest("POST", "/a2a/invoke", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
			Username: "conformance-user",
			Groups:   []string{"sre"},
			Issuer:   "test-issuer",
			RawToken: "test-token",
		})
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))

		var response map[string]interface{}
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		Expect(err).NotTo(HaveOccurred())

		result, ok := response["result"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "result must be an object")

		Expect(result).To(HaveKey("id"), "task must have an id field")
		Expect(result["id"]).NotTo(BeEmpty(), "task id must be non-empty")

		Expect(result).To(HaveKey("status"), "task must have a status field")
		status, ok := result["status"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "status must be an object")
		Expect(status).To(HaveKey("state"), "status must have a state field")

		validStates := []string{"submitted", "working", "input-required", "completed", "canceled", "failed", "unknown"}
		Expect(validStates).To(ContainElement(status["state"]))
	})
})
