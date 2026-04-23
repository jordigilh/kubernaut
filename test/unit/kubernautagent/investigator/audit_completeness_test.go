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
	"log/slog"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// capturingAuditStore records all audit events for assertion without calling
// StoreAudit directly from test code (anti-pattern compliant per TESTING_GUIDELINES.md).
type capturingAuditStore struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (s *capturingAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *capturingAuditStore) eventsByType(eventType string) []*audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*audit.AuditEvent
	for _, e := range s.events {
		if e.EventType == eventType {
			out = append(out, e)
		}
	}
	return out
}

// scriptedLLMClient returns pre-configured ChatResponse values in order.
type scriptedLLMClient struct {
	mu        sync.Mutex
	responses []llm.ChatResponse
	callIdx   int
}

func newScriptedLLM(responses ...llm.ChatResponse) *scriptedLLMClient {
	return &scriptedLLMClient{responses: responses}
}

func (s *scriptedLLMClient) Close() error { return nil }

func (s *scriptedLLMClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.callIdx >= len(s.responses) {
		return llm.ChatResponse{
			Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"fallback"}`},
			Usage:   llm.TokenUsage{TotalTokens: 1},
		}, nil
	}
	resp := s.responses[s.callIdx]
	s.callIdx++
	return resp, nil
}

// longAnalysis produces a string longer than the 500-char preview truncation.
func longAnalysis(prefix string) string {
	return prefix + strings.Repeat(" detailed-analysis-padding", 30)
}

var _ = Describe("Phase 0: Audit Completeness — #592", func() {
	var (
		capStore *capturingAuditStore
		builder  *prompt.Builder
		signal   katypes.SignalContext
	)

	BeforeEach(func() {
		capStore = &capturingAuditStore{}
		var err error
		builder, err = prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		signal = katypes.SignalContext{
			Name:          "OOMKilled",
			Namespace:     "production",
			Severity:      "critical",
			Message:       "Container payment-svc exceeded memory limit",
			RemediationID: "rem-592-test",
			ResourceKind:  "Pod",
			ResourceName:  "payment-svc-abc123",
			IncidentID:    "inc-592-test",
		}
	})

	Describe("UT-CS-592-026: LLM request event contains structured messages array", func() {
		It("should store full messages in aiagent.llm.request event, not just prompt_preview", func() {
			rcaJSON := `{"rca_summary":"OOM due to memory leak","severity":"critical","needs_human_review":true,"reason":"needs operator confirmation"}`
			mockLLM := newScriptedLLM(llm.ChatResponse{
				Message: llm.Message{Role: "assistant", Content: rcaJSON},
				Usage:   llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
			})

			inv := investigator.New(investigator.Config{
				Client:       mockLLM,
				Builder:      builder,
				ResultParser: parser.NewResultParser(),
				AuditStore:   capStore,
				Logger:       slog.Default(),
				MaxTurns:     5,
				ModelName:    "test-model",
			})

			_, _ = inv.Investigate(context.Background(), signal)

			reqEvents := capStore.eventsByType(audit.EventTypeLLMRequest)
			Expect(reqEvents).NotTo(BeEmpty(), "should emit at least one aiagent.llm.request event")

			firstReq := reqEvents[0]
			Expect(firstReq.Data).To(HaveKey("messages"),
				"LLM request event must contain 'messages' with the full structured message array, not just 'prompt_preview'")

			msgs, ok := firstReq.Data["messages"].([]map[string]interface{})
			Expect(ok).To(BeTrue(), "messages should be a []map[string]interface{}")
			Expect(len(msgs)).To(BeNumerically(">=", 2), "should have at least system + user messages")

			hasSystem := false
			hasUser := false
			for _, m := range msgs {
				role, _ := m["role"].(string)
				content, _ := m["content"].(string)
				if role == "system" {
					hasSystem = true
					Expect(len(content)).To(BeNumerically(">", 500),
						"system prompt should be stored in full, not truncated")
				}
				if role == "user" {
					hasUser = true
				}
			}
			Expect(hasSystem).To(BeTrue(), "messages should include system role")
			Expect(hasUser).To(BeTrue(), "messages should include user role")
		})
	})

	Describe("UT-CS-592-027: LLM response event contains full analysis_content", func() {
		It("should store full analysis in aiagent.llm.response event, not just 500-char preview", func() {
			fullContent := longAnalysis(`{"rca_summary":"OOM due to memory leak in connection pool handler that exhausts available heap space under sustained load",`)
			fullContent += `"severity":"critical","needs_human_review":true,"reason":"needs confirmation"}`

			mockLLM := newScriptedLLM(llm.ChatResponse{
				Message: llm.Message{Role: "assistant", Content: fullContent},
				Usage:   llm.TokenUsage{PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300},
			})

			inv := investigator.New(investigator.Config{
				Client:       mockLLM,
				Builder:      builder,
				ResultParser: parser.NewResultParser(),
				AuditStore:   capStore,
				Logger:       slog.Default(),
				MaxTurns:     5,
				ModelName:    "test-model",
			})

			_, _ = inv.Investigate(context.Background(), signal)

			respEvents := capStore.eventsByType(audit.EventTypeLLMResponse)
			Expect(respEvents).NotTo(BeEmpty(), "should emit at least one aiagent.llm.response event")

			firstResp := respEvents[0]
			Expect(firstResp.Data).To(HaveKey("analysis_content"),
				"LLM response event must contain 'analysis_content' with the full response, not just 'analysis_preview'")

			analysisContent, ok := firstResp.Data["analysis_content"].(string)
			Expect(ok).To(BeTrue(), "analysis_content should be a string")
			Expect(len(analysisContent)).To(BeNumerically(">", 500),
				"analysis_content must store the FULL response, not truncated to 500 chars")
			Expect(analysisContent).To(Equal(fullContent),
				"analysis_content must be byte-identical to the LLM's raw response")
		})
	})

	Describe("UT-CS-592-028: response.complete event contains complete InvestigationResult", func() {
		It("should serialize is_actionable, signal_name, and full alternative workflows in response_data", func() {
			rcaJSON := `{
				"root_cause_analysis": {
					"summary": "OOM due to memory leak in connection pool",
					"severity": "critical",
					"signal_name": "OOMKilled",
					"contributing_factors": ["memory_leak", "connection_pool_exhaustion"],
					"remediation_target": {"kind": "Deployment", "name": "payment-svc", "namespace": "production"}
				}
			}`
			workflowJSON := `{
				"rca_summary": "Memory increase recommended",
				"workflow_id": "oomkill-increase-memory",
				"execution_bundle": "quay.io/kubernaut/oomkill:v1.0",
				"confidence": 0.92,
				"alternative_workflows": [
					{
						"workflow_id": "restart-deployment",
						"execution_bundle": "quay.io/kubernaut/restart:v1.0",
						"confidence": 0.75,
						"rationale": "Temporary fix via restart"
					}
				],
				"actionable": true
			}`

			mockLLM := newScriptedLLM(
				llm.ChatResponse{
					Message: llm.Message{Role: "assistant", Content: rcaJSON},
					Usage:   llm.TokenUsage{TotalTokens: 100},
				},
				llm.ChatResponse{
					Message: llm.Message{Role: "assistant", Content: workflowJSON},
					Usage:   llm.TokenUsage{TotalTokens: 80},
				},
			)

			inv := investigator.New(investigator.Config{
				Client:       mockLLM,
				Builder:      builder,
				ResultParser: parser.NewResultParser(),
				AuditStore:   capStore,
				Logger:       slog.Default(),
				MaxTurns:     5,
				ModelName:    "test-model",
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			completeEvents := capStore.eventsByType(audit.EventTypeResponseComplete)
			Expect(completeEvents).To(HaveLen(1), "should emit exactly one aiagent.response.complete event")

			responseDataStr, ok := completeEvents[0].Data["response_data"].(string)
			Expect(ok).To(BeTrue(), "response_data should be a JSON string")

			var responseData map[string]interface{}
			Expect(json.Unmarshal([]byte(responseDataStr), &responseData)).To(Succeed())

			Expect(responseData).To(HaveKey("is_actionable"),
				"response_data must include is_actionable (currently omitted by resultToAuditJSON)")

			Expect(responseData).To(HaveKey("signal_name"),
				"response_data must include signal_name (currently omitted)")

			alts, ok := responseData["alternative_workflows"].([]interface{})
			Expect(ok).To(BeTrue(), "alternative_workflows must be present")
			Expect(alts).To(HaveLen(1))

			alt0, ok := alts[0].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(alt0).To(HaveKey("execution_bundle"),
				"each alternative workflow must include execution_bundle (currently omitted)")
			Expect(alt0).To(HaveKey("confidence"),
				"each alternative workflow must include confidence (currently omitted)")
		})
	})

	Describe("UT-CS-592-029: response.complete serializes both HumanReviewReason and Reason", func() {
		It("should store human_review_reason enum value AND reason string separately", func() {
			rcaJSON := `{
				"rca_summary": "Investigation inconclusive due to insufficient data",
				"severity": "critical"
			}`
			p3JSON := `{
				"rca_summary": "Investigation inconclusive due to insufficient data",
				"severity": "critical",
				"investigation_outcome": "inconclusive",
				"reason": "Unable to determine root cause with available observability data"
			}`

			mockLLM := newScriptedLLM(
				llm.ChatResponse{
					Message: llm.Message{Role: "assistant", Content: rcaJSON},
					Usage:   llm.TokenUsage{TotalTokens: 80},
				},
				llm.ChatResponse{
					Message: llm.Message{Role: "assistant", Content: p3JSON},
					Usage:   llm.TokenUsage{TotalTokens: 120},
				},
			)

			inv := investigator.New(investigator.Config{
				Client:       mockLLM,
				Builder:      builder,
				ResultParser: parser.NewResultParser(),
				AuditStore:   capStore,
				Logger:       slog.Default(),
				MaxTurns:     5,
				ModelName:    "test-model",
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue())

			completeEvents := capStore.eventsByType(audit.EventTypeResponseComplete)
			Expect(completeEvents).To(HaveLen(1))

			responseDataStr, ok := completeEvents[0].Data["response_data"].(string)
			Expect(ok).To(BeTrue())

			var responseData map[string]interface{}
			Expect(json.Unmarshal([]byte(responseDataStr), &responseData)).To(Succeed())

			Expect(responseData).To(HaveKey("human_review_reason"),
				"response_data must serialize HumanReviewReason (the enum value)")
			Expect(responseData["human_review_reason"]).To(Equal("no_matching_workflows"),
				"human_review_reason must be parser-derived from investigation_outcome per #700: "+
					"inconclusive + RCA present + no workflow = no_matching_workflows")

			Expect(responseData).To(HaveKey("reason"),
				"response_data must separately serialize Reason (the free-text explanation)")
			Expect(responseData["reason"]).To(Equal("Unable to determine root cause with available observability data"))
		})
	})
})
