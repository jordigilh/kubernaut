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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("TP-1044: retryRCASubmit correction message", func() {

	Describe("UT-KA-1044-030: retryRCASubmit correction example includes api_version", func() {
		It("should include api_version in the example JSON sent to the LLM (BR-AI-1044 AC8 spec compliance)", func() {
			logger := logr.Discard()
			store := &gateRecordingAuditStore{}
			mockClient := &gateMockLLMClient{}
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()
			enricher := enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, store, logger)
			phaseTools := investigator.DefaultPhaseToolMap()

			// First response: garbage that triggers parse retry (retryRCASubmit).
			// Second response: valid RCA. Third: workflow.
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary": "this is an intentionally malformed response that will trigger a parse retry because it has no confidence field and the root_cause_analysis is a string not an object`}},
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"fixed issue","confidence":0.85,"remediation_target":{"kind":"Deployment","name":"api","namespace":"ns"}}`}},
				gateWfToolResp(`{"workflow_id":"fix","confidence":0.9}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: store, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "pod-abc", Namespace: "default", Severity: "warning", Message: "test",
			})
			Expect(err).NotTo(HaveOccurred())

			// retryRCASubmit appends a correction user message.
			// Find the call that has "could not be parsed" or the correction pattern.
			found := false
			for _, call := range mockClient.calls {
				for _, msg := range call.Messages {
					if msg.Role == "user" && containsCorrectionPattern(msg.Content) {
						Expect(msg.Content).To(ContainSubstring("api_version"),
							"UT-KA-1044-030: retryRCASubmit correction example must include api_version in the JSON example")
						found = true
					}
				}
			}
			Expect(found).To(BeTrue(),
				"UT-KA-1044-030: retryRCASubmit correction message should have been sent to LLM")
		})
	})
})

func containsCorrectionPattern(content string) bool {
	return len(content) > 0 &&
		(containsSubstr(content, "could not be parsed") ||
			containsSubstr(content, "MUST call submit_result"))
}

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
