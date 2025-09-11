package holmesgpt_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
)

var _ = Describe("ToolsetConfigCache - Implementation Correctness Testing", func() {
	var (
		cache *holmesgpt.ToolsetConfigCache
		log   *logrus.Logger
	)

	BeforeEach(func() {
		log = logrus.New()
		log.SetLevel(logrus.ErrorLevel) // Reduce test noise
		cache = holmesgpt.NewToolsetConfigCache(5*time.Minute, log)
	})

	// BR-HOLMES-021: Unit tests for toolset configuration caching implementation
	Describe("ToolsetConfigCache Implementation", func() {
		Context("Basic Cache Operations", func() {
			It("should store and retrieve toolset configurations", func() {
				toolset := &holmesgpt.ToolsetConfig{
					Name:        "prometheus-monitoring-prometheus",
					ServiceType: "prometheus",
					Description: "Prometheus metrics analysis tools",
					Version:     "1.0.0",
					Enabled:     true,
					Priority:    80,
				}

				cache.SetToolset(toolset)
				retrieved := cache.GetToolset("prometheus-monitoring-prometheus")

				Expect(retrieved).ToNot(BeNil())
				Expect(retrieved.Name).To(Equal("prometheus-monitoring-prometheus"))
				Expect(retrieved.ServiceType).To(Equal("prometheus"))
				Expect(retrieved.Priority).To(Equal(80))
			})

			It("should return nil for non-existent toolset", func() {
				retrieved := cache.GetToolset("non-existent-toolset")
				Expect(retrieved).To(BeNil())
			})

			It("should handle nil toolset gracefully", func() {
				cache.SetToolset(nil)
				// Should not crash or cause issues
				retrieved := cache.GetToolset("any-key")
				Expect(retrieved).To(BeNil())
			})

			It("should update LastUpdated timestamp when storing", func() {
				beforeTime := time.Now().Add(-1 * time.Second)

				toolset := &holmesgpt.ToolsetConfig{
					Name:        "test-toolset",
					ServiceType: "test",
					LastUpdated: beforeTime,
				}

				cache.SetToolset(toolset)
				retrieved := cache.GetToolset("test-toolset")

				Expect(retrieved.LastUpdated).To(BeTemporally(">", beforeTime))
				Expect(retrieved.LastUpdated).To(BeTemporally("~", time.Now(), time.Second))
			})
		})

		Context("Cache Hit/Miss Tracking", func() {
			BeforeEach(func() {
				// Add some test toolsets
				cache.SetToolset(&holmesgpt.ToolsetConfig{
					Name:        "existing-toolset",
					ServiceType: "test",
				})
			})

			It("should track cache hits correctly", func() {
				initialStats := cache.GetStats()
				initialHits := initialStats.HitCount

				// Access existing toolset (hit)
				retrieved := cache.GetToolset("existing-toolset")
				Expect(retrieved).ToNot(BeNil())

				stats := cache.GetStats()
				Expect(stats.HitCount).To(Equal(initialHits + 1))
			})

			It("should track cache misses correctly", func() {
				initialStats := cache.GetStats()
				initialMisses := initialStats.MissCount

				// Access non-existent toolset (miss)
				retrieved := cache.GetToolset("non-existent-toolset")
				Expect(retrieved).To(BeNil())

				stats := cache.GetStats()
				Expect(stats.MissCount).To(Equal(initialMisses + 1))
			})

			It("should calculate hit rate correctly", func() {
				// Clear cache to start fresh
				cache.Clear()

				// Perform some hits and misses
				cache.SetToolset(&holmesgpt.ToolsetConfig{Name: "hit-test", ServiceType: "test"})

				// 2 hits
				cache.GetToolset("hit-test")
				cache.GetToolset("hit-test")

				// 1 miss
				cache.GetToolset("non-existent")

				hitRate := cache.GetHitRate()
				Expect(hitRate).To(BeNumerically("~", 2.0/3.0, 0.01)) // 66.67%
			})

			It("should handle zero operations for hit rate calculation", func() {
				freshCache := holmesgpt.NewToolsetConfigCache(5*time.Minute, log)
				hitRate := freshCache.GetHitRate()
				Expect(hitRate).To(Equal(0.0))
			})
		})

		Context("Toolset Retrieval by Type", func() {
			BeforeEach(func() {
				// Add toolsets of different types
				cache.SetToolset(&holmesgpt.ToolsetConfig{
					Name:        "prometheus-1",
					ServiceType: "prometheus",
					Priority:    80,
					Enabled:     true,
				})
				cache.SetToolset(&holmesgpt.ToolsetConfig{
					Name:        "prometheus-2",
					ServiceType: "prometheus",
					Priority:    75,
					Enabled:     true,
				})
				cache.SetToolset(&holmesgpt.ToolsetConfig{
					Name:        "grafana-1",
					ServiceType: "grafana",
					Priority:    70,
					Enabled:     true,
				})
				cache.SetToolset(&holmesgpt.ToolsetConfig{
					Name:        "disabled-toolset",
					ServiceType: "prometheus",
					Priority:    60,
					Enabled:     false,
				})
			})

			It("should return toolsets by service type", func() {
				prometheusToolsets := cache.GetToolsetsByType("prometheus")

				// Should return 3 prometheus toolsets (including disabled one)
				Expect(prometheusToolsets).To(HaveLen(3))

				for _, toolset := range prometheusToolsets {
					Expect(toolset.ServiceType).To(Equal("prometheus"))
				}
			})

			It("should return only enabled toolsets", func() {
				enabledToolsets := cache.GetEnabledToolsets()

				// Should return 3 enabled toolsets (excluding disabled-toolset)
				Expect(enabledToolsets).To(HaveLen(3))

				for _, toolset := range enabledToolsets {
					Expect(toolset.Enabled).To(BeTrue())
				}
			})

			It("should return empty slice for unknown service type", func() {
				unknownToolsets := cache.GetToolsetsByType("unknown-type")
				Expect(unknownToolsets).To(BeEmpty())
			})

			It("should return all toolsets", func() {
				allToolsets := cache.GetAllToolsets()
				Expect(allToolsets).To(HaveLen(4)) // All 4 toolsets we added
			})
		})

		Context("Toolset Priority Ordering", func() {
			BeforeEach(func() {
				// Add toolsets with different priorities
				cache.SetToolset(&holmesgpt.ToolsetConfig{
					Name:        "high-priority",
					ServiceType: "prometheus",
					Priority:    90,
					Enabled:     true,
				})
				cache.SetToolset(&holmesgpt.ToolsetConfig{
					Name:        "medium-priority",
					ServiceType: "grafana",
					Priority:    70,
					Enabled:     true,
				})
				cache.SetToolset(&holmesgpt.ToolsetConfig{
					Name:        "low-priority",
					ServiceType: "custom",
					Priority:    30,
					Enabled:     true,
				})
			})

			It("should return toolsets sorted by priority", func() {
				sortedToolsets := cache.GetToolsetsByPriority()

				Expect(sortedToolsets).To(HaveLen(3))

				// Should be sorted by priority (high to low)
				Expect(sortedToolsets[0].Priority).To(Equal(90))
				Expect(sortedToolsets[1].Priority).To(Equal(70))
				Expect(sortedToolsets[2].Priority).To(Equal(30))

				Expect(sortedToolsets[0].Name).To(Equal("high-priority"))
				Expect(sortedToolsets[1].Name).To(Equal("medium-priority"))
				Expect(sortedToolsets[2].Name).To(Equal("low-priority"))
			})
		})

		Context("Capability Analysis", func() {
			BeforeEach(func() {
				cache.SetToolset(&holmesgpt.ToolsetConfig{
					Name:         "metrics-toolset",
					ServiceType:  "prometheus",
					Capabilities: []string{"query_metrics", "alert_rules", "time_series"},
					Enabled:      true,
				})
				cache.SetToolset(&holmesgpt.ToolsetConfig{
					Name:         "visualization-toolset",
					ServiceType:  "grafana",
					Capabilities: []string{"get_dashboards", "visualization", "query_metrics"},
					Enabled:      true,
				})
				cache.SetToolset(&holmesgpt.ToolsetConfig{
					Name:         "disabled-toolset",
					ServiceType:  "jaeger",
					Capabilities: []string{"search_traces"},
					Enabled:      false, // Disabled
				})
			})

			It("should return all unique capabilities from enabled toolsets", func() {
				capabilities := cache.GetAvailableCapabilities()

				Expect(capabilities).To(ContainElement("query_metrics"))
				Expect(capabilities).To(ContainElement("alert_rules"))
				Expect(capabilities).To(ContainElement("time_series"))
				Expect(capabilities).To(ContainElement("get_dashboards"))
				Expect(capabilities).To(ContainElement("visualization"))

				// Should not include capabilities from disabled toolsets
				Expect(capabilities).ToNot(ContainElement("search_traces"))
			})

			It("should find toolsets by capability", func() {
				// Find toolsets with query_metrics capability
				toolsetsWithQueryMetrics := cache.FindToolsetsByCapability("query_metrics")

				Expect(toolsetsWithQueryMetrics).To(HaveLen(2))

				names := make([]string, len(toolsetsWithQueryMetrics))
				for i, toolset := range toolsetsWithQueryMetrics {
					names[i] = toolset.Name
				}

				Expect(names).To(ContainElement("metrics-toolset"))
				Expect(names).To(ContainElement("visualization-toolset"))
			})

			It("should return empty slice for non-existent capability", func() {
				toolsets := cache.FindToolsetsByCapability("non-existent-capability")
				Expect(toolsets).To(BeEmpty())
			})

			It("should return toolsets sorted by priority when finding by capability", func() {
				// Add another toolset with higher priority
				cache.SetToolset(&holmesgpt.ToolsetConfig{
					Name:         "high-priority-metrics",
					ServiceType:  "prometheus",
					Priority:     95,
					Capabilities: []string{"query_metrics"},
					Enabled:      true,
				})

				toolsets := cache.FindToolsetsByCapability("query_metrics")

				Expect(toolsets).To(HaveLen(3))
				// Should be sorted by priority (highest first)
				Expect(toolsets[0].Priority).To(BeNumerically(">=", toolsets[1].Priority))
				Expect(toolsets[1].Priority).To(BeNumerically(">=", toolsets[2].Priority))
			})
		})

		Context("Toolset Management Operations", func() {
			It("should remove toolset from cache", func() {
				toolset := &holmesgpt.ToolsetConfig{
					Name:        "to-be-removed",
					ServiceType: "test",
				}

				cache.SetToolset(toolset)
				Expect(cache.GetToolset("to-be-removed")).ToNot(BeNil())

				cache.RemoveToolset("to-be-removed")
				Expect(cache.GetToolset("to-be-removed")).To(BeNil())
			})

			It("should handle removal of non-existent toolset gracefully", func() {
				// Should not crash or cause issues
				cache.RemoveToolset("non-existent-toolset")

				// Cache should still be functional
				cache.SetToolset(&holmesgpt.ToolsetConfig{Name: "test", ServiceType: "test"})
				Expect(cache.GetToolset("test")).ToNot(BeNil())
			})

			It("should update toolset enabled status", func() {
				toolset := &holmesgpt.ToolsetConfig{
					Name:        "toggle-enabled",
					ServiceType: "test",
					Enabled:     true,
				}

				cache.SetToolset(toolset)

				// Update enabled status
				result := cache.UpdateToolsetEnabled("toggle-enabled", false)
				Expect(result).To(BeTrue())

				// Verify change
				updated := cache.GetToolset("toggle-enabled")
				Expect(updated.Enabled).To(BeFalse())
				Expect(updated.LastUpdated).To(BeTemporally("~", time.Now(), time.Second))
			})

			It("should return false when updating non-existent toolset", func() {
				result := cache.UpdateToolsetEnabled("non-existent", true)
				Expect(result).To(BeFalse())
			})

			It("should clear all toolsets", func() {
				// Add some toolsets
				cache.SetToolset(&holmesgpt.ToolsetConfig{Name: "test1", ServiceType: "test"})
				cache.SetToolset(&holmesgpt.ToolsetConfig{Name: "test2", ServiceType: "test"})

				Expect(cache.GetAllToolsets()).To(HaveLen(2))

				cache.Clear()

				Expect(cache.GetAllToolsets()).To(BeEmpty())

				// Stats should also be reset
				stats := cache.GetStats()
				Expect(stats.HitCount).To(Equal(int64(0)))
				Expect(stats.MissCount).To(Equal(int64(0)))
			})
		})

		Context("Cache Expiration Logic", func() {
			It("should not expire baseline toolsets", func() {
				// Create cache with very short TTL
				shortCache := holmesgpt.NewToolsetConfigCache(10*time.Millisecond, log)

				kubernetesToolset := &holmesgpt.ToolsetConfig{
					Name:        "kubernetes",
					ServiceType: "kubernetes",                   // Baseline service type
					LastUpdated: time.Now().Add(-1 * time.Hour), // Very old
				}

				internetToolset := &holmesgpt.ToolsetConfig{
					Name:        "internet",
					ServiceType: "internet",                     // Baseline service type
					LastUpdated: time.Now().Add(-1 * time.Hour), // Very old
				}

				shortCache.SetToolset(kubernetesToolset)
				shortCache.SetToolset(internetToolset)

				// Wait for TTL to expire
				time.Sleep(20 * time.Millisecond)

				// Baseline toolsets should still be retrievable
				Expect(shortCache.GetToolset("kubernetes")).ToNot(BeNil())
				Expect(shortCache.GetToolset("internet")).ToNot(BeNil())
			})

			It("should expire non-baseline toolsets after TTL", func() {
				// Create cache with very short TTL for testing
				shortCache := holmesgpt.NewToolsetConfigCache(10*time.Millisecond, log)

				toolset := &holmesgpt.ToolsetConfig{
					Name:        "expiring-toolset",
					ServiceType: "prometheus",                   // Non-baseline
					LastUpdated: time.Now().Add(-1 * time.Hour), // Very old
				}

				shortCache.SetToolset(toolset)

				// Should be immediately expired due to old timestamp
				retrieved := shortCache.GetToolset("expiring-toolset")
				Expect(retrieved).To(BeNil()) // Should be expired
			})
		})

		Context("Cache Statistics", func() {
			It("should provide accurate cache statistics", func() {
				cache.Clear() // Start fresh

				// Add some toolsets
				cache.SetToolset(&holmesgpt.ToolsetConfig{Name: "stats1", ServiceType: "test"})
				cache.SetToolset(&holmesgpt.ToolsetConfig{Name: "stats2", ServiceType: "test"})

				// Perform some operations
				cache.GetToolset("stats1")  // Hit
				cache.GetToolset("stats1")  // Hit
				cache.GetToolset("missing") // Miss

				stats := cache.GetStats()

				Expect(stats.Size).To(Equal(2))
				Expect(stats.HitCount).To(Equal(int64(2)))
				Expect(stats.MissCount).To(Equal(int64(1)))
				Expect(stats.HitRate).To(BeNumerically("~", 2.0/3.0, 0.01))
				Expect(stats.TTL).To(Equal(5 * time.Minute))
			})
		})
	})
})
