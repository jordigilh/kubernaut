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

package agent

import (
	"encoding/json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// Token in/out reporting (#2387 follow-up): harness-renamed token counts
// flow into present_decision's RCAData exactly like tool_calls_count/
// llm_turns — server-substituted, never LLM-supplied, never schema-required
// (#2073/#2074 lesson).
var _ = Describe("phase guard token reporting — #2387", func() {
	It("UT-AF-2586-004: grounded decision preserves absent and zero RCA confidence (BR-AI-012, AU-3)", func() {
		for _, tc := range []struct {
			wire    string
			present bool
		}{{`{"severity":"critical"}`, false}, {`{"severity":"critical","confidence":0}`, true}} {
			var rca tools.InvestigateRCA
			Expect(json.Unmarshal([]byte(tc.wire), &rca)).To(Succeed())
			got := canonicalGroundedRCA(&rca)
			if tc.present {
				Expect(got).To(HaveKeyWithValue("confidence", 0.0))
			} else {
				Expect(got).NotTo(HaveKey("confidence"))
			}
		}
		schema, err := jsonschema.For[tools.RCAData](nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(schema.Required).NotTo(ContainElement("confidence"))
	})

	Describe("UT-AF-2387-021: canonicalGroundedRCA renames token fields", func() {
		It("should map prompt/completion/total tokens into RCAData names", func() {
			rca := &tools.InvestigateRCA{
				Severity:         "critical",
				Confidence:       func() *float64 { v := 0.92; return &v }(),
				CausalChain:      []string{"Memory leak"},
				Target:           "Deployment/worker",
				TotalToolCalls:   10,
				TotalLLMTurns:    8,
				PromptTokens:     1200,
				CompletionTokens: 450,
				TotalTokens:      1650,
			}

			got := canonicalGroundedRCA(rca)
			Expect(got).NotTo(BeNil())
			Expect(got["prompt_tokens"]).To(Equal(1200))
			Expect(got["completion_tokens"]).To(Equal(450))
			Expect(got["total_tokens"]).To(Equal(1650))
		})
	})

	Describe("UT-AF-2387-023: RCAData carries optional token fields, never required", func() {
		It("should expose prompt/completion/total tokens as omitempty schema fields", func() {
			schema, err := jsonschema.For[tools.RCAData](nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(schema.Properties).To(HaveKey("prompt_tokens"))
			Expect(schema.Properties).To(HaveKey("completion_tokens"))
			Expect(schema.Properties).To(HaveKey("total_tokens"))
			Expect(schema.Required).NotTo(ContainElement("prompt_tokens"),
				"the LLM is never told to supply token counts — marking them required repeats #2073/#2074")
			Expect(schema.Required).NotTo(ContainElement("completion_tokens"))
			Expect(schema.Required).NotTo(ContainElement("total_tokens"))
		})
	})
})
