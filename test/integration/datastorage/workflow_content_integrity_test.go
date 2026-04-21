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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// createIntegrityTestServer creates an in-process httptest.Server with real PostgreSQL
// for content integrity integration tests. Uses correct ports (15433/16379).
func createIntegrityTestServer(schemaYAML string) (*httptest.Server, *server.Server) {
	serverCfg := &server.Config{
		Port:         18090,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	pgHost := os.Getenv("POSTGRES_HOST")
	if pgHost == "" {
		pgHost = "localhost"
	}
	pgPort := os.Getenv("POSTGRES_PORT")
	if pgPort == "" {
		pgPort = "15433"
	}
	dbConnStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable", pgHost, pgPort)

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "16379"
	}
	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)

	appCfg := &config.Config{
		Database: config.DatabaseConfig{
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: "5m",
			ConnMaxIdleTime: "10m",
		},
	}

	mockAuthenticator := &auth.MockAuthenticator{
		ValidUsers: map[string]string{
			"test-token": "system:serviceaccount:integrity-test:integrity-validation",
		},
	}
	mockAuthorizer := &auth.MockAuthorizer{
		AllowedUsers: map[string]bool{
			"system:serviceaccount:integrity-test:integrity-validation": true,
		},
	}

	mockPuller := oci.NewMockImagePuller(schemaYAML)
	schemaExtractor := oci.NewSchemaExtractor(mockPuller, schema.NewParser())

	srv, err := server.NewServer(server.ServerDeps{
		DBConnStr:     dbConnStr,
		RedisAddr:     redisAddr,
		RedisPassword: "",
		Logger:        logger,
		AppConfig:     appCfg,
		ServerConfig:  serverCfg,
		DLQMaxLen:     1000,
		Authenticator: mockAuthenticator,
		Authorizer:    mockAuthorizer,
		AuthNamespace: "integrity-test",
		HandlerOpts: []server.HandlerOption{
			server.WithSchemaExtractor(schemaExtractor),
		},
	})
	Expect(err).ToNot(HaveOccurred(), "Integrity test server creation should succeed")

	httpServer := httptest.NewServer(srv.Handler())
	return httpServer, srv
}

func postInlineWorkflow(serverURL, yamlContent string) (*http.Response, error) {
	body := fmt.Sprintf(`{"content":%s}`, jsonEscape(yamlContent))
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/v1/workflows", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	return http.DefaultClient.Do(req)
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

type workflowResponse struct {
	StatusCode int
	WorkflowID string
	Raw        map[string]interface{}
}

func registerIntegrityWorkflow(serverURL, yamlContent string) workflowResponse {
	resp, err := postInlineWorkflow(serverURL, yamlContent)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	wr := workflowResponse{StatusCode: resp.StatusCode, Raw: make(map[string]interface{})}
	if len(body) > 0 {
		ExpectWithOffset(1, json.Unmarshal(body, &wr.Raw)).To(Succeed())
		if id, ok := wr.Raw["workflowId"].(string); ok {
			wr.WorkflowID = id
		}
	}
	return wr
}

func queryWorkflowStatus(workflowID string) string {
	var status string
	_ = db.QueryRow(
		"SELECT status FROM remediation_workflow_catalog WHERE workflow_id = $1", workflowID,
	).Scan(&status)
	return status
}

func countWorkflowsByName(workflowName string) int {
	var count int
	_ = db.QueryRow(
		"SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_name = $1", workflowName,
	).Scan(&count)
	return count
}

func integrityTestYAML(testID, description string) string {
	return integrityTestYAMLWithVersion(testID, description, "1.0.0")
}

func integrityTestYAMLWithVersion(testID, description, version string) string {
	crd := testutil.NewTestWorkflowCRD(testID, "IncreaseMemoryLimits", "job")
	crd.Spec.Version = version
	crd.Spec.Description = sharedtypes.StructuredDescription{
		What:      description,
		WhenToUse: "Integration test",
	}
	crd.Spec.Execution.Bundle = "quay.io/kubernaut/workflows/scale-memory:v1.0.0@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abc1"
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target resource"},
	}
	return testutil.MarshalWorkflowCRD(crd)
}

var _ = Describe("Workflow Content Integrity Integration Tests (BR-WORKFLOW-006)", func() {
	var (
		workflowRepo *workflow.Repository
	)

	BeforeEach(func() {
		workflowRepo = workflow.NewRepository(db, logger)
	})

	// ========================================
	// IT-DS-INTEGRITY-001: First registration stores content_hash
	// ========================================
	Describe("IT-DS-INTEGRITY-001: Content hash stored on first registration", func() {
		It("should populate content_hash in the database", func() {
			testID := fmt.Sprintf("integrity-hash-%s", uuid.New().String()[:8])
			yamlContent := integrityTestYAML(testID, "First registration hash test")

			httpServer, srv := createIntegrityTestServer(yamlContent)
			defer httpServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			wr := registerIntegrityWorkflow(httpServer.URL, yamlContent)
			Expect(wr.StatusCode).To(Equal(http.StatusCreated))
			Expect(len(wr.WorkflowID)).To(BeNumerically(">", 0), "DS must return a non-empty workflow UUID")

			Eventually(func() string {
				var contentHash string
				_ = db.QueryRow(
					"SELECT content_hash FROM remediation_workflow_catalog WHERE workflow_id = $1", wr.WorkflowID,
				).Scan(&contentHash)
				return contentHash
			}, 5*time.Second, 500*time.Millisecond).ShouldNot(BeEmpty(),
				"content_hash should be populated in database")
		})
	})

	// ========================================
	// IT-DS-INTEGRITY-002: Idempotent re-apply (same content hash) returns 200
	// ========================================
	Describe("IT-DS-INTEGRITY-002: Idempotent re-apply with same content", func() {
		It("should return 200 with same workflow_id on re-apply", func() {
			testID := fmt.Sprintf("integrity-idempotent-%s", uuid.New().String()[:8])
			yamlContent := integrityTestYAML(testID, "Idempotent re-apply test")

			httpServer, srv := createIntegrityTestServer(yamlContent)
			defer httpServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			wr1 := registerIntegrityWorkflow(httpServer.URL, yamlContent)
			Expect(wr1.StatusCode).To(Equal(http.StatusCreated))

			wr2 := registerIntegrityWorkflow(httpServer.URL, yamlContent)
			Expect(wr2.StatusCode).To(Equal(http.StatusOK),
				"Idempotent re-apply should return 200")
			Expect(wr2.WorkflowID).To(Equal(wr1.WorkflowID),
				"Same content should return same workflow UUID")
		})
	})

	// ========================================
	// IT-DS-INTEGRITY-003: Same (name, version) + different content → 409 Conflict
	// Issue #773: Version-locked content immutability. Must bump version to register
	// new content for an active workflow.
	// ========================================
	Describe("IT-DS-INTEGRITY-003: Same-version content change rejected with 409", func() {
		It("should return 409 Conflict with content-integrity-violation", func() {
			testID := fmt.Sprintf("integrity-reject-%s", uuid.New().String()[:8])
			yamlOriginal := integrityTestYAML(testID, "Original content before change")
			yamlModified := integrityTestYAML(testID, "Modified content triggers rejection")

			httpServer, srv := createIntegrityTestServer(yamlOriginal)
			defer httpServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			wr1 := registerIntegrityWorkflow(httpServer.URL, yamlOriginal)
			Expect(wr1.StatusCode).To(Equal(http.StatusCreated))

			wr2 := registerIntegrityWorkflow(httpServer.URL, yamlModified)
			Expect(wr2.StatusCode).To(Equal(http.StatusConflict),
				"Same version + different content should return 409 Conflict")
			Expect(wr2.Raw["type"]).To(ContainSubstring("content-integrity-violation"),
				"RFC7807 problem type should be content-integrity-violation")
		})
	})

	// ========================================
	// IT-DS-INTEGRITY-004: Re-enable disabled workflow with same content
	// ========================================
	Describe("IT-DS-INTEGRITY-004: Re-enable disabled workflow", func() {
		It("should re-enable and return 200 with same UUID", func() {
			testID := fmt.Sprintf("integrity-reenable-%s", uuid.New().String()[:8])
			yamlContent := integrityTestYAML(testID, "Re-enable test content")

			httpServer, srv := createIntegrityTestServer(yamlContent)
			defer httpServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			wr1 := registerIntegrityWorkflow(httpServer.URL, yamlContent)
			Expect(wr1.StatusCode).To(Equal(http.StatusCreated))

			err := workflowRepo.UpdateStatus(ctx, wr1.WorkflowID, "1.0.0", "Disabled", "test disable", "test-user")
			Expect(err).ToNot(HaveOccurred())

			wr2 := registerIntegrityWorkflow(httpServer.URL, yamlContent)
			Expect(wr2.StatusCode).To(Equal(http.StatusOK),
				"Re-enable should return 200")
			Expect(wr2.WorkflowID).To(Equal(wr1.WorkflowID),
				"Re-enabled workflow should have the original UUID")

			Eventually(func() string {
				return queryWorkflowStatus(wr1.WorkflowID)
			}, 5*time.Second, 500*time.Millisecond).Should(Equal("Active"))
		})
	})

	// ========================================
	// IT-DS-INTEGRITY-005: New workflow for disabled + different content
	// ========================================
	Describe("IT-DS-INTEGRITY-005: Disabled with different content creates new", func() {
		It("should create new record with new UUID", func() {
			testID := fmt.Sprintf("integrity-dis-diff-%s", uuid.New().String()[:8])
			yamlOriginal := integrityTestYAML(testID, "Original disabled content")
			yamlModified := integrityTestYAML(testID, "Modified content for disabled")

			httpServer, srv := createIntegrityTestServer(yamlOriginal)
			defer httpServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			wr1 := registerIntegrityWorkflow(httpServer.URL, yamlOriginal)
			Expect(wr1.StatusCode).To(Equal(http.StatusCreated))

			err := workflowRepo.UpdateStatus(ctx, wr1.WorkflowID, "1.0.0", "Disabled", "test disable", "test-user")
			Expect(err).ToNot(HaveOccurred())

			wr2 := registerIntegrityWorkflow(httpServer.URL, yamlModified)
			Expect(wr2.StatusCode).To(Equal(http.StatusCreated),
				"Different content for disabled should return 201")
			Expect(wr2.WorkflowID).ToNot(Equal(wr1.WorkflowID),
				"New record should have a different UUID")
		})
	})

	// ========================================
	// IT-DS-INTEGRITY-006: Cross-version superseded record preserved in DB (audit trail)
	// Issue #773: Same-version supersede is no longer possible (409 Conflict).
	// Cross-version supersede (v1.0.0 → v2.0.0) preserves the old record.
	// ========================================
	Describe("IT-DS-INTEGRITY-006: Cross-version superseded record preserved for audit", func() {
		It("should keep the old record with status=superseded after version bump", func() {
			testID := fmt.Sprintf("integrity-audit-%s", uuid.New().String()[:8])
			yamlV1 := integrityTestYAMLWithVersion(testID, "Audit trail v1", "1.0.0")
			yamlV2 := integrityTestYAMLWithVersion(testID, "Audit trail v2", "2.0.0")

			httpServer, srv := createIntegrityTestServer(yamlV1)
			defer httpServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			wr1 := registerIntegrityWorkflow(httpServer.URL, yamlV1)
			Expect(wr1.StatusCode).To(Equal(http.StatusCreated))

			wr2 := registerIntegrityWorkflow(httpServer.URL, yamlV2)
			Expect(wr2.StatusCode).To(Equal(http.StatusCreated),
				"Cross-version supersede should return 201")

			Eventually(func() int {
				return countWorkflowsByName(testID)
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(2),
				"Both v1 and v2 records should exist in DB")

			var oldContent string
			err := db.QueryRow(
				"SELECT content FROM remediation_workflow_catalog WHERE workflow_id = $1", wr1.WorkflowID,
			).Scan(&oldContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(oldContent).To(ContainSubstring("Audit trail v1"),
				"Old record should retain its original content for audit")
		})
	})

	// ========================================
	// IT-DS-INTEGRITY-007: Multiple cross-version supersedes preserve full chain
	// Issue #773: Same-version supersede is no longer possible (409 Conflict).
	// Cross-version chain (v1 → v2 → v3) preserves all historical records.
	// ========================================
	Describe("IT-DS-INTEGRITY-007: Multiple cross-version supersedes preserve full history", func() {
		It("should keep all historical records across version bumps", func() {
			testID := fmt.Sprintf("integrity-chain-%s", uuid.New().String()[:8])
			yaml1 := integrityTestYAMLWithVersion(testID, "Version A of workflow", "1.0.0")
			yaml2 := integrityTestYAMLWithVersion(testID, "Version B of workflow", "2.0.0")
			yaml3 := integrityTestYAMLWithVersion(testID, "Version C of workflow", "3.0.0")

			httpServer, srv := createIntegrityTestServer(yaml1)
			defer httpServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			wrA := registerIntegrityWorkflow(httpServer.URL, yaml1)
			Expect(wrA.StatusCode).To(Equal(http.StatusCreated))

			wrB := registerIntegrityWorkflow(httpServer.URL, yaml2)
			Expect(wrB.StatusCode).To(Equal(http.StatusCreated), "v2 supersedes v1")

			wrC := registerIntegrityWorkflow(httpServer.URL, yaml3)
			Expect(wrC.StatusCode).To(Equal(http.StatusCreated), "v3 supersedes v2")

			Eventually(func() int {
				return countWorkflowsByName(testID)
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(3),
				"Should have 3 records: v1(superseded) + v2(superseded) + v3(active)")

			var activeCount int
			err := db.QueryRow(
				"SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_name = $1 AND status = 'Active'", testID,
			).Scan(&activeCount)
			Expect(err).ToNot(HaveOccurred())
			Expect(activeCount).To(Equal(1), "Exactly one active record should exist")
		})
	})

	// ========================================
	// IT-DS-INTEGRITY-008: Unique constraint as safety net
	// ========================================
	Describe("IT-DS-INTEGRITY-008: DB constraint prevents duplicate active workflows", func() {
		It("should not allow two active workflows with same name+version", func() {
			testID := fmt.Sprintf("integrity-constraint-%s", uuid.New().String()[:8])

			// Direct DB insert: create first active workflow
			wf1 := &models.RemediationWorkflow{
				WorkflowName:     testID,
				Version:          "1.0.0",
				SchemaVersion:    "1.0",
				Name:             testID,
				Status:           "Active",
				IsLatestVersion:  true,
				Content:          "content-1",
				ContentHash:      "hash-1",
				ActionType:       "IncreaseMemoryLimits",
				ExecutionEngine:  "job",
			}

			err := workflowRepo.Create(ctx, wf1)
			Expect(err).ToNot(HaveOccurred(), "First create should succeed")

			wf2 := &models.RemediationWorkflow{
				WorkflowName:     testID,
				Version:          "1.0.0",
				SchemaVersion:    "1.0",
				Name:             testID,
				Status:           "Active",
				IsLatestVersion:  true,
				Content:          "content-2",
				ContentHash:      "hash-2",
				ActionType:       "IncreaseMemoryLimits",
				ExecutionEngine:  "job",
			}

			err = workflowRepo.Create(ctx, wf2)
			Expect(err).To(HaveOccurred(),
				"Second create with same name+version should fail due to DB constraint")
		})
	})
})
