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

package investigator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
)

var _ = Describe("Investigation Metrics — TP-1395-1396 (#1396)", func() {

	Describe("UT-KA-1396-001: InvestigationMetrics.IncLLMTurns increments on each call", func() {
		It("should increment LLM turns counter", func() {
			m := investigator.NewInvestigationMetrics()

			m.IncLLMTurns()
			m.IncLLMTurns()
			m.IncLLMTurns()

			Expect(m.LLMTurns()).To(Equal(3))
		})
	})

	Describe("UT-KA-1396-002: InvestigationMetrics.IncToolCalls increments on each dispatch", func() {
		It("should increment tool calls counter", func() {
			m := investigator.NewInvestigationMetrics()

			m.IncToolCalls()
			m.IncToolCalls()

			Expect(m.ToolCalls()).To(Equal(2))
		})
	})

	Describe("UT-KA-1396-003: Metrics not reset between RCA and workflow phases", func() {
		It("should accumulate across multiple phases without reset", func() {
			m := investigator.NewInvestigationMetrics()

			// Simulate RCA phase: 5 LLM turns, 8 tool calls
			for i := 0; i < 5; i++ {
				m.IncLLMTurns()
			}
			for i := 0; i < 8; i++ {
				m.IncToolCalls()
			}

			// Simulate workflow selection phase: 3 more LLM turns, 2 more tool calls
			for i := 0; i < 3; i++ {
				m.IncLLMTurns()
			}
			for i := 0; i < 2; i++ {
				m.IncToolCalls()
			}

			Expect(m.LLMTurns()).To(Equal(8), "LLM turns should accumulate across phases")
			Expect(m.ToolCalls()).To(Equal(10), "tool calls should accumulate across phases")
		})
	})

	Describe("UT-KA-1396-003b: Zero-value metrics returns zeroes", func() {
		It("should return 0 when nothing has been incremented", func() {
			m := investigator.NewInvestigationMetrics()

			Expect(m.LLMTurns()).To(Equal(0))
			Expect(m.ToolCalls()).To(Equal(0))
		})
	})
})
