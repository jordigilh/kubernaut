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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

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
})
