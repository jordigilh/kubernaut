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

// =============================================================================
// IT-KA-1442: Graceful Disconnect Wiring (BR-INTERACTIVE-001, #1442)
//
// Pyramid Invariant:
//   UT (graceful_disconnect_test.go in internal/) proves component logic.
//   IT (this file) proves wiring: GracefulSessionClosedHandler +
//   LeaseSessionManager.SetReconnectCallback are wired together as in
//   cmd/kubernautagent/main.go (buildMCPHandler), and the callback chain
//   works end-to-end through the production dispatch pattern.
//
// Production wiring under test (from cmd/kubernautagent/main.go):
//   disconnectHandler := mcpkg.NewGracefulSessionClosedHandler(eventStore, onClose, gracePeriod, logger)
//   leaseMgr.SetReconnectCallback(func(sessionID string) {
//       disconnectHandler.CancelPendingRelease(sessionID)
//   })
//   go disconnectHandler.Run(ctx)
// =============================================================================

var _ = Describe("Graceful Disconnect Wiring — BR-INTERACTIVE-001, #1442", Label("integration", "disconnect", "1442"), func() {

	Describe("IT-KA-1442-001: Disconnect → Takeover reconnect → CancelPendingRelease (full wiring)", func() {
		It("should cancel the pending release when Takeover triggers the reconnect callback", func() {
			scheme := runtime.NewScheme()
			Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			logger := logr.Discard()

			es := mcpinternal.NewDelegatingEventStore()
			es.RegisterMCPSession("mcp-wiring-001", "interactive-wiring-001")

			var (
				mu     sync.Mutex
				closed []string
			)
			gracePeriod := 500 * time.Millisecond

			disconnectHandler := mcpinternal.NewGracefulSessionClosedHandler(es, func(mcpSessionID string) {
				mu.Lock()
				defer mu.Unlock()
				closed = append(closed, mcpSessionID)
			}, gracePeriod, logger)

			leaseMgr := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, "default", logger,
				mcpinternal.WithSessionTTL(30*time.Minute),
			)

			By("wiring reconnect callback as main.go does")
			leaseMgr.SetReconnectCallback(func(sessionID string) {
				disconnectHandler.CancelPendingRelease(sessionID)
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go disconnectHandler.Run(ctx)

			user := mcpinternal.UserInfo{Username: "alice"}

			By("acquiring initial lease via Takeover")
			session, err := leaseMgr.Takeover(ctx, "rr-wiring-001", user)
			Expect(err).NotTo(HaveOccurred())

			By("registering interactive session in event store (as main.go does after Takeover)")
			es.RegisterMCPSession("mcp-wiring-001", session.SessionID)

			By("simulating MCP disconnect")
			Expect(es.SessionClosed(context.Background(), "mcp-wiring-001")).To(Succeed())
			Eventually(disconnectHandler.PendingCount, 200*time.Millisecond, 10*time.Millisecond).Should(Equal(1))

			By("simulating reconnect via Takeover (same user, same rrID)")
			session2, err := leaseMgr.Takeover(ctx, "rr-wiring-001", user)
			Expect(err).NotTo(HaveOccurred())
			Expect(session2.SessionID).To(Equal(session.SessionID))
			Expect(session2.Reconnected).To(BeTrue())

			By("verifying the reconnect callback cancelled the pending release")
			Expect(disconnectHandler.PendingCount()).To(Equal(0),
				"CancelPendingRelease must have been called by the reconnect callback")

			By("verifying onClose never fires (lease preserved)")
			Consistently(func() int {
				mu.Lock()
				defer mu.Unlock()
				return len(closed)
			}, gracePeriod+200*time.Millisecond, 50*time.Millisecond).Should(Equal(0),
				"onClose must NOT fire — reconnect callback cancelled the pending release")
		})
	})

	Describe("IT-KA-1442-002: Disconnect without reconnect → grace period expiry → onClose (full wiring)", func() {
		It("should invoke onClose after grace period when no Takeover reconnect occurs", func() {
			scheme := runtime.NewScheme()
			Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			logger := logr.Discard()

			es := mcpinternal.NewDelegatingEventStore()

			var (
				mu     sync.Mutex
				closed []string
			)
			gracePeriod := 150 * time.Millisecond

			disconnectHandler := mcpinternal.NewGracefulSessionClosedHandler(es, func(mcpSessionID string) {
				mu.Lock()
				defer mu.Unlock()
				closed = append(closed, mcpSessionID)
			}, gracePeriod, logger)

			leaseMgr := mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, "default", logger,
				mcpinternal.WithSessionTTL(30*time.Minute),
			)

			By("wiring reconnect callback as main.go does")
			leaseMgr.SetReconnectCallback(func(sessionID string) {
				disconnectHandler.CancelPendingRelease(sessionID)
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go disconnectHandler.Run(ctx)

			user := mcpinternal.UserInfo{Username: "bob"}

			By("acquiring lease and registering MCP session")
			session, err := leaseMgr.Takeover(ctx, "rr-wiring-002", user)
			Expect(err).NotTo(HaveOccurred())
			es.RegisterMCPSession("mcp-wiring-002", session.SessionID)

			By("simulating MCP disconnect (no reconnect follows)")
			Expect(es.SessionClosed(context.Background(), "mcp-wiring-002")).To(Succeed())

			By("verifying onClose fires after grace period")
			Eventually(func() []string {
				mu.Lock()
				defer mu.Unlock()
				cp := make([]string, len(closed))
				copy(cp, closed)
				return cp
			}, 1*time.Second, 50*time.Millisecond).Should(ConsistOf("mcp-wiring-002"),
				"onClose must fire after grace period when no reconnect occurs")
		})
	})
})
