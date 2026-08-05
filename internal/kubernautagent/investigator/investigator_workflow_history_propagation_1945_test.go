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

package investigator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// White-box (package investigator, not investigator_test) because
// loopResultMessages is intentionally unexported. #1935/PR #1939 fixed
// runRCA's consumption of this helper but only extended its type switch to
// cover *SubmitResult/*TextResult (the RCA-phase sentinel/text results) —
// *SubmitWithWorkflowResult and *SubmitNoWorkflowResult (the
// workflow-discovery phase's sentinel types) fall through to the `default:
// return nil` branch despite `sentinelResult` already populating their
// Messages field. This is the read-side half of #1945's fix; the write-side
// (runWorkflowSelection actually reassigning `messages` from this helper)
// is proven by IT-KA-1945-002/003 in workflow_history_propagation_1945_test.go.
var _ = Describe("#1945: loopResultMessages must cover workflow-discovery sentinel types", func() {

	accumulated := []llm.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "RCA findings: ...\n\nSelect the appropriate remediation workflow."},
		{Role: "assistant", Content: "Checking available actions...", ToolCalls: []llm.ToolCall{
			{ID: "tc_1", Name: "list_available_actions", Arguments: "{}"},
		}},
		{Role: "tool", Content: `{"actions":["restart-pod"]}`, ToolCallID: "tc_1", ToolName: "list_available_actions"},
	}

	Describe("UT-KA-1945-001a: SubmitWithWorkflowResult carries prior tool-call/tool-result turns", func() {
		It("returns the accumulated Messages instead of nil", func() {
			r := &SubmitWithWorkflowResult{
				Content:  `{"workflow_id":"restart-pod","confidence":0.9}`,
				Messages: accumulated,
			}

			got := loopResultMessages(r)

			Expect(got).NotTo(BeEmpty(),
				"UT-KA-1945-001a: loopResultMessages must return SubmitWithWorkflowResult.Messages, "+
					"not silently fall through to the default nil case (#1935 root cause #2, workflow-discovery extension)")
			Expect(got).To(Equal(accumulated))
		})
	})

	Describe("UT-KA-1945-001b: SubmitNoWorkflowResult carries prior tool-call/tool-result turns", func() {
		It("returns the accumulated Messages instead of nil", func() {
			r := &SubmitNoWorkflowResult{
				Content:  `{"reasoning":"no workflow matches"}`,
				Messages: accumulated,
			}

			got := loopResultMessages(r)

			Expect(got).NotTo(BeEmpty(),
				"UT-KA-1945-001b: loopResultMessages must return SubmitNoWorkflowResult.Messages, "+
					"not silently fall through to the default nil case (#1935 root cause #2, workflow-discovery extension)")
			Expect(got).To(Equal(accumulated))
		})
	})
})
