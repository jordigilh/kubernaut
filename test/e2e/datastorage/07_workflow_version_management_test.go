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
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		testCancel    context.CancelFunc
		testLogger    logr.Logger
		httpClient    *http.Client
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
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario 7: Workflow Version Management - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique test ID for workflow isolation
		testID = fmt.Sprintf("e2e-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
		workflowName = fmt.Sprintf("oom-recovery-%s", testID)

		// Use shared deployment from SynchronizedBeforeSuite
		testNamespace = sharedNamespace
		serviceURL = dataStorageURL
		testLogger.Info("Using shared deployment", "namespace", testNamespace, "url", serviceURL)

		// Wait for service to be ready
		testLogger.Info("⏳ Waiting for Data Storage Service to be ready...")
		Eventually(func() error {
			resp, err := httpClient.Get(serviceURL + "/health/ready")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("service not ready: status %d", resp.StatusCode)
			}
			return nil
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
			db.Close()
		}

		testCancel()
		testLogger.Info("✅ Cleanup complete")
	})

	Context("when managing workflow versions with UUID primary key", func() {
		It("should create workflow v1.0.0 with UUID and is_latest_version=true", func() {
			testLogger.Info("📝 Creating workflow v1.0.0...")

			// DD-WORKFLOW-002 v3.0: Create workflow request
			workflowContent := "apiVersion: tekton.dev/v1beta1\nkind: Pipeline\nmetadata:\n  name: oom-recovery\nspec:\n  tasks:\n  - name: increase-memory\n    taskRef:\n      name: kubectl-patch"
			createReq := map[string]interface{}{
				"workflow_name":    workflowName,
				"version":          "v1.0.0",
				"name":             "OOM Recovery - Conservative Memory Increase",
				"description":      "Increases memory limits conservatively for OOMKilled pods",
				"content":          workflowContent,
				"content_hash":     fmt.Sprintf("%x", sha256.Sum256([]byte(workflowContent))),
				"execution_engine": "tekton",
				"status":           "active",
				"labels": map[string]interface{}{
					"signal_type": "OOMKilled",  // mandatory (DD-WORKFLOW-001 v1.4)
					"severity":    "critical",   // mandatory
					"component":   "deployment", // mandatory
					"priority":    "P0",         // mandatory
					"environment": "production", // mandatory
				},
				"container_image": "quay.io/kubernaut/workflow-oom:v1.0.0@sha256:abc123def456",
			}

			reqBody, err := json.Marshal(createReq)
			Expect(err).ToNot(HaveOccurred())

			resp, err := httpClient.Post(serviceURL+"/api/v1/workflows", "application/json", bytes.NewReader(reqBody))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			testLogger.Info("Create v1.0.0 response", "status", resp.StatusCode, "body", string(body))

			Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Expected 201 Created, got %d: %s", resp.StatusCode, string(body))

			// Parse response to get UUID
			var createResp map[string]interface{}
			err = json.Unmarshal(body, &createResp)
			Expect(err).ToNot(HaveOccurred())

			// DD-WORKFLOW-002 v3.0: workflow_id is UUID
			workflowV1UUID = createResp["workflow_id"].(string)
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

			workflowContent := "apiVersion: tekton.dev/v1beta1\nkind: Pipeline\nmetadata:\n  name: oom-recovery-v1.1\nspec:\n  tasks:\n  - name: increase-memory\n    taskRef:\n      name: kubectl-patch-v2"
			createReq := map[string]interface{}{
				"workflow_name":    workflowName,
				"version":          "v1.1.0",
				"name":             "OOM Recovery - Conservative Memory Increase (Improved)",
				"description":      "Improved version with better memory calculation",
				"content":          workflowContent,
				"content_hash":     fmt.Sprintf("%x", sha256.Sum256([]byte(workflowContent))),
				"execution_engine": "tekton",
				"status":           "active",
				"previous_version": "v1.0.0",
				"labels": map[string]interface{}{
					"signal_type": "OOMKilled",  // mandatory (DD-WORKFLOW-001 v1.4)
					"severity":    "critical",   // mandatory
					"component":   "deployment", // mandatory
					"priority":    "P0",         // mandatory
					"environment": "production", // mandatory
				},
				"container_image": "quay.io/kubernaut/workflow-oom:v1.1.0@sha256:def456ghi789",
			}

			reqBody, err := json.Marshal(createReq)
			Expect(err).ToNot(HaveOccurred())

			resp, err := httpClient.Post(serviceURL+"/api/v1/workflows", "application/json", bytes.NewReader(reqBody))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			testLogger.Info("Create v1.1.0 response", "status", resp.StatusCode, "body", string(body))

			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			var createResp map[string]interface{}
			err = json.Unmarshal(body, &createResp)
			Expect(err).ToNot(HaveOccurred())

			workflowV2UUID = createResp["workflow_id"].(string)
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

			workflowContent := "apiVersion: tekton.dev/v1beta1\nkind: Pipeline\nmetadata:\n  name: oom-recovery-v2\nspec:\n  tasks:\n  - name: scale-horizontal\n    taskRef:\n      name: kubectl-scale"
			createReq := map[string]interface{}{
				"workflow_name":    workflowName,
				"version":          "v2.0.0",
				"name":             "OOM Recovery - Major Refactor",
				"description":      "Major version with horizontal scaling support",
				"content":          workflowContent,
				"content_hash":     fmt.Sprintf("%x", sha256.Sum256([]byte(workflowContent))),
				"execution_engine": "tekton",
				"status":           "active",
				"previous_version": "v1.1.0",
				"labels": map[string]interface{}{
					"signal_type": "OOMKilled",  // mandatory (DD-WORKFLOW-001 v1.4)
					"severity":    "critical",   // mandatory
					"component":   "deployment", // mandatory
					"priority":    "P0",         // mandatory
					"environment": "production", // mandatory
				},
				"container_image": "quay.io/kubernaut/workflow-oom:v2.0.0@sha256:ghi789jkl012",
			}

			reqBody, err := json.Marshal(createReq)
			Expect(err).ToNot(HaveOccurred())

			resp, err := httpClient.Post(serviceURL+"/api/v1/workflows", "application/json", bytes.NewReader(reqBody))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			var createResp map[string]interface{}
			err = json.Unmarshal(body, &createResp)
			Expect(err).ToNot(HaveOccurred())

			workflowV3UUID = createResp["workflow_id"].(string)
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
			searchReq := map[string]interface{}{
				"filters": map[string]interface{}{
					"signal_type": "OOMKilled",  // mandatory
					"severity":    "critical",   // mandatory
					"component":   "deployment", // mandatory
					"environment": "production", // mandatory
					"priority":    "P0",         // mandatory
				},
				"top_k": 10,
			}

			reqBody, err := json.Marshal(searchReq)
			Expect(err).ToNot(HaveOccurred())

			resp, err := httpClient.Post(serviceURL+"/api/v1/workflows/search", "application/json", bytes.NewReader(reqBody))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			testLogger.Info("Search response", "status", resp.StatusCode, "body", string(body))

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var searchResp map[string]interface{}
			err = json.Unmarshal(body, &searchResp)
			Expect(err).ToNot(HaveOccurred())

			// DD-WORKFLOW-002 v3.0: Verify flat response structure
			workflows, ok := searchResp["workflows"].([]interface{})
			Expect(ok).To(BeTrue(), "Response should have 'workflows' array")
			Expect(len(workflows)).To(BeNumerically(">", 0), "Should return at least one workflow")

			// Check first workflow has flat structure
			firstWorkflow := workflows[0].(map[string]interface{})

			// DD-WORKFLOW-002 v3.0: workflow_id is UUID (top-level, not nested)
			workflowID, ok := firstWorkflow["workflow_id"].(string)
			Expect(ok).To(BeTrue(), "workflow_id should be string at top level")
			Expect(workflowID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "workflow_id should be UUID format")

			// DD-WORKFLOW-002 v3.0: 'title' instead of 'name'
			_, hasTitle := firstWorkflow["title"]
			Expect(hasTitle).To(BeTrue(), "Response should have 'title' field (not 'name')")

			// DD-WORKFLOW-002 v3.0: 'signal_type' (singular string) instead of 'signal_types' (array)
			signalType, hasSignalType := firstWorkflow["signal_type"]
			Expect(hasSignalType).To(BeTrue(), "Response should have 'signal_type' field (singular)")
			Expect(signalType).To(BeAssignableToTypeOf(""), "signal_type should be string (not array)")

			// DD-WORKFLOW-002 v3.0: 'confidence' at top level
			_, hasConfidence := firstWorkflow["confidence"]
			Expect(hasConfidence).To(BeTrue(), "Response should have 'confidence' at top level")

			// DD-WORKFLOW-002 v3.0: No 'version' in search response (LLM doesn't need it)
			_, hasVersion := firstWorkflow["version"]
			Expect(hasVersion).To(BeFalse(), "Response should NOT have 'version' field")

			// DD-WORKFLOW-002 v3.0: No nested 'workflow' object
			_, hasNestedWorkflow := firstWorkflow["workflow"]
			Expect(hasNestedWorkflow).To(BeFalse(), "Response should NOT have nested 'workflow' object")

			testLogger.Info("✅ Flat response structure verified",
				"workflow_id", workflowID,
				"signal_type", signalType)
		})

		It("should retrieve workflow by UUID", func() {
			testLogger.Info("🔍 Getting workflow by UUID...")

			// DD-WORKFLOW-002 v3.0: Get by UUID only (no version parameter)
			resp, err := httpClient.Get(fmt.Sprintf("%s/api/v1/workflows/%s", serviceURL, workflowV3UUID))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			testLogger.Info("Get workflow response", "status", resp.StatusCode, "body", string(body))

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var workflow map[string]interface{}
			err = json.Unmarshal(body, &workflow)
			Expect(err).ToNot(HaveOccurred())

			// Verify UUID matches
			Expect(workflow["workflow_id"]).To(Equal(workflowV3UUID))

			// Verify workflow_name and version are present (for human reference)
			Expect(workflow["workflow_name"]).To(Equal(workflowName))
			Expect(workflow["version"]).To(Equal("v2.0.0"))

			testLogger.Info("✅ Workflow retrieved by UUID", "uuid", workflowV3UUID)
		})

		It("should list all versions by workflow_name", func() {
			testLogger.Info("🔍 Listing versions by workflow_name...")

			// DD-WORKFLOW-002 v3.0: List versions by workflow_name
			resp, err := httpClient.Get(fmt.Sprintf("%s/api/v1/workflows/by-name/%s/versions", serviceURL, workflowName))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			testLogger.Info("List versions response", "status", resp.StatusCode, "body", string(body))

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var versionsResp map[string]interface{}
			err = json.Unmarshal(body, &versionsResp)
			Expect(err).ToNot(HaveOccurred())

			versions, ok := versionsResp["versions"].([]interface{})
			Expect(ok).To(BeTrue(), "Response should have 'versions' array")
			Expect(len(versions)).To(Equal(3), "Should have 3 versions (v1.0.0, v1.1.0, v2.0.0)")

			testLogger.Info("✅ All versions listed", "count", len(versions))
		})
	})
})
