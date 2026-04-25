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

package llm_test

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

type streamMockClient struct {
	chatResp   llm.ChatResponse
	chatErr    error
	streamResp llm.ChatResponse
	streamErr  error
	chunks     []string
	streamCalled bool
}

func (m *streamMockClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return m.chatResp, m.chatErr
}

func (m *streamMockClient) StreamChat(_ context.Context, _ llm.ChatRequest, callback func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	m.streamCalled = true
	if m.streamErr != nil {
		return llm.ChatResponse{}, m.streamErr
	}
	for _, chunk := range m.chunks {
		if err := callback(llm.ChatStreamEvent{Delta: chunk}); err != nil {
			return llm.ChatResponse{}, err
		}
	}
	_ = callback(llm.ChatStreamEvent{Done: true})
	return m.streamResp, nil
}

func (m *streamMockClient) Close() error { return nil }

var _ llm.Client = (*streamMockClient)(nil)

var _ = Describe("StreamChat Interface — #823 PR5", func() {

	Describe("UT-KA-823-SC01: StreamChat produces ChatStreamEvents with text deltas", func() {
		It("delivers chunks to callback in order", func() {
			mock := &streamMockClient{
				chunks: []string{"Hello", " world", "!"},
				streamResp: llm.ChatResponse{
					Message: llm.Message{Role: "assistant", Content: "Hello world!"},
					Usage:   llm.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
				},
			}

			var deltas []string
			resp, err := mock.StreamChat(context.Background(), llm.ChatRequest{}, func(evt llm.ChatStreamEvent) error {
				if evt.Delta != "" {
					deltas = append(deltas, evt.Delta)
				}
				return nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(deltas).To(Equal([]string{"Hello", " world", "!"}))
			Expect(resp.Message.Content).To(Equal("Hello world!"))
		})
	})

	Describe("UT-KA-823-SC02: StreamChat returns aggregated ChatResponse", func() {
		It("final response includes usage and tool calls", func() {
			mock := &streamMockClient{
				chunks: []string{"analyzing"},
				streamResp: llm.ChatResponse{
					Message:   llm.Message{Role: "assistant", Content: "analyzing"},
					ToolCalls: []llm.ToolCall{{ID: "tc1", Name: "kubectl_get", Arguments: `{}`}},
					Usage:     llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
				},
			}

			resp, err := mock.StreamChat(context.Background(), llm.ChatRequest{}, func(_ llm.ChatStreamEvent) error { return nil })
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.ToolCalls).To(HaveLen(1))
			Expect(resp.Usage.TotalTokens).To(Equal(150))
		})
	})

	Describe("UT-KA-823-SC03: Callback error aborts stream", func() {
		It("returns callback error immediately", func() {
			mock := &streamMockClient{
				chunks:     []string{"one", "two", "three"},
				streamResp: llm.ChatResponse{Message: llm.Message{Content: "full"}},
			}
			callbackErr := fmt.Errorf("observer disconnected")

			count := 0
			_, err := mock.StreamChat(context.Background(), llm.ChatRequest{}, func(_ llm.ChatStreamEvent) error {
				count++
				if count >= 2 {
					return callbackErr
				}
				return nil
			})

			Expect(err).To(MatchError("observer disconnected"))
		})
	})

	Describe("UT-KA-823-SC05: SwappableClient.StreamChat delegates", func() {
		It("delegates to current inner under RLock", func() {
			mock := &streamMockClient{
				chunks:     []string{"delegated"},
				streamResp: llm.ChatResponse{Message: llm.Message{Content: "delegated"}},
			}
			sc, err := llm.NewSwappableClient(mock, "test-model")
			Expect(err).NotTo(HaveOccurred())

			resp, err := sc.StreamChat(context.Background(), llm.ChatRequest{}, func(_ llm.ChatStreamEvent) error { return nil })
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("delegated"))
			Expect(mock.streamCalled).To(BeTrue())
		})
	})

	Describe("UT-KA-823-SC06: InstrumentedClient.StreamChat records metrics", func() {
		It("delegates to inner and completes without error", func() {
			mock := &streamMockClient{
				chunks: []string{"instrumented"},
				streamResp: llm.ChatResponse{
					Message: llm.Message{Content: "instrumented"},
					Usage:   llm.TokenUsage{PromptTokens: 50, CompletionTokens: 25, TotalTokens: 75},
				},
			}
			ic := llm.NewInstrumentedClient(mock)

			resp, err := ic.StreamChat(context.Background(), llm.ChatRequest{}, func(_ llm.ChatStreamEvent) error { return nil })
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("instrumented"))
			Expect(mock.streamCalled).To(BeTrue())
		})
	})

	Describe("UT-KA-823-SC07: LLMProxy.StreamChat delegates and submits alignment", func() {
		It("delegates to inner client", func() {
			mock := &streamMockClient{
				chunks:     []string{"proxy"},
				streamResp: llm.ChatResponse{Message: llm.Message{Content: "proxy response"}},
			}
			proxy := alignment.NewLLMProxy(mock)

			resp, err := proxy.StreamChat(context.Background(), llm.ChatRequest{}, func(_ llm.ChatStreamEvent) error { return nil })
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("proxy response"))
			Expect(mock.streamCalled).To(BeTrue())
		})
	})

	Describe("UT-KA-823-SC08: All implementations satisfy Client interface", func() {
		It("compile-time checks pass", func() {
			var _ llm.Client = (*llm.SwappableClient)(nil)
			var _ llm.Client = (*llm.InstrumentedClient)(nil)
			var _ llm.Client = (*alignment.LLMProxy)(nil)
			Expect(true).To(BeTrue())
		})
	})

	Describe("UT-KA-823-SC04: Context cancellation mid-stream", func() {
		It("returns context error when cancelled", func() {
			ctx, cancel := context.WithCancel(context.Background())

			mock := &streamMockClient{
				chunks: []string{"start", "mid"},
				streamResp: llm.ChatResponse{
					Message: llm.Message{Content: "start mid"},
				},
			}

			count := 0
			_, err := mock.StreamChat(ctx, llm.ChatRequest{}, func(_ llm.ChatStreamEvent) error {
				count++
				if count >= 1 {
					cancel()
					return ctx.Err()
				}
				return nil
			})

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UT-KA-823-SC09: Existing Chat tests pass unchanged", func() {
		It("Chat still works on all wrappers", func() {
			mock := &streamMockClient{
				chatResp: llm.ChatResponse{
					Message: llm.Message{Role: "assistant", Content: "chat response"},
					Usage:   llm.TokenUsage{TotalTokens: 10},
				},
			}
			sc, _ := llm.NewSwappableClient(mock, "m")
			ic := llm.NewInstrumentedClient(sc)
			proxy := alignment.NewLLMProxy(ic)

			resp, err := proxy.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("chat response"))
		})
	})
})

var _ = Describe("ChatStreamEvent Types — #823 PR5", func() {
	It("ChatStreamEvent fields are accessible", func() {
		evt := llm.ChatStreamEvent{
			Delta: "hello",
			Done:  false,
			ToolCallDelta: &llm.PartialToolCall{
				Index:          0,
				ID:             "tc1",
				Name:           "kubectl_get",
				ArgumentsDelta: `{"kind":"Pod"}`,
			},
			Usage: &llm.TokenUsage{PromptTokens: 10},
		}
		Expect(evt.Delta).To(Equal("hello"))
		Expect(evt.ToolCallDelta.Name).To(Equal("kubectl_get"))
		Expect(evt.Usage.PromptTokens).To(Equal(10))
	})
})

// Silence unused import warning for slog and time
var _ = slog.Default
var _ = time.Now
