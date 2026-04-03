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
