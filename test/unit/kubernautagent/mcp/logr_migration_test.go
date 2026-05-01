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
	"errors"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("Issue #885: slog-to-logr migration — MCP layer", func() {

	Describe("UT-KA-885-001: disconnect_handler constructors accept logr.Logger", func() {
		It("NewSessionClosedHandler accepts logr.Logger and processes events", func() {
			logger := logr.Discard()
			es := mcpinternal.NewDelegatingEventStore()

			closedCh := make(chan string, 1)
			handler := mcpinternal.NewSessionClosedHandler(es, func(id string) {
				closedCh <- id
			}, logger)
			Expect(handler).NotTo(BeNil())

			es.RegisterMCPSession("mcp-logr-001", "int-sess-001")
			err := es.SessionClosed(context.Background(), "mcp-logr-001")
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			go handler.Run(ctx)

			Eventually(closedCh, 2*time.Second).Should(Receive(Equal("mcp-logr-001")))
		})

		It("NewSessionJanitor accepts logr.Logger", func() {
			logger := logr.Discard()
			janitor := mcpinternal.NewSessionJanitor(30*time.Second, logger)
			Expect(janitor).NotTo(BeNil())
		})
	})

	Describe("UT-KA-885-002: session_manager constructors accept logr.Logger", func() {
		It("NewLeaseSessionManagerConcrete accepts logr.Logger", func() {
			logger := logr.Discard()
			mgr := mcpinternal.NewLeaseSessionManagerConcrete(nil, "test-ns", logger)
			Expect(mgr).NotTo(BeNil())
		})

		It("NewLeaseSessionManager accepts logr.Logger and returns SessionManager", func() {
			logger := logr.Discard()
			mgr := mcpinternal.NewLeaseSessionManager(nil, "test-ns", logger)
			Expect(mgr).NotTo(BeNil())
		})
	})

	Describe("UT-KA-885-003: ds_reconstructor constructor accepts logr.Logger", func() {
		It("NewDSContextReconstructor accepts logr.Logger", func() {
			logger := logr.Discard()
			recon := mcpinternal.NewDSContextReconstructor(nil, 5*time.Second, logger)
			Expect(recon).NotTo(BeNil())
		})
	})

	Describe("UT-KA-885-004: reconstruct spawner accepts logr.Logger and handles panic", func() {
		It("NewReconstructionSpawner accepts logr.Logger", func() {
			logger := logr.Discard()
			runner := &logrReconRunner{}
			recon := &logrReconReconstructor{}
			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, logger)
			Expect(spawner).NotTo(BeNil())
		})

		It("recovers from panic and logs via logr.Error", func() {
			logger := logr.Discard()
			panicRunner := &logrPanicRunner{}
			recon := &logrReconReconstructor{}
			spawner := mcpinternal.NewReconstructionSpawner(panicRunner, recon, logger)

			entry := &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-logr-panic",
				SessionID:     "sess-logr-panic",
			}

			err := spawner.SpawnReconstruct(context.Background(), entry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("panic"))
		})
	})
})

type logrReconRunner struct {
	called atomic.Int32
}

func (r *logrReconRunner) RunReconTurn(_ context.Context, _ []mcpinternal.ReconMessage, _ string) (string, error) {
	r.called.Add(1)
	return "ok", nil
}

type logrPanicRunner struct{}

func (r *logrPanicRunner) RunReconTurn(_ context.Context, _ []mcpinternal.ReconMessage, _ string) (string, error) {
	panic("logr migration panic test")
}

type logrReconReconstructor struct{}

func (r *logrReconReconstructor) Reconstruct(_ context.Context, _, _ string) ([]mcpinternal.ConversationTurn, error) {
	return nil, errors.New("simulated DS failure")
}
