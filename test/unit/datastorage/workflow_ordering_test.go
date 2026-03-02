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

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
)

// ========================================
// WORKFLOW CATALOG DETERMINISTIC ORDERING TESTS
// #213: All workflow catalog queries must include workflow_id ASC tiebreaker
// Authority: DD-WORKFLOW-016 line 653
// ========================================
var _ = Describe("Workflow Catalog Deterministic Ordering (#213)", func() {
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

	Describe("SearchByLabels", func() {
		It("UT-DS-213-001: ORDER BY must use workflow_id ASC tiebreaker per DD-WORKFLOW-016", func() {
			// The scored_workflows subquery must order by final_score DESC, workflow_id ASC
			sqlMock.ExpectQuery(`ORDER BY final_score DESC, workflow_id ASC`).
				WillReturnRows(sqlmock.NewRows([]string{
					"workflow_id", "workflow_name", "version", "name", "description",
					"content", "content_hash", "action_type", "status",
					"labels", "detected_labels", "custom_labels",
					"execution_engine", "execution_bundle",
					"owner", "maintainer",
					"is_latest_version",
					"expected_success_rate", "actual_success_rate",
					"total_executions", "successful_executions",
					"created_at", "updated_at",
					"detected_label_boost", "custom_label_boost", "label_penalty", "final_score",
				}))

			request := &models.WorkflowSearchRequest{
				Filters: &models.WorkflowSearchFilters{
					SignalName:  "HighCPU",
					Severity:    "critical",
					Component:   "api-server",
					Environment: "production",
					Priority:    "P1",
				},
				MinScore: 0.5,
				TopK:     10,
			}

			_, err := repo.SearchByLabels(ctx, request)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("ListWorkflowsByActionType", func() {
		It("UT-DS-220-001: ORDER BY must use final_score DESC, workflow_id ASC per DD-WORKFLOW-016", func() {
			// Count query
			sqlMock.ExpectQuery(`SELECT COUNT`).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

			// #220: Main query must compute final_score and order by it
			sqlMock.ExpectQuery(`ORDER BY final_score DESC, workflow_id ASC`).
				WillReturnRows(sqlmock.NewRows([]string{
					"workflow_id", "workflow_name", "version", "name", "description",
					"content", "content_hash", "action_type", "status",
					"labels", "detected_labels", "custom_labels",
					"execution_engine", "execution_bundle",
					"owner", "maintainer",
					"is_latest_version",
					"expected_success_rate", "actual_success_rate",
					"total_executions", "successful_executions",
					"created_at", "updated_at",
					"detected_label_boost", "custom_label_boost", "label_penalty", "final_score",
				}))

			_, _, err := repo.ListWorkflowsByActionType(ctx, "ScaleUp", nil, 0, 10)
			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-DS-220-002: query must include final_score computation with scoring subquery", func() {
			// Count query
			sqlMock.ExpectQuery(`SELECT COUNT`).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

			// #220: Query must include final_score computation
			sqlMock.ExpectQuery(`final_score`).
				WillReturnRows(sqlmock.NewRows([]string{
					"workflow_id", "workflow_name", "version", "name", "description",
					"content", "content_hash", "action_type", "status",
					"labels", "detected_labels", "custom_labels",
					"execution_engine", "execution_bundle",
					"owner", "maintainer",
					"is_latest_version",
					"expected_success_rate", "actual_success_rate",
					"total_executions", "successful_executions",
					"created_at", "updated_at",
					"detected_label_boost", "custom_label_boost", "label_penalty", "final_score",
				}))

			filters := &models.WorkflowDiscoveryFilters{
				Severity:  "critical",
				Component: "Deployment",
				DetectedLabels: &models.DetectedLabels{
					GitOpsManaged: true,
					GitOpsTool:    "argocd",
				},
			}
			_, _, err := repo.ListWorkflowsByActionType(ctx, "ScaleUp", filters, 0, 10)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("GetVersionsByName", func() {
		It("UT-DS-213-003: ORDER BY must include workflow_id ASC tiebreaker", func() {
			sqlMock.ExpectQuery(`ORDER BY created_at DESC, workflow_id ASC`).
				WillReturnRows(sqlmock.NewRows([]string{
					"workflow_id", "workflow_name", "version", "name", "description",
					"content", "content_hash", "action_type", "status",
					"labels", "detected_labels", "custom_labels",
					"execution_engine", "execution_bundle",
					"owner", "maintainer",
					"is_latest_version",
					"expected_success_rate", "actual_success_rate",
					"total_executions", "successful_executions",
					"created_at", "updated_at",
				}))

			_, err := repo.GetVersionsByName(ctx, "restart-pod")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("List", func() {
		It("UT-DS-213-004: ORDER BY must include workflow_id ASC tiebreaker", func() {
			// Count query
			sqlMock.ExpectQuery(`SELECT COUNT`).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

			// Main query must include workflow_id ASC
			sqlMock.ExpectQuery(`ORDER BY created_at DESC, workflow_id ASC`).
				WillReturnRows(sqlmock.NewRows([]string{
					"workflow_id", "workflow_name", "version", "name", "description",
					"content", "content_hash", "action_type", "status",
					"labels", "detected_labels", "custom_labels",
					"execution_engine", "execution_bundle",
					"owner", "maintainer",
					"is_latest_version",
					"expected_success_rate", "actual_success_rate",
					"total_executions", "successful_executions",
					"created_at", "updated_at",
				}))

			_, _, err := repo.List(ctx, nil, 10, 0)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
