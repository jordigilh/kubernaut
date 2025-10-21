package contextapi

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jmoiron/sqlx"
	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
	"github.com/jordigilh/kubernaut/pkg/contextapi/query"
)

// ===================================================================
// EDGE CASE TESTING: Cache Stampede Prevention (Scenario 1.1)
// Design Decision: DD-CONTEXT-001 (Option A - 90% confidence)
// ===================================================================

// ListIncidentsResult is a helper struct to store results from concurrent queries
type ListIncidentsResult struct {
	Incidents []*models.IncidentEvent
	Total     int
}

// Helper Functions for Cache Stampede Tests

// createCachedExecutorForStampede creates executor with Redis DB 7 for cache stampede tests
// Uses custom metrics registry to avoid conflicts with other test suites
func createCachedExecutorForStampede() (*query.CachedExecutor, *dbQueryCounter) {
	// Create connection string for test infrastructure (Data Storage Service)
	// DD-SCHEMA-001: Connect to Data Storage Service database
	connStr := "host=localhost port=5432 user=slm_user password=slm_password_dev dbname=action_history sslmode=disable"

	// Create database client
	db, err := sqlx.Connect("postgres", connStr)
	Expect(err).ToNot(HaveOccurred(), "Database connection should succeed")

	// Create cache manager with Redis DB 7 (parallel test isolation)
	cacheConfig := &cache.Config{
		RedisAddr:  "localhost:6379",
		RedisDB:    7, // Use DB 7 for cache stampede tests (parallel isolation)
		LRUSize:    1000,
		DefaultTTL: 5 * time.Minute,
	}
	cacheManager, err := cache.NewCacheManager(cacheConfig, logger)
	Expect(err).ToNot(HaveOccurred(), "Cache manager creation should succeed")

	// Wrap db with query counter
	counter := newDBQueryCounter(db)

	// Create cached executor with instrumented DB
	executorCfg := &query.Config{
		DB:    counter,
		Cache: cacheManager,
		TTL:   5 * time.Minute,
	}
	executor, err := query.NewCachedExecutor(executorCfg)
	Expect(err).ToNot(HaveOccurred(), "Cached executor creation should succeed")

	return executor, counter
}

// clearRedisCacheForStampede clears Redis DB 7 for cache stampede tests
func clearRedisCacheForStampede() {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   7, // DB 7 for cache stampede tests
	})
	defer client.Close()

	ctx := context.Background()
	err := client.FlushDB(ctx).Err()
	Expect(err).ToNot(HaveOccurred(), "Redis DB 7 flush should succeed")
}

// dbQueryCounter wraps sqlx.DB and counts SELECT queries
// This allows us to validate single-flight deduplication
type dbQueryCounter struct {
	*sqlx.DB
	queryCount int64 // Use atomic for thread safety
}

func newDBQueryCounter(db *sqlx.DB) *dbQueryCounter {
	return &dbQueryCounter{
		DB:         db,
		queryCount: 0,
	}
}

// SelectContext wraps sqlx.DB.SelectContext and increments counter
func (c *dbQueryCounter) SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	atomic.AddInt64(&c.queryCount, 1) // Increment counter atomically
	return c.DB.SelectContext(ctx, dest, query, args...)
}

// GetContext wraps sqlx.DB.GetContext and increments counter
func (c *dbQueryCounter) GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	atomic.AddInt64(&c.queryCount, 1) // Increment counter atomically
	return c.DB.GetContext(ctx, dest, query, args...)
}

// GetQueryCount returns current query count (atomic read)
func (c *dbQueryCounter) GetQueryCount() int64 {
	return atomic.LoadInt64(&c.queryCount)
}

// ResetQueryCount resets query count to zero (atomic write)
func (c *dbQueryCounter) ResetQueryCount() {
	atomic.StoreInt64(&c.queryCount, 0)
}

var _ = Describe("Cache Stampede Prevention Integration Tests", func() {
	var (
		testCtx context.Context
		cancel  context.CancelFunc
	)

	BeforeEach(func() {
		testCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
		clearRedisCacheForStampede() // Clear DB 7 for stampede tests

		// Set up test data specifically for cache stampede tests
		// Day 11: Ensure fresh test data exists for stampede validation
		// Use timestamp-based IDs for uniqueness across test runs
		baseID := time.Now().UnixNano() / 1000000 // Use milliseconds as base ID
		for i := 0; i < 10; i++ {
			incident := CreateIncidentWithEmbedding(baseID+int64(i), "stampede-test")
			err := InsertTestIncident(sqlxDB, incident)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Test incident %d should be inserted", baseID+int64(i)))
		}
	})

	AfterEach(func() {
		cancel()
	})

	Context("Edge Case 1.1: Cache Stampede Prevention (P1)", func() {
		It("should prevent database stampede with single-flight pattern", func() {
			// Day 11 Scenario 1.1 (DO-GREEN Phase - Pure TDD)
			// Design Decision: DD-CONTEXT-001 (Option A - 90% confidence)
			// BR-CONTEXT-005: Cache performance under high concurrency
			//
			// Production Reality: ✅ Very Common
			// - Happens during cache expiration at high traffic
			// - Can cause database overload (10 concurrent requests = 10 DB queries)
			// - Observed in every multi-tier cache service
			//
			// ✅ Pure TDD: Integration test created FIRST (RED), then implement (GREEN)
			//
			// Expected Behavior:
			// - WITHOUT single-flight: 10 concurrent requests = 10 DB queries (stampede!)
			// - WITH single-flight: 10 concurrent requests = 1 DB query (deduplication)

			// Create executor with query counter
			executor, counter := createCachedExecutorForStampede()

			// Ensure cache is empty (all requests will miss cache and hit DB)
			// This simulates cache expiration scenario in production
			clearRedisCacheForStampede()

			// Reset query counter before test
			counter.ResetQueryCount()

			// Launch 10 concurrent goroutines calling ListIncidents with SAME params
			// This simulates high traffic hitting an expired cache entry
			const numConcurrent = 10
			var wg sync.WaitGroup
			results := make([]*ListIncidentsResult, numConcurrent)
			errors := make([]error, numConcurrent)

			// Query all namespaces (no filter) to ensure we get results
			// Day 11: Test data from previous suites should exist
			params := &models.ListIncidentsParams{
				Namespace: nil, // No namespace filter - query all incidents
				Limit:     10,
				Offset:    0,
			}

			for i := 0; i < numConcurrent; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()

					incidents, total, err := executor.ListIncidents(testCtx, params)
					if err != nil {
						errors[idx] = err
						return
					}

					results[idx] = &ListIncidentsResult{
						Incidents: incidents,
						Total:     total,
					}
				}(i)
			}

			// Wait for all goroutines to complete
			wg.Wait()

			// ✅ Business Value Assertion: Single-flight prevented database stampede
			// Expected: Only 1 database query executed despite 10 concurrent requests
			// This validates that single-flight pattern deduplicated the concurrent requests
			//
			// Query count breakdown:
			// - 1 SELECT query (for ListIncidents data)
			// - 1 COUNT query (for pagination total)
			// Total: 2 queries (NOT 20 without single-flight!)
			dbQueryCount := counter.GetQueryCount()
			Expect(dbQueryCount).To(Equal(int64(2)),
				fmt.Sprintf("Single-flight should deduplicate concurrent requests (expected 2 queries, got %d)", dbQueryCount))

			// ✅ Validate all goroutines succeeded
			for i, err := range errors {
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("Goroutine %d should succeed", i))
			}

			// ✅ Validate all goroutines got consistent results
			firstResult := results[0]
			Expect(firstResult).ToNot(BeNil(),
				"First result should not be nil")
			Expect(len(firstResult.Incidents)).To(BeNumerically(">", 0),
				"Should return incidents")

			for i := 1; i < numConcurrent; i++ {
				Expect(results[i]).ToNot(BeNil(),
					fmt.Sprintf("Result %d should not be nil", i))
				Expect(results[i].Total).To(Equal(firstResult.Total),
					fmt.Sprintf("Result %d should have same total as first result", i))
				Expect(len(results[i].Incidents)).To(Equal(len(firstResult.Incidents)),
					fmt.Sprintf("Result %d should have same number of incidents", i))
			}
		})

		It("should handle concurrent requests with different parameters independently", func() {
			// Day 11 Scenario 1.1 - Additional Test (GREEN Phase)
			// BR-CONTEXT-005: Cache performance validation
			//
			// ✅ Validates that single-flight groups requests by cache key
			// Different parameters = different cache keys = independent execution

			executor, counter := createCachedExecutorForStampede()
			clearRedisCacheForStampede()
			counter.ResetQueryCount()

			// Launch 10 concurrent requests with 2 different parameter sets (5 each)
			const numConcurrent = 10
			var wg sync.WaitGroup

			params1 := &models.ListIncidentsParams{Namespace: strPtr("default"), Limit: 10, Offset: 0}
			params2 := &models.ListIncidentsParams{Namespace: strPtr("kube-system"), Limit: 10, Offset: 0}

			for i := 0; i < numConcurrent; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()

					// Alternate between params1 and params2
					params := params1
					if idx%2 == 1 {
						params = params2
					}

					_, _, err := executor.ListIncidents(testCtx, params)
					Expect(err).ToNot(HaveOccurred())
				}(i)
			}

			wg.Wait()

			// ✅ Business Value Assertion: Different params execute independently
			// Expected: 2 parameter sets = 4 queries total (2 SELECT + 2 COUNT)
			// This validates single-flight groups by cache key correctly
			dbQueryCount := counter.GetQueryCount()
			Expect(dbQueryCount).To(Equal(int64(4)),
				fmt.Sprintf("Two different parameter sets should execute 4 queries (expected 4, got %d)", dbQueryCount))
		})
	})
})
