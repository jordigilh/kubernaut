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

package ka_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

var _ = Describe("Pool onRelease callback (#1438)", func() {
	var pool *ka.KASessionPool

	Describe("UT-AF-1438-030 (SI-4): InjectWithCleanup stores onRelease; Release invokes it", func() {
		It("should invoke onRelease when Release is called", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(ctx context.Context) (ka.PoolSession, error) {
					return &mockPoolSession{}, nil
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			var released atomic.Bool
			pool.InjectWithCleanup("rr-1438-030", "alice", &mockPoolSession{id: 1}, func() {
				released.Store(true)
			})

			pool.Release("rr-1438-030", "alice")

			Expect(released.Load()).To(BeTrue(),
				"SI-4: Release must invoke onRelease so watcher goroutine exits deterministically")
		})
	})

	Describe("UT-AF-1438-031: EvictIdle invokes onRelease for each evicted entry", func() {
		It("should invoke onRelease for each idle-evicted entry", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(ctx context.Context) (ka.PoolSession, error) {
					return &mockPoolSession{}, nil
				},
				MaxEntries: 10,
				IdleTTL:    1 * time.Millisecond,
				Logger:     logr.Discard(),
			})

			var count atomic.Int32
			pool.InjectWithCleanup("rr-1438-031a", "alice", &mockPoolSession{id: 1}, func() {
				count.Add(1)
			})
			pool.InjectWithCleanup("rr-1438-031b", "bob", &mockPoolSession{id: 2}, func() {
				count.Add(1)
			})

			time.Sleep(5 * time.Millisecond)
			evicted := pool.EvictIdle()
			Expect(evicted).To(Equal(2))
			Expect(count.Load()).To(Equal(int32(2)),
				"EvictIdle must invoke onRelease for each evicted entry")
		})
	})

	Describe("UT-AF-1438-032: DrainAll invokes onRelease for all entries", func() {
		It("should invoke onRelease for each drained entry", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(ctx context.Context) (ka.PoolSession, error) {
					return &mockPoolSession{}, nil
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			var count atomic.Int32
			pool.InjectWithCleanup("rr-1438-032a", "alice", &mockPoolSession{id: 1}, func() {
				count.Add(1)
			})
			pool.InjectWithCleanup("rr-1438-032b", "bob", &mockPoolSession{id: 2}, func() {
				count.Add(1)
			})

			err := pool.DrainAll(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(count.Load()).To(Equal(int32(2)),
				"DrainAll must invoke onRelease for all entries")
		})
	})

	Describe("UT-AF-1438-033: Acquire stale-eviction invokes onRelease", func() {
		It("should invoke onRelease when Acquire evicts a dead session", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(ctx context.Context) (ka.PoolSession, error) {
					return &mockPoolSession{id: 99}, nil
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			var released atomic.Bool
			deadSession := &mockPoolSession{
				id: 1,
				pingFn: func(_ context.Context, _ *mcp.PingParams) error {
					return fmt.Errorf("transport closed")
				},
			}
			pool.InjectWithCleanup("rr-1438-033", "alice", deadSession, func() {
				released.Store(true)
			})

			_, err := pool.Acquire(context.Background(), "rr-1438-033", "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(released.Load()).To(BeTrue(),
				"Acquire stale-eviction must invoke onRelease before closing dead session")
		})
	})

	Describe("UT-AF-1438-034: InjectWithCleanup replacing existing entry invokes old onRelease", func() {
		It("should invoke old entry's onRelease when replaced", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(ctx context.Context) (ka.PoolSession, error) {
					return &mockPoolSession{}, nil
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			var oldReleased atomic.Bool
			pool.InjectWithCleanup("rr-1438-034", "alice", &mockPoolSession{id: 1}, func() {
				oldReleased.Store(true)
			})

			var newReleased atomic.Bool
			pool.InjectWithCleanup("rr-1438-034", "alice", &mockPoolSession{id: 2}, func() {
				newReleased.Store(true)
			})

			Expect(oldReleased.Load()).To(BeTrue(),
				"replacing entry via InjectWithCleanup must invoke old entry's onRelease")
			Expect(newReleased.Load()).To(BeFalse(),
				"new entry's onRelease must not be invoked yet")
		})
	})

	Describe("UT-AF-1438-035: Inject (no callback) does not panic on Release", func() {
		It("should not panic when Release is called on entry without onRelease", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(ctx context.Context) (ka.PoolSession, error) {
					return &mockPoolSession{}, nil
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			pool.Inject("rr-1438-035", "alice", &mockPoolSession{id: 1})

			Expect(func() {
				pool.Release("rr-1438-035", "alice")
			}).NotTo(Panic(),
				"Release on entry without onRelease must be backward-compatible")
		})
	})
})
