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
})
