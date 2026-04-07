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

package conversation_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/conversation"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

// capturingLLMClient captures Chat calls for verification.
type capturingLLMClient struct {
	calls     []llm.ChatRequest
	responses []llm.ChatResponse
	err       error
	callIdx   int
}

func (c *capturingLLMClient) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	c.calls = append(c.calls, req)
	if c.err != nil {
		return llm.ChatResponse{}, c.err
	}
	idx := c.callIdx
	c.callIdx++
	if idx < len(c.responses) {
		return c.responses[idx], nil
	}
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: "default response"},
	}, nil
}

// capturingAdapterAuditStore captures audit events for verification.
type capturingAdapterAuditStore struct {
	events []*audit.AuditEvent
}

func (s *capturingAdapterAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.events = append(s.events, event)
	return nil
}

var _ = Describe("ConversationLLM Interface — #592 Phase 5A", func() {

	Describe("UT-CS-592-G4-001: defaultLLM emits message event via emit callback", func() {
		It("should emit exactly one message event", func() {
			llmImpl := conversation.NewDefaultLLM()
			var events []conversation.ConversationEvent

			err := llmImpl.Respond(context.Background(), "session-1", "hello",
				func(ev conversation.ConversationEvent) {
					events = append(events, ev)
				})

			Expect(err).ToNot(HaveOccurred())
			Expect(events).To(HaveLen(1))
			Expect(events[0].Type).To(Equal("message"))

			var payload map[string]string
			Expect(json.Unmarshal(events[0].Data, &payload)).To(Succeed())
			Expect(payload).To(HaveKey("content"))
		})
	})

	Describe("UT-CS-592-G4-002: failingLLM returns error without emitting events", func() {
		It("should return error and emit no events", func() {
			llmImpl := conversation.NewFailingLLM()
			var events []conversation.ConversationEvent

			err := llmImpl.Respond(context.Background(), "session-1", "hello",
				func(ev conversation.ConversationEvent) {
					events = append(events, ev)
				})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("LLM service unavailable"))
			Expect(events).To(BeEmpty(), "failing LLM must not emit any events")
		})
	})
})

var _ = Describe("LLMAdapter — #592 Phase 5B-5D", func() {
	var (
		builder      *prompt.Builder
		sessions     *conversation.SessionManager
		mockClient   *capturingLLMClient
		auditSt      *capturingAdapterAuditStore
		reg          *registry.Registry
		adapter      *conversation.LLMAdapter
	)

	BeforeEach(func() {
		var err error
		builder, err = prompt.NewBuilder()
		Expect(err).ToNot(HaveOccurred())

		sessions = conversation.NewSessionManager(30*time.Minute, builder)
		mockClient = &capturingLLMClient{}
		auditSt = &capturingAdapterAuditStore{}
		reg = registry.New()
		adapter = conversation.NewLLMAdapter(conversation.LLMAdapterDeps{
			Client:       mockClient,
			Sessions:     sessions,
			ToolRegistry: reg,
			AuditStore:   auditSt,
			Logger:       slog.Default(),
			MaxToolTurns: 15,
		})
	})

	Describe("UT-CS-592-G4-004: Basic text response (no tools)", func() {
		It("should call Chat with system prompt + user message and emit message event", func() {
			s, err := sessions.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: "The pod ran out of memory."}},
			}

			var events []conversation.ConversationEvent
			err = adapter.Respond(context.Background(), s.ID, "What happened?",
				func(ev conversation.ConversationEvent) {
					events = append(events, ev)
				})

			Expect(err).ToNot(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(1))
			Expect(mockClient.calls[0].Messages[0].Role).To(Equal("system"))
			Expect(mockClient.calls[0].Messages[1].Role).To(Equal("user"))
			Expect(mockClient.calls[0].Messages[1].Content).To(Equal("What happened?"))

			Expect(len(events)).To(BeNumerically(">=", 1), "at least one SSE event expected")
			lastEvent := events[len(events)-1]
			Expect(lastEvent.Type).To(Equal("message"))
		})
	})

	Describe("UT-CS-592-G4-006: Cross-message history", func() {
		It("should include prior messages in second Chat call", func() {
			s, err := sessions.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: "Answer 1"}},
				{Message: llm.Message{Role: "assistant", Content: "Answer 2"}},
			}

			err = adapter.Respond(context.Background(), s.ID, "Question 1",
				func(ev conversation.ConversationEvent) {})
			Expect(err).ToNot(HaveOccurred())

			err = adapter.Respond(context.Background(), s.ID, "Question 2",
				func(ev conversation.ConversationEvent) {})
			Expect(err).ToNot(HaveOccurred())

			Expect(mockClient.calls).To(HaveLen(2))

			secondCall := mockClient.calls[1]
			Expect(secondCall.Messages[0].Role).To(Equal("system"))
			Expect(secondCall.Messages[1].Role).To(Equal("user"))
			Expect(secondCall.Messages[1].Content).To(Equal("Question 1"))
			Expect(secondCall.Messages[2].Role).To(Equal("assistant"))
			Expect(secondCall.Messages[2].Content).To(Equal("Answer 1"))
			Expect(secondCall.Messages[3].Role).To(Equal("user"))
			Expect(secondCall.Messages[3].Content).To(Equal("Question 2"))
		})
	})

	Describe("UT-CS-592-G4-007: Chat failure", func() {
		It("should return error when Chat fails", func() {
			s, err := sessions.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			mockClient.err = fmt.Errorf("network timeout")

			err = adapter.Respond(context.Background(), s.ID, "hello",
				func(ev conversation.ConversationEvent) {})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("network timeout"))
		})
	})

	Describe("UT-CS-592-G4-008: Tool call loop (Phase 5C)", func() {
		It("should execute tool call, append result, call Chat again", func() {
			reg.Register(&stubTool{name: "kubectl_get"})
			s, err := sessions.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			mockClient.responses = []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant"},
					ToolCalls: []llm.ToolCall{{ID: "tc-1", Name: "kubectl_get", Arguments: `{"namespace":"ns-1"}`}},
				},
				{Message: llm.Message{Role: "assistant", Content: "Pod is healthy."}},
			}

			var events []conversation.ConversationEvent
			err = adapter.Respond(context.Background(), s.ID, "Check pod",
				func(ev conversation.ConversationEvent) {
					events = append(events, ev)
				})

			Expect(err).ToNot(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2), "should call Chat twice: initial + after tool result")

			eventTypes := make([]string, len(events))
			for i, ev := range events {
				eventTypes[i] = ev.Type
			}
			Expect(eventTypes).To(ContainElement("tool_call"))
			Expect(eventTypes).To(ContainElement("tool_result"))
			Expect(eventTypes).To(ContainElement("message"))
		})
	})

	Describe("UT-CS-592-G4-009: Guardrails rejection", func() {
		It("should emit tool_error when guardrail rejects tool call", func() {
			s, err := sessions.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			mockClient.responses = []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant"},
					ToolCalls: []llm.ToolCall{{ID: "tc-1", Name: "kubectl_delete", Arguments: `{}`}},
				},
				{Message: llm.Message{Role: "assistant", Content: "Cannot delete."}},
			}

			var events []conversation.ConversationEvent
			err = adapter.Respond(context.Background(), s.ID, "Delete the pod",
				func(ev conversation.ConversationEvent) {
					events = append(events, ev)
				})

			Expect(err).ToNot(HaveOccurred())
			eventTypes := make([]string, len(events))
			for i, ev := range events {
				eventTypes[i] = ev.Type
			}
			Expect(eventTypes).To(ContainElement("tool_error"))
		})
	})

	Describe("UT-CS-592-G4-010: Max tool turns exceeded", func() {
		It("should emit error event when max tool turns reached", func() {
			reg.Register(&stubTool{name: "kubectl_get"})
			adapterLow := conversation.NewLLMAdapter(conversation.LLMAdapterDeps{
				Client:       mockClient,
				Sessions:     sessions,
				ToolRegistry: reg,
				AuditStore:   auditSt,
				Logger:       slog.Default(),
				MaxToolTurns: 2,
			})

			s, err := sessions.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant"}, ToolCalls: []llm.ToolCall{{ID: "tc-1", Name: "kubectl_get", Arguments: `{"namespace":"ns-1"}`}}},
				{Message: llm.Message{Role: "assistant"}, ToolCalls: []llm.ToolCall{{ID: "tc-2", Name: "kubectl_get", Arguments: `{"namespace":"ns-1"}`}}},
				{Message: llm.Message{Role: "assistant"}, ToolCalls: []llm.ToolCall{{ID: "tc-3", Name: "kubectl_get", Arguments: `{"namespace":"ns-1"}`}}},
			}

			var events []conversation.ConversationEvent
			err = adapterLow.Respond(context.Background(), s.ID, "loop forever",
				func(ev conversation.ConversationEvent) {
					events = append(events, ev)
				})

			Expect(err).To(MatchError(conversation.ErrMaxToolTurnsExceeded))
			eventTypes := make([]string, len(events))
			for i, ev := range events {
				eventTypes[i] = ev.Type
			}
			Expect(eventTypes).To(ContainElement("error"))
		})
	})

	Describe("UT-CS-592-G4-005: Chat messages include correct roles", func() {
		It("should send system + user roles on first call, system + user + assistant + user on second", func() {
			s, err := sessions.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: "A1"}},
				{Message: llm.Message{Role: "assistant", Content: "A2"}},
			}

			err = adapter.Respond(context.Background(), s.ID, "Q1",
				func(ev conversation.ConversationEvent) {})
			Expect(err).ToNot(HaveOccurred())

			Expect(mockClient.calls[0].Messages).To(HaveLen(2))
			Expect(mockClient.calls[0].Messages[0].Role).To(Equal("system"))
			Expect(mockClient.calls[0].Messages[1].Role).To(Equal("user"))

			err = adapter.Respond(context.Background(), s.ID, "Q2",
				func(ev conversation.ConversationEvent) {})
			Expect(err).ToNot(HaveOccurred())

			second := mockClient.calls[1].Messages
			Expect(second[0].Role).To(Equal("system"))
			Expect(second[1].Role).To(Equal("user"))
			Expect(second[1].Content).To(Equal("Q1"))
			Expect(second[2].Role).To(Equal("assistant"))
			Expect(second[2].Content).To(Equal("A1"))
			Expect(second[3].Role).To(Equal("user"))
			Expect(second[3].Content).To(Equal("Q2"))
		})
	})

	Describe("UT-CS-592-G4-011: todo_write routed to per-session tool", func() {
		It("should execute todo_write via session.TodoWrite(), not global registry", func() {
			s, err := sessions.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(s.TodoWrite().Name()).To(Equal("todo_write"),
				"session must expose a per-session todo_write tool")

			mockClient.responses = []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant"},
					ToolCalls: []llm.ToolCall{{ID: "tc-tw", Name: "todo_write", Arguments: `{"todos":[]}`}},
				},
				{Message: llm.Message{Role: "assistant", Content: "Plan created."}},
			}

			var events []conversation.ConversationEvent
			err = adapter.Respond(context.Background(), s.ID, "plan the fix",
				func(ev conversation.ConversationEvent) {
					events = append(events, ev)
				})
			Expect(err).ToNot(HaveOccurred())

			eventTypes := make([]string, len(events))
			for i, ev := range events {
				eventTypes[i] = ev.Type
			}
			Expect(eventTypes).To(ContainElement("tool_call"))
			Expect(eventTypes).To(ContainElement("tool_result"))
			Expect(eventTypes).To(ContainElement("message"))
		})
	})

	Describe("UT-CS-592-G4-012: Audit events emitted with investigator-aligned field names", func() {
		It("should emit audit events with correct data keys for LLM request, response, and tool calls", func() {
			reg.Register(&stubTool{name: "kubectl_get"})
			s, err := sessions.Create("rar-1", "ns-1", "user:alice", "corr-123")
			Expect(err).ToNot(HaveOccurred())

			mockClient.responses = []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant"},
					ToolCalls: []llm.ToolCall{{ID: "tc-1", Name: "kubectl_get", Arguments: `{"namespace":"ns-1"}`}},
				},
				{Message: llm.Message{Role: "assistant", Content: "Done."}},
			}

			err = adapter.Respond(context.Background(), s.ID, "check",
				func(ev conversation.ConversationEvent) {})
			Expect(err).ToNot(HaveOccurred())

			auditTypes := make([]string, len(auditSt.events))
			for i, ev := range auditSt.events {
				auditTypes[i] = ev.EventType
			}
			Expect(auditTypes).To(ContainElement(audit.EventTypeLLMRequest))
			Expect(auditTypes).To(ContainElement(audit.EventTypeLLMResponse))
			Expect(auditTypes).To(ContainElement(audit.EventTypeLLMToolCall))

			for _, ev := range auditSt.events {
				Expect(ev.CorrelationID).To(Equal("corr-123"),
					"all audit events must carry the session correlation ID")
			}

			// Verify investigator-aligned field names in LLM request audit
			for _, ev := range auditSt.events {
				if ev.EventType == audit.EventTypeLLMRequest {
					Expect(ev.Data).To(HaveKey("model"), "LLM request must include model")
					Expect(ev.Data).To(HaveKey("prompt_length"), "LLM request must include prompt_length")
					Expect(ev.Data).To(HaveKey("prompt_preview"), "LLM request must include prompt_preview")
					Expect(ev.Data).To(HaveKey("toolsets_enabled"), "LLM request must include toolsets_enabled")
					break
				}
			}

			// Verify investigator-aligned field names in LLM response audit
			for _, ev := range auditSt.events {
				if ev.EventType == audit.EventTypeLLMResponse {
					Expect(ev.Data).To(HaveKey("has_analysis"), "LLM response must include has_analysis")
					Expect(ev.Data).To(HaveKey("analysis_length"), "LLM response must include analysis_length")
					Expect(ev.Data).To(HaveKey("analysis_preview"), "LLM response must include analysis_preview")
					Expect(ev.Data).To(HaveKey("total_tokens"), "LLM response must include total_tokens")
					Expect(ev.Data).To(HaveKey("tool_call_count"), "LLM response must include tool_call_count")
					break
				}
			}

			// Verify investigator-aligned field names in tool call audit
			for _, ev := range auditSt.events {
				if ev.EventType == audit.EventTypeLLMToolCall {
					Expect(ev.Data).To(HaveKey("tool_call_index"), "tool call must include tool_call_index")
					Expect(ev.Data).To(HaveKey("tool_name"), "tool call must include tool_name")
					Expect(ev.Data).To(HaveKey("tool_arguments"), "tool call must include tool_arguments")
					Expect(ev.Data).To(HaveKey("tool_result"), "tool call must include tool_result")
					Expect(ev.Data).To(HaveKey("tool_result_preview"), "tool call must include tool_result_preview")
					break
				}
			}
		})
	})

	Describe("UT-CS-592-G4-013: Chat failure emits response failure audit (F13)", func() {
		It("should emit aiagent.llm.response with failure outcome when Chat returns error", func() {
			s, err := sessions.Create("rar-1", "ns-1", "user:alice", "corr-f13")
			Expect(err).ToNot(HaveOccurred())

			mockClient.err = fmt.Errorf("provider unavailable")

			err = adapter.Respond(context.Background(), s.ID, "hello",
				func(ev conversation.ConversationEvent) {})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("provider unavailable"))

			// Should have LLM request (pending) + LLM response (failure)
			var foundFailure bool
			for _, ev := range auditSt.events {
				if ev.EventType == audit.EventTypeLLMResponse && ev.EventOutcome == audit.OutcomeFailure {
					foundFailure = true
					Expect(ev.Data).To(HaveKey("error"))
					Expect(ev.Data).To(HaveKey("session_id"))
					Expect(ev.CorrelationID).To(Equal("corr-f13"))
					break
				}
			}
			Expect(foundFailure).To(BeTrue(), "must emit aiagent.llm.response with failure outcome on Chat error")
		})
	})
})
