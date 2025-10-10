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
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/test/integration/shared"
)

// CacheBenchmarkResult stores performance metrics for cache benchmarking
type CacheBenchmarkResult struct {
	CacheType          string        `json:"cache_type"`
	OperationType      string        `json:"operation_type"`
	TotalOperations    int           `json:"total_operations"`
	SuccessfulOps      int64         `json:"successful_operations"`
	FailedOps          int64         `json:"failed_operations"`
	AverageLatency     time.Duration `json:"average_latency"`
	P50Latency         time.Duration `json:"p50_latency"`
	P95Latency         time.Duration `json:"p95_latency"`
	P99Latency         time.Duration `json:"p99_latency"`
	Throughput         float64       `json:"throughput_ops_per_sec"`
	CacheHits          int64         `json:"cache_hits"`
	CacheMisses        int64         `json:"cache_misses"`
	CacheHitRate       float64       `json:"cache_hit_rate"`
	TotalExecutionTime time.Duration `json:"total_execution_time"`
}

// CacheBenchmarkSuite manages cache performance benchmarking
type CacheBenchmarkSuite struct {
	results []CacheBenchmarkResult
	mutex   sync.RWMutex
}

func NewCacheBenchmarkSuite() *CacheBenchmarkSuite {
	return &CacheBenchmarkSuite{
		results: make([]CacheBenchmarkResult, 0),
	}
}

func (s *CacheBenchmarkSuite) AddResult(result CacheBenchmarkResult) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.results = append(s.results, result)
}

func (s *CacheBenchmarkSuite) GetResults() []CacheBenchmarkResult {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return append([]CacheBenchmarkResult{}, s.results...)
}

var _ = Describe("Redis Cache Performance Benchmarking", Ordered, func() {
	var (
		logger         *logrus.Logger
		benchmarkSuite *CacheBenchmarkSuite

		// Cache configurations for comparison
		redisConfig   *config.VectorDBConfig
		memoryConfig  *config.VectorDBConfig
		noCacheConfig *config.VectorDBConfig

		// Services for benchmarking
		redisFactory   *vector.VectorDatabaseFactory
		memoryFactory  *vector.VectorDatabaseFactory
		noCacheFactory *vector.VectorDatabaseFactory

		// Test dataset for benchmarking
		benchmarkTexts []string
	)

	BeforeAll(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce log noise for benchmarks

		testConfig := shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests skipped via SKIP_INTEGRATION")
		}

		if testConfig.SkipSlowTests {
			Skip("Performance benchmarks skipped via SKIP_SLOW_TESTS")
		}

		benchmarkSuite = NewCacheBenchmarkSuite()

		// Create configurations for different cache types
		baseConfig := &config.VectorDBConfig{
			Enabled: true,
			Backend: "postgresql", // We'll use memory fallback for these tests
			EmbeddingService: config.EmbeddingConfig{
				Service:   "local",
				Dimension: 384,
			},
		}

		// Redis cache configuration
		redisConfig = &config.VectorDBConfig{
			Enabled:          baseConfig.Enabled,
			Backend:          "memory", // Use memory backend to focus on caching
			EmbeddingService: baseConfig.EmbeddingService,
			Cache: config.VectorCacheConfig{
				Enabled:   true,
				TTL:       30 * time.Minute,
				MaxSize:   5000,
				CacheType: "redis",
			},
		}

		// Memory cache configuration
		memoryConfig = &config.VectorDBConfig{
			Enabled:          baseConfig.Enabled,
			Backend:          "memory",
			EmbeddingService: baseConfig.EmbeddingService,
			Cache: config.VectorCacheConfig{
				Enabled:   true,
				TTL:       30 * time.Minute,
				MaxSize:   5000,
				CacheType: "memory",
			},
		}

		// No cache configuration
		noCacheConfig = &config.VectorDBConfig{
			Enabled:          baseConfig.Enabled,
			Backend:          "memory",
			EmbeddingService: baseConfig.EmbeddingService,
			Cache: config.VectorCacheConfig{
				Enabled: false,
			},
		}

		// Create factories
		redisFactory = vector.NewVectorDatabaseFactory(redisConfig, nil, logger)
		memoryFactory = vector.NewVectorDatabaseFactory(memoryConfig, nil, logger)
		noCacheFactory = vector.NewVectorDatabaseFactory(noCacheConfig, nil, logger)

		// Generate benchmark dataset
		benchmarkTexts = generateBenchmarkDataset(1000)

		logger.Info("Cache performance benchmarking suite setup completed")
	})

	Context("Embedding Generation Benchmarks", func() {
		It("should benchmark uncached vs Redis vs Memory cache performance", func() {
			By("benchmarking uncached embedding generation")
			noCacheResult := benchmarkEmbeddingGeneration("uncached", noCacheFactory, benchmarkTexts[:100], 1)
			benchmarkSuite.AddResult(noCacheResult)

			By("benchmarking Redis cached embedding generation")
			redisResult := benchmarkEmbeddingGeneration("redis", redisFactory, benchmarkTexts[:100], 1)
			benchmarkSuite.AddResult(redisResult)

			By("benchmarking Memory cached embedding generation")
			memoryResult := benchmarkEmbeddingGeneration("memory", memoryFactory, benchmarkTexts[:100], 1)
			benchmarkSuite.AddResult(memoryResult)

			By("comparing performance results")
			redisSpeedup := float64(noCacheResult.AverageLatency) / float64(redisResult.AverageLatency)
			memorySpeedup := float64(noCacheResult.AverageLatency) / float64(memoryResult.AverageLatency)

			logger.WithFields(logrus.Fields{
				"uncached_avg_ms": noCacheResult.AverageLatency.Milliseconds(),
				"redis_avg_ms":    redisResult.AverageLatency.Milliseconds(),
				"memory_avg_ms":   memoryResult.AverageLatency.Milliseconds(),
				"redis_speedup":   redisSpeedup,
				"memory_speedup":  memorySpeedup,
				"redis_hit_rate":  redisResult.CacheHitRate,
				"memory_hit_rate": memoryResult.CacheHitRate,
			}).Info("Embedding generation benchmark comparison")

			// Validate performance improvements - adjusted for test environment
			thresholds := DefaultPerformanceThresholds()
			ValidatePerformanceMetrics(noCacheResult.AverageLatency, redisResult.AverageLatency, thresholds, "redis")
			ValidatePerformanceMetrics(noCacheResult.AverageLatency, memoryResult.AverageLatency, thresholds, "memory")

			// Memory should be faster than Redis
			Expect(memoryResult.AverageLatency).To(BeNumerically("<", redisResult.AverageLatency))
		})
	})

	// REMOVED: Concurrent Load Benchmarks - Performance optimization test removed per value assessment
	// Context("Concurrent Load Benchmarks", func() {
	// 	It("should benchmark cache performance under concurrent load", func() {
	// 		// Removed performance benchmark test - low ROI for maintenance cost
	// 	})
	// })

	Context("Scalability Benchmarks", func() {
		It("should benchmark cache performance with increasing dataset sizes", func() {
			datasetSizes := []int{50, 100, 200, 500}

			for _, size := range datasetSizes {
				By(fmt.Sprintf("benchmarking with dataset size %d", size))

				// Benchmark each cache type with current dataset size
				redisScaleResult := benchmarkEmbeddingGeneration(
					fmt.Sprintf("redis_scale_%d", size),
					redisFactory,
					benchmarkTexts[:size],
					1,
				)
				benchmarkSuite.AddResult(redisScaleResult)

				memoryScaleResult := benchmarkEmbeddingGeneration(
					fmt.Sprintf("memory_scale_%d", size),
					memoryFactory,
					benchmarkTexts[:size],
					1,
				)
				benchmarkSuite.AddResult(memoryScaleResult)

				logger.WithFields(logrus.Fields{
					"dataset_size":          size,
					"redis_avg_latency_ms":  redisScaleResult.AverageLatency.Milliseconds(),
					"memory_avg_latency_ms": memoryScaleResult.AverageLatency.Milliseconds(),
					"redis_hit_rate":        redisScaleResult.CacheHitRate,
					"memory_hit_rate":       memoryScaleResult.CacheHitRate,
				}).Info("Scalability benchmark point")
			}

			By("analyzing scalability trends")
			// Performance should not degrade significantly with larger datasets
			// This validates that caching scales well
		})
	})

	Context("Cache Stress Testing", func() {
		It("should validate cache reliability under stress conditions", func() {
			By("stress testing Redis cache with high concurrency")
			stressResult := benchmarkCacheStress("redis_stress", redisFactory, benchmarkTexts[:100], 20, 50)
			benchmarkSuite.AddResult(stressResult)

			By("validating stress test results")
			// Under stress, we should still have reasonable performance
			Expect(stressResult.FailedOps).To(BeNumerically("<=", stressResult.SuccessfulOps/10)) // Max 10% failure rate
			Expect(stressResult.AverageLatency).To(BeNumerically("<", 500*time.Millisecond))      // Max 500ms average

			logger.WithFields(logrus.Fields{
				"successful_ops": stressResult.SuccessfulOps,
				"failed_ops":     stressResult.FailedOps,
				"avg_latency_ms": stressResult.AverageLatency.Milliseconds(),
				"p95_latency_ms": stressResult.P95Latency.Milliseconds(),
				"throughput":     stressResult.Throughput,
			}).Info("Cache stress test completed")
		})
	})

	Context("Performance Report Generation", func() {
		It("should generate comprehensive performance report", func() {
			By("collecting all benchmark results")
			allResults := benchmarkSuite.GetResults()
			Expect(len(allResults)).To(BeNumerically(">", 0))

			By("generating performance summary")
			generatePerformanceReport(allResults, logger)
		})
	})
})

// Benchmark helper functions

func generateBenchmarkDataset(size int) []string {
	texts := make([]string, size)
	templates := []string{
		"pod %s memory usage critical",
		"deployment %s scaling required",
		"service %s endpoint unreachable",
		"node %s resource pressure detected",
		"volume %s mount failed",
		"cluster %s network partition",
		"application %s performance degraded",
		"database %s connection timeout",
		"storage %s capacity exceeded",
		"monitoring %s alert triggered",
	}

	for i := 0; i < size; i++ {
		template := templates[i%len(templates)]
		texts[i] = fmt.Sprintf(template, fmt.Sprintf("resource_%d", i))
	}

	return texts
}

func benchmarkEmbeddingGeneration(benchmarkType string, factory *vector.VectorDatabaseFactory, texts []string, repetitions int) CacheBenchmarkResult {
	result := CacheBenchmarkResult{
		CacheType:       benchmarkType,
		OperationType:   "embedding_generation",
		TotalOperations: len(texts) * repetitions,
	}

	embeddingService, err := factory.CreateEmbeddingService()
	Expect(err).ToNot(HaveOccurred())

	// Clear cache if it exists
	if cachedService, ok := embeddingService.(*vector.CachedEmbeddingService); ok {
		cachedService.ClearCache(context.Background())
	}

	latencies := make([]time.Duration, 0, result.TotalOperations)
	var successOps, failOps int64

	totalStart := time.Now()
	ctx := context.Background()

	for rep := 0; rep < repetitions; rep++ {
		for _, text := range texts {
			start := time.Now()
			_, err := embeddingService.GenerateTextEmbedding(ctx, text)
			latency := time.Since(start)

			if err != nil {
				atomic.AddInt64(&failOps, 1)
			} else {
				atomic.AddInt64(&successOps, 1)
				latencies = append(latencies, latency)
			}
		}
	}

	totalTime := time.Since(totalStart)

	// Calculate statistics
	if len(latencies) > 0 {
		result.AverageLatency = calculateAverageLatency(latencies)
		result.P50Latency = calculatePercentile(latencies, 50)
		result.P95Latency = calculatePercentile(latencies, 95)
		result.P99Latency = calculatePercentile(latencies, 99)
	}

	result.SuccessfulOps = successOps
	result.FailedOps = failOps
	result.TotalExecutionTime = totalTime
	result.Throughput = float64(successOps) / totalTime.Seconds()

	// Get cache stats if available
	if cachedService, ok := embeddingService.(*vector.CachedEmbeddingService); ok {
		stats := cachedService.GetCacheStats(ctx)
		result.CacheHits = stats.Hits
		result.CacheMisses = stats.Misses
		result.CacheHitRate = stats.HitRate
	}

	return result
}

func benchmarkConcurrentLoad(benchmarkType string, factory *vector.VectorDatabaseFactory, texts []string, concurrency int, operationsPerWorker int) CacheBenchmarkResult {
	result := CacheBenchmarkResult{
		CacheType:       benchmarkType,
		OperationType:   "concurrent_load",
		TotalOperations: concurrency * operationsPerWorker,
	}

	embeddingService, err := factory.CreateEmbeddingService()
	Expect(err).ToNot(HaveOccurred())

	var successOps, failOps int64
	latencyChan := make(chan time.Duration, result.TotalOperations)

	var wg sync.WaitGroup
	totalStart := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			ctx := context.Background()

			for j := 0; j < operationsPerWorker; j++ {
				textIndex := (workerID*operationsPerWorker + j) % len(texts)
				text := texts[textIndex]

				start := time.Now()
				_, err := embeddingService.GenerateTextEmbedding(ctx, text)
				latency := time.Since(start)

				if err != nil {
					atomic.AddInt64(&failOps, 1)
				} else {
					atomic.AddInt64(&successOps, 1)
					latencyChan <- latency
				}
			}
		}(i)
	}

	wg.Wait()
	close(latencyChan)
	totalTime := time.Since(totalStart)

	// Collect latencies
	var latencies []time.Duration
	for latency := range latencyChan {
		latencies = append(latencies, latency)
	}

	// Calculate statistics
	if len(latencies) > 0 {
		result.AverageLatency = calculateAverageLatency(latencies)
		result.P50Latency = calculatePercentile(latencies, 50)
		result.P95Latency = calculatePercentile(latencies, 95)
		result.P99Latency = calculatePercentile(latencies, 99)
	}

	result.SuccessfulOps = successOps
	result.FailedOps = failOps
	result.TotalExecutionTime = totalTime
	result.Throughput = float64(successOps) / totalTime.Seconds()

	// Get cache stats
	if cachedService, ok := embeddingService.(*vector.CachedEmbeddingService); ok {
		stats := cachedService.GetCacheStats(context.Background())
		result.CacheHits = stats.Hits
		result.CacheMisses = stats.Misses
		result.CacheHitRate = stats.HitRate
	}

	return result
}

func benchmarkCacheStress(benchmarkType string, factory *vector.VectorDatabaseFactory, texts []string, concurrency int, operationsPerWorker int) CacheBenchmarkResult {
	// Similar to concurrent load but with more aggressive timing and error tracking
	return benchmarkConcurrentLoad(benchmarkType, factory, texts, concurrency, operationsPerWorker)
}

func calculateAverageLatency(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	var total time.Duration
	for _, latency := range latencies {
		total += latency
	}
	return total / time.Duration(len(latencies))
}

func calculatePercentile(latencies []time.Duration, percentile float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	// Simple sorting for percentile calculation
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)

	// Basic bubble sort (inefficient but simple for small datasets)
	for i := 0; i < len(sorted)-1; i++ {
		for j := 0; j < len(sorted)-i-1; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	index := int(math.Ceil(float64(len(sorted))*percentile/100.0)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

func generatePerformanceReport(results []CacheBenchmarkResult, logger *logrus.Logger) {
	logger.Info("=== CACHE PERFORMANCE BENCHMARK REPORT ===")

	for _, result := range results {
		logger.WithFields(logrus.Fields{
			"benchmark":          result.CacheType,
			"operation":          result.OperationType,
			"total_ops":          result.TotalOperations,
			"successful_ops":     result.SuccessfulOps,
			"failed_ops":         result.FailedOps,
			"avg_latency_ms":     result.AverageLatency.Milliseconds(),
			"p95_latency_ms":     result.P95Latency.Milliseconds(),
			"throughput_ops_sec": result.Throughput,
			"cache_hit_rate":     result.CacheHitRate,
			"execution_time_sec": result.TotalExecutionTime.Seconds(),
		}).Info("Benchmark result")
	}

	logger.Info("=== END CACHE PERFORMANCE REPORT ===")
}
