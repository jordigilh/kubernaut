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
	"errors"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// Issue #1585: ChatWithParams retries on any non-nil error for the full
// MaxRetries budget, including permanent failures (400/401/403/404) that
// will never succeed on retry. These tests RED-prove the provider-agnostic
// classification primitives (IsNonRetryableHTTPStatus, MarkNonRetryable,
// IsRetryable) that provider adapters (anthropicfamily, openai) use to
// classify their own errors at the translation boundary, per DD-HAPI-019
// Framework Isolation — this package must never import a provider SDK.
var _ = Describe("LLM error classification — #1585", func() {

	DescribeTable("UT-KA-1585-001: IsNonRetryableHTTPStatus blocklist",
		func(code int, wantNonRetryable bool) {
			Expect(llm.IsNonRetryableHTTPStatus(code)).To(Equal(wantNonRetryable))
		},
		Entry("400 Bad Request is non-retryable", http.StatusBadRequest, true),
		Entry("401 Unauthorized is non-retryable", http.StatusUnauthorized, true),
		Entry("403 Forbidden is non-retryable", http.StatusForbidden, true),
		Entry("404 Not Found is non-retryable", http.StatusNotFound, true),
		Entry("429 Too Many Requests is retryable", http.StatusTooManyRequests, false),
		Entry("500 Internal Server Error is retryable", http.StatusInternalServerError, false),
		Entry("502 Bad Gateway is retryable", http.StatusBadGateway, false),
		Entry("503 Service Unavailable is retryable", http.StatusServiceUnavailable, false),
	)

	Describe("UT-KA-1585-002: MarkNonRetryable / IsRetryable round-trip", func() {
		It("IsRetryable returns false for a MarkNonRetryable-wrapped error", func() {
			original := errors.New("401: invalid api key")
			wrapped := llm.MarkNonRetryable(original)

			Expect(llm.IsRetryable(wrapped)).To(BeFalse())
			Expect(wrapped.Error()).To(ContainSubstring("invalid api key"), "wrapping must preserve the original message")
			Expect(errors.Unwrap(wrapped)).To(Equal(original), "must unwrap to the original error")
		})

		It("composes with fmt.Errorf %w wrapping (adapter's own context wrap)", func() {
			original := errors.New("401: invalid api key")
			adapterWrapped := fmt.Errorf("anthropicfamily: %w", llm.MarkNonRetryable(original))

			Expect(llm.IsRetryable(adapterWrapped)).To(BeFalse(), "classification must survive additional %w wrapping")
			Expect(errors.Is(adapterWrapped, original)).To(BeTrue())
		})

		It("MarkNonRetryable(nil) returns nil", func() {
			Expect(llm.MarkNonRetryable(nil)).To(BeNil())
		})
	})

	Describe("UT-KA-1585-003: unknown/unwrapped error defaults to retryable (fail-safe, AC3)", func() {
		It("IsRetryable returns true for a plain error never classified by an adapter", func() {
			Expect(llm.IsRetryable(errors.New("some transient network blip"))).To(BeTrue())
		})

		It("IsRetryable returns false for nil error (nothing to retry)", func() {
			Expect(llm.IsRetryable(nil)).To(BeFalse())
		})
	})

	Describe("UT-KA-1585-004: ChatWithParams fails fast on a non-retryable error", func() {
		It("makes exactly one call, consuming none of the retry budget", func() {
			mock := &nonRetryableErrorClient{err: llm.MarkNonRetryable(errors.New("401: invalid api key"))}
			params := llm.RuntimeParams{
				Temperature:  0.7,
				MaxRetries:   5,
				RetryBackoff: fastBackoff,
			}
			req := llm.ChatRequest{Messages: []llm.Message{{Role: "user", Content: "test"}}}

			_, err := llm.ChatWithParams(context.Background(), mock, req, params)

			Expect(err).To(HaveOccurred())
			Expect(mock.calls).To(Equal(1), "must not retry a non-retryable error even with budget remaining")
		})
	})
})

// nonRetryableErrorClient always fails with the configured (pre-classified)
// error, counting calls to prove ChatWithParams does not retry it.
type nonRetryableErrorClient struct {
	calls int
	err   error
}

func (c *nonRetryableErrorClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	c.calls++
	return llm.ChatResponse{}, c.err
}

func (c *nonRetryableErrorClient) StreamChat(ctx context.Context, req llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return c.Chat(ctx, req)
}

func (c *nonRetryableErrorClient) Close() error { return nil }
