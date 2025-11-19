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
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/go-redis/redis/v8"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

func TestStormBufferEnhancement(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "StormBuffer Enhancement Unit Test Suite")
}

var _ = Describe("StormAggregator Enhancement - Strict TDD", func() {
	var (
		aggregator   *processing.StormAggregator
		redisServer  *miniredis.Miniredis
		redisClient  *goredis.Client
		ctx          context.Context
		testSettings *config.StormSettings
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create miniredis server for testing
		var err error
		redisServer, err = miniredis.Run()
		Expect(err).ToNot(HaveOccurred())

		redisClient = goredis.NewClient(&goredis.Options{
			Addr: redisServer.Addr(),
		})

		testSettings = &config.StormSettings{
			RateThreshold:     10,
			PatternThreshold:  5,
			AggregationWindow: 60 * time.Second,
		}

		// For now, use existing constructor
		aggregator = processing.NewStormAggregatorWithWindow(redisClient, testSettings.AggregationWindow)
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
			It("should return buffer count of 1 and not trigger aggregation", func() {
				signal := &types.NormalizedSignal{
					Namespace: "prod-api",
					AlertName: "PodCrashLooping",
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "payment-api-1",
					},
				}

				// BEHAVIOR: What does the system do?
				bufferSize, shouldAggregate, err := aggregator.BufferFirstAlert(ctx, signal)

				// CORRECTNESS: Are the results correct?
				Expect(err).ToNot(HaveOccurred())
				Expect(bufferSize).To(Equal(1), "First alert should result in buffer count of 1")
				Expect(shouldAggregate).To(BeFalse(), "Should NOT trigger aggregation below threshold")

			// BUSINESS OUTCOME: No CRD created yet (cost savings)
			// This validates BR-GATEWAY-016: Buffer alerts before aggregation
		})

		Context("when threshold alert arrives", func() {
			It("should return buffer count of 5 and trigger aggregation", func() {
				signal := &types.NormalizedSignal{
					Namespace: "prod-api",
					AlertName: "PodCrashLooping",
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "payment-api-1",
					},
				}

				// Buffer 5 alerts (threshold)
				for i := 1; i <= 5; i++ {
					signal.Resource.Name = fmt.Sprintf("payment-api-%d", i)
					bufferSize, shouldAggregate, err := aggregator.BufferFirstAlert(ctx, signal)
					Expect(err).ToNot(HaveOccurred())

					if i < 5 {
						// First 4 alerts: should NOT trigger aggregation
						Expect(shouldAggregate).To(BeFalse(), fmt.Sprintf("Alert %d should not trigger aggregation", i))
						Expect(bufferSize).To(Equal(i), fmt.Sprintf("Buffer size should be %d", i))
					} else {
						// 5th alert: SHOULD trigger aggregation
						Expect(shouldAggregate).To(BeTrue(), "5th alert should trigger aggregation")
						Expect(bufferSize).To(Equal(5), "Buffer size should be 5 at threshold")
					}
				}

				// BUSINESS OUTCOME: Aggregation triggered at threshold (BR-GATEWAY-016)
			})
		})
	})

	// TDD Cycle 3: ExtendWindow - Sliding Window Behavior
	Describe("ExtendWindow - Sliding Window (BR-GATEWAY-008)", func() {
		Context("when alert arrives during active window", func() {
			It("should reset the window expiration time", func() {
				windowID := "test-window-123"

				// Create a window in Redis with short TTL first
				windowKey := fmt.Sprintf("alert:storm:aggregate:PodCrashLooping")
				err := redisClient.Set(ctx, windowKey, windowID, 10*time.Second).Err()
				Expect(err).ToNot(HaveOccurred())

				// Get initial TTL (should be ~10s)
				initialTTL, err := redisClient.TTL(ctx, windowKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(initialTTL).To(BeNumerically("<=", 10*time.Second))

				// BEHAVIOR: Extend the window (should reset to 60s)
				newExpiration, err := aggregator.ExtendWindow(ctx, windowID, time.Now())

				// CORRECTNESS: Should succeed
				Expect(err).ToNot(HaveOccurred())
				Expect(newExpiration).ToNot(BeZero())

				// Verify TTL was reset to full window duration (60s > 10s)
				newTTL, err := redisClient.TTL(ctx, windowKey).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(newTTL).To(BeNumerically(">", 50*time.Second), "TTL should be reset to ~60s")

				// BUSINESS OUTCOME: Window stays open for ongoing storm (BR-GATEWAY-008)
			})
		})
	})
})
})

