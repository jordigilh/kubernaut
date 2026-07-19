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

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/actiontype"
)

// ========================================
// Issue #1674 (nilnil sentinel-error refactor), Batch 1: GetByName previously
// returned (nil, nil) on sql.ErrNoRows, ambiguous with a caller forgetting to
// check the error. This package had zero unit-test coverage of GetByName
// before this batch.
// BR-WORKFLOW-007 (ActionType CRD lifecycle).
// ========================================
var _ = Describe("ActionType CRUD sentinel-error refactor (Issue #1674 Batch 1)", func() {
	var (
		mockDB  *sql.DB
		sqlMock sqlmock.Sqlmock
		repo    *actiontype.Repository
		ctx     context.Context
	)

	BeforeEach(func() {
		var err error
		mockDB, sqlMock, err = sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		Expect(err).ToNot(HaveOccurred())

		logger := kubelog.NewLogger(kubelog.DevelopmentOptions())
		ctx = context.Background()

		sqlxDB := sqlx.NewDb(mockDB, "sqlmock")
		repo = actiontype.NewRepository(sqlxDB, logger)
	})

	AfterEach(func() {
		Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
		_ = mockDB.Close()
	})

	Describe("GetByName", func() {
		It("UT-DS-1674-001: returns ErrActionTypeNotFound when no action type matches the name", func() {
			// Issue #1674: GetByName must return a typed sentinel instead of the
			// ambiguous (nil, nil), matching the ErrActionTypeNotFound convention
			// already used by UpdateDescription/Disable/ForceDisable in this file.
			sqlMock.ExpectQuery(`SELECT .* FROM action_type_taxonomy WHERE action_type = \$1`).
				WillReturnError(sql.ErrNoRows)

			at, err := repo.GetByName(ctx, "NoSuchAction")

			Expect(err).To(HaveOccurred(), "Issue #1674: not-found must be a typed error, not (nil, nil)")
			Expect(errors.Is(err, actiontype.ErrActionTypeNotFound)).To(BeTrue())
			Expect(at).To(BeNil())
		})

		It("UT-DS-1674-002: returns the action type when the name matches an existing row", func() {
			rows := sqlmock.NewRows([]string{
				"action_type", "description", "status", "disabled_at", "disabled_by", "created_at", "updated_at",
			}).AddRow(
				"RestartPod", []byte(`{"what":"restart"}`), "Active", nil, nil, time.Now(), time.Now(),
			)
			sqlMock.ExpectQuery(`SELECT .* FROM action_type_taxonomy WHERE action_type = \$1`).
				WillReturnRows(rows)

			at, err := repo.GetByName(ctx, "RestartPod")

			Expect(err).ToNot(HaveOccurred())
			Expect(at).ToNot(BeNil())
			Expect(at.ActionType).To(Equal("RestartPod"))
		})

		It("UT-DS-1674-003: propagates a real database error instead of masking it as not-found", func() {
			sqlMock.ExpectQuery(`SELECT .* FROM action_type_taxonomy WHERE action_type = \$1`).
				WillReturnError(errors.New("connection refused"))

			at, err := repo.GetByName(ctx, "RestartPod")

			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, actiontype.ErrActionTypeNotFound)).To(BeFalse(),
				"a real DB error must not be conflated with the not-found sentinel")
			Expect(at).To(BeNil())
		})
	})

	Describe("Create", func() {
		It("UT-DS-1674-004: creates a new action type when GetByName returns ErrActionTypeNotFound", func() {
			// Regression guard: Create's existing-check must tolerate the new
			// ErrActionTypeNotFound sentinel as "proceed to insert", not treat it
			// as a fatal lookup failure.
			sqlMock.ExpectQuery(`SELECT .* FROM action_type_taxonomy WHERE action_type = \$1`).
				WillReturnError(sql.ErrNoRows)
			sqlMock.ExpectExec(`INSERT INTO action_type_taxonomy`).
				WillReturnResult(sqlmock.NewResult(1, 1))
			rows := sqlmock.NewRows([]string{
				"action_type", "description", "status", "disabled_at", "disabled_by", "created_at", "updated_at",
			}).AddRow(
				"NewAction", []byte(`{"what":"new"}`), "Active", nil, nil, time.Now(), time.Now(),
			)
			sqlMock.ExpectQuery(`SELECT .* FROM action_type_taxonomy WHERE action_type = \$1`).
				WillReturnRows(rows)

			desc := models.ActionTypeDescription{What: "does something new", WhenToUse: "when needed"}
			result, err := repo.Create(ctx, "NewAction", desc, "tester")

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Status).To(Equal("created"))
		})
	})
})
