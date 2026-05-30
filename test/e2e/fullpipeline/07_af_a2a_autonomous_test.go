package fullpipeline

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2E-FP-1189-002: A2A Autonomous — A single message/send instructs the AF agent to
// create a remediation request. The mock-LLM returns a kubernaut_remediate tool call,
// the agent executes it, and the full downstream pipeline processes the RR.
// Issue #1332: Autonomous flow must NOT create an InvestigationSession.
var _ = Describe("AF A2A Autonomous Full Pipeline [E2E-FP-1189-002]", Label("fp", "af", "a2a", "issue-1189", "issue-1332"), func() {

	It("should create RR via A2A and trigger full pipeline execution without IS", func() {
		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1189-002")
		}
		_ = resp.Body.Close()

		By("Waiting for memory-eater pod to crash (F-SIG-08: ensures Warning events exist for DominantEventReason)")
		fpWaitForPodCrash("memory-eater", 2*time.Minute)

		By("Creating RR via A2A message/send (kubernaut_remediate — autonomous, no IS)")
		body := fpA2ATasksSend("fp-auto-1",
			"create a remediation request for deployment memory-eater in kubernaut-system")
		resp, err = fpA2AInvoke(body)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"A2A message/send should return 200")
		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "A2A should not return JSON-RPC error")
		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		Expect(task.ID).NotTo(BeEmpty(), "A2A task ID must not be empty")
		GinkgoWriter.Printf("  A2A task: %s (state: %s)\n", task.ID, task.Status.State)

		By("Waiting for full pipeline execution")
		rrName := fpWaitForRRWithTargetNS("memory-eater", namespace, 120*time.Second)
		Expect(rrName).NotTo(BeEmpty())
		fpWaitForWEComplete(rrName, 5*time.Minute)
		GinkgoWriter.Printf("  Full pipeline completed for %s\n", rrName)

		By("Verifying no InvestigationSession was created (Issue #1332: autonomous flow)")
		fpAssertNoISForRR(rrName, namespace)
	})
})
