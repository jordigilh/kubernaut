package schema

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

// VersionValidator validates PostgreSQL and pgvector versions for HNSW support
type VersionValidator struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewVersionValidator creates a new version validator
func NewVersionValidator(db *sql.DB, logger *zap.Logger) *VersionValidator {
	return &VersionValidator{
		db:     db,
		logger: logger,
	}
}

// ValidateHNSWSupport enforces PostgreSQL 16+ and pgvector 0.5.1+ requirements
// BR-STORAGE-012: Vector similarity search requires HNSW index support
func (v *VersionValidator) ValidateHNSWSupport(ctx context.Context) error {
	v.logger.Info("validating PostgreSQL and pgvector versions for HNSW support")

	// Step 1: Validate PostgreSQL version (16+ only)
	pgVersion, err := v.getPostgreSQLVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to detect PostgreSQL version: %w", err)
	}

	pgMajor := v.parsePostgreSQLMajorVersion(pgVersion)
	if pgMajor < 16 {
		return fmt.Errorf(
			"PostgreSQL version %d is not supported. Required: PostgreSQL 16.x or higher. Current: %s. "+
				"Please upgrade to PostgreSQL 16+ for HNSW vector index support",
			pgMajor, pgVersion)
	}

	v.logger.Info("PostgreSQL version validated",
		zap.String("version", pgVersion),
		zap.Int("major", pgMajor),
		zap.Bool("hnsw_supported", true))

	// Step 2: Validate pgvector version (0.5.1+ only)
	pgvectorVersion, err := v.getPgvectorVersion(ctx)
	if err != nil {
		return fmt.Errorf("pgvector extension not installed: %w. "+
			"Please install with: CREATE EXTENSION vector", err)
	}

	if !v.isPgvector051OrHigher(pgvectorVersion) {
		return fmt.Errorf(
			"pgvector version %s is not supported. Required: 0.5.1 or higher. "+
				"Please upgrade pgvector to 0.5.1+ for HNSW support",
			pgvectorVersion)
	}

	v.logger.Info("pgvector version validated",
		zap.String("version", pgvectorVersion),
		zap.Bool("hnsw_supported", true))

	// Step 3: Test HNSW index creation (dry-run)
	err = v.testHNSWIndexCreation(ctx)
	if err != nil {
		return fmt.Errorf("HNSW index creation test failed: %w. "+
			"Your PostgreSQL/pgvector installation does not support HNSW", err)
	}

	v.logger.Info("HNSW support validation complete - all checks passed",
		zap.String("postgres_version", pgVersion),
		zap.String("pgvector_version", pgvectorVersion))

	return nil
}

// ValidateMemoryConfiguration validates PostgreSQL memory configuration
// Warns if shared_buffers is below recommended size, but does not block startup
func (v *VersionValidator) ValidateMemoryConfiguration(ctx context.Context) error {
	v.logger.Info("validating PostgreSQL memory configuration")

	var sharedBuffers string
	err := v.db.QueryRowContext(ctx, "SELECT current_setting('shared_buffers')").Scan(&sharedBuffers)
	if err != nil {
		v.logger.Warn("failed to read shared_buffers configuration",
			zap.Error(err),
			zap.String("impact", "unable to validate memory configuration"))
		return nil // Don't block startup
	}

	bufferSize, err := parsePostgreSQLSize(sharedBuffers)
	if err != nil {
		v.logger.Warn("failed to parse shared_buffers",
			zap.Error(err),
			zap.String("value", sharedBuffers))
		return nil // Don't block startup
	}

	const recommendedBufferSize = 1024 * 1024 * 1024 // 1GB

	if bufferSize < recommendedBufferSize {
		v.logger.Warn("shared_buffers below recommended size for optimal HNSW performance",
			zap.String("current", sharedBuffers),
			zap.String("recommended", "1GB+"),
			zap.String("impact", "vector search may be slower than optimal due to disk I/O"),
			zap.String("action", "consider increasing shared_buffers in postgresql.conf"))
	} else {
		v.logger.Info("memory configuration optimal for HNSW",
			zap.String("shared_buffers", sharedBuffers))
	}

	return nil // Never block, only warn
}

// getPostgreSQLVersion retrieves the PostgreSQL version string
func (v *VersionValidator) getPostgreSQLVersion(ctx context.Context) (string, error) {
	var version string
	err := v.db.QueryRowContext(ctx, "SELECT version()").Scan(&version)
	return version, err
}

// parsePostgreSQLMajorVersion extracts the major version number from PostgreSQL version string
// Example: "PostgreSQL 16.1 on x86_64..." → 16
func (v *VersionValidator) parsePostgreSQLMajorVersion(version string) int {
	re := regexp.MustCompile(`PostgreSQL (\d+)\.`)
	matches := re.FindStringSubmatch(version)
	if len(matches) < 2 {
		v.logger.Warn("failed to parse PostgreSQL major version",
			zap.String("version", version))
		return 0
	}
	major, _ := strconv.Atoi(matches[1])
	return major
}

// getPgvectorVersion retrieves the installed pgvector extension version
func (v *VersionValidator) getPgvectorVersion(ctx context.Context) (string, error) {
	var version string
	err := v.db.QueryRowContext(ctx, `
		SELECT extversion
		FROM pg_extension
		WHERE extname = 'vector'
	`).Scan(&version)

	if err == sql.ErrNoRows {
		return "", fmt.Errorf("pgvector extension is not installed")
	}
	return version, err
}

// isPgvector051OrHigher checks if pgvector version is 0.5.1 or higher
// Examples: "0.5.1" → true, "0.6.0" → true, "0.5.0" → false, "0.4.x" → false
func (v *VersionValidator) isPgvector051OrHigher(version string) bool {
	re := regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
	matches := re.FindStringSubmatch(version)
	if len(matches) < 4 {
		v.logger.Warn("failed to parse pgvector version",
			zap.String("version", version))
		return false
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	// Require 0.5.1 or higher
	if major > 0 {
		return true // 1.0.0+
	}
	if minor > 5 {
		return true // 0.6.0+
	}
	if minor == 5 && patch >= 1 {
		return true // 0.5.1+
	}
	return false
}

// testHNSWIndexCreation performs a dry-run test of HNSW index creation
// Creates a temporary table and attempts to create an HNSW index
func (v *VersionValidator) testHNSWIndexCreation(ctx context.Context) error {
	v.logger.Debug("performing HNSW index creation test")

	// Create temporary table with vector column
	_, err := v.db.ExecContext(ctx, `
		CREATE TEMP TABLE hnsw_validation_test (
			id SERIAL PRIMARY KEY,
			embedding vector(384)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create test table: %w", err)
	}

	// Attempt HNSW index creation
	_, err = v.db.ExecContext(ctx, `
		CREATE INDEX hnsw_validation_test_idx ON hnsw_validation_test
		USING hnsw (embedding vector_cosine_ops)
		WITH (m = 16, ef_construction = 64)
	`)
	if err != nil {
		return fmt.Errorf("HNSW index creation failed: %w", err)
	}

	v.logger.Debug("HNSW index creation test passed")
	return nil
}

// parsePostgreSQLSize parses PostgreSQL size strings into bytes
// Examples: "128MB" → 134217728, "1GB" → 1073741824, "8192kB" → 8388608
func parsePostgreSQLSize(size string) (int64, error) {
	size = strings.TrimSpace(size)
	re := regexp.MustCompile(`(\d+)\s*(kB|MB|GB|TB)?`)
	matches := re.FindStringSubmatch(size)
	if len(matches) < 2 {
		return 0, fmt.Errorf("invalid size format: %s", size)
	}

	value, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value: %s", matches[1])
	}

	unit := strings.ToUpper(strings.TrimSpace(matches[2]))

	switch unit {
	case "TB":
		return value * 1024 * 1024 * 1024 * 1024, nil
	case "GB":
		return value * 1024 * 1024 * 1024, nil
	case "MB", "":
		return value * 1024 * 1024, nil
	case "KB":
		return value * 1024, nil
	default:
		// PostgreSQL default unit is 8kB blocks
		return value * 8192, nil
	}
}

