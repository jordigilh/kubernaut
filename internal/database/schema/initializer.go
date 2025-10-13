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

package schema

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	"go.uber.org/zap"
)

//go:embed remediation_audit.sql
var remediationAuditSchema string

//go:embed ai_analysis_audit.sql
var aiAnalysisAuditSchema string

//go:embed workflow_audit.sql
var workflowAuditSchema string

//go:embed execution_audit.sql
var executionAuditSchema string

// Initializer handles database schema initialization
// BR-STORAGE-008: Idempotent schema initialization
type Initializer struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewInitializer creates a new schema initializer
func NewInitializer(db *sql.DB, logger *zap.Logger) *Initializer {
	return &Initializer{
		db:     db,
		logger: logger,
	}
}

// Initialize creates all required database schemas (idempotent)
// This method can be called multiple times safely
func (i *Initializer) Initialize(ctx context.Context) error {
	i.logger.Info("Initializing database schema")

	schemas := []struct {
		name   string
		schema string
	}{
		{"remediation_audit", remediationAuditSchema},
		{"ai_analysis_audit", aiAnalysisAuditSchema},
		{"workflow_audit", workflowAuditSchema},
		{"execution_audit", executionAuditSchema},
	}

	for _, s := range schemas {
		if err := i.executeSchema(ctx, s.name, s.schema); err != nil {
			return fmt.Errorf("failed to initialize %s schema: %w", s.name, err)
		}
		i.logger.Info("Schema initialized successfully", zap.String("table", s.name))
	}

	i.logger.Info("Database schema initialization complete")
	return nil
}

// executeSchema executes a schema DDL script
func (i *Initializer) executeSchema(ctx context.Context, name, schema string) error {
	_, err := i.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to execute schema for %s: %w", name, err)
	}
	return nil
}

// Verify checks that all tables exist
func (i *Initializer) Verify(ctx context.Context) error {
	tables := []string{
		"remediation_audit",
		"ai_analysis_audit",
		"workflow_audit",
		"execution_audit",
	}

	for _, table := range tables {
		exists, err := i.tableExists(ctx, table)
		if err != nil {
			return fmt.Errorf("failed to check table %s: %w", table, err)
		}
		if !exists {
			return fmt.Errorf("table %s does not exist", table)
		}
	}

	i.logger.Info("All tables verified successfully")
	return nil
}

// tableExists checks if a table exists in the database
func (i *Initializer) tableExists(ctx context.Context, tableName string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = current_schema()
			AND table_name = $1
		)
	`

	var exists bool
	err := i.db.QueryRowContext(ctx, query, tableName).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}
