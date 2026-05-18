package e2e_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DS Audit Sink (G8)", Ordered, ContinueOnFailure, Label("e2e", "phase4", "g8"), func() {
	var (
		authToken    string
		mcpSessionID string
		dsAuditURL   string
	)

	BeforeAll(func() {
		dsAuditURL = getEnvOrDefault("AF_E2E_DS_AUDIT_URL", "https://localhost:8089/api/v1/audit/events")

		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		mcpSessionID, err = initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred())
	})

	mcpToolCall := func(toolName string, args map[string]interface{}) (string, error) {
		callBody := buildJSONRPC(fmt.Sprintf("g8-%s-%d", toolName, time.Now().UnixNano()),
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

	dsClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // E2E test only
		Timeout:   10 * time.Second,
	}

	fetchAuditBody := func() ([]byte, int, error) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, dsAuditURL, http.NoBody)
		if err != nil {
			return nil, 0, err
		}
		resp, err := dsClient.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer func() { _ = resp.Body.Close() }()
		b, rerr := io.ReadAll(resp.Body)
		return b, resp.StatusCode, rerr
	}

	auditBodyContainsTool := func(body []byte, toolSubstring string) bool {
		if len(body) == 0 {
			return false
		}
		s := strings.ToLower(string(body))
		return strings.Contains(s, strings.ToLower(toolSubstring))
	}

	It("TC-E2E-AUDIT-01: After A2A tool call -> DS QueryAuditEvents returns matching entry", func() {
		// Pre-check: verify DS audit endpoint is reachable from the test host.
		func() {
			body, code, rerr := fetchAuditBody()
			if rerr != nil {
				Skip(fmt.Sprintf("DS audit endpoint not reachable from test host (%s): %v", dsAuditURL, rerr))
			}
			if code == http.StatusUnauthorized || code == http.StatusForbidden ||
				code == http.StatusNotFound || code == http.StatusBadGateway || code == http.StatusServiceUnavailable {
				Skip(fmt.Sprintf("DS audit endpoint returned %d — service not accessible from test host", code))
			}
			_ = body
		}()

		marker := fmt.Sprintf("g8-audit-01-%d", time.Now().UnixNano())
		_, err := mcpToolCall("af_get_pods", map[string]interface{}{
			"namespace":     "default",
			"labelSelector": fmt.Sprintf("e2e-audit=%s", marker),
		})
		if err != nil {
			_, err = mcpToolCall("af_get_pods", map[string]interface{}{
				"namespace": "default",
			})
		}
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			body, code, rerr := fetchAuditBody()
			if rerr != nil || code != http.StatusOK {
				return false
			}
			return auditBodyContainsTool(body, "af_get_pods")
		}, 60*time.Second, 2*time.Second).Should(BeTrue(), "DS audit API should list an event referencing af_get_pods")
	})

	It("TC-E2E-AUDIT-04: Audit events contain redacted Detail (no raw secrets)", func() {
		body, code, err := fetchAuditBody()
		Expect(err).NotTo(HaveOccurred())
		if code != http.StatusOK {
			Skip(fmt.Sprintf("DS audit API not reachable (%d) — %s", code, string(body)))
		}
		Expect(len(body)).To(BeNumerically(">", 2))
		Expect(json.Valid(body)).To(BeTrue(), "DS audit response should be JSON")

		lower := string(body)
		dangerPatterns := []string{
			`"password":"`,
			`"token":"`,
			`"secret":"`,
			`'password':`,
		}
		for _, p := range dangerPatterns {
			Expect(strings.ToLower(lower)).NotTo(ContainSubstring(strings.ToLower(p)),
				"audit payload should not contain raw %s material", p)
		}
	})
})
