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
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

var _ = Describe("Kubernaut Agent Custom Tools Integration — #433", func() {

	var reg *registry.Registry

	BeforeEach(func() {
		Expect(ogenClient).NotTo(BeNil(), "ogen client must be initialized by SynchronizedBeforeSuite")

		reg = registry.New()

		allTools := custom.NewAllTools(ogenClient)
		Expect(allTools).To(HaveLen(3), "should create 3 custom tools")
		for _, t := range allTools {
			reg.Register(t)
		}
	})

	Describe("IT-KA-433-033: list_available_actions queries real DataStorage API", func() {
		It("should return action types from the real DataStorage", func() {
			result, err := reg.Execute(context.Background(), "list_available_actions",
				json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring("actionTypes"))
		})
	})

	Describe("IT-KA-433-034: list_workflows searches real DataStorage with criteria", func() {
		It("should return seeded workflows from real DataStorage", func() {
			result, err := reg.Execute(context.Background(), "list_workflows",
				json.RawMessage(`{"action_type":"IncreaseMemory"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			Expect(result).To(ContainSubstring("workflows"))
		})
	})

	Describe("IT-KA-433-035: get_workflow retrieves specific workflow from real DataStorage", func() {
		It("should return the seeded workflow definition by UUID", func() {
			Expect(workflowUUIDs).NotTo(BeEmpty(), "workflow UUIDs must be seeded")

			var wfUUID string
			for _, v := range workflowUUIDs {
				wfUUID = v
				break
			}
			Expect(wfUUID).NotTo(BeEmpty())

			result, err := reg.Execute(context.Background(), "get_workflow",
				json.RawMessage(fmt.Sprintf(`{"workflow_id":"%s"}`, wfUUID)))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("oom-recovery"))
		})
	})
})

var _ = Describe("Cursor-Based Pagination over Real DataStorage — #688", func() {

	var reg *registry.Registry

	BeforeEach(func() {
		Expect(ogenClient).NotTo(BeNil(), "ogen client must be initialized by SynchronizedBeforeSuite")

		reg = registry.New()
		for _, t := range custom.NewAllTools(ogenClient) {
			reg.Register(t)
		}
	})

	Describe("IT-KA-688-401: list_workflows cursor pagination through real DS HTTP wire", func() {
		// Two seeded IncreaseMemoryLimits workflows match the hardcoded filters
		// (oomkill-increase-memory-v1 and oom-recovery-aggressive-v1 both have
		// severity=critical, component=*, environment=production, priority=*).
		// A cursor with limit=1 forces pagination so each page returns one workflow.

		It("should paginate forward and backward using cursor tokens", func() {
			By("Page 1: first page with limit=1 cursor")
			cursor1 := custom.EncodeCursor(0, 1)
			args1 := json.RawMessage(fmt.Sprintf(
				`{"action_type":"IncreaseMemoryLimits","page":"next","cursor":"%s"}`, cursor1))

			result1, err := reg.Execute(context.Background(), "list_workflows", args1)
			Expect(err).NotTo(HaveOccurred())

			var page1 map[string]json.RawMessage
			Expect(json.Unmarshal([]byte(result1), &page1)).To(Succeed())

			var workflows1 []json.RawMessage
			Expect(json.Unmarshal(page1["workflows"], &workflows1)).To(Succeed())
			Expect(workflows1).To(HaveLen(1), "limit=1 should return exactly 1 workflow")

			Expect(page1).To(HaveKey("pagination"), "page 1 of 2 must have pagination")
			var pag1 map[string]interface{}
			Expect(json.Unmarshal(page1["pagination"], &pag1)).To(Succeed())
			Expect(pag1["hasNext"]).To(BeTrue(), "more workflows exist on next page")
			Expect(pag1).To(HaveKey("nextCursor"))
			Expect(pag1).NotTo(HaveKey("hasPrevious"), "first page has no previous")
			Expect(pag1).NotTo(HaveKey("totalCount"), "totalCount must never be exposed to LLM")

			By("Page 2: navigate forward using nextCursor")
			nextCursor := pag1["nextCursor"].(string)
			args2 := json.RawMessage(fmt.Sprintf(
				`{"action_type":"IncreaseMemoryLimits","page":"next","cursor":"%s"}`, nextCursor))

			result2, err := reg.Execute(context.Background(), "list_workflows", args2)
			Expect(err).NotTo(HaveOccurred())

			var page2 map[string]json.RawMessage
			Expect(json.Unmarshal([]byte(result2), &page2)).To(Succeed())

			var workflows2 []json.RawMessage
			Expect(json.Unmarshal(page2["workflows"], &workflows2)).To(Succeed())
			Expect(workflows2).To(HaveLen(1), "second page should return the remaining workflow")

			Expect(page2).To(HaveKey("pagination"), "offset>0 means pagination present")
			var pag2 map[string]interface{}
			Expect(json.Unmarshal(page2["pagination"], &pag2)).To(Succeed())
			Expect(pag2).NotTo(HaveKey("hasNext"), "last page has no next")
			Expect(pag2["hasPrevious"]).To(BeTrue(), "second page can go back")
			Expect(pag2).To(HaveKey("previousCursor"))
			Expect(pag2).NotTo(HaveKey("totalCount"))

			By("Page 3: navigate backward using previousCursor")
			prevCursor := pag2["previousCursor"].(string)
			args3 := json.RawMessage(fmt.Sprintf(
				`{"action_type":"IncreaseMemoryLimits","page":"previous","cursor":"%s"}`, prevCursor))

			result3, err := reg.Execute(context.Background(), "list_workflows", args3)
			Expect(err).NotTo(HaveOccurred())

			var page3 map[string]json.RawMessage
			Expect(json.Unmarshal([]byte(result3), &page3)).To(Succeed())

			var workflows3 []json.RawMessage
			Expect(json.Unmarshal(page3["workflows"], &workflows3)).To(Succeed())
			Expect(workflows3).To(HaveLen(1), "back to first page, 1 workflow")

			Expect(page3).To(HaveKey("pagination"))
			var pag3 map[string]interface{}
			Expect(json.Unmarshal(page3["pagination"], &pag3)).To(Succeed())
			Expect(pag3["hasNext"]).To(BeTrue(), "first page still has more")
			Expect(pag3).NotTo(HaveKey("hasPrevious"), "back at first page")
			Expect(pag3).NotTo(HaveKey("totalCount"))

			By("Verifying page 1 and page 3 return the same workflow (idempotent navigation)")
			Expect(string(workflows1[0])).To(Equal(string(workflows3[0])),
				"navigating back should return the same first-page workflow")
		})
	})

	Describe("IT-KA-688-402: list_workflows without cursor returns all matching workflows", func() {
		It("should return all matching workflows with pagination stripped (single page)", func() {
			By("Calling without page/cursor — DS uses default limit=10, all 2 workflows fit in one page")
			result, err := reg.Execute(context.Background(), "list_workflows",
				json.RawMessage(`{"action_type":"IncreaseMemoryLimits"}`))
			Expect(err).NotTo(HaveOccurred())

			var resp map[string]json.RawMessage
			Expect(json.Unmarshal([]byte(result), &resp)).To(Succeed())

			var workflows []json.RawMessage
			Expect(json.Unmarshal(resp["workflows"], &workflows)).To(Succeed())
			Expect(len(workflows)).To(BeNumerically(">=", 2),
				"default limit=10 should return all seeded matching workflows")

			Expect(resp).NotTo(HaveKey("pagination"),
				"single-page response should have pagination stripped by TransformPagination")
		})
	})
})
