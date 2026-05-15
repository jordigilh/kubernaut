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

package backoff_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
)

func TestBackoff(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shared Backoff Utility Suite")
}

// Test Tier: UNIT ONLY
// Rationale: Pure computational utility with zero external dependencies
// Integration/E2E: Covered by services using this utility (WE, NT)
//
// Business Requirements Enabled (not tested directly by this utility):
// - BR-WE-012: WorkflowExecution - Pre-execution Failure Backoff
// - BR-NOT-052: Notification - Automatic Retry with Custom Retry Policies
// - BR-NOT-055: Notification - Graceful Degradation (jitter for anti-thundering herd)
//
// Per TESTING_GUIDELINES.md: Test function behavior, edge cases, internal logic
var _ = Describe("Backoff Utility", func() {
	// =================================================================
	// Scenario 1: Standard Exponential Strategy (Multiplier=2)
	// =================================================================
	Describe("Calculate - Standard Exponential (multiplier=2)", func() {
		It("should calculate correct backoff progression without jitter", func() {
			config := backoff.Config{
				BasePeriod:    30 * time.Second,
				MaxPeriod:     480 * time.Second,
				Multiplier:    2.0,
				JitterPercent: 0, // Deterministic for testing
			}

			// Test progression: 30s → 1m → 2m → 4m → 8m (capped at 480s)
			Expect(config.Calculate(1)).To(Equal(30 * time.Second))   // 30 * 2^0
			Expect(config.Calculate(2)).To(Equal(60 * time.Second))   // 30 * 2^1
			Expect(config.Calculate(3)).To(Equal(120 * time.Second))  // 30 * 2^2
			Expect(config.Calculate(4)).To(Equal(240 * time.Second))  // 30 * 2^3
			Expect(config.Calculate(5)).To(Equal(480 * time.Second))  // 30 * 2^4 = 480 (capped)
			Expect(config.Calculate(6)).To(Equal(480 * time.Second))  // Would be 960s, capped at 480s
			Expect(config.Calculate(10)).To(Equal(480 * time.Second)) // Would be 15360s, capped at 480s
		})
	})

	// =================================================================
	// Scenario 2: Conservative Strategy (Multiplier=1.5)
	// =================================================================
	Describe("Calculate - Conservative Strategy (multiplier=1.5)", func() {
		It("should calculate slower growth progression", func() {
			config := backoff.Config{
				BasePeriod:    10 * time.Second,
				MaxPeriod:     120 * time.Second,
				Multiplier:    1.5,
				JitterPercent: 0,
			}

			// Progression: 10s → 15s → 22s → 33s → 50s → 76s → 114s → 120s (capped)
			Expect(config.Calculate(1)).To(Equal(10 * time.Second))                            // 10 * 1.5^0
			Expect(config.Calculate(2)).To(Equal(15 * time.Second))                            // 10 * 1.5^1
			Expect(config.Calculate(3)).To(BeNumerically("~", 22*time.Second, 1*time.Second))  // 10 * 1.5^2 = 22.5s
			Expect(config.Calculate(4)).To(BeNumerically("~", 33*time.Second, 1*time.Second))  // 10 * 1.5^3 = 33.75s
			Expect(config.Calculate(5)).To(BeNumerically("~", 50*time.Second, 1*time.Second))  // 10 * 1.5^4 = 50.625s
			Expect(config.Calculate(6)).To(BeNumerically("~", 76*time.Second, 1*time.Second))  // 10 * 1.5^5 = 75.9375s
			Expect(config.Calculate(7)).To(BeNumerically("~", 114*time.Second, 2*time.Second)) // 10 * 1.5^6 = 113.9s
			Expect(config.Calculate(8)).To(Equal(120 * time.Second))                           // Would be 170.9s, capped at 120s
		})

		It("should match Notification's transient error pattern", func() {
			// NT production use case: Transient Slack API errors
			config := backoff.Config{
				BasePeriod:    10 * time.Second,
				MaxPeriod:     120 * time.Second,
				Multiplier:    1.5,
				JitterPercent: 0, // Remove jitter for deterministic test
			}

			// Validate progression matches NT's expectations
			firstAttempt := config.Calculate(1)
			secondAttempt := config.Calculate(2)
			Expect(secondAttempt).To(Equal(firstAttempt * 3 / 2)) // 1.5x growth
		})
	})

	// =================================================================
	// Scenario 3: Aggressive Strategy (Multiplier=3)
	// =================================================================
	Describe("Calculate - Aggressive Strategy (multiplier=3)", func() {
		It("should calculate faster growth progression", func() {
			config := backoff.Config{
				BasePeriod:    30 * time.Second,
				MaxPeriod:     300 * time.Second,
				Multiplier:    3.0,
				JitterPercent: 0,
			}

			// Progression: 30s → 90s → 270s → 300s (capped)
			Expect(config.Calculate(1)).To(Equal(30 * time.Second))  // 30 * 3^0
			Expect(config.Calculate(2)).To(Equal(90 * time.Second))  // 30 * 3^1
			Expect(config.Calculate(3)).To(Equal(270 * time.Second)) // 30 * 3^2
			Expect(config.Calculate(4)).To(Equal(300 * time.Second)) // 30 * 3^3 = 810s, capped at 300s
			Expect(config.Calculate(5)).To(Equal(300 * time.Second)) // Capped
		})

		It("should match Notification's critical alert pattern", func() {
			// NT production use case: Critical alerts need fast retry progression
			config := backoff.Config{
				BasePeriod:    30 * time.Second,
				MaxPeriod:     300 * time.Second,
				Multiplier:    3.0,
				JitterPercent: 0,
			}

			// After 3 attempts, should be at or near max backoff
			thirdAttempt := config.Calculate(3)
			Expect(thirdAttempt).To(BeNumerically(">=", 270*time.Second))
			Expect(thirdAttempt).To(BeNumerically("<=", 300*time.Second))
		})
	})

	// =================================================================
	// Scenario 4: Jitter Distribution (Statistical)
	// =================================================================
	Describe("Calculate - Jitter Distribution", func() {
		It("should add jitter within expected range (±10%)", func() {
			config := backoff.Config{
				BasePeriod:    30 * time.Second,
				Multiplier:    2.0,
				JitterPercent: 10, // ±10%
			}

			// Run 100 times to verify statistical distribution
			// Per TESTING_GUIDELINES.md line 609: This tests timing behavior itself
			for i := 0; i < 100; i++ {
				duration := config.Calculate(1)
				// Should be 30s ±10% = [27s, 33s]
				Expect(duration).To(BeNumerically(">=", 27*time.Second),
					"Jitter should not reduce duration below -10%%")
				Expect(duration).To(BeNumerically("<=", 33*time.Second),
					"Jitter should not increase duration above +10%%")
			}
		})

		It("should add jitter within expected range (±20%)", func() {
			config := backoff.Config{
				BasePeriod:    60 * time.Second,
				Multiplier:    2.0,
				JitterPercent: 20, // ±20% (aggressive anti-thundering herd)
			}

			for i := 0; i < 100; i++ {
				duration := config.Calculate(1)
				// Should be 60s ±20% = [48s, 72s]
				Expect(duration).To(BeNumerically(">=", 48*time.Second))
				Expect(duration).To(BeNumerically("<=", 72*time.Second))
			}
		})

		It("should clamp jitter to stay within bounds [BasePeriod, MaxPeriod]", func() {
			config := backoff.Config{
				BasePeriod:    30 * time.Second,
				MaxPeriod:     60 * time.Second,
				Multiplier:    2.0,
				JitterPercent: 50, // ±50% (extreme, for testing bounds)
			}

			// Even with large jitter, should never violate bounds
			for i := 0; i < 100; i++ {
				duration := config.Calculate(1)
				Expect(duration).To(BeNumerically(">=", 30*time.Second),
					"Jitter should not reduce below BasePeriod")
				Expect(duration).To(BeNumerically("<=", 60*time.Second),
					"Jitter should not exceed MaxPeriod")
			}
		})

		It("should distribute jitter around base duration", func() {
			config := backoff.Config{
				BasePeriod:    30 * time.Second,
				Multiplier:    2.0,
				JitterPercent: 10,
			}

			// Collect 100 samples (smaller sample for faster test)
			samples := make([]float64, 100)
			for i := 0; i < 100; i++ {
				duration := config.Calculate(1)
				samples[i] = duration.Seconds()
			}

			// Calculate mean (should be close to 30s)
			sum := 0.0
			for _, s := range samples {
				sum += s
			}
			mean := sum / float64(len(samples))

			// Mean should be reasonably close to target
			// With small samples, allow ±5% tolerance
			Expect(mean).To(BeNumerically("~", 30.0, 1.5), // ±5% of 30s
				"Jitter mean should be roughly centered around base duration")

			// Verify samples are within bounds [27s, 33s]
			// This is the critical test - jitter must not violate bounds
			for _, s := range samples {
				Expect(s).To(BeNumerically(">=", 27.0),
					"All samples should be >= 27s (30s - 10%%)")
				Expect(s).To(BeNumerically("<=", 33.0),
					"All samples should be <= 33s (30s + 10%%)")
			}
		})
	})

	// =================================================================
	// Scenario 5: Edge Cases
	// =================================================================
	Describe("Calculate - Edge Cases", func() {
		// Deterministic boundary conditions: config → attempt → expected duration.
		// All entries use JitterPercent=0 for deterministic assertions.
		// DescribeTable consolidates 12 identical-structure edge-case tests.
		DescribeTable("deterministic boundary conditions",
			func(config backoff.Config, attempt int, expected time.Duration) {
				Expect(config.Calculate(int32(attempt))).To(Equal(expected))
			},
			// Zero/negative attempts → always returns BasePeriod
			Entry("zero attempts → BasePeriod",
				backoff.Config{BasePeriod: 30 * time.Second, Multiplier: 2.0},
				0, 30*time.Second),
			Entry("negative attempt (-1) → BasePeriod",
				backoff.Config{BasePeriod: 30 * time.Second, Multiplier: 2.0},
				-1, 30*time.Second),
			Entry("negative attempt (-100) → BasePeriod",
				backoff.Config{BasePeriod: 30 * time.Second, Multiplier: 2.0},
				-100, 30*time.Second),

			// Zero base period → zero duration regardless of attempt
			Entry("zero base period, attempt 1 → zero",
				backoff.Config{BasePeriod: 0, Multiplier: 2.0},
				1, time.Duration(0)),
			Entry("zero base period, attempt 5 → zero",
				backoff.Config{BasePeriod: 0, Multiplier: 2.0},
				5, time.Duration(0)),

			// Zero multiplier → defaults to standard (2.0)
			Entry("zero multiplier, attempt 1 → 30s (defaults to 2.0)",
				backoff.Config{BasePeriod: 30 * time.Second, MaxPeriod: 300 * time.Second, Multiplier: 0},
				1, 30*time.Second),
			Entry("zero multiplier, attempt 2 → 60s (defaults to 2.0)",
				backoff.Config{BasePeriod: 30 * time.Second, MaxPeriod: 300 * time.Second, Multiplier: 0},
				2, 60*time.Second),
			Entry("zero multiplier, attempt 3 → 120s (defaults to 2.0)",
				backoff.Config{BasePeriod: 30 * time.Second, MaxPeriod: 300 * time.Second, Multiplier: 0},
				3, 120*time.Second),

			// Very high multiplier → capped at MaxPeriod without overflow
			Entry("multiplier=10, attempt 1 → 30s (below cap)",
				backoff.Config{BasePeriod: 30 * time.Second, MaxPeriod: 300 * time.Second, Multiplier: 10.0},
				1, 30*time.Second),
			Entry("multiplier=10, attempt 2 → 300s (capped at MaxPeriod)",
				backoff.Config{BasePeriod: 30 * time.Second, MaxPeriod: 300 * time.Second, Multiplier: 10.0},
				2, 300*time.Second),
			Entry("multiplier=10, attempt 3 → 300s (remains capped)",
				backoff.Config{BasePeriod: 30 * time.Second, MaxPeriod: 300 * time.Second, Multiplier: 10.0},
				3, 300*time.Second),
			Entry("multiplier=10, attempt 100 → 300s (no overflow)",
				backoff.Config{BasePeriod: 30 * time.Second, MaxPeriod: 300 * time.Second, Multiplier: 10.0},
				100, 300*time.Second),

			// No max period → unlimited exponential growth
			Entry("no max period, attempt 10 → 30s × 2^9 = 15360s",
				backoff.Config{BasePeriod: 30 * time.Second, MaxPeriod: 0, Multiplier: 2.0},
				10, 30*512*time.Second),
		)

		// Jitter-at-bounds tests require statistical loops — not table-driven.
		Context("with jitter at bounds", func() {
			It("should never reduce below BasePeriod even with jitter", func() {
				config := backoff.Config{
					BasePeriod:    30 * time.Second,
					MaxPeriod:     100 * time.Second,
					Multiplier:    2.0,
					JitterPercent: 50, // Extreme jitter
				}

				for i := 0; i < 100; i++ {
					duration := config.Calculate(1)
					Expect(duration).To(BeNumerically(">=", 30*time.Second))
				}
			})

			It("should never exceed MaxPeriod even with jitter", func() {
				config := backoff.Config{
					BasePeriod:    30 * time.Second,
					MaxPeriod:     100 * time.Second,
					Multiplier:    2.0,
					JitterPercent: 50, // Extreme jitter
				}

				for i := 0; i < 100; i++ {
					duration := config.Calculate(5) // High enough to hit MaxPeriod
					Expect(duration).To(BeNumerically("<=", 100*time.Second))
				}
			})
		})
	})

	// =================================================================
	// Convenience Functions
	// =================================================================
	Describe("CalculateWithDefaults", func() {
		It("should provide sensible default progression with jitter", func() {
			// Default: 30s → 1m → 2m → 4m → 5m (with ±10% jitter)
			// Test multiple times due to jitter
			for i := 0; i < 10; i++ {
				attempt1 := backoff.CalculateWithDefaults(1)
				Expect(attempt1).To(BeNumerically("~", 30*time.Second, 3*time.Second)) // 30s ±10%

				attempt2 := backoff.CalculateWithDefaults(2)
				Expect(attempt2).To(BeNumerically("~", 60*time.Second, 6*time.Second)) // 1m ±10%

				attempt5 := backoff.CalculateWithDefaults(5)
				Expect(attempt5).To(BeNumerically("~", 300*time.Second, 30*time.Second)) // 5m ±10% (capped)
			}
		})

		It("should use jitter by default (production-ready)", func() {
			// Call multiple times, should get different results due to jitter
			results := make(map[time.Duration]bool)
			for i := 0; i < 20; i++ {
				duration := backoff.CalculateWithDefaults(1)
				results[duration] = true
			}

			// Should have multiple different values (not deterministic)
			// With ±10% jitter on 30s, expect at least 3-4 different values in 20 runs
			Expect(len(results)).To(BeNumerically(">=", 3),
				"CalculateWithDefaults should include jitter by default")
		})
	})

	Describe("CalculateWithoutJitter", func() {
		It("should provide exact progression without jitter", func() {
			// Exact: 30s → 1m → 2m → 4m → 5m (no jitter)
			Expect(backoff.CalculateWithoutJitter(1)).To(Equal(30 * time.Second))
			Expect(backoff.CalculateWithoutJitter(2)).To(Equal(60 * time.Second))
			Expect(backoff.CalculateWithoutJitter(3)).To(Equal(120 * time.Second))
			Expect(backoff.CalculateWithoutJitter(4)).To(Equal(240 * time.Second))
			Expect(backoff.CalculateWithoutJitter(5)).To(Equal(300 * time.Second)) // Capped at 5m
		})

		It("should be deterministic (no jitter)", func() {
			// Call multiple times, should always get same result
			first := backoff.CalculateWithoutJitter(1)
			for i := 0; i < 20; i++ {
				Expect(backoff.CalculateWithoutJitter(1)).To(Equal(first))
			}
		})
	})

	// =================================================================
	// BR-WE-012: WorkflowExecution Exponential Backoff Configuration
	// =================================================================
	Describe("Calculate - WorkflowExecution Configuration (BR-WE-012)", func() {
		It("should calculate correct backoff sequence for WE pre-execution failures", func() {
			// BR-WE-012 configuration:
			// - BasePeriod: 1 minute
			// - MaxPeriod: 10 minutes
			// - Multiplier: 2.0 (power-of-2 exponential)
			// - JitterPercent: 10 (±10% variance)
			config := backoff.Config{
				BasePeriod:    1 * time.Minute,
				MaxPeriod:     10 * time.Minute,
				Multiplier:    2.0,
				JitterPercent: 0, // No jitter for deterministic testing
			}

			// Test progression: 1m → 2m → 4m → 8m → 10m (capped)
			Expect(config.Calculate(1)).To(Equal(1 * time.Minute))   // 1min * 2^0
			Expect(config.Calculate(2)).To(Equal(2 * time.Minute))   // 1min * 2^1
			Expect(config.Calculate(3)).To(Equal(4 * time.Minute))   // 1min * 2^2
			Expect(config.Calculate(4)).To(Equal(8 * time.Minute))   // 1min * 2^3
			Expect(config.Calculate(5)).To(Equal(10 * time.Minute))  // 1min * 2^4 = 16m, capped at 10m
			Expect(config.Calculate(6)).To(Equal(10 * time.Minute))  // Stays at cap
			Expect(config.Calculate(10)).To(Equal(10 * time.Minute)) // Stays at cap
		})

		It("should apply ±10% jitter for WE production configuration", func() {
			config := backoff.Config{
				BasePeriod:    1 * time.Minute,
				MaxPeriod:     10 * time.Minute,
				Multiplier:    2.0,
				JitterPercent: 10,
			}

			// Run 50 iterations to validate jitter distribution for first failure
			for i := 0; i < 50; i++ {
				duration := config.Calculate(1)
				// Should be 1min ±10% = 54s-66s
				Expect(duration).To(BeNumerically(">=", 54*time.Second))
				Expect(duration).To(BeNumerically("<=", 66*time.Second))
			}

			// Validate jitter at 5th failure (10 minutes base, but capped)
			for i := 0; i < 50; i++ {
				duration := config.Calculate(5)
				// Should be 10min ±10% = 9min-11min, but capped at 10min
				// So actual range is 9min-10min (cannot exceed cap)
				Expect(duration).To(BeNumerically(">=", 9*time.Minute))
				Expect(duration).To(BeNumerically("<=", 10*time.Minute))
			}
		})

		It("should match BR-WE-012 acceptance criteria for backoff escalation", func() {
			config := backoff.Config{
				BasePeriod:    1 * time.Minute,
				MaxPeriod:     10 * time.Minute,
				Multiplier:    2.0,
				JitterPercent: 0,
			}

			// BR-WE-012 Acceptance Criteria:
			// - First pre-execution failure triggers 1-minute cooldown
			firstFailure := config.Calculate(1)
			Expect(firstFailure).To(Equal(1*time.Minute),
				"BR-WE-012: First pre-execution failure should trigger 1-minute cooldown")

			// - Consecutive pre-execution failures double cooldown (capped at 10 min)
			secondFailure := config.Calculate(2)
			Expect(secondFailure).To(Equal(2*time.Minute),
				"BR-WE-012: Second failure should double to 2 minutes")

			thirdFailure := config.Calculate(3)
			Expect(thirdFailure).To(Equal(4*time.Minute),
				"BR-WE-012: Third failure should double to 4 minutes")

			fourthFailure := config.Calculate(4)
			Expect(fourthFailure).To(Equal(8*time.Minute),
				"BR-WE-012: Fourth failure should double to 8 minutes")

			fifthFailure := config.Calculate(5)
			Expect(fifthFailure).To(Equal(10*time.Minute),
				"BR-WE-012: Fifth failure should be capped at 10 minutes")

			// - After 5 consecutive pre-execution failures, WFE marked Skipped with ExhaustedRetries
			// (This is tested in integration tests - backoff calculation just provides the duration)
		})

		It("should prevent remediation storms with exponential backoff", func() {
			config := backoff.Config{
				BasePeriod:    1 * time.Minute,
				MaxPeriod:     10 * time.Minute,
				Multiplier:    2.0,
				JitterPercent: 10,
			}

			// Simulate 100 WorkflowExecutions all failing at the same time (thundering herd)
			// With jitter, they should be distributed over time

			firstAttemptResults := make(map[time.Duration]int)
			for i := 0; i < 100; i++ {
				duration := config.Calculate(1)
				firstAttemptResults[duration]++
			}

			// With ±10% jitter on 1 minute:
			// - Range: 54s-66s (12 second span)
			// - 100 executions distributed across this range
			// - Should have multiple different values (not all at exactly 1 minute)
			Expect(len(firstAttemptResults)).To(BeNumerically(">", 5),
				"Jitter should distribute retry attempts to prevent thundering herd")

			// Validate all results are within bounds
			for duration := range firstAttemptResults {
				Expect(duration).To(BeNumerically(">=", 54*time.Second))
				Expect(duration).To(BeNumerically("<=", 66*time.Second))
			}
		})
	})

})
