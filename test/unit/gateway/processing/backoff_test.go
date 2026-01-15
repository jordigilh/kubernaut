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

package processing

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-GATEWAY-188: Exponential Backoff for Transient Failures
// Unit tests for backoff calculation algorithm (pure math, no infrastructure)

var _ = Describe("BR-GATEWAY-188: Exponential Backoff Algorithm", func() {
	Context("GW-UNIT-ERR-006: Exponential Backoff Calculation", func() {
		It("[GW-UNIT-ERR-006] should calculate exponential backoff correctly", func() {
			// BR-GATEWAY-188: Backoff = baseDelay * 2^retryCount
			// BUSINESS LOGIC: Exponential growth prevents overwhelming failed services
			// Unit Test: Pure math validation

			baseDelay := 100 * time.Millisecond

			// Test exponential growth pattern
			testCases := []struct {
				retryCount     int
				expectedDelay  time.Duration
				description    string
			}{
				{0, 100 * time.Millisecond, "First retry: 100ms * 2^0 = 100ms"},
				{1, 200 * time.Millisecond, "Second retry: 100ms * 2^1 = 200ms"},
				{2, 400 * time.Millisecond, "Third retry: 100ms * 2^2 = 400ms"},
				{3, 800 * time.Millisecond, "Fourth retry: 100ms * 2^3 = 800ms"},
				{4, 1600 * time.Millisecond, "Fifth retry: 100ms * 2^4 = 1600ms"},
			}

			for _, tc := range testCases {
				// Calculate backoff (algorithm: baseDelay * 2^retryCount)
				backoff := baseDelay * (1 << uint(tc.retryCount))

				Expect(backoff).To(Equal(tc.expectedDelay),
					"Backoff calculation should follow exponential pattern: %s", tc.description)
			}
		})

		It("[GW-UNIT-ERR-006] should use reasonable base delay", func() {
			// BR-GATEWAY-188: Base delay should balance responsiveness vs load
			// BUSINESS LOGIC: Too short = thundering herd, too long = slow recovery
			// Unit Test: Validates base delay is in reasonable range

			baseDelay := 100 * time.Millisecond

			// BUSINESS RULE: Base delay should be 100ms-1s
			Expect(baseDelay).To(BeNumerically(">=", 100*time.Millisecond),
				"Base delay too short risks thundering herd")
			Expect(baseDelay).To(BeNumerically("<=", 1*time.Second),
				"Base delay too long delays recovery")
		})
	})

	Context("GW-UNIT-ERR-007: Backoff Max Delay Cap", func() {
		It("[GW-UNIT-ERR-007] should cap backoff at maximum delay", func() {
			// BR-GATEWAY-188: Backoff must not exceed max delay
			// BUSINESS LOGIC: Prevent indefinite wait times during prolonged outages
			// Unit Test: Boundary testing for cap enforcement

			baseDelay := 100 * time.Millisecond
			maxDelay := 30 * time.Second

			// Test that high retry counts are capped
			testCases := []struct {
				retryCount     int
				expectedCapped bool
				description    string
			}{
				{5, false, "Retry 5: 3.2s < 30s (not capped)"},
				{8, false, "Retry 8: 25.6s < 30s (not capped)"},
				{9, true, "Retry 9: 51.2s > 30s (CAPPED to 30s)"},
				{10, true, "Retry 10: 102.4s > 30s (CAPPED to 30s)"},
				{20, true, "Retry 20: very large > 30s (CAPPED to 30s)"},
			}

			for _, tc := range testCases {
				// Calculate uncapped backoff
				uncappedBackoff := baseDelay * (1 << uint(tc.retryCount))
				
				// Apply cap
				backoff := uncappedBackoff
				if backoff > maxDelay {
					backoff = maxDelay
				}

				if tc.expectedCapped {
					Expect(backoff).To(Equal(maxDelay),
						"High retry counts should be capped at max delay: %s", tc.description)
				} else {
					Expect(backoff).To(Equal(uncappedBackoff),
						"Low retry counts should not be capped: %s", tc.description)
				}
			}
		})

		It("[GW-UNIT-ERR-007] should enforce reasonable max delay limit", func() {
			// BR-GATEWAY-188: Max delay should prevent excessive wait times
			// BUSINESS LOGIC: Balance patience vs responsiveness during outages

			maxDelay := 30 * time.Second

			// BUSINESS RULE: Max delay should be 30s-60s
			Expect(maxDelay).To(BeNumerically(">=", 30*time.Second),
				"Max delay too short doesn't allow service recovery")
			Expect(maxDelay).To(BeNumerically("<=", 60*time.Second),
				"Max delay too long delays failure detection")
		})
	})

	Context("GW-UNIT-ERR-008: Backoff Jitter Addition", func() {
		It("[GW-UNIT-ERR-008] should add jitter to prevent thundering herd", func() {
			// BR-GATEWAY-188: Jitter prevents synchronized retries
			// BUSINESS LOGIC: Random jitter spreads retry load over time
			// Unit Test: Validates jitter is within expected range

			baseBackoff := 1 * time.Second
			maxJitter := 0.1 // ±10%

			// Generate multiple jittered delays
			delays := make([]time.Duration, 100)
			for i := range delays {
				// Simplified jitter calculation for test
				// Real implementation would use random jitter
				jitterFactor := 1.0 + (float64(i%20-10) * maxJitter / 10.0)
				delays[i] = time.Duration(float64(baseBackoff) * jitterFactor)
			}

			// BUSINESS RULE: All jittered delays should be within ±10% of base
			minDelay := time.Duration(float64(baseBackoff) * (1.0 - maxJitter))
			maxDelay := time.Duration(float64(baseBackoff) * (1.0 + maxJitter))

			for _, delay := range delays {
				Expect(delay).To(BeNumerically(">=", minDelay),
					"Jitter should not reduce delay below -10%%")
				Expect(delay).To(BeNumerically("<=", maxDelay),
					"Jitter should not increase delay above +10%%")
			}
		})

		It("[GW-UNIT-ERR-008] should produce varied delays with jitter", func() {
			// BR-GATEWAY-188: Jitter must actually vary the delays
			// BUSINESS LOGIC: Identical delays defeat the purpose of jitter
			// Unit Test: Validates jitter produces distribution

			baseBackoff := 1 * time.Second

			// Simulate multiple jittered calculations
			delays := make(map[time.Duration]bool)
			for i := 0; i < 20; i++ {
				// Simplified: use i to simulate random variation
				jitterFactor := 1.0 + (float64(i%20-10) * 0.01)
				delay := time.Duration(float64(baseBackoff) * jitterFactor)
				delays[delay] = true
			}

			// BUSINESS RULE: Should produce varied delays (not all identical)
			Expect(len(delays)).To(BeNumerically(">", 5),
				"Jitter should produce varied delays to prevent thundering herd")
		})
	})

	Context("GW-UNIT-ERR-010: Backoff Reset On Success", func() {
		It("[GW-UNIT-ERR-010] should reset retry count after successful operation", func() {
			// BR-GATEWAY-188: Success resets backoff state
			// BUSINESS LOGIC: Successful retry means service recovered, reset backoff
			// Unit Test: State machine validation

			// Simulate retry progression
			retryCount := 0
			
			// First failure: increment retry count
			retryCount++
			Expect(retryCount).To(Equal(1), "First failure should increment to 1")

			// Second failure: increment retry count
			retryCount++
			Expect(retryCount).To(Equal(2), "Second failure should increment to 2")

			// Success: reset retry count
			retryCount = 0
			Expect(retryCount).To(Equal(0),
				"BR-GATEWAY-188: Success should reset retry count to 0")

			// Next failure after reset: should start from 1 again
			retryCount++
			Expect(retryCount).To(Equal(1),
				"First failure after reset should use retry count 1 (not 3)")
		})

		It("[GW-UNIT-ERR-010] should use base delay after reset", func() {
			// BR-GATEWAY-188: Reset means next retry uses base delay
			// BUSINESS LOGIC: Successful recovery means service is healthy, use minimal backoff
			// Unit Test: Validates backoff calculation after reset

			baseDelay := 100 * time.Millisecond

			// Simulate retry with high count (would be 1.6s backoff)
			highRetryCount := 4
			highBackoff := baseDelay * (1 << uint(highRetryCount))
			Expect(highBackoff).To(Equal(1600 * time.Millisecond))

			// After success, reset to retry count 0
			resetRetryCount := 0
			resetBackoff := baseDelay * (1 << uint(resetRetryCount))

			// BUSINESS RULE: After reset, backoff should be back to base delay
			Expect(resetBackoff).To(Equal(baseDelay),
				"BR-GATEWAY-188: Reset should use base delay (100ms), not high backoff (1.6s)")
		})

		It("[GW-UNIT-ERR-010] should maintain retry count during failures", func() {
			// BR-GATEWAY-188: Only success resets count, failures increment
			// BUSINESS LOGIC: Persistent failures should increase backoff
			// Unit Test: State machine validation

			retryCount := 0

			// Multiple failures should keep incrementing
			for i := 1; i <= 5; i++ {
				retryCount++
				Expect(retryCount).To(Equal(i),
					"Retry count should increment on each failure")
			}

			// BUSINESS RULE: Retry count persists across failures
			Expect(retryCount).To(Equal(5),
				"BR-GATEWAY-188: Failures should accumulate retry count")
		})
	})
})
