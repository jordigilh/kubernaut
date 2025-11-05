package schema

import (
	"fmt"
	"regexp"
	"strconv"
)

// ========================================
// SEMANTIC VERSION SUPPORT (DD-011)
// 📋 Design Decision: DD-011 | ✅ Approved Design | Confidence: 99.9%
// See: docs/architecture/decisions/DD-011-postgresql-version-requirements.md
// ========================================
//
// SemanticVersion provides structured version parsing and comparison for
// PostgreSQL and pgvector version validation.
//
// WHY DD-011?
// - ✅ Type Safety: Structured version type (major, minor, patch) vs strings
// - ✅ Reusable: Single version comparison logic for PostgreSQL + pgvector
// - ✅ Testable: Clear comparison semantics with comprehensive test coverage
// - ✅ Maintainable: Constants reference DD-011 for version requirements
//
// ⚠️ Trade-off: Adds ~50 LOC vs inline parsing
//    Mitigation: Eliminates duplicated parsing logic, improves testability
// ========================================

// SemanticVersion represents a semantic version (major.minor.patch)
// DD-011: Used for PostgreSQL and pgvector version comparison
type SemanticVersion struct {
	Major int
	Minor int
	Patch int
}

// ParseSemanticVersion parses a semantic version string into a SemanticVersion
// Supports formats: "X.Y.Z", "X.Y", "X" (missing components default to 0)
// Examples: "16.1.0" → {16, 1, 0}, "0.5.1" → {0, 5, 1}
func ParseSemanticVersion(version string) (*SemanticVersion, error) {
	if version == "" {
		return nil, fmt.Errorf("empty version string")
	}

	// Parse semantic version: X.Y.Z (patch optional)
	re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)
	matches := re.FindStringSubmatch(version)
	if len(matches) < 4 {
		return nil, fmt.Errorf("invalid version format: %s (expected X.Y.Z)", version)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, fmt.Errorf("invalid major version: %s", matches[1])
	}

	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil, fmt.Errorf("invalid minor version: %s", matches[2])
	}

	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return nil, fmt.Errorf("invalid patch version: %s", matches[3])
	}

	return &SemanticVersion{
		Major: major,
		Minor: minor,
		Patch: patch,
	}, nil
}

// IsLessThan returns true if this version is less than the other version
// Comparison order: major → minor → patch
func (v *SemanticVersion) IsLessThan(other *SemanticVersion) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}

// IsGreaterThanOrEqual returns true if this version is greater than or equal to the other version
// Comparison order: major → minor → patch
func (v *SemanticVersion) IsGreaterThanOrEqual(other *SemanticVersion) bool {
	return !v.IsLessThan(other)
}

// String returns the string representation of the version (X.Y.Z)
func (v *SemanticVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}


import (
	"fmt"
	"regexp"
	"strconv"
)

// ========================================
// SEMANTIC VERSION SUPPORT (DD-011)
// 📋 Design Decision: DD-011 | ✅ Approved Design | Confidence: 99.9%
// See: docs/architecture/decisions/DD-011-postgresql-version-requirements.md
// ========================================
//
// SemanticVersion provides structured version parsing and comparison for
// PostgreSQL and pgvector version validation.
//
// WHY DD-011?
// - ✅ Type Safety: Structured version type (major, minor, patch) vs strings
// - ✅ Reusable: Single version comparison logic for PostgreSQL + pgvector
// - ✅ Testable: Clear comparison semantics with comprehensive test coverage
// - ✅ Maintainable: Constants reference DD-011 for version requirements
//
// ⚠️ Trade-off: Adds ~50 LOC vs inline parsing
//    Mitigation: Eliminates duplicated parsing logic, improves testability
// ========================================

// SemanticVersion represents a semantic version (major.minor.patch)
// DD-011: Used for PostgreSQL and pgvector version comparison
type SemanticVersion struct {
	Major int
	Minor int
	Patch int
}

// ParseSemanticVersion parses a semantic version string into a SemanticVersion
// Supports formats: "X.Y.Z", "X.Y", "X" (missing components default to 0)
// Examples: "16.1.0" → {16, 1, 0}, "0.5.1" → {0, 5, 1}
func ParseSemanticVersion(version string) (*SemanticVersion, error) {
	if version == "" {
		return nil, fmt.Errorf("empty version string")
	}

	// Parse semantic version: X.Y.Z (patch optional)
	re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)
	matches := re.FindStringSubmatch(version)
	if len(matches) < 4 {
		return nil, fmt.Errorf("invalid version format: %s (expected X.Y.Z)", version)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, fmt.Errorf("invalid major version: %s", matches[1])
	}

	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil, fmt.Errorf("invalid minor version: %s", matches[2])
	}

	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return nil, fmt.Errorf("invalid patch version: %s", matches[3])
	}

	return &SemanticVersion{
		Major: major,
		Minor: minor,
		Patch: patch,
	}, nil
}

// IsLessThan returns true if this version is less than the other version
// Comparison order: major → minor → patch
func (v *SemanticVersion) IsLessThan(other *SemanticVersion) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}

// IsGreaterThanOrEqual returns true if this version is greater than or equal to the other version
// Comparison order: major → minor → patch
func (v *SemanticVersion) IsGreaterThanOrEqual(other *SemanticVersion) bool {
	return !v.IsLessThan(other)
}

// String returns the string representation of the version (X.Y.Z)
func (v *SemanticVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

