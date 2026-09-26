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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Workflow selection discovery membership", func() {
	Describe("UT-KA-2442-003: validator enforces presented workflow membership (BR-KA-017-003, BR-KA-191, FedRAMP AC-6/SI-10, ASVS V2/V8)", func() {
		It("should reject a catalog workflow that was not returned by list_workflows", func() {
			state := katypes.NewDiscoveredWorkflowState()
			state.Add("workflow-presented")

			validator := parser.NewValidator([]string{"workflow-presented", "workflow-hidden"})
			validator.SetDiscoveredWorkflowState(state)

			Expect(validator.Validate(&katypes.InvestigationResult{
				WorkflowID: "workflow-presented",
				Confidence: 0.9,
			})).NotTo(HaveOccurred())
			Expect(validator.Validate(&katypes.InvestigationResult{
				WorkflowID: "workflow-hidden",
				Confidence: 0.9,
			})).To(MatchError(ContainSubstring("not returned by workflow discovery")))
		})
	})

	Describe("UT-KA-2442-004: validator observes rediscovery (BR-KA-017-004, BR-KA-191, FedRAMP SI-10, ASVS V2)", func() {
		It("should accept a workflow added after the validator was created", func() {
			state := katypes.NewDiscoveredWorkflowState()
			validator := parser.NewValidator([]string{"workflow-hidden"})
			validator.SetDiscoveredWorkflowState(state)

			result := &katypes.InvestigationResult{WorkflowID: "workflow-hidden", Confidence: 0.9}
			Expect(validator.Validate(result)).To(HaveOccurred())

			state.Add("workflow-hidden")
			Expect(validator.Validate(result)).NotTo(HaveOccurred())
		})
	})
})
