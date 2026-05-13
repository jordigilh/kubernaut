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

package datastorage

import (
	"context"
	"database/sql"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubelog "github.com/jordigilh/kubernaut/pkg/log"

	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
)

// ========================================
// PHASE 6: SELECT * NARROWING TESTS (TP-1088-P1)
// ========================================
//
// Business Requirement: BR-STORAGE-005 (list with filtering)
// Issue: #1088 Phase 6 (Performance)
// TDD Phase: RED — these tests FAIL against current SELECT * implementation
//
// The queries in ListRemediationAudits, ListWorkflowsByActionType, and
// GetWorkflowWithContextFilters currently use SELECT * which is fragile
// against schema drift and fetches unnecessary columns (e.g., embedding).
//
// These tests verify that queries use explicit column lists derived from
// the Go struct db: tags.
//
// ========================================

var _ = Describe("Phase 6: SELECT * Narrowing (TP-1088-P1)", func() {
	var (
		mockDB  *sql.DB
		sqlMock sqlmock.Sqlmock
		svc     *query.Service
		ctx     context.Context
	)

	BeforeEach(func() {
		var err error
		mockDB, sqlMock, err = sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		Expect(err).ToNot(HaveOccurred())

		logger := kubelog.NewLogger(kubelog.DevelopmentOptions())
		ctx = context.Background()

		sqlxDB := sqlx.NewDb(mockDB, "sqlmock")
		svc = query.NewService(sqlxDB, logger)
	})

	AfterEach(func() {
		Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
		_ = mockDB.Close()
	})

	// The 20 columns from RemediationAuditResult db: tags (pkg/datastorage/query/types.go)
	remediationAuditColumns := []string{
		"id", "name", "namespace", "phase", "action_type", "status",
		"start_time", "end_time", "duration",
		"remediation_request_id", "signal_fingerprint",
		"severity", "environment", "cluster_name", "target_resource",
		"error_message", "metadata",
		"created_at", "updated_at",
	}

	Describe("ListRemediationAudits", func() {
		It("UT-DS-1088-P6-001: query must use explicit column list, not SELECT *", func() {
			// RED: The current implementation uses "SELECT * FROM remediation_audit WHERE 1=1".
			// This test expects explicit columns: "SELECT id, name, namespace, ..."
			// It will FAIL because the regex requires explicit column names.

			sqlMock.ExpectQuery(`SELECT id, name, namespace, phase, action_type, status`).
				WillReturnRows(sqlmock.NewRows(remediationAuditColumns))

			opts := &query.ListOptions{
				Limit:  10,
				Offset: 0,
			}
			_, err := svc.ListRemediationAudits(ctx, opts)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
