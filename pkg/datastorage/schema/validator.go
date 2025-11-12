package schema

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/mod/semver"
)

// ========================================
// VERSION REQUIREMENTS (DD-011)
// 📋 Design Decision: DD-011 | ✅ Approved Design | Confidence: 99.9%
// See: docs/architecture/decisions/DD-011-postgresql-version-requirements.md
// ========================================
//
// PostgreSQL 16+ and pgvector 0.5.1+ are MANDATORY requirements for Kubernaut.
//
// WHY DD-011 (Alternative A: PostgreSQL 16+ Only)?
// - ✅ Stable HNSW: PostgreSQL 16+ provides mature HNSW vector index support
// - ✅ Performance: pgvector 0.5.1+ includes 20-30% HNSW performance improvements
// - ✅ Simplicity: Single version requirement eliminates compatibility matrix
// - ✅ Cloud-Ready: All major cloud providers support PostgreSQL 16+
// - ✅ Future-Proof: PostgreSQL 16 released Sept 2023, long support lifecycle
//
// ⚠️ Trade-off: Deployments on PostgreSQL 15 must upgrade
//    Mitigation: Clear error messages, upgrade documentation provided
// ========================================

// MinPostgreSQLMajorVersion is the minimum required PostgreSQL major version
// DD-011: PostgreSQL 16+ required for stable HNSW support
// BR-STORAGE-012: Vector similarity search requires HNSW index support
const MinPostgreSQLMajorVersion = 16

// MinPgvectorVersion is the minimum required pgvector version
// DD-011: pgvector 0.5.1+ required for HNSW performance optimizations
// BR-STORAGE-012: Vector similarity search requires HNSW index support
// Using golang.org/x/mod/semver (official Go versioning library)
const MinPgvectorVersion = "v0.5.1"

// RecommendedSharedBuffersBytes is the recommended PostgreSQL shared_buffers size
// DD-011: 1GB+ recommended for optimal HNSW vector search performance
// Note: This is a recommendation, not a requirement (validation warns but doesn't block)
const RecommendedSharedBuffersBytes = int64(1024 * 1024 * 1024) // 1GB

// DefaultHNSWM is the default HNSW index 'm' parameter (max connections per layer)
// DD-011: m=16 provides good balance of recall and build time
// Higher values: Better recall, slower build time, more memory
// Lower values: Faster build time, less memory, lower recall
const DefaultHNSWM = 16

// DefaultHNSWEfConstruction is the default HNSW index 'ef_construction' parameter
// DD-011: ef_construction=64 provides good recall with reasonable build time
// Higher values: Better recall, slower build time
// Lower values: Faster build time, lower recall
const DefaultHNSWEfConstruction = 64

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
	if pgMajor < MinPostgreSQLMajorVersion {
		return fmt.Errorf(
			"PostgreSQL version %d is not supported. Required: PostgreSQL %d.x or higher. Current: %s. "+
				"Please upgrade to PostgreSQL %d+ for HNSW vector index support (DD-011)",
			pgMajor, MinPostgreSQLMajorVersion, pgVersion, MinPostgreSQLMajorVersion)
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

	bufferSize, err := v.parsePostgreSQLSize(sharedBuffers)
	if err != nil {
		v.logger.Warn("failed to parse shared_buffers",
			zap.Error(err),
			zap.String("value", sharedBuffers))
		return nil // Don't block startup
	}

	// DD-011: Use recommended buffer size constant
	if bufferSize < RecommendedSharedBuffersBytes {
		v.logger.Warn("shared_buffers below recommended size for optimal HNSW performance (DD-011)",
			zap.String("current", sharedBuffers),
			zap.String("recommended", "1GB+"),
			zap.String("impact", "vector search may be slower than optimal due to disk I/O"),
			zap.String("action", "consider increasing shared_buffers in postgresql.conf"))
	} else {
		v.logger.Info("memory configuration optimal for HNSW (DD-011)",
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
// DD-011: Uses SemanticVersion for type-safe version comparison
// Examples: "0.5.1" → true, "0.6.0" → true, "0.5.0" → false, "0.4.x" → false
func (v *VersionValidator) isPgvector051OrHigher(versionStr string) bool {
	// Normalize version string to semver format (add "v" prefix if missing)
	// golang.org/x/mod/semver requires "v" prefix (e.g., "v0.5.1")
	if !strings.HasPrefix(versionStr, "v") {
		versionStr = "v" + versionStr
	}

	// Validate version format
	if !semver.IsValid(versionStr) {
		v.logger.Warn("invalid pgvector version format",
			zap.String("version", versionStr))
		return false
	}

	// Compare with DD-011 minimum version (v0.5.1)
	// semver.Compare returns: -1 (less), 0 (equal), 1 (greater)
	return semver.Compare(versionStr, MinPgvectorVersion) >= 0
}

// testHNSWIndexCreation performs a dry-run test of HNSW index creation
// Creates a temporary table and attempts to create an HNSW index
func (v *VersionValidator) testHNSWIndexCreation(ctx context.Context) error {
	v.logger.Debug("performing HNSW index creation test")

	// Create temporary table with vector column
	// NOTE: Use public.vector to ensure type is found when search_path is set to test schemas
	// NOTE: IF NOT EXISTS prevents collisions when multiple tests run in parallel
	_, err := v.db.ExecContext(ctx, `
		CREATE TEMP TABLE IF NOT EXISTS hnsw_validation_test (
			id SERIAL PRIMARY KEY,
			embedding public.vector(384)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create test table: %w", err)
	}

	// Attempt HNSW index creation
	// NOTE: Use public.vector_cosine_ops to ensure operator class is found
	// NOTE: IF NOT EXISTS prevents collisions when multiple tests run in parallel
	// DD-011: Use default HNSW parameters (m=16, ef_construction=64)
	query := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS hnsw_validation_test_idx ON hnsw_validation_test
		USING hnsw (embedding public.vector_cosine_ops)
		WITH (m = %d, ef_construction = %d)
	`, DefaultHNSWM, DefaultHNSWEfConstruction)

	_, err = v.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("HNSW index creation failed: %w", err)
	}

	v.logger.Debug("HNSW index creation test passed")
	return nil
}

// parsePostgreSQLSize parses PostgreSQL size strings into bytes
// Examples: "128MB" → 134217728, "1GB" → 1073741824, "8192kB" → 8388608
// DD-011: Refactored to method for consistency with other validator methods
func (v *VersionValidator) parsePostgreSQLSize(size string) (int64, error) {
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
