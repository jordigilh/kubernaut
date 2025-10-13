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

package dualwrite

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
)

const (
	// RequiredEmbeddingDimension is the required embedding vector dimension
	RequiredEmbeddingDimension = 384
)

// Coordinator orchestrates dual-write operations to PostgreSQL and Vector DB.
// Business Requirement: BR-STORAGE-014 (Atomic dual-write)
type Coordinator struct {
	db       DB
	vectorDB VectorDBClient
	logger   *zap.Logger
}

// NewCoordinator creates a new dual-write coordinator.
func NewCoordinator(db DB, vectorDB VectorDBClient, logger *zap.Logger) *Coordinator {
	return &Coordinator{
		db:       db,
		vectorDB: vectorDB,
		logger:   logger,
	}
}

// Write performs an atomic dual-write to both PostgreSQL and Vector DB.
// If either operation fails, both are rolled back.
// Business Requirement: BR-STORAGE-014
func (c *Coordinator) Write(ctx context.Context, audit *models.RemediationAudit, embedding []float32) (*WriteResult, error) {
	// Validate inputs
	if audit == nil {
		return nil, fmt.Errorf("audit is nil")
	}
	if embedding == nil {
		return nil, fmt.Errorf("embedding is nil")
	}
	if len(embedding) != RequiredEmbeddingDimension {
		return nil, fmt.Errorf("embedding dimension must be %d, got %d", RequiredEmbeddingDimension, len(embedding))
	}

	c.logger.Debug("starting dual-write operation",
		zap.String("name", audit.Name),
		zap.String("namespace", audit.Namespace),
		zap.Int("embedding_dimension", len(embedding)))

	// Begin PostgreSQL transaction with context
	// BR-STORAGE-016: Use BeginTx for context propagation (cancellation, timeout)
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		c.logger.Error("failed to begin transaction",
			zap.Error(err),
			zap.String("name", audit.Name))
		return nil, fmt.Errorf("begin transaction failed: %w", err)
	}

	// Track if we need to rollback
	shouldRollback := true
	defer func() {
		if shouldRollback {
			if rbErr := tx.Rollback(); rbErr != nil {
				c.logger.Error("failed to rollback transaction",
					zap.Error(rbErr),
					zap.String("name", audit.Name))
			}
		}
	}()

	// Write to PostgreSQL
	pgID, err := c.writeToPostgreSQL(tx, audit, embedding)
	if err != nil {
		c.logger.Error("failed to write to PostgreSQL",
			zap.Error(err),
			zap.String("name", audit.Name))
		return nil, fmt.Errorf("postgresql write failed: %w", err)
	}

	c.logger.Debug("wrote to PostgreSQL",
		zap.Int64("postgresql_id", pgID),
		zap.String("name", audit.Name))

	// Write to Vector DB (if available)
	if c.vectorDB != nil {
		metadata := buildMetadata(audit)
		if err := c.vectorDB.Insert(ctx, pgID, embedding, metadata); err != nil {
			c.logger.Error("failed to write to Vector DB, rolling back",
				zap.Error(err),
				zap.Int64("postgresql_id", pgID),
				zap.String("name", audit.Name))
			return nil, fmt.Errorf("vector DB insert failed: %w", err)
		}

		c.logger.Debug("wrote to Vector DB",
			zap.Int64("postgresql_id", pgID),
			zap.String("name", audit.Name))
	} else {
		c.logger.Warn("Vector DB not configured, skipping vector write",
			zap.Int64("postgresql_id", pgID),
			zap.String("name", audit.Name))
	}

	// Commit PostgreSQL transaction
	if err := tx.Commit(); err != nil {
		c.logger.Error("failed to commit transaction",
			zap.Error(err),
			zap.Int64("postgresql_id", pgID),
			zap.String("name", audit.Name))
		// Rollback will be called by defer
		return nil, fmt.Errorf("commit failed: %w", err)
	}

	// Commit successful, don't rollback
	shouldRollback = false

	c.logger.Info("dual-write completed successfully",
		zap.Int64("postgresql_id", pgID),
		zap.String("name", audit.Name),
		zap.String("namespace", audit.Namespace))

	return &WriteResult{
		PostgreSQLID:      pgID,
		PostgreSQLSuccess: true,
		VectorDBSuccess:   true,
		FallbackMode:      false,
	}, nil
}

// WriteWithFallback attempts dual-write but falls back to PostgreSQL-only on Vector DB failure.
// Business Requirement: BR-STORAGE-015 (Graceful degradation)
func (c *Coordinator) WriteWithFallback(ctx context.Context, audit *models.RemediationAudit, embedding []float32) (*WriteResult, error) {
	// Validate inputs
	if audit == nil {
		return nil, fmt.Errorf("audit is nil")
	}
	if embedding == nil {
		return nil, fmt.Errorf("embedding is nil")
	}
	if len(embedding) != RequiredEmbeddingDimension {
		return nil, fmt.Errorf("embedding dimension must be %d, got %d", RequiredEmbeddingDimension, len(embedding))
	}

	c.logger.Debug("starting dual-write with fallback",
		zap.String("name", audit.Name),
		zap.String("namespace", audit.Namespace))

	// Try normal dual-write first
	result, err := c.Write(ctx, audit, embedding)
	if err == nil {
		return result, nil
	}

	// Check if error is Vector DB related
	if !isVectorDBError(err) {
		// PostgreSQL error - cannot fall back
		c.logger.Error("PostgreSQL error, cannot fall back",
			zap.Error(err),
			zap.String("name", audit.Name))
		return nil, err
	}

	// Vector DB error - fall back to PostgreSQL-only
	c.logger.Warn("Vector DB unavailable, falling back to PostgreSQL-only",
		zap.Error(err),
		zap.String("name", audit.Name))

	pgID, pgErr := c.writePostgreSQLOnly(ctx, audit, embedding)
	if pgErr != nil {
		c.logger.Error("PostgreSQL-only write failed",
			zap.Error(pgErr),
			zap.String("name", audit.Name))
		return nil, fmt.Errorf("postgresql-only write failed: %w", pgErr)
	}

	c.logger.Info("completed with PostgreSQL-only fallback",
		zap.Int64("postgresql_id", pgID),
		zap.String("name", audit.Name))

	return &WriteResult{
		PostgreSQLID:      pgID,
		PostgreSQLSuccess: true,
		VectorDBSuccess:   false,
		VectorDBError:     err.Error(),
		FallbackMode:      true,
	}, nil
}

// writeToPostgreSQL writes audit and embedding to PostgreSQL within a transaction.
func (c *Coordinator) writeToPostgreSQL(tx Tx, audit *models.RemediationAudit, embedding []float32) (int64, error) {
	sqlQuery := `
		INSERT INTO remediation_audit (
			name, namespace, phase, action_type, status,
			start_time, end_time, remediation_request_id, alert_fingerprint,
			severity, environment, cluster_name, target_resource,
			error_message, metadata, embedding
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id
	`

	// Convert []float32 to query.Vector for pgvector compatibility
	// BR-STORAGE-008: Use Vector type which implements driver.Valuer
	vectorEmbedding := query.Vector(embedding)

	// PostgreSQL doesn't support LastInsertId(), use QueryRow with RETURNING instead
	var id int64
	err := tx.QueryRow(sqlQuery,
		audit.Name, audit.Namespace, audit.Phase, audit.ActionType, audit.Status,
		audit.StartTime, audit.EndTime, audit.RemediationRequestID, audit.AlertFingerprint,
		audit.Severity, audit.Environment, audit.ClusterName, audit.TargetResource,
		audit.ErrorMessage, audit.Metadata, vectorEmbedding).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// writePostgreSQLOnly writes audit and embedding to PostgreSQL without transaction.
// Used for fallback when Vector DB is unavailable.
func (c *Coordinator) writePostgreSQLOnly(ctx context.Context, audit *models.RemediationAudit, embedding []float32) (int64, error) {
	// BR-STORAGE-016: Use BeginTx for context propagation (cancellation, timeout)
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction failed: %w", err)
	}

	shouldRollback := true
	defer func() {
		if shouldRollback {
			_ = tx.Rollback()
		}
	}()

	id, err := c.writeToPostgreSQL(tx, audit, embedding)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit failed: %w", err)
	}

	shouldRollback = false
	return id, nil
}

// buildMetadata constructs metadata map for Vector DB.
func buildMetadata(audit *models.RemediationAudit) map[string]interface{} {
	return map[string]interface{}{
		"name":                   audit.Name,
		"namespace":              audit.Namespace,
		"phase":                  audit.Phase,
		"action_type":            audit.ActionType,
		"status":                 audit.Status,
		"remediation_request_id": audit.RemediationRequestID,
		"alert_fingerprint":      audit.AlertFingerprint,
		"severity":               audit.Severity,
		"environment":            audit.Environment,
		"cluster_name":           audit.ClusterName,
		"target_resource":        audit.TargetResource,
	}
}

// isVectorDBError checks if an error is related to Vector DB operations.
func isVectorDBError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return containsAny(errMsg, []string{"vector DB", "vector db", "vectordb", "Vector DB"})
}

// containsAny checks if a string contains any of the given substrings.
func containsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}
