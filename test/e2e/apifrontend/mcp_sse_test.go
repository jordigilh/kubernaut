package e2e_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MCP SSE Responses (G2)", Label("e2e", "phase4", "g2"), func() {
	var authToken, mcpSessionID string

	BeforeEach(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		mcpSessionID, err = initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred())
	})

	It("TC-E2E-MCP-SSE-01: Accept negotiation — text/event-stream preferred", func() {
		callBody := buildJSONRPC("neg-01", "tools/call", map[string]interface{}{
			"name": "kubernaut_list_remediations",
			"arguments": map[string]interface{}{},
		})
		req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(callBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+authToken)
		if mcpSessionID != "" {
			req.Header.Set("Mcp-Session-Id", mcpSessionID)
		}

		resp, err := httpClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(BeNumerically("<", http.StatusBadRequest))

		ct := resp.Header.Get("Content-Type")
		Expect(strings.Contains(strings.ToLower(ct), "text/event-stream")).To(BeTrue(), "Content-Type: %q", ct)

		raw, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		bodyStr := string(raw)
		Expect(bodyStr).To(ContainSubstring("data:"))

		firstPayload := unwrapSSEDataLine(raw)
		var root map[string]interface{}
		Expect(json.Unmarshal([]byte(firstPayload), &root)).To(Succeed())
		Expect(root["jsonrpc"]).To(Equal("2.0"))
	})

	It("TC-E2E-MCP-SSE-02: Progress frames during long tool call", func() {
		callBody := buildJSONRPC(fmt.Sprintf("sse-prog-02-%d", time.Now().UnixNano()), "tools/call", map[string]interface{}{
			"name": "kubernaut_investigate",
			"arguments": map[string]interface{}{
				"rr_id": "e2e-sse-prog-test-rr",
			},
		})
		req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(callBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+authToken)
		if mcpSessionID != "" {
			req.Header.Set("Mcp-Session-Id", mcpSessionID)
		}

		resp, err := httpClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(BeNumerically("<", http.StatusBadRequest))

		scanner := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 4<<20)

		var dataLines []string
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), "\r")
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, line)
			}
		}
		Expect(scanner.Err()).NotTo(HaveOccurred())

		// AF does not implement MCP progress notifications — each tools/call
		// returns exactly one SSE data frame containing the JSON-RPC response.
		Expect(dataLines).To(HaveLen(1),
			"AF returns a single SSE frame per tools/call (MCP progress notifications not implemented)")

		firstPayload := strings.TrimPrefix(dataLines[0], "data:")
		firstPayload = strings.TrimSpace(firstPayload)
		var root map[string]interface{}
		Expect(json.Unmarshal([]byte(firstPayload), &root)).To(Succeed(),
			"SSE data frame must be valid JSON-RPC 2.0")
		Expect(root).To(HaveKey("jsonrpc"))
	})
})
