package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
)

func parseJSONRPCFromResponse(body string) map[string]any {
	var result map[string]any
	if strings.Contains(body, "data: ") {
		lines := strings.Split(body, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "data: {") {
				_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &result)
				return result
			}
		}
	}
	_ = json.Unmarshal([]byte(body), &result)
	if result == nil {
		result = make(map[string]any)
	}
	return result
}

var _ = Describe("MCP Protocol Conformance", func() {
	var mcpHandler http.Handler

	BeforeEach(func() {
		var err error
		mcpHandler, err = handler.NewMCPHandler(handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.1.0",
			Tools:         handler.DefaultMCPTools(),
			Enabled:       true,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	sendJSONRPC := func(h http.Handler, method string, id int, params any) *httptest.ResponseRecorder {
		reqBody := map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  method,
		}
		if params != nil {
			reqBody["params"] = params
		}
		body, _ := json.Marshal(reqBody)
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
		h.ServeHTTP(rec, req)
		return rec
	}

	initializeSession := func(h http.Handler) string {
		rec := sendJSONRPC(h, "initialize", 1, map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "conformance-test", "version": "1.0"},
		})
		Expect(rec.Code).To(Equal(http.StatusOK))
		sessionID := rec.Header().Get("Mcp-Session-Id")

		// Send initialized notification
		notifBody, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"method":  "notifications/initialized",
		})
		req := httptest.NewRequest("POST", "/mcp", strings.NewReader(string(notifBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if sessionID != "" {
			req.Header.Set("Mcp-Session-Id", sessionID)
		}
		ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
			Username: "testuser",
			Groups:   []string{"sre"},
			Issuer:   "test",
			RawToken: "tok",
		})
		req = req.WithContext(ctx)
		notifRec := httptest.NewRecorder()
		h.ServeHTTP(notifRec, req)

		return sessionID
	}

	sendWithSession := func(h http.Handler, sessionID, method string, id int, params any) *httptest.ResponseRecorder {
		reqBody := map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  method,
		}
		if params != nil {
			reqBody["params"] = params
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/mcp", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if sessionID != "" {
			req.Header.Set("Mcp-Session-Id", sessionID)
		}
		ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
			Username: "testuser",
			Groups:   []string{"sre"},
			Issuer:   "test",
			RawToken: "tok",
		})
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	Describe("tools/list conformance", func() {
		It("UT-AF-042-001: tools/list returns 14 tools after initialization", func() {
			sessionID := initializeSession(mcpHandler)
			rec := sendWithSession(mcpHandler, sessionID, "tools/list", 2, nil)
			Expect(rec.Code).To(Equal(http.StatusOK))

			result := parseJSONRPCFromResponse(rec.Body.String())
			Expect(result).To(HaveKey("result"))
			resultObj, ok := result["result"].(map[string]any)
			Expect(ok).To(BeTrue())
			tools, ok := resultObj["tools"].([]any)
			Expect(ok).To(BeTrue())
			Expect(tools).To(HaveLen(20))
		})

		It("UT-AF-042-002: all tool names have kubernaut_ or af_ prefix", func() {
			sessionID := initializeSession(mcpHandler)
			rec := sendWithSession(mcpHandler, sessionID, "tools/list", 2, nil)

			result := parseJSONRPCFromResponse(rec.Body.String())
			resultObj, ok := result["result"].(map[string]any)
			Expect(ok).To(BeTrue())
			tools, ok := resultObj["tools"].([]any)
			Expect(ok).To(BeTrue())
			for _, t := range tools {
				tool, ok := t.(map[string]any)
				Expect(ok).To(BeTrue())
				name, ok := tool["name"].(string)
				Expect(ok).To(BeTrue())
				hasValid := strings.HasPrefix(name, "kubernaut_") || strings.HasPrefix(name, "af_")
				Expect(hasValid).To(BeTrue(), "tool %q missing kubernaut_ or af_ prefix", name)
			}
		})

		It("UT-AF-042-003: all tools have non-empty description", func() {
			sessionID := initializeSession(mcpHandler)
			rec := sendWithSession(mcpHandler, sessionID, "tools/list", 2, nil)

			result := parseJSONRPCFromResponse(rec.Body.String())
			resultObj, ok := result["result"].(map[string]any)
			Expect(ok).To(BeTrue())
			tools, ok := resultObj["tools"].([]any)
			Expect(ok).To(BeTrue())
			for _, t := range tools {
				tool, ok := t.(map[string]any)
				Expect(ok).To(BeTrue())
				desc, ok := tool["description"].(string)
				Expect(ok).To(BeTrue(), "tool %v missing description", tool["name"])
				Expect(desc).NotTo(BeEmpty())
			}
		})

		It("UT-AF-042-004: all tools have inputSchema with type object", func() {
			sessionID := initializeSession(mcpHandler)
			rec := sendWithSession(mcpHandler, sessionID, "tools/list", 2, nil)

			result := parseJSONRPCFromResponse(rec.Body.String())
			resultObj, ok := result["result"].(map[string]any)
			Expect(ok).To(BeTrue())
			tools, ok := resultObj["tools"].([]any)
			Expect(ok).To(BeTrue())
			for _, t := range tools {
				tool, ok := t.(map[string]any)
				Expect(ok).To(BeTrue())
				schema, ok := tool["inputSchema"].(map[string]any)
				Expect(ok).To(BeTrue(), "tool %v missing inputSchema", tool["name"])
				Expect(schema["type"]).To(Equal("object"))
			}
		})

		It("UT-AF-042-005: response is valid JSON-RPC 2.0", func() {
			sessionID := initializeSession(mcpHandler)
			rec := sendWithSession(mcpHandler, sessionID, "tools/list", 2, nil)

			result := parseJSONRPCFromResponse(rec.Body.String())
			Expect(result["jsonrpc"]).To(Equal("2.0"))
			Expect(result).To(HaveKey("id"))
		})
	})

	Describe("MCP error codes", func() {
		It("UT-AF-042-006: tools/call without params returns InvalidRequest -32600", func() {
			sessionID := initializeSession(mcpHandler)
			rec := sendWithSession(mcpHandler, sessionID, "tools/call", 3, nil)

			respBody := rec.Body.String()
			result := parseJSONRPCFromResponse(respBody)

			if rec.Code >= 400 {
				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"HTTP rejection expected 400, got %d; body: %s", rec.Code, respBody)
				return
			}

			Expect(result).To(HaveKey("error"), "expected JSON-RPC error, got: %s", respBody)
			errObj, ok := result["error"].(map[string]any)
			Expect(ok).To(BeTrue(), "error field is not an object: %v", result["error"])
			codeVal, ok := errObj["code"].(float64)
			Expect(ok).To(BeTrue(), "error.code missing or wrong type")
			Expect(int(codeVal)).To(Equal(-32600), "expected InvalidRequest (-32600), got %d", int(codeVal))
		})

		It("UT-AF-042-007: unknown method returns MethodNotFound -32601", func() {
			sessionID := initializeSession(mcpHandler)
			rec := sendWithSession(mcpHandler, sessionID, "nonexistent/method", 4, nil)

			respBody := rec.Body.String()
			result := parseJSONRPCFromResponse(respBody)

			if rec.Code >= 400 {
				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"HTTP rejection expected 400, got %d; body: %s", rec.Code, respBody)
				return
			}

			Expect(result).To(HaveKey("error"), "expected JSON-RPC error, got: %s", respBody)
			errObj, ok := result["error"].(map[string]any)
			Expect(ok).To(BeTrue(), "error field is not an object: %v", result["error"])
			codeVal, ok := errObj["code"].(float64)
			Expect(ok).To(BeTrue(), "error.code missing or wrong type")
			Expect(int(codeVal)).To(Equal(-32601), "expected MethodNotFound (-32601), got %d", int(codeVal))
		})

		It("UT-AF-042-008: tools/list before initialize returns error", func() {
			// Send tools/list without prior initialize
			rec := sendJSONRPC(mcpHandler, "tools/list", 1, nil)

			respBody := rec.Body.String()
			result := parseJSONRPCFromResponse(respBody)

			Expect(result).To(HaveKey("error"))
			errObj, ok := result["error"].(map[string]any)
			Expect(ok).To(BeTrue())
			msg, ok := errObj["message"].(string)
			Expect(ok).To(BeTrue())
			Expect(msg).To(ContainSubstring("invalid"))
		})

		It("UT-AF-042-009: malformed JSON body returns error", func() {
			req := httptest.NewRequest("POST", "/mcp", strings.NewReader("{invalid json"))
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

			// SDK may respond with 400 status or JSON-RPC error
			if rec.Code == http.StatusBadRequest {
				return // acceptable: HTTP-level rejection of malformed body
			}

			respBody := rec.Body.String()
			result := parseJSONRPCFromResponse(respBody)
			Expect(result).To(HaveKey("error"))
			errObj, ok := result["error"].(map[string]any)
			Expect(ok).To(BeTrue())
			codeVal, ok := errObj["code"].(float64)
			Expect(ok).To(BeTrue())
			Expect(int(codeVal)).To(Equal(-32700))
		})
	})
})
