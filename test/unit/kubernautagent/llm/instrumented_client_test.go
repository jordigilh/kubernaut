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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

type stubClient struct {
	resp llm.ChatResponse
	err  error
}

func (s *stubClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return s.resp, s.err
}

func (s *stubClient) StreamChat(ctx context.Context, req llm.ChatRequest, cb func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	resp, err := s.Chat(ctx, req)
	if err == nil {
		_ = cb(llm.ChatStreamEvent{Delta: resp.Message.Content, Done: true})
	}
	return resp, err
}

func (s *stubClient) Close() error { return nil }

var _ = Describe("InstrumentedClient — TP-433-PARITY (#433)", func() {

	Describe("UT-KA-433-LM-001: InstrumentedClient delegates to inner client", func() {
		It("should return the inner client's response and not alter it", func() {
			inner := &stubClient{
				resp: llm.ChatResponse{
					Message: llm.Message{Role: "assistant", Content: "hello"},
					Usage:   llm.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
				},
			}
			ic := llm.NewInstrumentedClient(inner)

			resp, err := ic.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("hello"))
			Expect(resp.Usage.PromptTokens).To(Equal(10))
			Expect(resp.Usage.CompletionTokens).To(Equal(5))
			Expect(resp.Usage.TotalTokens).To(Equal(15))
		})
	})

	Describe("UT-KA-433-LM-002: InstrumentedClient propagates errors", func() {
		It("should propagate errors from the inner client", func() {
			inner := &stubClient{
				err: fmt.Errorf("provider timeout"),
			}
			ic := llm.NewInstrumentedClient(inner)

			_, err := ic.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).To(MatchError(ContainSubstring("provider timeout")))
		})
	})

	Describe("UT-KA-433-LM-003: InstrumentedClient satisfies llm.Client interface (compile-time)", func() {
		It("should be assignable to llm.Client variable", func() {
			inner := &stubClient{}
			ic := llm.NewInstrumentedClient(inner)
			var client llm.Client = ic
			Expect(client).NotTo(BeNil(),
				"InstrumentedClient must satisfy llm.Client interface; behavioral delegation tested by UT-KA-433-LM-001")
		})
	})

	Describe("UT-KA-433-LM-004: Prometheus metrics are recorded on success", func() {
		It("should increment request counter and token counters", func() {
			inner := &stubClient{
				resp: llm.ChatResponse{
					Message: llm.Message{Role: "assistant", Content: "ok"},
					Usage:   llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
				},
			}
			ic := llm.NewInstrumentedClient(inner)

			beforeRequests := collectCounter("aiagent_api_llm_requests_total", "status", "success")
			beforePromptTokens := collectCounter("aiagent_api_llm_tokens_total", "type", "prompt")
			beforeCompletionTokens := collectCounter("aiagent_api_llm_tokens_total", "type", "completion")

			_, err := ic.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).NotTo(HaveOccurred())

			afterRequests := collectCounter("aiagent_api_llm_requests_total", "status", "success")
			afterPromptTokens := collectCounter("aiagent_api_llm_tokens_total", "type", "prompt")
			afterCompletionTokens := collectCounter("aiagent_api_llm_tokens_total", "type", "completion")

			Expect(afterRequests - beforeRequests).To(BeNumerically("==", 1))
			Expect(afterPromptTokens - beforePromptTokens).To(BeNumerically("==", 100))
			Expect(afterCompletionTokens - beforeCompletionTokens).To(BeNumerically("==", 50))
		})
	})

	Describe("UT-KA-433-LM-005: Prometheus error metric incremented on failure", func() {
		It("should increment error counter on inner client failure", func() {
			inner := &stubClient{err: fmt.Errorf("timeout")}
			ic := llm.NewInstrumentedClient(inner)

			beforeErrors := collectCounter("aiagent_api_llm_requests_total", "status", "error")

			_, _ = ic.Chat(context.Background(), llm.ChatRequest{})

			afterErrors := collectCounter("aiagent_api_llm_requests_total", "status", "error")
			Expect(afterErrors - beforeErrors).To(BeNumerically("==", 1))
		})
	})

	Describe("UT-KA-433-LM-006: Duration histogram records observations", func() {
		It("should record at least one observation in the duration histogram", func() {
			inner := &stubClient{
				resp: llm.ChatResponse{Message: llm.Message{Content: "ok"}},
			}
			ic := llm.NewInstrumentedClient(inner)

			beforeCount := collectHistogramCount("aiagent_api_llm_request_duration_seconds")

			_, err := ic.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).NotTo(HaveOccurred())

			afterCount := collectHistogramCount("aiagent_api_llm_request_duration_seconds")
			Expect(afterCount - beforeCount).To(BeNumerically("==", 1))
		})
	})
})

func collectCounter(metricName, labelName, labelValue string) float64 {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return 0
	}
	for _, f := range families {
		if f.GetName() != metricName {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == labelName && lp.GetValue() == labelValue {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

func collectHistogramCount(metricName string) uint64 {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return 0
	}
	for _, f := range families {
		if f.GetName() != metricName {
			continue
		}
		for _, m := range f.GetMetric() {
			return m.GetHistogram().GetSampleCount()
		}
	}
	return 0
}

