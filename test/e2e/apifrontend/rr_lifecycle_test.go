package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func kubectlApplyYAML(manifest string) error {
	kubeconfigPath := os.Getenv("HOME") + "/.kube/apifrontend-e2e-config"
	cmd := exec.CommandContext(context.Background(), "kubectl", //nolint:gosec // G702: kubeconfig path from controlled E2E env
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

var _ = Describe("RR CRD Lifecycle (G4)", Label("e2e", "phase2", "g4"), func() {

	// mcpToolCallWith creates a short-lived MCP session and invokes a tool.
	// Each test gets its own session for parallel safety.
	mcpToolCallWith := func(token, sessionID, toolName string, args map[string]interface{}) (string, error) {
		callBody := buildJSONRPC(fmt.Sprintf("g4-%s-%d", toolName, time.Now().UnixNano()),
			"tools/call", map[string]interface{}{
				"name":      toolName,
				"arguments": args,
			})
		raw, code, err := mcpPOST(token, sessionID, callBody)
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

	// TC-E2E-RR-01..05 collapsed: create → cancel → list → get detail
	It("TC-E2E-RR-01..05: RR create → cancel → list → get lifecycle", func() {
		authToken, err := fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		mcpSessionID, err := initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred())

		const rrName = "e2e-rr-lifecycle-01"
		const rrNamespace = "default"
		rrID := rrNamespace + "/" + rrName

		By("TC-E2E-RR-01: Create RR via kubectl CRD fixture")
		Expect(kubectlCreateRR(rrNamespace, rrName, "Deployment", "test-deploy-rr01")).To(Succeed())
		DeferCleanup(func() { kubectlDeleteRR(rrNamespace, rrName) })

		By("TC-E2E-RR-03: kubernaut_cancel_remediation sets RR to Cancelled")
		text, err := mcpToolCallWith(authToken, mcpSessionID, "kubernaut_cancel_remediation", map[string]interface{}{
			"namespace": rrNamespace,
			"name":      rrName,
		})
		Expect(err).NotTo(HaveOccurred(), text)
		Expect(strings.ToLower(text)).To(Or(
			ContainSubstring("cancel"),
			ContainSubstring("cancelled"),
		))

		By("TC-E2E-RR-04: kubernaut_list_remediations returns the RR")
		var out map[string]interface{}
		text, err = mcpToolCallWith(authToken, mcpSessionID, "kubernaut_list_remediations", map[string]interface{}{
			"namespace": rrNamespace,
		})
		Expect(err).NotTo(HaveOccurred(), text)
		Expect(json.Unmarshal([]byte(text), &out)).To(Succeed())
		rem, ok := out["remediations"].([]interface{})
		Expect(ok).To(BeTrue(), "list result should include remediations array")
		Expect(len(rem)).To(BeNumerically(">=", 1))
		Expect(strings.ToLower(text)).To(ContainSubstring(strings.ToLower(rrName)))

		By("TC-E2E-RR-05: kubernaut_get_remediation returns detail for RR")
		text, err = mcpToolCallWith(authToken, mcpSessionID, "kubernaut_get_remediation", map[string]interface{}{
			"rr_id": rrID,
		})
		Expect(err).NotTo(HaveOccurred(), text)
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

	// TC-E2E-RR-06 deleted: idempotency tests af_create_rr (internal tool) — covered by UT.
	// TC-E2E-RR-07: kubernaut_watch with kubectl-seeded RR
	It("TC-E2E-RR-07: kubernaut_watch returns structured watch result", func() {
		authToken, err := fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		mcpSessionID, err := initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred())

		const rrName = "e2e-rr-watch-07"
		Expect(kubectlCreateRR("default", rrName, "Deployment", "test-deploy-rr07")).To(Succeed())
		DeferCleanup(func() { kubectlDeleteRR("default", rrName) })

		text, err := mcpToolCallWith(authToken, mcpSessionID, "kubernaut_watch", map[string]interface{}{
			"namespace": "default",
			"name":      rrName,
		})
		Expect(err).NotTo(HaveOccurred(), text)
		var out map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &out)).To(Succeed())
		Expect(out).To(HaveKey("events"))
		Expect(out).To(HaveKey("status"))
	})

	// TC-E2E-RR-08 deleted: af_create_rr validation (internal tool) — covered by UT.
	// TC-E2E-RR-09 deleted: af_create_rr RBAC denial — covered by A2A RBAC-06.
})

var _ = Describe("RAR Flow (G5)", Label("e2e", "phase2", "g5"), func() {

	const rrNamespace = "default"

	It("TC-E2E-RAR-01: kubernaut_approve succeeds for RAR referencing existing RR", func() {
		const rrName = "e2e-rr-rar01"
		Expect(kubectlCreateRR(rrNamespace, rrName, "Deployment", "test-deploy-rar01")).To(Succeed())
		DeferCleanup(func() { kubectlDeleteRR(rrNamespace, rrName) })

		rarName := "e2e-rar-g5-01"
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
		const rrName = "e2e-rr-rar03"
		Expect(kubectlCreateRR(rrNamespace, rrName, "Deployment", "test-deploy-rar03")).To(Succeed())
		DeferCleanup(func() { kubectlDeleteRR(rrNamespace, rrName) })

		sreTok, err := fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		sreSession, err := initMCPSession(sreTok)
		Expect(err).NotTo(HaveOccurred())

		rarName := "e2e-rar-g5-03"
		Expect(kubectlApplyYAML(remediationApprovalManifest(rrNamespace, rarName, rrName))).To(Succeed())
		DeferCleanup(func() {
			kubeconfigPath := os.Getenv("HOME") + "/.kube/apifrontend-e2e-config"
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

