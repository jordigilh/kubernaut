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
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

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
			}, 5*time.Second, 500*time.Millisecond).Should(Equal("superseded"),
				"v1.0.0 should be marked superseded after v1.0.1 registration")

			// Step 4: Verify v1.0.1 is active
			Expect(queryWorkflowStatus(wr2.WorkflowID)).To(Equal("active"),
				"v1.0.1 should be active")

			// Step 5: Verify exactly one active entry for this workflow name
			var activeCount int
			err = db.QueryRow(
				"SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_name = $1 AND status = 'active'", testID,
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
			err := workflowRepo.UpdateStatus(ctx, wr1.WorkflowID, "1.0.0", "disabled", "CRD deleted", "test-user")
			Expect(err).ToNot(HaveOccurred())

			// Step 3: Register v1.0.1 (new version after delete)
			wr2 := registerIntegrityWorkflow(httpServer.URL, yamlV2)
			Expect(wr2.StatusCode).To(Equal(http.StatusCreated),
				"New version after disable should be created (201), got %d", wr2.StatusCode)

			// Step 4: Verify v1.0.0 remains disabled (not changed to superseded)
			Expect(queryWorkflowStatus(wr1.WorkflowID)).To(Equal("disabled"),
				"Disabled entry should remain disabled (DELETE semantics unchanged)")

			// Step 5: Verify v1.0.1 is active
			Expect(queryWorkflowStatus(wr2.WorkflowID)).To(Equal("active"),
				"New entry should be active")

			// Step 6: Verify exactly one active entry
			var activeCount int
			err = db.QueryRow(
				"SELECT COUNT(*) FROM remediation_workflow_catalog WHERE workflow_name = $1 AND status = 'active'", testID,
			).Scan(&activeCount)
			Expect(err).ToNot(HaveOccurred())
			Expect(activeCount).To(Equal(1),
				"Exactly one active entry should exist after delete+recreate")
		})
	})
})
