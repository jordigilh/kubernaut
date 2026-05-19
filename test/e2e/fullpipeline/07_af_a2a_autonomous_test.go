package fullpipeline

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2E-FP-1189-002: A2A Autonomous — A single message/send instructs the AF agent to
// create a remediation request. The mock-LLM returns an af_create_rr tool call, the
// agent executes it, and the full downstream pipeline processes the RR.
var _ = Describe("AF A2A Autonomous Full Pipeline [E2E-FP-1189-002]", Ordered, Label("fp", "af", "a2a", "issue-1189"), func() {

	BeforeAll(func() {
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1189-002")
		}
		_ = resp.Body.Close()
	})

	It("should create RR via single A2A message/send", func() {
		body := fpA2ATasksSend("fp-auto-1",
			"create a remediation request for deployment memory-eater in kubernaut-system")
		resp, err := fpA2AInvoke(body)
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
		GinkgoWriter.Printf("  📋 A2A task: %s (state: %s)\n", task.ID, task.Status.State)
	})

	It("should trigger full pipeline execution", func() {
		rrName := fpWaitForRR("memory-eater", 120*time.Second)
		Expect(rrName).NotTo(BeEmpty())

		fpWaitForWEComplete(rrName, 5*time.Minute)
		GinkgoWriter.Printf("  ✅ Full pipeline completed for %s\n", rrName)
	})
})
