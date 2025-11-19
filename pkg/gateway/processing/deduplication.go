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

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
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
	k8sClient   *k8s.Client // DD-GATEWAY-009: K8s client for state-based deduplication
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
func NewDeduplicationService(redisClient *redis.Client, k8sClient *k8s.Client, logger *zap.Logger, metricsInstance *metrics.Metrics) *DeduplicationService {
	// If metrics is nil (e.g., unit tests), create a test-isolated metrics instance
	if metricsInstance == nil {
		// Use custom registry to avoid "duplicate metrics collector registration" in tests
		registry := prometheus.NewRegistry()
		metricsInstance = metrics.NewMetricsWithRegistry(registry)
	}
	return &DeduplicationService{
		redisClient: redisClient,
		k8sClient:   k8sClient,
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
func NewDeduplicationServiceWithTTL(redisClient *redis.Client, k8sClient *k8s.Client, ttl time.Duration, logger *zap.Logger, metricsInstance *metrics.Metrics) *DeduplicationService {
	// If metrics is nil (e.g., unit tests), create a test-isolated metrics instance
	if metricsInstance == nil {
		// Use custom registry to avoid "duplicate metrics collector registration" in tests
		registry := prometheus.NewRegistry()
		metricsInstance = metrics.NewMetricsWithRegistry(registry)
	}
	return &DeduplicationService{
		redisClient: redisClient,
		k8sClient:   k8sClient,
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

// Check verifies if an alert is a duplicate using state-based deduplication
//
// DD-GATEWAY-009: State-Based Deduplication
// This method checks CRD state in Kubernetes to determine if alert is duplicate:
// 1. Generate CRD name from fingerprint
// 2. Query Kubernetes for existing CRD
// 3. If CRD exists:
//   - Phase = "Pending" or "Processing" → Duplicate (update occurrenceCount)
//   - Phase = "Completed", "Failed", "Cancelled" → New incident (create new CRD)
//
// 4. If CRD doesn't exist → New incident (create CRD)
//
// Graceful Degradation:
// - If K8s API unavailable: Fall back to Redis time-based check
// - If Redis also unavailable: Return error (HTTP 503)
//
// Returns:
// - bool: true if duplicate, false if new alert
// - *DeduplicationMetadata: Metadata for duplicate alerts (nil if new)
// - error: Validation errors or infrastructure failures
func (s *DeduplicationService) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *DeduplicationMetadata, error) {
	startTime := time.Now()
	defer func() {
		// Record operation duration for monitoring
		s.metrics.RedisOperationDuration.WithLabelValues("deduplication_check").Observe(time.Since(startTime).Seconds())
	}()

	// BR-GATEWAY-006: Fingerprint validation
	if signal.Fingerprint == "" {
		return false, nil, fmt.Errorf("invalid fingerprint: empty fingerprint not allowed")
	}

	// DD-GATEWAY-009: If K8s client is nil (e.g., unit tests), fall back to Redis-based check
	if s.k8sClient == nil {
		s.logger.Debug("K8s client is nil, falling back to Redis-based deduplication",
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("namespace", signal.Namespace))
		return s.checkRedisDeduplication(ctx, signal)
	}

	// DD-GATEWAY-009: v1.0 uses direct K8s API queries (no Redis caching)
	// v1.1 will add informer pattern to reduce API load
	s.logger.Debug("Using K8s API for state-based deduplication",
		zap.String("fingerprint", signal.Fingerprint),
		zap.String("namespace", signal.Namespace))

	// DD-GATEWAY-009 + DD-015: Find CRDs by fingerprint using label selector
	// With DD-015, CRD names include timestamps, so we can't predict exact names.
	// Query by fingerprint label to find all matching CRDs, then filter for in-progress ones.
	crdList, err := s.k8sClient.ListRemediationRequestsByFingerprint(ctx, signal.Fingerprint)
	if err != nil {
		// K8s API error → graceful degradation (fall back to Redis)
		s.logger.Warn("K8s API unavailable for deduplication, falling back to Redis check",
			zap.Error(err),
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("namespace", signal.Namespace))

		// Fall back to existing Redis-based check
		return s.checkRedisDeduplication(ctx, signal)
	}

	// DD-GATEWAY-009 + DD-015: WHITELIST FINAL STATES for safety
	// PHILOSOPHY: Only allow new CRDs when we KNOW the state is final.
	//             Unknown states are conservatively treated as "in-progress" to prevent duplicate CRDs.
	//
	// WHITELIST (allow new CRD):
	// - Completed: Remediation succeeded, allow retry if issue recurs
	// - Failed: Remediation failed, allow retry
	// - Cancelled: User cancelled, allow retry
	//
	// ALL OTHER STATES (treat as duplicate, increment occurrenceCount):
	// - Pending: Known in-progress state
	// - Processing: Known in-progress state
	// - Unknown/Future states: Conservatively assume in-progress (safer than creating duplicates)
	//
	// RATIONALE: Future CRD states might include:
	// - "Validating", "Analyzing", "WaitingForApproval", "Paused", etc.
	// - Better to block a legitimate alert (duplicate) than create duplicate CRDs

	var inProgressCRD *remediationv1alpha1.RemediationRequest
	var finalStateCount, pendingCount, processingCount, unknownCount int

	for i := range crdList.Items {
		crd := &crdList.Items[i]

		// Skip if different namespace (fingerprints should be namespace-scoped)
		if crd.Namespace != signal.Namespace {
			continue
		}

		// Check if CRD is in a FINAL state (whitelist approach)
		switch crd.Status.OverallPhase {
		case "Completed", "Failed", "Cancelled":
			// WHITELIST: These are known final states → allow new CRD
			finalStateCount++
			// Don't set inProgressCRD, continue to next CRD

		default:
			// ALL OTHER STATES (Pending, Processing, Unknown, Future) → treat as in-progress
			if inProgressCRD == nil {
				inProgressCRD = crd

				// Log known vs unknown states for debugging
				switch crd.Status.OverallPhase {
				case "Pending":
					pendingCount++
				case "Processing":
					processingCount++
				default:
					unknownCount++
					s.logger.Warn("Unknown CRD state treated as in-progress (conservative)",
						zap.String("fingerprint", signal.Fingerprint),
						zap.String("crd_name", crd.Name),
						zap.String("unknown_phase", crd.Status.OverallPhase),
						zap.String("namespace", crd.Namespace))
				}
			}
		}
	}

	// No in-progress CRD found (all are in final states) → not a duplicate
	if inProgressCRD == nil {
		s.metrics.DeduplicationCacheMissesTotal.Inc()

		s.logger.Debug("All CRDs in final states, treating as new incident",
			zap.String("fingerprint", signal.Fingerprint),
			zap.String("namespace", signal.Namespace),
			zap.Int("total_crds", len(crdList.Items)),
			zap.Int("final_state_count", finalStateCount))

		return false, nil, nil
	}

	// In-progress CRD found → this is a duplicate
	s.metrics.DeduplicationCacheHitsTotal.Inc()

	metadata := &DeduplicationMetadata{
		Fingerprint:           signal.Fingerprint,
		Count:                 inProgressCRD.Spec.Deduplication.OccurrenceCount + 1,
		RemediationRequestRef: fmt.Sprintf("%s/%s", inProgressCRD.Namespace, inProgressCRD.Name),
		FirstSeen:             inProgressCRD.Spec.Deduplication.FirstSeen.Format(time.RFC3339),
		LastSeen:              time.Now().Format(time.RFC3339),
	}

	s.logger.Debug("Duplicate detected (in-progress CRD found)",
		zap.String("fingerprint", signal.Fingerprint),
		zap.String("crd_name", inProgressCRD.Name),
		zap.String("phase", inProgressCRD.Status.OverallPhase),
		zap.Int("occurrence_count", metadata.Count),
		zap.Int("total_crds_for_fingerprint", len(crdList.Items)),
		zap.Int("final_state_crds", finalStateCount))

	return true, metadata, nil
}

// GetCRDNameFromFingerprint generates CRD name prefix from fingerprint
//
// DD-GATEWAY-009 + DD-015: CRD naming strategy
// With DD-015, CRD names include timestamps: rr-<fingerprint-12>-<timestamp>
// This method returns the fingerprint prefix only (first 12 chars)
//
// Example: fingerprint "abc123...xyz789" → CRD name prefix "rr-abc123..."
//
// NOTE: With DD-015, this cannot generate the exact CRD name (missing timestamp).
// This is used as a fallback when Redis doesn't have the CRD reference.
// Deduplication relies on Redis storing the full CRD name (with timestamp).
//
// This method is public so server.go can use it for fallback CRD name generation
func (s *DeduplicationService) GetCRDNameFromFingerprint(fingerprint string) string {
	// Use first 12 chars of fingerprint for CRD name prefix
	// (matches DD-015 naming logic in crd_creator.go)
	fingerprintPrefix := fingerprint
	if len(fingerprintPrefix) > 12 {
		fingerprintPrefix = fingerprintPrefix[:12]
	}
	return fmt.Sprintf("rr-%s", fingerprintPrefix)
}

// checkRedisDeduplication performs time-based deduplication using Redis
//
// DD-GATEWAY-009: Graceful degradation fallback
// This method provides the original Redis-based deduplication logic
// as a fallback when K8s API is unavailable.
//
// Returns:
// - bool: true if duplicate (Redis key exists), false if new
// - *DeduplicationMetadata: Metadata for duplicate alerts
// - error: Redis errors or validation errors
func (s *DeduplicationService) checkRedisDeduplication(ctx context.Context, signal *types.NormalizedSignal) (bool, *DeduplicationMetadata, error) {
	// Check Redis connection
	if err := s.ensureConnection(ctx); err != nil {
		s.logger.Warn("Redis unavailable, cannot guarantee deduplication",
			zap.Error(err),
			zap.String("fingerprint", signal.Fingerprint))

		s.metrics.DeduplicationCacheMissesTotal.Inc()
		return false, nil, fmt.Errorf("redis unavailable: %w", err)
	}

	key := fmt.Sprintf("gateway:dedup:fingerprint:%s", signal.Fingerprint)

	// Check if key exists in Redis
	exists, err := s.redisClient.Exists(ctx, key).Result()
	if err != nil {
		// Redis operation failed
		s.logger.Warn("Redis operation failed, skipping deduplication",
			zap.Error(err),
			zap.String("fingerprint", signal.Fingerprint))

		s.connected.Store(false)
		s.metrics.DeduplicationCacheMissesTotal.Inc()
		return false, nil, nil // Treat as new alert
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

	// Update lastSeen timestamp
	now := time.Now().Format(time.RFC3339Nano)
	if err := s.redisClient.HSet(ctx, key, "lastSeen", now).Err(); err != nil {
		return false, nil, fmt.Errorf("failed to update lastSeen: %w", err)
	}

	// Refresh TTL on duplicate detection
	if err := s.redisClient.Expire(ctx, key, s.ttl).Err(); err != nil {
		return false, nil, fmt.Errorf("failed to refresh TTL: %w", err)
	}

	// Retrieve metadata for response
	metadata := &DeduplicationMetadata{
		Fingerprint:           signal.Fingerprint,
		Count:                 int(count),
		RemediationRequestRef: s.redisClient.HGet(ctx, key, "remediationRequestRef").Val(),
		FirstSeen:             s.redisClient.HGet(ctx, key, "firstSeen").Val(),
		LastSeen:              now,
	}

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
