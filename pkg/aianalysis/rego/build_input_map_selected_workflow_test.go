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

package rego

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// UT-AA-2714-005: buildRegoInputMap wire-shape contract for selected_workflow
// (Issue #2714, BR-AI-085 FR-AI-085-007).
//
// Business outcome: when no workflow was selected, the `selected_workflow`
// key is ABSENT from the Rego input (not null). This distinction is a policy
// wire contract: policies written against pre-#2714 inputs, or using
// object.get / `!= null` discrimination, must observe the same input shape
// the nil-guarded builder has always produced for optional keys
// (identity, confidence_threshold). A regression that emitted
// `"selected_workflow": nil` would pass every behavioral fixture test while
// silently changing the wire shape.
var _ = Describe("buildRegoInputMap selected_workflow wire shape — #2714", func() {
	It("should omit the selected_workflow key entirely when SelectedWorkflow is nil", func() {
		input := &PolicyInput{
			ActionType: "RestartPod",
		}

		inputMap := buildRegoInputMap(input)

		Expect(inputMap).NotTo(HaveKey("selected_workflow"),
			"selected_workflow must be absent (not null) when no workflow was selected")
	})

	It("should emit id, name and version when SelectedWorkflow is present", func() {
		input := &PolicyInput{
			ActionType: "ScaleMemory",
			SelectedWorkflow: &SelectedWorkflowInput{
				ID:      "a93e5f6a-d967-510d-8faa-9a83c5420e78",
				Name:    "increase-memory-limits-gitops-v1",
				Version: "1.4.1",
			},
		}

		inputMap := buildRegoInputMap(input)

		Expect(inputMap).To(HaveKey("selected_workflow"))
		sw, ok := inputMap["selected_workflow"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "selected_workflow must be an object")
		Expect(sw).To(HaveKeyWithValue("id", "a93e5f6a-d967-510d-8faa-9a83c5420e78"))
		Expect(sw).To(HaveKeyWithValue("name", "increase-memory-limits-gitops-v1"))
		Expect(sw).To(HaveKeyWithValue("version", "1.4.1"))
	})
})
