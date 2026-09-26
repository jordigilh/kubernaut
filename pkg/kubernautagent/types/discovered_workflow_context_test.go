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

package types_test

import (
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Workflow discovery context state", func() {
	Describe("UT-KA-2442-001: discovery state tracks current-session results (BR-KA-017-003, FedRAMP AC-6, ASVS V8)", func() {
		It("should deduplicate workflow IDs and remain isolated between contexts", func() {
			first := katypes.NewDiscoveredWorkflowState()
			second := katypes.NewDiscoveredWorkflowState()
			firstCtx := katypes.WithDiscoveredWorkflowState(context.Background(), first)
			secondCtx := katypes.WithDiscoveredWorkflowState(context.Background(), second)

			first.Add("workflow-a", "workflow-a", "workflow-b")

			storedFirst, ok := katypes.DiscoveredWorkflowStateFromContext(firstCtx)
			Expect(ok).To(BeTrue())
			Expect(storedFirst.Contains("workflow-a")).To(BeTrue())
			Expect(storedFirst.Contains("workflow-b")).To(BeTrue())

			storedSecond, ok := katypes.DiscoveredWorkflowStateFromContext(secondCtx)
			Expect(ok).To(BeTrue())
			Expect(storedSecond.Contains("workflow-a")).To(BeFalse())
		})
	})

	Describe("UT-KA-2442-002: discovery state supports concurrent tool calls (BR-KA-017-002, FedRAMP SI-10, ASVS V2)", func() {
		It("should preserve every workflow ID added concurrently", func() {
			state := katypes.NewDiscoveredWorkflowState()
			var group sync.WaitGroup

			for i := 0; i < 20; i++ {
				workflowID := "workflow-" + string(rune('a'+i))
				group.Add(1)
				go func() {
					defer group.Done()
					state.Add(workflowID)
				}()
			}
			group.Wait()

			for i := 0; i < 20; i++ {
				Expect(state.Contains("workflow-" + string(rune('a'+i)))).To(BeTrue())
			}
		})
	})
})
