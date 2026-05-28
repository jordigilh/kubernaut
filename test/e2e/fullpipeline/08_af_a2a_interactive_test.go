package fullpipeline

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2E-FP-1189-003: A2A Interactive 4-Phase — Simulates a multi-turn conversation
// where the user investigates, discovers workflows, selects one, and creates an RR.
// Depends on Phase 0 mock-LLM fix (match_last_only) for correct keyword resolution.
//
// Turn 1: "start investigation…"        → kubernaut_investigate (namespace/name/kind)
// Turn 2: "stream the investigation…"   → kubernaut_investigate (session_id resume)
// Turn 3: "discover available workflows" → kubernaut_discover_workflows
// Turn 4: "create a remediation request" → af_create_rr
var _ = Describe("AF A2A Interactive 4-Phase Full Pipeline [E2E-FP-1189-003]", Label("fp", "af", "a2a", "interactive", "issue-1189"), func() {

	It("should complete 4-turn interactive conversation and trigger full pipeline", func() {
		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1189-003")
		}
		_ = resp.Body.Close()

		By("Waiting for memory-eater pod to crash (F-SIG-08: ensures Warning events exist for DominantEventReason)")
		fpWaitForPodCrash("memory-eater", 2*time.Minute)

		By("Turn 1: start investigation")
		body := fpA2ATasksSend("fp-int-1",
			"start investigation for deployment memory-eater in kubernaut-system")
		resp, err = fpA2AInvoke(body)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 1 should not return JSON-RPC error")
		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		taskID := task.ID
		Expect(taskID).NotTo(BeEmpty())
		GinkgoWriter.Printf("  Turn 1 — task: %s (state: %s)\n", taskID, task.Status.State)

		By("Turn 2: stream the investigation events")
		body = fpA2ATasksSendWithTask("fp-int-2", taskID,
			"stream the investigation events")
		resp2, err := fpA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp2.Body.Close() }()
		Expect(resp2.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp2)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 2 should not return JSON-RPC error")
		GinkgoWriter.Printf("  Turn 2 — stream investigation OK\n")

		By("Turn 3: discover available workflows")
		body = fpA2ATasksSendWithTask("fp-int-3", taskID,
			"discover available workflows")
		resp3, err := fpA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp3.Body.Close() }()
		Expect(resp3.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp3)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 3 should not return JSON-RPC error")
		GinkgoWriter.Printf("  Turn 3 — discover workflows OK\n")

		By("Turn 4: create a remediation request")
		body = fpA2ATasksSendWithTask("fp-int-4", taskID,
			"create a remediation request")
		resp4, err := fpA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp4.Body.Close() }()
		Expect(resp4.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp4)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 4 should not return JSON-RPC error")
		GinkgoWriter.Printf("  Turn 4 — create RR OK\n")

		By("Waiting for full pipeline execution")
		rrName := fpWaitForRR("memory-eater", 120*time.Second)
		Expect(rrName).NotTo(BeEmpty())
		fpWaitForWEComplete(rrName, 5*time.Minute)
		GinkgoWriter.Printf("  Full pipeline completed for %s\n", rrName)
	})
})
