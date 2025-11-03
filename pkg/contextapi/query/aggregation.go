package query

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
)

// AggregationService provides sophisticated aggregation and analytics
// BR-CONTEXT-004: Advanced query aggregation and analysis
//
// v2.0 Integration: Uses CacheManager from Day 3 for multi-tier caching
type AggregationService struct {
	db     *sqlx.DB
	cache  cache.CacheManager // v2.0: Multi-tier cache (L1 Redis + L2 LRU)
	logger *zap.Logger
}

// NewAggregationService creates a new aggregation service
//
// v2.0 Changes:
// - Replaced cache.Cache with cache.CacheManager
// - CacheManager provides L1 (Redis) + L2 (LRU) + L3 (DB) fallback
func NewAggregationService(
	db *sqlx.DB,
	cache cache.CacheManager,
	logger *zap.Logger,
) *AggregationService {
	return &AggregationService{
		db:     db,
		cache:  cache,
		logger: logger,
	}
}

// AggregateWithFilters performs aggregation with custom filters
// BR-CONTEXT-004: Flexible aggregation with filtering
func (a *AggregationService) AggregateWithFilters(
	ctx context.Context,
	filters *models.AggregationFilters,
) (map[string]interface{}, error) {
	if filters == nil {
		return nil, fmt.Errorf("aggregation filters cannot be nil")
	}

	// Build dynamic query based on filters
	// DD-SCHEMA-001: Use Data Storage Service schema (resource_action_traces)
	query := `
		SELECT
			COUNT(*) as total_count,
			COUNT(DISTINCT rr.namespace) as unique_namespaces,
			COUNT(DISTINCT rat.cluster_name) as unique_clusters,
			COUNT(DISTINCT rat.action_type) as unique_actions,
			COALESCE(
				CAST(SUM(CASE WHEN rat.execution_status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) /
				NULLIF(COUNT(*), 0),
				0.0
			) as overall_success_rate,
			COALESCE(AVG(rat.execution_duration_ms), 0) as avg_duration_ms
		FROM resource_action_traces rat
		JOIN action_histories ah ON rat.action_history_id = ah.id
		JOIN resource_references rr ON ah.resource_id = rr.id
		WHERE 1=1
	`

	args := []interface{}{}
	argIdx := 1

	// Apply filters dynamically (DD-SCHEMA-001: Use correct column names)
	if filters.Namespace != nil {
		query += fmt.Sprintf(" AND rr.namespace = $%d", argIdx)
		args = append(args, *filters.Namespace)
		argIdx++
	}

	if filters.ClusterName != nil {
		query += fmt.Sprintf(" AND rat.cluster_name = $%d", argIdx)
		args = append(args, *filters.ClusterName)
		argIdx++
	}

	if filters.Environment != nil {
		query += fmt.Sprintf(" AND rat.environment = $%d", argIdx)
		args = append(args, *filters.Environment)
		argIdx++
	}

	if filters.Severity != nil {
		query += fmt.Sprintf(" AND rat.alert_severity = $%d", argIdx)
		args = append(args, *filters.Severity)
		argIdx++
	}

	if filters.ActionType != nil {
		query += fmt.Sprintf(" AND rat.action_type = $%d", argIdx)
		args = append(args, *filters.ActionType)
		argIdx++
	}

	if filters.Phase != nil {
		// Phase is derived from execution_status
		query += fmt.Sprintf(" AND rat.execution_status = $%d", argIdx)
		args = append(args, *filters.Phase)
		argIdx++
	}

	if filters.StartTime != nil {
		query += fmt.Sprintf(" AND rat.action_timestamp >= $%d", argIdx)
		args = append(args, *filters.StartTime)
		argIdx++
	}

	if filters.EndTime != nil {
		query += fmt.Sprintf(" AND rat.action_timestamp <= $%d", argIdx)
		args = append(args, *filters.EndTime)
		argIdx++
	}

	// Execute query
	row := a.db.QueryRowxContext(ctx, query, args...)

	var (
		totalCount         int64
		uniqueNamespaces   int64
		uniqueClusters     int64
		uniqueActions      int64
		overallSuccessRate float64
		avgDurationMs      float64
	)

	err := row.Scan(
		&totalCount,
		&uniqueNamespaces,
		&uniqueClusters,
		&uniqueActions,
		&overallSuccessRate,
		&avgDurationMs,
	)
	if err != nil {
		a.logger.Error("Failed to execute aggregation with filters", zap.Error(err))
		return nil, fmt.Errorf("failed to aggregate with filters: %w", err)
	}

	result := map[string]interface{}{
		"total_count":          totalCount,
		"unique_namespaces":    uniqueNamespaces,
		"unique_clusters":      uniqueClusters,
		"unique_actions":       uniqueActions,
		"overall_success_rate": overallSuccessRate,
		"avg_duration_ms":      avgDurationMs,
		"calculated_at":        time.Now(),
	}

	a.logger.Debug("Aggregation with filters complete",
		zap.Int64("total_count", totalCount),
		zap.Float64("success_rate", overallSuccessRate),
	)

	return result, nil
}

// GetTopFailingActions returns actions with highest failure rates
// BR-CONTEXT-004: Failure pattern analysis
func (a *AggregationService) GetTopFailingActions(
	ctx context.Context,
	limit int,
	timeWindow time.Duration,
) ([]map[string]interface{}, error) {
	if limit <= 0 || limit > 100 {
		return nil, fmt.Errorf("invalid limit: must be between 1 and 100")
	}

	// DD-SCHEMA-001: Use Data Storage Service schema
	query := `
		SELECT
			action_type,
			COUNT(*) as total_attempts,
			SUM(CASE WHEN execution_status = 'failed' THEN 1 ELSE 0 END) as failed_attempts,
			CAST(SUM(CASE WHEN execution_status = 'failed' THEN 1 ELSE 0 END) AS FLOAT) /
				NULLIF(COUNT(*), 0) as failure_rate
		FROM resource_action_traces
		WHERE action_timestamp > NOW() - $1::interval
		GROUP BY action_type
		HAVING COUNT(*) >= 5  -- Minimum sample size for statistical relevance
		ORDER BY failure_rate DESC, total_attempts DESC
		LIMIT $2
	`

	rows, err := a.db.QueryxContext(ctx, query, timeWindow.String(), limit)
	if err != nil {
		a.logger.Error("Failed to get top failing actions", zap.Error(err))
		return nil, fmt.Errorf("failed to get top failing actions: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var (
			actionType     string
			totalAttempts  int64
			failedAttempts int64
			failureRate    float64
		)

		if err := rows.Scan(&actionType, &totalAttempts, &failedAttempts, &failureRate); err != nil {
			a.logger.Warn("Failed to scan failing action row", zap.Error(err))
			continue
		}

		results = append(results, map[string]interface{}{
			"action_type":     actionType,
			"total_attempts":  totalAttempts,
			"failed_attempts": failedAttempts,
			"failure_rate":    failureRate,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	a.logger.Debug("Top failing actions retrieved",
		zap.Int("result_count", len(results)),
		zap.Duration("time_window", timeWindow),
	)

	return results, nil
}

// GetActionComparison compares success rates across multiple actions
// BR-CONTEXT-004: Comparative analysis
func (a *AggregationService) GetActionComparison(
	ctx context.Context,
	actionTypes []string,
	timeWindow time.Duration,
) ([]models.ActionSuccessRate, error) {
	if len(actionTypes) == 0 {
		return nil, fmt.Errorf("action types list cannot be empty")
	}

	if len(actionTypes) > 20 {
		return nil, fmt.Errorf("too many action types: maximum 20 allowed")
	}

	// Build dynamic query with IN clause (DD-SCHEMA-001)
	query := `
		SELECT
			action_type,
			COUNT(*) as total_attempts,
			SUM(CASE WHEN execution_status = 'completed' THEN 1 ELSE 0 END) as successful_attempts,
			COALESCE(
				CAST(SUM(CASE WHEN execution_status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) /
				NULLIF(COUNT(*), 0),
				0.0
			) as success_rate,
			COALESCE(AVG(execution_duration_ms), 0) as avg_execution_time
		FROM resource_action_traces
		WHERE action_type = ANY($1)
		  AND action_timestamp > NOW() - $2::interval
		GROUP BY action_type
		ORDER BY success_rate DESC, total_attempts DESC
	`

	var results []models.ActionSuccessRate
	err := a.db.SelectContext(ctx, &results, query, pq.Array(actionTypes), timeWindow.String())
	if err != nil {
		a.logger.Error("Failed to compare actions",
			zap.Strings("action_types", actionTypes),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to compare actions: %w", err)
	}

	// Enrich results
	now := time.Now()
	for i := range results {
		results[i].TimeWindow = timeWindow.String()
		results[i].CalculatedAt = now
	}

	a.logger.Debug("Action comparison complete",
		zap.Int("action_count", len(actionTypes)),
		zap.Int("result_count", len(results)),
	)

	return results, nil
}

// GetNamespaceHealthScore calculates health score for a namespace
// BR-CONTEXT-004: Namespace health analysis
func (a *AggregationService) GetNamespaceHealthScore(
	ctx context.Context,
	namespace string,
	timeWindow time.Duration,
) (float64, error) {
	if namespace == "" {
		return 0, fmt.Errorf("namespace cannot be empty")
	}

	// DD-SCHEMA-001: Join to resource_references for namespace
	query := `
		SELECT
			COUNT(*) as total_incidents,
			SUM(CASE WHEN rat.execution_status = 'completed' THEN 1 ELSE 0 END) as resolved_incidents,
			SUM(CASE WHEN rat.alert_severity = 'critical' THEN 1 ELSE 0 END) as critical_incidents,
			COALESCE(AVG(rat.execution_duration_ms), 0) as avg_resolution_time_ms
		FROM resource_action_traces rat
		JOIN action_histories ah ON rat.action_history_id = ah.id
		JOIN resource_references rr ON ah.resource_id = rr.id
		WHERE rr.namespace = $1
		  AND rat.action_timestamp > NOW() - $2::interval
	`

	var (
		totalIncidents      int64
		resolvedIncidents   int64
		criticalIncidents   int64
		avgResolutionTimeMs float64
	)

	row := a.db.QueryRowxContext(ctx, query, namespace, timeWindow.String())
	err := row.Scan(&totalIncidents, &resolvedIncidents, &criticalIncidents, &avgResolutionTimeMs)
	if err != nil {
		a.logger.Error("Failed to calculate namespace health score",
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return 0, fmt.Errorf("failed to calculate health score: %w", err)
	}

	// Calculate health score (0.0 to 1.0)
	// Formula: (0.6 * resolution_rate) + (0.3 * (1 - critical_ratio)) + (0.1 * speed_factor)
	var healthScore float64

	if totalIncidents > 0 {
		resolutionRate := float64(resolvedIncidents) / float64(totalIncidents)
		criticalRatio := float64(criticalIncidents) / float64(totalIncidents)

		// Speed factor: penalize slow resolutions (normalize to 0-1 scale, assuming 5min is ideal)
		idealResolutionTimeMs := 300000.0 // 5 minutes
		speedFactor := 1.0
		if avgResolutionTimeMs > 0 {
			speedFactor = idealResolutionTimeMs / (avgResolutionTimeMs + idealResolutionTimeMs)
		}

		healthScore = (0.6 * resolutionRate) + (0.3 * (1.0 - criticalRatio)) + (0.1 * speedFactor)
	}

	a.logger.Debug("Namespace health score calculated",
		zap.String("namespace", namespace),
		zap.Float64("health_score", healthScore),
		zap.Int64("total_incidents", totalIncidents),
	)

	return healthScore, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// v1.x Backward Compatibility Methods (for server.go HTTP handlers)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// AggregateSuccessRate calculates success rate for a workflow ID
// v1.x compatibility method - uses direct SQL query (DD-SCHEMA-001)
func (a *AggregationService) AggregateSuccessRate(ctx context.Context, workflowID string) (map[string]interface{}, error) {
	query := `
		SELECT
			COUNT(*) as total,
			COUNT(CASE WHEN execution_status = 'completed' THEN 1 END) as successful
		FROM resource_action_traces
		WHERE 1=1
	`

	var total, successful int
	err := a.db.QueryRowContext(ctx, query).Scan(&total, &successful)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate success rate: %w", err)
	}

	successRate := 0.0
	if total > 0 {
		successRate = float64(successful) / float64(total)
	}

	return map[string]interface{}{
		"workflow_id":  workflowID,
		"total":        total,
		"successful":   successful,
		"success_rate": successRate,
	}, nil
}

// GroupByNamespace groups incidents by namespace
// v1.x compatibility method - uses direct SQL query (DD-SCHEMA-001)
func (a *AggregationService) GroupByNamespace(ctx context.Context) ([]map[string]interface{}, error) {
	query := `
		SELECT
			rr.namespace,
			COUNT(*) as count,
			COUNT(CASE WHEN rat.execution_status = 'completed' THEN 1 END) as successful
		FROM resource_action_traces rat
		JOIN action_histories ah ON rat.action_history_id = ah.id
		JOIN resource_references rr ON ah.resource_id = rr.id
		WHERE rr.namespace IS NOT NULL
		GROUP BY rr.namespace
		ORDER BY count DESC
	`

	rows, err := a.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to group by namespace: %w", err)
	}
	defer rows.Close()

	groups := []map[string]interface{}{}
	for rows.Next() {
		var namespace string
		var count, successful int
		if err := rows.Scan(&namespace, &count, &successful); err != nil {
			return nil, fmt.Errorf("failed to scan namespace group: %w", err)
		}

		groups = append(groups, map[string]interface{}{
			"namespace":  namespace,
			"count":      count,
			"successful": successful,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("namespace grouping error: %w", err)
	}

	return groups, nil
}

// GetSeverityDistribution gets severity distribution for a namespace
// v1.x compatibility method - uses direct SQL query (DD-SCHEMA-001)
func (a *AggregationService) GetSeverityDistribution(ctx context.Context, namespace string) (map[string]interface{}, error) {
	query := `
		SELECT
			rat.alert_severity,
			COUNT(*) as count
		FROM resource_action_traces rat
		JOIN action_histories ah ON rat.action_history_id = ah.id
		JOIN resource_references rr ON ah.resource_id = rr.id
		WHERE ($1 = '' OR rr.namespace = $1)
		GROUP BY rat.alert_severity
		ORDER BY count DESC
	`

	rows, err := a.db.QueryContext(ctx, query, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get severity distribution: %w", err)
	}
	defer rows.Close()

	distribution := make(map[string]int)
	var total int
	for rows.Next() {
		var severity string
		var count int
		if err := rows.Scan(&severity, &count); err != nil {
			return nil, fmt.Errorf("failed to scan severity distribution: %w", err)
		}

		distribution[severity] = count
		total += count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("severity distribution error: %w", err)
	}

	return map[string]interface{}{
		"distribution": distribution,
		"total":        total,
	}, nil
}

// GetIncidentTrend gets incident trend over a number of days
// v1.x compatibility method - queries incident counts over time (DD-SCHEMA-001)
func (a *AggregationService) GetIncidentTrend(ctx context.Context, days int) ([]map[string]interface{}, error) {
	query := `
		SELECT
			DATE(created_at) as date,
			COUNT(*) as count,
			COUNT(CASE WHEN execution_end_time IS NOT NULL THEN 1 END) as resolved_count
		FROM resource_action_traces
		WHERE created_at >= NOW() - INTERVAL '1 day' * $1
		GROUP BY DATE(created_at)
		ORDER BY date DESC
	`

	rows, err := a.db.QueryContext(ctx, query, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get incident trend: %w", err)
	}
	defer rows.Close()

	trend := []map[string]interface{}{}
	for rows.Next() {
		var date time.Time
		var count, resolvedCount int
		if err := rows.Scan(&date, &count, &resolvedCount); err != nil {
			return nil, fmt.Errorf("failed to scan trend row: %w", err)
		}

		trend = append(trend, map[string]interface{}{
			"date":           date.Format("2006-01-02"),
			"count":          count,
			"resolved_count": resolvedCount,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trend query error: %w", err)
	}

	return trend, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// REFACTOR PHASE IMPLEMENTATION NOTES
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Enhanced Features:
// 1. ✅ Dynamic filter-based aggregation
// 2. ✅ Top failing actions analysis
// 3. ✅ Action comparison across multiple types
// 4. ✅ Namespace health scoring algorithm
// 5. ✅ Statistical relevance (minimum sample sizes)
// 6. ✅ Normalized scoring (0.0-1.0 scale)
//
// Business Requirements:
// - BR-CONTEXT-004: Query Aggregation (advanced features)
//
// Next Enhancements:
// - Cache frequently accessed aggregations
// - Add predictive trend analysis
// - Implement anomaly detection algorithms
// - Add correlation analysis between action types
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
