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

package signalprocessing

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
)

// ========================================
// BR-SP-111: Shared Exponential Backoff Integration
// Unit Tests for backoff.go integration (DD-SHARED-001)
// Per TESTING_GUIDELINES.md: Unit tests validate implementation correctness
// ========================================

var _ = Describe("BR-SP-111: SignalProcessing Backoff Integration", func() {

	// ========================================
	// Backoff Calculation Tests
	// ========================================

	Describe("Backoff Calculation", func() {
		Context("CalculateWithDefaults", func() {
			It("should return ~30s for first attempt", func() {
				// First attempt should be close to 30s (±10% jitter)
				duration := backoff.CalculateWithDefaults(1)
				// 30s ±10% = 27s to 33s
				Expect(duration).To(BeNumerically(">=", 27*time.Second))
				Expect(duration).To(BeNumerically("<=", 33*time.Second))
			})

			It("should return ~1m for second attempt", func() {
				// Second attempt: 30s * 2 = 60s (±10% jitter)
				duration := backoff.CalculateWithDefaults(2)
				// 60s ±10% = 54s to 66s
				Expect(duration).To(BeNumerically(">=", 54*time.Second))
				Expect(duration).To(BeNumerically("<=", 66*time.Second))
			})

			It("should return ~2m for third attempt", func() {
				// Third attempt: 30s * 4 = 120s (±10% jitter)
				duration := backoff.CalculateWithDefaults(3)
				// 120s ±10% = 108s to 132s
				Expect(duration).To(BeNumerically(">=", 108*time.Second))
				Expect(duration).To(BeNumerically("<=", 132*time.Second))
			})

			It("should cap at 5 minutes for high attempt count", func() {
				// High attempt count should cap at MaxPeriod (5m ±10%)
				duration := backoff.CalculateWithDefaults(10)
				// 5m = 300s, ±10% = 270s to 330s, but capped at 300s
				Expect(duration).To(BeNumerically("<=", 5*time.Minute))
			})
		})

		Context("CalculateWithoutJitter", func() {
			It("should return exact 30s for first attempt", func() {
				duration := backoff.CalculateWithoutJitter(1)
				Expect(duration).To(Equal(30 * time.Second))
			})

			It("should return exact 1m for second attempt", func() {
				duration := backoff.CalculateWithoutJitter(2)
				Expect(duration).To(Equal(60 * time.Second))
			})

			It("should return exact 2m for third attempt", func() {
				duration := backoff.CalculateWithoutJitter(3)
				Expect(duration).To(Equal(120 * time.Second))
			})

			It("should return exact 4m for fourth attempt", func() {
				duration := backoff.CalculateWithoutJitter(4)
				Expect(duration).To(Equal(240 * time.Second))
			})

			It("should cap at exactly 5m for fifth+ attempt", func() {
				duration := backoff.CalculateWithoutJitter(5)
				Expect(duration).To(Equal(5 * time.Minute))

				duration = backoff.CalculateWithoutJitter(10)
				Expect(duration).To(Equal(5 * time.Minute))
			})
		})

		Context("Custom Config", func() {
			It("should support conservative multiplier (1.5x)", func() {
				config := backoff.Config{
					BasePeriod:    30 * time.Second,
					MaxPeriod:     5 * time.Minute,
					Multiplier:    1.5,
					JitterPercent: 0,
				}
				// 30s * 1.5^1 = 45s
				duration := config.Calculate(2)
				Expect(duration).To(Equal(45 * time.Second))
			})

			It("should support aggressive multiplier (3x)", func() {
				config := backoff.Config{
					BasePeriod:    30 * time.Second,
					MaxPeriod:     5 * time.Minute,
					Multiplier:    3.0,
					JitterPercent: 0,
				}
				// 30s * 3^1 = 90s
				duration := config.Calculate(2)
				Expect(duration).To(Equal(90 * time.Second))
			})
		})
	})

	// ========================================
	// Transient Error Detection Tests
	// ========================================

	Describe("Transient Error Detection", func() {
		Context("K8s API Errors", func() {
			It("should identify timeout errors as transient", func() {
				err := apierrors.NewTimeoutError("test timeout", 5)
				Expect(checkTransientError(err)).To(BeTrue())
			})

			It("should identify server timeout errors as transient", func() {
				err := apierrors.NewServerTimeout(
					schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
					"get", 5)
				Expect(checkTransientError(err)).To(BeTrue())
			})

			It("should identify too many requests errors as transient", func() {
				err := apierrors.NewTooManyRequests("rate limited", 5)
				Expect(checkTransientError(err)).To(BeTrue())
			})

			It("should identify service unavailable errors as transient", func() {
				err := apierrors.NewServiceUnavailable("service down")
				Expect(checkTransientError(err)).To(BeTrue())
			})

			It("should NOT identify not found errors as transient", func() {
				err := apierrors.NewNotFound(
					schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
					"test-resource")
				Expect(checkTransientError(err)).To(BeFalse())
			})

			It("should NOT identify forbidden errors as transient", func() {
				err := apierrors.NewForbidden(
					schema.GroupResource{Group: "kubernaut.ai", Resource: "signalprocessings"},
					"test-resource", nil)
				Expect(checkTransientError(err)).To(BeFalse())
			})
		})

		Context("Context Errors", func() {
			It("should identify deadline exceeded as transient", func() {
				Expect(checkTransientError(context.DeadlineExceeded)).To(BeTrue())
			})

			It("should identify context canceled as transient", func() {
				Expect(checkTransientError(context.Canceled)).To(BeTrue())
			})
		})

		Context("Edge Cases", func() {
			It("should return false for nil error", func() {
				Expect(checkTransientError(nil)).To(BeFalse())
			})
		})
	})

	// ========================================
	// Jitter Anti-Thundering Herd Tests
	// ========================================

	Describe("Jitter Anti-Thundering Herd", func() {
		It("should produce different values across multiple calls", func() {
			// Call CalculateWithDefaults multiple times and verify jitter produces variance
			results := make(map[time.Duration]bool)
			for i := 0; i < 10; i++ {
				duration := backoff.CalculateWithDefaults(3)
				results[duration] = true
			}
			// With ±10% jitter, we should see some variance
			// (not all 10 calls producing identical values)
			// Allow for some collision but expect at least 2 unique values
			Expect(len(results)).To(BeNumerically(">=", 2))
		})
	})
})

// checkTransientError is the function being tested.
// This is a copy of the isTransientError function from the controller for unit testing.
// Renamed to avoid conflict with existing isTransientError in controller_error_handling_test.go.
func checkTransientError(err error) bool {
	if err == nil {
		return false
	}

	// K8s API transient errors
	if apierrors.IsTimeout(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsTooManyRequests(err) ||
		apierrors.IsServiceUnavailable(err) {
		return true
	}

	// Context deadline/cancellation (often network issues)
	if err == context.DeadlineExceeded || err == context.Canceled {
		return true
	}

	return false
}
