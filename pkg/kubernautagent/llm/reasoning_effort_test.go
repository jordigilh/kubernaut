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

package llm_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// BR-AI-087 / #1778: EffortToThinkingConfig/EffortToThinkingLevel were
// promoted here from anthropicfamily (where they were already covered
// end-to-end via anthropicfamily's own Chat-level Effort tests, #1604) so
// the new geminifamily client can reuse the identical Effort->ThinkingConfig
// mapping instead of re-deriving it (DD-LLM-010).
var _ = Describe("UT-KA-1778-001: EffortToThinkingLevel", func() {
	DescribeTable("maps the canonical Effort vocabulary onto genai.ThinkingLevel",
		func(effort string, wantLevel genai.ThinkingLevel, wantOK bool) {
			level, ok := llm.EffortToThinkingLevel(effort)
			Expect(ok).To(Equal(wantOK))
			if wantOK {
				Expect(level).To(Equal(wantLevel))
			}
		},
		Entry("none maps to Minimal", "none", genai.ThinkingLevelMinimal, true),
		Entry("minimal maps to Minimal", "minimal", genai.ThinkingLevelMinimal, true),
		Entry("low maps to Low", "low", genai.ThinkingLevelLow, true),
		Entry("medium maps to Medium", "medium", genai.ThinkingLevelMedium, true),
		Entry("high maps to High", "high", genai.ThinkingLevelHigh, true),
		Entry("xhigh clamps to High", "xhigh", genai.ThinkingLevelHigh, true),
		Entry("unset (empty string) is unhandled here", "", genai.ThinkingLevel(""), false),
	)
})

var _ = Describe("UT-KA-1778-002: EffortToThinkingConfig", func() {
	It("prefers an explicit BudgetTokens over Effort", func() {
		cfg := llm.EffortToThinkingConfig(&llm.ReasoningRequest{Enabled: true, Effort: "low", BudgetTokens: 2048})
		Expect(cfg.ThinkingBudget).NotTo(BeNil())
		Expect(*cfg.ThinkingBudget).To(BeNumerically("==", 2048))
	})

	It("maps a recognized Effort value to the matching ThinkingLevel when no BudgetTokens is set", func() {
		cfg := llm.EffortToThinkingConfig(&llm.ReasoningRequest{Enabled: true, Effort: "medium"})
		Expect(cfg.ThinkingBudget).To(BeNil())
		Expect(cfg.ThinkingLevel).To(Equal(genai.ThinkingLevelMedium))
	})

	It("defaults to ThinkingLevelHigh when neither BudgetTokens nor a recognized Effort is set (zero-regression default)", func() {
		cfg := llm.EffortToThinkingConfig(&llm.ReasoningRequest{Enabled: true})
		Expect(cfg.ThinkingBudget).To(BeNil())
		Expect(cfg.ThinkingLevel).To(Equal(genai.ThinkingLevelHigh))
	})
})
