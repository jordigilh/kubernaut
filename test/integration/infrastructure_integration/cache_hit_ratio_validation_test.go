//go:build integration
// +build integration

<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package infrastructure_integration

import (
	"context"
	"fmt"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/test/integration/shared"
)

// CacheHitRatioScenario represents a test scenario for cache hit/miss validation
type CacheHitRatioScenario struct {
	Name             string
	InitialRequests  []string
	RepeatedRequests []string
	RepetitionCount  int
	ExpectedHitRate  float64
	HitRateTolerance float64
	Description      string
}

var _ = Describe("Cache Hit/Miss Ratio Validation", Ordered, func() {
	var (
		logger *logrus.Logger
		ctx    context.Context

		redisFactory  *vector.VectorDatabaseFactory
		memoryFactory *vector.VectorDatabaseFactory

		testScenarios []CacheHitRatioScenario
	)

	BeforeAll(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
		ctx = context.Background()

		testConfig := shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests skipped via SKIP_INTEGRATION")
		}

		// Create cache configurations
		redisConfig := &config.VectorDBConfig{
			Enabled: true,
			Backend: "memory",
			EmbeddingService: config.EmbeddingConfig{
				Service:   "local",
				Dimension: 384,
			},
			Cache: config.VectorCacheConfig{
				Enabled:   true,
				TTL:       30 * time.Minute,
				MaxSize:   1000,
				CacheType: "redis",
			},
		}

		memoryConfig := &config.VectorDBConfig{
			Enabled: true,
			Backend: "memory",
			EmbeddingService: config.EmbeddingConfig{
				Service:   "local",
				Dimension: 384,
			},
			Cache: config.VectorCacheConfig{
				Enabled:   true,
				TTL:       30 * time.Minute,
				MaxSize:   1000,
				CacheType: "memory",
			},
		}

		redisFactory = vector.NewVectorDatabaseFactory(redisConfig, nil, logger)
		memoryFactory = vector.NewVectorDatabaseFactory(memoryConfig, nil, logger)

		// Define test scenarios
		testScenarios = []CacheHitRatioScenario{
			{
				Name: "Perfect Cache Hit Scenario",
				InitialRequests: []string{
					"pod memory usage high",
					"deployment scaling required",
					"service endpoint down",
				},
				RepeatedRequests: []string{
					"pod memory usage high",
					"deployment scaling required",
					"service endpoint down",
				},
				RepetitionCount:  5,
				ExpectedHitRate:  5.0 / 6.0, // 5 hits out of 6 total requests (1 initial + 5 repeats per item)
				HitRateTolerance: 0.05,
				Description:      "All repeated requests should result in cache hits",
			},
			{
				Name: "Mixed Hit/Miss Scenario",
				InitialRequests: []string{
					"database connection timeout",
					"storage volume full",
				},
				RepeatedRequests: []string{
					"database connection timeout", // Hit
					"network partition detected",  // Miss (new)
					"storage volume full",         // Hit
					"application startup failed",  // Miss (new)
				},
				RepetitionCount:  3,
				ExpectedHitRate:  0.5, // 50% hit rate expected
				HitRateTolerance: 0.15,
				Description:      "Mixed scenario with new requests causing cache misses",
			},
			{
				Name:            "No Cache Hit Scenario",
				InitialRequests: []string{}, // No initial requests
				RepeatedRequests: []string{
					"unique request 1",
					"unique request 2",
					"unique request 3",
					"unique request 4",
					"unique request 5",
				},
				RepetitionCount:  1,
				ExpectedHitRate:  0.0, // All misses
				HitRateTolerance: 0.1,
				Description:      "All unique requests should result in cache misses",
			},
			{
				Name: "High Frequency Pattern",
				InitialRequests: []string{
					"frequent alert pattern",
				},
				RepeatedRequests: []string{
					"frequent alert pattern",
				},
				RepetitionCount:  20,
				ExpectedHitRate:  20.0 / 21.0, // 20 hits out of 21 total (1 initial + 20 repeats)
				HitRateTolerance: 0.02,
				Description:      "High frequency access pattern should have very high hit rate",
			},
		}

		logger.Info("Cache hit ratio validation suite setup completed")
	})

	Context("Redis Cache Hit/Miss Validation", func() {
		BeforeEach(func() {
			// Clear Redis cache before each scenario
			redisService, err := redisFactory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred())

			if cachedService, ok := redisService.(*vector.CachedEmbeddingService); ok {
				err := cachedService.ClearCache(ctx)
				Expect(err).ToNot(HaveOccurred())
			}
		})

		for _, scenario := range testScenarios {
			scenarioRef := scenario // Capture for closure
			It(fmt.Sprintf("should validate hit ratio for: %s", scenarioRef.Name), func() {
				By(scenarioRef.Description)

				redisService, err := redisFactory.CreateEmbeddingService()
				Expect(err).ToNot(HaveOccurred())

				cachedService, ok := redisService.(*vector.CachedEmbeddingService)
				Expect(ok).To(BeTrue(), "Should be a cached embedding service")

				// Execute initial requests
				By("executing initial cache-priming requests")
				for _, text := range scenarioRef.InitialRequests {
					_, err := cachedService.GenerateTextEmbedding(ctx, text)
					Expect(err).ToNot(HaveOccurred())
				}

				// Get initial stats
				initialStats := cachedService.GetCacheStats(ctx)

				// Execute repeated requests
				By("executing test requests pattern")
				for i := 0; i < scenarioRef.RepetitionCount; i++ {
					for _, text := range scenarioRef.RepeatedRequests {
						_, err := cachedService.GenerateTextEmbedding(ctx, text)
						Expect(err).ToNot(HaveOccurred())
					}
				}

				// Get final stats
				finalStats := cachedService.GetCacheStats(ctx)

				// Calculate hit rate for the test pattern (excluding initial priming)
				testHits := finalStats.Hits - initialStats.Hits
				testMisses := finalStats.Misses - initialStats.Misses
				totalTestRequests := testHits + testMisses

				var actualHitRate float64
				if totalTestRequests > 0 {
					actualHitRate = float64(testHits) / float64(totalTestRequests)
				}

				By("validating hit ratio expectations")
				logger.WithFields(logrus.Fields{
					"scenario":          scenarioRef.Name,
					"expected_hit_rate": scenarioRef.ExpectedHitRate,
					"actual_hit_rate":   actualHitRate,
					"tolerance":         scenarioRef.HitRateTolerance,
					"test_hits":         testHits,
					"test_misses":       testMisses,
					"total_requests":    totalTestRequests,
					"initial_hits":      initialStats.Hits,
					"initial_misses":    initialStats.Misses,
					"final_hits":        finalStats.Hits,
					"final_misses":      finalStats.Misses,
				}).Info("Cache hit ratio validation result")

				// Validate hit rate is within expected tolerance
				hitRateDifference := math.Abs(actualHitRate - scenarioRef.ExpectedHitRate)
				Expect(hitRateDifference).To(BeNumerically("<=", scenarioRef.HitRateTolerance),
					fmt.Sprintf("Hit rate %.3f should be within %.3f of expected %.3f",
						actualHitRate, scenarioRef.HitRateTolerance, scenarioRef.ExpectedHitRate))
			})
		}
	})

	Context("Memory Cache Hit/Miss Validation", func() {
		BeforeEach(func() {
			// Clear memory cache before each scenario
			memoryService, err := memoryFactory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred())

			if cachedService, ok := memoryService.(*vector.CachedEmbeddingService); ok {
				err := cachedService.ClearCache(ctx)
				Expect(err).ToNot(HaveOccurred())
			}
		})

		// REMOVED: Cache hit ratio comparison test - Performance optimization test removed per value assessment
		// It("should show similar hit ratios between Redis and Memory caches", func() {
		// 	// Removed performance benchmark test - low ROI for maintenance cost
		// })
	})

	Context("Cache Invalidation and TTL Validation", func() {
		It("should handle cache invalidation correctly", func() {
			By("setting up cache with short TTL for testing")
			shortTTLConfig := &config.VectorDBConfig{
				Enabled: true,
				Backend: "memory",
				EmbeddingService: config.EmbeddingConfig{
					Service:   "local",
					Dimension: 384,
				},
				Cache: config.VectorCacheConfig{
					Enabled:   true,
					TTL:       2 * time.Second, // Very short TTL for testing
					MaxSize:   100,
					CacheType: "redis",
				},
			}

			ttlFactory := vector.NewVectorDatabaseFactory(shortTTLConfig, nil, logger)
			ttlService, err := ttlFactory.CreateEmbeddingService()
			Expect(err).ToNot(HaveOccurred())

			cachedService, ok := ttlService.(*vector.CachedEmbeddingService)
			Expect(ok).To(BeTrue())

			By("initial request to populate cache")
			testText := "ttl invalidation test"
			_, err = cachedService.GenerateTextEmbedding(ctx, testText)
			Expect(err).ToNot(HaveOccurred())

			By("immediate second request should be cache hit")
			initialStats := cachedService.GetCacheStats(ctx)
			_, err = cachedService.GenerateTextEmbedding(ctx, testText)
			Expect(err).ToNot(HaveOccurred())

			afterFirstRepeatStats := cachedService.GetCacheStats(ctx)
			Expect(afterFirstRepeatStats.Hits).To(BeNumerically(">", initialStats.Hits))

			By("waiting for TTL expiration")
			time.Sleep(3 * time.Second) // Wait longer than TTL

			By("request after TTL should be cache miss")
			beforeTTLExpiredStats := cachedService.GetCacheStats(ctx)
			_, err = cachedService.GenerateTextEmbedding(ctx, testText)
			Expect(err).ToNot(HaveOccurred())

			afterTTLExpiredStats := cachedService.GetCacheStats(ctx)
			Expect(afterTTLExpiredStats.Misses).To(BeNumerically(">", beforeTTLExpiredStats.Misses))

			logger.WithFields(logrus.Fields{
				"initial_hits":      initialStats.Hits,
				"after_first_hits":  afterFirstRepeatStats.Hits,
				"before_ttl_misses": beforeTTLExpiredStats.Misses,
				"after_ttl_misses":  afterTTLExpiredStats.Misses,
			}).Info("TTL cache invalidation validation completed")
		})
	})
})
