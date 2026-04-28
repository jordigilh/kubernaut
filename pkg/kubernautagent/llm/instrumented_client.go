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

package llm

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	llmRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "aiagent",
		Subsystem: "api",
		Name:      "llm_requests_total",
		Help:      "Total number of LLM API calls.",
	}, []string{"status"})

	llmRequestDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "aiagent",
		Subsystem: "api",
		Name:      "llm_request_duration_seconds",
		Help:      "Duration of LLM API calls in seconds.",
		Buckets:   prometheus.DefBuckets,
	})

	llmTokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "aiagent",
		Subsystem: "api",
		Name:      "llm_tokens_total",
		Help:      "Total LLM tokens consumed.",
	}, []string{"type"})
)

// InstrumentedClient wraps an llm.Client and records Prometheus metrics
// per BR-HAPI-011/301: business logic wrapper (NOT HTTP middleware).
type InstrumentedClient struct {
	inner Client
}

// NewInstrumentedClient wraps the given Client with Prometheus instrumentation.
func NewInstrumentedClient(inner Client) *InstrumentedClient {
	return &InstrumentedClient{inner: inner}
}

// Chat delegates to the inner client and records metrics.
func (ic *InstrumentedClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	start := time.Now()

	resp, err := ic.inner.Chat(ctx, req)
	duration := time.Since(start).Seconds()

	llmRequestDuration.Observe(duration)

	if err != nil {
		llmRequestsTotal.WithLabelValues("error").Inc()
		return resp, err
	}

	llmRequestsTotal.WithLabelValues("success").Inc()
	llmTokensTotal.WithLabelValues("prompt").Add(float64(resp.Usage.PromptTokens))
	llmTokensTotal.WithLabelValues("completion").Add(float64(resp.Usage.CompletionTokens))

	return resp, nil
}

// StreamChat delegates to the inner client's StreamChat and records metrics.
func (ic *InstrumentedClient) StreamChat(ctx context.Context, req ChatRequest, callback func(ChatStreamEvent) error) (ChatResponse, error) {
	start := time.Now()
	resp, err := ic.inner.StreamChat(ctx, req, callback)
	duration := time.Since(start).Seconds()
	llmRequestDuration.Observe(duration)
	if err != nil {
		llmRequestsTotal.WithLabelValues("error").Inc()
		return resp, err
	}
	llmRequestsTotal.WithLabelValues("success").Inc()
	llmTokensTotal.WithLabelValues("prompt").Add(float64(resp.Usage.PromptTokens))
	llmTokensTotal.WithLabelValues("completion").Add(float64(resp.Usage.CompletionTokens))
	return resp, nil
}

// Close delegates to the inner client's Close method.
func (ic *InstrumentedClient) Close() error {
	return ic.inner.Close()
}
