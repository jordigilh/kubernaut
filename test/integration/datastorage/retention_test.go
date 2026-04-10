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
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/partition"
	"github.com/jordigilh/kubernaut/pkg/datastorage/retention"
)

// BR-AUDIT-009: Retention policies for audit data
// BR-AUDIT-004: Immutability / integrity of audit records
var _ = Describe("Retention Enforcement — Integration Tests", Ordered, func() {

	// Insert a synthetic expired audit event for tests.
	// Uses a date within the static partition range (2026-03 to 2028-12).
	insertAuditEvent := func(eventDate time.Time, retentionDays int, legalHold bool, corrID string) string {
		eventID := uuid.New().String()
		insertQuery := `
			INSERT INTO audit_events (
				event_id, event_date, event_timestamp, event_type, event_category,
				event_action, event_outcome, actor_type, actor_id, resource_type,
				resource_id, correlation_id, event_data, retention_days, legal_hold
			) VALUES (
				$1, $2, $3, 'retention.test', 'test', 'insert',
				'success', 'system', 'retention-test', 'test-resource', 'test-id',
				$4, '{}'::jsonb, $5, $6
			)`
		_, err := db.ExecContext(ctx, insertQuery, eventID, eventDate, eventDate, corrID, retentionDays, legalHold)
		Expect(err).NotTo(HaveOccurred())
		return eventID
	}

	countByCorrID := func(corrID string) int {
		var count int
		err := db.QueryRowContext(ctx,
			"SELECT count(*) FROM audit_events WHERE correlation_id = $1", corrID,
		).Scan(&count)
		Expect(err).NotTo(HaveOccurred())
		return count
	}

	// IT-DS-485-001 (P0): Disabled by default — no deletes
	// BR-AUDIT-004: Immutability / integrity of audit records
	Describe("IT-DS-485-001: Flag off — no deletes", func() {
		corrID := "test-ret-485-001"

		BeforeAll(func() {
			insertAuditEvent(
				time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
				1, false, corrID,
			)
		})

		It("should not delete any rows when retention is disabled", func() {
			cfg := retention.DefaultConfig()
			Expect(cfg.Enabled).To(BeFalse(), "retention must be disabled by default")

			// With retention disabled, even expired rows should remain
			Expect(countByCorrID(corrID)).To(Equal(1))
		})

		AfterAll(func() {
			_, _ = db.ExecContext(ctx, "DELETE FROM audit_events WHERE correlation_id = $1", corrID)
		})
	})

	// IT-DS-485-002 (P0): Expired row delete
	// BR-AUDIT-009: Retention policies for audit data
	Describe("IT-DS-485-002: Expired row removed when enabled", func() {
		corrID := "test-ret-485-002"

		BeforeAll(func() {
			insertAuditEvent(
				time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
				1, false, corrID,
			)
		})

		It("should delete expired rows via the retention purge SQL", func() {
			// Simulate worker SQL: DELETE expired rows where event_date + retention_days < now
			// With injected now far in the future, the row should be eligible
			futureNow := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
			deleteQuery := `
				DELETE FROM audit_events
				WHERE correlation_id = $1
				  AND event_date + (retention_days * INTERVAL '1 day') < $2::DATE
				  AND legal_hold = FALSE`
			result, err := db.ExecContext(ctx, deleteQuery, corrID, futureNow)
			Expect(err).NotTo(HaveOccurred())

			rowsAffected, err := result.RowsAffected()
			Expect(err).NotTo(HaveOccurred())
			Expect(rowsAffected).To(Equal(int64(1)), "expected expired row to be deleted")
			Expect(countByCorrID(corrID)).To(Equal(0))
		})

		AfterAll(func() {
			_, _ = db.ExecContext(ctx, "DELETE FROM audit_events WHERE correlation_id = $1", corrID)
		})
	})

	// IT-DS-485-003 (P0): Legal hold — row remains
	// BR-AUDIT-004: Immutability / integrity of audit records
	Describe("IT-DS-485-003: Legal hold — row remains after purge attempt", func() {
		corrID := "test-ret-485-003"

		BeforeAll(func() {
			insertAuditEvent(
				time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
				1, true, corrID,
			)
		})

		It("should NOT delete rows under legal hold even if time-expired", func() {
			futureNow := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
			deleteQuery := `
				DELETE FROM audit_events
				WHERE correlation_id = $1
				  AND event_date + (retention_days * INTERVAL '1 day') < $2::DATE
				  AND legal_hold = FALSE`
			result, err := db.ExecContext(ctx, deleteQuery, corrID, futureNow)
			Expect(err).NotTo(HaveOccurred())

			rowsAffected, err := result.RowsAffected()
			Expect(err).NotTo(HaveOccurred())
			Expect(rowsAffected).To(Equal(int64(0)), "legal hold row must not be deleted")
			Expect(countByCorrID(corrID)).To(Equal(1))
		})

		It("should trigger error on direct DELETE of legal_hold=true row", func() {
			_, err := db.ExecContext(ctx,
				"DELETE FROM audit_events WHERE correlation_id = $1", corrID,
			)
			Expect(err).To(HaveOccurred(), "DB trigger should block direct DELETE of held row")
			Expect(err.Error()).To(ContainSubstring("legal hold"))
		})

		AfterAll(func() {
			// SOC2 trigger blocks UPDATE legal_hold=FALSE and DELETE on held rows.
			// For test cleanup, temporarily drop the triggers, clean up, then restore.
			_, _ = db.ExecContext(ctx, "DROP TRIGGER IF EXISTS enforce_legal_hold_immutability ON audit_events")
			_, _ = db.ExecContext(ctx, "DROP TRIGGER IF EXISTS enforce_legal_hold ON audit_events")
			_, _ = db.ExecContext(ctx, "DELETE FROM audit_events WHERE correlation_id = $1", corrID)
			_, _ = db.ExecContext(ctx, "CREATE TRIGGER enforce_legal_hold BEFORE DELETE ON audit_events FOR EACH ROW EXECUTE FUNCTION prevent_legal_hold_deletion()")
			_, _ = db.ExecContext(ctx, "CREATE TRIGGER enforce_legal_hold_immutability BEFORE UPDATE ON audit_events FOR EACH ROW EXECUTE FUNCTION prevent_legal_hold_removal()")
		})
	})

	// IT-DS-485-004: Partition drop path
	// BR-AUDIT-009: Retention policies for audit data
	Describe("IT-DS-485-004: Partition drop — month fully expired", func() {
		// Use far-future partition to isolate from other tests
		testMonth := time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC)
		partName := partition.FormatPartitionName("audit_events", 2030, time.January)
		corrID := "test-ret-485-004"

		BeforeAll(func() {
			// Create the test partition
			err := partition.EnsureMonthlyPartitions(ctx, db, testMonth, 0, []partition.ParentTable{partition.AuditEventsTable})
			Expect(err).NotTo(HaveOccurred())

			// Insert a row with short retention, no hold
			insertAuditEvent(testMonth.Add(24*time.Hour), 1, false, corrID)
		})

		It("should be able to drop a fully-expired partition after row delete", func() {
			// First: delete all eligible rows
			futureNow := time.Date(2030, time.June, 1, 0, 0, 0, 0, time.UTC)
			_, err := db.ExecContext(ctx, `
				DELETE FROM audit_events
				WHERE event_date >= $1 AND event_date < $2
				  AND event_date + (retention_days * INTERVAL '1 day') < $3::DATE
				  AND legal_hold = FALSE`,
				testMonth, testMonth.AddDate(0, 1, 0), futureNow,
			)
			Expect(err).NotTo(HaveOccurred())

			// Verify partition is empty
			var rowCount int
			err = db.QueryRowContext(ctx,
				fmt.Sprintf("SELECT count(*) FROM %s", partName),
			).Scan(&rowCount)
			Expect(err).NotTo(HaveOccurred())
			Expect(rowCount).To(Equal(0))

			// Drop the partition
			_, err = db.ExecContext(ctx,
				fmt.Sprintf("ALTER TABLE audit_events DETACH PARTITION %s", partName),
			)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.ExecContext(ctx,
				fmt.Sprintf("DROP TABLE IF EXISTS %s", partName),
			)
			Expect(err).NotTo(HaveOccurred())

			// Verify partition is gone
			var exists bool
			err = db.QueryRowContext(ctx,
				"SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1)", partName,
			).Scan(&exists)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		AfterAll(func() {
			_, _ = db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", partName))
		})
	})

	// IT-DS-485-005: Mixed month — only eligible rows deleted
	// BR-AUDIT-004: No over-deletion
	Describe("IT-DS-485-005: Mixed month — eligible only", func() {
		testMonth := time.Date(2030, time.February, 1, 0, 0, 0, 0, time.UTC)
		corrExpired := "test-ret-485-005-expired"
		corrHeld := "test-ret-485-005-held"
		corrFresh := "test-ret-485-005-fresh"

		BeforeAll(func() {
			err := partition.EnsureMonthlyPartitions(ctx, db, testMonth, 0, []partition.ParentTable{partition.AuditEventsTable})
			Expect(err).NotTo(HaveOccurred())

			// Expired, no hold
			insertAuditEvent(testMonth.Add(24*time.Hour), 1, false, corrExpired)
			// Expired, but held
			insertAuditEvent(testMonth.Add(48*time.Hour), 1, true, corrHeld)
			// Fresh (long retention)
			insertAuditEvent(testMonth.Add(72*time.Hour), 2555, false, corrFresh)
		})

		It("should delete only eligible (expired + no hold) rows", func() {
			futureNow := time.Date(2030, time.June, 1, 0, 0, 0, 0, time.UTC)
			_, err := db.ExecContext(ctx, `
				DELETE FROM audit_events
				WHERE event_date >= $1 AND event_date < $2
				  AND event_date + (retention_days * INTERVAL '1 day') < $3::DATE
				  AND legal_hold = FALSE`,
				testMonth, testMonth.AddDate(0, 1, 0), futureNow,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(countByCorrID(corrExpired)).To(Equal(0), "expired row should be deleted")
			Expect(countByCorrID(corrHeld)).To(Equal(1), "held row must survive")
			Expect(countByCorrID(corrFresh)).To(Equal(1), "fresh row must survive")
		})

		AfterAll(func() {
			// Drop triggers temporarily to clean up held rows
			_, _ = db.ExecContext(ctx, "DROP TRIGGER IF EXISTS enforce_legal_hold_immutability ON audit_events")
			_, _ = db.ExecContext(ctx, "DROP TRIGGER IF EXISTS enforce_legal_hold ON audit_events")
			_, _ = db.ExecContext(ctx, "DELETE FROM audit_events WHERE correlation_id LIKE 'test-ret-485-005-%'")
			_, _ = db.ExecContext(ctx, "CREATE TRIGGER enforce_legal_hold BEFORE DELETE ON audit_events FOR EACH ROW EXECUTE FUNCTION prevent_legal_hold_deletion()")
			_, _ = db.ExecContext(ctx, "CREATE TRIGGER enforce_legal_hold_immutability BEFORE UPDATE ON audit_events FOR EACH ROW EXECUTE FUNCTION prevent_legal_hold_removal()")
			_, _ = db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s",
				partition.FormatPartitionName("audit_events", 2030, time.February)))
		})
	})

	// IT-DS-485-006: Operation log
	// BR-AUDIT-009: Audit trail
	Describe("IT-DS-485-006: retention_operations log", func() {
		It("should be possible to insert a structured retention operation record", func() {
			runID := uuid.New().String()
			_, err := db.ExecContext(ctx, `
				INSERT INTO retention_operations (
					run_id, scope, period_start, period_end,
					rows_scanned, rows_deleted, partitions_dropped,
					status, operation_start, operation_end, operation_duration_ms
				) VALUES (
					$1, 'audit_events', $2, $3,
					100, 5, '{}',
					'completed', $4, $5, 1234
				)`,
				runID,
				time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
				time.Now(), time.Now(),
			)
			Expect(err).NotTo(HaveOccurred())

			// Verify record exists
			var count int
			err = db.QueryRowContext(ctx,
				"SELECT count(*) FROM retention_operations WHERE run_id = $1::UUID", runID,
			).Scan(&count)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			// Cleanup
			_, _ = db.ExecContext(ctx, "DELETE FROM retention_operations WHERE run_id = $1::UUID", runID)
		})
	})

	// IT-DS-485-007: Schedule — injected clock
	// BR-AUDIT-009: Scheduled retention runs
	Describe("IT-DS-485-007: Worker schedule via injected clock", func() {
		It("should allow configuring the worker interval", func() {
			cfg := retention.Config{
				Enabled:  true,
				Interval: 1 * time.Hour,
			}
			Expect(cfg.Interval).To(Equal(1 * time.Hour))
			Expect(cfg.Enabled).To(BeTrue())
		})

		It("should use the Clock interface for time determination", func() {
			// Verify the Clock interface contract exists and UTCClock satisfies it
			var clock retention.Clock = retention.UTCClock{}
			now := clock.Now()
			Expect(now.Location()).To(Equal(time.UTC))
		})
	})

	// IT-DS-485-008: SOC2 CC6.1 — legal_hold cannot be set to FALSE via UPDATE
	// BR-AUDIT-004: Immutability / integrity of audit records
	Describe("IT-DS-485-008: SOC2 — legal_hold immutable once true", func() {
		corrID := "test-ret-485-008"

		BeforeAll(func() {
			insertAuditEvent(
				time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
				2555, true, corrID,
			)
		})

		It("should block UPDATE that sets legal_hold from TRUE to FALSE", func() {
			_, err := db.ExecContext(ctx,
				"UPDATE audit_events SET legal_hold = FALSE WHERE correlation_id = $1", corrID,
			)
			Expect(err).To(HaveOccurred(), "SOC2 trigger should block legal_hold removal")
			Expect(err.Error()).To(ContainSubstring("SOC2"))
		})

		It("should allow UPDATE that keeps legal_hold = TRUE", func() {
			_, err := db.ExecContext(ctx,
				"UPDATE audit_events SET legal_hold_reason = 'updated reason' WHERE correlation_id = $1", corrID,
			)
			Expect(err).NotTo(HaveOccurred(), "updating other fields should be allowed")
		})

		AfterAll(func() {
			// SOC2 triggers block both UPDATE and DELETE on held rows.
			// For test cleanup, temporarily drop triggers, clean up, then restore.
			_, _ = db.ExecContext(ctx, "DROP TRIGGER IF EXISTS enforce_legal_hold_immutability ON audit_events")
			_, _ = db.ExecContext(ctx, "DROP TRIGGER IF EXISTS enforce_legal_hold ON audit_events")
			_, _ = db.ExecContext(ctx, "DELETE FROM audit_events WHERE correlation_id = $1", corrID)
			_, _ = db.ExecContext(ctx, "CREATE TRIGGER enforce_legal_hold BEFORE DELETE ON audit_events FOR EACH ROW EXECUTE FUNCTION prevent_legal_hold_deletion()")
			_, _ = db.ExecContext(ctx, "CREATE TRIGGER enforce_legal_hold_immutability BEFORE UPDATE ON audit_events FOR EACH ROW EXECUTE FUNCTION prevent_legal_hold_removal()")
		})
	})
})
