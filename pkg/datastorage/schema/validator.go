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
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
)

// ========================================
// VERSION REQUIREMENTS (DD-011)
// 📋 Design Decision: DD-011 | ✅ Approved Design | Confidence: 99.9%
// See: docs/architecture/decisions/DD-011-postgresql-version-requirements.md
// ========================================
//
// V1.0 UPDATE (2025-12-11): Label-only architecture (no vector embeddings)
// PostgreSQL 16+ is MANDATORY for Kubernaut V1.0.
//
// WHY PostgreSQL 16+ Only?
// - ✅ Modern Features: PostgreSQL 16+ provides latest performance improvements
// - ✅ JSONB Performance: Enhanced JSONB indexing for label matching
// - ✅ Cloud-Ready: All major cloud providers support PostgreSQL 16+
// - ✅ Future-Proof: PostgreSQL 16 released Sept 2023, long support lifecycle
//
// ⚠️ Trade-off: Deployments on PostgreSQL 15 must upgrade
//    Mitigation: Clear error messages, upgrade documentation provided
// ========================================

// MinPostgreSQLMajorVersion is the minimum required PostgreSQL major version
// DD-011: PostgreSQL 16+ required for optimal performance
const MinPostgreSQLMajorVersion = 16

// RecommendedSharedBuffersBytes is the recommended PostgreSQL shared_buffers size
// DD-011: 1GB+ recommended for optimal query performance
// Note: This is a recommendation, not a requirement (validation warns but doesn't block)
const RecommendedSharedBuffersBytes = int64(1024 * 1024 * 1024) // 1GB

// VersionValidator validates PostgreSQL version and configuration
type VersionValidator struct {
	db     *sql.DB
	logger logr.Logger
}

// NewVersionValidator creates a new version validator
func NewVersionValidator(db *sql.DB, logger logr.Logger) *VersionValidator {
	return &VersionValidator{
		db:     db,
		logger: logger,
	}
}

// ValidatePostgreSQLVersion enforces PostgreSQL 16+ requirement
func (v *VersionValidator) ValidatePostgreSQLVersion(ctx context.Context) error {
	v.logger.Info("validating PostgreSQL version")

	pgVersion, err := v.getPostgreSQLVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to detect PostgreSQL version: %w", err)
	}

	pgMajor := v.parsePostgreSQLMajorVersion(pgVersion)
	if pgMajor < MinPostgreSQLMajorVersion {
		return fmt.Errorf(
			"PostgreSQL version %d is not supported. Required: PostgreSQL %d.x or higher. Current: %s. "+
				"Please upgrade to PostgreSQL %d+ (DD-011)",
			pgMajor, MinPostgreSQLMajorVersion, pgVersion, MinPostgreSQLMajorVersion)
	}

	v.logger.Info("PostgreSQL version validated",
		"version", pgVersion,
		"major", pgMajor)

	return nil
}

// ValidateMemoryConfiguration validates PostgreSQL memory configuration
// Warns if shared_buffers is below recommended size, but does not block startup
func (v *VersionValidator) ValidateMemoryConfiguration(ctx context.Context) error {
	v.logger.Info("validating PostgreSQL memory configuration")

	var sharedBuffers string
	err := v.db.QueryRowContext(ctx, "SELECT current_setting('shared_buffers')").Scan(&sharedBuffers)
	if err != nil {
		v.logger.Info("failed to read shared_buffers configuration",
			"error", err,
			"impact", "unable to validate memory configuration")
		return nil // Don't block startup
	}

	bufferSize, err := v.parsePostgreSQLSize(sharedBuffers)
	if err != nil {
		v.logger.Info("failed to parse shared_buffers",
			"error", err,
			"value", sharedBuffers)
		return nil // Don't block startup
	}

	// DD-011: Use recommended buffer size constant
	if bufferSize < RecommendedSharedBuffersBytes {
		v.logger.Info("shared_buffers below recommended size for optimal performance (DD-011)",
			"current", sharedBuffers,
			"recommended", "1GB+",
			"impact", "queries may be slower due to disk I/O",
			"action", "consider increasing shared_buffers in postgresql.conf")
	} else {
		v.logger.Info("memory configuration optimal (DD-011)",
			"shared_buffers", sharedBuffers)
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
		v.logger.Info("failed to parse PostgreSQL major version",
			"version", version)
		return 0
	}
	major, _ := strconv.Atoi(matches[1])
	return major
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
