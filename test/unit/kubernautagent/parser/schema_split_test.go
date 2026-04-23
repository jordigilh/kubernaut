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

var _ = Describe("Split Workflow Submit Tool Schemas — #760 v2", func() {

	Describe("UT-KA-760-010: NoWorkflowResultSchema is valid JSON with expected properties", func() {
		It("should return valid JSON with root_cause_analysis and reasoning properties", func() {
			raw := parser.NoWorkflowResultSchema()
			Expect(raw).NotTo(BeEmpty(), "NoWorkflowResultSchema must not be empty")

			var schema map[string]interface{}
			err := json.Unmarshal(raw, &schema)
			Expect(err).NotTo(HaveOccurred(), "NoWorkflowResultSchema must be valid JSON")

			props, ok := schema["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "schema must have properties object")

			Expect(props).To(HaveKey("root_cause_analysis"),
				"NoWorkflowResultSchema must include root_cause_analysis")
			Expect(props).To(HaveKey("reasoning"),
				"NoWorkflowResultSchema must include reasoning")

			Expect(props).NotTo(HaveKey("selected_workflow"),
				"NoWorkflowResultSchema must NOT include selected_workflow")
			Expect(props).NotTo(HaveKey("alternative_workflows"),
				"NoWorkflowResultSchema must NOT include alternative_workflows")
		})
	})

	Describe("UT-KA-760-011: WithWorkflowResultSchema is valid JSON with workflow properties", func() {
		It("should return valid JSON with workflow_id, root_cause_analysis, and confidence properties", func() {
			raw := parser.WithWorkflowResultSchema()
			Expect(raw).NotTo(BeEmpty(), "WithWorkflowResultSchema must not be empty")

			var schema map[string]interface{}
			err := json.Unmarshal(raw, &schema)
			Expect(err).NotTo(HaveOccurred(), "WithWorkflowResultSchema must be valid JSON")

			props, ok := schema["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "schema must have properties object")

			Expect(props).To(HaveKey("root_cause_analysis"),
				"WithWorkflowResultSchema must include root_cause_analysis")
			Expect(props).To(HaveKey("selected_workflow"),
				"WithWorkflowResultSchema must include selected_workflow")
			Expect(props).To(HaveKey("confidence"),
				"WithWorkflowResultSchema must include confidence")
		})
	})
})
