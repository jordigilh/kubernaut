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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("GracefulSessionClosedHandler — BR-INTERACTIVE-001", Label("unit", "disconnect", "1442"), func() {

	Describe("UT-KA-1442-001: Disconnect starts grace timer, does NOT release lease immediately", func() {
		It("should defer the onClose callback for the grace period duration", func() {
			es := mcpinternal.NewDelegatingEventStore()
			es.RegisterMCPSession("mcp-001", "interactive-001")

			var (
				mu     sync.Mutex
				closed []string
			)
			handler := mcpinternal.NewGracefulSessionClosedHandler(es, func(mcpSessionID string) {
				mu.Lock()
				defer mu.Unlock()
				closed = append(closed, mcpSessionID)
			}, 500*time.Millisecond, logr.Discard())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go handler.Run(ctx)

			By("simulating MCP disconnect")
			Expect(es.SessionClosed(context.Background(), "mcp-001")).To(Succeed())

			By("verifying onClose is NOT called immediately")
			Consistently(func() int {
				mu.Lock()
				defer mu.Unlock()
				return len(closed)
			}, 200*time.Millisecond, 50*time.Millisecond).Should(Equal(0),
				"onClose must NOT fire during grace period")

			By("verifying pending count is 1")
			Eventually(handler.PendingCount, 100*time.Millisecond, 10*time.Millisecond).Should(Equal(1))

			By("waiting for grace period to expire and verifying onClose fires")
			Eventually(func() []string {
				mu.Lock()
				defer mu.Unlock()
				cp := make([]string, len(closed))
				copy(cp, closed)
				return cp
			}, 1*time.Second, 50*time.Millisecond).Should(ConsistOf("mcp-001"),
				"onClose must fire after grace period expires")
		})
	})

	Describe("UT-KA-1442-002: CancelPendingRelease cancels timer before expiry", func() {
		It("should prevent onClose from ever firing when cancelled before grace period", func() {
			es := mcpinternal.NewDelegatingEventStore()
			es.RegisterMCPSession("mcp-002", "interactive-002")

			var (
				mu     sync.Mutex
				closed []string
			)
			handler := mcpinternal.NewGracefulSessionClosedHandler(es, func(mcpSessionID string) {
				mu.Lock()
				defer mu.Unlock()
				closed = append(closed, mcpSessionID)
			}, 500*time.Millisecond, logr.Discard())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go handler.Run(ctx)

			By("simulating MCP disconnect")
			Expect(es.SessionClosed(context.Background(), "mcp-002")).To(Succeed())

			By("waiting for the handler to register the pending release")
			Eventually(handler.PendingCount, 200*time.Millisecond, 10*time.Millisecond).Should(Equal(1))

			By("cancelling the pending release (simulating reconnect)")
			cancelled := handler.CancelPendingRelease("interactive-002")
			Expect(cancelled).To(BeTrue(), "CancelPendingRelease should return true when timer was active")
			Expect(handler.PendingCount()).To(Equal(0))

			By("verifying onClose is never called even after grace period")
			Consistently(func() int {
				mu.Lock()
				defer mu.Unlock()
				return len(closed)
			}, 700*time.Millisecond, 50*time.Millisecond).Should(Equal(0),
				"onClose must NOT fire after cancel")
		})
	})

	Describe("UT-KA-1442-003: Timer expiry triggers release and reconstruction", func() {
		It("should invoke onClose with the original MCP session ID after grace period", func() {
			es := mcpinternal.NewDelegatingEventStore()
			es.RegisterMCPSession("mcp-003", "interactive-003")

			var (
				mu     sync.Mutex
				closed []string
			)
			handler := mcpinternal.NewGracefulSessionClosedHandler(es, func(mcpSessionID string) {
				mu.Lock()
				defer mu.Unlock()
				closed = append(closed, mcpSessionID)
			}, 100*time.Millisecond, logr.Discard())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go handler.Run(ctx)

			Expect(es.SessionClosed(context.Background(), "mcp-003")).To(Succeed())

			Eventually(func() []string {
				mu.Lock()
				defer mu.Unlock()
				cp := make([]string, len(closed))
				copy(cp, closed)
				return cp
			}, 500*time.Millisecond, 20*time.Millisecond).Should(ConsistOf("mcp-003"),
				"onClose must fire exactly once with the original MCP session ID")

			Expect(handler.PendingCount()).To(Equal(0),
				"pending map must be cleaned up after timer fires")
		})
	})

	Describe("UT-KA-1442-005: Context cancellation stops Run and cancels pending timers", func() {
		It("should cancel all pending timers when context is cancelled", func() {
			es := mcpinternal.NewDelegatingEventStore()
			es.RegisterMCPSession("mcp-005", "interactive-005")

			var (
				mu     sync.Mutex
				closed []string
			)
			handler := mcpinternal.NewGracefulSessionClosedHandler(es, func(mcpSessionID string) {
				mu.Lock()
				defer mu.Unlock()
				closed = append(closed, mcpSessionID)
			}, 2*time.Second, logr.Discard())

			ctx, cancel := context.WithCancel(context.Background())
			go handler.Run(ctx)

			Expect(es.SessionClosed(context.Background(), "mcp-005")).To(Succeed())
			Eventually(handler.PendingCount, 200*time.Millisecond, 10*time.Millisecond).Should(Equal(1))

			By("cancelling context")
			cancel()

			By("verifying onClose is NOT called and pending map is cleaned")
			time.Sleep(100 * time.Millisecond)
			Expect(handler.PendingCount()).To(Equal(0))
			mu.Lock()
			Expect(closed).To(BeEmpty(), "onClose must NOT fire after context cancel")
			mu.Unlock()
		})
	})

	Describe("UT-KA-1442-006: Multiple disconnects tracked independently", func() {
		It("should maintain separate timers for different interactive sessions", func() {
			es := mcpinternal.NewDelegatingEventStore()
			es.RegisterMCPSession("mcp-006a", "interactive-006a")
			es.RegisterMCPSession("mcp-006b", "interactive-006b")

			var (
				mu     sync.Mutex
				closed []string
			)
			handler := mcpinternal.NewGracefulSessionClosedHandler(es, func(mcpSessionID string) {
				mu.Lock()
				defer mu.Unlock()
				closed = append(closed, mcpSessionID)
			}, 500*time.Millisecond, logr.Discard())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go handler.Run(ctx)

			Expect(es.SessionClosed(context.Background(), "mcp-006a")).To(Succeed())
			Expect(es.SessionClosed(context.Background(), "mcp-006b")).To(Succeed())
			Eventually(handler.PendingCount, 200*time.Millisecond, 10*time.Millisecond).Should(Equal(2))

			By("cancelling only one session's pending release")
			cancelled := handler.CancelPendingRelease("interactive-006a")
			Expect(cancelled).To(BeTrue())
			Expect(handler.PendingCount()).To(Equal(1))

			By("verifying only the other session fires onClose")
			Eventually(func() []string {
				mu.Lock()
				defer mu.Unlock()
				cp := make([]string, len(closed))
				copy(cp, closed)
				return cp
			}, 1*time.Second, 50*time.Millisecond).Should(ConsistOf("mcp-006b"))
		})
	})
})

var _ = Describe("LeaseSessionManager.SetReconnectCallback — BR-INTERACTIVE-001", Label("unit", "1442"), func() {

	Describe("UT-KA-1442-004: Takeover reconnect triggers callback with session ID", func() {
		It("should call the reconnect callback when same user re-takes the same rrID", func() {
			scheme := runtime.NewScheme()
			Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			logger := logr.Discard()

			var (
				mu             sync.Mutex
				reconnectedIDs []string
			)

			leaseMgr := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, "default", logger,
				mcpinternal.WithSessionTTL(30*time.Minute),
			)
			leaseMgr.SetReconnectCallback(func(sessionID string) {
				mu.Lock()
				defer mu.Unlock()
				reconnectedIDs = append(reconnectedIDs, sessionID)
			})

			user := mcpinternal.UserInfo{Username: "alice"}

			By("first takeover creates a new session")
			session1, err := leaseMgr.Takeover(context.Background(), "rr-reconnect-001", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(session1).NotTo(BeNil())

			By("second takeover for same user + rrID triggers reconnect")
			session2, err := leaseMgr.Takeover(context.Background(), "rr-reconnect-001", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(session2.SessionID).To(Equal(session1.SessionID))
			Expect(session2.Reconnected).To(BeTrue())

			mu.Lock()
			defer mu.Unlock()
			Expect(reconnectedIDs).To(ConsistOf(session1.SessionID),
				"reconnect callback must fire with the session ID on same-user reconnect")
		})
	})
})
