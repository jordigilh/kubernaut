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

package custom_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom"
)

var _ = Describe("Kubernaut Agent Custom Tool Schemas — #433", func() {

	Describe("UT-KA-433-170: list_available_actions has valid JSON schema", func() {
		It("should return a non-nil parameter schema", func() {
			schema := custom.ListAvailableActionsSchema()
			Expect(schema).NotTo(BeNil())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())
			Expect(parsed["type"]).To(Equal("object"))
		})
	})

	Describe("UT-KA-433-171: list_workflows has valid JSON schema with required action_type", func() {
		It("should require action_type parameter", func() {
			schema := custom.ListWorkflowsSchema()
			Expect(schema).NotTo(BeNil())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())
			Expect(parsed["type"]).To(Equal("object"))

			required, ok := parsed["required"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(required).To(ContainElement("action_type"))
		})

		It("should include offset and limit optional parameters", func() {
			schema := custom.ListWorkflowsSchema()

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())

			props, ok := parsed["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(props).To(HaveKey("offset"))
			Expect(props).To(HaveKey("limit"))
		})
	})

	Describe("UT-KA-433-172: get_workflow has valid JSON schema with required workflow_id", func() {
		It("should require workflow_id parameter", func() {
			schema := custom.GetWorkflowSchema()
			Expect(schema).NotTo(BeNil())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(schema, &parsed)).To(Succeed())
			Expect(parsed["type"]).To(Equal("object"))

			required, ok := parsed["required"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(required).To(ContainElement("workflow_id"))
		})
	})

	Describe("UT-KA-433-173: All existing custom tools return non-nil Parameters()", func() {
		It("should have non-nil schemas for all 3 DataStorage tools", func() {
			Expect(custom.ListAvailableActionsSchema()).NotTo(BeNil())
			Expect(custom.ListWorkflowsSchema()).NotTo(BeNil())
			Expect(custom.GetWorkflowSchema()).NotTo(BeNil())
		})
	})
})
