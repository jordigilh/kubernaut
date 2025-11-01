package contextapi

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/metrics"
	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
	"github.com/jordigilh/kubernaut/pkg/contextapi/query"
)

var _ = Describe("Query Lifecycle Integration Tests", func() {
	var (
		testCtx        context.Context
		cancel         context.CancelFunc
		cacheManager   cache.CacheManager
		cachedExecutor *query.CachedExecutor
		queryRouter    *query.Router
	)

	BeforeEach(func() {
		testCtx, cancel = context.WithTimeout(ctx, 30*time.Second)

		// BR-CONTEXT-005: Multi-tier caching setup
		cacheConfig := &cache.Config{
			RedisAddr:  "localhost:6379",
			LRUSize:    1000,
			DefaultTTL: 5 * time.Minute,
		}
		var err error
		cacheManager, err = cache.NewCacheManager(cacheConfig, logger)
		Expect(err).ToNot(HaveOccurred(), "Cache manager should initialize")

		// BR-CONTEXT-001: Query executor with caching
		// DD-005: Create metrics for executor (required)
		registry := prometheus.NewRegistry()
		metricsInstance := metrics.NewMetricsWithRegistry("contextapi", "", registry)


		executorCfg := &query.Config{
			DB:      sqlxDB,
			Cache:   cacheManager,
			TTL:     5 * time.Minute,
			Metrics: metricsInstance,
		}
		cachedExecutor, err = query.NewCachedExecutor(executorCfg)
		Expect(err).ToNot(HaveOccurred(), "Cached executor should initialize")

		// Create aggregation service for router
		aggregation := query.NewAggregationService(sqlxDB, cacheManager, logger)

		// BR-CONTEXT-004: Query router
		queryRouter = query.NewRouter(cachedExecutor, nil, aggregation, logger)
		Expect(queryRouter).ToNot(BeNil(), "Query router should initialize")

		// Setup test data
		_, err = SetupTestData(sqlxDB, 10)
		Expect(err).ToNot(HaveOccurred(), "Test data should be inserted")
	})

	AfterEach(func() {
		defer cancel()

		// Clean up test data (Data Storage schema)
		_, err := db.ExecContext(testCtx, `
			DELETE FROM resource_action_traces WHERE action_id LIKE 'test-%' OR action_id LIKE 'rr-%';
			DELETE FROM action_histories WHERE id IN (
				SELECT ah.id FROM action_histories ah
				JOIN resource_references rr ON ah.resource_id = rr.id
				WHERE rr.resource_uid LIKE 'test-uid-%'
			);
			DELETE FROM resource_references WHERE resource_uid LIKE 'test-uid-%';
		`)
		if err != nil {
			GinkgoWriter.Printf("⚠️  Test data cleanup warning: %v\n", err)
		}
	})

	Context("Cache Miss → Database Query → Cache Population", func() {
		It("should populate cache on first query (cache miss)", func() {
			// Day 8 DO-GREEN: Infrastructure validation test activated
			// BR-CONTEXT-001: Query with cache miss
			params := &models.ListIncidentsParams{
				Namespace: strPtr("default"),
				Limit:     10,
				Offset:    0,
			}

			// First query (cache miss, hits database)
			start := time.Now()
			results, total, err := cachedExecutor.ListIncidents(testCtx, params)
			duration := time.Since(start)

			Expect(err).ToNot(HaveOccurred(), "Query should succeed")
			// ✅ TDD Compliance Fix: Validate actual business values, not just "not empty"
			// Test data setup creates 10 incidents across 4 namespaces (round-robin)
			// For namespace="default" with count=10: incidents 0, 4, 8 = 3 incidents
			Expect(len(results)).To(Equal(3), "Should return exactly 3 results for namespace=default")
			Expect(total).To(Equal(3), "Total should match 3 incidents in 'default' namespace")

			// ✅ TDD Compliance Fix: Add absolute performance threshold per BR-CONTEXT-005
			// Database query should be < 500ms (acceptable performance)
			// Note: With pgvector indexes and small datasets, actual time is typically <5ms
			Expect(duration).To(BeNumerically("<", 500*time.Millisecond),
				"Database query should complete in <500ms per BR-CONTEXT-005")

			// Warn if approaching threshold (80% = 400ms)
			if duration > 400*time.Millisecond {
				GinkgoWriter.Printf("⚠️ Database query took %v (approaching 500ms threshold)\n", duration)
			}
			GinkgoWriter.Printf("First query (database) took: %v\n", duration)

			// BR-CONTEXT-005: Verify cache was populated asynchronously
			cacheKey := GenerateCacheKey(params)
			WaitForCachePopulation(testCtx, cacheManager, cacheKey, 2*time.Second)
		})
	})

	Context("Cache Hit → Fast Retrieval from Redis", func() {
		It("should serve from cache on second query (cache hit)", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-005: Multi-tier cache hit
			params := &models.ListIncidentsParams{
				Namespace: strPtr("default"),
				Limit:     10,
				Offset:    0,
			}

			// First query (populates cache)
			_, _, err := cachedExecutor.ListIncidents(testCtx, params)
			Expect(err).ToNot(HaveOccurred())

			// Wait for cache population
			cacheKey := GenerateCacheKey(params)
			WaitForCachePopulation(testCtx, cacheManager, cacheKey, 2*time.Second)

			// Second query (should hit cache)
			start := time.Now()
			results2, total2, err := cachedExecutor.ListIncidents(testCtx, params)
			duration := time.Since(start)

			Expect(err).ToNot(HaveOccurred(), "Cached query should succeed")
			// ✅ TDD Compliance Fix: Validate actual cached values match expected
			// Same query params as Test 1: namespace="default" returns 3 incidents
			Expect(len(results2)).To(Equal(3), "Cached results should match database query (3 incidents)")
			Expect(total2).To(Equal(3), "Cached total should match database total (3)")

			// ✅ TDD Compliance Fix: Absolute performance threshold per BR-CONTEXT-005
			// Cache hit should be <50ms (target performance)
			AssertLatency(duration, 50*time.Millisecond, "Cache hit query")
		})
	})

	Context("Multiple Queries → Consistent Results", func() {
		It("should return consistent results across multiple queries", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-001: Data consistency validation
			params := &models.ListIncidentsParams{
				Namespace: strPtr("default"),
				Limit:     10,
				Offset:    0,
			}

			// Execute query 3 times
			results1, total1, err := cachedExecutor.ListIncidents(testCtx, params)
			Expect(err).ToNot(HaveOccurred())

			results2, total2, err := cachedExecutor.ListIncidents(testCtx, params)
			Expect(err).ToNot(HaveOccurred())

			results3, total3, err := cachedExecutor.ListIncidents(testCtx, params)
			Expect(err).ToNot(HaveOccurred())

			// Verify consistency
			Expect(total1).To(Equal(total2), "Total count should be consistent")
			Expect(total2).To(Equal(total3), "Total count should be consistent")
			Expect(len(results1)).To(Equal(len(results2)), "Result count should be consistent")
			Expect(len(results2)).To(Equal(len(results3)), "Result count should be consistent")
		})
	})

	Context("Concurrent Queries → No Race Conditions", func() {
		It("should handle concurrent queries without race conditions", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-001: Concurrent query safety
			params := &models.ListIncidentsParams{
				Namespace: strPtr("default"),
				Limit:     10,
				Offset:    0,
			}

			// Execute 10 concurrent queries
			type result struct {
				incidents []*models.IncidentEvent
				total     int
				err       error
			}
			results := make(chan result, 10)

			for i := 0; i < 10; i++ {
				go func() {
					incidents, total, err := cachedExecutor.ListIncidents(testCtx, params)
					results <- result{incidents, total, err}
				}()
			}

			// Collect all results
			for i := 0; i < 10; i++ {
				res := <-results
				Expect(res.err).ToNot(HaveOccurred(), "Concurrent query should succeed")
				// ✅ TDD Compliance Fix: Validate actual concurrent query results
				// Same params: namespace="default" returns 3 incidents
				Expect(len(res.incidents)).To(Equal(3), "Each concurrent query should return 3 incidents")
				Expect(res.total).To(Equal(3), "Each concurrent query should return total=3")
			}
		})
	})

	Context("Empty Results → Cache Correctly Handles Empty", func() {
		It("should cache and return empty results correctly", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-001: Empty result handling
			params := &models.ListIncidentsParams{
				Namespace: strPtr("nonexistent-namespace"),
				Limit:     10,
				Offset:    0,
			}

			// Query non-existent namespace
			results, total, err := cachedExecutor.ListIncidents(testCtx, params)
			Expect(err).ToNot(HaveOccurred(), "Query should succeed even with no results")
			Expect(results).To(BeEmpty(), "Should return empty array")
			Expect(total).To(Equal(0), "Total should be zero")

			// Verify cache was still populated (even for empty results)
			cacheKey := GenerateCacheKey(params)
			WaitForCachePopulation(testCtx, cacheManager, cacheKey, 2*time.Second)

			// Second query should still hit cache
			start := time.Now()
			results2, total2, err := cachedExecutor.ListIncidents(testCtx, params)
			duration := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			Expect(results2).To(BeEmpty())
			Expect(total2).To(Equal(0))
			// ✅ TDD Compliance Fix: Absolute performance threshold per BR-CONTEXT-005
			// Cache hit should be <50ms even for empty results
			AssertLatency(duration, 50*time.Millisecond, "Cached empty result query")
		})
	})

	Context("Cache Key Generation → Deterministic Keys", func() {
		It("should generate consistent cache keys for identical parameters", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-005: Deterministic cache key generation
			params1 := &models.ListIncidentsParams{
				Namespace: strPtr("default"),
				Limit:     10,
				Offset:    0,
			}

			params2 := &models.ListIncidentsParams{
				Namespace: strPtr("default"),
				Limit:     10,
				Offset:    0,
			}

			key1 := GenerateCacheKey(params1)
			key2 := GenerateCacheKey(params2)

			Expect(key1).To(Equal(key2), "Identical parameters should generate identical cache keys")
		})

		It("should generate different cache keys for different parameters", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-005: Cache key uniqueness
			params1 := &models.ListIncidentsParams{
				Namespace: strPtr("default"),
				Limit:     10,
				Offset:    0,
			}

			params2 := &models.ListIncidentsParams{
				Namespace: strPtr("kube-system"),
				Limit:     10,
				Offset:    0,
			}

			key1 := GenerateCacheKey(params1)
			key2 := GenerateCacheKey(params2)

			Expect(key1).ToNot(Equal(key2), "Different parameters should generate different cache keys")
		})
	})

})
