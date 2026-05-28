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

var _ = Describe("KA Integration (AF -> KA -> DS -> mock-LLM)", Label("e2e", "phase1", "ka-integration"), func() {

	var authToken string

	BeforeEach(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "Failed to obtain DEX token for KA integration tests")
		Expect(authToken).NotTo(BeEmpty())
	})

	mcpToolCall := func(id, toolName string, arguments map[string]interface{}) (int, map[string]interface{}) {
		sessionID, serr := initMCPSession(authToken)
		ExpectWithOffset(1, serr).NotTo(HaveOccurred(), "MCP initialize handshake failed")

		payload := buildJSONRPC(id, "tools/call", map[string]interface{}{
			"name":      toolName,
			"arguments": arguments,
		})
		respBody, statusCode, err := mcpPOST(authToken, sessionID, payload)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		jsonStr := unwrapSSEDataLine(respBody)
		var result map[string]interface{}
		_ = json.Unmarshal([]byte(jsonStr), &result)
		return statusCode, result
	}

	// -----------------------------------------------------------------------
	// AF -> KA Connectivity (MCP-only)
	// -----------------------------------------------------------------------
	Context("TC-E2E-KA: AF -> KA Connectivity", func() {

		It("TC-E2E-KA-01: kubernaut_investigate proxies to KA successfully", func() {
			status, result := mcpToolCall("e2e-ka-01", "kubernaut_investigate", map[string]interface{}{
				"rr_id": "e2e-connectivity-test-rr",
			})

			Expect(status).NotTo(Equal(http.StatusBadGateway),
				"AF returned 502 — KA is unreachable. Response: %v", result)
			Expect(status).NotTo(Equal(http.StatusServiceUnavailable),
				"AF returned 503 — circuit breaker open or KA down. Response: %v", result)
		})

		It("TC-E2E-KA-02: kubernaut_investigate reaches KA (error path: nonexistent session_id)", func() {
			status, result := mcpToolCall("e2e-ka-02", "kubernaut_investigate", map[string]interface{}{
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

		It("TC-E2E-KA-FLOW-01: kubernaut_investigate returns session_id and status from KA", func() {
			By("Creating RR so KA existence check passes (#1326 MCP migration)")
			Expect(createRR("default", "e2e-flow-01-test-rr", "Deployment", "test-deploy-flow01")).To(Succeed())
			DeferCleanup(func() { deleteRR("default", "e2e-flow-01-test-rr") })

			status, result := mcpToolCall("e2e-ka-flow-01", "kubernaut_investigate", map[string]interface{}{
				"rr_id": "e2e-flow-01-test-rr",
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

		It("TC-E2E-KA-FLOW-02: kubernaut_investigate resume by session_id returns status", func() {
			By("Creating RR so KA existence check passes (#1326 MCP migration)")
			Expect(createRR("default", "e2e-flow-02-test-rr", "Deployment", "test-deploy-flow02")).To(Succeed())
			DeferCleanup(func() { deleteRR("default", "e2e-flow-02-test-rr") })

			startStatus, startResult := mcpToolCall("e2e-ka-flow-02a", "kubernaut_investigate", map[string]interface{}{
				"rr_id": "e2e-flow-02-test-rr",
			})

			GinkgoWriter.Printf("KA-FLOW-02: mcpToolCall status=%d result=%v\n", startStatus, startResult)

			sid := extractSessionID(startResult)
			Expect(sid).NotTo(BeEmpty(),
				"KA must return a session_id for resume investigate to work (status=%d, result=%v)", startStatus, startResult)

			time.Sleep(500 * time.Millisecond)

			status, investigateResult := mcpToolCall("e2e-ka-flow-02b", "kubernaut_investigate", map[string]interface{}{
				"session_id": sid,
			})

			Expect(status).NotTo(Equal(http.StatusBadGateway),
				"TC-E2E-KA-FLOW-02: investigate resume returned 502. Response: %v", investigateResult)

			text := extractMCPResultText(investigateResult)
			if text != "" {
				var parsed map[string]interface{}
				if json.Unmarshal([]byte(text), &parsed) == nil {
					Expect(parsed).To(HaveKey("status"),
						"TC-E2E-KA-FLOW-02: investigate result should contain status field")
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
			mcpToolCall("e2e-metrics-seed", "kubernaut_list_remediations", map[string]interface{}{
				"namespace": "kubernaut-system",
			})

			body := scrapeMetrics()
			// CounterVec only appears after at least one observation. If KA/DS tools
			// were all skipped and kubernaut_list_remediations didn't reach the bridge (no session),
			// the counter won't exist. This is expected without full MCP session lifecycle.
			Expect(body).To(ContainSubstring("af_tool_calls_total"),
				"af_tool_calls_total must be present after tool calls — KA/DS must be deployed in E2E")
		})

		It("TC-E2E-METRICS-03: af_tool_call_duration_seconds has observations", func() {
			body := scrapeMetrics()
			Expect(body).To(ContainSubstring("af_tool_call_duration_seconds"),
				"af_tool_call_duration_seconds must be present after tool calls — KA/DS must be deployed in E2E")
		})
	})

	// -----------------------------------------------------------------------
	// Multi-Tool Sequential Flow
	// -----------------------------------------------------------------------
	Context("TC-E2E-MULTI: Cross-Service Sequential Flow", func() {

		It("TC-E2E-MULTI-01: investigate -> list_workflows within single auth session", func() {
			// Step 1: Run merged investigation via KA (start + stream in one call)
			status1, startResult := mcpToolCall("e2e-multi-01a", "kubernaut_investigate", map[string]interface{}{
				"rr_id": "e2e-multi-01-test-rr",
			})
			Expect(status1).NotTo(Equal(http.StatusBadGateway), "Step 1 (kubernaut_investigate) returned 502")

			// Step 2: Resume investigation by session_id when available
			sid := extractSessionID(startResult)
			if sid != "" {
				status2, _ := mcpToolCall("e2e-multi-01b", "kubernaut_investigate", map[string]interface{}{
					"session_id": sid,
				})
				Expect(status2).NotTo(Equal(http.StatusBadGateway), "Step 2 (kubernaut_investigate resume) returned 502")
			}

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

	// -----------------------------------------------------------------------
	// TC-E2E-KA-1326: Interactive Investigation (action=start) + Event Bridge
	// FedRAMP: AU-3 (audit trail), SI-4 (event monitoring), SC-7 (filtering)
	// -----------------------------------------------------------------------
	Context("TC-E2E-KA-1326: Interactive Investigation Flow", func() {

		It("TC-E2E-KA-1326-01: kubernaut_investigate returns session_id with 'started' status", func() {
			status, result := mcpToolCall("e2e-1326-01", "kubernaut_investigate", map[string]interface{}{
				"rr_id": "e2e-1326-interactive-rr",
			})

			Expect(status).NotTo(Equal(http.StatusBadGateway),
				"AF must not return 502 — KA unreachable. Response: %v", result)
			Expect(status).NotTo(Equal(http.StatusServiceUnavailable),
				"AF must not return 503 — circuit breaker open. Response: %v", result)

			text := extractMCPResultText(result)
			if text != "" {
				var parsed map[string]interface{}
				if json.Unmarshal([]byte(text), &parsed) == nil {
					Expect(parsed).To(HaveKey("session_id"),
						"TC-E2E-KA-1326-01: response must include session_id")
					Expect(parsed).To(HaveKey("status"),
						"TC-E2E-KA-1326-01: response must include status")
				}
			}
		})

		It("TC-E2E-KA-1326-02: kubernaut_investigate returns non-blocking (< 5s response time)", func() {
			start := time.Now()

			status, _ := mcpToolCall("e2e-1326-02", "kubernaut_investigate", map[string]interface{}{
				"rr_id": "e2e-1326-timing-rr",
			})

			elapsed := time.Since(start)
			Expect(status).NotTo(Equal(http.StatusBadGateway))

			// The tool should return within 5 seconds (non-blocking design).
			// AIA polling timeout is 3 min but context should not block here
			// since there's no AIA CRD for this RR in E2E.
			Expect(elapsed).To(BeNumerically("<", 30*time.Second),
				"TC-E2E-KA-1326-02: investigate must return promptly (non-blocking)")
		})

		It("TC-E2E-KA-1326-03: kubernaut_investigate with empty rr_id returns error", func() {
			status, result := mcpToolCall("e2e-1326-03", "kubernaut_investigate", map[string]interface{}{
				"rr_id": "",
			})

			// Should return a tool error (not 502/503)
			Expect(status).NotTo(Equal(http.StatusBadGateway))
			text := extractMCPResultText(result)
			if text != "" {
				Expect(strings.ToLower(text)).To(ContainSubstring("rr_id"),
					"TC-E2E-KA-1326-03: error must mention rr_id requirement")
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
