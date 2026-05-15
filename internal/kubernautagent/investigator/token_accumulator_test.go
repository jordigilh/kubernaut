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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("Token Accumulator — TP-433-PARITY (#433)", func() {

	Describe("UT-KA-433-TK-001: Accumulate token usage across multiple turns", func() {
		It("should sum prompt, completion, and total tokens across Add calls", func() {
			acc := &investigator.TokenAccumulator{}

			acc.Add(llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})
			acc.Add(llm.TokenUsage{PromptTokens: 200, CompletionTokens: 80, TotalTokens: 280})
			acc.Add(llm.TokenUsage{PromptTokens: 50, CompletionTokens: 30, TotalTokens: 80})

			Expect(acc.PromptTokens()).To(Equal(350), "prompt tokens should sum to 350")
			Expect(acc.CompletionTokens()).To(Equal(160), "completion tokens should sum to 160")
			Expect(acc.TotalTokens()).To(Equal(510), "total tokens should sum to 510")
		})
	})

	Describe("UT-KA-433-TK-002: AuditData produces correct map for audit events", func() {
		It("should return a map with the expected keys and accumulated values", func() {
			acc := &investigator.TokenAccumulator{}
			acc.Add(llm.TokenUsage{PromptTokens: 120, CompletionTokens: 60, TotalTokens: 180})

			data := acc.AuditData()
			Expect(data).To(HaveKeyWithValue("total_prompt_tokens", 120))
			Expect(data).To(HaveKeyWithValue("total_completion_tokens", 60))
			Expect(data).To(HaveKeyWithValue("total_tokens", 180))
		})
	})

	Describe("UT-KA-433-TK-003: Zero-value accumulator returns zeroes", func() {
		It("should return 0 for all token counts when nothing is added", func() {
			acc := &investigator.TokenAccumulator{}

			Expect(acc.PromptTokens()).To(Equal(0))
			Expect(acc.CompletionTokens()).To(Equal(0))
			Expect(acc.TotalTokens()).To(Equal(0))
		})
	})
})
