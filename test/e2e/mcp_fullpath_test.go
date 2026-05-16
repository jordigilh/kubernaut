package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MCP Full-Path Validation (G1)", Ordered, Label("e2e", "phase2", "g1"), func() {
	const g1RRDeployName = "e2e-mcp-test-deploy"

	var authToken string
	var mcpSessionID string
	var g1RRID string

	BeforeAll(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token")
		Expect(authToken).NotTo(BeEmpty())

		initBody := buildJSONRPC("mcp-init", "initialize", map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "e2e-mcp-full",
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

	mcpToolCall := func(rpcID, toolName string, args map[string]interface{}) (string, error) {
		callBody := buildJSONRPC(rpcID, "tools/call", map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		})
		raw, code, err := mcpPOST(authToken, mcpSessionID, callBody)
		if err != nil {
			return "", err
		}
		if code >= http.StatusBadRequest {
			return "", fmt.Errorf("HTTP %d: %s", code, string(raw))
		}
		payload := unwrapSSEDataLine(raw)
		text, toolErr, parseErr := parseMCPToolPayload(payload)
		if parseErr != nil {
			return text, parseErr
		}
		if toolErr {
			return text, fmt.Errorf("%s", text)
		}
		return text, nil
	}

	mcpToolsList := func(rpcID string) (map[string]interface{}, error) {
		listBody := buildJSONRPC(rpcID, "tools/list", map[string]interface{}{})
		raw, code, err := mcpPOST(authToken, mcpSessionID, listBody)
		if err != nil {
			return nil, err
		}
		if code >= http.StatusBadRequest {
			return nil, fmt.Errorf("HTTP %d: %s", code, string(raw))
		}
		payload := unwrapSSEDataLine(raw)
		var root map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &root); err != nil {
			return nil, fmt.Errorf("parse tools/list JSON: %w", err)
		}
		return root, nil
	}

	mcpToolCallRaw := func(rpcID, toolName string, args map[string]interface{}) ([]byte, int, error) {
		callBody := buildJSONRPC(rpcID, "tools/call", map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		})
		return mcpPOST(authToken, mcpSessionID, callBody)
	}

	It("TC-E2E-MCP-FULL-01: MCP tools/call -> af_get_pods returns pod data for kubernaut-system", func() {
		text, err := mcpToolCall("mcp-full-01", "af_get_pods", map[string]interface{}{
			"namespace": "kubernaut-system",
		})
		Expect(err).NotTo(HaveOccurred(), "af_get_pods: %s", text)

		lower := strings.ToLower(text)
		Expect(lower).To(Or(
			ContainSubstring("apifrontend"),
			ContainSubstring("mock-llm"),
			ContainSubstring("dex"),
		), "expected AF stack pod data in result: %s", text)
	})

	It("TC-E2E-MCP-FULL-02: MCP tools/call -> af_create_rr succeeds for test deployment", func() {
		text, err := mcpToolCall("mcp-full-02", "af_create_rr", map[string]interface{}{
			"namespace":   "default",
			"name":        g1RRDeployName,
			"kind":        "Deployment",
			"description": "MCP E2E test",
		})
		Expect(err).NotTo(HaveOccurred(), "af_create_rr: %s", text)
		Expect(strings.ToLower(text)).To(Or(
			ContainSubstring("created"),
			ContainSubstring("remediationrequest"),
			ContainSubstring("exists"),
		))

		g1RRID = parseJSONStringField(text, "rr_id")
		Expect(g1RRID).NotTo(BeEmpty())
	})

	It("TC-E2E-MCP-FULL-03: MCP tools/call -> kubernaut_list_workflows returns workflows from DS", func() {
		text, err := mcpToolCall("mcp-full-03", "kubernaut_list_workflows", map[string]interface{}{})
		Expect(err).NotTo(HaveOccurred(), "kubernaut_list_workflows: %s", text)

		var parsed map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &parsed)).To(Succeed())
		Expect(parsed).To(HaveKey("workflows"))
		wf, ok := parsed["workflows"].([]interface{})
		Expect(ok).To(BeTrue(), "workflows should be an array: %s", text)
		if len(wf) == 0 {
			Skip("DS has no seeded workflow entries — workflow catalog empty in this E2E environment")
		}
	})

	It("TC-E2E-MCP-FULL-04: MCP tools/call -> kubernaut_approve after RR exists creates successful approval", func() {
		Expect(g1RRID).NotTo(BeEmpty(), "RR from TC-E2E-MCP-FULL-02 must exist")
		rrNamespace := "default"
		rrName := rrNameFromRRID(g1RRID)
		Expect(rrName).NotTo(BeEmpty())

		rarName := fmt.Sprintf("e2e-rar-g1-%d", time.Now().UnixNano())
		Expect(kubectlApplyYAML(remediationApprovalManifest(rrNamespace, rarName, rrName))).To(Succeed())
		DeferCleanup(func() {
			kubeconfigPath := os.Getenv("HOME") + "/.kube/apifrontend-e2e-config"
			_, _ = exec.CommandContext(context.Background(), "kubectl", "--kubeconfig", kubeconfigPath,
				"delete", "remediationapprovalrequest", rarName, "-n", rrNamespace, "--ignore-not-found").CombinedOutput()
		})

		approverTok, err := fetchDEXTokenForPersona("remediation-approver")
		Expect(err).NotTo(HaveOccurred())
		approverSession, err := initMCPSession(approverTok)
		Expect(err).NotTo(HaveOccurred())

		apBody := buildJSONRPC("mcp-full-04-approve", "tools/call", map[string]interface{}{
			"name": "kubernaut_approve",
			"arguments": map[string]interface{}{
				"namespace": rrNamespace,
				"rar_name":  rarName,
				"decision":  "approved",
				"reason":    "MCP G1 full-path E2E",
			},
		})
		araw, acode, err := mcpPOST(approverTok, approverSession, apBody)
		Expect(err).NotTo(HaveOccurred())
		Expect(acode).To(BeNumerically("<", http.StatusBadRequest))
		atext, toolErr, aperr := parseMCPToolPayload(unwrapSSEDataLine(araw))
		Expect(aperr).NotTo(HaveOccurred())
		Expect(toolErr).To(BeFalse(), "kubernaut_approve should succeed: %s", atext)
		Expect(strings.ToLower(atext)).To(Or(
			ContainSubstring("approved"),
			ContainSubstring("approval"),
		))
	})

	It("TC-E2E-MCP-FULL-05: MCP tools/list returns exactly 19 tools", func() {
		root, err := mcpToolsList("mcp-full-05")
		Expect(err).NotTo(HaveOccurred())
		Expect(root).NotTo(HaveKey("error"))

		res, ok := root["result"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "tools/list should include result object: %#v", root)

		toolsRaw, ok := res["tools"].([]interface{})
		Expect(ok).To(BeTrue(), "result.tools should be an array: %#v", res)
		Expect(len(toolsRaw)).To(Equal(19))

		for _, t := range toolsRaw {
			tm, ok := t.(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(tm).To(HaveKey("name"))
			Expect(tm).To(HaveKey("inputSchema"))
		}
	})

	It("TC-E2E-MCP-FULL-06: MCP tools/call with unknown tool returns error (JSON-RPC or CallToolResult.isError)", func() {
		raw, code, err := mcpToolCallRaw("mcp-full-06", "nonexistent_tool_xyz", map[string]interface{}{})
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(BeNumerically("<", http.StatusBadRequest))

		payload := unwrapSSEDataLine(raw)
		var root map[string]interface{}
		Expect(json.Unmarshal([]byte(payload), &root)).To(Succeed())

		if root["error"] != nil {
			return
		}

		text, toolIsErr, perr := parseMCPToolPayload(payload)
		Expect(perr).NotTo(HaveOccurred())
		Expect(toolIsErr).To(BeTrue(), "expected tool error for unknown tool; text=%q", text)
	})

	It("TC-E2E-MCP-FULL-07: MCP tools/call af_create_rr without required name yields validation error", func() {
		raw, code, err := mcpToolCallRaw("mcp-full-07", "af_create_rr", map[string]interface{}{
			"namespace":   "default",
			"kind":        "Deployment",
			"description": "missing name field",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(BeNumerically("<", http.StatusBadRequest))

		payload := unwrapSSEDataLine(raw)
		text, toolIsErr, perr := parseMCPToolPayload(payload)

		var root map[string]interface{}
		_ = json.Unmarshal([]byte(payload), &root)
		rpcErr, rpcErrSet := root["error"]
		hasRPCError := rpcErrSet && rpcErr != nil

		Expect(hasRPCError || perr != nil || toolIsErr).To(BeTrue(), "expected JSON-RPC error or tool/validation error, payload=%q", payload)

		diag := strings.ToLower(payload)
		if text != "" {
			diag += " " + strings.ToLower(text)
		}
		if perr != nil {
			diag += " " + strings.ToLower(perr.Error())
		}
		Expect(diag).To(Or(
			ContainSubstring("invalid"),
			ContainSubstring("required"),
			ContainSubstring("validation"),
			ContainSubstring("parameter"),
			ContainSubstring("name"),
		))
	})
})
