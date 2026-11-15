package e2e_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MCP tool response validation", func() {
	It("UT-AF-2598-001 [BR-INTERACTIVE-001]: rejects notification-only responses", func() {
		_, _, err := parseMCPToolPayload(`{"jsonrpc":"2.0","method":"notifications/tools/list_changed"}`, "rpc-notification")

		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-2598-002 [BR-INTERACTIVE-001]: rejects a result for a different RPC request", func() {
		_, _, err := parseMCPToolPayload(`{"jsonrpc":"2.0","id":"other-request","result":{"content":[{"type":"text","text":"unexpected"}]}}`, "rpc-request")

		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-2598-003 [BR-INTERACTIVE-001]: accepts a result for the requested RPC", func() {
		text, toolIsError, err := parseMCPToolPayload(`{"jsonrpc":"2.0","id":"rpc-request","result":{"content":[{"type":"text","text":"completed"}]}}`, "rpc-request")

		Expect(err).NotTo(HaveOccurred())
		Expect(toolIsError).To(BeFalse())
		Expect(text).To(Equal("completed"))
	})
})
