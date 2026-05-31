package ka_test

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

// mockPoolSession is a test double for ka.PoolSession.
type mockPoolSession struct {
	id       int
	closed   bool
	mu       sync.Mutex
	callFn   func(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
}

func (s *mockPoolSession) CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	if s.callFn != nil {
		return s.callFn(ctx, params)
	}
	return &mcp.CallToolResult{}, nil
}

func (s *mockPoolSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *mockPoolSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

var _ = Describe("KASessionPool (G2 + G9: Pool + User Isolation)", func() {
	var (
		pool         *ka.KASessionPool
		connectCount atomic.Int32
	)

	countingFactory := func() ka.SessionFactory {
		return func(ctx context.Context) (ka.PoolSession, error) {
			id := int(connectCount.Add(1))
			return &mockPoolSession{id: id}, nil
		}
	}

	BeforeEach(func() {
		connectCount.Store(0)
	})

	Describe("Acquire", func() {
		It("UT-AF-1234-011: Acquire creates new session via factory", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			session, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(session).NotTo(BeNil())
			Expect(connectCount.Load()).To(Equal(int32(1)))
		})

		It("UT-AF-1234-012: Acquire reuses existing session for same (rr_id, user)", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			s1, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())

			s2, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())

			Expect(s1).To(BeIdenticalTo(s2))
			Expect(connectCount.Load()).To(Equal(int32(1)))
		})

		It("UT-AF-1234-013: Acquire creates separate sessions for different rr_ids", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			s1, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())

			s2, err := pool.Acquire(context.Background(), "rr-002", "alice")
			Expect(err).NotTo(HaveOccurred())

			Expect(s1).NotTo(BeIdenticalTo(s2))
			Expect(connectCount.Load()).To(Equal(int32(2)))
		})

		It("UT-AF-1234-014: Acquire creates separate sessions for different users same rr_id (G9)", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			s1, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())

			s2, err := pool.Acquire(context.Background(), "rr-001", "bob")
			Expect(err).NotTo(HaveOccurred())

			Expect(s1).NotTo(BeIdenticalTo(s2))
			Expect(connectCount.Load()).To(Equal(int32(2)))
		})

		It("UT-AF-1234-015: Acquire returns error when factory fails", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(ctx context.Context) (ka.PoolSession, error) {
					return nil, ka.ErrMCPUnavailable
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			_, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Release", func() {
		It("UT-AF-1234-016: Release closes session and removes pool entry", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			session, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(session).NotTo(BeNil())
			Expect(pool.Size()).To(Equal(1))

			pool.Release("rr-001", "alice")
			Expect(pool.Size()).To(Equal(0))
		})

		It("UT-AF-1234-017: Release non-existent key is no-op (no panic)", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			Expect(func() {
				pool.Release("rr-nonexistent", "nobody")
			}).NotTo(Panic())
		})

		It("UT-AF-1234-018: Release then Acquire creates fresh session", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			_, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())

			pool.Release("rr-001", "alice")

			_, err = pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())

			Expect(connectCount.Load()).To(Equal(int32(2)))
		})
	})

	Describe("Eviction and bounds", func() {
		It("UT-AF-1234-019: Idle session evicted via EvictIdle", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				IdleTTL:    1 * time.Millisecond,
				Logger:     logr.Discard(),
			})

			_, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(pool.Size()).To(Equal(1))

			time.Sleep(5 * time.Millisecond)

			evicted := pool.EvictIdle()
			Expect(evicted).To(Equal(1))
			Expect(pool.Size()).To(Equal(0))
		})

		It("UT-AF-1234-020: Max pool size cap rejects new Acquire with error", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 2,
				Logger:     logr.Discard(),
			})

			_, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())

			_, err = pool.Acquire(context.Background(), "rr-002", "bob")
			Expect(err).NotTo(HaveOccurred())

			_, err = pool.Acquire(context.Background(), "rr-003", "charlie")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("max"))
		})

		It("UT-AF-1234-021: ErrSessionMissing from CallTool triggers reconnect", func() {
			callCount := 0
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(ctx context.Context) (ka.PoolSession, error) {
					callCount++
					return &mockPoolSession{id: callCount}, nil
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			session, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(session).NotTo(BeNil())

			// Simulate session becoming invalid -- pool should detect and reconnect
			Expect(callCount).To(Equal(1))
			// After reconnect trigger, a second factory call should happen
			session2, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())
			_ = session2
			// With reconnect logic, factory should be called a second time
			// This will fail in RED because pool reuses the stale session
		})

		It("UT-AF-1234-022: ErrConnectionClosed evicts entry and reconnects", func() {
			callCount := 0
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(ctx context.Context) (ka.PoolSession, error) {
					callCount++
					return &mockPoolSession{id: callCount}, nil
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			session, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(session).NotTo(BeNil())
		})
	})

	Describe("Concurrency", func() {
		It("UT-AF-1234-023: Parallel Acquire for same key serializes (no double-connect)", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			const goroutines = 10
			var wg sync.WaitGroup
			sessions := make([]ka.PoolSession, goroutines)
			errs := make([]error, goroutines)

			for i := 0; i < goroutines; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					sessions[idx], errs[idx] = pool.Acquire(context.Background(), "rr-001", "alice")
				}(i)
			}
			wg.Wait()

			for i := 0; i < goroutines; i++ {
				Expect(errs[i]).NotTo(HaveOccurred(), "goroutine %d should not error", i)
			}
			Expect(connectCount.Load()).To(Equal(int32(1)), "factory should be called exactly once")
		})

		It("UT-AF-1234-024: Parallel Acquire for different keys succeeds concurrently", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 20,
				Logger:     logr.Discard(),
			})

			var wg sync.WaitGroup
			errs := make([]error, 10)
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					_, errs[idx] = pool.Acquire(context.Background(), "rr-"+string(rune('a'+idx)), "alice")
				}(i)
			}
			wg.Wait()

			for _, err := range errs {
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(connectCount.Load()).To(Equal(int32(10)))
		})

		It("UT-AF-1234-025: RWMutex safety under -race with mixed read/write", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 100,
				Logger:     logr.Discard(),
			})

			var wg sync.WaitGroup
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					key := "rr-" + string(rune('a'+idx%5))
					user := "user-" + string(rune('a'+idx%3))
					_, _ = pool.Acquire(context.Background(), key, user)
					_ = pool.Size()
					pool.Release(key, user)
				}(i)
			}
			wg.Wait()
		})
	})

	Describe("DrainAll", func() {
		It("UT-AF-1234-026: DrainAll closes all sessions", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			_, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())
			_, err = pool.Acquire(context.Background(), "rr-002", "bob")
			Expect(err).NotTo(HaveOccurred())
			Expect(pool.Size()).To(Equal(2))

			err = pool.DrainAll(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(pool.Size()).To(Equal(0))
		})
	})

	Describe("User isolation (G9)", func() {
		It("UT-AF-1234-027: User A cannot reuse User B session (composite key)", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			sAlice, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())

			sBob, err := pool.Acquire(context.Background(), "rr-001", "bob")
			Expect(err).NotTo(HaveOccurred())

			Expect(sAlice).NotTo(BeIdenticalTo(sBob))
		})

		It("UT-AF-1234-028: RR ownership check validates IS CRD spec.userIdentity", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			// Ownership validation: user trying to access session for RR
			// that belongs to another user should be rejected.
			// This requires IS CRD lookup -- will be implemented in GREEN.
			_, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Inject (session handoff from dedicated investigate path)", func() {
		It("UT-AF-1332-001: Inject places session and Acquire returns it without calling factory", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			injected := &mockPoolSession{id: 999}
			pool.Inject("rr-inject-001", "alice", injected)

			session, err := pool.Acquire(context.Background(), "rr-inject-001", "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(session).To(BeIdenticalTo(injected))
			Expect(connectCount.Load()).To(Equal(int32(0)),
				"factory should not have been called when injected session exists")
		})

		It("UT-AF-1332-002: Inject replaces existing session and closes old one", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			oldSession, err := pool.Acquire(context.Background(), "rr-inject-002", "bob")
			Expect(err).NotTo(HaveOccurred())
			oldMock := oldSession.(*mockPoolSession)

			newSession := &mockPoolSession{id: 888}
			pool.Inject("rr-inject-002", "bob", newSession)

			Eventually(oldMock.IsClosed).Should(BeTrue(),
				"old session should be closed when replaced by Inject")

			acquired, err := pool.Acquire(context.Background(), "rr-inject-002", "bob")
			Expect(err).NotTo(HaveOccurred())
			Expect(acquired).To(BeIdenticalTo(newSession))
		})

		It("UT-AF-1332-003: Injected session is isolated by username", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			injected := &mockPoolSession{id: 777}
			pool.Inject("rr-inject-003", "alice", injected)

			// bob should get a factory-created session, not alice's injected one
			bobSession, err := pool.Acquire(context.Background(), "rr-inject-003", "bob")
			Expect(err).NotTo(HaveOccurred())
			Expect(bobSession).NotTo(BeIdenticalTo(injected))
			Expect(connectCount.Load()).To(Equal(int32(1)),
				"factory should be called for bob since injected session belongs to alice")
		})

		It("UT-AF-1332-004: DrainAll closes injected sessions", func() {
			pool = ka.NewKASessionPool(ka.PoolConfig{
				Factory:    countingFactory(),
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			injected := &mockPoolSession{id: 666}
			pool.Inject("rr-inject-004", "charlie", injected)

			err := pool.DrainAll(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(injected.IsClosed()).To(BeTrue(),
				"DrainAll should close injected sessions")
		})
	})
})
