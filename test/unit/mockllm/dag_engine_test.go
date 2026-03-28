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
package mockllm_test

import (
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

var _ = Describe("DAG Conversation Engine", func() {

	Describe("UT-MOCK-010-001: DAG transitions between nodes based on condition evaluation", func() {
		It("should reach the correct terminal node via condition-driven transitions", func() {
			// Build a simple A -> B -> C DAG where B is reached when flag is true
			dag := conversation.NewDAG("start")
			dag.AddNode("start", &stubHandler{name: "start"})
			dag.AddNode("middle", &stubHandler{name: "middle"})
			dag.AddNode("end", &stubHandler{name: "end"})

			dag.AddTransition("start", "middle", &alwaysTrueCondition{}, 0)
			dag.AddTransition("middle", "end", &alwaysTrueCondition{}, 0)

			ctx := conversation.NewContext(nil)
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.TerminalNode).To(Equal("end"))
		})
	})

	Describe("UT-MOCK-010-002: DAG records full traversal path", func() {
		It("should return ordered list of visited node names", func() {
			dag := conversation.NewDAG("a")
			dag.AddNode("a", &stubHandler{name: "a"})
			dag.AddNode("b", &stubHandler{name: "b"})
			dag.AddNode("c", &stubHandler{name: "c"})

			dag.AddTransition("a", "b", &alwaysTrueCondition{}, 0)
			dag.AddTransition("b", "c", &alwaysTrueCondition{}, 0)

			ctx := conversation.NewContext(nil)
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Path).To(Equal([]string{"a", "b", "c"}))
		})
	})

	Describe("UT-MOCK-010-003: DAG returns error when no transition matches", func() {
		It("should return error instead of silently deadlocking", func() {
			dag := conversation.NewDAG("start")
			dag.AddNode("start", &stubHandler{name: "start"})
			dag.AddNode("unreachable", &stubHandler{name: "unreachable"})

			// Transition with never-true condition
			dag.AddTransition("start", "unreachable", &alwaysFalseCondition{}, 0)

			ctx := conversation.NewContext(nil)
			_, err := dag.Execute(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("start"))
		})
	})

	Describe("UT-MOCK-010-004: Concurrent conversations do not leak state", func() {
		It("should maintain separate paths for two concurrent executions", func() {
			dag := conversation.NewDAG("start")
			dag.AddNode("start", &stubHandler{name: "start"})
			dag.AddNode("end", &stubHandler{name: "end"})
			dag.AddTransition("start", "end", &alwaysTrueCondition{}, 0)

			var wg sync.WaitGroup
			results := make([]*conversation.ExecutionResult, 2)
			errs := make([]error, 2)

			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					ctx := conversation.NewContext(nil)
					results[idx], errs[idx] = dag.Execute(ctx)
				}(i)
			}
			wg.Wait()

			for i := 0; i < 2; i++ {
				Expect(errs[i]).NotTo(HaveOccurred())
				Expect(results[i].Path).To(Equal([]string{"start", "end"}))
			}
			// Paths are independent objects
			Expect(&results[0].Path).NotTo(BeIdenticalTo(&results[1].Path))
		})
	})
})

// Test helpers

type stubHandler struct {
	name string
}

func (h *stubHandler) Handle(_ *conversation.Context) (*conversation.HandlerResult, error) {
	return &conversation.HandlerResult{NodeName: h.name}, nil
}

type alwaysTrueCondition struct{}

func (c *alwaysTrueCondition) Evaluate(_ *conversation.Context) bool { return true }

type alwaysFalseCondition struct{}

func (c *alwaysFalseCondition) Evaluate(_ *conversation.Context) bool { return false }
