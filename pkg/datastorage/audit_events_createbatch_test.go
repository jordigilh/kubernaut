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
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// CHARACTERIZATION TESTS: AuditEventsRepository.CreateBatch
// 📋 Business Requirement: BR-AUDIT-001 (Complete audit trail, no data loss)
// 📋 Compliance: SOC2 CC8.1 (audit completeness), AU-9 (hash-chain tamper evidence)
//
// Written per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3 (Phase 6, DataStorage)
// coverage-before-refactor mandate: CreateBatch (cyclomatic 25) had only
// integration-level coverage before this file. These tests pin down its
// current behavior (field defaulting, per-correlation-id hash chaining,
// transaction semantics) so the planned decomposition is provably
// behavior-preserving.
// ========================================
var _ = Describe("AuditEventsRepository.CreateBatch", func() {
	var (
		mockDB *sql.DB
		mock   sqlmock.Sqlmock
		repo   *repository.AuditEventsRepository
		ctx    context.Context
		logger logr.Logger
	)

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

	It("returns an error without touching the database when the batch is empty", func() {
		created, err := repo.CreateBatch(ctx, []*repository.AuditEvent{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("batch cannot be empty"))
		Expect(created).To(BeNil())
		Expect(mock.ExpectationsWereMet()).To(Succeed())
	})

	It("stamps defaults (event_id, version, retention_days, hash_algorithm) and persists a single event", func() {
		event := &repository.AuditEvent{
			EventType:     "test.batch.single",
			EventCategory: "test",
			EventAction:   "create",
			EventOutcome:  "success",
			CorrelationID: "batch-single",
			EventData:     map[string]interface{}{},
		}

		mock.ExpectBegin()
		mock.ExpectPrepare("INSERT INTO audit_events")
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("batch-single").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT event_hash").WithArgs("batch-single").WillReturnRows(sqlmock.NewRows([]string{"event_hash"}))
		mock.ExpectQuery("INSERT INTO audit_events").
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), repository.HashAlgorithmSHA256Unkeyed,
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"event_timestamp"}).AddRow(time.Now()))
		mock.ExpectCommit()

		created, err := repo.CreateBatch(ctx, []*repository.AuditEvent{event})
		Expect(err).ToNot(HaveOccurred())
		Expect(created).To(HaveLen(1))
		Expect(created[0].EventID).ToNot(BeZero(), "CreateBatch must generate an EventID when the caller omits one")
		Expect(created[0].Version).To(Equal("1.0"))
		Expect(created[0].RetentionDays).To(Equal(2555))
		Expect(created[0].HashAlgorithm).To(Equal(repository.HashAlgorithmSHA256Unkeyed))
		Expect(created[0].PreviousEventHash).To(BeEmpty(), "the first event for a fresh correlation_id has no previous hash")
		Expect(created[0].EventHash).ToNot(BeEmpty())
		Expect(mock.ExpectationsWereMet()).To(Succeed())
	})

	It("chains previous_event_hash across events sharing a correlation_id within the same batch", func() {
		e1 := &repository.AuditEvent{
			EventType: "test.batch.chain.1", EventCategory: "test", EventAction: "create", EventOutcome: "success",
			CorrelationID: "batch-chain", EventData: map[string]interface{}{},
		}
		e2 := &repository.AuditEvent{
			EventType: "test.batch.chain.2", EventCategory: "test", EventAction: "create", EventOutcome: "success",
			CorrelationID: "batch-chain", EventData: map[string]interface{}{},
		}

		mock.ExpectBegin()
		mock.ExpectPrepare("INSERT INTO audit_events")
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("batch-chain").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT event_hash").WithArgs("batch-chain").WillReturnRows(sqlmock.NewRows([]string{"event_hash"}))
		mock.ExpectQuery("INSERT INTO audit_events").WillReturnRows(sqlmock.NewRows([]string{"event_timestamp"}).AddRow(time.Now()))
		mock.ExpectQuery("INSERT INTO audit_events").WillReturnRows(sqlmock.NewRows([]string{"event_timestamp"}).AddRow(time.Now()))
		mock.ExpectCommit()

		created, err := repo.CreateBatch(ctx, []*repository.AuditEvent{e1, e2})
		Expect(err).ToNot(HaveOccurred())
		Expect(created).To(HaveLen(2))
		Expect(created[0].PreviousEventHash).To(BeEmpty())
		Expect(created[1].PreviousEventHash).To(Equal(created[0].EventHash),
			"the second event in the same correlation_id batch must chain to the first event's hash")
	})

	It("preserves input order in the returned slice even when correlation_ids are processed in sorted order", func() {
		// SortedCorrelationIDs processes "corr-a" before "corr-b" regardless of input order;
		// CreateBatch must still return results indexed to match the original input slice.
		eB := &repository.AuditEvent{
			EventType: "test.batch.order.b", EventCategory: "test", EventAction: "create", EventOutcome: "success",
			CorrelationID: "corr-b", EventData: map[string]interface{}{},
		}
		eA := &repository.AuditEvent{
			EventType: "test.batch.order.a", EventCategory: "test", EventAction: "create", EventOutcome: "success",
			CorrelationID: "corr-a", EventData: map[string]interface{}{},
		}

		mock.ExpectBegin()
		mock.ExpectPrepare("INSERT INTO audit_events")
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("corr-a").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT event_hash").WithArgs("corr-a").WillReturnRows(sqlmock.NewRows([]string{"event_hash"}))
		mock.ExpectQuery("INSERT INTO audit_events").WillReturnRows(sqlmock.NewRows([]string{"event_timestamp"}).AddRow(time.Now()))
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("corr-b").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT event_hash").WithArgs("corr-b").WillReturnRows(sqlmock.NewRows([]string{"event_hash"}))
		mock.ExpectQuery("INSERT INTO audit_events").WillReturnRows(sqlmock.NewRows([]string{"event_timestamp"}).AddRow(time.Now()))
		mock.ExpectCommit()

		created, err := repo.CreateBatch(ctx, []*repository.AuditEvent{eB, eA})
		Expect(err).ToNot(HaveOccurred())
		Expect(created).To(HaveLen(2))
		Expect(created[0].EventType).To(Equal("test.batch.order.b"), "result order must match input order, not sorted correlation-id processing order")
		Expect(created[1].EventType).To(Equal("test.batch.order.a"))
		Expect(mock.ExpectationsWereMet()).To(Succeed())
	})

	It("rolls back and returns an error without any created events when the transaction cannot begin", func() {
		event := &repository.AuditEvent{
			EventType: "test.batch.beginfail", EventCategory: "test", EventAction: "create", EventOutcome: "success",
			CorrelationID: "batch-beginfail", EventData: map[string]interface{}{},
		}

		mock.ExpectBegin().WillReturnError(sql.ErrConnDone)

		created, err := repo.CreateBatch(ctx, []*repository.AuditEvent{event})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to begin transaction"))
		Expect(created).To(BeNil())
		Expect(mock.ExpectationsWereMet()).To(Succeed())
	})

	It("rolls back and returns an error when the INSERT for an event fails", func() {
		event := &repository.AuditEvent{
			EventType: "test.batch.insertfail", EventCategory: "test", EventAction: "create", EventOutcome: "success",
			CorrelationID: "batch-insertfail", EventData: map[string]interface{}{},
		}

		mock.ExpectBegin()
		mock.ExpectPrepare("INSERT INTO audit_events")
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("batch-insertfail").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("SELECT event_hash").WithArgs("batch-insertfail").WillReturnRows(sqlmock.NewRows([]string{"event_hash"}))
		mock.ExpectQuery("INSERT INTO audit_events").WillReturnError(sql.ErrTxDone)
		mock.ExpectRollback()

		created, err := repo.CreateBatch(ctx, []*repository.AuditEvent{event})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to insert event"))
		Expect(created).To(BeNil())
		Expect(mock.ExpectationsWereMet()).To(Succeed())
	})
})
