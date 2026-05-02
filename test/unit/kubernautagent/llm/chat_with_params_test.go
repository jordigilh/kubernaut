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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
)

var fastBackoff = &backoff.Config{
	BasePeriod:    1 * time.Millisecond,
	MaxPeriod:     5 * time.Millisecond,
	Multiplier:    2.0,
	JitterPercent: 0,
}

type capturingClient struct {
	mu          sync.Mutex
	capturedCtx context.Context
	capturedReq llm.ChatRequest
	resp        llm.ChatResponse
	err         error
}

func (c *capturingClient) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capturedCtx = ctx
	c.capturedReq = req
	return c.resp, c.err
}

func (c *capturingClient) Close() error { return nil }

// countingErrorClient returns errors for the first N calls, then succeeds.
// Thread-safe for use with retry logic.
type countingErrorClient struct {
	mu          sync.Mutex
	failCount   int
	totalCalls  int
	successResp llm.ChatResponse
}

func (c *countingErrorClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.totalCalls++
	if c.totalCalls <= c.failCount {
		return llm.ChatResponse{}, fmt.Errorf("transient error attempt %d", c.totalCalls)
	}
	return c.successResp, nil
}

func (c *countingErrorClient) Close() error { return nil }

func (c *countingErrorClient) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.totalCalls
}

var _ = Describe("ChatWithParams — BUG-1/BUG-3 fixes", func() {

	Describe("UT-KA-967-002: applies context timeout when TimeoutSeconds > 0", func() {
		It("should set a deadline on the context passed to Chat", func() {
			mock := &capturingClient{resp: llm.ChatResponse{
				Message: llm.Message{Role: "assistant", Content: "ok"},
			}}

			params := llm.RuntimeParams{
				Temperature:    0.7,
				TimeoutSeconds: 5,
			}
			req := llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "test"}},
			}

			_, err := llm.ChatWithParams(context.Background(), mock, req, params)
			Expect(err).NotTo(HaveOccurred())

			deadline, ok := mock.capturedCtx.Deadline()
			Expect(ok).To(BeTrue(), "context must have a deadline when TimeoutSeconds > 0")
			Expect(deadline).To(BeTemporally("~", time.Now().Add(5*time.Second), 2*time.Second))
		})
	})

	Describe("UT-KA-967-003: injects temperature via pointer", func() {
		It("should set Temperature on the ChatRequest passed to Chat", func() {
			mock := &capturingClient{resp: llm.ChatResponse{
				Message: llm.Message{Role: "assistant", Content: "ok"},
			}}

			params := llm.RuntimeParams{
				Temperature: 0.7,
			}
			req := llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "test"}},
				Options:  llm.ChatOptions{JSONMode: true},
			}

			_, err := llm.ChatWithParams(context.Background(), mock, req, params)
			Expect(err).NotTo(HaveOccurred())

			Expect(mock.capturedReq.Options.Temperature).NotTo(BeNil(),
				"Temperature must be set by ChatWithParams")
			Expect(*mock.capturedReq.Options.Temperature).To(BeNumerically("~", 0.7, 0.001))
			Expect(mock.capturedReq.Options.JSONMode).To(BeTrue(),
				"other options must be preserved")
		})
	})

	Describe("UT-KA-967-004: no timeout when TimeoutSeconds is 0", func() {
		It("should not wrap context with timeout", func() {
			mock := &capturingClient{resp: llm.ChatResponse{
				Message: llm.Message{Role: "assistant", Content: "ok"},
			}}

			params := llm.RuntimeParams{
				Temperature:    0.5,
				TimeoutSeconds: 0,
			}
			req := llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "test"}},
			}

			_, err := llm.ChatWithParams(context.Background(), mock, req, params)
			Expect(err).NotTo(HaveOccurred())

			_, ok := mock.capturedCtx.Deadline()
			Expect(ok).To(BeFalse(), "context must NOT have a deadline when TimeoutSeconds is 0")
		})
	})

	DescribeTable("ChatWithParams retry behavior",
		func(failCount, maxRetries, expectedCalls int, expectSuccess bool) {
			mock := &countingErrorClient{
				failCount:   failCount,
				successResp: llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "ok"}},
			}
			params := llm.RuntimeParams{
				Temperature:  0.7,
				MaxRetries:   maxRetries,
				RetryBackoff: fastBackoff,
			}
			req := llm.ChatRequest{Messages: []llm.Message{{Role: "user", Content: "test"}}}

			resp, err := llm.ChatWithParams(context.Background(), mock, req, params)
			if expectSuccess {
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Message.Content).To(Equal("ok"))
			} else {
				Expect(err).To(HaveOccurred())
			}
			Expect(mock.calls()).To(Equal(expectedCalls))
		},
		Entry("UT-KA-967-006: retries twice then succeeds",
			2, 2, 3, true),
		Entry("UT-KA-967-007: no retry when MaxRetries=0",
			5, 0, 1, false),
		Entry("UT-KA-967-008: succeeds on first attempt, no retry needed",
			0, 3, 1, true),
		Entry("UT-KA-967-010: all retries exhausted returns last error",
			5, 2, 3, false),
		Entry("UT-KA-967-011: negative MaxRetries treated as zero (1 attempt)",
			0, -1, 1, true),
	)

	Describe("UT-KA-967-009: respects parent context cancellation during retry", func() {
		It("should return context error when parent context is cancelled mid-retry", func() {
			mock := &countingErrorClient{
				failCount:   10,
				successResp: llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "ok"}},
			}
			// Use a backoff slow enough (50ms) that the 100ms context
			// timeout expires before all 5 retries can complete.
			slowBackoff := &backoff.Config{
				BasePeriod: 50 * time.Millisecond, MaxPeriod: 200 * time.Millisecond,
				Multiplier: 2.0, JitterPercent: 0,
			}
			params := llm.RuntimeParams{
				Temperature:  0.7,
				MaxRetries:   5,
				RetryBackoff: slowBackoff,
			}
			req := llm.ChatRequest{Messages: []llm.Message{{Role: "user", Content: "test"}}}

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			_, err := llm.ChatWithParams(ctx, mock, req, params)
			Expect(err).To(HaveOccurred())
			Expect(mock.calls()).To(BeNumerically("<", 6),
				"should not complete all retries when context is cancelled")
		})
	})
})
