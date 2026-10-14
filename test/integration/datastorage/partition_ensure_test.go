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

package datastorage

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/partition"
)

// BR-AUDIT-029: Automatic partition management for audit storage
// Tests use far-future dates (2029+) to avoid overlap with static partitions
// from 001_v1_schema.sql (which covers 2026-03 through 2028-12).
var _ = Describe("Partition Ensure — Integration Tests", Ordered, ContinueOnFailure, func() {
	var (
		partitionDB  *sql.DB
		databaseName string
	)

	BeforeAll(func() {
		// BR-AUDIT-029: real partition DDL must not lock the audit parent
		// used by parallel HTTP/audit tests. Keep the same PostgreSQL instance.
		var sharedDatabase string
		Expect(db.QueryRowContext(ctx, "SELECT current_database()").Scan(&sharedDatabase)).To(Succeed())
		databaseName = fmt.Sprintf("partition_test_p%d_%s", GinkgoParallelProcess(), strings.ReplaceAll(uuid.NewString(), "-", ""))
		databaseIdentifier := pgx.Identifier{databaseName}.Sanitize()
		_, err := db.ExecContext(ctx, "CREATE DATABASE "+databaseIdentifier)
		Expect(err).NotTo(HaveOccurred(), "the test PostgreSQL role must create its own fixture database")

		DeferCleanup(func() {
			if partitionDB != nil {
				Expect(partitionDB.Close()).To(Succeed())
			}
			if os.Getenv("KEEP_CONTAINERS_ON_FAILURE") != "" {
				GinkgoWriter.Printf("Retaining test-owned database %s for verification\n", databaseName)
				return
			}
			// Drop only the unique database whose creation succeeded above.
			// Never force-close sessions or alter the shared database.
			_, cleanupErr := db.ExecContext(ctx, "DROP DATABASE "+databaseIdentifier)
			Expect(cleanupErr).NotTo(HaveOccurred())
		})

		partitionDB = mustConnectPostgreSQLDatabase(databaseName).DB
		var actualDatabase string
		Expect(partitionDB.QueryRowContext(ctx, "SELECT current_database()").Scan(&actualDatabase)).To(Succeed())
		Expect(actualDatabase).To(Equal(databaseName))
		Expect(actualDatabase).NotTo(Equal(sharedDatabase), "partition DDL must not touch the shared audit database")
		// Confirm ownership before the real helper initializes the schema.
		applyMigrationsWithPropagationTo(partitionDB)
		GinkgoWriter.Printf("Partition fixtures isolated in %s; shared database %s unchanged\n", actualDatabase, sharedDatabase)
	})

	// Far-future instant: no static partitions exist for 2029
	futureNow := time.Date(2029, time.March, 15, 10, 0, 0, 0, time.UTC)

	// IT-DS-235-001 (P0): Startup ensure creates partitions for both tables
	// BR-AUDIT-029: Automatic partition management
	Describe("IT-DS-235-001: Startup ensure creates partitions for both tables", func() {
		It("should create partitions for current month + 3 months for audit_events", func() {
			err := partition.EnsureMonthlyPartitions(ctx, partitionDB, futureNow, partition.DefaultLookaheadMonths, partition.AllTables())
			Expect(err).NotTo(HaveOccurred())

			// Verify audit_events partitions exist: 2029-03 through 2029-06
			for monthOffset := 0; monthOffset <= 3; monthOffset++ {
				m := futureNow.AddDate(0, monthOffset, 0)
				name := partition.FormatPartitionName("audit_events", m.Year(), m.Month())
				Expect(partitionExists(partitionDB, name)).To(BeTrue(),
					fmt.Sprintf("expected partition %s to exist after EnsureMonthlyPartitions", name))
				Expect(partitionExists(db.DB, name)).To(BeFalse(), "private test partitions must not be created in the shared database")
			}
		})
	})

	// IT-DS-235-002: Double ensure = no error, no duplicate children
	// BR-AUDIT-029: Automatic partition management
	Describe("IT-DS-235-002: Idempotency — second startup", func() {
		It("should not error when called twice for the same time window", func() {
			err := partition.EnsureMonthlyPartitions(ctx, partitionDB, futureNow, partition.DefaultLookaheadMonths, partition.AllTables())
			Expect(err).NotTo(HaveOccurred())

			// Second call: must not error
			err = partition.EnsureMonthlyPartitions(ctx, partitionDB, futureNow, partition.DefaultLookaheadMonths, partition.AllTables())
			Expect(err).NotTo(HaveOccurred())

			// Verify no duplicate children in pg_inherits for audit_events
			for monthOffset := 0; monthOffset <= 3; monthOffset++ {
				m := futureNow.AddDate(0, monthOffset, 0)
				name := partition.FormatPartitionName("audit_events", m.Year(), m.Month())
				count := countInherits(partitionDB, name)
				Expect(count).To(Equal(1),
					fmt.Sprintf("partition %s should appear exactly once in pg_inherits, got %d", name, count))
			}
		})
	})

	// IT-DS-235-003: Boundary insert lands in named partition
	// BR-AUDIT-029: Automatic partition management
	Describe("IT-DS-235-003: Boundary insert into newly created partition", func() {
		It("should route an audit_events insert to the correct named partition", func() {
			err := partition.EnsureMonthlyPartitions(ctx, partitionDB, futureNow, partition.DefaultLookaheadMonths, partition.AllTables())
			Expect(err).NotTo(HaveOccurred())

			// Insert an audit event with event_date in 2029-03 (first month of window)
			eventDate := time.Date(2029, time.March, 15, 0, 0, 0, 0, time.UTC)
			insertQuery := `
				INSERT INTO audit_events (
					event_id, event_date, event_timestamp, event_type, event_category,
					event_action, event_outcome, actor_type, actor_id, resource_type,
					resource_id, correlation_id, event_data
				) VALUES (
					gen_random_uuid(), $1, $2, 'test.partition', 'test', 'insert',
					'success', 'system', 'test-actor', 'test-resource', 'test-id',
					'test-corr-235-003', '{}'::jsonb
				)`
			_, err = partitionDB.ExecContext(ctx, insertQuery, eventDate, eventDate)
			Expect(err).NotTo(HaveOccurred(), "insert into 2029-03 partition should succeed")

			// Verify row landed in the named partition (not _default)
			targetPartition := partition.FormatPartitionName("audit_events", 2029, time.March)
			var count int
			err = partitionDB.QueryRowContext(ctx,
				fmt.Sprintf("SELECT count(*) FROM %s WHERE correlation_id = 'test-corr-235-003'", targetPartition),
			).Scan(&count)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1), "row should exist in named partition, not _default")
		})

		It("should fail INSERT when no partition exists for the target date (fail-loud strategy)", func() {
			// 2099-01: no partition exists, no _default after migration 006
			farFutureDate := time.Date(2099, time.January, 15, 0, 0, 0, 0, time.UTC)
			insertQuery := `
				INSERT INTO audit_events (
					event_id, event_date, event_timestamp, event_type, event_category,
					event_action, event_outcome, actor_type, actor_id, resource_type,
					resource_id, correlation_id, event_data
				) VALUES (
					gen_random_uuid(), $1, $2, 'test.nop', 'test', 'insert',
					'success', 'system', 'test-actor', 'test-resource', 'test-id',
					'test-corr-235-003-nop', '{}'::jsonb
				)`
			_, err := partitionDB.ExecContext(ctx, insertQuery, farFutureDate, farFutureDate)
			Expect(err).To(HaveOccurred(), "insert should fail when no partition matches and _default is absent")
		})
	})

	// IT-DS-235-004 (P0): pg_inherits confirms children for both parents
	// BR-AUDIT-029: Automatic partition management
	Describe("IT-DS-235-004: pg_inherits catalog verification for both tables", func() {
		It("should list monthly children in pg_inherits for audit_events", func() {
			err := partition.EnsureMonthlyPartitions(ctx, partitionDB, futureNow, partition.DefaultLookaheadMonths, partition.AllTables())
			Expect(err).NotTo(HaveOccurred())

			// Check audit_events children via pg_inherits
			auditChildren := listChildPartitions(partitionDB, "audit_events")
			for monthOffset := 0; monthOffset <= 3; monthOffset++ {
				m := futureNow.AddDate(0, monthOffset, 0)
				expectedName := partition.FormatPartitionName("audit_events", m.Year(), m.Month())
				Expect(auditChildren).To(ContainElement(expectedName),
					fmt.Sprintf("pg_inherits should list %s as child of audit_events", expectedName))
			}
		})
	})

	// IT-DS-235-005 (P2): Two sequential connections ensure without error
	// BR-AUDIT-029: Automatic partition management
	Describe("IT-DS-235-005: Concurrent-safe sequential ensure from different connections", func() {
		It("should not error when two sequential connections call ensure for the same window", func() {
			// First connection: use this group's private database.
			err := partition.EnsureMonthlyPartitions(ctx, partitionDB, futureNow, partition.DefaultLookaheadMonths, partition.AllTables())
			Expect(err).NotTo(HaveOccurred())

			// Second connection: open a new DB handle (reuse the suite helper)
			secondDB := mustConnectPostgreSQLDatabase(databaseName)
			defer func() { Expect(secondDB.Close()).To(Succeed()) }()
			var secondDatabase string
			Expect(secondDB.QueryRowContext(ctx, "SELECT current_database()").Scan(&secondDatabase)).To(Succeed())
			Expect(secondDatabase).To(Equal(databaseName), "both connections must exercise the same private database")

			err = partition.EnsureMonthlyPartitions(ctx, secondDB, futureNow, partition.DefaultLookaheadMonths, partition.AllTables())
			Expect(err).NotTo(HaveOccurred())

			// Verify: no duplicates
			name := partition.FormatPartitionName("audit_events", 2029, time.March)
			count := countInherits(partitionDB, name)
			Expect(count).To(Equal(1), "no duplicate inherits after sequential ensure from two connections")
		})
	})

	// Cleanup only this group's private far-future partitions and rows.
	AfterAll(func() {
		if partitionDB == nil {
			return // BeforeAll already reports any fixture creation/connection failure.
		}
		for monthOffset := 0; monthOffset <= 3; monthOffset++ {
			m := futureNow.AddDate(0, monthOffset, 0)
			for _, parent := range []string{"audit_events"} {
				name := partition.FormatPartitionName(parent, m.Year(), m.Month())
				//nolint:gosec
				_, err := partitionDB.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", name))
				Expect(err).NotTo(HaveOccurred())
			}
		}
		// Clean up any test rows
		_, err := partitionDB.ExecContext(ctx, "DELETE FROM audit_events WHERE correlation_id LIKE 'test-corr-235-%'")
		Expect(err).NotTo(HaveOccurred())
	})
})

// partitionExists checks if a partition table exists in pg_class.
func partitionExists(database *sql.DB, name string) bool {
	var exists bool
	err := database.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1)", name).Scan(&exists)
	Expect(err).NotTo(HaveOccurred())
	return exists
}

// countInherits returns how many times a child table appears in pg_inherits.
func countInherits(database *sql.DB, childName string) int {
	var count int
	err := database.QueryRowContext(ctx, `
		SELECT count(*)
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		WHERE c.relname = $1
	`, childName).Scan(&count)
	Expect(err).NotTo(HaveOccurred())
	return count
}

// listChildPartitions returns the names of all child partitions for a given parent table.
func listChildPartitions(database *sql.DB, parentName string) []string {
	rows, err := database.QueryContext(ctx, `
		SELECT c.relname
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname = $1
		ORDER BY c.relname
	`, parentName)
	Expect(err).NotTo(HaveOccurred())
	defer func() { Expect(rows.Close()).To(Succeed()) }()

	var children []string
	for rows.Next() {
		var name string
		Expect(rows.Scan(&name)).To(Succeed())
		children = append(children, name)
	}
	Expect(rows.Err()).NotTo(HaveOccurred())
	return children
}
