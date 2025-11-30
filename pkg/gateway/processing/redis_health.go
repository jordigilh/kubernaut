package processing

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	goredis "github.com/go-redis/redis/v8"
)

// RedisHealthMonitor monitors Redis availability and updates metrics.
// DD-GATEWAY-003: Background health monitoring for Redis HA observability.
type RedisHealthMonitor struct {
	client               *goredis.Client
	logger               logr.Logger
	checkInterval        time.Duration
	onAvailabilityChange func(service string, available bool, duration time.Duration)
}

// NewRedisHealthMonitor creates a new Redis health monitor.
func NewRedisHealthMonitor(
	client *goredis.Client,
	logger logr.Logger,
	checkInterval time.Duration,
	onAvailabilityChange func(service string, available bool, duration time.Duration),
) *RedisHealthMonitor {
	return &RedisHealthMonitor{
		client:               client,
		logger:               logger.WithName("redis-health"),
		checkInterval:        checkInterval,
		onAvailabilityChange: onAvailabilityChange,
	}
}

// Start begins the background health monitoring goroutine.
// Checks Redis availability every checkInterval and updates metrics via callback.
//
// Context cancellation stops the monitoring loop gracefully.
func (m *RedisHealthMonitor) Start(ctx context.Context) {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	var unavailableSince time.Time
	wasAvailable := true

	m.logger.Info("Starting Redis health monitor",
		"interval", m.checkInterval)

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Redis health monitor stopped")
			return

		case <-ticker.C:
			available := m.checkRedisHealth(ctx)

			// Detect availability state change
			if available && !wasAvailable {
				// Redis became available
				outage := time.Since(unavailableSince)
				m.logger.Info("Redis became available",
					"outage_duration", outage)
				m.onAvailabilityChange("deduplication", true, outage)
				m.onAvailabilityChange("storm_detection", true, outage)
				wasAvailable = true
			} else if !available && wasAvailable {
				// Redis became unavailable
				unavailableSince = time.Now()
				m.logger.Info("Redis became unavailable")
				m.onAvailabilityChange("deduplication", false, 0)
				m.onAvailabilityChange("storm_detection", false, 0)
				wasAvailable = false
			} else if !available {
				// Redis still unavailable - update duration
				outage := time.Since(unavailableSince)
				m.onAvailabilityChange("deduplication", false, outage)
				m.onAvailabilityChange("storm_detection", false, outage)
			}
		}
	}
}

// checkRedisHealth performs a Redis PING to verify availability.
// Returns true if Redis is healthy, false otherwise.
func (m *RedisHealthMonitor) checkRedisHealth(ctx context.Context) bool {
	// Use short timeout for health check (don't block monitoring loop)
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err := m.client.Ping(checkCtx).Err()
	if err != nil {
		m.logger.V(1).Info("Redis health check failed", "error", err)
		return false
	}

	return true
}
