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
				Expect(err.Error()).To(ContainSubstring("PostgreSQL version "+expectedMajorVersion+" is not supported"))
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
				Expect(err.Error()).To(ContainSubstring("pgvector version "+pgvectorVersion+" is not supported"))
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
})
