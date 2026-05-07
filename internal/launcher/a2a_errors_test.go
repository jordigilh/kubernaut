package launcher_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/agent"
	adksession "google.golang.org/adk/session"

	agentpkg "github.com/jordigilh/kubernaut-apifrontend/internal/agent"
	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/launcher"
)

var _ = Describe("A2A Error Wrapping (P0 ProdSec)", func() {
	var (
		rootAgent  agent.Agent
		sessionSvc adksession.Service
	)

	BeforeEach(func() {
		var err error
		rootAgent, _, err = agentpkg.NewRootAgent(agentpkg.AgentConfig{
			Instruction: "Test agent for error contract verification.",
			SkipTools:   false,
		})
		Expect(err).NotTo(HaveOccurred())
		sessionSvc = adksession.InMemoryService()
	})

	Describe("AfterExecuteCallback error sanitization", func() {
		It("UT-AF-PR6A-001: audit event Detail does not contain raw error paths", func() {
			var capturedEvents []*audit.Event
			mockAuditor := &capturingEmitter{events: &capturedEvents}

			h, err := launcher.NewA2AHandler(launcher.A2AConfig{
				Agent:          rootAgent,
				SessionService: sessionSvc,
				AppName:        "kubernaut-apifrontend",
				Auditor:        mockAuditor,
				BeforeExecute: func(ctx context.Context) (context.Context, error) {
					return ctx, nil
				},
			})
			Expect(err).NotTo(HaveOccurred())

			body := `{"jsonrpc":"2.0","id":"err-1","method":"message/send","params":{"message":{"messageId":"msg-err-001","role":"user","parts":[{"kind":"text","text":"trigger error"}]}}}`
			req := httptest.NewRequest("POST", "/a2a/invoke", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
				Username: "testuser",
				Groups:   []string{"sre"},
				Issuer:   "test",
				RawToken: "tok",
			})
			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusOK))

			for _, evt := range capturedEvents {
				if evt.Detail != nil {
					errField := evt.Detail["error"]
					Expect(errField).NotTo(ContainSubstring("/Users/"))
					Expect(errField).NotTo(ContainSubstring("goroutine"))
				}
			}
		})

		It("UT-AF-PR6A-002: JSON-RPC error response does not leak internal details", func() {
			h, err := launcher.NewA2AHandler(launcher.A2AConfig{
				Agent:          rootAgent,
				SessionService: sessionSvc,
				AppName:        "kubernaut-apifrontend",
			})
			Expect(err).NotTo(HaveOccurred())

			body := `{"jsonrpc":"2.0","id":"err-2","method":"message/send","params":{"message":{"messageId":"msg-err-002","role":"user","parts":[{"kind":"text","text":"hello"}]}}}`
			req := httptest.NewRequest("POST", "/a2a/invoke", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
				Username: "testuser",
				Groups:   []string{"sre"},
				Issuer:   "test",
				RawToken: "tok",
			})
			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			responseBody := rec.Body.String()
			var jsonResp map[string]interface{}
			if err := json.Unmarshal([]byte(responseBody), &jsonResp); err == nil {
				if errObj, ok := jsonResp["error"].(map[string]interface{}); ok {
					if data, ok := errObj["data"].(map[string]interface{}); ok {
						for _, v := range data {
							if s, ok := v.(string); ok {
								Expect(s).NotTo(ContainSubstring("/Users/"))
								Expect(s).NotTo(ContainSubstring("goroutine"))
								Expect(s).NotTo(MatchRegexp(`https?://[^\s]+`))
							}
						}
					}
				}
			}
		})
	})
})

type capturingEmitter struct {
	events *[]*audit.Event
}

func (c *capturingEmitter) Emit(_ context.Context, evt *audit.Event) {
	*c.events = append(*c.events, evt)
}
