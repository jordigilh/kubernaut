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
	"errors"
	"sync"
	"time"
)

const oldClientCloseTimeout = 5 * time.Second

// SwappableClient wraps an llm.Client with a sync.RWMutex so the underlying
// client can be hot-swapped at runtime without restarting the process.
//
// Design constraints (per #783 adversarial review):
//   - Chat copies the inner client under RLock then releases before calling
//     Chat on the copy. This ensures Swap is never blocked by an in-flight
//     LLM network call.
//   - Swap closes the old client in a background goroutine with a timeout
//     so it never blocks callers.
//   - Snapshot returns a bare llm.Client pinned at the current moment;
//     subsequent Swap calls do not affect outstanding snapshots.
type SwappableClient struct {
	mu        sync.RWMutex
	inner     Client
	modelName string
}

// NewSwappableClient creates a SwappableClient wrapping the given initial client.
func NewSwappableClient(initial Client, modelName string) (*SwappableClient, error) {
	if initial == nil {
		return nil, errors.New("llm: SwappableClient requires non-nil initial client")
	}
	return &SwappableClient{inner: initial, modelName: modelName}, nil
}

// Chat delegates to the inner client. The inner reference is copied under
// RLock and released before the actual network call, so Swap is never
// blocked by slow LLM round-trips.
func (sc *SwappableClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	sc.mu.RLock()
	c := sc.inner
	sc.mu.RUnlock()
	return c.Chat(ctx, req)
}

// StreamChat delegates to the inner client's StreamChat. The inner reference
// is copied under RLock and released before the network call.
func (sc *SwappableClient) StreamChat(ctx context.Context, req ChatRequest, callback func(ChatStreamEvent) error) (ChatResponse, error) {
	sc.mu.RLock()
	c := sc.inner
	sc.mu.RUnlock()
	return c.StreamChat(ctx, req, callback)
}

// Close closes the current inner client.
func (sc *SwappableClient) Close() error {
	sc.mu.RLock()
	c := sc.inner
	sc.mu.RUnlock()
	return c.Close()
}

// Swap atomically replaces the inner client and model name. The old client's
// Close is called in a background goroutine with a timeout to avoid blocking.
func (sc *SwappableClient) Swap(newClient Client, newModelName string) error {
	if newClient == nil {
		return errors.New("llm: cannot swap to nil client")
	}
	sc.mu.Lock()
	old := sc.inner
	sc.inner = newClient
	sc.modelName = newModelName
	sc.mu.Unlock()

	go closeWithTimeout(old, oldClientCloseTimeout)
	return nil
}

// Snapshot returns the current inner client under RLock. The returned client
// is a direct reference to the concrete implementation at this point in time;
// subsequent Swap calls do not affect it.
func (sc *SwappableClient) Snapshot() Client {
	sc.mu.RLock()
	c := sc.inner
	sc.mu.RUnlock()
	return c
}

// ModelName returns the model name associated with the current inner client.
func (sc *SwappableClient) ModelName() string {
	sc.mu.RLock()
	name := sc.modelName
	sc.mu.RUnlock()
	return name
}

// closeWithTimeout closes c in a background goroutine, abandoning it after
// timeout. This accepts a potential goroutine leak (#62) as a deliberate
// trade-off: blocking Swap indefinitely on a hung Close would stall all
// in-flight LLM calls waiting for the write lock.
func closeWithTimeout(c Client, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		_ = c.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

var _ Client = (*SwappableClient)(nil)
