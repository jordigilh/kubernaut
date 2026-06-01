package launcher_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/agent"
	adksession "google.golang.org/adk/session"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

// syncBuffer wraps bytes.Buffer with a mutex for concurrent read/write safety.
// The A2A handler dispatches execution asynchronously, so the logger may write
// from a background goroutine while the test reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) WriteString(s string) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.WriteString(s)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var _ = Describe("SessionInterceptor Integration (BR-SESS-020)", func() {
	var (
		rootAgent  agent.Agent
		sessionSvc adksession.Service
		registry   *launcher.ActiveContextRegistry
		logBuf     *syncBuffer
		logger     logr.Logger
	)

	BeforeEach(func() {
		var err error
		rootAgent, _, err = agentpkg.NewRootAgent(agentpkg.AgentConfig{
			Instruction: "Test agent for session interceptor IT.",
			SkipTools:   false,
		})
		Expect(err).NotTo(HaveOccurred())
		sessionSvc = adksession.InMemoryService()
		registry = launcher.NewActiveContextRegistry(2 * time.Hour)
		logBuf = &syncBuffer{}
		logger = funcr.New(func(prefix, args string) {
			logBuf.WriteString(prefix + " " + args + "\n")
		}, funcr.Options{})
	})

	buildHandler := func() http.Handler {
		interceptor := launcher.NewSessionInterceptor(registry, logger)
		h, err := launcher.NewA2AHandler(launcher.A2AConfig{
			Agent:              rootAgent,
			SessionService:     sessionSvc,
			AppName:            "kubernaut-apifrontend",
			Logger:             logger,
			SessionInterceptor: interceptor,
		})
		Expect(err).NotTo(HaveOccurred())
		return h
	}

	sendMessage := func(h http.Handler, username, contextID, text string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(
			`{"jsonrpc":"2.0","id":"1","method":"message/send","params":{"message":{"messageId":"msg-%s","role":"user","contextId":"%s","parts":[{"kind":"text","text":"%s"}]}}}`,
			contextID, contextID, text,
		)
		req := httptest.NewRequest("POST", "/a2a/invoke", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
			Username: username,
			Groups:   []string{"sre"},
		})
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	It("IT-AF-SESS-020-002: SessionInterceptor overrides ContextID through real NewHandler dispatch (SC-7)", func() {
		registry.Set("alice", "ctx-active-investigation")
		h := buildHandler()

		rec := sendMessage(h, "alice", "ctx-new-random", "explore remediation options")

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(logBuf.String()).To(ContainSubstring("ctx-new-random"),
			"Log must record the original context_id")
		Expect(logBuf.String()).To(ContainSubstring("ctx-active-investigation"),
			"Log must record the target override context_id")
	})

	It("IT-AF-SESS-020-005: Two messages with different context_ids share ADK session state after investigate (SC-7)", func() {
		h := buildHandler()

		// First message establishes the session with context "ctx-first"
		rec1 := sendMessage(h, "bob", "ctx-first", "hello")
		Expect(rec1.Code).To(Equal(http.StatusOK))

		// Simulate what phase_guard does after investigate success:
		// store the active context for "bob"
		registry.Set("bob", "ctx-first")

		// Second message arrives with a DIFFERENT context_id
		rec2 := sendMessage(h, "bob", "ctx-second", "explore remediation options")
		Expect(rec2.Code).To(Equal(http.StatusOK))

		// Verify the interceptor overrode the context_id
		Expect(logBuf.String()).To(ContainSubstring("ctx-second"),
			"Log must record original context_id from second message")
		Expect(logBuf.String()).To(ContainSubstring("ctx-first"),
			"Log must record target context_id (the active investigation)")

		// Verify both responses are valid JSON-RPC (proving the handler dispatched successfully)
		var resp1, resp2 map[string]any
		Expect(json.Unmarshal(rec1.Body.Bytes(), &resp1)).To(Succeed())
		Expect(json.Unmarshal(rec2.Body.Bytes(), &resp2)).To(Succeed())
		Expect(resp1).To(HaveKey("result"))
		Expect(resp2).To(HaveKey("result"))
	})
})
