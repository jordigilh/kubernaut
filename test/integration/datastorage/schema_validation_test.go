package datastorage

import (
	"database/sql"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ColumnMetadata represents database column metadata for schema validation
// Replaces map[string]interface{} for type safety (IMPLEMENTATION_PLAN_V4.9 #21)
type ColumnMetadata struct {
	DataType   string
	IsNullable string
	MaxLength  sql.NullInt64
}

// BR-STORAGE-003: Database Schema Validation (Integration Tests)
// These tests use the shared PostgreSQL infrastructure from suite_test.go
// to validate the database schema created by migration files.
//
// Moved from test/unit/datastorage/ because schema validation requires
// real database connectivity (integration test by definition).

var _ = Describe("BR-STORAGE-003: Notification Audit Table Schema", Ordered, func() {
	// Use shared 'db' from suite_test.go (PostgreSQL Podman container)

	Context("Table Existence", func() {
		It("should have notification_audit table", func() {
			var exists bool
			query := `
				SELECT EXISTS (
					SELECT FROM pg_tables
					WHERE schemaname = 'public'
					AND tablename = 'notification_audit'
				)
			`
			err := db.QueryRow(query).Scan(&exists)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue(), "notification_audit table should exist")
		})
	})

	Context("Table Columns", func() {
		It("should have all required columns with correct types", func() {
			query := `
				SELECT column_name, data_type, is_nullable, character_maximum_length
				FROM information_schema.columns
				WHERE table_schema = 'public'
				AND table_name = 'notification_audit'
				ORDER BY ordinal_position
			`
			rows, err := db.Query(query)
		Expect(err).ToNot(HaveOccurred())
		defer rows.Close()

		columns := make(map[string]ColumnMetadata)
		for rows.Next() {
			var colName, dataType, isNullable string
			var maxLength sql.NullInt64
			err := rows.Scan(&colName, &dataType, &isNullable, &maxLength)
			Expect(err).ToNot(HaveOccurred())

			columns[colName] = ColumnMetadata{
				DataType:   dataType,
				IsNullable: isNullable,
				MaxLength:  maxLength,
			}
		}

			// Verify required columns exist with correct types
			expectedColumns := map[string]struct {
				dataType   string
				isNullable string
				maxLength  int64
			}{
				"id":                {"bigint", "NO", 0},
				"remediation_id":    {"character varying", "NO", 255},
				"notification_id":   {"character varying", "NO", 255},
				"recipient":         {"character varying", "NO", 255},
				"channel":           {"character varying", "NO", 50},
				"message_summary":   {"text", "NO", 0},
				"status":            {"character varying", "NO", 50},
				"sent_at":           {"timestamp with time zone", "NO", 0},
				"delivery_status":   {"text", "YES", 0},
				"error_message":     {"text", "YES", 0},
				"escalation_level":  {"integer", "YES", 0},
				"created_at":        {"timestamp with time zone", "YES", 0},
				"updated_at":        {"timestamp with time zone", "YES", 0},
			}

		for colName, expected := range expectedColumns {
			col, exists := columns[colName]
			Expect(exists).To(BeTrue(), fmt.Sprintf("Column %s should exist", colName))

			Expect(col.DataType).To(Equal(expected.dataType),
				fmt.Sprintf("Column %s should have type %s", colName, expected.dataType))

			Expect(col.IsNullable).To(Equal(expected.isNullable),
				fmt.Sprintf("Column %s nullable should be %s", colName, expected.isNullable))

			if expected.maxLength > 0 {
				Expect(col.MaxLength.Valid).To(BeTrue())
				Expect(col.MaxLength.Int64).To(Equal(expected.maxLength),
					fmt.Sprintf("Column %s should have max length %d", colName, expected.maxLength))
			}
		}
		})
	})

	Context("Table Constraints", func() {
		It("should have primary key on id column", func() {
			query := `
				SELECT constraint_name, constraint_type
				FROM information_schema.table_constraints
				WHERE table_schema = 'public'
				AND table_name = 'notification_audit'
				AND constraint_type = 'PRIMARY KEY'
			`
			var constraintName, constraintType string
			err := db.QueryRow(query).Scan(&constraintName, &constraintType)
			Expect(err).ToNot(HaveOccurred())
			Expect(constraintType).To(Equal("PRIMARY KEY"))
		})

		It("should have unique constraint on notification_id", func() {
			query := `
				SELECT constraint_name
				FROM information_schema.table_constraints
				WHERE table_schema = 'public'
				AND table_name = 'notification_audit'
				AND constraint_type = 'UNIQUE'
			`
			rows, err := db.Query(query)
			Expect(err).ToNot(HaveOccurred())
			defer rows.Close()

			hasUniqueConstraint := false
			for rows.Next() {
				var constraintName string
				err := rows.Scan(&constraintName)
				Expect(err).ToNot(HaveOccurred())
				if constraintName == "notification_audit_notification_id_key" {
					hasUniqueConstraint = true
					break
				}
			}
			Expect(hasUniqueConstraint).To(BeTrue(), "notification_id should have unique constraint")
		})

		It("should have check constraint on status column", func() {
			query := `
				SELECT constraint_name, check_clause
				FROM information_schema.check_constraints
				WHERE constraint_schema = 'public'
				AND constraint_name LIKE '%status%'
			`
			rows, err := db.Query(query)
			Expect(err).ToNot(HaveOccurred())
			defer rows.Close()

			hasStatusCheck := false
			for rows.Next() {
				var constraintName, checkClause string
				err := rows.Scan(&constraintName, &checkClause)
				Expect(err).ToNot(HaveOccurred())
				// Check if the constraint validates status enum values
				if constraintName != "" {
					hasStatusCheck = true
					break
				}
			}
			Expect(hasStatusCheck).To(BeTrue(), "status column should have check constraint")
		})
	})

	Context("Table Indexes", func() {
		It("should have index on remediation_id", func() {
			query := `
				SELECT indexname
				FROM pg_indexes
				WHERE schemaname = 'public'
				AND tablename = 'notification_audit'
				AND indexname = 'idx_notification_audit_remediation_id'
			`
			var indexName string
			err := db.QueryRow(query).Scan(&indexName)
			Expect(err).ToNot(HaveOccurred())
			Expect(indexName).To(Equal("idx_notification_audit_remediation_id"))
		})

		It("should have index on recipient", func() {
			query := `
				SELECT indexname
				FROM pg_indexes
				WHERE schemaname = 'public'
				AND tablename = 'notification_audit'
				AND indexname = 'idx_notification_audit_recipient'
			`
			var indexName string
			err := db.QueryRow(query).Scan(&indexName)
			Expect(err).ToNot(HaveOccurred())
			Expect(indexName).To(Equal("idx_notification_audit_recipient"))
		})

		It("should have index on channel", func() {
			query := `
				SELECT indexname
				FROM pg_indexes
				WHERE schemaname = 'public'
				AND tablename = 'notification_audit'
				AND indexname = 'idx_notification_audit_channel'
			`
			var indexName string
			err := db.QueryRow(query).Scan(&indexName)
			Expect(err).ToNot(HaveOccurred())
			Expect(indexName).To(Equal("idx_notification_audit_channel"))
		})

		It("should have index on status", func() {
			query := `
				SELECT indexname
				FROM pg_indexes
				WHERE schemaname = 'public'
				AND tablename = 'notification_audit'
				AND indexname = 'idx_notification_audit_status'
			`
			var indexName string
			err := db.QueryRow(query).Scan(&indexName)
			Expect(err).ToNot(HaveOccurred())
			Expect(indexName).To(Equal("idx_notification_audit_status"))
		})

		It("should have index on sent_at", func() {
			query := `
				SELECT indexname
				FROM pg_indexes
				WHERE schemaname = 'public'
				AND tablename = 'notification_audit'
				AND indexname = 'idx_notification_audit_sent_at'
			`
			var indexName string
			err := db.QueryRow(query).Scan(&indexName)
			Expect(err).ToNot(HaveOccurred())
			Expect(indexName).To(Equal("idx_notification_audit_sent_at"))
		})
	})

	Context("Table Triggers", func() {
		It("should have updated_at trigger", func() {
			query := `
				SELECT trigger_name
				FROM information_schema.triggers
				WHERE event_object_schema = 'public'
				AND event_object_table = 'notification_audit'
				AND trigger_name = 'trigger_notification_audit_updated_at'
			`
			var triggerName string
			err := db.QueryRow(query).Scan(&triggerName)
			Expect(err).ToNot(HaveOccurred())
			Expect(triggerName).To(Equal("trigger_notification_audit_updated_at"))
		})
	})

	Context("pgvector Extension", func() {
		It("should have pgvector extension installed", func() {
			query := `
				SELECT extname
				FROM pg_extension
				WHERE extname = 'vector'
			`
			var extName string
			err := db.QueryRow(query).Scan(&extName)
			Expect(err).ToNot(HaveOccurred())
			Expect(extName).To(Equal("vector"))
		})
	})
})

