package handler_test

import (
	"context"
	"encoding/json"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
)

// IT-AF-2584-009 closes the service-account parity facet TP-2584 v1.9
// declared unproven (#2667): the Console routes are proven to reject service
// accounts, but nothing proved the existing A2A/MCP paths still accept one
// with unchanged attribution.
//
// This spec deliberately does not use the pass-through auth stub the other
// router-wiring suites use. It mounts the real production router, the real
// auth middleware in its TokenReview mode (the mode that authenticates
// service-account tokens, pkg/apifrontend/auth/tokenreview.go), the real MCP
// handler/bridge, the real A2A handler, and the real Console read handler, and
// drives every request with the same bearer token. Fixtures are limited to the
// Kubernetes API (TokenReview, dynamic/typed clients) and one deterministic
// test LLM whose only scripted answer is final text, so the ADK task can reach
// a terminal state without executing a tool or contacting a model provider.

const (
	saToken    = "test-token"
	saUsername = "system:serviceaccount:kubernaut-system:automation"
)

var _ = Describe("BR-INTERACTIVE-010: service-account parity across A2A/MCP and the Console routes (#2667)", func() {
	var (
		router         http.Handler
		auditor        *fakeAuditor
		consoleReader  *consoleInventoryStub
		toolAuthorizer *recordingToolAuthorizer
	)

	BeforeEach(func() {
		auditor = &fakeAuditor{}
		consoleReader = &consoleInventoryStub{page: successfulPage()}
		toolAuthorizer = &recordingToolAuthorizer{}

		validator, err := auth.NewJWTValidator(auth.Config{}, auth.WithTokenReviewer(auth.NewTokenReviewer(saTokenReviewClient())))
		Expect(err).NotTo(HaveOccurred())

		mcpHandler, err := handler.NewMCPHandler(handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.1.0-test",
			Enabled:       true,
			Bridge: &handler.MCPBridgeConfig{
				K8sClient:   newFakeDynamicClient(),
				TypedClient: newBridgeTypedClient(),
				Namespace:   "default",
				// The token's groups include system:serviceaccounts, so the real
				// per-tool authorization path admits the service account exactly
				// as it did before the Console routes existed.
				Authorizer:         toolAuthorizer,
				Auditor:            auditor,
				Metrics:            newBridgeMetrics(),
				ToolTimeout:        5 * time.Second,
				MaxConcurrentTools: 5,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		a2aHandler, err := launcher.NewA2AHandler(launcher.A2AConfig{
			Agent:          newServiceAccountParityAgent(),
			SessionService: adksession.InMemoryService(),
			AppName:        "kubernaut-apifrontend",
			Auditor:        auditor,
		})
		Expect(err).NotTo(HaveOccurred())

		router, err = handler.NewRouter(handler.RouterConfig{
			MetricsRegistry:          metrics.NewRegistry(),
			A2AHandler:               a2aHandler,
			MCPHandler:               mcpHandler,
			ConsolePolicyReadHandler: newConsoleReadHandler(&consoleAuthorizer{allow: true}, consoleReader),
			PublicRoutes: handler.PublicRouteConfig{
				AgentCardHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
				BackendContract:  handler.BackendContractConfig{Incarnation: "01234567-89ab-4cde-8123-456789abcdef"},
			},
			AuthMiddleware: auth.MiddlewareWithConfig(auth.MiddlewareConfig{
				Validator: validator,
				Logger:    logr.Discard(),
				Auditor:   auditor,
			}),
			ReadyChecker: func() bool { return true },
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("IT-AF-2584-009 [BR-INTERACTIVE-010, FedRAMP AU-2/AU-3]: keeps accepting a service-account token on MCP with the same actor attribution", func() {
		sessionID := mcpInitializeThroughRouter(router)
		Expect(sessionID).NotTo(BeEmpty(), "the service-account token must complete the authenticated MCP handshake")

		status, body := mcpCallToolThroughRouter(router, sessionID, kubernautListRemediations, map[string]any{})
		Expect(status).To(Equal(http.StatusOK))
		Expect(isErrorResult(body)).To(BeFalse(), "the service account keeps its per-tool authorization")
		Expect(toolAuthorizer.identities).To(HaveLen(1),
			"the tool authorization seam must see the service account, not an empty or substituted identity")
		Expect(toolAuthorizer.identities[0].username).To(Equal(saUsername))
		Expect(toolAuthorizer.identities[0].groups).To(ContainElement("system:serviceaccounts"))

		Expect(eventsOfType(auditor, audit.EventToolExecuted)).To(HaveLen(1))
		event := eventsOfType(auditor, audit.EventToolExecuted)[0]
		Expect(event.UserID).To(Equal(saUsername), "the verified service-account identity must reach the audit trail unchanged")
		Expect(event.Detail["tool_name"]).To(Equal(kubernautListRemediations))

		authEvents := eventsOfType(auditor, audit.EventAuthSuccess)
		Expect(authEvents).NotTo(BeEmpty())
		Expect(authEvents[len(authEvents)-1].UserID).To(Equal(saUsername))
		Expect(authEvents[len(authEvents)-1].Detail["auth_method"]).To(Equal(auth.AuthMethodTokenReview))
	})

	It("IT-AF-2584-009 [BR-INTERACTIVE-010, FedRAMP AU-2/AU-3]: keeps accepting a service-account token on A2A with the same actor attribution", func() {
		recorder := postServiceAccountA2A(router)

		Expect(recorder.Code).To(Equal(http.StatusOK), "A2A must not reject the verified service account")
		var response map[string]any
		Expect(json.Unmarshal(recorder.Body.Bytes(), &response)).To(Succeed())
		result, ok := response["result"].(map[string]any)
		Expect(ok).To(BeTrue(), "the A2A request must reach the real protocol handler")
		status, ok := result["status"].(map[string]any)
		Expect(ok).To(BeTrue(), "message/send must return the terminal task")
		Expect(status["state"]).To(Equal("completed"),
			"the service-account task must reach a terminal completed state")
		taskID, ok := result["id"].(string)
		Expect(ok).To(BeTrue(), "the terminal task must carry an ID")
		Expect(taskID).NotTo(BeEmpty())

		Eventually(func() int {
			return len(eventsOfType(auditor, audit.EventA2ATaskStarted))
		}, 5*time.Second).Should(Equal(1))
		started := eventsOfType(auditor, audit.EventA2ATaskStarted)[0]
		Expect(started.UserID).To(Equal(saUsername), "the verified service-account identity must reach the A2A audit trail unchanged")
		Expect(started.Detail["method"]).To(Equal("message/send"))
		Expect(started.CorrelationID).NotTo(BeEmpty(), "a non-empty correlation is required for the equality below to mean anything")

		Eventually(func() int {
			return len(eventsOfType(auditor, audit.EventA2ATaskCompleted))
		}, 5*time.Second).Should(Equal(1))
		completed := eventsOfType(auditor, audit.EventA2ATaskCompleted)[0]
		Expect(completed.UserID).To(Equal(saUsername),
			"terminal attribution must stay the verified service account")
		Expect(completed.Detail["task_id"]).To(Equal(taskID))
		Expect(completed.CorrelationID).NotTo(BeEmpty())
		Expect(completed.CorrelationID).To(Equal(started.CorrelationID))

		triage := eventsOfType(auditor, audit.EventTriageCompleted)
		Expect(triage).To(HaveLen(1), "the success callback pairs completion with one triage outcome")
		Expect(triage[0].UserID).To(Equal(saUsername))
		Expect(triage[0].Detail["task_id"]).To(Equal(taskID))
		Expect(triage[0].CorrelationID).To(Equal(started.CorrelationID))
		Expect(eventsOfType(auditor, audit.EventA2ATaskFailed)).To(BeEmpty())

		authEvents := eventsOfType(auditor, audit.EventAuthSuccess)
		Expect(authEvents).NotTo(BeEmpty())
		Expect(authEvents[len(authEvents)-1].UserID).To(Equal(saUsername))
		Expect(authEvents[len(authEvents)-1].Detail["auth_method"]).To(Equal(auth.AuthMethodTokenReview))
	})

	It("IT-AF-2584-009 [BR-SECURITY-016, FedRAMP AC-4/AC-6]: still rejects the same service-account token on the Console read route", func() {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/policies/scope?cluster_id=local", http.NoBody)
		req.Header.Set("Authorization", "Bearer "+saToken)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusForbidden))
		Expect(recorder.Body.String()).To(ContainSubstring(`"state":"forbidden"`))
		Expect(recorder.Body.String()).NotTo(ContainSubstring("checkout"))
		Expect(recorder.Body.String()).NotTo(ContainSubstring(`"items"`))
		Expect(consoleReader.listCalls).To(BeZero(),
			"the service account is rejected before any inventory read")

		// The rejection is authorization, not authentication: the token was
		// verified and attributed, then denied.
		authEvents := eventsOfType(auditor, audit.EventAuthSuccess)
		Expect(authEvents).NotTo(BeEmpty())
		Expect(authEvents[len(authEvents)-1].UserID).To(Equal(saUsername))
		Expect(eventsOfType(auditor, audit.EventAuthFailure)).To(BeEmpty())
	})
})

// saTokenReviewClient answers TokenReview for exactly one service-account
// token, matching the production TokenReview authentication path.
func saTokenReviewClient() *k8sfake.Clientset {
	client := k8sfake.NewSimpleClientset()
	client.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review, ok := action.(k8stesting.CreateAction).GetObject().(*authv1.TokenReview)
		if !ok {
			return false, nil, nil
		}
		if review.Spec.Token == saToken {
			review.Status = authv1.TokenReviewStatus{
				Authenticated: true,
				User: authv1.UserInfo{
					Username: saUsername,
					Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:kubernaut-system"},
				},
			}
		} else {
			review.Status = authv1.TokenReviewStatus{Authenticated: false}
		}
		return true, review, nil
	})
	return client
}

// newServiceAccountParityAgent builds the real A2A launcher over the ADK root
// agent. Tools are registered because NewRootAgent rejects a tool-less agent;
// the deterministic test LLM never asks to run them.
func newServiceAccountParityAgent() agent.Agent {
	rootAgent, _, err := agentpkg.NewRootAgent(agentpkg.AgentConfig{
		Instruction: "Service-account parity fixture agent.",
		SkipTools:   false,
		LLMModel:    finalTextLLM{},
	})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return rootAgent
}

func postServiceAccountA2A(router http.Handler) *httptest.ResponseRecorder {
	body := `{"jsonrpc":"2.0","id":"sa-parity-1","method":"message/send","params":{"message":{` +
		`"messageId":"msg-sa-1","role":"user","parts":[{"kind":"text","text":"hello"}]}}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a/invoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+saToken)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func eventsOfType(auditor *fakeAuditor, eventType audit.EventType) []*audit.Event {
	var matching []*audit.Event
	for _, event := range auditor.Events() {
		if event.Type == eventType {
			matching = append(matching, event)
		}
	}
	return matching
}

type authorizedIdentity struct {
	username string
	groups   []string
}

// finalTextLLM is the deterministic external LLM fixture: it answers with one
// final text turn and never requests a tool, so the ADK loop terminates.
type finalTextLLM struct{}

func (finalTextLLM) Name() string { return "sa-parity-final-text" }

func (finalTextLLM) GenerateContent(context.Context, *model.LLMRequest, bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{
			Content:      genai.NewContentFromText("service-account parity", genai.RoleModel),
			TurnComplete: true,
		}, nil)
	}
}

// recordingToolAuthorizer records the identity that reaches the real per-tool
// authorization seam and allows it, standing in for the SAR checker (an
// external dependency).
type recordingToolAuthorizer struct {
	identities []authorizedIdentity
}

func (a *recordingToolAuthorizer) Check(_ context.Context, user string, groups []string, _ string) (bool, error) {
	a.identities = append(a.identities, authorizedIdentity{username: user, groups: groups})
	return true, nil
}
