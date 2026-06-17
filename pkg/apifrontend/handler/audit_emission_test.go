package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
)

type handlerAuditSpy struct {
	mu     sync.Mutex
	events []*audit.Event
}

func (s *handlerAuditSpy) Emit(_ context.Context, event *audit.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *handlerAuditSpy) eventsByType(t audit.EventType) []*audit.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*audit.Event
	for _, e := range s.events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

var _ = Describe("Audit event emission – MCP handler (PR2 wiring)", func() {
	It("UT-AF-1156-061: emits mcp.session_init on MCP session initialization", func() {
		spy := &handlerAuditSpy{}
		h, err := handler.NewMCPHandler(handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.1.0",
			Enabled:       true,
			Auditor:       spy,
		})
		Expect(err).NotTo(HaveOccurred())

		initReq := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{},
				"clientInfo":      map[string]any{"name": "test-client", "version": "1.0"},
			},
		}
		body, _ := json.Marshal(initReq)
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusOK))

		events := spy.eventsByType(audit.EventMCPSessionInit)
		Expect(events).To(HaveLen(1), "expected exactly one mcp.session_init event")
		Expect(events[0].Detail).To(HaveKeyWithValue("protocol_version", "2025-03-26"))
	})

	It("UT-AF-1442-001: emits mcp.session_closed when console MCP session is closed", func() {
		spy := &handlerAuditSpy{}
		h, err := handler.NewMCPHandler(handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.1.0",
			Enabled:       true,
			Auditor:       spy,
		})
		Expect(err).NotTo(HaveOccurred())

		By("initializing an MCP session")
		initReq := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{},
				"clientInfo":      map[string]any{"name": "test-client", "version": "1.0"},
			},
		}
		body, _ := json.Marshal(initReq)
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusOK))

		sessionID := rec.Header().Get("Mcp-Session-Id")
		Expect(sessionID).NotTo(BeEmpty(), "SDK should assign a session ID")

		By("closing the session via DELETE")
		delReq := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
		delReq.Header.Set("Mcp-Session-Id", sessionID)
		delRec := httptest.NewRecorder()
		h.ServeHTTP(delRec, delReq)
		Expect(delRec.Code).To(SatisfyAny(
			Equal(http.StatusOK),
			Equal(http.StatusNoContent),
			Equal(http.StatusAccepted),
			Equal(http.StatusMethodNotAllowed),
		))

		By("verifying the mcp.session_closed audit event was emitted")
		closedEvents := spy.eventsByType(audit.EventMCPSessionClosed)
		Expect(closedEvents).To(HaveLen(1),
			"BR-OPS-013: mcp.session_closed audit event must be emitted on session close")
		Expect(closedEvents[0].Detail).To(HaveKeyWithValue("mcp_session_id", sessionID))
		Expect(closedEvents[0].Detail).To(HaveKey("reason"))
	})

	It("UT-AF-1156-065: does NOT emit duplicate mcp.session_init for same session", func() {
		spy := &handlerAuditSpy{}
		h, err := handler.NewMCPHandler(handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.1.0",
			Enabled:       true,
			Auditor:       spy,
		})
		Expect(err).NotTo(HaveOccurred())

		initReq := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{},
				"clientInfo":      map[string]any{"name": "test-client", "version": "1.0"},
			},
		}
		body, _ := json.Marshal(initReq)

		req1 := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(body)))
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("Accept", "application/json, text/event-stream")
		rec1 := httptest.NewRecorder()
		h.ServeHTTP(rec1, req1)

		sessionID := rec1.Header().Get("Mcp-Session-Id")

		req2 := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(body)))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Accept", "application/json, text/event-stream")
		if sessionID != "" {
			req2.Header.Set("Mcp-Session-Id", sessionID)
		}
		rec2 := httptest.NewRecorder()
		h.ServeHTTP(rec2, req2)

		events := spy.eventsByType(audit.EventMCPSessionInit)
		Expect(events).To(HaveLen(1), "second request with same session should NOT emit another mcp.session_init")
	})
})
