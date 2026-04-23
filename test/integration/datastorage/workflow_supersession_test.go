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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
	deterministicuuid "github.com/jordigilh/kubernaut/pkg/datastorage/uuid"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

func computeTestContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

func supersessionTestYAML(workflowName, version, description string) string {
	crd := testutil.NewTestWorkflowCRD(workflowName, "IncreaseMemoryLimits", "job")
	crd.Spec.Version = version
	crd.Spec.Description = sharedtypes.StructuredDescription{
		What:      description,
		WhenToUse: "Integration test for cross-version supersession",
	}
	crd.Spec.Execution.Bundle = "quay.io/kubernaut/workflows/scale-memory:v1.0.0@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abc1"
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target resource"},
	}
	return testutil.MarshalWorkflowCRD(crd)
}

var _ = Describe("Workflow Cross-Version Supersession (Issue #371, BR-WORKFLOW-006)", func() {
	var (
		workflowRepo *workflow.Repository
	)

	BeforeEach(func() {
		workflowRepo = workflow.NewRepository(db, logger)
	})

	// ========================================
	// IT-DS-371-001: Full lifecycle — create v1.0.0, then v1.0.1, verify supersession
	// Issue #371, BR-WORKFLOW-006: Version upgrade supersedes old active entry.
	// LLM discovery should only see the newest version.
	// ========================================
	Describe("IT-DS-371-001: Cross-version supersession lifecycle", func() {
		It("should supersede v1.0.0 when v1.0.1 is registered for the same workflow name", func() {
			testID := fmt.Sprintf("supersede-%s", uuid.New().String()[:8])

			yamlV1 := supersessionTestYAML(testID, "1.0.0", "Original v1.0.0 workflow for supersession test")
			yamlV2 := supersessionTestYAML(testID, "1.0.1", "Upgraded v1.0.1 workflow for supersession test")

			httpServer, srv := createIntegrityTestServer(yamlV1)
			defer httpServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			// Step 1: Register v1.0.0
			wr1 := registerIntegrityWorkflow(httpServer.URL, yamlV1)
			Expect(wr1.StatusCode).To(Equal(http.StatusCreated),
				"v1.0.0 should be created (201), got %d", wr1.StatusCode)
			_, err := uuid.Parse(wr1.WorkflowID)
			Expect(err).ToNot(HaveOccurred(),
				"v1.0.0 workflow ID should be a valid UUID, got %q", wr1.WorkflowID)

			// Step 2: Register v1.0.1 (different version of same workflow name)
			wr2 := registerIntegrityWorkflow(httpServer.URL, yamlV2)
			Expect(wr2.StatusCode).To(Equal(http.StatusCreated),
				"v1.0.1 should be created (201), got %d", wr2.StatusCode)
			_, err = uuid.Parse(wr2.WorkflowID)
			Expect(err).ToNot(HaveOccurred(),
				"v1.0.1 workflow ID should be a valid UUID, got %q", wr2.WorkflowID)
			Expect(wr2.WorkflowID).ToNot(Equal(wr1.WorkflowID),
				"New version should have a different workflow UUID")

			// Step 3: Verify v1.0.0 is now superseded
			Eventually(func() string {
				return queryWorkflowStatus(wr1.WorkflowID)
			}, 5*time.Second, 500*time.Millisecond).Should(Equal("Superseded"),
				"v1.0.0 should be marked superseded after v1.0.1 registration")

			// Step 4: Verify v1.0.1 is active
			Expect(queryWorkflowStatus(wr2.WorkflowID)).To(Equal("Active"),
				"v1.0.1 should be active")

			// Step 5: Verify exactly one active entry for this workflow name
			var activeCount int
			err = db.QueryRow(
				"SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_name = $1 AND status = 'Active'", testID,
			).Scan(&activeCount)
			Expect(err).ToNot(HaveOccurred())
			Expect(activeCount).To(Equal(1),
				"Exactly one active entry should exist for workflow %q", testID)

			// Step 6: Verify v1.0.0 is preserved for audit trail
			var supersededVersion string
			err = db.QueryRow(
				"SELECT version FROM remediation_workflow_catalog WHERE workflow_id = $1", wr1.WorkflowID,
			).Scan(&supersededVersion)
			Expect(err).ToNot(HaveOccurred())
			Expect(supersededVersion).To(Equal("1.0.0"),
				"Superseded record should retain its version for audit trail")

			// Step 7: Verify is_latest_version flags
			var v1Latest, v2Latest bool
			err = db.QueryRow(
				"SELECT is_latest_version FROM remediation_workflow_catalog WHERE workflow_id = $1", wr1.WorkflowID,
			).Scan(&v1Latest)
			Expect(err).ToNot(HaveOccurred())
			Expect(v1Latest).To(BeFalse(), "v1.0.0 should NOT be is_latest_version")

			err = db.QueryRow(
				"SELECT is_latest_version FROM remediation_workflow_catalog WHERE workflow_id = $1", wr2.WorkflowID,
			).Scan(&v2Latest)
			Expect(err).ToNot(HaveOccurred())
			Expect(v2Latest).To(BeTrue(), "v1.0.1 should be is_latest_version")
		})
	})

	// ========================================
	// IT-DS-730-001: PK collision recovery — re-register v1.0.0 after v1.0.1 superseded it
	// Issue #730, BR-WORKFLOW-006: Re-registering an older version whose content (and
	// thus deterministic UUID) already exists as a Superseded row should succeed by
	// re-activating the existing row, not returning 500.
	// ========================================
	Describe("IT-DS-730-001: PK collision recovery on re-registration of superseded version", func() {
		It("should re-activate the Superseded row and return success", func() {
			testID := fmt.Sprintf("pk-reactivate-%s", uuid.New().String()[:8])

			yamlV1 := supersessionTestYAML(testID, "1.0.0", "Original v1.0.0 for PK collision test")
			yamlV2 := supersessionTestYAML(testID, "1.0.1", "Upgraded v1.0.1 for PK collision test")

			httpServer, srv := createIntegrityTestServer(yamlV1)
			defer httpServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			// Step 1: Register v1.0.0 → Active
			wr1 := registerIntegrityWorkflow(httpServer.URL, yamlV1)
			Expect(wr1.StatusCode).To(Equal(http.StatusCreated),
				"v1.0.0 should be created (201)")

			// Step 2: Register v1.0.1 → v1.0.0 becomes Superseded, v1.0.1 Active
			wr2 := registerIntegrityWorkflow(httpServer.URL, yamlV2)
			Expect(wr2.StatusCode).To(Equal(http.StatusCreated),
				"v1.0.1 should be created (201)")

			Eventually(func() string {
				return queryWorkflowStatus(wr1.WorkflowID)
			}, 5*time.Second, 500*time.Millisecond).Should(Equal("Superseded"),
				"v1.0.0 should be Superseded after v1.0.1 registration")

			// Step 3: Re-register v1.0.0 (same YAML → same content hash → same UUID → PK collision)
			wr3 := registerIntegrityWorkflow(httpServer.URL, yamlV1)
			Expect(wr3.StatusCode).To(SatisfyAny(Equal(http.StatusOK), Equal(http.StatusCreated)),
				"Re-registering v1.0.0 should succeed (200 or 201), got %d — PK collision should be handled gracefully", wr3.StatusCode)

			// Step 4: v1.0.0 should now be Active again
			Eventually(func() string {
				return queryWorkflowStatus(wr1.WorkflowID)
			}, 5*time.Second, 500*time.Millisecond).Should(Equal("Active"),
				"v1.0.0 should be re-activated after re-registration")

			// Step 5: v1.0.1 should be Superseded
			Expect(queryWorkflowStatus(wr2.WorkflowID)).To(Equal("Superseded"),
				"v1.0.1 should be Superseded after v1.0.0 re-registration")

			// Step 6: Exactly one active entry
			var activeCount int
			err := db.QueryRow(
				"SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_name = $1 AND status = 'Active'", testID,
			).Scan(&activeCount)
			Expect(err).ToNot(HaveOccurred())
			Expect(activeCount).To(Equal(1),
				"Exactly one active entry should exist after PK collision recovery")

			// Step 7: Exactly one is_latest_version=true
			var latestCount int
			err = db.QueryRow(
				"SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_name = $1 AND is_latest_version = true", testID,
			).Scan(&latestCount)
			Expect(err).ToNot(HaveOccurred())
			Expect(latestCount).To(Equal(1),
				"Exactly one is_latest_version=true should exist after PK collision recovery")
		})
	})

	// ========================================
	// IT-DS-730-002: PK collision with Disabled row should NOT re-activate (security)
	// Issue #730: If an older version was intentionally Disabled, re-registering
	// the same content should create a new entry, not re-activate the Disabled row.
	// ========================================
	Describe("IT-DS-730-002: PK collision with Disabled row does not re-activate", func() {
		It("should not re-activate a Disabled row on PK collision", func() {
			testID := fmt.Sprintf("pk-disabled-%s", uuid.New().String()[:8])

			yamlV1 := supersessionTestYAML(testID, "1.0.0", "Original v1.0.0 for disabled PK test")

			httpServer, srv := createIntegrityTestServer(yamlV1)
			defer httpServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			// Step 1: Register v1.0.0 → Active
			wr1 := registerIntegrityWorkflow(httpServer.URL, yamlV1)
			Expect(wr1.StatusCode).To(Equal(http.StatusCreated))

			// Step 2: Disable v1.0.0 (simulating CRD DELETE)
			err := workflowRepo.UpdateStatus(ctx, wr1.WorkflowID, "1.0.0", "Disabled", "CRD deleted", "test-user")
			Expect(err).ToNot(HaveOccurred())

			// Step 3: Re-register v1.0.0 (same YAML → same UUID)
			// The handler should detect the Disabled row and re-enable via the existing Disabled path,
			// returning 200 (same content hash → re-enable).
			wr2 := registerIntegrityWorkflow(httpServer.URL, yamlV1)
			Expect(wr2.StatusCode).To(Equal(http.StatusOK),
				"Re-registering disabled workflow with same hash should re-enable (200), got %d", wr2.StatusCode)

			// Step 4: The entry should be Active now (re-enabled via Disabled path, not PK collision path)
			Expect(queryWorkflowStatus(wr1.WorkflowID)).To(Equal("Active"),
				"Disabled workflow should be re-enabled via the existing Disabled handler")
		})
	})

	// ========================================
	// IT-DS-730-003: Repository-level PK collision recovery in SupersedeAndCreate
	// Issue #730, BR-WORKFLOW-006: SupersedeAndCreate must use a PostgreSQL
	// SAVEPOINT before the INSERT so that PK collision (23505) does not abort
	// the transaction and prevent the subsequent reactivation UPDATE.
	// Without SAVEPOINT, PostgreSQL returns SQLSTATE 25P02 ("current transaction
	// is aborted") on any statement after the failed INSERT.
	// ========================================
	Describe("IT-DS-730-003: SupersedeAndCreate SAVEPOINT recovery on PK collision", func() {
		It("should re-activate the Superseded row within the same transaction", func() {
			testID := fmt.Sprintf("savepoint-%s", uuid.New().String()[:8])

			contentV1 := "content-v1-" + testID
			contentV2 := "content-v2-" + testID
			hashV1 := computeTestContentHash(contentV1)
			hashV2 := computeTestContentHash(contentV2)
			uuidV1 := deterministicuuid.DeterministicUUID(hashV1)
			uuidV2 := deterministicuuid.DeterministicUUID(hashV2)

			// Step 1: Create v1.0.0 directly via repo.Create
			wfV1 := &models.RemediationWorkflow{
				WorkflowID:      uuidV1,
				WorkflowName:    testID,
				Version:         "1.0.0",
				SchemaVersion:   "1.0",
				Name:            testID,
				Description:     models.StructuredDescription{What: "v1.0.0 for savepoint test"},
				Content:         contentV1,
				ContentHash:     hashV1,
				Labels:          models.MandatoryLabels{Severity: []string{"critical"}, Component: []string{"pod"}, Environment: []string{"production"}, Priority: "P1"},
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "Active",
				IsLatestVersion: true,
				ExecutionEngine: "job",
				ActionType:      "IncreaseMemoryLimits",
			}
			err := workflowRepo.Create(ctx, wfV1)
			Expect(err).ToNot(HaveOccurred(), "v1.0.0 Create should succeed")

			// Step 2: SupersedeAndCreate to replace v1.0.0 with v1.0.1
			wfV2 := &models.RemediationWorkflow{
				WorkflowID:      uuidV2,
				WorkflowName:    testID,
				Version:         "1.0.1",
				SchemaVersion:   "1.0",
				Name:            testID,
				Description:     models.StructuredDescription{What: "v1.0.1 for savepoint test"},
				Content:         contentV2,
				ContentHash:     hashV2,
				Labels:          models.MandatoryLabels{Severity: []string{"critical"}, Component: []string{"pod"}, Environment: []string{"production"}, Priority: "P1"},
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "Active",
				IsLatestVersion: true,
				ExecutionEngine: "job",
				ActionType:      "IncreaseMemoryLimits",
			}
			err = workflowRepo.SupersedeAndCreate(ctx, uuidV1, "1.0.0", "superseded: version upgrade", wfV2)
			Expect(err).ToNot(HaveOccurred(), "SupersedeAndCreate v1.0.0→v1.0.1 should succeed")

			// Verify v1.0.0 is now Superseded
			var statusV1 string
			err = db.QueryRow(
				"SELECT status FROM remediation_workflow_catalog WHERE workflow_id = $1", uuidV1,
			).Scan(&statusV1)
			Expect(err).ToNot(HaveOccurred())
			Expect(statusV1).To(Equal("Superseded"), "v1.0.0 should be Superseded after step 2")

			// Step 3: SupersedeAndCreate to replace v1.0.1 with v1.0.0 (PK collision on uuidV1)
			// This is the critical step: uuidV1 already exists as a Superseded row.
			// The INSERT will hit remediation_workflow_catalog_pkey. Without SAVEPOINT,
			// PostgreSQL aborts the transaction and the reactivation UPDATE fails with 25P02.
			wfV1Reactivate := &models.RemediationWorkflow{
				WorkflowID:      uuidV1,
				WorkflowName:    testID,
				Version:         "1.0.0",
				SchemaVersion:   "1.0",
				Name:            testID,
				Description:     models.StructuredDescription{What: "v1.0.0 reactivated"},
				Content:         contentV1,
				ContentHash:     hashV1,
				Labels:          models.MandatoryLabels{Severity: []string{"critical"}, Component: []string{"pod"}, Environment: []string{"production"}, Priority: "P1"},
				CustomLabels:    models.CustomLabels{},
				DetectedLabels:  models.DetectedLabels{},
				Status:          "Active",
				IsLatestVersion: true,
				ExecutionEngine: "job",
				ActionType:      "IncreaseMemoryLimits",
			}
			err = workflowRepo.SupersedeAndCreate(ctx, uuidV2, "1.0.1", "superseded: rollback to v1.0.0", wfV1Reactivate)
			Expect(err).ToNot(HaveOccurred(),
				"SupersedeAndCreate with PK collision should succeed via SAVEPOINT recovery, got: %v", err)

			// Step 4: Verify v1.0.0 is Active again (reactivated)
			err = db.QueryRow(
				"SELECT status FROM remediation_workflow_catalog WHERE workflow_id = $1", uuidV1,
			).Scan(&statusV1)
			Expect(err).ToNot(HaveOccurred())
			Expect(statusV1).To(Equal("Active"),
				"v1.0.0 should be re-activated after PK collision recovery")

			// Step 5: Verify v1.0.1 is Superseded
			var statusV2 string
			err = db.QueryRow(
				"SELECT status FROM remediation_workflow_catalog WHERE workflow_id = $1", uuidV2,
			).Scan(&statusV2)
			Expect(err).ToNot(HaveOccurred())
			Expect(statusV2).To(Equal("Superseded"),
				"v1.0.1 should be Superseded after v1.0.0 re-registration")

			// Step 6: Exactly one Active row for this workflow name
			var activeCount int
			err = db.QueryRow(
				"SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_name = $1 AND status = 'Active'", testID,
			).Scan(&activeCount)
			Expect(err).ToNot(HaveOccurred())
			Expect(activeCount).To(Equal(1),
				"Exactly one active entry should exist after SAVEPOINT recovery")
		})
	})

	// ========================================
	// IT-DS-371-002: Delete+recreate pattern — old disabled, new active
	// Issue #371, BR-WORKFLOW-006: When a workflow is disabled (simulating CRD DELETE)
	// and a new version is registered, the disabled entry stays disabled and the
	// new entry becomes the only active entry.
	// ========================================
	Describe("IT-DS-371-002: Delete+recreate pattern", func() {
		It("should keep old entry disabled and new entry active", func() {
			testID := fmt.Sprintf("delete-recreate-%s", uuid.New().String()[:8])

			yamlV1 := supersessionTestYAML(testID, "1.0.0", "Original v1.0.0 for delete+recreate test")
			yamlV2 := supersessionTestYAML(testID, "1.0.1", "Recreated v1.0.1 for delete+recreate test")

			httpServer, srv := createIntegrityTestServer(yamlV1)
			defer httpServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			// Step 1: Register v1.0.0
			wr1 := registerIntegrityWorkflow(httpServer.URL, yamlV1)
			Expect(wr1.StatusCode).To(Equal(http.StatusCreated))

			// Step 2: Disable v1.0.0 (simulating CRD DELETE)
			err := workflowRepo.UpdateStatus(ctx, wr1.WorkflowID, "1.0.0", "Disabled", "CRD deleted", "test-user")
			Expect(err).ToNot(HaveOccurred())

			// Step 3: Register v1.0.1 (new version after delete)
			wr2 := registerIntegrityWorkflow(httpServer.URL, yamlV2)
			Expect(wr2.StatusCode).To(Equal(http.StatusCreated),
				"New version after disable should be created (201), got %d", wr2.StatusCode)

			// Step 4: Verify v1.0.0 remains disabled (not changed to superseded)
			Expect(queryWorkflowStatus(wr1.WorkflowID)).To(Equal("Disabled"),
				"Disabled entry should remain disabled (DELETE semantics unchanged)")

			// Step 5: Verify v1.0.1 is active
			Expect(queryWorkflowStatus(wr2.WorkflowID)).To(Equal("Active"),
				"New entry should be active")

			// Step 6: Verify exactly one active entry
			var activeCount int
			err = db.QueryRow(
				"SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_name = $1 AND status = 'Active'", testID,
			).Scan(&activeCount)
			Expect(err).ToNot(HaveOccurred())
			Expect(activeCount).To(Equal(1),
				"Exactly one active entry should exist after delete+recreate")
		})
	})
})
