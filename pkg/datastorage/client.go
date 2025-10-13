/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF THE KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package datastorage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dualwrite"
	"github.com/jordigilh/kubernaut/pkg/datastorage/embedding"
	"github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
	"go.uber.org/zap"
)

// Client defines the Data Storage service interface
// BR-STORAGE-005: Service client interface
type Client interface {
	// RemediationAudit operations
	CreateRemediationAudit(ctx context.Context, audit *models.RemediationAudit) error
	UpdateRemediationAudit(ctx context.Context, audit *models.RemediationAudit) error
	GetRemediationAudit(ctx context.Context, id int64) (*models.RemediationAudit, error)
	ListRemediationAudits(ctx context.Context, opts *query.ListOptions) ([]*models.RemediationAudit, error)

	// Other audit operations
	CreateAIAnalysisAudit(ctx context.Context, audit *models.AIAnalysisAudit) error
	CreateWorkflowAudit(ctx context.Context, audit *models.WorkflowAudit) error
	CreateExecutionAudit(ctx context.Context, audit *models.ExecutionAudit) error

	// Semantic search
	SemanticSearch(ctx context.Context, embedding []float32, limit int) ([]*models.RemediationAudit, error)

	// Health check
	Ping(ctx context.Context) error
}

// ClientImpl implements the Client interface
type ClientImpl struct {
	db                *sql.DB
	logger            *zap.Logger
	validator         *validation.Validator
	embeddingPipeline *embedding.Pipeline
	coordinator       *dualwrite.Coordinator
	queryService      *query.Service
}

// NewClient creates a new Data Storage client
// BR-STORAGE-006: Client initialization
// BR-STORAGE-012: Vector similarity search with PostgreSQL 16+ and pgvector 0.5.1+ validation
func NewClient(ctx context.Context, db *sql.DB, logger *zap.Logger) (Client, error) {
	// CRITICAL: Validate PostgreSQL 16+ and pgvector 0.5.1+ support
	// This ensures HNSW index compatibility before any operations
	versionValidator := schema.NewVersionValidator(db, logger)

	// Step 1: Validate HNSW support (PostgreSQL 16+ and pgvector 0.5.1+)
	if err := versionValidator.ValidateHNSWSupport(ctx); err != nil {
		return nil, fmt.Errorf("HNSW validation failed: %w. "+
			"Please upgrade to PostgreSQL 16+ and pgvector 0.5.1+ for vector similarity search support", err)
	}

	// Step 2: Validate memory configuration (warns if suboptimal, but doesn't block)
	if err := versionValidator.ValidateMemoryConfiguration(ctx); err != nil {
		// Log error but continue - memory validation is non-blocking
		logger.Warn("memory configuration validation failed",
			zap.Error(err),
			zap.String("impact", "unable to validate shared_buffers"))
	}

	logger.Info("PostgreSQL and pgvector validation complete - HNSW support confirmed")

	// Initialize data validator
	validator := validation.NewValidator(logger)

	// Initialize embedding pipeline with mock dependencies
	// TODO Day 10: Replace with real AI API and Redis cache
	embeddingAPI := &mockEmbeddingAPI{}
	cache := &mockCache{}
	embeddingPipeline := embedding.NewPipeline(embeddingAPI, cache, logger)

	// Initialize dual-write coordinator with mock Vector DB
	// TODO Day 10: Replace with real Vector DB client
	vectorDB := &mockVectorDB{}
	dbWrapper := &dbAdapter{db: db}
	coordinator := dualwrite.NewCoordinator(dbWrapper, vectorDB, logger)

	// Initialize query service
	sqlxDB := sqlx.NewDb(db, "postgres")
	queryService := query.NewService(sqlxDB, logger)

	return &ClientImpl{
		db:                db,
		logger:            logger,
		validator:         validator,
		embeddingPipeline: embeddingPipeline,
		coordinator:       coordinator,
		queryService:      queryService,
	}, nil
}

// CreateRemediationAudit stores a new remediation audit record
// BR-STORAGE-001: Basic audit persistence
// BR-STORAGE-010: Input validation
// BR-STORAGE-011: Input sanitization
// BR-STORAGE-008: Embedding generation
// BR-STORAGE-014: Atomic dual-write
func (c *ClientImpl) CreateRemediationAudit(ctx context.Context, audit *models.RemediationAudit) error {
	// BR-STORAGE-010: Validate input
	if err := c.validator.ValidateRemediationAudit(audit); err != nil {
		c.logger.Error("validation failed",
			zap.Error(err),
			zap.String("name", audit.Name))
		return fmt.Errorf("validation failed: %w", err)
	}

	// BR-STORAGE-011: Sanitize input
	audit.Name = c.validator.SanitizeString(audit.Name)
	audit.Namespace = c.validator.SanitizeString(audit.Namespace)
	audit.ActionType = c.validator.SanitizeString(audit.ActionType)
	audit.Status = c.validator.SanitizeString(audit.Status)
	if audit.ErrorMessage != nil {
		sanitized := c.validator.SanitizeString(*audit.ErrorMessage)
		audit.ErrorMessage = &sanitized
	}

	// BR-STORAGE-008: Generate embedding
	// Track embedding generation duration
	embeddingStart := time.Now()
	embeddingResult, err := c.embeddingPipeline.Generate(ctx, audit)
	if err != nil {
		c.logger.Error("embedding generation failed",
			zap.Error(err),
			zap.String("name", audit.Name))
		return fmt.Errorf("embedding generation failed: %w", err)
	}

	// Track embedding generation metrics
	// BR-STORAGE-019: Observability for embedding pipeline
	metrics.EmbeddingGenerationDuration.Observe(time.Since(embeddingStart).Seconds())
	if embeddingResult.CacheHit {
		metrics.CacheHits.Inc()
	} else {
		metrics.CacheMisses.Inc()
	}

	// BR-STORAGE-014: Dual-write (atomic write to PostgreSQL + Vector DB)
	writeResult, err := c.coordinator.Write(ctx, audit, embeddingResult.Embedding)
	if err != nil {
		c.logger.Error("dual-write failed",
			zap.Error(err),
			zap.String("name", audit.Name))
		return fmt.Errorf("dual-write failed: %w", err)
	}

	c.logger.Info("remediation audit created",
		zap.Int64("postgresql_id", writeResult.PostgreSQLID),
		zap.Bool("vector_db_success", writeResult.VectorDBSuccess),
		zap.String("name", audit.Name))

	return nil
}

// UpdateRemediationAudit updates an existing remediation audit record
func (c *ClientImpl) UpdateRemediationAudit(ctx context.Context, audit *models.RemediationAudit) error {
	// TODO: Implement during Day 5-6
	c.logger.Info("UpdateRemediationAudit called", zap.Int64("id", audit.ID))
	return nil
}

// GetRemediationAudit retrieves a remediation audit by ID
// BR-STORAGE-005: Query by ID
func (c *ClientImpl) GetRemediationAudit(ctx context.Context, id int64) (*models.RemediationAudit, error) {
	query := "SELECT * FROM remediation_audit WHERE id = $1"
	var audit models.RemediationAudit

	sqlxDB := sqlx.NewDb(c.db, "postgres")
	if err := sqlxDB.GetContext(ctx, &audit, query, id); err != nil {
		if err == sql.ErrNoRows {
			c.logger.Warn("audit not found", zap.Int64("id", id))
			return nil, fmt.Errorf("audit not found: %d", id)
		}
		c.logger.Error("query failed", zap.Error(err), zap.Int64("id", id))
		return nil, fmt.Errorf("query failed: %w", err)
	}

	c.logger.Info("audit retrieved", zap.Int64("id", id))
	return &audit, nil
}

// ListRemediationAudits retrieves remediation audits with filtering and pagination
// BR-STORAGE-007: Query filtering and pagination
func (c *ClientImpl) ListRemediationAudits(ctx context.Context, opts *query.ListOptions) ([]*models.RemediationAudit, error) {
	return c.queryService.ListRemediationAudits(ctx, opts)
}

// CreateAIAnalysisAudit stores a new AI analysis audit record
func (c *ClientImpl) CreateAIAnalysisAudit(ctx context.Context, audit *models.AIAnalysisAudit) error {
	// TODO: Implement during Day 5
	c.logger.Info("CreateAIAnalysisAudit called", zap.String("analysis_id", audit.AnalysisID))
	return nil
}

// CreateWorkflowAudit stores a new workflow audit record
func (c *ClientImpl) CreateWorkflowAudit(ctx context.Context, audit *models.WorkflowAudit) error {
	// TODO: Implement during Day 5
	c.logger.Info("CreateWorkflowAudit called", zap.String("workflow_id", audit.WorkflowID))
	return nil
}

// CreateExecutionAudit stores a new execution audit record
func (c *ClientImpl) CreateExecutionAudit(ctx context.Context, audit *models.ExecutionAudit) error {
	// TODO: Implement during Day 5
	c.logger.Info("CreateExecutionAudit called", zap.String("execution_id", audit.ExecutionID))
	return nil
}

// SemanticSearch performs vector similarity search on remediation audits
func (c *ClientImpl) SemanticSearch(ctx context.Context, embedding []float32, limit int) ([]*models.RemediationAudit, error) {
	// TODO: Implement during Day 6
	c.logger.Info("SemanticSearch called", zap.Int("limit", limit))
	return nil, nil
}

// Ping checks database connectivity
func (c *ClientImpl) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// Mock implementations for dependencies (replaced in Day 10 with real implementations)

// mockEmbeddingAPI simulates AI embedding generation
type mockEmbeddingAPI struct{}

func (m *mockEmbeddingAPI) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Generate simple mock embedding (384 dimensions)
	embedding := make([]float32, 384)
	for i := range embedding {
		embedding[i] = float32(i) / 384.0
	}
	return embedding, nil
}

// mockCache simulates Redis cache
type mockCache struct{}

func (m *mockCache) Get(ctx context.Context, key string) ([]float32, error) {
	return nil, fmt.Errorf("cache miss") // Always miss for now
}

func (m *mockCache) Set(ctx context.Context, key string, embedding []float32, ttl time.Duration) error {
	return nil // No-op for now
}

// mockVectorDB simulates Vector DB operations
type mockVectorDB struct{}

func (m *mockVectorDB) Insert(ctx context.Context, id int64, embedding []float32, metadata map[string]interface{}) error {
	// No-op for integration tests (PostgreSQL is tested, Vector DB is mocked)
	return nil
}

// dbAdapter wraps *sql.DB to implement dualwrite.DB interface
type dbAdapter struct {
	db *sql.DB
}

func (d *dbAdapter) Begin() (dualwrite.Tx, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	return &txAdapter{tx: tx}, nil
}

func (d *dbAdapter) BeginTx(ctx context.Context, opts *sql.TxOptions) (dualwrite.Tx, error) {
	tx, err := d.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &txAdapter{tx: tx}, nil
}

// txAdapter wraps *sql.Tx to implement dualwrite.Tx interface
type txAdapter struct {
	tx *sql.Tx
}

func (t *txAdapter) Exec(query string, args ...interface{}) (sql.Result, error) {
	return t.tx.Exec(query, args...)
}

func (t *txAdapter) QueryRow(query string, args ...interface{}) dualwrite.Row {
	return t.tx.QueryRow(query, args...)
}

func (t *txAdapter) Commit() error {
	return t.tx.Commit()
}

func (t *txAdapter) Rollback() error {
	return t.tx.Rollback()
}
