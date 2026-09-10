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

// Token in/out reporting (#2387 follow-up): the KA complete event carries
// raw provider token counts for console display. They are bookkeeping like
// tool_calls_count/llm_turns — parsed from the wire, never requested from
// the LLM. BR-KA-OBSERVABILITY-001.
var _ = Describe("bridgeEventsCollectSummary token reporting — #2387", func() {

	Describe("UT-AF-2387-020: complete event token keys populate InvestigateRCA", func() {
		It("should extract prompt/completion/total tokens from EventTypeComplete Data", func() {
			events := make(chan ka.InvestigationEvent, 5)

			rcaPayload := map[string]interface{}{
				"severity":          "critical",
				"confidence":        0.92,
				"causal_chain":      []string{"Memory leak", "OOMKill"},
				"target":            "Deployment/worker in production",
				"rca_summary":       "OOMKill caused by memory leak",
				"total_llm_turns":   8,
				"total_tool_calls":  10,
				"prompt_tokens":     1200,
				"completion_tokens": 450,
				"total_tokens":      1650,
			}
			rcaJSON, err := json.Marshal(rcaPayload)
			Expect(err).NotTo(HaveOccurred())

			events <- ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
				Data: rcaJSON,
			}

			_, rca, _, _ := tools.BridgeEventsCollectSummary(context.Background(), events, 5*time.Second)

			Expect(rca).NotTo(BeNil())
			Expect(rca.PromptTokens).To(Equal(1200))
			Expect(rca.CompletionTokens).To(Equal(450))
			Expect(rca.TotalTokens).To(Equal(1650))
		})
	})
})
