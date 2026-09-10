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

package session_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("RCA event payload tracked-zero — #2387", func() {

	Describe("UT-KA-2387-003: tracked-zero serializes so AF distinguishes it from untracked (BR-KA-OBSERVABILITY-001, FedRAMP AU-3)", func() {
		It("emits total_* keys even when both counts are zero", func() {
			result := &katypes.InvestigationResult{
				Severity:   "info",
				Confidence: 0.5,
				RCASummary: "genuinely zero tool activity",
			}

			data := session.MarshalRCASubset(result)
			Expect(data).NotTo(BeNil())
			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("total_llm_turns"),
				"UT-KA-2387-003: omitempty drops 0, so AF cannot distinguish untracked from tracked-zero — the key must always be present")
			Expect(parsed).To(HaveKey("total_tool_calls"))
		})
	})

	Describe("UT-KA-2387-009: token usage rides the same payload for console reporting", func() {
		It("emits prompt/completion/total token keys from TokenUsage", func() {
			result := &katypes.InvestigationResult{
				Severity:   "critical",
				Confidence: 0.98,
				RCASummary: "OOMKill caused by memory leak in worker",
				TokenUsage: &katypes.TokenUsageSummary{
					PromptTokens: 30, CompletionTokens: 15, TotalTokens: 45,
				},
			}

			data := session.MarshalRCASubset(result)
			Expect(data).NotTo(BeNil())
			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())
			Expect(parsed).To(HaveKeyWithValue("prompt_tokens", BeNumerically("==", 30)))
			Expect(parsed).To(HaveKeyWithValue("completion_tokens", BeNumerically("==", 15)))
			Expect(parsed).To(HaveKeyWithValue("total_tokens", BeNumerically("==", 45)))
		})

		It("omits token keys when TokenUsage was never recorded", func() {
			result := &katypes.InvestigationResult{Severity: "info", RCASummary: "no usage tracked"}

			data := session.MarshalRCASubset(result)
			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())
			Expect(parsed).NotTo(HaveKey("prompt_tokens"))
			Expect(parsed).NotTo(HaveKey("completion_tokens"))
			Expect(parsed).NotTo(HaveKey("total_tokens"))
		})
	})
})
