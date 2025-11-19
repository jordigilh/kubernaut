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

package server

import (
	"testing"

	goredis "github.com/go-redis/redis/v8"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	gatewayMetrics "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

func TestRedisPoolMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Redis Pool Metrics Suite")
}

// mockRedisClient is a test double for Redis client
type mockRedisClient struct {
	stats *goredis.PoolStats
}

func (m *mockRedisClient) PoolStats() *goredis.PoolStats {
	return m.stats
}

var _ = Describe("Redis Pool Metrics Collection", func() {
	var (
		metrics     *gatewayMetrics.Metrics
		registry    *prometheus.Registry
		redisClient *mockRedisClient
	)

	BeforeEach(func() {
		// Create custom registry for test isolation
		registry = prometheus.NewRegistry()
		metrics = gatewayMetrics.NewMetricsWithRegistry(registry)

		// Create mock Redis client with test stats
		redisClient = &mockRedisClient{
			stats: &goredis.PoolStats{
				Hits:       100,
				Misses:     10,
				Timeouts:   2,
				TotalConns: 20,
				IdleConns:  15,
				StaleConns: 0,
			},
		}
	})

	Describe("Pool Stats Collection", func() {
		It("should collect total connections", func() {
			// Arrange: Set pool stats
			stats := redisClient.PoolStats()

			// Act: Update metrics
			metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))

			// Assert: Verify metric value
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_redis_pool_connections_total" {
					found = true
					Expect(mf.GetType()).To(Equal(dto.MetricType_GAUGE))
					metricList := mf.GetMetric()
					Expect(metricList).ToNot(BeEmpty())
					Expect(metricList[0].GetGauge().GetValue()).To(Equal(float64(20)))
				}
			}
			Expect(found).To(BeTrue(), "gateway_redis_pool_connections_total metric should exist")
		})

		It("should collect idle connections", func() {
			// Arrange: Set pool stats
			stats := redisClient.PoolStats()

			// Act: Update metrics
			metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))

			// Assert: Verify metric value
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_redis_pool_connections_idle" {
					found = true
					Expect(mf.GetType()).To(Equal(dto.MetricType_GAUGE))
					metricList := mf.GetMetric()
					Expect(metricList).ToNot(BeEmpty())
					Expect(metricList[0].GetGauge().GetValue()).To(Equal(float64(15)))
				}
			}
			Expect(found).To(BeTrue(), "gateway_redis_pool_connections_idle metric should exist")
		})

		It("should calculate active connections correctly", func() {
			// Arrange: Set pool stats
			stats := redisClient.PoolStats()
			activeConns := stats.TotalConns - stats.IdleConns

			// Act: Update metrics
			metrics.RedisPoolConnectionsActive.Set(float64(activeConns))

			// Assert: Verify metric value (20 - 15 = 5)
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_redis_pool_connections_active" {
					found = true
					Expect(mf.GetType()).To(Equal(dto.MetricType_GAUGE))
					metricList := mf.GetMetric()
					Expect(metricList).ToNot(BeEmpty())
					Expect(metricList[0].GetGauge().GetValue()).To(Equal(float64(5)))
				}
			}
			Expect(found).To(BeTrue(), "gateway_redis_pool_connections_active metric should exist")
		})

		It("should collect pool hits", func() {
			// Arrange: Set pool stats
			stats := redisClient.PoolStats()

			// Act: Update metrics
			metrics.RedisPoolHitsTotal.Add(float64(stats.Hits))

			// Assert: Verify metric value
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_redis_pool_hits_total" {
					found = true
					Expect(mf.GetType()).To(Equal(dto.MetricType_COUNTER))
					metricList := mf.GetMetric()
					Expect(metricList).ToNot(BeEmpty())
					Expect(metricList[0].GetCounter().GetValue()).To(Equal(float64(100)))
				}
			}
			Expect(found).To(BeTrue(), "gateway_redis_pool_hits_total metric should exist")
		})

		It("should collect pool misses", func() {
			// Arrange: Set pool stats
			stats := redisClient.PoolStats()

			// Act: Update metrics
			metrics.RedisPoolMissesTotal.Add(float64(stats.Misses))

			// Assert: Verify metric value
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_redis_pool_misses_total" {
					found = true
					Expect(mf.GetType()).To(Equal(dto.MetricType_COUNTER))
					metricList := mf.GetMetric()
					Expect(metricList).ToNot(BeEmpty())
					Expect(metricList[0].GetCounter().GetValue()).To(Equal(float64(10)))
				}
			}
			Expect(found).To(BeTrue(), "gateway_redis_pool_misses_total metric should exist")
		})

		It("should collect pool timeouts", func() {
			// Arrange: Set pool stats
			stats := redisClient.PoolStats()

			// Act: Update metrics
			metrics.RedisPoolTimeoutsTotal.Add(float64(stats.Timeouts))

			// Assert: Verify metric value
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_redis_pool_timeouts_total" {
					found = true
					Expect(mf.GetType()).To(Equal(dto.MetricType_COUNTER))
					metricList := mf.GetMetric()
					Expect(metricList).ToNot(BeEmpty())
					Expect(metricList[0].GetCounter().GetValue()).To(Equal(float64(2)))
				}
			}
			Expect(found).To(BeTrue(), "gateway_redis_pool_timeouts_total metric should exist")
		})
	})

	Describe("Edge Cases", func() {
		It("should handle zero connections", func() {
			// Arrange: Empty pool
			redisClient.stats = &goredis.PoolStats{
				TotalConns: 0,
				IdleConns:  0,
			}
			stats := redisClient.PoolStats()

			// Act: Update metrics
			metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
			metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
			metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))

			// Assert: All should be 0
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_redis_pool_connections_total" ||
					mf.GetName() == "gateway_redis_pool_connections_idle" ||
					mf.GetName() == "gateway_redis_pool_connections_active" {
					metricList := mf.GetMetric()
					Expect(metricList).ToNot(BeEmpty())
					Expect(metricList[0].GetGauge().GetValue()).To(Equal(float64(0)))
				}
			}
		})

		It("should handle all connections active", func() {
			// Arrange: All connections in use
			redisClient.stats = &goredis.PoolStats{
				TotalConns: 10,
				IdleConns:  0,
			}
			stats := redisClient.PoolStats()

			// Act: Update metrics
			metrics.RedisPoolConnectionsTotal.Set(float64(stats.TotalConns))
			metrics.RedisPoolConnectionsIdle.Set(float64(stats.IdleConns))
			metrics.RedisPoolConnectionsActive.Set(float64(stats.TotalConns - stats.IdleConns))

			// Assert: Active should equal total
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var totalValue, activeValue float64
			for _, mf := range metricFamilies {
				metricList := mf.GetMetric()
				if len(metricList) > 0 {
					if mf.GetName() == "gateway_redis_pool_connections_total" {
						totalValue = metricList[0].GetGauge().GetValue()
					}
					if mf.GetName() == "gateway_redis_pool_connections_active" {
						activeValue = metricList[0].GetGauge().GetValue()
					}
				}
			}

			Expect(totalValue).To(Equal(float64(10)))
			Expect(activeValue).To(Equal(float64(10)))
			Expect(activeValue).To(Equal(totalValue))
		})
	})
})
