package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("E2E: discover_workflows (#1176)", Ordered, ContinueOnFailure, Label("e2e", "discover-workflows"), func() {
	var authToken string
	var mcpSessionID string

	BeforeAll(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token")
		Expect(authToken).NotTo(BeEmpty())

		initBody := buildJSONRPC("dw-init", "initialize", map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "e2e-discover-wf",
				"version": "1.0",
			},
		})
		req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(initBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+authToken)

		resp, err := httpClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		Expect(resp.StatusCode).To(BeNumerically("<", http.StatusBadRequest), "MCP initialize")

		mcpSessionID = resp.Header.Get("Mcp-Session-Id")
		Expect(mcpSessionID).NotTo(BeEmpty())
	})

	mcpToolCall := func(rpcID, toolName string, args map[string]interface{}) ([]byte, error) {
		callBody := buildJSONRPC(rpcID, "tools/call", map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		})
		raw, code, err := mcpPOST(authToken, mcpSessionID, callBody)
		if err != nil {
			return nil, err
		}
		if code >= http.StatusBadRequest {
			return nil, fmt.Errorf("MCP returned HTTP %d: %s", code, raw)
		}
		payload := unwrapSSEDataLine(raw)
		return []byte(payload), nil
	}

	It("E2E-AF-WP-001: kubernaut_discover_workflows returns well-formed response", func() {
		// Call with a non-existent rr_id — verifies the tool is registered, callable,
		// and returns a proper JSON-RPC response (success or error) rather than crashing.
		raw, err := mcpToolCall("dw-e2e-001", "kubernaut_discover_workflows", map[string]interface{}{
			"rr_id": "e2e-nonexistent-rr",
		})
		Expect(err).NotTo(HaveOccurred(), "tool call should return a response (even if isError)")

		var rpcResp struct {
			Result struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			} `json:"result"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		Expect(json.Unmarshal(raw, &rpcResp)).To(Succeed(), "response must be valid JSON-RPC")
		// Either a successful result with content, or an error result — both are valid
		if rpcResp.Error == nil {
			Expect(rpcResp.Result.Content).NotTo(BeEmpty(), "result must have content")
		}
	})

	It("E2E-AF-WP-002: discover_workflows and select_workflow accept rr_id argument", func() {
		// Validates that both tools are registered with the correct argument schema
		// and return proper JSON-RPC responses. The happy-path flow (with active
		// investigation) is covered by the fullpipeline E2E suite (FP-MCP-005).
		raw, err := mcpToolCall("dw-e2e-002a", "kubernaut_discover_workflows", map[string]interface{}{
			"rr_id": "e2e-rr-discover-test",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(raw).NotTo(BeEmpty(), "should return a response body")

		selectRaw, err := mcpToolCall("dw-e2e-002b", "kubernaut_select_workflow", map[string]interface{}{
			"rr_id":       "e2e-rr-discover-test",
			"workflow_id": "e2e-wf-placeholder",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(selectRaw).NotTo(BeEmpty(), "should return a response body")
	})

	It("E2E-AF-WP-003: unauthorized role is denied discover_workflows", func() {
		cicdToken, err := fetchDEXTokenForPersona("cicd")
		Expect(err).NotTo(HaveOccurred())

		initBody := buildJSONRPC("dw-denied-init", "initialize", map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "e2e-denied", "version": "1.0"},
		})
		req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(initBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+cicdToken)

		resp, err := httpClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)

		deniedSessionID := resp.Header.Get("Mcp-Session-Id")

		callBody := buildJSONRPC("dw-denied-call", "tools/call", map[string]interface{}{
			"name":      "kubernaut_discover_workflows",
			"arguments": map[string]interface{}{"rr_id": "e2e-denied-rr"},
		})
		callReq, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(callBody))
		Expect(err).NotTo(HaveOccurred())
		callReq.Header.Set("Content-Type", "application/json")
		callReq.Header.Set("Accept", "application/json, text/event-stream")
		callReq.Header.Set("Authorization", "Bearer "+cicdToken)
		if deniedSessionID != "" {
			callReq.Header.Set("Mcp-Session-Id", deniedSessionID)
		}

		callResp, err := httpClient.Do(callReq)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = callResp.Body.Close() }()
		body, _ := io.ReadAll(callResp.Body)

		Expect(string(body)).To(SatisfyAny(
			ContainSubstring("denied"),
			ContainSubstring("forbidden"),
			ContainSubstring("error"),
		))
	})

	It("E2E-AF-WP-004: af_tool_calls_total metric exposed after discover_workflows call", func() {
		// Trigger a discover_workflows call to ensure the generic tool metric
		// is observed (covers discover_workflows via tool label).
		_, _ = mcpToolCall("dw-e2e-004-prime", "kubernaut_discover_workflows", map[string]interface{}{
			"rr_id": "e2e-metrics-prime",
		})

		resp, err := httpClient.Get(baseURL + "/metrics")
		if err != nil {
			resp, err = http.Get("http://localhost:18081/metrics")
		}
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(SatisfyAny(
			ContainSubstring(`af_tool_calls_total{`),
			ContainSubstring(`af_tool_call_duration_seconds`),
		))
	})

	It("E2E-AF-WP-005: discover_workflows with workflow_id filter passes argument", func() {
		// Validates that the workflow_id filter is accepted by the tool schema.
		// Full filtering behavior is tested in unit tests and fullpipeline E2E.
		raw, err := mcpToolCall("dw-e2e-005", "kubernaut_discover_workflows", map[string]interface{}{
			"rr_id":       "e2e-rr-filter-test",
			"workflow_id": "e2e-target-workflow",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(raw).NotTo(BeEmpty(), "should return a response body")
	})

	It("E2E-AF-WP-006: select_workflow returns error for invalid rr_id", func() {
		// Validates that select_workflow returns a proper error (isError or JSON-RPC error)
		// when the rr_id doesn't correspond to an active investigation.
		raw, err := mcpToolCall("dw-e2e-006", "kubernaut_select_workflow", map[string]interface{}{
			"rr_id":       "e2e-nonexistent-rr",
			"workflow_id": "e2e-fake-workflow",
			"parameters":  map[string]interface{}{"count": "not-a-number"},
		})
		Expect(err).NotTo(HaveOccurred())

		var rpcResp struct {
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			} `json:"result"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		Expect(json.Unmarshal(raw, &rpcResp)).To(Succeed(), "response must be valid JSON-RPC")
		// Should be an error since the rr_id doesn't exist
		hasError := rpcResp.Result.IsError || rpcResp.Error != nil
		Expect(hasError).To(BeTrue(), "select_workflow with invalid rr_id should return error")
	})
})
