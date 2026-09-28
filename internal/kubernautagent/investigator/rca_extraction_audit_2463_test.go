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
	"encoding/json"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("BR-AUDIT-005 / issue 2463: conversation RCA extraction audit trace", func() {
	It("UT-KA-2463-001: preserves the extracted target and session ID in the fallback RCA audit trail", func() {
		store := &capturingAuditStore{}
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())

		client := newScriptedLLM(llm.ChatResponse{
			ToolCalls: []llm.ToolCall{{
				ID:        "submit-2463",
				Name:      investigator.SubmitResultToolName,
				Arguments: `{"rca_summary":"AuthorizationPolicy denies traffic","confidence":0.91,"remediation_target":{"kind":"AuthorizationPolicy","name":"istio-authz-fix-v1","namespace":"demo","api_version":"security.istio.io/v1"}}`,
			}},
			Usage: llm.TokenUsage{PromptTokens: 17, CompletionTokens: 23, TotalTokens: 40},
		})
		inv := investigator.New(investigator.Config{
			Client:       client,
			Builder:      builder,
			ResultParser: parser.NewResultParser(),
			AuditStore:   store,
			Logger:       logr.Discard(),
			MaxTurns:     5,
			ModelName:    "mock-llm",
		})

		ctx := session.WithSessionID(context.Background(), "interactive-session-2463")
		result, err := inv.RunRCAExtractionFromConversation(ctx, []llm.Message{
			{Role: "user", Content: "the pod cannot reach the service"},
			{Role: "assistant", Content: "the authorization policy is a likely cause"},
		}, "rr-2463")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RemediationTarget.APIVersion).To(Equal("security.istio.io/v1"))

		requestEvents := store.eventsByType(audit.EventTypeLLMRequest)
		Expect(requestEvents).To(HaveLen(1))
		Expect(requestEvents[0].CorrelationID).To(Equal("rr-2463"))
		Expect(requestEvents[0].SessionID).To(Equal("interactive-session-2463"))

		responseEvents := store.eventsByType(audit.EventTypeLLMResponse)
		Expect(responseEvents).To(HaveLen(1))
		Expect(responseEvents[0].SessionID).To(Equal("interactive-session-2463"))

		completeEvents := store.eventsByType(audit.EventTypeRCAComplete)
		Expect(completeEvents).To(HaveLen(1))
		Expect(completeEvents[0].CorrelationID).To(Equal("rr-2463"))
		Expect(completeEvents[0].SessionID).To(Equal("interactive-session-2463"))
		Expect(completeEvents[0].Data["total_tokens"]).To(Equal(40))

		responseData, ok := completeEvents[0].Data["response_data"].(string)
		Expect(ok).To(BeTrue())
		var auditResult map[string]interface{}
		Expect(json.Unmarshal([]byte(responseData), &auditResult)).To(Succeed())
		target, ok := auditResult["remediation_target"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(target["api_version"]).To(Equal("security.istio.io/v1"))
	})
})
