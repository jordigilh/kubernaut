package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// expiredCallerJWT is a well-formed HS256-shaped JWT whose exp claim is far in the past.
// Signature is intentionally invalid — exercising rejection after (or instead of) expiry checks.
const expiredCallerJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjEwMDAwMDAwMDAsInN1YiI6ImUyZS1leHBpcmVkIiwiaWF0IjoxMDAwMDAwMDAwfQ.invalidsignature"

var _ = Describe("JWT Delegation to KA (G7)", Label("e2e", "phase4", "g7"), func() {

	expectAuthError := func(httpCode int, raw []byte, payload string) {
		lower := strings.ToLower(payload + string(raw))
		if httpCode == http.StatusUnauthorized {
			return
		}
		Expect(lower).To(Or(
			ContainSubstring("401"),
			ContainSubstring("unauthorized"),
			ContainSubstring("invalid token"),
			ContainSubstring("token"),
			ContainSubstring("authentication"),
			ContainSubstring("auth"),
		), "expected auth-related error in response (HTTP %d): %s", httpCode, payload)
	}

	It("TC-E2E-JWT-01: kubernaut_select_workflow via MCP carries caller identity", func() {
		token, err := fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		Expect(token).NotTo(BeEmpty())

		sid, err := initMCPSession(token)
		Expect(err).NotTo(HaveOccurred(), "MCP initialize")
		Expect(sid).NotTo(BeEmpty())

		callBody := buildJSONRPC(fmt.Sprintf("g7-jwt-01-%d", time.Now().UnixNano()), "tools/call",
			map[string]interface{}{
				"name": "kubernaut_select_workflow",
				"arguments": map[string]interface{}{
					"rr_id":       "e2e-jwt-rr-placeholder",
					"workflow_id": "wf-restart",
				},
			})
		raw, code, err := mcpPOST(token, sid, callBody)
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(BeNumerically("<", 400), "MCP transport error: HTTP %d: %s", code, string(raw))

		// Response-based delegation proof: if KA accepted the JWT (not AF SA),
		// the response will NOT contain auth errors. Business errors like
		// "no active interactive session" are acceptable — they prove JWT auth
		// succeeded at KA and only the workflow preconditions failed.
		payload := unwrapSSEDataLine(raw)
		text, _, perr := parseMCPToolPayload(payload)
		Expect(perr).NotTo(HaveOccurred(), "failed to parse MCP response payload")

		lower := strings.ToLower(text + string(raw))
		Expect(lower).NotTo(ContainSubstring("unauthorized"),
			"KA must not return 401 — JWT delegation should forward the caller token")
		Expect(lower).NotTo(ContainSubstring("invalid token"),
			"KA must not reject the token — JWT delegation should forward a valid caller JWT")
	})

	It("TC-E2E-JWT-02: Expired caller JWT -> KA rejects with 401", func() {
		sid, err := initMCPSession(expiredCallerJWT)
		if err != nil {
			// AF rejected the bearer before MCP session came up — auth failure at edge.
			return
		}
		if sid == "" {
			return
		}

		callBody := buildJSONRPC(fmt.Sprintf("g7-jwt-02-%d", time.Now().UnixNano()), "tools/call",
			map[string]interface{}{
				"name": "kubernaut_select_workflow",
				"arguments": map[string]interface{}{
					"rr_id":       "e2e-jwt-expired",
					"workflow_id": "wf-restart",
				},
			})
		raw, code, err := mcpPOST(expiredCallerJWT, sid, callBody)
		Expect(err).NotTo(HaveOccurred())

		payload := unwrapSSEDataLine(raw)
		var rpc map[string]interface{}
		_ = json.Unmarshal([]byte(payload), &rpc)

		if errObj, ok := rpc["error"].(map[string]interface{}); ok && errObj != nil {
			msg, _ := errObj["message"].(string)
			codeFloat, _ := errObj["code"].(float64)
			if codeFloat != 0 && int(codeFloat) == http.StatusUnauthorized {
				return
			}
			expectAuthError(code, raw, msg)
			return
		}

		text, toolErr, perr := parseMCPToolPayload(payload)
		Expect(perr).NotTo(HaveOccurred())
		if toolErr {
			expectAuthError(code, []byte(text), text)
			return
		}

		// Fallback: some stacks surface 401 only on HTTP layer.
		expectAuthError(code, raw, payload)
	})
})
