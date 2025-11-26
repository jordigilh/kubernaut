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

package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// StormAggregator Enhancement tests - part of main Gateway unit test suite
var _ = Describe("StormAggregator Enhancement - Strict TDD", func() {
	var (
		aggregator   *processing.StormAggregator
		redisServer  *miniredis.Miniredis
		redisClient  *redis.Client
		ctx          context.Context
		testSettings *config.StormSettings
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create miniredis server for testing
		var err error
		redisServer, err = miniredis.Run()
		Expect(err).ToNot(HaveOccurred())

		redisClient = redis.NewClient(&redis.Options{
			Addr: redisServer.Addr(),
		})

		testSettings = &config.StormSettings{
			RateThreshold:     10,
			PatternThreshold:  5,
			AggregationWindow: 60 * time.Second,
		}

		// Use NewStormAggregatorWithConfig for full feature support
		aggregator = processing.NewStormAggregatorWithConfig(
			redisClient,
			5,                              // bufferThreshold
			testSettings.AggregationWindow, // inactivityTimeout
			5*time.Minute,                  // maxWindowDuration
			1000,                           // defaultMaxSize
			5000,                           // globalMaxSize
			nil,                            // perNamespaceLimits
			0.95,                           // samplingThreshold
			0.5,                            // samplingRate
		)
	})

	AfterEach(func() {
		if redisClient != nil {
			redisClient.Close()
		}
		if redisServer != nil {
			redisServer.Close()
		}
	})

	// TDD Cycle 1: BufferFirstAlert - ONE test at a time
	Describe("BufferFirstAlert - First Test (BR-GATEWAY-016)", func() {
		Context("when first alert arrives below threshold", func() {
			It("should accept alert without triggering aggregation", func() {
				// BUSINESS SCENARIO: Single pod crashes in prod-api
				signal := &types.NormalizedSignal{
					Namespace: "prod-api",
					AlertName: "PodCrashLooping",
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "payment-api-1",
					},
				}

				// BEHAVIOR: System accepts alert but delays aggregation
				_, shouldAggregate, err := aggregator.BufferFirstAlert(ctx, signal)

				// CORRECTNESS: Alert accepted, aggregation not triggered
				Expect(err).ToNot(HaveOccurred(), "System should accept first alert")
				Expect(shouldAggregate).To(BeFalse(), "System should delay aggregation below threshold")

				// BUSINESS OUTCOME: No CRD created yet (cost savings from delayed AI analysis)
				// This validates BR-GATEWAY-016: Buffer alerts before aggregation
			})

			Context("when threshold alert arrives", func() {
				It("should trigger aggregation after buffering threshold alerts", func() {
					// BUSINESS SCENARIO: 5 pods crash in rapid succession (storm detected)
					signal := &types.NormalizedSignal{
						Namespace: "prod-api",
						AlertName: "PodCrashLooping",
						Resource: types.ResourceIdentifier{
							Kind: "Pod",
							Name: "payment-api-1",
						},
					}

					// BEHAVIOR: System buffers alerts until threshold is reached
					for i := 1; i <= 5; i++ {
						signal.Resource.Name = fmt.Sprintf("payment-api-%d", i)
						_, shouldAggregate, err := aggregator.BufferFirstAlert(ctx, signal)
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Alert %d should be accepted", i))

						if i < 5 {
							// CORRECTNESS: Alerts 1-4 should NOT trigger aggregation
							Expect(shouldAggregate).To(BeFalse(), fmt.Sprintf("Alert %d should delay aggregation (below threshold)", i))
						} else {
							// CORRECTNESS: Alert 5 SHOULD trigger aggregation
							Expect(shouldAggregate).To(BeTrue(), "Alert 5 should trigger aggregation (threshold reached)")
						}
					}

					// BUSINESS OUTCOME: Storm detected → aggregation triggered → 1 CRD instead of 5
					// Cost savings: 5 alerts → 1 AI analysis (BR-GATEWAY-016)
				})
			})
		})

		// TDD Cycle 3: ExtendWindow - Sliding Window Behavior
		Describe("ExtendWindow - Sliding Window (BR-GATEWAY-008)", func() {
			Context("when alert arrives during active window", func() {
				It("should extend window lifetime to capture ongoing storm", func() {
					// BUSINESS SCENARIO: Storm window is about to expire, new alert arrives
					windowID := "test-window-123"

					// Setup: Create a window that's about to expire (10s remaining)
					windowKey := fmt.Sprintf("alert:storm:aggregate:PodCrashLooping")
					err := redisClient.Set(ctx, windowKey, windowID, 10*time.Second).Err()
					Expect(err).ToNot(HaveOccurred())

					initialTTL, err := redisClient.TTL(ctx, windowKey).Result()
					Expect(err).ToNot(HaveOccurred())

					// BEHAVIOR: New alert extends the window (sliding window behavior)
					newExpiration, err := aggregator.ExtendWindow(ctx, windowID, time.Now())

					// CORRECTNESS: Window lifetime is extended
					Expect(err).ToNot(HaveOccurred(), "Window extension should succeed")
					Expect(newExpiration).ToNot(BeZero(), "New expiration time should be set")

					// Verify window lifetime was extended (new TTL > initial TTL)
					newTTL, err := redisClient.TTL(ctx, windowKey).Result()
					Expect(err).ToNot(HaveOccurred())
					Expect(newTTL).To(BeNumerically(">", initialTTL), "Window lifetime should be extended")

					// BUSINESS OUTCOME: Ongoing storm continues to aggregate alerts
					// Prevents premature window closure during active incident (BR-GATEWAY-008)
				})
			})
		})

		// TDD Cycle 4: IsWindowExpired - Max Duration Safety
		Describe("IsWindowExpired - Max Duration Safety (BR-GATEWAY-008)", func() {
			Context("when window duration exceeds max limit", func() {
				It("should enforce max window duration to prevent unbounded aggregation", func() {
					// BUSINESS SCENARIO: Storm window has been open for 6 minutes (exceeds 5-minute safety limit)
					windowStartTime := time.Now().Add(-6 * time.Minute)
					currentTime := time.Now()
					maxDuration := 5 * time.Minute

					// BEHAVIOR: System checks if window has exceeded safety limit
					expired := aggregator.IsWindowExpired(windowStartTime, currentTime, maxDuration)

					// CORRECTNESS: Window should be marked as expired (6 min > 5 min max)
					Expect(expired).To(BeTrue(), "Window exceeding max duration must be closed for safety")

					// BUSINESS OUTCOME: Prevents unbounded windows that could delay incident response
					// Ensures timely CRD creation and AI analysis (BR-GATEWAY-008 safety limit)
				})
			})
		})

		// TDD Cycle 5: GetNamespaceUtilization - Multi-Tenant Isolation
		Describe("GetNamespaceUtilization - Multi-Tenant (BR-GATEWAY-011)", func() {
			Context("when namespace has buffered alerts", func() {
				It("should track namespace capacity to prevent resource exhaustion", func() {
					// BUSINESS SCENARIO: prod-api namespace has some buffered alerts
					namespace := "prod-api"

					// Setup: Buffer 5 alerts in this namespace
					for i := 1; i <= 5; i++ {
						signal := &types.NormalizedSignal{
							Namespace: namespace,
							AlertName: "PodCrashLooping",
							Resource: types.ResourceIdentifier{
								Kind: "Pod",
								Name: fmt.Sprintf("payment-api-%d", i),
							},
						}
						_, _, err := aggregator.BufferFirstAlert(ctx, signal)
						Expect(err).ToNot(HaveOccurred())
					}

					// BEHAVIOR: System reports namespace buffer utilization
					utilization, err := aggregator.GetNamespaceUtilization(ctx, namespace)

					// CORRECTNESS: Utilization should be low (5 alerts is minimal usage)
					Expect(err).ToNot(HaveOccurred(), "Utilization calculation should succeed")
					Expect(utilization).To(BeNumerically("<", 0.1), "Utilization should be low (<10%)")

					// BUSINESS OUTCOME: Enables capacity-based decisions (sampling, blocking)
					// Prevents one namespace from exhausting shared resources (BR-GATEWAY-011)
				})
			})
		})

		// TDD Cycle 6: ShouldSample - Overflow Protection
		Describe("ShouldSample - Overflow Protection (BR-GATEWAY-011)", func() {
			Context("when utilization exceeds sampling threshold", func() {
				It("should activate sampling to prevent buffer overflow", func() {
					// BUSINESS SCENARIO: Namespace buffer is near capacity (96% full)
					currentUtilization := 0.96 // 96% utilization
					samplingThreshold := 0.95  // 95% threshold

					// BEHAVIOR: System decides whether to enable sampling
					shouldSample := aggregator.ShouldSample(currentUtilization, samplingThreshold)

					// CORRECTNESS: Sampling should be enabled (96% > 95%)
					Expect(shouldSample).To(BeTrue(), "Sampling should activate above threshold")

					// BUSINESS OUTCOME: Prevents buffer overflow by dropping low-priority alerts
					// Protects system stability under extreme load (BR-GATEWAY-011)
				})
			})

			Context("when utilization is below sampling threshold", func() {
				It("should not activate sampling when capacity is available", func() {
					// BUSINESS SCENARIO: Namespace buffer has plenty of capacity (50% full)
					currentUtilization := 0.50 // 50% utilization
					samplingThreshold := 0.95  // 95% threshold

					// BEHAVIOR: System decides whether to enable sampling
					shouldSample := aggregator.ShouldSample(currentUtilization, samplingThreshold)

					// CORRECTNESS: Sampling should NOT be enabled (50% < 95%)
					Expect(shouldSample).To(BeFalse(), "Sampling should not activate below threshold")

					// BUSINESS OUTCOME: All alerts are accepted when capacity is available
					// Maximizes alert capture during normal operations (BR-GATEWAY-011)
				})
			})
		})
	})
})
