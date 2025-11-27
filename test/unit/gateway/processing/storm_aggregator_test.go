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

package processing_test

import (
	"context"
	"fmt"
	"time"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// BR-GATEWAY-016: Storm Aggregation
// BUSINESS REQUIREMENT: Aggregate alerts during storm windows to reduce CRD count
// BUSINESS OUTCOME: 10-50x reduction in CRD count during storms
//
// TDD RED PHASE: These tests will FAIL until we implement mock Redis client
// Following 03-testing-strategy.mdc: Use real business logic with mocks ONLY for external dependencies

var _ = Describe("BR-GATEWAY-016: Storm Aggregation", func() {
	var (
		aggregator  *processing.StormAggregator
		redisServer *miniredis.Miniredis
		redisClient *redis.Client
		ctx         context.Context
		signal      *types.NormalizedSignal
	)

	BeforeEach(func() {
		ctx = context.Background()

		// TDD GREEN: Setup miniredis (in-memory Redis server)
		var err error
		redisServer, err = miniredis.Run()
		Expect(err).NotTo(HaveOccurred(), "miniredis should start successfully")

		// Create real Redis client pointing to miniredis
		redisClient = redis.NewClient(&redis.Options{
			Addr: redisServer.Addr(),
		})

		// TDD GREEN: Create real StormAggregator with default config
		aggregator = processing.NewStormAggregatorWithConfig(
			redisClient,
			nil,            // logger (nil = use nop logger for tests)
			5,              // bufferThreshold (5 alerts before creating window)
			60*time.Second, // inactivityTimeout (1 minute sliding window)
			5*time.Minute,  // maxWindowDuration (5 minute safety limit)
			1000,           // defaultMaxSize (1000 alerts per namespace)
			5000,           // globalMaxSize (5000 alerts total)
			nil,            // perNamespaceLimits (no overrides)
			0.95,           // samplingThreshold (95% utilization)
			0.5,            // samplingRate (50% sampling when threshold exceeded)
		)

		// Create test signal with realistic data
		signal = &types.NormalizedSignal{
			AlertName: "PodCrashLooping",
			Namespace: "prod-api",
			Severity:  "critical",
			Resource: types.ResourceIdentifier{
				Namespace: "prod-api",
				Kind:      "Pod",
				Name:      "payment-api-789",
			},
			Fingerprint:  "abc123def456",
			SourceType:   "prometheus",
			Source:       "prometheus-server",
			FiringTime:   time.Now(),
			ReceivedTime: time.Now(),
		}
	})

	AfterEach(func() {
		// Cleanup: Close Redis client and stop miniredis
		if redisClient != nil {
			_ = redisClient.Close()
		}
		if redisServer != nil {
			redisServer.Close()
		}
	})

	// ========================================
	// BUSINESS BEHAVIOR TESTS
	// Focus: WHAT the system does (business outcomes), not HOW it does it
	// Per TESTING_GUIDELINES.md: Test business value, not implementation
	// ========================================

	Describe("Window Expiration Behavior", func() {
		// BR-GATEWAY-008: Maximum window duration safety limit
		// BUSINESS OUTCOME: Prevent runaway aggregation windows that never close
		// WHY IT MATTERS: Ensures alerts eventually create CRDs (no infinite buffering)

		type windowExpirationTest struct {
			description     string
			windowStartTime time.Time
			currentTime     time.Time
			maxDuration     time.Duration
			expectedExpired bool
		}

		DescribeTable("window expiration decisions",
			func(test windowExpirationTest) {
				// BUSINESS BEHAVIOR: Does the system correctly identify when to close a window?
				// OUTCOME: Windows close after max duration to prevent infinite aggregation

				isExpired := aggregator.IsWindowExpired(test.windowStartTime, test.currentTime, test.maxDuration)

				Expect(isExpired).To(Equal(test.expectedExpired), test.description)
			},
			// Boundary value analysis per 03-testing-strategy.mdc
			// Use fixed time to avoid timing issues between time.Now() calls
			Entry("window just started (0min < 5min max)",
				windowExpirationTest{
					description:     "Newly created window should not be expired",
					windowStartTime: time.Date(2025, 11, 23, 12, 0, 0, 0, time.UTC),
					currentTime:     time.Date(2025, 11, 23, 12, 0, 0, 0, time.UTC),
					maxDuration:     5 * time.Minute,
					expectedExpired: false,
				}),
			Entry("window within max duration (3min < 5min max)",
				windowExpirationTest{
					description:     "Active window should not be expired",
					windowStartTime: time.Date(2025, 11, 23, 12, 0, 0, 0, time.UTC),
					currentTime:     time.Date(2025, 11, 23, 12, 3, 0, 0, time.UTC),
					maxDuration:     5 * time.Minute,
					expectedExpired: false,
				}),
			Entry("window at exact max duration (5min = 5min max)",
				windowExpirationTest{
					description:     "Window at exact max duration should not be expired (boundary: elapsed > maxDuration)",
					windowStartTime: time.Date(2025, 11, 23, 12, 0, 0, 0, time.UTC),
					currentTime:     time.Date(2025, 11, 23, 12, 5, 0, 0, time.UTC),
					maxDuration:     5 * time.Minute,
					expectedExpired: false, // Implementation uses >, not >=, so exactly at max is NOT expired
				}),
			Entry("window exceeds max duration (6min > 5min max)",
				windowExpirationTest{
					description:     "Window exceeding max duration should be expired",
					windowStartTime: time.Date(2025, 11, 23, 12, 0, 0, 0, time.UTC),
					currentTime:     time.Date(2025, 11, 23, 12, 6, 0, 0, time.UTC),
					maxDuration:     5 * time.Minute,
					expectedExpired: true,
				}),
			Entry("window far exceeds max duration (10min > 5min max)",
				windowExpirationTest{
					description:     "Window far exceeding max duration should be expired",
					windowStartTime: time.Date(2025, 11, 23, 12, 0, 0, 0, time.UTC),
					currentTime:     time.Date(2025, 11, 23, 12, 10, 0, 0, time.UTC),
					maxDuration:     5 * time.Minute,
					expectedExpired: true,
				}),
		)
	})

	Describe("Namespace Capacity Limits", func() {
		// BR-GATEWAY-011: Multi-tenant isolation
		// BUSINESS OUTCOME: Prevent single namespace from exhausting resources
		// WHY IT MATTERS: Ensures fair resource allocation across tenants

		type namespaceLimitTest struct {
			description   string
			namespace     string
			expectedLimit int
		}

		DescribeTable("namespace capacity allocation",
			func(test namespaceLimitTest) {
				// BUSINESS BEHAVIOR: Does the system enforce fair resource limits per namespace?
				// OUTCOME: Each namespace gets appropriate capacity allocation

				limit := aggregator.GetNamespaceLimit(test.namespace)

				Expect(limit).To(Equal(test.expectedLimit), test.description)
			},
			Entry("default limit for unknown namespace",
				namespaceLimitTest{
					description:   "Should return defaultMaxSize (1000)",
					namespace:     "unknown-namespace",
					expectedLimit: 1000,
				}),
		)

		Context("with namespace-specific overrides", func() {
			It("should apply custom capacity limits per namespace", func() {
				// BUSINESS BEHAVIOR: High-priority namespaces get more capacity
				// OUTCOME: Fair resource allocation based on business priorities

				// Create aggregator with namespace-specific overrides
				aggregatorWithOverrides := processing.NewStormAggregatorWithConfig(
					redisClient,
					nil,            // logger
					5,              // bufferThreshold
					60*time.Second, // inactivityTimeout
					5*time.Minute,  // maxWindowDuration
					1000,           // defaultMaxSize (default for most namespaces)
					5000,           // globalMaxSize
					map[string]int{ // perNamespaceLimits (overrides for specific namespaces)
						"prod-api":      2000, // Production gets 2x capacity
						"prod-database": 3000, // Critical systems get 3x capacity
					},
					0.95, // samplingThreshold
					0.5,  // samplingRate
				)

				// Verify business outcome: Custom limits applied
				prodAPILimit := aggregatorWithOverrides.GetNamespaceLimit("prod-api")
				Expect(prodAPILimit).To(Equal(2000), "Production API gets higher capacity")

				prodDBLimit := aggregatorWithOverrides.GetNamespaceLimit("prod-database")
				Expect(prodDBLimit).To(Equal(3000), "Critical database gets highest capacity")

				defaultLimit := aggregatorWithOverrides.GetNamespaceLimit("dev-test")
				Expect(defaultLimit).To(Equal(1000), "Non-priority namespaces get default capacity")
			})
		})
	})

	Describe("Overflow Protection Decisions", func() {
		// BR-GATEWAY-011: Overflow protection through sampling
		// BUSINESS OUTCOME: System remains operational during extreme alert storms
		// WHY IT MATTERS: Prevents system crash from memory exhaustion

		type samplingTest struct {
			description       string
			utilization       float64
			samplingThreshold float64
			expectedSample    bool
		}

		DescribeTable("overflow protection triggers",
			func(test samplingTest) {
				// BUSINESS BEHAVIOR: Does the system protect itself when nearing capacity?
				// OUTCOME: Sampling activates before system exhausts resources

				shouldSample := aggregator.ShouldSample(test.utilization, test.samplingThreshold)

				Expect(shouldSample).To(Equal(test.expectedSample), test.description)
			},
			// Boundary value analysis per 03-testing-strategy.mdc
			Entry("low utilization (50% < 95% threshold) - no sampling",
				samplingTest{
					description:       "Should not sample at low utilization",
					utilization:       0.50,
					samplingThreshold: 0.95,
					expectedSample:    false,
				}),
			Entry("medium utilization (75% < 95% threshold) - no sampling",
				samplingTest{
					description:       "Should not sample at medium utilization",
					utilization:       0.75,
					samplingThreshold: 0.95,
					expectedSample:    false,
				}),
			Entry("high utilization (90% < 95% threshold) - no sampling",
				samplingTest{
					description:       "Should not sample just below threshold",
					utilization:       0.90,
					samplingThreshold: 0.95,
					expectedSample:    false,
				}),
			Entry("at threshold (95% = 95% threshold) - no sampling",
				samplingTest{
					description:       "Should not sample at exact threshold",
					utilization:       0.95,
					samplingThreshold: 0.95,
					expectedSample:    false,
				}),
			Entry("above threshold (96% > 95% threshold) - enable sampling",
				samplingTest{
					description:       "Should sample when exceeding threshold",
					utilization:       0.96,
					samplingThreshold: 0.95,
					expectedSample:    true,
				}),
			Entry("very high utilization (99% > 95% threshold) - enable sampling",
				samplingTest{
					description:       "Should sample at very high utilization",
					utilization:       0.99,
					samplingThreshold: 0.95,
					expectedSample:    true,
				}),
		)
	})

	// ========================================
	// STORM DETECTION AND AGGREGATION BEHAVIOR
	// Focus: Business outcomes during alert storms
	// ========================================

	Describe("Alert Buffering Behavior", func() {
		// BR-GATEWAY-016: Buffer first N alerts before creating window
		// BUSINESS OUTCOME: Avoid creating aggregation windows for short-lived alert bursts
		// WHY IT MATTERS: Reduces false positives (brief spikes shouldn't trigger storm mode)

		Context("during initial alert burst", func() {
			It("should wait for threshold before starting aggregation", func() {
				// BUSINESS BEHAVIOR: Don't overreact to first few alerts
				// OUTCOME: System waits to confirm it's a real storm, not a blip

				bufferSize, shouldAggregate, err := aggregator.BufferFirstAlert(ctx, signal)

				Expect(err).ToNot(HaveOccurred(), "System should accept initial alerts")
				Expect(bufferSize).To(Equal(1), "System tracks alert count")
				Expect(shouldAggregate).To(BeFalse(), "System waits for more alerts before aggregating")
			})

			It("should continue buffering as alert burst grows", func() {
				// BUSINESS BEHAVIOR: System observes the growing alert pattern
				// OUTCOME: Tracks multiple alerts without triggering aggregation yet

				// First alert arrives
				size1, agg1, err1 := aggregator.BufferFirstAlert(ctx, signal)
				Expect(err1).ToNot(HaveOccurred(), "System accepts first alert")
				Expect(size1).To(Equal(1), "System tracks 1 alert")
				Expect(agg1).To(BeFalse(), "Not enough alerts yet")

				// Second alert arrives (different pod)
				signal.Resource.Name = "payment-api-790"
				size2, agg2, err2 := aggregator.BufferFirstAlert(ctx, signal)
				Expect(err2).ToNot(HaveOccurred(), "System accepts second alert")
				Expect(size2).To(Equal(2), "System tracks 2 alerts")
				Expect(agg2).To(BeFalse(), "Still not enough alerts")

				// Third alert arrives (different pod)
				signal.Resource.Name = "payment-api-791"
				size3, agg3, err3 := aggregator.BufferFirstAlert(ctx, signal)
				Expect(err3).ToNot(HaveOccurred(), "System accepts third alert")
				Expect(size3).To(Equal(3), "System tracks 3 alerts")
				Expect(agg3).To(BeFalse(), "Waiting for threshold (5 alerts)")
			})
		})

		Context("when alert burst confirms a storm", func() {
			It("should trigger aggregation mode at threshold", func() {
				// BUSINESS BEHAVIOR: System confirms this is a real storm, not a blip
				// OUTCOME: Switches from buffering to aggregation (cost reduction mode)

				// First 4 alerts: System is still observing
				for i := 0; i < 4; i++ {
					signal.Resource.Name = "pod-" + string(rune('a'+i))
					_, shouldAgg, err := aggregator.BufferFirstAlert(ctx, signal)
					Expect(err).ToNot(HaveOccurred(), "System accepts alerts during observation")
					Expect(shouldAgg).To(BeFalse(), "System still observing pattern")
				}

				// 5th alert: Storm confirmed!
				signal.Resource.Name = "pod-e"
				bufferSize, shouldAggregate, err := aggregator.BufferFirstAlert(ctx, signal)

				Expect(err).ToNot(HaveOccurred(), "System accepts 5th alert")
				Expect(bufferSize).To(Equal(5), "System tracked all 5 alerts")
				Expect(shouldAggregate).To(BeTrue(), "Storm confirmed - switch to aggregation mode")
			})
		})

		Context("when namespace reaches capacity limit", func() {
			It("should protect system by rejecting excess alerts", func() {
				// BR-GATEWAY-011: Multi-tenant isolation
				// BUSINESS BEHAVIOR: Prevent single namespace from exhausting system resources
				// OUTCOME: System remains operational for other namespaces

				// Create system with limited capacity per namespace
				aggregatorLowCap := processing.NewStormAggregatorWithConfig(
					redisClient,
					nil,            // logger
					5,              // bufferThreshold
					60*time.Second, // inactivityTimeout
					5*time.Minute,  // maxWindowDuration
					10,             // defaultMaxSize (LOW CAPACITY for test)
					50,             // globalMaxSize
					nil,            // perNamespaceLimits
					0.95,           // samplingThreshold
					0.5,            // samplingRate
				)

				// Namespace fills its allocated capacity
				for i := 0; i < 10; i++ {
					signal.Resource.Name = "pod-" + string(rune('a'+i))
					_, _, err := aggregatorLowCap.BufferFirstAlert(ctx, signal)
					Expect(err).ToNot(HaveOccurred(), "System accepts alerts up to capacity")
				}

				// Namespace tries to exceed its capacity
				signal.Resource.Name = "pod-overflow"
				_, _, err := aggregatorLowCap.BufferFirstAlert(ctx, signal)

				Expect(err).To(HaveOccurred(), "System protects itself from resource exhaustion")
				Expect(err.Error()).To(ContainSubstring("over capacity"), "Clear error message for operators")
			})
		})

		Context("BR-GATEWAY-011: Sampling for overflow protection", func() {
			It("should detect high utilization and enable sampling mode", func() {
				// BUSINESS BEHAVIOR: System detects resource pressure and activates protection
				// OUTCOME: Gateway enables sampling to prevent exhaustion
				// TDD GREEN: Real Redis operations via miniredis

				// Create aggregator with low capacity and sampling enabled
				aggregatorSampling := processing.NewStormAggregatorWithConfig(
					redisClient,
					nil,            // logger
					5,              // bufferThreshold
					60*time.Second, // inactivityTimeout
					5*time.Minute,  // maxWindowDuration
					100,            // defaultMaxSize (low capacity for test)
					5000,           // globalMaxSize
					nil,            // perNamespaceLimits
					0.80,           // samplingThreshold (80% utilization triggers sampling)
					0.5,            // samplingRate (50% sampling when triggered)
				)

				// Fill buffer to 85% capacity (85 alerts out of 100 max)
				// This exceeds 80% threshold, triggering sampling
				for i := 0; i < 85; i++ {
					testSignal := *signal // Copy signal
					testSignal.Resource.Name = fmt.Sprintf("pod-fill-%d", i)
					_, _, err := aggregatorSampling.BufferFirstAlert(ctx, &testSignal)
					Expect(err).ToNot(HaveOccurred(), "System accepts alerts until threshold")
				}

				// BUSINESS VALIDATION 1: System detects high utilization
				utilization, err := aggregatorSampling.GetNamespaceUtilization(ctx, signal.Resource.Namespace)
				Expect(err).ToNot(HaveOccurred(), "System can measure utilization")
				Expect(utilization).To(BeNumerically(">", 0.80), "System detects high utilization (85% > 80% threshold)")

				// BUSINESS VALIDATION 2: System enables sampling mode
				shouldSample := aggregatorSampling.ShouldSample(utilization, 0.80)
				Expect(shouldSample).To(BeTrue(), "System enables sampling to protect capacity")

				// BUSINESS VALIDATION 3: System continues accepting alerts (not completely blocked)
				testSignal := *signal
				testSignal.Resource.Name = "pod-after-threshold"
				_, _, err = aggregatorSampling.BufferFirstAlert(ctx, &testSignal)
				// System may accept or reject (sampling is probabilistic), but should not crash
				// The key business outcome is that the system REMAINS OPERATIONAL
				Expect(err).To(Or(
					BeNil(), // Alert accepted (passed sampling)
					MatchError(ContainSubstring("over capacity")), // Alert rejected (sampling or capacity)
				), "System remains operational under high load")
			})
		})
	})

	Describe("Storm Aggregation Window Lifecycle", func() {
		// BR-GATEWAY-016: Aggregate alerts into single CRD during storms
		// BUSINESS OUTCOME: 10-50x reduction in CRD count (cost savings, reduced API load)

		Context("during initial observation phase", func() {
			It("should not create aggregation window yet", func() {
				// BUSINESS BEHAVIOR: System waits to confirm storm before aggregating
				// OUTCOME: Avoids premature aggregation for brief alert bursts

				stormMetadata := &processing.StormMetadata{
					StormType:  "rate-based",
					Window:     "60s",
					AlertCount: 1,
				}

				windowID, err := aggregator.StartAggregation(ctx, signal, stormMetadata)

				Expect(err).ToNot(HaveOccurred(), "System operates normally during observation")
				Expect(windowID).To(BeEmpty(), "No aggregation window created yet")
			})
		})

		Context("when storm is confirmed", func() {
			It("should activate cost-reduction mode", func() {
				// BUSINESS BEHAVIOR: Storm confirmed - switch to aggregation mode
				// OUTCOME: Begin collecting alerts for single aggregated CRD

				// Storm observation phase (4 alerts)
				for i := 0; i < 4; i++ {
					signal.Resource.Name = "pod-" + string(rune('a'+i))
					aggregator.BufferFirstAlert(ctx, signal)
				}

				// 5th alert confirms storm
				signal.Resource.Name = "pod-e"
				stormMetadata := &processing.StormMetadata{
					StormType:  "rate-based",
					Window:     "60s",
					AlertCount: 5,
				}

				windowID, err := aggregator.StartAggregation(ctx, signal, stormMetadata)

				Expect(err).ToNot(HaveOccurred(), "System successfully enters aggregation mode")
				Expect(windowID).NotTo(BeEmpty(), "Aggregation window created for collecting alerts")
				// DD-GATEWAY-008 + BR-GATEWAY-011: Window ID includes namespace for multi-tenant isolation
				Expect(windowID).To(MatchRegexp(`^prod-api:PodCrashLooping-\d+$`), "Window identified by namespace:alert type")
			})

			It("should consolidate all observed alerts into aggregation window", func() {
				// BUSINESS BEHAVIOR: Don't lose the alerts we observed during buffering
				// OUTCOME: All alerts (buffered + current) go into aggregation window

				// Observation phase: Buffer 5 alerts
				for i := 0; i < 5; i++ {
					signal.Resource.Name = "pod-" + string(rune('a'+i))
					aggregator.BufferFirstAlert(ctx, signal)
				}

				// Start aggregation
				stormMetadata := &processing.StormMetadata{
					StormType:  "rate-based",
					Window:     "60s",
					AlertCount: 5,
				}

				windowID, err := aggregator.StartAggregation(ctx, signal, stormMetadata)
				Expect(err).ToNot(HaveOccurred(), "Aggregation starts successfully")

				// Verify business outcome: All observed alerts are tracked
				resources, err := aggregator.GetAggregatedResources(ctx, windowID)
				Expect(err).ToNot(HaveOccurred())
				Expect(resources).To(HaveLen(5), "All observed alerts tracked in aggregation window")
			})
		})
	})

	Describe("Ongoing Storm Aggregation", func() {
		// BR-GATEWAY-008: Sliding window with inactivity timeout
		// BUSINESS OUTCOME: Continue aggregating during active storms, close when storm ends
		// WHY IT MATTERS: Maximizes cost reduction during storms, creates CRD when storm subsides

		Context("during active storm", func() {
			It("should continue collecting alerts into aggregation window", func() {
				// BUSINESS BEHAVIOR: Storm is ongoing - keep aggregating
				// OUTCOME: More alerts added to window = greater cost reduction

				// Create aggregation window with current timestamp (within max duration)
				currentTime := time.Now().Unix()
				windowID := fmt.Sprintf("PodCrashLooping-p1-%d", currentTime)

				// New alert arrives during storm
				err := aggregator.AddResource(ctx, windowID, signal)

				Expect(err).ToNot(HaveOccurred(), "System continues aggregating during storm")

				// Verify business outcome: Alert added to aggregation
				count, err := aggregator.GetResourceCount(ctx, windowID)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(1), "Alert successfully aggregated")
			})
		})

		// BR-GATEWAY-008: Maximum window duration safety limit
		// ✅ COVERED: Pure function tests at lines 109-173 validate IsWindowExpired() logic
		// ✅ COVERED: Integration test needed for Redis + expiration check (Next Sprint)
		// REMOVED: Pending test moved to integration tier (test/integration/gateway/storm_window_lifecycle_test.go)
	})
})
