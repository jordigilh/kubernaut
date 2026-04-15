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

package parser_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
)

var _ = Describe("Phase Separation: Schema Contracts — #700", func() {

	Describe("UT-KA-700-001: RCAResultSchema contains only RCA fields", func() {
		It("should include root_cause_analysis, confidence, investigation_outcome, actionable, severity, detected_labels", func() {
			schema := parser.RCAResultSchema()
			Expect(schema).NotTo(BeEmpty(), "RCAResultSchema must not be empty")

			var parsed map[string]interface{}
			err := json.Unmarshal(schema, &parsed)
			Expect(err).NotTo(HaveOccurred(), "RCAResultSchema must be valid JSON")

			props, ok := parsed["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "schema must have properties object")

			By("containing RCA-specific fields")
			Expect(props).To(HaveKey("root_cause_analysis"))
			Expect(props).To(HaveKey("confidence"))
			Expect(props).To(HaveKey("investigation_outcome"))
			Expect(props).To(HaveKey("actionable"))
			Expect(props).To(HaveKey("severity"))
			Expect(props).To(HaveKey("detected_labels"))

			By("excluding workflow and escalation fields")
			Expect(props).NotTo(HaveKey("selected_workflow"),
				"RCA schema must NOT include selected_workflow (workflow selection is Phase 3)")
			Expect(props).NotTo(HaveKey("alternative_workflows"),
				"RCA schema must NOT include alternative_workflows")
			Expect(props).NotTo(HaveKey("needs_human_review"),
				"RCA schema must NOT include needs_human_review (parser-driven, not LLM-driven)")
			Expect(props).NotTo(HaveKey("human_review_reason"),
				"RCA schema must NOT include human_review_reason")
		})

		It("should require root_cause_analysis and confidence", func() {
			schema := parser.RCAResultSchema()
			var parsed map[string]interface{}
			err := json.Unmarshal(schema, &parsed)
			Expect(err).NotTo(HaveOccurred())

			required, ok := parsed["required"].([]interface{})
			Expect(ok).To(BeTrue(), "schema must have required array")
			requiredStrings := make([]string, len(required))
			for i, r := range required {
				requiredStrings[i] = r.(string)
			}
			Expect(requiredStrings).To(ContainElement("root_cause_analysis"))
			Expect(requiredStrings).To(ContainElement("confidence"))
		})
	})

	Describe("UT-KA-700-002: InvestigationResultSchema unchanged (regression guard)", func() {
		It("should still contain selected_workflow, alternative_workflows, needs_human_review", func() {
			schema := parser.InvestigationResultSchema()
			Expect(schema).NotTo(BeEmpty())

			var parsed map[string]interface{}
			err := json.Unmarshal(schema, &parsed)
			Expect(err).NotTo(HaveOccurred())

			props, ok := parsed["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			By("retaining all workflow selection fields")
			Expect(props).To(HaveKey("selected_workflow"))
			Expect(props).To(HaveKey("alternative_workflows"))
			Expect(props).To(HaveKey("needs_human_review"))
			Expect(props).To(HaveKey("human_review_reason"))

			By("retaining all RCA fields")
			Expect(props).To(HaveKey("root_cause_analysis"))
			Expect(props).To(HaveKey("confidence"))
		})
	})
})
