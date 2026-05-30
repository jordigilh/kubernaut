package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

var _ = Describe("MCP Full-Path Validation (G1)", Label("e2e", "phase2", "g1"), func() {
	const g1RRDeployName = "e2e-mcp-test-deploy"

	var authToken string
	var mcpSessionID string

	BeforeEach(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token")
		Expect(authToken).NotTo(BeEmpty())

		mcpSessionID, err = initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred(), "MCP initialize")
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

	// TC-E2E-MCP-FULL-01 deleted: af_get_pods covered by A2A T15.

	// TC-E2E-MCP-FULL-02 + 04 collapsed: create RR → approve it
	It("TC-E2E-MCP-FULL-02+04: RR fixture then kubernaut_approve lifecycle", func() {
		const rrNamespace = "default"
		const rrName = "e2e-rr-mcp-full-02"

		By("TC-E2E-MCP-FULL-02: Create RR via k8s client CRD fixture")
		Expect(createRR(rrNamespace, rrName, "Deployment", g1RRDeployName)).To(Succeed())
		DeferCleanup(func() { deleteRR(rrNamespace, rrName) })

		By("TC-E2E-MCP-FULL-04: kubernaut_approve after RR exists")
		rarName := fmt.Sprintf("e2e-rar-g1-%d", time.Now().UnixNano())
		Expect(k8sClient.Create(context.Background(), buildRAR(rrNamespace, rarName, rrName))).To(Succeed())
		DeferCleanup(func() {
			rar := &remediationv1alpha1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: rarName, Namespace: rrNamespace},
			}
			_ = client.IgnoreNotFound(k8sClient.Delete(context.Background(), rar))
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

	It("TC-E2E-MCP-FULL-03: MCP tools/call -> kubernaut_list_workflows returns workflows from DS", func() {
		text, err := mcpToolCall("mcp-full-03", "kubernaut_list_workflows", map[string]interface{}{})
		Expect(err).NotTo(HaveOccurred(), "kubernaut_list_workflows: %s", text)

		var parsed map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &parsed)).To(Succeed())
		Expect(parsed).To(HaveKey("workflows"))
		wf, ok := parsed["workflows"].([]interface{})
		Expect(ok).To(BeTrue(), "workflows should be an array: %s", text)
		Expect(wf).NotTo(BeEmpty(),
			"DS must have seeded workflow entries in E2E — workflow catalog must not be empty")
	})

	It("TC-E2E-MCP-FULL-05: MCP tools/list returns exactly 22 domain tools", func() {
		root, err := mcpToolsList("mcp-full-05")
		Expect(err).NotTo(HaveOccurred())
		Expect(root).NotTo(HaveKey("error"))

		res, ok := root["result"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "tools/list should include result object: %#v", root)

		toolsRaw, ok := res["tools"].([]interface{})
		Expect(ok).To(BeTrue(), "result.tools should be an array: %#v", res)
		Expect(len(toolsRaw)).To(Equal(22))

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

	// TC-E2E-MCP-FULL-07 deleted: kubernaut_remediate validation (internal tool) — covered by UT.
})
