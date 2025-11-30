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

var _ = Describe("BR-STORAGE-003: Notification Audit Table Schema", Serial, Ordered, func() {
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
				"id":               {"bigint", "NO", 0},
				"remediation_id":   {"character varying", "NO", 255},
				"notification_id":  {"character varying", "NO", 255},
				"recipient":        {"character varying", "NO", 255},
				"channel":          {"character varying", "NO", 50},
				"message_summary":  {"text", "NO", 0},
				"status":           {"character varying", "NO", 50},
				"sent_at":          {"timestamp with time zone", "NO", 0},
				"delivery_status":  {"text", "YES", 0},
				"error_message":    {"text", "YES", 0},
				"escalation_level": {"integer", "YES", 0},
				"created_at":       {"timestamp with time zone", "YES", 0},
				"updated_at":       {"timestamp with time zone", "YES", 0},
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

var _ = Describe("BR-STORAGE-003: Resource Action Traces Table Schema", Serial, Ordered, func() {
	// CRITICAL: This test prevents schema mismatches between test code and migrations
	// Issue #17: ADR-033 tests used 'status' instead of 'execution_status' for 6 days
	// without detection because tests were never run in CI until containerized workflow

	Context("Table Existence", func() {
		It("should have resource_action_traces table", func() {
			var exists bool
			query := `
				SELECT EXISTS (
					SELECT FROM pg_class c
					JOIN pg_namespace n ON n.oid = c.relnamespace
					WHERE n.nspname = 'public'
					AND c.relname = 'resource_action_traces'
					AND c.relkind IN ('r', 'p')  -- regular or partitioned table
				)
			`
			err := db.QueryRow(query).Scan(&exists)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue(), "resource_action_traces table should exist")
		})
	})

	Context("Critical Columns for Test Code", func() {
		// These are the columns commonly used in test INSERT statements
		// Validates that test code matches actual schema

		It("should have execution_status column (NOT 'status')", func() {
			query := `
				SELECT column_name, data_type
				FROM information_schema.columns
				WHERE table_schema = 'public'
				AND table_name = 'resource_action_traces'
				AND column_name = 'execution_status'
			`
			var colName, dataType string
			err := db.QueryRow(query).Scan(&colName, &dataType)
			Expect(err).ToNot(HaveOccurred(), "execution_status column must exist")
			Expect(colName).To(Equal("execution_status"))
			Expect(dataType).To(Equal("character varying"))
		})

		It("should NOT have a column named 'status'", func() {
			query := `
				SELECT COUNT(*)
				FROM information_schema.columns
				WHERE table_schema = 'public'
				AND table_name = 'resource_action_traces'
				AND column_name = 'status'
			`
			var count int
			err := db.QueryRow(query).Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(0), "column 'status' should NOT exist (use 'execution_status' instead)")
		})

		It("should have all columns used in ADR-033 test inserts", func() {
			// These are the columns used in repository_adr033_integration_test.go
			requiredColumns := []string{
				"action_history_id",
				"action_id",
				"action_type",
				"action_timestamp",
				"execution_status", // CRITICAL: Not 'status'
				"signal_name",      // Migration 011: renamed from alert_name
				"signal_severity",  // Migration 011: renamed from alert_severity
				"model_used",
				"model_confidence",
				"incident_type",
				"workflow_id",
				"workflow_version",
				"ai_selected_workflow",
				"ai_chained_workflows",
			}

			for _, colName := range requiredColumns {
				query := `
					SELECT column_name
					FROM information_schema.columns
					WHERE table_schema = 'public'
					AND table_name = 'resource_action_traces'
					AND column_name = $1
				`
				var foundCol string
				err := db.QueryRow(query, colName).Scan(&foundCol)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("Column '%s' must exist for test INSERT statements", colName))
				Expect(foundCol).To(Equal(colName))
			}
		})
	})

	Context("Table Columns - Complete Schema", func() {
		It("should have all required columns with correct types", func() {
			query := `
				SELECT column_name, data_type, is_nullable
				FROM information_schema.columns
				WHERE table_schema = 'public'
				AND table_name = 'resource_action_traces'
				ORDER BY ordinal_position
			`
			rows, err := db.Query(query)
			Expect(err).ToNot(HaveOccurred())
			defer rows.Close()

			columns := make(map[string]ColumnMetadata)
			for rows.Next() {
				var colName, dataType, isNullable string
				err := rows.Scan(&colName, &dataType, &isNullable)
				Expect(err).ToNot(HaveOccurred())

				columns[colName] = ColumnMetadata{
					DataType:   dataType,
					IsNullable: isNullable,
				}
			}

			// Verify critical columns exist with correct types
			// Note: resource_type, resource_name, resource_namespace are in resource_references table,
			// not in resource_action_traces. This table references them via action_history_id -> action_histories -> resource_references.
			expectedColumns := map[string]struct {
				dataType   string
				isNullable string
			}{
				"id":                              {"bigint", "NO"},
				"action_history_id":               {"bigint", "NO"},
				"action_id":                       {"character varying", "NO"},
				"action_type":                     {"character varying", "NO"},
				"action_timestamp":                {"timestamp with time zone", "NO"},
				"execution_status":                {"character varying", "YES"}, // CRITICAL: execution_status, not status
				"model_used":                      {"character varying", "NO"},
				"model_confidence":                {"numeric", "NO"},
				"incident_type":                   {"character varying", "YES"},
				"workflow_id":                     {"character varying", "YES"},
				"workflow_version":                {"character varying", "YES"},
				"ai_selected_workflow":            {"boolean", "YES"},
				"ai_chained_workflows":            {"boolean", "YES"},
				"effectiveness_score":             {"numeric", "YES"},
				"effectiveness_assessment_method": {"character varying", "YES"},
			}

			for colName, expected := range expectedColumns {
				col, exists := columns[colName]
				Expect(exists).To(BeTrue(), fmt.Sprintf("Column %s should exist", colName))

				Expect(col.DataType).To(Equal(expected.dataType),
					fmt.Sprintf("Column %s should have type %s", colName, expected.dataType))

				Expect(col.IsNullable).To(Equal(expected.isNullable),
					fmt.Sprintf("Column %s nullable should be %s", colName, expected.isNullable))
			}
		})
	})

	Context("Table Indexes", func() {
		It("should have index on execution_status", func() {
			query := `
				SELECT indexname
				FROM pg_indexes
				WHERE schemaname = 'public'
				AND tablename = 'resource_action_traces'
				AND indexname = 'idx_rat_execution_status'
			`
			var indexName string
			err := db.QueryRow(query).Scan(&indexName)
			Expect(err).ToNot(HaveOccurred())
			Expect(indexName).To(Equal("idx_rat_execution_status"))
		})
	})

	Context("Partitioning", func() {
		It("should be a partitioned table", func() {
			query := `
				SELECT relkind
				FROM pg_class c
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE n.nspname = 'public'
				AND c.relname = 'resource_action_traces'
			`
			var relkind string
			err := db.QueryRow(query).Scan(&relkind)
			Expect(err).ToNot(HaveOccurred())
			Expect(relkind).To(Equal("p"), "resource_action_traces should be a partitioned table")
		})

		It("should have at least one partition", func() {
			query := `
				SELECT COUNT(*)
				FROM pg_inherits i
				JOIN pg_class parent ON i.inhparent = parent.oid
				JOIN pg_namespace n ON n.oid = parent.relnamespace
				WHERE n.nspname = 'public'
				AND parent.relname = 'resource_action_traces'
			`
			var count int
			err := db.QueryRow(query).Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(BeNumerically(">", 0), "resource_action_traces should have at least one partition")
		})
	})
})
