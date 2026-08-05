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

package investigator

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// White-box (package investigator) because this calls chatOrStream directly
// — the exact unexported function named in #1949's Component 3 fix.

// capturingStreamClient records the ctx it was called with (so the test can
// assert on its deadline) and returns immediately — no need to actually
// block for the full fallback duration to prove a deadline was applied.
type capturingStreamClient struct {
	mu          sync.Mutex
	capturedCtx context.Context
}

func (c *capturingStreamClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, nil
}

func (c *capturingStreamClient) StreamChat(ctx context.Context, _ llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	c.mu.Lock()
	c.capturedCtx = ctx
	c.mu.Unlock()
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "ok"}}, nil
}

func (c *capturingStreamClient) Close() error { return nil }

func (c *capturingStreamClient) ctx() context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.capturedCtx
}

var _ = Describe("Kubernaut Agent Investigator — chatOrStream unconditional timeout (#1949, SC-5)", func() {

	Describe("UT-KA-1949-006: chatOrStream applies a default timeout even when RuntimeParams.TimeoutSeconds == 0", func() {
		It("sets a deadline on the context passed to StreamChat instead of forwarding the bare, unbounded parent ctx", func() {
			inv := New(Config{
				Logger:     logr.Discard(),
				AuditStore: audit.NopAuditStore{},
				Metrics:    nil,
			})

			// A sink in ctx forces the streaming branch — the branch
			// #1949's Component 3 fixes (the non-streaming ChatWithParams
			// path is a separate call chain, out of scope here).
			eventCh := make(chan session.InvestigationEvent, 8)
			ctx := session.WithEventSink(context.Background(), eventCh)

			req := llm.ChatRequest{Messages: []llm.Message{{Role: "user", Content: "diagnose"}}}
			client := &capturingStreamClient{}

			_, err := inv.chatOrStream(ctx, client, req, 0, "rca", "test-model", llm.RuntimeParams{TimeoutSeconds: 0})
			Expect(err).NotTo(HaveOccurred())

			deadline, ok := client.ctx().Deadline()
			Expect(ok).To(BeTrue(), "chatOrStream must apply a default timeout when RuntimeParams.TimeoutSeconds is unset (0), not pass the bare unbounded parent ctx straight through to StreamChat")
			Expect(deadline).To(BeTemporally("~", time.Now().Add(DefaultLLMCallTimeout), 5*time.Second))
		})
	})

	Describe("UT-KA-1949-007: chatOrStream still honors an explicit RuntimeParams.TimeoutSeconds", func() {
		It("sets a deadline derived from TimeoutSeconds, not the default fallback, when TimeoutSeconds > 0", func() {
			inv := New(Config{
				Logger:     logr.Discard(),
				AuditStore: audit.NopAuditStore{},
				Metrics:    nil,
			})

			eventCh := make(chan session.InvestigationEvent, 8)
			ctx := session.WithEventSink(context.Background(), eventCh)

			req := llm.ChatRequest{Messages: []llm.Message{{Role: "user", Content: "diagnose"}}}
			client := &capturingStreamClient{}

			_, err := inv.chatOrStream(ctx, client, req, 0, "rca", "test-model", llm.RuntimeParams{TimeoutSeconds: 5})
			Expect(err).NotTo(HaveOccurred())

			deadline, ok := client.ctx().Deadline()
			Expect(ok).To(BeTrue())
			Expect(deadline).To(BeTemporally("~", time.Now().Add(5*time.Second), 2*time.Second),
				"an explicitly configured TimeoutSeconds must still take precedence over DefaultLLMCallTimeout")
		})
	})
})
