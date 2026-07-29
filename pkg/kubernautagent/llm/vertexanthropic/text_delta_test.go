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

package vertexanthropic

import (
	"github.com/anthropics/anthropic-sdk-go"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// UT-VA-1775-001..003 (BR-AI-086 live reasoning narration): extractTextDelta
// must surface extended-thinking content (thinking_delta events), not just
// regular text_delta events. #1775: prior to this fix, event.Delta.Thinking
// was never read, so any thinking_delta stream event was silently dropped —
// zero visible output in the console's live panel the moment extended
// thinking is enabled on a request.
var _ = Describe("extractTextDelta — #1775", func() {
	It("UT-VA-1775-001: extracts thinking_delta content", func() {
		event := anthropic.MessageStreamEventUnion{
			Type: "content_block_delta",
			Delta: anthropic.MessageStreamEventUnionDelta{
				Type:     "thinking_delta",
				Thinking: "investigating pod crash...",
			},
		}
		delta, ok := extractTextDelta(event)
		Expect(ok).To(BeTrue(), "thinking_delta events must be extracted, not silently dropped")
		Expect(delta).To(Equal("investigating pod crash..."))
	})

	It("UT-VA-1775-002: text_delta still extracts as before (no regression)", func() {
		event := anthropic.MessageStreamEventUnion{
			Type: "content_block_delta",
			Delta: anthropic.MessageStreamEventUnionDelta{
				Type: "text_delta",
				Text: "hello",
			},
		}
		delta, ok := extractTextDelta(event)
		Expect(ok).To(BeTrue())
		Expect(delta).To(Equal("hello"))
	})

	It("UT-VA-1775-003: non-content_block_delta events return false", func() {
		event := anthropic.MessageStreamEventUnion{Type: "message_stop"}
		_, ok := extractTextDelta(event)
		Expect(ok).To(BeFalse())
	})

	It("UT-VA-1775-004: content_block_delta with neither text nor thinking returns false", func() {
		event := anthropic.MessageStreamEventUnion{
			Type: "content_block_delta",
			Delta: anthropic.MessageStreamEventUnionDelta{
				Type:        "input_json_delta",
				PartialJSON: `{"kind":`,
			},
		}
		_, ok := extractTextDelta(event)
		Expect(ok).To(BeFalse())
	})
})
