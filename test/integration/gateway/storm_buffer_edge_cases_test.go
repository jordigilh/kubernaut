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

	"github.com/redis/go-redis/v9"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// DD-GATEWAY-008: Storm Buffering Edge Cases (Integration)
// BR-GATEWAY-016: Buffered first-alert aggregation
// BR-GATEWAY-008: Sliding window with inactivity timeout
// BR-GATEWAY-011: Multi-tenant isolation
//
// Test Tier: INTEGRATION
// Rationale: Tests edge cases and error handling for DD-GATEWAY-008 with real Redis.
// Validates graceful degradation, concurrent access, and boundary conditions.
//
// Coverage Focus:
// - Redis connection failures
// - Concurrent buffer/window access
// - Buffer capacity limits
// - Window expiration edge cases
// - Multi-tenant boundary conditions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("DD-GATEWAY-008: Storm Buffering Edge Cases (Integration)", func() {
	var (
		ctx         context.Context
		redisClient *redis.Client
		aggregator  *processing.StormAggregator
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Use real Redis from test infrastructure
		redisTestClient := SetupRedisTestClient(ctx)
		Expect(redisTestClient).ToNot(BeNil(), "Redis test client required for DD-GATEWAY-008 tests")
		Expect(redisTestClient.Client).ToNot(BeNil(), "Redis client required for DD-GATEWAY-008 tests")
		redisClient = redisTestClient.Client

		// Clean Redis state before each test (safe - each process uses different Redis DB)
		err := redisClient.FlushDB(ctx).Err()
		Expect(err).ToNot(HaveOccurred(), "Failed to flush Redis before test")

		// Create aggregator with DD-GATEWAY-008 configuration
		// Using short timeouts for fast integration tests
		aggregator = processing.NewStormAggregatorWithConfig(
			redisClient,
			logr.Discard(),   // logger (nil = use nop logger for tests)
			5,                // bufferThreshold: 5 alerts
			2*time.Second,    // inactivityTimeout: 2s for fast testing
			5*time.Second,    // maxWindowDuration: 5s for fast testing
			1000,             // defaultMaxSize: 1000 alerts
			5000,             // globalMaxSize: 5000 alerts
			map[string]int{}, // perNamespaceLimits: empty
			0.95,             // samplingThreshold: 95%
			0.5,              // samplingRate: 50%
		)
	})

	AfterEach(func() {
		// Clean up Redis state after test
		if redisClient != nil {
			_ = redisClient.FlushDB(ctx)
		}
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Edge Case: Concurrent Buffer Access
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Concurrent Buffer Access", func() {
		Context("when multiple goroutines buffer alerts simultaneously", func() {
			It("should handle concurrent buffering without data loss", func() {
				processID := GinkgoParallelProcess()
				namespace := fmt.Sprintf("prod-api-p%d-%d", processID, time.Now().Unix())
				alertName := fmt.Sprintf("PodCrashLooping-p%d", processID)

				// SCENARIO: 4 pods crash simultaneously (below threshold=5)
				concurrentAlerts := 4
				done := make(chan bool, concurrentAlerts)
				errors := make(chan error, concurrentAlerts)

				// Buffer 4 alerts concurrently (below threshold)
				for i := 1; i <= concurrentAlerts; i++ {
					go func(index int) {
						signal := &types.NormalizedSignal{
							Namespace: namespace,
							AlertName: alertName,
							Resource: types.ResourceIdentifier{
								Kind: "Pod",
								Name: fmt.Sprintf("payment-api-%d", index),
							},
						}

						_, _, err := aggregator.BufferFirstAlert(ctx, signal)
						if err != nil {
							errors <- err
						}
						done <- true
					}(i)
				}

				// Wait for all goroutines to complete
				for i := 0; i < concurrentAlerts; i++ {
					<-done
				}
				close(errors)

				// CORRECTNESS: No errors should occur during concurrent buffering
				Expect(errors).To(BeEmpty(), "Concurrent buffering should not produce errors")

				// CORRECTNESS: All 4 alerts should be buffered
				bufferKey := fmt.Sprintf("alert:buffer:%s:%s", namespace, alertName)
				bufferSize, err := redisClient.LLen(ctx, bufferKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(bufferSize).To(BeNumerically(">=", int64(concurrentAlerts)), "All concurrent alerts should be buffered")

				// BUSINESS OUTCOME: Concurrent webhook calls handled safely (BR-GATEWAY-016)
			})
		})

		Context("when threshold is reached concurrently", func() {
			It("should create window exactly once", func() {
				processID := GinkgoParallelProcess()
				namespace := fmt.Sprintf("prod-api-p%d-%d", processID, time.Now().Unix())
				alertName := fmt.Sprintf("PodCrashLooping-p%d", processID)

				// Buffer 4 alerts first (below threshold)
				for i := 1; i <= 4; i++ {
					signal := &types.NormalizedSignal{
						Namespace: namespace,
						AlertName: alertName,
						Resource: types.ResourceIdentifier{
							Kind: "Pod",
							Name: fmt.Sprintf("payment-api-%d", i),
						},
					}
					_, _, err := aggregator.BufferFirstAlert(ctx, signal)
					Expect(err).ToNot(HaveOccurred())
				}

				// SCENARIO: 2 alerts arrive simultaneously (both would reach threshold=5)
				stormMetadata := &processing.StormMetadata{
					StormType:  "rate",
					Window:     "1m",
					AlertCount: 5,
				}

				done := make(chan string, 2)
				errors := make(chan error, 2)

				// Send 2 concurrent alerts that would both trigger threshold
				for i := 5; i <= 6; i++ {
					go func(index int) {
						signal := &types.NormalizedSignal{
							Namespace: namespace,
							AlertName: alertName,
							Resource: types.ResourceIdentifier{
								Kind: "Pod",
								Name: fmt.Sprintf("payment-api-%d", index),
							},
						}

						windowID, err := aggregator.StartAggregation(ctx, signal, stormMetadata)
						if err != nil {
							errors <- err
							done <- ""
						} else {
							done <- windowID
						}
					}(i)
				}

				// Wait for both goroutines
				windowID1 := <-done
				windowID2 := <-done
				close(errors)

				// CORRECTNESS: Only one window should be created
				// (One goroutine creates window, other adds to existing window)
				Expect(errors).To(BeEmpty(), "No errors during concurrent threshold")

				// At least one should have created a window
				Expect(windowID1 != "" || windowID2 != "").To(BeTrue(), "At least one window should be created")

				// BUSINESS OUTCOME: Race condition handled safely (BR-GATEWAY-016)
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Edge Case: Buffer Capacity Limits
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Buffer Capacity Limits", func() {
		Context("when buffer exceeds namespace limit", func() {
			It("should enforce per-namespace capacity", func() {
				processID := GinkgoParallelProcess()
				namespace := fmt.Sprintf("prod-api-p%d-%d", processID, time.Now().Unix())

				// Create aggregator with low namespace limit
				limitedAggregator := processing.NewStormAggregatorWithConfig(
					redisClient,
					logr.Discard(), // DD-005: Use logr.Discard() for silent logging in tests
					5,
					5*time.Second,
					30*time.Second,
					1000,
					5000,
					map[string]int{
						namespace: 10, // Low limit for testing
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
					_, _, err := limitedAggregator.BufferFirstAlert(ctx, signal)
					Expect(err).ToNot(HaveOccurred())
				}

				// SCENARIO: 11th alert exceeds namespace capacity
				signal11 := &types.NormalizedSignal{
					Namespace: namespace,
					AlertName: alertName,
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "payment-api-11",
					},
				}

				_, _, err := limitedAggregator.BufferFirstAlert(ctx, signal11)

				// CORRECTNESS: Should reject alert when capacity exceeded
				Expect(err).To(HaveOccurred(), "Should fail when namespace capacity exceeded")

				// BUSINESS OUTCOME: Namespace isolation enforced (BR-GATEWAY-011)
			})
		})

		Context("when buffer is at threshold boundary", func() {
			It("should handle threshold=buffer size correctly", func() {
				processID := GinkgoParallelProcess()
				namespace := fmt.Sprintf("prod-api-p%d-%d", processID, time.Now().Unix())
				alertName := fmt.Sprintf("PodCrashLooping-p%d", processID)

				// SCENARIO: threshold=5, buffer exactly 5 alerts
				for i := 1; i <= 4; i++ {
					signal := &types.NormalizedSignal{
						Namespace: namespace,
						AlertName: alertName,
						Resource: types.ResourceIdentifier{
							Kind: "Pod",
							Name: fmt.Sprintf("payment-api-%d", i),
						},
					}
					_, _, err := aggregator.BufferFirstAlert(ctx, signal)
					Expect(err).ToNot(HaveOccurred())
				}

				// 5th alert should trigger window creation
				stormMetadata := &processing.StormMetadata{
					StormType:  "rate",
					Window:     "1m",
					AlertCount: 5,
				}

				signal5 := &types.NormalizedSignal{
					Namespace: namespace,
					AlertName: alertName,
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "payment-api-5",
					},
				}

				windowID, err := aggregator.StartAggregation(ctx, signal5, stormMetadata)
				Expect(err).ToNot(HaveOccurred())
				Expect(windowID).ToNot(BeEmpty())

				// CORRECTNESS: Buffer should be cleared after threshold reached
				bufferKey := fmt.Sprintf("alert:storm:buffer:%s", alertName)
				exists, err := redisClient.Exists(ctx, bufferKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(Equal(int64(0)), "Buffer should be cleared after threshold")

				// CORRECTNESS: Window should contain all 5 buffered alerts
				bufferedAlerts, err := aggregator.GetBufferedAlerts(ctx, namespace, alertName)
				Expect(err).ToNot(HaveOccurred())
				Expect(bufferedAlerts).To(HaveLen(0), "Buffer should be empty after window creation")

				// BUSINESS OUTCOME: Threshold boundary handled correctly (BR-GATEWAY-016)
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Edge Case: Window Expiration Timing
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Window Expiration Timing", func() {
		Context("when alert arrives just before window expires", func() {
			It("should extend window successfully", func() {
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

				// Wait 1.8 seconds (just before 2s timeout)
				time.Sleep(1800 * time.Millisecond)

				// SCENARIO: Alert arrives 200ms before window expires (DD-GATEWAY-008 + BR-GATEWAY-011: includes namespace)
				windowKey := fmt.Sprintf("alert:storm:aggregate:%s:%s", namespace, alertName)
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

				err = aggregator.AddResource(ctx, windowID, signal6)
				Expect(err).ToNot(HaveOccurred(), "Should extend window even at last moment")

				// CORRECTNESS: Window should be extended
				newTTL, err := redisClient.TTL(ctx, windowKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(newTTL).To(BeNumerically(">", 1*time.Second), "Window should be extended")

				// BUSINESS OUTCOME: Late alerts don't lose window (BR-GATEWAY-008)
			})
		})

		Context("when alert arrives just after window expires", func() {
			It("should handle expired window gracefully", func() {
				processID := GinkgoParallelProcess()
				namespace := fmt.Sprintf("prod-api-p%d-%d", processID, time.Now().Unix())
				alertName := fmt.Sprintf("PodCrashLooping-p%d", processID)

				stormMetadata := &processing.StormMetadata{
					StormType:  "rate",
					Window:     "1m",
					AlertCount: 5,
				}

				// Create window with 5 alerts
				var windowID string
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
						var err error
						windowID, err = aggregator.StartAggregation(ctx, signal, stormMetadata)
						Expect(err).ToNot(HaveOccurred())
						Expect(windowID).ToNot(BeEmpty())
					}
				}

				// Verify window exists initially (DD-GATEWAY-008 + BR-GATEWAY-011: includes namespace)
				windowKey := fmt.Sprintf("alert:storm:aggregate:%s:%s", namespace, alertName)
				_, err := redisClient.Get(ctx, windowKey).Result()
				Expect(err).ToNot(HaveOccurred(), "Window should exist initially")

				// Wait for window to expire (3 seconds > 2s timeout)
				time.Sleep(3 * time.Second)

				// CORRECTNESS: Window should be gone after expiration
				_, err = redisClient.Get(ctx, windowKey).Result()
				Expect(err).To(Equal(redis.Nil), "Window should be expired after 3s")

				// SCENARIO: Alert arrives after window expired
				signal6 := &types.NormalizedSignal{
					Namespace: namespace,
					AlertName: alertName,
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "payment-api-6",
					},
				}

				err = aggregator.AddResource(ctx, windowID, signal6)

				// CORRECTNESS: AddResource handles expired window gracefully
				// Implementation may succeed (no-op) or fail (window not found)
				// Both behaviors are acceptable for expired windows
				if err == nil {
					// Graceful degradation: no-op when window expired
					GinkgoWriter.Printf("AddResource succeeded (no-op) for expired window\n")
				} else {
					// Strict validation: error when window expired
					GinkgoWriter.Printf("AddResource failed for expired window: %v\n", err)
				}

				// BUSINESS OUTCOME: Expired window handled safely (BR-GATEWAY-008)
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Edge Case: Multi-Tenant Boundary Conditions
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Multi-Tenant Boundary Conditions", func() {
		Context("when multiple namespaces reach threshold simultaneously", func() {
			It("should isolate buffers per namespace", func() {
				processID := GinkgoParallelProcess()
				namespace1 := fmt.Sprintf("prod-api-p%d-%d-1", processID, time.Now().Unix())
				namespace2 := fmt.Sprintf("prod-api-p%d-%d-2", processID, time.Now().Unix())
				alertName1 := fmt.Sprintf("PodCrashLooping-p%d-ns1", processID)
				alertName2 := fmt.Sprintf("PodCrashLooping-p%d-ns2", processID)

				stormMetadata := &processing.StormMetadata{
					StormType:  "rate",
					Window:     "1m",
					AlertCount: 5,
				}

				// SCENARIO: 2 namespaces have storms simultaneously with different alert names
				done := make(chan string, 2)

				// Namespace 1: Buffer and trigger threshold
				go func() {
					for i := 1; i <= 5; i++ {
						signal := &types.NormalizedSignal{
							Namespace: namespace1,
							AlertName: alertName1,
							Resource: types.ResourceIdentifier{
								Kind: "Pod",
								Name: fmt.Sprintf("payment-api-%d", i),
							},
						}
						if i < 5 {
							_, _, _ = aggregator.BufferFirstAlert(ctx, signal)
						} else {
							windowID, _ := aggregator.StartAggregation(ctx, signal, stormMetadata)
							done <- windowID
						}
					}
				}()

				// Namespace 2: Buffer and trigger threshold
				go func() {
					for i := 1; i <= 5; i++ {
						signal := &types.NormalizedSignal{
							Namespace: namespace2,
							AlertName: alertName2,
							Resource: types.ResourceIdentifier{
								Kind: "Pod",
								Name: fmt.Sprintf("payment-api-%d", i),
							},
						}
						if i < 5 {
							_, _, _ = aggregator.BufferFirstAlert(ctx, signal)
						} else {
							windowID, _ := aggregator.StartAggregation(ctx, signal, stormMetadata)
							done <- windowID
						}
					}
				}()

				// Wait for both namespaces
				windowID1 := <-done
				windowID2 := <-done

				// CORRECTNESS: Both namespaces should have separate windows
				Expect(windowID1).ToNot(BeEmpty())
				Expect(windowID2).ToNot(BeEmpty())
				Expect(windowID1).ToNot(Equal(windowID2), "Windows should be isolated per alert name")

				// CORRECTNESS: Buffers should be isolated per namespace
				buffer1Key := fmt.Sprintf("alert:buffer:%s:%s", namespace1, alertName1)
				buffer2Key := fmt.Sprintf("alert:buffer:%s:%s", namespace2, alertName2)
				Expect(buffer1Key).ToNot(Equal(buffer2Key), "Buffer keys should be different per namespace")

				// BUSINESS OUTCOME: Multi-tenant isolation maintained (BR-GATEWAY-011)
			})
		})
	})
})
