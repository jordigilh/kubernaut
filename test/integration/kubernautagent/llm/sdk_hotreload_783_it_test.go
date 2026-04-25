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
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
)

// recordingClient captures Chat calls and tracks Close for assertions.
type recordingClient struct {
	id         string
	chatCount  atomic.Int64
	closed     atomic.Bool
	closeDelay time.Duration
}

func (r *recordingClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	r.chatCount.Add(1)
	return llm.ChatResponse{Message: llm.Message{Content: r.id}}, nil
}

func (r *recordingClient) StreamChat(ctx context.Context, req llm.ChatRequest, cb func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	resp, err := r.Chat(ctx, req)
	if err == nil {
		_ = cb(llm.ChatStreamEvent{Delta: resp.Message.Content, Done: true})
	}
	return resp, err
}

func (r *recordingClient) Close() error {
	if r.closeDelay > 0 {
		time.Sleep(r.closeDelay)
	}
	r.closed.Store(true)
	return nil
}

var _ = Describe("SDK Hot-Reload Integration — TP-783-IT (#783)", func() {

	Describe("IT-KA-783-001: FileWatcher + SwappableClient full reload flow", func() {
		It("detects file change, invokes callback, and swaps the client", func() {
			tmpDir := GinkgoT().TempDir()
			sdkFile := filepath.Join(tmpDir, "sdk-config.yaml")

			Expect(os.WriteFile(sdkFile, []byte("version: 1"), 0644)).To(Succeed())

			clientV1 := &recordingClient{id: "v1"}
			clientV2 := &recordingClient{id: "v2"}
			sc, err := llm.NewSwappableClient(clientV1, "model-v1")
			Expect(err).NotTo(HaveOccurred())

			var callbackInvoked atomic.Int64
			callback := func(newContent string) error {
				callbackInvoked.Add(1)
				return sc.Swap(clientV2, "model-v2")
			}

			fw, err := hotreload.NewFileWatcher(sdkFile, callback, logr.Discard())
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			Expect(fw.Start(ctx)).To(Succeed())
			defer fw.Stop()

			// The initial load triggers the callback once
			Expect(callbackInvoked.Load()).To(Equal(int64(1)))

			// Write new content to trigger a reload
			Expect(os.WriteFile(sdkFile, []byte("version: 2"), 0644)).To(Succeed())

			Eventually(func() int64 {
				return callbackInvoked.Load()
			}, 5*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 2),
				"FileWatcher should detect the file change and invoke the callback")

			Expect(sc.ModelName()).To(Equal("model-v2"))
			resp, err := sc.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("v2"))
		})
	})

	Describe("IT-KA-783-002: Concurrent investigation during swap", func() {
		It("pinned snapshot continues using original client while new Chat uses swapped client", func() {
			original := &recordingClient{id: "investigation-model-A"}
			sc, err := llm.NewSwappableClient(original, "model-A")
			Expect(err).NotTo(HaveOccurred())

			pinned := sc.Snapshot()
			pinnedModel := sc.ModelName()
			Expect(pinnedModel).To(Equal("model-A"))

			replacement := &recordingClient{id: "investigation-model-B"}
			Expect(sc.Swap(replacement, "model-B")).To(Succeed())

			// Simulate multiple "turns" in an investigation using the pinned client
			for i := 0; i < 5; i++ {
				resp, err := pinned.Chat(context.Background(), llm.ChatRequest{})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Message.Content).To(Equal("investigation-model-A"),
					"in-flight investigation must use the pinned client, not the swapped one")
			}
			Expect(original.chatCount.Load()).To(Equal(int64(5)))

			// New investigation should use the new client
			newPinned := sc.Snapshot()
			resp, err := newPinned.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("investigation-model-B"))
			Expect(sc.ModelName()).To(Equal("model-B"))
		})
	})

	Describe("IT-KA-783-003: Rejection preserves old client", func() {
		It("continues serving with original client when callback rejects", func() {
			tmpDir := GinkgoT().TempDir()
			sdkFile := filepath.Join(tmpDir, "sdk-config.yaml")
			Expect(os.WriteFile(sdkFile, []byte("version: 1"), 0644)).To(Succeed())

			original := &recordingClient{id: "stable"}
			sc, err := llm.NewSwappableClient(original, "stable-model")
			Expect(err).NotTo(HaveOccurred())

			var rejectCount atomic.Int64
			callback := func(newContent string) error {
				if newContent == "version: 1" {
					return nil // Accept initial load
				}
				rejectCount.Add(1)
				return fmt.Errorf("rejected: simulated provider change")
			}

			fw, err := hotreload.NewFileWatcher(sdkFile, callback, logr.Discard())
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			Expect(fw.Start(ctx)).To(Succeed())
			defer fw.Stop()

			Expect(os.WriteFile(sdkFile, []byte("version: REJECTED"), 0644)).To(Succeed())

			Eventually(func() int64 {
				return rejectCount.Load()
			}, 5*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			Expect(sc.ModelName()).To(Equal("stable-model"),
				"model must not change after rejected reload")
			resp, err := sc.Chat(context.Background(), llm.ChatRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("stable"),
				"original client must continue serving after rejection")
		})
	})

	Describe("IT-KA-783-004: Rapid successive reloads (debounce)", func() {
		It("coalesces rapid writes into a single callback after debounce period", func() {
			tmpDir := GinkgoT().TempDir()
			sdkFile := filepath.Join(tmpDir, "sdk-config.yaml")
			Expect(os.WriteFile(sdkFile, []byte("version: 0"), 0644)).To(Succeed())

			var mu sync.Mutex
			var receivedContents []string
			callback := func(newContent string) error {
				mu.Lock()
				receivedContents = append(receivedContents, newContent)
				mu.Unlock()
				return nil
			}

			fw, err := hotreload.NewFileWatcher(sdkFile, callback, logr.Discard())
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			Expect(fw.Start(ctx)).To(Succeed())
			defer fw.Stop()

			mu.Lock()
			initialCount := len(receivedContents)
			mu.Unlock()
			Expect(initialCount).To(Equal(1), "initial load should trigger one callback")

			// Write 3 times in rapid succession
			for i := 1; i <= 3; i++ {
				Expect(os.WriteFile(sdkFile, []byte(fmt.Sprintf("version: %d", i)), 0644)).To(Succeed())
				time.Sleep(20 * time.Millisecond)
			}

			// Wait for debounce (200ms) + processing margin
			Eventually(func() int {
				mu.Lock()
				defer mu.Unlock()
				return len(receivedContents)
			}, 5*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 2),
				"at least one additional callback should fire after rapid writes")

			mu.Lock()
			lastContent := receivedContents[len(receivedContents)-1]
			mu.Unlock()
			Expect(lastContent).To(Equal("version: 3"),
				"final callback should receive the last written content")
		})
	})

	Describe("IT-KA-783-005: Old client Close invoked on swap", func() {
		It("verifies old client is explicitly closed when swap succeeds", func() {
			original := &recordingClient{id: "old"}
			replacement := &recordingClient{id: "new"}

			sc, err := llm.NewSwappableClient(original, "model-v1")
			Expect(err).NotTo(HaveOccurred())

			Expect(sc.Swap(replacement, "model-v2")).To(Succeed())

			Eventually(func() bool {
				return original.closed.Load()
			}, 3*time.Second, 10*time.Millisecond).Should(BeTrue(),
				"old client must be closed after swap")
			Expect(replacement.closed.Load()).To(BeFalse(),
				"new client must not be closed")
		})
	})

	Describe("IT-KA-783-006: Concurrent Chat during FileWatcher reload", func() {
		It("Chat calls succeed during an active reload", func() {
			tmpDir := GinkgoT().TempDir()
			sdkFile := filepath.Join(tmpDir, "sdk-config.yaml")
			Expect(os.WriteFile(sdkFile, []byte("version: 0"), 0644)).To(Succeed())

			clientV1 := &recordingClient{id: "v1"}
			sc, err := llm.NewSwappableClient(clientV1, "model-v1")
			Expect(err).NotTo(HaveOccurred())

			swapIdx := atomic.Int64{}
			callback := func(newContent string) error {
				idx := swapIdx.Add(1)
				newC := &recordingClient{id: fmt.Sprintf("v%d", idx+1)}
				return sc.Swap(newC, fmt.Sprintf("model-v%d", idx+1))
			}

			fw, err := hotreload.NewFileWatcher(sdkFile, callback, logr.Discard())
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			Expect(fw.Start(ctx)).To(Succeed())
			defer fw.Stop()

			var wg sync.WaitGroup
			chatErrors := make(chan error, 200)

			// Fire concurrent Chat calls
			for i := 0; i < 100; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := sc.Chat(context.Background(), llm.ChatRequest{})
					if err != nil {
						chatErrors <- err
					}
				}()
			}

			// Trigger a reload while Chat calls are in flight
			Expect(os.WriteFile(sdkFile, []byte("version: concurrent"), 0644)).To(Succeed())

			wg.Wait()
			close(chatErrors)

			for err := range chatErrors {
				Fail(fmt.Sprintf("Chat should not fail during reload: %v", err))
			}
		})
	})
})
