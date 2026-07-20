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

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubelog "github.com/jordigilh/kubernaut/pkg/log"

	workflowrepo "github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
)

// ========================================
// PHASE 6: SELECT * NARROWING — WORKFLOW REPOSITORY (TP-1088-P1)
// ========================================
//
// Business Requirement: BR-WORKFLOW-004 (workflow catalog)
// Issue: #1088 Phase 6.1 (Performance — schema drift protection)
// TDD Phase: RED — these tests FAIL against current SELECT * implementation
//
// The remediation_workflow_catalog table has ~40 columns. SELECT * fetches
// ALL columns including deprecated ones (e.g., embedding). Explicit column
// lists protect against schema drift and reduce I/O.
//
// 38 db: tags from RemediationWorkflow struct (pkg/datastorage/models/workflow.go)
// ========================================

// workflowColumns is the authoritative column list derived from
// RemediationWorkflow struct db: tags. Used by all tests in this file.
var workflowColumns = []string{
	"workflow_id", "workflow_name", "version", "schema_version",
	"name", "description", "owner", "maintainer",
	"content", "content_hash",
	"action_type",
	"parameters", "execution_engine",
	"schema_image", "schema_digest",
	"execution_bundle", "execution_bundle_digest",
	"engine_config", "service_account_name",
	"labels", "custom_labels", "detected_labels",
	"status", "status_reason",
	"disabled_at", "disabled_by", "disabled_reason",
	"is_latest_version", "previous_version", "deprecation_notice",
	"version_notes", "change_summary", "approved_by", "approved_at",
	"expected_success_rate", "expected_duration_seconds",
	"actual_success_rate", "total_executions", "successful_executions",
	"created_at", "updated_at", "created_by", "updated_by",
}

// workflowColumnsRegex is a regex fragment matching the start of an explicit
// column list. sqlmock's QueryMatcherRegexp matches anywhere in the query.
const workflowColumnsRegex = `SELECT workflow_id, workflow_name, version, schema_version`

var _ = Describe("Phase 6: SELECT * Narrowing — Workflow Repository (TP-1088-P1)", func() {
	var (
		mockDB  *sql.DB
		sqlMock sqlmock.Sqlmock
		repo    *workflowrepo.Repository
		ctx     context.Context
	)

	BeforeEach(func() {
		var err error
		mockDB, sqlMock, err = sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		Expect(err).ToNot(HaveOccurred())

		logger := kubelog.NewLogger(kubelog.DevelopmentOptions())
		ctx = context.Background()

		sqlxDB := sqlx.NewDb(mockDB, "sqlmock")
		repo = workflowrepo.NewRepository(sqlxDB, logger)
	})

	AfterEach(func() {
		Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
		_ = mockDB.Close()
	})

	emptyWorkflowRows := func() *sqlmock.Rows {
		return sqlmock.NewRows(workflowColumns)
	}

	// ========================================
	// crud.go queries (7 queries)
	// ========================================

	Describe("GetByID (crud.go:359)", func() {
		It("UT-DS-1088-P6-002: query must use explicit column list, not SELECT *", func() {
			sqlMock.ExpectQuery(workflowColumnsRegex).
				WithArgs("test-uuid").
				WillReturnRows(emptyWorkflowRows())

			_, err := repo.GetByID(ctx, "test-uuid")
			Expect(errors.Is(err, workflowrepo.ErrNotFound)).To(BeTrue(),
				"Issue #1674: an empty result set is expected not-found, not a real error")
		})
	})

	Describe("GetByNameAndVersion (crud.go:381)", func() {
		It("UT-DS-1088-P6-003: query must use explicit column list, not SELECT *", func() {
			sqlMock.ExpectQuery(workflowColumnsRegex).
				WithArgs("pod-restart", "v1.0.0").
				WillReturnRows(emptyWorkflowRows())

			_, err := repo.GetByNameAndVersion(ctx, "pod-restart", "v1.0.0")
			Expect(errors.Is(err, workflowrepo.ErrNotFound)).To(BeTrue(),
				"Issue #1674: an empty result set is expected not-found, not a real error")
		})
	})

	Describe("GetActiveByNameAndVersion (crud.go:404)", func() {
		It("UT-DS-1088-P6-004: query must use explicit column list, not SELECT *", func() {
			sqlMock.ExpectQuery(workflowColumnsRegex).
				WithArgs("pod-restart", "v1.0.0").
				WillReturnRows(emptyWorkflowRows())

			_, err := repo.GetActiveByNameAndVersion(ctx, "pod-restart", "v1.0.0")
			Expect(errors.Is(err, workflowrepo.ErrNotFound)).To(BeTrue(),
				"Issue #1674: an empty result set is expected not-found, not a real error")
		})
	})

	Describe("GetLatestDisabledByNameAndVersion (crud.go:426)", func() {
		It("UT-DS-1088-P6-005: query must use explicit column list, not SELECT *", func() {
			sqlMock.ExpectQuery(workflowColumnsRegex).
				WithArgs("pod-restart", "v1.0.0").
				WillReturnRows(emptyWorkflowRows())

			_, err := repo.GetLatestDisabledByNameAndVersion(ctx, "pod-restart", "v1.0.0")
			Expect(errors.Is(err, workflowrepo.ErrNotFound)).To(BeTrue(),
				"Issue #1674: an empty result set is expected not-found, not a real error")
		})
	})

	Describe("GetActiveByWorkflowName (crud.go:450)", func() {
		It("UT-DS-1088-P6-006: query must use explicit column list, not SELECT *", func() {
			sqlMock.ExpectQuery(workflowColumnsRegex).
				WithArgs("pod-restart").
				WillReturnRows(emptyWorkflowRows())

			_, err := repo.GetActiveByWorkflowName(ctx, "pod-restart")
			Expect(errors.Is(err, workflowrepo.ErrNotFound)).To(BeTrue(),
				"Issue #1674: an empty result set is expected not-found, not a real error")
		})
	})

	Describe("GetLatestVersion (crud.go:473)", func() {
		It("UT-DS-1088-P6-007: query must use explicit column list, not SELECT *", func() {
			sqlMock.ExpectQuery(workflowColumnsRegex).
				WithArgs("pod-restart").
				WillReturnRows(emptyWorkflowRows())

			_, err := repo.GetLatestVersion(ctx, "pod-restart")
			Expect(errors.Is(err, workflowrepo.ErrNotFound)).To(BeTrue(),
				"Issue #1674: an empty result set is expected not-found, not a real error")
		})
	})

	Describe("GetVersionsByName (crud.go:495)", func() {
		It("UT-DS-1088-P6-008: query must use explicit column list, not SELECT *", func() {
			sqlMock.ExpectQuery(workflowColumnsRegex).
				WithArgs("pod-restart").
				WillReturnRows(emptyWorkflowRows())

			_, err := repo.GetVersionsByName(ctx, "pod-restart")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	// ========================================
	// discovery.go queries (2 queries)
	// ========================================

	Describe("ListWorkflowsByActionType (discovery.go:173)", func() {
		It("UT-DS-1088-P6-009: scoring subquery must use explicit column list, not SELECT *", func() {
			// The scoring query has nested SELECT * FROM (SELECT *, ...).
			// Both inner and outer SELECTs must use explicit columns.
			// The regex matches if the query contains explicit column names.

			sqlMock.ExpectQuery(`SELECT COUNT`).
				WithArgs("restart").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

			// The scoring subquery: should use explicit columns, not SELECT *
			sqlMock.ExpectQuery(workflowColumnsRegex).
				WithArgs("restart", 0, 10).
				WillReturnRows(sqlmock.NewRows(append(workflowColumns,
					"detected_label_boost", "custom_label_boost",
					"label_penalty", "final_score")))

			_, _, err := repo.ListWorkflowsByActionType(ctx, "restart", nil, 0, 10)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("GetWorkflowWithContextFilters (discovery.go:237)", func() {
		It("UT-DS-1088-P6-013: query must use explicit column list, not SELECT *", func() {
			// When filters is nil, GetWorkflowWithContextFilters delegates to GetByID,
			// so we test with nil filters which exercises the GetByID path.
			sqlMock.ExpectQuery(workflowColumnsRegex).
				WithArgs("test-uuid").
				WillReturnRows(emptyWorkflowRows())

			_, err := repo.GetWorkflowWithContextFilters(ctx, "test-uuid", nil)
			Expect(errors.Is(err, workflowrepo.ErrNotFound)).To(BeTrue(),
				"Issue #1674: an empty result set is expected not-found, not a real error")
		})
	})
})
