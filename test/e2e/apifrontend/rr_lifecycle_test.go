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
		rrNamespace := e2eNamespace

		By("TC-E2E-RR-01: Create RR via k8s client CRD fixture")
		Expect(createRR(rrNamespace, rrName, "Deployment", "test-deploy-rr01")).To(Succeed())
		DeferCleanup(func() { deleteRR(rrNamespace, rrName) })

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
			"rr_id": rrName,
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

	// TC-E2E-RR-06 deleted: idempotency tests kubernaut_remediate (internal tool) — covered by UT.
	// TC-E2E-RR-07: kubernaut_watch with k8s-seeded RR
	It("TC-E2E-RR-07: kubernaut_watch returns structured watch result", NodeTimeout(60*time.Second), func(_ SpecContext) {
		authToken, err := fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		mcpSessionID, err := initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred())

		const rrName = "e2e-rr-watch-07"
		Expect(createRR(e2eNamespace, rrName, "Deployment", "test-deploy-rr07")).To(Succeed())
		DeferCleanup(func() { deleteRR(e2eNamespace, rrName) })

		// AF-only E2E has no SP/RO controllers, so the RR never transitions
		// phases on its own. Simulate a terminal phase change after a delay
		// so kubernaut_watch observes the event and returns.
		go func() {
			defer GinkgoRecover()
			time.Sleep(3 * time.Second)
			rr := &remediationv1alpha1.RemediationRequest{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKey{
				Namespace: e2eNamespace, Name: rrName,
			}, rr)).To(Succeed())
			rr.Status.OverallPhase = "Completed"
			rr.Status.Message = "E2E: simulated terminal phase for kubernaut_watch"
			Expect(k8sClient.Status().Update(context.Background(), rr)).To(Succeed())
		}()

		text, err := mcpToolCallWith(authToken, mcpSessionID, "kubernaut_watch", map[string]interface{}{
			"namespace": e2eNamespace,
			"name":      rrName,
		})
		Expect(err).NotTo(HaveOccurred(), text)
		var out map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &out)).To(Succeed())
		Expect(out).To(HaveKey("events"))
		Expect(out).To(HaveKey("status"))
	})

	// TC-E2E-RR-08 deleted: kubernaut_remediate validation (internal tool) — covered by UT.
	// TC-E2E-RR-09 deleted: kubernaut_remediate RBAC denial — covered by A2A RBAC-06.
})

var _ = Describe("RAR Flow (G5)", Label("e2e", "phase2", "g5"), func() {

	rrNamespace := e2eNamespace

	It("TC-E2E-RAR-01: kubernaut_approve succeeds for RAR referencing existing RR", func() {
		const rrName = "e2e-rr-rar01"
		Expect(createRR(rrNamespace, rrName, "Deployment", "test-deploy-rar01")).To(Succeed())
		DeferCleanup(func() { deleteRR(rrNamespace, rrName) })

		rarName := "e2e-rar-g5-01"
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
		Expect(createRR(rrNamespace, rrName, "Deployment", "test-deploy-rar03")).To(Succeed())
		DeferCleanup(func() { deleteRR(rrNamespace, rrName) })

		sreTok, err := fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		sreSession, err := initMCPSession(sreTok)
		Expect(err).NotTo(HaveOccurred())

		rarName := "e2e-rar-g5-03"
		Expect(k8sClient.Create(context.Background(), buildRAR(rrNamespace, rarName, rrName))).To(Succeed())
		DeferCleanup(func() {
			rar := &remediationv1alpha1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: rarName, Namespace: rrNamespace},
			}
			_ = client.IgnoreNotFound(k8sClient.Delete(context.Background(), rar))
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
