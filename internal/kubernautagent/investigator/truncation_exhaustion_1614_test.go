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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// Issue #1614: a response truncated (FinishReasonLength) on the initial
// attempt AND on the escalated retry was silently returned as a normal
// TextResult, shipping partial/garbage content downstream as if it were a
// complete answer. BR-HAPI-197 requires this "AI cannot produce a reliable
// result" condition to set HumanReviewNeeded instead. Reuses
// scriptedMockClient/loopCharTestInvestigator/testSignal from
// investigator_loop_characterization_test.go and cancel_test.go.
var _ = Describe("UT-KA-1614-001: double-truncation (initial + escalated retry) exhaustion", func() {
	It("classifies as human-review-needed with a truncation reason instead of returning partial content (BR-HAPI-197)", func() {
		mockClient := &scriptedMockClient{
			steps: []scriptedStep{
				{resp: llm.ChatResponse{
					Message:      llm.Message{Role: "assistant", Content: "partial analysis, ran out of budget..."},
					FinishReason: llm.FinishReasonLength,
					Usage:        llm.TokenUsage{CompletionTokens: 4096},
				}},
				{resp: llm.ChatResponse{
					Message:      llm.Message{Role: "assistant", Content: "still partial even after escalation..."},
					FinishReason: llm.FinishReasonLength,
					Usage:        llm.TokenUsage{CompletionTokens: 8192},
				}},
			},
		}
		inv := loopCharTestInvestigator(mockClient, audit.NopAuditStore{}, 15, investigator.Pipeline{}, nil)

		result, err := inv.Investigate(context.Background(), testSignal)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.HumanReviewNeeded).To(BeTrue(),
			"a response that is still truncated after the escalated retry must never be accepted as a complete answer")
		Expect(result.Reason).To(ContainSubstring("output truncated after retry"))
		Expect(result.Reason).To(ContainSubstring("during RCA"))
		Expect(result.RCASummary).To(BeEmpty(),
			"no RCA summary should be extracted from truncated, unparsed content")
		Expect(mockClient.callIdx).To(Equal(2),
			"the truncation retry must still fire exactly once (truncationRetried guard) before exhausting, not loop indefinitely")
	})
})
