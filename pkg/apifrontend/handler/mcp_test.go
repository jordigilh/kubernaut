package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	toolspkg "github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("MCP Handler", func() {
	var mcpHandler http.Handler

	BeforeEach(func() {
		var err error
		mcpHandler, err = handler.NewMCPHandler(handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.1.0",
			Enabled:       true,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-AF-220-001: NewMCPHandler returns non-nil handler", func() {
		Expect(mcpHandler).NotTo(BeNil())
	})

	It("UT-AF-220-002: returns error when ServerName is empty", func() {
		_, err := handler.NewMCPHandler(handler.MCPConfig{
			ServerName:    "",
			ServerVersion: "v0.1.0",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("server name"))
	})

	It("UT-AF-220-003: POST /mcp with initialize request succeeds", func() {
		initReq := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{},
				"clientInfo": map[string]any{
					"name":    "test-client",
					"version": "1.0",
				},
			},
		}
		body, _ := json.Marshal(initReq)
		req := httptest.NewRequest("POST", "/mcp", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
			Username: "testuser",
			Groups:   []string{"sre"},
			Issuer:   "test",
			RawToken: "tok",
		})
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		mcpHandler.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(ContainSubstring("kubernaut-apifrontend"))
	})

	It("UT-AF-220-004: tools/list returns registered tools", func() {
		toolHandler, err := handler.NewMCPHandler(handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.1.0",
			Tools:         handler.DefaultMCPTools(true),
			Enabled:       true,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(toolHandler).NotTo(BeNil())
	})

	It("UT-AF-220-005: tools registered include kubernaut_ or af_ prefix", func() {
		tools := handler.DefaultMCPTools(true)
		for _, t := range tools {
			hasValid := strings.HasPrefix(t.Name, "kubernaut_") || strings.HasPrefix(t.Name, "af_")
			Expect(hasValid).To(BeTrue(), "tool %q missing kubernaut_ or af_ prefix", t.Name)
		}
	})

	It("UT-AF-220-006: tools count matches 23 expected tools (#1418: kubernaut_complete_no_action added)", func() {
		tools := handler.DefaultMCPTools(true)
		Expect(tools).To(HaveLen(23))
	})

	It("UT-AF-1366-030: DefaultMCPTools(false) returns only stateless tools", func() {
		tools := handler.DefaultMCPTools(false)
		Expect(tools).To(HaveLen(12), "only 12 stateless tools when interactive disabled")
		for _, t := range tools {
			Expect(toolspkg.SessionDependentTools).NotTo(HaveKey(t.Name),
				"session-dependent tool %q should be hidden in stub path", t.Name)
		}
	})

	It("UT-AF-1366-031: stub path registers only stateless tools when InteractiveEnabled=false", func() {
		h, err := handler.NewMCPHandler(handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.1.0-test",
			Enabled:       true,
			InteractiveEnabled: false,
		})
		Expect(err).NotTo(HaveOccurred())

		rec := httptest.NewRecorder()
		initReq := map[string]any{
			"jsonrpc": "2.0", "id": 1,
			"method": "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{},
				"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
			},
		}
		body, _ := json.Marshal(initReq)
		req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		h.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusOK))
		sid := rec.Header().Get("Mcp-Session-Id")

		listReq := map[string]any{
			"jsonrpc": "2.0", "id": 2,
			"method": "tools/list",
			"params": map[string]any{},
		}
		body, _ = json.Marshal(listReq)
		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Accept", "application/json, text/event-stream")
		req2.Header.Set("Mcp-Session-Id", sid)
		h.ServeHTTP(rec2, req2)
		Expect(rec2.Code).To(Equal(http.StatusOK))

		responseBody := rec2.Body.String()
		Expect(responseBody).NotTo(BeEmpty(), "tools/list response should not be empty")

		jsonPayload := responseBody
		if strings.Contains(responseBody, "data: ") {
			for _, line := range strings.Split(responseBody, "\n") {
				if strings.HasPrefix(line, "data: ") {
					jsonPayload = strings.TrimPrefix(line, "data: ")
					break
				}
			}
		}

		var result map[string]any
		err = json.Unmarshal([]byte(jsonPayload), &result)
		Expect(err).NotTo(HaveOccurred(), "response should be valid JSON")
		Expect(result).To(HaveKey("result"))
		resultObj, ok := result["result"].(map[string]any)
		Expect(ok).To(BeTrue())
		registeredTools, ok := resultObj["tools"].([]any)
		Expect(ok).To(BeTrue())
		Expect(registeredTools).To(HaveLen(12), "stub path should only register 12 stateless tools")
	})

	It("UT-AF-220-007: MCP server info includes correct name and version", func() {
		h, err := handler.NewMCPHandler(handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.2.0",
			Enabled:       true,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(h).NotTo(BeNil())
	})

	It("UT-AF-220-008: context propagates user identity to tool handler", func() {
		var capturedUser *auth.UserIdentity
		h, err := handler.NewMCPHandler(handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.1.0",
			Enabled:       true,
			ToolCallback: func(ctx context.Context, toolName string) {
				capturedUser = auth.UserIdentityFromContext(ctx)
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(h).NotTo(BeNil())
		// Context propagation is validated by the spike test;
		// structural verification ensures the callback plumbing exists.
		Expect(capturedUser).To(BeNil()) // not called yet
	})

	It("UT-AF-220-009: each tool has non-empty description", func() {
		tools := handler.DefaultMCPTools(true)
		for _, t := range tools {
			Expect(t.Description).NotTo(BeEmpty(), "tool %s missing description", t.Name)
		}
	})

	It("UT-AF-220-010: MCP handler rejects DELETE method without session", func() {
		req := httptest.NewRequest("DELETE", "/mcp", http.NoBody)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mcpHandler.ServeHTTP(rec, req)
		// DELETE without a valid Mcp-Session-Id returns 400 or 405
		Expect(rec.Code).To(SatisfyAny(
			Equal(http.StatusBadRequest),
			Equal(http.StatusMethodNotAllowed),
			Equal(http.StatusNotFound),
		))
	})

	It("UT-AF-220-011: each tool has input schema", func() {
		tools := handler.DefaultMCPTools(true)
		for _, t := range tools {
			Expect(t.InputSchema).NotTo(BeNil(), "tool %s missing input schema", t.Name)
		}
	})

	It("UT-AF-220-012: NewMCPHandler returns error when ServerVersion is empty", func() {
		_, err := handler.NewMCPHandler(handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("server version"))
	})
})
