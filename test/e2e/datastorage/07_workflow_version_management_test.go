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
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	_ "github.com/jackc/pgx/v5/stdlib"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
)

// Scenario 7: Workflow Version Management (DD-WORKFLOW-002 v3.0)
//
// Design Decision: DD-WORKFLOW-002 v3.0 - MCP Workflow Catalog Architecture
// Design Decision: DD-WORKFLOW-012 v2.0 - Workflow Immutability Constraints
//
// Business Requirements:
// - BR-STORAGE-012: Workflow catalog persistence with UUID primary key
// - BR-STORAGE-013: Semantic search returns flat response structure
//
// Business Value: Verify workflow version management and UUID-based identity
//
// Test Flow:
// 1. Create workflow v1.0.0 - verify UUID returned, is_latest_version=true
// 2. Create workflow v1.1.0 (same workflow_name) - verify new UUID, v1.0.0 becomes is_latest_version=false
// 3. Create workflow v2.0.0 (same workflow_name) - verify version management continues
// 4. Search workflows - verify flat response structure with title, signal_type (singular)
// 5. Get workflow by UUID - verify complete details returned
// 6. List versions by workflow_name - verify all versions returned
//
// Expected Results:
// - Each workflow creation returns unique UUID
// - is_latest_version properly managed (only latest version is true)
// - Search response is flat (no nested workflow object)
// - Search response uses 'title' (not 'name'), 'signal_type' (not 'signal_types')
// - workflow_id is UUID format
//
// Parallel Execution: ✅ ENABLED
// - Each test gets unique namespace (datastorage-e2e-p{N}-{timestamp})
// - Complete infrastructure isolation
// - No data pollution between tests

var _ = Describe("Scenario 7: Workflow Version Management (DD-WORKFLOW-002 v3.0)", Label("e2e", "workflow-version", "p0"), Ordered, func() {
	var (
		testCancel context.CancelFunc
		testLogger logr.Logger
		// DD-AUTH-014: Use exported HTTPClient from suite setup
		testNamespace string
		serviceURL    string
		db            *sql.DB
		testID        string

		// Track created workflow UUIDs for cleanup and verification
		workflowV1UUID string
		workflowV2UUID string
		workflowV3UUID string
		workflowName   string
	)

	BeforeAll(func() {
		_, testCancel = context.WithTimeout(ctx, 15*time.Minute)
		testLogger = logger.WithValues("test", "workflow-version-management")
		// DD-AUTH-014: HTTPClient is now provided by suite setup with ServiceAccount auth

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario 7: Workflow Version Management - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique test ID for workflow isolation
		testID = fmt.Sprintf("e2e-%d-%s", GinkgoParallelProcess(), uuid.New().String()[:8])
		workflowName = fmt.Sprintf("oom-recovery-%s", testID)

		// Use shared deployment from SynchronizedBeforeSuite
		testNamespace = sharedNamespace
		serviceURL = dataStorageURL
		testLogger.Info("Using shared deployment", "namespace", testNamespace, "url", serviceURL)

		// Wait for service to be ready using typed OpenAPI client
		testLogger.Info("⏳ Waiting for Data Storage Service to be ready...")
		Eventually(func() error {
			_, err := DSClient.ReadinessCheck(ctx)
			return err
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
		testLogger.Info("✅ Data Storage Service is ready")

		// Connect to PostgreSQL for direct verification
		testLogger.Info("🔌 Connecting to PostgreSQL for verification...")
		var err error
		db, err = sql.Open("pgx", postgresURL)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() error {
			return db.Ping()
		}, 30*time.Second, 2*time.Second).Should(Succeed())
		testLogger.Info("✅ PostgreSQL connection established")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario 7: Workflow Version Management - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Cleanup test workflows from database
		if db != nil {
			_, err := db.Exec("DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE $1", workflowName+"%")
			if err != nil {
				testLogger.Info("warning: Failed to cleanup test workflows", "error", err)
			}
			_ = db.Close()
		}

		testCancel()
		testLogger.Info("✅ Cleanup complete")
	})

	Context("when managing workflow versions with UUID primary key", func() {
		It("should create workflow v1.0.0 with UUID and is_latest_version=true", func() {
			testLogger.Info("📝 Creating workflow v1.0.0...")

			// DD-WORKFLOW-002 v3.0: Create workflow request
			// DD-API-001: Use typed OpenAPI struct
			// ADR-043: WorkflowSchema format required (rejects raw Tekton Pipeline YAML)
			workflowID := fmt.Sprintf("%s-v1-0-0", workflowName)
			workflowContent := fmt.Sprintf(`apiVersion: kubernaut.io/v1alpha1
kind: WorkflowSchema
metadata:
  workflow_id: %s
  version: "v1.0.0"
  description: Increases memory limits conservatively for OOMKilled pods
labels:
  signal_type: OOMKilled
  severity: critical
  risk_tolerance: medium
  component: deployment
  environment: production
  priority: P0
parameters:
  - name: DEPLOYMENT_NAME
    type: string
    required: true
    description: Name of the deployment to remediate
execution:
  engine: tekton
  bundle: ghcr.io/kubernaut/workflows/oom-recovery:v1.0.0
`, workflowID)
			contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(workflowContent)))
			containerImage := "quay.io/kubernaut/workflow-oom:v1.0.0@sha256:abc123def456"

			createReq := dsgen.RemediationWorkflow{
				WorkflowName:    workflowName,
				Version:         "v1.0.0",
				Name:            "OOM Recovery - Conservative Memory Increase",
				Description:     "Increases memory limits conservatively for OOMKilled pods",
				Content:         workflowContent,
				ContentHash:     contentHash,
				ExecutionEngine: "tekton",
				Status:          dsgen.RemediationWorkflowStatusActive,
				Labels: dsgen.MandatoryLabels{
					SignalType:  "OOMKilled",                                                                                // mandatory (DD-WORKFLOW-001 v1.4)
					Severity:    dsgen.MandatoryLabelsSeverity_critical,                                                      // mandatory
					Component:   "deployment",                                                                               // mandatory
					Priority:    dsgen.MandatoryLabelsPriority_P0,                                                           // mandatory
					Environment: []dsgen.MandatoryLabelsEnvironmentItem{dsgen.MandatoryLabelsEnvironmentItem("production")}, // mandatory
				},
				ContainerImage: dsgen.NewOptString(containerImage),
			}

			createResp, err := DSClient.CreateWorkflow(ctx, &createReq)
			Expect(err).ToNot(HaveOccurred())
			testLogger.Info("Create v1.0.0 response", "status", 201)

			// DD-WORKFLOW-002 v3.0: workflow_id is UUID - extract from response
			workflowResp, ok := createResp.(*dsgen.RemediationWorkflow)
			Expect(ok).To(BeTrue(), "Response should be *RemediationWorkflow")
			workflowV1UUID = workflowResp.WorkflowID.Value.String()
			Expect(workflowV1UUID).ToNot(BeEmpty())
			testLogger.Info("✅ Workflow v1.0.0 created", "uuid", workflowV1UUID)

			// Verify UUID format (8-4-4-4-12)
			Expect(workflowV1UUID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`))

			// Verify is_latest_version=true in database
			var isLatest bool
			err = db.QueryRow("SELECT is_latest_version FROM remediation_workflow_catalog WHERE workflow_id = $1", workflowV1UUID).Scan(&isLatest)
			Expect(err).ToNot(HaveOccurred())
			Expect(isLatest).To(BeTrue(), "v1.0.0 should have is_latest_version=true")
			testLogger.Info("✅ is_latest_version verified", "is_latest", isLatest)
		})

		It("should create workflow v1.1.0 and mark v1.0.0 as not latest", func() {
			testLogger.Info("📝 Creating workflow v1.1.0...")

			// DD-API-001: Use typed OpenAPI struct
			// ADR-043: WorkflowSchema format required (rejects raw Tekton Pipeline YAML)
			workflowID := fmt.Sprintf("%s-v1-1-0", workflowName)
			workflowContent := fmt.Sprintf(`apiVersion: kubernaut.io/v1alpha1
kind: WorkflowSchema
metadata:
  workflow_id: %s
  version: "v1.1.0"
  description: Improved version with better memory calculation
labels:
  signal_type: OOMKilled
  severity: critical
  risk_tolerance: medium
  component: deployment
  environment: production
  priority: P0
parameters:
  - name: DEPLOYMENT_NAME
    type: string
    required: true
    description: Name of the deployment to remediate
execution:
  engine: tekton
  bundle: ghcr.io/kubernaut/workflows/oom-recovery:v1.1.0
`, workflowID)
			contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(workflowContent)))
			containerImage := "quay.io/kubernaut/workflow-oom:v1.1.0@sha256:def456ghi789"
			previousVersion := "v1.0.0"

			createReq := dsgen.RemediationWorkflow{
				WorkflowName:    workflowName,
				Version:         "v1.1.0",
				Name:            "OOM Recovery - Conservative Memory Increase (Improved)",
				Description:     "Improved version with better memory calculation",
				Content:         workflowContent,
				ContentHash:     contentHash,
				ExecutionEngine: "tekton",
				Status:          dsgen.RemediationWorkflowStatusActive,
				PreviousVersion: dsgen.NewOptString(previousVersion),
				Labels: dsgen.MandatoryLabels{
					SignalType:  "OOMKilled",                                                                                // mandatory (DD-WORKFLOW-001 v1.4)
					Severity:    dsgen.MandatoryLabelsSeverity_critical,                                                      // mandatory
					Component:   "deployment",                                                                               // mandatory
					Priority:    dsgen.MandatoryLabelsPriority_P0,                                                           // mandatory
					Environment: []dsgen.MandatoryLabelsEnvironmentItem{dsgen.MandatoryLabelsEnvironmentItem("production")}, // mandatory
				},
				ContainerImage: dsgen.NewOptString(containerImage),
			}

			createResp, err := DSClient.CreateWorkflow(ctx, &createReq)
			Expect(err).ToNot(HaveOccurred())
			testLogger.Info("Create v1.1.0 response", "status", 201)

			// DD-WORKFLOW-002 v3.0: workflow_id is UUID - extract from response
			workflowResp, ok := createResp.(*dsgen.RemediationWorkflow)
			Expect(ok).To(BeTrue(), "Response should be *RemediationWorkflow")
			workflowV2UUID = workflowResp.WorkflowID.Value.String()
			Expect(workflowV2UUID).ToNot(BeEmpty())
			Expect(workflowV2UUID).ToNot(Equal(workflowV1UUID), "v1.1.0 should have different UUID than v1.0.0")
			testLogger.Info("✅ Workflow v1.1.0 created", "uuid", workflowV2UUID)

			// DD-WORKFLOW-012 v2.0: Verify is_latest_version management
			// v1.0.0 should now be is_latest_version=false
			var v1IsLatest bool
			err = db.QueryRow("SELECT is_latest_version FROM remediation_workflow_catalog WHERE workflow_id = $1", workflowV1UUID).Scan(&v1IsLatest)
			Expect(err).ToNot(HaveOccurred())
			Expect(v1IsLatest).To(BeFalse(), "v1.0.0 should have is_latest_version=false after v1.1.0 creation")

			// v1.1.0 should be is_latest_version=true
			var v2IsLatest bool
			err = db.QueryRow("SELECT is_latest_version FROM remediation_workflow_catalog WHERE workflow_id = $1", workflowV2UUID).Scan(&v2IsLatest)
			Expect(err).ToNot(HaveOccurred())
			Expect(v2IsLatest).To(BeTrue(), "v1.1.0 should have is_latest_version=true")

			testLogger.Info("✅ is_latest_version management verified",
				"v1.0.0_is_latest", v1IsLatest,
				"v1.1.0_is_latest", v2IsLatest)
		})

		It("should create workflow v2.0.0 and only latest version is marked", func() {
			testLogger.Info("📝 Creating workflow v2.0.0...")

			// DD-API-001: Use typed OpenAPI struct
			// ADR-043: WorkflowSchema format required (rejects raw Tekton Pipeline YAML)
			workflowID := fmt.Sprintf("%s-v2-0-0", workflowName)
			workflowContent := fmt.Sprintf(`apiVersion: kubernaut.io/v1alpha1
kind: WorkflowSchema
metadata:
  workflow_id: %s
  version: "v2.0.0"
  description: Major version with horizontal scaling support
labels:
  signal_type: OOMKilled
  severity: critical
  risk_tolerance: medium
  component: deployment
  environment: production
  priority: P0
parameters:
  - name: DEPLOYMENT_NAME
    type: string
    required: true
    description: Name of the deployment to remediate
execution:
  engine: tekton
  bundle: ghcr.io/kubernaut/workflows/oom-recovery:v2.0.0
`, workflowID)
			contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(workflowContent)))
			containerImage := "quay.io/kubernaut/workflow-oom:v2.0.0@sha256:ghi789jkl012"
			previousVersion := "v1.1.0"

			createReq := dsgen.RemediationWorkflow{
				WorkflowName:    workflowName,
				Version:         "v2.0.0",
				Name:            "OOM Recovery - Major Refactor",
				Description:     "Major version with horizontal scaling support",
				Content:         workflowContent,
				ContentHash:     contentHash,
				ExecutionEngine: "tekton",
				Status:          dsgen.RemediationWorkflowStatusActive,
				PreviousVersion: dsgen.NewOptString(previousVersion),
				Labels: dsgen.MandatoryLabels{
					SignalType:  "OOMKilled",                                                                                // mandatory (DD-WORKFLOW-001 v1.4)
					Severity:    dsgen.MandatoryLabelsSeverity_critical,                                                      // mandatory
					Component:   "deployment",                                                                               // mandatory
					Priority:    dsgen.MandatoryLabelsPriority_P0,                                                           // mandatory
					Environment: []dsgen.MandatoryLabelsEnvironmentItem{dsgen.MandatoryLabelsEnvironmentItem("production")}, // mandatory
				},
				ContainerImage: dsgen.NewOptString(containerImage),
			}

			createResp, err := DSClient.CreateWorkflow(ctx, &createReq)
			Expect(err).ToNot(HaveOccurred())

			// DD-WORKFLOW-002 v3.0: workflow_id is UUID - extract from response
			workflowResp, ok := createResp.(*dsgen.RemediationWorkflow)
			Expect(ok).To(BeTrue(), "Response should be *RemediationWorkflow")
			workflowV3UUID = workflowResp.WorkflowID.Value.String()
			testLogger.Info("✅ Workflow v2.0.0 created", "uuid", workflowV3UUID)

			// Verify only v2.0.0 is latest
			var latestCount int
			err = db.QueryRow("SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_name = $1 AND is_latest_version = true", workflowName).Scan(&latestCount)
			Expect(err).ToNot(HaveOccurred())
			Expect(latestCount).To(Equal(1), "Only one version should be is_latest_version=true")

			// Verify it's v2.0.0
			var latestUUID string
			err = db.QueryRow("SELECT workflow_id FROM remediation_workflow_catalog WHERE workflow_name = $1 AND is_latest_version = true", workflowName).Scan(&latestUUID)
			Expect(err).ToNot(HaveOccurred())
			Expect(latestUUID).To(Equal(workflowV3UUID), "v2.0.0 should be the latest version")

			testLogger.Info("✅ Only v2.0.0 is marked as latest", "latest_count", latestCount)
		})

		It("should return flat response structure in search (DD-WORKFLOW-002 v3.0)", func() {
			testLogger.Info("🔍 Testing search response structure...")

			// V1.0: Label-only search with 5 mandatory filters (DD-WORKFLOW-001 v1.4)
			// DD-API-001: Use typed OpenAPI struct
			topK := 10
			searchReq := dsgen.WorkflowSearchRequest{
				Filters: dsgen.WorkflowSearchFilters{
					SignalType:  "OOMKilled",                                 // mandatory
					Severity:    dsgen.WorkflowSearchFiltersSeverityCritical, // mandatory
					Component:   "deployment",                                // mandatory
					Environment: "production",                                // mandatory
					Priority:    dsgen.WorkflowSearchFiltersPriorityP0,       // mandatory
				},
				TopK: dsgen.NewOptInt(topK),
			}

			resp, err := DSClient.SearchWorkflows(ctx, &searchReq)
			Expect(err).ToNot(HaveOccurred())
			searchResults, ok := resp.(*dsgen.WorkflowSearchResponse)
			Expect(ok).To(BeTrue(), "Expected *WorkflowSearchResponse type")
			testLogger.Info("Search response", "status", 201)

			Expect(searchResults).ToNot(BeNil())
			Expect(searchResults.Workflows).ToNot(BeNil())

			// DD-WORKFLOW-002 v3.0: Verify flat response structure
			workflows := searchResults.Workflows
			Expect(len(workflows)).To(BeNumerically(">", 0), "Should return at least one workflow")

			// Check first workflow has flat structure
			firstWorkflow := workflows[0]

			// DD-WORKFLOW-002 v3.0: workflow_id is UUID (top-level, not nested)
			workflowID := firstWorkflow.WorkflowID.String()
			Expect(workflowID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "workflow_id should be UUID format")

			// DD-WORKFLOW-002 v3.0: 'title' field exists (typed as string in generated client)
			Expect(firstWorkflow.Title).ToNot(BeEmpty(), "Response should have non-empty 'title' field")

			// DD-WORKFLOW-002 v3.0: 'signal_type' (singular string) instead of 'signal_types' (array)
			Expect(firstWorkflow.SignalType).ToNot(BeEmpty(), "Response should have 'signal_type' field (singular)")

			// DD-WORKFLOW-002 v3.0: 'confidence' at top level
			Expect(firstWorkflow.Confidence).ToNot(BeNil(), "Response should have 'confidence' at top level")

			// DD-WORKFLOW-002 v3.0: Typed response enforces flat structure
			// (No nested 'workflow' object possible in generated client)

			testLogger.Info("✅ Flat response structure verified",
				"workflow_id", workflowID,
				"signal_type", firstWorkflow.SignalType)
		})

		It("should retrieve workflow by UUID", func() {
			testLogger.Info("🔍 Getting workflow by UUID...")

			// DD-WORKFLOW-002 v3.0: Get by UUID only (no version parameter)
			workflowUUID, err := uuid.Parse(workflowV3UUID)
			Expect(err).ToNot(HaveOccurred())

			// Use typed OpenAPI client
			resp, err := DSClient.GetWorkflowByID(ctx, dsgen.GetWorkflowByIDParams{
				WorkflowID: workflowUUID,
			})
			Expect(err).ToNot(HaveOccurred())

			// Type assert to RemediationWorkflow (success response)
			workflow, ok := resp.(*dsgen.RemediationWorkflow)
			Expect(ok).To(BeTrue(), "Response should be *RemediationWorkflow")

			// Verify UUID matches
			Expect(workflow.WorkflowID.Value.String()).To(Equal(workflowV3UUID))

			// Verify workflow_name and version are present (for human reference)
			Expect(workflow.WorkflowName).To(Equal(workflowName))
			Expect(workflow.Version).To(Equal("v2.0.0"))

			testLogger.Info("✅ Workflow retrieved by UUID", "uuid", workflowV3UUID)
		})

		It("should list all versions by workflow_name", func() {
			testLogger.Info("🔍 Listing versions by workflow_name...")

			// DD-WORKFLOW-002 v3.0: List versions by workflow_name
			// Use ListWorkflows with WorkflowName filter (replaces non-existent /by-name/{name}/versions endpoint)
			resp, err := DSClient.ListWorkflows(ctx, dsgen.ListWorkflowsParams{
				WorkflowName: dsgen.NewOptString(workflowName),
			})
			Expect(err).ToNot(HaveOccurred())

			// Type assert to WorkflowListResponse (success response)
			workflowList, ok := resp.(*dsgen.WorkflowListResponse)
			Expect(ok).To(BeTrue(), "Response should be *WorkflowListResponse")

			// Verify all 3 versions returned (v1.0.0, v1.1.0, v2.0.0)
			Expect(workflowList.Workflows).To(HaveLen(3), "Should have 3 versions (v1.0.0, v1.1.0, v2.0.0)")

			testLogger.Info("✅ All versions listed", "count", len(workflowList.Workflows))
		})
	})
})
