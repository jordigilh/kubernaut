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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var _ = Describe("Interactive Session Security — HARM BR-INTERACTIVE-002/003", Label("integration", "interactive", "security"), func() {

	Describe("IT-KA-HARM-01: non-driver cannot complete another user's session", func() {
		It("should reject complete from a different user", func() {
			nsName := uniqueNamespace("harm01")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			stack := newRealMCPTestStack(sharedK8sClient, nsName, defaultRealStackOpts())
			defer stack.Close()

			By("alice starts a session")
			aliceSession, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = aliceSession.Close() }()

			_, err = callInvestigate(aliceSession, map[string]any{
				"rr_id":  "rr-harm-01",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("bob attempts to complete alice's session")
			bobSession, err := connectMCP(stack.Server, "bob@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = bobSession.Close() }()

			result, err := callInvestigate(bobSession, map[string]any{
				"rr_id":  "rr-harm-01",
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue(), "bob should not be able to complete alice's session")

			By("verifying alice's session is still active")
			aliceResult, err := callInvestigate(aliceSession, map[string]any{
				"rr_id":   "rr-harm-01",
				"action":  "message",
				"message": "Still here",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(aliceResult.IsError).To(BeFalse(), "alice's session should still be active")
		})
	})

	Describe("IT-KA-HARM-02: non-driver cannot send messages to another user's session", func() {
		It("should reject message from a different user", func() {
			nsName := uniqueNamespace("harm02")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			stack := newRealMCPTestStack(sharedK8sClient, nsName, defaultRealStackOpts())
			defer stack.Close()

			aliceSession, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = aliceSession.Close() }()

			_, err = callInvestigate(aliceSession, map[string]any{
				"rr_id":  "rr-harm-02",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			bobSession, err := connectMCP(stack.Server, "bob@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = bobSession.Close() }()

			result, err := callInvestigate(bobSession, map[string]any{
				"rr_id":   "rr-harm-02",
				"action":  "message",
				"message": "Trying to hijack",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue(), "bob should not be able to send messages to alice's session")
		})
	})

	Describe("IT-KA-HARM-03: non-driver cannot cancel another user's session", func() {
		It("should reject cancel from a different user", func() {
			nsName := uniqueNamespace("harm03")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			stack := newRealMCPTestStack(sharedK8sClient, nsName, defaultRealStackOpts())
			defer stack.Close()

			aliceSession, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = aliceSession.Close() }()

			_, err = callInvestigate(aliceSession, map[string]any{
				"rr_id":  "rr-harm-03",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			bobSession, err := connectMCP(stack.Server, "bob@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = bobSession.Close() }()

			result, err := callInvestigate(bobSession, map[string]any{
				"rr_id":  "rr-harm-03",
				"action": "cancel",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue(), "bob should not be able to cancel alice's session")
		})
	})

	Describe("IT-KA-HARM-04: rate limiter enforces max messages per minute", func() {
		It("should reject messages after exceeding rate limit", func() {
			nsName := uniqueNamespace("harm04")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			opts := defaultRealStackOpts()
			opts.maxPerMinute = 3
			stack := newRealMCPTestStack(sharedK8sClient, nsName, opts)
			defer stack.Close()

			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			_, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-harm-04",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("sending messages up to the limit")
			for i := 0; i < 3; i++ {
				result, err := callInvestigate(session, map[string]any{
					"rr_id":   "rr-harm-04",
					"action":  "message",
					"message": "msg",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsError).To(BeFalse(), "message %d should succeed", i+1)
			}

			By("exceeding the limit")
			result, err := callInvestigate(session, map[string]any{
				"rr_id":   "rr-harm-04",
				"action":  "message",
				"message": "one too many",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue(), "should be rate limited")
		})
	})

	Describe("IT-KA-HARM-06: rate limiter rejects oversized messages", func() {
		It("should reject messages exceeding maxMessageSize", func() {
			nsName := uniqueNamespace("harm06")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			opts := defaultRealStackOpts()
			opts.maxMessageSize = 100
			stack := newRealMCPTestStack(sharedK8sClient, nsName, opts)
			defer stack.Close()

			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			_, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-harm-06",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			bigMsg := strings.Repeat("x", 200)
			result, err := callInvestigate(session, map[string]any{
				"rr_id":   "rr-harm-06",
				"action":  "message",
				"message": bigMsg,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue(), "oversized message should be rejected")
		})
	})

	Describe("IT-KA-HARM-07: inactivity timeout expires session", func() {
		It("should expire the session and reject subsequent messages", func() {
			nsName := uniqueNamespace("harm07")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			opts := defaultRealStackOpts()
			opts.inactivityTimeout = 500 * time.Millisecond
			stack := newRealMCPTestStack(sharedK8sClient, nsName, opts)
			defer stack.Close()

			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-harm-07",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			startOutput, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			sessionID := startOutput["session_id"].(string)

			By("waiting for inactivity timeout")
			Eventually(func() []string {
				return stack.GetExpiredSessions()
			}, 2*time.Second, 50*time.Millisecond).Should(ContainElement(sessionID))

			By("verifying session is no longer active")
			result, err = callInvestigate(session, map[string]any{
				"rr_id":   "rr-harm-07",
				"action":  "message",
				"message": "Too late",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue(), "session should be expired")
		})
	})

	Describe("IT-KA-HARM-08: message resets inactivity timer", func() {
		It("should keep session alive when user is active", func() {
			nsName := uniqueNamespace("harm08")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			opts := defaultRealStackOpts()
			opts.inactivityTimeout = 800 * time.Millisecond
			stack := newRealMCPTestStack(sharedK8sClient, nsName, opts)
			defer stack.Close()

			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-harm-08",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			startOutput, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			sessionID := startOutput["session_id"].(string)

			By("sending messages to keep session alive")
			for i := 0; i < 3; i++ {
				time.Sleep(400 * time.Millisecond)
				result, err := callInvestigate(session, map[string]any{
					"rr_id":   "rr-harm-08",
					"action":  "message",
					"message": "keep alive",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsError).To(BeFalse(), "message %d should succeed - session should not have timed out", i+1)
			}

			Expect(stack.GetExpiredSessions()).NotTo(ContainElement(sessionID))
		})
	})

	Describe("IT-KA-HARM-09: unauthenticated request returns 401", func() {
		It("should reject connections without auth", func() {
			nsName := uniqueNamespace("harm09")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			stack := newRealMCPTestStack(sharedK8sClient, nsName, defaultRealStackOpts())
			defer stack.Close()

			client := mcpsdk.NewClient(&mcpsdk.Implementation{
				Name:    "no-auth-client",
				Version: "0.0.1",
			}, nil)

			transport := &mcpsdk.StreamableClientTransport{
				Endpoint: stack.Server.URL + "/mcp",
			}
			connectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := client.Connect(connectCtx, transport, nil)
			Expect(err).To(HaveOccurred())
		})
	})
})
