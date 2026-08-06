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
	"encoding/json"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// White-box (package investigator, not investigator_test) because these
// specs call processToolCalls directly, which is intentionally unexported —
// it is the exact production dispatch point named in #1949's RCA, and
// asserting on it directly (rather than walking the full Investigate()
// pipeline) keeps the hang scenario isolated to the one mechanism under
// test (Wiring Manifest: Investigator.processToolCalls).

// hangingToolRegistry.Execute blocks on ctx.Done() forever, exactly
// mirroring the "context-aware I/O call that never returns because nobody
// ever cancels ctx" shape identified in #1949's RCA (client-go/net-http
// tool implementations that honor ctx cancellation but received an
// unbounded parent ctx). Before the #1949 fix, calling this through
// processToolCalls with context.Background() would hang indefinitely.
type hangingToolRegistry struct{}

func (hangingToolRegistry) Execute(ctx context.Context, _ string, _ json.RawMessage) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}
func (hangingToolRegistry) ToolsForPhase(_ katypes.Phase, _ katypes.PhaseToolMap) []tools.Tool {
	return nil
}
func (hangingToolRegistry) All() []tools.Tool { return nil }

// instantToolRegistry.Execute returns immediately with a fixed payload,
// used to prove the timeout wrapper introduces no regression/false-positive
// timeout under normal (fast) tool latency.
type instantToolRegistry struct{}

func (instantToolRegistry) Execute(_ context.Context, _ string, _ json.RawMessage) (string, error) {
	return `{"ok":true}`, nil
}
func (instantToolRegistry) ToolsForPhase(_ katypes.Phase, _ katypes.PhaseToolMap) []tools.Tool {
	return nil
}
func (instantToolRegistry) All() []tools.Tool { return nil }

var _ = Describe("Kubernaut Agent Investigator — processToolCalls bounded execution (#1949, SC-5)", func() {

	Describe("UT-KA-1949-001: a hung tool call is bounded by ToolCallTimeout, not left to hang forever", func() {
		It("returns within ToolCallTimeout+margin and records a timeout error as the tool result", func() {
			inv := New(Config{
				Logger:          logr.Discard(),
				AuditStore:      audit.NopAuditStore{},
				Registry:        hangingToolRegistry{},
				ToolCallTimeout: 50 * time.Millisecond,
			})

			resp := llm.ChatResponse{
				Message: llm.Message{Role: "assistant", Content: "investigating"},
				ToolCalls: []llm.ToolCall{
					{ID: "tc_1", Name: "kubectl_describe", Arguments: `{}`},
				},
			}

			done := make(chan []llm.Message, 1)
			go func() {
				defer GinkgoRecover()
				messages, _, _ := inv.processToolCalls(context.Background(), nil, resp, 0, "rca", "corr-1949-001")
				done <- messages
			}()

			var messages []llm.Message
			Eventually(done, "1s", "10ms").Should(Receive(&messages),
				"processToolCalls must return within ToolCallTimeout+margin even when the underlying tool call never returns on its own — this is the regression guard for #1949 (before the fix, this Eventually times out because processToolCalls never returns)")

			Expect(messages).To(HaveLen(2), "assistant message + one tool-result message")
			toolResultMsg := messages[1]
			Expect(toolResultMsg.Role).To(Equal("tool"))
			Expect(toolResultMsg.Content).To(ContainSubstring("context deadline exceeded"),
				"the tool-call result must record a timeout error (AU-3: accurate audit content), not be left empty or silently swallowed")
		})
	})

	Describe("UT-KA-1949-002: a fast tool call under a generous timeout completes normally (no regression)", func() {
		It("does not misclassify a normal fast tool call as a timeout", func() {
			inv := New(Config{
				Logger:          logr.Discard(),
				AuditStore:      audit.NopAuditStore{},
				Registry:        instantToolRegistry{},
				ToolCallTimeout: 5 * time.Second,
			})

			resp := llm.ChatResponse{
				Message: llm.Message{Role: "assistant", Content: "investigating"},
				ToolCalls: []llm.ToolCall{
					{ID: "tc_1", Name: "kubectl_describe", Arguments: `{}`},
				},
			}

			messages, sentinel, budgetExhausted := inv.processToolCalls(context.Background(), nil, resp, 0, "rca", "corr-1949-002")

			Expect(sentinel).To(BeNil())
			Expect(budgetExhausted).To(BeFalse())
			Expect(messages).To(HaveLen(2))
			Expect(messages[1].Content).To(Equal(`{"ok":true}`))
		})
	})
})
