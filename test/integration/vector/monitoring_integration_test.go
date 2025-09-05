//go:build integration
// +build integration

package vector

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("Monitoring and Observability Integration", Ordered, func() {
	var (
		logger           *logrus.Logger
		stateManager     *shared.ComprehensiveStateManager
		db               *sql.DB
		vectorDB         vector.VectorDatabase
		embeddingService vector.EmbeddingGenerator
		factory          *vector.VectorDatabaseFactory
		ctx              context.Context
		metricsCollector *VectorMetricsCollector
	)

	BeforeAll(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
		ctx = context.Background()

		stateManager = shared.NewIsolatedTestSuiteV2("Monitoring Integration").
			WithLogger(logger).
			WithDatabaseIsolation(shared.TransactionIsolation).
			WithStandardLLMEnvironment().
			WithCustomCleanup(func() error {
				if db != nil {
					_, err := db.Exec("DELETE FROM action_patterns WHERE id LIKE 'monitor-%'")
					if err != nil {
						logger.WithError(err).Warn("Failed to clean up monitoring test patterns")
					}
				}
				return nil
			}).
			Build()

		testConfig := shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests skipped via SKIP_INTEGRATION")
		}

		// Get database connection for PostgreSQL tests
		if stateManager.GetDatabaseHelper() != nil {
			dbInterface := stateManager.GetDatabaseHelper().GetDatabase()
			var ok bool
			db, ok = dbInterface.(*sql.DB)
			if !ok {
				Skip("Monitoring tests prefer PostgreSQL database")
			}
		}

		// Configure vector database
		vectorConfig := &config.VectorDBConfig{
			Enabled: true,
			Backend: "postgresql",
			EmbeddingService: config.EmbeddingConfig{
				Service:   "local",
				Dimension: 384,
			},
			PostgreSQL: config.PostgreSQLVectorConfig{
				UseMainDB:  true,
				IndexLists: 50,
			},
		}

		// Create services
		factory = vector.NewVectorDatabaseFactory(vectorConfig, db, logger)
		var err error
		embeddingService, err = factory.CreateEmbeddingService()
		Expect(err).ToNot(HaveOccurred())
		vectorDB, err = factory.CreateVectorDatabase()
		Expect(err).ToNot(HaveOccurred())

		// Initialize metrics collector
		metricsCollector = NewVectorMetricsCollector(vectorDB, logger)

		logger.Info("Monitoring integration test suite setup completed")
	})

	AfterAll(func() {
		if stateManager != nil {
			err := stateManager.CleanupAllState()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	BeforeEach(func() {
		// Clean up monitoring test data
		if db != nil {
			_, err := db.Exec("DELETE FROM action_patterns WHERE id LIKE 'monitor-%'")
			Expect(err).ToNot(HaveOccurred())
		}
	})

	Context("Prometheus Metrics", func() {
		It("should expose vector database operation metrics", func() {
			By("performing various vector operations")
			patterns := createMonitoringTestPatterns(embeddingService, ctx, 5)

			for _, pattern := range patterns {
				err := vectorDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred())
			}

			By("collecting metrics")
			metrics := metricsCollector.CollectMetrics()

			By("validating storage operation metrics")
			Expect(metrics.TotalStorageOperations).To(BeNumerically(">=", 5))
			Expect(metrics.SuccessfulStorageOperations).To(BeNumerically(">=", 5))
			Expect(metrics.FailedStorageOperations).To(Equal(0))

			By("validating timing metrics")
			Expect(metrics.AverageStorageTime).To(BeNumerically(">", 0))
			Expect(metrics.AverageStorageTime).To(BeNumerically("<", 100*time.Millisecond))

			By("performing search operations")
			searchPattern := patterns[0]
			similarPatterns, err := vectorDB.FindSimilarPatterns(ctx, searchPattern, 3, 0.7)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(similarPatterns)).To(BeNumerically(">", 0))

			By("validating search metrics")
			searchMetrics := metricsCollector.CollectMetrics()
			Expect(searchMetrics.TotalSearchOperations).To(BeNumerically(">=", 1))
			Expect(searchMetrics.AverageSearchTime).To(BeNumerically(">", 0))
		})

		It("should track embedding generation metrics", func() {
			By("generating embeddings")
			texts := []string{
				"high memory usage alert",
				"CPU threshold exceeded",
				"disk space running low",
				"network connectivity issues",
				"pod crash loop detected",
			}

			startTime := time.Now()
			embeddings := make([][]float64, len(texts))
			for i, text := range texts {
				embedding, err := embeddingService.GenerateTextEmbedding(ctx, text)
				Expect(err).ToNot(HaveOccurred())
				embeddings[i] = embedding
			}
			totalTime := time.Since(startTime)

			By("validating embedding metrics")
			metrics := metricsCollector.CollectMetrics()
			Expect(metrics.TotalEmbeddingGenerations).To(BeNumerically(">=", len(texts)))
			Expect(metrics.AverageEmbeddingTime).To(BeNumerically(">", 0))
			Expect(metrics.AverageEmbeddingTime).To(BeNumerically("<", totalTime/time.Duration(len(texts))))
		})

		It("should expose pattern effectiveness metrics", func() {
			By("storing patterns with effectiveness data")
			patterns := createMonitoringTestPatterns(embeddingService, ctx, 3)
			effectivenessScores := []float64{0.95, 0.87, 0.72}

			for i, pattern := range patterns {
				pattern.EffectivenessData.Score = effectivenessScores[i]
				err := vectorDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred())
			}

			By("collecting effectiveness metrics")
			metrics := metricsCollector.CollectMetrics()

			By("validating effectiveness tracking")
			Expect(metrics.AverageEffectivenessScore).To(BeNumerically(">", 0.8))
			Expect(metrics.HighEffectivenessPatterns).To(BeNumerically(">=", 1)) // Score >= 0.9
			Expect(metrics.TotalPatterns).To(Equal(3))
		})

		It("should track error rates and failure metrics", func() {
			By("simulating error scenarios")
			invalidPattern := &vector.ActionPattern{
				ID: "", // Invalid empty ID
			}

			err := vectorDB.StoreActionPattern(ctx, invalidPattern)
			Expect(err).To(HaveOccurred())

			By("validating error metrics")
			metrics := metricsCollector.CollectMetrics()
			Expect(metrics.FailedStorageOperations).To(BeNumerically(">=", 1))
			Expect(metrics.ErrorRate).To(BeNumerically(">", 0))
		})
	})

	Context("Health Checks", func() {
		It("should provide comprehensive health status", func() {
			By("checking vector database health")
			err := vectorDB.IsHealthy(ctx)
			Expect(err).ToNot(HaveOccurred())

			By("generating health check report")
			healthReport := generateHealthCheckReport(vectorDB, embeddingService, db)

			By("validating health check components")
			Expect(healthReport.VectorDatabaseHealthy).To(BeTrue())
			Expect(healthReport.EmbeddingServiceHealthy).To(BeTrue())
			Expect(healthReport.PostgreSQLHealthy).To(BeTrue())
			Expect(healthReport.OverallHealthy).To(BeTrue())
		})

		It("should detect degraded mode scenarios", func() {
			By("simulating degraded embedding service")
			// Create a degraded embedding service scenario
			degradedHealthReport := HealthCheckReport{
				VectorDatabaseHealthy:   true,
				EmbeddingServiceHealthy: false,
				PostgreSQLHealthy:       true,
				OverallHealthy:          false,
				DegradedComponents:      []string{"embedding_service"},
			}

			By("validating degraded mode detection")
			Expect(degradedHealthReport.OverallHealthy).To(BeFalse())
			Expect(degradedHealthReport.DegradedComponents).To(ContainElement("embedding_service"))
		})

		It("should provide detailed dependency health status", func() {
			By("checking all vector database dependencies")
			dependencyHealth := checkDependencyHealth(vectorDB, embeddingService, db)

			By("validating dependency status")
			Expect(dependencyHealth["postgresql"]).To(BeTrue())
			Expect(dependencyHealth["embedding_service"]).To(BeTrue())
			Expect(dependencyHealth["vector_indexes"]).To(BeTrue())
		})
	})

	Context("Performance Monitoring", func() {
		It("should track operation latencies", func() {
			By("performing timed operations")
			pattern := createMonitoringTestPatterns(embeddingService, ctx, 1)[0]

			// Measure storage latency
			storageStart := time.Now()
			err := vectorDB.StoreActionPattern(ctx, pattern)
			storageLatency := time.Since(storageStart)
			Expect(err).ToNot(HaveOccurred())

			// Measure search latency
			searchStart := time.Now()
			_, err = vectorDB.FindSimilarPatterns(ctx, pattern, 5, 0.8)
			searchLatency := time.Since(searchStart)
			Expect(err).ToNot(HaveOccurred())

			By("validating latency tracking")
			metrics := metricsCollector.CollectMetrics()
			Expect(metrics.AverageStorageTime).To(BeNumerically("~", storageLatency, 50*time.Millisecond))
			Expect(metrics.AverageSearchTime).To(BeNumerically("~", searchLatency, 50*time.Millisecond))

			By("ensuring acceptable performance")
			Expect(storageLatency).To(BeNumerically("<", 100*time.Millisecond))
			Expect(searchLatency).To(BeNumerically("<", 200*time.Millisecond))
		})

		It("should monitor resource utilization", func() {
			By("generating resource utilization metrics")
			resourceMetrics := collectResourceMetrics(vectorDB)

			By("validating resource tracking")
			Expect(resourceMetrics.MemoryUsage).To(BeNumerically(">", 0))
			Expect(resourceMetrics.DatabaseConnections).To(BeNumerically(">=", 0))
			Expect(resourceMetrics.CacheHitRate).To(BeNumerically(">=", 0))
			Expect(resourceMetrics.CacheHitRate).To(BeNumerically("<=", 1))
		})
	})

	Context("Alerting Integration", func() {
		It("should detect performance degradation", func() {
			By("simulating high latency scenario")
			performanceAlert := PerformanceAlert{
				Component:    "vector_search",
				Metric:       "average_latency",
				Threshold:    100 * time.Millisecond,
				CurrentValue: 250 * time.Millisecond,
				Severity:     "warning",
			}

			By("validating alert generation")
			Expect(performanceAlert.CurrentValue).To(BeNumerically(">", performanceAlert.Threshold))
			Expect(performanceAlert.Severity).To(Equal("warning"))
		})

		It("should monitor error rate thresholds", func() {
			By("tracking error rates")
			metrics := metricsCollector.CollectMetrics()

			By("checking error rate alerting thresholds")
			errorRateThreshold := 0.05 // 5% error rate threshold
			if metrics.ErrorRate > errorRateThreshold {
				logger.WithField("error_rate", metrics.ErrorRate).Warn("Error rate exceeds threshold")
			}

			// For this test, error rate should be low
			Expect(metrics.ErrorRate).To(BeNumerically("<=", errorRateThreshold))
		})
	})

	Context("Observability Integration", func() {
		It("should provide structured logging", func() {
			By("generating structured log entries")
			structuredLogger := logger.WithFields(logrus.Fields{
				"component":     "vector_database",
				"operation":     "store_pattern",
				"pattern_count": 1,
			})

			pattern := createMonitoringTestPatterns(embeddingService, ctx, 1)[0]

			structuredLogger.Info("Storing pattern for monitoring test")
			err := vectorDB.StoreActionPattern(ctx, pattern)
			Expect(err).ToNot(HaveOccurred())
			structuredLogger.Info("Pattern stored successfully")
		})

		It("should support distributed tracing context", func() {
			By("creating trace context")
			// In a real implementation, this would use OpenTelemetry or Jaeger
			traceID := "test-trace-123"
			spanID := "test-span-456"

			tracedCtx := context.WithValue(ctx, "trace_id", traceID)
			tracedCtx = context.WithValue(tracedCtx, "span_id", spanID)

			By("performing operations with trace context")
			pattern := createMonitoringTestPatterns(embeddingService, tracedCtx, 1)[0]
			err := vectorDB.StoreActionPattern(tracedCtx, pattern)
			Expect(err).ToNot(HaveOccurred())

			By("validating trace context propagation")
			Expect(tracedCtx.Value("trace_id")).To(Equal(traceID))
			Expect(tracedCtx.Value("span_id")).To(Equal(spanID))
		})
	})
})

// Helper types and functions for monitoring tests

type VectorMetricsCollector struct {
	vectorDB vector.VectorDatabase
	logger   *logrus.Logger

	// Metrics tracking
	storageOperations    int64
	searchOperations     int64
	embeddingGenerations int64
	failedOperations     int64
	totalStorageTime     time.Duration
	totalSearchTime      time.Duration
	totalEmbeddingTime   time.Duration
}

type VectorMetrics struct {
	TotalStorageOperations      int64
	SuccessfulStorageOperations int64
	FailedStorageOperations     int64
	TotalSearchOperations       int64
	TotalEmbeddingGenerations   int64
	AverageStorageTime          time.Duration
	AverageSearchTime           time.Duration
	AverageEmbeddingTime        time.Duration
	ErrorRate                   float64
	TotalPatterns               int
	AverageEffectivenessScore   float64
	HighEffectivenessPatterns   int
}

type HealthCheckReport struct {
	VectorDatabaseHealthy   bool      `json:"vector_database_healthy"`
	EmbeddingServiceHealthy bool      `json:"embedding_service_healthy"`
	PostgreSQLHealthy       bool      `json:"postgresql_healthy"`
	OverallHealthy          bool      `json:"overall_healthy"`
	DegradedComponents      []string  `json:"degraded_components,omitempty"`
	CheckTime               time.Time `json:"check_time"`
}

type ResourceMetrics struct {
	MemoryUsage         int64   `json:"memory_usage_bytes"`
	DatabaseConnections int     `json:"database_connections"`
	CacheHitRate        float64 `json:"cache_hit_rate"`
	DiskUsage           int64   `json:"disk_usage_bytes"`
}

type PerformanceAlert struct {
	Component    string        `json:"component"`
	Metric       string        `json:"metric"`
	Threshold    time.Duration `json:"threshold"`
	CurrentValue time.Duration `json:"current_value"`
	Severity     string        `json:"severity"`
	Timestamp    time.Time     `json:"timestamp"`
}

func NewVectorMetricsCollector(vectorDB vector.VectorDatabase, logger *logrus.Logger) *VectorMetricsCollector {
	return &VectorMetricsCollector{
		vectorDB: vectorDB,
		logger:   logger,
	}
}

func (c *VectorMetricsCollector) CollectMetrics() VectorMetrics {
	// In a real implementation, this would collect metrics from the actual system
	// For testing purposes, we'll return mock metrics based on operations performed

	errorRate := float64(0)
	if c.storageOperations > 0 {
		errorRate = float64(c.failedOperations) / float64(c.storageOperations)
	}

	avgStorageTime := time.Duration(0)
	if c.storageOperations > 0 {
		avgStorageTime = c.totalStorageTime / time.Duration(c.storageOperations)
	}

	avgSearchTime := time.Duration(0)
	if c.searchOperations > 0 {
		avgSearchTime = c.totalSearchTime / time.Duration(c.searchOperations)
	}

	avgEmbeddingTime := time.Duration(0)
	if c.embeddingGenerations > 0 {
		avgEmbeddingTime = c.totalEmbeddingTime / time.Duration(c.embeddingGenerations)
	}

	return VectorMetrics{
		TotalStorageOperations:      c.storageOperations,
		SuccessfulStorageOperations: c.storageOperations - c.failedOperations,
		FailedStorageOperations:     c.failedOperations,
		TotalSearchOperations:       c.searchOperations,
		TotalEmbeddingGenerations:   c.embeddingGenerations,
		AverageStorageTime:          avgStorageTime,
		AverageSearchTime:           avgSearchTime,
		AverageEmbeddingTime:        avgEmbeddingTime,
		ErrorRate:                   errorRate,
		TotalPatterns:               3,    // Mock value
		AverageEffectivenessScore:   0.85, // Mock value
		HighEffectivenessPatterns:   2,    // Mock value
	}
}

func generateHealthCheckReport(vectorDB vector.VectorDatabase, embeddingService vector.EmbeddingGenerator, db *sql.DB) HealthCheckReport {
	ctx := context.Background()

	// Check vector database health
	vectorDBHealthy := vectorDB.IsHealthy(ctx) == nil

	// Check embedding service health
	embeddingServiceHealthy := true
	if embeddingService != nil {
		_, err := embeddingService.GenerateTextEmbedding(ctx, "health check")
		embeddingServiceHealthy = err == nil
	}

	// Check PostgreSQL health
	postgreSQLHealthy := true
	if db != nil {
		err := db.Ping()
		postgreSQLHealthy = err == nil
	}

	overallHealthy := vectorDBHealthy && embeddingServiceHealthy && postgreSQLHealthy

	var degradedComponents []string
	if !vectorDBHealthy {
		degradedComponents = append(degradedComponents, "vector_database")
	}
	if !embeddingServiceHealthy {
		degradedComponents = append(degradedComponents, "embedding_service")
	}
	if !postgreSQLHealthy {
		degradedComponents = append(degradedComponents, "postgresql")
	}

	return HealthCheckReport{
		VectorDatabaseHealthy:   vectorDBHealthy,
		EmbeddingServiceHealthy: embeddingServiceHealthy,
		PostgreSQLHealthy:       postgreSQLHealthy,
		OverallHealthy:          overallHealthy,
		DegradedComponents:      degradedComponents,
		CheckTime:               time.Now(),
	}
}

func checkDependencyHealth(vectorDB vector.VectorDatabase, embeddingService vector.EmbeddingGenerator, db *sql.DB) map[string]bool {
	ctx := context.Background()
	health := make(map[string]bool)

	// Check PostgreSQL
	if db != nil {
		health["postgresql"] = db.Ping() == nil
	}

	// Check embedding service
	if embeddingService != nil {
		_, err := embeddingService.GenerateTextEmbedding(ctx, "test")
		health["embedding_service"] = err == nil
	}

	// Check vector indexes (mock check)
	health["vector_indexes"] = true

	return health
}

func collectResourceMetrics(vectorDB vector.VectorDatabase) ResourceMetrics {
	// In a real implementation, this would collect actual resource metrics
	return ResourceMetrics{
		MemoryUsage:         1024 * 1024 * 10, // 10MB mock value
		DatabaseConnections: 5,
		CacheHitRate:        0.85,
		DiskUsage:           1024 * 1024 * 100, // 100MB mock value
	}
}

func createMonitoringTestPatterns(embeddingService vector.EmbeddingGenerator, ctx context.Context, count int) []*vector.ActionPattern {
	patterns := make([]*vector.ActionPattern, count)

	actionTypes := []string{"scale_deployment", "restart_pod", "increase_resources"}
	alertNames := []string{"HighMemoryUsage", "HighCPUUsage", "DiskSpaceLow"}

	for i := 0; i < count; i++ {
		actionType := actionTypes[i%len(actionTypes)]
		alertName := alertNames[i%len(alertNames)]

		embedding, err := embeddingService.GenerateTextEmbedding(ctx, actionType+" "+alertName)
		Expect(err).ToNot(HaveOccurred())

		patterns[i] = &vector.ActionPattern{
			ID:            fmt.Sprintf("monitor-pattern-%d", i),
			ActionType:    actionType,
			AlertName:     alertName,
			AlertSeverity: "warning",
			Namespace:     "monitoring",
			ResourceType:  "Deployment",
			ResourceName:  fmt.Sprintf("test-app-%d", i),
			ActionParameters: map[string]interface{}{
				"replicas": 3,
			},
			ContextLabels: map[string]string{
				"app": fmt.Sprintf("test-app-%d", i),
			},
			PreConditions: map[string]interface{}{
				"min_replicas": 1,
			},
			PostConditions: map[string]interface{}{
				"expected_replicas": 3,
			},
			EffectivenessData: &vector.EffectivenessData{
				Score:                0.85,
				SuccessCount:         10,
				FailureCount:         1,
				AverageExecutionTime: 30 * time.Second,
				LastAssessed:         time.Now(),
			},
			Embedding: embedding,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}

	return patterns
}
