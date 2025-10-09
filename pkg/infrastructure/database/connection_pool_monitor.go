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

package database

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// ConnectionPoolMetrics contains metrics for database connection pool monitoring
// Business Requirement: BR-DATABASE-001 - Database connection pool monitoring
type ConnectionPoolMetrics struct {
	// Connection pool statistics
	MaxOpenConnections int32 `json:"max_open_connections"`
	OpenConnections    int32 `json:"open_connections"`
	InUseConnections   int32 `json:"in_use_connections"`
	IdleConnections    int32 `json:"idle_connections"`

	// Timing statistics
	WaitCount         int64 `json:"wait_count"`                // Total number of connections waited for
	WaitDuration      int64 `json:"wait_duration_nanoseconds"` // Total time blocked waiting for new connections
	MaxIdleClosed     int64 `json:"max_idle_closed"`           // Total number of connections closed due to SetMaxIdleConns
	MaxIdleTimeClosed int64 `json:"max_idle_time_closed"`      // Total number of connections closed due to SetConnMaxIdleTime
	MaxLifetimeClosed int64 `json:"max_lifetime_closed"`       // Total number of connections closed due to SetConnMaxLifetime

	// Performance metrics
	SuccessfulConnections int64   `json:"successful_connections"`
	FailedConnections     int64   `json:"failed_connections"`
	ConnectionUtilization float64 `json:"connection_utilization"` // Percentage of connections in use
	AverageWaitTime       float64 `json:"average_wait_time_ms"`   // Average wait time in milliseconds

	// Health indicators
	IsHealthy       bool      `json:"is_healthy"`
	LastHealthCheck time.Time `json:"last_health_check"`
	HealthScore     float64   `json:"health_score"` // 0.0 to 1.0 health score
}

// ConnectionPoolMonitor monitors database connection pool health and performance
// Business Requirement: BR-DATABASE-001 - Database connection pool monitoring with alerting
type ConnectionPoolMonitor struct {
	db           *sql.DB
	logger       *logrus.Logger
	config       *MonitorConfig
	metrics      *ConnectionPoolMetrics
	metricsMutex sync.RWMutex

	// Monitoring state
	isRunning         int32 // Atomic flag for monitoring status
	stopChannel       chan struct{}
	healthCheckTicker *time.Ticker

	// Atomic counters for thread-safe updates
	successCount int64
	failureCount int64
}

// MonitorConfig contains configuration for database connection pool monitoring
type MonitorConfig struct {
	HealthCheckInterval   time.Duration `yaml:"health_check_interval" json:"health_check_interval"`
	UtilizationThreshold  float64       `yaml:"utilization_threshold" json:"utilization_threshold"`   // Alert when utilization exceeds this
	WaitTimeThreshold     time.Duration `yaml:"wait_time_threshold" json:"wait_time_threshold"`       // Alert when average wait time exceeds this
	FailureRateThreshold  float64       `yaml:"failure_rate_threshold" json:"failure_rate_threshold"` // Alert when failure rate exceeds this
	LogMetricsInterval    time.Duration `yaml:"log_metrics_interval" json:"log_metrics_interval"`
	EnableDetailedMetrics bool          `yaml:"enable_detailed_metrics" json:"enable_detailed_metrics"`
}

// DefaultMonitorConfig returns a default monitoring configuration
func DefaultMonitorConfig() *MonitorConfig {
	return &MonitorConfig{
		HealthCheckInterval:   30 * time.Second,
		UtilizationThreshold:  0.8, // Alert when 80% of connections are in use
		WaitTimeThreshold:     100 * time.Millisecond,
		FailureRateThreshold:  0.05, // Alert when 5% of connections fail
		LogMetricsInterval:    5 * time.Minute,
		EnableDetailedMetrics: true,
	}
}

// NewConnectionPoolMonitor creates a new database connection pool monitor
func NewConnectionPoolMonitor(db *sql.DB, config *MonitorConfig, logger *logrus.Logger) *ConnectionPoolMonitor {
	if config == nil {
		config = DefaultMonitorConfig()
	}

	monitor := &ConnectionPoolMonitor{
		db:                db,
		logger:            logger,
		config:            config,
		metrics:           &ConnectionPoolMetrics{},
		stopChannel:       make(chan struct{}),
		healthCheckTicker: time.NewTicker(config.HealthCheckInterval),
	}

	logger.WithFields(logrus.Fields{
		"health_check_interval": config.HealthCheckInterval,
		"utilization_threshold": config.UtilizationThreshold,
		"wait_time_threshold":   config.WaitTimeThreshold,
	}).Info("Database connection pool monitor initialized")

	return monitor
}

// Start begins monitoring the database connection pool
func (cpm *ConnectionPoolMonitor) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&cpm.isRunning, 0, 1) {
		return nil // Already running
	}

	cpm.logger.Info("Starting database connection pool monitoring")

	// Start monitoring goroutines
	go cpm.healthCheckLoop(ctx)
	go cpm.metricsLoggingLoop(ctx)

	return nil
}

// Stop stops the connection pool monitor
func (cpm *ConnectionPoolMonitor) Stop() {
	if !atomic.CompareAndSwapInt32(&cpm.isRunning, 1, 0) {
		return // Already stopped
	}

	cpm.logger.Info("Stopping database connection pool monitoring")

	close(cpm.stopChannel)
	cpm.healthCheckTicker.Stop()
}

// GetMetrics returns current connection pool metrics
func (cpm *ConnectionPoolMonitor) GetMetrics() ConnectionPoolMetrics {
	cpm.metricsMutex.RLock()
	defer cpm.metricsMutex.RUnlock()

	// Return a copy to prevent data races
	return *cpm.metrics
}

// IsHealthy returns true if the connection pool is healthy
func (cpm *ConnectionPoolMonitor) IsHealthy() bool {
	cpm.metricsMutex.RLock()
	defer cpm.metricsMutex.RUnlock()

	return cpm.metrics.IsHealthy
}

// GetHealthScore returns the current health score (0.0 to 1.0)
func (cpm *ConnectionPoolMonitor) GetHealthScore() float64 {
	cpm.metricsMutex.RLock()
	defer cpm.metricsMutex.RUnlock()

	return cpm.metrics.HealthScore
}

// healthCheckLoop runs periodic health checks
func (cpm *ConnectionPoolMonitor) healthCheckLoop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			cpm.logger.WithField("panic", r).Error("Database connection pool monitor health check loop panicked")
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cpm.stopChannel:
			return
		case <-cpm.healthCheckTicker.C:
			cpm.updateMetrics()
		}
	}
}

// metricsLoggingLoop logs metrics periodically
func (cpm *ConnectionPoolMonitor) metricsLoggingLoop(ctx context.Context) {
	ticker := time.NewTicker(cpm.config.LogMetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cpm.stopChannel:
			return
		case <-ticker.C:
			cpm.logCurrentMetrics()
		}
	}
}

// updateMetrics updates connection pool metrics
func (cpm *ConnectionPoolMonitor) updateMetrics() {
	stats := cpm.db.Stats()

	cpm.metricsMutex.Lock()
	defer cpm.metricsMutex.Unlock()

	// Update basic connection statistics
	cpm.metrics.MaxOpenConnections = int32(stats.MaxOpenConnections)
	cpm.metrics.OpenConnections = int32(stats.OpenConnections)
	cpm.metrics.InUseConnections = int32(stats.InUse)
	cpm.metrics.IdleConnections = int32(stats.Idle)

	// Update timing statistics
	cpm.metrics.WaitCount = stats.WaitCount
	cpm.metrics.WaitDuration = int64(stats.WaitDuration)
	cpm.metrics.MaxIdleClosed = stats.MaxIdleClosed
	cpm.metrics.MaxIdleTimeClosed = stats.MaxIdleTimeClosed
	cpm.metrics.MaxLifetimeClosed = stats.MaxLifetimeClosed

	// Calculate derived metrics
	if cpm.metrics.MaxOpenConnections > 0 {
		cpm.metrics.ConnectionUtilization = float64(cpm.metrics.InUseConnections) / float64(cpm.metrics.MaxOpenConnections)
	}

	if cpm.metrics.WaitCount > 0 {
		cpm.metrics.AverageWaitTime = float64(cpm.metrics.WaitDuration) / float64(cpm.metrics.WaitCount) / 1e6 // Convert to milliseconds
	}

	// Update performance counters (atomic reads)
	cpm.metrics.SuccessfulConnections = atomic.LoadInt64(&cpm.successCount)
	cpm.metrics.FailedConnections = atomic.LoadInt64(&cpm.failureCount)

	// Calculate health score and status
	cpm.calculateHealthScore()
	cpm.metrics.LastHealthCheck = time.Now()

	// Check for alerts
	cpm.checkAlertConditions()
}

// calculateHealthScore calculates a health score based on various metrics
func (cpm *ConnectionPoolMonitor) calculateHealthScore() {
	score := 1.0

	// Factor in connection utilization (penalize high utilization)
	if cpm.metrics.ConnectionUtilization > cpm.config.UtilizationThreshold {
		utilizationPenalty := (cpm.metrics.ConnectionUtilization - cpm.config.UtilizationThreshold) / (1.0 - cpm.config.UtilizationThreshold)
		score -= utilizationPenalty * 0.4 // Up to 40% penalty for high utilization
	}

	// Factor in wait times (penalize long waits)
	if cpm.metrics.AverageWaitTime > cpm.config.WaitTimeThreshold.Seconds()*1000 {
		waitTimePenalty := (cpm.metrics.AverageWaitTime - cpm.config.WaitTimeThreshold.Seconds()*1000) / (1000.0) // Normalize to seconds
		if waitTimePenalty > 1.0 {
			waitTimePenalty = 1.0
		}
		score -= waitTimePenalty * 0.3 // Up to 30% penalty for long wait times
	}

	// Factor in failure rate
	totalConnections := cpm.metrics.SuccessfulConnections + cpm.metrics.FailedConnections
	if totalConnections > 0 {
		failureRate := float64(cpm.metrics.FailedConnections) / float64(totalConnections)
		if failureRate > cpm.config.FailureRateThreshold {
			failurePenalty := (failureRate - cpm.config.FailureRateThreshold) / (1.0 - cpm.config.FailureRateThreshold)
			score -= failurePenalty * 0.3 // Up to 30% penalty for high failure rate
		}
	}

	// Ensure score is between 0 and 1
	if score < 0 {
		score = 0
	}

	cpm.metrics.HealthScore = score
	cpm.metrics.IsHealthy = score > 0.7 // Consider healthy if score > 70%
}

// checkAlertConditions checks for alert conditions and logs warnings
func (cpm *ConnectionPoolMonitor) checkAlertConditions() {
	// High utilization alert
	if cpm.metrics.ConnectionUtilization > cpm.config.UtilizationThreshold {
		cpm.logger.WithFields(logrus.Fields{
			"utilization":          cpm.metrics.ConnectionUtilization,
			"threshold":            cpm.config.UtilizationThreshold,
			"open_connections":     cpm.metrics.OpenConnections,
			"max_open_connections": cpm.metrics.MaxOpenConnections,
		}).Warn("Database connection pool utilization is high")
	}

	// Long wait time alert
	if cpm.metrics.AverageWaitTime > cpm.config.WaitTimeThreshold.Seconds()*1000 {
		cpm.logger.WithFields(logrus.Fields{
			"average_wait_time_ms": cpm.metrics.AverageWaitTime,
			"threshold_ms":         cpm.config.WaitTimeThreshold.Seconds() * 1000,
			"wait_count":           cpm.metrics.WaitCount,
		}).Warn("Database connection pool average wait time is high")
	}

	// High failure rate alert
	totalConnections := cpm.metrics.SuccessfulConnections + cpm.metrics.FailedConnections
	if totalConnections > 0 {
		failureRate := float64(cpm.metrics.FailedConnections) / float64(totalConnections)
		if failureRate > cpm.config.FailureRateThreshold {
			cpm.logger.WithFields(logrus.Fields{
				"failure_rate":       failureRate,
				"threshold":          cpm.config.FailureRateThreshold,
				"failed_connections": cpm.metrics.FailedConnections,
				"total_connections":  totalConnections,
			}).Warn("Database connection failure rate is high")
		}
	}
}

// logCurrentMetrics logs current metrics for monitoring
func (cpm *ConnectionPoolMonitor) logCurrentMetrics() {
	metrics := cpm.GetMetrics()

	cpm.logger.WithFields(logrus.Fields{
		"max_open_connections":   metrics.MaxOpenConnections,
		"open_connections":       metrics.OpenConnections,
		"in_use_connections":     metrics.InUseConnections,
		"idle_connections":       metrics.IdleConnections,
		"connection_utilization": metrics.ConnectionUtilization,
		"wait_count":             metrics.WaitCount,
		"average_wait_time_ms":   metrics.AverageWaitTime,
		"successful_connections": metrics.SuccessfulConnections,
		"failed_connections":     metrics.FailedConnections,
		"health_score":           metrics.HealthScore,
		"is_healthy":             metrics.IsHealthy,
	}).Info("Database connection pool metrics")
}

// RecordSuccessfulConnection records a successful database connection
func (cpm *ConnectionPoolMonitor) RecordSuccessfulConnection() {
	atomic.AddInt64(&cpm.successCount, 1)
}

// RecordFailedConnection records a failed database connection
func (cpm *ConnectionPoolMonitor) RecordFailedConnection() {
	atomic.AddInt64(&cpm.failureCount, 1)
}

// TestConnection performs a test connection to validate database health
func (cpm *ConnectionPoolMonitor) TestConnection(ctx context.Context) error {
	start := time.Now()

	// Test with a simple ping
	err := cpm.db.PingContext(ctx)

	duration := time.Since(start)

	if err != nil {
		cpm.RecordFailedConnection()
		cpm.logger.WithError(err).WithField("duration_ms", duration.Milliseconds()).Error("Database connection test failed")
		return err
	}

	cpm.RecordSuccessfulConnection()
	cpm.logger.WithField("duration_ms", duration.Milliseconds()).Debug("Database connection test successful")

	return nil
}
