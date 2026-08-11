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

package investigator_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	afka "github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	aftools "github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// IT-AF-2086-030 (FedRAMP SI-4: status text must actually reach the operator):
// proves the real tool_call_start event KA puts on the wire (via the actual
// emitToSink call site in runLLMLoop, not a hand-built fixture) is consumable
// by AF's production FormatEventForUser, closing the loop across the KA/AF
// package boundary. Regression target: #2086 Fix 5 -- KA emits key
// "tool_name" (investigator.go) but AF's FormatEventForUser read "tool",
// so genuine "Calling kubectl_get..." status text never rendered in
// production; AF's own unit tests passed only because they hand-constructed
// fixtures with the (wrong) "tool" key, never exercising the real KA
// emission path.
var _ = Describe("IT-AF-2086-030: tool_call_start wire contract between KA and AF", func() {
	It("round-trips a real KA tool_call_start event through AF's FormatEventForUser and produces the display text", func() {
		eventCh := make(chan session.InvestigationEvent, 64)
		ctx := session.WithEventSink(context.Background(), eventCh)

		mockClient := &cancelAwareMockClient{
			responses: []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: "investigating..."},
					ToolCalls: []llm.ToolCall{
						{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"test","namespace":"default"}`},
					},
					Usage: llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
				},
				{
					Message: llm.Message{
						Role:    "assistant",
						Content: `{"rca_summary":"pod OOM killed","confidence":0.9}`,
					},
					Usage: llm.TokenUsage{PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300},
				},
			},
		}

		inv := streamTestInvestigator(mockClient)
		go func() {
			_, _ = inv.Investigate(ctx, streamSignal)
			close(eventCh)
		}()

		events := collectEvents(eventCh)

		var kaEvent *session.InvestigationEvent
		for i := range events {
			if events[i].Type == session.EventTypeToolCallStart {
				kaEvent = &events[i]
				break
			}
		}
		Expect(kaEvent).NotTo(BeNil(), "KA must emit a tool_call_start event for the LLM-requested tool call")

		// Simulate the actual wire hop: KA's InvestigationEvent is marshaled to
		// JSON for the MCP LoggingMessage transport, then AF unmarshals the raw
		// bytes into its own wire-compatible ka.InvestigationEvent type.
		wireBytes, err := json.Marshal(kaEvent)
		Expect(err).NotTo(HaveOccurred())

		var afEvent afka.InvestigationEvent
		Expect(json.Unmarshal(wireBytes, &afEvent)).To(Succeed())

		displayText := aftools.FormatEventForUser(afEvent)
		Expect(displayText).To(Equal("Calling kubectl_describe..."),
			"AF's production FormatEventForUser must extract the tool name KA put on the wire under "+
				"the \"tool_name\" key; a field-name mismatch here means real tool-call status text "+
				"never renders in production (#2086 Fix 5)")
	})
})
