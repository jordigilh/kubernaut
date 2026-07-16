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
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var _ = Describe("Notification Bus Wiring — GAP-13 / BR-INTERACTIVE-005", Label("integration", "notification"), func() {

	// ---------------------------------------------------------------
	// IT-KA-NOTIF-001: Timeout warning reaches MCP client via notification bus
	//
	// Validates the full chain:
	//   TimeoutManager → SessionNotifier.Notify → registration.go callback
	//   → ServerSession.Log → MCP SDK LoggingMessage → client handler
	//
	// BR: BR-INTERACTIVE-005, UX-01
	// ---------------------------------------------------------------
	Describe("IT-KA-NOTIF-001: Timeout warning reaches MCP client via LoggingMessage", func() {
		It("should deliver inactivity warning to MCP client as LoggingMessage notification", func() {
			nsName := uniqueNamespace("notif01")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			opts := defaultRealStackOpts()
			opts.inactivityTimeout = 4 * time.Second
			opts.warningIntervals = []time.Duration{1 * time.Second}
			stack := newRealMCPTestStack(sharedK8sClient, nsName, opts)
			defer stack.Close()

			By("Creating MCP client with LoggingMessageHandler")
			var logMessagesReceived atomic.Int32
			var lastLogMessage atomic.Value

			mcpClient := mcpsdk.NewClient(&mcpsdk.Implementation{
				Name:    "notif-test-client",
				Version: "0.0.1",
			}, &mcpsdk.ClientOptions{
				LoggingMessageHandler: func(_ context.Context, req *mcpsdk.LoggingMessageRequest) {
					logMessagesReceived.Add(1)
					if msg, ok := req.Params.Data.(string); ok {
						lastLogMessage.Store(msg)
					}
				},
			})

			transport := &mcpsdk.StreamableClientTransport{
				Endpoint:   stack.Server.URL + "/mcp",
				HTTPClient: authedHTTPClient("alice@example.com"),
			}
			sess, err := mcpClient.Connect(context.Background(), transport, nil)
			Expect(err).NotTo(HaveOccurred())
			defer sess.Close()

			By("Setting log level to receive warning messages")
			err = sess.SetLoggingLevel(context.Background(), &mcpsdk.SetLoggingLevelParams{
				Level: "warning",
			})
			Expect(err).NotTo(HaveOccurred())

			By("Starting session to activate timeout tracking")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-notif-001",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("Waiting for warning interval to fire (1s into 4s timeout)")
			Eventually(logMessagesReceived.Load, 3*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1),
				"MCP client should receive at least one LoggingMessage notification")

			By("Asserting the warning message content")
			msg, ok := lastLogMessage.Load().(string)
			if ok {
				Expect(msg).To(ContainSubstring("timeout"),
					"warning message should mention timeout")
				GinkgoWriter.Printf("Received LoggingMessage: %s\n", msg)
			}

			By("Completing session before full expiry")
			_, _ = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-notif-001",
				"action": "complete",
			})

			GinkgoWriter.Println("NOTIF-001: Timeout warning via LoggingMessage validated")
		})
	})
})
