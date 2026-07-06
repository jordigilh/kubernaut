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

package types_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// UT-KA-AUDIT-001: InvestigationResult.Reasoning carries an audit-safe
// summary of the LLM's reasoning/thinking content that led to the RCA
// (BR-AI-086 AC6, SOC2 CC7.2/CC8.1). Only visible Text + a Redacted flag are
// carried — the opaque replay Signature (llm.ReasoningBlock.Signature) never
// reaches this type, since it has no forensic value and would be a
// retention/liability cost with no compliance upside (AU-11).
var _ = Describe("UT-KA-AUDIT-001: InvestigationResult.Reasoning field", func() {
	It("should default to nil (omitted) when reasoning was not captured", func() {
		result := katypes.InvestigationResult{RCASummary: "no reasoning captured"}

		Expect(result.Reasoning).To(BeNil())

		data, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(ContainSubstring(`"reasoning"`),
			"reasoning key must be omitted entirely (omitempty) when nil, not serialized as null")
	})

	It("should carry visible reasoning text and round-trip through JSON", func() {
		result := katypes.InvestigationResult{
			RCASummary: "OOM due to memory leak",
			Reasoning: &katypes.ReasoningSummary{
				Text: "The pod's memory usage climbed steadily over 6 hours, consistent with a leak rather than a spike.",
			},
		}

		data, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())

		var restored katypes.InvestigationResult
		Expect(json.Unmarshal(data, &restored)).To(Succeed())
		Expect(restored.Reasoning).NotTo(BeNil())
		Expect(restored.Reasoning.Text).To(Equal(result.Reasoning.Text))
		Expect(restored.Reasoning.Redacted).To(BeFalse())
	})

	It("should carry Redacted=true with empty Text when the provider withheld reasoning content", func() {
		result := katypes.InvestigationResult{
			RCASummary: "OOM due to memory leak",
			Reasoning: &katypes.ReasoningSummary{
				Redacted: true,
			},
		}

		data, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())

		var restored katypes.InvestigationResult
		Expect(json.Unmarshal(data, &restored)).To(Succeed())
		Expect(restored.Reasoning).NotTo(BeNil())
		Expect(restored.Reasoning.Text).To(BeEmpty())
		Expect(restored.Reasoning.Redacted).To(BeTrue())
	})

	It("ReasoningSummary must not expose a Signature field (compliance decision: text+redacted only, no opaque replay bytes)", func() {
		// Compile-time-shaped check: ReasoningSummary intentionally has a
		// narrower surface than llm.ReasoningBlock. Marshal with all fields
		// set and confirm no signature-like key leaks through.
		summary := katypes.ReasoningSummary{Text: "some reasoning", Redacted: false}
		data, err := json.Marshal(summary)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(ContainSubstring("signature"))
	})
})
