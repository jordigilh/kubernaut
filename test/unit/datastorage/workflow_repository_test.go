/*
Copyright 2025 Jordi Gil.

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
	"encoding/json"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvector/pgvector-go"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

var _ = Describe("WorkflowRepository", func() {
	var (
		ctx    context.Context
		repo   *repository.WorkflowRepository
		db     *sqlx.DB
		mock   sqlmock.Sqlmock
		logger *zap.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop()

		// Create mock database
		mockDB, mockSQL, err := sqlmock.New()
		Expect(err).ToNot(HaveOccurred())

		db = sqlx.NewDb(mockDB, "sqlmock")
		mock = mockSQL

		// Create repository
		repo = repository.NewWorkflowRepository(db, logger)
	})

	AfterEach(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			Fail(err.Error())
		}
	})

	// ========================================
	// UNIT TEST: Create Method
	// ========================================
	// Purpose: Validate implementation correctness
	// Focus: BEHAVIOR (SQL execution) + CORRECTNESS (error handling)
	// NOT testing: Business value (that's in BR tests)
	//
	// Per TESTING_GUIDELINES.md:
	// - BEHAVIOR: Does the method execute SQL correctly?
	// - CORRECTNESS: Does it handle errors properly?

	Describe("Create", func() {
		type createTestCase struct {
			description      string
			workflowID       string
			version          string
			name             string
			status           string
			isLatestVersion  bool
			mockError        error
			expectError      bool
			expectedErrorMsg string
			testFocus        string // "behavior" or "correctness"
		}

		DescribeTable("should validate behavior and correctness",
			func(tc createTestCase) {
				// ARRANGE: Prepare test workflow
				labels := map[string]interface{}{
					"signal_types":      []string{"MemoryLeak", "OOMKilled"},
					"business_category": "payments",
					"risk_tolerance":    "low",
					"environment":       "production",
				}
				labelsJSON, err := json.Marshal(labels)
				Expect(err).ToNot(HaveOccurred())

				embeddingVec := pgvector.NewVector(make([]float32, 384))
				for i := range embeddingVec.Slice() {
					embeddingVec.Slice()[i] = 0.1
				}

				workflow := &models.RemediationWorkflow{
					WorkflowID:           tc.workflowID,
					Version:              tc.version,
					Name:                 tc.name,
					Description:          "Test workflow description",
					Content:              "apiVersion: tekton.dev/v1...",
					ContentHash:          "abc123def456...",
					Labels:               labelsJSON,
					Embedding:            &embeddingVec,
					Status:               tc.status,
					IsLatestVersion:      tc.isLatestVersion,
					TotalExecutions:      0,
					SuccessfulExecutions: 0,
				}

				// EXPECT: Mock database behavior
				expectation := mock.ExpectExec(`INSERT INTO remediation_workflow_catalog`).
					WithArgs(
						workflow.WorkflowID,
						workflow.Version,
						workflow.Name,
						workflow.Description,
						workflow.Owner,
						workflow.Maintainer,
						workflow.Content,
						workflow.ContentHash,
						workflow.Labels,
						workflow.Embedding,
						workflow.Status,
						workflow.IsLatestVersion,
						workflow.PreviousVersion,
						workflow.VersionNotes,
						workflow.ChangeSummary,
						workflow.ApprovedBy,
						workflow.ApprovedAt,
						workflow.ExpectedSuccessRate,
						workflow.ExpectedDurationSeconds,
						workflow.CreatedBy,
					)

				if tc.mockError != nil {
					expectation.WillReturnError(tc.mockError)
				} else {
					expectation.WillReturnResult(sqlmock.NewResult(1, 1))
				}

				// ACT: Create workflow
				err = repo.Create(ctx, workflow)

				// ASSERT: Verify behavior and correctness
				if tc.expectError {
					Expect(err).To(HaveOccurred(), "Expected error for: %s", tc.description)
					if tc.expectedErrorMsg != "" {
						Expect(err.Error()).To(ContainSubstring(tc.expectedErrorMsg))
					}
				} else {
					Expect(err).ToNot(HaveOccurred(), "Should succeed for: %s", tc.description)
				}
			},
			// ========================================
			// BEHAVIOR TESTS: Does SQL execute correctly?
			// ========================================
			Entry("BEHAVIOR: executes INSERT for active workflow", createTestCase{
				description:     "Validates SQL INSERT execution",
				workflowID:      "pod-oom-recovery",
				version:         "v1.0.0",
				name:            "Pod OOM Recovery",
				status:          "active",
				isLatestVersion: true,
				expectError:     false,
				testFocus:       "behavior",
			}),
			Entry("BEHAVIOR: executes INSERT for disabled workflow", createTestCase{
				description:     "Validates SQL INSERT with disabled status",
				workflowID:      "disabled-workflow",
				version:         "v0.9.0",
				name:            "Disabled Workflow",
				status:          "disabled",
				isLatestVersion: false,
				expectError:     false,
				testFocus:       "behavior",
			}),
			Entry("BEHAVIOR: executes INSERT for deprecated workflow", createTestCase{
				description:     "Validates SQL INSERT with deprecated status",
				workflowID:      "old-workflow",
				version:         "v0.5.0",
				name:            "Old Workflow",
				status:          "deprecated",
				isLatestVersion: false,
				expectError:     false,
				testFocus:       "behavior",
			}),
			Entry("BEHAVIOR: executes INSERT for archived workflow", createTestCase{
				description:     "Validates SQL INSERT with archived status",
				workflowID:      "archived-workflow",
				version:         "v0.1.0",
				name:            "Archived Workflow",
				status:          "archived",
				isLatestVersion: false,
				expectError:     false,
				testFocus:       "behavior",
			}),
			// ========================================
			// CORRECTNESS TESTS: Error handling
			// ========================================
			Entry("CORRECTNESS: handles database connection error", createTestCase{
				description:      "Validates error handling for connection failure",
				workflowID:       "error-workflow",
				version:          "v1.0.0",
				name:             "Error Workflow",
				status:           "active",
				isLatestVersion:  true,
				mockError:        sqlmock.ErrCancelled,
				expectError:      true,
				expectedErrorMsg: "failed to create workflow",
				testFocus:        "correctness",
			}),
		)
	})

	// ========================================
	// UNIT TEST: GetByID Method
	// ========================================
	// Purpose: Validate retrieval behavior and error handling
	// Focus: BEHAVIOR (SQL SELECT) + CORRECTNESS (not found, errors)

	Describe("GetByID", func() {
		type getByIDTestCase struct {
			description      string
			workflowID       string
			version          string
			mockRows         *sqlmock.Rows
			mockError        error
			expectError      bool
			expectedErrorMsg string
			testFocus        string
		}

		DescribeTable("should validate behavior and correctness",
			func(tc getByIDTestCase) {
				// ARRANGE
				query := `SELECT \* FROM remediation_workflow_catalog WHERE workflow_id = \$1 AND version = \$2`

				// EXPECT: Mock database behavior
				if tc.mockError != nil {
					mock.ExpectQuery(query).
						WithArgs(tc.workflowID, tc.version).
						WillReturnError(tc.mockError)
				} else if tc.mockRows != nil {
					mock.ExpectQuery(query).
						WithArgs(tc.workflowID, tc.version).
						WillReturnRows(tc.mockRows)
				}

				// ACT
				workflow, err := repo.GetByID(ctx, tc.workflowID, tc.version)

				// ASSERT
				if tc.expectError {
					Expect(err).To(HaveOccurred(), "Expected error for: %s", tc.description)
					if tc.expectedErrorMsg != "" {
						Expect(err.Error()).To(ContainSubstring(tc.expectedErrorMsg))
					}
					Expect(workflow).To(BeNil())
				} else {
					Expect(err).ToNot(HaveOccurred(), "Should succeed for: %s", tc.description)
					Expect(workflow).ToNot(BeNil())
					Expect(workflow.WorkflowID).To(Equal(tc.workflowID))
					Expect(workflow.Version).To(Equal(tc.version))
				}
			},
			// ========================================
			// BEHAVIOR TESTS: Does SELECT execute correctly?
			// ========================================
			Entry("BEHAVIOR: retrieves workflow by ID and version", getByIDTestCase{
				description: "Successful retrieval",
				workflowID:  "pod-oom-recovery",
				version:     "v1.0.0",
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "version", "name", "description", "content", "content_hash",
					"labels", "embedding", "status", "is_latest_version",
					"disabled_at", "disabled_by", "disabled_reason",
					"previous_version", "deprecation_notice",
					"version_notes", "change_summary", "approved_by", "approved_at",
					"expected_success_rate", "expected_duration_seconds",
					"actual_success_rate", "total_executions", "successful_executions",
					"created_at", "updated_at", "created_by", "updated_by",
				}).AddRow(
					"pod-oom-recovery", "v1.0.0", "Pod OOM Recovery", "Test description",
					"content", "hash123", []byte(`{}`), nil, "active", true,
					nil, nil, nil, nil, nil, nil, nil, nil, nil,
					nil, nil, nil, 0, 0,
					time.Now(), time.Now(), nil, nil,
				),
				expectError: false,
				testFocus:   "behavior",
			}),
			// ========================================
			// CORRECTNESS TESTS: Error handling
			// ========================================
			Entry("CORRECTNESS: returns error when workflow not found", getByIDTestCase{
				description:      "Workflow does not exist",
				workflowID:       "non-existent",
				version:          "v1.0.0",
				mockError:        sql.ErrNoRows,
				expectError:      true,
				expectedErrorMsg: "workflow not found",
				testFocus:        "correctness",
			}),
			Entry("CORRECTNESS: handles database connection error", getByIDTestCase{
				description:      "Database connection failure",
				workflowID:       "error-workflow",
				version:          "v1.0.0",
				mockError:        sqlmock.ErrCancelled,
				expectError:      true,
				expectedErrorMsg: "failed to get workflow",
				testFocus:        "correctness",
			}),
		)
	})

	// ========================================
	// UNIT TEST: SearchByEmbedding Method
	// ========================================
	// Purpose: Validate semantic search behavior and error handling
	// Focus: BEHAVIOR (pgvector search) + CORRECTNESS (validation, errors)

	Describe("SearchByEmbedding", func() {
		type searchTestCase struct {
			description      string
			embedding        *pgvector.Vector
			topK             int
			minSimilarity    *float64
			includeDisabled  bool
			mockRows         *sqlmock.Rows
			mockError        error
			expectError      bool
			expectedErrorMsg string
			expectedCount    int
			testFocus        string
		}

		DescribeTable("should validate behavior and correctness",
			func(tc searchTestCase) {
				// ARRANGE
				request := &models.WorkflowSearchRequest{
					Query:           "test query",
					Embedding:       tc.embedding,
					TopK:            tc.topK,
					MinSimilarity:   tc.minSimilarity,
					IncludeDisabled: tc.includeDisabled,
				}

				// EXPECT: Mock database behavior
				if tc.mockError != nil {
					mock.ExpectQuery(`SELECT`).
						WillReturnError(tc.mockError)
				} else if tc.mockRows != nil {
					mock.ExpectQuery(`SELECT`).
						WillReturnRows(tc.mockRows)
				}

				// ACT
				response, err := repo.SearchByEmbedding(ctx, request)

				// ASSERT
				if tc.expectError {
					Expect(err).To(HaveOccurred(), "Expected error for: %s", tc.description)
					if tc.expectedErrorMsg != "" {
						Expect(err.Error()).To(ContainSubstring(tc.expectedErrorMsg))
					}
					Expect(response).To(BeNil())
				} else {
					Expect(err).ToNot(HaveOccurred(), "Should succeed for: %s", tc.description)
					Expect(response).ToNot(BeNil())
					Expect(response.Workflows).To(HaveLen(tc.expectedCount))
					Expect(response.Query).To(Equal("test query"))
				}
			},
			// ========================================
			// BEHAVIOR TESTS: Does pgvector search work?
			// ========================================
			Entry("BEHAVIOR: executes semantic search with results", searchTestCase{
				description: "Successful semantic search",
				embedding:   func() *pgvector.Vector { v := pgvector.NewVector(make([]float32, 384)); return &v }(),
				topK:        5,
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "version", "name", "description", "content", "content_hash",
					"labels", "embedding", "status", "is_latest_version",
					"disabled_at", "disabled_by", "disabled_reason",
					"previous_version", "deprecation_notice",
					"version_notes", "change_summary", "approved_by", "approved_at",
					"expected_success_rate", "expected_duration_seconds",
					"actual_success_rate", "total_executions", "successful_executions",
					"created_at", "updated_at", "created_by", "updated_by",
					"similarity_score",
				}).AddRow(
					"pod-oom-recovery", "v1.0.0", "Pod OOM Recovery", "Test description",
					"content", "hash123", []byte(`{}`), nil, "active", true,
					nil, nil, nil, nil, nil, nil, nil, nil, nil,
					nil, nil, nil, 0, 0,
					time.Now(), time.Now(), nil, nil,
					0.95, // similarity score
				),
				expectError:   false,
				expectedCount: 1,
				testFocus:     "behavior",
			}),
			Entry("BEHAVIOR: returns empty results when no matches", searchTestCase{
				description:   "No matching workflows",
				embedding:     func() *pgvector.Vector { v := pgvector.NewVector(make([]float32, 384)); return &v }(),
				topK:          5,
				mockRows:      sqlmock.NewRows([]string{"workflow_id", "similarity_score"}),
				expectError:   false,
				expectedCount: 0,
				testFocus:     "behavior",
			}),
			// ========================================
			// CORRECTNESS TESTS: Validation & errors
			// ========================================
			Entry("CORRECTNESS: returns error when embedding is nil", searchTestCase{
				description:      "Missing embedding vector",
				embedding:        nil,
				topK:             5,
				expectError:      true,
				expectedErrorMsg: "embedding is required",
				testFocus:        "correctness",
			}),
			Entry("CORRECTNESS: handles database query error", searchTestCase{
				description:      "Database query failure",
				embedding:        func() *pgvector.Vector { v := pgvector.NewVector(make([]float32, 384)); return &v }(),
				topK:             5,
				mockError:        sqlmock.ErrCancelled,
				expectError:      true,
				expectedErrorMsg: "failed to search workflows",
				testFocus:        "correctness",
			}),
		)
	})

	// ========================================
	// TDD CYCLE 1: Mandatory Label Filtering
	// ========================================
	// Business Requirement: BR-STORAGE-013 (Hybrid Weighted Scoring)
	// Design Decision: DD-WORKFLOW-004 v1.1 (Hybrid Weighted Label Scoring)
	// Authority: DD-LLM-001 v1.0 (MCP Search Taxonomy)
	//
	// Purpose: Validate mandatory label filtering (signal-type, severity)
	// Focus: BEHAVIOR (SQL WHERE clause) + CORRECTNESS (filtering logic)
	//
	// TDD Phase: RED → GREEN → REFACTOR
	// Expected: FAIL initially (not implemented yet)

	Describe("SearchByEmbedding - Mandatory Label Filtering", func() {
		type mandatoryFilterTestCase struct {
			description       string
			signalType        string
			severity          string
			mockRows          *sqlmock.Rows
			expectedCount     int
			expectedWorkflows []string // workflow IDs expected in results
			testFocus         string
		}

		DescribeTable("should filter workflows by mandatory labels (signal-type, severity)",
			func(tc mandatoryFilterTestCase) {
				// ARRANGE: Create search request with mandatory filters
				embeddingVec := pgvector.NewVector(make([]float32, 384))
				for i := range embeddingVec.Slice() {
					embeddingVec.Slice()[i] = 0.1
				}

				request := &models.WorkflowSearchRequest{
					Query:     "test query",
					Embedding: &embeddingVec,
					TopK:      10,
					Filters: &models.WorkflowSearchFilters{
						SignalType: tc.signalType,
						Severity:   tc.severity,
					},
				}

				// EXPECT: Mock database to return workflows matching filters
				// SQL should include: WHERE labels->>'signal-type' = $2 AND labels->>'severity' = $3
				mock.ExpectQuery(`SELECT`).
					WillReturnRows(tc.mockRows)

				// ACT: Execute search
				response, err := repo.SearchByEmbedding(ctx, request)

				// ASSERT: Verify filtering behavior
				Expect(err).ToNot(HaveOccurred(), "Should succeed for: %s", tc.description)
				Expect(response).ToNot(BeNil())
				Expect(response.Workflows).To(HaveLen(tc.expectedCount), "Expected %d workflows for: %s", tc.expectedCount, tc.description)

				// Verify correct workflows returned
				if tc.expectedCount > 0 {
					for i, expectedID := range tc.expectedWorkflows {
						Expect(response.Workflows[i].Workflow.WorkflowID).To(Equal(expectedID),
							"Workflow at position %d should be %s", i, expectedID)
					}
				}
			},
			// ========================================
			// BEHAVIOR TEST: Mandatory filtering
			// ========================================
			Entry("BEHAVIOR: filters by critical severity", mandatoryFilterTestCase{
				description: "Returns only critical severity workflows",
				signalType:  "OOMKilled",
				severity:    "critical",
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "version", "name", "description", "content", "content_hash",
					"labels", "embedding", "status", "is_latest_version",
					"disabled_at", "disabled_by", "disabled_reason",
					"previous_version", "deprecation_notice",
					"version_notes", "change_summary", "approved_by", "approved_at",
					"expected_success_rate", "expected_duration_seconds",
					"actual_success_rate", "total_executions", "successful_executions",
					"created_at", "updated_at", "created_by", "updated_by",
					"base_similarity", "label_boost", "label_penalty", "final_score",
				}).AddRow(
					"oom-critical-workflow", "v1.0.0", "OOM Critical Recovery", "Critical OOM recovery",
					"content", "hash123", []byte(`{"signal-type":"OOMKilled","severity":"critical"}`),
					nil, "active", true,
					nil, nil, nil, nil, nil, nil, nil, nil, nil,
					nil, nil, nil, 0, 0,
					time.Now(), time.Now(), nil, nil,
					0.95, 0.0, 0.0, 0.95, // base_similarity, label_boost, label_penalty, final_score
				),
				expectedCount:     1,
				expectedWorkflows: []string{"oom-critical-workflow"},
				testFocus:         "behavior",
			}),
			Entry("BEHAVIOR: filters by high severity", mandatoryFilterTestCase{
				description: "Returns only high severity workflows",
				signalType:  "OOMKilled",
				severity:    "high",
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "version", "name", "description", "content", "content_hash",
					"labels", "embedding", "status", "is_latest_version",
					"disabled_at", "disabled_by", "disabled_reason",
					"previous_version", "deprecation_notice",
					"version_notes", "change_summary", "approved_by", "approved_at",
					"expected_success_rate", "expected_duration_seconds",
					"actual_success_rate", "total_executions", "successful_executions",
					"created_at", "updated_at", "created_by", "updated_by",
					"base_similarity", "label_boost", "label_penalty", "final_score",
				}).AddRow(
					"oom-high-workflow", "v1.0.0", "OOM High Recovery", "High OOM recovery",
					"content", "hash456", []byte(`{"signal-type":"OOMKilled","severity":"high"}`),
					nil, "active", true,
					nil, nil, nil, nil, nil, nil, nil, nil, nil,
					nil, nil, nil, 0, 0,
					time.Now(), time.Now(), nil, nil,
					0.90, 0.0, 0.0, 0.90,
				),
				expectedCount:     1,
				expectedWorkflows: []string{"oom-high-workflow"},
				testFocus:         "behavior",
			}),
			Entry("BEHAVIOR: returns empty when no workflows match mandatory filters", mandatoryFilterTestCase{
				description: "No workflows match critical + MemoryLeak",
				signalType:  "MemoryLeak",
				severity:    "critical",
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "base_similarity", "label_boost", "label_penalty", "final_score",
				}), // Empty result set
				expectedCount:     0,
				expectedWorkflows: []string{},
				testFocus:         "behavior",
			}),
		)
	})

	// ========================================
	// TDD CYCLE 2: Optional Label Boosting
	// ========================================
	// Business Requirement: BR-STORAGE-013 (Hybrid Weighted Scoring)
	// Design Decision: DD-WORKFLOW-004 v1.1 (Hybrid Weighted Label Scoring)
	//
	// Purpose: Validate optional label boosting for matching labels
	// Focus: BEHAVIOR (boost calculation) + CORRECTNESS (boost values)
	//
	// TDD Phase: RED → GREEN → REFACTOR
	// Expected: FAIL initially (boost logic not implemented yet)

	Describe("SearchByEmbedding - Optional Label Boosting", func() {
		type boostTestCase struct {
			description       string
			signalType        string
			severity          string
			resourceMgmt      *string
			gitOpsTool        *string
			environment       *string
			businessCategory  *string
			priority          *string
			riskTolerance     *string
			mockRows          *sqlmock.Rows
			expectedBoost     float64
			expectedWorkflows []string
			testFocus         string
		}

		DescribeTable("should boost workflows with matching optional labels",
			func(tc boostTestCase) {
				// ARRANGE: Create search request with optional filters
				embeddingVec := pgvector.NewVector(make([]float32, 384))
				for i := range embeddingVec.Slice() {
					embeddingVec.Slice()[i] = 0.1
				}

				request := &models.WorkflowSearchRequest{
					Query:     "test query",
					Embedding: &embeddingVec,
					TopK:      10,
					Filters: &models.WorkflowSearchFilters{
						SignalType:         tc.signalType,
						Severity:           tc.severity,
						ResourceManagement: tc.resourceMgmt,
						GitOpsTool:         tc.gitOpsTool,
						Environment:        tc.environment,
						BusinessCategory:   tc.businessCategory,
						Priority:           tc.priority,
						RiskTolerance:      tc.riskTolerance,
					},
				}

				// EXPECT: Mock database to return workflows with boost scores
				mock.ExpectQuery(`SELECT`).
					WillReturnRows(tc.mockRows)

				// ACT: Execute search
				response, err := repo.SearchByEmbedding(ctx, request)

				// ASSERT: Verify boosting behavior
				Expect(err).ToNot(HaveOccurred(), "Should succeed for: %s", tc.description)
				Expect(response).ToNot(BeNil())
				Expect(response.Workflows).To(HaveLen(len(tc.expectedWorkflows)), "Expected %d workflows for: %s", len(tc.expectedWorkflows), tc.description)

				// Verify boost score
				if len(response.Workflows) > 0 {
					Expect(response.Workflows[0].LabelBoost).To(BeNumerically(">=", tc.expectedBoost),
						"Expected boost >= %.2f for: %s", tc.expectedBoost, tc.description)

					// Verify correct workflow returned
					Expect(response.Workflows[0].Workflow.WorkflowID).To(Equal(tc.expectedWorkflows[0]),
						"Expected workflow %s for: %s", tc.expectedWorkflows[0], tc.description)
				}
			},
			// ========================================
			// BEHAVIOR TEST: Resource management boost
			// ========================================
			Entry("BEHAVIOR: boosts gitops workflow when resource-management=gitops", boostTestCase{
				description:  "GitOps workflow gets +0.10 boost",
				signalType:   "OOMKilled",
				severity:     "critical",
				resourceMgmt: func() *string { s := "gitops"; return &s }(),
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "version", "name", "description", "content", "content_hash",
					"labels", "embedding", "status", "is_latest_version",
					"disabled_at", "disabled_by", "disabled_reason",
					"previous_version", "deprecation_notice",
					"version_notes", "change_summary", "approved_by", "approved_at",
					"expected_success_rate", "expected_duration_seconds",
					"actual_success_rate", "total_executions", "successful_executions",
					"created_at", "updated_at", "created_by", "updated_by",
					"base_similarity", "label_boost", "label_penalty", "final_score",
				}).AddRow(
					"oom-gitops-workflow", "v1.0.0", "OOM GitOps Recovery", "GitOps OOM recovery",
					"content", "hash123", []byte(`{"signal-type":"OOMKilled","severity":"critical","resource-management":"gitops"}`),
					nil, "active", true,
					nil, nil, nil, nil, nil, nil, nil, nil, nil,
					nil, nil, nil, 0, 0,
					time.Now(), time.Now(), nil, nil,
					0.90, 0.10, 0.0, 1.0, // base=0.90, boost=0.10, penalty=0.0, final=1.0 (capped)
				),
				expectedBoost:     0.10,
				expectedWorkflows: []string{"oom-gitops-workflow"},
				testFocus:         "behavior",
			}),
			Entry("BEHAVIOR: boosts workflow with multiple matching optional labels", boostTestCase{
				description:      "Multiple label matches get cumulative boost",
				signalType:       "OOMKilled",
				severity:         "critical",
				resourceMgmt:     func() *string { s := "gitops"; return &s }(),
				environment:      func() *string { s := "production"; return &s }(),
				businessCategory: func() *string { s := "payments"; return &s }(),
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "version", "name", "description", "content", "content_hash",
					"labels", "embedding", "status", "is_latest_version",
					"disabled_at", "disabled_by", "disabled_reason",
					"previous_version", "deprecation_notice",
					"version_notes", "change_summary", "approved_by", "approved_at",
					"expected_success_rate", "expected_duration_seconds",
					"actual_success_rate", "total_executions", "successful_executions",
					"created_at", "updated_at", "created_by", "updated_by",
					"base_similarity", "label_boost", "label_penalty", "final_score",
				}).AddRow(
					"oom-multi-label-workflow", "v1.0.0", "OOM Multi-Label Recovery", "Multi-label OOM recovery",
					"content", "hash456", []byte(`{"signal-type":"OOMKilled","severity":"critical","resource-management":"gitops","environment":"production","business-category":"payments"}`),
					nil, "active", true,
					nil, nil, nil, nil, nil, nil, nil, nil, nil,
					nil, nil, nil, 0, 0,
					time.Now(), time.Now(), nil, nil,
					0.85, 0.26, 0.0, 1.0, // base=0.85, boost=0.10+0.08+0.08=0.26, penalty=0.0, final=1.0 (capped)
				),
				expectedBoost:     0.26, // resource-management(0.10) + environment(0.08) + business-category(0.08)
				expectedWorkflows: []string{"oom-multi-label-workflow"},
				testFocus:         "behavior",
			}),
			Entry("BEHAVIOR: no boost when optional labels don't match", boostTestCase{
				description:  "Manual workflow gets 0.0 boost when searching for gitops",
				signalType:   "OOMKilled",
				severity:     "critical",
				resourceMgmt: func() *string { s := "gitops"; return &s }(),
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "version", "name", "description", "content", "content_hash",
					"labels", "embedding", "status", "is_latest_version",
					"disabled_at", "disabled_by", "disabled_reason",
					"previous_version", "deprecation_notice",
					"version_notes", "change_summary", "approved_by", "approved_at",
					"expected_success_rate", "expected_duration_seconds",
					"actual_success_rate", "total_executions", "successful_executions",
					"created_at", "updated_at", "created_by", "updated_by",
					"base_similarity", "label_boost", "label_penalty", "final_score",
				}).AddRow(
					"oom-manual-workflow", "v1.0.0", "OOM Manual Recovery", "Manual OOM recovery",
					"content", "hash789", []byte(`{"signal-type":"OOMKilled","severity":"critical","resource-management":"manual"}`),
					nil, "active", true,
					nil, nil, nil, nil, nil, nil, nil, nil, nil,
					nil, nil, nil, 0, 0,
					time.Now(), time.Now(), nil, nil,
					0.88, 0.0, 0.0, 0.88, // base=0.88, boost=0.0 (no match), penalty=0.0, final=0.88
				),
				expectedBoost:     0.0,
				expectedWorkflows: []string{"oom-manual-workflow"},
				testFocus:         "behavior",
			}),
		)
	})

	// ========================================
	// TDD CYCLE 3: Optional Label Penalty
	// ========================================
	// Business Requirement: BR-STORAGE-013 (Hybrid Weighted Scoring)
	// Design Decision: DD-WORKFLOW-004 v1.1 (Hybrid Weighted Label Scoring)
	//
	// Purpose: Validate optional label penalty for conflicting labels
	// Focus: BEHAVIOR (penalty calculation) + CORRECTNESS (penalty values)
	//
	// TDD Phase: RED → GREEN → REFACTOR
	// Expected: FAIL initially (penalty logic not implemented yet)

	Describe("SearchByEmbedding - Optional Label Penalty", func() {
		type penaltyTestCase struct {
			description       string
			signalType        string
			severity          string
			resourceMgmt      *string
			gitOpsTool        *string
			mockRows          *sqlmock.Rows
			expectedPenalty   float64
			expectedWorkflows []string
			testFocus         string
		}

		DescribeTable("should penalize workflows with conflicting optional labels",
			func(tc penaltyTestCase) {
				// ARRANGE: Create search request with optional filters
				embeddingVec := pgvector.NewVector(make([]float32, 384))
				for i := range embeddingVec.Slice() {
					embeddingVec.Slice()[i] = 0.1
				}

				request := &models.WorkflowSearchRequest{
					Query:     "test query",
					Embedding: &embeddingVec,
					TopK:      10,
					Filters: &models.WorkflowSearchFilters{
						SignalType:         tc.signalType,
						Severity:           tc.severity,
						ResourceManagement: tc.resourceMgmt,
						GitOpsTool:         tc.gitOpsTool,
					},
				}

				// EXPECT: Mock database to return workflows with penalty scores
				mock.ExpectQuery(`SELECT`).
					WillReturnRows(tc.mockRows)

				// ACT: Execute search
				response, err := repo.SearchByEmbedding(ctx, request)

				// ASSERT: Verify penalty behavior
				Expect(err).ToNot(HaveOccurred(), "Should succeed for: %s", tc.description)
				Expect(response).ToNot(BeNil())
				Expect(response.Workflows).To(HaveLen(len(tc.expectedWorkflows)), "Expected %d workflows for: %s", len(tc.expectedWorkflows), tc.description)

				// Verify penalty score
				if len(response.Workflows) > 0 {
					Expect(response.Workflows[0].LabelPenalty).To(BeNumerically(">=", tc.expectedPenalty),
						"Expected penalty >= %.2f for: %s", tc.expectedPenalty, tc.description)

					// Verify correct workflow returned
					Expect(response.Workflows[0].Workflow.WorkflowID).To(Equal(tc.expectedWorkflows[0]),
						"Expected workflow %s for: %s", tc.expectedWorkflows[0], tc.description)
				}
			},
			// ========================================
			// BEHAVIOR TEST: Resource management penalty
			// ========================================
			Entry("BEHAVIOR: penalizes manual workflow when searching for gitops", penaltyTestCase{
				description:  "Manual workflow gets -0.10 penalty when searching for gitops",
				signalType:   "OOMKilled",
				severity:     "critical",
				resourceMgmt: func() *string { s := "gitops"; return &s }(),
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "version", "name", "description", "content", "content_hash",
					"labels", "embedding", "status", "is_latest_version",
					"disabled_at", "disabled_by", "disabled_reason",
					"previous_version", "deprecation_notice",
					"version_notes", "change_summary", "approved_by", "approved_at",
					"expected_success_rate", "expected_duration_seconds",
					"actual_success_rate", "total_executions", "successful_executions",
					"created_at", "updated_at", "created_by", "updated_by",
					"base_similarity", "label_boost", "label_penalty", "final_score",
				}).AddRow(
					"oom-manual-workflow", "v1.0.0", "OOM Manual Recovery", "Manual OOM recovery",
					"content", "hash123", []byte(`{"signal-type":"OOMKilled","severity":"critical","resource-management":"manual"}`),
					nil, "active", true,
					nil, nil, nil, nil, nil, nil, nil, nil, nil,
					nil, nil, nil, 0, 0,
					time.Now(), time.Now(), nil, nil,
					0.90, 0.0, 0.10, 0.80, // base=0.90, boost=0.0, penalty=0.10, final=0.80
				),
				expectedPenalty:   0.10,
				expectedWorkflows: []string{"oom-manual-workflow"},
				testFocus:         "behavior",
			}),
			Entry("BEHAVIOR: penalizes workflow with multiple conflicting labels", penaltyTestCase{
				description:  "Multiple conflicting labels get cumulative penalty",
				signalType:   "OOMKilled",
				severity:     "critical",
				resourceMgmt: func() *string { s := "gitops"; return &s }(),
				gitOpsTool:   func() *string { s := "argocd"; return &s }(),
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "version", "name", "description", "content", "content_hash",
					"labels", "embedding", "status", "is_latest_version",
					"disabled_at", "disabled_by", "disabled_reason",
					"previous_version", "deprecation_notice",
					"version_notes", "change_summary", "approved_by", "approved_at",
					"expected_success_rate", "expected_duration_seconds",
					"actual_success_rate", "total_executions", "successful_executions",
					"created_at", "updated_at", "created_by", "updated_by",
					"base_similarity", "label_boost", "label_penalty", "final_score",
				}).AddRow(
					"oom-conflict-workflow", "v1.0.0", "OOM Conflict Recovery", "Conflicting OOM recovery",
					"content", "hash456", []byte(`{"signal-type":"OOMKilled","severity":"critical","resource-management":"manual","gitops-tool":"flux"}`),
					nil, "active", true,
					nil, nil, nil, nil, nil, nil, nil, nil, nil,
					nil, nil, nil, 0, 0,
					time.Now(), time.Now(), nil, nil,
					0.92, 0.0, 0.20, 0.72, // base=0.92, boost=0.0, penalty=0.10+0.10=0.20, final=0.72
				),
				expectedPenalty:   0.20, // resource-management(0.10) + gitops-tool(0.10)
				expectedWorkflows: []string{"oom-conflict-workflow"},
				testFocus:         "behavior",
			}),
			Entry("BEHAVIOR: no penalty when labels match", penaltyTestCase{
				description:  "GitOps workflow gets 0.0 penalty when searching for gitops",
				signalType:   "OOMKilled",
				severity:     "critical",
				resourceMgmt: func() *string { s := "gitops"; return &s }(),
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "version", "name", "description", "content", "content_hash",
					"labels", "embedding", "status", "is_latest_version",
					"disabled_at", "disabled_by", "disabled_reason",
					"previous_version", "deprecation_notice",
					"version_notes", "change_summary", "approved_by", "approved_at",
					"expected_success_rate", "expected_duration_seconds",
					"actual_success_rate", "total_executions", "successful_executions",
					"created_at", "updated_at", "created_by", "updated_by",
					"base_similarity", "label_boost", "label_penalty", "final_score",
				}).AddRow(
					"oom-gitops-workflow", "v1.0.0", "OOM GitOps Recovery", "GitOps OOM recovery",
					"content", "hash789", []byte(`{"signal-type":"OOMKilled","severity":"critical","resource-management":"gitops"}`),
					nil, "active", true,
					nil, nil, nil, nil, nil, nil, nil, nil, nil,
					nil, nil, nil, 0, 0,
					time.Now(), time.Now(), nil, nil,
					0.88, 0.10, 0.0, 0.98, // base=0.88, boost=0.10, penalty=0.0 (no conflict), final=0.98
				),
				expectedPenalty:   0.0,
				expectedWorkflows: []string{"oom-gitops-workflow"},
				testFocus:         "behavior",
			}),
		)
	})

	// ========================================
	// TDD CYCLE 4: Final Score Calculation
	// ========================================
	// Business Requirement: BR-STORAGE-013 (Hybrid Weighted Scoring)
	// Design Decision: DD-WORKFLOW-004 v1.1 (Hybrid Weighted Label Scoring)
	//
	// Purpose: Validate final score calculation and capping at 1.0
	// Focus: BEHAVIOR (score formula) + CORRECTNESS (capping logic)
	//
	// TDD Phase: RED → GREEN → REFACTOR
	// Expected: FAIL initially (capping logic not implemented yet)

	Describe("SearchByEmbedding - Final Score Calculation", func() {
		type finalScoreTestCase struct {
			description        string
			signalType         string
			severity           string
			resourceMgmt       *string
			environment        *string
			mockRows           *sqlmock.Rows
			expectedFinalScore float64
			expectedCapped     bool
			expectedWorkflows  []string
			testFocus          string
		}

		DescribeTable("should calculate final score as base + boost - penalty, capped at 1.0",
			func(tc finalScoreTestCase) {
				// ARRANGE: Create search request
				embeddingVec := pgvector.NewVector(make([]float32, 384))
				for i := range embeddingVec.Slice() {
					embeddingVec.Slice()[i] = 0.1
				}

				request := &models.WorkflowSearchRequest{
					Query:     "test query",
					Embedding: &embeddingVec,
					TopK:      10,
					Filters: &models.WorkflowSearchFilters{
						SignalType:         tc.signalType,
						Severity:           tc.severity,
						ResourceManagement: tc.resourceMgmt,
						Environment:        tc.environment,
					},
				}

				// EXPECT: Mock database to return workflows with final scores
				mock.ExpectQuery(`SELECT`).
					WillReturnRows(tc.mockRows)

				// ACT: Execute search
				response, err := repo.SearchByEmbedding(ctx, request)

				// ASSERT: Verify final score calculation
				Expect(err).ToNot(HaveOccurred(), "Should succeed for: %s", tc.description)
				Expect(response).ToNot(BeNil())
				Expect(response.Workflows).To(HaveLen(len(tc.expectedWorkflows)), "Expected %d workflows for: %s", len(tc.expectedWorkflows), tc.description)

				// Verify final score
				if len(response.Workflows) > 0 {
					workflow := response.Workflows[0]

					// Verify final score value
					Expect(workflow.FinalScore).To(BeNumerically("~", tc.expectedFinalScore, 0.01),
						"Expected final score %.2f for: %s", tc.expectedFinalScore, tc.description)

					// Verify capping at 1.0
					Expect(workflow.FinalScore).To(BeNumerically("<=", 1.0),
						"Final score must be <= 1.0 for: %s", tc.description)

					// Verify formula: final = base + boost - penalty (or 1.0 if capped)
					if !tc.expectedCapped {
						expectedFormula := workflow.BaseSimilarity + workflow.LabelBoost - workflow.LabelPenalty
						Expect(workflow.FinalScore).To(BeNumerically("~", expectedFormula, 0.01),
							"Final score should equal base(%.2f) + boost(%.2f) - penalty(%.2f) = %.2f",
							workflow.BaseSimilarity, workflow.LabelBoost, workflow.LabelPenalty, expectedFormula)
					}

					// Verify correct workflow returned
					Expect(workflow.Workflow.WorkflowID).To(Equal(tc.expectedWorkflows[0]),
						"Expected workflow %s for: %s", tc.expectedWorkflows[0], tc.description)
				}
			},
			// ========================================
			// BEHAVIOR TEST: Final score calculation
			// ========================================
			Entry("BEHAVIOR: calculates final score without capping", finalScoreTestCase{
				description:  "Final score = base + boost - penalty (not capped)",
				signalType:   "OOMKilled",
				severity:     "critical",
				resourceMgmt: func() *string { s := "gitops"; return &s }(),
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "version", "name", "description", "content", "content_hash",
					"labels", "embedding", "status", "is_latest_version",
					"disabled_at", "disabled_by", "disabled_reason",
					"previous_version", "deprecation_notice",
					"version_notes", "change_summary", "approved_by", "approved_at",
					"expected_success_rate", "expected_duration_seconds",
					"actual_success_rate", "total_executions", "successful_executions",
					"created_at", "updated_at", "created_by", "updated_by",
					"base_similarity", "label_boost", "label_penalty", "final_score",
				}).AddRow(
					"oom-normal-score", "v1.0.0", "OOM Normal Score", "Normal score workflow",
					"content", "hash123", []byte(`{"signal-type":"OOMKilled","severity":"critical","resource-management":"gitops"}`),
					nil, "active", true,
					nil, nil, nil, nil, nil, nil, nil, nil, nil,
					nil, nil, nil, 0, 0,
					time.Now(), time.Now(), nil, nil,
					0.80, 0.10, 0.0, 0.90, // base=0.80, boost=0.10, penalty=0.0, final=0.90 (not capped)
				),
				expectedFinalScore: 0.90,
				expectedCapped:     false,
				expectedWorkflows:  []string{"oom-normal-score"},
				testFocus:          "behavior",
			}),
			Entry("BEHAVIOR: caps final score at 1.0", finalScoreTestCase{
				description:  "Final score capped at 1.0 when base + boost > 1.0",
				signalType:   "OOMKilled",
				severity:     "critical",
				resourceMgmt: func() *string { s := "gitops"; return &s }(),
				environment:  func() *string { s := "production"; return &s }(),
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "version", "name", "description", "content", "content_hash",
					"labels", "embedding", "status", "is_latest_version",
					"disabled_at", "disabled_by", "disabled_reason",
					"previous_version", "deprecation_notice",
					"version_notes", "change_summary", "approved_by", "approved_at",
					"expected_success_rate", "expected_duration_seconds",
					"actual_success_rate", "total_executions", "successful_executions",
					"created_at", "updated_at", "created_by", "updated_by",
					"base_similarity", "label_boost", "label_penalty", "final_score",
				}).AddRow(
					"oom-capped-score", "v1.0.0", "OOM Capped Score", "Capped score workflow",
					"content", "hash456", []byte(`{"signal-type":"OOMKilled","severity":"critical","resource-management":"gitops","environment":"production"}`),
					nil, "active", true,
					nil, nil, nil, nil, nil, nil, nil, nil, nil,
					nil, nil, nil, 0, 0,
					time.Now(), time.Now(), nil, nil,
					0.95, 0.18, 0.0, 1.0, // base=0.95, boost=0.10+0.08=0.18, penalty=0.0, final=1.0 (capped from 1.13)
				),
				expectedFinalScore: 1.0,
				expectedCapped:     true,
				expectedWorkflows:  []string{"oom-capped-score"},
				testFocus:          "behavior",
			}),
			Entry("BEHAVIOR: calculates final score with penalty", finalScoreTestCase{
				description:  "Final score = base + boost - penalty",
				signalType:   "OOMKilled",
				severity:     "critical",
				resourceMgmt: func() *string { s := "gitops"; return &s }(),
				mockRows: sqlmock.NewRows([]string{
					"workflow_id", "version", "name", "description", "content", "content_hash",
					"labels", "embedding", "status", "is_latest_version",
					"disabled_at", "disabled_by", "disabled_reason",
					"previous_version", "deprecation_notice",
					"version_notes", "change_summary", "approved_by", "approved_at",
					"expected_success_rate", "expected_duration_seconds",
					"actual_success_rate", "total_executions", "successful_executions",
					"created_at", "updated_at", "created_by", "updated_by",
					"base_similarity", "label_boost", "label_penalty", "final_score",
				}).AddRow(
					"oom-penalty-score", "v1.0.0", "OOM Penalty Score", "Penalty score workflow",
					"content", "hash789", []byte(`{"signal-type":"OOMKilled","severity":"critical","resource-management":"manual"}`),
					nil, "active", true,
					nil, nil, nil, nil, nil, nil, nil, nil, nil,
					nil, nil, nil, 0, 0,
					time.Now(), time.Now(), nil, nil,
					0.92, 0.0, 0.10, 0.82, // base=0.92, boost=0.0, penalty=0.10, final=0.82
				),
				expectedFinalScore: 0.82,
				expectedCapped:     false,
				expectedWorkflows:  []string{"oom-penalty-score"},
				testFocus:          "behavior",
			}),
		)
	})
})
