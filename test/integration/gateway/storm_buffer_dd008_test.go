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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	goredis "github.com/go-redis/redis/v8"

	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	// No need to import test/integration/gateway - we're already in package gateway
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// DD-GATEWAY-008: Storm Buffering Integration Tests
// BR-GATEWAY-016: Buffered first-alert aggregation
// BR-GATEWAY-008: Sliding window with inactivity timeout
// BR-GATEWAY-011: Multi-tenant isolation
//
// Test Tier: INTEGRATION
// Rationale: Tests Redis infrastructure interaction with DD-GATEWAY-008 features.
// Validates buffering, sliding window, and multi-tenant isolation with real Redis.
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%): Business logic in isolation (19/19 passing)
// - Integration tests (>50%): Infrastructure interaction (THIS FILE - Redis)
// - E2E tests (10-15%): Complete workflow (webhook → buffer → aggregated CRD)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("DD-GATEWAY-008: Storm Buffering (Integration)", func() {
	var (
		ctx         context.Context
		redisClient *goredis.Client
		aggregator  *processing.StormAggregator
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Use real Redis from test infrastructure
		redisTestClient := SetupRedisTestClient(ctx)
		Expect(redisTestClient).ToNot(BeNil(), "Redis test client required for DD-GATEWAY-008 tests")
		Expect(redisTestClient.Client).ToNot(BeNil(), "Redis client required for DD-GATEWAY-008 tests")
		redisClient = redisTestClient.Client

		// Clean Redis state before each test
		err := redisClient.FlushDB(ctx).Err()
		Expect(err).ToNot(HaveOccurred(), "Failed to flush Redis before test")

		// Create aggregator with DD-GATEWAY-008 configuration
		// Using short window duration for faster tests (5 seconds instead of 60)
		aggregator = processing.NewStormAggregatorWithConfig(
			redisClient,
			5,                // bufferThreshold: 5 alerts before window
			5*time.Second,    // inactivityTimeout: 5s for testing (60s in production)
			30*time.Second,   // maxWindowDuration: 30s for testing (5m in production)
			1000,             // defaultMaxSize: 1000 alerts per namespace
			5000,             // globalMaxSize: 5000 alerts total
			map[string]int{}, // perNamespaceLimits: empty (use defaults)
			0.95,             // samplingThreshold: 95% utilization
			0.5,              // samplingRate: 50% when sampling
		)
	})

	AfterEach(func() {
		// Clean up Redis state after test
		if redisClient != nil {
			_ = redisClient.FlushDB(ctx)
		}
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-016: Buffered First-Alert Aggregation
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("BR-GATEWAY-016: Buffered First-Alert Aggregation", func() {
	Context("when alerts arrive below threshold", func() {
		It("should delay aggregation until threshold is reached", func() {
			// BUSINESS SCENARIO: 4 pods crash in prod-api namespace
			processID := GinkgoParallelProcess()
			namespace := fmt.Sprintf("prod-api-p%d-%d", processID, time.Now().Unix())
			alertName := fmt.Sprintf("PodCrashLooping-p%d", processID)

				// BEHAVIOR: System buffers alerts without triggering aggregation
				// (Delaying CRD creation saves AI analysis costs)
				for i := 1; i <= 4; i++ {
					signal := &types.NormalizedSignal{
						Namespace: namespace,
						AlertName: alertName,
						Resource: types.ResourceIdentifier{
							Kind: "Pod",
							Name: fmt.Sprintf("payment-api-%d", i),
						},
					}

					_, shouldAggregate, err := aggregator.BufferFirstAlert(ctx, signal)

					// CORRECTNESS: Each alert is accepted but doesn't trigger aggregation
					Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Alert %d should be accepted", i))
					Expect(shouldAggregate).To(BeFalse(), fmt.Sprintf("Alert %d should not trigger aggregation (below threshold)", i))
				}

				// BUSINESS OUTCOME: No aggregation window created yet
				// This means no CRD created, no AI analysis triggered, cost savings achieved
				shouldExist, _, err := aggregator.ShouldAggregate(ctx, &types.NormalizedSignal{
					Namespace: namespace,
					AlertName: alertName,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(shouldExist).To(BeFalse(), "Aggregation window should not exist before threshold (BR-GATEWAY-016)")
			})
		})

	Context("when 5th alert arrives (threshold reached)", func() {
		It("should trigger aggregation of all buffered alerts", func() {
			// BUSINESS SCENARIO: 5 pods crash in prod-api (storm threshold reached)
			processID := GinkgoParallelProcess()
			namespace := fmt.Sprintf("prod-api-p%d-%d", processID, time.Now().Unix())
			alertName := fmt.Sprintf("PodCrashLooping-p%d", processID)
				stormMetadata := &processing.StormMetadata{
					StormType:  "rate",
					Window:     "1m",
					AlertCount: 5,
				}

				// BEHAVIOR: First 4 alerts are buffered (no aggregation yet)
				for i := 1; i <= 4; i++ {
					signal := &types.NormalizedSignal{
						Namespace: namespace,
						AlertName: alertName,
						Resource: types.ResourceIdentifier{
							Kind: "Pod",
							Name: fmt.Sprintf("payment-api-%d", i),
						},
					}
					_, shouldAggregate, err := aggregator.BufferFirstAlert(ctx, signal)
					Expect(err).ToNot(HaveOccurred())
					Expect(shouldAggregate).To(BeFalse(), "Alerts 1-4 should not trigger aggregation")
				}

				// BEHAVIOR: 5th alert triggers aggregation window creation
				signal5 := &types.NormalizedSignal{
					Namespace: namespace,
					AlertName: alertName,
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "payment-api-5",
					},
				}

				windowID, err := aggregator.StartAggregation(ctx, signal5, stormMetadata)

				// CORRECTNESS: Aggregation window is created when threshold reached
				Expect(err).ToNot(HaveOccurred(), "5th alert should trigger aggregation successfully")
				Expect(windowID).ToNot(BeEmpty(), "Aggregation window should be created at threshold")

				// CORRECTNESS: All 5 alerts are included in aggregation
				resources, err := aggregator.GetAggregatedResources(ctx, windowID)
				Expect(err).ToNot(HaveOccurred())
				Expect(resources).To(HaveLen(5), "All 5 alerts should be aggregated together")

				// BUSINESS OUTCOME: 5 alerts → 1 aggregation window → 1 CRD (80% cost reduction)
				// Without buffering: 5 alerts → 5 CRDs → 5 AI analyses
				// With buffering: 5 alerts → 1 CRD → 1 AI analysis (BR-GATEWAY-016)
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-008: Sliding Window with Inactivity Timeout
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("BR-GATEWAY-008: Sliding Window Behavior", func() {
	Context("when alerts keep arriving within timeout", func() {
		It("should extend window timer on each alert", func() {
			processID := GinkgoParallelProcess()
			namespace := fmt.Sprintf("prod-api-p%d-%d", processID, time.Now().Unix())
			alertName := fmt.Sprintf("PodCrashLooping-p%d", processID)
				stormMetadata := &processing.StormMetadata{
					StormType:  "rate",
					Window:     "1m",
					AlertCount: 5,
				}

				// Create initial window with 5 alerts
				for i := 1; i <= 5; i++ {
					signal := &types.NormalizedSignal{
						Namespace: namespace,
						AlertName: alertName,
						Resource: types.ResourceIdentifier{
							Kind: "Pod",
							Name: fmt.Sprintf("payment-api-%d", i),
						},
					}
					if i < 5 {
						_, _, err := aggregator.BufferFirstAlert(ctx, signal)
						Expect(err).ToNot(HaveOccurred())
					} else {
						// 5th alert creates window
						windowID, err := aggregator.StartAggregation(ctx, signal, stormMetadata)
						Expect(err).ToNot(HaveOccurred())
						Expect(windowID).ToNot(BeEmpty())
					}
				}

				// Get window key and initial TTL
				windowKey := fmt.Sprintf("alert:storm:aggregate:%s", alertName)
				initialTTL, err := redisClient.TTL(ctx, windowKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(initialTTL).To(BeNumerically("<=", 5*time.Second), "Initial TTL should be ~5s")

				// Wait 3 seconds (more than half the window)
				time.Sleep(3 * time.Second)

				// Send another alert (should extend window)
				windowID, err := redisClient.Get(ctx, windowKey).Result()
				Expect(err).ToNot(HaveOccurred())

				signal6 := &types.NormalizedSignal{
					Namespace: namespace,
					AlertName: alertName,
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "payment-api-6",
					},
				}

				// BEHAVIOR: AddResource should extend window timer
				err = aggregator.AddResource(ctx, windowID, signal6)
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: TTL should be reset to full duration
				newTTL, err := redisClient.TTL(ctx, windowKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(newTTL).To(BeNumerically(">", 4*time.Second), "TTL should be reset to ~5s (sliding window)")

				// BUSINESS OUTCOME: Window stays open as long as alerts keep coming (BR-GATEWAY-008)
			})
		})

	Context("when no alerts arrive within timeout", func() {
		It("should let window expire naturally", func() {
			processID := GinkgoParallelProcess()
			namespace := fmt.Sprintf("prod-api-p%d-%d", processID, time.Now().Unix())
			alertName := fmt.Sprintf("PodCrashLooping-p%d", processID)
				stormMetadata := &processing.StormMetadata{
					StormType:  "rate",
					Window:     "1m",
					AlertCount: 5,
				}

				// Create window with 5 alerts
				for i := 1; i <= 5; i++ {
					signal := &types.NormalizedSignal{
						Namespace: namespace,
						AlertName: alertName,
						Resource: types.ResourceIdentifier{
							Kind: "Pod",
							Name: fmt.Sprintf("payment-api-%d", i),
						},
					}
					if i < 5 {
						_, _, err := aggregator.BufferFirstAlert(ctx, signal)
						Expect(err).ToNot(HaveOccurred())
					} else {
						windowID, err := aggregator.StartAggregation(ctx, signal, stormMetadata)
						Expect(err).ToNot(HaveOccurred())
						Expect(windowID).ToNot(BeEmpty())
					}
				}

				// Verify window exists
				windowKey := fmt.Sprintf("alert:storm:aggregate:%s", alertName)
				_, err := redisClient.Get(ctx, windowKey).Result()
				Expect(err).ToNot(HaveOccurred(), "Window should exist initially")

				// Wait for window to expire (6 seconds > 5 second timeout)
				time.Sleep(6 * time.Second)

				// BEHAVIOR: Window should have expired
				_, err = redisClient.Get(ctx, windowKey).Result()

				// CORRECTNESS: Window should be gone
				Expect(err).To(Equal(goredis.Nil), "Window should expire after inactivity timeout")

				// BUSINESS OUTCOME: Window closes after 5s inactivity, ready for CRD creation (BR-GATEWAY-008)
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-011: Multi-Tenant Isolation
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("BR-GATEWAY-011: Multi-Tenant Isolation", func() {
		Context("when namespace has custom limit", func() {
			It("should enforce per-namespace buffer limits", func() {
				processID := GinkgoParallelProcess()
				namespace := fmt.Sprintf("prod-api-p%d-%d", processID, time.Now().Unix())

				// Create aggregator with per-namespace limits (use dynamic namespace)
				aggregatorWithLimits := processing.NewStormAggregatorWithConfig(
					redisClient,
					5,
					5*time.Second,
					30*time.Second,
					1000,
					5000,
					map[string]int{
						namespace: 10, // Low limit for testing (use dynamic namespace)
					},
					0.95,
			0.5,
		)
		alertName := fmt.Sprintf("PodCrashLooping-p%d", processID)

				// Fill buffer to capacity (10 alerts)
				for i := 1; i <= 10; i++ {
					signal := &types.NormalizedSignal{
						Namespace: namespace,
						AlertName: alertName,
						Resource: types.ResourceIdentifier{
							Kind: "Pod",
							Name: fmt.Sprintf("payment-api-%d", i),
						},
					}
					_, _, err := aggregatorWithLimits.BufferFirstAlert(ctx, signal)
					Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Alert %d should succeed", i))
				}

				// 11th alert should be rejected (over capacity)
				signal11 := &types.NormalizedSignal{
					Namespace: namespace,
					AlertName: alertName,
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "payment-api-11",
					},
				}

				// BEHAVIOR: Should reject alert when over capacity
				_, _, err := aggregatorWithLimits.BufferFirstAlert(ctx, signal11)

				// CORRECTNESS: Should return capacity error
				Expect(err).To(HaveOccurred(), "Should reject alert when over capacity")
				Expect(err.Error()).To(ContainSubstring("over capacity"), "Error should mention capacity")

				// BUSINESS OUTCOME: Namespace isolation prevents one namespace from affecting others (BR-GATEWAY-011)
			})
		})

	Context("when multiple namespaces have storms", func() {
		It("should isolate buffers per namespace", func() {
			processID := GinkgoParallelProcess()
			namespace1 := fmt.Sprintf("prod-api-p%d-%d", processID, time.Now().Unix())
			namespace2 := fmt.Sprintf("dev-test-p%d-%d", processID, time.Now().Unix()+1)
			alertName := fmt.Sprintf("PodCrashLooping-p%d", processID)

				// Buffer 3 alerts in prod-api
				for i := 1; i <= 3; i++ {
					signal := &types.NormalizedSignal{
						Namespace: namespace1,
						AlertName: alertName,
						Resource: types.ResourceIdentifier{
							Kind: "Pod",
							Name: fmt.Sprintf("payment-api-%d", i),
						},
					}
					_, _, err := aggregator.BufferFirstAlert(ctx, signal)
					Expect(err).ToNot(HaveOccurred())
				}

				// Buffer 2 alerts in dev-test
				for i := 1; i <= 2; i++ {
					signal := &types.NormalizedSignal{
						Namespace: namespace2,
						AlertName: alertName,
						Resource: types.ResourceIdentifier{
							Kind: "Pod",
							Name: fmt.Sprintf("test-api-%d", i),
						},
					}
					_, _, err := aggregator.BufferFirstAlert(ctx, signal)
					Expect(err).ToNot(HaveOccurred())
				}

				// BEHAVIOR: Each namespace should have separate buffer
				buffer1Key := fmt.Sprintf("alert:buffer:%s:%s", namespace1, alertName)
				buffer2Key := fmt.Sprintf("alert:buffer:%s:%s", namespace2, alertName)

				// CORRECTNESS: Verify separate buffers
				count1, err := redisClient.LLen(ctx, buffer1Key).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(count1).To(Equal(int64(3)), "prod-api should have 3 buffered alerts")

				count2, err := redisClient.LLen(ctx, buffer2Key).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(count2).To(Equal(int64(2)), "dev-test should have 2 buffered alerts")

				// BUSINESS OUTCOME: Namespaces are isolated, storms don't affect each other (BR-GATEWAY-011)
			})
		})
	})
})
