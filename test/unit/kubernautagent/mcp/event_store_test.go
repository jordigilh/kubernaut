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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("DelegatingEventStore + SessionClosedHandler + Janitor — PR4 OPS-01 DES-01", func() {

	Describe("UT-KA-SESS-009: EventStore.Open delegates to SDK MemoryEventStore", func() {
		It("should forward Open calls to the wrapped SDK EventStore", func() {
			des := mcpinternal.NewDelegatingEventStore()

			err := des.Open(context.Background(), "mcp-session-001", "stream-1")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-SESS-010: EventStore.SessionClosed publishes to closedSessions channel", func() {
		It("should send sessionID to the closedSessions channel on SessionClosed", func() {
			des := mcpinternal.NewDelegatingEventStore()

			des.RegisterMCPSession("mcp-sess-001", "interactive-sess-001")

			err := des.SessionClosed(context.Background(), "mcp-sess-001")
			Expect(err).NotTo(HaveOccurred())

			Eventually(des.ClosedSessions()).Should(Receive(Equal("mcp-sess-001")))
		})
	})

	Describe("UT-KA-SESS-006: MCP disconnect cleanup — SessionClosed triggers Release", func() {
		It("should trigger session Release when SessionClosed is called", func() {
			des := mcpinternal.NewDelegatingEventStore()

			released := make(chan string, 1)
			handler := mcpinternal.NewSessionClosedHandler(des, func(mcpSessionID string) {
				released <- mcpSessionID
			}, logr.Discard())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go handler.Run(ctx)

			des.RegisterMCPSession("mcp-sess-002", "interactive-sess-002")
			err := des.SessionClosed(context.Background(), "mcp-sess-002")
			Expect(err).NotTo(HaveOccurred())

			Eventually(released).Should(Receive(Equal("mcp-sess-002")))
		})
	})

	Describe("UT-KA-SESS-011: SessionClosedHandler calls Release with reason disconnect", func() {
		It("should invoke the release callback with the closed session ID", func() {
			des := mcpinternal.NewDelegatingEventStore()

			released := make(chan string, 1)
			handler := mcpinternal.NewSessionClosedHandler(des, func(mcpSessionID string) {
				released <- mcpSessionID
			}, logr.Discard())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go handler.Run(ctx)

			des.RegisterMCPSession("mcp-sess-003", "interactive-sess-003")
			_ = des.SessionClosed(context.Background(), "mcp-sess-003")

			Eventually(released).Should(Receive(Equal("mcp-sess-003")))
		})
	})

	Describe("UT-KA-SESS-012: Janitor removes stale sessions exceeding TTL", func() {
		It("should clean sessions older than TTL", func() {
			janitor := mcpinternal.NewSessionJanitor(50*time.Millisecond, logr.Discard())

			expiredCh := make(chan string, 1)
			janitor.Track("stale-sess-001", time.Now().Add(-1*time.Hour), func(sessionID string) {
				expiredCh <- sessionID
			})

			ctx, cancel := context.WithCancel(context.Background())
			go janitor.Run(ctx)

			Eventually(expiredCh).Should(Receive(Equal("stale-sess-001")))
			cancel()
		})
	})
})
