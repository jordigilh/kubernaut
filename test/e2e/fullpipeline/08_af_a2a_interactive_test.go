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
// Turn 1: "start investigation…"        → kubernaut_start_investigation
// Turn 2: "stream the investigation…"   → kubernaut_stream_investigation
// Turn 3: "discover available workflows" → kubernaut_discover_workflows
// Turn 4: "create a remediation request" → af_create_rr
var _ = Describe("AF A2A Interactive 4-Phase Full Pipeline [E2E-FP-1189-003]", Ordered, Label("fp", "af", "a2a", "interactive", "issue-1189"), func() {
	var taskID string

	BeforeAll(func() {
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1189-003")
		}
		_ = resp.Body.Close()
	})

	It("Turn 1: start investigation", func() {
		body := fpA2ATasksSend("fp-int-1",
			"start investigation for deployment memory-eater in kubernaut-system")
		resp, err := fpA2AInvoke(body)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 1 should not return JSON-RPC error")

		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		taskID = task.ID
		Expect(taskID).NotTo(BeEmpty())
		GinkgoWriter.Printf("  📋 Turn 1 — task: %s (state: %s)\n", taskID, task.Status.State)
	})

	It("Turn 2: stream the investigation events", func() {
		Expect(taskID).NotTo(BeEmpty(), "requires Turn 1 to have produced a task ID")
		body := fpA2ATasksSendWithTask("fp-int-2", taskID,
			"stream the investigation events")
		resp, err := fpA2AInvoke(body)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 2 should not return JSON-RPC error")
		GinkgoWriter.Printf("  📋 Turn 2 — stream investigation OK\n")
	})

	It("Turn 3: discover available workflows", func() {
		Expect(taskID).NotTo(BeEmpty(), "requires prior turns")
		body := fpA2ATasksSendWithTask("fp-int-3", taskID,
			"discover available workflows")
		resp, err := fpA2AInvoke(body)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 3 should not return JSON-RPC error")
		GinkgoWriter.Printf("  📋 Turn 3 — discover workflows OK\n")
	})

	It("Turn 4: create a remediation request", func() {
		Expect(taskID).NotTo(BeEmpty(), "requires prior turns")
		body := fpA2ATasksSendWithTask("fp-int-4", taskID,
			"create a remediation request")
		resp, err := fpA2AInvoke(body)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 4 should not return JSON-RPC error")
		GinkgoWriter.Printf("  📋 Turn 4 — create RR OK\n")
	})

	It("should complete full pipeline after interactive conversation", func() {
		rrName := fpWaitForRR("memory-eater", 120*time.Second)
		Expect(rrName).NotTo(BeEmpty())

		fpWaitForWEComplete(rrName, 5*time.Minute)
		GinkgoWriter.Printf("  ✅ Full pipeline completed for %s\n", rrName)
	})
})
