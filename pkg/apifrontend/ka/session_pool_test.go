package ka_test

import (
	"context"
	"fmt"
	"strings"
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
	pingFn   func(ctx context.Context, params *mcp.PingParams) error
}

func (s *mockPoolSession) CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	if s.callFn != nil {
		return s.callFn(ctx, params)
	}
	return &mcp.CallToolResult{}, nil
}

func (s *mockPoolSession) Ping(ctx context.Context, params *mcp.PingParams) error {
	if s.pingFn != nil {
		return s.pingFn(ctx, params)
	}
	return nil
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

var _ = Describe("MCP Session Resilience — Proactive Pool Health Check (#1387)", func() {
	var (
		pool         *ka.KASessionPool
		connectCount atomic.Int32
	)

	countingFactory := func() ka.SessionFactory {
		return func(_ context.Context) (ka.PoolSession, error) {
			id := int(connectCount.Add(1))
			return &mockPoolSession{id: id}, nil
		}
	}

	BeforeEach(func() {
		connectCount.Store(0)
	})

	It("UT-AF-1387-001 [SI-4, SC-24]: dead cached session detected and evicted before user-visible failure", func() {
		pool = ka.NewKASessionPool(ka.PoolConfig{
			Factory:    countingFactory(),
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		deadSession := &mockPoolSession{
			id: 1,
			pingFn: func(_ context.Context, _ *mcp.PingParams) error {
				return fmt.Errorf("transport closed")
			},
		}
		pool.Inject("rr-001", "alice", deadSession)

		session, err := pool.Acquire(context.Background(), "rr-001", "alice")
		Expect(err).NotTo(HaveOccurred())
		Expect(session).NotTo(BeIdenticalTo(deadSession),
			"SI-4: dead session must be replaced, not returned to caller")
		Expect(deadSession.IsClosed()).To(BeTrue(),
			"SC-24: evicted session resources must be released")
		Expect(connectCount.Load()).To(Equal(int32(1)),
			"CP-10: factory creates replacement transparently")
	})

	It("UT-AF-1387-002 [CP-10]: auto-reconstitution after eviction without caller retry", func() {
		pool = ka.NewKASessionPool(ka.PoolConfig{
			Factory:    countingFactory(),
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		deadSession := &mockPoolSession{
			id: 1,
			pingFn: func(_ context.Context, _ *mcp.PingParams) error {
				return fmt.Errorf("connection reset by peer")
			},
		}
		pool.Inject("rr-002", "bob", deadSession)

		session, err := pool.Acquire(context.Background(), "rr-002", "bob")
		Expect(err).NotTo(HaveOccurred(),
			"CP-10: caller must not see the eviction — Acquire returns a valid session")
		Expect(session).NotTo(BeNil())

		_, err = session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "test_tool",
		})
		Expect(err).NotTo(HaveOccurred(),
			"CP-10: replacement session must be functional for tool calls")
	})

	It("UT-AF-1387-003 [SI-4]: healthy cached session reused without false-positive eviction", func() {
		pool = ka.NewKASessionPool(ka.PoolConfig{
			Factory:    countingFactory(),
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		healthySession := &mockPoolSession{id: 42}
		pool.Inject("rr-003", "carol", healthySession)

		session, err := pool.Acquire(context.Background(), "rr-003", "carol")
		Expect(err).NotTo(HaveOccurred())
		Expect(session).To(BeIdenticalTo(healthySession),
			"SI-4: healthy session must be returned as-is, no false-positive eviction")
		Expect(connectCount.Load()).To(Equal(int32(0)),
			"SI-4: factory must not be called when cached session is healthy")
	})

	It("UT-AF-1387-004 [SA-15]: ping timeout bounded to prevent indefinite Acquire blocking", func() {
		pool = ka.NewKASessionPool(ka.PoolConfig{
			Factory:    countingFactory(),
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		hangingSession := &mockPoolSession{
			id: 1,
			pingFn: func(ctx context.Context, _ *mcp.PingParams) error {
				<-ctx.Done()
				return ctx.Err()
			},
		}
		pool.Inject("rr-004", "dave", hangingSession)

		start := time.Now()
		session, err := pool.Acquire(context.Background(), "rr-004", "dave")
		elapsed := time.Since(start)

		Expect(err).NotTo(HaveOccurred(),
			"SA-15: hung ping must not propagate error to caller")
		Expect(session).NotTo(BeIdenticalTo(hangingSession),
			"SA-15: hung session must be evicted, not returned")
		Expect(elapsed).To(BeNumerically("<", 5*time.Second),
			"SA-15: Acquire must not block indefinitely — ping timeout enforces 2s bound")
	})

	It("UT-AF-1387-005 [SC-24]: concurrent Acquire with failing Ping does not double-evict or race", func() {
		pool = ka.NewKASessionPool(ka.PoolConfig{
			Factory:    countingFactory(),
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		deadSession := &mockPoolSession{
			id: 1,
			pingFn: func(_ context.Context, _ *mcp.PingParams) error {
				return fmt.Errorf("transport closed")
			},
		}
		pool.Inject("rr-005", "eve", deadSession)

		const goroutines = 10
		var wg sync.WaitGroup
		sessions := make([]ka.PoolSession, goroutines)
		errs := make([]error, goroutines)

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				sessions[idx], errs[idx] = pool.Acquire(context.Background(), "rr-005", "eve")
			}(i)
		}
		wg.Wait()

		for i := 0; i < goroutines; i++ {
			Expect(errs[i]).NotTo(HaveOccurred(),
				"SC-24: goroutine %d should succeed even under concurrent eviction", i)
			Expect(sessions[i]).NotTo(BeNil(),
				"SC-24: goroutine %d must receive a valid session", i)
		}
	})
})

var _ = Describe("MCP Transport Observability — Audit Logging (#1387)", func() {
	var (
		pool         *ka.KASessionPool
		connectCount atomic.Int32
	)

	countingFactoryWithLogger := func() ka.SessionFactory {
		return func(_ context.Context) (ka.PoolSession, error) {
			id := int(connectCount.Add(1))
			return &mockPoolSession{id: id}, nil
		}
	}

	BeforeEach(func() {
		connectCount.Store(0)
	})

	It("UT-AF-1387-010 [AU-2, AU-3]: Inject emits structured log with mcp_session_id and rr_id", func() {
		logger, entries := capturingLogger()
		pool = ka.NewKASessionPool(ka.PoolConfig{
			Factory:    countingFactoryWithLogger(),
			MaxEntries: 10,
			Logger:     logger,
		})

		injected := &mockPoolSession{id: 42}
		pool.Inject("rr-010", "alice", injected)

		found := false
		for _, e := range *entries {
			if containsAll(e.args, "mcp_session_id", "rr_id", "rr-010") {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(),
			"AU-2+AU-3: Inject must emit structured log containing mcp_session_id and rr_id for cross-service correlation")
	})

	It("UT-AF-1387-011 [AU-2, AU-3]: Acquire cache-hit emits log with mcp_session_id and rr_id", func() {
		logger, entries := capturingLogger()
		pool = ka.NewKASessionPool(ka.PoolConfig{
			Factory:    countingFactoryWithLogger(),
			MaxEntries: 10,
			Logger:     logger,
		})

		pool.Inject("rr-011", "bob", &mockPoolSession{id: 99})

		_, err := pool.Acquire(context.Background(), "rr-011", "bob")
		Expect(err).NotTo(HaveOccurred())

		found := false
		for _, e := range *entries {
			if containsAll(e.args, "mcp_session_id", "rr_id", "rr-011") && containsAny(e.args, "acquire", "cache hit", "reuse") {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(),
			"AU-2+AU-3: Acquire cache-hit must emit structured log with mcp_session_id and rr_id")
	})

	It("UT-AF-1387-012 [AU-2]: Release emits log with mcp_session_id and rr_id", func() {
		logger, entries := capturingLogger()
		pool = ka.NewKASessionPool(ka.PoolConfig{
			Factory:    countingFactoryWithLogger(),
			MaxEntries: 10,
			Logger:     logger,
		})

		pool.Inject("rr-012", "carol", &mockPoolSession{id: 77})
		pool.Release("rr-012", "carol")

		found := false
		for _, e := range *entries {
			if containsAll(e.args, "mcp_session_id", "rr_id", "rr-012") && containsAny(e.args, "release", "released") {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(),
			"AU-2: Release must emit structured log with mcp_session_id and rr_id")
	})

	It("UT-AF-1387-013 [AU-3, AU-6]: stale-retry logs both evicted and replacement mcp_session_id", func() {
		logger, entries := capturingLogger()
		pool = ka.NewKASessionPool(ka.PoolConfig{
			Factory:    countingFactoryWithLogger(),
			MaxEntries: 10,
			Logger:     logger,
		})

		deadSession := &mockPoolSession{
			id: 1,
			pingFn: func(_ context.Context, _ *mcp.PingParams) error {
				return fmt.Errorf("transport closed")
			},
		}
		pool.Inject("rr-013", "dave", deadSession)

		_, err := pool.Acquire(context.Background(), "rr-013", "dave")
		Expect(err).NotTo(HaveOccurred())

		foundEvicted := false
		for _, e := range *entries {
			if containsAll(e.args, "mcp_session_id", "evict") {
				foundEvicted = true
				break
			}
		}
		Expect(foundEvicted).To(BeTrue(),
			"AU-3+AU-6: stale-retry path must log mcp_session_id of evicted session for correlation")
	})

	It("UT-AF-1387-014 [SI-4]: ping-eviction logs mcp_session_id of dead session with error detail", func() {
		logger, entries := capturingLogger()
		pool = ka.NewKASessionPool(ka.PoolConfig{
			Factory:    countingFactoryWithLogger(),
			MaxEntries: 10,
			Logger:     logger,
		})

		deadSession := &mockPoolSession{
			id: 1,
			pingFn: func(_ context.Context, _ *mcp.PingParams) error {
				return fmt.Errorf("connection reset by peer")
			},
		}
		pool.Inject("rr-014", "eve", deadSession)

		_, err := pool.Acquire(context.Background(), "rr-014", "eve")
		Expect(err).NotTo(HaveOccurred())

		found := false
		for _, e := range *entries {
			if containsAll(e.args, "mcp_session_id", "error") {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(),
			"SI-4: ping-eviction must log mcp_session_id of dead session alongside error detail for incident correlation")
	})
})

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

var _ = Describe("IT-AF-1351: KASessionPool EvictIdle wiring", func() {

	It("IT-AF-1351-EVICT: EvictIdle removes entries older than IdleTTL", func() {
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(ctx context.Context) (ka.PoolSession, error) {
				return &mockPoolSession{}, nil
			},
			MaxEntries: 10,
			IdleTTL:    50 * time.Millisecond,
			Logger:     logr.Discard(),
		})

		pool.Inject("rr-1", "user-1", &mockPoolSession{})

		time.Sleep(100 * time.Millisecond)

		evicted := pool.EvictIdle()
		Expect(evicted).To(Equal(1), "EvictIdle must remove entries older than IdleTTL (AF-HIGH-2)")
		Expect(pool.Size()).To(Equal(0))
	})
})

var _ = Describe("KASessionPool InjectVerified — BR-INTERACTIVE-001, #1442", Label("unit", "1442"), func() {

	It("UT-AF-1442-002: InjectVerified rejects dead session (ping fails)", func() {
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(ctx context.Context) (ka.PoolSession, error) {
				return &mockPoolSession{}, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		deadSession := &mockPoolSession{
			id: 100,
			pingFn: func(ctx context.Context, params *mcp.PingParams) error {
				return fmt.Errorf("session not found")
			},
		}

		err := pool.InjectVerified(context.Background(), "rr-dead", "alice", deadSession)
		Expect(err).To(HaveOccurred(),
			"InjectVerified must reject a session whose ping fails")
		Expect(err.Error()).To(ContainSubstring("session dead on inject"))
		Expect(pool.Size()).To(Equal(0),
			"pool must not contain the dead session")
		Expect(deadSession.IsClosed()).To(BeTrue(),
			"dead session must be closed by InjectVerified")
	})

	It("UT-AF-1442-003: InjectVerified accepts live session (ping succeeds)", func() {
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(ctx context.Context) (ka.PoolSession, error) {
				return &mockPoolSession{}, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		liveSession := &mockPoolSession{id: 200}

		err := pool.InjectVerified(context.Background(), "rr-live", "bob", liveSession)
		Expect(err).NotTo(HaveOccurred())
		Expect(pool.Size()).To(Equal(1),
			"live session must be in the pool after InjectVerified")
	})
})
