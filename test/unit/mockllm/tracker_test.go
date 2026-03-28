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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/tracker"
)

var _ = Describe("Tool Call Tracker", func() {

	var t *tracker.Tracker

	BeforeEach(func() {
		t = tracker.New()
	})

	Describe("UT-MOCK-040-001: Tracker records tool calls with name, arguments, and timestamp", func() {
		It("should record and retrieve tool calls", func() {
			t.RecordToolCall("search_workflow_catalog", `{"query": "oomkill"}`)
			t.RecordToolCall("list_available_actions", `{"limit": 100}`)

			calls := t.GetToolCalls()
			Expect(calls).To(HaveLen(2))
			Expect(calls[0].Name).To(Equal("search_workflow_catalog"))
			Expect(calls[0].Arguments).To(Equal(`{"query": "oomkill"}`))
			Expect(calls[1].Name).To(Equal("list_available_actions"))
		})
	})

	Describe("UT-MOCK-040-002: Tracker records detected scenario", func() {
		It("should record and retrieve scenario name", func() {
			t.RecordScenario("oomkilled")
			Expect(t.GetLastScenario()).To(Equal("oomkilled"))

			t.RecordScenario("crashloop")
			Expect(t.GetLastScenario()).To(Equal("crashloop"))
		})
	})

	Describe("UT-MOCK-043-001: Reset clears all tracked state", func() {
		It("should clear tool calls and scenario after reset", func() {
			t.RecordToolCall("some_tool", `{}`)
			t.RecordScenario("test")

			t.Reset()

			Expect(t.GetToolCalls()).To(BeEmpty())
			Expect(t.GetLastScenario()).To(BeEmpty())
		})
	})
})
