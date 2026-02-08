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

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
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
	logger logr.Logger
}

// NewService creates a new query service
// BR-STORAGE-005: Service initialization
func NewService(db DBQuerier, logger logr.Logger) *Service {
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

	s.logger.V(1).Info("executing query",
		"query", query,
		"arg_count", len(args))

	// Execute query and scan results into intermediate type
	var results []RemediationAuditResult
	if err := s.db.SelectContext(ctx, &results, query, args...); err != nil {
		metrics.QueryTotal.WithLabelValues(metrics.OperationList, metrics.StatusFailure).Inc()
		s.logger.Error(err, "query failed",
			"query", query)
		return nil, fmt.Errorf("query failed: %w", err)
	}

	// Convert to models.RemediationAudit
	audits := make([]*models.RemediationAudit, len(results))
	for i := range results {
		audits[i] = results[i].ToRemediationAudit()
	}

	metrics.QueryTotal.WithLabelValues(metrics.OperationList, metrics.StatusSuccess).Inc()
	s.logger.Info("query successful",
		"result_count", len(audits))

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
		"total_count", totalCount,
		"page", page,
		"total_pages", totalPages)

	return result, nil
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
		s.logger.Error(err, "count query failed",
			"query", query)
		return 0, fmt.Errorf("count query failed: %w", err)
	}

	return count, nil
}
