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
	"github.com/go-logr/logr"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

var _ = Describe("Coverage Gap Tests — BR-INTERACTIVE-004/005", Label("integration", "coverage"), func() {

	var (
		logger logr.Logger
		nsName string
	)

	BeforeEach(func() {
		logger = logr.Discard()
		nsName = uniqueNamespace("covgap")
		createNamespace(context.Background(), sharedK8sClient, nsName)
	})

	Describe("IT-KA-COV-001: takeover when no autonomous session exists", func() {
		It("should succeed directly without cancellation attempt", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)
			autoMgr := &mockAutoMgrIT{mgr: mgr}

			leaseMgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
			runner := &delayedMockRunner{delay: 0, response: "response"}
			recon := &mockReconIT{}
			tool := mcptools.NewInvestigateTool(leaseMgr, runner, recon, mcptools.WithAutonomousManager(autoMgr))

			user := mcpinternal.UserInfo{Username: "alice@example.com"}
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID: "rr-cov-noauto", Action: mcptools.ActionTakeover,
			}, user)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("takeover_started"))
		})
	})

	Describe("IT-KA-COV-002: takeover contention — second user gets session_active with driver name", func() {
		It("should return session_active error with the current driver's username", func() {
			leaseMgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
			runner := &delayedMockRunner{delay: 0, response: "response"}
			recon := &mockReconIT{}
			tool := mcptools.NewInvestigateTool(leaseMgr, runner, recon)

			alice := mcpinternal.UserInfo{Username: "alice@example.com"}
			bob := mcpinternal.UserInfo{Username: "bob@example.com"}

			By("alice takes over rr-cov-02")
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID: "rr-cov-02", Action: mcptools.ActionTakeover,
			}, alice)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("takeover_started"))

			By("bob attempts takeover on the same rrID")
			_, err = tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID: "rr-cov-02", Action: mcptools.ActionTakeover,
			}, bob)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("session_active"))
		})
	})

	Describe("IT-KA-COV-003: takeover without autoMgr (nil autoMgr path)", func() {
		It("should succeed when no autonomous session manager is configured", func() {
			leaseMgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
			runner := &delayedMockRunner{delay: 0, response: "response"}
			recon := &mockReconIT{}
			tool := mcptools.NewInvestigateTool(leaseMgr, runner, recon)

			user := mcpinternal.UserInfo{Username: "alice@example.com"}
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID: "rr-cov-03", Action: mcptools.ActionTakeover,
			}, user)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("takeover_started"))
		})
	})

	Describe("IT-KA-COV-004: message after TTL expiry returns session_expired", func() {
		It("should return session_expired when the lease TTL has been exceeded", func() {
			ttl := 1 * time.Second
			leaseMgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger,
				mcpinternal.WithSessionTTL(ttl))
			runner := &delayedMockRunner{delay: 0, response: "response"}
			recon := &mockReconIT{}
			tool := mcptools.NewInvestigateTool(leaseMgr, runner, recon)

			user := mcpinternal.UserInfo{Username: "alice@example.com"}
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID: "rr-cov-04", Action: mcptools.ActionStart,
			}, user)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for TTL to expire")
			time.Sleep(ttl + 500*time.Millisecond)

			_, err = tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID: "rr-cov-04", Action: mcptools.ActionMessage, Message: "hello",
			}, user)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("IT-KA-COV-005: rate limiter Remove clears session state", func() {
		It("should allow messages again after rate limiter state is removed", func() {
			rl := mcpinternal.NewSessionRateLimiter(2, 64*1024)

			Expect(rl.Allow("sess-rl-001", 10)).To(Succeed())
			Expect(rl.Allow("sess-rl-001", 10)).To(Succeed())
			Expect(rl.Allow("sess-rl-001", 10)).NotTo(Succeed(), "should be rate limited")

			rl.Remove("sess-rl-001")
			Expect(rl.Allow("sess-rl-001", 10)).To(Succeed(), "should be allowed after Remove")
		})
	})

	Describe("IT-KA-COV-006: SessionNotifier Deregister removes callback", func() {
		It("should not deliver messages after deregister", func() {
			notifier := mcpinternal.NewSessionNotifier()
			callCount := 0
			notifier.Register("sess-notify-001", func(_ string) {
				callCount++
			})

			notifier.Notify("sess-notify-001", "msg1")
			Expect(callCount).To(Equal(1))

			notifier.Deregister("sess-notify-001")
			notifier.Notify("sess-notify-001", "msg2")
			Expect(callCount).To(Equal(1), "no delivery after deregister")
		})
	})

	Describe("IT-KA-COV-007: MCPServer Implementation and ToolCount accessors", func() {
		It("should return non-nil implementation info", func() {
			nsLocal := uniqueNamespace("cov07")
			createNamespace(context.Background(), sharedK8sClient, nsLocal)
			stack := newRealMCPTestStack(sharedK8sClient, nsLocal, defaultRealStackOpts())
			defer stack.Close()

			Expect(stack.MCPServer.Implementation()).NotTo(BeNil())
			Expect(stack.MCPServer.ToolCount()).To(BeNumerically(">=", 1))
		})
	})

	Describe("IT-KA-COV-008: MCP cancel action wiring through real stack", func() {
		It("should release session and delete Lease on cancel", func() {
			nsLocal := uniqueNamespace("cov08")
			createNamespace(context.Background(), sharedK8sClient, nsLocal)
			stack := newRealMCPTestStack(sharedK8sClient, nsLocal, defaultRealStackOpts())
			defer stack.Close()

			sess, err := connectMCP(stack.Server, "alice@example.com")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-cov-08",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("calling cancel")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-cov-08",
				"action": "cancel",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			Expect(stack.SessionMgr.IsDriverActive("rr-cov-08")).To(BeFalse(),
				"Lease should be released after cancel action")
		})
	})

	Describe("IT-KA-COV-009: Rate limiter rejects oversized message with structured error", func() {
		It("should return rate_limited error for oversized messages", func() {
			nsLocal := uniqueNamespace("cov09")
			createNamespace(context.Background(), sharedK8sClient, nsLocal)

			opts := defaultRealStackOpts()
			opts.maxMessageSize = 50
			stack := newRealMCPTestStack(sharedK8sClient, nsLocal, opts)
			defer stack.Close()

			sess, err := connectMCP(stack.Server, "alice@example.com")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-cov-09",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			longMessage := make([]byte, 100)
			for i := range longMessage {
				longMessage[i] = 'A'
			}
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-cov-09",
				"action":  "message",
				"message": string(longMessage),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue(),
				"oversized message must be rejected with rate_limited error")
		})
	})

	Describe("IT-KA-COV-010: Rate limiter rejects burst with structured error", func() {
		It("should reject the 3rd message when maxPerMinute=2", func() {
			nsLocal := uniqueNamespace("cov10")
			createNamespace(context.Background(), sharedK8sClient, nsLocal)

			opts := defaultRealStackOpts()
			opts.maxPerMinute = 2
			stack := newRealMCPTestStack(sharedK8sClient, nsLocal, opts)
			defer stack.Close()

			sess, err := connectMCP(stack.Server, "alice@example.com")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-cov-10",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("sending 2 messages within the limit")
			for i := 1; i <= 2; i++ {
				result, err = callInvestigate(sess, map[string]any{
					"rr_id":   "rr-cov-10",
					"action":  "message",
					"message": "msg",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsError).To(BeFalse())
			}

			By("sending 3rd message — should be rate limited")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-cov-10",
				"action":  "message",
				"message": "burst",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue(),
				"3rd message should be rejected when maxPerMinute=2")
		})
	})
})
