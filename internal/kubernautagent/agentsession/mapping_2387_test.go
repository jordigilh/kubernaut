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

package agentsession_test

import (
	"encoding/json"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/agentsession"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// Gap 1 (#2387 acceptance criterion #1): AgentSession.status.result must
// carry the server-computed call-level counts. RootCauseAnalysis is free-form
// JSON (PreserveUnknownFields), so this is a mapping-only change — no CRD
// alteration. BR-KA-OBSERVABILITY-001, FedRAMP AU-3.
var _ = Describe("AgentSession result call-level counts — #2387 Gap 1", func() {

	Describe("UT-KA-2387-011: mapping copies totals into rootCauseAnalysis when non-zero", func() {
		It("emits total_llm_turns/total_tool_calls alongside the RCA narrative", func() {
			result := &katypes.InvestigationResult{
				RCASummary:     "OOMKill caused by memory leak in worker",
				TotalLLMTurns:  8,
				TotalToolCalls: 10,
			}

			res := agentsession.MapInvestigationResultToAgentSessionResult(logr.Discard(), result, "incident-2387-011")
			Expect(res.RootCauseAnalysis).NotTo(BeNil())
			var rca map[string]interface{}
			Expect(json.Unmarshal(res.RootCauseAnalysis.Raw, &rca)).To(Succeed())
			Expect(rca).To(HaveKeyWithValue("total_llm_turns", BeNumerically("==", 8)))
			Expect(rca).To(HaveKeyWithValue("total_tool_calls", BeNumerically("==", 10)))
		})
	})

	Describe("UT-KA-2387-012: mapping omits totals when zero (nil-RCA contract preserved)", func() {
		It("leaves untracked investigations indistinguishable from before", func() {
			result := &katypes.InvestigationResult{RCASummary: "done"}

			res := agentsession.MapInvestigationResultToAgentSessionResult(logr.Discard(), result, "incident-2387-012")
			Expect(res.RootCauseAnalysis).NotTo(BeNil())
			var rca map[string]interface{}
			Expect(json.Unmarshal(res.RootCauseAnalysis.Raw, &rca)).To(Succeed())
			Expect(rca).NotTo(HaveKey("total_llm_turns"))
			Expect(rca).NotTo(HaveKey("total_tool_calls"))
		})
	})
})
