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

// Package datastorage contains unit tests for the DataStorage service.
// BR-STORAGE-015: Workflow success-rate tracking.
package datastorage_test

import (
	"context"
	"database/sql"
	"errors"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// ========================================
// UNIT TESTS: on-demand workflow success-rate aggregation (Issue #1661 Change 7)
// ========================================
// Authority: DD-WORKFLOW-018. total_executions/successful_executions/
// actual_success_rate stop being stored remediation_workflow_catalog columns
// (updated by the now-deleted UpdateSuccessMetrics) and become an aggregation
// query against audit_events, grouping workflowexecution.workflow.completed/
// .failed events by event_data.workflow_id, computed at query time in
// workflow_query_handlers.go.
//
// RED: AuditEventsRepository.GetSuccessMetrics does not exist yet -- this
// file must fail to compile.
// ========================================
var _ = Describe("AuditEventsRepository.GetSuccessMetrics (Issue #1661 Change 7)", func() {
	var (
		mockDB  *sql.DB
		sqlMock sqlmock.Sqlmock
		repo    *repository.AuditEventsRepository
		logger  logr.Logger
		ctx     context.Context
	)

	BeforeEach(func() {
		var err error
		mockDB, sqlMock, err = sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		Expect(err).ToNot(HaveOccurred())

		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())
		ctx = context.Background()

		repo = repository.NewAuditEventsRepository(mockDB, logger)
	})

	AfterEach(func() {
		Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
		_ = mockDB.Close()
	})

	It("UT-DS-1661-701-001: computes actual_success_rate as successful/total from audit_events rows, not a stored column", func() {
		// BR-STORAGE-015: total_executions=10, successful_executions=7 must
		// yield actual_success_rate=0.7, aggregated from audit_events at
		// query time -- there is no remediation_workflow_catalog column read here.
		rows := sqlmock.NewRows([]string{"workflow_id", "total_executions", "successful_executions"}).
			AddRow("wf-1", 10, 7)
		sqlMock.ExpectQuery(`SELECT .* FROM audit_events WHERE event_type = ANY`).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnRows(rows)

		metrics, err := repo.GetSuccessMetrics(ctx, []string{"wf-1"})

		Expect(err).ToNot(HaveOccurred())
		Expect(metrics).To(HaveKey("wf-1"))
		Expect(metrics["wf-1"].TotalExecutions).To(Equal(10))
		Expect(metrics["wf-1"].SuccessfulExecutions).To(Equal(7))
		Expect(metrics["wf-1"].ActualSuccessRate).ToNot(BeNil())
		Expect(*metrics["wf-1"].ActualSuccessRate).To(BeNumerically("~", 0.7, 0.0001))
	})

	It("UT-DS-1661-701-002: a workflow with zero matching audit_events rows is absent from the result (caller defaults to nil rate)", func() {
		// BR-STORAGE-015: a never-executed workflow must not spuriously
		// appear with a fabricated 0/0 row -- absence is the signal.
		rows := sqlmock.NewRows([]string{"workflow_id", "total_executions", "successful_executions"})
		sqlMock.ExpectQuery(`SELECT .* FROM audit_events WHERE event_type = ANY`).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnRows(rows)

		metrics, err := repo.GetSuccessMetrics(ctx, []string{"wf-never-executed"})

		Expect(err).ToNot(HaveOccurred())
		Expect(metrics).ToNot(HaveKey("wf-never-executed"))
	})

	It("UT-DS-1661-701-003: an empty workflowIDs slice short-circuits without issuing a query", func() {
		metrics, err := repo.GetSuccessMetrics(ctx, nil)

		Expect(err).ToNot(HaveOccurred())
		Expect(metrics).To(BeEmpty())
	})

	It("UT-DS-1661-701-004: propagates a real database error instead of masking it as empty results", func() {
		sqlMock.ExpectQuery(`SELECT .* FROM audit_events WHERE event_type = ANY`).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnError(errors.New("connection refused"))

		metrics, err := repo.GetSuccessMetrics(ctx, []string{"wf-1"})

		Expect(err).To(HaveOccurred())
		Expect(metrics).To(BeNil())
	})
})
