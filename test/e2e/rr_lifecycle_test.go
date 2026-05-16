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

func unwrapSSEDataLine(raw []byte) string {
	s := string(raw)
	if !strings.Contains(s, "data:") {
		return strings.TrimSpace(s)
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "data:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
	return strings.TrimSpace(s)
}

func initMCPSession(token string) (string, error) {
	body := buildJSONRPC("init-1", "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "e2e",
			"version": "1.0",
		},
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("MCP initialize: HTTP %d", resp.StatusCode)
	}
	return resp.Header.Get("Mcp-Session-Id"), nil
}

func mcpPOST(token, sessionID, jsonBody string) (body []byte, statusCode int, err error) {
	req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(jsonBody))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err = io.ReadAll(resp.Body)
	statusCode = resp.StatusCode
	return body, statusCode, err
}

func parseMCPToolPayload(payload string) (text string, toolIsError bool, err error) {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return "", false, fmt.Errorf("parse MCP JSON: %w", err)
	}
	if e, ok := root["error"]; ok && e != nil {
		return "", false, fmt.Errorf("json-rpc error: %v", e)
	}
	res, ok := root["result"].(map[string]interface{})
	if !ok {
		return payload, false, nil
	}
	toolIsError, _ = res["isError"].(bool)
	text = extractMCPResultText(root)
	return text, toolIsError, nil
}

func kubectlApplyYAML(manifest string) error {
	kubeconfigPath := getEnvOrDefault("KUBECONFIG", os.Getenv("HOME")+"/.kube/config")
	cmd := exec.CommandContext(context.Background(), "kubectl",
		"--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func remediationApprovalManifest(namespace, rarName, rrName string) string {
	now := time.Now().UTC().Format(time.RFC3339)
	return fmt.Sprintf(`apiVersion: kubernaut.ai/v1alpha1
kind: RemediationApprovalRequest
metadata:
  name: %s
  namespace: %s
spec:
  remediationRequestRef:
    name: %s
    namespace: %s
  aiAnalysisRef:
    name: e2e-analysis-%s
  confidence: 0.65
  confidenceLevel: medium
  investigationSummary: E2E RAR flow — RR %s
  reason: E2E approval gate
  whyApprovalRequired: E2E coverage G5
  requiredBy: "%s"
  recommendedActions:
    - action: RestartPod
      rationale: E2E recommended action
  recommendedWorkflow:
    workflowId: wf-restart-pod-v1
    version: 1.0.0
    executionBundle: ghcr.io/jordigilh/kubernaut/bundles/restart-pod@sha256:e2e
    rationale: E2E workflow
`, rarName, namespace, rrName, namespace, rarName, rrName, now)
}

var _ = Describe("RR CRD Lifecycle (G4)", Ordered, Label("e2e", "phase2", "g4"), func() {
	var authToken string
	var mcpSessionID string

	var rr01RRID string
	var rr06RRID string

	BeforeAll(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token")
		Expect(authToken).NotTo(BeEmpty())

		mcpSessionID, err = initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred(), "MCP initialize")
	})

	mcpToolCall := func(toolName string, args map[string]interface{}) (string, error) {
		callBody := buildJSONRPC(fmt.Sprintf("g4-%s-%d", toolName, time.Now().UnixNano()),
			"tools/call", map[string]interface{}{
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

	It("TC-E2E-RR-01: af_create_rr via MCP creates RR with expected target", func() {
		// CreateRRArgs use namespace, name, kind, description (target identity for the RR).
		text, err := mcpToolCall("af_create_rr", map[string]interface{}{
			"namespace":   "default",
			"name":        "test-deploy-rr01",
			"kind":        "Deployment",
			"description": "E2E test RR",
		})
		Expect(err).NotTo(HaveOccurred(), "af_create_rr: %s", text)
		Expect(strings.ToLower(text)).To(Or(
			ContainSubstring("created"),
			ContainSubstring("remediationrequest"),
		))
		Expect(text).To(ContainSubstring("rr_id"))

		rr01RRID = parseJSONStringField(text, "rr_id")
		Expect(rr01RRID).NotTo(BeEmpty())

		var parsed map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &parsed)).To(Succeed())
		tr, _ := parsed["targetResource"].(map[string]interface{})
		if tr != nil {
			Expect(tr["name"]).To(Equal("test-deploy-rr01"))
			Expect(tr["kind"]).To(Equal("Deployment"))
		} else {
			// Some responses only surface rr_id + message; RR name encodes target.
			Expect(text).To(ContainSubstring("test-deploy-rr01"))
		}
	})

	It("TC-E2E-RR-02: af_check_existing_rr finds RR for same fingerprint", func() {
		Expect(rr01RRID).NotTo(BeEmpty(), "run TC-E2E-RR-01 first")

		text, err := mcpToolCall("af_check_existing_rr", map[string]interface{}{
			"namespace": "default",
			"kind":      "Deployment",
			"name":      "test-deploy-rr01",
		})
		Expect(err).NotTo(HaveOccurred(), text)

		var out map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &out)).To(Succeed())
		Expect(out["exists"]).To(BeTrue(), "non-terminal RR should exist for fingerprint")
		Expect(out["rr_id"]).To(Equal(rr01RRID))
	})

	It("TC-E2E-RR-03: kubernaut_cancel_remediation sets RR to Cancelled", func() {
		Expect(rr01RRID).NotTo(BeEmpty())

		rrName := rrNameFromRRID(rr01RRID)
		Expect(rrName).NotTo(BeEmpty())

		text, err := mcpToolCall("kubernaut_cancel_remediation", map[string]interface{}{
			"namespace": "default",
			"name":      rrName,
		})
		Expect(err).NotTo(HaveOccurred(), text)
		Expect(strings.ToLower(text)).To(Or(
			ContainSubstring("cancel"),
			ContainSubstring("cancelled"),
		))
	})

	It("TC-E2E-RR-04: kubernaut_list_remediations returns the RR", func() {
		Expect(rr01RRID).NotTo(BeEmpty())

		text, err := mcpToolCall("kubernaut_list_remediations", map[string]interface{}{
			"namespace": "default",
		})
		Expect(err).NotTo(HaveOccurred(), text)

		var out map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &out)).To(Succeed())
		rem, ok := out["remediations"].([]interface{})
		Expect(ok).To(BeTrue(), "list result should include remediations array")
		Expect(len(rem)).To(BeNumerically(">=", 1))

		joined := strings.ToLower(text)
		Expect(joined).To(ContainSubstring(strings.ToLower(rrNameFromRRID(rr01RRID))))
	})

	It("TC-E2E-RR-05: kubernaut_get_remediation returns detail for RR", func() {
		Expect(rr01RRID).NotTo(BeEmpty())

		text, err := mcpToolCall("kubernaut_get_remediation", map[string]interface{}{
			"rr_id": rr01RRID,
		})
		Expect(err).NotTo(HaveOccurred(), text)

		var out map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &out)).To(Succeed())
		Expect(out).To(HaveKey("namespace"))
		Expect(out).To(HaveKey("name"))
		kind, ok := out["kind"].(string)
		Expect(ok).To(BeTrue(), "kind should be a string")
		Expect(kind).NotTo(BeEmpty())
		target, ok := out["target"].(string)
		Expect(ok).To(BeTrue(), "target should be a string")
		Expect(target).NotTo(BeEmpty())
	})

	It("TC-E2E-RR-06: af_create_rr twice is idempotent (dedup / already_exists)", func() {
		args := map[string]interface{}{
			"namespace":   "default",
			"name":        "test-deploy-rr06",
			"kind":        "Deployment",
			"description": "E2E idempotent RR",
		}
		first, err := mcpToolCall("af_create_rr", args)
		Expect(err).NotTo(HaveOccurred(), first)

		second, err := mcpToolCall("af_create_rr", args)
		Expect(err).NotTo(HaveOccurred(), second)

		id1 := parseJSONStringField(first, "rr_id")
		id2 := parseJSONStringField(second, "rr_id")
		Expect(id1).NotTo(BeEmpty())
		Expect(id2).NotTo(BeEmpty())
		Expect(id1).To(Equal(id2), "second create should return the same rr_id")

		var s2 map[string]interface{}
		_ = json.Unmarshal([]byte(second), &s2)
		if already, ok := s2["already_exists"].(bool); ok {
			Expect(already).To(BeTrue())
		}
		// Track RR name for watch
		rr06RRID = id1
	})

	It("TC-E2E-RR-07: kubernaut_watch returns structured watch result", func() {
		Expect(rr06RRID).NotTo(BeEmpty(), "run TC-E2E-RR-06 first")

		name := rrNameFromRRID(rr06RRID)
		text, err := mcpToolCall("kubernaut_watch", map[string]interface{}{
			"namespace": "default",
			"name":      name,
		})
		Expect(err).NotTo(HaveOccurred(), text)

		var out map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &out)).To(Succeed())
		Expect(out).To(HaveKey("events"))
		Expect(out).To(HaveKey("status"))
	})

	It("TC-E2E-RR-08: af_create_rr with empty name returns validation error", func() {
		raw, code, err := mcpPOST(authToken, mcpSessionID,
			buildJSONRPC(fmt.Sprintf("g4-invalid-%d", time.Now().UnixNano()), "tools/call",
				map[string]interface{}{
					"name": "af_create_rr",
					"arguments": map[string]interface{}{
						"namespace":   "default",
						"name":        "",
						"kind":        "Deployment",
						"description": "invalid",
					},
				}))
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(BeNumerically("<", 400))

		payload := unwrapSSEDataLine(raw)
		text, toolErr, perr := parseMCPToolPayload(payload)
		Expect(perr).NotTo(HaveOccurred())
		Expect(toolErr).To(BeTrue())
		Expect(strings.ToLower(text)).To(Or(
			ContainSubstring("name must not be empty"),
			ContainSubstring("invalid"),
			ContainSubstring("error"),
		))
	})

	It("TC-E2E-RR-09: af_create_rr denied for observability (read-only) persona", func() {
		obsToken, err := fetchDEXTokenForPersona("observability")
		Expect(err).NotTo(HaveOccurred())

		obsSession, err := initMCPSession(obsToken)
		Expect(err).NotTo(HaveOccurred())

		raw, code, err := mcpPOST(obsToken, obsSession, buildJSONRPC(
			fmt.Sprintf("g4-rbac-%d", time.Now().UnixNano()), "tools/call",
			map[string]interface{}{
				"name": "af_create_rr",
				"arguments": map[string]interface{}{
					"namespace":   "default",
					"name":        "test-deploy-rr09",
					"kind":        "Deployment",
					"description": "should be denied",
				},
			}))
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(BeNumerically("<", 400))

		payload := unwrapSSEDataLine(raw)
		text, toolErr, perr := parseMCPToolPayload(payload)
		Expect(perr).NotTo(HaveOccurred())
		Expect(toolErr).To(BeTrue())
		Expect(strings.ToLower(text)).To(Or(
			ContainSubstring("denied"),
			ContainSubstring("permission"),
			ContainSubstring("forbidden"),
			ContainSubstring("not allowed"),
		))
	})
})

var _ = Describe("RAR Flow (G5)", Ordered, Label("e2e", "phase2", "g5"), func() {

	const rrNamespace = "default"

	It("TC-E2E-RAR-01: kubernaut_approve succeeds for RAR referencing existing RR", func() {
		sreTok, err := fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		sreSession, err := initMCPSession(sreTok)
		Expect(err).NotTo(HaveOccurred())

		createBody := buildJSONRPC("g5-01-create", "tools/call", map[string]interface{}{
			"name": "af_create_rr",
			"arguments": map[string]interface{}{
				"namespace":   rrNamespace,
				"name":        "test-deploy-rar01",
				"kind":        "Deployment",
				"description": "E2E RAR flow — RR",
			},
		})
		raw, code, err := mcpPOST(sreTok, sreSession, createBody)
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(BeNumerically("<", 400))
		text, _, perr := parseMCPToolPayload(unwrapSSEDataLine(raw))
		Expect(perr).NotTo(HaveOccurred())
		rrID := parseJSONStringField(text, "rr_id")
		Expect(rrID).NotTo(BeEmpty())
		rrName := rrNameFromRRID(rrID)

		rarName := "e2e-rar-g5-01"
		Expect(kubectlApplyYAML(remediationApprovalManifest(rrNamespace, rarName, rrName))).To(Succeed())
		DeferCleanup(func() {
			kubeconfigPath := getEnvOrDefault("KUBECONFIG", os.Getenv("HOME")+"/.kube/config")
			_, _ = exec.CommandContext(context.Background(), "kubectl", "--kubeconfig", kubeconfigPath,
				"delete", "remediationapprovalrequest", rarName, "-n", rrNamespace, "--ignore-not-found").CombinedOutput()
		})

		approverTok, err := fetchDEXTokenForPersona("remediation-approver")
		Expect(err).NotTo(HaveOccurred())
		approverSession, err := initMCPSession(approverTok)
		Expect(err).NotTo(HaveOccurred())

		apBody := buildJSONRPC("g5-01-approve", "tools/call", map[string]interface{}{
			"name": "kubernaut_approve",
			"arguments": map[string]interface{}{
				"namespace": rrNamespace,
				"rar_name":  rarName,
				"decision":  "approved",
				"reason":    "E2E G5 approval",
			},
		})
		araw, acode, err := mcpPOST(approverTok, approverSession, apBody)
		Expect(err).NotTo(HaveOccurred())
		Expect(acode).To(BeNumerically("<", 400))
		atext, toolErr, aperr := parseMCPToolPayload(unwrapSSEDataLine(araw))
		Expect(aperr).NotTo(HaveOccurred())
		Expect(toolErr).To(BeFalse(), "approve should succeed: %s", atext)
		Expect(strings.ToLower(atext)).To(Or(
			ContainSubstring("approved"),
			ContainSubstring("approval"),
		))
	})

	It("TC-E2E-RAR-02: kubernaut_approve on non-existent RAR returns error", func() {
		tok, err := fetchDEXTokenForPersona("remediation-approver")
		Expect(err).NotTo(HaveOccurred())
		sid, err := initMCPSession(tok)
		Expect(err).NotTo(HaveOccurred())

		body := buildJSONRPC("g5-02", "tools/call", map[string]interface{}{
			"name": "kubernaut_approve",
			"arguments": map[string]interface{}{
				"namespace": rrNamespace,
				"rar_name":  "e2e-rar-does-not-exist-xyz",
				"decision":  "approved",
			},
		})
		raw, code, err := mcpPOST(tok, sid, body)
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(BeNumerically("<", 400))

		text, toolErr, perr := parseMCPToolPayload(unwrapSSEDataLine(raw))
		Expect(perr).NotTo(HaveOccurred())
		Expect(toolErr).To(BeTrue())
		Expect(strings.ToLower(text)).To(Or(
			ContainSubstring("not found"),
			ContainSubstring("error"),
			ContainSubstring("fail"),
		))
	})

	It("TC-E2E-RAR-03: sre persona may kubernaut_approve (RBAC includes tool)", func() {
		sreTok, err := fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		sreSession, err := initMCPSession(sreTok)
		Expect(err).NotTo(HaveOccurred())

		createBody := buildJSONRPC("g5-03-create", "tools/call", map[string]interface{}{
			"name": "af_create_rr",
			"arguments": map[string]interface{}{
				"namespace":   rrNamespace,
				"name":        "test-deploy-rar03",
				"kind":        "Deployment",
				"description": "E2E SRE approve path",
			},
		})
		raw, code, err := mcpPOST(sreTok, sreSession, createBody)
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(BeNumerically("<", 400))
		text, _, perr := parseMCPToolPayload(unwrapSSEDataLine(raw))
		Expect(perr).NotTo(HaveOccurred())
		rrID := parseJSONStringField(text, "rr_id")
		Expect(rrID).NotTo(BeEmpty())
		rrName := rrNameFromRRID(rrID)

		rarName := "e2e-rar-g5-03"
		Expect(kubectlApplyYAML(remediationApprovalManifest(rrNamespace, rarName, rrName))).To(Succeed())
		DeferCleanup(func() {
			kubeconfigPath := getEnvOrDefault("KUBECONFIG", os.Getenv("HOME")+"/.kube/config")
			_, _ = exec.CommandContext(context.Background(), "kubectl", "--kubeconfig", kubeconfigPath,
				"delete", "remediationapprovalrequest", rarName, "-n", rrNamespace, "--ignore-not-found").CombinedOutput()
		})

		apBody := buildJSONRPC("g5-03-approve", "tools/call", map[string]interface{}{
			"name": "kubernaut_approve",
			"arguments": map[string]interface{}{
				"namespace": rrNamespace,
				"rar_name":  rarName,
				"decision":  "approved",
			},
		})
		araw, acode, err := mcpPOST(sreTok, sreSession, apBody)
		Expect(err).NotTo(HaveOccurred())
		Expect(acode).To(BeNumerically("<", 400))
		atext, toolErr, aperr := parseMCPToolPayload(unwrapSSEDataLine(araw))
		Expect(aperr).NotTo(HaveOccurred())
		Expect(toolErr).To(BeFalse(), "SRE should be allowed to approve: %s", atext)
		Expect(strings.ToLower(atext)).To(ContainSubstring("approved"))
	})
})

func parseJSONStringField(text, field string) string {
	var m map[string]interface{}
	if json.Unmarshal([]byte(text), &m) != nil {
		return ""
	}
	s, _ := m[field].(string)
	return s
}

func rrNameFromRRID(rrid string) string {
	parts := strings.SplitN(rrid, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}
