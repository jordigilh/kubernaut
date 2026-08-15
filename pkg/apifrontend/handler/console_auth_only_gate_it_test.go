package handler_test

import (
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
)

// denyAllSAR makes every SubjectAccessReview call return Allowed: false --
// used below to prove auth.ConsoleAuthOnlyGate genuinely bypasses the SAR
// call for console access (rather than merely happening to agree with an
// allowing SAR result), while a real *auth.SARChecker underneath it would
// have denied.
//
//nolint:unparam // action unused but must match k8stesting.ReactionFunc signature required by PrependReactor
func denyAllSAR(_ k8stesting.Action) (bool, runtime.Object, error) {
	return true, &authorizationv1.SubjectAccessReview{
		Status: authorizationv1.SubjectAccessReviewStatus{Allowed: false, Reason: "no RBAC bindings configured"},
	}, nil
}

// IT-AF-2148-001/002 prove auth.ConsoleAuthOnlyGate's wiring through the real
// production dispatch path (handler.NewRouter, handler.NewConsoleAccessHandler,
// handler.NewMCPHandler), not just the gate's own logic in isolation
// (UT-AF-2148-*, pkg/apifrontend/auth). Both wrap a real *auth.SARChecker
// backed by a fake K8s clientset that unconditionally denies, matching the
// #2148 target scenario of an install with no console/RBAC bindings
// configured at all.
var _ = Describe("ConsoleAuthOnlyGate wiring (#2148)", func() {
	var (
		metricsReg *metrics.Registry
		gate       *auth.ConsoleAuthOnlyGate
	)

	BeforeEach(func() {
		metricsReg = metrics.NewRegistry()
		fakeK8s := k8sfake.NewSimpleClientset()
		fakeK8s.PrependReactor("create", "subjectaccessreviews", denyAllSAR)
		checker := auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())
		gate = auth.NewConsoleAuthOnlyGate(checker)
	})

	It("IT-AF-2148-001 [AC-3]: GET /a2a/access returns 200 through the real router even though the underlying SAR would deny", func() {
		consoleAuditor := &fakeAuditor{}
		router, err := handler.NewRouter(handler.RouterConfig{
			MetricsRegistry:      metricsReg,
			A2AHandler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
			MCPHandler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
			AgentCardHandler:     http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
			ConsoleAccessHandler: handler.NewConsoleAccessHandler(gate, consoleAuditor, logr.Discard()),
			AuthMiddleware:       fakeIdentityAuthMiddleware,
			ReadyChecker:         func() bool { return true },
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest(http.MethodGet, "/a2a/access", http.NoBody)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK),
			"#2148: auth-only mode must grant console access through the real router despite a denying SAR")
		Expect(consoleAuditor.Events()).To(BeEmpty(), "no deny event should have been emitted")
	})

	It("IT-AF-2148-002 [AC-3/AC-6]: real POST /mcp tool call still fail-closes on per-tool Check even though the console gate is bypassed", func() {
		fakeK8s := newFakeDynamicClient()
		bridgeAuditor := &fakeAuditor{}
		mcpCfg := handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.1.0-test",
			Enabled:       true,
			Bridge: &handler.MCPBridgeConfig{
				K8sClient:          fakeK8s,
				TypedClient:        newBridgeTypedClient(),
				Namespace:          "default",
				Authorizer:         gate,
				Auditor:            bridgeAuditor,
				Metrics:            newBridgeMetrics(),
				ToolTimeout:        2 * time.Second,
				MaxConcurrentTools: 5,
			},
		}
		mcpHandler, err := handler.NewMCPHandler(mcpCfg)
		Expect(err).NotTo(HaveOccurred())

		router, err := handler.NewRouter(handler.RouterConfig{
			MetricsRegistry:  metricsReg,
			A2AHandler:       http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
			MCPHandler:       mcpHandler,
			AgentCardHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
			AuthMiddleware:   fakeIdentityAuthMiddleware,
			ReadyChecker:     func() bool { return true },
		})
		Expect(err).NotTo(HaveOccurred())

		// The console gate is bypassed (auth-only), so a plain user gets past
		// it -- but the per-tool SAR (denyAllSAR) must still fail-close the
		// actual tool call through the real bridge/router, proving Check is
		// untouched by ConsoleAuthOnlyGate end-to-end, not just when called
		// directly (UT-AF-2148-003).
		sid := mcpInitializeThroughRouter(router)
		status, respBody := mcpCallToolThroughRouter(router, sid, "kubernaut_list_remediations", map[string]any{})
		Expect(status).To(Equal(http.StatusOK), "MCP transport itself succeeds; denial is inside the tool result")
		Expect(isErrorResult(respBody)).To(BeTrue(),
			"#2148: per-tool authorization must remain unconditionally fail-closed even under the console auth-only gate")
	})
})
