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

package processing

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// StormDetector identifies alert storms using hybrid detection (rate + pattern)
//
// Storm detection prevents Gateway from creating hundreds of RemediationRequest CRDs
// when a widespread issue occurs (e.g., cluster-wide memory pressure, node failures).
//
// Two detection methods:
// 1. Rate-based: >10 alerts/minute for the same alertname
// 2. Pattern-based: >5 similar alerts across different resources in 2 minutes
//
// When a storm is detected:
// - Metrics are recorded (gateway_alert_storms_detected_total)
// - RemediationRequest CRD includes storm metadata (isStorm=true, stormType, affectedResources)
// - Downstream services can aggregate remediation actions
//
// Redis keys:
// - Rate: alert:storm:rate:<alertname> (1-minute TTL)
// - Pattern: alert:pattern:<alertname> (2-minute TTL, sorted set)
type StormDetector struct {
	redisClient      *redis.Client
	rateThreshold    int              // Default: 10 alerts/minute
	patternThreshold int              // Default: 5 similar alerts
	metrics          *metrics.Metrics // Day 9 Phase 6B Option C1: Centralized metrics
}

// NewStormDetector creates a new storm detector with configurable thresholds
//
// Parameters:
// - redisClient: Redis client for storm tracking
// - rateThreshold: Number of alerts/minute to trigger rate-based storm (default: 10)
// - patternThreshold: Number of similar alerts across resources to trigger pattern-based storm (default: 5)
//
// Default thresholds (when 0 is passed):
// - Rate: 10 alerts/minute (production default)
// - Pattern: 5 similar alerts (production default)
//
// Test thresholds (recommended for integration tests):
// - Rate: 2-3 alerts/minute (early storm detection for testing)
// - Pattern: 2 similar alerts (early pattern detection for testing)
//
// Thresholds are based on:
// - Prometheus default evaluation interval: 30-60 seconds
// - Typical remediation time: 3-10 minutes
// - Balance between early detection and false positives
func NewStormDetector(redisClient *redis.Client, rateThreshold, patternThreshold int, metricsInstance *metrics.Metrics) *StormDetector {
	// Apply defaults if zero values provided
	if rateThreshold == 0 {
		rateThreshold = 10 // >10 alerts/minute (production default)
	}
	if patternThreshold == 0 {
		patternThreshold = 5 // >5 similar alerts (production default)
	}

	return &StormDetector{
		redisClient:      redisClient,
		rateThreshold:    rateThreshold,
		patternThreshold: patternThreshold,
		metrics:          metricsInstance,
	}
}

// Check detects if an alert is part of a storm
//
// Detection order:
// 1. Rate-based storm check (fast, single INCR operation)
// 2. Pattern-based storm check (slower, ZADD + ZCARD operations)
//
// Returns:
// - bool: true if storm detected
// - *StormMetadata: Storm information (type, count, affected resources)
// - error: Redis errors
func (d *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
	startTime := time.Now()
	defer func() {
		d.metrics.RedisOperationDuration.WithLabelValues("storm_detection").Observe(time.Since(startTime).Seconds())
	}()

	// Check rate-based storm (faster check first)
	isRateStorm, err := d.checkRateStorm(ctx, signal)
	if err != nil {
		return false, nil, err
	}

	if isRateStorm {
		count := d.getRateCount(ctx, signal)
		d.metrics.AlertStormsDetectedTotal.WithLabelValues("rate", signal.AlertName).Inc()

		return true, &StormMetadata{
			StormType:  "rate",
			AlertCount: count,
			Window:     "1m",
		}, nil
	}

	// Check pattern-based storm
	isPatternStorm, affectedResources, err := d.checkPatternStorm(ctx, signal)
	if err != nil {
		return false, nil, err
	}

	if isPatternStorm {
		d.metrics.AlertStormsDetectedTotal.WithLabelValues("pattern", signal.AlertName).Inc()

		return true, &StormMetadata{
			StormType:         "pattern",
			AlertCount:        len(affectedResources),
			Window:            "2m",
			AffectedResources: affectedResources,
		}, nil
	}

	return false, nil, nil
}

// checkRateStorm detects if alert firing rate exceeds threshold
//
// Algorithm:
// 1. INCR alert:storm:rate:<alertname>
// 2. SET TTL to 1 minute on first increment
// 3. Check if count > threshold (10)
//
// Redis key: alert:storm:rate:<alertname>
// TTL: 1 minute (sliding window)
// Threshold: >10 alerts/minute
//
// Examples:
// - Normal: 2 alerts/minute → no storm
// - Storm: 15 alerts/minute → rate-based storm detected
//
// Returns:
// - bool: true if rate exceeds threshold
// - error: Redis errors
func (d *StormDetector) checkRateStorm(ctx context.Context, signal *types.NormalizedSignal) (bool, error) {
	startTime := time.Now()
	defer func() {
		d.metrics.RedisOperationDuration.WithLabelValues("storm_detection_rate").Observe(time.Since(startTime).Seconds())
	}()

	key := fmt.Sprintf("alert:storm:rate:%s", signal.AlertName)

	// Increment counter in Redis
	count, err := d.redisClient.Incr(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to increment storm counter: %w", err)
	}

	// Set 1-minute TTL on first increment
	if count == 1 {
		d.redisClient.Expire(ctx, key, 60*time.Second)
	}

	// Check if threshold exceeded (>10 alerts/minute)
	return count > int64(d.rateThreshold), nil
}

// getRateCount retrieves current rate counter value
//
// Used to populate StormMetadata.AlertCount for rate-based storms.
//
// Returns:
// - int: Current alert count in the 1-minute window
func (d *StormDetector) getRateCount(ctx context.Context, signal *types.NormalizedSignal) int {
	key := fmt.Sprintf("alert:storm:rate:%s", signal.AlertName)
	count, _ := d.redisClient.Get(ctx, key).Int()
	return count
}

// checkPatternStorm detects similar alerts across different resources
//
// Algorithm:
// 1. ZADD alert:pattern:<alertname> <timestamp> <resource-id>
// 2. ZREMRANGEBYSCORE to remove entries older than 2 minutes
// 3. ZCARD to count affected resources
// 4. Check if count > threshold (5)
//
// Redis key: alert:pattern:<alertname>
// Type: Sorted set (score = timestamp, member = resource identifier)
// TTL: 2 minutes
// Threshold: >5 similar alerts across different resources
//
// Examples:
// - Normal: 2 pods with same alert → no storm
// - Storm: 8 pods with OOMKilled alert in 2 minutes → pattern-based storm
//
// Resource identifier format: "namespace:Kind:name"
// Example: "prod-api:Pod:payment-api-789"
//
// Returns:
// - bool: true if pattern storm detected
// - []string: List of affected resource identifiers
// - error: Redis errors
func (d *StormDetector) checkPatternStorm(ctx context.Context, signal *types.NormalizedSignal) (bool, []string, error) {
	startTime := time.Now()
	defer func() {
		d.metrics.RedisOperationDuration.WithLabelValues("storm_detection_pattern").Observe(time.Since(startTime).Seconds())
	}()

	key := fmt.Sprintf("alert:pattern:%s", signal.AlertName)

	// Resource identifier: namespace:Kind:name
	resourceID := signal.Resource.String()
	now := float64(time.Now().Unix())

	// Add resource to sorted set (score = timestamp)
	if err := d.redisClient.ZAdd(ctx, key, &redis.Z{
		Score:  now,
		Member: resourceID,
	}).Err(); err != nil {
		return false, nil, fmt.Errorf("failed to add pattern entry: %w", err)
	}

	// Remove old entries (older than 2 minutes)
	twoMinutesAgo := now - 120
	d.redisClient.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%f", twoMinutesAgo))

	// Set 2-minute TTL
	d.redisClient.Expire(ctx, key, 2*time.Minute)

	// Count affected resources
	count, err := d.redisClient.ZCard(ctx, key).Result()
	if err != nil {
		return false, nil, fmt.Errorf("failed to count pattern entries: %w", err)
	}

	// Check if threshold exceeded (>5 similar alerts)
	if count > int64(d.patternThreshold) {
		// Get affected resources for metadata
		members, err := d.redisClient.ZRange(ctx, key, 0, -1).Result()
		if err != nil {
			return false, nil, fmt.Errorf("failed to retrieve pattern entries: %w", err)
		}
		return true, members, nil
	}

	return false, nil, nil
}

// StormMetadata contains information about a detected storm
//
// This struct is returned by Check() when a storm is detected.
// It's used to:
// - Populate RemediationRequest.Spec.IsStorm, StormType, StormAlertCount
// - Log storm detection events
// - Populate HTTP response (status: "accepted", isStorm: true)
type StormMetadata struct {
	// StormType indicates the detection method
	// Values: "rate" (frequency-based) or "pattern" (similar alerts)
	StormType string

	// AlertCount is the number of alerts in the storm
	// - Rate-based: Count in 1-minute window
	// - Pattern-based: Number of affected resources
	AlertCount int

	// Window is the time window for detection
	// - Rate-based: "1m" (1 minute)
	// - Pattern-based: "2m" (2 minutes)
	Window string

	// AffectedResources lists resources affected by the storm (pattern-based only)
	// Format: "namespace:Kind:name"
	// Example: ["prod-api:Pod:payment-api-789", "prod-api:Pod:payment-api-456"]
	// Max: 100 resources (to prevent excessive CRD size)
	AffectedResources []string
}
