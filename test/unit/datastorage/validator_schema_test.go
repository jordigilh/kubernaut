package datastorage

import (
	"context"
	"database/sql"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
)

// Test entry point moved to schema_validation_test.go to avoid "Rerunning Suite" error

var _ = Describe("VersionValidator", func() {
	var (
		mockDB    *sql.DB
		mock      sqlmock.Sqlmock
		validator *schema.VersionValidator
		ctx       context.Context
		logger    *zap.Logger
	)

	BeforeEach(func() {
		var err error
		mockDB, mock, err = sqlmock.New()
		Expect(err).ToNot(HaveOccurred())

		logger = zap.NewNop()
		validator = schema.NewVersionValidator(mockDB, logger)
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = mockDB.Close()
	})

	Describe("ValidateHNSWSupport", func() {
		// ✅ BR-STORAGE-003: Table-Driven Tests for Supported PostgreSQL + pgvector Versions
		// Following Implementation Plan: Use DescribeTable for edge cases and correctness validation
		DescribeTable("should pass validation for supported PostgreSQL and pgvector versions",
			func(pgVersion string, pgvectorVersion string, description string) {
				// Mock PostgreSQL version query
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow(pgVersion))

				// Mock pgvector version query
				mock.ExpectQuery("SELECT extversion FROM pg_extension WHERE extname = 'vector'").
					WillReturnRows(sqlmock.NewRows([]string{"extversion"}).
						AddRow(pgvectorVersion))

				// Mock HNSW test table creation
				mock.ExpectExec("CREATE TEMP TABLE IF NOT EXISTS hnsw_validation_test").
					WillReturnResult(sqlmock.NewResult(0, 0))

				// Mock HNSW index creation
				mock.ExpectExec("CREATE INDEX IF NOT EXISTS hnsw_validation_test_idx").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).ToNot(HaveOccurred(), description)
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			},
			Entry("PostgreSQL 16.0 with pgvector 0.5.1 (minimum supported)",
				"PostgreSQL 16.0 on x86_64-pc-linux-gnu, compiled by gcc", "0.5.1",
				"Minimum supported versions should pass"),
			Entry("PostgreSQL 16.2 on ARM with pgvector 0.6.0",
				"PostgreSQL 16.2 on aarch64-apple-darwin23.0.0", "0.6.0",
				"ARM architecture with newer pgvector should pass"),
			Entry("PostgreSQL 17.0 with pgvector 0.7.0 (future-proof)",
				"PostgreSQL 17.0 on x86_64-pc-linux-gnu", "0.7.0",
				"Future PostgreSQL versions should be supported"),
			Entry("PostgreSQL 16.1 with pgvector 0.5.1 (edge case: exact minimum)",
				"PostgreSQL 16.1 on x86_64-pc-linux-gnu", "0.5.1",
				"Exact minimum versions should pass"),
			Entry("PostgreSQL 18.0 with pgvector 1.0.0 (far future version)",
				"PostgreSQL 18.0 on x86_64-pc-linux-gnu", "1.0.0",
				"Far future versions should be supported"),
		)

		// ✅ BR-STORAGE-003: Table-Driven Tests for Unsupported PostgreSQL Versions
		DescribeTable("should fail validation for unsupported PostgreSQL versions",
			func(pgVersion string, expectedMajorVersion string, description string) {
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow(pgVersion))

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).To(HaveOccurred(), description)
				Expect(err.Error()).To(ContainSubstring("PostgreSQL version " + expectedMajorVersion + " is not supported"))
				Expect(err.Error()).To(ContainSubstring("Required: PostgreSQL 16.x or higher"))
			},
			Entry("PostgreSQL 15.4 (one version below minimum)",
				"PostgreSQL 15.4 on x86_64-pc-linux-gnu", "15",
				"PostgreSQL 15 should be rejected"),
			Entry("PostgreSQL 14.9 (two versions below minimum)",
				"PostgreSQL 14.9 on x86_64-pc-linux-gnu", "14",
				"PostgreSQL 14 should be rejected"),
			Entry("PostgreSQL 13.12 (three versions below minimum)",
				"PostgreSQL 13.12 on x86_64-pc-linux-gnu", "13",
				"PostgreSQL 13 should be rejected"),
			Entry("PostgreSQL 12.16 (four versions below minimum)",
				"PostgreSQL 12.16 on x86_64-pc-linux-gnu", "12",
				"PostgreSQL 12 should be rejected"),
			Entry("PostgreSQL 11.20 (far below minimum)",
				"PostgreSQL 11.20 on x86_64-pc-linux-gnu", "11",
				"Very old PostgreSQL should be rejected"),
			Entry("PostgreSQL 10.23 (ancient version)",
				"PostgreSQL 10.23 on x86_64-pc-linux-gnu", "10",
				"Ancient PostgreSQL should be rejected"),
		)

		// ✅ BR-STORAGE-003: Table-Driven Tests for Unsupported pgvector Versions
		DescribeTable("should fail validation for unsupported pgvector versions",
			func(pgvectorVersion string, description string) {
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow("PostgreSQL 16.1 on x86_64-pc-linux-gnu"))

				mock.ExpectQuery("SELECT extversion FROM pg_extension WHERE extname = 'vector'").
					WillReturnRows(sqlmock.NewRows([]string{"extversion"}).
						AddRow(pgvectorVersion))

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).To(HaveOccurred(), description)
				Expect(err.Error()).To(ContainSubstring("pgvector version " + pgvectorVersion + " is not supported"))
				Expect(err.Error()).To(ContainSubstring("Required: 0.5.1 or higher"))
			},
			Entry("pgvector 0.5.0 (one patch below minimum)",
				"0.5.0",
				"pgvector 0.5.0 should be rejected (HNSW introduced in 0.5.1)"),
			Entry("pgvector 0.4.0 (one minor below minimum)",
				"0.4.0",
				"pgvector 0.4.0 lacks HNSW support"),
			Entry("pgvector 0.3.0 (two minors below minimum)",
				"0.3.0",
				"pgvector 0.3.0 lacks HNSW support"),
			Entry("pgvector 0.2.0 (very old version)",
				"0.2.0",
				"Very old pgvector should be rejected"),
			Entry("pgvector 0.1.0 (ancient version)",
				"0.1.0",
				"Ancient pgvector should be rejected"),
		)

		Context("when pgvector extension is not installed", func() {
			It("should fail with clear error message", func() {
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow("PostgreSQL 16.1 on x86_64-pc-linux-gnu"))

				mock.ExpectQuery("SELECT extversion FROM pg_extension WHERE extname = 'vector'").
					WillReturnError(sql.ErrNoRows)

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pgvector extension is not installed"))
				Expect(err.Error()).To(ContainSubstring("CREATE EXTENSION vector"))
			})
		})

		Context("when HNSW index creation fails", func() {
			It("should fail with clear error message", func() {
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow("PostgreSQL 16.1 on x86_64-pc-linux-gnu"))

				mock.ExpectQuery("SELECT extversion FROM pg_extension WHERE extname = 'vector'").
					WillReturnRows(sqlmock.NewRows([]string{"extversion"}).
						AddRow("0.5.1"))

				mock.ExpectExec("CREATE TEMP TABLE hnsw_validation_test").
					WillReturnResult(sqlmock.NewResult(0, 0))

				mock.ExpectExec("CREATE INDEX hnsw_validation_test_idx").
					WillReturnError(sql.ErrConnDone)

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("HNSW index creation test failed"))
				Expect(err.Error()).To(ContainSubstring("does not support HNSW"))
			})
		})
	})

	Describe("ValidateMemoryConfiguration", func() {
		// ✅ BR-STORAGE-003: Table-Driven Tests for Memory Configuration Validation
		DescribeTable("should handle various shared_buffers configurations",
			func(sharedBuffers string, shouldWarn bool, description string) {
				mock.ExpectQuery("SELECT current_setting\\('shared_buffers'\\)").
					WillReturnRows(sqlmock.NewRows([]string{"current_setting"}).
						AddRow(sharedBuffers))

				// Memory validation never fails startup, only warns
				err := validator.ValidateMemoryConfiguration(ctx)
				Expect(err).ToNot(HaveOccurred(), description)
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			},
			Entry("1GB shared_buffers (recommended minimum)",
				"1GB", false,
				"1GB should pass without warnings"),
			Entry("2GB shared_buffers (comfortable)",
				"2GB", false,
				"2GB should pass without warnings"),
			Entry("4GB shared_buffers (generous)",
				"4GB", false,
				"4GB should pass without warnings"),
			Entry("8GB shared_buffers (high performance)",
				"8GB", false,
				"8GB should pass without warnings"),
			Entry("512MB shared_buffers (low but functional)",
				"512MB", true,
				"512MB should warn but not fail"),
			Entry("128MB shared_buffers (minimal)",
				"128MB", true,
				"128MB should warn but not fail"),
			Entry("64MB shared_buffers (very low)",
				"64MB", true,
				"64MB should warn but not fail"),
		)

		Context("when unable to read memory configuration", func() {
			It("should not fail startup if query fails", func() {
				mock.ExpectQuery("SELECT current_setting\\('shared_buffers'\\)").
					WillReturnError(sql.ErrConnDone)

				// Should not return error, only warn
				err := validator.ValidateMemoryConfiguration(ctx)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	// ========================================
	// TDD REFACTOR: Semantic Version Support
	// BR-STORAGE-003: Version Parsing and Comparison
	// TESTING PRINCIPLE: Behavior + Correctness (Implementation Plan V4.9)
	// ========================================
	Describe("SemanticVersion", func() {
		Context("parsing version strings", func() {
			// BEHAVIOR: ParseSemanticVersion extracts major, minor, patch from "X.Y.Z" format
			// CORRECTNESS: Validate exact numeric values are extracted correctly
			DescribeTable("should parse valid version strings into correct SemanticVersion components",
				func(versionStr string, expectedMajor, expectedMinor, expectedPatch int) {
					// ACT: Parse version string
					version, err := schema.ParseSemanticVersion(versionStr)

					// ASSERT: No error occurred
					Expect(err).ToNot(HaveOccurred(), "ParseSemanticVersion(%q) should not return error", versionStr)

					// ASSERT: Version object created (not nil)
					Expect(version).ToNot(BeNil(), "ParseSemanticVersion(%q) should return non-nil version", versionStr)

					// CORRECTNESS: Validate each component is exactly correct
					Expect(version.Major).To(Equal(expectedMajor), "Major version should be %d for %q", expectedMajor, versionStr)
					Expect(version.Minor).To(Equal(expectedMinor), "Minor version should be %d for %q", expectedMinor, versionStr)
					Expect(version.Patch).To(Equal(expectedPatch), "Patch version should be %d for %q", expectedPatch, versionStr)

					// CORRECTNESS: Validate String() round-trips correctly
					Expect(version.String()).To(Equal(versionStr), "String() should return original version string")
				},
				Entry("standard version 1.2.3", "1.2.3", 1, 2, 3),
				Entry("pgvector minimum 0.5.1 (DD-011)", "0.5.1", 0, 5, 1),
				Entry("pgvector 0.6.0", "0.6.0", 0, 6, 0),
				Entry("pgvector 1.0.0", "1.0.0", 1, 0, 0),
				Entry("PostgreSQL 16.1.0 (DD-011 minimum)", "16.1.0", 16, 1, 0),
				Entry("PostgreSQL 17.0.0", "17.0.0", 17, 0, 0),
				Entry("edge case: all zeros", "0.0.0", 0, 0, 0),
				Entry("edge case: large version numbers", "99.99.99", 99, 99, 99),
			)

			// BEHAVIOR: Leading zeros are normalized during parsing
			// CORRECTNESS: Numeric value is correct, String() returns normalized form
			DescribeTable("should normalize leading zeros in version components",
				func(versionStrWithZeros string, expectedNormalized string, expectedMajor, expectedMinor, expectedPatch int) {
					// ACT: Parse version string with leading zeros
					version, err := schema.ParseSemanticVersion(versionStrWithZeros)

					// ASSERT: No error occurred
					Expect(err).ToNot(HaveOccurred(), "ParseSemanticVersion(%q) should not return error", versionStrWithZeros)

					// CORRECTNESS: Numeric values are correct (leading zeros stripped)
					Expect(version.Major).To(Equal(expectedMajor), "Major version should be %d", expectedMajor)
					Expect(version.Minor).To(Equal(expectedMinor), "Minor version should be %d", expectedMinor)
					Expect(version.Patch).To(Equal(expectedPatch), "Patch version should be %d", expectedPatch)

					// CORRECTNESS: String() returns normalized version (no leading zeros)
					Expect(version.String()).To(Equal(expectedNormalized),
						"String() should return normalized version %q for input %q", expectedNormalized, versionStrWithZeros)
				},
				Entry("leading zero in major", "01.2.3", "1.2.3", 1, 2, 3),
				Entry("leading zero in minor", "1.02.3", "1.2.3", 1, 2, 3),
				Entry("leading zero in patch", "1.2.03", "1.2.3", 1, 2, 3),
				Entry("multiple leading zeros in major", "001.2.3", "1.2.3", 1, 2, 3),
				Entry("multiple leading zeros in minor", "1.002.3", "1.2.3", 1, 2, 3),
				Entry("multiple leading zeros in patch", "1.2.003", "1.2.3", 1, 2, 3),
				Entry("all components with leading zeros", "01.02.03", "1.2.3", 1, 2, 3),
			)

			// BEHAVIOR: ParseSemanticVersion rejects invalid formats
			// CORRECTNESS: Validate specific error messages guide user to correct format
			DescribeTable("should return descriptive errors for invalid version strings",
				func(versionStr string, expectedError string, description string) {
					// ACT: Parse invalid version string
					version, err := schema.ParseSemanticVersion(versionStr)

					// ASSERT: Error occurred
					Expect(err).To(HaveOccurred(), "ParseSemanticVersion(%q) should return error: %s", versionStr, description)

					// CORRECTNESS: Error message contains expected substring
					Expect(err.Error()).To(ContainSubstring(expectedError),
						"Error message should contain %q for input %q", expectedError, versionStr)

					// ASSERT: No version object returned on error
					Expect(version).To(BeNil(), "ParseSemanticVersion(%q) should return nil version on error", versionStr)
				},
				Entry("empty string", "", "empty version string", "empty string not allowed"),
				Entry("invalid format: no dots", "invalid", "invalid version format", "must have X.Y.Z format"),
				Entry("invalid format: missing patch", "1.2", "invalid version format", "must have all three components"),
				Entry("invalid format: non-numeric major", "a.b.c", "invalid version format", "components must be numeric"),
				Entry("invalid format: too many components", "1.2.3.4", "invalid version format", "only X.Y.Z supported"),
				Entry("edge case: negative numbers", "-1.2.3", "invalid version format", "negative versions not allowed"),
				Entry("edge case: whitespace", " 1.2.3 ", "invalid version format", "whitespace not trimmed"),
			)
		})

		Context("comparing versions", func() {
			// BEHAVIOR: Version comparison uses major → minor → patch precedence
			// CORRECTNESS: Validate mathematical correctness of comparison operations
			DescribeTable("should correctly compare versions with proper precedence",
				func(v1Str, v2Str string, expectedComparison string, description string) {
					// ARRANGE: Parse both versions
					v1, err := schema.ParseSemanticVersion(v1Str)
					Expect(err).ToNot(HaveOccurred(), "Failed to parse v1: %s", v1Str)
					v2, err := schema.ParseSemanticVersion(v2Str)
					Expect(err).ToNot(HaveOccurred(), "Failed to parse v2: %s", v2Str)

					// ACT & ASSERT: Validate comparison behavior
					switch expectedComparison {
					case "less":
						// CORRECTNESS: v1 < v2 means IsLessThan returns true
						Expect(v1.IsLessThan(v2)).To(BeTrue(),
							"%s should be less than %s: %s", v1Str, v2Str, description)

						// CORRECTNESS: v1 < v2 means IsGreaterThanOrEqual returns false
						Expect(v1.IsGreaterThanOrEqual(v2)).To(BeFalse(),
							"%s is less than %s, so should NOT be >=: %s", v1Str, v2Str, description)

						// CORRECTNESS: Validate inverse relationship (v2 > v1)
						Expect(v2.IsLessThan(v1)).To(BeFalse(),
							"%s is greater than %s, so should NOT be <: %s", v2Str, v1Str, description)
						Expect(v2.IsGreaterThanOrEqual(v1)).To(BeTrue(),
							"%s is greater than %s, so should be >=: %s", v2Str, v1Str, description)

					case "equal":
						// CORRECTNESS: v1 == v2 means neither is less than the other
						Expect(v1.IsLessThan(v2)).To(BeFalse(),
							"%s equals %s, so should NOT be <: %s", v1Str, v2Str, description)
						Expect(v2.IsLessThan(v1)).To(BeFalse(),
							"%s equals %s, so should NOT be <: %s", v2Str, v1Str, description)

						// CORRECTNESS: v1 == v2 means both are >= each other
						Expect(v1.IsGreaterThanOrEqual(v2)).To(BeTrue(),
							"%s equals %s, so should be >=: %s", v1Str, v2Str, description)
						Expect(v2.IsGreaterThanOrEqual(v1)).To(BeTrue(),
							"%s equals %s, so should be >=: %s", v2Str, v1Str, description)

					case "greater":
						// CORRECTNESS: v1 > v2 means IsGreaterThanOrEqual returns true
						Expect(v1.IsGreaterThanOrEqual(v2)).To(BeTrue(),
							"%s should be greater than %s: %s", v1Str, v2Str, description)

						// CORRECTNESS: v1 > v2 means IsLessThan returns false
						Expect(v1.IsLessThan(v2)).To(BeFalse(),
							"%s is greater than %s, so should NOT be <: %s", v1Str, v2Str, description)

						// CORRECTNESS: Validate inverse relationship (v2 < v1)
						Expect(v2.IsLessThan(v1)).To(BeTrue(),
							"%s is less than %s, so should be <: %s", v2Str, v1Str, description)
						Expect(v2.IsGreaterThanOrEqual(v1)).To(BeFalse(),
							"%s is less than %s, so should NOT be >=: %s", v2Str, v1Str, description)
					}
				},
				// Major version precedence (major difference overrides minor/patch)
				Entry("major precedence: 2.0.0 > 1.9.9", "2.0.0", "1.9.9", "greater", "major version takes precedence over minor/patch"),
				Entry("major precedence: 1.0.0 < 2.0.0", "1.0.0", "2.0.0", "less", "lower major is always less"),
				Entry("major precedence: 10.0.0 > 9.9.9", "10.0.0", "9.9.9", "greater", "double digit major comparison"),

				// Minor version precedence (when major equal, minor decides)
				Entry("minor precedence: 0.6.0 > 0.5.9", "0.6.0", "0.5.9", "greater", "minor version precedence when major equal"),
				Entry("minor precedence: 0.5.1 < 0.6.0", "0.5.1", "0.6.0", "less", "lower minor is less when major equal"),
				Entry("minor precedence: 1.10.0 > 1.9.0", "1.10.0", "1.9.0", "greater", "double digit minor comparison"),

				// Patch version precedence (when major and minor equal, patch decides)
				Entry("patch precedence: 0.5.1 > 0.5.0", "0.5.1", "0.5.0", "greater", "patch decides when major/minor equal"),
				Entry("patch precedence: 0.5.0 < 0.5.1", "0.5.0", "0.5.1", "less", "lower patch is less when major/minor equal"),
				Entry("patch precedence: 1.2.10 > 1.2.9", "1.2.10", "1.2.9", "greater", "double digit patch comparison"),

				// Equality (all components equal)
				Entry("equality: 1.2.3 == 1.2.3", "1.2.3", "1.2.3", "equal", "identical versions are equal"),
				Entry("equality: 0.5.1 == 0.5.1", "0.5.1", "0.5.1", "equal", "pgvector minimum version equality"),
				Entry("equality: 16.1.0 == 16.1.0", "16.1.0", "16.1.0", "equal", "PostgreSQL minimum version equality"),

				// DD-011 specific validation scenarios
				Entry("DD-011: PostgreSQL 16.1.0 >= 16.0.0 minimum", "16.1.0", "16.0.0", "greater", "PostgreSQL 16.1 meets DD-011 requirement"),
				Entry("DD-011: PostgreSQL 15.5.0 < 16.0.0 minimum", "15.5.0", "16.0.0", "less", "PostgreSQL 15 fails DD-011 requirement"),
				Entry("DD-011: pgvector 0.5.1 >= 0.5.1 minimum", "0.5.1", "0.5.1", "equal", "pgvector 0.5.1 exactly meets DD-011 requirement"),
				Entry("DD-011: pgvector 0.6.0 > 0.5.1 minimum", "0.6.0", "0.5.1", "greater", "pgvector 0.6.0 exceeds DD-011 requirement"),
				Entry("DD-011: pgvector 0.5.0 < 0.5.1 minimum", "0.5.0", "0.5.1", "less", "pgvector 0.5.0 fails DD-011 requirement"),

				// Edge cases
				Entry("edge case: 0.0.0 < 0.0.1", "0.0.0", "0.0.1", "less", "zero version comparison"),
				Entry("edge case: 99.99.99 > 1.1.1", "99.99.99", "1.1.1", "greater", "large version numbers"),
			)
		})
	})

	// ========================================
	// TDD REFACTOR: Version Constants
	// BR-STORAGE-012: HNSW Version Requirements (DD-011)
	// TESTING PRINCIPLE: Behavior + Correctness (Implementation Plan V4.9)
	// ========================================
	Describe("Version Constants (DD-011)", func() {
		Context("PostgreSQL version requirements", func() {
			It("should define MinPostgreSQLMajorVersion as 16 per DD-011", func() {
				// CORRECTNESS: DD-011 requires PostgreSQL 16+
				Expect(schema.MinPostgreSQLMajorVersion).To(Equal(16),
					"DD-011 mandates PostgreSQL 16.x+ for stable HNSW support")

				// CORRECTNESS: Validate it's a positive integer
				Expect(schema.MinPostgreSQLMajorVersion).To(BeNumerically(">", 0),
					"PostgreSQL major version must be positive")

				// CORRECTNESS: Validate it's reasonable (not absurdly high)
				Expect(schema.MinPostgreSQLMajorVersion).To(BeNumerically("<", 100),
					"PostgreSQL major version should be realistic")
			})
		})

		Context("pgvector version requirements", func() {
			It("should define MinPgvectorVersion as 0.5.1 per DD-011", func() {
				// ASSERT: Version object exists
				Expect(schema.MinPgvectorVersion).ToNot(BeNil(),
					"MinPgvectorVersion must be defined for DD-011 validation")

				// CORRECTNESS: DD-011 requires pgvector 0.5.1+
				Expect(schema.MinPgvectorVersion.Major).To(Equal(0),
					"DD-011 mandates pgvector major version 0")
				Expect(schema.MinPgvectorVersion.Minor).To(Equal(5),
					"DD-011 mandates pgvector minor version 5")
				Expect(schema.MinPgvectorVersion.Patch).To(Equal(1),
					"DD-011 mandates pgvector patch version 1 (0.5.1+ required)")

				// CORRECTNESS: Validate String() representation
				Expect(schema.MinPgvectorVersion.String()).To(Equal("0.5.1"),
					"MinPgvectorVersion should stringify to 0.5.1")
			})

			It("should validate that pgvector 0.5.0 fails DD-011 requirement", func() {
				// ARRANGE: Create pgvector 0.5.0 (below minimum)
				unsupportedVersion, err := schema.ParseSemanticVersion("0.5.0")
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: 0.5.0 < 0.5.1 (fails DD-011)
				Expect(unsupportedVersion.IsLessThan(schema.MinPgvectorVersion)).To(BeTrue(),
					"pgvector 0.5.0 should be less than DD-011 minimum 0.5.1")
			})

			It("should validate that pgvector 0.5.1 meets DD-011 requirement", func() {
				// ARRANGE: Create pgvector 0.5.1 (exactly minimum)
				minimumVersion, err := schema.ParseSemanticVersion("0.5.1")
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: 0.5.1 >= 0.5.1 (meets DD-011)
				Expect(minimumVersion.IsGreaterThanOrEqual(schema.MinPgvectorVersion)).To(BeTrue(),
					"pgvector 0.5.1 should meet DD-011 minimum requirement")
			})

			It("should validate that pgvector 0.6.0 exceeds DD-011 requirement", func() {
				// ARRANGE: Create pgvector 0.6.0 (above minimum)
				newerVersion, err := schema.ParseSemanticVersion("0.6.0")
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: 0.6.0 >= 0.5.1 (exceeds DD-011)
				Expect(newerVersion.IsGreaterThanOrEqual(schema.MinPgvectorVersion)).To(BeTrue(),
					"pgvector 0.6.0 should exceed DD-011 minimum 0.5.1")
			})
		})

		Context("memory configuration", func() {
			It("should define RecommendedSharedBuffersBytes as 1GB per DD-011", func() {
				// CORRECTNESS: DD-011 recommends 1GB shared_buffers
				const oneGB = int64(1024 * 1024 * 1024)
				Expect(schema.RecommendedSharedBuffersBytes).To(Equal(oneGB),
					"DD-011 recommends 1GB shared_buffers for optimal HNSW performance")

				// CORRECTNESS: Validate it's a positive value
				Expect(schema.RecommendedSharedBuffersBytes).To(BeNumerically(">", 0),
					"Recommended buffer size must be positive")

				// CORRECTNESS: Validate it's reasonable (between 512MB and 100GB)
				const minReasonable = int64(512 * 1024 * 1024)  // 512MB
				const maxReasonable = int64(100 * 1024 * 1024 * 1024) // 100GB
				Expect(schema.RecommendedSharedBuffersBytes).To(BeNumerically(">=", minReasonable),
					"Recommended buffer size should be at least 512MB")
				Expect(schema.RecommendedSharedBuffersBytes).To(BeNumerically("<=", maxReasonable),
					"Recommended buffer size should not exceed 100GB")
			})
		})

		Context("HNSW index parameters", func() {
			It("should define DefaultHNSWM as 16 per DD-011", func() {
				// CORRECTNESS: DD-011 specifies m=16 for good recall/build time balance
				Expect(schema.DefaultHNSWM).To(Equal(16),
					"DD-011 specifies HNSW m=16 for balanced recall and build time")

				// CORRECTNESS: Validate it's a positive integer
				Expect(schema.DefaultHNSWM).To(BeNumerically(">", 0),
					"HNSW m parameter must be positive")

				// CORRECTNESS: Validate it's reasonable (typically 4-64)
				Expect(schema.DefaultHNSWM).To(BeNumerically(">=", 4),
					"HNSW m parameter should be at least 4")
				Expect(schema.DefaultHNSWM).To(BeNumerically("<=", 64),
					"HNSW m parameter should not exceed 64")
			})

			It("should define DefaultHNSWEfConstruction as 64 per DD-011", func() {
				// CORRECTNESS: DD-011 specifies ef_construction=64 for good recall
				Expect(schema.DefaultHNSWEfConstruction).To(Equal(64),
					"DD-011 specifies HNSW ef_construction=64 for good recall")

				// CORRECTNESS: Validate it's a positive integer
				Expect(schema.DefaultHNSWEfConstruction).To(BeNumerically(">", 0),
					"HNSW ef_construction parameter must be positive")

				// CORRECTNESS: Validate it's reasonable (typically 10-200)
				Expect(schema.DefaultHNSWEfConstruction).To(BeNumerically(">=", 10),
					"HNSW ef_construction parameter should be at least 10")
				Expect(schema.DefaultHNSWEfConstruction).To(BeNumerically("<=", 200),
					"HNSW ef_construction parameter should not exceed 200")
			})

			It("should validate HNSW parameters relationship (ef_construction >= m)", func() {
				// CORRECTNESS: Best practice is ef_construction >= m for optimal build
				Expect(schema.DefaultHNSWEfConstruction).To(BeNumerically(">=", schema.DefaultHNSWM),
					"HNSW ef_construction should be >= m for optimal index building")
			})
		})
	})
})
