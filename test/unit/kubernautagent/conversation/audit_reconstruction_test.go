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
	"fmt"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/conversation"
)

// mockAuditReader returns pre-configured audit event slices.
type mockAuditReader struct {
	events    []conversation.AuditEvent
	callCount atomic.Int32
	err       error
}

func (m *mockAuditReader) QueryAuditEvents(_ context.Context, _ string) ([]conversation.AuditEvent, error) {
	m.callCount.Add(1)
	if m.err != nil {
		return nil, m.err
	}
	return m.events, nil
}

func sampleAuditChain() []conversation.AuditEvent {
	return []conversation.AuditEvent{
		{
			EventType: "aiagent.llm.request",
			Data: map[string]interface{}{
				"model": "gpt-4",
				"messages": []map[string]interface{}{
					{"role": "system", "content": "You are Kubernaut Agent..."},
					{"role": "user", "content": "Investigate: critical OOMKilled in production"},
				},
			},
		},
		{
			EventType: "aiagent.llm.response",
			Data: map[string]interface{}{
				"analysis_content": "The pod is experiencing OOM due to memory leak in the connection pool...",
				"has_analysis":     true,
			},
		},
		{
			EventType: "aiagent.llm.tool_call",
			Data: map[string]interface{}{
				"tool_name":   "kubectl_describe",
				"tool_result": `{"status":"success","data":"Pod payment-svc-abc123 OOMKilled"}`,
			},
		},
		{
			EventType: "aiagent.llm.request",
			Data: map[string]interface{}{
				"model": "gpt-4",
				"messages": []map[string]interface{}{
					{"role": "system", "content": "You are Kubernaut Agent..."},
					{"role": "user", "content": "Investigate: critical OOMKilled in production"},
					{"role": "assistant", "content": ""},
					{"role": "tool", "content": `{"status":"success","data":"Pod payment-svc-abc123 OOMKilled"}`, "name": "kubectl_describe"},
				},
			},
		},
		{
			EventType: "aiagent.response.complete",
			Data: map[string]interface{}{
				"response_data": `{"rca_summary":"OOM due to memory leak","severity":"critical","workflow_id":"oomkill-increase-memory"}`,
			},
		},
	}
}

var _ = Describe("Audit Reconstruction — #592", func() {

	Describe("UT-CS-592-001: Audit chain -> LLM messages with correct roles", func() {
		It("should reconstruct 5 audit events into LLM messages preserving role structure", func() {
			reader := &mockAuditReader{events: sampleAuditChain()}
			fetcher := conversation.NewAuditChainFetcher(reader)

			messages, err := fetcher.FetchInvestigationHistory(context.Background(), "rem-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(len(messages)).To(BeNumerically(">=", 2),
				"must reconstruct at least system + user messages from 5-event audit chain")

			hasSystem := false
			hasUser := false
			for _, m := range messages {
				if m.Role == "system" {
					hasSystem = true
				}
				if m.Role == "user" {
					hasUser = true
				}
			}
			Expect(hasSystem).To(BeTrue(), "reconstructed messages must include system role")
			Expect(hasUser).To(BeTrue(), "reconstructed messages must include user role")
		})
	})

	Describe("UT-CS-592-002: Audit event type -> correct LLM message role", func() {
		It("should map each event type to the correct LLM role", func() {
			events := sampleAuditChain()
			messages := conversation.EventsToMessages(events)

			Expect(len(messages)).To(BeNumerically(">=", 3),
				"5-event chain with request/response/tool events must produce system + assistant + tool messages")

			roleMap := map[string]bool{}
			for _, m := range messages {
				roleMap[m.Role] = true
			}
			Expect(roleMap).To(HaveKey("system"),
				"aiagent.llm.request events should produce system messages")
			Expect(roleMap).To(HaveKey("assistant"),
				"aiagent.llm.response events should produce assistant messages")
			Expect(roleMap).To(HaveKey("tool"),
				"aiagent.llm.tool_call events should produce tool messages")
		})
	})

	Describe("UT-CS-592-003: Incomplete chain -> retries with backoff", func() {
		It("should retry fetching when audit chain is incomplete", func() {
			reader := &mockAuditReader{events: nil}
			fetcher := conversation.NewAuditChainFetcher(reader)

			_, _ = fetcher.FetchInvestigationHistory(context.Background(), "rem-002")

			Expect(reader.callCount.Load()).To(BeNumerically(">", 1),
				"should retry at least once when audit chain is empty on first attempt")
		})
	})

	Describe("UT-CS-592-004: Empty chain after retries -> error", func() {
		It("should return error when audit chain remains empty after all retries", func() {
			reader := &mockAuditReader{events: nil}
			fetcher := conversation.NewAuditChainFetcher(reader)

			_, err := fetcher.FetchInvestigationHistory(context.Background(), "rem-003")
			Expect(err).To(HaveOccurred(),
				"must return error when audit chain is empty after retries")
			Expect(err.Error()).To(ContainSubstring("empty"),
				"error should indicate the audit chain was empty")
		})
	})

	Describe("UT-CS-592-005: Large chain -> older turns summarized", func() {
		It("should summarize older messages when chain exceeds token budget", func() {
			longPadding := strings.Repeat("x", 3000)
			var largeChain []conversation.AuditEvent
			for i := 0; i < 50; i++ {
				largeChain = append(largeChain, conversation.AuditEvent{
					EventType: "aiagent.llm.request",
					Data: map[string]interface{}{
						"messages": []map[string]interface{}{
							{"role": "user", "content": fmt.Sprintf("Turn %d: %s", i, longPadding)},
						},
					},
				})
			}
			reader := &mockAuditReader{events: largeChain}
			fetcher := conversation.NewAuditChainFetcher(reader)

			messages, err := fetcher.FetchInvestigationHistory(context.Background(), "rem-004")
			Expect(err).NotTo(HaveOccurred())
			Expect(len(messages)).To(BeNumerically("<", 50),
				"should summarize older turns to stay within token budget")
		})
	})
})
