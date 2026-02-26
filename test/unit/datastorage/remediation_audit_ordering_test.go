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
// REMEDIATION AUDIT DETERMINISTIC ORDERING TESTS
// #213: ListRemediationAudits must include id DESC tiebreaker
// Authority: Paginated queries on remediation_audit need deterministic ordering
// ========================================
var _ = Describe("Remediation Audit Deterministic Ordering (#213)", func() {
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

	Describe("ListRemediationAudits", func() {
		It("UT-DS-213-006: ORDER BY must include id DESC tiebreaker for pagination correctness", func() {
			sqlMock.ExpectQuery(`ORDER BY start_time DESC, id DESC`).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "name", "namespace", "phase", "action_type", "status",
					"start_time", "end_time", "duration",
					"remediation_request_id", "signal_fingerprint",
					"severity", "environment", "cluster_name", "target_resource",
					"error_message", "metadata",
					"created_at", "updated_at",
				}))

			opts := &query.ListOptions{
				Limit:  10,
				Offset: 0,
			}
			_, err := svc.ListRemediationAudits(ctx, opts)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
