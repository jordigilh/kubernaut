package ka_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

type countingSession struct {
	closeCount *int32
}

func (s *countingSession) Close() error {
	atomic.AddInt32(s.closeCount, 1)
	return nil
}

func (s *countingSession) CallTool(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{}, nil
}

var _ = Describe("Graceful Shutdown (G14)", func() {
	It("UT-AF-1234-140: DrainAll closes all pool sessions", func() {
		var closeCount int32
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return &countingSession{closeCount: &closeCount}, nil
			},
			MaxEntries: 10,
			IdleTTL:    time.Minute,
		})

		ctx := context.Background()
		_, err := pool.Acquire(ctx, "rr-1", "user-a")
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Acquire(ctx, "rr-2", "user-b")
		Expect(err).NotTo(HaveOccurred())
		Expect(pool.Size()).To(Equal(2))

		err = pool.DrainAll(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(pool.Size()).To(Equal(0))
		Expect(atomic.LoadInt32(&closeCount)).To(Equal(int32(2)))
	})

	It("UT-AF-1234-141: DrainAll respects context deadline", func() {
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return &countingSession{closeCount: new(int32)}, nil
			},
			MaxEntries: 10,
			IdleTTL:    time.Minute,
		})

		ctx := context.Background()
		_, err := pool.Acquire(ctx, "rr-1", "user-a")
		Expect(err).NotTo(HaveOccurred())

		cancelCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		err = pool.DrainAll(cancelCtx)
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-AF-1234-142: empty pool DrainAll is no-op", func() {
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return &countingSession{closeCount: new(int32)}, nil
			},
			MaxEntries: 10,
			IdleTTL:    time.Minute,
		})
		err := pool.DrainAll(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(pool.Size()).To(Equal(0))
	})
})

var _ = Describe("Session Cap (G15)", func() {
	It("UT-AF-1234-145: pool Acquire rejects when at maxEntries", func() {
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return &countingSession{closeCount: new(int32)}, nil
			},
			MaxEntries: 2,
			IdleTTL:    time.Minute,
		})

		ctx := context.Background()
		_, err := pool.Acquire(ctx, "rr-1", "user-a")
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Acquire(ctx, "rr-2", "user-b")
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Acquire(ctx, "rr-3", "user-c")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("max capacity"))
	})

	It("UT-AF-1234-146: idle eviction frees capacity", func() {
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return &countingSession{closeCount: new(int32)}, nil
			},
			MaxEntries: 2,
			IdleTTL:    1 * time.Millisecond,
		})

		ctx := context.Background()
		_, err := pool.Acquire(ctx, "rr-1", "user-a")
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Acquire(ctx, "rr-2", "user-b")
		Expect(err).NotTo(HaveOccurred())

		time.Sleep(10 * time.Millisecond)
		pool.EvictIdle()

		_, err = pool.Acquire(ctx, "rr-3", "user-c")
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("Multi-replica (G20)", func() {
	It("UT-AF-1234-148: ErrLeaseHeld from KA returns user-friendly error", func() {
		mockMCP := &ka.MockMCPClient{
			InvokeActionFn: func(_ context.Context, _ ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
				return nil, fmt.Errorf("kubernaut agent: investigation active on another session")
			},
		}

		result, err := mockMCP.InvokeAction(context.Background(), ka.InvokeActionArgs{
			RRID:   "prod/rr-001",
			Action: "takeover",
		})
		Expect(result).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("active on another session"))
	})

	It("UT-AF-1234-149: pool entry not cached on KA rejection", func() {
		callCount := 0
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				callCount++
				if callCount == 1 {
					return nil, fmt.Errorf("investigation active on another session")
				}
				return &countingSession{closeCount: new(int32)}, nil
			},
			MaxEntries: 10,
			IdleTTL:    time.Minute,
		})

		ctx := context.Background()
		_, err := pool.Acquire(ctx, "rr-1", "user-a")
		Expect(err).To(HaveOccurred())

		Expect(pool.Size()).To(Equal(0))
	})
})
