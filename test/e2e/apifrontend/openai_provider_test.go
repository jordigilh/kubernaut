package e2e_test

import (
	"net/http"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	trueFixture = "true"
)

// E2E-AF-1254-001: Full A2A journey with openai_compatible provider (mock-LLM)
//
// This test verifies the complete end-to-end journey:
//
//	AF configured with provider: openai_compatible → in-house adapter →
//	mock-LLM OpenAI handler (/v1/chat/completions) → A2A response
//
// Prerequisites:
//   - AF deployed in Kind cluster with agent.llm.provider=openai_compatible
//     and agent.llm.endpoint pointing to mock-LLM's OpenAI endpoint
//   - mock-LLM deployed with OpenAI chat completions handler
//   - DEX running for auth tokens
//   - AF_E2E_OPENAI_LANE=true set by the CI job that deploys the
//     openai_compatible overlay (not yet implemented — see #1254)
//
// FedRAMP Controls: AC-4, CM-6, SI-10, IA-5, SC-8
var _ = Describe("OpenAI Provider E2E (BR-INTEGRATION-1254)", Label("e2e", "openai"), func() {

	var sreToken string

	BeforeEach(func() {
		if os.Getenv("AF_E2E_OPENAI_LANE") != trueFixture {
			Skip("OpenAI E2E lane not active — set AF_E2E_OPENAI_LANE=true with openai_compatible overlay (deferred per #1254)")
		}

		if !setupSucceeded {
			Skip("E2E infrastructure not available")
		}

		var err error
		sreToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "Failed to obtain SRE token")
	})

	// E2E-AF-1254-001 [AC-4, CM-6]: Operator configures AF with openai_compatible
	// provider pointing at mock-LLM; A2A conversation completes successfully.
	It("E2E-AF-1254-001 A2A conversation completes with openai_compatible provider", func() {
		body := a2aTasksSend("e2e-1254-001",
			"Investigate pod crash in namespace default for deployment test-firing-target")

		resp, err := a2aInvoke(httpClient, baseURL, sreToken, body)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"A2A endpoint must return 200 when using openai_compatible provider")

		rpc, err := parseRPCResponse(resp)
		Expect(err).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(),
			"JSON-RPC error must be nil — openai_compatible provider should not cause protocol errors: %+v", rpc.Error)
		Expect(rpc.Result).NotTo(BeNil())

		task, err := extractTaskFromResult(rpc.Result)
		Expect(err).NotTo(HaveOccurred())
		Expect(task.ID).NotTo(BeEmpty(), "task ID must not be empty")
		Expect(task.Status.State).To(BeElementOf(completed, "working"),
			"task should reach completed or working state — not failed")
	})
})
