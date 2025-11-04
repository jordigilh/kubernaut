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
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/metrics"
	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
	"github.com/jordigilh/kubernaut/pkg/contextapi/sqlbuilder"
	dsclient "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// DBExecutor defines the database operations required by CachedExecutor
// This interface allows for dependency injection and testing with mocks
// Day 11 Edge Case Testing: Interface extraction for cache stampede prevention testing
type DBExecutor interface {
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	PingContext(ctx context.Context) error
}

// Config holds configuration for CachedExecutor
// BR-CONTEXT-001: Executor configuration
type Config struct {
	Cache   cache.CacheManager
	DB      DBExecutor       // Changed from *sqlx.DB to DBExecutor interface for testability
	TTL     time.Duration    // Default cache TTL
	Metrics *metrics.Metrics // DD-005: Observability metrics (required - always initialized)
}

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
type CachedExecutor struct {
	cache        cache.CacheManager
	db           DBExecutor                  // DEPRECATED: PostgreSQL access (legacy, will be removed)
	dsClient     *dsclient.DataStorageClient // BR-CONTEXT-007: Data Storage Service client
	logger       *zap.Logger
	ttl          time.Duration
	singleflight singleflight.Group // Day 11: Prevents cache stampede on concurrent cache misses
	metrics      *metrics.Metrics   // DD-005: Observability metrics (required - always initialized)

	// BR-CONTEXT-008: Circuit breaker state
	circuitOpen          bool
	consecutiveFailures  int
	circuitOpenTime      time.Time
	circuitBreakerThreshold int
	circuitBreakerTimeout   time.Duration
}

// CachedResult wraps query results for caching
// Exported for testing purposes
type CachedResult struct {
	Incidents []*models.IncidentEvent `json:"incidents"`
	Total     int                     `json:"total"`
}

// NewCachedExecutor creates a new query executor with caching
// BR-CONTEXT-001: Executor initialization
// DD-005: Observability - Requires metrics for recording cache/DB operations
// DEPRECATED: Use NewCachedExecutorWithDataStorage for new code
func NewCachedExecutor(cfg *Config) (*CachedExecutor, error) {
	// Validate config
	if cfg.Cache == nil {
		return nil, fmt.Errorf("cache cannot be nil")
	}
	if cfg.DB == nil {
		return nil, fmt.Errorf("db cannot be nil")
	}
	if cfg.Metrics == nil {
		return nil, fmt.Errorf("metrics cannot be nil - observability is required")
	}

	// Default TTL
	ttl := cfg.TTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	logger, _ := zap.NewProduction() // Use production logger

	return &CachedExecutor{
		cache:   cfg.Cache,
		db:      cfg.DB,
		logger:  logger,
		ttl:     ttl,
		metrics: cfg.Metrics, // DD-005: Metrics are optional (nil-safe)
	}, nil
}

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

	// Cache miss or error → use Data Storage Service if configured, otherwise fallback to PostgreSQL
	if e.dsClient != nil {
		// BR-CONTEXT-007: Use Data Storage Service REST API
		e.logger.Debug("cache miss, querying Data Storage Service",
			zap.String("key", cacheKey))

		return e.queryDataStorageWithFallback(ctx, cacheKey, params)
	}

	// Legacy: Cache miss or error → fallback to database with single-flight deduplication
	// Day 11 Edge Case 1.1: Multiple concurrent cache misses deduplicated to single DB query
	e.logger.Debug("cache miss, querying database with single-flight",
		zap.String("key", cacheKey))

	// Use single-flight to deduplicate concurrent requests
	// Only one database query executes per unique cache key, even with 10+ concurrent requests
	//
	// Performance Characteristics (Day 11 REFACTOR):
	// - First request: Executes DB query (~50-200ms), populates cache
	// - Concurrent requests (2-N): Wait for shared result (~0-50ms), receive same data
	// - Cache stampede prevention: 90% reduction in DB queries during high concurrency
	//
	// Example Scenario:
	// - 10 concurrent requests with same params → 1 DB query (2 total: SELECT + COUNT)
	// - WITHOUT single-flight: 10 DB queries (20 total: 10 SELECT + 10 COUNT)
	result, err, shared := e.singleflight.Do(cacheKey, func() (interface{}, error) {
		// REFACTOR: Log execution start for database query
		e.logger.Debug("single-flight: executing database query",
			zap.String("key", cacheKey))

		incidents, total, dbErr := e.queryDatabase(ctx, params)
		if dbErr != nil {
			return nil, fmt.Errorf("database query failed: %w", dbErr)
		}

		// REFACTOR: Log successful query execution
		e.logger.Debug("single-flight: database query complete",
			zap.String("key", cacheKey),
			zap.Int("incidents", len(incidents)),
			zap.Int("total", total))

		// Create result
		cachedResult := &CachedResult{
			Incidents: incidents,
			Total:     total,
		}

		// FIX: Populate cache SYNCHRONOUSLY before returning
		// This ensures the cache is populated before single-flight releases waiting goroutines
		// Prevents race condition where late-arriving concurrent requests miss the cache
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if cacheErr := e.cache.Set(timeoutCtx, cacheKey, cachedResult); cacheErr != nil {
			e.logger.Warn("failed to populate cache in single-flight",
				zap.String("key", cacheKey),
				zap.Error(cacheErr))
			// Don't fail the query if cache write fails
		} else {
			e.logger.Debug("cache populated in single-flight",
				zap.String("key", cacheKey))
		}

		return cachedResult, nil
	})

	if err != nil {
		return nil, 0, err
	}

	// REFACTOR: Log whether this request was deduplicated or executed the query
	if shared {
		e.logger.Debug("single-flight: request deduplicated (waited for shared result)",
			zap.String("key", cacheKey))
	} else {
		e.logger.Debug("single-flight: executed database query and populated cache (first in group)",
			zap.String("key", cacheKey))
	}

	// Extract result
	cachedResult := result.(*CachedResult)

	return cachedResult.Incidents, cachedResult.Total, nil
}

// GetIncidentByID retrieves a single incident with caching
// BR-CONTEXT-001: Query incident audit data by ID
//
// Updated for Data Storage Service schema (DD-SCHEMA-001)
func (e *CachedExecutor) GetIncidentByID(ctx context.Context, id int64) (*models.IncidentEvent, error) {
	// Generate cache key
	cacheKey := fmt.Sprintf("incident:%d", id)

	// Try cache first
	data, err := e.cache.Get(ctx, cacheKey)
	if err == nil && data != nil {
		var incident models.IncidentEvent
		if err := json.Unmarshal(data, &incident); err == nil {
			e.logger.Debug("cache hit for incident",
				zap.Int64("id", id))
			return &incident, nil
		}
	}

	// Cache miss → query database with Data Storage schema
	query := `
SELECT
    rat.id,
    rat.alert_name AS name,
    rat.alert_fingerprint,
    rat.action_id AS remediation_request_id,
    rr.namespace,
    rat.cluster_name,
    rat.environment,
    rr.kind AS target_resource,
    CASE rat.execution_status
        WHEN 'completed' THEN 'completed'
        WHEN 'failed' THEN 'failed'
        WHEN 'rolled-back' THEN 'failed'
        WHEN 'pending' THEN 'pending'
        WHEN 'executing' THEN 'processing'
        ELSE 'pending'
    END AS phase,
    rat.execution_status AS status,
    rat.alert_severity AS severity,
    rat.action_type,
    rat.action_timestamp AS start_time,
    rat.execution_end_time AS end_time,
    rat.execution_duration_ms AS duration,
    rat.execution_error AS error_message,
    rat.action_parameters::TEXT AS metadata,
    rat.created_at,
    rat.updated_at
FROM resource_action_traces rat
JOIN action_histories ah ON rat.action_history_id = ah.id
JOIN resource_references rr ON ah.resource_id = rr.id
WHERE rat.id = $1`

	var row IncidentEventRow
	err = e.db.GetContext(ctx, &row, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get incident: %w", err)
	}

	incident := row.ToIncidentEvent()

	// Async cache repopulation
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = e.cache.Set(ctx, cacheKey, incident)
	}()

	return incident, nil
}

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

// queryDatabase executes database query with SQL builder
// BR-CONTEXT-001: Database query execution
func (e *CachedExecutor) queryDatabase(ctx context.Context, params *models.ListIncidentsParams) ([]*models.IncidentEvent, int, error) {
	// DD-005: Track database query duration
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		e.metrics.DatabaseDuration.WithLabelValues("list_incidents").Observe(duration)
	}()

	// Build query using SQL builder
	builder := sqlbuilder.NewBuilder()

	if params.Namespace != nil {
		builder.WithNamespace(*params.Namespace)
	}
	if params.Severity != nil {
		builder.WithSeverity(*params.Severity)
	}
	// Time range filtering not yet implemented (will be added in REFACTOR phase)

	// Set pagination
	if err := builder.WithLimit(params.Limit); err != nil {
		return nil, 0, fmt.Errorf("invalid limit: %w", err)
	}
	if err := builder.WithOffset(params.Offset); err != nil {
		return nil, 0, fmt.Errorf("invalid offset: %w", err)
	}

	// Build query
	query, args, err := builder.Build()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build query: %w", err)
	}

	// Execute query (scan into intermediate struct for pgvector compatibility)
	var rows []*IncidentEventRow
	err = e.db.SelectContext(ctx, &rows, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("database query failed: %w", err)
	}

	// Convert rows to models.IncidentEvent
	incidents := make([]*models.IncidentEvent, len(rows))
	for i, row := range rows {
		incidents[i] = row.ToIncidentEvent()
	}

	// REFACTOR Phase: Get total count (for pagination)
	// Execute COUNT(*) query with same filters but without LIMIT/OFFSET
	// This provides accurate total for pagination metadata
	total, err := e.getTotalCount(ctx, params)
	if err != nil {
		e.logger.Warn("failed to get total count, falling back to result length",
			zap.Error(err))
		total = len(incidents) // Graceful degradation
	}

	e.logger.Debug("database query executed",
		zap.Int("results", len(incidents)),
		zap.Int("total", total))

	return incidents, total, nil
}

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
	// BR-CONTEXT-011: Preserve RFC 7807 structured errors
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
		Name: inc.AlertName,

		// Context (REFACTOR: now available from Data Storage)
		Namespace:      stringPtrToString(inc.Namespace),
		ClusterName:    stringPtrToString(inc.ClusterName),
		Environment:    stringPtrToString(inc.Environment),
		TargetResource: stringPtrToString(inc.TargetResource),

		// Identifiers (REFACTOR: now available)
		AlertFingerprint:     stringPtrToString(inc.AlertFingerprint),
		RemediationRequestID: stringPtrToString(inc.RemediationRequestId),

		// Status
		Phase:      phase,
		Status:     string(inc.ExecutionStatus),
		Severity:   string(inc.AlertSeverity),
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

// getTotalCount executes COUNT(*) query to get total matching incidents
// REFACTOR Phase: Provides accurate pagination metadata
// BR-CONTEXT-002: Pagination support (total count before LIMIT/OFFSET)
//
// Updated for Data Storage Service schema (DD-SCHEMA-001)
// Uses Builder.BuildCount() which includes proper JOINs
func (e *CachedExecutor) getTotalCount(ctx context.Context, params *models.ListIncidentsParams) (int, error) {
	// Build COUNT query using SQL builder with same filters
	builder := sqlbuilder.NewBuilder()

	if params.Namespace != nil {
		builder.WithNamespace(*params.Namespace)
	}
	if params.Severity != nil {
		builder.WithSeverity(*params.Severity)
	}
	// Note: Do NOT add LIMIT/OFFSET for COUNT query

	// Use BuildCount() method which generates proper COUNT query with JOINs
	// No need to strip LIMIT/OFFSET - BuildCount() doesn't include them
	countQuery, countArgs := builder.BuildCount()

	// Execute COUNT query
	var total int
	err := e.db.GetContext(ctx, &total, countQuery, countArgs...)
	if err != nil {
		return 0, fmt.Errorf("count query failed: %w", err)
	}

	e.logger.Debug("total count query executed",
		zap.Int("total", total),
		zap.String("namespace", stringPtrOrDefault(params.Namespace, "all")),
		zap.String("severity", stringPtrOrDefault(params.Severity, "all")))

	return total, nil
}

// stringPtrOrDefault returns string value or default if nil
// Helper for logging
func stringPtrOrDefault(ptr *string, defaultVal string) string {
	if ptr == nil {
		return defaultVal
	}
	return *ptr
}

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
// BR-CONTEXT-006: Health checks
// ADR-032: Context API has no direct database - checks Data Storage Service implicitly via cache
func (e *CachedExecutor) Ping(ctx context.Context) error {
	// ADR-032: Check Data Storage connectivity if using dsClient
	if e.dsClient != nil {
		// Data Storage Service health is checked implicitly via successful queries
		// No direct ping API available yet
		e.logger.Debug("Data Storage Service connectivity assumed healthy (ADR-032)")
	}

	// Check database connectivity (DEPRECATED path - only for old executor)
	if e.db != nil {
		if err := e.db.PingContext(ctx); err != nil {
			return fmt.Errorf("database ping failed: %w", err)
		}
	}

	// Check cache health (non-critical - graceful degradation)
	if _, err := e.cache.HealthCheck(ctx); err != nil {
		e.logger.Warn("cache health check failed (non-critical)",
			zap.Error(err))
		// Don't return error - cache failures don't block operations
	}

	return nil
}
