package launcher_test

import (
	"context"
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

var _ = Describe("Launcher", func() {
	var (
		rootAgent  agent.Agent
		sessionSvc adksession.Service
	)

	BeforeEach(func() {
		var err error
		rootAgent, _, err = agentpkg.NewRootAgent(agentpkg.AgentConfig{
			Instruction: "Test agent instruction for unit tests.",
			SkipTools:   false,
		})
		Expect(err).NotTo(HaveOccurred())
		sessionSvc = adksession.InMemoryService()
	})

	Describe("NewA2AHandler", func() {
		It("UT-AF-210-001: returns non-nil http.Handler", func() {
			h, err := launcher.NewA2AHandler(launcher.A2AConfig{
				Agent:          rootAgent,
				SessionService: sessionSvc,
				AppName:        "kubernaut-apifrontend",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(h).NotTo(BeNil())
		})

		It("UT-AF-210-002: returns error when Agent is nil", func() {
			_, err := launcher.NewA2AHandler(launcher.A2AConfig{
				Agent:          nil,
				SessionService: sessionSvc,
				AppName:        "kubernaut-apifrontend",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("agent"))
		})

		It("UT-AF-210-003: returns error when SessionService is nil", func() {
			_, err := launcher.NewA2AHandler(launcher.A2AConfig{
				Agent:          rootAgent,
				SessionService: nil,
				AppName:        "kubernaut-apifrontend",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("session service"))
		})

		It("UT-AF-210-004: handler accepts POST with JSON-RPC body", func() {
			h, err := launcher.NewA2AHandler(launcher.A2AConfig{
				Agent:          rootAgent,
				SessionService: sessionSvc,
				AppName:        "kubernaut-apifrontend",
			})
			Expect(err).NotTo(HaveOccurred())

			body := `{"jsonrpc":"2.0","id":"1","method":"message/send","params":{"message":{"messageId":"msg-001","role":"user","parts":[{"kind":"text","text":"hello"}]}}}`
			req := httptest.NewRequest("POST", "/a2a/invoke", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
				Username: "testuser",
				Groups:   []string{"sre"},
				Issuer:   "test-issuer",
				RawToken: "fake-token",
			})
			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Body.String()).To(ContainSubstring(`"jsonrpc":"2.0"`))
		})

		It("UT-AF-210-005: BeforeExecuteCallback injects user identity into context", func() {
			var capturedUser *auth.UserIdentity
			h, err := launcher.NewA2AHandler(launcher.A2AConfig{
				Agent:          rootAgent,
				SessionService: sessionSvc,
				AppName:        "kubernaut-apifrontend",
				BeforeExecute: func(ctx context.Context) (context.Context, error) {
					capturedUser = auth.UserIdentityFromContext(ctx)
					return ctx, nil
				},
			})
			Expect(err).NotTo(HaveOccurred())

			body := `{"jsonrpc":"2.0","id":"2","method":"message/send","params":{"message":{"messageId":"msg-002","role":"user","parts":[{"kind":"text","text":"test"}]}}}`
			req := httptest.NewRequest("POST", "/a2a/invoke", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
				Username: "sre-engineer",
				Groups:   []string{"sre"},
				Issuer:   "corporate-idp",
				RawToken: "real-jwt",
			})
			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(capturedUser).NotTo(BeNil())
			Expect(capturedUser.Username).To(Equal("sre-engineer"))
		})
	})

	Describe("A2AConfig validation", func() {
		It("UT-AF-210-006: returns error when AppName is empty", func() {
			_, err := launcher.NewA2AHandler(launcher.A2AConfig{
				Agent:          rootAgent,
				SessionService: sessionSvc,
				AppName:        "",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("app name"))
		})
	})

	Describe("AutoCreateSession", func() {
		It("UT-AF-210-007: executor creates session automatically when none exists", func() {
			h, err := launcher.NewA2AHandler(launcher.A2AConfig{
				Agent:          rootAgent,
				SessionService: sessionSvc,
				AppName:        "kubernaut-apifrontend",
			})
			Expect(err).NotTo(HaveOccurred())

			body := `{"jsonrpc":"2.0","id":"3","method":"message/send","params":{"message":{"messageId":"msg-003","role":"user","parts":[{"kind":"text","text":"create session test"}]}}}`
			req := httptest.NewRequest("POST", "/a2a/invoke", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
				Username: "user1",
				Groups:   []string{"viewer"},
				Issuer:   "test",
				RawToken: "tok",
			})
			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})
})
