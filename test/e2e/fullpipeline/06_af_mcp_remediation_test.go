package fullpipeline

import (
	"encoding/json"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2E-FP-1189-001: MCP Path — AF creates a RemediationRequest via MCP tools/call,
// triggering the full downstream pipeline (RO → SP → AA → KA → WE → Notification).
var _ = Describe("AF MCP Path Full Pipeline [E2E-FP-1189-001]", Label("fp", "af", "mcp", "issue-1189"), func() {

	It("should create RR via MCP and trigger full pipeline execution", func() {
		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1189-001")
		}
		_ = resp.Body.Close()

		By("Initializing MCP session")
		sessionID, err := fpInitMCPSession()
		Expect(err).NotTo(HaveOccurred())
		Expect(sessionID).NotTo(BeEmpty(), "MCP session ID must not be empty")

		By("Creating RemediationRequest via MCP tools/call af_create_rr")
		respBody, status, err := fpMCPCall(sessionID, "af_create_rr", map[string]interface{}{
			"namespace":   namespace,
			"kind":        "Deployment",
			"name":        "memory-eater",
			"description": "E2E-FP-1189-001: MCP path full pipeline",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(http.StatusOK), "MCP tools/call should return 200, got body: %s", string(respBody))
		var rpc fpRPCResponse
		Expect(json.Unmarshal(respBody, &rpc)).To(Succeed())
		Expect(rpc.Error).To(BeNil(), "MCP tools/call should not return JSON-RPC error")

		By("Waiting for full pipeline execution (RR → WE completion)")
		rrName := fpWaitForRR("memory-eater", 120*time.Second)
		Expect(rrName).NotTo(BeEmpty())
		GinkgoWriter.Printf("  RemediationRequest created: %s\n", rrName)

		fpWaitForWEComplete(rrName, 5*time.Minute)
		GinkgoWriter.Printf("  WorkflowExecution completed for %s\n", rrName)
	})
})
