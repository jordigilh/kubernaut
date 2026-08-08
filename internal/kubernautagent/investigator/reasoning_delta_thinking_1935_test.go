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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// White-box (package investigator) because reasoningDeltaText is an
// unexported helper. #1935 finding #3: the console's ThinkingPanel is fed
// exclusively by resp.Message.Content via the EventTypeReasoningDelta event
// (3 call sites in investigator.go). Confirmed against live production audit
// data for rr-618ac7d3b894-ba320bf0: every claude-sonnet-5 RCA-phase turn
// (5/5) had Content=="" while making 1-4 tool calls, because Sonnet 5 puts
// its narrative in the private, signed thinking block instead of visible
// Content. Without this fix the console ThinkingPanel goes silently blank
// for the entire diagnostic phase of a Sonnet-5 investigation, even after
// the #1935 finding #1 client-level capture fix makes Reasoning.Text
// available on llm.Message. Authority: BR-AI-086, FedRAMP CC7.2 (decision
// audit trails visible to the operator in real time).
var _ = Describe("UT-KA-1935-011: reasoningDeltaText combines Reasoning.Text with Content for the console ThinkingPanel", func() {
	It("returns Reasoning.Text when Content is empty (Sonnet-5 tool-calling turn)", func() {
		msg := llm.Message{
			Role:      "assistant",
			Content:   "",
			Reasoning: &llm.ReasoningBlock{Text: "Checking the Deployment spec for a bad command override..."},
		}
		Expect(reasoningDeltaText(msg)).To(Equal("Checking the Deployment spec for a bad command override..."),
			"UT-KA-1935-011: must fall back to Reasoning.Text so the console ThinkingPanel isn't blank "+
				"during Sonnet-5 tool-calling turns where Content is empty")
	})

	It("combines Reasoning.Text and Content when both are present", func() {
		msg := llm.Message{
			Role:      "assistant",
			Content:   "Calling kubectl_describe now.",
			Reasoning: &llm.ReasoningBlock{Text: "The pod is crash-looping; I should inspect the spec first."},
		}
		Expect(reasoningDeltaText(msg)).To(Equal(
			"The pod is crash-looping; I should inspect the spec first.\n\nCalling kubectl_describe now."),
			"UT-KA-1935-011: reasoning must lead, followed by the visible content, when both are present")
	})

	It("returns Content unchanged when Reasoning is nil (non-thinking models, e.g. claude-sonnet-4-6)", func() {
		msg := llm.Message{Role: "assistant", Content: "Investigating the crash loop."}
		Expect(reasoningDeltaText(msg)).To(Equal("Investigating the crash loop."),
			"UT-KA-1935-011: models without extended thinking must see identical behavior to today (no regression)")
	})

	It("returns Content unchanged when Reasoning.Text is empty (redacted thinking block)", func() {
		msg := llm.Message{
			Role:      "assistant",
			Content:   "Proceeding with the fix.",
			Reasoning: &llm.ReasoningBlock{Redacted: true},
		}
		Expect(reasoningDeltaText(msg)).To(Equal("Proceeding with the fix."),
			"UT-KA-1935-011: a redacted thinking block has no visible Text to surface; Content must still show")
	})

	It("returns empty string when both Content and Reasoning are empty", func() {
		msg := llm.Message{Role: "assistant"}
		Expect(reasoningDeltaText(msg)).To(Equal(""))
	})
})

// #2010 (BR-AI-086, FedRAMP CC7.2/SI-10): Claude's extended-thinking +
// tool-use behavior frequently ends the private "thinking" block with a
// short one-line plan and then repeats that identical line as the visible
// "text" block immediately before the tool_use block (the same underlying
// LLM behavior documented for #2006 on main, via DD-LLM-009). Before this
// fix, reasoningDeltaText's unconditional concatenation produced a single
// reasoning_delta event whose text was literally "X\n\nX", so the Console's
// ThinkingPanel showed the same sentence twice, separated by a blank line.
// Observed live in the PR #2000 dev environment (console-e2e-approval run).
var _ = Describe("UT-KA-2010-001: reasoningDeltaText does not repeat identical Reasoning.Text and Content", func() {
	It("returns a single copy when Reasoning.Text and Content are byte-for-byte identical", func() {
		msg := llm.Message{
			Role:      "assistant",
			Content:   "Let me now gather the pod logs and events for complete evidence.",
			Reasoning: &llm.ReasoningBlock{Text: "Let me now gather the pod logs and events for complete evidence."},
		}
		Expect(reasoningDeltaText(msg)).To(Equal("Let me now gather the pod logs and events for complete evidence."),
			"UT-KA-2010-001: identical Reasoning.Text and Content must not be concatenated into a "+
				"self-duplicated \"X\\n\\nX\" ThinkingPanel entry")
	})

	It("returns a single copy when Reasoning.Text and Content differ only by surrounding whitespace", func() {
		msg := llm.Message{
			Role:      "assistant",
			Content:   "  Checking the ConfigMap as well.\n",
			Reasoning: &llm.ReasoningBlock{Text: "Checking the ConfigMap as well."},
		}
		Expect(reasoningDeltaText(msg)).To(Equal("Checking the ConfigMap as well."),
			"UT-KA-2010-001: a whitespace-only difference is still the same sentence to the operator "+
				"and must not be shown twice")
	})

	It("still concatenates when Reasoning.Text and Content genuinely differ (no false-positive suppression)", func() {
		msg := llm.Message{
			Role:      "assistant",
			Content:   "Calling kubectl_describe now.",
			Reasoning: &llm.ReasoningBlock{Text: "The pod is crash-looping; I should inspect the spec first."},
		}
		Expect(reasoningDeltaText(msg)).To(Equal(
			"The pod is crash-looping; I should inspect the spec first.\n\nCalling kubectl_describe now."),
			"UT-KA-2010-001: distinct reasoning and content must both still reach the operator, "+
				"exactly as UT-KA-1935-011 already proves")
	})
})
