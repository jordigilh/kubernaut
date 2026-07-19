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
				"expected_success_rate",
				"created_at", "updated_at",
			}).AddRow(
				"wf-1", "restart-pod", "v1", "1", "Restart Pod", []byte(`{"what":"restart"}`),
				"content-body", "hash1", "RestartPod", "Active",
				[]byte("{}"), []byte("{}"), []byte("{}"),
				"tekton", []byte("{}"),
				"team-a", "team-a",
				true,
				0.9,
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

	// GetVersionsByName (UT-DS-6f-004) was removed by Issue #1661 Phase F:
	// it is a Postgres-only "dying" method with zero production callers
	// post-Phase-B and no cache-backed equivalent (DD-WORKFLOW-018 makes
	// metadata.name the workflow's sole identity, eliminating the "versions
	// of a name" concept this method existed to enumerate) -- pruned outright
	// in Phase C, not carried forward.

	// UpdateSuccessMetrics (UT-DS-6f-005..008) was removed by Issue #1661
	// Change 7 (DD-WORKFLOW-018): success metrics are no longer a stored,
	// UPDATE-maintained catalog column -- see
	// pkg/datastorage/repository/audit_events_success_metrics_test.go for the
	// on-demand audit_events aggregation that replaces it.
})
