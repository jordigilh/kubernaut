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

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("GracefulSessionClosedHandler IT — BR-INTERACTIVE-001, #1442", Label("integration", "disconnect", "1442"), func() {

	Describe("IT-KA-1442-001: Disconnect within grace period + reconnect preserves lease", func() {
		It("should defer release and cancel when a reconnect re-registers the same rrID", func() {
			es := mcpinternal.NewDelegatingEventStore()
			es.RegisterMCPSession("mcp-it-001", "interactive-it-001")

			var (
				mu     sync.Mutex
				closed []string
			)
			gracePeriod := 500 * time.Millisecond
			handler := mcpinternal.NewGracefulSessionClosedHandler(es, func(mcpSessionID string) {
				mu.Lock()
				defer mu.Unlock()
				closed = append(closed, mcpSessionID)
			}, gracePeriod, logr.Discard())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go handler.Run(ctx)

			By("simulating MCP disconnect (old session closes)")
			Expect(es.SessionClosed(context.Background(), "mcp-it-001")).To(Succeed())

			By("waiting for handler to register the pending release")
			Eventually(handler.PendingCount, 200*time.Millisecond, 10*time.Millisecond).Should(Equal(1))

			By("simulating reconnect: new MCP session registered, pending release cancelled")
			es.RegisterMCPSession("mcp-it-001-new", "interactive-it-001")
			cancelled := handler.CancelPendingRelease("interactive-it-001")
			Expect(cancelled).To(BeTrue(), "CancelPendingRelease must succeed for an active pending release")

			By("verifying the lease was never released (onClose never called)")
			Consistently(func() int {
				mu.Lock()
				defer mu.Unlock()
				return len(closed)
			}, gracePeriod+200*time.Millisecond, 50*time.Millisecond).Should(Equal(0),
				"onClose must NOT fire after CancelPendingRelease — lease preserved")
		})
	})

	Describe("IT-KA-1442-002: ReattachMCPSession via EventStore maps new MCP session", func() {
		It("should register new MCP session for the same interactive session after reconnect", func() {
			es := mcpinternal.NewDelegatingEventStore()

			By("registering original MCP-to-interactive mapping")
			es.RegisterMCPSession("mcp-it-002-old", "interactive-it-002")

			interactiveID, found := es.LookupInteractiveSession("mcp-it-002-old")
			Expect(found).To(BeTrue())
			Expect(interactiveID).To(Equal("interactive-it-002"))

			By("registering new MCP session for same interactive session (reconnect)")
			es.RegisterMCPSession("mcp-it-002-new", "interactive-it-002")

			newInteractiveID, found := es.LookupInteractiveSession("mcp-it-002-new")
			Expect(found).To(BeTrue())
			Expect(newInteractiveID).To(Equal("interactive-it-002"),
				"new MCP session must map to the same interactive session after reconnect")

			By("cleaning up old mapping")
			es.DeleteMCPSession("mcp-it-002-old")
			_, found = es.LookupInteractiveSession("mcp-it-002-old")
			Expect(found).To(BeFalse(), "old MCP mapping must be cleaned up")
		})
	})

	Describe("IT-KA-1442-003: Grace period expires without reconnect triggers full release", func() {
		It("should invoke onClose after grace period when no reconnect occurs", func() {
			es := mcpinternal.NewDelegatingEventStore()
			es.RegisterMCPSession("mcp-it-003", "interactive-it-003")

			var (
				mu     sync.Mutex
				closed []string
			)
			gracePeriod := 150 * time.Millisecond
			handler := mcpinternal.NewGracefulSessionClosedHandler(es, func(mcpSessionID string) {
				mu.Lock()
				defer mu.Unlock()
				closed = append(closed, mcpSessionID)
			}, gracePeriod, logr.Discard())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go handler.Run(ctx)

			Expect(es.SessionClosed(context.Background(), "mcp-it-003")).To(Succeed())

			By("verifying onClose fires after grace period (no reconnect)")
			Eventually(func() []string {
				mu.Lock()
				defer mu.Unlock()
				cp := make([]string, len(closed))
				copy(cp, closed)
				return cp
			}, 1*time.Second, 50*time.Millisecond).Should(ConsistOf("mcp-it-003"),
				"onClose must fire after grace period when no reconnect occurs")

			Expect(handler.PendingCount()).To(Equal(0),
				"pending map must be empty after release fires")
		})
	})
})
