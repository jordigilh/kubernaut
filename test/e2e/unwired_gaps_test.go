package e2e_test

import (
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These tests document known unwired code paths in the apifrontend binary.
// Each test asserts the current behavior (stub/absent) so regressions or
// accidental wiring changes are caught. When a gap is closed, the test
// should be updated to assert the new behavior.

var _ = Describe("Unwired Code Gaps (E2E)", Ordered, ContinueOnFailure, Label("e2e", "phase1", "unwired-gaps"), func() {

	var authToken string

	BeforeAll(func() {
		var err error
		authToken, err = fetchDEXToken(dexURL, clientID, clientSecret, username, password)
		Expect(err).NotTo(HaveOccurred(), "Failed to obtain DEX token")
		Expect(authToken).NotTo(BeEmpty())
	})

	authenticatedPost := func(path, body string) (*http.Response, error) {
		req, err := http.NewRequest(http.MethodPost, baseURL+path, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if path == "/mcp" {
			req.Header.Set("Accept", "application/json, text/event-stream")
		}
		req.Header.Set("Authorization", "Bearer "+authToken)
		return httpClient.Do(req)
	}

	// -------------------------------------------------------------------
	// TC-E2E-UNWIRED-001: POST /a2a/invoke accepts JSON-RPC (mock-LLM wired)
	// kubernaut#1157 landed — A2A handler is live with mock-LLM Gemini backend.
	// -------------------------------------------------------------------
	It("TC-E2E-UNWIRED-001: POST /a2a/invoke returns JSON-RPC response (not 501)", func() {
		body := `{"jsonrpc":"2.0","method":"message/send","id":"unwired-001","params":{"message":{"messageId":"msg-unwired-001","role":"user","parts":[{"kind":"text","text":"hello"}]}}}`
		resp, err := authenticatedPost("/a2a/invoke", body)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).NotTo(Equal(http.StatusNotImplemented),
			"A2A endpoint should no longer return 501 — mock-LLM is wired")

		respBody, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(respBody)).NotTo(ContainSubstring("A2A not configured"))
	})

	// -------------------------------------------------------------------
	// TC-E2E-UNWIRED-002: kubernaut_cancel_investigation tool not exposed
	// -------------------------------------------------------------------
	It("TC-E2E-UNWIRED-002: MCP tools/list does not expose kubernaut_cancel_investigation", func() {
		// Initialize MCP session first (required by Streamable HTTP transport)
		initPayload := `{"jsonrpc":"2.0","method":"initialize","id":"unwired-002-init","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e-unwired","version":"1.0"}}}`
		initResp, err := authenticatedPost("/mcp", initPayload)
		Expect(err).NotTo(HaveOccurred())
		sessionID := initResp.Header.Get("Mcp-Session-Id")
		_ = initResp.Body.Close()

		// List tools using the session
		listReq, err := http.NewRequest(http.MethodPost, baseURL+"/mcp",
			strings.NewReader(`{"jsonrpc":"2.0","method":"tools/list","id":"unwired-002","params":{}}`))
		Expect(err).NotTo(HaveOccurred())
		listReq.Header.Set("Content-Type", "application/json")
		listReq.Header.Set("Accept", "application/json, text/event-stream")
		listReq.Header.Set("Authorization", "Bearer "+authToken)
		if sessionID != "" {
			listReq.Header.Set("Mcp-Session-Id", sessionID)
		}

		resp, err := httpClient.Do(listReq)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		respBody, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())

		// The MCP response may be SSE-wrapped or direct JSON
		bodyStr := string(respBody)
		if strings.Contains(bodyStr, "data:") {
			for _, line := range strings.Split(bodyStr, "\n") {
				if strings.HasPrefix(line, "data:") {
					bodyStr = strings.TrimPrefix(line, "data:")
					break
				}
			}
		}

		Expect(bodyStr).NotTo(ContainSubstring("kubernaut_cancel_investigation"),
			"kubernaut_cancel_investigation should not be exposed (ka.Cancel is unwired)")
	})

	// -------------------------------------------------------------------
	// TC-E2E-UNWIRED-003: af_sessions_active gauge is registered
	// -------------------------------------------------------------------
	It("TC-E2E-UNWIRED-003: af_sessions_active gauge is present in /metrics", func() {
		metrics := scrapeMetrics()

		Expect(metrics).To(ContainSubstring("af_sessions_active"),
			"buildSessionInfra should register the sessions active gauge")

		for _, phase := range []string{"Active", "Disconnected", "Completed", "Cancelled", "Failed"} {
			Expect(metrics).To(ContainSubstring(
				`af_sessions_active{phase="`+phase+`"}`),
				"af_sessions_active should have label phase=%s", phase)
		}
	})

	// -------------------------------------------------------------------
	// TC-E2E-UNWIRED-004: af_session_ttl_actions_total counter is registered
	// -------------------------------------------------------------------
	It("TC-E2E-UNWIRED-004: af_session_ttl_actions_total counter is present in /metrics", func() {
		metrics := scrapeMetrics()
		Expect(metrics).To(ContainSubstring("af_session_ttl_actions_total"),
			"SessionCleanupReconciler should register the TTL actions counter")
	})
})
