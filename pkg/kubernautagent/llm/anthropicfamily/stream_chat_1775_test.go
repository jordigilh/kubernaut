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

package anthropicfamily_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/anthropicfamily"
)

// thinkingBlockSSE is a real Anthropic Messages API streaming response
// containing a thinking_delta content block, in the exact
// "event: <type>\ndata: <json>\n\n" wire format the SDK's SSE decoder
// expects (mirrors anthropic-sdk-go's own message_test.go "thinking block"
// fixture).
const thinkingBlockSSE = `event: message_start
data: {"type":"message_start","message":{"id":"msg_thinking","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":50,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"investigating pod crash..."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"The pod is OOMKilled."}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":10}}

event: message_stop
data: {"type":"message_stop"}

`

// IT-VA-1775-001 (BR-AI-086 live reasoning narration): proves the fix closes
// the loop end-to-end through the real Anthropic SSE wire format and
// StreamChat's actual callback dispatch, not just the extractTextDelta unit
// in isolation. Regression target: #1775 — thinking_delta content silently
// dropped from the token_delta stream.
var _ = Describe("StreamChat — #1775 thinking_delta wire contract", func() {
	It("IT-VA-1775-001: delivers thinking_delta content to the callback alongside text_delta", func() {
		tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"fake-token","token_type":"Bearer","expires_in":3600}`))
		}))
		defer tokenSrv.Close()
		fakeCreds := generateFakeServiceAccountJSONWithTokenURL(tokenSrv.URL)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(thinkingBlockSSE))
		}))
		defer server.Close()

		client, err := anthropicfamily.New(context.Background(),
			"claude-sonnet-4-6", fakeCreds, "my-project", "us-central1",
			anthropicfamily.WithSDKOptions(option.WithBaseURL(server.URL)),
		)
		Expect(err).NotTo(HaveOccurred())

		var deltas []string
		_, err = client.StreamChat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "Why is the pod crashing?"}},
		}, func(evt llm.ChatStreamEvent) error {
			if evt.Delta != "" {
				deltas = append(deltas, evt.Delta)
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		joined := strings.Join(deltas, "")
		Expect(joined).To(ContainSubstring("investigating pod crash..."),
			"thinking_delta content must reach the callback; a missing Delta.Thinking check "+
				"means extended-thinking output is silently dropped from the live console panel (#1775)")
		Expect(joined).To(ContainSubstring("The pod is OOMKilled."),
			"text_delta content must keep working alongside thinking_delta (no regression)")
	})
})
