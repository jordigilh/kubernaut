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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("API-version correction discovery budget", func() {
	It("UT-KA-2609-025: rejects an oversized discovery batch before executing tools [BR-AI-212]", func() {
		calls := []llm.ToolCall{
			{ID: "one", Name: "api_resources_list", Arguments: `{}`},
			{ID: "two", Name: "resources_list", Arguments: `{}`},
			{ID: "three", Name: "resources_list", Arguments: `{}`},
		}
		event := audit.NewEvent(audit.EventTypeLLMToolCall, "corr-budget")

		messages, outcome, ok := (&Investigator{}).executeCorrectionDiscoveryCalls(
			context.Background(), LLMInvocationContext{}, calls, 0, event)

		Expect(ok).To(BeFalse())
		Expect(outcome).To(Equal(gateRetryExhausted))
		Expect(messages).To(BeNil())
		Expect(event.Data).To(HaveKeyWithValue("discovery_tool_error", "correction discovery response exceeded the tool-call budget"))
	})
})
