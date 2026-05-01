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

var _ = Describe("DelegatingEventStore + SessionClosedHandler IT — BR-INTERACTIVE-008", Label("integration", "disconnect"), func() {

	Describe("IT-KA-DES-001: SessionClosed fires on the closedSessions channel", func() {
		It("should deliver the mcpSessionID when SessionClosed is called", func() {
			es := mcpinternal.NewDelegatingEventStore()

			err := es.SessionClosed(context.Background(), "mcp-sess-001")
			Expect(err).NotTo(HaveOccurred())

			Eventually(es.ClosedSessions()).Should(Receive(Equal("mcp-sess-001")))
		})
	})

	Describe("IT-KA-DES-002: RegisterMCPSession + LookupInteractiveSession round-trip", func() {
		It("should map MCP session to interactive session ID", func() {
			es := mcpinternal.NewDelegatingEventStore()
			es.RegisterMCPSession("mcp-abc", "interactive-xyz")

			interactiveID, found := es.LookupInteractiveSession("mcp-abc")
			Expect(found).To(BeTrue())
			Expect(interactiveID).To(Equal("interactive-xyz"))

			_, found = es.LookupInteractiveSession("nonexistent")
			Expect(found).To(BeFalse())
		})
	})

	Describe("IT-KA-DES-003: SessionClosedHandler invokes onClose callback", func() {
		It("should call the callback for each disconnect event", func() {
			es := mcpinternal.NewDelegatingEventStore()
			logger := logr.Discard()

			var (
				mu     sync.Mutex
				closed []string
			)
			handler := mcpinternal.NewSessionClosedHandler(es, func(mcpSessionID string) {
				mu.Lock()
				defer mu.Unlock()
				closed = append(closed, mcpSessionID)
			}, logger)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go handler.Run(ctx)

			By("simulating two session close events")
			_ = es.SessionClosed(context.Background(), "sess-a")
			_ = es.SessionClosed(context.Background(), "sess-b")

			Eventually(func() int {
				mu.Lock()
				defer mu.Unlock()
				return len(closed)
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(2))

			mu.Lock()
			defer mu.Unlock()
			Expect(closed).To(ConsistOf("sess-a", "sess-b"))
		})
	})

	Describe("IT-KA-DES-004: SessionClosedHandler stops on context cancellation", func() {
		It("should exit Run when context is cancelled", func() {
			es := mcpinternal.NewDelegatingEventStore()
			logger := logr.Discard()

			handler := mcpinternal.NewSessionClosedHandler(es, func(_ string) {}, logger)
			ctx, cancel := context.WithCancel(context.Background())

			done := make(chan struct{})
			go func() {
				handler.Run(ctx)
				close(done)
			}()

			cancel()
			Eventually(done).Should(BeClosed())
		})
	})

	Describe("IT-KA-DES-005: SessionJanitor sweeps stale sessions", func() {
		It("should expire sessions older than the interval", func() {
			logger := logr.Discard()
			janitor := mcpinternal.NewSessionJanitor(200*time.Millisecond, logger)

			var (
				mu      sync.Mutex
				expired []string
			)

			janitor.Track("sess-stale", time.Now().Add(-5*time.Minute), func(id string) {
				mu.Lock()
				defer mu.Unlock()
				expired = append(expired, id)
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go janitor.Run(ctx)

			Eventually(func() []string {
				mu.Lock()
				defer mu.Unlock()
				cp := make([]string, len(expired))
				copy(cp, expired)
				return cp
			}, 2*time.Second, 50*time.Millisecond).Should(ContainElement("sess-stale"))
		})
	})

	Describe("IT-KA-DES-006: SessionJanitor does not expire tracked-then-untracked sessions", func() {
		It("should skip sessions that were untracked before sweep", func() {
			logger := logr.Discard()
			janitor := mcpinternal.NewSessionJanitor(200*time.Millisecond, logger)

			expiredCount := 0
			janitor.Track("sess-clean", time.Now().Add(-5*time.Minute), func(_ string) {
				expiredCount++
			})
			janitor.Untrack("sess-clean")

			ctx, cancel := context.WithCancel(context.Background())
			go janitor.Run(ctx)

			time.Sleep(500 * time.Millisecond)
			cancel()
			Expect(expiredCount).To(Equal(0))
		})
	})
})
