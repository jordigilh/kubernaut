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
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

type recordingClient struct {
	id        string
	chatCount atomic.Int64
	closed    atomic.Bool
	closeDelay time.Duration
}

func (r *recordingClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	r.chatCount.Add(1)
	return llm.ChatResponse{Message: llm.Message{Content: r.id}}, nil
}

func (r *recordingClient) Close() error {
	if r.closeDelay > 0 {
		time.Sleep(r.closeDelay)
	}
	r.closed.Store(true)
	return nil
}

var _ = Describe("SwappableClient — TP-783-SC (#783)", func() {

	Describe("UT-KA-783-SC-001: Chat delegates to inner client", func() {
		It("should return the inner client's response", func() {
			inner := &recordingClient{id: "original"}
			sc, err := llm.NewSwappableClient(inner, "model-v1")
			Expect(err).NotTo(HaveOccurred())

			resp, err := sc.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("original"))
			Expect(inner.chatCount.Load()).To(Equal(int64(1)))
		})
	})

	Describe("UT-KA-783-SC-002: Swap atomically replaces inner client", func() {
		It("should use the new client after swap", func() {
			original := &recordingClient{id: "original"}
			replacement := &recordingClient{id: "replacement"}

			sc, err := llm.NewSwappableClient(original, "model-v1")
			Expect(err).NotTo(HaveOccurred())

			Expect(sc.Swap(replacement, "model-v2")).To(Succeed())

			resp, err := sc.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("replacement"))
		})
	})

	Describe("UT-KA-783-SC-003: Swap calls Close on old client", func() {
		It("should close the old client after replacement", func() {
			original := &recordingClient{id: "original"}
			replacement := &recordingClient{id: "replacement"}

			sc, err := llm.NewSwappableClient(original, "model-v1")
			Expect(err).NotTo(HaveOccurred())

			Expect(sc.Swap(replacement, "model-v2")).To(Succeed())

			Eventually(func() bool {
				return original.closed.Load()
			}, 2*time.Second, 10*time.Millisecond).Should(BeTrue(),
				"Swap must close the old client")
		})
	})

	Describe("UT-KA-783-SC-004: Swap does not block on slow Close", func() {
		It("should return from Swap immediately even if old Close is slow", func() {
			original := &recordingClient{id: "original", closeDelay: 5 * time.Second}
			replacement := &recordingClient{id: "replacement"}

			sc, err := llm.NewSwappableClient(original, "model-v1")
			Expect(err).NotTo(HaveOccurred())

			start := time.Now()
			Expect(sc.Swap(replacement, "model-v2")).To(Succeed())
			swapDuration := time.Since(start)

			Expect(swapDuration).To(BeNumerically("<", 500*time.Millisecond),
				"Swap must not block waiting for old client Close")
		})
	})

	Describe("UT-KA-783-SC-005: Snapshot returns pinned client", func() {
		It("should return the current inner client", func() {
			inner := &recordingClient{id: "original"}
			sc, err := llm.NewSwappableClient(inner, "model-v1")
			Expect(err).NotTo(HaveOccurred())

			pinned := sc.Snapshot()
			Expect(pinned).NotTo(BeNil())

			resp, err := pinned.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("original"))
		})
	})

	Describe("UT-KA-783-SC-006: ModelName returns current model", func() {
		It("should return the model name set at construction", func() {
			inner := &recordingClient{id: "original"}
			sc, err := llm.NewSwappableClient(inner, "claude-3.5-sonnet")
			Expect(err).NotTo(HaveOccurred())
			Expect(sc.ModelName()).To(Equal("claude-3.5-sonnet"))
		})
	})

	Describe("UT-KA-783-SC-007: Close closes inner", func() {
		It("should close the current inner client", func() {
			inner := &recordingClient{id: "original"}
			sc, err := llm.NewSwappableClient(inner, "model-v1")
			Expect(err).NotTo(HaveOccurred())
			Expect(sc.Close()).To(Succeed())
			Expect(inner.closed.Load()).To(BeTrue())
		})
	})

	Describe("UT-KA-783-SC-008: Swap(nil) rejected", func() {
		It("should return an error when nil client is passed", func() {
			inner := &recordingClient{id: "original"}
			sc, err := llm.NewSwappableClient(inner, "model-v1")
			Expect(err).NotTo(HaveOccurred())
			Expect(sc.Swap(nil, "model-v2")).To(MatchError(ContainSubstring("nil")))
		})
	})

	Describe("UT-KA-783-SC-009: NewSwappableClient(nil) rejected", func() {
		It("should return an error when nil client is passed to constructor", func() {
			_, err := llm.NewSwappableClient(nil, "model-v1")
			Expect(err).To(MatchError(ContainSubstring("nil")))
		})
	})

	Describe("UT-KA-783-SC-010: Concurrent Chat+Swap no data race", func() {
		It("should handle concurrent Chat and Swap without race conditions", func() {
			original := &recordingClient{id: "original"}
			sc, err := llm.NewSwappableClient(original, "model-v1")
			Expect(err).NotTo(HaveOccurred())

			var wg sync.WaitGroup
			ctx := context.Background()

			for i := 0; i < 100; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, _ = sc.Chat(ctx, llm.ChatRequest{})
				}()
			}

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					newClient := &recordingClient{id: "swap"}
					_ = sc.Swap(newClient, "model-swap")
				}(i)
			}

			wg.Wait()
		})
	})

	Describe("UT-KA-783-SC-011: Concurrent Snapshot+Swap returns valid client", func() {
		It("should never return nil from Snapshot during concurrent Swap", func() {
			original := &recordingClient{id: "original"}
			sc, err := llm.NewSwappableClient(original, "model-v1")
			Expect(err).NotTo(HaveOccurred())

			var wg sync.WaitGroup

			for i := 0; i < 50; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					pinned := sc.Snapshot()
					Expect(pinned).NotTo(BeNil(),
						"Snapshot must never return nil during concurrent Swap")
				}()
			}

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_ = sc.Swap(&recordingClient{id: "swap"}, "model-swap")
				}()
			}

			wg.Wait()
		})
	})

	Describe("UT-KA-783-SC-012: Snapshot unaffected by subsequent Swap", func() {
		It("should use the original client even after Swap replaces inner", func() {
			original := &recordingClient{id: "pinned-original"}
			sc, err := llm.NewSwappableClient(original, "model-v1")
			Expect(err).NotTo(HaveOccurred())

			pinned := sc.Snapshot()

			replacement := &recordingClient{id: "new-after-swap"}
			Expect(sc.Swap(replacement, "model-v2")).To(Succeed())

			resp, err := pinned.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("pinned-original"),
				"Snapshot must use the client that was active at snapshot time, not the swapped-in client")

			resp2, err := sc.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp2.Message.Content).To(Equal("new-after-swap"),
				"SwappableClient must use the new client after Swap")
		})
	})

	Describe("UT-KA-783-SC-ModelName: ModelName updates after Swap", func() {
		It("should reflect the new model name after Swap", func() {
			original := &recordingClient{id: "original"}
			sc, err := llm.NewSwappableClient(original, "gpt-4")
			Expect(err).NotTo(HaveOccurred())
			Expect(sc.ModelName()).To(Equal("gpt-4"))

			Expect(sc.Swap(&recordingClient{id: "new"}, "claude-3.5-sonnet")).To(Succeed())
			Expect(sc.ModelName()).To(Equal("claude-3.5-sonnet"))
		})
	})

	Describe("UT-KA-783-SC-SlowCloseNonBlocking: Slow close does not block Chat", func() {
		It("should allow Chat calls while old client Close is running in background", func() {
			original := &recordingClient{id: "original", closeDelay: 3 * time.Second}
			replacement := &recordingClient{id: "replacement"}
			sc, err := llm.NewSwappableClient(original, "model-v1")
			Expect(err).NotTo(HaveOccurred())

			Expect(sc.Swap(replacement, "model-v2")).To(Succeed())

			start := time.Now()
			resp, err := sc.Chat(context.Background(), llm.ChatRequest{})
			chatDuration := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("replacement"))
			Expect(chatDuration).To(BeNumerically("<", 500*time.Millisecond),
				"Chat must not be blocked by background close of old client")
		})
	})

	Describe("Interface compliance", func() {
		It("should satisfy llm.Client interface", func() {
			inner := &recordingClient{id: "test"}
			sc, err := llm.NewSwappableClient(inner, "model")
			Expect(err).NotTo(HaveOccurred())
			var client llm.Client = sc
			Expect(client).NotTo(BeNil())
		})
	})
})
