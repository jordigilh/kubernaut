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
		})
	})
})

