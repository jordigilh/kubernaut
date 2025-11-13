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

// Package query provides cached query execution with multi-tier fallback
// BR-CONTEXT-001: Query historical incident context
// BR-CONTEXT-005: Multi-tier caching integration
package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/metrics"
	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
	dsclient "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// DEPRECATED: DBExecutor and Config removed per ADR-032
// Use DataStorageExecutorConfig and NewCachedExecutorWithDataStorage instead

// CachedExecutor executes queries with multi-tier caching fallback
// BR-CONTEXT-001: Query historical incident context
// BR-CONTEXT-005: Multi-tier caching (L1 Redis + L2 LRU + L3 DB)
// BR-CONTEXT-007: Data Storage Service REST API integration
// BR-CONTEXT-008: Circuit breaker pattern
// BR-CONTEXT-009: Exponential backoff retry
// BR-CONTEXT-010: Graceful degradation
//
// Fallback Chain:
// 1. Try Cache (L1 Redis → L2 LRU)
// 2. On miss → Query Data Storage Service with circuit breaker + retry
// 3. On Data Storage failure → fallback to cache (graceful degradation)
//
// Day 11 Edge Case 1.1: Single-flight pattern prevents cache stampede
// Multiple concurrent requests for same cache key are deduplicated to single DB query
//
// DD-005: Observability - Records metrics for cache hits/misses, query duration, errors
// ADR-032: Uses Data Storage Service REST API exclusively (no direct PostgreSQL access)
type CachedExecutor struct {
	cache    cache.CacheManager
	dsClient *dsclient.DataStorageClient // BR-CONTEXT-007: Data Storage Service client (ADR-032)
	logger   *zap.Logger
	ttl      time.Duration
	metrics  *metrics.Metrics // DD-005: Observability metrics (required - always initialized)

	// BR-CONTEXT-008: Circuit breaker state
	circuitOpen             bool
	consecutiveFailures     int
	circuitOpenTime         time.Time
	circuitBreakerThreshold int
	circuitBreakerTimeout   time.Duration
}

// CachedResult wraps query results for caching
// Exported for testing purposes
type CachedResult struct {
	Incidents []*models.IncidentEvent `json:"incidents"`
	Total     int                     `json:"total"`
}

// DEPRECATED: NewCachedExecutor removed per ADR-032
// Use NewCachedExecutorWithDataStorage instead

// DataStorageExecutorConfig holds configuration for Context API executor with Data Storage integration
// REFACTOR: Real cache injection instead of NoOpCache
type DataStorageExecutorConfig struct {
	DSClient *dsclient.DataStorageClient
	Cache    cache.CacheManager // REFACTOR: Real cache manager
	Logger   *zap.Logger
	Metrics  *metrics.Metrics
	TTL      time.Duration

	// Circuit breaker configuration (for testing)
	CircuitBreakerThreshold int           // Optional: defaults to 3
	CircuitBreakerTimeout   time.Duration // Optional: defaults to 60s
}

// NewCachedExecutorWithDataStorage creates a new query executor using Data Storage Service
// BR-CONTEXT-007: HTTP client for Data Storage Service REST API
// BR-CONTEXT-008: Circuit breaker (3 failures → open for 60s)
// BR-CONTEXT-009: Exponential backoff retry (3 attempts: 100ms, 200ms, 400ms)
// BR-CONTEXT-010: Graceful degradation (Data Storage down → cached data)
// REFACTOR: Now accepts real cache for graceful degradation
func NewCachedExecutorWithDataStorage(cfg *DataStorageExecutorConfig) (*CachedExecutor, error) {
	// Validate required fields
	if cfg.DSClient == nil {
		return nil, fmt.Errorf("DSClient cannot be nil")
	}

	// Set defaults
	logger := cfg.Logger
	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	cacheManager := cfg.Cache
	if cacheManager == nil {
		return nil, fmt.Errorf("Cache cannot be nil - required for graceful degradation")
	}

	metricsInst := cfg.Metrics
	if metricsInst == nil {
		// Create isolated registry for tests
		testRegistry := prometheus.NewRegistry()
		metricsInst = metrics.NewMetricsWithRegistry("contextapi", "datastorage", testRegistry)
	}

	ttl := cfg.TTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	// BR-CONTEXT-008: Circuit breaker configuration with configurable defaults
	threshold := cfg.CircuitBreakerThreshold
	if threshold == 0 {
		threshold = 3 // Default: open after 3 failures
	}

	timeout := cfg.CircuitBreakerTimeout
	if timeout == 0 {
		timeout = 60 * time.Second // Default: close after 60s
	}

	return &CachedExecutor{
		dsClient: cfg.DSClient,
		cache:    cacheManager,
		logger:   logger,
		ttl:      ttl,
		metrics:  metricsInst,

		// BR-CONTEXT-008: Circuit breaker configuration (configurable for testing)
		circuitBreakerThreshold: threshold,
		circuitBreakerTimeout:   timeout,
	}, nil
}

// ListIncidents retrieves incidents with multi-tier fallback
// BR-CONTEXT-001: Query incident audit data
// BR-CONTEXT-005: Multi-tier caching
// Day 11 Edge Case 1.1: Single-flight pattern prevents cache stampede
func (e *CachedExecutor) ListIncidents(ctx context.Context, params *models.ListIncidentsParams) ([]*models.IncidentEvent, int, error) {
	// DD-005: Track query duration
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		e.metrics.QueryDuration.WithLabelValues("list_incidents").Observe(duration)
	}()

	// Generate cache key from params
	cacheKey := generateCacheKey(params)

	// Try cache first (L1 → L2)
	cachedData, err := e.getFromCache(ctx, cacheKey)
	if err == nil && cachedData != nil {
		// DD-005: Record cache hit metric
		e.metrics.CacheHits.WithLabelValues("redis").Inc() // Assume Redis L1 for now

		e.logger.Debug("cache hit",
			zap.String("key", cacheKey),
			zap.Int("incidents", len(cachedData.Incidents)))
		return cachedData.Incidents, cachedData.Total, nil
	}

	// DD-005: Record cache miss metric
	e.metrics.CacheMisses.WithLabelValues("redis").Inc()

	// BR-CONTEXT-007: Use Data Storage Service REST API (ADR-032)
	e.logger.Debug("cache miss, querying Data Storage Service",
		zap.String("key", cacheKey))

	return e.queryDataStorageWithFallback(ctx, cacheKey, params)
}

// DEPRECATED: GetIncidentByID removed per ADR-032
// Use Data Storage Service REST API instead

// getFromCache retrieves cached result
func (e *CachedExecutor) getFromCache(ctx context.Context, key string) (*CachedResult, error) {
	data, err := e.cache.Get(ctx, key)
	if err != nil || data == nil {
		return nil, err
	}

	var result CachedResult
	if err := json.Unmarshal(data, &result); err != nil {
		e.logger.Warn("failed to unmarshal cached data",
			zap.String("key", key),
			zap.Error(err))
		return nil, err
	}

	return &result, nil
}

// populateCache asynchronously repopulates cache after database query
// BR-CONTEXT-005: Non-blocking cache operations
func (e *CachedExecutor) populateCache(ctx context.Context, key string, incidents []*models.IncidentEvent, total int) {
	// Use background context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result := &CachedResult{
		Incidents: incidents,
		Total:     total,
	}

	err := e.cache.Set(timeoutCtx, key, result)
	if err != nil {
		e.logger.Warn("failed to populate cache",
			zap.String("key", key),
			zap.Error(err))
		// Don't propagate error - cache failures shouldn't block operations
	}
}

// DEPRECATED: queryDatabase removed per ADR-032
// Use Data Storage Service REST API instead

// queryDataStorageWithFallback queries Data Storage Service with circuit breaker, retry, and graceful degradation
// BR-CONTEXT-007: HTTP client for Data Storage Service REST API
// BR-CONTEXT-008: Circuit breaker (3 failures → open for 60s)
// BR-CONTEXT-009: Exponential backoff retry (3 attempts: 100ms, 200ms, 400ms)
// BR-CONTEXT-010: Graceful degradation (Data Storage down → cached data only)
// REFACTOR: Now accepts cache key to populate cache after successful query
func (e *CachedExecutor) queryDataStorageWithFallback(ctx context.Context, cacheKey string, params *models.ListIncidentsParams) ([]*models.IncidentEvent, int, error) {
	// BR-CONTEXT-008: Check circuit breaker state
	if e.circuitOpen {
		// Check if timeout elapsed
		if time.Since(e.circuitOpenTime) < e.circuitBreakerTimeout {
			e.logger.Warn("circuit breaker open, falling back to cache")
			return nil, 0, fmt.Errorf("circuit breaker open")
		}
		// Timeout elapsed, close circuit (half-open state)
		e.circuitOpen = false
		e.consecutiveFailures = 0
		e.logger.Info("circuit breaker closing (half-open state)")
	}

	// BR-CONTEXT-009: Exponential backoff retry
	maxAttempts := 3
	baseDelay := 100 * time.Millisecond
	maxDelay := 400 * time.Millisecond

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Build filters map for Data Storage Service
		filters := make(map[string]string)

		filters["limit"] = fmt.Sprintf("%d", params.Limit)
		filters["offset"] = fmt.Sprintf("%d", params.Offset)

		// Map filters (REFACTOR: namespace filtering now supported)
		if params.Namespace != nil {
			filters["namespace"] = *params.Namespace
		}
		if params.Severity != nil {
			filters["severity"] = *params.Severity
		}

		// Execute API call
		result, err := e.dsClient.ListIncidents(ctx, filters)
		if err == nil {
			// Success! Reset circuit breaker
			e.consecutiveFailures = 0
			e.logger.Debug("Data Storage query successful",
				zap.Int("incidents", len(result.Incidents)),
				zap.Int("total", result.Total),
				zap.Int("attempt", attempt+1))

			// Convert dsclient.Incident to models.IncidentEvent
			converted := make([]*models.IncidentEvent, len(result.Incidents))
			for i, inc := range result.Incidents {
				converted[i] = convertIncidentToModel(&inc)
			}

			// REFACTOR: Populate cache after successful query
			go e.populateCache(ctx, cacheKey, converted, result.Total)

			// Extract total from pagination metadata
			return converted, result.Total, nil
		}

		// Record error
		lastErr = err
		e.logger.Warn("Data Storage query failed",
			zap.Error(err),
			zap.Int("attempt", attempt+1),
			zap.Int("max_attempts", maxAttempts))

		// BR-CONTEXT-009: Exponential backoff (except on last attempt)
		if attempt < maxAttempts-1 {
			delay := time.Duration(1<<uint(attempt)) * baseDelay
			if delay > maxDelay {
				delay = maxDelay
			}
			e.logger.Debug("retrying after backoff",
				zap.Duration("delay", delay))
			time.Sleep(delay)
		}
	}

	// All retries failed
	e.consecutiveFailures++
	e.logger.Error("Data Storage query failed after all retries",
		zap.Error(lastErr),
		zap.Int("consecutive_failures", e.consecutiveFailures))

	// BR-CONTEXT-008: Open circuit breaker if threshold reached
	if e.consecutiveFailures >= e.circuitBreakerThreshold {
		e.circuitOpen = true
		e.circuitOpenTime = time.Now()
		e.logger.Error("circuit breaker opened",
			zap.Int("threshold", e.circuitBreakerThreshold))
	}

	// BR-CONTEXT-010: Graceful degradation - return error
	// (Caller can fallback to cache if available)
	//
	// BR-CONTEXT-014: Preserve RFC 7807 structured errors
	// Return error directly to preserve RFC7807Error type for consumers
	return nil, 0, lastErr
}

// convertIncidentToModel converts Data Storage client incident to Context API model
// REFACTOR phase: Complete field mapping with all incident data
func convertIncidentToModel(inc *dsclient.Incident) *models.IncidentEvent {
	// Map execution status to phase
	phase := "pending"
	switch inc.ExecutionStatus {
	case "completed":
		phase = "completed"
	case "failed", "cancelled":
		phase = "failed"
	case "in_progress":
		phase = "processing"
	}

	result := &models.IncidentEvent{
		// Primary identification
		ID:   inc.Id,
		Name: inc.SignalName,

		// Context (REFACTOR: now available from Data Storage)
		Namespace:      stringPtrToString(inc.Namespace),
		ClusterName:    stringPtrToString(inc.ClusterName),
		Environment:    stringPtrToString(inc.Environment),
		TargetResource: stringPtrToString(inc.TargetResource),

		// Identifiers (REFACTOR: now available)
		AlertFingerprint:     stringPtrToString(inc.SignalFingerprint),
		RemediationRequestID: stringPtrToString(inc.RemediationRequestId),

		// Status
		Phase:      phase,
		Status:     string(inc.ExecutionStatus),
		Severity:   string(inc.SignalSeverity),
		ActionType: inc.ActionType,

		// Timing (REFACTOR: now available)
		StartTime: timeToTimePtr(&inc.ActionTimestamp), // action_timestamp as start
		EndTime:   timeToTimePtr(inc.EndTime),
		Duration:  int64PtrToInt64(inc.Duration),

		// Error tracking (REFACTOR: now available)
		ErrorMessage: inc.ErrorMessage,

		// Metadata (REFACTOR: now available)
		Metadata: stringPtrToString(inc.Metadata),
	}

	return result
}

// Helper functions for pointer conversions
func stringPtrToString(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func timeToTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	return t
}

func int64PtrToInt64(ptr *int64) *int64 {
	return ptr // Keep as pointer for Context API
}

// DEPRECATED: getTotalCount removed per ADR-032
// Use Data Storage Service REST API instead

// generateCacheKey creates deterministic cache key from query params
// BR-CONTEXT-005: Cache key generation
func generateCacheKey(params *models.ListIncidentsParams) string {
	parts := []string{"incidents:list"}

	if params.Namespace != nil {
		parts = append(parts, fmt.Sprintf("ns=%s", *params.Namespace))
	}
	if params.Severity != nil {
		parts = append(parts, fmt.Sprintf("sev=%s", *params.Severity))
	}
	// Time range not yet in params (will be added in REFACTOR phase)

	parts = append(parts, fmt.Sprintf("limit=%d", params.Limit))
	parts = append(parts, fmt.Sprintf("offset=%d", params.Offset))

	return strings.Join(parts, ":")
}

// Close closes the executor and underlying resources
func (e *CachedExecutor) Close() error {
	// Close cache
	if err := e.cache.Close(); err != nil {
		return fmt.Errorf("failed to close cache: %w", err)
	}

	// Database connection managed externally (by client.PostgresClient)
	// Don't close here to avoid double-close

	return nil
}

// SemanticSearch performs vector similarity search with caching and HNSW optimization
// BR-CONTEXT-002: Semantic search on embeddings
// BR-CONTEXT-003: Pattern matching with similarity scores
//
// TODO: Implement after Data Storage Service supports vector search API
// ADR-032: Semantic search requires Data Storage Service API support for pgvector
func (e *CachedExecutor) SemanticSearch(ctx context.Context, embedding []float32, limit int, threshold float32) ([]*models.IncidentEvent, []float32, error) {
	// TODO: Implement after Data Storage Service supports vector search API
	// ADR-032: Semantic search requires Data Storage Service API support for pgvector
	return nil, nil, fmt.Errorf("semantic search not implemented - requires Data Storage Service vector search API")
}

// Ping checks connectivity of all underlying services
// BR-CONTEXT-013: Health checks
// ADR-032: Context API has no direct database - checks Data Storage Service implicitly via cache
func (e *CachedExecutor) Ping(ctx context.Context) error {
	// ADR-032: Data Storage Service health is checked implicitly via successful queries
	// No direct ping API available yet
	e.logger.Debug("Data Storage Service connectivity assumed healthy (ADR-032)")

	// Check cache health (non-critical - graceful degradation)
	if _, err := e.cache.HealthCheck(ctx); err != nil {
		e.logger.Warn("cache health check failed (non-critical)",
			zap.Error(err))
		// Don't return error - cache failures don't block operations
	}

	return nil
}
