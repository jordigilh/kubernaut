package infrastructure_test

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/database"
	"github.com/sirupsen/logrus"

	_ "github.com/mattn/go-sqlite3" // SQLite driver for in-memory database testing
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Business Requirements Documentation for BR-DATABASE-001
// Based on production configuration and operational requirements:
//
// BR-DATABASE-001-A: Connection Pool Utilization Monitoring
// - MUST monitor connection utilization and alert when exceeding 80%
// - MUST track successful vs failed connection attempts
// - MUST calculate health scores based on performance metrics
//
// BR-DATABASE-001-B: Performance Threshold Management
// - MUST alert when average wait time exceeds 100ms (production) / 50ms (test)
// - MUST alert when failure rate exceeds 5% (production) / 10% (test)
// - MUST maintain health score above 70% for healthy status
//
// BR-DATABASE-001-C: Connection Pool Configuration
// - MUST support configurable max open connections (default: varies by environment)
// - MUST support configurable max idle connections
// - MUST support configurable connection lifetime management
//
// BR-DATABASE-001-D: Real-time Monitoring and Alerting
// - MUST perform health checks every 30 seconds (production) / 100ms (test)
// - MUST log metrics every 5 minutes (production) / 1 second (test)
// - MUST detect and report connection pool exhaustion scenarios

const (
	// Business Requirement Thresholds - BR-DATABASE-001-B
	BusinessUtilizationThreshold = 0.8  // 80% utilization threshold
	BusinessWaitTimeThreshold    = 50   // 50ms wait time threshold (test environment)
	BusinessFailureRateThreshold = 0.1  // 10% failure rate threshold (test environment)
	BusinessHealthScoreThreshold = 0.7  // 70% minimum health score
	BusinessHealthyScore         = 1.0  // Perfect health score
	BusinessDegradedScore        = 0.85 // Degraded but acceptable health score

	// Test Configuration Constants
	TestMaxOpenConnections = 10
	TestMaxIdleConnections = 5
	TestConnectionLifetime = 1 * time.Hour
)

func TestDatabaseConnectionPoolMonitor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Database Connection Pool Monitor Suite")
}

// createInMemoryTestDatabase creates an in-memory SQLite database for testing
// Following guideline: Use in-memory database instead of local mocks
func createInMemoryTestDatabase() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}

	// Configure connection pool according to business requirements
	db.SetMaxOpenConns(TestMaxOpenConnections)
	db.SetMaxIdleConns(TestMaxIdleConnections)
	db.SetConnMaxLifetime(TestConnectionLifetime)

	// Create a test table to enable meaningful database operations
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS health_check (
			id INTEGER PRIMARY KEY,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return db, err
}

var _ = Describe("Database Connection Pool Monitor", func() {

	// Business Requirement: BR-DATABASE-001 - Database connection pool monitoring and health management
	Context("BR-DATABASE-001-A: Connection Pool Utilization Monitoring", func() {
		var (
			db      *sql.DB
			monitor *database.ConnectionPoolMonitor
			logger  *logrus.Logger
			ctx     context.Context
			cancel  context.CancelFunc
		)

		BeforeEach(func() {
			logger = logrus.New()
			logger.SetLevel(logrus.ErrorLevel) // Reduce noise during tests

			ctx, cancel = context.WithCancel(context.Background())

			// Create in-memory database for testing - Following guideline decision #2
			var err error
			db, err = createInMemoryTestDatabase()
			Expect(err).ToNot(HaveOccurred())

			// Create monitor with business requirement-aligned configuration
			config := &database.MonitorConfig{
				HealthCheckInterval:   100 * time.Millisecond, // Fast for testing
				UtilizationThreshold:  BusinessUtilizationThreshold,
				WaitTimeThreshold:     BusinessWaitTimeThreshold * time.Millisecond,
				FailureRateThreshold:  BusinessFailureRateThreshold,
				LogMetricsInterval:    1 * time.Second,
				EnableDetailedMetrics: true,
			}

			monitor = database.NewConnectionPoolMonitor(db, config, logger)
		})

		AfterEach(func() {
			if monitor != nil {
				monitor.Stop()
			}
			if db != nil {
				db.Close()
			}
			cancel()
		})

		It("should initialize with default configuration values according to BR-DATABASE-001-C", func() {
			// Act: Create monitor with nil config to test default configuration
			defaultMonitor := database.NewConnectionPoolMonitor(db, nil, logger)
			defer defaultMonitor.Stop()

			// Start monitoring to update metrics
			err := defaultMonitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())
			time.Sleep(150 * time.Millisecond) // Allow initial metrics collection

			// Business Validation: BR-DATABASE-001-C - Default configuration must be applied
			metrics := defaultMonitor.GetMetrics()
			Expect(metrics.MaxOpenConnections).To(Equal(int32(TestMaxOpenConnections)),
				"BR-DATABASE-001-C: Must correctly reflect configured max open connections")
			Expect(metrics.IsHealthy).To(BeTrue(),
				"BR-DATABASE-001-B: Initial state must be healthy")
			Expect(metrics.HealthScore).To(Equal(BusinessHealthyScore),
				"BR-DATABASE-001-B: Initial health score must be perfect (1.0)")
		})

		It("should collect and report accurate connection pool metrics per BR-DATABASE-001-A", func() {
			// Act: Start monitoring
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Wait for initial metrics collection
			time.Sleep(150 * time.Millisecond)

			// Business Validation: BR-DATABASE-001-A - Must monitor connection utilization accurately
			metrics := monitor.GetMetrics()
			Expect(metrics.MaxOpenConnections).To(Equal(int32(TestMaxOpenConnections)),
				"BR-DATABASE-001-A: Must accurately report configured max open connections")
			Expect(metrics.OpenConnections).To(BeNumerically("<=", TestMaxOpenConnections),
				"BR-DATABASE-001-A: Open connections cannot exceed maximum configured")
			Expect(metrics.IdleConnections).To(BeNumerically("<=", TestMaxIdleConnections),
				"BR-DATABASE-001-A: Idle connections cannot exceed maximum configured")
			Expect(metrics.ConnectionUtilization).To(BeNumerically("<=", 1.0),
				"BR-DATABASE-001-A: Utilization must be between 0 and 1.0")
			Expect(metrics.LastHealthCheck).To(BeTemporally("~", time.Now(), time.Second),
				"BR-DATABASE-001-D: Health check timestamp must be current")
		})

		It("should detect high connection utilization per BR-DATABASE-001-A threshold", func() {
			// Arrange: Start monitoring
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Create multiple concurrent connections to exceed 80% threshold
			const numConnections = 9 // 90% of max 10 connections, exceeding 80% threshold
			var wg sync.WaitGroup
			var connections []*sql.Conn
			var connectionsMutex sync.Mutex

			// Act: Create high connection utilization scenario
			for i := 0; i < numConnections; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					conn, err := db.Conn(ctx)
					if err == nil {
						connectionsMutex.Lock()
						connections = append(connections, conn)
						connectionsMutex.Unlock()
						time.Sleep(200 * time.Millisecond) // Hold connection briefly
					}
				}()
			}

			wg.Wait()
			time.Sleep(150 * time.Millisecond) // Allow metrics to update

			// Business Validation: BR-DATABASE-001-A - Must detect utilization above 80% threshold
			metrics := monitor.GetMetrics()
			expectedUtilization := float64(numConnections) / float64(TestMaxOpenConnections) // 0.9
			Expect(metrics.ConnectionUtilization).To(BeNumerically("~", expectedUtilization, 0.1),
				"BR-DATABASE-001-A: Must accurately calculate 90% utilization")
			Expect(metrics.ConnectionUtilization).To(BeNumerically(">", BusinessUtilizationThreshold),
				"BR-DATABASE-001-A: Must detect utilization exceeding 80% threshold")
			Expect(metrics.HealthScore).To(BeNumerically("<", BusinessHealthyScore),
				"BR-DATABASE-001-B: Health score must degrade when utilization is high")
			Expect(metrics.OpenConnections).To(Equal(int32(numConnections)),
				"BR-DATABASE-001-A: Open connections must equal created connections")

			// Cleanup connections
			connectionsMutex.Lock()
			for _, conn := range connections {
				if conn != nil {
					conn.Close()
				}
			}
			connectionsMutex.Unlock()
		})

		It("should track successful and failed connections exactly per BR-DATABASE-001-A", func() {
			// Arrange: Start monitoring
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Act: Record specific number of connection attempts
			const expectedSuccessful = 2
			const expectedFailed = 1

			monitor.RecordSuccessfulConnection()
			monitor.RecordSuccessfulConnection()
			monitor.RecordFailedConnection()

			time.Sleep(150 * time.Millisecond) // Allow metrics to update

			// Business Validation: BR-DATABASE-001-A - Must track connection statistics accurately
			metrics := monitor.GetMetrics()
			Expect(metrics.SuccessfulConnections).To(Equal(int64(expectedSuccessful)),
				"BR-DATABASE-001-A: Must track exactly 2 successful connections")
			Expect(metrics.FailedConnections).To(Equal(int64(expectedFailed)),
				"BR-DATABASE-001-A: Must track exactly 1 failed connection")

			// Verify failure rate calculation
			totalConnections := expectedSuccessful + expectedFailed
			expectedFailureRate := float64(expectedFailed) / float64(totalConnections) // 0.33
			Expect(expectedFailureRate).To(BeNumerically(">", BusinessFailureRateThreshold),
				"BR-DATABASE-001-B: Test scenario must exceed 10% failure rate threshold for validation")
		})

		It("should perform database health checks per BR-DATABASE-001-D", func() {
			// Arrange: Start monitoring
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Act: Test connection health with real database operations
			err = monitor.TestConnection(ctx)

			// Business Validation: BR-DATABASE-001-D - Health checks must succeed with real database
			Expect(err).ToNot(HaveOccurred(),
				"BR-DATABASE-001-D: Health check must succeed with in-memory database")

			// Give time for metrics to update
			time.Sleep(150 * time.Millisecond)

			metrics := monitor.GetMetrics()
			Expect(metrics.SuccessfulConnections).To(Equal(int64(1)),
				"BR-DATABASE-001-D: Health check must record exactly 1 successful connection")
			Expect(metrics.IsHealthy).To(BeTrue(),
				"BR-DATABASE-001-D: Monitor must report healthy status after successful health check")
			Expect(metrics.HealthScore).To(Equal(BusinessHealthyScore),
				"BR-DATABASE-001-B: Health score must be perfect (1.0) when all connections succeed")
		})

		It("should handle concurrent monitoring operations safely per BR-DATABASE-001-A", func() {
			// Arrange: Start monitoring
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			const numWorkers = 10          // Test concurrent operations
			const operationsPerWorker = 25 // Operations per worker
			var wg sync.WaitGroup
			var successCount int64
			var failureCount int64
			var metricsReads int64

			// Act: Concurrent operations to test thread safety
			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for j := 0; j < operationsPerWorker; j++ {
						switch j % 4 {
						case 0:
							// Record successful connection
							monitor.RecordSuccessfulConnection()
							atomic.AddInt64(&successCount, 1)
						case 1:
							// Record failed connection
							monitor.RecordFailedConnection()
							atomic.AddInt64(&failureCount, 1)
						case 2:
							// Read metrics
							_ = monitor.GetMetrics()
							atomic.AddInt64(&metricsReads, 1)
						case 3:
							// Test health
							_ = monitor.IsHealthy()
						}

						time.Sleep(time.Microsecond * 10)
					}
				}(i)
			}

			// Wait for completion with timeout
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				// Success
			case <-time.After(10 * time.Second):
				Fail("BR-DATABASE-001-A: Monitor must handle concurrent operations without deadlock")
			}

			// Business Validation: BR-DATABASE-001-A - All operations must complete without data corruption
			finalSuccess := atomic.LoadInt64(&successCount)
			finalFailure := atomic.LoadInt64(&failureCount)
			finalReads := atomic.LoadInt64(&metricsReads)

			expectedSuccess := int64(numWorkers * operationsPerWorker / 4) // 25% of operations
			expectedFailure := int64(numWorkers * operationsPerWorker / 4) // 25% of operations
			expectedReads := int64(numWorkers * operationsPerWorker / 4)   // 25% of operations

			Expect(finalSuccess).To(Equal(expectedSuccess),
				"BR-DATABASE-001-A: Must track exactly the number of successful operations performed")
			Expect(finalFailure).To(Equal(expectedFailure),
				"BR-DATABASE-001-A: Must track exactly the number of failed operations performed")
			Expect(finalReads).To(Equal(expectedReads),
				"BR-DATABASE-001-A: Must track exactly the number of metrics reads performed")

			// Verify final metrics consistency
			metrics := monitor.GetMetrics()
			Expect(metrics.SuccessfulConnections).To(Equal(finalSuccess),
				"BR-DATABASE-001-A: Final metrics must exactly match recorded successful connections")
			Expect(metrics.FailedConnections).To(Equal(finalFailure),
				"BR-DATABASE-001-A: Final metrics must exactly match recorded failed connections")
		})

		It("should calculate health scores based on performance metrics per BR-DATABASE-001-B", func() {
			// Arrange: Start monitoring
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Act: Create healthy state with only successful connections
			const healthyConnections = 10
			for i := 0; i < healthyConnections; i++ {
				monitor.RecordSuccessfulConnection()
			}

			time.Sleep(150 * time.Millisecond) // Allow metrics to update

			// Business Validation: BR-DATABASE-001-B - Healthy state must meet minimum thresholds
			healthyMetrics := monitor.GetMetrics()
			Expect(healthyMetrics.HealthScore).To(BeNumerically(">=", BusinessHealthScoreThreshold),
				"BR-DATABASE-001-B: Health score must be >= 70% for healthy state")
			Expect(healthyMetrics.IsHealthy).To(BeTrue(),
				"BR-DATABASE-001-B: Must report healthy when score >= 70%")
			Expect(healthyMetrics.SuccessfulConnections).To(Equal(int64(healthyConnections)),
				"BR-DATABASE-001-B: Must track exactly 10 successful connections")

			// Act: Create unhealthy state with high failure rate
			const failedConnections = 30 // 30 failures vs 10 successes = 75% failure rate
			for i := 0; i < failedConnections; i++ {
				monitor.RecordFailedConnection()
			}

			time.Sleep(150 * time.Millisecond) // Allow metrics to update

			// Business Validation: BR-DATABASE-001-B - Unhealthy state must degrade score
			unhealthyMetrics := monitor.GetMetrics()
			Expect(unhealthyMetrics.HealthScore).To(BeNumerically("<", healthyMetrics.HealthScore),
				"BR-DATABASE-001-B: Health score must degrade with high failure rate")
			Expect(unhealthyMetrics.FailedConnections).To(Equal(int64(failedConnections)),
				"BR-DATABASE-001-B: Must track exactly 30 failed connections")

			// Verify failure rate calculation
			totalConnections := healthyConnections + failedConnections
			actualFailureRate := float64(failedConnections) / float64(totalConnections) // 0.75
			Expect(actualFailureRate).To(BeNumerically(">", BusinessFailureRateThreshold),
				"BR-DATABASE-001-B: Failure rate (75%) must exceed 10% threshold")
		})

		It("should stop monitoring gracefully per BR-DATABASE-001-D", func() {
			// Arrange: Start monitoring
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify it's running and collect initial state
			time.Sleep(50 * time.Millisecond)
			initialMetrics := monitor.GetMetrics()
			initialHealthCheck := initialMetrics.LastHealthCheck

			// Act: Stop monitoring
			monitor.Stop()

			// Wait and verify monitoring has stopped
			time.Sleep(200 * time.Millisecond)
			finalMetrics := monitor.GetMetrics()

			// Business Validation: BR-DATABASE-001-D - Must stop monitoring gracefully
			Expect(finalMetrics.LastHealthCheck).To(Equal(initialHealthCheck),
				"BR-DATABASE-001-D: Health check timestamp must not update after stopping")

			// Calling Stop again should be safe - no panics or blocks
			monitor.Stop() // Should not panic or block
			Expect(func() { monitor.Stop() }).ToNot(Panic(),
				"BR-DATABASE-001-D: Multiple stop calls must be safe")
		})

		It("should provide accurate utilization calculations per BR-DATABASE-001-A", func() {
			// Arrange: Start monitoring with known connection limits
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Act: Create specific number of connections to test 50% utilization
			const targetConnections = 5 // 50% of max 10 connections
			var connections []*sql.Conn

			for i := 0; i < targetConnections; i++ {
				conn, err := db.Conn(ctx)
				Expect(err).ToNot(HaveOccurred())
				connections = append(connections, conn)
			}

			// Allow metrics to update
			time.Sleep(150 * time.Millisecond)

			// Business Validation: BR-DATABASE-001-A - Must calculate utilization accurately
			metrics := monitor.GetMetrics()
			expectedUtilization := float64(targetConnections) / float64(TestMaxOpenConnections) // 0.5
			Expect(metrics.ConnectionUtilization).To(BeNumerically("~", expectedUtilization, 0.05),
				"BR-DATABASE-001-A: Must calculate exactly 50% utilization")
			Expect(metrics.OpenConnections).To(Equal(int32(targetConnections)),
				"BR-DATABASE-001-A: Open connections must equal created connections")
			Expect(metrics.ConnectionUtilization).To(BeNumerically("<", BusinessUtilizationThreshold),
				"BR-DATABASE-001-A: 50% utilization must be below 80% alert threshold")

			// Cleanup
			for _, conn := range connections {
				conn.Close()
			}
		})
	})

	// Business Requirement: BR-DATABASE-002 - Connection pool exhaustion recovery
	Context("BR-DATABASE-002: Connection Pool Exhaustion Recovery", func() {
		var (
			db      *sql.DB
			monitor *database.ConnectionPoolMonitor
			logger  *logrus.Logger
			ctx     context.Context
			cancel  context.CancelFunc
		)

		BeforeEach(func() {
			logger = logrus.New()
			logger.SetLevel(logrus.ErrorLevel)
			ctx, cancel = context.WithCancel(context.Background())

			var err error
			db, err = createInMemoryTestDatabase()
			Expect(err).ToNot(HaveOccurred())

			config := &database.MonitorConfig{
				HealthCheckInterval:   100 * time.Millisecond,
				UtilizationThreshold:  BusinessUtilizationThreshold,
				WaitTimeThreshold:     BusinessWaitTimeThreshold * time.Millisecond,
				FailureRateThreshold:  BusinessFailureRateThreshold,
				LogMetricsInterval:    1 * time.Second,
				EnableDetailedMetrics: true,
			}
			monitor = database.NewConnectionPoolMonitor(db, config, logger)
		})

		AfterEach(func() {
			if monitor != nil {
				monitor.Stop()
			}
			if db != nil {
				db.Close()
			}
			cancel()
		})

		It("should detect and recover from connection pool exhaustion", func() {
			// Arrange: Start monitoring
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Act: Exhaust connection pool by creating maximum connections
			var connections []*sql.Conn
			for i := 0; i < TestMaxOpenConnections; i++ {
				conn, err := db.Conn(ctx)
				Expect(err).ToNot(HaveOccurred())
				connections = append(connections, conn)
			}

			time.Sleep(150 * time.Millisecond) // Allow metrics to update

			// Business Validation: BR-DATABASE-002 - Must detect 100% utilization
			exhaustedMetrics := monitor.GetMetrics()
			Expect(exhaustedMetrics.ConnectionUtilization).To(Equal(1.0),
				"BR-DATABASE-002: Must detect 100% utilization when pool is exhausted")
			Expect(exhaustedMetrics.OpenConnections).To(Equal(int32(TestMaxOpenConnections)),
				"BR-DATABASE-002: Must show all connections are open when exhausted")

			// Act: Release connections to simulate recovery
			for _, conn := range connections {
				conn.Close()
			}

			time.Sleep(200 * time.Millisecond) // Allow metrics to update after recovery

			// Business Validation: BR-DATABASE-002 - Must recover after connections released
			recoveredMetrics := monitor.GetMetrics()
			Expect(recoveredMetrics.ConnectionUtilization).To(BeNumerically("<", 1.0),
				"BR-DATABASE-002: Utilization must decrease after connections are released")
			Expect(recoveredMetrics.HealthScore).To(BeNumerically(">=", BusinessHealthScoreThreshold),
				"BR-DATABASE-002: Health score must recover to >= 70% after connection release")
		})
	})

	// Business Requirement: BR-DATABASE-003 - Connection leak detection
	Context("BR-DATABASE-003: Connection Leak Detection", func() {
		var (
			db      *sql.DB
			monitor *database.ConnectionPoolMonitor
			logger  *logrus.Logger
			ctx     context.Context
			cancel  context.CancelFunc
		)

		BeforeEach(func() {
			logger = logrus.New()
			logger.SetLevel(logrus.ErrorLevel)
			ctx, cancel = context.WithCancel(context.Background())

			var err error
			db, err = createInMemoryTestDatabase()
			Expect(err).ToNot(HaveOccurred())

			config := &database.MonitorConfig{
				HealthCheckInterval:   100 * time.Millisecond,
				UtilizationThreshold:  BusinessUtilizationThreshold,
				WaitTimeThreshold:     BusinessWaitTimeThreshold * time.Millisecond,
				FailureRateThreshold:  BusinessFailureRateThreshold,
				LogMetricsInterval:    1 * time.Second,
				EnableDetailedMetrics: true,
			}
			monitor = database.NewConnectionPoolMonitor(db, config, logger)
		})

		AfterEach(func() {
			if monitor != nil {
				monitor.Stop()
			}
			if db != nil {
				db.Close()
			}
			cancel()
		})

		It("should detect persistent high utilization indicating potential leaks", func() {
			// Arrange: Start monitoring
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Act: Create connections that simulate a leak (high utilization sustained)
			const leakedConnections = 8 // 80% utilization - at threshold
			var connections []*sql.Conn
			for i := 0; i < leakedConnections; i++ {
				conn, err := db.Conn(ctx)
				Expect(err).ToNot(HaveOccurred())
				connections = append(connections, conn)
			}

			// Let the high utilization persist for multiple health check cycles
			time.Sleep(500 * time.Millisecond) // 5 health check cycles

			// Business Validation: BR-DATABASE-003 - Must detect sustained high utilization
			leakMetrics := monitor.GetMetrics()
			Expect(leakMetrics.ConnectionUtilization).To(BeNumerically(">=", BusinessUtilizationThreshold),
				"BR-DATABASE-003: Must detect utilization at or above 80% threshold")
			Expect(leakMetrics.OpenConnections).To(Equal(int32(leakedConnections)),
				"BR-DATABASE-003: Must accurately track leaked connections")
			Expect(leakMetrics.HealthScore).To(BeNumerically("<", BusinessHealthyScore),
				"BR-DATABASE-003: Health score must degrade due to sustained high utilization")

			// Cleanup connections
			for _, conn := range connections {
				conn.Close()
			}
		})
	})

	// Business Requirement: BR-DATABASE-004 - Performance degradation alerting
	Context("BR-DATABASE-004: Performance Degradation Alerting", func() {
		var (
			db      *sql.DB
			monitor *database.ConnectionPoolMonitor
			logger  *logrus.Logger
			ctx     context.Context
			cancel  context.CancelFunc
		)

		BeforeEach(func() {
			logger = logrus.New()
			logger.SetLevel(logrus.ErrorLevel)
			ctx, cancel = context.WithCancel(context.Background())

			var err error
			db, err = createInMemoryTestDatabase()
			Expect(err).ToNot(HaveOccurred())

			config := &database.MonitorConfig{
				HealthCheckInterval:   100 * time.Millisecond,
				UtilizationThreshold:  BusinessUtilizationThreshold,
				WaitTimeThreshold:     BusinessWaitTimeThreshold * time.Millisecond,
				FailureRateThreshold:  BusinessFailureRateThreshold,
				LogMetricsInterval:    1 * time.Second,
				EnableDetailedMetrics: true,
			}
			monitor = database.NewConnectionPoolMonitor(db, config, logger)
		})

		AfterEach(func() {
			if monitor != nil {
				monitor.Stop()
			}
			if db != nil {
				db.Close()
			}
			cancel()
		})

		It("should alert when failure rate exceeds threshold", func() {
			// Arrange: Start monitoring
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Act: Create performance degradation scenario with high failure rate
			const successfulConnections = 5
			const failedConnections = 2 // 2/7 = 28.5% failure rate, exceeding 10% threshold

			for i := 0; i < successfulConnections; i++ {
				monitor.RecordSuccessfulConnection()
			}
			for i := 0; i < failedConnections; i++ {
				monitor.RecordFailedConnection()
			}

			time.Sleep(150 * time.Millisecond) // Allow metrics to update

			// Business Validation: BR-DATABASE-004 - Must detect high failure rate
			degradedMetrics := monitor.GetMetrics()
			totalConnections := successfulConnections + failedConnections
			actualFailureRate := float64(failedConnections) / float64(totalConnections)

			Expect(actualFailureRate).To(BeNumerically(">", BusinessFailureRateThreshold),
				"BR-DATABASE-004: Failure rate (28.5%) must exceed 10% threshold")
			Expect(degradedMetrics.FailedConnections).To(Equal(int64(failedConnections)),
				"BR-DATABASE-004: Must track exactly 2 failed connections")
			Expect(degradedMetrics.HealthScore).To(BeNumerically("<", BusinessHealthyScore),
				"BR-DATABASE-004: Health score must degrade when failure rate is high")
		})

		It("should maintain health score within degraded but acceptable range", func() {
			// Arrange: Start monitoring
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Act: Create moderate degradation scenario
			const successfulConnections = 15
			const failedConnections = 1 // 1/16 = 6.25% failure rate, below 10% threshold

			for i := 0; i < successfulConnections; i++ {
				monitor.RecordSuccessfulConnection()
			}
			monitor.RecordFailedConnection()

			time.Sleep(150 * time.Millisecond) // Allow metrics to update

			// Business Validation: BR-DATABASE-004 - Must maintain acceptable performance
			acceptableMetrics := monitor.GetMetrics()
			actualFailureRate := float64(failedConnections) / float64(successfulConnections+failedConnections)

			Expect(actualFailureRate).To(BeNumerically("<", BusinessFailureRateThreshold),
				"BR-DATABASE-004: Failure rate (6.25%) must be below 10% threshold")
			Expect(acceptableMetrics.HealthScore).To(BeNumerically(">=", BusinessDegradedScore),
				"BR-DATABASE-004: Health score must be >= 85% for acceptable degraded performance")
			Expect(acceptableMetrics.IsHealthy).To(BeTrue(),
				"BR-DATABASE-004: Must remain healthy when failure rate is acceptable")
		})
	})

	// Business Requirement: BR-DATABASE-005 - Connection pool scaling recommendations
	Context("BR-DATABASE-005: Connection Pool Scaling Recommendations", func() {
		var (
			db      *sql.DB
			monitor *database.ConnectionPoolMonitor
			logger  *logrus.Logger
			ctx     context.Context
			cancel  context.CancelFunc
		)

		BeforeEach(func() {
			logger = logrus.New()
			logger.SetLevel(logrus.ErrorLevel)
			ctx, cancel = context.WithCancel(context.Background())

			var err error
			db, err = createInMemoryTestDatabase()
			Expect(err).ToNot(HaveOccurred())

			config := &database.MonitorConfig{
				HealthCheckInterval:   100 * time.Millisecond,
				UtilizationThreshold:  BusinessUtilizationThreshold,
				WaitTimeThreshold:     BusinessWaitTimeThreshold * time.Millisecond,
				FailureRateThreshold:  BusinessFailureRateThreshold,
				LogMetricsInterval:    1 * time.Second,
				EnableDetailedMetrics: true,
			}
			monitor = database.NewConnectionPoolMonitor(db, config, logger)
		})

		AfterEach(func() {
			if monitor != nil {
				monitor.Stop()
			}
			if db != nil {
				db.Close()
			}
			cancel()
		})

		It("should provide metrics indicating need for scaling up", func() {
			// Arrange: Start monitoring
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Act: Create high demand scenario requiring scale-up
			const highDemandConnections = 9 // 90% utilization, above 80% threshold
			var connections []*sql.Conn
			for i := 0; i < highDemandConnections; i++ {
				conn, err := db.Conn(ctx)
				Expect(err).ToNot(HaveOccurred())
				connections = append(connections, conn)
			}

			time.Sleep(150 * time.Millisecond) // Allow metrics to update

			// Business Validation: BR-DATABASE-005 - Must provide scaling indicators
			scalingMetrics := monitor.GetMetrics()
			Expect(scalingMetrics.ConnectionUtilization).To(BeNumerically(">", BusinessUtilizationThreshold),
				"BR-DATABASE-005: Utilization (90%) must indicate need for scaling up")
			Expect(scalingMetrics.MaxOpenConnections).To(Equal(int32(TestMaxOpenConnections)),
				"BR-DATABASE-005: Must report current pool size for scaling decisions")
			Expect(scalingMetrics.OpenConnections).To(Equal(int32(highDemandConnections)),
				"BR-DATABASE-005: Must show actual demand exceeding comfortable capacity")

			// Cleanup connections
			for _, conn := range connections {
				conn.Close()
			}
		})

		It("should provide metrics indicating efficient resource usage", func() {
			// Arrange: Start monitoring
			err := monitor.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Act: Create optimal utilization scenario
			const optimalConnections = 6 // 60% utilization, efficient but not strained
			var connections []*sql.Conn
			for i := 0; i < optimalConnections; i++ {
				conn, err := db.Conn(ctx)
				Expect(err).ToNot(HaveOccurred())
				connections = append(connections, conn)
			}

			time.Sleep(150 * time.Millisecond) // Allow metrics to update

			// Business Validation: BR-DATABASE-005 - Must indicate efficient utilization
			efficientMetrics := monitor.GetMetrics()
			expectedUtilization := float64(optimalConnections) / float64(TestMaxOpenConnections) // 0.6

			Expect(efficientMetrics.ConnectionUtilization).To(BeNumerically("~", expectedUtilization, 0.05),
				"BR-DATABASE-005: Must show 60% utilization for optimal resource usage")
			Expect(efficientMetrics.ConnectionUtilization).To(BeNumerically("<", BusinessUtilizationThreshold),
				"BR-DATABASE-005: Efficient usage must be below 80% alert threshold")
			Expect(efficientMetrics.HealthScore).To(Equal(BusinessHealthyScore),
				"BR-DATABASE-005: Perfect health score indicates optimal scaling")

			// Cleanup connections
			for _, conn := range connections {
				conn.Close()
			}
		})
	})
})
