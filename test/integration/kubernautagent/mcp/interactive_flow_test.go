/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mcp_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var _ = Describe("Interactive Session Lifecycle — INT BR-INTERACTIVE-001/004/005", Label("integration", "interactive"), func() {

	var (
		stack  *realMCPTestStack
		nsName string
	)

	BeforeEach(func() {
		nsName = uniqueNamespace("int")
		createNamespace(context.Background(), sharedK8sClient, nsName)
		stack = newRealMCPTestStack(sharedK8sClient, nsName, defaultRealStackOpts())
	})

	AfterEach(func() {
		stack.Close()
	})

	Describe("IT-KA-INT-01: start -> message -> complete full lifecycle", func() {
		It("should complete a full interactive session lifecycle over HTTP", func() {
			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			By("starting a session")
			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-01",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("started"))
			sessionID := output["session_id"].(string)
			Expect(sessionID).NotTo(BeEmpty(), "real Lease-backed session_id should be a UUID")

			By("sending a message")
			result, err = callInvestigate(session, map[string]any{
				"rr_id":   "rr-int-01",
				"action":  "message",
				"message": "Why is this pod restarting?",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err = decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("message_received"))
			Expect(output["response"]).NotTo(BeEmpty(), "real investigator should return LLM response text")

			By("completing the session")
			result, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-01",
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err = decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("completed"))
		})
	})

	Describe("IT-KA-INT-02: start returns session_active when lease is held", func() {
		It("should reject second start on same rr_id with session_active error", func() {
			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			_, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-02",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-02",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue())
		})
	})

	Describe("IT-KA-INT-03: message before start returns not_driving", func() {
		It("should reject message without an active session", func() {
			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			result, err := callInvestigate(session, map[string]any{
				"rr_id":   "rr-int-03",
				"action":  "message",
				"message": "Hello",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue())
		})
	})

	Describe("IT-KA-INT-04: cancel releases session", func() {
		It("should release session and allow restart after cancel", func() {
			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			By("starting a session")
			_, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-04",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("cancelling")
			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-04",
				"action": "cancel",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("cancelled"))

			By("verifying restart is allowed")
			result, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-04",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
		})
	})

	Describe("IT-KA-INT-05: status reports current session state", func() {
		It("should return interactive mode with driver info", func() {
			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			_, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-05",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-05",
				"action": "status",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("status"))
			Expect(output["response"]).To(ContainSubstring("interactive"))
		})
	})

	Describe("IT-KA-INT-06: multiple concurrent sessions on different rr_ids", func() {
		It("should allow independent sessions on different remediation requests", func() {
			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			By("starting session on rr-a")
			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-06-a",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("starting session on rr-b")
			result, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-06-b",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("sending message to each")
			result, err = callInvestigate(session, map[string]any{
				"rr_id":   "rr-int-06-a",
				"action":  "message",
				"message": "Check rr-a",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			result, err = callInvestigate(session, map[string]any{
				"rr_id":   "rr-int-06-b",
				"action":  "message",
				"message": "Check rr-b",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			Expect(stack.MockLLM.GetRequests()).To(HaveLen(2), "two LLM calls expected — one per session message")
		})
	})

	Describe("IT-KA-INT-07: complete after complete returns not_found", func() {
		It("should return not_found on double complete", func() {
			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			_, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-07",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-07",
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred())

			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-int-07",
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue())
		})
	})
})

// connectAndStartReal connects and starts a session on the real stack, returning session handle and session ID.
func connectAndStartReal(stack *realMCPTestStack, user, rrID string) (*mcpsdk.ClientSession, string) {
	session, err := connectMCP(stack.Server, user)
	Expect(err).NotTo(HaveOccurred())
	result, err := callInvestigate(session, map[string]any{
		"rr_id":  rrID,
		"action": "start",
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(result.IsError).To(BeFalse())
	output, err := decodeOutput(result)
	Expect(err).NotTo(HaveOccurred())
	return session, output["session_id"].(string)
}
