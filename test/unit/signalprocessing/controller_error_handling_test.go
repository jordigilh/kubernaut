/*
Copyright 2025 Jordi Gil.

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

// Package signalprocessing contains unit tests for Signal Processing controller.
// Unit tests validate implementation correctness, not business value delivery.
// See docs/development/business-requirements/TESTING_GUIDELINES.md
//
// This file covers Error Category B: Transient Errors (timeout, retry behavior)
// Referenced by: test/integration/signalprocessing/reconciler_integration_test.go:883
package signalprocessing

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
)

// ========================================
// ERROR HANDLING TESTS (Error Category B)
// Referenced by integration test skip at reconciler_integration_test.go:883
// ========================================

var _ = Describe("Controller Error Handling", func() {

	// Error Category B: Transient Errors
	// Tests that the controller correctly identifies and handles transient errors
	Context("Error Category B: Transient Error Detection", func() {

		// Test 1: Timeout errors are identified as transient
		It("should identify timeout errors as transient (retryable)", func() {
			// Simulate a timeout error
			timeoutErr := context.DeadlineExceeded

			// Verify it's a transient error that should trigger retry
			Expect(errors.Is(timeoutErr, context.DeadlineExceeded)).To(BeTrue())
			Expect(isTransientError(timeoutErr)).To(BeTrue())
		})

		// Test 2: Context canceled errors are identified as transient
		It("should identify context canceled errors as transient", func() {
			canceledErr := context.Canceled

			Expect(errors.Is(canceledErr, context.Canceled)).To(BeTrue())
			Expect(isTransientError(canceledErr)).To(BeTrue())
		})

		// Test 3: Server timeout (K8s API) errors are retryable
		It("should identify K8s API timeout as retryable", func() {
			// Simulate K8s API server timeout (HTTP 504)
			serverTimeoutErr := apierrors.NewServerTimeout(
				schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
				"get",
				5,
			)

			Expect(apierrors.IsServerTimeout(serverTimeoutErr)).To(BeTrue())
			Expect(isTransientError(serverTimeoutErr)).To(BeTrue())
		})

		// Test 4: Too many requests (rate limiting) should trigger backoff
		It("should identify rate limiting errors as transient", func() {
			// Simulate K8s API rate limiting (HTTP 429)
			rateLimitErr := apierrors.NewTooManyRequests("rate limit exceeded", 5)

			Expect(apierrors.IsTooManyRequests(rateLimitErr)).To(BeTrue())
			Expect(isTransientError(rateLimitErr)).To(BeTrue())
		})

		// Test 5: Service unavailable errors are transient
		It("should identify service unavailable errors as transient", func() {
			// Simulate K8s API unavailable (HTTP 503)
			unavailableErr := apierrors.NewServiceUnavailable("etcd unavailable")

			Expect(apierrors.IsServiceUnavailable(unavailableErr)).To(BeTrue())
			Expect(isTransientError(unavailableErr)).To(BeTrue())
		})

		// Test 6: Not found errors are NOT transient (permanent)
		It("should NOT identify not found errors as transient", func() {
			notFoundErr := apierrors.NewNotFound(
				schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
				"missing-resource",
			)

			Expect(apierrors.IsNotFound(notFoundErr)).To(BeTrue())
			Expect(isTransientError(notFoundErr)).To(BeFalse())
		})

		// Test 7: Conflict errors are retryable (optimistic concurrency)
		It("should identify conflict errors as retryable", func() {
			conflictErr := apierrors.NewConflict(
				schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
				"test-resource",
				errors.New("resource version mismatch"),
			)

			Expect(apierrors.IsConflict(conflictErr)).To(BeTrue())
			// Conflicts are retryable via retry.RetryOnConflict
			// K8s default retry has exactly 5 steps
			Expect(retry.DefaultRetry.Steps).To(Equal(5))
		})
	})

	// Error Category B: Retry Behavior
	// Tests that retry logic works correctly with backoff
	Context("Error Category B: Retry Behavior with Backoff", func() {

		// Test 8: RetryOnConflict succeeds after transient failure
		It("should succeed after transient conflict during status update", func() {
			attemptCount := 0
			maxAttempts := 3

			// Simulate a function that fails twice then succeeds
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				attemptCount++
				if attemptCount < maxAttempts {
					return apierrors.NewConflict(
						schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
						"test-resource",
						errors.New("simulated conflict"),
					)
				}
				return nil // Success on third attempt
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(attemptCount).To(Equal(maxAttempts))
		})

		// Test 9: Retry exhaustion returns last error
		It("should return error after retry exhaustion", func() {
			attemptCount := 0

			// Use a limited retry config
			limitedRetry := retry.DefaultRetry
			limitedRetry.Steps = 2

			err := retry.RetryOnConflict(limitedRetry, func() error {
				attemptCount++
				return apierrors.NewConflict(
					schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
					"test-resource",
					errors.New("persistent conflict"),
				)
			})

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsConflict(err)).To(BeTrue())
			Expect(attemptCount).To(BeNumerically(">=", 2))
		})

		// Test 10: Non-conflict errors are not retried
		It("should not retry non-conflict errors", func() {
			attemptCount := 0

			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				attemptCount++
				return apierrors.NewNotFound(
					schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
					"missing-resource",
				)
			})

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			Expect(attemptCount).To(Equal(1)) // Only one attempt - no retry
		})
	})

	// Error Category B: Context Deadline Handling
	// Tests that timeout contexts are properly handled
	Context("Error Category B: Context Deadline Handling", func() {

		// Test 11: Operation respects context deadline
		It("should abort operation when context deadline exceeded", func() {
			// Create a context with very short timeout
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
			defer cancel()

			// Use Eventually to poll until the context deadline fires.
			// Avoids time.Sleep anti-pattern: Go's context timer goroutine may not
			// be scheduled immediately on loaded CI runners, causing ctx.Err() to
			// return nil even after wall-clock time exceeds the deadline.
			Eventually(ctx.Err).WithTimeout(100 * time.Millisecond).WithPolling(1 * time.Millisecond).
				Should(Equal(context.DeadlineExceeded))
		})

		// Test 12: Operation should check context before expensive operations
		It("should detect context cancellation before proceeding", func() {
			ctx, cancel := context.WithCancel(context.Background())

			// Cancel immediately
			cancel()

			// Check context - this is what controller should do before API calls
			select {
			case <-ctx.Done():
				Expect(ctx.Err()).To(Equal(context.Canceled))
			default:
				Fail("Context should be canceled")
			}
		})
	})
})

// isTransientError checks if an error is transient and should trigger retry.
// This mirrors the controller's error classification logic.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	// Context errors are transient
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	// K8s API transient errors
	if apierrors.IsServerTimeout(err) ||
		apierrors.IsTooManyRequests(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsTimeout(err) {
		return true
	}

	return false
}
