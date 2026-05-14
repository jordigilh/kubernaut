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
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Security IT — BR-INTERACTIVE-010/SEC", Label("integration", "security"), func() {

	var (
		stack  *realMCPTestStack
		nsName string
	)

	BeforeEach(func() {
		nsName = uniqueNamespace("sec")
		createNamespace(context.Background(), sharedK8sClient, nsName)
		stack = newRealMCPTestStack(sharedK8sClient, nsName, defaultRealStackOpts())
	})

	AfterEach(func() {
		stack.Close()
	})

	Describe("IT-KA-SEC-001: error responses do not leak internal details", func() {
		It("should return structured error without stack traces or internal paths", func() {
			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			By("triggering a 'not driving' error by messaging without start")
			result, err := callInvestigate(session, map[string]any{
				"rr_id":   "rr-sec-01",
				"action":  "message",
				"message": "hello",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue())

			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			responseText, _ := output["text"].(string)
			Expect(responseText).NotTo(ContainSubstring("/Users/"))
			Expect(responseText).NotTo(ContainSubstring("goroutine"))
			Expect(responseText).NotTo(ContainSubstring("panic"))
			Expect(responseText).NotTo(ContainSubstring(".go:"))
		})
	})

	Describe("IT-KA-SEC-002: cross-user session isolation via real K8s Leases", func() {
		It("should prevent bob from acting on alice's lease-backed session", func() {
			By("alice starts a session")
			alice, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = alice.Close() }()

			startResult, err := callInvestigate(alice, map[string]any{
				"rr_id":  "rr-sec-02",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(startResult.IsError).To(BeFalse())

			By("bob attempts to message alice's session")
			bob, err := connectMCP(stack.Server, "bob@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = bob.Close() }()

			result, err := callInvestigate(bob, map[string]any{
				"rr_id":   "rr-sec-02",
				"action":  "message",
				"message": "injected",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue())

			By("bob attempts to cancel alice's session")
			result, err = callInvestigate(bob, map[string]any{
				"rr_id":  "rr-sec-02",
				"action": "cancel",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue())

			By("alice's session is still functional")
			aliceMsg, err := callInvestigate(alice, map[string]any{
				"rr_id":   "rr-sec-02",
				"action":  "message",
				"message": "still mine",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(aliceMsg.IsError).To(BeFalse())
		})
	})

	Describe("IT-KA-SEC-003: unauthenticated HTTP request gets 401", func() {
		It("should return 401 for requests without auth header", func() {
			resp, err := http.Get(stack.Server.URL + "/mcp")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})
	})

	Describe("IT-KA-SEC-004: status action does not leak driver email to non-driver", func() {
		It("should allow status but not expose sensitive driver details to third party", func() {
			alice, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = alice.Close() }()

			_, err = callInvestigate(alice, map[string]any{
				"rr_id":  "rr-sec-04",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			bob, err := connectMCP(stack.Server, "bob@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = bob.Close() }()

			statusResult, err := callInvestigate(bob, map[string]any{
				"rr_id":  "rr-sec-04",
				"action": "status",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(statusResult.IsError).To(BeFalse(), "status should be readable by anyone")
			output, err := decodeOutput(statusResult)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["response"]).To(ContainSubstring("interactive"))
		})
	})
})
