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

package investigation_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/investigation"
)

var _ = Describe("TodoWrite Tool — #433 Phase 1", func() {

	Describe("UT-KA-433-200: TodoWrite creates items from array", func() {
		It("should create todo items and return summary", func() {
			tool := investigation.NewTodoWriteTool()
			args := json.RawMessage(`{"todos":[{"id":"t1","content":"check logs","status":"pending"},{"id":"t2","content":"analyze metrics","status":"in_progress"}]}`)

			result, err := tool.Execute(context.Background(), args)
			Expect(err).NotTo(HaveOccurred())

			var summary map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &summary)).To(Succeed())
			Expect(summary).To(HaveKey("total"))
			Expect(summary["total"]).To(BeNumerically("==", 2))
		})
	})

	Describe("UT-KA-433-201: TodoWrite merges items by id", func() {
		It("should update existing items when called again with same id", func() {
			tool := investigation.NewTodoWriteTool()

			args1 := json.RawMessage(`{"todos":[{"id":"t1","content":"check logs","status":"pending"}]}`)
			_, err := tool.Execute(context.Background(), args1)
			Expect(err).NotTo(HaveOccurred())

			args2 := json.RawMessage(`{"todos":[{"id":"t1","content":"check logs","status":"completed"},{"id":"t2","content":"new task","status":"pending"}]}`)
			result, err := tool.Execute(context.Background(), args2)
			Expect(err).NotTo(HaveOccurred())

			var summary map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &summary)).To(Succeed())
			Expect(summary["total"]).To(BeNumerically("==", 2))
		})
	})

	Describe("UT-KA-433-202: TodoWrite returns JSON summary with count by status", func() {
		It("should include status counts in response", func() {
			tool := investigation.NewTodoWriteTool()
			args := json.RawMessage(`{"todos":[
				{"id":"t1","content":"done task","status":"completed"},
				{"id":"t2","content":"working","status":"in_progress"},
				{"id":"t3","content":"next","status":"pending"},
				{"id":"t4","content":"dropped","status":"cancelled"}
			]}`)

			result, err := tool.Execute(context.Background(), args)
			Expect(err).NotTo(HaveOccurred())

			var summary map[string]interface{}
			Expect(json.Unmarshal([]byte(result), &summary)).To(Succeed())
			Expect(summary["total"]).To(BeNumerically("==", 4))

			byStatus, ok := summary["by_status"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "by_status should be a map")
			Expect(byStatus["completed"]).To(BeNumerically("==", 1))
			Expect(byStatus["in_progress"]).To(BeNumerically("==", 1))
			Expect(byStatus["pending"]).To(BeNumerically("==", 1))
			Expect(byStatus["cancelled"]).To(BeNumerically("==", 1))
		})
	})

	Describe("UT-KA-433-203: TodoWrite has valid JSON parameter schema", func() {
		It("should return a non-empty valid JSON schema", func() {
			tool := investigation.NewTodoWriteTool()
			params := tool.Parameters()
			Expect(params).NotTo(BeNil())
			Expect(string(params)).NotTo(Equal("{}"))

			var schema map[string]interface{}
			Expect(json.Unmarshal(params, &schema)).To(Succeed())
			Expect(schema).To(HaveKey("type"))
			Expect(schema).To(HaveKey("properties"))
		})
	})

	Describe("UT-KA-433-204: TodoWrite accepts all status values", func() {
		It("should accept pending, in_progress, completed, cancelled", func() {
			tool := investigation.NewTodoWriteTool()
			for _, status := range []string{"pending", "in_progress", "completed", "cancelled"} {
				args := json.RawMessage(`{"todos":[{"id":"test","content":"task","status":"` + status + `"}]}`)
				_, err := tool.Execute(context.Background(), args)
				Expect(err).NotTo(HaveOccurred(), "should accept status: "+status)
			}
		})
	})

	Describe("TodoWrite satisfies Tool interface", func() {
		It("should have correct name and description", func() {
			tool := investigation.NewTodoWriteTool()
			Expect(tool.Name()).To(Equal("todo_write"))
			Expect(tool.Description()).NotTo(BeEmpty())
		})
	})
})
