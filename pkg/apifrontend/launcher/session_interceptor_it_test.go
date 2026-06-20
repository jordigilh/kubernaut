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
		registry = launcher.NewActiveContextRegistry(2*time.Hour, 10*time.Minute)
		logBuf = &syncBuffer{}
		logger = funcr.New(func(prefix, args string) {
			_, _ = logBuf.WriteString(prefix + " " + args + "\n")
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

	It("IT-AF-SESS-020-002: SessionInterceptor overrides empty ContextID through real dispatch (SC-7)", func() {
		registry.Set("alice", "ctx-active-investigation")
		h := buildHandler()

		rec := sendMessage(h, "alice", "", "explore remediation options")

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(logBuf.String()).To(ContainSubstring("overriding context_id"),
			"Log must record the context_id override")
		Expect(logBuf.String()).To(ContainSubstring("ctx-active-investigation"),
			"Log must record the target override context_id")
	})

	It("IT-AF-SESS-020-005: Empty contextId follows active session after investigate (SC-7)", func() {
		h := buildHandler()

		// First message establishes the session with explicit context "ctx-first"
		rec1 := sendMessage(h, "bob", "ctx-first", "hello")
		Expect(rec1.Code).To(Equal(http.StatusOK))

		// Simulate what phase_guard does after investigate success:
		// store the active context for "bob"
		registry.Set("bob", "ctx-first")

		// Second message arrives with EMPTY context_id — should be routed
		// to the active investigation
		rec2 := sendMessage(h, "bob", "", "explore remediation options")
		Expect(rec2.Code).To(Equal(http.StatusOK))

		// Verify the interceptor overrode the empty context_id
		Expect(logBuf.String()).To(ContainSubstring("overriding context_id"),
			"Log must record that context_id was overridden")
		Expect(logBuf.String()).To(ContainSubstring("ctx-first"),
			"Log must record target context_id (the active investigation)")

		// Verify both responses are valid JSON-RPC
		var resp1, resp2 map[string]any
		Expect(json.Unmarshal(rec1.Body.Bytes(), &resp1)).To(Succeed())
		Expect(json.Unmarshal(rec2.Body.Bytes(), &resp2)).To(Succeed())
		Expect(resp1).To(HaveKey("result"))
		Expect(resp2).To(HaveKey("result"))
	})

	It("IT-AF-1446-004: SC-7 — Message with empty contextId is NOT redirected when registry entry is idle-expired (#1446)", func() {
		shortIdleRegistry := launcher.NewActiveContextRegistry(2*time.Hour, 1*time.Millisecond)
		shortIdleRegistry.Set("diana", "ctx-stale-investigation")

		time.Sleep(5 * time.Millisecond)

		interceptor := launcher.NewSessionInterceptor(shortIdleRegistry, logger)
		h, err := launcher.NewA2AHandler(launcher.A2AConfig{
			Agent:              rootAgent,
			SessionService:     sessionSvc,
			AppName:            "kubernaut-apifrontend",
			Logger:             logger,
			SessionInterceptor: interceptor,
		})
		Expect(err).NotTo(HaveOccurred())

		rec := sendMessage(h, "diana", "", "list active alerts")
		Expect(rec.Code).To(Equal(http.StatusOK))

		Expect(logBuf.String()).NotTo(ContainSubstring("overriding context_id"),
			"SC-7: idle-expired sessions must NOT hijack new conversations through the full dispatch chain")
		Expect(logBuf.String()).To(ContainSubstring("clearing stale context"),
			"SC-7: interceptor must log stale entry clearing for audit trail")

		_, ok := shortIdleRegistry.Get("diana")
		Expect(ok).To(BeFalse(),
			"SC-7: stale registry entry must be evicted after idle-expired redirect attempt")
	})

	It("IT-AF-1446-005: SC-7, AC-2 — Active session stays alive via Refresh through tool call sequence (#1446)", func() {
		shortIdleRegistry := launcher.NewActiveContextRegistry(2*time.Hour, 50*time.Millisecond)
		shortIdleRegistry.Set("edgar", "ctx-active-investigation")

		interceptor := launcher.NewSessionInterceptor(shortIdleRegistry, logger)
		h, err := launcher.NewA2AHandler(launcher.A2AConfig{
			Agent:              rootAgent,
			SessionService:     sessionSvc,
			AppName:            "kubernaut-apifrontend",
			Logger:             logger,
			SessionInterceptor: interceptor,
		})
		Expect(err).NotTo(HaveOccurred())

		time.Sleep(30 * time.Millisecond)
		shortIdleRegistry.Refresh("edgar")
		time.Sleep(30 * time.Millisecond)

		rec := sendMessage(h, "edgar", "", "show me the RCA")
		Expect(rec.Code).To(Equal(http.StatusOK))

		Expect(logBuf.String()).To(ContainSubstring("overriding context_id"),
			"SC-7, AC-2: active sessions must maintain multi-turn continuity — boundary is intentional")
	})

	It("IT-AF-SESS-020-006: Explicit contextId is preserved even when active context exists (SC-7)", func() {
		registry.Set("carol", "ctx-active-investigation")
		h := buildHandler()

		rec := sendMessage(h, "carol", "ctx-explicit-new", "start a new conversation")
		Expect(rec.Code).To(Equal(http.StatusOK))

		Expect(logBuf.String()).NotTo(ContainSubstring("overriding context_id"),
			"Explicit ContextID must not trigger override")

		var resp map[string]any
		Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp).To(HaveKey("result"))
	})
})

var _ = Describe("SessionInterceptor stale context validation Integration (BR-SESS-025, #1472)", func() {
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
			Instruction: "Test agent for stale session validation IT.",
			SkipTools:   false,
		})
		Expect(err).NotTo(HaveOccurred())
		sessionSvc = adksession.InMemoryService()
		registry = launcher.NewActiveContextRegistry(2*time.Hour, 10*time.Minute)
		logBuf = &syncBuffer{}
		logger = funcr.New(func(prefix, args string) {
			_, _ = logBuf.WriteString(prefix + " " + args + "\n")
		}, funcr.Options{})
	})

	buildHandlerWithValidator := func(validator launcher.StaleSessionValidator) http.Handler {
		var opts []launcher.SessionInterceptorOption
		if validator != nil {
			opts = append(opts, launcher.WithStaleSessionValidator(validator))
		}
		interceptor := launcher.NewSessionInterceptor(registry, logger, opts...)
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

	It("IT-AF-1472-001: Stale context cleared through full interceptor stack (SC-7, SI-10)", func() {
		validator := launcher.NewInMemorySessionValidator(sessionSvc, "kubernaut-apifrontend", logger)
		h := buildHandlerWithValidator(validator)

		rec := sendMessage(h, "alice", "stale-ctx-from-previous-pod", "hello")
		Expect(rec.Code).To(Equal(http.StatusOK))

		Expect(logBuf.String()).To(ContainSubstring("stale-ctx-from-previous-pod"),
			"SC-7: log must record the stale context_id that was cleared")

		var resp map[string]any
		Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp).To(HaveKey("result"),
			"A valid JSON-RPC response must be returned after stale context is cleared")
	})

	It("IT-AF-1472-002: Active session passes through full interceptor stack", func() {
		validator := launcher.NewInMemorySessionValidator(sessionSvc, "kubernaut-apifrontend", logger)
		h := buildHandlerWithValidator(validator)

		rec1 := sendMessage(h, "bob", "ctx-first-msg", "hello")
		Expect(rec1.Code).To(Equal(http.StatusOK))

		rec2 := sendMessage(h, "bob", "ctx-first-msg", "continue conversation")
		Expect(rec2.Code).To(Equal(http.StatusOK))

		var resp map[string]any
		Expect(json.Unmarshal(rec2.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp).To(HaveKey("result"),
			"Second message with same context_id must succeed (session now exists in memory)")
	})
})
