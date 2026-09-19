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
	"errors"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
)

type toolCallDeltaClient2439 struct {
	mu             sync.Mutex
	callCount      int
	partials       []*llm.PartialToolCall
	response       llm.ChatResponse
	firstCallError error
}

func (c *toolCallDeltaClient2439) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return c.response, nil
}

func (c *toolCallDeltaClient2439) StreamChat(_ context.Context, _ llm.ChatRequest, callback func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	c.mu.Lock()
	call := c.callCount
	c.callCount++
	c.mu.Unlock()

	for _, partial := range c.partials {
		if err := callback(llm.ChatStreamEvent{ToolCallDelta: partial}); err != nil {
			return llm.ChatResponse{}, err
		}
	}
	if call == 0 && c.firstCallError != nil {
		return llm.ChatResponse{}, c.firstCallError
	}
	if err := callback(llm.ChatStreamEvent{Done: true}); err != nil {
		return llm.ChatResponse{}, err
	}
	return c.response, nil
}

func (c *toolCallDeltaClient2439) Close() error { return nil }

func (c *toolCallDeltaClient2439) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.callCount
}

var _ = Describe("Gemini tool-call delta propagation — #2439", func() {
	var response llm.ChatResponse

	BeforeEach(func() {
		response = llm.ChatResponse{
			Message: llm.Message{
				Role:    "assistant",
				Content: `{"rca_summary":"pod OOM killed","confidence":0.9}`,
			},
		}
	})

	It("IT-KA-2439-002: forwards each partial tool-call fragment with turn and phase metadata", func() {
		client := &toolCallDeltaClient2439{
			partials: []*llm.PartialToolCall{
				{Index: 0, ID: "call-1", Name: "kubectl_get", ArgumentsDelta: `{"kind":"Pod",`},
				{Index: 0, ArgumentsDelta: `"name":"payments"}`},
			},
			response: response,
		}
		eventCh := make(chan session.InvestigationEvent, 128)
		ctx := session.WithEventSink(context.Background(), eventCh)

		inv := tokenStreamTestInvestigator(client)
		go func() {
			_, _ = inv.Investigate(ctx, streamSignal)
			close(eventCh)
		}()

		events := collectEvents(eventCh)
		var deltas []session.InvestigationEvent
		for _, event := range events {
			if event.Type == session.EventTypeToolCallDelta {
				deltas = append(deltas, event)
			}
		}

		Expect(deltas).To(HaveLen(4), "the production investigation runs the RCA and workflow-discovery phases")
		Expect(deltas[0].Turn).To(BeNumerically(">=", 0))
		Expect(deltas[0].Phase).To(Equal("rca"))
		Expect(deltas[2].Phase).To(Equal("workflow_discovery"))
		var first map[string]interface{}
		Expect(json.Unmarshal(deltas[0].Data, &first)).To(Succeed())
		Expect(first).To(HaveKeyWithValue("index", BeNumerically("==", 0)))
		Expect(first).To(HaveKeyWithValue("id", "call-1"))
		Expect(first).To(HaveKeyWithValue("name", "kubectl_get"))
		Expect(first).To(HaveKeyWithValue("arguments_delta", `{"kind":"Pod",`))
	})

	It("UT-KA-2439-002: does not execute or audit a tool from partial fragments alone", func() {
		store := &capturingAuditStore{}
		client := &toolCallDeltaClient2439{
			partials: []*llm.PartialToolCall{{Index: 0, Name: "kubectl_get", ArgumentsDelta: `{"kind":"Pod"`}},
			response: response,
		}
		eventCh := make(chan session.InvestigationEvent, 128)
		ctx := session.WithEventSink(context.Background(), eventCh)

		inv := streamingRetryTestInvestigator(client, llm.RuntimeParams{}, store)
		_, err := inv.Investigate(ctx, streamingRetrySignal)

		Expect(err).NotTo(HaveOccurred())
		Expect(store.eventsByType(audit.EventTypeLLMToolCall)).To(BeEmpty())
		close(eventCh)
		for event := range eventCh {
			Expect(event.Type).NotTo(Equal(session.EventTypeToolCall))
			Expect(event.Type).NotTo(Equal(session.EventTypeToolCallStart))
		}
	})

	It("UT-KA-2439-003: does not retry after a partial tool-call fragment was delivered", func() {
		client := &toolCallDeltaClient2439{
			partials:       []*llm.PartialToolCall{{Index: 0, ArgumentsDelta: `{"kind":"Pod"`}},
			response:       response,
			firstCallError: errors.New("transient stream failure"),
		}
		runtimeParams := llm.RuntimeParams{
			MaxRetries:   2,
			RetryBackoff: &backoff.Config{BasePeriod: 1, MaxPeriod: 1, Multiplier: 1},
		}
		eventCh := make(chan session.InvestigationEvent, 128)
		ctx := session.WithEventSink(context.Background(), eventCh)

		inv := streamingRetryTestInvestigator(client, runtimeParams, audit.NopAuditStore{})
		result, err := inv.Investigate(ctx, streamingRetrySignal)

		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(client.calls()).To(Equal(1))
	})
})
