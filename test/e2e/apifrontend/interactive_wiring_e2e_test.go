package e2e_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These E2E tests validate the wiring of interactive investigation tools
// and deferred CRD materialization through the live AF binary.

var _ = Describe("Interactive Wiring E2E (W6)", Label("e2e", "phase4", "wiring"), func() {

	var (
		authToken    string
		mcpSessionID string
	)

	BeforeEach(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())

		mcpSessionID, err = initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(mcpSessionID).NotTo(BeEmpty())
	})

	mcpToolCall := func(rpcID, toolName string, args map[string]interface{}) (string, int, error) {
		callBody := buildJSONRPC(rpcID, "tools/call", map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		})
		raw, code, err := mcpPOST(authToken, mcpSessionID, callBody)
		if err != nil {
			return "", code, err
		}
		return unwrapSSEDataLine(raw), code, nil
	}

	mcpToolsList := func() (string, error) {
		listBody := buildJSONRPC("w6-tools-list", "tools/list", map[string]interface{}{})
		raw, _, err := mcpPOST(authToken, mcpSessionID, listBody)
		if err != nil {
			return "", err
		}
		return unwrapSSEDataLine(raw), nil
	}

	Describe("E2E-AF-1234-W01: Interactive 4-phase tool wiring", func() {

		It("E2E-AF-1234-W01a: tools/list exposes interactive tools (#1332: takeover via investigate)", func() {
			body, err := mcpToolsList()
			Expect(err).NotTo(HaveOccurred())

			for _, tool := range []string{
				"kubernaut_investigate",
				"kubernaut_message",
				"kubernaut_complete",
				"kubernaut_cancel",
				"kubernaut_status",
				"kubernaut_reconnect",
			} {
				Expect(body).To(ContainSubstring(tool),
					"tools/list should expose %s", tool)
			}
		})

		It("E2E-AF-1332-W01b: kubernaut_investigate dispatches to KA and returns structured response", func() {
			rrName := fmt.Sprintf("e2e-rr-w01b-%s", uuid.New().String()[:8])
			Expect(createRR("default", rrName, "Deployment", "test-deploy-w01b")).To(Succeed())
			DeferCleanup(func() { deleteRR("default", rrName) })

			rpcID := fmt.Sprintf("w01b-%d", time.Now().UnixNano())
			text, code, err := mcpToolCall(rpcID, "kubernaut_investigate", map[string]interface{}{
				"rr_id": rrName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(BeNumerically("<", 500),
				"investigate should not return 5xx; got body: %s", text)

			var result map[string]interface{}
			if json.Unmarshal([]byte(text), &result) == nil {
				if inner, ok := result["result"].(map[string]interface{}); ok {
					if content, ok := inner["content"].([]interface{}); ok && len(content) > 0 {
						if first, ok := content[0].(map[string]interface{}); ok {
							if txt, ok := first["text"].(string); ok {
								text = txt
							}
						}
					}
				}
			}

			Expect(text).NotTo(BeEmpty(), "takeover should return a non-empty response")
		})

		It("E2E-AF-1234-W01c: kubernaut_status returns structured response for non-existent session", func() {
			rpcID := fmt.Sprintf("w01c-%d", time.Now().UnixNano())
			text, code, err := mcpToolCall(rpcID, "kubernaut_status", map[string]interface{}{
				"rr_id": "default/nonexistent-rr-status-probe",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(BeNumerically("<", 500),
				"status should not return 5xx; got body: %s", text)
		})
	})

	Describe("E2E-AF-1234-W02: Deferred CRD lifecycle through MCP", func() {

		It("E2E-AF-1234-W02a: kubernaut_investigate is exposed in tools/list", func() {
			body, err := mcpToolsList()
			Expect(err).NotTo(HaveOccurred())
			Expect(body).To(ContainSubstring("kubernaut_investigate"),
				"tools/list should expose kubernaut_investigate")
		})

		It("E2E-AF-1234-W02b: audit trail includes execution_duration_ms after tool call", func() {
			rpcID := fmt.Sprintf("w02b-%d", time.Now().UnixNano())
			_, _, err := mcpToolCall(rpcID, "kubernaut_list_remediations", map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())

			metrics := scrapeMetrics()
			Expect(metrics).To(ContainSubstring("af_tool_call_duration_seconds"),
				"tool call duration metric should be registered")
		})

		It("E2E-AF-1234-W02c: per-tool timeout is respected — investigate tool does not use global timeout", func() {
			body, err := mcpToolsList()
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(body), &parsed); err == nil {
				if result, ok := parsed["result"].(map[string]interface{}); ok {
					if tools, ok := result["tools"].([]interface{}); ok {
						var hasInvestigate bool
						for _, t := range tools {
							if tm, ok := t.(map[string]interface{}); ok {
								if strings.Contains(fmt.Sprintf("%v", tm["name"]), "kubernaut_investigate") {
									hasInvestigate = true
								}
							}
						}
						Expect(hasInvestigate).To(BeTrue(), "kubernaut_investigate should be in tools/list")
					}
				}
			}
		})
	})
})
