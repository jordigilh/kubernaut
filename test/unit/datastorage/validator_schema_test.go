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
// func TestSchemaValidator(t *testing.T) {
// 	RegisterFailHandler(Fail)
// 	RunSpecs(t, "Schema Validator Suite")
// }

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
		Context("with PostgreSQL 16+ and pgvector 0.5.1+", func() {
			It("should pass validation for PostgreSQL 16.0", func() {
				// Mock PostgreSQL version query
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow("PostgreSQL 16.0 on x86_64-pc-linux-gnu, compiled by gcc"))

				// Mock pgvector version query
				mock.ExpectQuery("SELECT extversion FROM pg_extension WHERE extname = 'vector'").
					WillReturnRows(sqlmock.NewRows([]string{"extversion"}).
						AddRow("0.5.1"))

				// Mock HNSW test table creation (with IF NOT EXISTS)
				mock.ExpectExec("CREATE TEMP TABLE IF NOT EXISTS hnsw_validation_test").
					WillReturnResult(sqlmock.NewResult(0, 0))

				// Mock HNSW index creation (with IF NOT EXISTS)
				mock.ExpectExec("CREATE INDEX IF NOT EXISTS hnsw_validation_test_idx").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})

			It("should pass validation for PostgreSQL 16.2", func() {
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow("PostgreSQL 16.2 on aarch64-apple-darwin23.0.0"))

				mock.ExpectQuery("SELECT extversion FROM pg_extension WHERE extname = 'vector'").
					WillReturnRows(sqlmock.NewRows([]string{"extversion"}).
						AddRow("0.6.0"))

				mock.ExpectExec("CREATE TEMP TABLE IF NOT EXISTS hnsw_validation_test").
					WillReturnResult(sqlmock.NewResult(0, 0))

				mock.ExpectExec("CREATE INDEX IF NOT EXISTS hnsw_validation_test_idx").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})

			It("should pass validation for PostgreSQL 17+ (future-proof)", func() {
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow("PostgreSQL 17.0 on x86_64-pc-linux-gnu"))

				mock.ExpectQuery("SELECT extversion FROM pg_extension WHERE extname = 'vector'").
					WillReturnRows(sqlmock.NewRows([]string{"extversion"}).
						AddRow("0.7.0"))

				mock.ExpectExec("CREATE TEMP TABLE IF NOT EXISTS hnsw_validation_test").
					WillReturnResult(sqlmock.NewResult(0, 0))

				mock.ExpectExec("CREATE INDEX IF NOT EXISTS hnsw_validation_test_idx").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})
		})

		Context("with unsupported PostgreSQL versions", func() {
			It("should fail validation for PostgreSQL 15", func() {
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow("PostgreSQL 15.4 on x86_64-pc-linux-gnu"))

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("PostgreSQL version 15 is not supported"))
				Expect(err.Error()).To(ContainSubstring("Required: PostgreSQL 16.x or higher"))
			})

			It("should fail validation for PostgreSQL 14", func() {
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow("PostgreSQL 14.9 on x86_64-pc-linux-gnu"))

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("PostgreSQL version 14 is not supported"))
			})

			It("should fail validation for PostgreSQL 13", func() {
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow("PostgreSQL 13.12 on x86_64-pc-linux-gnu"))

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("PostgreSQL version 13 is not supported"))
			})

			It("should fail validation for PostgreSQL 12", func() {
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow("PostgreSQL 12.16 on x86_64-pc-linux-gnu"))

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("PostgreSQL version 12 is not supported"))
			})
		})

		Context("with unsupported pgvector versions", func() {
			It("should fail validation for pgvector 0.5.0", func() {
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow("PostgreSQL 16.1 on x86_64-pc-linux-gnu"))

				mock.ExpectQuery("SELECT extversion FROM pg_extension WHERE extname = 'vector'").
					WillReturnRows(sqlmock.NewRows([]string{"extversion"}).
						AddRow("0.5.0"))

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pgvector version 0.5.0 is not supported"))
				Expect(err.Error()).To(ContainSubstring("Required: 0.5.1 or higher"))
			})

			It("should fail validation for pgvector 0.4.0", func() {
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow("PostgreSQL 16.1 on x86_64-pc-linux-gnu"))

				mock.ExpectQuery("SELECT extversion FROM pg_extension WHERE extname = 'vector'").
					WillReturnRows(sqlmock.NewRows([]string{"extversion"}).
						AddRow("0.4.0"))

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pgvector version 0.4.0 is not supported"))
			})

			It("should fail validation for pgvector 0.3.0", func() {
				mock.ExpectQuery("SELECT version\\(\\)").
					WillReturnRows(sqlmock.NewRows([]string{"version"}).
						AddRow("PostgreSQL 16.1 on x86_64-pc-linux-gnu"))

				mock.ExpectQuery("SELECT extversion FROM pg_extension WHERE extname = 'vector'").
					WillReturnRows(sqlmock.NewRows([]string{"extversion"}).
						AddRow("0.3.0"))

				err := validator.ValidateHNSWSupport(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pgvector version 0.3.0 is not supported"))
			})
		})

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
		Context("with sufficient memory", func() {
			It("should pass validation for 1GB shared_buffers", func() {
				mock.ExpectQuery("SELECT current_setting\\('shared_buffers'\\)").
					WillReturnRows(sqlmock.NewRows([]string{"current_setting"}).
						AddRow("1GB"))

				err := validator.ValidateMemoryConfiguration(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})

			It("should pass validation for 2GB shared_buffers", func() {
				mock.ExpectQuery("SELECT current_setting\\('shared_buffers'\\)").
					WillReturnRows(sqlmock.NewRows([]string{"current_setting"}).
						AddRow("2GB"))

				err := validator.ValidateMemoryConfiguration(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})
		})

		Context("with insufficient memory", func() {
			It("should warn but not fail for 512MB shared_buffers", func() {
				mock.ExpectQuery("SELECT current_setting\\('shared_buffers'\\)").
					WillReturnRows(sqlmock.NewRows([]string{"current_setting"}).
						AddRow("512MB"))

				// Should not return error, only warn
				err := validator.ValidateMemoryConfiguration(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})

			It("should warn but not fail for 128MB shared_buffers", func() {
				mock.ExpectQuery("SELECT current_setting\\('shared_buffers'\\)").
					WillReturnRows(sqlmock.NewRows([]string{"current_setting"}).
						AddRow("128MB"))

				// Should not return error, only warn
				err := validator.ValidateMemoryConfiguration(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).To(Succeed())
			})
		})

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
