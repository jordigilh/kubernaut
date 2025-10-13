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

package query

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"go.uber.org/zap"
)

// ListOptions defines filtering and pagination options
// BR-STORAGE-007: Query filtering and pagination
type ListOptions struct {
	Namespace string
	Phase     string
	Status    string
	Limit     int
	Offset    int
	OrderBy   string // e.g., "start_time DESC"
}

// DBQuerier defines the database query interface (sqlx-compatible)
// BR-STORAGE-005: Database query abstraction
//
// DESIGN RATIONALE: Why args ...interface{}?
// The args parameter uses interface{} for direct compatibility with database/sql
// and sqlx standard library signatures. This is the industry standard approach used
// by all major Go SQL libraries (database/sql, sqlx, pgx, gorm).
//
// Supported parameter types (enforced by PostgreSQL driver at query execution):
//   - string, int, int64, float32, float64, bool
//   - time.Time, []byte
//   - query.Vector ([]float32 for pgvector)
//   - Other types supported by lib/pq PostgreSQL driver
//
// Type safety is enforced at:
//  1. Query execution time by PostgreSQL driver
//  2. Integration tests validating common query patterns
//  3. Database schema constraints
//
// Alternative approaches (strongly-typed parameters, generics) would break
// compatibility with *sqlx.DB, requiring wrapper layers with runtime overhead.
type DBQuerier interface {
	// SelectContext binds query results to a slice of structs
	// args accepts SQL parameter values of any type supported by the PostgreSQL driver
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error

	// GetContext binds query result to a single struct
	// args accepts SQL parameter values of any type supported by the PostgreSQL driver
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error

	// ExecContext executes a query without returning rows (for SET commands, etc.)
	// Returns sql.Result for compatibility with sqlx.DB
	// args accepts SQL parameter values of any type supported by the PostgreSQL driver
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// Service handles query operations
// BR-STORAGE-005: Query service implementation
type Service struct {
	db     DBQuerier
	logger *zap.Logger
}

// NewService creates a new query service
// BR-STORAGE-005: Service initialization
func NewService(db DBQuerier, logger *zap.Logger) *Service {
	return &Service{
		db:     db,
		logger: logger,
	}
}

// ListRemediationAudits queries remediation audits with filters and pagination
// BR-STORAGE-005: List with filtering
func (s *Service) ListRemediationAudits(ctx context.Context, opts *ListOptions) ([]*models.RemediationAudit, error) {
	// Track query duration for observability
	// BR-STORAGE-019: Prometheus metrics
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.QueryDuration.WithLabelValues(metrics.OperationList).Observe(duration.Seconds())
	}()

	// Build base query
	query := "SELECT * FROM remediation_audit WHERE 1=1"
	args := []interface{}{}
	argCount := 1

	// Apply filters dynamically
	if opts.Namespace != "" {
		query += fmt.Sprintf(" AND namespace = $%d", argCount)
		args = append(args, opts.Namespace)
		argCount++
	}
	if opts.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argCount)
		args = append(args, opts.Status)
		argCount++
	}
	if opts.Phase != "" {
		query += fmt.Sprintf(" AND phase = $%d", argCount)
		args = append(args, opts.Phase)
		argCount++
	}

	// Add ordering (always by start_time DESC for consistency)
	query += " ORDER BY start_time DESC"

	// Apply pagination
	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, opts.Limit)
		argCount++
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, opts.Offset)
	}

	s.logger.Debug("executing query",
		zap.String("query", query),
		zap.Int("arg_count", len(args)))

	// Execute query and scan results into intermediate type
	// We use RemediationAuditResult because it has query.Vector which implements sql.Scanner
	var results []RemediationAuditResult
	if err := s.db.SelectContext(ctx, &results, query, args...); err != nil {
		metrics.QueryTotal.WithLabelValues(metrics.OperationList, metrics.StatusFailure).Inc()
		s.logger.Error("query failed",
			zap.Error(err),
			zap.String("query", query))
		return nil, fmt.Errorf("query failed: %w", err)
	}

	// Convert to models.RemediationAudit
	audits := make([]*models.RemediationAudit, len(results))
	for i := range results {
		audits[i] = results[i].ToRemediationAudit()
	}

	metrics.QueryTotal.WithLabelValues(metrics.OperationList, metrics.StatusSuccess).Inc()
	s.logger.Info("query successful",
		zap.Int("result_count", len(audits)))

	return audits, nil
}

// PaginatedList returns paginated results with metadata
// BR-STORAGE-006: Pagination support
func (s *Service) PaginatedList(ctx context.Context, opts *ListOptions) (*PaginationResult, error) {
	// Get total count with same filters
	totalCount, err := s.countRemediationAudits(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("count query failed: %w", err)
	}

	// Get paginated data
	audits, err := s.ListRemediationAudits(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list query failed: %w", err)
	}

	// Calculate pagination metadata
	page := 1
	if opts.Limit > 0 && opts.Offset > 0 {
		page = (opts.Offset / opts.Limit) + 1
	}

	totalPages := 1
	if opts.Limit > 0 && totalCount > 0 {
		totalPages = int((totalCount + int64(opts.Limit) - 1) / int64(opts.Limit))
	}

	result := &PaginationResult{
		Data:       audits,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   opts.Limit,
		TotalPages: totalPages,
	}

	s.logger.Info("pagination successful",
		zap.Int64("total_count", totalCount),
		zap.Int("page", page),
		zap.Int("total_pages", totalPages))

	return result, nil
}

// SemanticSearch performs vector similarity search
// BR-STORAGE-012: Semantic search with HNSW index optimization
func (s *Service) SemanticSearch(ctx context.Context, queryText string) ([]*SemanticResult, error) {
	// Track semantic search duration for observability
	// BR-STORAGE-019: Prometheus metrics for vector search performance
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.QueryDuration.WithLabelValues(metrics.OperationSemanticSearch).Observe(duration.Seconds())
	}()

	// Validate query
	if queryText == "" {
		metrics.QueryTotal.WithLabelValues(metrics.OperationSemanticSearch, metrics.StatusFailure).Inc()
		return nil, fmt.Errorf("query string cannot be empty")
	}

	// TODO: Generate embedding for query (mock for now, will integrate with embedding pipeline)
	// For DO-GREEN phase, we'll use a mock embedding
	queryEmbedding := generateMockEmbedding(queryText)

	// Convert embedding to pgvector string format
	queryEmbeddingStr := embeddingToString(queryEmbedding)

	// Set query planner hints to force HNSW index usage
	// This ensures PostgreSQL uses the HNSW index even with complex WHERE clauses
	// SET LOCAL ensures hints only apply to this transaction, not the entire session
	plannerHints := `
		SET LOCAL enable_seqscan = off;
		SET LOCAL enable_indexscan = on;
	`

	if _, err := s.db.ExecContext(ctx, plannerHints); err != nil {
		// Log warning but don't fail the query
		// Planner hints are an optimization, not a requirement
		s.logger.Warn("failed to set query planner hints for HNSW optimization",
			zap.Error(err),
			zap.String("impact", "query may not use HNSW index optimally, performance could be degraded"))
	} else {
		s.logger.Debug("query planner hints set successfully",
			zap.String("hint", "enable_seqscan=off, enable_indexscan=on"))
	}

	// Perform vector similarity search using pgvector
	// <=> is the cosine distance operator in pgvector
	// 1 - distance = similarity score (0 to 1, where 1 is most similar)
	// With planner hints, PostgreSQL will prefer using the HNSW index
	// NOTE: Cast parameter to vector (not public.vector) since the column is already typed
	// and the <=> operator is defined for vector type
	sqlQuery := `
		SELECT
			id, name, namespace, phase, action_type, status, start_time, end_time,
			duration, remediation_request_id, alert_fingerprint, severity,
			environment, cluster_name, target_resource, error_message, metadata,
			embedding, created_at, updated_at,
			(1 - (embedding <=> $1::vector)) as similarity
		FROM remediation_audit
		WHERE embedding IS NOT NULL
		ORDER BY embedding <=> $1::vector
		LIMIT 10
	`

	// Execute query and scan results into SemanticResultRow (with Vector type)
	rows := make([]*SemanticResultRow, 0)
	if err := s.db.SelectContext(ctx, &rows, sqlQuery, queryEmbeddingStr); err != nil {
		metrics.QueryTotal.WithLabelValues(metrics.OperationSemanticSearch, metrics.StatusFailure).Inc()
		s.logger.Error("semantic search failed",
			zap.Error(err),
			zap.String("query", queryText))
		return nil, fmt.Errorf("semantic search failed: %w", err)
	}

	metrics.QueryTotal.WithLabelValues(metrics.OperationSemanticSearch, metrics.StatusSuccess).Inc()

	// Convert to SemanticResult (with []float32)
	results := make([]*SemanticResult, len(rows))
	for i, row := range rows {
		results[i] = row.ToSemanticResult()
	}

	s.logger.Info("semantic search successful",
		zap.String("query", queryText),
		zap.Int("result_count", len(results)))

	return results, nil
}

// countRemediationAudits returns total count for pagination
func (s *Service) countRemediationAudits(ctx context.Context, opts *ListOptions) (int64, error) {
	// Build COUNT query with same filters as ListRemediationAudits
	query := "SELECT COUNT(*) FROM remediation_audit WHERE 1=1"
	args := []interface{}{}
	argCount := 1

	// Apply same filters
	if opts.Namespace != "" {
		query += fmt.Sprintf(" AND namespace = $%d", argCount)
		args = append(args, opts.Namespace)
		argCount++
	}
	if opts.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argCount)
		args = append(args, opts.Status)
		argCount++
	}
	if opts.Phase != "" {
		query += fmt.Sprintf(" AND phase = $%d", argCount)
		args = append(args, opts.Phase)
	}

	// Execute count query
	var count int64
	if err := s.db.GetContext(ctx, &count, query, args...); err != nil {
		s.logger.Error("count query failed",
			zap.Error(err),
			zap.String("query", query))
		return 0, fmt.Errorf("count query failed: %w", err)
	}

	return count, nil
}

// generateMockEmbedding creates a mock 384-dimensional embedding for testing
// TODO: Replace with real embedding generation in integration with embedding pipeline
func generateMockEmbedding(text string) []float32 {
	embedding := make([]float32, 384)
	// Simple hash-based mock embedding
	for i := range embedding {
		embedding[i] = float32((len(text)+i)%100) * 0.01
	}
	return embedding
}

// embeddingToString converts a float32 slice to pgvector string format '[x,y,z,...]'
func embeddingToString(embedding []float32) string {
	if len(embedding) == 0 {
		return "[]"
	}

	result := "["
	for i, val := range embedding {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf("%f", val)
	}
	result += "]"
	return result
}
