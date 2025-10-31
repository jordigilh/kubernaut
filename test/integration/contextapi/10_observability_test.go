package contextapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	prommodel "github.com/prometheus/client_model/go"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/query"
)

// RED PHASE: These tests define observability requirements
// DD-005: Observability Standards
// BR-CONTEXT-006: Observability and monitoring
//
// Business Requirement: ALL operations MUST record Prometheus metrics
//
// Expected Metrics:
// 1. Cache hit/miss counters by tier (redis, lru, database)
// 2. Query duration histograms by type
// 3. Database query duration histograms
// 4. HTTP request counters and duration
// 5. Error counters by type

var _ = Describe("DD-005 Observability Standards - RED PHASE", func() {
	var (
		testCtx        context.Context
		cancel         context.CancelFunc
		cacheManager   cache.CacheManager
		cachedExecutor *query.CachedExecutor
		registry       *prometheus.Registry
	)

	BeforeEach(func() {
		testCtx, cancel = context.WithTimeout(ctx, 30*time.Second)

		// Create custom registry for metric collection
		registry = prometheus.NewRegistry()
		_ = registry // Will be used in GREEN phase for metric validation

		// Setup cache manager
		cacheConfig := &cache.Config{
			RedisAddr:  "localhost:6379",
			LRUSize:    1000,
			DefaultTTL: 5 * time.Minute,
		}
		var err error
		cacheManager, err = cache.NewCacheManager(cacheConfig, logger)
		Expect(err).ToNot(HaveOccurred())

		// Setup cached executor with custom metrics registry
		executorCfg := &query.Config{
			DB:    sqlxDB,
			Cache: cacheManager,
			TTL:   5 * time.Minute,
		}
		cachedExecutor, err = query.NewCachedExecutor(executorCfg)
		_ = cachedExecutor // Will be used in GREEN phase for executor metrics
		Expect(err).ToNot(HaveOccurred())

		// Setup test data
		_, err = SetupTestData(sqlxDB, 10)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		defer cancel()

		// Clean up test data
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

	Context("Business Requirement: Cache Metrics Recording", func() {
		It("MUST record cache hit metrics when query hits cache", func() {
			// Business Scenario: User makes same query twice
			// Expected: First query populates cache, second query hits cache and increments counter

			testServer := createHTTPTestServer()
			defer testServer.Close()

			// Make first query (cache miss + populate)
			resp1, err := http.Get(testServer.URL + "/api/v1/context/query?limit=10")
			Expect(err).ToNot(HaveOccurred())
			resp1.Body.Close()

			// Make second query (cache hit)
			resp2, err := http.Get(testServer.URL + "/api/v1/context/query?limit=10")
			Expect(err).ToNot(HaveOccurred())
			resp2.Body.Close()

			// Get metrics and verify cache hit was recorded
			metricsResp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			body, _ := io.ReadAll(metricsResp.Body)
			metricsText := string(body)

			// Business Outcome: Cache hit metric exists and is > 0
			Expect(metricsText).To(ContainSubstring("contextapi_cache_hits_total"))
			Expect(metricsText).To(MatchRegexp(`contextapi_cache_hits_total.*[1-9]`),
				"Cache hit counter MUST increment when query hits cache (value > 0)")
		})

		It("MUST record cache miss metrics on first query", func() {
			// Business Scenario: User makes new query never cached before
			// Expected: Cache miss counter increments, database counter increments

			testServer := createHTTPTestServer()
			defer testServer.Close()

			// Make unique query (guaranteed cache miss)
			uniqueQuery := fmt.Sprintf("/api/v1/context/query?limit=10&offset=%d", time.Now().Unix())
			resp, err := http.Get(testServer.URL + uniqueQuery)
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			// Get metrics and verify cache miss was recorded
			metricsResp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			body, _ := io.ReadAll(metricsResp.Body)
			metricsText := string(body)

			// Business Outcome: Cache miss metric incremented
			Expect(metricsText).To(ContainSubstring("contextapi_cache_misses_total"))
			Expect(metricsText).To(MatchRegexp(`contextapi_cache_misses_total.*[1-9]`),
				"Cache miss counter MUST increment on first query (value > 0)")
		})

		It("MUST record metrics by cache tier (redis, lru, database)", func() {
			// Business Scenario: Cache has multi-tier architecture
			// Expected: Metrics distinguish between L1 Redis, L2 LRU, L3 Database

			testServer := createHTTPTestServer()
			defer testServer.Close()

			// Make queries to exercise different cache tiers
			resp, err := http.Get(testServer.URL + "/api/v1/context/query?limit=10")
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			// Get metrics and verify tier labels
			metricsResp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			body, _ := io.ReadAll(metricsResp.Body)
			metricsText := string(body)

			// Business Outcome: Metrics have 'tier' label with values: redis, lru, database
			Expect(metricsText).To(MatchRegexp(`contextapi_cache_.*\{.*tier="(redis|lru|database)".*\}`),
				"Cache metrics MUST distinguish between tiers (redis, lru, database)")
		})
	})

	Context("Business Requirement: Query Duration Metrics", func() {
		It("MUST record query duration histogram for list operations", func() {
			// Business Scenario: Monitor API performance SLAs
			// Expected: Query duration recorded with type label

			testServer := createHTTPTestServer()
			defer testServer.Close()

			// Make query
			resp, err := http.Get(testServer.URL + "/api/v1/context/query?limit=10")
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			// Get metrics and verify duration histogram
			metricsResp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			body, _ := io.ReadAll(metricsResp.Body)
			metricsText := string(body)

			// Business Outcome: Query duration histogram recorded
			Expect(metricsText).To(ContainSubstring("contextapi_query_duration_seconds"))
			Expect(metricsText).To(MatchRegexp(`contextapi_query_duration_seconds_count.*[1-9]`),
				"Query duration histogram MUST record list operations (count > 0)")
		})

		It("MUST record database query duration separately", func() {
			// Business Scenario: Identify slow database queries
			// Expected: Database-specific duration metrics

			testServer := createHTTPTestServer()
			defer testServer.Close()

			// Make query that hits database
			uniqueQuery := fmt.Sprintf("/api/v1/context/query?limit=10&offset=%d", time.Now().Unix())
			resp, err := http.Get(testServer.URL + uniqueQuery)
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			// Get metrics and verify database duration
			metricsResp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			body, _ := io.ReadAll(metricsResp.Body)
			metricsText := string(body)

			// Business Outcome: Database duration metrics recorded
			Expect(metricsText).To(ContainSubstring("contextapi_db_query_duration_seconds"))
			Expect(metricsText).To(MatchRegexp(`contextapi_db_query_duration_seconds_count.*[1-9]`),
				"Database duration histogram MUST record query operations (count > 0)")
		})
	})

	Context("Business Requirement: HTTP Metrics Recording", func() {
		It("MUST record HTTP request counters by endpoint and status", func() {
			// Business Scenario: Track API usage patterns
			// Expected: Counter increments with method, path, status labels

			testServer := createHTTPTestServer()
			defer testServer.Close()

			// Make HTTP request
			resp, err := http.Get(testServer.URL + "/api/v1/context/query?limit=10")
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			// Get metrics and verify HTTP request counter
			metricsResp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			body, _ := io.ReadAll(metricsResp.Body)
			metricsText := string(body)

			// Business Outcome: HTTP request counter incremented with labels
			Expect(metricsText).To(ContainSubstring("contextapi_http_requests_total"))
			Expect(metricsText).To(MatchRegexp(`contextapi_http_requests_total\{.*method="GET".*\}`),
				"HTTP request counter MUST track method label")
			Expect(metricsText).To(MatchRegexp(`contextapi_http_requests_total\{.*status="200".*\}`),
				"HTTP request counter MUST track status label")
		})

		It("MUST record HTTP request duration histogram", func() {
			// Business Scenario: Monitor end-to-end latency
			// Expected: Duration histogram with endpoint labels

			testServer := createHTTPTestServer()
			defer testServer.Close()

			// Make HTTP request
			resp, err := http.Get(testServer.URL + "/api/v1/context/query?limit=10")
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			// Get metrics and verify HTTP duration histogram
			metricsResp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			body, _ := io.ReadAll(metricsResp.Body)
			metricsText := string(body)

			// Business Outcome: HTTP duration histogram recorded
			Expect(metricsText).To(ContainSubstring("contextapi_http_duration_seconds"))
			Expect(metricsText).To(MatchRegexp(`contextapi_http_duration_seconds_count.*[1-9]`),
				"HTTP duration histogram MUST record request latency (count > 0)")
		})
	})

	Context("Business Requirement: Error Metrics Recording", func() {
		It("MUST increment error counter on query failures", func() {
			// Business Scenario: Track error rates for alerting
			// Expected: Error counter increments with operation and type labels

			testServer := createHTTPTestServer()
			defer testServer.Close()

			// Trigger error (invalid parameter)
			resp, err := http.Get(testServer.URL + "/api/v1/context/query?limit=invalid")
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			// Get metrics and verify error counter
			metricsResp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			body, _ := io.ReadAll(metricsResp.Body)
			metricsText := string(body)

			// Business Outcome: Error counter incremented
			Expect(metricsText).To(ContainSubstring("contextapi_errors_total"))
			Expect(metricsText).To(MatchRegexp(`contextapi_errors_total\{.*operation="query".*\}`),
				"Error counter MUST track operation label")
			Expect(metricsText).To(MatchRegexp(`contextapi_errors_total.*[1-9]`),
				"Error counter MUST increment on query failures (value > 0)")
		})

		It("MUST record different error types separately", func() {
			// Business Scenario: Distinguish between validation vs system errors
			// Expected: Error metrics have category labels

			testServer := createHTTPTestServer()
			defer testServer.Close()

			// Trigger validation error
			resp, err := http.Get(testServer.URL + "/api/v1/context/query?limit=invalid")
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			// Get metrics and verify error type labels
			metricsResp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			body, _ := io.ReadAll(metricsResp.Body)
			metricsText := string(body)

			// Business Outcome: Error metrics distinguish by type
			Expect(metricsText).To(ContainSubstring("contextapi_errors_total"))
			Expect(metricsText).To(MatchRegexp(`contextapi_errors_total\{.*type="(validation|system)".*\}`),
				"Error metrics MUST distinguish validation from system errors using 'type' label")
		})
	})

	Context("Business Requirement: Metrics Endpoint Validation", func() {
		It("MUST expose metrics at /metrics endpoint in correct Prometheus format", func() {
			// Business Scenario: Prometheus scrapes metrics endpoint
			// Expected: Valid Prometheus text format with all metrics

			testServer := createHTTPTestServer()
			defer testServer.Close()

			resp, err := http.Get(fmt.Sprintf("%s/metrics", testServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Business Outcome 1: Metrics endpoint accessible
			Expect(resp.StatusCode).To(Equal(200))

			// Business Outcome 2: Prometheus text format
			contentType := resp.Header.Get("Content-Type")
			Expect(contentType).To(ContainSubstring("text/plain"))

			// Business Outcome 3: Contains DD-005 required metrics
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			metricsText := string(body)

			// Required DD-005 metrics must be present
			Expect(metricsText).To(ContainSubstring("contextapi_queries_total"))
			Expect(metricsText).To(ContainSubstring("contextapi_query_duration_seconds"))
			Expect(metricsText).To(ContainSubstring("contextapi_cache_hits_total"))
			Expect(metricsText).To(ContainSubstring("contextapi_cache_misses_total"))
			Expect(metricsText).To(ContainSubstring("contextapi_http_requests_total"))
			Expect(metricsText).To(ContainSubstring("contextapi_http_duration_seconds"))
		})
	})

	Context("Business Requirement: Metric Labels Correctness", func() {
		It("MUST use correct label names for cache tier metrics", func() {
			// Business Scenario: Grafana dashboards query by tier
			// Expected: Labels include 'tier' with values: redis, lru, database

			testServer := createHTTPTestServer()
			defer testServer.Close()

			// Make query to trigger cache metrics
			resp, err := http.Get(testServer.URL + "/api/v1/context/query?limit=10")
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			// Get metrics and verify label format
			metricsResp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			body, _ := io.ReadAll(metricsResp.Body)
			metricsText := string(body)

			// Business Outcome: Metrics use 'tier' label
			Expect(metricsText).To(MatchRegexp(`contextapi_cache_.*\{.*tier="(redis|lru|database)".*\}`),
				"Cache metrics MUST use 'tier' label with values: redis, lru, database")
		})

		It("MUST use correct label names for query type metrics", func() {
			// Business Scenario: Monitor specific query patterns
			// Expected: Labels include 'type' with values: list_incidents, get_incident, etc.

			testServer := createHTTPTestServer()
			defer testServer.Close()

			// Make query
			resp, err := http.Get(testServer.URL + "/api/v1/context/query?limit=10")
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			// Get metrics and verify label format
			metricsResp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			body, _ := io.ReadAll(metricsResp.Body)
			metricsText := string(body)

			// Business Outcome: Metrics use 'type' label for query types
			Expect(metricsText).To(MatchRegexp(`contextapi_queries_total\{.*type=".*".*\}`),
				"Query metrics MUST use 'type' label to distinguish query types")
		})
	})
})

// Helper function to get metric value from registry
func getCounterValue(registry *prometheus.Registry, metricName string, labels prometheus.Labels) (float64, error) {
	metricFamilies, err := registry.Gather()
	if err != nil {
		return 0, err
	}

	for _, mf := range metricFamilies {
		if mf.GetName() == metricName {
			for _, m := range mf.GetMetric() {
				if labelsMatch(m.GetLabel(), labels) {
					if m.Counter != nil {
						return m.Counter.GetValue(), nil
					}
				}
			}
		}
	}

	return 0, fmt.Errorf("metric %s with labels %v not found", metricName, labels)
}

// Helper function to check if labels match
func labelsMatch(metricLabels []*prommodel.LabelPair, expectedLabels prometheus.Labels) bool {
	if len(metricLabels) != len(expectedLabels) {
		return false
	}

	for _, label := range metricLabels {
		expectedValue, exists := expectedLabels[label.GetName()]
		if !exists || expectedValue != label.GetValue() {
			return false
		}
	}

	return true
}

// Helper function to get histogram sample count
func getHistogramCount(registry *prometheus.Registry, metricName string, labels prometheus.Labels) (uint64, error) {
	metricFamilies, err := registry.Gather()
	if err != nil {
		return 0, err
	}

	for _, mf := range metricFamilies {
		if mf.GetName() == metricName {
			for _, m := range mf.GetMetric() {
				if labelsMatch(m.GetLabel(), labels) {
					if m.Histogram != nil {
						return m.Histogram.GetSampleCount(), nil
					}
				}
			}
		}
	}

	return 0, fmt.Errorf("histogram %s with labels %v not found", metricName, labels)
}

// Helper function to check if metric exists in output
func metricExists(metricsOutput string, metricName string) bool {
	return strings.Contains(metricsOutput, metricName)
}
