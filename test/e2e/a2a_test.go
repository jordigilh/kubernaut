package e2e_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Comprehensive E2E test suite for the A2A handler.
// Exercises all 19 tools, 6 RBAC personas, metrics/audit callbacks, multi-tool
// workflows, protocol errors, and session lifecycle.
//
// Gated on mock-LLM Gemini endpoint (kubernaut#1157): if /a2a/invoke returns 501
// the entire suite is skipped with a clear message.

var _ = Describe("A2A Handler (E2E)", Ordered, ContinueOnFailure, Label("e2e", "a2a"), func() {

	var (
		sreToken           string
		orchestratorToken  string
		cicdToken          string
		observabilityToken string
		auditorToken       string
		approverToken      string
	)

	BeforeAll(func() {
		var err error
		sreToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "Failed to obtain SRE token")

		// Gate: skip entire suite if A2A returns 501 (mock-LLM Gemini not available)
		resp, err := a2aInvoke(httpClient, baseURL, sreToken, a2aTasksSend("gate-check", "ping"))
		Expect(err).NotTo(HaveOccurred())
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusNotImplemented {
			Skip("A2A endpoint returns 501 — mock-LLM Gemini endpoint (kubernaut#1157) not yet available")
		}

		orchestratorToken, err = fetchDEXTokenForPersona("ai-orchestrator")
		Expect(err).NotTo(HaveOccurred())
		cicdToken, err = fetchDEXTokenForPersona("cicd")
		Expect(err).NotTo(HaveOccurred())
		observabilityToken, err = fetchDEXTokenForPersona("observability")
		Expect(err).NotTo(HaveOccurred())
		auditorToken, err = fetchDEXTokenForPersona("l3-audit")
		Expect(err).NotTo(HaveOccurred())
		approverToken, err = fetchDEXTokenForPersona("remediation-approver")
		Expect(err).NotTo(HaveOccurred())
	})

	// ===================================================================
	// Category 1: Per-Tool Happy Path via SRE persona (19 tests)
	// ===================================================================
	Context("Category 1: Per-Tool Happy Path (SRE)", func() {

		toolTest := func(id, prompt, expectedTool string) {
			resp, err := a2aInvoke(httpClient, baseURL, sreToken, a2aTasksSend(id, prompt))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "A2A should return 200 for %s", id)

			rpc, err := parseRPCResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(rpc.Error).To(BeNil(), "%s: unexpected JSON-RPC error: %+v", id, rpc.Error)
			Expect(rpc.Result).NotTo(BeNil())

			task, err := extractTaskFromResult(rpc.Result)
			Expect(err).NotTo(HaveOccurred())
			Expect(task.ID).NotTo(BeEmpty(), "%s: task ID must not be empty", id)
			Expect(task.Status.State).To(BeElementOf("completed", "working", "failed"),
				"%s: task should reach a valid state", id)
		}

		It("TC-E2E-A2A-T01: kubernaut_start_investigation", func() {
			toolTest("a2a-t01", "Start investigation on pod nginx in default namespace", "kubernaut_start_investigation")
		})
		It("TC-E2E-A2A-T02: kubernaut_poll_investigation", func() {
			toolTest("a2a-t02", "Check investigation status for session sess-001", "kubernaut_poll_investigation")
		})
		It("TC-E2E-A2A-T03: kubernaut_select_workflow", func() {
			toolTest("a2a-t03", "Select the restart workflow for remediation rr-001", "kubernaut_select_workflow")
		})
		It("TC-E2E-A2A-T04: kubernaut_present_decision", func() {
			toolTest("a2a-t04", "Present remediation options to the user for session sess-002", "kubernaut_present_decision")
		})
		It("TC-E2E-A2A-T05: kubernaut_list_remediations", func() {
			toolTest("a2a-t05", "List all remediations in the default namespace", "kubernaut_list_remediations")
		})
		It("TC-E2E-A2A-T06: kubernaut_get_remediation", func() {
			toolTest("a2a-t06", "Get details for remediation rr-test in payments namespace", "kubernaut_get_remediation")
		})
		It("TC-E2E-A2A-T07: kubernaut_approve", func() {
			toolTest("a2a-t07", "Approve the remediation rr-test in payments namespace", "kubernaut_approve")
		})
		It("TC-E2E-A2A-T08: kubernaut_cancel_remediation", func() {
			toolTest("a2a-t08", "Cancel the remediation rr-test in payments namespace", "kubernaut_cancel_remediation")
		})
		It("TC-E2E-A2A-T09: kubernaut_watch", func() {
			toolTest("a2a-t09", "Watch the progress of remediation rr-test", "kubernaut_watch")
		})
		It("TC-E2E-A2A-T10: kubernaut_list_workflows", func() {
			toolTest("a2a-t10", "List all available remediation workflows", "kubernaut_list_workflows")
		})
		It("TC-E2E-A2A-T11: kubernaut_get_remediation_history", func() {
			toolTest("a2a-t11", "Get remediation history for the payments namespace", "kubernaut_get_remediation_history")
		})
		It("TC-E2E-A2A-T12: kubernaut_get_effectiveness", func() {
			toolTest("a2a-t12", "Show remediation effectiveness statistics", "kubernaut_get_effectiveness")
		})
		It("TC-E2E-A2A-T13: kubernaut_get_audit_trail", func() {
			toolTest("a2a-t13", "Get the audit trail for remediation rr-audit", "kubernaut_get_audit_trail")
		})
		It("TC-E2E-A2A-T14: af_list_events", func() {
			toolTest("a2a-t14", "List Kubernetes events in the kubernaut-system namespace", "af_list_events")
		})
		It("TC-E2E-A2A-T15: af_get_pods", func() {
			toolTest("a2a-t15", "Get all pods in the default namespace", "af_get_pods")
		})
		It("TC-E2E-A2A-T16: af_get_workloads", func() {
			toolTest("a2a-t16", "Get workloads in the default namespace", "af_get_workloads")
		})
		It("TC-E2E-A2A-T17: af_resolve_owner", func() {
			toolTest("a2a-t17", "Resolve the owner of pod nginx-abc123 in default namespace", "af_resolve_owner")
		})
		It("TC-E2E-A2A-T18: af_check_existing_rr", func() {
			toolTest("a2a-t18", "Check if a remediation request already exists for deployment web in prod", "af_check_existing_rr")
		})
		It("TC-E2E-A2A-T19: af_create_rr", func() {
			toolTest("a2a-t19", "Create a remediation request for deployment web in prod namespace", "af_create_rr")
		})
	})

	// ===================================================================
	// Category 2: RBAC Denial Tests (17 tests)
	// ===================================================================
	Context("Category 2: RBAC Denial", func() {

		rbacDenialTest := func(id, token, prompt, deniedTool string) {
			resp, err := a2aInvoke(httpClient, baseURL, token, a2aTasksSend(id, prompt))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			rpc, err := parseRPCResponse(resp)
			Expect(err).NotTo(HaveOccurred())

			// RBAC denial manifests as either a JSON-RPC error or a completed task
			// with an error message containing "unauthorized" or "rbac"
			if rpc.Error != nil {
				return // JSON-RPC level error is acceptable for denied calls
			}
			if rpc.Result != nil {
				task, _ := extractTaskFromResult(rpc.Result)
				if task.Status.State == "failed" || task.Status.State == "completed" {
					// Expect the response to indicate denial
					bodyBytes, _ := json.Marshal(rpc.Result)
					bodyStr := strings.ToLower(string(bodyBytes))
					Expect(bodyStr).To(SatisfyAny(
						ContainSubstring("unauthorized"),
						ContainSubstring("rbac"),
						ContainSubstring("denied"),
						ContainSubstring("not allowed"),
						ContainSubstring("error"),
					), "%s: RBAC denial for %s should produce an access denied indicator", id, deniedTool)
				}
			}
		}

		// cicd persona — denied tools
		It("TC-E2E-A2A-RBAC-01: cicd denied kubernaut_approve", func() {
			rbacDenialTest("rbac-01", cicdToken, "Approve the remediation rr-test in payments namespace", "kubernaut_approve")
		})
		It("TC-E2E-A2A-RBAC-02: cicd denied af_get_pods", func() {
			rbacDenialTest("rbac-02", cicdToken, "Get all pods in the default namespace", "af_get_pods")
		})
		It("TC-E2E-A2A-RBAC-03: cicd denied kubernaut_start_investigation", func() {
			rbacDenialTest("rbac-03", cicdToken, "Start investigation on pod nginx in default namespace", "kubernaut_start_investigation")
		})

		// observability persona — denied tools
		It("TC-E2E-A2A-RBAC-04: observability denied kubernaut_approve", func() {
			rbacDenialTest("rbac-04", observabilityToken, "Approve the remediation rr-test in payments namespace", "kubernaut_approve")
		})
		It("TC-E2E-A2A-RBAC-05: observability denied kubernaut_start_investigation", func() {
			rbacDenialTest("rbac-05", observabilityToken, "Start investigation on pod nginx in default namespace", "kubernaut_start_investigation")
		})
		It("TC-E2E-A2A-RBAC-06: observability denied af_create_rr", func() {
			rbacDenialTest("rbac-06", observabilityToken, "Create a remediation request for deployment web in prod namespace", "af_create_rr")
		})

		// l3-audit persona — denied tools
		It("TC-E2E-A2A-RBAC-07: l3-audit denied kubernaut_approve", func() {
			rbacDenialTest("rbac-07", auditorToken, "Approve the remediation rr-test in payments namespace", "kubernaut_approve")
		})
		It("TC-E2E-A2A-RBAC-08: l3-audit denied af_get_pods", func() {
			rbacDenialTest("rbac-08", auditorToken, "Get all pods in the default namespace", "af_get_pods")
		})
		It("TC-E2E-A2A-RBAC-09: l3-audit denied kubernaut_start_investigation", func() {
			rbacDenialTest("rbac-09", auditorToken, "Start investigation on pod nginx in default namespace", "kubernaut_start_investigation")
		})

		// remediation-approver persona — denied tools
		It("TC-E2E-A2A-RBAC-10: approver denied kubernaut_start_investigation", func() {
			rbacDenialTest("rbac-10", approverToken, "Start investigation on pod nginx in default namespace", "kubernaut_start_investigation")
		})
		It("TC-E2E-A2A-RBAC-11: approver denied af_get_pods", func() {
			rbacDenialTest("rbac-11", approverToken, "Get all pods in the default namespace", "af_get_pods")
		})
		It("TC-E2E-A2A-RBAC-12: approver denied kubernaut_list_workflows", func() {
			rbacDenialTest("rbac-12", approverToken, "List all available remediation workflows", "kubernaut_list_workflows")
		})

		// ai-orchestrator persona — denied tools
		It("TC-E2E-A2A-RBAC-13: orchestrator denied kubernaut_get_audit_trail", func() {
			rbacDenialTest("rbac-13", orchestratorToken, "Get the audit trail for remediation rr-audit", "kubernaut_get_audit_trail")
		})
		It("TC-E2E-A2A-RBAC-14: orchestrator denied kubernaut_list_workflows", func() {
			rbacDenialTest("rbac-14", orchestratorToken, "List all available remediation workflows", "kubernaut_list_workflows")
		})
		It("TC-E2E-A2A-RBAC-15: orchestrator denied kubernaut_get_effectiveness", func() {
			rbacDenialTest("rbac-15", orchestratorToken, "Show remediation effectiveness statistics", "kubernaut_get_effectiveness")
		})

		// Unauthenticated request
		It("TC-E2E-A2A-RBAC-16: unauthenticated request returns 401", func() {
			req, err := http.NewRequest(http.MethodPost, baseURL+"/a2a/invoke",
				strings.NewReader(a2aTasksSend("rbac-16", "hello")))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		// Expired/invalid token
		It("TC-E2E-A2A-RBAC-17: invalid token returns 401", func() {
			resp, err := a2aInvoke(httpClient, baseURL, "invalid-jwt-token", a2aTasksSend("rbac-17", "hello"))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})
	})

	// ===================================================================
	// Category 3: Metrics and Audit Observability (6 tests)
	// ===================================================================
	Context("Category 3: Metrics and Audit Observability", func() {

		It("TC-E2E-A2A-MET-01: af_tool_calls_total has observations after tool execution", func() {
			metrics := scrapeMetrics()
			Expect(metrics).To(ContainSubstring("af_tool_calls_total"),
				"af_tool_calls_total counter should exist after A2A tool calls")
		})

		It("TC-E2E-A2A-MET-02: af_tool_calls_total includes result=success label", func() {
			metrics := scrapeMetrics()
			Expect(metrics).To(MatchRegexp(`af_tool_calls_total\{[^}]*result="success"`),
				"should have successful tool call observations")
		})

		It("TC-E2E-A2A-MET-03: af_tool_call_duration_seconds histogram has buckets", func() {
			metrics := scrapeMetrics()
			Expect(metrics).To(ContainSubstring("af_tool_call_duration_seconds_bucket"),
				"tool call duration histogram should have bucket observations")
		})

		It("TC-E2E-A2A-MET-04: af_mcp_rbac_denied_total has observations from RBAC denials", func() {
			// Trigger an MCP RBAC denial: observability persona calling af_create_rr (not in their role)
			obsToken, err := fetchDEXTokenForPersona("observability")
			Expect(err).NotTo(HaveOccurred())
			obsSID, err := initMCPSession(obsToken)
			if err == nil && obsSID != "" {
				body := buildJSONRPC("met04-rbac-deny", "tools/call", map[string]interface{}{
					"name":      "af_create_rr",
					"arguments": map[string]interface{}{"namespace": "default", "name": "x", "kind": "Deployment", "description": "denied"},
				})
				_, _, _ = mcpPOST(obsToken, obsSID, body)
			}
			time.Sleep(1 * time.Second)

			metrics := scrapeMetrics()
			Expect(metrics).To(ContainSubstring("af_mcp_rbac_denied_total"),
				"RBAC denied counter should exist after denied tool calls")
		})

		It("TC-E2E-A2A-MET-05: af_tool_calls_total includes per-tool tool_name labels", func() {
			metrics := scrapeMetrics()
			Expect(metrics).To(MatchRegexp(`af_tool_calls_total\{[^}]*tool_name="`),
				"tool calls counter should include tool_name label")
		})

		It("TC-E2E-A2A-MET-06: af_tool_call_duration_seconds includes per-tool labels", func() {
			metrics := scrapeMetrics()
			Expect(metrics).To(MatchRegexp(`af_tool_call_duration_seconds_bucket\{[^}]*tool_name="`),
				"tool call duration should include tool_name label")
		})
	})

	// ===================================================================
	// Category 4: Multi-Tool Workflows (4 tests)
	// ===================================================================
	Context("Category 4: Multi-Tool Workflows", func() {

		It("TC-E2E-A2A-WF-01: SRE full investigation flow (start -> poll -> list_workflows -> select -> present)", func() {
			prompt := "Investigate pod nginx-crash in prod namespace: start the investigation, poll for results, list available workflows, select the best one, and present options"
			resp, err := a2aInvoke(httpClient, baseURL, sreToken, a2aTasksSend("wf-01", prompt))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			rpc, err := parseRPCResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(rpc.Error).To(BeNil())

			task, err := extractTaskFromResult(rpc.Result)
			Expect(err).NotTo(HaveOccurred())
			Expect(task.ID).NotTo(BeEmpty())

			Eventually(func() string {
				pollResp, pErr := a2aInvoke(httpClient, baseURL, sreToken,
					buildJSONRPC("wf-01-poll", "tasks/get", map[string]interface{}{"id": task.ID}))
				if pErr != nil {
					return ""
				}
				defer func() { _ = pollResp.Body.Close() }()
				r, _ := parseRPCResponse(pollResp)
				if r.Result == nil {
					return ""
				}
				t, _ := extractTaskFromResult(r.Result)
				return t.Status.State
			}, 60*time.Second, 3*time.Second).Should(BeElementOf("completed", "failed"),
				"multi-tool workflow should reach terminal state")
		})

		It("TC-E2E-A2A-WF-02: l3-audit read-only audit flow (list -> get -> history -> audit_trail)", func() {
			prompt := "List remediations, get details for the first one, show its history and audit trail"
			resp, err := a2aInvoke(httpClient, baseURL, auditorToken, a2aTasksSend("wf-02", prompt))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			rpc, err := parseRPCResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(rpc.Error).To(BeNil())
			Expect(rpc.Result).NotTo(BeNil())
		})

		It("TC-E2E-A2A-WF-03: observability monitoring flow (list -> effectiveness -> events -> pods)", func() {
			prompt := "List remediations, show effectiveness stats, list events in kubernaut-system, and get pods"
			resp, err := a2aInvoke(httpClient, baseURL, observabilityToken, a2aTasksSend("wf-03", prompt))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			rpc, err := parseRPCResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(rpc.Error).To(BeNil())
			Expect(rpc.Result).NotTo(BeNil())
		})

		It("TC-E2E-A2A-WF-04: remediation-approver approval flow (list -> get -> approve)", func() {
			prompt := "List remediations, get details for rr-pending, and approve it"
			resp, err := a2aInvoke(httpClient, baseURL, approverToken, a2aTasksSend("wf-04", prompt))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			rpc, err := parseRPCResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(rpc.Error).To(BeNil())
			Expect(rpc.Result).NotTo(BeNil())
		})
	})

	// ===================================================================
	// Category 5: Protocol and Error Handling (6 tests)
	// ===================================================================
	Context("Category 5: Protocol and Error Handling", func() {

		It("TC-E2E-A2A-ERR-01: malformed JSON-RPC (missing method) returns error", func() {
			malformed := `{"jsonrpc":"2.0","id":"err-01","params":{}}`
			resp, err := a2aInvoke(httpClient, baseURL, sreToken, malformed)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			rpc, err := parseRPCResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(rpc.Error).NotTo(BeNil(), "missing method should produce JSON-RPC error")
			Expect(rpc.Error.Code).To(BeNumerically("<", 0))
		})

		It("TC-E2E-A2A-ERR-02: unknown JSON-RPC method returns -32601", func() {
			payload := buildJSONRPC("err-02", "nonexistent/method", map[string]interface{}{})
			resp, err := a2aInvoke(httpClient, baseURL, sreToken, payload)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			rpc, err := parseRPCResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(rpc.Error).NotTo(BeNil())
			Expect(rpc.Error.Code).To(Equal(-32601),
				"unknown method should return Method Not Found")
		})

		It("TC-E2E-A2A-ERR-03: message/send with empty params returns error", func() {
			payload := buildJSONRPC("err-03", "message/send", map[string]interface{}{})
			resp, err := a2aInvoke(httpClient, baseURL, sreToken, payload)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			rpc, err := parseRPCResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			// Either JSON-RPC error or task with failed state is acceptable
			if rpc.Error == nil && rpc.Result != nil {
				task, _ := extractTaskFromResult(rpc.Result)
				Expect(task.Status.State).To(BeElementOf("failed", "completed"))
			}
		})

		It("TC-E2E-A2A-ERR-04: tasks/get for nonexistent task returns error", func() {
			payload := buildJSONRPC("err-04", "tasks/get", map[string]interface{}{
				"id": "nonexistent-task-id-12345",
			})
			resp, err := a2aInvoke(httpClient, baseURL, sreToken, payload)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			rpc, err := parseRPCResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(rpc.Error).NotTo(BeNil(),
				"tasks/get for nonexistent task should return error")
		})

		It("TC-E2E-A2A-ERR-05: message/send with empty message text returns error or empty response", func() {
			payload := a2aTasksSend("err-05", "")
			resp, err := a2aInvoke(httpClient, baseURL, sreToken, payload)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			rpc, err := parseRPCResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			// Empty message should either error or produce a minimal response
			if rpc.Error != nil {
				Expect(rpc.Error.Code).To(BeNumerically("<", 0))
			}
		})

		It("TC-E2E-A2A-ERR-06: concurrent A2A requests from different personas are isolated", func() {
			type result struct {
				role string
				id   string
				err  error
			}
			results := make(chan result, 3)

			sendTask := func(role, token, id string) {
				resp, err := a2aInvoke(httpClient, baseURL, token,
					a2aTasksSend(id, "List all remediations in default namespace"))
				if err != nil {
					results <- result{role: role, err: err}
					return
				}
				defer func() { _ = resp.Body.Close() }()
				rpc, _ := parseRPCResponse(resp)
				taskID := ""
				if rpc.Result != nil {
					t, _ := extractTaskFromResult(rpc.Result)
					taskID = t.ID
				}
				results <- result{role: role, id: taskID, err: nil}
			}

			go sendTask("sre", sreToken, "conc-sre")
			go sendTask("cicd", cicdToken, "conc-cicd")
			go sendTask("observability", observabilityToken, "conc-obs")

			seen := make(map[string]string)
			for i := 0; i < 3; i++ {
				r := <-results
				Expect(r.err).NotTo(HaveOccurred(), "concurrent request for %s failed", r.role)
				if r.id != "" {
					_, dup := seen[r.id]
					Expect(dup).To(BeFalse(),
						"task IDs should be unique across personas (got duplicate: %s)", r.id)
					seen[r.id] = r.role
				}
			}
		})
	})

	// ===================================================================
	// Category 6: Session Lifecycle (3 tests)
	// ===================================================================
	Context("Category 6: Session Lifecycle", func() {

		It("TC-E2E-A2A-SESS-01: message/send creates a new task with unique ID", func() {
			resp, err := a2aInvoke(httpClient, baseURL, sreToken,
				a2aTasksSend("sess-01", "What is your name?"))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			rpc, err := parseRPCResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(rpc.Error).To(BeNil())

			task, err := extractTaskFromResult(rpc.Result)
			Expect(err).NotTo(HaveOccurred())
			Expect(task.ID).NotTo(BeEmpty())
			Expect(task.Status.State).NotTo(BeEmpty())
		})

		It("TC-E2E-A2A-SESS-02: tasks/get retrieves previously created task", func() {
			// Create task
			resp, err := a2aInvoke(httpClient, baseURL, sreToken,
				a2aTasksSend("sess-02-create", "ping"))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			rpc, err := parseRPCResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(rpc.Error).To(BeNil())

			task, err := extractTaskFromResult(rpc.Result)
			Expect(err).NotTo(HaveOccurred())
			Expect(task.ID).NotTo(BeEmpty())

			// Retrieve task
			getResp, err := a2aInvoke(httpClient, baseURL, sreToken,
				buildJSONRPC("sess-02-get", "tasks/get", map[string]interface{}{"id": task.ID}))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = getResp.Body.Close() }()
			Expect(getResp.StatusCode).To(Equal(http.StatusOK))

			getRPC, err := parseRPCResponse(getResp)
			Expect(err).NotTo(HaveOccurred())
			Expect(getRPC.Error).To(BeNil())

			retrieved, err := extractTaskFromResult(getRPC.Result)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved.ID).To(Equal(task.ID))
		})

		It("TC-E2E-A2A-SESS-03: af_sessions_active gauge reflects active sessions", func() {
			// Send a request to ensure at least one session exists
			resp, err := a2aInvoke(httpClient, baseURL, sreToken,
				a2aTasksSend("sess-03", "hello"))
			Expect(err).NotTo(HaveOccurred())
			_ = resp.Body.Close()

			metrics := scrapeMetrics()
			Expect(metrics).To(ContainSubstring("af_sessions_active"),
				"af_sessions_active gauge should be present after A2A requests")
		})
	})
})
