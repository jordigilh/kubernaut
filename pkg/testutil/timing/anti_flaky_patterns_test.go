package timing

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAntiFlakyPatterns(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Anti-Flaky Test Patterns Suite")
}

// These tests validate the anti-flaky patterns themselves
// Run 100 times in CI to ensure <1% flakiness: go test -count=100

var _ = Describe("SyncPoint Pattern", func() {
	It("should coordinate goroutines deterministically", func() {
		ctx := context.Background()
		syncPoint := NewSyncPoint()
		var executed atomic.Bool

		// Start goroutine waiting for signal
		go func() {
			defer GinkgoRecover()
			err := syncPoint.WaitForReady(ctx)
			Expect(err).NotTo(HaveOccurred())
			executed.Store(true)
		}()

		// Brief sleep to ensure goroutine is waiting
		time.Sleep(10 * time.Millisecond)
		Expect(executed.Load()).To(BeFalse(), "should not execute before signal")

		// Signal and proceed
		<-syncPoint.Signal()
		syncPoint.Proceed()

		// Wait for execution
		Eventually(func() bool {
			return executed.Load()
		}, 1*time.Second).Should(BeTrue())
	})

	It("should handle context cancellation", func() {
		ctx, cancel := context.WithCancel(context.Background())
		syncPoint := NewSyncPoint()

		// Cancel immediately
		cancel()

		err := syncPoint.WaitForReady(ctx)
		Expect(err).To(Equal(context.Canceled))
	})

	It("should work with multiple goroutines", func() {
		ctx := context.Background()
		syncPoint := NewSyncPoint()
		const numGoroutines = 5
		var counter atomic.Int32

		// Start multiple goroutines
		var wg sync.WaitGroup
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				err := syncPoint.WaitForReady(ctx)
				Expect(err).NotTo(HaveOccurred())
				counter.Add(1)
			}()
		}

		// All should be waiting
		time.Sleep(10 * time.Millisecond)
		Expect(counter.Load()).To(Equal(int32(0)))

		// Signal all to proceed
		<-syncPoint.Signal()
		syncPoint.Proceed()

		// Wait for completion
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		Eventually(done, 2*time.Second).Should(BeClosed())
		Expect(counter.Load()).To(Equal(int32(numGoroutines)))
	})
})

var _ = Describe("Barrier Pattern", func() {
	It("should synchronize N goroutines", func() {
		ctx := context.Background()
		const numGoroutines = 3
		barrier := NewBarrier(numGoroutines)
		var readyCount atomic.Int32
		var proceedCount atomic.Int32

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer GinkgoRecover()
				defer wg.Done()

				// Simulate setup work
				time.Sleep(time.Duration(id*10) * time.Millisecond)
				readyCount.Add(1)

				// Wait at barrier
				err := barrier.Wait(ctx)
				Expect(err).NotTo(HaveOccurred())

				// All should be ready now
				Expect(readyCount.Load()).To(Equal(int32(numGoroutines)))
				proceedCount.Add(1)
			}(i)
		}

		// Wait for completion
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		Eventually(done, 2*time.Second).Should(BeClosed())
		Expect(proceedCount.Load()).To(Equal(int32(numGoroutines)))
	})
})

var _ = Describe("EventuallyWithRetry", func() {
	It("should retry with exponential backoff", func() {
		attempts := 0
		start := time.Now()

		EventuallyWithRetry(func() error {
			attempts++
			if attempts < 3 {
				return errors.New("not ready")
			}
			return nil
		}, 5, 100*time.Millisecond).Should(Succeed())

		elapsed := time.Since(start)
		Expect(attempts).To(Equal(3))
		Expect(elapsed).To(BeNumerically(">=", 200*time.Millisecond))
	})

	It("should timeout after max attempts", func() {
		EventuallyWithRetry(func() error {
			return errors.New("always fails")
		}, 3, 100*time.Millisecond).Should(HaveOccurred())
	})
})

var _ = Describe("WaitForConditionWithDeadline", func() {
	It("should wait for condition to become true", func() {
		ctx := context.Background()
		var ready atomic.Bool

		// Set ready after 100ms
		go func() {
			time.Sleep(100 * time.Millisecond)
			ready.Store(true)
		}()

		err := WaitForConditionWithDeadline(
			ctx,
			func() bool { return ready.Load() },
			10*time.Millisecond,
			1*time.Second,
		)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should timeout if condition never true", func() {
		ctx := context.Background()

		err := WaitForConditionWithDeadline(
			ctx,
			func() bool { return false },
			10*time.Millisecond,
			100*time.Millisecond,
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("condition not met"))
	})

	It("should return immediately if condition already true", func() {
		ctx := context.Background()
		start := time.Now()

		err := WaitForConditionWithDeadline(
			ctx,
			func() bool { return true },
			10*time.Millisecond,
			1*time.Second,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(time.Since(start)).To(BeNumerically("<", 50*time.Millisecond))
	})
})

var _ = Describe("RetryWithBackoff", func() {
	It("should retry until success", func() {
		attempts := 0

		err := RetryWithBackoff(
			context.Background(),
			5,
			10*time.Millisecond,
			func() error {
				attempts++
				if attempts < 3 {
					return errors.New("transient error")
				}
				return nil
			},
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(attempts).To(Equal(3))
	})

	It("should return error after max attempts", func() {
		attempts := 0

		err := RetryWithBackoff(
			context.Background(),
			3,
			10*time.Millisecond,
			func() error {
				attempts++
				return errors.New("permanent error")
			},
		)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("operation failed after 3 attempts"))
		Expect(attempts).To(Equal(3))
	})

	It("should respect context cancellation", func() {
		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0

		// Cancel after first attempt
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		err := RetryWithBackoff(
			ctx,
			10,
			100*time.Millisecond,
			func() error {
				attempts++
				return errors.New("slow operation")
			},
		)

		Expect(err).To(Equal(context.Canceled))
		Expect(attempts).To(BeNumerically("<=", 2))
	})
})

var _ = Describe("ConcurrentExecutor", func() {
	It("should execute tasks concurrently with limit", func() {
		ctx := context.Background()
		executor := NewConcurrentExecutor(ctx, 3)

		var activeCount atomic.Int32
		var maxActive atomic.Int32
		var completed atomic.Int32

		// Submit 10 tasks
		for i := 0; i < 10; i++ {
			executor.Submit(func(ctx context.Context) error {
				// Track active count
				active := activeCount.Add(1)
				defer activeCount.Add(-1)

				// Update max active
				for {
					current := maxActive.Load()
					if active <= current || maxActive.CompareAndSwap(current, active) {
						break
					}
				}

				// Simulate work
				time.Sleep(10 * time.Millisecond)
				completed.Add(1)
				return nil
			})
		}

		errors := executor.Wait(5 * time.Second)
		Expect(errors).To(BeEmpty())
		Expect(completed.Load()).To(Equal(int32(10)))
		Expect(maxActive.Load()).To(BeNumerically("<=", 3))
	})

	It("should collect errors from failed tasks", func() {
		ctx := context.Background()
		executor := NewConcurrentExecutor(ctx, 2)

		// Submit tasks that fail
		for i := 0; i < 5; i++ {
			i := i
			executor.Submit(func(ctx context.Context) error {
				if i%2 == 0 {
					return errors.New("even task failed")
				}
				return nil
			})
		}

		errors := executor.Wait(2 * time.Second)
		Expect(errors).To(HaveLen(3)) // 0, 2, 4 failed
	})

	It("should timeout if tasks don't complete", func() {
		ctx := context.Background()
		executor := NewConcurrentExecutor(ctx, 1)

		// Submit slow task
		executor.Submit(func(ctx context.Context) error {
			time.Sleep(5 * time.Second)
			return nil
		})

		errors := executor.Wait(100 * time.Millisecond)
		Expect(errors).NotTo(BeEmpty())
		Expect(errors[0].Error()).To(ContainSubstring("timeout"))
	})
})

var _ = Describe("Race Condition Test Example", func() {
	// This demonstrates how to test concurrent status updates without flakiness
	It("should handle concurrent status updates with optimistic locking", func() {
		ctx := context.Background()
		syncPoint := NewSyncPoint()

		resource := &MockResource{Version: 1}
		var successCount atomic.Int32
		var conflictCount atomic.Int32

		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine 1: Try to update
		go func() {
			defer GinkgoRecover()
			defer wg.Done()

			Expect(syncPoint.WaitForReady(ctx)).To(Succeed())

			if resource.TryUpdate("status1") {
				successCount.Add(1)
			} else {
				conflictCount.Add(1)
			}
		}()

		// Goroutine 2: Try to update
		go func() {
			defer GinkgoRecover()
			defer wg.Done()

			Expect(syncPoint.WaitForReady(ctx)).To(Succeed())

			if resource.TryUpdate("status2") {
				successCount.Add(1)
			} else {
				conflictCount.Add(1)
			}
		}()

		// Signal both to proceed simultaneously
		<-syncPoint.Signal()
		syncPoint.Proceed()

		// Wait for completion
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		Eventually(done, 2*time.Second).Should(BeClosed())

		// Verify: exactly one succeeded, one conflicted
		Expect(successCount.Load()).To(Equal(int32(1)))
		Expect(conflictCount.Load()).To(Equal(int32(1)))
		Expect(resource.Status).To(Or(Equal("status1"), Equal("status2")))
	})
})

// MockResource simulates a Kubernetes resource with optimistic locking
type MockResource struct {
	mu      sync.Mutex
	Version int
	Status  string
}

func (m *MockResource) TryUpdate(status string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simulate optimistic locking: only first update succeeds
	if m.Status == "" {
		m.Status = status
		m.Version++
		return true
	}
	return false
}
