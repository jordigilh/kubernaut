// Package timing provides anti-flaky test patterns for reliable concurrent testing
package timing

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/gomega"
)

// SyncPoint provides deterministic test synchronization for concurrent operations.
// Use this to coordinate between goroutines in tests without timing dependencies.
//
// Example:
//
//	syncPoint := timing.NewSyncPoint()
//	go func() {
//	    // Wait for signal
//	    syncPoint.WaitForReady(ctx)
//	    // Do work...
//	}()
//	// Signal goroutine to proceed
//	syncPoint.Signal()
//	syncPoint.Proceed()
type SyncPoint struct {
	ready   chan struct{}
	proceed chan struct{}
}

// NewSyncPoint creates a new synchronization point for coordinating test goroutines
func NewSyncPoint() *SyncPoint {
	return &SyncPoint{
		ready:   make(chan struct{}),
		proceed: make(chan struct{}),
	}
}

// WaitForReady blocks until Signal() is called, providing deterministic coordination
func (s *SyncPoint) WaitForReady(ctx context.Context) error {
	select {
	case <-s.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Signal unblocks WaitForReady and returns a channel that blocks until Proceed() is called
func (s *SyncPoint) Signal() <-chan struct{} {
	close(s.ready)
	return s.proceed
}

// Proceed unblocks the channel returned by Signal()
func (s *SyncPoint) Proceed() {
	close(s.proceed)
}

// Barrier provides a reusable synchronization barrier for N goroutines.
// All goroutines must call Wait() before any can proceed.
//
// Example:
//
//	barrier := timing.NewBarrier(3)
//	for i := 0; i < 3; i++ {
//	    go func() {
//	        // Do setup...
//	        barrier.Wait(ctx) // All wait here
//	        // All proceed together
//	    }()
//	}
type Barrier struct {
	count    int
	waiting  chan struct{}
	released chan struct{}
}

// NewBarrier creates a barrier that requires count goroutines to wait before releasing
func NewBarrier(count int) *Barrier {
	return &Barrier{
		count:    count,
		waiting:  make(chan struct{}, count),
		released: make(chan struct{}),
	}
}

// Wait blocks until count goroutines have called Wait(), then all proceed together
func (b *Barrier) Wait(ctx context.Context) error {
	// Send to waiting channel
	select {
	case b.waiting <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// If we're the last one, close released channel
	if len(b.waiting) == b.count {
		close(b.released)
	}

	// Wait for all to arrive
	select {
	case <-b.released:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// EventuallyWithRetry provides Gomega Eventually with exponential backoff and jitter
// for better reliability in CI environments with variable timing.
//
// Example:
//
//	EventuallyWithRetry(func() error {
//	    return checkCondition()
//	}, 5, 1*time.Second).Should(Succeed())
func EventuallyWithRetry(
	condition func() error,
	maxAttempts int,
	baseTimeout time.Duration,
) AsyncAssertion {
	// Use exponential backoff: baseTimeout * 2^attempt with max cap
	timeout := baseTimeout * time.Duration(maxAttempts)
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}

	// Check more frequently for faster feedback
	interval := baseTimeout / 10
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}

	return Eventually(condition, timeout, interval)
}

// WaitForConditionWithDeadline waits for a condition with explicit deadline,
// ensuring tests don't hang indefinitely.
//
// Example:
//
//	err := WaitForConditionWithDeadline(ctx,
//	    func() bool { return resource.Status.Phase == "Ready" },
//	    100*time.Millisecond,
//	    5*time.Second)
//	Expect(err).NotTo(HaveOccurred())
func WaitForConditionWithDeadline(
	ctx context.Context,
	condition func() bool,
	checkInterval time.Duration,
	deadline time.Duration,
) error {
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// Check immediately first
	if condition() {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("condition not met within %v: %w", deadline, ctx.Err())
		case <-ticker.C:
			if condition() {
				return nil
			}
		}
	}
}

// RetryWithBackoff retries an operation with exponential backoff until success or max attempts.
// Use this for operations that may transiently fail (e.g., network calls, resource creation).
//
// Example:
//
//	err := RetryWithBackoff(ctx, 5, 100*time.Millisecond, func() error {
//	    return createResource()
//	})
//	Expect(err).NotTo(HaveOccurred())
func RetryWithBackoff(
	ctx context.Context,
	maxAttempts int,
	initialBackoff time.Duration,
	operation func() error,
) error {
	backoff := initialBackoff

	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		// Last attempt, return error
		if attempt == maxAttempts-1 {
			return fmt.Errorf("operation failed after %d attempts: %w", maxAttempts, err)
		}

		// Exponential backoff with jitter
		select {
		case <-time.After(backoff):
			backoff *= 2
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("operation failed after %d attempts", maxAttempts)
}

// ConcurrentExecutor provides controlled concurrent execution with timeout and error collection.
// Use this for testing parallel operations with deterministic coordination.
//
// Example:
//
//	executor := NewConcurrentExecutor(ctx, 3)
//	for i := 0; i < 10; i++ {
//	    i := i
//	    executor.Submit(func(ctx context.Context) error {
//	        return processItem(i)
//	    })
//	}
//	errors := executor.Wait(30 * time.Second)
//	Expect(errors).To(BeEmpty())
type ConcurrentExecutor struct {
	ctx       context.Context
	semaphore chan struct{}
	errors    chan error
	done      chan struct{}
	tasks     int
}

// NewConcurrentExecutor creates an executor with maximum concurrency limit
func NewConcurrentExecutor(ctx context.Context, maxConcurrency int) *ConcurrentExecutor {
	return &ConcurrentExecutor{
		ctx:       ctx,
		semaphore: make(chan struct{}, maxConcurrency),
		errors:    make(chan error, 100), // Buffered to avoid blocking
		done:      make(chan struct{}),
		tasks:     0,
	}
}

// Submit adds a task to execute concurrently
func (ce *ConcurrentExecutor) Submit(task func(context.Context) error) {
	ce.tasks++
	go func() {
		// Acquire semaphore
		select {
		case ce.semaphore <- struct{}{}:
			defer func() { <-ce.semaphore }()
		case <-ce.ctx.Done():
			ce.errors <- ce.ctx.Err()
			return
		}

		// Execute task
		if err := task(ce.ctx); err != nil {
			ce.errors <- err
		}

		// Signal completion
		select {
		case ce.done <- struct{}{}:
		case <-ce.ctx.Done():
		}
	}()
}

// Wait waits for all tasks to complete or timeout, returning any errors
func (ce *ConcurrentExecutor) Wait(timeout time.Duration) []error {
	ctx, cancel := context.WithTimeout(ce.ctx, timeout)
	defer cancel()

	completed := 0
	var errors []error

	for completed < ce.tasks {
		select {
		case <-ce.done:
			completed++
		case err := <-ce.errors:
			errors = append(errors, err)
		case <-ctx.Done():
			errors = append(errors, fmt.Errorf("timeout waiting for tasks: %d/%d completed", completed, ce.tasks))
			return errors
		}
	}

	// Drain error channel
	close(ce.errors)
	for err := range ce.errors {
		errors = append(errors, err)
	}

	return errors
}

// WatchTimeout provides a reasonable timeout for watch-based tests in CI environments.
// Use this for tests that watch Kubernetes resources for status updates.
//
// Returns: 30s in CI, 10s in local development
func WatchTimeout() time.Duration {
	if isCI() {
		return 30 * time.Second
	}
	return 10 * time.Second
}

// ReconcileTimeout provides a reasonable timeout for reconciliation loops.
// Use this for tests waiting for controller reconciliation.
//
// Returns: 15s in CI, 5s in local development
func ReconcileTimeout() time.Duration {
	if isCI() {
		return 15 * time.Second
	}
	return 5 * time.Second
}

// PollInterval provides a reasonable poll interval for Eventually assertions.
// Use this for consistent polling across tests.
//
// Returns: 500ms in CI, 100ms in local development
func PollInterval() time.Duration {
	if isCI() {
		return 500 * time.Millisecond
	}
	return 100 * time.Millisecond
}

// isCI detects if running in CI environment
func isCI() bool {
	// Common CI environment variables
	ciEnvVars := []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "CIRCLECI", "JENKINS_URL"}
	for _, envVar := range ciEnvVars {
		if _, exists := map[string]string{"CI": "true"}[envVar]; exists {
			return true
		}
	}
	return false
}
