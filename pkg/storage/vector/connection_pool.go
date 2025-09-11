package vector

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
)

// ConnectionPool manages database connections with proper pooling, health checks, and retry logic
type ConnectionPool struct {
	db           *sql.DB
	config       *config.DatabaseConfig
	vectorConfig *config.VectorDBConfig
	logger       *logrus.Logger
	retrier      *DatabaseRetrier

	// Health check state
	healthCheckMutex    sync.RWMutex
	lastHealthCheck     time.Time
	isHealthy           bool
	healthCheckInterval time.Duration

	// Connection metrics
	connectionMetrics ConnectionMetrics
}

// ConnectionMetrics tracks connection pool statistics
type ConnectionMetrics struct {
	mutex                  sync.RWMutex
	TotalConnections       int
	ActiveConnections      int
	IdleConnections        int
	FailedConnections      int
	LastConnectionTime     time.Time
	AverageResponseTime    time.Duration
	HealthCheckFailures    int
	ConnectionRecycleCount int
}

// NewConnectionPool creates a new connection pool with the given configuration
func NewConnectionPool(dbConfig *config.DatabaseConfig, vectorConfig *config.VectorDBConfig, logger *logrus.Logger) (*ConnectionPool, error) {
	if logger == nil {
		logger = logrus.New()
	}

	pool := &ConnectionPool{
		config:              dbConfig,
		vectorConfig:        vectorConfig,
		logger:              logger,
		retrier:             NewDatabaseRetrier(logger),
		healthCheckInterval: 30 * time.Second, // Default health check interval
		connectionMetrics:   ConnectionMetrics{},
	}

	if err := pool.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize connection pool: %w", err)
	}

	return pool, nil
}

// initialize sets up the database connection with proper pooling configuration
func (cp *ConnectionPool) initialize() error {
	if !cp.config.Enabled {
		return fmt.Errorf("database is not enabled in configuration")
	}

	// Build connection string
	connStr := cp.buildConnectionString()

	// Open database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	// Configure connection pool settings
	cp.configureConnectionPool(db)

	cp.db = db

	// Test initial connection
	if err := cp.testConnection(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			cp.logger.WithError(closeErr).Error("Failed to close database connection during cleanup")
		}
		return fmt.Errorf("failed to establish initial connection: %w", err)
	}

	cp.logger.WithFields(logrus.Fields{
		"host":         cp.config.Host,
		"port":         cp.config.Port,
		"database":     cp.config.Database,
		"max_open":     cp.config.MaxOpenConns,
		"max_idle":     cp.config.MaxIdleConns,
		"max_lifetime": cp.config.ConnMaxLifetimeMinutes,
	}).Info("Database connection pool initialized successfully")

	return nil
}

// buildConnectionString creates a PostgreSQL connection string from configuration
func (cp *ConnectionPool) buildConnectionString() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cp.config.Host,
		cp.config.Port,
		cp.config.Username,
		cp.config.Password,
		cp.config.Database,
		cp.config.SSLMode,
	)
}

// configureConnectionPool sets up connection pool parameters
func (cp *ConnectionPool) configureConnectionPool(db *sql.DB) {
	// Set maximum number of open connections
	maxOpen := cp.config.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 25 // Default
	}
	db.SetMaxOpenConns(maxOpen)

	// Set maximum number of idle connections
	maxIdle := cp.config.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5 // Default
	}
	db.SetMaxIdleConns(maxIdle)

	// Set connection lifetime
	lifetimeMinutes := cp.config.ConnMaxLifetimeMinutes
	if lifetimeMinutes <= 0 {
		lifetimeMinutes = 5 // Default 5 minutes
	}
	db.SetConnMaxLifetime(time.Duration(lifetimeMinutes) * time.Minute)

	// Set connection idle timeout (Go 1.15+)
	db.SetConnMaxIdleTime(30 * time.Second)
}

// testConnection performs an initial connection test
func (cp *ConnectionPool) testConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := cp.retrier.ExecuteDBOperation(ctx, "initial_connection_test", func(ctx context.Context, attempt int) (any, error) {
		if err := cp.db.PingContext(ctx); err != nil {
			return nil, WrapRetryableError(err, true, "connection test failed")
		}
		return nil, nil
	})
	return err
}

// GetConnection returns the database connection (for backward compatibility)
func (cp *ConnectionPool) GetConnection() *sql.DB {
	return cp.db
}

// ExecuteWithRetry executes a database operation with retry logic and connection health monitoring
func (cp *ConnectionPool) ExecuteWithRetry(ctx context.Context, operationName string, operation func(*sql.DB) error) error {
	start := time.Now()

	_, err := cp.retrier.ExecuteDBOperation(ctx, operationName, func(ctx context.Context, attempt int) (any, error) {
		// Check connection health before operation
		if err := cp.ensureHealthyConnection(ctx); err != nil {
			return nil, WrapRetryableError(err, true, "connection health check failed")
		}

		// Execute the operation
		if err := operation(cp.db); err != nil {
			cp.recordConnectionError()

			// Determine if error is retryable
			if IsRetryableError(err) {
				return nil, WrapRetryableError(err, true, "database operation failed")
			}
			return nil, err // Non-retryable error
		}

		return nil, nil
	})

	// Record metrics
	cp.recordOperationMetrics(start, err == nil)

	return err
}

// ExecuteQueryWithRetry executes a query with retry logic and returns results
func (cp *ConnectionPool) ExecuteQueryWithRetry(ctx context.Context, operationName string, query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()

	result, err := cp.retrier.ExecuteDBOperation(ctx, operationName, func(ctx context.Context, attempt int) (any, error) {
		// Check connection health
		if err := cp.ensureHealthyConnection(ctx); err != nil {
			return nil, WrapRetryableError(err, true, "connection health check failed")
		}

		// Execute query
		rows, err := cp.db.QueryContext(ctx, query, args...)
		if err != nil {
			cp.recordConnectionError()
			if IsRetryableError(err) {
				return nil, WrapRetryableError(err, true, "query execution failed")
			}
			return nil, err
		}

		return rows, nil
	})

	// Record metrics
	cp.recordOperationMetrics(start, err == nil)

	if err != nil {
		return nil, err
	}

	return result.(*sql.Rows), nil
}

// ExecuteQueryRowWithRetry executes a single-row query with retry logic
func (cp *ConnectionPool) ExecuteQueryRowWithRetry(ctx context.Context, operationName string, query string, args ...interface{}) *sql.Row {
	// For QueryRow, we don't need retry logic as it doesn't return an error immediately
	// The error is returned when scanning the row
	return cp.db.QueryRowContext(ctx, query, args...)
}

// ensureHealthyConnection checks if the connection is healthy and reconnects if necessary
func (cp *ConnectionPool) ensureHealthyConnection(ctx context.Context) error {
	cp.healthCheckMutex.RLock()
	shouldCheck := time.Since(cp.lastHealthCheck) > cp.healthCheckInterval || !cp.isHealthy
	cp.healthCheckMutex.RUnlock()

	if !shouldCheck {
		return nil // Connection was recently checked and is healthy
	}

	cp.healthCheckMutex.Lock()
	defer cp.healthCheckMutex.Unlock()

	// Double-check pattern
	if time.Since(cp.lastHealthCheck) <= cp.healthCheckInterval && cp.isHealthy {
		return nil
	}

	// Perform health check
	err := cp.db.PingContext(ctx)
	cp.lastHealthCheck = time.Now()

	if err != nil {
		cp.isHealthy = false
		cp.connectionMetrics.mutex.Lock()
		cp.connectionMetrics.HealthCheckFailures++
		cp.connectionMetrics.mutex.Unlock()

		cp.logger.WithError(err).Warn("Database connection health check failed")
		return err
	}

	cp.isHealthy = true
	return nil
}

// recordConnectionError updates connection error metrics
func (cp *ConnectionPool) recordConnectionError() {
	cp.connectionMetrics.mutex.Lock()
	defer cp.connectionMetrics.mutex.Unlock()

	cp.connectionMetrics.FailedConnections++
}

// recordOperationMetrics updates operation performance metrics
func (cp *ConnectionPool) recordOperationMetrics(start time.Time, success bool) {
	duration := time.Since(start)

	cp.connectionMetrics.mutex.Lock()
	defer cp.connectionMetrics.mutex.Unlock()

	// Update average response time (simple moving average)
	if cp.connectionMetrics.AverageResponseTime == 0 {
		cp.connectionMetrics.AverageResponseTime = duration
	} else {
		cp.connectionMetrics.AverageResponseTime = (cp.connectionMetrics.AverageResponseTime + duration) / 2
	}

	if success {
		cp.connectionMetrics.LastConnectionTime = time.Now()
	}
}

// GetConnectionStats returns current connection pool statistics
func (cp *ConnectionPool) GetConnectionStats() *ConnectionStats {
	if cp.db == nil {
		return &ConnectionStats{
			Available: false,
		}
	}

	stats := cp.db.Stats()

	cp.connectionMetrics.mutex.RLock()
	defer cp.connectionMetrics.mutex.RUnlock()

	return &ConnectionStats{
		Available:           true,
		MaxOpenConnections:  stats.MaxOpenConnections,
		OpenConnections:     stats.OpenConnections,
		InUse:               stats.InUse,
		Idle:                stats.Idle,
		WaitCount:           stats.WaitCount,
		WaitDuration:        stats.WaitDuration,
		MaxIdleClosed:       stats.MaxIdleClosed,
		MaxLifetimeClosed:   stats.MaxLifetimeClosed,
		AverageResponseTime: cp.connectionMetrics.AverageResponseTime,
		FailedConnections:   cp.connectionMetrics.FailedConnections,
		HealthCheckFailures: cp.connectionMetrics.HealthCheckFailures,
		LastHealthCheck:     cp.lastHealthCheck,
		IsHealthy:           cp.isHealthy,
	}
}

// ConnectionStats represents connection pool statistics
type ConnectionStats struct {
	Available           bool
	MaxOpenConnections  int
	OpenConnections     int
	InUse               int
	Idle                int
	WaitCount           int64
	WaitDuration        time.Duration
	MaxIdleClosed       int64
	MaxLifetimeClosed   int64
	AverageResponseTime time.Duration
	FailedConnections   int
	HealthCheckFailures int
	LastHealthCheck     time.Time
	IsHealthy           bool
}

// IsHealthy performs a comprehensive health check of the connection pool
func (cp *ConnectionPool) IsHealthy(ctx context.Context) error {
	if cp.db == nil {
		return fmt.Errorf("database connection not initialized")
	}

	// Check basic connectivity
	if err := cp.ensureHealthyConnection(ctx); err != nil {
		return fmt.Errorf("connection health check failed: %w", err)
	}

	// Check connection pool statistics
	stats := cp.db.Stats()
	if stats.OpenConnections == 0 {
		return fmt.Errorf("no open connections available")
	}

	// Check if we're hitting connection limits
	if stats.WaitCount > 0 && stats.WaitDuration > 5*time.Second {
		cp.logger.WithFields(logrus.Fields{
			"wait_count":    stats.WaitCount,
			"wait_duration": stats.WaitDuration,
		}).Warn("Connection pool experiencing delays")
	}

	return nil
}

// Close closes all connections in the pool
func (cp *ConnectionPool) Close() error {
	if cp.db == nil {
		return nil
	}

	cp.logger.Info("Closing database connection pool")

	err := cp.db.Close()
	cp.db = nil

	if err != nil {
		return fmt.Errorf("failed to close database connection pool: %w", err)
	}

	return nil
}

// RecycleConnections forces recycling of idle connections
func (cp *ConnectionPool) RecycleConnections() {
	if cp.db == nil {
		return
	}

	cp.connectionMetrics.mutex.Lock()
	cp.connectionMetrics.ConnectionRecycleCount++
	cp.connectionMetrics.mutex.Unlock()

	// Force connection recycling by reducing max lifetime temporarily
	originalLifetime := 5 * time.Minute // Assume default
	cp.db.SetConnMaxLifetime(1 * time.Second)

	// Wait a moment for connections to expire
	time.Sleep(2 * time.Second)

	// Restore original lifetime
	cp.db.SetConnMaxLifetime(originalLifetime)

	cp.logger.Info("Recycled database connections")
}

// SetHealthCheckInterval configures the health check interval
func (cp *ConnectionPool) SetHealthCheckInterval(interval time.Duration) {
	cp.healthCheckMutex.Lock()
	defer cp.healthCheckMutex.Unlock()

	cp.healthCheckInterval = interval
	cp.logger.WithField("interval", interval).Info("Updated health check interval")
}
