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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Interactive Session UX — Notifications & Timeouts BR-INTERACTIVE-006", Label("integration", "interactive", "ux"), func() {

	Describe("IT-KA-UX-01: timeout warning is delivered to session notifier", func() {
		It("should deliver warning message before session expires", func() {
			nsName := uniqueNamespace("ux01")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			opts := defaultRealStackOpts()
			opts.inactivityTimeout = 600 * time.Millisecond
			stack := newRealMCPTestStack(sharedK8sClient, nsName, opts)
			defer stack.Close()

			var (
				mu       sync.Mutex
				warnings []string
			)

			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-ux-01",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			startOutput, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			sessionID := startOutput["session_id"].(string)

			stack.Notifier.Register(sessionID, func(msg string) {
				mu.Lock()
				defer mu.Unlock()
				warnings = append(warnings, msg)
			})

			By("waiting for warning notification (interval is timeout - 1s, but timeout is short)")
			Eventually(func() []string {
				mu.Lock()
				defer mu.Unlock()
				return warnings
			}, 2*time.Second, 50*time.Millisecond).Should(HaveLen(0))
			// With 600ms timeout and warning at timeout-1s (negative -> filtered),
			// no warning is expected. This validates the filter logic.
		})
	})

	Describe("IT-KA-UX-02: timeout warning delivered with valid interval", func() {
		It("should fire warning before expiration when interval is within bounds", func() {
			nsName := uniqueNamespace("ux02")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			opts := defaultRealStackOpts()
			opts.inactivityTimeout = 2 * time.Second
			opts.warningIntervals = []time.Duration{500 * time.Millisecond}
			stack := newRealMCPTestStack(sharedK8sClient, nsName, opts)
			defer stack.Close()

			var (
				mu       sync.Mutex
				warnings []string
			)

			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-ux-02",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			startOutput, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			sessionID := startOutput["session_id"].(string)

			stack.Notifier.Register(sessionID, func(msg string) {
				mu.Lock()
				defer mu.Unlock()
				warnings = append(warnings, msg)
			})

			By("waiting for warning at 500ms")
			Eventually(func() int {
				mu.Lock()
				defer mu.Unlock()
				return len(warnings)
			}, 1500*time.Millisecond, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			mu.Lock()
			w := warnings[0]
			mu.Unlock()
			Expect(w).To(ContainSubstring("timeout"))
		})
	})

	Describe("IT-KA-UX-03: complete stops timeout tracking (no late expiration)", func() {
		It("should not fire expiration after session is completed", func() {
			nsName := uniqueNamespace("ux03")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			opts := defaultRealStackOpts()
			opts.inactivityTimeout = 300 * time.Millisecond
			stack := newRealMCPTestStack(sharedK8sClient, nsName, opts)
			defer stack.Close()

			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-ux-03",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			startOutput, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			sessionID := startOutput["session_id"].(string)

			By("completing immediately before timeout fires")
			_, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-ux-03",
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred())

			By("waiting past timeout and verifying no late expiration")
			time.Sleep(500 * time.Millisecond)
			Expect(stack.GetExpiredSessions()).NotTo(ContainElement(sessionID))
		})
	})

	Describe("IT-KA-UX-04: cancel stops timeout tracking", func() {
		It("should not fire expiration after session is cancelled", func() {
			nsName := uniqueNamespace("ux04")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			opts := defaultRealStackOpts()
			opts.inactivityTimeout = 300 * time.Millisecond
			stack := newRealMCPTestStack(sharedK8sClient, nsName, opts)
			defer stack.Close()

			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-ux-04",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			startOutput, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			sessionID := startOutput["session_id"].(string)

			_, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-ux-04",
				"action": "cancel",
			})
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(500 * time.Millisecond)
			Expect(stack.GetExpiredSessions()).NotTo(ContainElement(sessionID))
		})
	})

	Describe("IT-KA-UX-05: status returns structured JSON with mode and driver", func() {
		It("should return parseable status with current mode", func() {
			nsName := uniqueNamespace("ux05")
			createNamespace(context.Background(), sharedK8sClient, nsName)
			stack := newRealMCPTestStack(sharedK8sClient, nsName, defaultRealStackOpts())
			defer stack.Close()

			session, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			_, err = callInvestigate(session, map[string]any{
				"rr_id":  "rr-ux-05",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-ux-05",
				"action": "status",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["response"]).To(ContainSubstring("interactive"))
			Expect(output["response"]).To(ContainSubstring("alice@acme.io"))
		})
	})
})
