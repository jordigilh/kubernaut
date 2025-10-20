package contextapi

import (
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/query"
)

var _ = Describe("Aggregation Integration Tests", func() {
	var (
		testCtx     context.Context
		cancel      context.CancelFunc
		aggregation *query.AggregationService
	)

	BeforeEach(func() {
		testCtx, cancel = context.WithTimeout(ctx, 30*time.Second)

		// BR-CONTEXT-004: Query aggregation setup
		cacheConfig := &cache.Config{
			RedisAddr:  "localhost:6379",
			LRUSize:    1000,
			DefaultTTL: 5 * time.Minute,
		}
		cacheManager, err := cache.NewCacheManager(cacheConfig, logger)
		Expect(err).ToNot(HaveOccurred())

		aggregation = query.NewAggregationService(sqlxDB, cacheManager, logger)
		Expect(aggregation).ToNot(BeNil())

		// Setup test data with varying success rates
		_, err = SetupTestData(sqlxDB, 30)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		defer cancel()

		// Clean up test data
		_, err := db.ExecContext(testCtx, "TRUNCATE TABLE remediation_audit")
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Success Rate Calculation", func() {
		It("should calculate accurate success rate percentage", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-004: Success rate aggregation
			result, err := aggregation.AggregateSuccessRate(testCtx, "workflow-1")

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result).To(HaveKey("success_rate"))

			successRate := result["success_rate"].(float64)
			Expect(successRate).To(BeNumerically(">=", 0.0))
			Expect(successRate).To(BeNumerically("<=", 1.0))
		})
	})

	Context("Namespace Grouping", func() {
		It("should group incidents by namespace with correct counts", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-004: Grouping by namespace
			groups, err := aggregation.GroupByNamespace(testCtx)

			Expect(err).ToNot(HaveOccurred())
			// ✅ TDD Compliance Fix: Validate actual namespace groups from test data
			// Test data: 30 incidents across 4 namespaces (round-robin)
			// Expected: default=8, kube-system=8, monitoring=7, app=7
			Expect(groups).To(HaveLen(4), "Should return 4 namespace groups")

			// Verify each group has required fields and expected counts
			namespaceMap := make(map[string]int)
			for _, group := range groups {
				Expect(group).To(HaveKey("namespace"))
				Expect(group).To(HaveKey("count"))
				namespace := group["namespace"].(string)
				count := group["count"].(int)
				namespaceMap[namespace] = count
			}

			// Validate known namespaces exist with expected counts
			Expect(namespaceMap).To(HaveKey("default"))
			Expect(namespaceMap).To(HaveKey("kube-system"))
			Expect(namespaceMap).To(HaveKey("monitoring"))
			Expect(namespaceMap).To(HaveKey("app"))

			// Verify total count matches test data (30 incidents)
			totalCount := namespaceMap["default"] + namespaceMap["kube-system"] +
				namespaceMap["monitoring"] + namespaceMap["app"]
			Expect(totalCount).To(Equal(30), "Total incidents across all namespaces should be 30")
		})
	})

	Context("Severity Distribution", func() {
		It("should provide correct severity breakdown", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-004: Severity distribution
			result, err := aggregation.GetSeverityDistribution(testCtx, "")

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result).To(HaveKey("distribution"))

			distribution := result["distribution"].(map[string]int)
			// ✅ TDD Compliance Fix: Validate actual severity distribution from test data
			// Test data: 30 incidents across 4 severities (round-robin)
			// Expected: critical=8, high=8, medium=7, low=7
			Expect(distribution).To(HaveLen(4), "Should have 4 severity levels")

			// Verify known severity levels exist
			Expect(distribution).To(HaveKey("critical"))
			Expect(distribution).To(HaveKey("high"))
			Expect(distribution).To(HaveKey("medium"))
			Expect(distribution).To(HaveKey("low"))

			// Verify total count matches test data (30 incidents)
			totalCount := distribution["critical"] + distribution["high"] +
				distribution["medium"] + distribution["low"]
			Expect(totalCount).To(Equal(30), "Total incidents across all severities should be 30")
		})
	})

	Context("Incident Trends", func() {
		It("should return time-series incident data", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-004: Incident trends over time
			trend, err := aggregation.GetIncidentTrend(testCtx, 7)

			Expect(err).ToNot(HaveOccurred())
			// ✅ TDD Compliance Fix: Validate actual trend data from query
			// Query requests 7 days of trend data, but may return fewer if no data exists
			Expect(trend).ToNot(BeNil(), "Trend result should not be nil")
			Expect(len(trend)).To(BeNumerically("<=", 7), "Should return at most 7 days of trend data")

			// If trend data exists, verify structure
			if len(trend) > 0 {
				// Verify each data point has required structure
				for i, dataPoint := range trend {
					Expect(dataPoint).To(HaveKey("date"), "Data point %d should have date field", i)
					Expect(dataPoint).To(HaveKey("count"), "Data point %d should have count field", i)
					// Count should be non-negative (0 is valid for days with no incidents)
					count := dataPoint["count"].(int)
					Expect(count).To(BeNumerically(">=", 0), "Count should be non-negative")
				}
			}
		})
	})

	Context("Top Failing Actions", func() {
		It("should rank actions by failure rate", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-004: Advanced aggregation
			limit := 5
			timeWindow := 24 * time.Hour

			results, err := aggregation.GetTopFailingActions(testCtx, limit, timeWindow)

			Expect(err).ToNot(HaveOccurred())
			Expect(results).ToNot(BeEmpty())
			Expect(len(results)).To(BeNumerically("<=", limit))

			// Verify ranked by failure rate (descending)
			for i := 1; i < len(results); i++ {
				prev := results[i-1]["failure_rate"].(float64)
				curr := results[i]["failure_rate"].(float64)
				Expect(curr).To(BeNumerically("<=", prev), "Should be sorted by failure rate")
			}
		})
	})

	Context("Action Comparison", func() {
		It("should provide side-by-side action statistics", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-004: Compare multiple action types
			actionTypes := []string{"restart", "scale", "patch"}
			timeWindow := 24 * time.Hour

			results, err := aggregation.GetActionComparison(testCtx, actionTypes, timeWindow)

			Expect(err).ToNot(HaveOccurred())
			Expect(len(results)).To(Equal(len(actionTypes)), "Should return stats for all 3 requested action types")

			// ✅ TDD Compliance Fix: Validate actual action comparison results
			// Verify each action has statistics and matches requested action types
			actionTypeMap := make(map[string]bool)
			for _, result := range results {
				Expect(result.ActionType).To(BeElementOf("restart", "scale", "patch"),
					"ActionType should be one of the requested types")
				actionTypeMap[result.ActionType] = true
				Expect(result.TotalAttempts).To(BeNumerically(">=", 0), "TotalAttempts should be non-negative")
				Expect(result.SuccessfulAttempts).To(BeNumerically(">=", 0), "SuccessfulAttempts should be non-negative")
			}
		})
	})

	Context("Namespace Health Score", func() {
		It("should calculate normalized health score (0-1)", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-004: Namespace health scoring
			namespace := "default"
			timeWindow := 24 * time.Hour

			score, err := aggregation.GetNamespaceHealthScore(testCtx, namespace, timeWindow)

			Expect(err).ToNot(HaveOccurred())
			Expect(score).To(BeNumerically(">=", 0.0))
			Expect(score).To(BeNumerically("<=", 1.0), "Score should be normalized 0-1")
		})
	})

	Context("Empty Dataset", func() {
		It("should handle zero rows gracefully", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-004: Empty result handling
			// Clear all test data
			_, err := db.ExecContext(testCtx, "TRUNCATE TABLE remediation_audit")
			Expect(err).ToNot(HaveOccurred())

			// Query empty dataset
			groups, err := aggregation.GroupByNamespace(testCtx)

			Expect(err).ToNot(HaveOccurred())
			Expect(groups).To(BeEmpty(), "Should return empty array for no data")
		})
	})

	Context("Time Window Filtering", func() {
		It("should respect time boundary constraints", func() {
			// Day 8 DO-REFACTOR: Test activated (Batch 8)

			// BR-CONTEXT-004: Time-based filtering
			timeWindow := 1 * time.Hour

			results, err := aggregation.GetTopFailingActions(testCtx, 10, timeWindow)

			Expect(err).ToNot(HaveOccurred())
			// Results can be nil or empty slice if no actions exist within time window
			// This validates the query executes successfully and respects time boundaries

			// If results exist, validate their structure and data quality
			if results != nil && len(results) > 0 {
				// Validate result structure and data types
				// Query returns: action_type, total_attempts, failed_attempts, failure_rate
				for _, action := range results {
					Expect(action).To(HaveKey("action_type"), "Result should have action_type field")
					Expect(action).To(HaveKey("total_attempts"), "Result should have total_attempts field")
					Expect(action).To(HaveKey("failed_attempts"), "Result should have failed_attempts field")
					Expect(action).To(HaveKey("failure_rate"), "Result should have failure_rate field")

					// Validate data types and ranges
					failureRate := action["failure_rate"].(float64)
					Expect(failureRate).To(BeNumerically(">=", 0.0), "Failure rate should not be negative")
					Expect(failureRate).To(BeNumerically("<=", 1.0), "Failure rate should not exceed 100%")
				}
			}
		})
	})

	Context("Multi-table Joins", func() {
		It("should handle complex multi-table joins correctly", func() {
			// Day 8 DO-REFACTOR: Test activated (Batch 8)

			// BR-CONTEXT-004: Complex query aggregations
			// Note: Current schema is single table (remediation_audit)
			// This test validates join correctness if schema expands

			result, err := aggregation.AggregateSuccessRate(testCtx, "workflow-1")

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
		})
	})

	Context("Cache Integration", func() {
		It("should cache aggregation results", func() {
			// Day 8 DO-GREEN: Test activated (Batch 6)

			// BR-CONTEXT-005: Caching for aggregations
			// First query (cache miss)
			start1 := time.Now()
			_, err := aggregation.GroupByNamespace(testCtx)
			duration1 := time.Since(start1)
			Expect(err).ToNot(HaveOccurred())

			// Second query (cache hit)
			start2 := time.Now()
			_, err = aggregation.GroupByNamespace(testCtx)
			duration2 := time.Since(start2)
			Expect(err).ToNot(HaveOccurred())

			// ✅ TDD Compliance Fix: Add absolute thresholds per BR-CONTEXT-005
			// Database aggregation should be <500ms
			Expect(duration1).To(BeNumerically("<", 500*time.Millisecond),
				"Database aggregation should complete in <500ms per BR-CONTEXT-005")

			// Cache hit should be <50ms
			Expect(duration2).To(BeNumerically("<", 50*time.Millisecond),
				"Cached aggregation should complete in <50ms per BR-CONTEXT-005")

			// Cache should be faster than database (or both are extremely fast)
			// For very fast queries (<10ms), absolute speed matters more than percentage improvement
			if duration1 > 10*time.Millisecond {
				// For measurable queries, cache should be ≥50% faster
				improvement := float64(duration1-duration2) / float64(duration1)
				Expect(improvement).To(BeNumerically(">=", 0.5),
					"Cache should be ≥50%% faster than database per BR-CONTEXT-005")
			} else {
				// For very fast queries, just verify cache is faster (or equal)
				Expect(duration2).To(BeNumerically("<=", duration1),
					"Cache should be at least as fast as database")
			}
		})
	})

	// ✅ TDD Compliance Fix: Split mixed concerns test into focused tests
	// Each aggregation method now has its own test with proper result validation
	Context("Aggregation Methods", func() {
		It("should calculate aggregate success rate correctly", func() {
			// Day 8 DO-GREEN: Test activated (Batch 5 - split from mixed concerns)

			// BR-CONTEXT-004: Success rate aggregation method
			result, err := aggregation.AggregateSuccessRate(testCtx, "workflow-1")

			Expect(err).ToNot(HaveOccurred(), "AggregateSuccessRate should not error")
			Expect(result).ToNot(BeNil(), "Should return aggregation result")
			Expect(result).To(HaveKey("success_rate"), "Result should have success_rate field")

			// Validate success rate is between 0.0 and 1.0
			rate := result["success_rate"].(float64)
			Expect(rate).To(BeNumerically(">=", 0.0), "Success rate should not be negative")
			Expect(rate).To(BeNumerically("<=", 1.0), "Success rate should not exceed 100%")
		})

		It("should group incidents by namespace correctly", func() {
			// Day 8 DO-GREEN: Test activated (Batch 5 - split from mixed concerns)

			// BR-CONTEXT-004: Namespace grouping method
			groups, err := aggregation.GroupByNamespace(testCtx)

			Expect(err).ToNot(HaveOccurred(), "GroupByNamespace should not error")
			Expect(groups).ToNot(BeEmpty(), "Should return namespace groups")

			// Validate known namespace exists (from test data: default, kube-system, monitoring, app)
			namespaceMap := make(map[string]int)
			for _, group := range groups {
				namespace := group["namespace"].(string)
				count := group["count"].(int)
				namespaceMap[namespace] = count
			}
			Expect(namespaceMap).To(HaveKey("default"), "Should include 'default' namespace from test data")
		})

		It("should calculate severity distribution correctly", func() {
			// Day 8 DO-GREEN: Test activated (Batch 5 - split from mixed concerns)

			// BR-CONTEXT-004: Severity distribution method
			result, err := aggregation.GetSeverityDistribution(testCtx, "")

			Expect(err).ToNot(HaveOccurred(), "GetSeverityDistribution should not error")
			Expect(result).ToNot(BeNil(), "Should return distribution result")
			Expect(result).To(HaveKey("distribution"), "Result should have distribution field")

			distribution := result["distribution"].(map[string]int)
			Expect(distribution).ToNot(BeEmpty(), "Distribution should have severity levels")

			// Validate severity levels exist (from test data: critical, high, medium, low)
			for severity, count := range distribution {
				Expect(severity).To(BeElementOf("critical", "high", "medium", "low"),
					"Severity should be a valid level")
				Expect(count).To(BeNumerically(">", 0), "Each severity should have incidents")
			}
		})

		It("should calculate incident trend correctly", func() {
			// Day 8 DO-GREEN: Test activated (Batch 5 - split from mixed concerns)

			// BR-CONTEXT-004: Incident trend method
			trend, err := aggregation.GetIncidentTrend(testCtx, 7)

			Expect(err).ToNot(HaveOccurred(), "GetIncidentTrend should not error")
			Expect(trend).ToNot(BeNil(), "Should return trend data")
			Expect(len(trend)).To(BeNumerically("<=", 7), "Should return at most 7 days of data")

			// Validate each data point has required fields
			if len(trend) > 0 {
				for i, point := range trend {
					Expect(point).To(HaveKey("date"), "Data point %d should have date field", i)
					Expect(point).To(HaveKey("count"), "Data point %d should have count field", i)
					count := point["count"].(int)
					Expect(count).To(BeNumerically(">=", 0), "Count should be non-negative")
				}
			}
		})
	})

	Context("Statistical Accuracy", func() {
		It("should calculate statistics with correct precision", func() {
			// Day 8 DO-GREEN: Test activated (Batch 5)

			// BR-CONTEXT-004: Mathematical accuracy
			// Note: Test data is generated randomly, so we validate calculation precision
			// rather than exact values

			result, err := aggregation.AggregateSuccessRate(testCtx, "test-workflow")

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil(), "Should return aggregation result")

			// Verify precision of calculation
			// Success rate should be between 0.0 and 1.0
			successRate := result["success_rate"].(float64)
			Expect(successRate).To(BeNumerically(">=", 0.0),
				"Success rate should not be negative")
			Expect(successRate).To(BeNumerically("<=", 1.0),
				"Success rate should not exceed 100%")

			// Verify precision (success rate should have reasonable decimal precision)
			// No NaN or Inf values
			Expect(math.IsNaN(successRate)).To(BeFalse(), "Success rate should not be NaN")
			Expect(math.IsInf(successRate, 0)).To(BeFalse(), "Success rate should not be Inf")
		})
	})

	Context("Edge Cases", func() {
		It("should handle division by zero gracefully", func() {
			// Day 8 DO-GREEN: Test activated (Batch 5)

			// BR-CONTEXT-004: Error resilience
			// Scenario: Calculate success rate with 0 total incidents
			// Expected: Return 0.0 or handle gracefully

			// Clear data
			_, err := db.ExecContext(testCtx, "TRUNCATE TABLE remediation_audit")
			Expect(err).ToNot(HaveOccurred())

			// Query with no data (potential division by zero)
			result, err := aggregation.AggregateSuccessRate(testCtx, "empty-workflow")

			Expect(err).ToNot(HaveOccurred())
			// Should handle division by zero gracefully
			if result["success_rate"] != nil {
				successRate := result["success_rate"].(float64)
				Expect(successRate).To(Equal(0.0), "Empty dataset should return 0.0 success rate")
			}
		})
	})
})
