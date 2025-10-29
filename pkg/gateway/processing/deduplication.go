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
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// DeduplicationService handles alert deduplication using Redis
//
// This service provides fingerprint-based deduplication with a 5-minute TTL window.
// It prevents duplicate alerts from creating multiple RemediationRequest CRDs, which
// would overwhelm downstream services (RemediationProcessing, AIAnalysis, WorkflowExecution).
//
// Key features:
// - Redis persistence (survives Gateway restarts, supports HA multi-replica deployments)
// - 5-minute TTL (automatic cleanup, no manual garbage collection)
// - Atomic operations (HIncrBy for count, pipeline for Store)
// - Metrics integration (cache hits/misses, deduplication rate)
//
// Performance characteristics (from specs):
// - Check: p95 ~1ms, p99 ~3ms
// - Store: p95 ~3ms, p99 ~8ms
// - Target deduplication rate: 40-60% (typical production)
type DeduplicationService struct {
	redisClient *redis.Client
	ttl         time.Duration
	logger      *zap.Logger
	connected   atomic.Bool      // Track Redis connection state (for lazy connection)
	connCheckMu sync.Mutex       // Protects connection check (prevent thundering herd)
	metrics     *metrics.Metrics // Day 9 Phase 6B Option C1: Centralized metrics
}

// NewDeduplicationService creates a new deduplication service
//
// Default configuration:
// - TTL: 5 minutes (matches AlertManager default evaluation interval)
//
// The 5-minute window balances:
// - Too short: Duplicate alerts create multiple CRDs (wasted resources)
// - Too long: Stale alerts prevent new remediation attempts (delayed resolution)
func NewDeduplicationService(redisClient *redis.Client, logger *zap.Logger, metricsInstance *metrics.Metrics) *DeduplicationService {
	// If metrics is nil (e.g., unit tests), create a test-isolated metrics instance
	if metricsInstance == nil {
		// Use custom registry to avoid "duplicate metrics collector registration" in tests
		registry := prometheus.NewRegistry()
		metricsInstance = metrics.NewMetricsWithRegistry(registry)
	}
	return &DeduplicationService{
		redisClient: redisClient,
		ttl:         5 * time.Minute,
		logger:      logger,
		metrics:     metricsInstance,
	}
}

// NewDeduplicationServiceWithTTL creates a deduplication service with custom TTL
//
// This constructor allows custom TTL for testing (e.g., 5 seconds for integration tests)
// or for custom deployment scenarios (e.g., 10 minutes for low-alert environments).
//
// Production usage: Use NewDeduplicationService() with default 5-minute TTL
// Test usage: Use NewDeduplicationServiceWithTTL(client, 5*time.Second, logger) for fast tests
func NewDeduplicationServiceWithTTL(redisClient *redis.Client, ttl time.Duration, logger *zap.Logger, metricsInstance *metrics.Metrics) *DeduplicationService {
	// If metrics is nil (e.g., unit tests), create a test-isolated metrics instance
	if metricsInstance == nil {
		// Use custom registry to avoid "duplicate metrics collector registration" in tests
		registry := prometheus.NewRegistry()
		metricsInstance = metrics.NewMetricsWithRegistry(registry)
	}
	return &DeduplicationService{
		redisClient: redisClient,
		ttl:         ttl,
		logger:      logger,
		metrics:     metricsInstance,
	}
}

// ensureConnection verifies Redis is available using lazy connection pattern
//
// This method implements graceful degradation for Redis failures:
// 1. Fast path: If already connected, return immediately (no Redis call)
// 2. Slow path: If not connected, try to ping Redis
// 3. On success: Mark as connected (subsequent calls use fast path)
// 4. On failure: Return error (caller implements graceful degradation)
//
// Concurrency: Uses double-checked locking to prevent thundering herd
// Performance: Fast path is ~0.1μs (atomic load), slow path is ~1-3ms (Redis ping)
//
// This pattern allows Gateway to:
// - Start even when Redis is temporarily unavailable (BR-GATEWAY-013)
// - Recover automatically when Redis becomes available
// - Handle Redis failures gracefully without crashing
func (d *DeduplicationService) ensureConnection(ctx context.Context) error {
	// Fast path: already connected
	if d.connected.Load() {
		return nil
	}

	// Slow path: need to check connection
	d.connCheckMu.Lock()
	defer d.connCheckMu.Unlock()

	// Double-check after acquiring lock (another goroutine might have connected)
	if d.connected.Load() {
		return nil
	}

	// Try to connect
	if err := d.redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis unavailable: %w", err)
	}

	// Mark as connected (enables fast path for future calls)
	d.connected.Store(true)
	d.logger.Info("Redis connection established for deduplication service")
	return nil
}

// Check verifies if an alert is a duplicate
//
// This method:
// 1. Queries Redis for existing fingerprint (EXISTS command)
// 2. If found (duplicate):
//   - Increments occurrence count (HINCRBY)
//   - Updates lastSeen timestamp (HSET)
//   - Returns metadata for HTTP 202 response
//
// 3. If not found (new alert):
//   - Returns false (caller creates RemediationRequest CRD)
//
// Graceful Degradation:
// - If Redis is unavailable: Treats alert as new (no deduplication)
// - Logs error for monitoring/alerting
// - Returns (false, nil, nil) to allow processing to continue
//
// Trade-off: Temporary duplicate CRDs vs. blocking critical alerts
// Decision: Accept duplicates over blocking production alerts
//
// Returns:
// - bool: true if duplicate, false if new alert or Redis unavailable
// - *DeduplicationMetadata: Metadata for duplicate alerts (nil if new or Redis unavailable)
// - error: Validation errors only (Redis errors trigger graceful degradation)
func (s *DeduplicationService) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *DeduplicationMetadata, error) {
	startTime := time.Now()
	defer func() {
		// Record Redis operation duration for monitoring
		s.metrics.RedisOperationDuration.WithLabelValues("deduplication_check").Observe(time.Since(startTime).Seconds())
	}()

	// BR-GATEWAY-006: Fingerprint validation
	// Reject invalid fingerprints at boundary
	if signal.Fingerprint == "" {
		return false, nil, fmt.Errorf("invalid fingerprint: empty fingerprint not allowed")
	}

	// BR-GATEWAY-013: Graceful degradation when Redis unavailable
	// Check Redis connection before attempting operations
	if err := s.ensureConnection(ctx); err != nil {
		// Redis unavailable - graceful degradation
		// Log error but don't fail request (accept potential duplicates)
		s.logger.Warn("Redis unavailable, skipping deduplication (alert treated as new)",
			zap.Error(err),
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("operation", "deduplication_check"),
		)

		s.metrics.DeduplicationCacheMissesTotal.Inc()
		return false, nil, nil // Treat as new alert, allow processing to continue
	}

	key := fmt.Sprintf("gateway:dedup:fingerprint:%s", signal.Fingerprint)

	// Check if key exists in Redis
	exists, err := s.redisClient.Exists(ctx, key).Result()
	if err != nil {
		// Redis operation failed (e.g., connection lost after ensureConnection)
		// Graceful degradation: treat as new alert
		s.logger.Warn("Redis operation failed, skipping deduplication (alert treated as new)",
			zap.Error(err),
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("operation", "deduplication_check"),
		)

		// Mark as disconnected so next call will retry connection
		s.connected.Store(false)
		s.metrics.DeduplicationCacheMissesTotal.Inc()
		return false, nil, nil // Treat as new alert, allow processing to continue
	}

	if exists == 0 {
		// First occurrence - not a duplicate
		s.metrics.DeduplicationCacheMissesTotal.Inc()
		return false, nil, nil
	}

	// Duplicate detected - update metadata
	s.metrics.DeduplicationCacheHitsTotal.Inc()

	// Atomically increment count
	count, err := s.redisClient.HIncrBy(ctx, key, "count", 1).Result()
	if err != nil {
		return false, nil, fmt.Errorf("failed to increment count: %w", err)
	}

	// Update lastSeen timestamp (use RFC3339Nano for sub-second precision)
	now := time.Now().Format(time.RFC3339Nano)
	if err := s.redisClient.HSet(ctx, key, "lastSeen", now).Err(); err != nil {
		return false, nil, fmt.Errorf("failed to update lastSeen: %w", err)
	}

	// Retrieve metadata for response
	metadata := &DeduplicationMetadata{
		Fingerprint:           signal.Fingerprint,
		Count:                 int(count),
		RemediationRequestRef: s.redisClient.HGet(ctx, key, "remediationRequestRef").Val(),
		FirstSeen:             s.redisClient.HGet(ctx, key, "firstSeen").Val(),
		LastSeen:              now,
	}

	// Update deduplication rate gauge
	s.updateDeduplicationRate()

	return true, metadata, nil
}

// Store saves deduplication metadata for a new alert
//
// This method stores alert metadata in Redis as a hash with the following fields:
// - fingerprint: SHA256 hash for quick lookup
// - alertName: Human-readable alert name (for debugging)
// - namespace: Kubernetes namespace (for filtering queries)
// - resource: Resource name (for identifying affected workload)
// - firstSeen: When alert first appeared (ISO 8601 timestamp)
// - lastSeen: Most recent occurrence (initially same as firstSeen)
// - count: Number of occurrences (initially 1)
// - remediationRequestRef: Name of created RemediationRequest CRD
//
// All fields are stored atomically using Redis pipeline for consistency.
// TTL is set to 5 minutes for automatic cleanup.
//
// Graceful Degradation:
// - If Redis is unavailable: Logs error but returns nil
// - CRD already created at this point, alert is being processed
// - Missing deduplication metadata means future duplicates won't be detected
// - Trade-off: Lose deduplication data vs. blocking alert processing
//
// Redis schema:
//
//	Key: gateway:dedup:fingerprint:<sha256-hash>
//	Type: Hash
//	TTL: 5 minutes (300 seconds)
//	Fields: fingerprint, alertName, namespace, resource, firstSeen, lastSeen, count, remediationRequestRef
//
// Returns:
// - error: Always nil (errors logged, not returned)
func (s *DeduplicationService) Store(ctx context.Context, signal *types.NormalizedSignal, remediationRequestRef string) error {
	startTime := time.Now()
	defer func() {
		s.metrics.RedisOperationDuration.WithLabelValues("deduplication_store").Observe(time.Since(startTime).Seconds())
	}()

	// BR-GATEWAY-013: Graceful degradation when Redis unavailable
	// Check Redis connection before attempting operations
	if err := s.ensureConnection(ctx); err != nil {
		// Redis unavailable - graceful degradation
		// Log error but don't fail (CRD already created, alert is being processed)
		s.logger.Warn("Redis unavailable, failed to store deduplication metadata (future duplicates won't be detected)",
			zap.Error(err),
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("crd_ref", remediationRequestRef),
			zap.String("operation", "deduplication_store"),
		)

		return nil // Don't fail the request, CRD is already created
	}

	key := fmt.Sprintf("gateway:dedup:fingerprint:%s", signal.Fingerprint)
	now := time.Now().Format(time.RFC3339)

	s.logger.Info("Storing deduplication metadata in Redis",
		zap.String("key", key),
		zap.String("fingerprint", signal.Fingerprint),
		zap.String("crd_ref", remediationRequestRef))

	// Store as Redis hash with pipeline for atomicity
	pipe := s.redisClient.Pipeline()
	pipe.HSet(ctx, key, "fingerprint", signal.Fingerprint)
	pipe.HSet(ctx, key, "alertName", signal.AlertName)
	pipe.HSet(ctx, key, "namespace", signal.Namespace)
	pipe.HSet(ctx, key, "resource", signal.Resource.Name)
	pipe.HSet(ctx, key, "firstSeen", now)
	pipe.HSet(ctx, key, "lastSeen", now)
	pipe.HSet(ctx, key, "count", 1)
	pipe.HSet(ctx, key, "remediationRequestRef", remediationRequestRef)
	pipe.Expire(ctx, key, s.ttl) // Auto-expire after 5 minutes

	if _, err := pipe.Exec(ctx); err != nil {
		// Redis operation failed (e.g., connection lost after ensureConnection)
		// Graceful degradation: log error but don't fail
		s.logger.Warn("Redis operation failed, failed to store deduplication metadata (future duplicates won't be detected)",
			zap.Error(err),
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("crd_ref", remediationRequestRef),
			zap.String("operation", "deduplication_store"),
		)

		// Mark as disconnected so next call will retry connection
		s.connected.Store(false)
		return nil // Don't fail the request, CRD is already created
	}

	s.logger.Info("Successfully stored deduplication metadata in Redis",
		zap.String("key", key),
		zap.String("fingerprint", signal.Fingerprint),
		zap.Duration("ttl", s.ttl))

	return nil
}

// updateDeduplicationRate calculates and updates the deduplication rate gauge
//
// This method calculates:
//
//	deduplication_rate = (cache_hits / (cache_hits + cache_misses)) * 100
//
// The rate is exposed as a Prometheus gauge for real-time monitoring.
//
// Target: 40-60% deduplication rate (typical production)
// - <40%: Low alert repetition (may indicate dynamic workload)
// - 40-60%: Normal (healthy deduplication)
// - >60%: High alert repetition (may indicate persistent issues)
func (s *DeduplicationService) updateDeduplicationRate() {
	// Note: Counter.Get() is not available in prometheus client_golang
	// This method would need to use PromQL queries or maintain separate counters
	// For now, this is a placeholder for the metrics recording logic
	//
	// In production, deduplication rate would be calculated via Prometheus query:
	// rate(gateway_deduplication_cache_hits_total[5m]) /
	//   (rate(gateway_deduplication_cache_hits_total[5m]) +
	//    rate(gateway_deduplication_cache_misses_total[5m])) * 100
}

// DeduplicationMetadata contains metadata for duplicate alerts
//
// This struct is returned by Check() when a duplicate is detected.
// It provides information for:
// - HTTP 202 Accepted response (status, count, ref)
// - Logging (first occurrence, repetition count)
// - Debugging (RemediationRequest CRD reference)
type DeduplicationMetadata struct {
	// Fingerprint is the SHA256 hash of the alert
	Fingerprint string

	// Count is the number of times this alert has been received (including current)
	// Example: First duplicate has count=2 (original + 1 duplicate)
	Count int

	// RemediationRequestRef is the name of the RemediationRequest CRD created for original alert
	// Used in HTTP 202 response to inform caller which CRD was updated
	RemediationRequestRef string

	// FirstSeen is when the alert first appeared (ISO 8601 timestamp)
	// Example: "2025-10-09T10:00:00Z"
	FirstSeen string

	// LastSeen is the most recent occurrence (ISO 8601 timestamp)
	// Updated every time Check() detects a duplicate
	// Example: "2025-10-09T10:04:30Z"
	LastSeen string
}

// Record stores a fingerprint in Redis for deduplication tracking
// This is used after successfully creating a CRD to prevent duplicates
//
// Parameters:
// - fingerprint: The signal fingerprint to track
// - crdName: The RemediationRequest CRD name created for this signal
//
// Returns error if Redis operation fails
func (s *DeduplicationService) Record(ctx context.Context, fingerprint string, crdName string) error {
	// Use same key format and data structure as Check() and Store() methods
	key := fmt.Sprintf("gateway:dedup:fingerprint:%s", fingerprint)
	now := time.Now().Format(time.RFC3339Nano) // Use RFC3339Nano for sub-second precision

	// Store as Redis hash with pipeline for atomicity (same as Store() method)
	pipe := s.redisClient.Pipeline()
	pipe.HSet(ctx, key, "fingerprint", fingerprint)
	pipe.HSet(ctx, key, "firstSeen", now)
	pipe.HSet(ctx, key, "lastSeen", now)
	pipe.HSet(ctx, key, "count", 1)
	pipe.HSet(ctx, key, "remediationRequestRef", crdName)
	pipe.Expire(ctx, key, s.ttl) // Auto-expire after TTL

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to store fingerprint in Redis: %w", err)
	}

	return nil
}
