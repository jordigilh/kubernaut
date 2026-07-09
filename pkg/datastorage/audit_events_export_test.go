/*
Copyright 2026 Jordi Gil.

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

package datastorage_test

import (
	"context"
	"database/sql"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// CHARACTERIZATION TESTS: AuditEventsRepository.Export
// 📋 Business Requirement: BR-AUDIT-007 (Tamper-evident audit exports)
// 📋 Compliance: SOC2 CC8.1 (audit export), AU-9 (protection of audit information)
//
// Written per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3 (Phase 6, DataStorage)
// coverage-before-refactor mandate: Export (cyclomatic 25) had only
// integration-level coverage before this file. These tests pin down its
// current hash-chain verification and filter-building behavior so the
// planned decomposition is provably behavior-preserving.
// ========================================
var _ = Describe("AuditEventsRepository.Export", func() {
	var (
		mockDB *sql.DB
		mock   sqlmock.Sqlmock
		repo   *repository.AuditEventsRepository
		ctx    context.Context
		logger logr.Logger
	)

	exportColumns := []string{
		"event_id", "event_version", "event_type", "event_timestamp",
		"event_category", "event_action", "event_outcome", "correlation_id",
		"parent_event_id", "parent_event_date", "resource_type", "resource_id",
		"namespace", "cluster_id", "actor_id", "actor_type", "actor_ip",
		"severity", "duration_ms", "error_code", "error_message",
		"retention_days", "is_sensitive", "event_data",
		"event_hash", "previous_event_hash", "hash_algorithm", "legal_hold",
	}

	BeforeEach(func() {
		var err error
		mockDB, mock, err = sqlmock.New()
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()
		logger = kubelog.NewLogger(kubelog.DefaultOptions())
		repo = repository.NewAuditEventsRepository(mockDB, logger)
	})

	AfterEach(func() {
		_ = mockDB.Close()
	})

	It("wraps and returns the underlying error when the query fails", func() {
		mock.ExpectQuery("SELECT").WillReturnError(sql.ErrConnDone)

		result, err := repo.Export(ctx, repository.ExportFilters{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to query audit events"))
		Expect(result).To(BeNil())
	})

	It("returns 100% chain integrity and no events when the query matches nothing", func() {
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(exportColumns))

		result, err := repo.Export(ctx, repository.ExportFilters{})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.TotalEventsQueried).To(Equal(0))
		Expect(result.ChainIntegrityPercent).To(Equal(float32(100.0)))
		Expect(result.Events).To(BeEmpty())
		Expect(*result.TamperedEventIDs).To(BeEmpty())
	})

	It("marks a legacy event (no hash fields) as a valid chain link without recomputing a hash", func() {
		eventID := uuid.New()
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(exportColumns).AddRow(
			eventID.String(), "1.0", "test.export.legacy", time.Now(),
			"test", "export", "success", "corr-legacy",
			nil, nil, nil, nil,
			nil, nil, nil, nil, nil,
			nil, nil, nil, nil,
			2555, false, []byte(`{}`),
			"", "", "", false,
		))

		result, err := repo.Export(ctx, repository.ExportFilters{})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Events).To(HaveLen(1))
		Expect(result.Events[0].HashChainValid).To(BeTrue())
		Expect(result.ValidChainEvents).To(Equal(1))
		Expect(result.BrokenChainEvents).To(Equal(0))
		Expect(result.ChainIntegrityPercent).To(Equal(float32(100.0)))
	})

	It("flags an event as tampered when previous_event_hash does not match the actual chain predecessor", func() {
		eventID := uuid.New()
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(exportColumns).AddRow(
			eventID.String(), "1.0", "test.export.tampered", time.Now(),
			"test", "export", "success", "corr-tampered",
			nil, nil, nil, nil,
			nil, nil, nil, nil, nil,
			nil, nil, nil, nil,
			2555, false, []byte(`{}`),
			"some-hash", "unexpected-previous-hash", repository.HashAlgorithmSHA256Unkeyed, false,
		))

		result, err := repo.Export(ctx, repository.ExportFilters{})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Events).To(HaveLen(1))
		Expect(result.Events[0].HashChainValid).To(BeFalse())
		Expect(result.BrokenChainEvents).To(Equal(1))
		Expect(result.ValidChainEvents).To(Equal(0))
		Expect(*result.TamperedEventIDs).To(ContainElement(eventID.String()))
		Expect(result.ChainIntegrityPercent).To(Equal(float32(0.0)))
	})

	It("verifies a correctly chained event (event_hash matches the recomputed hash) as valid", func() {
		// Produce a legitimately hashed event via CreateBatch (already characterized above)
		// so this test proves Export's verification is self-consistent with write-time
		// hashing, rather than asserting against an independently-constructed hash that
		// could silently drift from CreateBatch's exact field set.
		created := &repository.AuditEvent{
			EventType: "test.export.valid", EventCategory: "test", EventAction: "export", EventOutcome: "success",
			CorrelationID: "corr-valid", EventData: map[string]interface{}{},
		}
		mock.ExpectBegin()
		mock.ExpectPrepare("INSERT INTO audit_events")
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("corr-valid").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT event_hash").WithArgs("corr-valid").WillReturnRows(sqlmock.NewRows([]string{"event_hash"}))
		mock.ExpectQuery("INSERT INTO audit_events").WillReturnRows(sqlmock.NewRows([]string{"event_timestamp"}).AddRow(time.Now()))
		mock.ExpectCommit()

		batchResult, err := repo.CreateBatch(ctx, []*repository.AuditEvent{created})
		Expect(err).ToNot(HaveOccurred())
		event := batchResult[0]

		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(exportColumns).AddRow(
			event.EventID.String(), event.Version, event.EventType, event.EventTimestamp,
			event.EventCategory, event.EventAction, event.EventOutcome, event.CorrelationID,
			nil, nil, nil, nil,
			nil, nil, nil, nil, nil,
			nil, nil, nil, nil,
			event.RetentionDays, event.IsSensitive, []byte(`{}`),
			event.EventHash, event.PreviousEventHash, event.HashAlgorithm, false,
		))

		result, err := repo.Export(ctx, repository.ExportFilters{})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Events).To(HaveLen(1))
		Expect(result.Events[0].HashChainValid).To(BeTrue(),
			"an event freshly hashed by CreateBatch must verify as valid when Export re-derives the same hash")
		Expect(result.ValidChainEvents).To(Equal(1))
		Expect(result.ChainIntegrityPercent).To(Equal(float32(100.0)))
	})

	It("applies StartTime, EndTime, CorrelationID, and EventCategory filters as additive WHERE clauses", func() {
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery("SELECT").
			WithArgs(start, end, "corr-filtered", "test-category", 1000, 5).
			WillReturnRows(sqlmock.NewRows(exportColumns))

		_, err := repo.Export(ctx, repository.ExportFilters{
			StartTime:     &start,
			EndTime:       &end,
			CorrelationID: "corr-filtered",
			EventCategory: "test-category",
			Offset:        5,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(mock.ExpectationsWereMet()).To(Succeed(), "filters must be applied as positional args in declaration order, defaulting limit to 1000")
	})

	It("uses the caller-provided Limit instead of the default 1000 when non-zero", func() {
		mock.ExpectQuery("SELECT").
			WithArgs(50, 0).
			WillReturnRows(sqlmock.NewRows(exportColumns))

		_, err := repo.Export(ctx, repository.ExportFilters{Limit: 50})
		Expect(err).ToNot(HaveOccurred())
		Expect(mock.ExpectationsWereMet()).To(Succeed())
	})
})
