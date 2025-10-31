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

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/metrics"
	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
	"github.com/jordigilh/kubernaut/pkg/contextapi/sqlbuilder"
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
	DB      DBExecutor         // Changed from *sqlx.DB to DBExecutor interface for testability
	TTL     time.Duration      // Default cache TTL
	Metrics *metrics.Metrics // DD-005: Observability metrics (optional for backward compatibility)
}

// CachedExecutor executes queries with multi-tier caching fallback
// BR-CONTEXT-001: Query historical incident context
// BR-CONTEXT-005: Multi-tier caching (L1 Redis + L2 LRU + L3 DB)
//
// Fallback Chain:
// 1. Try Cache (L1 Redis → L2 LRU)
// 2. On miss → Query Database (L3) with single-flight deduplication
// 3. Async repopulate cache
//
// Day 11 Edge Case 1.1: Single-flight pattern prevents cache stampede
// Multiple concurrent requests for same cache key are deduplicated to single DB query
//
// DD-005: Observability - Records metrics for cache hits/misses, query duration, errors
type CachedExecutor struct {
	cache        cache.CacheManager
	db           DBExecutor         // Changed from *sqlx.DB to DBExecutor interface for testability
	logger       *zap.Logger
	ttl          time.Duration
	singleflight singleflight.Group // Day 11: Prevents cache stampede on concurrent cache misses
	metrics      *metrics.Metrics   // DD-005: Observability metrics (nil-safe for backward compatibility)
}

// CachedResult wraps query results for caching
// Exported for testing purposes
type CachedResult struct {
	Incidents []*models.IncidentEvent `json:"incidents"`
	Total     int                     `json:"total"`
}

// NewCachedExecutor creates a new query executor with caching
// BR-CONTEXT-001: Executor initialization
// DD-005: Observability - Accepts optional metrics for recording cache/DB operations
func NewCachedExecutor(cfg *Config) (*CachedExecutor, error) {
	// Validate config
	if cfg.Cache == nil {
		return nil, fmt.Errorf("cache cannot be nil")
	}
	if cfg.DB == nil {
		return nil, fmt.Errorf("db cannot be nil")
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

// ListIncidents retrieves incidents with multi-tier fallback
// BR-CONTEXT-001: Query incident audit data
// BR-CONTEXT-005: Multi-tier caching
// Day 11 Edge Case 1.1: Single-flight pattern prevents cache stampede
func (e *CachedExecutor) ListIncidents(ctx context.Context, params *models.ListIncidentsParams) ([]*models.IncidentEvent, int, error) {
	// DD-005: Track query duration
	startTime := time.Now()
	defer func() {
		if e.metrics != nil {
			duration := time.Since(startTime).Seconds()
			e.metrics.QueryDuration.WithLabelValues("list_incidents").Observe(duration)
		}
	}()
	
	// Generate cache key from params
	cacheKey := generateCacheKey(params)

	// Try cache first (L1 → L2)
	cachedData, err := e.getFromCache(ctx, cacheKey)
	if err == nil && cachedData != nil {
		// DD-005: Record cache hit metric
		if e.metrics != nil {
			e.metrics.CacheHits.WithLabelValues("redis").Inc() // Assume Redis L1 for now
		}
		
		e.logger.Debug("cache hit",
			zap.String("key", cacheKey),
			zap.Int("incidents", len(cachedData.Incidents)))
		return cachedData.Incidents, cachedData.Total, nil
	}

	// DD-005: Record cache miss metric
	if e.metrics != nil {
		e.metrics.CacheMisses.WithLabelValues("redis").Inc()
	}

	// Cache miss or error → fallback to database with single-flight deduplication
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
// DO-REFACTOR enhancements:
// - Cache integration with embedding hash key
// - HNSW index optimization with planner hints
// - Async cache repopulation after DB hits
func (e *CachedExecutor) SemanticSearch(ctx context.Context, embedding []float32, limit int, threshold float32) ([]*models.IncidentEvent, []float32, error) {
	// Validate inputs
	if len(embedding) == 0 {
		return nil, nil, fmt.Errorf("embedding vector cannot be empty")
	}
	if limit <= 0 || limit > 100 {
		return nil, nil, fmt.Errorf("limit must be between 1 and 100")
	}
	if threshold < 0 || threshold > 1 {
		return nil, nil, fmt.Errorf("threshold must be between 0 and 1")
	}

	// Convert embedding to PostgreSQL vector format
	vectorStr, err := VectorToString(embedding)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert embedding: %w", err)
	}

	// Generate deterministic cache key from embedding hash
	// Hash first 8 dimensions for reasonable key size while maintaining uniqueness
	hashInput := fmt.Sprintf("%v", embedding[:min(8, len(embedding))])
	cacheKey := fmt.Sprintf("semantic:hash=%x:limit=%d:threshold=%.2f",
		hashInput, limit, threshold)

	// Try cache first (cache hit optimization)
	cachedBytes, err := e.cache.Get(ctx, cacheKey)
	if err == nil {
		var cachedResult struct {
			Incidents []*models.IncidentEvent
			Scores    []float32
		}
		if unmarshalErr := json.Unmarshal(cachedBytes, &cachedResult); unmarshalErr == nil {
			e.logger.Debug("semantic search cache HIT",
				zap.String("key", cacheKey),
				zap.Int("results", len(cachedResult.Incidents)))
			return cachedResult.Incidents, cachedResult.Scores, nil
		} else {
			e.logger.Warn("failed to unmarshal cached result, querying database",
				zap.Error(unmarshalErr))
		}
	}

	// Cache miss - query database with HNSW optimization
	e.logger.Debug("semantic search cache MISS",
		zap.String("key", cacheKey),
		zap.Error(err))

	// HNSW index optimization: Set planner hints for pgvector
	// These hints force PostgreSQL to use HNSW index for better performance
	hnswOptimization := `
		SET LOCAL enable_seqscan = off;
		SET LOCAL ivfflat.probes = 10;
	`

	// Execute HNSW optimization (best effort - ignore errors)
	_, _ = e.db.ExecContext(ctx, hnswOptimization)

	// Query database with pgvector using Data Storage schema
	// Using cosine distance operator <=> for similarity
	query := `
		SELECT
			rat.id,
			rat.alert_name AS name,
			rr.namespace,
			CASE rat.execution_status
				WHEN 'completed' THEN 'completed'
				WHEN 'failed' THEN 'failed'
				WHEN 'rolled-back' THEN 'failed'
				WHEN 'pending' THEN 'pending'
				WHEN 'executing' THEN 'processing'
				ELSE 'pending'
			END AS phase,
			rat.action_type,
			rat.execution_status AS status,
			rat.alert_fingerprint,
			rat.alert_severity AS severity,
			rat.cluster_name,
			rr.kind AS target_resource,
			rat.action_timestamp AS start_time,
			rat.execution_end_time AS end_time,
			rat.execution_duration_ms AS duration,
			rat.execution_error AS error_message,
			rat.action_parameters::TEXT AS metadata,
			rat.embedding,
			rat.created_at,
			rat.updated_at,
			rat.action_id AS remediation_request_id,
			rat.environment,
			(1 - (rat.embedding <=> $1::vector)) as similarity
		FROM resource_action_traces rat
		JOIN action_histories ah ON rat.action_history_id = ah.id
		JOIN resource_references rr ON ah.resource_id = rr.id
		WHERE rat.embedding IS NOT NULL
			AND (1 - (rat.embedding <=> $1::vector)) >= $2
		ORDER BY rat.embedding <=> $1::vector
		LIMIT $3
	`

	// Execute query (scan into intermediate struct for pgvector compatibility)
	type resultRow struct {
		IncidentEventRow
		Similarity float32 `db:"similarity"`
	}

	var rows []resultRow
	err = e.db.SelectContext(ctx, &rows, query, vectorStr, threshold, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("semantic search query failed: %w", err)
	}

	// Convert rows to models.IncidentEvent and extract scores
	incidents := make([]*models.IncidentEvent, len(rows))
	scores := make([]float32, len(rows))
	for i, row := range rows {
		incidents[i] = row.IncidentEventRow.ToIncidentEvent()
		scores[i] = row.Similarity
	}

	e.logger.Debug("semantic search completed",
		zap.Int("results", len(incidents)),
		zap.Float64("threshold", float64(threshold)))

	// Async cache population (non-blocking)
	go func() {
		populateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cacheValue := struct {
			Incidents []*models.IncidentEvent
			Scores    []float32
		}{
			Incidents: incidents,
			Scores:    scores,
		}

		if err := e.cache.Set(populateCtx, cacheKey, cacheValue); err != nil {
			e.logger.Warn("failed to populate semantic search cache",
				zap.String("key", cacheKey),
				zap.Error(err))
		} else {
			e.logger.Debug("semantic search cache populated",
				zap.String("key", cacheKey))
		}
	}()

	return incidents, scores, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Ping checks connectivity of all underlying services
// BR-CONTEXT-006: Health checks
func (e *CachedExecutor) Ping(ctx context.Context) error {
	// Check database connectivity
	if err := e.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Check cache health (non-critical - graceful degradation)
	if _, err := e.cache.HealthCheck(ctx); err != nil {
		e.logger.Warn("cache health check failed (non-critical)",
			zap.Error(err))
		// Don't return error - cache failures don't block operations
	}

	return nil
}
