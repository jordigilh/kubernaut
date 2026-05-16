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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var _ = Describe("Timeout Wiring — GAP-8 / BR-INTERACTIVE-003", Label("integration", "timeout"), func() {

	Describe("IT-KA-TIMEOUT-001: Inactivity timeout expires session and rejects subsequent messages", func() {
		It("should return session_expired after inactivity timeout", func() {
			nsName := uniqueNamespace("timeout01")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			opts := defaultRealStackOpts()
			opts.inactivityTimeout = 2 * time.Second
			opts.warningIntervals = []time.Duration{1 * time.Second}
			stack := newRealMCPTestStack(sharedK8sClient, nsName, opts)
			defer stack.Close()

			sess, err := connectMCP(stack.Server, "alice@example.com")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			By("starting a session")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-timeout-001",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("sending an initial message to confirm active session")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-timeout-001",
				"action":  "message",
				"message": "hello",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("waiting for inactivity timeout to expire")
			time.Sleep(3 * time.Second)

			By("sending message after timeout — should be rejected")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-timeout-001",
				"action":  "message",
				"message": "after timeout",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue(),
				"message after inactivity timeout must be rejected")
		})
	})

	Describe("IT-KA-TIMEOUT-002: Warning notification fires before expiry", func() {
		It("should deliver a warning via TimeoutManager before session expires", func() {
			nsName := uniqueNamespace("timeout02")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			opts := defaultRealStackOpts()
			opts.inactivityTimeout = 4 * time.Second
			opts.warningIntervals = []time.Duration{1 * time.Second}
			stack := newRealMCPTestStack(sharedK8sClient, nsName, opts)
			defer stack.Close()

			var notificationReceived bool
			sess, err := connectMCP(stack.Server, "alice@example.com")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-timeout-002",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			sessionID, ok := output["session_id"].(string)
			Expect(ok).To(BeTrue(), "start response should contain session_id")

			stack.Notifier.Register(sessionID, func(msg string) {
				notificationReceived = true
			})

			By("waiting for the warning interval (1s into 4s timeout)")
			time.Sleep(2 * time.Second)

			Expect(notificationReceived).To(BeTrue(),
				"warning notification should fire before session expires")

			By("completing session before full expiry")
			_, _ = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-timeout-002",
				"action": "complete",
			})
		})
	})

	Describe("IT-KA-TIMEOUT-003: Activity resets inactivity timer", func() {
		It("should keep session active when messages are sent within the timeout window", func() {
			nsName := uniqueNamespace("timeout03")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			opts := defaultRealStackOpts()
			opts.inactivityTimeout = 2 * time.Second
			opts.warningIntervals = nil
			stack := newRealMCPTestStack(sharedK8sClient, nsName, opts)
			defer stack.Close()

			sess, err := connectMCP(stack.Server, "alice@example.com")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-timeout-003",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("sleeping 1.5s, then sending a message (resets timer)")
			time.Sleep(1500 * time.Millisecond)
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-timeout-003",
				"action":  "message",
				"message": "keep alive 1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("sleeping 1.5s again, then sending another message (resets timer again)")
			time.Sleep(1500 * time.Millisecond)
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-timeout-003",
				"action":  "message",
				"message": "keep alive 2",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse(),
				"session should still be active — total elapsed > 2s but never idle > 2s")

			_, _ = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-timeout-003",
				"action": "complete",
			})
		})
	})
})

// suppress unused import
var _ = mcpsdk.Implementation{}
