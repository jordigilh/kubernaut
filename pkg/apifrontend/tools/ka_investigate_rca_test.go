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

package tools_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("bridgeEventsCollectSummary RCA Parsing — TP-1395-1396 (#1396)", func() {

	Describe("UT-AF-1396-020: bridgeEventsCollectSummary populates RCA from complete event Data", func() {
		It("should extract structured RCA from EventTypeComplete Data field", func() {
			events := make(chan ka.InvestigationEvent, 5)

			rcaPayload := map[string]interface{}{
				"severity":         "critical",
				"confidence":       0.92,
				"causal_chain":     []string{"Memory leak", "OOMKill"},
				"target":           "Deployment/worker in production",
				"rca_summary":      "OOMKill caused by memory leak",
				"total_llm_turns":  17,
				"total_tool_calls": 19,
			}
			rcaJSON, err := json.Marshal(rcaPayload)
			Expect(err).NotTo(HaveOccurred())

			events <- ka.InvestigationEvent{
				Type: ka.EventTypeTokenDelta,
				Data: json.RawMessage(`{"delta":"Investigating..."}`),
			}
			events <- ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
				Data: rcaJSON,
			}

			ctx := context.Background()
			summary, rca := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

			Expect(summary).To(ContainSubstring("Investigating..."))
			Expect(rca).NotTo(BeNil(), "RCA should be populated from complete event Data")
			Expect(rca.Severity).To(Equal("critical"))
			Expect(rca.Confidence).To(BeNumerically("~", 0.92, 0.001))
			Expect(rca.CausalChain).To(ConsistOf("Memory leak", "OOMKill"))
			Expect(rca.Target).To(Equal("Deployment/worker in production"))
			Expect(rca.RCASummary).To(Equal("OOMKill caused by memory leak"))
			Expect(rca.TotalLLMTurns).To(Equal(17))
			Expect(rca.TotalToolCalls).To(Equal(19))
		})
	})

	Describe("UT-AF-1396-021: bridgeEventsCollectSummary with empty Data on complete event", func() {
		It("should fall back to text summary with nil RCA", func() {
			events := make(chan ka.InvestigationEvent, 5)

			events <- ka.InvestigationEvent{
				Type: ka.EventTypeTokenDelta,
				Data: json.RawMessage(`{"delta":"Found the issue."}`),
			}
			events <- ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
			}

			ctx := context.Background()
			summary, rca := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

			Expect(summary).To(Equal("Found the issue."))
			Expect(rca).To(BeNil(), "RCA should be nil when complete event has no Data")
		})
	})
})
