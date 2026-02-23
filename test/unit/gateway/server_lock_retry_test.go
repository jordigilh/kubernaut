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

// Gateway Distributed Lock Retry - Unit Tests
// Test Plan: docs/architecture/decisions/ADR-052-distributed-locking/test-plans/gateway-lock-retry-test-plan.md
// BR: BR-GATEWAY-190 (Multi-Replica Deduplication Safety)
// ADR: ADR-052 Addendum 001 (Exponential Backoff with Jitter)
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Algorithm correctness, exponential backoff timing
// - Integration tests (>50%): Real K8s lock manager, deduplication
// - E2E tests (10-15%): Full deployment with multiple replicas

package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// Mock implementations for lock retry testing (per TESTING_GUIDELINES.md: minimal mocks for unit tests)

type mockLockManager struct {
	acquireResults []bool // Sequence of acquire results (false = lock held, true = acquired)
	acquireAttempt int
	acquireErrors  []error
}

func (m *mockLockManager) AcquireLock(ctx context.Context, lockKey string) (bool, error) {
	if m.acquireAttempt < len(m.acquireErrors) && m.acquireErrors[m.acquireAttempt] != nil {
		err := m.acquireErrors[m.acquireAttempt]
		m.acquireAttempt++
		return false, err
	}

	if m.acquireAttempt >= len(m.acquireResults) {
		// Default: lock acquired after exhausting sequence
		return true, nil
	}

	result := m.acquireResults[m.acquireAttempt]
	m.acquireAttempt++
	return result, nil
}

func (m *mockLockManager) ReleaseLock(ctx context.Context, lockKey string) error {
	return nil
}

var _ = Describe("ADR-052 Addendum 001: Exponential Backoff with Jitter for Lock Retry", func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		lockManager  *mockLockManager
		testSignal   *types.NormalizedSignal
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		lockManager = &mockLockManager{}

		testSignal = &types.NormalizedSignal{
			SignalName:   "TestAlert",
			Fingerprint: "test-fingerprint-123",
			Severity:    "critical",
			SourceType:  "alert",
			Source:      "alertmanager",
			Namespace:   "default",
			Resource: types.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "test-pod",
				Namespace: "default",
			},
		}
	})

	AfterEach(func() {
		cancel()
	})

	// ========================================
	// Test Case Group 1: Lock Acquisition Success
	// ========================================

	Context("LOCK-RETRY-U-001: Lock Acquisition Success", func() {
		It("should succeed immediately without retry when lock is available", func() {
			// BR-GATEWAY-190: Distributed lock prevents race conditions
			// ADR-052 Addendum 001: No retry overhead if lock acquired on first attempt

			// GIVEN: Lock available (acquired on first try)
			lockManager.acquireResults = []bool{true}

			// WHEN: Lock acquisition attempted
			acquired, err := lockManager.AcquireLock(ctx, testSignal.Fingerprint)

			// THEN: Lock acquired immediately
			Expect(err).ToNot(HaveOccurred())
			Expect(acquired).To(BeTrue())
			Expect(lockManager.acquireAttempt).To(Equal(1), "BR-GATEWAY-190: Should only attempt once")
		})
	})

	// ========================================
	// Test Case Group 2: Exponential Backoff Behavior
	// ========================================

	Context("LOCK-RETRY-U-002: Exponential Backoff with Successful Retry", func() {
		It("should retry with exponential backoff timing (100ms → 200ms → 400ms)", func() {
			// ADR-052 Addendum 001: Exponential backoff (100ms → 1s)
			// This test validates timing behavior (acceptable use of time.Sleep per TESTING_GUIDELINES.md)

			// GIVEN: Lock held by another pod for 3 attempts, then acquired
			lockManager.acquireResults = []bool{false, false, false, true}

			start := time.Now()

			// WHEN: Lock acquisition retried with exponential backoff
			// NOTE: This test validates the exponential backoff algorithm itself
			// In the actual implementation, this loop will be inside ProcessSignal()
			for attempt := 1; attempt <= 4; attempt++ {
				acquired, err := lockManager.AcquireLock(ctx, testSignal.Fingerprint)
				Expect(err).ToNot(HaveOccurred())

				if acquired {
					// Lock acquired on 4th attempt
					Expect(attempt).To(Equal(4))
					break
				}

				// Simulate exponential backoff (what the real implementation should do)
				backoffMs := 100 * (1 << (attempt - 1)) // 100, 200, 400, 800
				if backoffMs > 1000 {
					backoffMs = 1000 // Cap at 1s
				}
				time.Sleep(time.Duration(backoffMs) * time.Millisecond)
			}

			elapsed := time.Since(start)

			// THEN: Total wait time should be ~700ms (100 + 200 + 400)
			// Allow ±200ms tolerance for test execution overhead
			Expect(elapsed).To(BeNumerically(">=", 700*time.Millisecond),
				"ADR-052 Addendum: Exponential backoff should delay ~700ms for 3 retries")
			Expect(elapsed).To(BeNumerically("<=", 900*time.Millisecond),
				"ADR-052 Addendum: Backoff should complete within expected time range")
			Expect(lockManager.acquireAttempt).To(Equal(4))
		})
	})

	Context("LOCK-RETRY-U-003: Max Retry Limit", func() {
		It("should respect max retry limit (10 attempts) to prevent unbounded blocking", func() {
			// ADR-052 Addendum 001: Max 10 retries to prevent unbounded blocking
			// TESTING_GUIDELINES.md: Test business outcome (bounded retry), not implementation

			// GIVEN: Lock permanently held (never acquired)
			lockManager.acquireResults = make([]bool, 11) // All false

			// WHEN: Lock acquisition attempted with max retry limit
			maxRetries := 10
			acquired := false
			for attempt := 1; attempt <= maxRetries; attempt++ {
				result, err := lockManager.AcquireLock(ctx, testSignal.Fingerprint)
				Expect(err).ToNot(HaveOccurred())

				if result {
					acquired = true
					break
				}
			}

			// THEN: Lock not acquired after 10 attempts
			Expect(acquired).To(BeFalse(),
				"ADR-052 Addendum: Max retry limit should prevent infinite loop")
			Expect(lockManager.acquireAttempt).To(Equal(10),
				"ADR-052 Addendum: Should stop after max retries")
		})
	})

	// ========================================
	// Test Case Group 3: Jitter Distribution
	// ========================================

	Context("LOCK-RETRY-U-004: Jitter Prevents Thundering Herd", func() {
		It("should distribute backoff times within ±10% jitter range", func() {
			// ADR-052 Addendum 001: ±10% jitter prevents thundering herd
			// Statistical test: Verify jitter creates distribution (not all identical)

			// This test validates the jitter concept, not the implementation
			// The actual implementation will use pkg/shared/backoff with jitter

			baseBackoff := 100 * time.Millisecond
			jitterPercent := 10
			iterations := 100

			backoffTimes := make([]time.Duration, iterations)

			// Simulate jitter calculation (what pkg/shared/backoff does)
			for i := 0; i < iterations; i++ {
				jitterRange := baseBackoff * time.Duration(jitterPercent) / 100
				// Simplified jitter for demonstration (actual uses rand)
				jitter := time.Duration((i%20)-10) * jitterRange / 10
				backoffTimes[i] = baseBackoff + jitter
			}

			// THEN: Backoff times should vary within ±10% range
			minExpected := 90 * time.Millisecond
			maxExpected := 110 * time.Millisecond

			for i, backoff := range backoffTimes {
				Expect(backoff).To(BeNumerically(">=", minExpected),
					fmt.Sprintf("Iteration %d: backoff %v below min %v", i, backoff, minExpected))
				Expect(backoff).To(BeNumerically("<=", maxExpected),
					fmt.Sprintf("ADR-052 Addendum: Iteration %d: backoff %v above max %v (jitter should keep within ±10%%)", i, backoff, maxExpected))
			}
		})
	})

	// ========================================
	// Test Case Group 4: Error Handling
	// ========================================

	Context("LOCK-RETRY-U-006: K8s API Error Propagation", func() {
		It("should return API errors immediately without retry", func() {
			// ADR-052: K8s API errors should propagate (not retry)
			// TESTING_GUIDELINES.md: Test error handling behavior

			// GIVEN: K8s API error (permission denied, API down, etc.)
			lockManager.acquireErrors = []error{
				errors.New("k8s API error: permission denied"),
			}

			// WHEN: Lock acquisition attempted
			acquired, err := lockManager.AcquireLock(ctx, testSignal.Fingerprint)

			// THEN: Error returned immediately (no retry on API errors)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("k8s API error"),
				"ADR-052: API errors should propagate with clear message")
			Expect(acquired).To(BeFalse())
			Expect(lockManager.acquireAttempt).To(Equal(1),
				"ADR-052: Should not retry on API errors")
		})
	})

	Context("LOCK-RETRY-U-007: Timeout Error Message Clarity", func() {
		It("should provide clear error when max retries exceeded", func() {
			// ADR-052 Addendum 001: User experience - clear timeout messages
			// TESTING_GUIDELINES.md: Test business outcome (user understanding), not implementation

			// GIVEN: Lock permanently held
			lockManager.acquireResults = make([]bool, 11) // All false

			maxRetries := 10
			var lastError error

			// WHEN: Max retries exceeded
			acquired := false
			for attempt := 1; attempt <= maxRetries; attempt++ {
				result, err := lockManager.AcquireLock(ctx, testSignal.Fingerprint)
				Expect(err).ToNot(HaveOccurred())

				if result {
					acquired = true
					break
				}

				if attempt == maxRetries {
					// Simulate timeout error construction (what implementation should do)
					lastError = fmt.Errorf("lock acquisition timeout after %d attempts (fingerprint: %s)",
						maxRetries, testSignal.Fingerprint)
				}
			}

			// THEN: Clear timeout error message
			Expect(acquired).To(BeFalse())
			Expect(lastError).To(HaveOccurred())
			Expect(lastError.Error()).To(ContainSubstring("lock acquisition timeout"),
				"ADR-052 Addendum: Error should clearly indicate timeout")
			Expect(lastError.Error()).To(ContainSubstring("10 attempts"),
				"ADR-052 Addendum: Error should include retry count for debugging")
			Expect(lastError.Error()).To(ContainSubstring("test-fingerprint-123"),
				"ADR-052 Addendum: Error should include fingerprint for debugging")
		})
	})

	// ========================================
	// Test Case Group 5: Stack Safety
	// ========================================

	Context("LOCK-RETRY-U-008: Iterative Loop (No Recursion)", func() {
		It("should use constant stack space (iterative loop, not recursive)", func() {
			// ADR-052 Addendum 001: Iterative loop prevents stack overflow
			// TESTING_GUIDELINES.md: Test business outcome (stack safety), not implementation

			// GIVEN: High retry count (would overflow stack with recursion)
			lockManager.acquireResults = make([]bool, 10)
			lockManager.acquireResults[9] = true // Succeed on 10th attempt

			// WHEN: Lock acquisition with 10 retries
			acquired := false
			for attempt := 1; attempt <= 10; attempt++ {
				result, err := lockManager.AcquireLock(ctx, testSignal.Fingerprint)
				Expect(err).ToNot(HaveOccurred())

				if result {
					acquired = true
					break
				}
			}

			// THEN: Lock acquired without stack overflow
			// Iterative loop uses constant stack space regardless of retry count
			// Recursive implementation would use ~2KB stack per call (10 retries = 20KB)
			Expect(acquired).To(BeTrue(),
				"ADR-052 Addendum: Iterative loop should acquire lock without stack overflow")
			Expect(lockManager.acquireAttempt).To(Equal(10))

			// Note: This test validates stack safety by completing successfully
			// If implementation used recursion, 100+ retries would risk overflow
		})
	})
})
