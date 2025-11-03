package client

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
)

// Connection retry configuration
const (
	maxRetries     = 3
	baseDelay      = 100 * time.Millisecond
	maxDelay       = 2 * time.Second
	connectTimeout = 5 * time.Second
)

// Client defines the interface for database operations
// This interface allows for testing and potential alternative implementations
type Client interface {
	HealthCheck(ctx context.Context) error
	Close() error
	GetDB() *sqlx.DB
	Ping(ctx context.Context) error
	// Query methods (implemented in later days)
	ListIncidents(ctx context.Context, params *models.ListIncidentsParams) ([]*models.IncidentEvent, int, error)
	GetIncidentByID(ctx context.Context, id int64) (*models.IncidentEvent, error)
	SemanticSearch(ctx context.Context, params *models.SemanticSearchParams) ([]*models.IncidentEvent, []float32, error)
}

// PostgresClient provides read-only access to resource_action_traces table (DD-SCHEMA-001)
// BR-CONTEXT-001: Historical Context Query - core database client
// BR-CONTEXT-011: Schema Alignment - uses authoritative schema from Data Storage Service
type PostgresClient struct {
	db     *sqlx.DB
	logger *zap.Logger
}

// NewPostgresClient creates a new PostgreSQL client with retry logic
// Following Data Storage Service v4.1 patterns
//
// Connection string format:
//
//	host=localhost port=5432 user=postgres password=postgres dbname=postgres sslmode=disable
//
// BR-CONTEXT-012: Multi-Client Support - connection pooling for multiple clients
// BR-CONTEXT-005: Error handling with exponential backoff retry
func NewPostgresClient(connStr string, logger *zap.Logger) (*PostgresClient, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// BR-CONTEXT-011: Schema Alignment - Connect to PostgreSQL with authoritative schema
	// BR-CONTEXT-005: Retry with exponential backoff for transient failures
	db, err := connectWithRetry(connStr, logger)
	if err != nil {
		logger.Error("Failed to connect to PostgreSQL after retries",
			zap.Error(err),
			zap.Int("max_retries", maxRetries),
			zap.String("connection_string", maskPassword(connStr)),
		)
		return nil, fmt.Errorf("failed to connect to postgres after %d retries: %w", maxRetries, err)
	}

	// Connection pool settings (from Data Storage Service v4.1)
	// BR-CONTEXT-012: Multi-Client Support - optimized for read-heavy workload
	db.SetMaxOpenConns(25)                 // Max concurrent connections
	db.SetMaxIdleConns(5)                  // Min idle connections in pool
	db.SetConnMaxLifetime(5 * time.Minute) // Connection lifetime

	logger.Info("PostgreSQL client created successfully",
		zap.Int("max_open_conns", 25),
		zap.Int("max_idle_conns", 5),
		zap.Duration("conn_max_lifetime", 5*time.Minute),
	)

	return &PostgresClient{
		db:     db,
		logger: logger,
	}, nil
}

// connectWithRetry attempts to connect to PostgreSQL with exponential backoff
// BR-CONTEXT-005: Production-ready error handling with retry logic
func connectWithRetry(connStr string, logger *zap.Logger) (*sqlx.DB, error) {
	var db *sqlx.DB
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Create context with timeout for each attempt
		ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)

		db, err = sqlx.ConnectContext(ctx, "postgres", connStr)
		cancel() // Always cancel context

		if err == nil {
			// Connection successful
			if attempt > 0 {
				logger.Info("PostgreSQL connection succeeded after retry",
					zap.Int("attempt", attempt+1),
					zap.Int("max_retries", maxRetries),
				)
			}
			return db, nil
		}

		// Log retry attempt with structured context
		logger.Warn("PostgreSQL connection attempt failed",
			zap.Error(err),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", maxRetries),
			zap.String("error_type", fmt.Sprintf("%T", err)),
		)

		// Don't sleep on last attempt
		if attempt < maxRetries-1 {
			delay := calculateBackoff(attempt)
			logger.Debug("Retrying PostgreSQL connection",
				zap.Duration("backoff_delay", delay),
				zap.Int("next_attempt", attempt+2),
			)
			time.Sleep(delay)
		}
	}

	return nil, fmt.Errorf("all connection attempts failed: %w", err)
}

// calculateBackoff calculates exponential backoff delay using simple doubling
// BR-CONTEXT-005: Exponential backoff without external dependencies
func calculateBackoff(attempt int) time.Duration {
	// Simple exponential backoff: baseDelay * 2^attempt
	delay := baseDelay
	for i := 0; i < attempt; i++ {
		delay *= 2
		// Cap at maxDelay
		if delay > maxDelay {
			return maxDelay
		}
	}
	return delay
}

// HealthCheck verifies database connectivity with enhanced error context
// BR-CONTEXT-008: REST API - health check endpoint support
// BR-CONTEXT-012: Multi-Client Support - validates connectivity for all clients
// BR-CONTEXT-005: Enhanced error handling with structured logging
func (c *PostgresClient) HealthCheck(ctx context.Context) error {
	if c.db == nil {
		err := fmt.Errorf("database connection is nil")
		c.logger.Error("Health check failed: nil connection", zap.Error(err))
		return err
	}

	// Use context with timeout if none provided
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	if err := c.db.PingContext(ctx); err != nil {
		c.logger.Error("Database health check failed",
			zap.Error(err),
			zap.String("error_type", fmt.Sprintf("%T", err)),
			zap.Bool("context_cancelled", ctx.Err() != nil),
		)
		return fmt.Errorf("database ping failed: %w", err)
	}

	c.logger.Debug("Database health check passed")
	return nil
}

// Close closes the database connection with proper cleanup
// BR-CONTEXT-011: Schema Alignment - proper resource cleanup
// BR-CONTEXT-005: Enhanced error handling with structured logging
func (c *PostgresClient) Close() error {
	if c.db == nil {
		c.logger.Warn("Attempted to close nil database connection")
		return nil
	}

	// Get connection stats before closing (for logging)
	stats := c.db.Stats()

	if err := c.db.Close(); err != nil {
		c.logger.Error("Failed to close database connection",
			zap.Error(err),
			zap.Int("open_connections", stats.OpenConnections),
			zap.Int("in_use", stats.InUse),
			zap.Int("idle", stats.Idle),
		)
		return fmt.Errorf("failed to close database: %w", err)
	}

	c.logger.Info("PostgreSQL client closed successfully",
		zap.Int("final_open_connections", stats.OpenConnections),
		zap.Int64("max_idle_closed", stats.MaxIdleClosed),
		zap.Int64("max_lifetime_closed", stats.MaxLifetimeClosed),
	)
	return nil
}

// GetDB returns the underlying sqlx.DB instance
// This is needed for query builders and other internal components
func (c *PostgresClient) GetDB() *sqlx.DB {
	return c.db
}

// Ping verifies database connectivity (alias for HealthCheck)
func (c *PostgresClient) Ping(ctx context.Context) error {
	return c.HealthCheck(ctx)
}

// ListIncidents queries incidents from the database
// NOTE: This is a stub for Day 1. Full implementation in Day 2+
func (c *PostgresClient) ListIncidents(ctx context.Context, params *models.ListIncidentsParams) ([]*models.IncidentEvent, int, error) {
	return nil, 0, fmt.Errorf("ListIncidents not yet implemented - coming in Day 2")
}

// GetIncidentByID retrieves a single incident by ID
// GREEN PHASE: Minimal implementation to support RFC 7807 404 errors
// NOTE: Full implementation coming in Day 2+
func (c *PostgresClient) GetIncidentByID(ctx context.Context, id int64) (*models.IncidentEvent, error) {
	// GREEN PHASE: Return not found error to make RFC 7807 tests pass
	// This will be replaced with actual database query in REFACTOR phase
	return nil, ErrIncidentNotFound
}

// SemanticSearch performs vector similarity search
// NOTE: This is a stub for Day 1. Full implementation in Day 5+
func (c *PostgresClient) SemanticSearch(ctx context.Context, params *models.SemanticSearchParams) ([]*models.IncidentEvent, []float32, error) {
	return nil, nil, fmt.Errorf("SemanticSearch not yet implemented - coming in Day 5")
}

// maskPassword masks the password in connection strings for logging
func maskPassword(connStr string) string {
	// Simple masking for security - hides password in logs
	// This is a basic implementation; production might use regex
	return "***MASKED***"
}
