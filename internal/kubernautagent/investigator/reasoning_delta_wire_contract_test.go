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
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// IT-KA-1771-001 (BR-AI-086 / AU-3 audit content, SI-10 input validation):
// proves the reasoning_delta event KA puts on the wire is actually consumable
// by AF's production display-formatting entry point, closing the loop across
// the KA/AF package boundary rather than asserting on either side in
// isolation. Regression target: #1771/#1634 (AF silently dropped every
// reasoning_delta event because KA emitted "content"/"content_preview" while
// AF's extractJSONField read "text").
var _ = Describe("IT-KA-1771-001: reasoning_delta wire contract between KA and AF", func() {
	It("round-trips a KA reasoning_delta event through AF's FormatEventForUser and produces the display text", func() {
		eventCh := make(chan session.InvestigationEvent, 64)
		ctx := session.WithEventSink(context.Background(), eventCh)

		mockClient := &cancelAwareMockClient{
			responses: []llm.ChatResponse{
				{
					Message: llm.Message{
						Role:    "assistant",
						Content: `{"rca_summary":"pod OOM killed","confidence":0.9}`,
					},
					Usage: llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
				},
			},
		}

		inv := streamTestInvestigator(mockClient)
		go func() {
			_, _ = inv.Investigate(ctx, katypes.SignalContext{
				Name: "pod-wire", Namespace: "default", Severity: "critical", Message: "OOMKilled",
			})
			close(eventCh)
		}()

		events := collectEvents(eventCh)

		var kaEvent *session.InvestigationEvent
		for i := range events {
			if events[i].Type == session.EventTypeReasoningDelta {
				kaEvent = &events[i]
				break
			}
		}
		Expect(kaEvent).NotTo(BeNil(), "KA must emit a reasoning_delta event for the LLM response")

		// Simulate the actual wire hop: KA's InvestigationEvent is marshaled to
		// JSON for the MCP LoggingMessage transport, then AF unmarshals the raw
		// bytes into its own wire-compatible ka.InvestigationEvent type.
		wireBytes, err := json.Marshal(kaEvent)
		Expect(err).NotTo(HaveOccurred())

		var afEvent afka.InvestigationEvent
		Expect(json.Unmarshal(wireBytes, &afEvent)).To(Succeed())

		displayText := aftools.FormatEventForUser(afEvent)
		Expect(displayText).To(Equal(`{"rca_summary":"pod OOM killed","confidence":0.9}`),
			"AF's production FormatEventForUser must extract the reasoning text KA put on the wire; "+
				"a field-name mismatch here means AF silently drops every reasoning_delta event (#1771)")
	})
})
