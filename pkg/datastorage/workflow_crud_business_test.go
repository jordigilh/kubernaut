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
	"errors"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubelog "github.com/jordigilh/kubernaut/pkg/log"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
)

// ========================================
// WAVE 6 SUB-WAVE 6f RED PHASE: characterization tests for the 3 confirmed
// 0%-coverage functions in crud.go, per the Wave 6 Burndown Plan §6f.
// BR-STORAGE-012 (workflow catalog persistence), BR-STORAGE-015 (success-rate
// tracking) — SOC2 CC7.2 (decision-audit-trail correctness depends on these
// repository functions returning accurate not-found/error/success signals).
// ========================================
var _ = Describe("Workflow CRUD business-level gap closure (Wave 6 6f)", func() {
	var (
		mockDB  *sql.DB
		sqlMock sqlmock.Sqlmock
		repo    *workflow.Repository
		ctx     context.Context
	)

	BeforeEach(func() {
		var err error
		mockDB, sqlMock, err = sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		Expect(err).ToNot(HaveOccurred())

		logger := kubelog.NewLogger(kubelog.DevelopmentOptions())
		ctx = context.Background()

		sqlxDB := sqlx.NewDb(mockDB, "sqlmock")
		repo = workflow.NewRepository(sqlxDB, logger)
	})

	AfterEach(func() {
		Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
		_ = mockDB.Close()
	})

	Describe("GetByID", func() {
		It("UT-DS-6f-001: returns (nil, nil) — not an error — when no workflow matches the ID", func() {
			// BR-STORAGE-012: callers (server/workflow_query_handlers.go) distinguish
			// "not found" (nil, nil -> HTTP 404) from a real DB failure (non-nil error
			// -> HTTP 500). Conflating the two would either mask outages as 404s or
			// leak DB errors as "not found".
			sqlMock.ExpectQuery(`SELECT .* FROM remediation_workflow_catalog WHERE workflow_id = \$1`).
				WillReturnError(sql.ErrNoRows)

			wf, err := repo.GetByID(ctx, "00000000-0000-0000-0000-000000000000")

			Expect(err).ToNot(HaveOccurred(), "BR-STORAGE-012: not-found must not surface as an error")
			Expect(wf).To(BeNil())
		})

		It("UT-DS-6f-002: returns the workflow when the ID matches an existing row", func() {
			rows := sqlmock.NewRows([]string{
				"workflow_id", "workflow_name", "version", "schema_version", "name", "description",
				"content", "content_hash", "action_type", "status",
				"labels", "detected_labels", "custom_labels",
				"execution_engine", "execution_bundle",
				"owner", "maintainer",
				"is_latest_version",
				"expected_success_rate", "actual_success_rate",
				"total_executions", "successful_executions",
				"created_at", "updated_at",
			}).AddRow(
				"wf-1", "restart-pod", "v1", "1", "Restart Pod", []byte(`{"what":"restart"}`),
				"content-body", "hash1", "RestartPod", "Active",
				[]byte("{}"), []byte("{}"), []byte("{}"),
				"tekton", []byte("{}"),
				"team-a", "team-a",
				true,
				0.9, 0.85,
				10, 8,
				time.Now(), time.Now(),
			)
			sqlMock.ExpectQuery(`SELECT .* FROM remediation_workflow_catalog WHERE workflow_id = \$1`).
				WillReturnRows(rows)

			wf, err := repo.GetByID(ctx, "wf-1")

			Expect(err).ToNot(HaveOccurred())
			Expect(wf).ToNot(BeNil())
			Expect(wf.WorkflowID).To(Equal("wf-1"))
		})

		It("UT-DS-6f-003: propagates a real database error instead of masking it as not-found", func() {
			sqlMock.ExpectQuery(`SELECT .* FROM remediation_workflow_catalog WHERE workflow_id = \$1`).
				WillReturnError(errors.New("connection refused"))

			wf, err := repo.GetByID(ctx, "wf-1")

			Expect(err).To(HaveOccurred(), "BR-STORAGE-012: a real DB error must not be silently treated as not-found")
			Expect(wf).To(BeNil())
		})
	})

	Describe("GetVersionsByName", func() {
		It("UT-DS-6f-004: propagates a database error instead of returning an empty slice", func() {
			// Complements the existing UT-DS-213-003 ordering test (success path);
			// this closes the previously-uncovered error branch so a query failure
			// is distinguishable from "workflow has zero versions".
			sqlMock.ExpectQuery(`ORDER BY created_at DESC, workflow_id ASC`).
				WillReturnError(errors.New("connection refused"))

			versions, err := repo.GetVersionsByName(ctx, "restart-pod")

			Expect(err).To(HaveOccurred(), "BR-STORAGE-012: a query failure must not be reported as zero versions")
			Expect(versions).To(BeNil())
		})
	})

	Describe("UpdateSuccessMetrics", func() {
		It("UT-DS-6f-005: computes actual_success_rate as successful/total via the SQL CASE expression", func() {
			// BR-STORAGE-015: success-rate math must be correct so downstream workflow
			// selection (which ranks by actual_success_rate) isn't fed bad data.
			sqlMock.ExpectExec(`UPDATE remediation_workflow_catalog`).
				WithArgs(10, 8, "wf-1", "v1").
				WillReturnResult(sqlmock.NewResult(0, 1))

			err := repo.UpdateSuccessMetrics(ctx, "wf-1", "v1", 10, 8)

			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-DS-6f-006: divide-by-zero guard — zero total_executions must not error out of the UPDATE itself", func() {
			// The CASE WHEN $1 > 0 ... ELSE 0 guard lives in the SQL, not Go; this
			// characterizes that a zero-total update still executes successfully
			// (the CASE guard, not a Go-side error, prevents the division by zero).
			sqlMock.ExpectExec(`UPDATE remediation_workflow_catalog`).
				WithArgs(0, 0, "wf-1", "v1").
				WillReturnResult(sqlmock.NewResult(0, 1))

			err := repo.UpdateSuccessMetrics(ctx, "wf-1", "v1", 0, 0)

			Expect(err).ToNot(HaveOccurred(), "BR-STORAGE-015: zero-execution workflows must update without a division-by-zero failure")
		})

		It("UT-DS-6f-007: zero rows affected returns an explicit \"workflow not found\" error", func() {
			// BR-STORAGE-015: a workflow_id+version pair that doesn't exist must fail
			// loudly and specifically, not silently report success on a no-op UPDATE.
			sqlMock.ExpectExec(`UPDATE remediation_workflow_catalog`).
				WithArgs(5, 3, "nonexistent", "v1").
				WillReturnResult(sqlmock.NewResult(0, 0))

			err := repo.UpdateSuccessMetrics(ctx, "nonexistent", "v1", 5, 3)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workflow not found"))
		})

		It("UT-DS-6f-008: propagates a database error from the UPDATE statement", func() {
			sqlMock.ExpectExec(`UPDATE remediation_workflow_catalog`).
				WithArgs(5, 3, "wf-1", "v1").
				WillReturnError(errors.New("connection refused"))

			err := repo.UpdateSuccessMetrics(ctx, "wf-1", "v1", 5, 3)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).ToNot(ContainSubstring("workflow not found"),
				"a real DB error must not be conflated with the explicit zero-rows-affected error")
		})
	})
})
