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
	"net/http"
	"net/http/httptest"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubelog "github.com/jordigilh/kubernaut/pkg/log"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// ========================================
// ISSUE #1674: NOT-FOUND VS REAL-ERROR DISTINCTION
// ========================================
// Characterization + regression tests for the two real bugs found while
// refactoring workflowRepo.GetByID's (nil, nil) "not found" contract to the
// workflow.ErrNotFound sentinel:
//
//  1. HandleUpdateWorkflow (workflow_update_lifecycle_handlers.go): with the
//     old (nil, nil) contract, `err != nil` never caught the not-found case,
//     so a nil *models.RemediationWorkflow reached applyMutableWorkflowUpdate
//     and dereferenced workflow.Status — a nil-pointer panic (recovered as
//     an HTTP 500) whenever the PATCH body set "status". Now GetByID returns
//     workflow.ErrNotFound, which is checked before use.
//  2. fetchWorkflowByIDWithGate (workflow_query_handlers.go): compared
//     err.Error() against a hand-built "workflow not found: <id>" string
//     that GetByID/GetWorkflowWithContextFilters never actually produced —
//     dead code. The real "not found" signal was the `wf == nil` check.
//     Both handlers also previously collapsed ANY GetByID error (including
//     real DB outages) into a 404, masking failures. Both are fixed by
//     checking errors.Is(err, workflow.ErrNotFound) and reserving 500 for
//     everything else.
//
// These tests use a real *repository.WorkflowRepository backed by sqlmock
// (rather than the interface test doubles in workflow_lifecycle_handler_test.go)
// because HandleUpdateWorkflow and HandleGetWorkflowByID call through
// h.workflowRepo, which is concretely typed as *repository.WorkflowRepository.
// ========================================

var _ = Describe("Issue #1674: workflow not-found vs real-error distinction", func() {
	var (
		mockDB  *sql.DB
		sqlMock sqlmock.Sqlmock
		repo    *repository.WorkflowRepository
	)

	BeforeEach(func() {
		var err error
		mockDB, sqlMock, err = sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		Expect(err).ToNot(HaveOccurred())

		logger := kubelog.NewLogger(kubelog.DevelopmentOptions())
		sqlxDB := sqlx.NewDb(mockDB, "sqlmock")
		repo = repository.NewWorkflowRepository(sqlxDB, logger)
	})

	AfterEach(func() {
		Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
		_ = mockDB.Close()
	})

	getByIDQuery := `SELECT .* FROM remediation_workflow_catalog WHERE workflow_id = \$1`

	Describe("PATCH /api/v1/workflows/{workflowID} (HandleUpdateWorkflow)", func() {
		It("UT-DS-1674-005: returns 404, not a nil-deref panic, when the workflow is not found and the request sets status", func() {
			sqlMock.ExpectQuery(getByIDQuery).WillReturnError(sql.ErrNoRows)

			handler := server.NewHandler(server.WithWorkflowRepository(repo))
			req := reqWithWorkflowID("", `{"status": "Disabled", "disabledReason": "test"}`)
			rr := httptest.NewRecorder()

			Expect(func() { handler.HandleUpdateWorkflow(rr, req) }).ToNot(Panic(),
				"Issue #1674: previously a nil workflow reached applyMutableWorkflowUpdate and panicked on workflow.Status")
			Expect(rr.Code).To(Equal(http.StatusNotFound))
		})

		It("UT-DS-1674-006: returns 500, not a misleading 404, when GetByID fails with a real database error", func() {
			sqlMock.ExpectQuery(getByIDQuery).WillReturnError(errors.New("connection refused"))

			handler := server.NewHandler(server.WithWorkflowRepository(repo))
			req := reqWithWorkflowID("", `{"status": "Disabled", "disabledReason": "test"}`)
			rr := httptest.NewRecorder()

			handler.HandleUpdateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusInternalServerError),
				"Issue #1674: a real DB error must not be reported as 404 Not Found")
		})
	})

	Describe("GET /api/v1/workflows/{workflowID} (HandleGetWorkflowByID)", func() {
		getRequest := func() *http.Request {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/"+testWorkflowID, nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("workflowID", testWorkflowID)
			return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		}

		It("UT-DS-1674-007: returns 404 when the workflow does not exist (dead-code string check removed)", func() {
			sqlMock.ExpectQuery(getByIDQuery).WillReturnError(sql.ErrNoRows)

			handler := server.NewHandler(server.WithWorkflowRepository(repo))
			rr := httptest.NewRecorder()

			handler.HandleGetWorkflowByID(rr, getRequest())

			Expect(rr.Code).To(Equal(http.StatusNotFound))
		})

		It("UT-DS-1674-008: returns 500, not a misleading 404, when GetByID fails with a real database error", func() {
			sqlMock.ExpectQuery(getByIDQuery).WillReturnError(errors.New("connection refused"))

			handler := server.NewHandler(server.WithWorkflowRepository(repo))
			rr := httptest.NewRecorder()

			handler.HandleGetWorkflowByID(rr, getRequest())

			Expect(rr.Code).To(Equal(http.StatusInternalServerError),
				"Issue #1674: a real DB error must not be reported as 404 Not Found")
		})
	})
})
