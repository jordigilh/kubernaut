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
	"context"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// DD-GATEWAY-008: Storm Aggregation First-Alert Handling - TDD Unit Tests
//
// Business Requirement: BR-GATEWAY-016 - Storm aggregation must reduce AI analysis costs by 90%+
//
// Design Decision: DD-GATEWAY-008 Alternative 2 - Buffered First-Alert Aggregation
// Source: docs/architecture/decisions/DD-GATEWAY-008-storm-aggregation-first-alert-handling.md
//
// APPROVED BEHAVIOR (Lines 99-146):
// 1. Buffer first N alerts in Redis (no CRD creation)
// 2. When threshold reached, create aggregated CRD with ALL buffered alerts
// 3. Return HTTP 201 with CRD name when threshold reached
// 4. Continue adding alerts to window (sliding window behavior)
//
// TDD Strategy: RED → GREEN → REFACTOR
// - RED: Write tests defining correct DD-GATEWAY-008 behavior (these will FAIL initially)
// - GREEN: Modify Gateway to pass tests (minimal implementation)
// - REFACTOR: Optimize and clean up implementation

var _ = Describe("DD-GATEWAY-008: Storm Aggregation First-Alert Handling", func() {
	var (
		ctx             context.Context
		stormAggregator *processing.StormAggregator
		redisServer     *miniredis.Miniredis
		redisClient     *redis.Client
		signal          *types.NormalizedSignal
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Setup miniredis (in-memory Redis server)
		var err error
		redisServer, err = miniredis.Run()
		Expect(err).NotTo(HaveOccurred(), "miniredis should start successfully")

		// Create real Redis client pointing to miniredis
		redisClient = redis.NewClient(&redis.Options{
			Addr: redisServer.Addr(),
		})

		// DD-GATEWAY-008: Use buffer_threshold: 2 for testing
		stormAggregator = processing.NewStormAggregatorWithConfig(
			redisClient,
			logr.Discard(), // logger
			2,              // bufferThreshold: 2 alerts
			5*time.Second,  // inactivityTimeout: 5s
			30*time.Second, // maxWindowDuration: 30s
			1000,           // defaultMaxSize
			5000,           // globalMaxSize
			nil,            // perNamespaceLimits
			0.95,           // samplingThreshold
			0.5,            // samplingRate
		)

		signal = &types.NormalizedSignal{
			AlertName: "PodCrashLooping",
			Namespace: "production",
			Resource: types.ResourceIdentifier{
				Namespace: "production",
				Kind:      "Pod",
				Name:      "app-pod-1",
			},
			Fingerprint: "test-fingerprint-1",
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
	// TEST 1: Buffer First Alert (Below Threshold)
	// ========================================
	// DD-GATEWAY-008 Expected Behavior:
	// - Alert 1: Buffer in Redis, return empty windowID
	// - No CRD created yet
	// - HTTP 202 Accepted
	Context("when first alert arrives (below threshold)", func() {
		It("should buffer alert and return empty windowID", func() {
			stormMetadata := &processing.StormMetadata{
				StormType:  "rate",
				AlertCount: 1,
				Window:     "1m",
			}

			// Call StartAggregation with first alert
			windowID, err := stormAggregator.StartAggregation(ctx, signal, stormMetadata)

			// Assertions per DD-GATEWAY-008
			Expect(err).ToNot(HaveOccurred())
			Expect(windowID).To(BeEmpty(), "First alert should be buffered (no window created yet)")

			// Verify alert was buffered in Redis
			bufferKey := "alert:buffer:production:PodCrashLooping"
			bufferSize, err := redisClient.LLen(ctx, bufferKey).Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(bufferSize).To(Equal(int64(1)), "Buffer should contain 1 alert")
		})
	})

	// ========================================
	// TEST 2: Create CRD When Threshold Reached
	// ========================================
	// DD-GATEWAY-008 Expected Behavior:
	// - Alert 2: Threshold reached (2 >= 2)
	// - Create aggregated CRD with ALL buffered alerts (Alert 1 + Alert 2)
	// - Return windowID (non-empty)
	// - HTTP 201 Created
	Context("when threshold is reached", func() {
		It("should create aggregation window and return windowID", func() {
			stormMetadata := &processing.StormMetadata{
				StormType:  "rate",
				AlertCount: 2,
				Window:     "1m",
			}

			// Send first alert (buffered)
			windowID1, err := stormAggregator.StartAggregation(ctx, signal, stormMetadata)
			Expect(err).ToNot(HaveOccurred())
			Expect(windowID1).To(BeEmpty(), "First alert should be buffered")

			// Send second alert (threshold reached)
			signal2 := &types.NormalizedSignal{
				AlertName: "PodCrashLooping",
				Namespace: "production",
				Resource: types.ResourceIdentifier{
					Namespace: "production",
					Kind:      "Pod",
					Name:      "app-pod-2", // Different pod
				},
				Fingerprint: "test-fingerprint-2",
			}

			windowID2, err := stormAggregator.StartAggregation(ctx, signal2, stormMetadata)

			// Assertions per DD-GATEWAY-008
			Expect(err).ToNot(HaveOccurred())
			Expect(windowID2).ToNot(BeEmpty(), "Threshold reached - window should be created")
			Expect(windowID2).To(MatchRegexp("^production:PodCrashLooping-\\d+$"), "WindowID should follow format: Namespace:AlertName-Timestamp")

			// Verify window exists in Redis (DD-GATEWAY-008 + BR-GATEWAY-011: includes namespace)
			windowKey := "alert:storm:aggregate:production:PodCrashLooping"
			windowExists, err := redisClient.Exists(ctx, windowKey).Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(windowExists).To(Equal(int64(1)), "Aggregation window should exist in Redis")
		})

		It("should include ALL buffered alerts in the window", func() {
			stormMetadata := &processing.StormMetadata{
				StormType:  "rate",
				AlertCount: 2,
				Window:     "1m",
			}

			// Send first alert (buffered)
			_, err := stormAggregator.StartAggregation(ctx, signal, stormMetadata)
			Expect(err).ToNot(HaveOccurred())

			// Send second alert (threshold reached)
			signal2 := &types.NormalizedSignal{
				AlertName: "PodCrashLooping",
				Namespace: "production",
				Resource: types.ResourceIdentifier{
					Namespace: "production",
					Kind:      "Pod",
					Name:      "app-pod-2",
				},
				Fingerprint: "test-fingerprint-2",
			}

			windowID, err := stormAggregator.StartAggregation(ctx, signal2, stormMetadata)
			Expect(err).ToNot(HaveOccurred())
			Expect(windowID).ToNot(BeEmpty())

			// Verify window contains BOTH alerts
			resourceCount, err := stormAggregator.GetResourceCount(ctx, windowID)
			Expect(err).ToNot(HaveOccurred())
			Expect(resourceCount).To(Equal(2), "Window should contain both buffered alerts")
		})
	})

	// ========================================
	// TEST 3: Add Alerts to Existing Window
	// ========================================
	// DD-GATEWAY-008 Expected Behavior:
	// - Alert 3+: Add to existing window
	// - Window already created (threshold reached on Alert 2)
	// - HTTP 202 Accepted
	Context("when alerts arrive after window created", func() {
		It("should add alerts to existing window", func() {
			stormMetadata := &processing.StormMetadata{
				StormType:  "rate",
				AlertCount: 3,
				Window:     "1m",
			}

			// Send first two alerts (create window)
			_, _ = stormAggregator.StartAggregation(ctx, signal, stormMetadata)

			signal2 := &types.NormalizedSignal{
				AlertName:   "PodCrashLooping",
				Namespace:   "production",
				Resource:    types.ResourceIdentifier{Namespace: "production", Kind: "Pod", Name: "app-pod-2"},
				Fingerprint: "test-fingerprint-2",
			}
			windowID, err := stormAggregator.StartAggregation(ctx, signal2, stormMetadata)
			Expect(err).ToNot(HaveOccurred())
			Expect(windowID).ToNot(BeEmpty())

			// Send third alert (add to window)
			signal3 := &types.NormalizedSignal{
				AlertName:   "PodCrashLooping",
				Namespace:   "production",
				Resource:    types.ResourceIdentifier{Namespace: "production", Kind: "Pod", Name: "app-pod-3"},
				Fingerprint: "test-fingerprint-3",
			}

			err = stormAggregator.AddResource(ctx, windowID, signal3)
			Expect(err).ToNot(HaveOccurred())

			// Verify window now contains 3 alerts
			resourceCount, err := stormAggregator.GetResourceCount(ctx, windowID)
			Expect(err).ToNot(HaveOccurred())
			Expect(resourceCount).To(Equal(3), "Window should contain all 3 alerts")
		})
	})

	// ========================================
	// TEST 4: Sliding Window Behavior
	// ========================================
	// DD-GATEWAY-008 Expected Behavior:
	// - Each new alert resets inactivity timeout
	// - Window closes after inactivity_timeout with no new alerts
	Context("when testing sliding window behavior", func() {
		It("should extend window TTL on each new alert", func() {
			stormMetadata := &processing.StormMetadata{
				StormType:  "rate",
				AlertCount: 2,
				Window:     "1m",
			}

			// Create window (send 2 alerts)
			_, _ = stormAggregator.StartAggregation(ctx, signal, stormMetadata)
			signal2 := &types.NormalizedSignal{
				AlertName:   "PodCrashLooping",
				Namespace:   "production",
				Resource:    types.ResourceIdentifier{Namespace: "production", Kind: "Pod", Name: "app-pod-2"},
				Fingerprint: "test-fingerprint-2",
			}
			windowID, err := stormAggregator.StartAggregation(ctx, signal2, stormMetadata)
			Expect(err).ToNot(HaveOccurred())

			// Get initial TTL
			windowKey := "alert:storm:aggregate:PodCrashLooping"
			initialTTL, err := redisClient.TTL(ctx, windowKey).Result()
			Expect(err).ToNot(HaveOccurred())

			// Wait 2 seconds
			time.Sleep(2 * time.Second)

			// Add another alert (should reset TTL)
			signal3 := &types.NormalizedSignal{
				AlertName:   "PodCrashLooping",
				Namespace:   "production",
				Resource:    types.ResourceIdentifier{Namespace: "production", Kind: "Pod", Name: "app-pod-3"},
				Fingerprint: "test-fingerprint-3",
			}
			err = stormAggregator.AddResource(ctx, windowID, signal3)
			Expect(err).ToNot(HaveOccurred())

			// Get new TTL (should be reset to ~5s)
			newTTL, err := redisClient.TTL(ctx, windowKey).Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(newTTL).To(BeNumerically(">", initialTTL-2*time.Second), "TTL should be reset (sliding window)")
		})
	})

	// ========================================
	// TEST 5: GetBufferedAlerts Method
	// ========================================
	// DD-GATEWAY-008 Required Method:
	// - Retrieve ALL buffered alerts for CRD creation
	// - Used by Gateway to create aggregated CRD
	Context("when retrieving buffered alerts", func() {
		It("should return all buffered alerts before threshold", func() {
			stormMetadata := &processing.StormMetadata{
				StormType:  "rate",
				AlertCount: 1,
				Window:     "1m",
			}

			// Buffer first alert
			_, err := stormAggregator.StartAggregation(ctx, signal, stormMetadata)
			Expect(err).ToNot(HaveOccurred())

			// Retrieve buffered alerts
			bufferedAlerts, err := stormAggregator.GetBufferedAlerts(ctx, signal.Namespace, signal.AlertName)
			Expect(err).ToNot(HaveOccurred())
			Expect(bufferedAlerts).To(HaveLen(1), "Should return 1 buffered alert")
			Expect(bufferedAlerts[0].Resource.Name).To(Equal("app-pod-1"))
		})

		It("should clear buffer after threshold reached (alerts moved to window)", func() {
			stormMetadata := &processing.StormMetadata{
				StormType:  "rate",
				AlertCount: 2,
				Window:     "1m",
			}

			// Buffer first alert
			_, _ = stormAggregator.StartAggregation(ctx, signal, stormMetadata)

			// Buffer second alert (threshold reached)
			signal2 := &types.NormalizedSignal{
				AlertName:   "PodCrashLooping",
				Namespace:   "production",
				Resource:    types.ResourceIdentifier{Namespace: "production", Kind: "Pod", Name: "app-pod-2"},
				Fingerprint: "test-fingerprint-2",
			}
			windowID, err := stormAggregator.StartAggregation(ctx, signal2, stormMetadata)
			Expect(err).ToNot(HaveOccurred())
			Expect(windowID).ToNot(BeEmpty())

			// DD-GATEWAY-008: Buffer should be cleared (alerts moved to window)
			bufferedAlerts, err := stormAggregator.GetBufferedAlerts(ctx, signal.Namespace, signal.AlertName)
			Expect(err).ToNot(HaveOccurred())
			Expect(bufferedAlerts).To(BeEmpty(), "Buffer should be cleared after threshold reached")

			// Verify alerts are in the window instead
			resourceCount, err := stormAggregator.GetResourceCount(ctx, windowID)
			Expect(err).ToNot(HaveOccurred())
			Expect(resourceCount).To(Equal(2), "Window should contain both alerts")
		})
	})
})
