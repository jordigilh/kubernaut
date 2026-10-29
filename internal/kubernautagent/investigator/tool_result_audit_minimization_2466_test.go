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
	"encoding/json"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

type workflowSchemaToolRegistry2466 struct {
	result string
}

func (r workflowSchemaToolRegistry2466) Execute(_ context.Context, _ string, _ json.RawMessage) (string, error) {
	return r.result, nil
}
func (workflowSchemaToolRegistry2466) ToolsForPhase(_ katypes.Phase, _ katypes.PhaseToolMap) []tools.Tool {
	return nil
}
func (workflowSchemaToolRegistry2466) All() []tools.Tool { return nil }

type auditStoreCapture2466 struct {
	events []*audit.AuditEvent
}

func (s *auditStoreCapture2466) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.events = append(s.events, event)
	return nil
}

var _ = Describe("UT-KA-2466-003: get_workflow audit result minimization", func() {
	It("UT-KA-2466-003: preserves the full permitted result for the LLM but omits it from audit events", func() {
		const llmWorkflowResult = `{"description":{"what":"Increase memory","whenToUse":"OOMKilled"},"parameters":{"schema":{"parameters":[{"name":"MEMORY_LIMIT_NEW","description":"synthetic-sensitive-audit-sentinel-2459"}]}}}`
		store := &auditStoreCapture2466{}
		inv := New(Config{
			Logger:     logr.Discard(),
			AuditStore: store,
			Registry:   workflowSchemaToolRegistry2466{result: llmWorkflowResult},
		})
		response := llm.ChatResponse{
			Message: llm.Message{Role: "assistant", Content: "selecting workflow"},
			ToolCalls: []llm.ToolCall{{
				ID:        "tc-get-workflow-2466",
				Name:      "get_workflow",
				Arguments: `{"workflow_id":"b57a97b6-b4b0-4f60-a9a0-5e1ea5c9d15c"}`,
			}},
		}

		messages, sentinel, budgetExhausted := inv.processToolCalls(context.Background(), nil, response, 0, string(katypes.PhaseWorkflowDiscovery), "corr-2466-003")

		Expect(sentinel).To(BeNil())
		Expect(budgetExhausted).To(BeFalse())
		Expect(messages).To(HaveLen(2))
		Expect(messages[1].Role).To(Equal("tool"))
		Expect(messages[1].Content).To(Equal(llmWorkflowResult), "the LLM must retain the permitted description and operational schema")
		Expect(store.events).To(HaveLen(1))
		event := store.events[0]
		Expect(event.EventType).To(Equal(audit.EventTypeLLMToolCall))
		auditResult, ok := event.Data["tool_result"].(string)
		Expect(ok).To(BeTrue())
		Expect(auditResult).To(ContainSubstring("omitted"))
		Expect(auditResult).NotTo(ContainSubstring("parameters"))
		Expect(auditResult).NotTo(ContainSubstring("MEMORY_LIMIT_NEW"))
		Expect(auditResult).NotTo(ContainSubstring("synthetic-sensitive-audit-sentinel-2459"))
		Expect(event.Data["tool_result_preview"]).To(Equal(auditResult))

		inv.emitLLMRequestAudit(context.Background(), "corr-2466-003", "test-model", katypes.PhaseWorkflowDiscovery, messages, nil, nil)
		Expect(store.events).To(HaveLen(2))
		var requestEvent *audit.AuditEvent
		for _, storedEvent := range store.events {
			if storedEvent.EventType == audit.EventTypeLLMRequest {
				requestEvent = storedEvent
			}
		}
		Expect(requestEvent).NotTo(BeNil())
		auditMessages, ok := requestEvent.Data["messages"].([]map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(auditMessages[1]["content"]).To(Equal(omittedWorkflowSchemaAuditResult),
			"the subsequent LLM request audit must not copy the complete get_workflow result into conversation history")
		Expect(auditMessages[1]["content"]).NotTo(ContainSubstring("MEMORY_LIMIT_NEW"))
		Expect(auditMessages[1]["content"]).NotTo(ContainSubstring("synthetic-sensitive-audit-sentinel-2459"))
	})

	It("UT-KA-2466-005: leaves non-workflow tool results unchanged in audit events", func() {
		const toolResult = `{"items":[{"name":"api-server"}]}`
		store := &auditStoreCapture2466{}
		inv := New(Config{
			Logger:     logr.Discard(),
			AuditStore: store,
			Registry:   workflowSchemaToolRegistry2466{result: toolResult},
		})
		response := llm.ChatResponse{
			Message: llm.Message{Role: "assistant", Content: "checking resource"},
			ToolCalls: []llm.ToolCall{{
				ID:        "tc-get-pods-2466",
				Name:      "kubectl_get_by_name",
				Arguments: `{"kind":"Pod"}`,
			}},
		}

		messages, _, _ := inv.processToolCalls(context.Background(), nil, response, 0, "rca", "corr-2466-004")

		Expect(messages[1].Content).To(Equal(toolResult))
		Expect(store.events).To(HaveLen(1))
		Expect(store.events[0].Data["tool_result"]).To(Equal(toolResult))
		Expect(store.events[0].Data["tool_result_preview"]).To(Equal(toolResult))
	})

	It("UT-KA-2521-009: persists only bounded list summary audit metadata", func() {
		// BR-AUDIT-005; FedRAMP AU-2/AU-3/AU-9/AU-11 and SI-10;
		// SOC 2 CC7.2; OWASP ASVS V5.1/V5.5.2/V7.1.1/V7.2.1.
		const rawSummary = `{"apiVersion":"v1","kind":"ConfigMapList","items":[{"name":"cfg","data":{"payload":"raw-bulk-body-must-not-persist"}}],"item_count":100,"total_count":600,"truncated":true}`
		store := &auditStoreCapture2466{}
		inv := New(Config{Logger: logr.Discard(), AuditStore: store})
		inv.emitToolCallAudit(context.Background(), llm.ToolCall{
			Name:      "kubectl_get_by_kind_in_cluster",
			Arguments: `{"kind":"ConfigMap"}`,
		}, 3, rawSummary, string(katypes.PhaseRCA), "rr-2521-audit")

		Expect(store.events).To(HaveLen(1))
		event := store.events[0]
		result, ok := event.Data["tool_result"].(string)
		Expect(ok).To(BeTrue())
		Expect(result).NotTo(ContainSubstring("raw-bulk-body-must-not-persist"))
		Expect(result).To(ContainSubstring(`"item_count":100`))
		Expect(result).To(ContainSubstring(`"total_count":600`))
		Expect(result).To(ContainSubstring(`"truncated":true`))
		Expect(result).To(ContainSubstring(`"reference":"rr-2521-audit/tool-call/3"`))
		summary, ok := event.Data["tool_result_summary"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(summary).To(HaveKeyWithValue("item_count", BeEquivalentTo(100)))
		Expect(summary).To(HaveKeyWithValue("total_count", BeEquivalentTo(600)))
		Expect(summary).To(HaveKeyWithValue("truncated", true))
		Expect(summary["digest"]).To(MatchRegexp(`^hmac-sha256:[0-9a-f]{64}$`))
	})

	It("UT-KA-2521-010: jq legacy list results persist the bounded summary instead of the full result", func() {
		// BR-AUDIT-005; FedRAMP AU-2/AU-3/AU-9/AU-11 and SI-10;
		// SOC 2 CC7.2; OWASP ASVS V5.1/V5.5.2/V7.1.1/V7.2.1.
		const jqResult = `{"items":[{"metadata":{"name":"cfg","namespace":"prod","labels":{"evidence":"raw-jq-body-must-not-persist"}}}]}`
		store := &auditStoreCapture2466{}
		inv := New(Config{Logger: logr.Discard(), AuditStore: store})
		inv.emitToolCallAudit(context.Background(), llm.ToolCall{
			Name:      "kubernetes_jq_query",
			Arguments: `{"kind":"ConfigMap","jq_expr":"{items: .items}"}`,
		}, 4, jqResult, string(katypes.PhaseRCA), "rr-2521-jq-audit")

		Expect(store.events).To(HaveLen(1))
		event := store.events[0]
		result, ok := event.Data["tool_result"].(string)
		Expect(ok).To(BeTrue())
		Expect(result).NotTo(ContainSubstring("raw-jq-body-must-not-persist"))
		Expect(result).To(ContainSubstring(`"item_count":1`))
		Expect(result).To(ContainSubstring(`"total_count":1`))
		Expect(result).To(ContainSubstring(`"truncated":false`))
		Expect(result).To(ContainSubstring(`"reference":"rr-2521-jq-audit/tool-call/4"`))
		summary, ok := event.Data["tool_result_summary"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(summary).To(HaveKeyWithValue("item_count", BeEquivalentTo(1)))
		Expect(summary).To(HaveKeyWithValue("total_count", BeEquivalentTo(1)))
		Expect(summary).To(HaveKeyWithValue("truncated", false))
		Expect(summary["digest"]).To(MatchRegexp(`^hmac-sha256:[0-9a-f]{64}$`))
	})
})
