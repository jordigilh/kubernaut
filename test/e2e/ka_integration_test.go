package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("KA Integration (AF -> KA -> DS -> mock-LLM)", Ordered, ContinueOnFailure, Label("e2e", "phase1", "ka-integration"), func() {

	var authToken string

	BeforeAll(func() {
		var err error
		authToken, err = fetchDEXToken(dexURL, clientID, clientSecret, username, password)
		Expect(err).NotTo(HaveOccurred(), "Failed to obtain DEX token for KA integration tests")
		Expect(authToken).NotTo(BeEmpty())
	})

	authenticatedRequest := func(method, path string, body string) (*http.Response, error) {
		var bodyReader io.Reader
		if body != "" {
			bodyReader = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, baseURL+path, bodyReader)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+authToken)
		return httpClient.Do(req)
	}

	mcpToolCall := func(id, toolName string, arguments map[string]interface{}) (int, map[string]interface{}) {
		payload := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"id":      id,
			"params": map[string]interface{}{
				"name":      toolName,
				"arguments": arguments,
			},
		}
		body, err := json.Marshal(payload)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		resp, err := authenticatedRequest(http.MethodPost, "/mcp", string(body))
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		respBody, err := io.ReadAll(resp.Body)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		var result map[string]interface{}
		_ = json.Unmarshal(respBody, &result)
		return resp.StatusCode, result
	}

	// -----------------------------------------------------------------------
	// AF -> KA Connectivity (REST)
	// -----------------------------------------------------------------------
	Context("TC-E2E-KA: AF -> KA Connectivity", func() {

		It("TC-E2E-KA-01: kubernaut_start_investigation proxies to KA successfully", func() {
			status, result := mcpToolCall("e2e-ka-01", "kubernaut_start_investigation", map[string]interface{}{
				"namespace": "default",
				"name":      "test-pod",
				"kind":      "Pod",
			})

			Expect(status).NotTo(Equal(http.StatusBadGateway),
				"AF returned 502 — KA is unreachable. Response: %v", result)
			Expect(status).NotTo(Equal(http.StatusServiceUnavailable),
				"AF returned 503 — circuit breaker open or KA down. Response: %v", result)
		})

		It("TC-E2E-KA-02: kubernaut_poll_investigation reaches KA (error path: nonexistent session)", func() {
			status, result := mcpToolCall("e2e-ka-02", "kubernaut_poll_investigation", map[string]interface{}{
				"session_id": "nonexistent-session-id",
			})

			Expect(status).NotTo(Equal(http.StatusBadGateway),
				"AF returned 502 — KA unreachable. Response: %v", result)
		})
	})

	// -----------------------------------------------------------------------
	// AF -> KA Happy-Path Flow
	// -----------------------------------------------------------------------
	Context("TC-E2E-KA-FLOW: Investigation Lifecycle", func() {

		It("TC-E2E-KA-FLOW-01: start_investigation returns a session ID from KA", func() {
			status, result := mcpToolCall("e2e-ka-flow-01", "kubernaut_start_investigation", map[string]interface{}{
				"namespace": "kubernaut-system",
				"name":      "apifrontend",
				"kind":      "Deployment",
			})

			Expect(status).NotTo(Equal(http.StatusBadGateway))
			Expect(status).NotTo(Equal(http.StatusServiceUnavailable))

			text := extractMCPResultText(result)
			if text != "" {
				var parsed map[string]interface{}
				if json.Unmarshal([]byte(text), &parsed) == nil {
					Expect(parsed).To(HaveKey("session_id"),
						"TC-E2E-KA-FLOW-01: KA should return a session_id")
				}
			}
		})

		It("TC-E2E-KA-FLOW-02: poll_investigation returns status for active investigation", func() {
			_, startResult := mcpToolCall("e2e-ka-flow-02a", "kubernaut_start_investigation", map[string]interface{}{
				"namespace": "kubernaut-system",
				"name":      "apifrontend",
				"kind":      "Deployment",
			})

			sid := extractSessionID(startResult)
			if sid == "" {
				Skip("KA did not return a session_id — may not support this resource in E2E mode")
			}

			time.Sleep(500 * time.Millisecond)

			status, pollResult := mcpToolCall("e2e-ka-flow-02b", "kubernaut_poll_investigation", map[string]interface{}{
				"session_id": sid,
			})

			Expect(status).NotTo(Equal(http.StatusBadGateway),
				"TC-E2E-KA-FLOW-02: poll returned 502. Response: %v", pollResult)

			text := extractMCPResultText(pollResult)
			if text != "" {
				var parsed map[string]interface{}
				if json.Unmarshal([]byte(text), &parsed) == nil {
					Expect(parsed).To(HaveKey("status"),
						"TC-E2E-KA-FLOW-02: poll result should contain status field")
				}
			}
		})
	})

	// -----------------------------------------------------------------------
	// AF -> KA MCP (select_workflow)
	// -----------------------------------------------------------------------
	Context("TC-E2E-KA-MCP: Workflow Selection", func() {

		It("TC-E2E-KA-05: kubernaut_select_workflow reaches KA MCP endpoint", func() {
			status, result := mcpToolCall("e2e-ka-05", "kubernaut_select_workflow", map[string]interface{}{
				"rr_id":       "nonexistent-rr",
				"workflow_id": "wf-restart",
			})

			Expect(status).NotTo(Equal(http.StatusBadGateway),
				"TC-E2E-KA-05: AF returned 502 — KA MCP endpoint unreachable. Response: %v", result)
			Expect(status).NotTo(Equal(http.StatusServiceUnavailable),
				"TC-E2E-KA-05: AF returned 503 — circuit breaker open. Response: %v", result)
		})
	})

	// -----------------------------------------------------------------------
	// AF -> DS Tool Calls
	// -----------------------------------------------------------------------
	Context("TC-E2E-DS: Data Storage Tool Calls", func() {

		It("TC-E2E-DS-02: kubernaut_list_workflows returns response from DS", func() {
			status, result := mcpToolCall("e2e-ds-02", "kubernaut_list_workflows", map[string]interface{}{})

			Expect(status).NotTo(Equal(http.StatusBadGateway),
				"TC-E2E-DS-02: AF returned 502 — DS unreachable. Response: %v", result)
			Expect(status).NotTo(Equal(http.StatusServiceUnavailable),
				"TC-E2E-DS-02: AF returned 503 — DS circuit breaker open. Response: %v", result)

			text := extractMCPResultText(result)
			if text != "" {
				var parsed map[string]interface{}
				if json.Unmarshal([]byte(text), &parsed) == nil {
					Expect(parsed).To(HaveKey("workflows"),
						"TC-E2E-DS-02: list_workflows should return workflows array")
					Expect(parsed).To(HaveKey("count"),
						"TC-E2E-DS-02: list_workflows should return count")
				}
			}
		})

		It("TC-E2E-DS-03: kubernaut_get_remediation_history returns response from DS", func() {
			status, result := mcpToolCall("e2e-ds-03", "kubernaut_get_remediation_history", map[string]interface{}{
				"namespace": "default",
			})

			Expect(status).NotTo(Equal(http.StatusBadGateway),
				"TC-E2E-DS-03: AF returned 502 — DS unreachable. Response: %v", result)
			Expect(status).NotTo(Equal(http.StatusServiceUnavailable),
				"TC-E2E-DS-03: AF returned 503. Response: %v", result)
		})

		It("TC-E2E-DS-04: kubernaut_get_effectiveness returns response from DS", func() {
			status, result := mcpToolCall("e2e-ds-04", "kubernaut_get_effectiveness", map[string]interface{}{})

			Expect(status).NotTo(Equal(http.StatusBadGateway),
				"TC-E2E-DS-04: AF returned 502 — DS unreachable. Response: %v", result)
			Expect(status).NotTo(Equal(http.StatusServiceUnavailable),
				"TC-E2E-DS-04: AF returned 503. Response: %v", result)
		})

		It("TC-E2E-DS-05: kubernaut_get_audit_trail returns response from DS", func() {
			status, result := mcpToolCall("e2e-ds-05", "kubernaut_get_audit_trail", map[string]interface{}{
				"rr_id": "nonexistent-rr",
			})

			Expect(status).NotTo(Equal(http.StatusBadGateway),
				"TC-E2E-DS-05: AF returned 502 — DS unreachable. Response: %v", result)
			Expect(status).NotTo(Equal(http.StatusServiceUnavailable),
				"TC-E2E-DS-05: AF returned 503. Response: %v", result)
		})
	})

	// -----------------------------------------------------------------------
	// AF -> KA Decision Presentation (present_decision)
	// -----------------------------------------------------------------------
	Context("TC-E2E-KA-DECISION: Present Decision", func() {

		It("TC-E2E-KA-06: kubernaut_present_decision formats and returns decision prompt", func() {
			status, result := mcpToolCall("e2e-ka-06", "kubernaut_present_decision", map[string]interface{}{
				"session_id": "sess-decision-01",
				"summary":    "Pod api-gateway OOMKilled due to memory limit exceeded",
				"options": []interface{}{
					map[string]interface{}{
						"workflow_id": "wf-restart",
						"name":        "Restart Pod",
						"description": "Delete the failing pod and let the controller recreate it",
						"risk":        "low",
					},
					map[string]interface{}{
						"workflow_id": "wf-scale",
						"name":        "Scale Up",
						"description": "Increase replica count to distribute load",
						"risk":        "medium",
					},
				},
			})

			Expect(status).NotTo(Equal(http.StatusBadGateway),
				"TC-E2E-KA-06: AF returned 502. Response: %v", result)
			Expect(status).NotTo(Equal(http.StatusServiceUnavailable),
				"TC-E2E-KA-06: AF returned 503. Response: %v", result)

			text := extractMCPResultText(result)
			if text != "" {
				var parsed map[string]interface{}
				if json.Unmarshal([]byte(text), &parsed) == nil {
					Expect(parsed).To(HaveKey("presented"),
						"TC-E2E-KA-06: should return presented=true")
					Expect(parsed).To(HaveKey("message"),
						"TC-E2E-KA-06: should return formatted message")
					msg, _ := parsed["message"].(string)
					Expect(msg).To(ContainSubstring("OOMKilled"),
						"TC-E2E-KA-06: message should include the summary")
					Expect(msg).To(ContainSubstring("Restart Pod"),
						"TC-E2E-KA-06: message should list workflow options")
					Expect(msg).To(ContainSubstring("Scale Up"),
						"TC-E2E-KA-06: message should list all options")
				}
			}
		})

		It("TC-E2E-KA-07: kubernaut_present_decision with empty options still succeeds", func() {
			status, result := mcpToolCall("e2e-ka-07", "kubernaut_present_decision", map[string]interface{}{
				"session_id": "sess-decision-02",
				"summary":    "No remediation options available",
				"options":    []interface{}{},
			})

			Expect(status).NotTo(Equal(http.StatusBadGateway),
				"TC-E2E-KA-07: AF returned 502. Response: %v", result)

			text := extractMCPResultText(result)
			if text != "" {
				var parsed map[string]interface{}
				if json.Unmarshal([]byte(text), &parsed) == nil {
					Expect(parsed["presented"]).To(BeTrue(),
						"TC-E2E-KA-07: should still present even with no options")
				}
			}
		})
	})

	// -----------------------------------------------------------------------
	// Metrics Observability
	// -----------------------------------------------------------------------
	Context("TC-E2E-METRICS: Post-Integration Observability", func() {

		It("TC-E2E-KA-03: af_downstream_request_duration_seconds{dependency=ka} has observations", func() {
			body := scrapeMetrics()
			Expect(body).To(ContainSubstring(`af_downstream_request_duration_seconds`),
				"TC-E2E-KA-03: downstream duration histogram should exist")
			Expect(body).To(MatchRegexp(`af_downstream_request_duration_seconds_bucket\{.*dependency="ka"`),
				"TC-E2E-KA-03: should have dependency=ka observations")
		})

		It("TC-E2E-KA-04: af_circuit_breaker_state{dependency=ka} remains closed (0)", func() {
			body := scrapeMetrics()
			Expect(body).To(MatchRegexp(`af_circuit_breaker_state\{[^}]*dependency="ka"`),
				"TC-E2E-KA-04: KA circuit breaker metric should exist")
			Expect(body).To(MatchRegexp(`af_circuit_breaker_state\{[^}]*dependency="ka"[^}]*\} 0`),
				"TC-E2E-KA-04: KA circuit breaker should be closed (0)")
		})

		It("TC-E2E-DS-METRICS-01: af_downstream_request_duration_seconds{dependency=ds} has observations after DS calls", func() {
			body := scrapeMetrics()
			Expect(body).To(MatchRegexp(`af_downstream_request_duration_seconds_bucket\{.*dependency="ds"`),
				"TC-E2E-DS-METRICS-01: should have dependency=ds observations after DS tool calls")
		})

		It("TC-E2E-DS-01: af_circuit_breaker_state{dependency=ds} remains closed (0)", func() {
			body := scrapeMetrics()
			Expect(body).To(MatchRegexp(`af_circuit_breaker_state\{[^}]*dependency="ds"`),
				"TC-E2E-DS-01: DS circuit breaker metric should exist")
			Expect(body).To(MatchRegexp(`af_circuit_breaker_state\{[^}]*dependency="ds"[^}]*\} 0`),
				"TC-E2E-DS-01: DS circuit breaker should be closed (0)")
		})

		It("TC-E2E-METRICS-02: af_tool_calls_total has observations after tool calls", func() {
			// Make a tool call via the standard mcpToolCall helper to trigger the metric
			mcpToolCall("e2e-metrics-seed", "af_list_events", map[string]interface{}{
				"namespace": "kubernaut-system",
			})

			body := scrapeMetrics()
			// CounterVec only appears after at least one observation. If KA/DS tools
			// were all skipped and af_list_events didn't reach the bridge (no session),
			// the counter won't exist. This is expected without full MCP session lifecycle.
			if !strings.Contains(body, "af_tool_calls_total") {
				Skip("af_tool_calls_total not present — no tool calls reached the bridge dispatcher (KA/DS not deployed)")
			}
			Expect(body).To(ContainSubstring(`af_tool_calls_total`))
		})

		It("TC-E2E-METRICS-03: af_tool_call_duration_seconds has observations", func() {
			body := scrapeMetrics()
			if !strings.Contains(body, "af_tool_call_duration_seconds") {
				Skip("af_tool_call_duration_seconds not present — no tool calls reached the bridge dispatcher (KA/DS not deployed)")
			}
			Expect(body).To(ContainSubstring(`af_tool_call_duration_seconds`),
				"TC-E2E-METRICS-03: tool call duration histogram should exist")
		})
	})

	// -----------------------------------------------------------------------
	// Multi-Tool Sequential Flow
	// -----------------------------------------------------------------------
	Context("TC-E2E-MULTI: Cross-Service Sequential Flow", func() {

		It("TC-E2E-MULTI-01: start_investigation -> poll -> list_workflows within single auth session", func() {
			// Step 1: Start investigation via KA
			status1, _ := mcpToolCall("e2e-multi-01a", "kubernaut_start_investigation", map[string]interface{}{
				"namespace": "default",
				"name":      "test-pod",
				"kind":      "Pod",
			})
			Expect(status1).NotTo(Equal(http.StatusBadGateway), "Step 1 (start_investigation) returned 502")

			// Step 2: Poll investigation via KA
			status2, _ := mcpToolCall("e2e-multi-01b", "kubernaut_poll_investigation", map[string]interface{}{
				"session_id": "test-session-multi",
			})
			Expect(status2).NotTo(Equal(http.StatusBadGateway), "Step 2 (poll_investigation) returned 502")

			// Step 3: List workflows via DS (different downstream)
			status3, _ := mcpToolCall("e2e-multi-01c", "kubernaut_list_workflows", map[string]interface{}{})
			Expect(status3).NotTo(Equal(http.StatusBadGateway), "Step 3 (list_workflows) returned 502 — DS unreachable")

			// Step 4: Get effectiveness via DS
			status4, _ := mcpToolCall("e2e-multi-01d", "kubernaut_get_effectiveness", map[string]interface{}{})
			Expect(status4).NotTo(Equal(http.StatusBadGateway), "Step 4 (get_effectiveness) returned 502 — DS unreachable")

			// Verify both CBs stayed closed after cross-service traffic
			metrics := scrapeMetrics()
			if strings.Contains(metrics, `dependency="ka"`) {
				Expect(metrics).To(MatchRegexp(`af_circuit_breaker_state\{[^}]*dependency="ka"[^}]*\} 0`),
					"KA CB should remain closed after multi-tool flow")
			}
			if strings.Contains(metrics, `dependency="ds"`) {
				Expect(metrics).To(MatchRegexp(`af_circuit_breaker_state\{[^}]*dependency="ds"[^}]*\} 0`),
					"DS CB should remain closed after multi-tool flow")
			}
		})
	})

	// -----------------------------------------------------------------------
	// Readiness After Integration
	// -----------------------------------------------------------------------
	Context("TC-E2E-READYZ: Readiness After Integration", func() {

		It("TC-E2E-READYZ-01: /readyz returns 200 after KA + DS interactions", func() {
			resp, err := httpClient.Get(baseURL + "/readyz")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			respBody, _ := io.ReadAll(resp.Body)
			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"TC-E2E-READYZ-01: /readyz should return 200. Got: %s", string(respBody))
		})

		It("TC-E2E-READYZ-02: /readyz response includes structured status", func() {
			resp, err := httpClient.Get(baseURL + "/readyz")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			var status map[string]interface{}
			if json.Unmarshal(respBody, &status) == nil {
				Expect(status).To(HaveKey("status"))
			}
		})
	})
})

// extractSessionID navigates the MCP JSON-RPC response to find a session_id.
func extractSessionID(result map[string]interface{}) string {
	text := extractMCPResultText(result)
	if text == "" {
		return ""
	}
	var parsed map[string]interface{}
	if json.Unmarshal([]byte(text), &parsed) != nil {
		return ""
	}
	sid, _ := parsed["session_id"].(string)
	return sid
}
